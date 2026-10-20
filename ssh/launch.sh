#!/bin/sh
# USB entry point; release inherited service sockets before any waiting.
PATH=/bin:/sbin:/usr/bin:/usr/sbin
export PATH
unset LD_LIBRARY_PATH LD_PRELOAD
USB=/mnt/sda1/JBLLocalSSH
[ -d "$USB" ] || exit 1
printf 'JBL_SSH_ENTRY\n' > "$USB/ENTRY.txt" || exit 1
exec </dev/null >> "$USB/launch.log" 2>&1
. "$USB/common.sh" || exit 1
release_sockets || exit 1
"$BB" mkdir /tmp/jbl-local-ssh-launch-lock 2>/dev/null || exit 0
trap '"$BB" rmdir /tmp/jbl-local-ssh-launch-lock' EXIT
if owned_store; then
    "$BB" mkdir -p "$USB/saved-state"
    for name in start.log sshd.log connection.txt host_ed25519.pub BOOT_STATUS.txt boot.log; do
        [ ! -f "$STORE/$name" ] || "$BB" cp "$STORE/$name" "$USB/saved-state/$name"
    done
fi
/bin/sh "$USB/start.sh"
"$BB" sync
