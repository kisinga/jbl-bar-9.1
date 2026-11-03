#!/usr/bin/env python3
"""Stage or verify a Bar 9.1 toolkit on an already formatted Linux FAT32 USB.

Never formats a device. Staging requires an empty mount and activates AutoScript
only after all payload files have been written, flushed and read back.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import subprocess
import zipfile

import build_usb
import build_ssh
import hardware


def package_files(archive: Path) -> dict[str, bytes]:
    files = {}
    names = set()
    with zipfile.ZipFile(archive) as source:
        if sum(i.file_size for i in source.infolist()) > 128 * 1024 * 1024:
            raise ValueError("Package exceeds 128 MiB")
        for info in source.infolist():
            name = info.filename
            path = PurePosixPath(name)
            if (not re.fullmatch(r"[A-Za-z0-9_./-]+", name) or path.is_absolute()
                    or any(p in ("", ".", "..") for p in name.split("/"))
                    or len(path.parts) > 5 or name.casefold() in names
                    or stat.S_ISLNK(info.external_attr >> 16) or info.is_dir()):
                raise ValueError("Unsafe, duplicate or unsupported archive entry")
            if path.parts[0] not in ("AutoScript", "JBLToolkit", "README.txt", "LICENSE.txt", "SETUP.html"):
                raise ValueError("Unexpected package root")
            names.add(name.casefold())
            files[name] = source.read(info)
    required = ("AutoScript/AutoScript", "JBLToolkit/package.json", "JBLToolkit/SHA256SUMS",
                "JBLToolkit/config.txt", "JBLToolkit/files/remote", "JBLToolkit/launch.sh",
                "JBLToolkit/check.sh", "JBLToolkit/verify.sh", "JBLToolkit/recover.sh")
    if not all(name in files for name in required):
        raise ValueError("Expected a complete current private installation package")
    entry = files['AutoScript/AutoScript']
    if entry not in (hardware.USB_ENTRY, hardware.LEGACY_USB_ENTRY):
        raise ValueError("Unexpected USB entry point")
    if entry == hardware.USB_ENTRY:
        for name in ('dispatch.sh', 'BOOTSTRAP.SHA256SUMS'):
            if 'JBLToolkit/' + name not in files:
                raise ValueError("Incomplete automatic hardware dispatcher")
        bootstrap = ('dispatch.sh', 'launch.sh', 'common.sh', 'recover.sh')
        if files['JBLToolkit/BOOTSTRAP.SHA256SUMS'] != build_ssh.manifest({name: files['JBLToolkit/' + name] for name in bootstrap}):
            raise ValueError("Recovery bootstrap manifest is incomplete or inconsistent")
    covered = set()
    for line in files['JBLToolkit/SHA256SUMS'].decode('ascii').splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_./-]+)", line)
        if not match:
            raise ValueError("Invalid package manifest")
        digest, relative = match.groups()
        name = 'JBLToolkit/' + relative
        if name in covered or name not in files or hashlib.sha256(files[name]).hexdigest() != digest:
            raise ValueError("Package integrity failed")
        covered.add(name)
    expected = {n for n in files if n.startswith('JBLToolkit/')} - {
        'JBLToolkit/SHA256SUMS', 'JBLToolkit/config.txt'}
    if covered != expected:
        raise ValueError("Manifest does not cover exactly the immutable payload")
    metadata = json.loads(files['JBLToolkit/package.json'])
    if (metadata.get('schema'), metadata.get('model'), metadata.get('profile'), metadata.get('architecture')) != (
            1, 'bar91', 'bar91-23.06.41.80', 'linux-armv7'):
        raise ValueError("Unsupported package profile")
    if entry == hardware.USB_ENTRY and (metadata.get('selection'), metadata.get('dispatcher_schema')) != ('automatic', 1):
        raise ValueError("Automatic dispatcher metadata mismatch")
    elf = files['JBLToolkit/files/remote']
    if elf[:7] != b'\x7fELF\x01\x01\x01' or elf[18:20] != b'\x28\x00':
        raise ValueError("Remote must be a little endian 32-bit ARM ELF")
    build_usb.parse_config(files['JBLToolkit/config.txt'])
    return files


def inspect_mount(mount: Path) -> dict:
    def command(*args):
        return json.loads(subprocess.check_output(args, text=True))
    if mount.is_symlink() or not mount.is_dir():
        raise ValueError("Supply the actual mounted USB directory")
    mount = mount.resolve()
    found = command('findmnt', '--json', '--target', str(mount), '-o', 'TARGET,SOURCE,FSTYPE,OPTIONS')['filesystems'][0]
    if found['target'] != str(mount) or found['fstype'] != 'vfat' or 'rw' not in found['options'].split(','):
        raise ValueError("Target must be an exact writable FAT32 mount")
    device = Path(found['source']).resolve()
    partition = Path('/sys/class/block') / device.name
    if (partition / 'partition').read_text().strip() != '1':
        raise ValueError("The soundbar requires the first USB partition")
    listing = command('lsblk', '--json', '--paths', '--bytes', '-o', 'NAME,TYPE,TRAN,RM,RO,FSTYPE,FSVER,PTTYPE')
    for disk in listing['blockdevices']:
        for child in disk.get('children', []):
            if child['name'] != str(device):
                continue
            if (disk['type'] != 'disk' or disk['tran'] != 'usb' or not disk['rm'] or disk['ro']
                    or disk['pttype'] != 'dos' or child['ro'] or child['fsver'] != 'FAT32'):
                raise ValueError("Expected removable USB, DOS/MBR and writable FAT32 first partition")
            return {'mount': str(mount), 'partition': str(device), 'disk': disk['name'],
                    'filesystem': 'FAT32', 'partition_table': 'dos', 'partition_number': 1}
    raise ValueError("Mount is not a directly attached USB partition")


def verify_files(mount: Path, files: dict[str, bytes]) -> None:
    for name, expected in files.items():
        target = mount / name
        if any(parent.is_symlink() for parent in [target, *target.parents] if parent != mount.parent):
            raise ValueError("Symlink in destination")
        if not target.is_file():
            raise ValueError(f"Missing file: {name}")
        data = target.read_bytes()
        if name == 'JBLToolkit/config.txt':
            build_usb.parse_config(data)  # Config is intentionally editable.
        elif data != expected:
            raise ValueError(f"USB readback differs: {name}")
    if (mount / 'JBLToolkit/RECOVER').exists():
        raise ValueError("Recovery marker takes precedence; remove deliberately before installation")


def stage_files(mount: Path, files: dict[str, bytes]) -> None:
    # Do not merge installers or overwrite a user's files, including partial runs.
    if any(mount.iterdir()):
        raise ValueError("Staging requires an empty USB mount")
    if shutil.disk_usage(mount).free < sum(map(len, files.values())) + 2 * 1024 * 1024:
        raise ValueError("Insufficient USB free space")
    entry = 'AutoScript/AutoScript'
    for name, data in files.items():
        if name == entry:
            continue
        path = mount / name
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open('xb') as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        if path.read_bytes() != data:
            raise ValueError(f"USB write/readback failed: {name}")
    # Flush directories/payload before exposing the entry point to the soundbar.
    os.sync()
    path = mount / entry
    path.parent.mkdir()
    with path.open('xb') as stream:
        stream.write(files[entry])
        stream.flush()
        os.fsync(stream.fileno())
    os.sync()
    verify_files(mount, files)


def prepare_package(package: Path, mount: Path, report: Path | None = None, *, verify: bool = False) -> dict:
    files = package_files(package)
    receipt = inspect_mount(mount)
    if verify:
        verify_files(mount, files)
    else:
        stage_files(mount, files)
    config = build_usb.parse_config((mount / 'JBLToolkit/config.txt').read_bytes())
    receipt.update(schema=1, package_sha256=hashlib.sha256(package.read_bytes()).hexdigest(),
                   files_verified=len(files), action=config['ACTION'], hardware_qualified=False)
    if report:
        report.parent.mkdir(parents=True, exist_ok=True)
        report.write_text(json.dumps(receipt, indent=2) + '\n')
    return receipt


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', required=True, type=Path)
    parser.add_argument('--mount', required=True, type=Path)
    parser.add_argument('--verify', action='store_true', help='Read back an already staged USB; never modify it')
    parser.add_argument('--report', type=Path, help='Write a host validation receipt')
    args = parser.parse_args()
    try:
        receipt = prepare_package(args.package, args.mount, args.report, verify=args.verify)
        print(json.dumps(receipt, indent=2))
    except (ValueError, OSError, KeyError, zipfile.BadZipFile, subprocess.CalledProcessError) as error:
        print(f'USB preparation failed: {error}')
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
