"""Check PR routing, including mixed changes, removals and unknown paths."""
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tools'))
from ci import plan, validate_results

cases = []
def check(name, paths, expected):
    assert plan(paths) == expected, (name, plan(paths))
    cases.append(name)

full = dict(host=True, native=True, remote=True, ui='all')
none = dict(host=False, native=False, remote=False, ui='none')
remote = dict(host=False, native=False, remote=True, ui='remote')
setup = dict(host=False, native=False, remote=False, ui='setup')
check('push_and_release_run_everything', None, full)
check('empty_diff', [], none)
check('documentation_only', ['README.md', 'docs/web-remote.png'], none)
check('remote_assets', ['apps/remote/www/style.css', 'docs/REMOTE-UI.md'], remote)
check('remote_regressions', ['tests/component-ui.mjs', 'tests/fixtures/remote-ui-baseline.json'], remote)
check('setup_assets', ['packaging/wizard/app.js', 'tests/setup-ui.mjs'], setup)
check('both_interfaces', ['apps/remote/www/index.html', 'packaging/wizard/index.html'],
      dict(host=False, native=False, remote=True, ui='all'))
for name in ['apps/remote/main.go', 'internal/usbsetup/ssh.go', 'models/bar91/usb/preflight.sh',
             '.github/workflows/package.yml', 'tools/test.py', 'go.mod', 'package-lock.json',
             'tests/bar91/check_usb.py', 'new-unclassified-file', 'docs/important-script.sh']:
    check('full_for_' + name, [name], full)
check('shared_changes_override_ui_scope', ['apps/remote/www/app.js', 'go.sum'], full)
def results(scope):
    jobs = {'preflight': {'result': 'success', 'outputs': {k: str(v).lower() for k, v in scope.items()}}}
    jobs.update({name: {'result': 'success' if required else 'skipped'} for name, required in (
        ('host', scope['host']), ('remote', scope['remote']),
        ('browser', scope['ui'] != 'none'), ('setup-native', scope['native']))})
    return jobs

for scope in (none, remote, setup, full):
    assert validate_results(results(scope)) == {k: str(v).lower() for k, v in scope.items()}
cases.append('gate_accepts_only_expected_successes_and_skips')
for job, result in [('host', 'failure'), ('browser', 'cancelled'), ('setup-native', 'skipped'),
                    ('remote', 'failure'), ('preflight', 'failure')]:
    jobs = results(full)
    jobs[job]['result'] = result
    try:
        validate_results(jobs)
    except ValueError:
        pass
    else:
        raise AssertionError((job, result))
    cases.append('gate_rejects_' + job + '_' + result)
out = ROOT / 'build/test-results'
out.mkdir(parents=True, exist_ok=True)
(out / 'ci-tests.json').write_text(json.dumps({'cases': cases}, indent=2) + '\n')
print('CI routing checks:', len(cases))
