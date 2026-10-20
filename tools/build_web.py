#!/usr/bin/env python3
"""Build a self-contained ARM remote plus an owner-specific private config."""
from pathlib import Path
import argparse
import base64
import hashlib
import ipaddress
import json
import os
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def build(output, allowed_cidr, bluetooth_helper=None, eq_parser=None, auto_standard=False):
    network = ipaddress.ip_network(allowed_cidr, strict=False)
    if network.version != 4 or not network.is_private or network.prefixlen < 16:
        raise ValueError("Choose your private IPv4 LAN subnet, /16 or narrower")
    output = Path(output).resolve()
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="arm", GOARM="7")
    ldflags = "-s -w -buildid="
    if bluetooth_helper:
        helper = Path(bluetooth_helper).read_bytes()
        if not 52 <= len(helper) <= 65536 or helper[:7] != b"\x7fELF\x01\x01\x01" or helper[18:20] != b"\x28\x00":
            raise ValueError("Bluetooth helper must be the owner-built 32-bit ARM ELF")
        ldflags += " -X main.bluetoothPayload=" + base64.b64encode(helper).decode("ascii")
    if eq_parser:
        helper = Path(eq_parser).read_bytes()
        if not 52 <= len(helper) <= 65536 or helper[:7] != b"\x7fELF\x01\x01\x01" or helper[18:20] != b"\x28\x00":
            raise ValueError("EQ parser must be the owner-built 32-bit ARM ELF")
        ldflags += " -X main.eqParserPayload=" + base64.b64encode(helper).decode("ascii")
    subprocess.run(["go", "build", "-buildvcs=false", "-trimpath", "-ldflags=" + ldflags, "-o", str(output / "remote"), "."],
                   cwd=ROOT / "webremote", env=env, check=True)
    (output / "config.json").write_text(json.dumps({
        "listen": ":80", "allowed_cidr": str(network), "auto_standard": bool(auto_standard),
    }, indent=2) + "\n")
    (output / "config.json").chmod(0o600)
    (output / "OWNER.txt").write_text("JBL_WEB_REMOTE_REV1\n")
    for name in ("start.sh", "install.sh", "uninstall.sh"):
        shutil.copyfile(ROOT / "webremote" / name, output / name)
    (output / "SHA256SUMS").write_text("".join(
        hashlib.sha256((output / name).read_bytes()).hexdigest() + "  " + name + "\n"
        for name in ("remote", "config.json", "OWNER.txt", "start.sh", "install.sh", "uninstall.sh")
    ))
    return output


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--allowed-cidr", required=True, help="Your trusted IPv4 LAN, e.g. 192.168.1.0/24")
    parser.add_argument("--output", type=Path, default=ROOT / "dist/webremote-private")
    parser.add_argument("--bluetooth-helper", type=Path, help="Optional native AVRCP helper built with tools/build_bt.py")
    parser.add_argument("--eq-parser", type=Path, help="Optional native EQ parser built with tools/build_eq.py")
    parser.add_argument("--auto-standard", action="store_true", help="Let Go own automatic Standard Mode after wake")
    args = parser.parse_args()
    print("Private device files:", build(args.output, args.allowed_cidr, args.bluetooth_helper, args.eq_parser, args.auto_standard))
