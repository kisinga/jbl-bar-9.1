#!/bin/sh
# Remove only this exact installation. Unknown files/hooks are left untouched.
BB=/bin/busybox
USB=/mnt/sda1/JBLInstall
DEST=/var/JBLStandard
HOOK=/var/AutoScript.TSS
RAM=/tmp/jbl-standard-mutation-active
EXPECTED_HOOK=87cf62cefa6b584547f2769662295c3e84d97b30b06e8ee871e50cfceeb92581
[ -x "$BB" ] && [ -d "$USB" ] || exit 1
"$BB" mkdir "$RAM" 2>/dev/null || exit 0
exec >> "$USB/uninstall.log" 2>&1
finish() {
    printf '%s\n' "$1" > "$USB/UNINSTALL_STATUS.txt"
    printf 'RESULT=%s\n' "$1"
    "$BB" sync
    "$BB" rmdir "$RAM"
    exit "$2"
}
[ "$("$BB" readlink -f /var)" = /mnt/ubi_boot/var ] || finish VAR_PATH_UNEXPECTED 1
[ ! -L "$DEST" ] || finish DESTINATION_CONFLICT 1
if [ ! -e "$DEST" ]; then
    [ ! -e "$HOOK" ] && [ ! -L "$HOOK" ] || finish UNOWNED_HOOK_PRESERVED 1
    finish ALREADY_UNINSTALLED 0
fi
case "$("$BB" cat "$DEST/OWNER.txt" 2>/dev/null)" in
 JBL_STANDARD_HOOK_REV2|JBL_STANDARD_HOOK_REV3|JBL_STANDARD_HOOK_REV4|JBL_STANDARD_HOOK_REV5) ;;
 *) finish UNOWNED_FILES_PRESERVED 1;;
esac
if [ -e "$HOOK" ] || [ -L "$HOOK" ]; then
    [ ! -L "$HOOK" ] && [ -f "$HOOK" ] || finish CHANGED_HOOK_PRESERVED 1
    hash=$("$BB" sha256sum "$HOOK") || finish HOOK_UNREADABLE 1
    set -- $hash
    [ "$1" = "$EXPECTED_HOOK" ] || finish CHANGED_HOOK_PRESERVED 1
fi
# Stop new invocations; a running invocation checks DISABLED before acting.
printf 'UNINSTALLING\n' > "$DEST/DISABLED" || finish CANNOT_DISABLE 1
[ ! -f "$HOOK" ] || "$BB" rm "$HOOK" || finish CANNOT_REMOVE_HOOK 1
"$BB" sync
# Let an in-flight controller finish and restore; do not kill it halfway through.
i=0
while { [ -d /tmp/jbl-standard-hook-active ] || [ -d /tmp/jbl-standard-watch-active ]; } && [ "$i" -lt 90 ]; do
    "$BB" sleep 1
    i=$((i + 1))
done
[ ! -d /tmp/jbl-standard-hook-active ] && [ ! -d /tmp/jbl-standard-watch-active ] || finish HOOK_DISABLED_REBOOT_TO_FINISH_UNINSTALL 1
"$BB" mkdir -p "$USB/uninstalled-backup" || finish BACKUP_FAILED_HOOK_DISABLED 1
"$BB" cp -R "$DEST" "$USB/uninstalled-backup/" || finish BACKUP_FAILED_HOOK_DISABLED 1
"$BB" rm -rf "$DEST" || finish REMOVE_FAILED_HOOK_DISABLED 1
finish UNINSTALLED 0
