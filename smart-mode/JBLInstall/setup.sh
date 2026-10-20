#!/bin/sh
PATH=/bin:/sbin:/usr/bin:/usr/sbin:/system/bin:/system/xbin
export PATH
BB=/bin/busybox
USB=/mnt/sda1/JBLInstall
DEST=/var/JBLStandard
HOOK=/var/AutoScript.TSS
RAM=/tmp/jbl-standard-mutation-active
EXPECTED_MANIFEST=214c8b621d92f5d528bf6a256f3e058bcda23438fdc05677ca242ba50ee95c80
[ -x "$BB" ] && [ -d "$USB" ] || exit 1
if [ -f "$USB/ROLLBACK" ]; then exec /bin/sh "$USB/uninstall.sh"; fi
"$BB" awk '$2=="/tmp" && $3=="tmpfs" {ok=1} END {exit !ok}' /proc/mounts || exit 1
"$BB" mkdir "$RAM" 2>/dev/null || exit 0
exec >> "$USB/install.log" 2>&1
created=0
activated=0
trial_pid=
finish() {
    result=$1
    code=$2
    trap '' HUP INT TERM
    if [ -n "$trial_pid" ]; then
        kill -TERM "$trial_pid" 2>/dev/null
        wait "$trial_pid"
        trial_pid=
    fi
    if [ "$activated" = 1 ] && [ "$code" != 0 ]; then
        printf 'INSTALL_INTERRUPTED\n' > "$DEST/DISABLED"
        if [ -f "$HOOK" ] && [ ! -L "$HOOK" ] && "$BB" cmp -s "$HOOK" "$USB/payload/hook.expected"; then
            "$BB" rm "$HOOK" && activated=0
        fi
    fi
    if [ "$created" = 1 ] && [ "$activated" = 0 ]; then
        "$BB" rm -rf "$DEST"
    fi
    printf 'RESULT=%s EXIT=%s\n' "$result" "$code"
    printf '%s\n' "$result" > "$USB/INSTALL_STATUS.txt"
    "$BB" sync
    "$BB" rmdir "$RAM"
    exit "$code"
}
trap 'finish INTERRUPTED 1' HUP INT TERM
printf '\nINSTALL_REV5 BOOT_ID='; "$BB" cat /proc/sys/kernel/random/boot_id
printf 'UPTIME='; "$BB" cat /proc/uptime
[ ! -e /tmp/jbl-standard-keeper ] || finish OLD_KEEPER_PRESENT_REBOOT_WITHOUT_USB 1
applets=$("$BB" --list) || finish BUSYBOX_UNAVAILABLE 1
for app in awk sha256sum cp mkdir rm rmdir readlink ln cmp sync dd od tr wc pidof grep timeout sleep touch cat stat cut mv; do
    printf '%s\n' "$applets" | "$BB" grep -qx "$app" || finish MISSING_APPLET 1
done
"$BB" ln --help 2>&1 | "$BB" grep -q -- '-T' || finish ATOMIC_LINK_UNAVAILABLE 1
hash=$("$BB" sha256sum "$USB/payload/checksums.txt") || finish MANIFEST_UNREADABLE 1
set -- $hash
[ "$1" = "$EXPECTED_MANIFEST" ] || finish MANIFEST_MISMATCH 1
(cd "$USB/payload" && "$BB" sha256sum -c checksums.txt) || finish PAYLOAD_MISMATCH 1
# Survey is evidence only; all decisions below use live checks.
/bin/sh "$USB/collect-storage.sh"
resolved=$("$BB" readlink -f /var) || finish VAR_UNRESOLVED 1
[ "$resolved" = /mnt/ubi_boot/var ] || finish VAR_NOT_VERIFIED_PERSISTENT 1
[ "$("$BB" readlink -f /mnt/ubi_boot)" = /mnt/ubi_boot ] || finish UBI_PATH_UNEXPECTED 1
# Require the most specific covering mount to be writable UBIFS at this mountpoint.
"$BB" awk -v target="$resolved" '
 ($2=="/" || target==$2 || index(target,$2 "/")==1) && length($2)>best {
   best=length($2); point=$2; type=$3; options="," $4 ","
 }
 END {exit !(point=="/mnt/ubi_boot" && type=="ubifs" && index(options,",rw,"))}
' /proc/mounts || finish VAR_MOUNT_NOT_VERIFIED_WRITABLE_UBIFS 1
[ -d "$resolved" ] || finish VAR_NOT_DIRECTORY 1
# Reinsertion after installation exports logs; it never replays keys or reinstalls.
if [ -e "$DEST" ] || [ -L "$DEST" ]; then
    [ ! -L "$DEST" ] && [ -d "$DEST" ] || finish DESTINATION_CONFLICT 1
    "$BB" cmp -s "$DEST/checksums.txt" "$USB/payload/checksums.txt" || finish INSTALLED_VERSION_CONFLICT 1
    (cd "$DEST" && "$BB" sha256sum -c checksums.txt) || finish INSTALLED_FILES_CHANGED 1
    [ ! -L "$HOOK" ] && [ -f "$HOOK" ] && "$BB" cmp -s "$HOOK" "$USB/payload/hook.expected" || finish INSTALLED_HOOK_CHANGED 1
    "$BB" rm -rf "$USB/runtime"
    "$BB" mkdir -p "$USB/runtime"
    for f in latest.log previous.log STATUS.txt DISABLED watch.log watch-previous.log WATCH_STATUS.txt; do
        [ ! -f "$DEST/$f" ] || "$BB" cp "$DEST/$f" "$USB/runtime/"
    done
    if [ -d "$DEST/last-attempt" ]; then
        "$BB" rm -rf "$USB/runtime/last-attempt"
        "$BB" cp -R "$DEST/last-attempt" "$USB/runtime/last-attempt"
    fi
    finish ALREADY_INSTALLED_LOGS_EXPORTED 0
fi
[ ! -e "$HOOK" ] && [ ! -L "$HOOK" ] || finish EXISTING_AUTOSCRIPT_PRESERVED_NO_INSTALL 1
# First prove this revised one-shot finishes before enabling any boot hook.
[ ! -L "$USB/trial" ] || finish TRIAL_PATH_CONFLICT 1
"$BB" mkdir -p "$USB/trial" || finish TRIAL_SETUP_FAILED 1
"$BB" rm -f "$USB/trial/STATUS.txt" || finish TRIAL_SETUP_FAILED 1
/bin/sh "$USB/payload/once.sh" "$USB/trial" &
trial_pid=$!
wait "$trial_pid"
trial_code=$?
trial_pid=
trial=$("$BB" cat "$USB/trial/STATUS.txt" 2>/dev/null)
printf 'TRIAL=%s EXIT=%s\n' "$trial" "$trial_code"
case "$trial:$trial_code" in STANDARD_SELECTED:0|ALREADY_STANDARD_NO_CHANGE:0) ;; *) finish TRIAL_NOT_SUCCESSFUL_NO_INSTALL 1;; esac
[ ! -e "$USB/ROLLBACK" ] || finish CANCELLED_NO_INSTALL 1
[ ! -e "$HOOK" ] && [ ! -L "$HOOK" ] || finish HOOK_APPEARED_NO_INSTALL 1
"$BB" mkdir "$DEST" || finish DESTINATION_CREATE_FAILED 1
created=1
for f in run.sh once.sh controller.sh helper.sh hook.expected OWNER.txt checksums.txt; do
    "$BB" cp "$USB/payload/$f" "$DEST/$f" || finish PAYLOAD_COPY_FAILED 1
done
(cd "$DEST" && "$BB" sha256sum -c checksums.txt) || finish INSTALLED_READBACK_FAILED 1
printf 'PREINSTALL_HOOK=ABSENT\nVERSION=5\nVAR=%s\n' "$resolved" > "$DEST/INSTALL.txt" || finish INSTALL_RECORD_FAILED 1
"$BB" cp "$DEST/INSTALL.txt" "$USB/INSTALL.txt" || finish BACKUP_RECORD_FAILED 1
"$BB" sync
# Same-filesystem hard link activates a complete hook atomically without clobbering.
[ ! -e "$USB/ROLLBACK" ] || finish CANCELLED_NO_INSTALL 1
# Do not interrupt the tiny activation/flag transition; no commands execute here.
trap '' HUP INT TERM
"$BB" ln -T "$DEST/hook.expected" "$HOOK"
link_code=$?
[ "$link_code" != 0 ] || activated=1
trap 'finish INTERRUPTED 1' HUP INT TERM
[ "$link_code" = 0 ] || finish HOOK_ACTIVATION_FAILED 1
[ -f "$HOOK" ] && [ ! -L "$HOOK" ] && "$BB" cmp -s "$HOOK" "$USB/payload/hook.expected" || finish HOOK_READBACK_FAILED 1
"$BB" sync
/bin/sh "$DEST/run.sh" >/dev/null 2>&1 &
finish INSTALLED_PENDING_REBOOT_TEST 0
