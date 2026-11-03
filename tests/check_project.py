"""Model entry point, offline credential vectors and public release contracts."""
from pathlib import Path
from contextlib import redirect_stderr
import hashlib
import io
import json
import subprocess
import sys
import tempfile
from unittest.mock import patch
import zipfile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tools'))
import build_distribution
import build_ssh
import build_usb
import prepare_usb
import project

cases = []
result = subprocess.run([str(ROOT / 'jbl'), 'models', '--json'], capture_output=True, text=True, check=True)
assert {m['id'] for m in json.loads(result.stdout)} == {'bar91','bar800','bar1000','bar1300'}
cases.append('unified_model_catalog')
with patch.object(project, 'catalog', return_value={'bar1300': {'name':'Bar 1300', 'guide':'research/', 'status':'qualified'}}):
    try:
        project.require_installer('bar1300')
    except ValueError:
        pass
    else:
        raise AssertionError('catalog edit granted installer eligibility')
cases.append('research_metadata_cannot_enable_installation')

for model in ('bar1000', 'bar1300'):
    result = subprocess.run([str(ROOT / 'jbl'), 'prepare', '--model', model, '--allowed-cidr', '192.168.1.0/24'], capture_output=True, text=True)
    assert result.returncode == 1 and 'not qualified' in result.stderr
cases.append('unsupported_models_refused_before_prompt_or_output')

result = subprocess.run([str(ROOT / 'jbl'), 'dev', '--listen', '0.0.0.0:8080'], capture_output=True, text=True)
assert result.returncode == 1 and '127.0.0.1' in result.stderr
cases.append('development_preview_refuses_network_exposure_before_launch')
result = subprocess.run([str(ROOT / 'jbl'), 'build', '--model', 'bar1000'], capture_output=True, text=True)
assert result.returncode == 1 and 'not qualified' in result.stderr
cases.append('public_build_cannot_enable_a_research_model')

for host in ('8.8.8.8', '127.0.0.1', 'jbl.local', '192.168.1.5:59152'):
    with patch.object(project.subprocess, 'run') as launch, redirect_stderr(io.StringIO()):
        assert project.main(['inspect', '--host', host]) == 1
        launch.assert_not_called()
cases.append('inspection_refuses_nonprivate_or_ambiguous_targets_before_launch')

with tempfile.TemporaryDirectory(prefix='inspection-refusal-') as temp:
    with patch.object(project.subprocess, 'run') as launch, redirect_stderr(io.StringIO()):
        assert project.main(['inspect', '--host', '192.168.1.5', '--output-dir', str(Path(temp)/'capture')]) == 1
        launch.assert_not_called()
private = ROOT/'research/private'; private.mkdir(parents=True, exist_ok=True)
with tempfile.TemporaryDirectory(prefix='inspection-cli-', dir=private) as temp:
    with patch.object(project.subprocess, 'run') as launch, redirect_stderr(io.StringIO()):
        assert project.main(['inspect', '--host', '192.168.1.5', '--output-dir', temp]) == 1
        launch.assert_not_called()
cases.append('inspection_refuses_public_output_and_existing_sessions_before_launch')

with tempfile.TemporaryDirectory(prefix='inspection-cli-', dir=private) as temp:
    output = Path(temp)/'new-session'
    with patch.object(project.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0)) as launch:
        assert project.main(['inspect', '--host', '192.168.1.5', '--output-dir', str(output)]) == 0
        assert launch.call_args.args[0] == ['go', 'run', './cmd/inspect', '--host', '192.168.1.5', '--output-dir', str(output)]
        assert launch.call_args.kwargs['cwd'] == ROOT/'apps/remote'
        assert not output.exists()  # The collector, not the launcher, creates the session.
cases.append('inspection_launches_the_fixed_go_collector_without_a_shell')

# Test the exact offline JS shipped in the ZIP against the independent Python
# implementation, including UTF-8. No browser or network needed in CI.
script = '''
globalThis.crypto = require('node:crypto').webcrypto;
const setup = require('./packaging/setup.js');
(async () => {
 const data = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
 const hashes = []; for (const password of data.passwords) hashes.push(await setup.passwordHash(password,'fixturesalt'));
 const wifi = await setup.wifiCredentials(data.ssid,data.wifiPassword);
 const config = await setup.configuration({address:'192.168.9.138',prefix:24,action:'check',web:true,ssh:true,standard:false,password:data.passwords[0],ssid:data.ssid,wifiPassword:data.wifiPassword});
 let rejected=0;
 for (const ip of ['8.8.8.8','192.168.001.2','192.168.1.999','$(id)']) { try { setup.subnet(ip,24); } catch { rejected++; } }
 console.log(JSON.stringify({hashes,wifi,config,rejected}));
})().catch(e => { console.error(e); process.exit(1); });
'''
fixture = {'passwords':['fixture-password','日本語-fixture-password'], 'ssid':'家 fixture', 'wifiPassword':'fixture-wifi-password'}
result = subprocess.run(['node','-e',script], cwd=ROOT, input=json.dumps(fixture), text=True, capture_output=True, check=True)
browser = json.loads(result.stdout)
assert browser['hashes'] == [build_ssh.password_hash(p,'fixturesalt') for p in fixture['passwords']]
ssid, psk = build_ssh.wifi_credentials(fixture['ssid'],fixture['wifiPassword'])
assert browser['wifi'] == {'ssid':ssid,'psk':psk} and browser['rejected']==4
settings = build_usb.parse_config(browser['config'].encode())
assert settings['ALLOWED_CIDR']=='192.168.9.0/24' and settings['SSH_PASSWORD_HASH'].startswith('$6$')
assert settings['WIFI_PSK_HEX']==psk
assert all(p not in browser['config'] for p in fixture['passwords']+[fixture['wifiPassword']])
cases.append('offline_setup_crypt_wifi_and_network_known_vectors')

base = build_distribution.CONFIG
for extra in ('ssh_password_hash=$(id)', 'ssh_password_hash=$6$abc$bad', 'wifi_ssid_hex=ff', 'wifi_ssid_hex=00\nwifi_psk_hex='+'a'*63, 'wifi_ssid_hex=$(id)\nwifi_psk_hex='+'a'*64):
    try:
        build_usb.parse_config(base+extra.encode()+b'\n')
    except ValueError:
        pass
    else:
        raise AssertionError('unsafe credentials parsed')
cases.append('credential_alphabets_lengths_and_pairs_checked')

(ROOT/'dist').mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix='distribution-test-',dir=ROOT/'dist') as temp:
    output = Path(temp)/'public'
    paths = build_distribution.build(output)
    assert len(paths)==4
    package, recovery, info, sums = paths
    files = prepare_usb.package_files(package)
    assert build_usb.parse_config(files['JBLToolkit/config.txt'])['SSH']=='false'
    assert files['JBLToolkit/setup.js']==(ROOT/'packaging/setup.js').read_bytes()
    assert files['SETUP.html']==(ROOT/'packaging/SETUP.html').read_bytes()
    assert not any(name.endswith(('/password.hash', '/wifi.ssid.hex', '/wifi.psk.hex')) for name in files)
    with zipfile.ZipFile(recovery) as archive:
        assert not any(name.endswith('/remote') or 'password' in name or 'wifi.' in name for name in archive.namelist())
        assert archive.read('JBLToolkit/RECOVER')==b'RECOVER\n'
    for line in sums.read_text().splitlines():
        digest, name = line.split('  ',1)
        assert hashlib.sha256((output/name).read_bytes()).hexdigest()==digest
    assert json.loads(info.read_text())['qualification']=='hardware-test-pending'
    assert json.loads(info.read_text())['selection']=='automatic'
    assert json.loads(info.read_text())['included_models']==['bar91']
    assert files['AutoScript/AutoScript'].endswith(b'/dispatch.sh &)\r\n')
    with tempfile.TemporaryDirectory(prefix='distribution-extract-') as mount:
        prepare_usb.stage_files(Path(mount),files)
        (Path(mount)/'JBLToolkit/config.txt').write_text(browser['config'])
        prepare_usb.verify_files(Path(mount),files)
    try:
        build_distribution.build(output)
    except ValueError:
        pass
    else:
        raise AssertionError('public package overwritten')
    specific = build_distribution.build(Path(temp)/'bar91', 'bar91')
    assert {p.name for p in paths}.isdisjoint(p.name for p in specific)
    assert json.loads(specific[2].read_text())['target']=='bar91'
    assert prepare_usb.package_files(specific[0])['AutoScript/AutoScript']==files['AutoScript/AutoScript']
cases.append('public_arm_zip_setup_recovery_checksums_and_local_config')
cases.append('automatic_and_model_specific_public_assets_do_not_collide')

matrix = subprocess.run([sys.executable, str(ROOT/'tools/build_distribution.py'), '--matrix'],capture_output=True,text=True,check=True)
assert {item['model'] for item in json.loads(matrix.stdout)['include']}=={'auto','bar91'}
cases.append('ci_packages_only_reviewed_implementations_and_automatic_selection')

out = ROOT/'build/test-results'; out.mkdir(parents=True,exist_ok=True)
(out/'project-tests.json').write_text(json.dumps({'scope':'Host-only model, credential vector and distribution checks.', 'cases':cases},indent=2)+'\n')
print('Project checks:',len(cases))
