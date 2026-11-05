#!/usr/bin/env python3
"""Select conservative PR checks; pushes, releases and unknown paths run everything."""
import argparse
import json
import os
from pathlib import Path
import subprocess


def plan(paths=None):
    full = dict(host=True, native=True, remote=True, ui='all')
    if paths is None:
        return full
    selected = dict(host=False, native=False, remote=False, ui='none')
    ui = set()
    for name in paths:
        if name.endswith('.md') or (name.startswith('docs/') and name.endswith(('.png', '.svg'))):
            continue
        if name.startswith('apps/remote/www/') or name in (
            'tests/webremote-ui.mjs', 'tests/component-ui.mjs',
            'tests/bass-offset-ui.mjs', 'tests/fixtures/remote-ui-baseline.json',
        ):
            selected['remote'] = True  # Includes an ARM build of embedded assets.
            ui.add('remote')
        elif name.startswith('packaging/wizard/') or name in ('tests/setup-ui.mjs', 'tests/setup-local.mjs'):
            ui.add('setup')
        else:
            return full  # Shared code, tooling, workflows and new paths fail closed.
    selected['ui'] = 'all' if len(ui) == 2 else next(iter(ui), 'none')
    return selected


def validate_results(jobs):
    if jobs['preflight']['result'] != 'success':
        raise ValueError('Preflight failed')
    scope = jobs['preflight']['outputs']
    expected = {'host': scope['host'] == 'true', 'remote': scope['remote'] == 'true',
                'browser': scope['ui'] != 'none', 'setup-native': scope['native'] == 'true'}
    for name, required in expected.items():
        result = jobs[name]['result']
        if result != ('success' if required else 'skipped'):
            raise ValueError(f'{name}: {result}')
    return scope


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--github-output', type=Path)
    parser.add_argument('--base', help='Compare this ref with --head locally')
    parser.add_argument('--head', default='HEAD')
    parser.add_argument('--verify', action='store_true', help='Verify selected CI results from NEEDS')
    args = parser.parse_args()
    if args.verify:
        scope = validate_results(json.loads(os.environ['NEEDS']))
        print('All selected checks passed: ' + json.dumps(scope, sort_keys=True))
        if summary := os.environ.get('GITHUB_STEP_SUMMARY'):
            with open(summary, 'a') as stream:
                stream.write('Selected checks: ' + json.dumps(scope, sort_keys=True) + '\n')
        return
    base = args.base
    head = args.head
    if not base and os.environ.get('GITHUB_EVENT_NAME') == 'pull_request':
        event = json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text())
        base = event['pull_request']['base']['sha']
        head = event['pull_request']['head']['sha']
    paths = None
    if base:
        result = subprocess.run(['git', 'diff', '--name-only', '-z', '--no-renames', base + '...' + head],
                                capture_output=True, check=True)
        paths = result.stdout.decode().strip('\0').split('\0') if result.stdout else []
    selected = plan(paths)
    print(json.dumps(selected, sort_keys=True))
    if args.github_output:
        with args.github_output.open('a') as stream:
            for name, value in selected.items():
                stream.write(f'{name}={str(value).lower()}\n')


if __name__ == '__main__':
    main()
