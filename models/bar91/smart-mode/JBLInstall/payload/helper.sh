#!/bin/sh
# One-shot test on an explicitly identified JBL firmware build.
# Uses normal key events. Reads process memory; never writes it.
PATH=/bin:/sbin:/usr/bin:/usr/sbin:/system/bin:/system/xbin
export PATH
BASE=$1
PROC=/proc
BB=/bin/busybox
CLI=/bin/cli
EXPECTED_BDP=86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63
EXPECTED_CLI=8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f

[ -d "$BASE" ] && [ -x "$BB" ] || exit 1
# Retain the first attempt and prevent another insertion from toggling again.
"$BB" mkdir "$BASE/attempt.lock" 2>/dev/null || exit 0
exec > "$BASE/result.txt" 2>&1

finish() {
    printf '%s\n' "$1" > "$BASE/STATUS.txt"
    printf 'RESULT: %s\n' "$1"
    "$BB" sync
    exit "$2"
}

read_state() {
    # $1 is a fixed numeric address, $2 is 1 or 4, $3 a fixed output label.
    "$BB" dd if="$PROC/$PID/mem" of="$BASE/$3.bin" bs=1 skip="$1" count="$2" 2>/dev/null || return 1
    n=$("$BB" wc -c < "$BASE/$3.bin")
    [ "$n" -eq "$2" ] || return 1
    "$BB" od -An -tu"$2" "$BASE/$3.bin" | "$BB" tr -d ' \n'
}

same_process() {
    now=$("$BB" awk '{print $22}' "$PROC/$PID/stat" 2>/dev/null)
    [ -n "$now" ] && [ "$now" = "$START_TIME" ]
}

awake() {
    same_process || return 1
    power=$(read_state 14191893 1 power-check) || return 1
    system=$(read_state 12962676 4 system-check) || return 1
    [ "$power:$system" = 1:0 ]
}

trap 'finish INTERRUPTED 1' HUP INT TERM
printf 'RUNNING\n' > "$BASE/STATUS.txt"
printf 'JBL Standard Mode one-shot test, revision 2 (qualified command and verified release)\n'
"$BB" date
"$BB" id
"$BB" uname -a

applets=$("$BB" --list) || finish BUSYBOX_UNAVAILABLE 1
for app in sha256sum dd od tr wc pidof awk grep timeout sleep sync; do
    printf '%s\n' "$applets" | "$BB" grep -qx "$app" || finish MISSING_APPLET 1
done

set -- $("$BB" pidof bdpprog)
[ "$#" -eq 1 ] || finish NEED_EXACTLY_ONE_BDPPROG 1
PID=$1
case "$PID" in ''|*[!0-9]*) finish INVALID_PID 1;; esac
START_TIME=$("$BB" awk '{print $22}' "$PROC/$PID/stat")
[ -n "$START_TIME" ] || finish PROCESS_NOT_READABLE 1
printf 'PID=%s START_TIME=%s\n' "$PID" "$START_TIME"
"$BB" readlink "$PROC/$PID/exe"
"$BB" cat "$PROC/$PID/maps" > "$BASE/process-maps.txt"

bdp_hash=$("$BB" sha256sum "$PROC/$PID/exe") || finish EXECUTABLE_HASH_FAILED 1
printf 'BDPPROG=%s\n' "$bdp_hash"
set -- $bdp_hash
[ "$1" = "$EXPECTED_BDP" ] || finish FIRMWARE_MISMATCH_NO_CHANGE 1

mode=$(read_state 13378532 4 mode-before) || finish MODE_UNREADABLE_NO_CHANGE 1
printf 'MODE_BEFORE=%s (0=Standard, 3=Smart)\n' "$mode"
case "$mode" in
    0) finish ALREADY_STANDARD_NO_CHANGE 0;;
    3) ;;
    *) finish UNEXPECTED_MODE_NO_CHANGE 1;;
esac

[ -x "$CLI" ] || finish CLI_UNAVAILABLE_NO_CHANGE 1
cli_hash=$("$BB" sha256sum "$CLI") || finish CLI_HASH_FAILED_NO_CHANGE 1
printf 'CLI=%s\n' "$cli_hash"
set -- $cli_hash
[ "$1" = "$EXPECTED_CLI" ] || finish CLI_MISMATCH_NO_CHANGE 1
"$BB" grep -F 'android:clid' "$PROC/net/unix" > "$BASE/cli-socket.txt" || finish CLI_SOCKET_ABSENT_NO_CHANGE 1

# Let any previous remote combination expire before generating a new one.
"$BB" sleep 2
adapter=$(read_state 13822200 4 adapter-type) || finish ADAPTER_UNREADABLE_NO_CHANGE 1
down=$(read_state 13822224 1 down-before) || finish INPUT_UNREADABLE_NO_CHANGE 1
[ "$adapter:$down" = 1:0 ] || finish INPUT_BUSY_NO_CHANGE 1
combo=$(read_state 14200904 4 combo-before) || finish COMBO_UNREADABLE_NO_CHANGE 1
repeat=$(read_state 14200898 1 repeat-before) || finish REPEAT_UNREADABLE_NO_CHANGE 1
printf 'COMBO_BEFORE=%s REPEAT_BEFORE=%s\n' "$combo" "$repeat"
[ "$combo" = 0 ] && [ "$repeat" = 0 ] || finish REMOTE_BUSY_NO_CHANGE 1
same_process || finish PROCESS_CHANGED_NO_CHANGE 1
mode=$(read_state 13378532 4 mode-recheck) || finish MODE_UNREADABLE_NO_CHANGE 1
[ "$mode" = 3 ] || finish MODE_CHANGED_NO_CHANGE 1

awake || finish NOT_AWAKE_NO_CHANGE 1
printf 'Sending Mute hold: mw.iom.se 0x6f002 3500\n'
"$BB" timeout 5 "$CLI" mw.iom.se 0x6f002 3500 || finish MUTE_COMMAND_FAILED 1

# CLI delivery is asynchronous. Wait for the actual long-press/release state,
# not merely a successful socket write. No automatic retries of either key.
ready=0
i=0
while [ "$i" -lt 60 ]; do
    same_process || finish PROCESS_CHANGED_DURING_TEST 1
    combo=$(read_state 14200904 4 combo-poll) || finish COMBO_READ_FAILED_DURING_TEST 1
    repeat=$(read_state 14200898 1 repeat-poll) || finish REPEAT_READ_FAILED_DURING_TEST 1
    if [ "$combo" = 1 ] && [ "$repeat" = 1 ]; then
        ready=1
        break
    fi
    "$BB" sleep 0.1 || finish SLEEP_FAILED_DURING_TEST 1
    i=$((i + 1))
done
printf 'LONG_PRESS_READY=%s COMBO=%s REPEAT=%s\n' "$ready" "$combo" "$repeat"
[ "$ready" = 1 ] || finish LONG_PRESS_NOT_CONFIRMED 1
mode=$(read_state 13378532 4 mode-before-volume) || finish MODE_READ_FAILED_DURING_TEST 1
[ "$mode" = 3 ] || finish MODE_CHANGED_DURING_TEST 1
same_process || finish PROCESS_CHANGED_DURING_TEST 1

awake || finish POWER_CHANGED_DURING_TEST 1
printf 'Sending Volume+: mw.iom.se 0x6f000 50\n'
"$BB" timeout 5 "$CLI" mw.iom.se 0x6f000 50 || finish VOLUME_COMMAND_FAILED 1
i=0
while [ "$i" -lt 30 ]; do
    same_process || finish PROCESS_CHANGED_DURING_TEST 1
    mode=$(read_state 13378532 4 mode-after) || finish MODE_READ_FAILED_DURING_TEST 1
    if [ "$mode" = 0 ]; then
        printf 'MODE_AFTER=0\n'
        # A mode change happens on key DOWN; prove sender completion and UP.
        [ ! -e "$BASE/CLI_DONE.txt" ] || finish PREEXISTING_DONE_MARKER 1
        "$BB" timeout 5 "$CLI" exec /bin/busybox touch "$BASE/CLI_DONE.txt" || finish COMPLETION_REQUEST_FAILED 1
        j=0
        while [ "$j" -lt 40 ] && [ ! -f "$BASE/CLI_DONE.txt" ]; do
            same_process || finish PROCESS_CHANGED_DURING_TEST 1
            "$BB" sleep 0.25
            j=$((j + 1))
        done
        [ -f "$BASE/CLI_DONE.txt" ] || finish COMMAND_COMPLETION_NOT_CONFIRMED 1
        printf 'CLI_COMMAND_COMPLETED=1\n'
        "$BB" sleep 2
        j=0
        while [ "$j" -lt 20 ]; do
            same_process || finish PROCESS_CHANGED_DURING_TEST 1
            down=$(read_state 13822224 1 down-after) || finish RELEASE_UNREADABLE 1
            combo=$(read_state 14200904 4 combo-after) || finish RELEASE_UNREADABLE 1
            repeat=$(read_state 14200898 1 repeat-after) || finish RELEASE_UNREADABLE 1
            printf 'RELEASE_CHECK DOWN=%s COMBO=%s REPEAT=%s\n' "$down" "$combo" "$repeat"
            if [ "$down:$combo:$repeat" = 0:0:0 ]; then
                mode=$(read_state 13378532 4 mode-final) || finish FINAL_MODE_UNREADABLE 1
                [ "$mode" = 0 ] || finish FINAL_MODE_CHANGED 1
                printf 'KEY_RELEASE_AND_COMBO_IDLE=1\n'
                finish STANDARD_SELECTED 0
            fi
            "$BB" sleep 0.25
            j=$((j + 1))
        done
        finish INPUT_RELEASE_NOT_CONFIRMED 1
    fi
    "$BB" sleep 0.1 || finish SLEEP_FAILED_DURING_TEST 1
    i=$((i + 1))
done
printf 'MODE_AFTER=%s\n' "$mode"
finish STANDARD_NOT_CONFIRMED 1
