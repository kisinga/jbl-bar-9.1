"""Local CLI API boundaries and idempotency; no soundbar or vendor tools."""
from pathlib import Path
import argparse
import contextlib
import copy
import importlib.util
import io
import json
import socket
import threading
import unittest
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("jbl_cli", ROOT / "tools/jbl.py")
cli = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cli)

STATE = {
    "available": True, "power": "On", "input": "Bluetooth", "mode": "Smart", "volume": 13,
    "muted": False, "sound": {"night": False}, "csrf": "fixture-private-csrf",
    "bluetooth": {"selected": True, "connected": True, "audio": "Streaming", "peer_name": "Fixture phone"},
    "media_controls": ["media_play", "media_pause", "media_next", "media_previous", "media_stop"],
}


class FixtureHandler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def handle_request(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self.server.requests.append((self.command, self.path, dict(self.headers), body))
        status, value = self.server.reply(self.command, self.path, body)
        self.send_response(status)
        if status == 302:
            self.send_header("Location", "http://8.8.8.8/should-not-be-requested")
        self.end_headers()
        self.wfile.write(value if isinstance(value, bytes) else json.dumps(value).encode())

    do_GET = handle_request
    do_POST = handle_request


class LocalFixtureTransport:
    """Map the validated private host to an ephemeral loopback HTTP fixture."""
    def __init__(self, server):
        self.server = server
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), cli.NoRedirects())

    def open(self, request, timeout):
        path = urllib.parse.urlsplit(request.full_url).path
        local = "http://127.0.0.1:" + str(self.server.server_port) + path
        replacement = urllib.request.Request(local, data=request.data, headers=dict(request.header_items()), method=request.method)
        return self.opener.open(replacement, timeout=timeout)


class CLITests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), FixtureHandler)
        cls.worker = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.worker.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.worker.join()

    def setUp(self):
        self.server.requests = []
        self.state = copy.deepcopy(STATE)
        self.server.reply = lambda method, path, body: (200, self.state)
        self.client = cli.Soundbar("192.168.50.8:8080", opener=LocalFixtureTransport(self.server))

    def test_host_validation(self):
        for value, want in (("jbl.local", "http://jbl.local"), ("http://JBL.LOCAL/", "http://jbl.local"), ("10.4.5.6:80", "http://10.4.5.6:80"), ("172.16.0.9:8080", "http://172.16.0.9:8080"), ("192.168.255.249", "http://192.168.255.249")):
            self.assertEqual(cli.host_url(value), want)
        for value in ("8.8.8.8", "127.0.0.1", "0.0.0.0", "169.254.1.2", "192.0.2.1", "172.32.0.1", "::1", "localhost", "example.com", "https://jbl.local", "http://user:secret@jbl.local", "http://jbl.local/api/action", "jbl.local?", "jbl.local#", "jbl.local?x=y", "jbl.local#x", "jbl.local:", "jbl.local:0", "jbl.local:65536", "jbl.local\\other", "jbl.local\n", "10.0.0.1:bad", "[::1]"):
            with self.subTest(value=value), self.assertRaises(argparse.ArgumentTypeError):
                cli.host_url(value)

    def test_timeout_bounds(self):
        self.assertEqual(cli.bounded_timeout("1"), 1)
        self.assertEqual(cli.bounded_timeout("30"), 30)
        for value in ("0", "31", "nan", "inf", "bad"):
            with self.assertRaises(argparse.ArgumentTypeError):
                cli.bounded_timeout(value)

    def test_parser_global_options_before_or_after_command(self):
        for args in (("--json", "--host", "10.0.0.8", "volume", "up"), ("volume", "up", "--json", "--host", "10.0.0.8")):
            parsed = cli.parser().parse_args(args)
            self.assertTrue(parsed.json)
            self.assertEqual(parsed.host, "http://10.0.0.8")
            self.assertEqual((parsed.command, parsed.value), ("volume", "up"))

    def test_status_is_get_only(self):
        state, changed = self.client.command("status")
        self.assertFalse(changed)
        self.assertEqual(state["volume"], 13)
        self.assertEqual([(r[0], r[1]) for r in self.server.requests], [("GET", "/api/state")])

    def test_all_controls_use_fixed_api_and_csrf(self):
        commands = [(name, None, action) for name, action in cli.ACTIONS.items()]
        commands += [("volume", "up", "volume_up"), ("volume", "down", "volume_down")]
        commands += [("source", name, action) for name, action in cli.SOURCES.items()]
        commands += [("night", "on", "night_toggle"), ("smart", "off", "standard")]
        for command, value, action in commands:
            self.server.requests = []
            self.client.command(command, value)
            self.assertEqual([(r[0], r[1]) for r in self.server.requests], [("GET", "/api/state"), ("POST", "/api/action")])
            headers = {k.lower(): v for k, v in self.server.requests[1][2].items()}
            self.assertEqual(headers["x-csrf-token"], STATE["csrf"])
            self.assertEqual(headers["origin"], "http://192.168.50.8:8080")
            self.assertEqual(headers["content-type"], "application/json")
            self.assertEqual(json.loads(self.server.requests[1][3]), {"action": action})

    def test_night_on_and_off_are_idempotent(self):
        for current, desired in ((True, "on"), (False, "off")):
            self.state["sound"]["night"] = current
            self.server.requests = []
            _, changed = self.client.command("night", desired)
            self.assertFalse(changed)
            self.assertEqual(len(self.server.requests), 1)
        self.state["sound"]["night"] = True
        self.server.requests = []
        _, changed = self.client.command("night", "off")
        self.assertTrue(changed)
        self.assertEqual(json.loads(self.server.requests[-1][3]), {"action": "night_toggle"})

    def test_unknown_night_is_not_treated_as_off(self):
        for sound in ({}, {"night": None}, {"night": 0}, "invalid", None):
            self.state["sound"] = sound
            self.server.requests = []
            with self.assertRaisesRegex(cli.CLIError, "status is unavailable"):
                self.client.command("night", "on")
            self.assertEqual(len(self.server.requests), 1)

    def test_smart_off_does_not_toggle_standard(self):
        self.state["mode"] = "Standard"
        _, changed = self.client.command("smart", "off")
        self.assertFalse(changed)
        self.assertEqual(len(self.server.requests), 1)
        self.state["mode"] = "Unknown"
        with self.assertRaisesRegex(cli.CLIError, "status is unavailable"):
            self.client.command("smart", "off")
        self.assertTrue(all(r[0] == "GET" for r in self.server.requests))

    def test_media_capabilities_required(self):
        for controls in (None, [], ["media_next"], "media_play"):
            self.state["media_controls"] = controls
            self.server.requests = []
            with self.assertRaisesRegex(cli.CLIError, "does not currently support play"):
                self.client.command("play")
            self.assertEqual(len(self.server.requests), 1)

    def test_unavailable_device_never_gets_action(self):
        self.state["available"] = False
        self.state["error"] = "soundbar is starting"
        with self.assertRaisesRegex(cli.CLIError, "Controls unavailable: soundbar is starting"):
            self.client.command("power")
        self.assertEqual(len(self.server.requests), 1)

    def test_missing_csrf_never_posts(self):
        del self.state["csrf"]
        with self.assertRaisesRegex(cli.CLIError, "Refresh soundbar status"):
            self.client.command("mute")
        self.assertEqual(len(self.server.requests), 1)

    def test_backend_conflict_is_error_and_never_retried(self):
        self.server.reply = lambda method, path, body: (409, {"error": "Another control is running " + STATE["csrf"], "state": self.state}) if method == "POST" else (200, self.state)
        with self.assertRaises(cli.CLIError) as error:
            self.client.command("mute")
        self.assertIn("HTTP 409: Another control is running", str(error.exception))
        self.assertNotIn(STATE["csrf"], str(error.exception))
        self.assertEqual(len(self.server.requests), 2)

    def test_http_failures_never_become_success(self):
        for code, value in ((401, b"private body secret"), (500, b"private body secret"), (302, b"redirect")):
            self.server.requests = []
            self.server.reply = lambda *args: (code, value)
            with self.assertRaises(cli.CLIError) as error:
                self.client.command("status")
            self.assertIn("HTTP " + str(code), str(error.exception))
            self.assertNotIn("private body", str(error.exception))
            self.assertEqual(len(self.server.requests), 1)

    def test_invalid_or_oversized_json_rejected(self):
        for body in (b"not JSON", b"[]", b"null", b"x" * (cli.MAX_RESPONSE + 1)):
            self.server.reply = lambda *args: (200, body)
            with self.assertRaises(cli.CLIError):
                self.client.command("status")

    def test_network_and_timeout_errors_are_concise_without_retry(self):
        for error in (socket.timeout("secret request"), urllib.error.URLError("secret request")):
            opener = unittest.mock.Mock()
            opener.open.side_effect = error
            client = cli.Soundbar(opener=opener)
            with self.assertRaises(cli.CLIError) as caught:
                client.command("status")
            self.assertNotIn("secret request", str(caught.exception))
            self.assertEqual(opener.open.call_count, 1)

    def test_default_transport_disables_proxies_and_redirects(self):
        client = cli.Soundbar()
        self.assertTrue(any(isinstance(h, cli.NoRedirects) for h in client.opener.handlers))
        self.assertFalse(any(isinstance(h, urllib.request.ProxyHandler) and h.proxies for h in client.opener.handlers))

    def test_arbitrary_commands_and_requests_rejected(self):
        for command, value in (("exec", None), ("source", "http://elsewhere"), ("volume", "99"), ("smart", "on"), ("night", "maybe")):
            with self.assertRaises(cli.CLIError):
                self.client.command(command, value)
        self.assertTrue(all(r[0] == "GET" for r in self.server.requests))
        with self.assertRaises(cli.CLIError):
            self.client.request("/setup/connect")
        with self.assertRaises(cli.CLIError):
            self.client.request("/api/action", "exec")

    def test_format_preserves_unknowns_and_reports_status_peer_track(self):
        text = cli.format_state(self.state)
        for want in ("Volume: 13", "Muted: no", "Mode: Smart", "Night: off", "Peer: Fixture phone", "Bluetooth: Streaming"):
            self.assertIn(want, text)
        self.state["volume"] = None
        self.state["muted"] = None
        self.state["sound"] = {}
        self.assertIn("Volume: unknown", cli.format_state(self.state))
        self.assertIn("Muted: unknown", cli.format_state(self.state))
        self.assertIn("Night: unknown", cli.format_state(self.state))
        self.state["streaming"] = {"service": "Chromecast", "status": "Paused", "cast": {"title": "Fixture song", "artist": "Fixture artist"}}
        self.assertIn("Track: Fixture song — Fixture artist", cli.format_state(self.state))
        self.state["bluetooth"]["connected"] = False
        self.assertNotIn("Peer:", cli.format_state(self.state))

    def test_json_output_recursively_removes_csrf(self):
        value = {"csrf": "secret", "volume": 0, "muted": False, "state": {"CSRF": "other"}, "items": [{"x-csrf-token": "third", "night": False}]}
        public = cli.public_state(value)
        self.assertEqual(public, {"volume": 0, "muted": False, "state": {}, "items": [{"night": False}]})
        self.assertIn("csrf", value)

    def test_main_outputs_json_or_nonzero_error(self):
        with patch.object(cli.Soundbar, "command", return_value=(self.state, False)), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(cli.main(["status", "--json"]), 0)
            self.assertNotIn("csrf", json.loads(output.getvalue()))
        with patch.object(cli.Soundbar, "command", side_effect=cli.CLIError("fixture conflict")), contextlib.redirect_stderr(io.StringIO()) as error:
            self.assertEqual(cli.main(["mute"]), 1)
            self.assertEqual(error.getvalue().strip(), "Error: fixture conflict")


if __name__ == "__main__":
    output = ROOT / "build/test-results/cli-tests.json"
    output.unlink(missing_ok=True)
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(CLITests)
    names = [test.id().rsplit(".", 1)[-1] for test in suite]
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    if result.wasSuccessful():
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps({"cases": names}, indent=2) + "\n")
    raise SystemExit(0 if result.wasSuccessful() else 1)
