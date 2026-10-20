#!/usr/bin/env python3
"""Control my JBL Bar 9.1 through its existing local Go remote API."""
import argparse
import ipaddress
import http.client
import json
import socket
import sys
import urllib.error
import urllib.parse
import urllib.request

PRIVATE_NETWORKS = tuple(ipaddress.IPv4Network(n) for n in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"))
ACTIONS = {
    "volumeup": "volume_up", "volumedown": "volume_down", "mute": "mute", "power": "power",
    "play": "media_play", "pause": "media_pause", "next": "media_next",
    "previous": "media_previous", "stop": "media_stop",
}
SOURCES = {"tv": "input_tv", "hdmi": "input_hdmi", "bluetooth": "input_bluetooth"}
MAX_RESPONSE = 131072


class CLIError(Exception):
    """An actionable owner-facing error without request headers or raw bodies."""


def host_url(value):
    """Accept one local host, never URL credentials, alternate paths or schemes."""
    if not isinstance(value, str) or any(c.isspace() or ord(c) < 32 for c in value):
        raise argparse.ArgumentTypeError("Use jbl.local or a private IPv4 address, optionally with :port")
    candidate = value if "://" in value else "http://" + value
    try:
        url = urllib.parse.urlsplit(candidate)
        port = url.port
        host = url.hostname
    except ValueError:
        raise argparse.ArgumentTypeError("Invalid local soundbar address") from None
    if url.scheme != "http" or not url.netloc or url.path not in ("", "/") or "?" in candidate or "#" in candidate or url.username is not None or url.password is not None:
        raise argparse.ArgumentTypeError("Use an HTTP host without credentials, paths, query or fragment")
    if host != "jbl.local":
        try:
            address = ipaddress.IPv4Address(host or "")
        except ipaddress.AddressValueError:
            raise argparse.ArgumentTypeError("Use jbl.local or a private IPv4 address") from None
        if not any(address in network for network in PRIVATE_NETWORKS):
            raise argparse.ArgumentTypeError("The soundbar address must be in a private IPv4 network")
        host = str(address)
    if port is not None and not 1 <= port <= 65535:
        raise argparse.ArgumentTypeError("Port must be between 1 and 65535")
    # Reject empty ports and URL syntax urllib would otherwise normalize away.
    if url.netloc.endswith(":") or "\\" in url.netloc:
        raise argparse.ArgumentTypeError("Invalid local soundbar address")
    return "http://" + host + (":" + str(port) if port is not None else "")


def bounded_timeout(value):
    try:
        timeout = float(value)
    except (ValueError, TypeError):
        raise argparse.ArgumentTypeError("Timeout must be between 1 and 30 seconds") from None
    if not 1 <= timeout <= 30:
        raise argparse.ArgumentTypeError("Timeout must be between 1 and 30 seconds")
    return timeout


class NoRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, new_url):
        return None


def safe_text(value, csrf=""):
    if not isinstance(value, str):
        return ""
    if csrf:
        value = value.replace(csrf, "[redacted]")
    value = "".join(c if c.isprintable() else " " for c in value)
    return " ".join(value.split())[:240]


def public_state(value):
    """Strip CSRF tokens from every output depth, including error-state shapes."""
    if isinstance(value, dict):
        return {k: public_state(v) for k, v in value.items() if k.lower() not in ("csrf", "x-csrf-token")}
    if isinstance(value, list):
        return [public_state(v) for v in value]
    return value


class Soundbar:
    def __init__(self, host="http://jbl.local", timeout=20, opener=None):
        self.origin = host_url(host)
        self.timeout = bounded_timeout(timeout)
        # LAN controls should not pass through environment-configured proxies.
        self.opener = opener or urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirects())
        self.csrf = ""

    def request(self, path, action=None):
        if path not in ("/api/state", "/api/action") or (path == "/api/action") != (action is not None):
            raise CLIError("Invalid local control request")
        headers = {"Accept": "application/json", "User-Agent": "jbl-bar-9.1-cli"}
        data = None
        if action is not None:
            allowed = set(ACTIONS.values()) | set(SOURCES.values()) | {"standard", "night_toggle"}
            if action not in allowed:
                raise CLIError("Unsupported control")
            if not self.csrf:
                raise CLIError("Refresh soundbar status before using controls")
            data = json.dumps({"action": action}, separators=(",", ":")).encode()
            headers.update({"Content-Type": "application/json", "X-CSRF-Token": self.csrf, "Origin": self.origin})
        request = urllib.request.Request(self.origin + path, data=data, headers=headers, method="POST" if data is not None else "GET")
        try:
            with self.opener.open(request, timeout=self.timeout) as response:
                if not 200 <= response.status < 300:
                    raise CLIError("Soundbar returned HTTP " + str(response.status))
                raw = response.read(MAX_RESPONSE + 1)
        except urllib.error.HTTPError as error:
            message = ""
            try:
                body = error.read(MAX_RESPONSE + 1)
                if len(body) <= MAX_RESPONSE:
                    parsed = json.loads(body)
                    if isinstance(parsed, dict):
                        message = safe_text(parsed.get("error"), self.csrf)
            except (ValueError, OSError, UnicodeError):
                pass
            finally:
                error.close()
            suffix = ": " + message if message else ""
            raise CLIError("Soundbar returned HTTP " + str(error.code) + suffix) from None
        except (TimeoutError, socket.timeout):
            raise CLIError("Request timed out; check status before trying again. The command was not retried.") from None
        except (urllib.error.URLError, OSError, http.client.HTTPException):
            raise CLIError("Cannot reach the soundbar. Check Wi-Fi and the local address; the command was not retried.") from None
        if len(raw) > MAX_RESPONSE:
            raise CLIError("Soundbar response was too large")
        try:
            state = json.loads(raw)
        except (ValueError, UnicodeError):
            raise CLIError("Soundbar returned invalid JSON") from None
        if not isinstance(state, dict):
            raise CLIError("Soundbar returned an invalid status object")
        token = state.get("csrf")
        if isinstance(token, str) and token and len(token) <= 256 and all(32 < ord(c) < 127 for c in token):
            self.csrf = token
        return state

    def command(self, command, value=None):
        if command not in set(ACTIONS) | {"status", "volume", "source", "night", "smart"}:
            raise CLIError("Unsupported control")
        state = self.request("/api/state")
        if command == "status":
            return state, False
        if state.get("available") is not True:
            detail = safe_text(state.get("error"), self.csrf) or "device status is unavailable"
            raise CLIError("Controls unavailable: " + detail)
        if command == "night":
            if value not in ("on", "off"):
                raise CLIError("Choose night on or night off")
            sound = state.get("sound")
            current = sound.get("night") if isinstance(sound, dict) else None
            if not isinstance(current, bool):
                raise CLIError("Night mode status is unavailable; no toggle was sent")
            if current == (value == "on"):
                return state, False
            action = "night_toggle"
        elif command == "smart":
            if value != "off":
                raise CLIError("Only smart off is supported")
            if state.get("mode") == "Standard":
                return state, False
            if state.get("mode") != "Smart":
                raise CLIError("Smart mode status is unavailable; no control was sent")
            action = "standard"
        elif command == "source":
            if value not in SOURCES:
                raise CLIError("Choose source tv, hdmi or bluetooth")
            action = SOURCES[value]
        elif command == "volume":
            if value not in ("up", "down"):
                raise CLIError("Choose volume up or volume down")
            action = "volume_" + value
        else:
            action = ACTIONS[command]
        if action.startswith("media_"):
            controls = state.get("media_controls")
            if not isinstance(controls, list) or action not in controls:
                raise CLIError("This source does not currently support " + command + "; no command was sent")
        return self.request("/api/action", action), True


def format_state(state):
    if state.get("available") is not True:
        return "Soundbar unavailable: " + (safe_text(state.get("error")) or "status unknown")
    volume = state.get("volume")
    volume = str(volume) if isinstance(volume, int) and not isinstance(volume, bool) else "unknown"
    muted = state.get("muted")
    muted = "yes" if muted is True else "no" if muted is False else "unknown"
    lines = [" | ".join(("Power: " + (safe_text(state.get("power")) or "unknown"), "Source: " + (safe_text(state.get("input")) or "unknown"), "Volume: " + volume, "Muted: " + muted))]
    sound = state.get("sound")
    night = sound.get("night") if isinstance(sound, dict) else None
    night = "on" if night is True else "off" if night is False else "unknown"
    lines.append("Mode: " + (safe_text(state.get("mode")) or "unknown") + " | Night: " + night)
    bluetooth = state.get("bluetooth")
    if isinstance(bluetooth, dict) and bluetooth.get("selected") is True:
        peer = safe_text(bluetooth.get("peer_name")) if bluetooth.get("connected") is True else ""
        status = safe_text(bluetooth.get("audio")) or "unknown"
        lines.append("Bluetooth: " + status + (" | Peer: " + peer if peer else ""))
    streaming = state.get("streaming")
    if isinstance(streaming, dict):
        service, status = safe_text(streaming.get("service")), safe_text(streaming.get("status"))
        if service or status:
            lines.append("Playback: " + " — ".join(v for v in (service, status) if v))
        cast = streaming.get("cast")
        if isinstance(cast, dict) and safe_text(cast.get("title")):
            track = " — ".join(v for v in (safe_text(cast.get("title")), safe_text(cast.get("artist"))) if v)
            lines.append("Track: " + track)
    return "\n".join(lines)


def parser():
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--host", type=host_url, default=argparse.SUPPRESS, help="jbl.local or private IPv4[:port] (optional http:// prefix)")
    common.add_argument("--timeout", type=bounded_timeout, default=argparse.SUPPRESS, help="request timeout, 1–30 seconds (default: 20)")
    common.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="print state as JSON without CSRF tokens")
    root = argparse.ArgumentParser(description=__doc__, parents=[common])
    commands = root.add_subparsers(dest="command", required=True)
    for name in ("status", *ACTIONS):
        commands.add_parser(name, parents=[common])
    for name, choices in (("volume", ("up", "down")), ("source", tuple(SOURCES)), ("night", ("on", "off")), ("smart", ("off",))):
        sub = commands.add_parser(name, parents=[common])
        sub.add_argument("value", choices=choices)
    return root


def main(argv=None):
    args = parser().parse_args(argv)
    try:
        client = Soundbar(getattr(args, "host", "http://jbl.local"), getattr(args, "timeout", 20))
        state, changed = client.command(args.command, getattr(args, "value", None))
        state = public_state(state)
        if getattr(args, "json", False):
            print(json.dumps(state, ensure_ascii=False, indent=2))
        else:
            if args.command != "status" and not changed:
                print("Already set; no command sent.")
            print(format_state(state))
        return 0
    except CLIError as error:
        print("Error: " + str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
