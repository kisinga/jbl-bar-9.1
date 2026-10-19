#!/bin/sh
# Scoped owned-device test: temporarily change TWO console data fields only.
# Restore them before exit. Sound change uses the normal key-event handler.
PATH=/bin:/sbin:/usr/bin:/usr/sbin:/system/bin:/system/xbin
export PATH
BASE=$1
PROC=/proc
BB=/bin/busybox
CLI=/bin/cli
EXPECTED_BDP=86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63
EXPECTED_CLI=8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f
EXPECTED_HELPER=ca1d2dd6d7b8ccc1590946432d04334d5a8f88c2320acbae447967d0a43a8644
CHANGED=0
helper_pid=
[ -d "$BASE" ] && [ -x "$BB" ] || exit 1
"$BB" mkdir "$BASE/attempt.lock" 2>/dev/null || exit 0
exec > "$BASE/result.txt" 2>&1
same_process() {
    now=$("$BB" awk '{print $22}' "$PROC/$PID/stat" 2>/dev/null)
    [ -n "$now" ] && [ "$now" = "$START" ]
}
read_state() {
    "$BB" dd if="$PROC/$PID/mem" of="$BASE/$3.bin" bs=1 skip="$1" count="$2" 2>/dev/null || return 1
    n=$("$BB" wc -c < "$BASE/$3.bin")
    [ "$n" -eq "$2" ] || return 1
    "$BB" od -An -tu"$2" "$BASE/$3.bin" | "$BB" tr -d ' \n'
}
write_state() {
    same_process || return 1
    "$BB" dd if="$BASE/$3.bin" of="$PROC/$PID/mem" bs=1 seek="$1" count="$2" conv=notrunc 2>/dev/null || return 1
    read_state "$1" "$2" write-check >/dev/null || return 1
    "$BB" cmp -s "$BASE/$3.bin" "$BASE/write-check.bin"
}
restore_console() {
    [ "$CHANGED" -eq 1 ] || return 0
    same_process || return 1
    # Close the command gate before returning the input enable flag.
    write_state 15427784 4 gate-original || return 1
    write_state 15427780 1 input-original || return 1
    CHANGED=0
    printf 'CONSOLE_RESTORED=1\n'
}
finish() {
    requested_status=$1
    code=$2
    trap '' HUP INT TERM
    if [ -n "$helper_pid" ]; then
        kill -TERM "$helper_pid" 2>/dev/null
        wait "$helper_pid"
        helper_pid=
    fi
    if ! restore_console; then
        printf 'REQUESTED_RESULT=%s\n' "$requested_status"
        requested_status=CONSOLE_RESTORE_FAILED
        code=1
    fi
    printf '%s\n' "$requested_status" > "$BASE/STATUS.txt"
    printf 'RESULT: %s\n' "$requested_status"
    "$BB" sync
    exit "$code"
}
trap 'finish INTERRUPTED 1' HUP INT TERM
printf 'RUNNING\n' > "$BASE/STATUS.txt"
printf 'JBL guarded temporary console enablement and Standard Mode test, revision 2\n'
"$BB" date
"$BB" id
applets=$("$BB" --list) || finish BUSYBOX_UNAVAILABLE 1
for app in sha256sum dd od tr wc pidof awk grep timeout sleep sync cmp touch; do
    printf '%s\n' "$applets" | "$BB" grep -qx "$app" || finish MISSING_APPLET 1
done
[ ! -e "$BASE/CLI_ACK.txt" ] || finish PREEXISTING_MARKER_NO_CHANGE 1
[ -d "$BASE/JBLTest" ] && [ ! -e "$BASE/JBLTest/attempt.lock" ] || finish HELPER_ALREADY_RUN_NO_CHANGE 1
set -- $("$BB" pidof bdpprog)
[ "$#" -eq 1 ] || finish NEED_EXACTLY_ONE_BDPPROG 1
PID=$1
START=$("$BB" awk '{print $22}' "$PROC/$PID/stat")
[ -n "$START" ] || finish PROCESS_NOT_READABLE 1
hash=$("$BB" sha256sum "$PROC/$PID/exe") || finish HASH_FAILED 1
printf 'BDPPROG=%s\n' "$hash"
set -- $hash
[ "$1" = "$EXPECTED_BDP" ] || finish FIRMWARE_MISMATCH_NO_CHANGE 1
hash=$("$BB" sha256sum "$CLI") || finish CLI_HASH_FAILED 1
set -- $hash
[ "$1" = "$EXPECTED_CLI" ] || finish CLI_MISMATCH_NO_CHANGE 1
hash=$("$BB" sha256sum "$BASE/JBLTest/run.sh") || finish HELPER_HASH_FAILED 1
set -- $hash
[ "$1" = "$EXPECTED_HELPER" ] || finish HELPER_MISMATCH_NO_CHANGE 1
"$BB" grep -F 'android:clid' "$PROC/net/unix" > "$BASE/cli-socket.txt" || finish CLI_SOCKET_ABSENT_NO_CHANGE 1
mode=$(read_state 13378532 4 mode-before) || finish MODE_UNREADABLE_NO_CHANGE 1
printf 'MODE_BEFORE=%s\n' "$mode"
case "$mode" in
    0) finish ALREADY_STANDARD_NO_CHANGE 0;;
    3) ;;
    *) finish UNEXPECTED_MODE_NO_CHANGE 1;;
esac
power=$(read_state 14191893 1 power-before) || finish POWER_UNREADABLE_NO_CHANGE 1
system=$(read_state 12962676 4 system-before) || finish SYSTEM_UNREADABLE_NO_CHANGE 1
[ "$power:$system" = 1:0 ] || finish NOT_AWAKE_NO_CHANGE 1
input=$(read_state 15427780 1 input-original) || finish CONSOLE_INPUT_UNREADABLE 1
gate=$(read_state 15427784 4 gate-original) || finish CONSOLE_GATE_UNREADABLE 1
printf 'CONSOLE_INPUT_BEFORE=%s CONSOLE_GATE_BEFORE=%s\n' "$input" "$gate"
case "$input:$gate" in 0:2|1:2|1:0) ;; *) finish UNEXPECTED_CONSOLE_STATE_NO_CHANGE 1;; esac
same_process || finish PROCESS_CHANGED_NO_CHANGE 1
if [ "$input:$gate" != 1:0 ]; then
    printf '\001' > "$BASE/input-enabled.bin"
    printf '\000\000\000\000' > "$BASE/gate-enabled.bin"
    CHANGED=1
    write_state 15427780 1 input-enabled || finish CONSOLE_INPUT_WRITE_FAILED 1
    write_state 15427784 4 gate-enabled || finish CONSOLE_GATE_WRITE_FAILED 1
    printf 'CONSOLE_TEMPORARILY_ENABLED=1\n'
fi
# Empty request wakes a getchar already blocked inside the disabled-input loop.
"$BB" timeout 5 "$CLI" '' || finish CLI_WAKE_FAILED 1
"$BB" sleep 0.5
same_process || finish PROCESS_CHANGED_DURING_TEST 1
"$BB" timeout 5 "$CLI" exec /bin/busybox touch "$BASE/CLI_ACK.txt" || finish CLI_MARKER_REQUEST_FAILED 1
i=0
while [ "$i" -lt 40 ] && [ ! -f "$BASE/CLI_ACK.txt" ]; do
    same_process || finish PROCESS_CHANGED_DURING_TEST 1
    "$BB" sleep 0.25 || finish SLEEP_FAILED 1
    i=$((i + 1))
done
[ -f "$BASE/CLI_ACK.txt" ] || finish CLI_MARKER_NOT_CONFIRMED 1
printf 'CLI_MARKER_CONFIRMED=1\n'
same_process || finish PROCESS_CHANGED_DURING_TEST 1
# This helper rechecks live identity/mode and gates Volume+ on a real long press.
/bin/sh "$BASE/JBLTest/run.sh" "$BASE/JBLTest" &
helper_pid=$!
wait "$helper_pid"
helper_code=$?
helper_pid=
helper_status=$("$BB" cat "$BASE/JBLTest/STATUS.txt" 2>/dev/null)
printf 'MODE_TEST_RESULT=%s EXIT=%s\n' "$helper_status" "$helper_code"
case "$helper_status" in
    STANDARD_SELECTED|ALREADY_STANDARD_NO_CHANGE) finish "$helper_status" 0;;
    '') finish MODE_TEST_NO_STATUS 1;;
    *) finish "$helper_status" 1;;
esac
