#!/bin/sh
# USB entry: upgrade only exact known rev5; otherwise let setup validate rev6.
BB=/bin/busybox
USB=/mnt/sda1/JBLInstall
DEST=/var/JBLStandard
HOOK=/var/AutoScript.TSS
RAM=/tmp/jbl-standard-usb-operation
OLD_MANIFEST=214c8b621d92f5d528bf6a256f3e058bcda23438fdc05677ca242ba50ee95c80
EXPECTED_MANIFEST=6c6119de762336efdaadb76202eb38085f0ff6c89393461e983ed33f20ebb91e
EXPECTED_HOOK=87cf62cefa6b584547f2769662295c3e84d97b30b06e8ee871e50cfceeb92581
[ -x "$BB" ] && [ -d "$USB" ] || exit 1
"$BB" mkdir "$RAM" 2>/dev/null || exit 0
exec >> "$USB/update.log" 2>&1
child=
finish() {
    trap '' HUP INT TERM
    if [ -n "$child" ]; then kill -TERM "$child" 2>/dev/null; wait "$child"; child=; fi
    printf '%s\n' "$1" > "$USB/UPDATE_STATUS.txt"
    printf 'RESULT=%s EXIT=%s\n' "$1" "$2"
    "$BB" sync
    "$BB" rmdir "$RAM"
    exit "$2"
}
trap 'finish INTERRUPTED 1' HUP INT TERM
run_child() {
    /bin/sh "$1" &
    child=$!
    wait "$child"
    result=$?
    child=
    return "$result"
}
if [ -f "$USB/ROLLBACK" ]; then
    run_child "$USB/uninstall.sh"
    code=$?
    finish ROLLBACK_FINISHED "$code"
fi
# Check replacement before touching the old installation.
hash=$("$BB" sha256sum "$USB/payload/checksums.txt") || finish NEW_MANIFEST_UNREADABLE 1
set -- $hash
[ "$1" = "$EXPECTED_MANIFEST" ] || finish NEW_MANIFEST_MISMATCH 1
(cd "$USB/payload" && "$BB" sha256sum -c checksums.txt) || finish NEW_PAYLOAD_CHANGED 1
if [ -e "$DEST" ] || [ -L "$DEST" ]; then
    [ -d "$DEST" ] && [ ! -L "$DEST" ] || finish DESTINATION_CONFLICT 1
    owner=$("$BB" cat "$DEST/OWNER.txt" 2>/dev/null)
    if [ "$owner" = JBL_STANDARD_HOOK_REV5 ]; then
        hash=$("$BB" sha256sum "$DEST/checksums.txt") || finish OLD_MANIFEST_UNREADABLE 1
        set -- $hash
        [ "$1" = "$OLD_MANIFEST" ] || finish UNKNOWN_REV5_PRESERVED 1
        (cd "$DEST" && "$BB" sha256sum -c checksums.txt) || finish CHANGED_REV5_PRESERVED 1
        [ -f "$HOOK" ] && [ ! -L "$HOOK" ] || finish OLD_HOOK_CONFLICT 1
        hash=$("$BB" sha256sum "$HOOK") || finish OLD_HOOK_UNREADABLE 1
        set -- $hash
        [ "$1" = "$EXPECTED_HOOK" ] || finish OLD_HOOK_CHANGED 1
        run_child "$USB/uninstall.sh" || finish OLD_UNINSTALL_INCOMPLETE 1
        [ ! -e "$DEST" ] && [ ! -L "$DEST" ] && [ ! -e "$HOOK" ] && [ ! -L "$HOOK" ] || finish OLD_UNINSTALL_INCOMPLETE 1
    fi
fi
run_child "$USB/setup.sh"
code=$?
finish SETUP_FINISHED "$code"
