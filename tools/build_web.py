#!/usr/bin/env python3
"""Build a self-contained ARM remote plus an owner-specific private config."""
from pathlib import Path
import argparse
import hashlib
import ipaddress
import json
import os
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def build(output, allowed_cidr):
    network = ipaddress.ip_network(allowed_cidr, strict=False)
    if network.version != 4 or not network.is_private or network.prefixlen < 16:
        raise ValueError("Choose your private IPv4 LAN subnet, /16 or narrower")
    output = Path(output).resolve()
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="arm", GOARM="7")
    subprocess.run(["go", "build", "-buildvcs=false", "-trimpath", "-ldflags=-s -w -buildid=", "-o", str(output / "remote"), "."],
                   cwd=ROOT / "webremote", env=env, check=True)
    (output / "config.json").write_text(json.dumps({
        "listen": ":80", "allowed_cidr": str(network),
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
    args = parser.parse_args()
    print("Private device files:", build(args.output, args.allowed_cidr))
