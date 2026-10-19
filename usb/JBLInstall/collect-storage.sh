#!/bin/sh
# Read-only device survey. Output goes only to the existing USB mount.
# Prepared for a later test; not installed or launched by the keeper package.
BB=/bin/busybox
OUT=/mnt/sda1/JBLPersistence
[ -x "$BB" ] || exit 1
"$BB" awk '$2=="/mnt/sda1" {found=1} END {exit !found}' /proc/mounts || exit 1
"$BB" mkdir -p "$OUT" || exit 1
"$BB" mkdir "$OUT/capture.lock" 2>/dev/null || exit 0
exec > "$OUT/storage.txt" 2>&1
printf 'JBL persistence survey: filesystem metadata only\n'
"$BB" uname -a
printf '\nMOUNTS\n'
"$BB" cat /proc/mounts
printf '\nMOUNTINFO\n'
"$BB" cat /proc/self/mountinfo
printf '\nMTD LAYOUT\n'
"$BB" cat /proc/mtd
printf '\nPATH METADATA\n'
for path in /var /mnt/ubi_boot /mnt/ubi_boot/var /etc /system/etc /etc/init.d /data /tmp /usr/local/bin/bdpprog /usr/bin/bdpprog.sh /system/usr/bin/bdpprog.sh; do
    printf '\nPATH=%s\n' "$path"
    "$BB" ls -ld "$path"
    "$BB" readlink -f "$path"
    "$BB" stat -L -c 'device=%d inode=%i mode=%a size=%s type=%F' "$path"
done
printf '\nEXISTING HOOK METADATA (no contents)\n'
for path in /var/AutoScript.TSS /mnt/ubi_boot/var/AutoScript.TSS /etc/init.d/rcS.custom /system/etc/init.d/rcS.custom; do
    printf '\nHOOK=%s\n' "$path"
    "$BB" ls -ld "$path"
    "$BB" readlink -f "$path"
    "$BB" stat -L -c 'device=%d inode=%i mode=%a size=%s type=%F' "$path"
    [ ! -f "$path" ] || "$BB" sha256sum "$path"
done
printf '\nINIT EXECUTABLE\n'
"$BB" readlink /proc/1/exe
printf '\nPROCESS IDENTITY (names only, no arguments/environment)\n'
for task in /proc/[0-9]*; do
    [ ! -r "$task/stat" ] || "$BB" awk '{print $1,$2,$3,$4}' "$task/stat"
done
printf '\nSURVEY_COMPLETE\n'
"$BB" sync
