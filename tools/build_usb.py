#!/usr/bin/env python3
"""Build a private, configurable all-in-one FAT32 USB toolkit and recovery ZIP."""
from __future__ import annotations

import argparse
import getpass
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import re

import build_ssh
import build_web
import hardware

from repo_layout import ROOT, USB as SOURCE, SSH, REMOTE_DEVICE
VERSION = (ROOT / "VERSION").read_text().strip()


def parse_config(data: bytes) -> dict[str, str]:
    if len(data) > 8192 or b"\0" in data:
        raise ValueError("Configuration must be plain text, at most 8 KiB, without NUL")
    busybox = shutil.which("busybox")
    if not busybox:
        raise ValueError("Install BusyBox to validate the device configuration parser")
    result = subprocess.run([busybox, "awk", "-f", str(SOURCE / "parse-config.awk")],
                            input=data, capture_output=True, check=False)
    if result.returncode:
        raise ValueError(result.stderr.decode("utf-8", errors="replace").strip())
    return {key: value.strip("'") for key, value in
            (line.split("=", 1) for line in result.stdout.decode("ascii").splitlines())}


def build(output: Path, config: bytes, *, password: str | None = None,
          ssh_password_hash_file: Path | None = None,
          wifi: tuple[str, str] | None = None, bluetooth_helper: Path | None = None,
          eq_parser: Path | None = None) -> tuple[Path, Path]:
    settings = parse_config(config)
    if settings["ACTION"] not in ("install", "check", "migrate", "update"):
        raise ValueError("Build with action=update, install, check or migrate; use the recovery ZIP for emergency removal")
    encoded_password = settings.get("SSH_PASSWORD_HASH")
    if encoded_password and (password is not None or ssh_password_hash_file is not None):
        raise ValueError("Credentials already provided in config; choose one credential source")
    if ssh_password_hash_file is not None:
        if password is not None:
            raise ValueError("Choose a password or an existing private hash, not both")
        encoded_password = Path(ssh_password_hash_file).read_text().strip()
        if not re.fullmatch(r"\$6\$[./0-9A-Za-z]{1,16}\$[./0-9A-Za-z]{86}", encoded_password):
            raise ValueError("Expected an existing SHA-512 crypt password hash")
    if settings["SSH"] == "true" and password is None and encoded_password is None:
        raise ValueError("SSH enabled: choose a private owner password")
    output = Path(output).resolve()
    if not output.is_relative_to((ROOT / "dist").resolve()):
        raise ValueError("Private packages must stay under the ignored dist/ directory")
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    install = output / f"jbl-bar9-toolkit-v{VERSION}-private.zip"
    recovery = output / f"jbl-bar9-toolkit-v{VERSION}-recovery.zip"
    if install.exists() or recovery.exists() or (output / "SHA256SUMS").exists():
        raise ValueError("Output archives already exist; use a new output directory")
    files = {p.name: p.read_bytes() for p in sorted(SOURCE.glob("*.sh"))}
    files["parse-config.awk"] = (SOURCE / "parse-config.awk").read_bytes()
    files["VERSION.txt"] = (VERSION + "\n").encode("ascii")
    for path in sorted(SSH.glob("*.sh")):
        files["files/ssh/" + path.name] = path.read_bytes()
    if password is not None:
        encoded_password = build_ssh.password_hash(password)
    if encoded_password is not None:
        files["files/ssh/password.hash"] = (encoded_password + "\n").encode("ascii")
    if wifi and settings.get("WIFI_SSID_HEX"):
        raise ValueError("Wi-Fi credentials already provided in config")
    if wifi:
        ssid, psk = build_ssh.wifi_credentials(*wifi)
        files["files/ssh/wifi.ssid.hex"] = (ssid + "\n").encode("ascii")
        files["files/ssh/wifi.psk.hex"] = (psk + "\n").encode("ascii")
    # Native helpers are project code only. Vendor libraries remain on the bar.
    with tempfile.TemporaryDirectory(prefix="jbl-toolkit-", dir=output) as temp:
        remote = build_web.build(Path(temp) / "remote", settings["ALLOWED_CIDR"],
                                 bluetooth_helper, eq_parser)
        files["files/remote"] = (remote / "remote").read_bytes()
    files["files/remote-start.sh"] = (REMOTE_DEVICE / "start.sh").read_bytes()
    files["setup.js"] = (ROOT / "packaging/setup.js").read_bytes()
    files["dispatch.sh"] = hardware.dispatcher("bar91")
    # Minimal recovery verifies its own launch chain without the remote/config.
    bootstrap = ("dispatch.sh", "launch.sh", "common.sh", "recover.sh")
    files["BOOTSTRAP.SHA256SUMS"] = build_ssh.manifest({key: files[key] for key in bootstrap})
    files["package.json"] = (json.dumps({
        "schema": 1, "version": VERSION, "model": "bar91",
        "profile": "bar91-23.06.41.80", "architecture": "linux-armv7",
        "bluetooth_helper": bluetooth_helper is not None, "front_eq_helper": eq_parser is not None,
        "qualification": "hardware-test-pending",
        "selection": "automatic", "bootstrap": "bar91-autoscript", "dispatcher_schema": 1,
    }, indent=2) + "\n").encode()
    # User configuration is deliberately outside the immutable payload manifest.
    files["SHA256SUMS"] = build_ssh.manifest(files)
    files["config.txt"] = config
    archive = {"JBLToolkit/" + name: data for name, data in files.items()}
    entry = hardware.USB_ENTRY
    archive["AutoScript/AutoScript"] = entry
    archive["README.txt"] = (SOURCE / "USB-README.txt").read_bytes()
    archive["SETUP.html"] = (ROOT / "packaging/SETUP.html").read_bytes()
    archive["LICENSE.txt"] = (ROOT / "LICENSE").read_bytes()
    if bluetooth_helper or eq_parser:
        archive["LICENSE.txt"] += b"\n" + (ROOT / "models/bar91/remote/native/bin/NOTICE.txt").read_bytes()
    # No credentials or application binary are needed to disable/restore boot.
    rescue = {
        "AutoScript/AutoScript": entry,
        "JBLToolkit/launch.sh": files["launch.sh"],
        "JBLToolkit/common.sh": files["common.sh"],
        "JBLToolkit/recover.sh": files["recover.sh"],
        "JBLToolkit/dispatch.sh": files["dispatch.sh"],
        "JBLToolkit/BOOTSTRAP.SHA256SUMS": files["BOOTSTRAP.SHA256SUMS"],
        "JBLToolkit/RECOVER": b"RECOVER\n",
        "README.txt": (SOURCE / "USB-README.txt").read_bytes(),
        "LICENSE.txt": archive["LICENSE.txt"],
    }
    for path in SOURCE.glob("*.sh"):
        subprocess.run(["/bin/sh", "-n", str(path)], check=True)
    subprocess.run(["/bin/sh", "-n"], input=files["dispatch.sh"], check=True)
    build_ssh.write_zip(install, archive)
    try:
        build_ssh.write_zip(recovery, rescue)
    except BaseException:
        install.unlink()
        raise
    checksums = output / "SHA256SUMS"
    # Output folder is private; archives also enforce 0600 regardless of umask.
    with checksums.open("x") as stream:
        for path in (install, recovery):
            stream.write(hashlib.sha256(path.read_bytes()).hexdigest() + "  " + path.name + "\n")
    return install, recovery


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=Path, required=True, help="Edited models/bar91/usb/config.example.txt")
    parser.add_argument("--output-dir", type=Path, default=ROOT / "dist/toolkit-private")
    parser.add_argument("--with-ssh", action="store_true", help="Bundle credentials even when ssh=false, allowing later enablement")
    parser.add_argument("--ssh-password-hash-file", type=Path, help="Reuse an existing private SHA-512 crypt hash without changing the password")
    parser.add_argument("--wifi", action="store_true", help="Prompt for optional WPA2 provisioning, independent of SSH")
    parser.add_argument("--bluetooth-helper", type=Path)
    parser.add_argument("--eq-parser", type=Path)
    args = parser.parse_args()
    try:
        config = args.config.read_bytes()
        settings = parse_config(config)
        password = None
        if (settings["SSH"] == "true" or args.with_ssh) and args.ssh_password_hash_file is None and not settings.get("SSH_PASSWORD_HASH"):
            if not sys.stdin.isatty():
                raise ValueError("Run interactively to choose SSH credentials without echoing them")
            password = getpass.getpass("Choose a NEW root SSH password (8+ characters): ")
            if password != getpass.getpass("Repeat root SSH password: "):
                raise ValueError("Passwords do not match")
        wifi = None
        if args.wifi:
            if not sys.stdin.isatty():
                raise ValueError("Run interactively to provide Wi-Fi credentials")
            wifi = (input("WPA2 Wi-Fi SSID: "), getpass.getpass("Wi-Fi passphrase: "))
        for path in build(args.output_dir, config, password=password, ssh_password_hash_file=args.ssh_password_hash_file, wifi=wifi,
                          bluetooth_helper=args.bluetooth_helper, eq_parser=args.eq_parser):
            print(path)
        print("Keep the install ZIP private. Extract its contents to a clean FAT32 USB root.")
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Build failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
