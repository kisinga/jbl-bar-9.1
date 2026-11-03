#!/bin/sh
# Runs only inside a new private mount namespace. Never edits vendor files.
BB=/bin/busybox
STORE=/var/JBLLocalSSH
[ "$("$BB" readlink /proc/self/ns/mnt)" != "$("$BB" readlink /proc/1/ns/mnt)" ] || exit 20
"$BB" mount --bind "$STORE/passwd" /etc/passwd || exit 21
if [ -f "$STORE/shadow" ]; then
    "$BB" mount --bind "$STORE/shadow" /etc/shadow || exit 22
fi
"$BB" mount --bind "$STORE/nsswitch.conf" /etc/nsswitch.conf || exit 23
# The device's default soft-float interpreter is not the vendor 2.18 loader.
# A private bind also covers sshd's later re-exec using its ELF interpreter.
# The separate hard-float interpreter and the parent namespace are untouched.
[ -f /lib/ld-linux.so.3 ] && [ ! -L /lib/ld-linux.so.3 ] || exit 25
"$BB" mount --bind /system/bdplib/ld-2.18.so /lib/ld-linux.so.3 || exit 26
# The system preload references an incompatible/missing compatibility stub.
# Hide that preload only in this SSH namespace.
if [ -f /etc/ld.so.preload ]; then
    "$BB" mount --bind "$STORE/ld.so.preload" /etc/ld.so.preload || exit 32
fi
# Keep the matching vendor runtime for validation, daemon and SSH re-exec.
# This environment is private to this helper; no system library is replaced.
LD_LIBRARY_PATH=/system/bdplib:/system/usr/lib:/system/usr/local/lib
LD_PRELOAD=
export LD_LIBRARY_PATH LD_PRELOAD
printf 'PHASE=HOST_KEY\n'
if [ ! -f "$STORE/host_ed25519" ]; then
    "$BB" rm -f "$STORE/host-new" "$STORE/host-new.pub"
    "$BB" timeout 20 /usr/local/bin/ssh-keygen -q -t ed25519 -N '' -f "$STORE/host-new" || exit 27
    "$BB" mv "$STORE/host-new" "$STORE/host_ed25519" && "$BB" mv "$STORE/host-new.pub" "$STORE/host_ed25519.pub" || exit 28
fi
[ -f "$STORE/host_ed25519.pub" ] || exit 29
"$BB" chmod 600 "$STORE/host_ed25519" || exit 30
/usr/local/bin/ssh-keygen -lf "$STORE/host_ed25519.pub" > "$STORE/host_fingerprint.txt" || exit 31
printf 'PHASE=SSHD_CONFIG\n'
/usr/local/sbin/sshd -t -f "$STORE/sshd_config" || exit 24
# Begin daemon lifetime.
printf '%s\n' "$$" > "$STORE/sshd.pid"
exec /usr/local/sbin/sshd -D -E "$STORE/sshd.log" -f "$STORE/sshd_config"
