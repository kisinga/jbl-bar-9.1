#!/bin/sh
# Read-only service/config/native verification. Does not reconfigure or recover.
USB=${JBL_TOOLKIT_USB:-/mnt/sda1/JBLToolkit}
. "$USB/common.sh" || exit 1
result=0
check() {
    label=$1; shift
    if "$@"; then printf 'PASS=%s\n' "$label"; else printf 'FAIL=%s\n' "$label"; result=1; fi
}
{
    printf 'SCHEMA=1\nACTION=verify\nBOOT_ID=%s\n' "$(boot_id)"
    if ! owned; then
        printf 'FAIL=TOOLKIT_NOT_OWNED\n'
        result=1
    else
        check ENABLED test ! -e "$STORE/DISABLED"
        check PACKAGE "$BB" cmp -s "$STORE/package.manifest" "$USB/SHA256SUMS"
        check RUNTIME_INTEGRITY runtime_integrity
        check MANAGED_HOOK "$BB" cmp -s "$HOOK" "$STORE/hook.managed"
        if "$BB" awk -f "$USB/parse-config.awk" "$STORE/config.txt" > "$LOCK/verify-plan"; then
            . "$LOCK/verify-plan"
            "$BB" rm "$LOCK/verify-plan"
            if [ "$SSH" = true ]; then check SSH_PROCESS process_owned "$SSH_STORE/sshd.pid" "" ssh; fi
            if [ "$WEB_REMOTE" = true ] || [ "$AUTO_STANDARD" = true ]; then
                check REMOTE_READY remote_ready
            fi
            check NATIVE_STATE probe_native
            if [ "$AUTO_STANDARD" = true ] && "$BB" grep -q '"power":"On"' "$USB/native-check.json"; then
                check STANDARD_MODE "$BB" grep -q '"mode":"Standard"' "$USB/native-check.json"
            fi
        else
            printf 'FAIL=INSTALLED_CONFIG\n'
            result=1
        fi
        current=$(boot_id)
        installed=$("$BB" cat "$STORE/install.boot_id" 2>/dev/null)
        booted=$("$BB" cat "$STORE/boot.id" 2>/dev/null)
        if [ -n "$current" ] && [ -n "$installed" ] && [ "$current" != "$installed" ] &&
           [ "$booted" = "$current" ] && [ "$("$BB" cat "$STORE/BOOT_STATUS.txt" 2>/dev/null)" = BOOT_SERVICES_READY ]; then
            printf 'PASS=NEW_KERNEL_BOOT_WITH_MANAGED_SERVICES\n'
        else
            printf 'FAIL=NEW_KERNEL_BOOT_NOT_YET_PROVEN\n'
            result=1
        fi
        printf 'PENDING=USB_PHYSICALLY_REMOVED_DURING_BOOT\nPENDING=SSH_LOGIN_AND_SFTP\nPENDING=AUDIO_AND_NORMAL_CONTROLS\nPENDING=STANDBY_AND_CEC\nPENDING=RECOVERY_AND_REBOOT\n'
    fi
} > "$USB/VERIFY.txt" 2>&1
"$BB" cat "$USB/VERIFY.txt"
if [ "$result" = 0 ]; then
    printf 'VERIFIED_CURRENT_BOOT_PENDING_USER_TESTS\n' > "$USB/STATUS.txt"
else
    printf 'VERIFICATION_INCOMPLETE_READ_VERIFY\n' > "$USB/STATUS.txt"
fi
"$BB" sync
exit "$result"
