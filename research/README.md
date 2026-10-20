# Research workspace

Use the root [instructions](../README.md) and [build guide](../CONTRIBUTING.md) for supported workflows. Keep experimental findings separate from hardware confirmation. Preserve the confirmed release, recovery paths and firmware guards while extending sound controls.

## Private archive

The original material remains in local, Git-ignored directories, absent from public clones/releases:

| Path under `research/` | Contents |
| --- | --- |
| `private/firmware-workspace/` | Vendor downloads, candidates, REA exports, scripts and hardware evidence |
| `private/spikes-jbl-firmware/` | Extracted rootfs, Ghidra projects/scripts and earlier binary analysis |
| `private/local-ssh/` | Wi-Fi/SSH attempts, private configuration and boot rollback evidence |
| `private/other-notes/XOR_ANALYSIS.md` | Earlier container/XOR exploration, not a release dependency |
| `private/migration.json` | Verified migration hashes, sizes and symlink targets |

Compatibility symlinks preserve historical paths; the original trees were moved and verified rather than duplicated. Do not upload this archive for support. Publish reviewed summaries and narrowly relevant text logs, excluding credentials, keys, personal configuration and proprietary binaries.

Useful paths beneath `private/firmware-workspace/`:

| Path | Evidence |
| --- | --- |
| `INVESTIGATION-STATUS.txt` | Chronological findings; later results supersede earlier uncertainty |
| `COMMUNITY-PRIOR-WORK.txt` | Bounded prior-art search and links |
| `analysis/reversible-smart-mode/runtime-socket-retry-1/` | Confirmed cold boot, checksummed logs and `ASSESSMENT.txt` |
| `analysis/reversible-smart-mode/runtime-wake-rollback-1/` | LOAD incident and recovery |
| `analysis/reversible-smart-mode/runtime-detached-retry-1/` | Revision 4 early exit |
| `analysis/reversible-smart-mode/startup-hang/` | IPC analysis and ARM-shell/socket fixtures |
| `analysis/reversible-smart-mode/socket-retry/` | Revision 5 implementation/staging |
| `analysis/reversible-smart-mode/scripts/` | Historical tests and REA automation |
| `analysis/eq-native/` | DSP identities, parser/instance evidence and exact tuning diffs |
| `analysis/bluetooth-ui/`, `analysis/cast-ui/`, `analysis/sound-controls/` | Native field/event evidence and passive captures |
| `analysis/track-ui/`, `analysis/discovery-ui/`, `analysis/community-cli/` | Metadata, AVRCP helper, Bonjour and upstream review |

Exploit/signature-bypass ideas in historical notes are exploratory, not working release prerequisites. The supported route uses the published AutoScript entry point with build-specific guards and startup fixes.

## EQ exploration — 20 October 2026

**Front EQ has audible confirmation over Bluetooth. Keep the optional implementation experimental: other sources/layouts remain unqualified.**

The active JBL HPP/HDSP pipeline supports custom tuning. Matching packaged/runtime libraries expose **22 EQ groups / 164 bands**; the initially identified four five-band groups are a subset. The captured parameter block is **3,840 bytes**. Frequency uses Hz, gain uses dB, and filter types 11/16/17 mean bell/high-shelf/low-shelf. The parser itself does not impose safe bounds.

The firmware’s JSON parser, run in isolated QEMU on the computer with read-only vendor libraries, produced:

| Offline case | Result |
| --- | --- |
| Reconstruct captured tuning as JSON | Exact 3,840-byte match |
| Reduce one LR gain by 0.5 dB | Only its four-byte gain word changes |
| Reparse original JSON | Exact original block/hash restored |
| Use `f` instead of `fc` | Frequency silently falls back to 300 Hz |

Use **`fc`**, explicit EQ tuples and all three captured gain corrections. This preservation recipe covers the captured snapshot only; it does not serialize arbitrary calibration or another device’s tuning. The parser starts from compiled defaults, so partial JSON can reset unrelated crossovers, limiters, delays and calibration.

The watched-file route `/mnt/ubi_boot/temp.json` applies a full block while forcing **internal DSP mode 5**, rebuilding filters and clearing histories. The read-only live capture used **mode 0 at 44.1 kHz**. Identical parameter bytes therefore do not guarantee identical processing. Returning to mode 0 can reload a preset and discard changes. Internal DSP mode is separate from user-facing Smart/Standard Mode.

The actual Go-generated recipe passed nine native-parser cases byte for byte; independent parser checks cover ten cases plus fourteen guards. Hardware native reset 1 → 0 restored all 3,840 original bytes, with user controls and console fields restored. A −6 dB cut at 125 Hz was audibly confirmed over Bluetooth. The earlier isolated audio probe faulted before producing valid frames and supplies no acoustic proof. HDMI/eARC, Cast and other channel layouts still need validation. Keep raw tuning/vendor files private.

Expose five bounded Front LR cuts with named relative presets and manual tuning, not raw native setters. Preserve each bar’s validated snapshot; reject unknown deviations. Application enters internal mode 5, with no claim of transparent stock routing. Save only owned recovery files, use a temporary recipe behind the watched symlink, and remove the link before native reset. A missing/dangling recipe leaves the native callback unchanged in the isolated test. Reset before web power/source/sound changes; monitor native route drift. Restore original mute state even when the initial mute attempt fails. Wait for native key release and post-ACK settling before continuing; the live API now preserves both initially muted and unmuted states across preset application/switching/reset. The API apply/reset test also confirmed the complete original parameter block and normal controls were restored. Restart clears the custom recipe; do not promise persistent EQ.

A narrower Cast path loads `libcast_equalizer_1.0.so`; status reports low/high shelves at 150/4,500 Hz, initially 0 dB. The [Google Home local API specification](https://github.com/rithvikvibhu/GHLocalApi/blob/master/GoogleHome.openapi3.json) documents a setter, but support/effect on this bar has not been established. It does not establish TV/eARC EQ. [JBL Soundbar CLI](https://github.com/k1rnt/jbl-soundbar-cli) and [JBL_Soundbar](https://github.com/MrBearPresident/JBL_Soundbar) target newer protocols; the bounded public search found no demonstrated custom-EQ implementation for Bar 9.1.

Preserve `analysis/eq-native/phase2/`, `offline-roundtrip/` and `phase2-state/` for native identities, REA exports, isolated parser helpers, exact diffs and read-only instance captures. Keep `implementation-offline/`, `apply-restore/` and `device-implementation/` alongside those results for actual recipe, reset and API/mute evidence.

## Verified-build field map

Use these decimal process addresses **only** with the exact `bdpprog` identity in [compatibility](../README.md#compatibility). Treat implementation source as authoritative for widths, guards and restoration order.

| Address | Bytes | Meaning |
| --- | --- | --- |
| 12962676 | 4 | System state; awake requires 0 |
| 13378532 | 4 | Standard 0 / Smart 3 |
| 14191893 | 1 | Power predicate requires 1 |
| 13822200 | 4 | Input adapter type |
| 13822224 | 1 | Key-down state |
| 14200904 | 4 | Combination state |
| 14200898 | 1 | Repeat/long-press indication |
| 15427780 | 1 | Temporary console input setting |
| 15427784 | 4 | Temporary console gate |

The Standard sequence uses `mw.iom.se`, mute `0x6f002` and volume-up `0x6f000`. Require mode readback, released input and restored console fields; CLI acceptance alone is insufficient. Preserve the exact controller under `smart-mode/JBLInstall/payload/`.

## Startup and SSH constraints

Close inherited service sockets before starting resident workers. Earlier prolonged LOAD coincided with a shell child retaining an accepted native IPC socket. A modeled AF_UNIX exchange under the extracted ARM shell blocked until that socket closed. Client/daemon transfer sizes differed (6168/4220 bytes); without the exact live client-library hash, the model does not independently prove the whole live causal chain.

Preserve revision 5’s disappearing-descriptor handling. Enumerating `/proc/$$/fd/*` can include the shell’s temporary directory descriptor; it may disappear before `readlink`. Skip already-absent entries, fail if an existing descriptor cannot be inspected, and preserve regular/script descriptors. Revision 4’s early exit reproduced under the ARM shell; revision 5 subsequently booted and selected Standard on hardware.

For SSH, retain:

- Vendor glibc 2.18 loader/libraries; a library-path change alone did not resolve the default glibc 2.27 mismatch.
- Private mount namespace for account/NSS lookup and loader compatibility, including host-key generation.
- Session wrapper covering commands, interactive shells and SFTP.
- Background startup, socket cleanup, bounded Wi-Fi wait, preserved Smart Mode hook and recognized-hook-only rollback.

USB-free root login, terminal and SFTP were confirmed on 20 October 2026 with normal controls and Wi-Fi restored. The portable installer still needs an independent clean-install test. The recorded device uses ARM32 Linux 4.9.134, about 448 MiB RAM and 25 MiB free writable storage; no normal APK package manager/framework was found. Keep chronological `private/local-ssh/README.txt` and captures private.

## Native sound, source and Bluetooth fields

Use config **365** for source labels: 0 TV/ARC, 1 Optical, 2 Bluetooth, 3 HDMI, 4 AUX, 5 USB, 6 Chromecast, 7 AirPlay. Config 51 is a separate raw setting; the observed Bluetooth value 10 invalidated the earlier value-9 label. The validated Bluetooth predicate is at `0x61af14`, used by the pairing worker `0x73e030`.

| Control | Short key | Config | Raw values |
| --- | --- | --- | --- |
| Bass | `0x1f030` | 238 | 0–4: Levels 1–5 |
| Atmos height | `0x1f02f` | 201 | 0–2: Low/Mid/High |
| Rear | `0x1f031` | 202 | 0–3: Low/Mid/High/Muted |

Rear adjustment requires both dock bytes `0xe435ec`/`0xe435ed` to be zero. Require the predicted next value and all existing awake/idle/console checks. Full bass/Atmos cycles and Night Mode Off → On → Off were hardware-confirmed with initial settings restored; docked rear changes were rejected. Undocked rear adjustment remains untested. Night Mode uses mute-hold/volume-down. Generic EQ/IIR strings do not establish a usable signal path.

Bluetooth GAP descriptor `0xc6d250` points to static context `0xc6d2c0`. Its uint32 at +8 maps 0 both connectable/discoverable, 1 connectable only, 2 discoverable only, 3 neither. Pairing confirmation requires both. GAP info pointer `0xc6d2cc` contains the bounded UTF-8/NUL local name at +0x28. Info+0xc8 supplies the peer address used internally to match `Name` in `/data/misc/bluedroid/bt_config.conf`; return only that name, recheck connection/address and never expose pairing keys or raw config. Cache size is bounded to 128 KiB; names may lag renames.

A2DP descriptor `0xcb6450` points to `0xe3757c`; uint32 +4 maps 0 disconnected, 1 connected, 2 suspended, 3 streaming. Check exact pointers and executable identity; leave unknown states unavailable. These describe stream activity, not the sending app’s playback.

Pairing handler `0x6432f0` accepts Bluetooth key `0x6f027` at repeat 25. The 400 ms initial/200 ms repeat intervals require a 6000 ms hold and nine-second completion window. Require released keys and GAP readback; the transient worker flag is insufficient. Pairing and subsequent phone-name/Streaming → Suspended readback were confirmed on hardware. Evidence stays in `analysis/bluetooth-ui/` and `analysis/sound-controls/`.

## Playback and metadata limits

The Bluetooth IOM focus handler discards ordinary media key events. Use the project’s callback-free native MediaTek AVRCP RPC helper: one key-down/key-up pair, then close. Audible Pause was confirmed; Play restored playback, while A2DP remained streaming. The integrated API accepted Pause/Play in 0.124/0.225 seconds without changing volume, mute, power, source or sound settings. Next/Previous/Stop still require audible validation. Keep callback subscribers, pairing state and service startup intact; link only against owner-supplied matching vendor libraries.

Normal Cast media controls use the native C4A handler with session/capability guards; mirroring is excluded. Receiver state at `0xe30710` maps 2 Playing/3 Paused. AirPlay `0xe3169c` maps 1 active session/0 stopped, without pause distinction. YouTube through Chrome Audio Mirroring (`85CDB22F`) retains receiver/native PLAYING when sender playback pauses; label transport activity without claiming sender-state detection.

The bounded read-only Cast V2 client queries fixed loopback port 8009 at most every two seconds while requested. Only status/connection/heartbeat messages are sent. Expose optional receiver metadata/progress and shelf gains; omit transport IDs/arbitrary payloads. Normal receiver media-state decoding has fixtures, but the live test validates mirroring only.

The connected Bluetooth middleware cache had no title/artist during the streaming test. Unlike that disconnect-cleared cache, older `bdpprog` metadata can retain the previous peer’s track. AirPlay legacy metadata pointers were empty, without a verified newer-SDK bridge. Keep the experimental reader/tests under `analysis/track-ui/experimental-reader/`; do not show stale track information or publish listening history.

## Network and auxiliary diagnostics

Wi-Fi uses fixed STATUS/SIGNAL_POLL requests over a connected UNIX datagram socket; no credential/configuration commands are exposed. The native hotspot defaults to `192.168.255.249/29` with DNS disabled. Require an active `ap0` address, matching client subnet and local HTTP destination. Preserve native setup/DHCP services. Automatic fallback after configured Wi-Fi loss remains unproven; the bounded `net_mgr` trace established start/retry paths only.

Register `jbl.local` with the existing Bonjour daemon using DNS-SD Any scope. Interface-specific registration stopped answering fresh queries despite live IPC; Any continued beyond the 120-second TTL. Still validate `wlan0` against the configured LAN and withdraw the record on address change/shutdown. Keep IP fallback for conflicts or blocked mDNS.

Keep the system monitor auxiliary to sound controls. Two-second cached samples provide aggregate CPU, cache-aware RAM, `/var` storage, Wi-Fi byte rates, load/uptime and process count. Display temperature only when reported; no sensor was exposed on the tested bar. Retain at most 60 unique browser-local samples, break unavailable data, and clear history across disconnect/reset/long gaps. No database or extra state polling is needed. Load average can include native tasks waiting on I/O; do not present it as CPU utilization or a standalone fault alarm.

Use the current fixture suite rather than historical intermediate check totals: **141 host checks, 158 browser checks**, plus Go race tests and vet. Screenshots contain example values. Original Smart Mode ZIP bytes remain unchanged. CLI/discovery/capability ideas came from [the reviewed k1rnt revision](https://github.com/k1rnt/jbl-soundbar-cli/tree/7d10bdb1ebe0c5a77c9a4c14ebe4580bd3735309); its newer Linkplay transport and one-shot Smart Mode setter do not supply this bar’s native implementation or wake watcher.

## Direct Smart Mode control

The pinned native audio table exposes `app.audio.eq_set VALUE`. Its handler reaches the same HPP parameter `0x840001` setter used by Smart Mode’s audio task: Standard maps to 0, Smart to 1 or 2 according to source. This establishes a direct DSP candidate, not a complete replacement. No live validation of this CLI entry was performed in this pass.

The normal full toggle also temporarily mutes, resets playback, updates the display/runtime word and saved config200, then queues audio command0x12. That queue uses in-process handles; no Bluetooth-style external bridge or dedicated single SoundMode key was found in the inspected paths. Do not replace the action with a mode-flag write or raw EEPROM edit. Go wake automation now shares polling, locks and native readback with the web controls. It retains key emulation and its native hold interval; a complete direct Smart Mode action remains unvalidated. Owned legacy-helper takeover and restoration preserve the existing startup hook and manual disable flags.

## Further validation

Prioritize repeated application/reset and per-source audio behavior. Revision 6 measured 18.26 seconds from observed awake to Standard versus a 34.64-second revision 5 control; one-second polling alone measured 27.00 seconds. Preserve the [revision 6 device record](validation/revision6-device.json), including the unexplained pre-action power-off/reboot. Optional Go automation subsequently measured **7.77–7.82 seconds** across three controlled wakes, with keys/console restored. Stop/uninstall/reinstall handed ownership between Go and the verified shell helper; see the [Go device record](validation/go-automation-device.json). Keep CEC/cold-boot qualification, sampling overhead, the transient installation long-press failure and additional firmware identities separate. Never substitute a passing host fixture, model suggestion or parser success for a hardware result.
