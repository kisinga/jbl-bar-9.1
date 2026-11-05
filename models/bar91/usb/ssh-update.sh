#!/bin/sh
# Network entry point for an existing managed installation, never first install.
PATH=/bin:/sbin:/usr/bin:/usr/sbin
export PATH
unset LD_LIBRARY_PATH LD_PRELOAD
umask 077
BB=${JBL_BUSYBOX:-/bin/busybox}
USB=${JBL_TOOLKIT_USB:?network staging directory required}
. "$USB/common.sh" || exit 1
release_sockets || exit 1
export JBL_TOOLKIT_TRANSPORT=ssh
/bin/sh "$USB/dispatch.sh"
result=$?
printf '%s\n' "$result" > "$USB/SSH-EXIT.txt.new" &&
    "$BB" mv "$USB/SSH-EXIT.txt.new" "$USB/SSH-EXIT.txt"
"$BB" sync
exit "$result"
