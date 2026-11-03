# Bass offset (experimental)

Use **Sound → Bass offset** to choose -3 to +3 dB in 0.5 dB steps, then
**Save offset**. Selecting a value alone does not change the sound. The offset
applies across all five native bass levels; the remote's original 1–5 control
and the physical remote continue to work.

The default is 0 dB. Until an offset is explicitly saved, this feature performs
no audio writes. Positive offsets can increase distortion, especially at high
volume. Start with a modest volume when comparing settings.

## Startup and playback

The setting is stored separately in `/var/JBLRemote/bass-offset.json`, for
example `{"offset_db":-1.5}`. It is loaded when the web remote starts; no extra
startup hook or daemon is required. Saving is also possible in standby.

The web remote waits for three stable samples of supported Standard playback,
then reapplies the saved offset after wake, a source/format transition or a
native bass-level change. Smart Mode, custom Front EQ, pending native controls
and unsupported routes defer adjustment. The offset does not turn Smart Mode
off: the existing optional automation handles that separately.

“Saved for startup” describes persistence, not proof of an audio write. The API
reports `original`, `waiting`, `applied` or `paused`; `applied` means verified
DSP readback on the current supported route. No change is promised on an
unsupported route. The polling interval can leave a brief interval of stock
bass after a native level change or wake.

Save 0 dB to restore stock bass on the next supported stable route. Removing
the setting file and restarting the web remote disables monitoring, but does
not immediately undo a currently applied runtime value. Restore 0 dB first.
A native bass-level change or audio-route rebuild normally resets the gain;
automatic reapplication stops when the web remote is no longer running.

Do not run another bass-trim worker alongside this controller. Its lock is
shared with this web remote's controls, not with separate processes or the
firmware's own physical/CEC controls.

## Implementation and limits

On the pinned firmware, the native bass table is -15, -10, -5, -1 and +3 dB.
The target amplitude is always `10^((native_dB + offset_dB)/20)`. It is never
derived by subtracting from the previously adjusted amplitude, so repeated
checks do not compound the offset.

The native integer-dB setter cannot represent half-decibel offsets. This
controller instead writes one aligned float32 amplitude word at offset
`0xdac` in the existing 3,840-byte HDSP block, then verifies the complete block.
It preserves the native bass setting/cache, DSP mode and all other block bytes.

The adapter requires the existing firmware and library hash pins, a stable
process/route, Standard mode, a matching native cached bass value and a known
stock or supported offset amplitude. It preserves unknown tuning. Bass alone
also accepts the qualified HPP movie page `0x840002`; Front EQ remains restricted
to `0x840000`. Unsupported firmware or layouts refuse writes.

Readback failures pause automatic writes for that route. Saving again does not
clear the pause. Repeated checks reduce the chance of writing during a route
transition, but cannot make process-memory writes atomic with firmware changes.
This is experimental runtime tuning, not a firmware patch or a calibrated
acoustic channel-isolation claim.

The underlying amplitude adjustment was listening-tested on one pinned bar
with TV/Standard playback at 48 kHz and the movie pipeline. The listener heard
less bass without an apparent change to voices or the other speakers. These
were subjective checks of the precursor worker. The integrated monitor in
this contribution still needs on-device startup/wake/level-change validation;
other firmware, inputs and formats are not qualified by those listening tests.

## API

The `bar91.bass-offset` extension is registered in
`apps/remote/extensions_bar91.go`. Its monitor uses the existing extension
worker lifecycle and shares the action gate with HTTP controls. The UI lives
in `apps/remote/www/components/bass-offset.js` and uses the same action queue.
No bass-specific fields are added to the shared server command.

`GET /api/features` advertises the `bass-offset` component and its `set` action.
The existing LAN, same-origin and CSRF protections apply:

```json
{"action":"bar91.bass-offset.set","parameters":{"offset_db":-1.5}}
```

Omitting or sending null for `offset_db`, values outside the range, non-half-dB
steps and mixed EQ/offset requests are rejected. `GET /api/state` includes:

```json
{"extensions":{"bar91.bass-offset":{"offset_db":-1.5,"status":"waiting"}}}
```

`error`, when present, explains why the saved value is not currently applied.
