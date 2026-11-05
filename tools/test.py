#!/usr/bin/env python3
"""Run isolated host fixtures. No soundbar or vendor binaries required."""
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor
import argparse
import json
import os
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]


def run(path):
    start = time.monotonic()
    print(f'{path.name}: RUN', flush=True)
    try:
        p = subprocess.run([sys.executable, str(path)], capture_output=True, text=True, timeout=600)
    except subprocess.TimeoutExpired as error:
        print(f'{path.name}: TIMEOUT after 600s', flush=True)
        for output in (error.stdout, error.stderr):
            if output:
                print(output.decode(errors='replace') if isinstance(output, bytes) else output, flush=True)
        return path.name, time.monotonic() - start, 1
    elapsed = time.monotonic() - start
    print(f'{path.name}: {"PASS" if p.returncode == 0 else "FAIL"} ({elapsed:.1f}s)', flush=True)
    if p.returncode:
        print(p.stdout + p.stderr, flush=True)
    return path.name, elapsed, p.returncode


if __name__ == '__main__':
    suites = {p.stem.removeprefix('check_'): p for p in sorted((ROOT / 'tests').rglob('check_*.py'))}
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--suite', action='append', choices=sorted(suites), help='Run only this suite; repeatable')
    parser.add_argument('--jobs', type=int, default=4, help='Parallel fixtures (default: 4)')
    parser.add_argument('--artifacts', type=Path, help='Export verified public distributions to a new build/ directory')
    args = parser.parse_args()
    if args.jobs < 1:
        parser.error('--jobs must be positive')
    if args.artifacts:
        target = args.artifacts.resolve()
        if not target.is_relative_to((ROOT / 'build').resolve()) or target == ROOT / 'build' or target.exists():
            parser.error('--artifacts must be a new directory under build/')
        if args.suite and 'project' not in args.suite:
            parser.error('--artifacts requires the project suite')
        os.environ['JBL_TEST_ARTIFACTS'] = str(target)
    assert Path('/usr/bin/busybox').is_file(), 'Install BusyBox for host fixtures'
    out = ROOT / 'build/test-results'
    out.mkdir(parents=True, exist_ok=True)
    for p in out.glob('*-tests.json'):
        p.unlink()
    subprocess.run([sys.executable, str(ROOT / 'tools/check_profiles.py')], check=True)
    subprocess.run([sys.executable, str(ROOT / 'tools/build_release.py')], check=True)
    # Start the longest fixtures first rather than leaving them behind short checks.
    priority = {'usb': 0, 'supervisor': 1, 'project': 2}
    paths = [suites[name] for name in dict.fromkeys(args.suite or suites)]
    paths.sort(key=lambda p: (priority.get(p.stem.removeprefix('check_'), 3), str(p)))
    with ThreadPoolExecutor(max_workers=args.jobs) as pool:
        results = list(pool.map(run, paths))
    timings = {name: elapsed for name, elapsed, _ in results}
    (out / 'timings.json').write_text(json.dumps(timings, indent=2) + '\n')
    failed = [name for name, _, code in results if code]
    if failed:
        sys.exit('Failed suites: ' + ', '.join(failed))
    counts = {p.name: len(json.loads(p.read_text())['cases']) for p in sorted(out.glob('*-tests.json'))}
    # Keep suite coverage explicit without forcing contributors to edit the
    # runner every time they add a regression case. Immutable payload hashes
    # remain checked independently by build_release.py.
    required = {name + '-tests.json' for name in (
        'controller', 'descriptor', 'drain', 'once', 'setup', 'ssh',
        'supervisor', 'update', 'cli', 'profile', 'usb', 'project', 'hardware')}
    if not args.suite:
        assert required.issubset(counts), {'missing_reports': sorted(required - counts.keys())}
    assert all(count > 0 for count in counts.values()), counts
    print(json.dumps({'cases': counts, 'total': sum(counts.values())}, indent=2))
