#!/usr/bin/env python3
"""Build deterministic USB archives, preserving the tested device bytes."""
from pathlib import Path
import hashlib
import json
import re
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
VERSION = '0.2.0-dev.1'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def validate():
    source = ROOT / 'smart-mode'
    files = {str(p.relative_to(source)): p.read_bytes()
             for p in sorted(source.rglob('*')) if p.is_file()}
    baseline = json.loads((ROOT / 'tests/fixtures/revision6-sha256.json').read_text())
    assert {n: sha(v) for n, v in files.items()} == baseline, 'Pinned candidate device bytes changed'
    payload = source / 'JBLInstall/payload'
    checks = (payload / 'checksums.txt').read_bytes()
    for line in checks.decode().splitlines():
        digest, name = line.split('  ', 1)
        assert '/' not in name and sha((payload / name).read_bytes()) == digest, name
    for name in ('install.sh', 'setup.sh'):
        text = (source / 'JBLInstall' / name).read_text()
        expected = re.search(r'^EXPECTED_MANIFEST=(\w+)$', text, re.M).group(1)
        assert expected == sha(checks), name
    hook = (payload / 'hook.expected').read_bytes()
    for name in ('install.sh', 'uninstall.sh'):
        text = (source / 'JBLInstall' / name).read_text()
        assert re.search(r'^EXPECTED_HOOK=(\w+)$', text, re.M).group(1) == sha(hook)
    controller = (payload / 'controller.sh').read_text()
    assert sha((payload / 'helper.sh').read_bytes()) in controller
    confirmed = json.loads((ROOT / 'tests/fixtures/revision5-sha256.json').read_text())
    dispatcher = (source / 'JBLInstall/install.sh').read_text()
    assert re.search(r'^OLD_MANIFEST=(\w+)$', dispatcher, re.M).group(1) == confirmed['JBLInstall/payload/checksums.txt']
    for name in ('JBLInstall/payload/controller.sh', 'JBLInstall/payload/helper.sh',
                 'JBLInstall/payload/hook.expected', 'AutoScript/AutoScript'):
        assert sha(files[name]) == confirmed[name], 'Key/action or hook changed: ' + name
    for name in ('AutoScript/AutoScript', 'JBLInstall/payload/hook.expected'):
        assert files[name].endswith(b'\r\n') and files[name].count(b'\n') == 1
    for p in source.rglob('*.sh'):
        subprocess.run(['/bin/sh', '-n', str(p)], check=True)
    assert 'JBLInstall/ROLLBACK' not in files
    return files


def build():
    files = validate()
    files['README.txt'] = (ROOT / 'USB-README.txt').read_bytes()
    files['ROLLBACK.txt'] = (ROOT / 'ROLLBACK.txt').read_bytes()
    files['LICENSE.txt'] = (ROOT / 'LICENSE').read_bytes()
    out = ROOT / 'dist'
    out.mkdir(exist_ok=True)
    archives = []
    for kind in ('install', 'uninstall'):
        content = dict(files)
        if kind == 'uninstall':
            content['JBLInstall/ROLLBACK'] = b'ROLLBACK\n'
        manifest = {'project': 'jbl-bar9-standard-mode', 'version': VERSION,
                    'device_revision': 6, 'kind': kind,
                    'status': 'experimental; one-bar SSH wake timing validated',
                    'files': {n: sha(v) for n, v in sorted(content.items())}}
        content['manifest.json'] = (json.dumps(manifest, indent=2) + '\n').encode()
        path = out / f'jbl-bar9-standard-mode-v{VERSION}-{kind}.zip'
        with zipfile.ZipFile(path, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            for name, data in sorted(content.items()):
                item = zipfile.ZipInfo(name, (2026, 10, 19, 0, 0, 0))
                item.create_system = 3
                item.external_attr = 0o100644 << 16
                item.compress_type = zipfile.ZIP_DEFLATED
                z.writestr(item, data, compresslevel=9)
        with zipfile.ZipFile(path) as z:
            assert z.testzip() is None
            assert set(z.namelist()) == set(content)
            for name, data in content.items():
                assert z.read(name) == data
        archives.append(path)
        print(path.name, path.stat().st_size, 'bytes')
    (out / 'SHA256SUMS').write_text(''.join(f'{sha(p.read_bytes())}  {p.name}\n' for p in archives))


if __name__ == '__main__':
    build()
