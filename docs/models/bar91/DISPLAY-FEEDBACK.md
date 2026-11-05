# Implementing Bar 9.1 display feedback

Status: 5 November 2026. The native text path is statically identified.
`apps/remote/internal/display/` implements host-tested event, validation and reply
contracts; `bar91.display-feedback` is advertised as unavailable. The native-entry
gate remains unresolved, so there is no adapter, screen animation or automatic
notification hook. The commands proposed below are not implemented or qualified.

Build an optional `notify(event)` hook that lets the installer and custom
actions show short status messages on the front display. Start with one
temporary message and automatic native expiry; then connect the hook to real
actions. Keep this feature specific to the verified Bar 9.1 executable.

Read the [firmware findings](../../../research/bar91-display.md) first. They
contain the evidence, exact build identity and limitations behind this guide.

## 1. Establish the native entry mechanism

This is the first implementation gate. The useful function executes **inside
`bdpprog`**. A standalone C executable, Go function-pointer cast or shell script
cannot call an address in another process. No stock external command accepting
custom display text has been established.

Audit the pinned firmware for a loadable module or native command-registration
mechanism that can host a small adapter. Establish all of the following before
implementing automatic feedback:

- How project-owned code enters the existing native process, with the correct
  ARM ABI and matching loader/library dependencies.
- How it registers one bounded notification endpoint after native startup.
- Which native thread/context may safely call the sender. Inspect its queue,
  logging and initialization dependencies before calling it from a new thread.
- How disable, process restart and uninstall stop requests and remove owned
  resources without stopping audio or changing vendor firmware files.

Prefer a supported, observed native extension mechanism if one is found. If
none exists, treat a transient in-process invocation experiment as a separate
research milestone: record exactly what is attached/changed and how execution
state is restored. Do not make debugger attachment, code injection, native
process restart, startup-wide `LD_PRELOAD`, or memory patching an implicit
installer dependency. A research invocation does not establish a deployable
loader.

Deliverable: a reproducible native-entry proof and lifecycle description. If
this gate remains unresolved, implement only host-side interface fixtures and
report the feature as unavailable. Do not publish a helper that claims to send
messages but cannot reach the native process.

## 2. Implement the bounded native adapter

Proposed source: `models/bar91/remote/native/display_bridge.c`.

Use the Tonly sender at virtual address `0x626d70` with command **`0x5b`**.
This borrows the native temporary version-text slot and its expiry. The sender
copies the payload into the display task queue; the adapter must not write
display selectors, text buffers or timer fields directly.

The sender is Thumb code. On the pinned ARM32 build, an indirect callable
function pointer needs the Thumb bit: **`0x626d71`**. Resolve the ELF load mapping
and verify the expected executable bytes; do not assume a numeric address is
portable across builds or mappings. Check the actual native calling convention
when building the adapter.

The following illustrates only the final in-process call, after admission and
identity checks. It is not a complete adapter or a standalone executable:

```c
#include <stdint.h>
#include <stddef.h>
#include <string.h>

typedef uint32_t (*native_display_sender)(uint32_t, const char *);

static int submit_text_in_native_context(const char *text, size_t length)
{
    char payload[32] = {0};
    if (text == NULL || length == 0 || length > 31)
        return -1;
    for (size_t i = 0; i < length; ++i)
        if ((unsigned char)text[i] < 0x20 || (unsigned char)text[i] > 0x7e)
            return -1;
    memcpy(payload, text, length);

    /* Pinned mapping/Thumb entry; use only after verifying this process. */
    native_display_sender send = (native_display_sender)(uintptr_t)0x626d71;
    (void)send(0x5b, payload);
    return 0; /* Submission attempted; does not prove native consumption. */
}
```

Use an explicit byte length. Reject empty, oversized and non-printable text,
including embedded NUL, line breaks and control bytes. Do not silently truncate.
Reject all-space text. Initially accept only uppercase letters, digits and
spaces at the public interface; expand punctuation after glyph testing.

Before each submission, verify:

- The live executable matches `models/bar91/profile.json`, including its SHA-256,
  mapping and process identity. Cache hashing per process lifetime, not globally.
- Display initialization at `0xd8a090` is true and the task handle at
  `0xd8a0ac` is available, with expected field widths/layout.
- The bar is awake and native controls are idle. Reuse existing state/key
  predicates and decline during calibration, pairing, an actual firmware
  upgrade, Smart Mode gestures or another critical operation.
- The native version-text slot is idle. Do not overwrite a user-requested
  version message. Unknown state means unavailable, not permission to write.

Perform the final admission check and call in the verified native context.
The native sender returns zero on some failures; never use its return value as
proof that text reached the display.

Do not use exported `a_vfdmg_set_command` as a substitute: its inspected body
does not submit the message. Do not use command `0xb3`, which selects a
persistent upgrade-related display state. The stock broker's `disp_msg` and
`scroll_msg start` cases contain fixed demo strings.

## 3. Expose one local notification contract

Proposed flow:

```text
installer/custom action -> bounded client -> local adapter endpoint
                       -> native admission -> Tonly sender(0x5b, text)
                       -> native queue -> display -> native expiry
```

Implement a private UNIX-domain socket, proposed path
`/tmp/jbl-display/notify.sock`, served by the native adapter or its reviewed
dispatcher. Use an owned directory with mode `0700`, socket mode `0600`, and
peer checks. Refuse symlink/unowned path collisions; remove only resources
created by this adapter. No network listener is needed.

Use one bounded JSON request and reply per connection. Proposed request:

```json
{"v":1,"id":"install-4","text":"SSH READY"}
```

Limit the entire request to 256 bytes, reject unknown fields and unsupported
versions, and use a bounded ASCII correlation ID. Reply with the same ID and
one of these precise outcomes:

| Outcome | Meaning |
| --- | --- |
| `submitted` | Native call attempted; consumption not established. |
| `consumed` | A fresh native state transition matched this request. |
| `skipped_busy` | Native state or another message prevented admission. |
| `unavailable` | Missing adapter, unsupported build or native restart/readiness failure. |
| `invalid` | Request validation failed. |
| `timeout` | The bounded request deadline expired; display outcome may be unknown. |

None means that a human saw the screen. Do not report `displayed` from memory
readback alone. Include native PID/start identity in adapter readiness and
invalidate outstanding work when that identity changes.

Suggested initial limits: one active submission, no waiting backlog, a
two-second client deadline, and no automatic retry on timeout. Drop feedback
when busy so old progress messages cannot appear after an action completes.
Clients should coalesce repeated events and never refresh text on every status
poll. Do not accept arbitrary native command IDs, addresses or custom timeouts.

For optional consumption readback, observe temporary selector `1`, the exact
bounded text at `0xd8a120`, and a changed native tick at `0xd8a114` relative to
the pre-submission snapshot. The process must remain the same. An identical
old message, a selector alone or a client correlation ID is insufficient. If
the transition is ambiguous, keep the outcome `submitted`. Bound observation
to the request deadline and keep polling off the native display thread.

## 4. Provide a hook callers can use

Proposed client command, to implement after the native-entry gate:

```sh
jbl-display notify --event ssh_ready
# Developer diagnostic, with the same text validation:
jbl-display notify --text "SSH READY"
```

These commands do not exist yet. Prefer named events for production call sites
so wording and validation live in one place:

| Event | Text | Emit when |
| --- | --- | --- |
| `install_start` | `INSTALLING` | Integrity/preflight passed and installation is starting. |
| `ssh_ready` | `SSH READY` | Current-boot SSH readiness checks succeeded. |
| `remote_ready` | `REMOTE READY` | The remote's current process readiness was verified. |
| `standard_ready` | `STANDARD` | Native readback confirmed Standard Mode. |
| `install_pending_reboot` | `REBOOT` | Receipt is `INSTALLED_PENDING_REBOOT_TEST`. |
| `action_done` | `DONE` | That particular custom action completed successfully. |
| `action_failed` | `FAILED` | That action failed and its normal receipt/log was written. |

The proposed event client translates to the text-only native contract. It
must return promptly when the adapter is absent. A fresh install may have no
adapter at its first stage; skip that message without affecting installation.

In shell callers, wrap the diagnostic client so notification is best effort:

```sh
# Proposed integration after installing/verifying the client implementation.
notify() {
    "$DISPLAY_CLIENT" notify --event "$1" >>"$DISPLAY_LOG" 2>&1 || :
}

# Keep the actual action's result; notification never replaces it.
run_custom_action
action_result=$?
if [ "$action_result" -eq 0 ]; then
    notify action_done
else
    notify action_failed
fi
exit "$action_result"
```

Resolve `DISPLAY_CLIENT` to a verified project-owned binary and `DISPLAY_LOG`
to the operation's existing log. The client must enforce its deadline itself.
Do not run the caller's text through a shell or interpolate it into native CLI
command strings.

## 5. Connect the repository call sites

Use the existing [architecture](../../ARCHITECTURE.md) and
[component contracts](../../COMPONENTS.md). Suggested ownership:

| Location | Work |
| --- | --- |
| `models/bar91/remote/native/display_bridge.c` | Native sender boundary and admission; proposed new source. |
| `apps/remote/display.go` | Typed event client and bounded native response; proposed new source. |
| `apps/remote/extensions_bar91.go` | Optional experimental `bar91.display-feedback` descriptor; include only when adapter support exists. |
| `apps/remote/lifecycle_bar91.go` | Join owned notification workers and close connections during shutdown. |
| `models/bar91/usb/manage.sh` | Stage and completion events after normal checks and receipts. |
| `models/bar91/usb/launch.sh` | Final failure/reboot events after authoritative results are written. |
| `models/bar91/usb/boot.sh` | Optional ready event after current-boot service checks, once qualified. |
| `tools/build_usb.py` and helper builders | Include owned binaries/manifests only after qualification; pin any required vendor dependencies in the existing profile workflow. |

Register the client independently of the browser UI: installer and headless
actions must not require the web remote to be running. Keep arbitrary text out
of a new public HTTP endpoint initially. If an administrative display action is
added later, use the existing authenticated/local action boundary and extension
parameter validation.

Serialize remote notifications with existing action coordination and avoid
recursive acquisition of `r.actionMu` or `device.mu`. Submit feedback after the
action's native cleanup/readback has completed. A Go mutex cannot coordinate a
separate shell installer: all clients must converge on the single adapter's
admission policy, with native busy-state checks performed at dispatch. If
stronger cross-process coordination is needed, define it before enabling hooks;
do not reuse an operation lock that the installer already holds.

Keep `release_sockets()` and detached startup behavior intact. A resident
adapter/client worker must not inherit an accepted vendor IPC socket. Do not
add synchronous display work to an AutoScript/native startup callback.
Emergency `RECOVER` must work without the adapter, its client or its config.

## 6. Qualify before enabling automatic feedback

First use host fixtures for request bounds, unavailable/busy handling, native
restart, stale readback, deadline/duplicate behavior and action exit-code
preservation. These checks validate contracts, not the display hardware.

Then use the owned, identity-matched bar:

1. Capture native process identity, power, source, volume, mute, sound mode,
   display selectors and idle keys. Start with no calibration/pairing/upgrade.
2. Through the reviewed native-entry mechanism, submit one short message,
   such as `TEST`, using `0x5b`. Observe actual text and natural expiry. Record
   expected selector/text/tick transitions without publishing private captures.
3. Verify native source/volume feedback still appears and captured sound
   settings are unchanged. Exercise a real version request before/after; it
   must not be overwritten while active.
4. Test longer text, repeated requests, native controls during visibility,
   busy rejection, initially muted/unmuted states and cancellation. Confirm
   the permitted glyphs, scrolling and observed timeout.
5. Test adapter/client shutdown and native restart. Verify no stale request
   targets a replacement process, no native task hangs and normal controls
   continue. Verify a timed-out client does not leave a persistent overlay.
6. Only then wire installer events. Test success, failure, missing adapter,
   update, uninstall and independent emergency recovery. Preserve exact
   `STATUS.txt` and exit results, including pending reboot qualification.

Record a reviewed summary under
`models/bar91/validation/display-feedback-device.json` with executable/helper
hashes, implementation revision, actual observations and unresolved cases.
Keep detailed native evidence under `research/private/display-access/`.

Completion requires a proven native entry mechanism, observed text and expiry,
normal-control restoration, bounded unavailable behavior, working hooks with
unchanged action results, and independent recovery. Until then, advertise this
as an experimental candidate and leave automatic feedback disabled.
