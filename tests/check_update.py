from pathlib import Path
import tempfile,subprocess,hashlib,json,shutil,re
base=Path(__file__).resolve().parents[1];package=base/'usb/JBLInstall'
results=[]
cases=['upgrade','changed_old_manifest','changed_old_files','changed_old_hook','unknown_owner','failed_trial','bad_new_payload','rollback_rev4','outer_busy']
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
if args[0]=='cp' and case=='copy_failure' and any('/JBLStandard/' in a for a in args[1:]):raise SystemExit(1)
if args[0]=='ln' and '-T' in args:
 if case=='link_failure':raise SystemExit(1)
 if case=='directory_race':(root/'ubi/var/AutoScript.TSS').mkdir()
os.execv('/usr/bin/busybox',['busybox']+args)
'''.replace('CASE',repr(case)));bb.chmod(0o700)
  paths={'/mnt/sda1/JBLInstall':str(usb),'/mnt/ubi_boot/var':str(var),'/mnt/ubi_boot':str(ubi),'/var/JBLStandard':str(vlink/'JBLStandard'),'/var/AutoScript.TSS':str(vlink/'AutoScript.TSS'),'/var':str(vlink),'/tmp/jbl-standard-usb-operation':str(mem/'usb-operation'),'/tmp/jbl-standard-mutation-active':str(mem/'mutation'),'/tmp/jbl-standard-hook-active':str(mem/'active'),'/tmp/jbl-standard-watch-active':str(mem/'watch'),'/tmp/jbl-standard-keeper':str(mem/'old'),'/proc':str(proc)}
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
  (usb/'setup.sh').write_text(ins);
  outer=adapt((package/'install.sh').read_text());outer=re.sub(r'^EXPECTED_MANIFEST=.*$', 'EXPECTED_MANIFEST='+hashlib.sha256(checks.encode()).hexdigest(),outer,flags=re.M)
  (usb/'install.sh').write_text(outer);(usb/'uninstall.sh').write_text(adapt((package/'uninstall.sh').read_text()));(usb/'collect-storage.sh').write_text('exit 0\n')
  shutil.copytree(package/'payload',dest)
  (dest/'OWNER.txt').write_text('JBL_STANDARD_HOOK_REV4\n')
  old_checks=''.join(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.name+'\n' for f in sorted(dest.iterdir()) if f.name!='checksums.txt')
  (dest/'checksums.txt').write_text(old_checks)
  # Synthetic old payload: test the transaction, not a vendor/old release binary.
  outer=re.sub(r'^OLD_MANIFEST=.*$', 'OLD_MANIFEST='+hashlib.sha256(old_checks.encode()).hexdigest(),outer,flags=re.M)
  (usb/'install.sh').write_text(outer)
  shutil.copyfile(dest/'hook.expected',hook)
  (dest/'latest.log').write_text('prior device evidence')
  if case=='changed_old_manifest':(dest/'checksums.txt').write_text('unknown manifest')
  if case=='changed_old_files':(dest/'run.sh').write_text('changed')
  if case=='changed_old_hook':hook.write_text('changed')
  if case=='unknown_owner':(dest/'OWNER.txt').write_text('someone else')
  if case=='bad_new_payload':(usb/'payload/once.sh').write_text('changed')
  if case=='rollback_rev4':(usb/'ROLLBACK').touch()
  if case=='outer_busy':(mem/'usb-operation').mkdir()
  result=subprocess.run(['/bin/sh',str(usb/'install.sh')],capture_output=True,text=True,timeout=25)
  status=(usb/'UPDATE_STATUS.txt').read_text().strip() if (usb/'UPDATE_STATUS.txt').exists() else 'NO_STATUS'
  if case=='upgrade':
   assert result.returncode==0 and status=='SETUP_FINISHED',(case,status,result.stderr)
   assert (dest/'OWNER.txt').read_text().strip()=='JBL_STANDARD_HOOK_REV5'
   assert hook.read_bytes()==(dest/'hook.expected').read_bytes()
   assert (usb/'uninstalled-backup/JBLStandard/latest.log').read_text()=='prior device evidence'
   again=subprocess.run(['/bin/sh',str(usb/'install.sh')],capture_output=True,text=True,timeout=25)
   assert again.returncode==0 and (usb/'INSTALL_STATUS.txt').read_text().strip()=='ALREADY_INSTALLED_LOGS_EXPORTED'
  elif case in ['failed_trial','rollback_rev4']:
   assert not dest.exists() and not hook.exists(),(case,status)
   assert (usb/'uninstalled-backup/JBLStandard/OWNER.txt').read_text().strip()=='JBL_STANDARD_HOOK_REV4'
   if case=='failed_trial':assert (usb/'INSTALL_STATUS.txt').read_text().strip()=='TRIAL_NOT_SUCCESSFUL_NO_INSTALL' and result.returncode==1
  else:
   assert dest.exists() and hook.exists(),(case,status)
   assert not (usb/'uninstalled-backup').exists(),(case,status)
   if case!='outer_busy':assert result.returncode==1,(case,status)
  results.append({'case':case,'update_status':status,'exit':result.returncode});print('PASS',case,status,flush=True)
report={'scope':'Actual dispatcher/setup/uninstaller with synthetic revision4 payload with a fixture-only expected manifest and stubbed new hardware trial; temp filesystem and mount metadata.','dispatcher_sha256':hashlib.sha256((package/'install.sh').read_bytes()).hexdigest(),'cases':results}
(base/'build/test-results/update-tests.json').write_text(json.dumps(report,indent=2)+'\n')
