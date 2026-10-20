#!/bin/sh
# Disable this remote and restore only its exact recorded startup hook.
BB=/bin/busybox
STORE=/var/JBLRemote
[ "$("$BB" cat "$STORE/OWNER.txt")" = JBL_WEB_REMOTE_REV1 ] || exit 1
: > "$STORE/DISABLED" || exit 1
if "$BB" cmp -s /var/AutoScript.TSS "$STORE/hook.with-web"; then
    if [ -f "$STORE/hook.was-absent" ]; then
        "$BB" rm /var/AutoScript.TSS || exit 1
    else
        [ ! -L /var/AutoScript.TSS.jbl-web-restore ] || exit 1
        "$BB" cp "$STORE/hook.before-web" /var/AutoScript.TSS.jbl-web-restore || exit 1
        "$BB" chmod 600 /var/AutoScript.TSS.jbl-web-restore || exit 1
        "$BB" mv /var/AutoScript.TSS.jbl-web-restore /var/AutoScript.TSS || exit 1
    fi
elif ! "$BB" cmp -s /var/AutoScript.TSS "$STORE/hook.before-web"; then
    printf 'Startup hook changed; preserved. Automatic launch is disabled.\n'
fi
if [ -f "$STORE/remote.pid" ]; then
    pid=$("$BB" cat "$STORE/remote.pid")
    case "$pid" in ''|*[!0-9]*) exit 1;; esac
    if [ -r "/proc/$pid/cmdline" ]; then
        cmd=$("$BB" tr '\000' ' ' < "/proc/$pid/cmdline")
        case "$cmd" in "$STORE/remote -config $STORE/config.json "*) kill -TERM "$pid";; esac
    fi
fi
"$BB" sync
printf 'Web remote disabled. Files retained in /var/JBLRemote.\n'
