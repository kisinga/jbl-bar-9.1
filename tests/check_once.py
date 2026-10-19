from pathlib import Path
import tempfile,subprocess,hashlib,json,struct
base=Path(__file__).resolve().parents[1];pkg=base/'usb/JBLInstall/payload';source=(pkg/'once.sh').read_text()
firmware=base/'tests/fixtures/firmware.txt'
results=[]
cases=['success_then_wake','already_standard','asleep','bad_firmware','bad_cli','failed_controller','disable_write_fails','duplicate','rollback','payload_changed']
for case in cases:
 with tempfile.TemporaryDirectory(prefix='jbl-hook-fixture-') as temp:
  t=Path(temp);src=t/'payload';store=t/'store';ram=t/'active';usb=t/'usb';proc=t/'proc';task=proc/'4242'
  for p in [src,store,usb,task,proc/'sys/kernel/random']:p.mkdir(parents=True,exist_ok=True)
  (proc/'mounts').write_text('tmpfs /tmp tmpfs rw 0 0\n');(proc/'sys/kernel/random/boot_id').write_text('fixture-boot\n');(proc/'uptime').write_text('100 100\n')
  (task/'stat').write_text('4242 (bdpprog) S '+'0 '*18+'12345\n')
  if case=='bad_firmware':(task/'exe').write_text('unknown')
  else:(task/'exe').symlink_to(firmware)
  with (task/'mem').open('wb') as f:
   f.truncate(16*1024*1024)
   for a,raw in [(14191893,bytes([0 if case=='asleep' else 1])),(12962676,struct.pack('<I',0)),(13378532,struct.pack('<I',0 if case=='already_standard' else 3))]:f.seek(a);f.write(raw)
  bb=t/'busybox';bb.write_text('''#!/bin/sh
if [ "$1" = pidof ]; then echo 4242; exit 0; fi
if [ "$1" = sleep ]; then exit 0; fi
exec /usr/bin/busybox "$@"
''');bb.chmod(0o700)
  cli=t/'cli';cli.write_text('fixture')
  s=source.replace('86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63',hashlib.sha256(firmware.read_bytes()).hexdigest()).replace('BB=/bin/busybox','BB='+str(bb)).replace('PROC=/proc','PROC='+str(proc)).replace('/var/JBLStandard',str(store)).replace('/tmp/jbl-standard-hook-active',str(ram)).replace('/mnt/sda1/JBLInstall',str(usb)).replace('CLI=/bin/cli','CLI='+str(cli)).replace('8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f','0'*64 if case=='bad_cli' else hashlib.sha256(cli.read_bytes()).hexdigest())
  (src/'run.sh').write_text(s)
  ctrl='''#!/bin/sh
printf 'attempt\\n' >> COUNTER
if [ CASE = failed_controller ] || [ CASE = disable_write_fails ]; then echo INPUT_RELEASE_NOT_CONFIRMED > "$1/STATUS.txt"; exit 1; fi
printf '\\000\\000\\000\\000' | /usr/bin/busybox dd of=MEMORY bs=1 seek=13378532 count=4 conv=notrunc 2>/dev/null
echo STANDARD_SELECTED > "$1/STATUS.txt"
'''.replace('CASE',case).replace('COUNTER',str(t/'attempts')).replace('MEMORY',str(task/'mem'))
  (src/'controller.sh').write_text(ctrl);(src/'helper.sh').write_text('fixture')
  checks=''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(src.iterdir()))
  (src/'checksums.txt').write_text(checks)
  if case=='disable_write_fails':(store/'DISABLED').mkdir()
  if case=='duplicate':ram.mkdir()
  if case=='rollback':(usb/'ROLLBACK').touch()
  if case=='payload_changed':(src/'helper.sh').write_text('changed')
  result=subprocess.run(['/bin/sh',str(src/'run.sh')],capture_output=True,text=True,timeout=25)
  status=(store/'STATUS.txt').read_text().strip() if (store/'STATUS.txt').exists() else 'NO_STATUS'
  expected={'success_then_wake':'STANDARD_SELECTED','already_standard':'ALREADY_STANDARD_NO_CHANGE','asleep':'NOT_READY_NO_CHANGE','bad_firmware':'FIRMWARE_MISMATCH','bad_cli':'CLI_MISMATCH','failed_controller':'STOPPED_INPUT_RELEASE_NOT_CONFIRMED','disable_write_fails':'STOPPED_INPUT_RELEASE_NOT_CONFIRMED','duplicate':'NO_STATUS','rollback':'NO_STATUS','payload_changed':'PAYLOAD_MISMATCH'}[case]
  assert status==expected,(case,status,result.stderr,(store/'latest.log').read_text() if (store/'latest.log').exists() else '')
  if case=='disable_write_fails':assert ram.exists()
  elif case not in ['duplicate','rollback']:assert not ram.exists(),case
  if case in ['failed_controller','bad_firmware','bad_cli','payload_changed']:assert (store/'DISABLED').is_file()
  if case=='success_then_wake':
   assert (t/'attempts').read_text().count('attempt')==1
   # A second callback while Standard must do nothing; later wake restores Smart.
   subprocess.run(['/bin/sh',str(src/'run.sh')],check=True,timeout=25)
   assert (t/'attempts').read_text().count('attempt')==1
   with (task/'mem').open('r+b') as f:f.seek(13378532);f.write(struct.pack('<I',3))
   subprocess.run(['/bin/sh',str(src/'run.sh')],check=True,timeout=25)
   assert (t/'attempts').read_text().count('attempt')==2
   assert (store/'previous.log').exists()
  results.append({'case':case,'status':status,'exit':result.returncode});print('PASS',case,status,flush=True)
report={'scope':'Host one-shot lifecycle fixtures with stubbed controller; actual controller separately tested. Does not prove firmware hooks run or standby behavior.','runner_sha256':hashlib.sha256(source.encode()).hexdigest(),'cases':results}
(base/'build/test-results/once-tests.json').write_text(json.dumps(report,indent=2)+'\n')
