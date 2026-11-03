#!/bin/sh
# Independent of the installed manager, config, remote binary and native firmware.
USB=${JBL_TOOLKIT_USB:-/mnt/sda1/JBLToolkit}
. "$USB/common.sh" || exit 1
[ -e "$STORE" ] || [ -L "$STORE" ] || { printf 'Toolkit is not installed.\n'; exit 0; }
owned || { printf 'No recognized toolkit installation; nothing changed.\n'; exit 1; }
marker "$STORE/DISABLED" || exit 1
result=0
# Disable both even if either process cannot be stopped. Never force-kill a gesture.
stop_remote || result=1
stop_ssh || result=1
if regular "$HOOK" && regular "$STORE/hook.managed" && "$BB" cmp -s "$HOOK" "$STORE/hook.managed"; then
    if regular "$STORE/hook.absent"; then
        "$BB" rm "$HOOK" || result=1
    else
        mode=$("$BB" cat "$STORE/hook.mode")
        if printf '%s\n' "$mode" | "$BB" grep -Eq '^[0-7]{3,4}$'; then
            atomic_copy "$STORE/hook.before" "$HOOK" && "$BB" chmod "$mode" "$HOOK" || result=1
        else
            printf 'Original hook permissions are missing/invalid; preserved for review.\n'
            result=1
        fi
    fi
elif [ ! -e "$HOOK" ] && [ ! -L "$HOOK" ] && regular "$STORE/hook.absent"; then
    :
elif regular "$HOOK" && regular "$STORE/hook.before" && "$BB" cmp -s "$HOOK" "$STORE/hook.before"; then
    :
else
    printf 'Startup hook changed: preserved. Toolkit launches disabled; inspect manually.\n'
    result=1
fi
"$BB" sync
printf 'Recovery retains files, credentials, Wi-Fi settings and existing SSH sessions. Reboot required.\n'
exit "$result"
