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
import tempfile

from repo_layout import ROOT, REMOTE, REMOTE_DEVICE, bar91_profile
from check_profiles import check as check_profiles


def build(output, allowed_cidr, bluetooth_helper=None, eq_parser=None, auto_standard=False, smart_mode_memory=False):
    if smart_mode_memory and not auto_standard:
        raise ValueError("--smart-mode-memory needs --auto-standard")
    check_profiles()
    profile = bar91_profile()
    network = ipaddress.ip_network(allowed_cidr, strict=False)
    if network.version != 4 or not network.is_private or network.prefixlen < 16:
        raise ValueError("Choose your private IPv4 LAN subnet, /16 or narrower")
    output = Path(output).resolve()
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    env = dict(os.environ, CGO_ENABLED="0", GOOS=profile["architecture"]["goos"],
               GOARCH=profile["architecture"]["goarch"], GOARM=profile["architecture"]["goarm"])
    # Embed helpers through a compiler overlay, keeping source clean and the
    # Go command line below Windows' 32 KiB process limit.
    with tempfile.TemporaryDirectory(prefix="jbl-native-overlay-") as folder:
        replacements = {}
        for helper_path, filename, variable, label in (
            (bluetooth_helper, "bt_rpc.go", "bluetoothPayload", "Bluetooth helper"),
            (eq_parser, "eq.go", "eqParserPayload", "EQ parser"),
        ):
            if helper_path is None:
                continue
            helper = Path(helper_path).read_bytes()
            if not 52 <= len(helper) <= 65536 or helper[:7] != b"\x7fELF\x01\x01\x01" or helper[18:20] != b"\x28\x00":
                raise ValueError(label + " must be the owner-built 32-bit ARM ELF")
            source = REMOTE / filename
            code = source.read_text()
            declaration = "var " + variable + " string"
            if code.count(declaration) != 1:
                raise ValueError("Native payload declaration changed: " + filename)
            target = Path(folder) / filename
            value = json.dumps(base64.b64encode(helper).decode("ascii"))
            target.write_text(code.replace(declaration, "var " + variable + " = " + value, 1))
            replacements[str(source)] = str(target)
        arguments = ["go", "build", "-buildvcs=false", "-trimpath", "-ldflags=-s -w -buildid="]
        if replacements:
            overlay = Path(folder) / "native-overlay.json"
            overlay.write_text(json.dumps({"Replace": replacements}))
            arguments.append("-overlay=" + str(overlay))
        subprocess.run([*arguments, "-o", str(output / "remote"), "."], cwd=REMOTE, env=env, check=True)
    (output / "config.json").write_text(json.dumps({
        "listen": ":80", "allowed_cidr": str(network), "auto_standard": bool(auto_standard),
        "smart_mode_memory": bool(smart_mode_memory),
    }, indent=2) + "\n")
    (output / "config.json").chmod(0o600)
    (output / "OWNER.txt").write_text("JBL_WEB_REMOTE_REV1\n")
    for name in ("start.sh", "install.sh", "uninstall.sh"):
        shutil.copyfile(REMOTE_DEVICE / name, output / name)
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
    parser.add_argument("--smart-mode-memory", action="store_true", help="With --auto-standard: after wake, restore the mode chosen before standby")
    args = parser.parse_args()
    print("Private device files:", build(args.output, args.allowed_cidr, args.bluetooth_helper, args.eq_parser, args.auto_standard, args.smart_mode_memory))
