#!/usr/bin/env python3
"""Prepare one jailbreak USB, develop the remote, or inspect and control a bar."""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import getpass
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile
import hardware

from repo_layout import ROOT


def catalog() -> dict[str, dict]:
    data = json.loads((ROOT / 'models/catalog.json').read_text())
    if data.get('schema') != 1:
        raise ValueError('Unsupported model catalog schema')
    models = {m['id']: m for m in data['models']}
    if len(models) != len(data['models']):
        raise ValueError('Duplicate model identity')
    return models


def require_installer(model: str) -> dict:
    models = catalog()
    if model not in models:
        raise ValueError('Unknown model; run ./jbl models')
    # Metadata describes support; only reviewed code grants installation.
    # Add a separate implementation here when a second model is qualified.
    if model not in hardware.INSTALLERS:
        raise ValueError(f"{models[model]['name']}: installation is not qualified. See {models[model]['guide']}")
    return models[model]


def configuration(model: str, subnet: str, features: str, action: str) -> bytes:
    item = require_installer(model)
    selected = set(features.split(','))
    if not selected or '' in selected or not selected.issubset(item['install_features']):
        raise ValueError('Choose comma-separated features: ' + ','.join(item['install_features']))
    if action not in ('check', 'install', 'migrate', 'update'):
        raise ValueError('Preparation action must be check, install, migrate or update')
    import build_usb
    text = ('format=1\naction=' + action + '\nssh=' + str('ssh' in selected).lower() +
            '\nweb_remote=' + str('web' in selected).lower() +
            '\nauto_standard=' + str('auto-standard' in selected).lower() +
            '\nsmart_mode_memory=' + str('smart-mode-memory' in selected).lower() +
            '\nallowed_cidr=' + subnet + '\nweb_port=80\n')
    data = text.encode('ascii')
    build_usb.parse_config(data)
    return data


def prepare(args) -> int:
    args.model = hardware.resolve(args.model)
    require_installer(args.model)  # Before credentials, outputs or device access.
    import build_usb
    import prepare_usb
    if args.config:
        data = args.config.read_bytes()
        if any((args.allowed_cidr, args.features, args.action)):
            raise ValueError('--config already defines network, features and action')
    else:
        if not args.allowed_cidr:
            raise ValueError('Provide --allowed-cidr or a complete --config file')
        data = configuration(args.model, args.allowed_cidr, args.features or 'web', args.action or 'check')
    settings = build_usb.parse_config(data)
    if args.mount:
        prepare_usb.inspect_mount(args.mount)
        if any(args.mount.iterdir()):
            raise ValueError('USB preparation needs an empty mounted FAT32 first partition')
    password = None
    if settings['SSH'] == 'true' and args.ssh_password_hash_file is None and not settings.get('SSH_PASSWORD_HASH'):
        if not sys.stdin.isatty():
            raise ValueError('SSH needs an interactive password prompt or --ssh-password-hash-file')
        password = getpass.getpass('Choose your SSH password (8+ characters): ')
        if password != getpass.getpass('Repeat SSH password: '):
            raise ValueError('Passwords do not match')
    wifi = None
    if args.wifi:
        if not sys.stdin.isatty():
            raise ValueError('Wi-Fi provisioning needs an interactive credential prompt')
        wifi = (input('WPA2 Wi-Fi SSID: '), getpass.getpass('Wi-Fi passphrase: '))
    (ROOT / 'dist').mkdir(exist_ok=True)
    output = args.output_dir or Path(tempfile.mkdtemp(prefix='jbl-' + args.model + '-', dir=ROOT / 'dist'))
    output = output.resolve()
    if not output.is_relative_to((ROOT / 'dist').resolve()):
        raise ValueError('Keep private installation packages under dist/')
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    package, recovery = build_usb.build(output, data, password=password,
        ssh_password_hash_file=args.ssh_password_hash_file,
        bluetooth_helper=args.bluetooth_helper, eq_parser=args.eq_parser, wifi=wifi)
    (output / 'config.txt').write_bytes(data)
    (output / 'config.txt').chmod(0o600)
    if args.mount:
        receipt = prepare_usb.prepare_package(package, args.mount, output / 'usb-preparation.json')
        print(f"USB verified: {receipt['partition']} ({receipt['filesystem']}), action={receipt['action']}")
    print('Installation package:', package)
    print('Matching recovery:', recovery)
    print('Next steps:', ROOT / 'docs/models/bar91/USB-TOOLKIT.md')
    if settings['ACTION'] == 'check':
        print('First run is check-only. Inspect the returned reports before installation.')
    elif settings['ACTION'] == 'migrate':
        print('This run retires the exact reviewed legacy chain and installs the toolkit. Allow five minutes.')
    else:
        print('This run installs selected features. Allow five minutes, then verify a USB-free restart.')
    if args.mount:
        print('Safely unmount the USB before removing it.')
    return 0


def main(argv=None) -> int:
    arguments = list(sys.argv[1:] if argv is None else argv)
    if arguments and arguments[0] == 'remote':
        import jbl as remote_cli
        return remote_cli.main(arguments[1:])
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command')
    models = commands.add_parser('models', help='Show model support and evidence boundaries')
    models.add_argument('--json', action='store_true')
    build = commands.add_parser('prepare', help='Build a private single-USB package; optionally stage an empty USB')
    build.add_argument('--model', default='auto', help='auto (default), or a reviewed hardware target')
    build.add_argument('--config', type=Path, help='Local config with settings and optional credentials')
    build.add_argument('--allowed-cidr', help='Your actual trusted LAN subnet')
    build.add_argument('--features', help='web,ssh,auto-standard,smart-mode-memory (default: web)')
    build.add_argument('--action', choices=('check', 'install', 'migrate', 'update'), help='Default: check')
    build.add_argument('--mount', type=Path)
    build.add_argument('--output-dir', type=Path)
    build.add_argument('--ssh-password-hash-file', type=Path)
    build.add_argument('--bluetooth-helper', type=Path)
    build.add_argument('--eq-parser', type=Path)
    build.add_argument('--wifi', action='store_true', help='Prompt privately for optional WPA2 provisioning')
    verify = commands.add_parser('usb', help='Verify an existing staged USB and print its status')
    verify.add_argument('--package', required=True, type=Path)
    verify.add_argument('--mount', required=True, type=Path)
    commands.add_parser('remote', help='Use the installed remote: ./jbl remote status --host <bar-ip>')
    dev = commands.add_parser('dev', help='Run a local UI preview; never connects to a soundbar')
    dev.add_argument('--listen', default='127.0.0.1:8080')
    dev.add_argument('--fixture', type=Path, help='Replay a normalized state JSON file read-only')
    inspect = commands.add_parser('inspect', help='Research: collect two fixed read-only JBL One-family getters')
    inspect.add_argument('--host', required=True, help='Explicit private IPv4 address; no discovery or scanning')
    inspect.add_argument('--output-dir', type=Path, help='New session directory under research/private/ (default: timestamped)')
    test = commands.add_parser('test', help='Run fast Go tests; no bar, credentials or firmware needed')
    test_modes = test.add_mutually_exclusive_group()
    test_modes.add_argument('--full', action='store_true', help='Host integration, Go race/vet and browser checks')
    test_modes.add_argument('--ui', action='store_true', help='Browser regressions only (npm ci first)')
    distribution = commands.add_parser('build', help='Build a public model-specific USB ZIP without credentials')
    distribution.add_argument('--model', default='auto', help='auto (default), or a reviewed hardware target')
    distribution.add_argument('--output-dir', type=Path)
    args = parser.parse_args(arguments)
    try:
        if args.command == 'inspect':
            # Validate before output creation or launching the Go collector.
            import ipaddress
            address = ipaddress.IPv4Address(args.host)
            if not any(address in ipaddress.IPv4Network(net) for net in ('10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16')):
                raise ValueError('Use the soundbar\'s private IPv4 address')
            private = (ROOT / 'research/private').resolve()
            output = (args.output_dir or private / 'bar-family/sessions' /
                      datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S.%fZ')).resolve()
            if not output.is_relative_to(private) or output == private:
                raise ValueError('Keep raw device captures under research/private/')
            if output.exists():
                raise ValueError('Use a new session directory; existing captures are never overwritten')
            return subprocess.run(['go', 'run', './cmd/inspect', '--host', str(address),
                                   '--output-dir', str(output)], cwd=ROOT / 'apps/remote').returncode
        if args.command == 'dev':
            host, separator, port = args.listen.rpartition(':')
            if not separator or host != '127.0.0.1' or not port.isdigit() or not 0 <= int(port) <= 65535:
                raise ValueError('Preview must listen on 127.0.0.1:<port>')
            command = ['go', 'run', './cmd/preview', '--listen', args.listen]
            if args.fixture:
                command += ['--fixture', str(args.fixture.resolve(strict=True))]
            return subprocess.run(command, cwd=ROOT / 'apps/remote').returncode
        if args.command == 'test':
            packages = ['./apps/build', './apps/usbsetup', './internal/buildtool', './internal/usbsetup', './packaging']
            if not args.ui:
                mode = '-race' if args.full else '-short'
                subprocess.run(['go', 'test', mode, *packages], cwd=ROOT, check=True)
                subprocess.run(['go', 'test', mode, './...'], cwd=ROOT / 'apps/remote', check=True)
            if args.full:
                subprocess.run(['go', 'vet', *packages], cwd=ROOT, check=True)
                subprocess.run(['go', 'vet', './...'], cwd=ROOT / 'apps/remote', check=True)
                subprocess.run([sys.executable, str(ROOT / 'tools/test.py')], cwd=ROOT, check=True)
            if args.full or args.ui:
                subprocess.run(['node', str(ROOT / 'tools/test-ui.mjs')], cwd=ROOT, check=True)
            return 0
        if args.command == 'build':
            if args.model != 'auto':
                require_installer(args.model)
            (ROOT / 'dist').mkdir(exist_ok=True)
            output = args.output_dir or Path(tempfile.mkdtemp(prefix='jbl-public-', dir=ROOT / 'dist'))
            return subprocess.run(['go', 'run', './apps/build', '--repo', str(ROOT),
                                   '--output-dir', str(output), '--model', args.model], cwd=ROOT).returncode
        if args.command == 'models':
            items = catalog()
            if args.json:
                print(json.dumps(list(items.values()), indent=2))
            else:
                for item in items.values():
                    print(f"{item['id']:9} {item['status']:12} {item['name']}\n          Install: {item['installation']}\n          Control: {item['control']}")
            return 0
        if args.command == 'prepare':
            return prepare(args)
        if args.command == 'usb':
            import prepare_usb
            files = prepare_usb.package_files(args.package)
            prepare_usb.inspect_mount(args.mount)
            prepare_usb.verify_files(args.mount, files)
            status = args.mount / 'JBLToolkit/STATUS.txt'
            print(status.read_text().strip() if status.is_file() else 'Prepared; no soundbar receipt yet')
            print('Reports:', args.mount / 'JBLToolkit')
            return 0
        parser.print_help()
        return 0
    except KeyboardInterrupt:
        return 130
    except (ValueError, OSError, zipfile.BadZipFile, subprocess.CalledProcessError) as error:
        print('Error:', error, file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
