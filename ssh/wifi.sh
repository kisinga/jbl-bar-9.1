#!/bin/sh
# Add the requested network through the existing supplicant; preserve others.
USB=/mnt/sda1/JBLLocalSSH
. "$USB/common.sh" || exit 1
. "$USB/settings.conf" || exit 1
PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin
export PATH
fail() { printf '%s\n' "$1" > "$USB/WIFI_STATUS.txt"; exit 1; }
ssid=$("$BB" cat "$USB/wifi.ssid.hex") || fail NO_SSID
psk=$("$BB" cat "$USB/wifi.psk.hex") || fail NO_PSK
case "$ssid" in ''|*[!0-9a-f]*) fail INVALID_SSID;; esac
[ "${#ssid}" -le 64 ] && [ $((${#ssid} % 2)) -eq 0 ] || fail INVALID_SSID
case "$psk" in *[!0-9a-f]*) fail INVALID_PSK;; esac
[ "${#psk}" -eq 64 ] || fail INVALID_PSK
WPA=
for p in /bin/wpa_cli /usr/bin/wpa_cli /usr/local/bin/wpa_cli; do
    [ ! -x "$p" ] || { WPA=$p; break; }
done
[ -n "$WPA" ] || fail NO_WPA_CLI
# The shipped CLI prefixes replies with a human-readable interface banner.
# Remove only that exact known banner, retaining return codes and protocol text.
cli() {
    cli_limit=$1
    shift
    cli_reply=$("$BB" timeout "$cli_limit" "$WPA" "$@")
    cli_code=$?
    printf '%s\n' "$cli_reply" | "$BB" sed -e "/^Using interface 'wlan0'$/d" -e "/^Using interface 'ra0'$/d"
    return "$cli_code"
}
IFACE=
CTRL=
CONTROL_KIND=directory
discover() {
    for iface in wlan0 ra0; do
        [ -d "/sys/class/net/$iface" ] || continue
        for ctrl in /data/wifi /var/run/wpa_supplicant /tmp/wpa_supplicant; do
            [ -S "$ctrl/$iface" ] || continue
            pong=$(cli 3 -p "$ctrl" -i "$iface" ping 2>/dev/null)
            case "$pong" in *PONG*) IFACE=$iface; CTRL=$ctrl; CONTROL_KIND=directory; return 0;; esac
        done
    done
    # Android reserves sockets under /dev/socket, without /data/wifi entries.
    for iface in wlan0 ra0; do
        [ -d "/sys/class/net/$iface" ] || continue
        endpoint=/dev/socket/wpa_$iface
        [ -S "$endpoint" ] || continue
        # Android socket names do not establish per-interface vs global scope.
        state=$(cli 3 -g "$endpoint" "IFNAME=$iface" status 2>> "$USB/wifi-control-errors.txt")
        if printf '%s\n' "$state" | "$BB" grep -q '^wpa_state='; then
            IFACE=$iface; CTRL=$endpoint; CONTROL_KIND=global; return 0
        fi
        state=$(cli 3 -g "$endpoint" status 2>> "$USB/wifi-control-errors.txt")
        if printf '%s\n' "$state" | "$BB" grep -q '^wpa_state='; then
            IFACE=$iface; CTRL=$endpoint; CONTROL_KIND=direct; return 0
        fi
    done
    # Read only this daemon's -g endpoint, never infer a controller from a port.
    for pid in $("$BB" pidof wpa_supplicant); do
        endpoint=$("$BB" tr '\000' '\n' < "/proc/$pid/cmdline" | "$BB" awk '$0=="-g" {getline; print; exit} /^-g./ {print substr($0,3); exit}')
        [ -n "$endpoint" ] || continue
        printf 'GLOBAL_ENDPOINT=%s\n' "$endpoint" >> "$USB/wifi-control.txt"
        pong=$(cli 3 -g "$endpoint" ping 2>> "$USB/wifi-control-errors.txt")
        [ "$pong" = PONG ] || continue
        state=$(cli 3 -g "$endpoint" IFNAME=wlan0 status 2>> "$USB/wifi-control-errors.txt")
        if printf '%s\n' "$state" | "$BB" grep -q '^wpa_state='; then
            IFACE=wlan0; CTRL=$endpoint; CONTROL_KIND=global; return 0
        fi
    done
    return 1
}
if ! discover; then
    # C4A vendor startup script uses this init service, not a second daemon.
    [ -x /bin/start ] || fail WIFI_SERVICE_COMMAND_MISSING
    printf '%s\n' STARTING_WIFI_CLIENT > "$USB/WIFI_STATUS.txt"
    "$BB" timeout 5 /bin/start wpa_supplicant > "$USB/wifi-service.txt" 2>&1 || fail WIFI_SERVICE_START_FAILED
    tries=0
    while [ "$tries" -lt 20 ]; do
        discover && break
        "$BB" sleep 1
        tries=$((tries + 1))
    done
    if [ -x /bin/getprop ]; then
        printf 'SERVICE_STATE=' >> "$USB/wifi-service.txt"
        /bin/getprop init.svc.wpa_supplicant >> "$USB/wifi-service.txt" 2>&1
    fi
fi
if [ -z "$IFACE" ]; then
    # A global-only daemon needs an interface attached before it creates sockets.
    for pid in $("$BB" pidof wpa_supplicant); do
        endpoint=$("$BB" tr '\000' '\n' < "/proc/$pid/cmdline" | "$BB" awk '$0=="-g" {getline; print; exit} /^-g./ {print substr($0,3); exit}')
        [ -n "$endpoint" ] || continue
        [ -d /sys/class/net/wlan0 ] && [ -f /data/wifi/wpa_supplicant.conf ] || continue
        pong=$(cli 3 -g "$endpoint" ping 2>> "$USB/wifi-control-errors.txt")
        [ "$pong" = PONG ] || continue
        interfaces=$(cli 3 -g "$endpoint" interface 2>> "$USB/wifi-control-errors.txt") || continue
        printf 'INTERFACES_BEFORE=%s\n' "$interfaces" >> "$USB/wifi-control.txt"
        case "$interfaces" in *wlan0*|*FAIL*|*UNKNOWN*) continue;; esac
        # One attach attempt, only on the already-running native service.
        answer=$(cli 5 -g "$endpoint" interface_add wlan0 /data/wifi/wpa_supplicant.conf nl80211 /data/wifi 2>> "$USB/wifi-control-errors.txt")
        printf 'INTERFACE_ADD=%s\n' "$answer" >> "$USB/wifi-control.txt"
        [ "$answer" = OK ] || continue
        tries=0
        while [ "$tries" -lt 10 ]; do
            discover && break
            "$BB" sleep 1; tries=$((tries + 1))
        done
        break
    done
fi
if [ -z "$IFACE" ]; then
    # Capture exact socket locations and daemon options for the next diagnosis.
    "$BB" awk 'NR==1 || /wpa|wifi|wlan/ {print}' /proc/net/unix > "$USB/wifi-sockets.txt"
    for pid in $("$BB" pidof wpa_supplicant); do
        "$BB" tr '\000' '\n' < "/proc/$pid/cmdline" >> "$USB/wifi-daemon-arguments.txt"
    done
fi
[ -n "$IFACE" ] || fail WIFI_CLIENT_SOCKET_NOT_READY
printf 'INTERFACE=%s\nCONTROL=%s\nCONTROL_KIND=%s\n' "$IFACE" "$CTRL" "$CONTROL_KIND" > "$USB/wifi-info.txt"
"$BB" ip addr show dev "$IFACE" >> "$USB/wifi-info.txt" 2>&1
wpa() {
    case "$CONTROL_KIND" in
        global) cli 4 -g "$CTRL" "IFNAME=$IFACE" "$@" 2>/dev/null;;
        direct) cli 4 -g "$CTRL" "$@" 2>/dev/null;;
        directory) cli 4 -p "$CTRL" -i "$IFACE" "$@" 2>/dev/null;;
    esac
}
ok() {
    answer=$(wpa "$@")
    [ "$answer" = OK ] && return 0
    # Never log command arguments or response text for credential-bearing SETs.
    printf 'FAILED_COMMAND=%s FIELD=%s\n' "$1" "${3:-none}" >> "$USB/wifi-failed-step.txt"
    return 1
}
# Preserve an already-connected target network instead of reprovisioning it.
current=$(wpa status)
current_ssid=$(printf '%s\n' "$current" | "$BB" sed -n 's/^ssid=//p')
current_hex=$(printf '%s' "$current_ssid" | "$BB" od -An -tx1 | "$BB" tr -d ' \n')
if [ "$current_hex" = "$ssid" ] && printf '%s\n' "$current" | "$BB" grep -q '^wpa_state=COMPLETED$'; then
    printf '%s\n' "$current" > "$USB/wifi-state.txt"
    printf '%s\n' CONNECTED_EXISTING_PROFILE_REBOOT_UNVERIFIED > "$USB/WIFI_STATUS.txt"
else
network=
created=0
# Reuse only a network created by this package, identified by its private tag.
for id in $(wpa list_networks | "$BB" awk 'NR>1 && $1 ~ /^[0-9]+$/ {print $1}'); do
    tag=$(wpa get_network "$id" id_str)
    if [ "$tag" = '"jbl-local-ssh"' ]; then network=$id; break; fi
done
if [ -z "$network" ]; then
    reply=$(wpa add_network)
    printf '%s\n' "$reply" > "$USB/add-network-response.txt"
    network=$(printf '%s\n' "$reply" | "$BB" awk '/^[0-9]+$/ {n++; id=$0} END {if(n==1) print id}')
    case "$network" in ''|*[!0-9]*) fail ADD_NETWORK_FAILED;; esac
    created=1
fi
cleanup() {
    [ "$created" != 1 ] || wpa remove_network "$network" >/dev/null
}
trap 'cleanup; fail INTERRUPTED' HUP INT TERM
configure() {
    ok set_network "$network" ssid "$ssid" &&
    ok set_network "$network" psk "$psk" &&
    ok set_network "$network" key_mgmt WPA-PSK &&
    ok set_network "$network" proto RSN &&
    ok set_network "$network" scan_ssid 1 &&
    ok set_network "$network" priority 100 &&
    ok set_network "$network" id_str '"jbl-local-ssh"' &&
    ok enable_network "$network" && ok reconnect
}
configure || { cleanup; fail CONFIGURE_FAILED; }
unset psk
tries=0
connected=0
while [ "$tries" -lt 45 ]; do
    state=$(wpa status)
    printf '%s\n' "$state" > "$USB/wifi-state.txt"
    active=$(printf '%s\n' "$state" | "$BB" sed -n 's/^id=//p')
    if [ "$active" = "$network" ] && printf '%s\n' "$state" | "$BB" grep -q '^wpa_state=COMPLETED$'; then connected=1; break; fi
    tries=$((tries + 1)); "$BB" sleep 1
done
[ "$connected" = 1 ] || { cleanup; fail WIFI_NOT_CONNECTED; }
# Save only through the daemon's own API; back up recognized config first.
for conf in /data/wifi/wpa_supplicant.conf /tmp/WPA_CONF; do
    if [ -f "$conf" ] && [ ! -e "$STORE/wpa-config-before.txt" ]; then
        "$BB" cp -p "$conf" "$STORE/wpa-config-before.txt" || fail WIFI_BACKUP_FAILED
        "$BB" chmod 600 "$STORE/wpa-config-before.txt"
        printf '%s\n' "$conf" > "$STORE/wpa-config-path.txt"
        break
    fi
done
if ok save_config; then
    printf '%s\n' CONNECTED_SAVE_ACCEPTED_REBOOT_UNVERIFIED > "$USB/WIFI_STATUS.txt"
else
    printf '%s\n' CONNECTED_NOT_SAVED > "$USB/WIFI_STATUS.txt"
fi
fi
# Let the existing network manager obtain DHCP; do not run a competing client.
tries=0
while [ "$tries" -lt 30 ]; do
    addr=$(find_address)
    if [ -n "$addr" ]; then
        printf '%s\n' "$addr" > "$STORE/listen-address"
        printf 'IP=%s\n' "$addr" >> "$USB/wifi-info.txt"
        exit 0
    fi
    tries=$((tries + 1)); "$BB" sleep 1
done
fail CONNECTED_WAITING_FOR_DHCP
