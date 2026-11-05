#!/bin/sh
PATH=/bin:/sbin:/usr/bin:/usr/sbin
export PATH
unset LD_LIBRARY_PATH LD_PRELOAD
umask 077
ulimit -c 0
USB=${JBL_TOOLKIT_USB:-/mnt/sda1/JBLToolkit}
[ -d "$USB" ] && [ ! -L "$USB" ] || exit 1
exec </dev/null >>"$USB/setup.log" 2>&1
. "$USB/common.sh" || exit 1
release_sockets || exit 1
if ! "$BB" mkdir "$LOCK" 2>/dev/null; then
    printf 'BUSY_RETRY_AFTER_BOOT_OR_CURRENT_OPERATION\n' > "$USB/STATUS.txt"
    exit 1
fi
trap '"$BB" rm -f "$LOCK/plan" "$LOCK/verify-plan" "$LOCK/inspect-remote" "$LOCK/publish-remote" "$LOCK/runtime.manifest"; "$BB" rmdir "$LOCK"' EXIT
finish() {
    printf '%s\n' "$1" > "$USB/STATUS.txt"
    printf 'RESULT=%s EXIT=%s\n' "$1" "$2"
    "$BB" sync
    exit "$2"
}
# Emergency recovery works without a valid config or intact application payload.
if [ -f "$USB/RECOVER" ]; then
    /bin/sh "$USB/recover.sh"
    result=$?
    [ "$result" != 0 ] || finish RECOVERED_REBOOT_REQUIRED 0
    finish RECOVERY_INCOMPLETE_READ_LOG 1
fi
(cd "$USB" && "$BB" sha256sum -c SHA256SUMS) || finish PACKAGE_INTEGRITY_FAILED 1
regular "$USB/config.txt" || finish CONFIG_MISSING_OR_UNSAFE 1
"$BB" awk -f "$USB/parse-config.awk" "$USB/config.txt" > "$LOCK/plan" || finish CONFIG_INVALID 1
. "$LOCK/plan" || finish CONFIG_INVALID 1
"$BB" rm "$LOCK/plan"
if [ "${JBL_TOOLKIT_TRANSPORT:-usb}" = ssh ]; then
    [ "$ACTION" = update ] && owned || finish SSH_UPDATE_REQUIRES_MANAGED_INSTALLATION 1
fi
if [ "$ACTION" = uninstall ]; then
    /bin/sh "$USB/recover.sh"
    result=$?
    [ "$result" != 0 ] || finish RECOVERED_REBOOT_REQUIRED 0
    finish RECOVERY_INCOMPLETE_READ_LOG 1
fi
if [ "$ACTION" = check ] || [ "$ACTION" = verify ]; then
    /bin/sh "$USB/$ACTION.sh"
    result=$?
    "$BB" sync
    exit "$result"
fi
# One user action: update a managed installation, migrate the reviewed legacy
# chain, or install on a clean bar. Unknown stores still fail identity checks.
if [ "$ACTION" = update ] && ! owned; then
    if [ -e "$SSH_STORE" ] || [ -L "$SSH_STORE" ] ||
       [ -e "$REMOTE_STORE" ] || [ -L "$REMOTE_STORE" ] ||
       [ -e "$STANDARD_STORE" ] || [ -L "$STANDARD_STORE" ] ||
       [ -e "$LEGACY_STORE" ] || [ -L "$LEGACY_STORE" ]; then
        /bin/sh "$USB/migrate.sh" || exit 1
    fi
fi
if [ "$ACTION" = migrate ]; then
    /bin/sh "$USB/migrate.sh" || exit 1
fi
/bin/sh "$USB/manage.sh"
result=$?
# manage.sh writes a precise receipt, including partial-install failure states.
"$BB" sync
exit "$result"
