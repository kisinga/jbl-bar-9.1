from pathlib import Path
import tempfile,subprocess,socket,os,fcntl,shlex,time,json,hashlib
b=Path(__file__).resolve().parents[1];w=b/'build'
(w/'test-results').mkdir(parents=True,exist_ok=True)
arm=os.environ.get('JBL_ARM_BUSYBOX')
bb=Path(arm) if arm else Path('/usr/bin/busybox')
launch=['/usr/bin/qemu-arm',str(bb)] if arm else [str(bb)]
rows=[]
for revision in [4,5]:
 source=(b/'usb/JBLInstall/payload/run.sh').read_text();prefix=source.split('\nPROC=/proc',1)[0]
 if revision==4:
  start=prefix.index('    # Glob enumeration')
  end=prefix.index('    case "$jbl_target"',start)
  prefix=prefix[:start]+'    jbl_target=$("$BB" readlink "$jbl_fd_path" 2>/dev/null) || exit 1\n'+prefix[end:]
 for count in [0,1,2,4,8,12,20]:
  with tempfile.TemporaryDirectory(prefix='jbl-socket-regression-') as temp:
   t=Path(temp);socks=[socket.socket() for _ in range(count)]
   regular=os.open(t/'regular',os.O_CREAT|os.O_RDWR,0o600);regular_high=fcntl.fcntl(regular,fcntl.F_DUPFD,55);os.close(regular)
   script=t/'worker';marker=t/'reached';script.write_text(prefix.replace('BB=/bin/busybox','BB='+shlex.quote(str(bb)))+'\nprintf preserved >&'+str(regular_high)+'\nprintf reached > '+shlex.quote(str(marker))+'\n"$BB" sleep 1\n')
   inherited=[s.fileno() for s in socks]+[regular_high]
   p=subprocess.Popen(launch+['ash',str(script)],pass_fds=tuple(inherited),stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
   os.close(regular_high)
   for s in socks:s.close()
   deadline=time.monotonic()+3
   while not marker.exists() and p.poll() is None and time.monotonic()<deadline:time.sleep(.01)
   targets=[]
   if marker.exists():
    for fd in Path(f'/proc/{p.pid}/fd').glob('*'):
     try:targets.append(str(fd.readlink()))
     except FileNotFoundError:pass
   p.wait(timeout=4)
   reached=marker.exists();preserved=(t/'regular').read_text()=='preserved'
   if revision==5:assert p.returncode==0 and reached and preserved and not any(s.startswith('socket:') for s in targets),(count,p.returncode,targets)
   elif count>=1:assert p.returncode==1 and not reached,(count,p.returncode)
   rows.append({'revision':revision,'inherited_socket_count':count,'inherited_fds':inherited,'exit':p.returncode,'reached_worker':reached,'regular_file_preserved':preserved,'remaining_sockets':sum(s.startswith('socket:') for s in targets) if reached else None})
   print('PASS',rows[-1],flush=True)
# A present but unreadable descriptor must still stop; missing BB stops too.
for case in ['readlink_failure_present','busybox_missing']:
 with tempfile.TemporaryDirectory(prefix='jbl-socket-guard-') as temp:
  t=Path(temp);s=socket.socket();dup=fcntl.fcntl(s.fileno(),fcntl.F_DUPFD,37);s.close()
  wrapper=t/'bb';wrapper.write_text('#!/bin/sh\nif [ "$1" = readlink ]; then exit 1; fi\nexec '+shlex.quote(str(bb))+' "$@"\n');wrapper.chmod(0o700)
  script=t/'worker';marker=t/'reached';script.write_text(prefix.replace('BB=/bin/busybox','BB='+shlex.quote(str(wrapper if case=='readlink_failure_present' else t/'missing')))+'\nprintf reached > '+shlex.quote(str(marker))+'\n')
  p=subprocess.run(launch+['ash',str(script)],pass_fds=(dup,),capture_output=True,timeout=5);os.close(dup)
  assert p.returncode!=0 and not marker.exists(),case
  rows.append({'case':case,'exit':p.returncode,'reached_worker':False});print('PASS',rows[-1],flush=True)
(w/'test-results/descriptor-tests.json').write_text(json.dumps({'scope':('Extracted ARM via QEMU/binfmt; ' if arm else 'Host BusyBox; ')+'host procfs. Actual package startup prefix. Revision4 negative controls reproduce early exit; revision5 must continue with sockets closed and regular file preserved. No soundbar boot proof.','busybox_sha256':hashlib.sha256(bb.read_bytes()).hexdigest(),'run_sha256':hashlib.sha256((b/'usb/JBLInstall/payload/run.sh').read_bytes()).hexdigest(),'cases':rows},indent=2)+'\n')
