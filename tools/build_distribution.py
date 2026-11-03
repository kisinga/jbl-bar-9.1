#!/usr/bin/env python3
"""Build credential-free, precompiled single-USB release assets. No vendor blobs."""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import tempfile
import zipfile

import build_ssh
import build_usb
import prepare_usb
import hardware
from repo_layout import ROOT

CONFIG = b'''# Open SETUP.html to create your local configuration before use.
format=1
action=check
ssh=false
web_remote=true
auto_standard=false
allowed_cidr=192.168.1.0/24
web_port=80
'''


def build(output: Path, model: str = "auto") -> list[Path]:
    selected = hardware.resolve(model)
    output = output.resolve()
    if not output.is_relative_to((ROOT / 'dist').resolve()):
        raise ValueError('Distribution output must be under dist/')
    if output.exists() and any(output.iterdir()):
        raise ValueError('Use an empty distribution output directory')
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='jbl-distribution-', dir=ROOT / 'dist') as temp:
        builders = {"bar91": build_usb.build}
        install, recovery = builders[selected](Path(temp), CONFIG)
        files = prepare_usb.package_files(install)
        if any(n.endswith(('/password.hash', '/wifi.ssid.hex', '/wifi.psk.hex')) for n in files):
            raise ValueError('Public package must not contain credentials')
        settings = build_usb.parse_config(files['JBLToolkit/config.txt'])
        if settings['SSH'] != 'false' or settings['ACTION'] != 'check' or len(settings) != 7:
            raise ValueError('Public configuration must be credential-free and check-only')
        stem = f'jbl-{"bar-toolkit" if model == "auto" else selected}-v{build_usb.VERSION}'
        package = output / (stem + '-usb.zip')
        rescue = output / (stem + '-recovery.zip')
        build_ssh.write_zip(package, files)
        with zipfile.ZipFile(recovery) as archive:
            rescue_files = {n: archive.read(n) for n in archive.namelist()}
        build_ssh.write_zip(rescue, rescue_files)
    prefix = '' if model == 'auto' else model + '-'
    info = output / (prefix + 'release.json')
    info.write_text(json.dumps({
        'schema': 1, 'version': build_usb.VERSION, 'model': 'bar91',
        'target': model, 'selection': 'automatic', 'included_models': [selected],
        'profile': 'bar91-23.06.41.80', 'architecture': 'linux-armv7',
        'qualification': 'hardware-test-pending', 'default_action': 'check',
        'setup': 'Extract the USB ZIP and open SETUP.html. No Go or Python needed.',
        'credentials': 'Owner adds credentials locally in JBLToolkit/config.txt; never upload configured USB files.',
        'assets': [package.name, rescue.name],
    }, indent=2) + '\n')
    checksums = output / (prefix + 'SHA256SUMS')
    checksums.write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name + '\n'
                                 for p in (package, rescue, info)))
    return [package, rescue, info, checksums]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output-dir', type=Path, default=ROOT / 'dist/release')
    parser.add_argument('--model', default='auto', help='Default: automatic hardware selection')
    parser.add_argument('--matrix', action='store_true', help='Print reviewed CI package targets without building')
    args = parser.parse_args()
    try:
        if args.matrix:
            print(json.dumps(hardware.matrix(), separators=(',', ':')))
            return
        for path in build(args.output_dir, args.model):
            print(path)
    except (ValueError, OSError) as error:
        parser.exit(1, f'Packaging failed: {error}\n')


if __name__ == '__main__':
    main()
