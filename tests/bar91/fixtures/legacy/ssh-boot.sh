#!/bin/sh
# Called in the background; bounded waits cannot hold the boot command open.
PATH=/bin:/sbin:/usr/bin:/usr/sbin
export PATH
unset LD_LIBRARY_PATH LD_PRELOAD
ulimit -c 0
exec </dev/null >/dev/null 2>&1
. /var/JBLLocalSSH/common.sh || exit 1
release_sockets || exit 1
owned_store || exit 1
[ ! -e "$STORE/BOOT_DISABLED" ] || exit 0
. "$STORE/settings.conf" || exit 1
[ "$AUTOSTART" = 1 ] || exit 0
LOCK=/tmp/jbl-local-ssh-boot-lock
"$BB" mkdir "$LOCK" 2>/dev/null || exit 0
trap '"$BB" rmdir "$LOCK"' EXIT
umask 077
[ ! -f "$STORE/boot.log" ] || "$BB" mv "$STORE/boot.log" "$STORE/boot-previous.log"
exec > "$STORE/boot.log" 2>&1
finish() { printf '%s\n' "$1" > "$STORE/BOOT_STATUS.txt"; printf 'RESULT=%s\n' "$1"; exit "$2"; }
verify_tools || finish UNSUPPORTED_NATIVE_SSH_BUILD 1
daemon_running && finish ALREADY_RUNNING 0
for name in namespace.sh session.sh passwd nsswitch.conf ld.so.preload host_ed25519 host_ed25519.pub sshd_config; do
    [ -f "$STORE/$name" ] || finish "MISSING_$name" 1
done
tries=0
addr=
while [ "$tries" -lt 90 ]; do
    [ ! -e "$STORE/BOOT_DISABLED" ] || finish DISABLED 0
    addr=$(find_address)
    [ -z "$addr" ] || break
    "$BB" sleep 2
    tries=$((tries + 1))
done
[ -n "$addr" ] || finish NO_ALLOWED_LAN_WITHIN_180_SECONDS 1
"$BB" awk -v ip="$addr" '/^ListenAddress / {print "ListenAddress " ip; n++; next} {print} END {if(n!=1) exit 1}' "$STORE/sshd_config" > "$STORE/sshd_config.new" || finish CONFIG_ADDRESS_FAILED 1
"$BB" mv "$STORE/sshd_config.new" "$STORE/sshd_config" || finish CONFIG_SAVE_FAILED 1
printf '%s\n' "$addr" > "$STORE/listen-address"
prepare_privsep || finish PRIVSEP_CHECK_FAILED 1
"$BB" unshare -m --propagation private /bin/sh "$STORE/namespace.sh" > "$STORE/start.log" 2>&1 </dev/null &
child=$!
wait_listener "$child" || finish SSH_LISTENER_NOT_CONFIRMED 1
connection_info || finish CONNECTION_INFO_FAILED 1
finish SSH_READY 0
