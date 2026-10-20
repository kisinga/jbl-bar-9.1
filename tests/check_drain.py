from pathlib import Path
import tempfile,subprocess,hashlib,json,shutil,re
base=Path(__file__).resolve().parents[1];package=base/'smart-mode/JBLInstall'
results=[]
cases=['watch_drain','once_drain','both_drain','watch_stuck']
for case in cases:
 with tempfile.TemporaryDirectory(prefix='jbl-install-fixture-') as temp:
  t=Path(temp);usb=t/'usb';ubi=t/'ubi';var=ubi/'var';mem=t/'ram';proc=t/'proc';dest=var/'JBLStandard';hook=var/'AutoScript.TSS';vlink=t/'var'
  for p in [usb,var,mem,proc]:p.mkdir(parents=True)
  vlink.symlink_to(var if case!='wrong_var' else mem,target_is_directory=True)
  (proc/'sys/kernel/random').mkdir(parents=True);(proc/'sys/kernel/random/boot_id').write_text('fixture-boot\n');(proc/'uptime').write_text('100 100\n')
  mounts=f'tmpfs /tmp tmpfs rw 0 0\nubi0:ubi_boot {ubi} ubifs '+('ro' if case=='readonly' else 'rw')+' 0 0\n'
  if case=='nested_tmpfs':mounts+=f'tmpfs {var} tmpfs rw 0 0\n'
  (proc/'mounts').write_text(mounts)
  bb=t/'busybox';bb.write_text('''#!/usr/bin/python3
from pathlib import Path
import os,sys
root=Path(__file__).parent
case=CASE
args=sys.argv[1:]
if args[0]=='sleep':
 n=int((root/'waits').read_text())+1 if (root/'waits').exists() else 1
 (root/'waits').write_text(str(n))
 if n==3 and case!='watch_stuck':
  for p in [root/'ram/watch',root/'ram/active']:
   if p.exists():p.rmdir()
 raise SystemExit()
if args[0]=='cp' and case=='copy_failure' and any('/JBLStandard/' in a for a in args[1:]):raise SystemExit(1)
if args[0]=='ln' and '-T' in args:
 if case=='link_failure':raise SystemExit(1)
 if case=='directory_race':(root/'ubi/var/AutoScript.TSS').mkdir()
os.execv('/usr/bin/busybox',['busybox']+args)
'''.replace('CASE',repr(case)));bb.chmod(0o700)
  paths={'/mnt/sda1/JBLInstall':str(usb),'/mnt/ubi_boot/var':str(var),'/mnt/ubi_boot':str(ubi),'/var/JBLStandard':str(vlink/'JBLStandard'),'/var/AutoScript.TSS':str(vlink/'AutoScript.TSS'),'/var':str(vlink),'/tmp/jbl-standard-mutation-active':str(mem/'mutation'),'/tmp/jbl-standard-hook-active':str(mem/'active'),'/tmp/jbl-standard-watch-active':str(mem/'watch'),'/tmp/jbl-standard-keeper':str(mem/'old'),'/proc':str(proc)}
  def adapt(s):
   items=sorted(paths.items(),key=lambda x:-len(x[0]))
   for i,(old,new) in enumerate(items):s=s.replace(old,f'@@PATH{i}@@')
   for i,(old,new) in enumerate(items):s=s.replace(f'@@PATH{i}@@',new)
   return s.replace('BB=/bin/busybox','BB='+str(bb))
  shutil.copytree(package/'payload',usb/'payload')
  # Stub only the hardware trial; transaction/rollback scripts are actual source.
  runner='''#!/bin/sh
if [ CASE = stale_trial ]; then exit 0; fi
if [ CASE = failed_trial ]; then echo INPUT_RELEASE_NOT_CONFIRMED > "$1/STATUS.txt"; exit 1; fi
echo STANDARD_SELECTED > "$1/STATUS.txt"
echo fixture > "$1/latest.log"
'''.replace('CASE',case)
  (usb/'payload/once.sh').write_text(runner)
  (usb/'payload/run.sh').write_text('#!/bin/sh\nexit 0\n')
  checks=''.join(hashlib.sha256((usb/'payload'/n).read_bytes()).hexdigest()+'  '+n+'\n' for n in ['run.sh','once.sh','controller.sh','helper.sh','hook.expected','OWNER.txt'])
  (usb/'payload/checksums.txt').write_text(checks)
  ins=adapt((package/'setup.sh').read_text());ins=re.sub(r'^EXPECTED_MANIFEST=.*$', 'EXPECTED_MANIFEST='+hashlib.sha256(checks.encode()).hexdigest(),ins,flags=re.M)
  (usb/'install.sh').write_text(ins);(usb/'uninstall.sh').write_text(adapt((package/'uninstall.sh').read_text()));(usb/'collect-storage.sh').write_text('exit 0\n')
  if case=='existing_hook':hook.write_text('preserve me')
  if case=='hook_symlink':hook.symlink_to(t/'absent')
  if case=='bad_payload':(usb/'payload/helper.sh').write_text('changed')
  if case=='stale_trial':(usb/'trial').mkdir();(usb/'trial/STATUS.txt').write_text('STANDARD_SELECTED\n')
  if case=='mutation_busy':(mem/'mutation').mkdir()
  if case=='unowned_destination':dest.mkdir();(dest/'mine').write_text('preserve me')
  result=subprocess.run(['/bin/sh',str(usb/'install.sh')],capture_output=True,text=True,timeout=25)
  status=(usb/'INSTALL_STATUS.txt').read_text().strip() if (usb/'INSTALL_STATUS.txt').exists() else 'NO_STATUS'
  successful=True
  assert (status=='INSTALLED_PENDING_REBOOT_TEST')==successful,(case,status,result.stderr,(usb/'install.log').read_text() if (usb/'install.log').exists() else '')
  if successful:
   assert hook.read_bytes()==(usb/'payload/hook.expected').read_bytes() and dest.is_dir()
   if case=='already_installed':
    (dest/'latest.log').write_text('runtime-log')
    again=subprocess.run(['/bin/sh',str(usb/'install.sh')],capture_output=True,text=True,timeout=15)
    assert (usb/'INSTALL_STATUS.txt').read_text().strip()=='ALREADY_INSTALLED_LOGS_EXPORTED'
    assert (usb/'runtime/latest.log').read_text()=='runtime-log'
   if case=='changed_hook':hook.write_text('changed hook preserve')
   if case in ['watch_drain','both_drain','watch_stuck']:(mem/'watch').mkdir()
   if case in ['once_drain','both_drain']:(mem/'active').mkdir()
   undo=subprocess.run(['/bin/sh',str(usb/'uninstall.sh')],capture_output=True,text=True,timeout=15)
   undo_status=(usb/'UNINSTALL_STATUS.txt').read_text().strip()
   if case=='watch_stuck':
    assert undo_status=='HOOK_DISABLED_REBOOT_TO_FINISH_UNINSTALL' and dest.exists() and not hook.exists() and (dest/'DISABLED').exists()
    assert int((t/'waits').read_text())==90
   elif case=='changed_hook':assert undo_status=='CHANGED_HOOK_PRESERVED' and dest.exists() and hook.read_text()=='changed hook preserve'
   else:
    assert int((t/'waits').read_text())==3
    assert undo_status=='UNINSTALLED' and not dest.exists() and not hook.exists(),(case,undo_status,undo.stderr)
    again=subprocess.run(['/bin/sh',str(usb/'uninstall.sh')],capture_output=True,text=True,timeout=15)
    assert (usb/'UNINSTALL_STATUS.txt').read_text().strip()=='ALREADY_UNINSTALLED'
  else:
   if case!='unowned_destination':assert not dest.exists(),(case,status)
   if case=='existing_hook':assert hook.read_text()=='preserve me'
   elif case=='hook_symlink':assert hook.is_symlink()
   elif case=='directory_race':assert hook.is_dir() and not list(hook.iterdir())
   else:assert not hook.exists()
  results.append({'case':case,'install_status':status,'exit':result.returncode})
  print('PASS',case,status,flush=True)
report={'scope':'Host filesystem transaction fixtures; mocked hardware trial and mount metadata. Actual installer/rollback scripts adapted to temp paths; no device install proven.','installer_sha256':hashlib.sha256((package/'setup.sh').read_bytes()).hexdigest(),'uninstaller_sha256':hashlib.sha256((package/'uninstall.sh').read_bytes()).hexdigest(),'cases':results}
(base/'build/test-results/drain-tests.json').write_text(json.dumps(report,indent=2)+'\n')
