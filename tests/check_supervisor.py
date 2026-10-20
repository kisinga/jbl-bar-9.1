from pathlib import Path
import tempfile,subprocess,hashlib,json,struct,time,signal
base=Path(__file__).resolve().parents[1];pkg=base/'smart-mode/JBLInstall/payload';source=(pkg/'run.sh').read_text()
firmware=base/'tests/fixtures/firmware.txt'
cases=['late_power_on','two_wakes','already_standard','manual_smart_latched','process_restart','bad_firmware','payload_changed','failed_once','stale_once_status','duplicate','rollback_idle','disabled_idle','disable_during_once','rollback_during_once','signal_during_once','bounded_logs']
results=[]
for case in cases:
 with tempfile.TemporaryDirectory(prefix='jbl-watch-fixture-') as temp:
  t=Path(temp);src=t/'store';ram=t/'watch';usb=t/'usb';proc=t/'proc';task=proc/'4242'
  for p in [src,usb,task,proc/'sys/kernel/random']:p.mkdir(parents=True,exist_ok=True)
  (proc/'mounts').write_text('tmpfs /tmp tmpfs rw 0 0\n');(proc/'sys/kernel/random/boot_id').write_text('fixture-boot\n');(proc/'uptime').write_text('0 0\n')
  (task/'stat').write_text('4242 (bdpprog) S '+'0 '*18+'12345\n')
  if case=='bad_firmware':(task/'exe').write_text('unknown')
  else:(task/'exe').symlink_to(firmware)
  with (task/'mem').open('wb') as f:
   f.truncate(16*1024*1024)
   for a,raw in [(14191893,b'\1'),(12962676,struct.pack('<I',2 if case in ['late_power_on','rollback_idle','disabled_idle','bounded_logs'] else 0)),(13378532,struct.pack('<I',0 if case=='already_standard' else 3))]:f.seek(a);f.write(raw)
  bb=t/'busybox';bb.write_text('''#!/usr/bin/python3
from pathlib import Path
import os,sys,struct,time
r=Path(__file__).parent;args=sys.argv[1:];case=CASE
if args[0]=='pidof':print(4242);raise SystemExit()
if args[0]=='sync':raise SystemExit()
if args[0]=='sleep':
 n=int((r/'ticks').read_text())+1 if (r/'ticks').exists() else 1
 (r/'ticks').write_text(str(n));(r/'proc/uptime').write_text(f'{n*5} 0\\n')
 def state(system=None,mode=None):
  with (r/'proc/4242/mem').open('r+b') as f:
   if system is not None:f.seek(12962676);f.write(struct.pack('<I',system))
   if mode is not None:f.seek(13378532);f.write(struct.pack('<I',mode))
 if case=='late_power_on' and n==18:state(0,3)
 if case=='two_wakes':
  if n==8:state(2,3)
  if n==15:state(0,3)
 if case=='manual_smart_latched' and n==8:state(0,3)
 if case=='process_restart' and n==8:
  state(0,3);(r/'proc/4242/stat').write_text('4242 (bdpprog) S '+'0 '*18+'99999\\n')
 if case=='bounded_logs':state(2 if n%2 else 4,3)
 if case=='rollback_idle' and n==4:(r/'usb/ROLLBACK').touch()
 if n >= (150 if case=='bounded_logs' else 30):(r/'store/DISABLED').write_text('fixture end')
 if case=='disabled_idle' and n==4:(r/'store/DISABLED').write_text('fixture cancel')
 time.sleep(.002);raise SystemExit()
os.execv('/usr/bin/busybox',['busybox']+args)
'''.replace('CASE',repr(case)));bb.chmod(0o700)
  s=source.replace('86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63',hashlib.sha256(firmware.read_bytes()).hexdigest()).replace('BB=/bin/busybox','BB='+str(bb)).replace('PROC=/proc','PROC='+str(proc)).replace('/var/JBLStandard',str(src)).replace('/tmp/jbl-standard-watch-active',str(ram)).replace('/mnt/sda1/JBLInstall',str(usb))
  (src/'run.sh').write_text(s)
  once='''#!/usr/bin/python3
from pathlib import Path
import struct,sys,signal,time
s=Path(sys.argv[1]);r=s.parent;case=CASE
with (r/'proc/4242/mem').open('r+b') as f:
 f.seek(12962676);assert struct.unpack('<I',f.read(4))[0]==0
 f.seek(13378532);assert struct.unpack('<I',f.read(4))[0]==3
with (r/'attempts').open('a') as f:f.write((r/'ticks').read_text()+'\\n')
if case=='failed_once':(s/'STATUS.txt').write_text('INPUT_RELEASE_NOT_CONFIRMED');raise SystemExit(1)
if case=='stale_once_status':raise SystemExit()
if case=='disable_during_once':(s/'DISABLED').write_text('uninstall')
if case=='rollback_during_once':(r/'usb/ROLLBACK').touch()
if case=='signal_during_once':
 def stop(*args):
  (r/'restored').write_text('restored before exit');raise SystemExit(1)
 signal.signal(signal.SIGTERM,stop)
 (r/'child_ready').touch()
 time.sleep(20)
with (r/'proc/4242/mem').open('r+b') as f:f.seek(13378532);f.write(struct.pack('<I',0))
(s/'STATUS.txt').write_text('STANDARD_SELECTED');(r/'restored').write_text('restored before exit')
'''.replace('CASE',repr(case))
  # Supervisor invokes /bin/sh, so place the Python fixture behind a shell wrapper.
  (t/'once.py').write_text(once)
  (src/'once.sh').write_text('#!/bin/sh\nexec /usr/bin/python3 '+str(t/'once.py')+' "$@"\n')
  (src/'checksums.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(src.iterdir())))
  (src/'STATUS.txt').write_text('STANDARD_SELECTED')
  (src/'watch.log').write_text('previous boot evidence\n')
  if case=='payload_changed':(src/'once.sh').write_text('changed')
  if case=='duplicate':ram.mkdir()
  start=time.monotonic()
  worker=subprocess.Popen(['/bin/sh',str(src/'run.sh')],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
  if case=='signal_during_once':
   while not (t/'child_ready').exists() and worker.poll() is None and time.monotonic()-start<15:time.sleep(.02)
   assert (t/'child_ready').exists(),case
   worker.send_signal(signal.SIGTERM)
  out,err=worker.communicate(timeout=100)
  attempts=(t/'attempts').read_text().splitlines() if (t/'attempts').exists() else []
  expected=2 if case in ['two_wakes','process_restart'] else 1 if case in ['late_power_on','manual_smart_latched','failed_once','stale_once_status','disable_during_once','rollback_during_once','signal_during_once'] else 0
  status=(src/'WATCH_STATUS.txt').read_text().strip() if (src/'WATCH_STATUS.txt').exists() else 'NO_STATUS'
  assert len(attempts)==expected,(case,attempts,status,err)
  if case=='late_power_on':
   assert int(attempts[0])>=20,(case,attempts)
   assert (src/'watch-previous.log').read_text()=='previous boot evidence\n'
   assert len((src/'watch.log').read_text().splitlines()) < 12
  if case not in ['duplicate']:assert not ram.exists(),(case,status)
  if case in ['failed_once','stale_once_status','bad_firmware','payload_changed','signal_during_once']:assert worker.returncode==1 and (src/'DISABLED').exists(),(case,status)
  if case in ['disable_during_once','rollback_during_once','signal_during_once']:assert (t/'restored').exists(),case
  if case=='bounded_logs':
   assert len((src/'watch.log').read_text().splitlines())<=64
   assert len((src/'watch-previous.log').read_text().splitlines())<=64
  results.append({'case':case,'attempts':attempts,'status':status,'exit':worker.returncode});print('PASS',case,len(attempts),status,flush=True)
report={'scope':'Actual supervisor on host with temp proc and clock fixtures; stubbed bounded action. Does not prove device wake coverage.','supervisor_sha256':hashlib.sha256(source.encode()).hexdigest(),'cases':results}
(base/'build/test-results/supervisor-tests.json').write_text(json.dumps(report,indent=2)+'\n')
