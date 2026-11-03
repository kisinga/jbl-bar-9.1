#!/bin/sh
# Per-session environment for the vendor's native utilities.
LD_LIBRARY_PATH=/system/bdplib:/system/usr/lib:/system/usr/local/lib
LD_PRELOAD=
# /bin also contains Android toolbox applets using another C runtime.
# Prefer the vendor utilities that match the library environment above.
PATH=/usr/local/bin:/usr/bin:/usr/local/sbin:/usr/sbin:/bin:/sbin
export LD_LIBRARY_PATH LD_PRELOAD PATH
case "${SSH_ORIGINAL_COMMAND-}" in
    internal-sftp) exec /usr/local/libexec/sftp-server;;
    '') exec /bin/busybox sh -i;;
    *) exec /bin/busybox sh -c "$SSH_ORIGINAL_COMMAND";;
esac
