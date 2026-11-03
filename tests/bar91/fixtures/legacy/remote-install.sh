#!/bin/sh
# Run over SSH after copying an owner-built directory to /var/JBLRemote.
BB=/bin/busybox
STORE=/var/JBLRemote
HOOK=/var/AutoScript.TSS
[ -d "$STORE" ] && [ ! -L "$STORE" ] || exit 1
[ "$("$BB" cat "$STORE/OWNER.txt")" = JBL_WEB_REMOTE_REV1 ] || exit 1
(cd "$STORE" && "$BB" sha256sum -c SHA256SUMS) || exit 1
"$BB" chmod 700 "$STORE" "$STORE/remote" || exit 1
"$BB" chmod 600 "$STORE/config.json" || exit 1
if [ ! -e "$STORE/hook.with-web" ]; then
    [ ! -e "$HOOK" ] || { [ -f "$HOOK" ] && [ ! -L "$HOOK" ]; } || exit 1
    if [ -e "$HOOK" ]; then
        "$BB" cp "$HOOK" "$STORE/hook.before-web" || exit 1
    else
        : > "$STORE/hook.before-web"; : > "$STORE/hook.was-absent"
    fi
    "$BB" cp "$STORE/hook.before-web" "$STORE/hook.with-web" || exit 1
    # A leading newline keeps a preceding unterminated command intact.
    printf '\r\nCLI(CLI_exec sh /var/JBLRemote/start.sh &)\r\n' >> "$STORE/hook.with-web"
elif ! "$BB" cmp -s "$HOOK" "$STORE/hook.with-web" && ! "$BB" cmp -s "$HOOK" "$STORE/hook.before-web"; then
    printf 'Startup hook changed; preserve it and review manually.\n'; exit 1
fi
[ ! -L "$HOOK.jbl-web-new" ] || exit 1
"$BB" cp "$STORE/hook.with-web" "$HOOK.jbl-web-new" || exit 1
"$BB" chmod 600 "$HOOK.jbl-web-new" || exit 1
"$BB" mv "$HOOK.jbl-web-new" "$HOOK" || exit 1
"$BB" rm -f "$STORE/DISABLED"
"$BB" sync
/bin/sh "$STORE/start.sh"
printf 'Web remote startup installed. Open http://SOUNDBAR-IP:8080\n'
