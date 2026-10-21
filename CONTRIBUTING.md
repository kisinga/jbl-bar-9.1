# Development

Keep scripts readable, packages reproducible and recovery working. Preserve firmware identities, native guards and existing startup hooks. Supporting another build requires field, event ABI and restoration validation; changing a hash is insufficient.

## Build and test

Install Python 3.10+ and BusyBox at `/usr/bin/busybox` on Linux (`python3 busybox` on Debian/Ubuntu). These are developer requirements; Smart Mode users only need the release ZIPs.

```sh
python3 tools/build_release.py
python3 tools/test.py
```

Find reproducible ZIPs and `SHA256SUMS` in `dist/`. The builder checks pinned device files, payload/embedded hashes and shell syntax; modified or stale payloads fail the baseline check. No firmware image or external Python package is required.

The current **141 host checks** comprise 121 core fixtures and 20 CLI checks. They use temporary fake process/CLI files and an isolated local HTTP server, without a soundbar or USB. Results stay in `build/test-results/`. The original Smart Mode release’s historical result is 102 checks.

For optional extracted ARM BusyBox descriptor checks, install `qemu-arm` and registered ARM binfmt, then run:

```sh
JBL_ARM_BUSYBOX=/absolute/path/to/busybox python3 tests/check_descriptors.py
```

Supply a legally obtained binary; vendor binaries are not bundled.

## Layout

| Path | Contents |
| --- | --- |
| `webremote/` | Go server and embedded browser interface |
| `smart-mode/` | Exact Smart Mode installable source, including CRLF launcher/hook |
| `ssh/` | Portable SSH setup templates |
| `tools/`, `tests/` | Builders, CLI, fixtures and integrity baselines |
| `research/README.md` | Findings and private archive index |

Keep generated credentials/packages, firmware, full captures, personal configuration and tool caches out of commits/releases. Use the issue template for hardware results. Root instructions cover use, recovery and community links; `USB-README.txt` and `ROLLBACK.txt` ship with Smart Mode.

Preserve the confirmed revision 5 download at `v0.1.0`. New payloads require checksums, ownership/upgrade handling, fixtures and fresh hardware evidence. Keep experimental latency changes on their branch until validated.

## SSH package

Hardware evidence covers USB-free startup, root password login, an interactive terminal and SFTP on the development bar. Independently validate a clean portable installation before calling it confirmed. Reuse native vendor binaries; do not distribute their loader/libraries.

Build on Linux with Python 3:

```sh
python3 tools/build_ssh.py --allowed-pc <computer-ipv4-address>
```

Enter a new root password twice, at least eight characters. The builder embeds its SHA-512 crypt hash and limits login to the supplied computer address. Keep generated packages private, including password hashes and derived Wi-Fi PSKs.

Default behavior reuses configured Wi-Fi and enables SSH autostart. Add `--wifi` for interactive SSID/WPA2 credentials or `--usb-only` to require USB insertion after each reboot. The default interface is `wlan0`.

1. Keep `dist/jbl-bar9-ssh-rollback-private.zip` on the computer.
2. Extract `dist/jbl-bar9-ssh-private.zip` directly to FAT32 USB, with `AutoScript/AutoScript` and `JBLLocalSSH/` at its root. Use one AutoScript package at a time.
3. Insert into the fully on, normally playing bar. Leave controls alone for three minutes, then return USB to the computer.
4. Require `JBLLocalSSH/STATUS.txt` to report `SSH_READY` or `ALREADY_RUNNING_PASSWORD_UNCHANGED`; inspect `setup.log`. Reinsertion preserves an already-running daemon’s password.
5. Verify the fingerprint in `connection.txt` against `host_ed25519.pub` before accepting the SSH host key.
6. Test `ssh -p 2222 root@<bar-ip-address>`, a normal command, an interactive terminal and SFTP. A listener or `sshd -t` alone is insufficient.
7. With recovery available, restart **without USB**. Verify SSH returns and LOAD, power, volume and standby remain normal.

Remove browser remote first. To disable SSH autostart, run `/bin/sh /var/JBLLocalSSH/rollback-boot.sh` over SSH, or use the matching rollback ZIP on clean FAT32 USB. Rollback restores the recorded pre-SSH hook only when recognized; active sessions, stored files and unknown later hook edits are preserved.

Preserve inherited-socket cleanup, background startup, bounded Wi-Fi wait and the Smart Mode hook. Native OpenSSH requires the compatible private loader/NSS/account view, including host-key generation and session commands.

## Browser remote

Requirements: working root SSH, Python 3 and Go 1.22+ on the computer (tested Go 1.25.5). The bar needs no Go runtime, external Go modules, database or ongoing computer service.

```sh
python3 tools/build_web.py --allowed-cidr <trusted-lan-subnet>
(cd webremote && go test -race ./... && go vet ./...)
```

Use the actual trusted IPv4 subnet, such as `192.168.1.0/24`. Output goes to `dist/webremote-private/`. The builder strips local source paths and VCS metadata; identical inputs/toolchains produce identical binaries. Configuration varies by owner. The normal Go-only build disables Bluetooth transport controls and Front EQ.

### Optional Go wake automation

Add `--auto-standard` to the build command to let the Go service select Standard Mode after wake. Keep the Bluetooth/EQ helper flags if those features are needed:

```sh
python3 tools/build_web.py --allowed-cidr <trusted-lan-subnet> --auto-standard \
  --bluetooth-helper build/bt-remote --eq-parser build/eq-parser
```

Go shares native controls, locking and readback with the web interface. An OS-held ownership lock excludes duplicate Go automation and recovery commands. It samples readiness every second, requires three stable awake samples, and acts once per wake. Standby has no deadline. Manual Smart selection is respected until the next standby or service/firmware restart. A failed gesture stops automatic attempts for that wake and leaves the reason in the page’s device details and `remote.log`.

An installed revision 5/6 shell helper is verified, paused with an owned `DISABLED` marker and retained intact. Unknown/modified helpers and existing manual disable flags are preserved. No legacy helper is required if SSH and the web service are already available. The original button sequence is still used; this is not a direct native Smart Mode setter.

Set `auto_standard` to `false` in `/var/JBLRemote/config.json` and restart the service to return automation to the shell helper. Normal service shutdown and remote uninstall also restore only the verified helper paused by Go. For recovery after an interrupted removal, run `/var/JBLRemote/remote -restore-smart-helper` with the service stopped. Do not delete an unknown disable flag. Removing the USB Smart Mode package alone does not disable Go’s separately enabled automation.

Volume/mute are checked against the state immediately before the gesture. The firmware can clear mute itself on power-on. Keep CEC, other input layouts and mains cold boots separate from the measured standby/wake tests ([device record](research/validation/go-automation-device.json)).

### Optional Bluetooth helper

Use an ARM hard-float GNU/Linux GCC toolchain and an extracted copy of the supported firmware:

```sh
python3 tools/build_bt.py --cc /path/arm-none-linux-gnueabihf-gcc \
  --rootfs /path/extracted/rootfs --output build/bt-remote
python3 tools/build_web.py --allowed-cidr <trusted-lan-subnet> \
  --bluetooth-helper build/bt-remote
```

Only the project’s fixed-command helper is embedded. Matching vendor libraries are checked locally and reused from the bar; do not redistribute them. The helper registers no event subscribers. The server checks firmware, current input and connected peer before sending playback requests.

Audible Pause and playback resumption are confirmed; Next/Previous/Stop remain unconfirmed. One Play/Pause button follows successful Bluetooth requests because A2DP activity cannot establish app playback. Native stream, source or peer changes reset intent; errors never flip it. Normal Cast controls require a nonmirrored session and advertised capabilities, with hardware validation still outstanding.

### Optional Front EQ helper

Use the same ARM hard-float GNU/Linux GCC toolchain and supported extracted firmware as the Bluetooth helper:

```sh
python3 tools/build_eq.py --cc /path/arm-none-linux-gnueabihf-gcc \
  --rootfs /path/extracted/rootfs --output build/eq-parser
python3 tools/build_web.py --allowed-cidr <trusted-lan-subnet> \
  --bluetooth-helper build/bt-remote --eq-parser build/eq-parser
```

Omit `--bluetooth-helper` if playback controls are not needed. Only project helper code is embedded; vendor libraries stay on the bar. Both build-time and runtime hash checks reject unsupported libraries.

Turn on Standard Mode and start audio. Select a preset or adjust the five sliders, then **Apply EQ**; **Reset** returns to original tuning. Presets are starting points rather than measured corrections: Original, Less bass, Clear dialogue, Softer treble, Warm, Bright and Vocal focus. They contain relative cuts only, not another device’s calibration. Front LR adjustment covers 125/500/2,000 Hz bells and overlapping 4,000/8,000 Hz high shelves, −6 to 0 dB in 0.5 dB steps. It does not tune the subwoofer, rears or height channels.

Application uses native custom DSP mode 5. It is not proven equivalent to stock processing plus EQ. Each request validates a complete 3,840-byte tuning reconstruction with the native parser before applying it; unknown calibration differences fail closed. Reset uses the native 1 → 0 preset transition and checks exact original bytes. Keep the bar’s normal mute state across both operations.

EQ is temporary: restart returns to original tuning, and web power/source/sound changes reset it first. The monitor also attempts a reset when the native route changes. Other native controls, standby/CEC and HDMI/eARC/Cast layouts still need qualification; Bluetooth has audible confirmation. Recovery files belong in `/var/JBLRemote`; preserve them after an error and try **Reset**. Do not edit another program’s `/mnt/ubi_boot/temp.json` or remove recovery files to force an unsupported route.

### Install and remove

Check that no unrelated installation owns `/var/JBLRemote`. Copy the output directory over SFTP, then install:

```text
sftp -P 2222 root@<bar-ip-address>
put -r dist/webremote-private /var/JBLRemote
bye
```

```sh
ssh -p 2222 root@<bar-ip-address> '/bin/sh /var/JBLRemote/install.sh'
```

Open `http://jbl.local`, or the bar’s local IP if mDNS is blocked/conflicting. Port `8080` remains available. Existing Bonjour handles probing/conflicts; registration follows Wi-Fi address changes and withdraws on exit. No silent alias renaming occurs.

Anyone on the configured trusted subnet can use the remote without a web login. Requests must match accepted Host/local interface addresses. Active native `ap0` hotspot clients are also allowed only for that interface’s HTTP destination. Fixed commands, JSON-only POSTs, same-origin checks and CSRF tokens guard mutations; no arbitrary command/filesystem endpoint is exposed.

Installation preserves the exact startup hook and adds one background command. Remove **browser remote → SSH → Smart Mode**. To remove only the remote:

```sh
ssh -p 2222 root@<bar-ip-address> '/bin/sh /var/JBLRemote/uninstall.sh'
```

Removal stops its own process and restores the recorded hook only when recognized; files and unknown later hook changes remain. Keep SSH usable during development.

### State and interface

Prioritize listening and sound controls. Use the optional Front EQ helper for supported native routes; [EQ findings](research/README.md#eq-exploration--20-october-2026) record parser, application and restoration evidence.

One native reader samples each second only while subscribed through `EventSource`; changed state is coalesced and slow clients retain only the newest update. Wi-Fi status is cached for five seconds, Cast status for two seconds while requested. The auxiliary system monitor caches two-second CPU/RAM/storage/network samples, with bounded two-minute browser history and no database. Missing sensors/readings remain unavailable. Monotonic revisions prevent older replies from overwriting newer observations. Both listeners share controls and access checks; an occupied port 80 leaves 8080 usable.

Run UI fixtures with Node and Playwright:

```sh
# First time only:
npm install --no-save --package-lock=false playwright
npx playwright install chromium
node tests/webremote-ui.mjs
```

Set `PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs` to reuse an installation. The current **164 browser checks** cover queues/cancellation, held and keyboard controls, failures/reconnects, stale replies, standby, Bluetooth/streaming, single Play/Pause, EQ presets/manual drafts/application gating, Go automation status/errors, system sample history/gaps/availability, reduced motion and 320/390/1024px layouts. Public screenshots use fixtures, never live personal data.

Direct controls use fixed native/protocol commands, fresh source/capability checks and readback. Bluetooth pairing uses `app.bt.pair`; qualified Bluetooth volume/mute uses Cast receiver volume. Cast media uses current receiver sessions; AirPlay uses the verified SDK command mapping. Uncertain requests are never retried through a second path. Keep IOM for unsupported volume routes and complete sound actions. **Do not use `app.amb.po`/`app.amb.sb`: the former crashed the native service during qualification.** See the [control qualification record](research/validation/direct-controls-device.json).

Go tests cover access boundaries, event streams, native Wi-Fi datagrams, readback, peer filtering, sound decoding, Bonjour, Cast framing/status, system parsing/cache behavior, native EQ route guards and full-block recipe preservation, wake state transitions and legacy takeover/restoration. Run race tests and vet after backend changes. Fixtures do not establish audible behavior.

Hardware confirmation includes volume/mute, pairing/name/native stream states, AirPlay/Wi-Fi readback, Night Mode Off → On → Off and bass/Atmos cycles restored to initial settings. Rear changes require an undocked test; product TV/ARC, HDMI and Bluetooth selection now have a controlled roundtrip, while broader power/source/manual Standard coverage remains experimental. Chrome Audio Mirroring (`85CDB22F`) keeps receiver PLAYING when sender playback pauses; label mirroring without claiming sender pause detection. Automatic hotspot activation/access needs a separate outage test.

### Optional CLI

Use Python’s standard library against the same deployed API:

```sh
python3 tools/jbl.py status
python3 tools/jbl.py volume up
python3 tools/jbl.py night on
python3 tools/jbl.py smart off
python3 tools/jbl.py --host <private-ipv4[:port]> status --json
python3 tests/check_cli.py
```

Also supported: `volume down`, `mute`, `power`, `source tv|hdmi|bluetooth`, and available `play`, `pause`, `next`, `previous`, `stop`. `--timeout` accepts 1–30 seconds. JSON excludes CSRF tokens. Night/Smart Mode commands check known state before changing it; unsupported playback is refused and failures are never retried automatically. Manual Smart Mode off preserves the persistent watcher.

[k1rnt/JBL Soundbar CLI](https://github.com/k1rnt/jbl-soundbar-cli/tree/7d10bdb1ebe0c5a77c9a4c14ebe4580bd3735309) supplied CLI/control inspiration. Its newer Linkplay/JBL One transport is separate; this CLI uses the fixed Bar 9.1 Go endpoints.

## Faster wake — revision 6

Build revision 6 from `main`; the former `work/faster-wake` branch is merged. Keep the original v0.1.0/revision 5 release and checksums as the recovery baseline.

| Setting | Revision 5 | Revision 6 |
| --- | --- | --- |
| Watcher polling | 5 seconds | 1 second |
| Identical awake samples | 3 | 3 |
| Runner initial delay | 10 seconds | 1 second |
| Readiness sampling | 2 seconds / 3 samples | Unchanged |
| Mute hold, release/completion guards | Original controller | Unchanged |
| Standby deadline | None | None |

Measured on the same owned bar on 20 October 2026, from first observed awake state to confirmed Standard Mode:

| Test | Seconds |
| --- | --- |
| Revision 5 control | 34.64 |
| One-second polling, original initial delay | 27.00 |
| Revision 6, 23-second standby | 18.26 |
| Revision 6, 9.6-second standby | 18.01 |
| Revision 6, 76.1-second standby | 17.16 |

Require released keys, exact console restoration and unchanged volume/mute/source/sound settings; a display message alone is insufficient. Raw logs remain private; [device validation](research/validation/revision6-device.json) records sanitized results. CEC wakes, mains cold boots and additional bars still need qualification.

One unexpected reboot occurred during an initial standby test before any Standard Mode sequence ran. The previous log ended in native power-off state `power=0/system=1`; subsequent revision 5 control and faster-wake tests completed normally. The cause remains unresolved; do not attribute it to the timing change or claim it fixed.

```sh
python3 tools/test.py
# ZIPs: dist/jbl-bar9-standard-mode-v0.2.0-dev.1-{install,uninstall}.zip
```

Use the matching revision 6 uninstall package, not the published v0.1.0 uninstaller. Exact revision 5 payloads/hooks may upgrade through the USB installer; unknown files and modified hooks remain protected. With SSH/web startup hooks already added, the USB upgrader intentionally refuses that modified hook. Preserve the entire helper and hook before a controlled SSH payload update; do not overwrite the combined startup hook with `hook.expected`.

The tested SSH update replaced only the owned Smart Mode payload, preserved the combined startup hook byte for byte, and retained the prior helper for rollback. Installation from USB and removal of the new version still need independent hardware tests. The v0.1.0 downloads are unchanged.
