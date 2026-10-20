from pathlib import Path
import tempfile,subprocess,hashlib,json,struct
base=Path(__file__).resolve().parents[1]
package=base/'smart-mode/JBLInstall/payload'
controller=(package/'controller.sh').read_text()
helper=(package/'helper.sh').read_text()
firmware=base/'tests/fixtures/firmware.txt'
expected_cli='8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f'
helper_hash=hashlib.sha256(helper.encode()).hexdigest()
cases=[
 ('bad_adapter',3,0,2,'INPUT_BUSY_NO_CHANGE',0),
 ('input_busy',3,0,2,'INPUT_BUSY_NO_CHANGE',0),
 ('no_completion',3,0,2,'COMMAND_COMPLETION_NOT_CONFIRMED',2),
 ('stuck_down',3,0,2,'INPUT_RELEASE_NOT_CONFIRMED',2),
 ('stuck_combo',3,0,2,'INPUT_RELEASE_NOT_CONFIRMED',2),
 ('mode_changed_after_release',3,0,2,'FINAL_MODE_CHANGED',2),
 ('asleep',3,0,2,'NOT_AWAKE_NO_CHANGE',0),
 ('str_standby',3,0,2,'NOT_AWAKE_NO_CHANGE',0),
 ('power_off_during_mute',3,0,2,'POWER_CHANGED_DURING_TEST',1),
 ('success',3,0,2,'STANDARD_SELECTED',2),
 ('input_enabled',3,1,2,'STANDARD_SELECTED',2),
 ('console_open',3,1,0,'STANDARD_SELECTED',2),
 ('already_standard',0,0,2,'ALREADY_STANDARD_NO_CHANGE',0),
 ('unknown_mode',7,0,2,'UNEXPECTED_MODE_NO_CHANGE',0),
 ('bad_firmware',3,0,2,'FIRMWARE_MISMATCH_NO_CHANGE',0),
 ('bad_cli',3,0,2,'CLI_MISMATCH_NO_CHANGE',0),
 ('bad_helper',3,0,2,'HELPER_MISMATCH_NO_CHANGE',0),
 ('missing_memory',3,0,2,'MODE_UNREADABLE_NO_CHANGE',0),
 ('missing_socket',3,0,2,'CLI_SOCKET_ABSENT_NO_CHANGE',0),
 ('login_active',3,1,1,'UNEXPECTED_CONSOLE_STATE_NO_CHANGE',0),
 ('inconsistent_console',3,0,0,'UNEXPECTED_CONSOLE_STATE_NO_CHANGE',0),
 ('input_write_fail',3,0,2,'CONSOLE_INPUT_WRITE_FAILED',0),
 ('gate_write_fail',3,0,2,'CONSOLE_GATE_WRITE_FAILED',0),
 ('wake_failure',3,0,2,'CLI_WAKE_FAILED',0),
 ('no_ack',3,0,2,'CLI_MARKER_NOT_CONFIRMED',0),
 ('remote_busy',3,0,2,'REMOTE_BUSY_NO_CHANGE',0),
 ('no_repeat',3,0,2,'LONG_PRESS_NOT_CONFIRMED',1),
 ('no_mode_change',3,0,2,'STANDARD_NOT_CONFIRMED',2),
 ('interrupt',3,0,2,'INTERRUPTED',0),
 ('process_changed',3,0,2,'CONSOLE_RESTORE_FAILED',0),
 ('restore_failure',3,0,2,'CONSOLE_RESTORE_FAILED',2),
]
results=[]
for case,mode,initial_input,initial_gate,expected,key_count in cases:
 with tempfile.TemporaryDirectory(prefix='jbl-console-fixture-') as temp:
  t=Path(temp);out=t/'output';(out/'JBLTest').mkdir(parents=True)
  proc=t/'proc';task=proc/'4242';task.mkdir(parents=True);(proc/'net').mkdir()
  (proc/'net/unix').write_text('test @android:clid\n' if case!='missing_socket' else '')
  stat='4242 (bdpprog) S '+'0 '*18+'12345\n';(task/'stat').write_text(stat);(task/'maps').write_text('fixture\n')
  if case=='bad_firmware':(task/'exe').write_text('wrong executable')
  else:(task/'exe').symlink_to(firmware)
  memory=task/'mem'
  if case!='missing_memory':
   with memory.open('wb') as f:
    f.truncate(16*1024*1024)
    for address,data in [(13822200,struct.pack('<I',7 if case=='bad_adapter' else 1)),(13822224,bytes([1 if case=='input_busy' else 0])),(14191893,bytes([0 if case=='asleep' else 1])),(12962676,struct.pack('<I',3 if case=='str_standby' else 0)),(13378532,struct.pack('<I',mode)),(14200904,struct.pack('<I',2 if case=='remote_busy' else 0)),(15427780,bytes([initial_input])),(15427784,struct.pack('<I',initial_gate))]:
     f.seek(address);f.write(data)
  bb=t/'busybox'
  wrapper='''#!/bin/sh
if [ "$1" = pidof ]; then echo 4242; exit 0; fi
if [ "$1" = sleep ]; then
 if [ CASE = interrupt ] && [ "$2" = 0.5 ]; then kill -TERM "$PPID"; fi
 exit 0
fi
if [ "$1" = dd ]; then
 case "$*" in
  *of=MEMORY*)
   case "$*" in
    *if=OUTPUT/input-enabled.bin*) [ CASE = input_write_fail ] && exit 1;;
    *if=OUTPUT/gate-enabled.bin*) [ CASE = gate_write_fail ] && exit 1;;
    *if=OUTPUT/gate-original.bin*) [ CASE = restore_failure ] && exit 1;;
   esac;;
 esac
fi
exec /usr/bin/busybox "$@"
'''.replace('CASE',case).replace('MEMORY',str(memory)).replace('OUTPUT',str(out))
  bb.write_text(wrapper);bb.chmod(0o700)
  cli=t/'cli';cli.write_text('''#!/usr/bin/python3
from pathlib import Path
import sys,struct,json
root=Path(__file__).parent
args=sys.argv[1:]
with (root/'calls.jsonl').open('a') as f:f.write(json.dumps(args)+'\\n')
case=CASE
with (root/'proc/4242/mem').open('r+b') as f:
 f.seek(15427780);enabled=f.read(1)
 f.seek(15427784);gate=struct.unpack('<I',f.read(4))[0]
 assert enabled==b'\\x01' and gate==0,(enabled,gate,args)
 if args==['']:
  if case=='wake_failure':raise SystemExit(1)
  if case=='process_changed':
   p=root/'proc/4242/stat';p.write_text(p.read_text().replace('12345','67890'))
 elif args[:3]==['exec','/bin/busybox','touch']:
  assert args[3] in [str(root/'output/CLI_ACK.txt'),str(root/'output/JBLTest/CLI_DONE.txt')]
  if args[3].endswith('CLI_ACK.txt'):
   if case!='no_ack':Path(args[3]).touch()
  else:
   if case!='no_completion':Path(args[3]).touch()
   f.seek(13822224);f.write(bytes([1 if case=='stuck_down' else 0]))
   f.seek(14200904);f.write(struct.pack('<I',14 if case=='stuck_combo' else 0))
   f.seek(14200898);f.write(b'\\x00')
   if case=='mode_changed_after_release':f.seek(13378532);f.write(struct.pack('<I',3))
 elif args==['mw.iom.se','0x6f002','3500']:
  if case=='power_off_during_mute':f.seek(14191893);f.write(b'\\x00')
  if case!='no_repeat':
   f.seek(14200904);f.write(struct.pack('<I',1))
   f.seek(14200898);f.write(b'\\x01')
 elif args==['mw.iom.se','0x6f000','50']:
  f.seek(13822224);f.write(b'\\x01')
  if case!='no_mode_change':f.seek(13378532);f.write(struct.pack('<I',0))
 else:raise AssertionError(args)
'''.replace('CASE',repr(case)));cli.chmod(0o700)
  cli_hash=hashlib.sha256(cli.read_bytes()).hexdigest()
  def adapt(s):
   return s.replace('86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63',hashlib.sha256(firmware.read_bytes()).hexdigest()).replace('/mnt/sda1/JBLConsole',str(out)).replace('PROC=/proc','PROC='+str(proc)).replace('BB=/bin/busybox','BB='+str(bb)).replace('CLI=/bin/cli','CLI='+str(cli)).replace(expected_cli,'0'*64 if case=='bad_cli' else cli_hash)
  h=adapt(helper);(out/'JBLTest/run.sh').write_text(h)
  c=adapt(controller).replace(helper_hash,'0'*64 if case=='bad_helper' else hashlib.sha256(h.encode()).hexdigest());run=t/'run.sh';run.write_text(c)
  result=subprocess.run(['/bin/sh',str(run),str(out)],capture_output=True,text=True,timeout=35)
  status=(out/'STATUS.txt').read_text().strip();events=[json.loads(x) for x in (t/'calls.jsonl').read_text().splitlines()] if (t/'calls.jsonl').exists() else []
  keys=[x for x in events if x and x[0]=='mw.iom.se']
  assert (status,len(keys))==(expected,key_count),(case,status,events,(out/'result.txt').read_text(),result.stderr)
  restored=None
  if memory.exists():
   with memory.open('rb') as f:
    f.seek(15427780);after_input=f.read(1)[0];f.seek(15427784);after_gate=struct.unpack('<I',f.read(4))[0]
   restored=(after_input,after_gate)==(initial_input,initial_gate)
   assert restored==(case not in ['restore_failure','process_changed']),(case,after_input,after_gate)
  before=(out/'result.txt').read_bytes();second=subprocess.run(['/bin/sh',str(run),str(out)],capture_output=True,text=True,timeout=10)
  assert second.returncode==0 and (out/'result.txt').read_bytes()==before
  afterevents=[json.loads(x) for x in (t/'calls.jsonl').read_text().splitlines()] if (t/'calls.jsonl').exists() else []
  assert afterevents==events
  results.append({'case':case,'status':status,'key_commands':len(keys),'console_original_values_retained_or_restored':restored,'second_invocation_did_nothing':True})
  print('PASS',case,status,flush=True)
report={'scope':'Host fixtures, not firmware emulation. Verifies identity/state guards, temporary field writes, restoration on controlled exits/signals, refusal to restore into replaced process, key sequencing and one-shot behavior. Does not prove hardware proc-mem writes, transport, timing or audio.','controller_sha256':hashlib.sha256(controller.encode()).hexdigest(),'helper_sha256':helper_hash,'cases':results}
(base/'build/test-results/controller-tests.json').write_text(json.dumps(report,indent=2)+'\n')
