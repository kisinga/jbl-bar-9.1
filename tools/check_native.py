#!/usr/bin/env python3
"""Verify project-owned native helpers; --write rebuilds them from reviewed source."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

import build_bt
import build_eq
from repo_layout import ROOT, bar91_profile

FOLDER = ROOT / 'models/bar91/remote/native'
SPECS = (('bt-remote', 'bt_remote.c', 'bluetooth', build_bt),
         ('eq-parser', 'eq_parser.c', 'eq_parser', build_eq))


def inventory():
    profile = bar91_profile()
    entries = {}
    for name, source, feature, builder in SPECS:
        entries[name] = dict(
            source=f'models/bar91/remote/native/{source}',
            source_sha256=hashlib.sha256((FOLDER/source).read_bytes()).hexdigest(),
            builder=f'tools/{builder.__name__}.py',
            builder_sha256=hashlib.sha256(Path(builder.__file__).read_bytes()).hexdigest(),
            sha256=hashlib.sha256((FOLDER/'bin'/name).read_bytes()).hexdigest(),
            dependencies=profile['native_helpers'][feature])
    return dict(schema=1, profile=profile['id'], helpers=entries, notice_sha256=hashlib.sha256((FOLDER/'bin/NOTICE.txt').read_bytes()).hexdigest())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--write', action='store_true')
    parser.add_argument('--cc')
    parser.add_argument('--rootfs', type=Path)
    args = parser.parse_args()
    manifest = FOLDER/'bin/manifest.json'
    if args.write:
        if not args.cc or not args.rootfs:
            parser.error('--write requires --cc and --rootfs')
        for name, _, _, builder in SPECS:
            builder.build(args.cc, args.rootfs, FOLDER/'bin'/name)
        report = inventory()
        report['compiler'] = subprocess.check_output([args.cc, '--version'], text=True).splitlines()[0]
        manifest.write_text(json.dumps(report, indent=2, sort_keys=True)+'\n')
    report = json.loads(manifest.read_text())
    expected = inventory()
    if any(report.get(key) != value for key,value in expected.items()):
        raise ValueError('Native source, dependencies or helper bytes changed; rebuild and review helpers')
    print('Native helper sources, dependencies and binaries verified')


if __name__ == '__main__':
    main()
