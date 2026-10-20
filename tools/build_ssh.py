#!/usr/bin/env python3
"""Build PRIVATE, owner-specific USB SSH archives. Python standard library only."""
from __future__ import annotations

import argparse
import getpass
import hashlib
import ipaddress
import os
from pathlib import Path
import re
import secrets
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "ssh"
CRYPT64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
PRIVATE_LANS = tuple(ipaddress.IPv4Network(n) for n in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"))


def allowed_ipv4(value: str) -> str:
    """Restrict access to one explicitly chosen PC on a private LAN."""
    address = ipaddress.IPv4Address(value)
    if not any(address in network for network in PRIVATE_LANS):
        raise ValueError("Choose the PC's private LAN IPv4 address")
    return str(address)


def password_hash(password: str, salt: str | None = None) -> str:
    """SHA-512 crypt ($6$), 5000 rounds, matching the bar's glibc crypt(3).

    Kept here because Python 3.13 removed crypt. No plaintext password enters
    a subprocess, command line, generated script, log, or archive.
    """
    if not 8 <= len(password) <= 256 or "\x00" in password:
        raise ValueError("Choose a password of 8–256 characters without NUL")
    salt = salt or "".join(secrets.choice(CRYPT64) for _ in range(16))
    if not re.fullmatch(r"[./0-9A-Za-z]{1,16}", salt):
        raise ValueError("Invalid crypt salt")
    key, salt_bytes = password.encode("utf-8"), salt.encode("ascii")

    def repeat(block: bytes, size: int) -> bytes:
        return (block * ((size + len(block) - 1) // len(block)))[:size]

    alternate = hashlib.sha512(key + salt_bytes + key).digest()
    context = hashlib.sha512(key + salt_bytes + repeat(alternate, len(key)))
    count = len(key)
    while count:
        context.update(alternate if count & 1 else key)
        count >>= 1
    alternate = context.digest()
    p_bytes = repeat(hashlib.sha512(key * len(key)).digest(), len(key))
    s_bytes = repeat(hashlib.sha512(salt_bytes * (16 + alternate[0])).digest(), len(salt_bytes))
    for count in range(5000):
        context = hashlib.sha512(p_bytes if count & 1 else alternate)
        if count % 3:
            context.update(s_bytes)
        if count % 7:
            context.update(p_bytes)
        context.update(alternate if count & 1 else p_bytes)
        alternate = context.digest()
    permutation = (
        (0, 21, 42), (22, 43, 1), (44, 2, 23), (3, 24, 45), (25, 46, 4),
        (47, 5, 26), (6, 27, 48), (28, 49, 7), (50, 8, 29), (9, 30, 51),
        (31, 52, 10), (53, 11, 32), (12, 33, 54), (34, 55, 13), (56, 14, 35),
        (15, 36, 57), (37, 58, 16), (59, 17, 38), (18, 39, 60), (40, 61, 19),
        (62, 20, 41),
    )
    encoded = ""
    for high, middle, low in permutation:
        value = alternate[high] << 16 | alternate[middle] << 8 | alternate[low]
        for _ in range(4):
            encoded += CRYPT64[value & 63]
            value >>= 6
    value = alternate[63]
    for _ in range(2):
        encoded += CRYPT64[value & 63]
        value >>= 6
    return f"$6${salt}${encoded}"


def wifi_credentials(ssid: str, passphrase: str) -> tuple[str, str]:
    ssid_bytes = ssid.encode("utf-8")
    if not 1 <= len(ssid_bytes) <= 32:
        raise ValueError("Wi-Fi SSID must be 1–32 UTF-8 bytes")
    if not 8 <= len(passphrase) <= 63 or any(not 32 <= ord(c) <= 126 for c in passphrase):
        raise ValueError("WPA2 passphrase must be 8–63 printable ASCII characters")
    psk = hashlib.pbkdf2_hmac("sha1", passphrase.encode("ascii"), ssid_bytes, 4096, 32)
    return ssid_bytes.hex(), psk.hex()


def manifest(files: dict[str, bytes]) -> bytes:
    return "".join(f"{hashlib.sha256(data).hexdigest()}  {name}\n" for name, data in sorted(files.items())).encode("ascii")


def write_zip(path: Path, files: dict[str, bytes]) -> None:
    """Create with private permissions, even if the caller's umask is open."""
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            with zipfile.ZipFile(stream, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                for name, data in sorted(files.items()):
                    info = zipfile.ZipInfo(name)
                    info.compress_type = zipfile.ZIP_DEFLATED
                    info.external_attr = 0o100600 << 16
                    archive.writestr(info, data)
    except BaseException:
        path.unlink(missing_ok=True)
        raise


def build(output_dir: Path, pc: str, password: str, *, interface: str = "wlan0",
          autostart: bool = True, wifi: tuple[str, str] | None = None) -> tuple[Path, Path]:
    pc = allowed_ipv4(pc)
    if not re.fullmatch(r"[A-Za-z][A-Za-z0-9_]{0,14}", interface):
        raise ValueError("Invalid network interface name")
    credentials = wifi_credentials(*wifi) if wifi else None
    hashed = password_hash(password)
    settings = f"ALLOWED_PC={pc}\nINTERFACE={interface}\nAUTOSTART={int(autostart)}\n".encode("ascii")
    payload = {p.name: p.read_bytes() for p in sorted(SCRIPTS.glob("*.sh"))}
    if not payload:
        raise ValueError("SSH source scripts missing")
    payload["settings.conf"] = settings
    payload["password.hash"] = (hashed + "\n").encode("ascii")
    if credentials:
        payload["wifi.ssid.hex"] = (credentials[0] + "\n").encode("ascii")
        payload["wifi.psk.hex"] = (credentials[1] + "\n").encode("ascii")
    payload["checksums.txt"] = manifest(payload)
    files = {f"JBLLocalSSH/{name}": data for name, data in payload.items()}
    files["AutoScript/AutoScript"] = b"CLI(CLI_exec sh /mnt/sda1/JBLLocalSSH/launch.sh &)\r\n"
    files["README.txt"] = (
        "PRIVATE OWNER-SPECIFIC JBL BAR 9.1 SSH PACKAGE\n"
        "Contains a password hash and possibly a Wi-Fi key. Never publish.\n"
        "Requires the exact supported native sshd/keygen builds.\n"
        "Copy contents to FAT32 USB root; insert with bar fully on.\n"
        "Wait 3 minutes; return USB to computer, leaving bar on.\n"
        "Read JBLLocalSSH/STATUS.txt and connection.txt. Verify the exported\n"
        "host_ed25519.pub fingerprint against your SSH client's prompt.\n"
        f"Only PC {pc} can log in; reserve its DHCP address. Port 2222.\n"
        "Password is the one you chose during the build; none is printed.\n"
        f"Automatic startup requested: {'yes' if autostart else 'no'}.\n"
        "An existing live installation keeps its current password.\n"
        "Rollback archive disables boot startup; running sessions remain.\n"
        "Remove USB after installing; test a reboot without USB.\n"
    ).encode("utf-8")
    # Recovery has no credentials and runs independently of installed helpers.
    rollback = {
        "AutoScript/AutoScript": b"CLI(CLI_exec sh /mnt/sda1/JBLSSHRollback/rollback.sh &)\r\n",
        "JBLSSHRollback/hooks.sh": payload["hooks.sh"],
        "JBLSSHRollback/rollback.sh": (
            b"#!/bin/sh\nexec </dev/null >/mnt/sda1/JBLSSHRollback/result.txt 2>&1\n"
            b"/bin/sh /mnt/sda1/JBLSSHRollback/hooks.sh rollback\n"
            b"result=$?\nprintf 'ROLLBACK_EXIT=%s\\n' \"$result\"\n/bin/busybox sync\nexit \"$result\"\n"
        ),
        "README.txt": (
            b"Copy contents to FAT32 USB root, replacing the SSH install AutoScript.\n"
            b"Insert with bar on; wait 30 seconds. Check JBLSSHRollback/result.txt.\n"
            b"Exit 0: boot startup disabled and original hook restored.\n"
            b"Nonzero: unknown hook changes are preserved; boot-disable marker remains.\n"
            b"Does not stop the running SSH daemon, remove files or alter Wi-Fi.\n"
        ),
    }
    output_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    install_path = output_dir / "jbl-bar9-ssh-private.zip"
    rollback_path = output_dir / "jbl-bar9-ssh-rollback-private.zip"
    if install_path.exists() or rollback_path.exists():
        raise ValueError("Output archives already exist; move them aside before building again")
    write_zip(install_path, files)
    try:
        write_zip(rollback_path, rollback)
    except BaseException:
        install_path.unlink()
        raise
    return install_path, rollback_path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--allowed-pc", required=True, help="PC's private LAN IPv4 address; only this client may log in")
    parser.add_argument("--interface", default="wlan0", help="bar's LAN interface (default: wlan0)")
    parser.add_argument("--usb-only", action="store_true", help="start by USB insertion; do not enable boot startup")
    parser.add_argument("--wifi", action="store_true", help="optionally provision WPA2 Wi-Fi; prompts for SSID/passphrase")
    parser.add_argument("--output-dir", type=Path, default=ROOT / "dist", help="private output directory inside ignored dist/")
    args = parser.parse_args()
    try:
        pc = allowed_ipv4(args.allowed_pc)
        output = args.output_dir.resolve()
        if not output.is_relative_to((ROOT / "dist").resolve()):
            raise ValueError("Output must stay inside the repository's ignored dist/ directory")
        if not sys.stdin.isatty():
            raise ValueError("Run interactively in a terminal so passwords are not echoed")
        password = getpass.getpass("Choose a NEW root SSH password (8+ characters): ")
        if password != getpass.getpass("Repeat root SSH password: "):
            raise ValueError("Passwords do not match")
        wifi = None
        if args.wifi:
            ssid = input("Your WPA2 Wi-Fi SSID: ")
            wifi = (ssid, getpass.getpass("Wi-Fi passphrase: "))
        paths = build(output, pc, password, interface=args.interface, autostart=not args.usb_only, wifi=wifi)
    except (ValueError, OSError) as error:
        print(f"Build failed: {error}", file=sys.stderr)
        return 1
    for path in paths:
        print(path)
    print("Keep these archives private. Copy the install ZIP contents to FAT32 USB root.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
