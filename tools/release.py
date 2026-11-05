#!/usr/bin/env python3
"""Validate release intent early and publish only complete, verified public assets."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile

import build_usb
import hardware
from repo_layout import ROOT


# Independent publication inventory; the shared Go builder owns production output.
SETUP_TARGETS = (("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"),
                 ("darwin", "arm64"), ("windows", "amd64"), ("windows", "arm64"))


def setup_binary_name(system, arch):
    return f'jbl-setup-{system}-{arch}' + ('.exe' if system == 'windows' else '')


def setup_asset_names(version):
    return [name for system, arch in SETUP_TARGETS for name in
            (setup_binary_name(system, arch), f'jbl-bar-toolkit-v{version}-setup-{system}-{arch}.zip')]


def source_commit():
    return subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()


def release_notes(changelog=None):
    text = (changelog or ROOT / 'CHANGELOG.md').read_text()
    version = build_usb.VERSION
    if not re.fullmatch(r'\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?', version):
        raise ValueError('Invalid release version in VERSION')
    sections = re.split(r'^## ', text, flags=re.M)[1:]
    matches = [section for section in sections if section.splitlines()[0] == version]
    if len(matches) != 1:
        raise ValueError(f'CHANGELOG.md needs exactly one ## {version} section')
    notes = matches[0].partition('\n')[2].strip()
    if not notes or not re.search(r'^- \S', notes, re.M) or re.search(r'\b(TODO|TBD)\b', notes):
        raise ValueError(f'Add user-facing changes to CHANGELOG.md under ## {version}')
    return notes + '\n\n' + (ROOT / 'packaging/RELEASE-NOTES.md').read_text()


def verify_assets(root, commit):
    expected = set()
    for target in hardware.matrix()['include']:
        model = target['model']
        prefix = '' if model == 'auto' else model + '-'
        stem = f'jbl-{"bar-toolkit" if model == "auto" else model}-v{build_usb.VERSION}'
        names = {stem + '-usb.zip', stem + '-recovery.zip', prefix + 'release.json'}
        if model == 'auto': names.update(setup_asset_names(build_usb.VERSION))
        manifest = prefix + 'SHA256SUMS'
        expected.update(names | {manifest})
        report = json.loads((root / (prefix + 'release.json')).read_text())
        if report['version'] != build_usb.VERSION or report['source_commit'] != commit or report['target'] != model:
            raise ValueError('Release metadata does not match the version, commit and target')
        if set(report['assets']) != names - {prefix + 'release.json'}:
            raise ValueError('Release metadata has an unexpected asset list')
        lines = (root / manifest).read_text().splitlines()
        listed = set()
        for line in lines:
            digest, name = line.split('  ', 1)
            if name not in names or name in listed:
                raise ValueError('Unexpected or duplicate checksum entry')
            if hashlib.sha256((root / name).read_bytes()).hexdigest() != digest:
                raise ValueError(f'Checksum mismatch: {name}')
            listed.add(name)
        if listed != names:
            raise ValueError(f'Incomplete checksums: {manifest}')
    if {p.name for p in root.iterdir()} != expected:
        raise ValueError('Missing or unexpected public release assets')
    return sorted(root / name for name in expected)


def publish(root, draft=False):
    commit = source_commit()
    notes = release_notes()
    assets = verify_assets(root, commit)
    tag = 'v' + build_usb.VERSION
    # Never attach these bytes to a tag that points at another revision.
    existing_tag = subprocess.run(['git', 'rev-parse', '--verify', f'refs/tags/{tag}^{{commit}}'],
                                  cwd=ROOT, capture_output=True, text=True)
    if existing_tag.returncode == 0 and existing_tag.stdout.strip() != commit:
        raise ValueError(f'{tag} already points to another commit; bump VERSION and add a changelog entry')
    existing = subprocess.run(['gh', 'release', 'view', tag, '--json', 'isDraft,targetCommitish'],
                              cwd=ROOT, capture_output=True, text=True)
    with tempfile.TemporaryDirectory(prefix='jbl-release-') as temp:
        notes_file = Path(temp) / 'notes.md'
        notes_file.write_text(notes)
        if existing.returncode == 0:
            record = json.loads(existing.stdout)
            if not record['isDraft']:
                raise ValueError(f'{tag} is already published; published assets are never replaced')
            if record['targetCommitish'] != commit:
                raise ValueError('Existing draft belongs to another commit; use a new version')
            # Only an unfinished draft may be repaired after a failed upload.
            subprocess.run(['gh', 'release', 'upload', tag, *map(str, assets), '--clobber'], cwd=ROOT, check=True)
        else:
            subprocess.run(['gh', 'release', 'create', tag, *map(str, assets), '--target', commit,
                            '--draft', '--prerelease', '--title', f'JBL Bar Toolkit {tag}',
                            '--notes-file', str(notes_file)], cwd=ROOT, check=True)
        subprocess.run(['gh', 'release', 'edit', tag, '--notes-file', str(notes_file), '--prerelease', '--latest=false',
                        '--draft=' + str(draft).lower()], cwd=ROOT, check=True)
    print(f'{"Draft" if draft else "Published prerelease"}: {tag} ({commit})')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['check', 'publish'])
    parser.add_argument('--assets', type=Path, default=ROOT / 'release-assets')
    parser.add_argument('--draft', action='store_true')
    parser.add_argument('--tag', help='Validate a pushed version tag before running tests')
    args = parser.parse_args()
    try:
        release_notes()
        if args.tag and args.tag != 'v' + build_usb.VERSION:
            raise ValueError('Tag/version mismatch: expected v' + build_usb.VERSION)
        if args.command == 'publish':
            publish(args.assets, args.draft)
        else:
            print('Release intent valid: v' + build_usb.VERSION)
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f'Release failed: {error}\n')


if __name__ == '__main__':
    main()
