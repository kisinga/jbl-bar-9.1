#!/bin/sh
# Install per-owner SSH settings; no firmware or vendor account file is edited.
PATH=/bin:/sbin:/usr/bin:/usr/sbin
export PATH
unset LD_LIBRARY_PATH LD_PRELOAD
ulimit -c 0
USB=/mnt/sda1/JBLLocalSSH
exec </dev/null >/dev/null 2>&1
. "$USB/common.sh" || exit 1
release_sockets || exit 1
LOCK=/tmp/jbl-local-ssh-setup-lock
"$BB" mkdir "$LOCK" 2>/dev/null || exit 0
trap '"$BB" rmdir "$LOCK"' EXIT
umask 077
exec >> "$USB/setup.log" 2>&1
finish() {
    printf '%s\n' "$1" > "$USB/STATUS.txt"
    printf 'RESULT=%s\n' "$1"
    "$BB" sync
    exit "$2"
}
trap 'finish INTERRUPTED 1' HUP INT TERM
[ "$("$BB" id -u)" = 0 ] || finish ROOT_REQUIRED 1
(cd "$USB" && "$BB" sha256sum -c checksums.txt) >/dev/null 2>&1 || finish USB_INTEGRITY_FAILED 1
verify_tools || finish UNSUPPORTED_NATIVE_SSH_BUILD 1
. "$USB/settings.conf" || finish SETTINGS_MISSING 1
[ -f /etc/passwd ] && [ -f /etc/nsswitch.conf ] || finish ACCOUNT_FILES_MISSING 1
if [ -e "$STORE" ] || [ -L "$STORE" ]; then
    owned_store || finish STORE_CONFLICT 1
    [ "$("$BB" stat -c %u "$STORE")" = 0 ] || finish STORE_OWNER_CONFLICT 1
else
    "$BB" mkdir -m 700 "$STORE" || finish STORE_CREATE_FAILED 1
    printf '%s\n' "$OWNER" > "$STORE/OWNER.txt"
fi
"$BB" chmod 700 "$STORE" || finish STORE_PERMISSION_FAILED 1
copy_file() {
    [ ! -L "$STORE/$1" ] && [ ! -e "$STORE/$1.new" ] && [ ! -L "$STORE/$1.new" ] || return 1
    "$BB" cp "$USB/$1" "$STORE/$1.new" && "$BB" chmod 600 "$STORE/$1.new" && "$BB" mv "$STORE/$1.new" "$STORE/$1"
}
support_files() {
    for name in common.sh settings.conf boot.sh hooks.sh rollback-boot.sh; do
        copy_file "$name" || return 1
    done
}
install_hook() {
    [ "$AUTOSTART" != 1 ] || /bin/sh "$STORE/hooks.sh" install
}
export_info() {
    for name in connection.txt host_ed25519.pub; do
        [ ! -f "$STORE/$name" ] || "$BB" cp "$STORE/$name" "$USB/$name"
    done
}
# Upgrading an existing daemon must still install the boot helper. Do not
# silently rotate a live installation's password or disconnect its sessions.
if daemon_running; then
    "$BB" grep -qx "AllowUsers root@$ALLOWED_PC" "$STORE/sshd_config" || finish RUNNING_ALLOWED_PC_DIFFERS 1
    support_files || finish COPY_FAILED 1
    install_hook || finish BOOT_HOOK_CONFLICT 1
    export_info
    finish ALREADY_RUNNING_PASSWORD_UNCHANGED 0
fi
# A live, unrelated process must never be replaced on the strength of a PID.
if [ -f "$STORE/sshd.pid" ]; then
    saved=$("$BB" cat "$STORE/sshd.pid")
    case "$saved" in
        ''|*[!0-9]*|0|1) finish INVALID_SAVED_PID 1;;
        *) kill -0 "$saved" 2>/dev/null && finish PID_CONFLICT 1;;
    esac
fi
if [ -f "$USB/wifi.ssid.hex" ]; then
    "$BB" timeout 130 /bin/sh "$USB/wifi.sh" || finish WIFI_SETUP_INCOMPLETE 1
fi
tries=0
addr=
while [ "$tries" -lt 30 ]; do
    addr=$(find_address)
    [ -z "$addr" ] || break
    "$BB" sleep 1
    tries=$((tries + 1))
done
[ -n "$addr" ] || finish NO_ADDRESS_ON_ALLOWED_PC_SUBNET 1
printf '%s\n' "$addr" > "$STORE/listen-address"
copy_file password.hash || finish COPY_FAILED 1
"$BB" awk 'NR==1 && /^\$6\$[A-Za-z0-9.\/]+\$[A-Za-z0-9.\/]+$/ {ok=1} END {exit !(ok && NR==1)}' "$STORE/password.hash" || finish INVALID_PASSWORD_HASH 1
"$BB" awk -F: '$1=="root" {n++; if($3!=0 || $7!="/bin/sh") exit 1} END {if(n!=1) exit 1}' /etc/passwd || finish ROOT_ACCOUNT_UNEXPECTED 1
if [ -e /etc/shadow ]; then
    [ -f /etc/shadow ] || finish SHADOW_TYPE_UNEXPECTED 1
    "$BB" awk -F: '$1=="root" {n++} END {exit n!=1}' /etc/shadow || finish ROOT_SHADOW_UNEXPECTED 1
    "$BB" awk -F: -v OFS=: '$1=="root" {$2="x"} {print}' /etc/passwd > "$STORE/passwd.new" || finish ACCOUNT_COPY_FAILED 1
    "$BB" awk -F: -v OFS=: -v hp="$STORE/password.hash" 'BEGIN {getline hash < hp; close(hp)} $1=="root" {$2=hash} {print}' /etc/shadow > "$STORE/shadow.new" || finish ACCOUNT_COPY_FAILED 1
    "$BB" mv "$STORE/shadow.new" "$STORE/shadow" || finish ACCOUNT_COPY_FAILED 1
else
    "$BB" rm -f "$STORE/shadow"
    "$BB" awk -F: -v OFS=: -v hp="$STORE/password.hash" 'BEGIN {getline hash < hp; close(hp)} $1=="root" {$2=hash} {print}' /etc/passwd > "$STORE/passwd.new" || finish ACCOUNT_COPY_FAILED 1
fi
"$BB" mv "$STORE/passwd.new" "$STORE/passwd" || finish ACCOUNT_COPY_FAILED 1
"$BB" awk '/^[[:space:]]*(passwd|group|shadow):/ {next} {print} END {print "passwd: files\ngroup: files\nshadow: files"}' /etc/nsswitch.conf > "$STORE/nsswitch.conf.new" || finish NSS_COPY_FAILED 1
"$BB" mv "$STORE/nsswitch.conf.new" "$STORE/nsswitch.conf" || finish NSS_COPY_FAILED 1
for name in namespace.sh session.sh; do copy_file "$name" || finish COPY_FAILED 1; done
: > "$STORE/ld.so.preload"
"$BB" chmod 644 "$STORE/passwd" "$STORE/nsswitch.conf" "$STORE/ld.so.preload" || finish MODE_FAILED 1
prepare_privsep || finish PRIVSEP_CHECK_FAILED 1
cat > "$STORE/sshd_config.new" <<CONFIG
Port 2222
AddressFamily inet
ListenAddress $addr
HostKey $STORE/host_ed25519
PidFile $STORE/sshd.pid
PermitRootLogin yes
AllowUsers root@$ALLOWED_PC
PasswordAuthentication yes
PermitEmptyPasswords no
ChallengeResponseAuthentication no
PubkeyAuthentication no
AuthorizedKeysFile none
StrictModes yes
UseDNS no
LoginGraceTime 30
MaxAuthTries 3
MaxStartups 3:30:6
AllowTcpForwarding no
AllowAgentForwarding no
X11Forwarding no
PermitTunnel no
PrintMotd no
PrintLastLog no
ForceCommand /bin/busybox sh /var/JBLLocalSSH/session.sh
Subsystem sftp internal-sftp
CONFIG
"$BB" mv "$STORE/sshd_config.new" "$STORE/sshd_config" || finish CONFIG_SAVE_FAILED 1
support_files || finish COPY_FAILED 1
"$BB" unshare -m --propagation private /bin/sh "$STORE/namespace.sh" > "$STORE/start.log" 2>&1 </dev/null &
child=$!
if ! wait_listener "$child"; then
    "$BB" cp "$STORE/start.log" "$USB/ssh-start.log"
    [ ! -f "$STORE/sshd.log" ] || "$BB" cp "$STORE/sshd.log" "$USB/sshd.log"
    finish SSH_LISTENER_NOT_CONFIRMED 1
fi
connection_info || finish CONNECTION_INFO_FAILED 1
export_info
# Enable automatic startup only after a listener has been confirmed.
install_hook || finish SSH_RUNNING_BOOT_HOOK_CONFLICT 1
finish SSH_READY 0
