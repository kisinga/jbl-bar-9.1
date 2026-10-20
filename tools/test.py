#!/usr/bin/env python3
"""Run isolated host fixtures. No soundbar or vendor binaries required."""
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor
import json
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def run(path):
    p = subprocess.run([sys.executable, str(path)], capture_output=True, text=True, timeout=600)
    print(f'{path.name}: {"PASS" if p.returncode == 0 else "FAIL"}', flush=True)
    if p.returncode:
        print(p.stdout + p.stderr, flush=True)
        raise RuntimeError(path.name)


if __name__ == '__main__':
    assert Path('/usr/bin/busybox').is_file(), 'Install BusyBox for host fixtures'
    out = ROOT / 'build/test-results'
    out.mkdir(parents=True, exist_ok=True)
    for p in out.glob('*-tests.json'):
        p.unlink()
    subprocess.run([sys.executable, str(ROOT / 'tools/build_release.py')], check=True)
    with ThreadPoolExecutor(max_workers=4) as pool:
        list(pool.map(run, sorted((ROOT / 'tests').glob('check_*.py'))))
    counts = {p.name: len(json.loads(p.read_text())['cases']) for p in sorted(out.glob('*-tests.json'))}
    assert counts.get('ssh-tests.json') == 19, counts
    assert sum(counts.values()) == 121, counts
    print(json.dumps({'cases': counts, 'total': sum(counts.values())}, indent=2))
