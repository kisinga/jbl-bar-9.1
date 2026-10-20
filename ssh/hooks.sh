#!/bin/sh
# Add one background command without discarding existing AutoScript bytes.
# Explicit paths let the same logic be exercised in isolated host fixtures.
BB=${JBL_BUSYBOX:-/bin/busybox}
STORE=${JBL_SSH_STORE:-/var/JBLLocalSSH}
HOOK=${JBL_SSH_HOOK:-/var/AutoScript.TSS}
[ -d "$STORE" ] && [ ! -L "$STORE" ] || exit 1
[ "$("$BB" cat "$STORE/OWNER.txt" 2>/dev/null)" = JBL_LOCAL_SSH_REV1 ] || exit 1
"$BB" mkdir "$STORE/hook-lock" 2>/dev/null || exit 1
trap '"$BB" rmdir "$STORE/hook-lock"' EXIT
umask 077
if [ "${1-}" = rollback ]; then
    [ ! -L "$STORE/BOOT_DISABLED" ] || exit 2
    : > "$STORE/BOOT_DISABLED" || exit 8
fi
for p in "$HOOK.ssh-new" "$STORE/hook.with-ssh.new"; do
    [ ! -e "$p" ] && [ ! -L "$p" ] || exit 2
done
for p in "$HOOK" "$STORE/hook.before-ssh" "$STORE/hook.with-ssh"; do
    [ ! -L "$p" ] && { [ ! -e "$p" ] || [ -f "$p" ]; } || exit 2
done
case "${1-}" in
    install)
        if [ ! -f "$STORE/hook.before-ssh" ]; then
            if [ -f "$HOOK" ]; then
                # Refuse an existing SSH command with no owned rollback copy.
                "$BB" grep -q '/var/JBLLocalSSH/boot.sh' "$HOOK" && exit 3
                "$BB" cp -p "$HOOK" "$STORE/hook.before-ssh" || exit 4
                printf 'yes\n' > "$STORE/hook-original-present"
            else
                : > "$STORE/hook.before-ssh" || exit 4
                printf 'no\n' > "$STORE/hook-original-present"
            fi
            "$BB" cp "$STORE/hook.before-ssh" "$STORE/hook.with-ssh.new" || exit 4
            printf '\r\nCLI(CLI_exec sh /var/JBLLocalSSH/boot.sh &)\r\n' >> "$STORE/hook.with-ssh.new"
            "$BB" mv "$STORE/hook.with-ssh.new" "$STORE/hook.with-ssh" || exit 4
        fi
        [ -f "$STORE/hook.with-ssh" ] || exit 5
        if "$BB" cmp -s "$HOOK" "$STORE/hook.with-ssh"; then
            "$BB" rm -f "$STORE/BOOT_DISABLED"
            exit 0
        fi
        if [ -e "$HOOK" ]; then
            "$BB" cmp -s "$HOOK" "$STORE/hook.before-ssh" || exit 6
        else
            [ "$("$BB" cat "$STORE/hook-original-present" 2>/dev/null)" = no ] || exit 6
        fi
        "$BB" cp "$STORE/hook.with-ssh" "$HOOK.ssh-new" || exit 7
        mode=$("$BB" stat -c %a "$STORE/hook.before-ssh") || exit 7
        "$BB" chmod "$mode" "$HOOK.ssh-new" && "$BB" mv "$HOOK.ssh-new" "$HOOK" || exit 7
        "$BB" rm -f "$STORE/BOOT_DISABLED"
        ;;
    rollback)
        # Stop future launches first, even if another tool changed the hook.
        if "$BB" cmp -s "$HOOK" "$STORE/hook.before-ssh"; then exit 0; fi
        if [ ! -e "$HOOK" ] && [ "$("$BB" cat "$STORE/hook-original-present" 2>/dev/null)" = no ]; then exit 0; fi
        "$BB" cmp -s "$HOOK" "$STORE/hook.with-ssh" || exit 9
        if [ "$("$BB" cat "$STORE/hook-original-present" 2>/dev/null)" = no ]; then
            "$BB" rm "$HOOK" || exit 10
        else
            "$BB" cp -p "$STORE/hook.before-ssh" "$HOOK.ssh-new" && "$BB" mv "$HOOK.ssh-new" "$HOOK" || exit 10
        fi
        ;;
    *) exit 11;;
esac
"$BB" sync
