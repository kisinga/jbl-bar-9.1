"""Automatic USB selection with synthetic identities; no device is contacted."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tools'))
import build_ssh
import hardware

cases = []
profile = hardware.profile('bar91')
assert {item['model'] for item in hardware.matrix()['include']} == {'auto', 'bar91'}
for model in ('bar800', 'bar1000', 'bar1300', 'bar1300x', 'mk2'):
    try:
        hardware.dispatcher(model)
    except ValueError:
        pass
    else:
        raise AssertionError('research entry granted an installer')
cases.append('release_matrix_and_dispatcher_exclude_unimplemented_hardware')

for scenario in ('matched', 'wrong_bdp', 'wrong_cli', 'missing_process', 'multiple_processes',
                 'corrupt_bootstrap', 'symlink_bootstrap', 'owned_recovery', 'unknown_owner_recovery'):
    with tempfile.TemporaryDirectory(prefix='hardware-dispatch-') as temp:
        folder = Path(temp)
        usb, store = folder/'usb', folder/'store'
        usb.mkdir()
        dispatch = hardware.dispatcher('bar91')
        (usb/'dispatch.sh').write_bytes(dispatch)
        (usb/'launch.sh').write_text('#!/bin/sh\nprintf "selected\\n" > "$JBL_TOOLKIT_USB/selected"\n')
        (usb/'common.sh').write_text('# fixture common\n')
        (usb/'recover.sh').write_text('# fixture recovery\n')
        names = ('dispatch.sh', 'launch.sh', 'common.sh', 'recover.sh')
        (usb/'BOOTSTRAP.SHA256SUMS').write_bytes(build_ssh.manifest({n:(usb/n).read_bytes() for n in names}))
        if scenario.endswith('recovery'):
            (usb/'RECOVER').write_text('RECOVER\n')
            store.mkdir()
            (store/'OWNER.txt').write_text('JBL_TOOLKIT_REV1\n' if scenario=='owned_recovery' else 'unknown\n')
        if scenario=='corrupt_bootstrap':
            (usb/'launch.sh').write_text('exit 99\n')
        if scenario=='symlink_bootstrap':
            (usb/'common.sh').unlink()
            (usb/'common.sh').symlink_to(usb/'recover.sh')
        wrapper=folder/'busybox'
        wrapper.write_text('''#!/bin/sh
case "$1" in
 pidof)
   case "$JBL_TEST_SCENARIO" in
     missing_process) exit 1;;
     multiple_processes) printf '123 456\\n';;
     *) printf '123\\n';;
   esac;;
 sha256sum)
   case "$2" in
     /proc/123/exe)
       case "$JBL_TEST_SCENARIO" in wrong_bdp|owned_recovery|unknown_owner_recovery) printf 'bad  exe\\n';;
         *) printf '%s  exe\\n' '''+profile['executables']['bdpprog']+''';; esac;;
     /bin/cli)
       if [ "$JBL_TEST_SCENARIO" = wrong_cli ]; then printf 'bad  cli\\n';
       else printf '%s  cli\\n' '''+profile['executables']['cli']+'''; fi;;
     *) exec /usr/bin/busybox "$@";;
   esac;;
 *) exec /usr/bin/busybox "$@";;
esac
''')
        wrapper.chmod(0o700)
        env=dict(os.environ,JBL_TOOLKIT_USB=str(usb),JBL_TOOLKIT_STORE=str(store),
                 JBL_BUSYBOX=str(wrapper),JBL_TEST_SCENARIO=scenario,MODEL='bar91')
        result=subprocess.run(['/bin/sh',str(usb/'dispatch.sh')],env=env,capture_output=True,timeout=10)
        success=scenario in ('matched','owned_recovery')
        assert (result.returncode==0)==success,(scenario,result.stderr)
        assert (usb/'selected').exists()==success,scenario
        report=(usb/'HARDWARE.txt').read_text()
        assert ('selection=bar91\n' if success else 'selection=unknown\n') in report,(scenario,report)
        if not scenario.endswith('recovery'):
            assert not store.exists(), 'detection created a persistent store'
        cases.append('automatic_selection_'+scenario)

out=ROOT/'build/test-results';out.mkdir(parents=True,exist_ok=True)
(out/'hardware-tests.json').write_text(json.dumps({'scope':'Host selection/ownership fixtures only; no physical USB bootstrap qualification.','cases':cases},indent=2)+'\n')
print('Hardware dispatch checks:',len(cases))
