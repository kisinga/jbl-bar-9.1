# Development

Keep the foundation small: readable scripts, reproducible packages and working recovery. Smart Mode, SSH and the browser remote are separate features. Preserve the working behavior and firmware identity guards. Do not add support for a build by merely changing its hash: its fields, event ABI and restoration behavior need validation.

## Build and test

On Linux, install Python 3.10+ and BusyBox (`/usr/bin/busybox`). On Debian/Ubuntu the packages are `python3` and `busybox`. These are developer requirements; users only need the ZIP.

```sh
python3 tools/build_release.py
python3 tools/test.py
```

Archives and `SHA256SUMS` appear in `dist/`. Rebuilding the same inputs produces identical ZIP bytes. The build verifies payload hashes, embedded expected hashes, exact tested device files and shell syntax. It refuses stale or modified device code against the pinned baseline. No firmware image or external Python package is needed.

The 141 host checks include the existing 121 core fixtures and 20 CLI checks. They use temporary fake process/CLI files or an isolated local HTTP server, never a soundbar or real USB. Synthetic identity replacement stays confined to fixtures. Results go in ignored `build/test-results/`. The 102 checks recorded for the original Smart Mode release remain its historical result.

Optional descriptor validation against a legally obtained extracted ARM BusyBox uses `JBL_ARM_BUSYBOX=/absolute/path/to/busybox python3 tests/check_descriptors.py`, with `qemu-arm` and registered ARM binfmt available for direct applet execution. That binary is not bundled.

## Layout

The three tools have separate source folders and share these root instructions:

- [`webremote/`](webremote/): Go server and embedded browser interface.
- [`smart-mode/`](smart-mode/): exact installable Smart Mode source, including the CRLF-preserved launcher/hook.
- [`ssh/`](ssh/): portable SSH setup templates; generated credentials and device keys stay private.

`tools/` contains builders, the optional Python CLI and the test runner; `tests/` contains fixtures and pinned device-file hashes. `README.md` covers use and recovery, `COMMUNITY.md` collects discussion links, and `USB-README.txt` / `ROLLBACK.txt` ship in the Smart Mode ZIPs. [`research/README.md`](research/README.md) indexes the public research archive; local captures and vendor artifacts remain ignored.

`main` and tag `v0.1.0` preserve the revision 5 I first confirmed on my bar. Experimental latency work belongs on a separate branch and must not replace that tested download until hardware validation. A new payload requires updated checksums, installer expectations, ownership/upgrade handling, tests and a fresh hardware result. The baseline verifier must not be casually bypassed to publish an untested change.

Report results with the issue template. Keep proprietary images, full device logs, credentials, personal paths and tool caches out of commits and releases.

## SSH package

The hardware result on 20 October 2026 covers my device: USB-free startup, root password login, an interactive terminal and SFTP. A clean generic installation must be tested separately before I call the portable package confirmed. I reuse existing device binaries; I do not distribute the vendor loader, libraries or firmware.

The builder needs Python 3 on Linux. It asks for a password, writes its SHA-512 crypt hash into the generated package and limits login to the computer IPv4 address supplied at build time. I keep generated packages private: even a password hash or derived Wi-Fi PSK is sensitive. I never commit an SSID, password, PSK, private host key, personal address or full device capture.

```sh
python3 tools/build_ssh.py --allowed-pc <computer-ipv4-address>
```

Enter a new root password twice when prompted (at least eight characters). By default I reuse Wi-Fi already configured on the bar and enable automatic SSH startup. Add `--wifi` to supply an SSID and WPA2 passphrase interactively, or `--usb-only` to require USB insertion after each reboot. `--interface wlan0` is the default.

The output is **`dist/jbl-bar9-ssh-private.zip`** plus **`dist/jbl-bar9-ssh-rollback-private.zip`**. These are personal installation artifacts, never public release assets. Extract the setup ZIP's contents directly to a FAT32 USB root, with `AutoScript/AutoScript` and `JBLLocalSSH/` at the top level. Keep the matching rollback ZIP on the computer. Use one AutoScript package at a time.

With the bar fully on and playing normally, insert USB, leave controls alone for three minutes, then return it to the computer and inspect `JBLLocalSSH/STATUS.txt` and `setup.log`. Success is `SSH_READY`, or `ALREADY_RUNNING_PASSWORD_UNCHANGED` when an existing daemon is preserved and automatic startup is updated. Reinsertion does not rotate an already-running installation's password.

`JBLLocalSSH/connection.txt` contains connection details and the host-key fingerprint; `host_ed25519.pub` is the public host key. Do not infer success from elapsed time. Verify that fingerprint before accepting the host key and logging in with `ssh -p 2222 root@<bar-ip-address>`.

For the default automatic-start package, test a restart with **USB removed**. Confirm SSH returns, normal LOAD completes and power/volume/standby still work. A listener alone is insufficient: verify a password login, a normal shell command, an interactive terminal and SFTP. Do not replace or overwrite unrelated AutoScript hooks.

To disable automatic SSH, use `/bin/sh /var/JBLLocalSSH/rollback-boot.sh` over SSH, or extract the matching rollback ZIP to a clean FAT32 USB root and run it on the bar. This restores the pre-SSH startup hook when it still matches the expected version; it keeps the active session and stored files. It preserves unrecognized later hook changes rather than overwriting them. Keep the USB recovery option available before the reboot test.

The boot hook must close inherited service sockets, run in the background, wait a bounded time for Wi-Fi and preserve the existing Smart Mode hook. Rollback disables automatic SSH while preserving an active session and unrelated later changes. The native daemon depends on a compatible private loader/NSS/account view; passing a host shell-syntax check alone does not establish device compatibility.

## Faster wake candidate — revision 6

**Working branch `work/faster-wake`; hardware untested.** The public v0.1.0 download remains the confirmed revision 5. Do not describe the faster candidate as device-validated.

The confirmed trace took 34.55 seconds from first awake observation to `STANDARD_THIS_WAKE`. Around ten seconds were spent accumulating awake samples at five-second intervals; `once.sh` then included a fixed ten-second delay and repeated readiness checks before invoking the controller.

| Setting | Confirmed revision 5 | Candidate revision 6 |
| --- | --- | --- |
| Watcher poll interval | 5 seconds | 1 second |
| Required identical awake samples | 3 | 3 |
| Bounded runner's initial settling delay | 10 seconds | 1 second |
| Runner readiness sample interval/count | 2 seconds / 3 samples | Unchanged |
| Mute hold, key-release/completion checks | Confirmed sequence | Byte-for-byte unchanged controller/helper |
| Standby deadline | None | None |

The candidate removes approximately 17 seconds of scheduled waiting, suggesting **about 18 seconds** from first awake detection on a similar run. That is an estimate, not a measurement. Initial detection also polls sooner. Read-only state sampling now occurs more frequently; hardware CPU/load and wake behavior must be observed. The change does not shorten mute duration, skip mode checks or weaken restoration.

Revision 6 has distinct ownership and checksums. Its dispatcher only upgrades the exact revision 5 package, backing it up first. It retains refusal for unknown payloads, conflicting hooks or unsupported executables. Use its **matching uninstall ZIP**; the original v0.1.0 uninstaller does not recognize revision 6 ownership.

### Build and hardware test

On this branch run `python3 tools/build_release.py` and `python3 tools/test.py`. The generated candidate archives are:

- `dist/jbl-bar9-standard-mode-v0.2.0-dev.1-install.zip`
- `dist/jbl-bar9-standard-mode-v0.2.0-dev.1-uninstall.zip`

These local archives are deliberately separate from the public confirmed release. Use the same install/test procedure, recording actual seconds after power-on. Confirm normal LOAD, delayed power-on, Smart-to-Standard readback, key release, console restoration and repeated standby/wake. Test standby of at least 20 seconds first. Return the USB logs. No hardware result for revision 6 exists yet.

The pinned revision 6 manifest is an integrity record for this candidate, not a claim of hardware qualification. `tests/fixtures/revision5-sha256.json` retains the confirmed baseline for comparison.

## Browser remote

I need working SSH, Python 3 and Go 1.22+ on the build computer (tested with
Go 1.25.5). The bar runs the resulting static ARM Go binary; no Go runtime or
third-party Go modules are needed on it. The normal Go-only build leaves
Bluetooth playback controls disabled; the optional helper below enables them.

```sh
python3 tools/build_web.py --allowed-cidr <trusted-lan-subnet>
(cd webremote && go test ./...)
```

Use the actual trusted IPv4 subnet, for example `192.168.1.0/24`. The output
directory is `dist/webremote-private/`; its config contains the listener and
local subnet. The browser remote requires no login. SSH keeps its own password.
The builder strips local source paths and VCS metadata. Identical sources
and Go toolchain produce the same server binary; owner settings differ.

For Bluetooth playback, I build the small fixed-command C helper with an ARM
hard-float GNU/Linux GCC toolchain and my own extracted supported firmware:

```sh
python3 tools/build_bt.py --cc /path/arm-none-linux-gnueabihf-gcc \
  --rootfs /path/extracted/rootfs --output build/bt-remote
python3 tools/build_web.py --allowed-cidr <trusted-lan-subnet> \
  --bluetooth-helper build/bt-remote
```

Only my helper is embedded. Vendor libraries are used locally for linking and
reused from the bar at runtime; I do not distribute them. The builder checks
pinned libraries; the server also checks the current input and connected peer.
The helper exposes five fixed playback commands and registers no Bluetooth
event subscriptions.
I confirmed audible Pause and subsequent playback resumption; Next, Previous
and Stop remain unconfirmed. Each button reports a request, not proof the
source app changed state. Separate Play/Pause buttons avoid guessing from an
A2DP stream that can remain active while the source app is paused. Normal Cast
controls require a nonmirrored session and advertised capabilities; their
hardware behavior remains unverified.

Copy that directory over SFTP to `/var/JBLRemote` (first check that no unrelated
installation owns that path), then run its installer:

```text
sftp -P 2222 root@<bar-ip-address>
put -r dist/webremote-private /var/JBLRemote
bye
```

```sh
ssh -p 2222 root@<bar-ip-address> '/bin/sh /var/JBLRemote/install.sh'
```

Open `http://jbl.local` (`:8080` remains available); there is no web login.
I register the alias through the existing Bonjour daemon, which handles name
probing and conflicts. Local multicast and mDNS support are required. If the
name conflicts or does not resolve, I use `http://<bar-ip-address>`; I do not
silently rename the alias. Registration follows Wi-Fi address changes and is
withdrawn when the remote stops. Anyone on the configured trusted subnet can
use these controls. Only this hostname and accepted local addresses pass the
Host checks; the HTTP destination must also match an allowed interface.
The server accepts requests from the configured subnet, plus clients of the
active native `ap0` hotspot when their HTTP destination matches that interface. Fixed control
names, JSON-only POSTs, CSRF tokens and same-origin checks protect mutations;
it does not accept arbitrary commands or serve filesystem paths.

The installer preserves the exact existing startup hook and adds one
background command. Remove features in reverse installation order: web remote,
SSH, then Smart Mode. To remove only the remote, run:

```sh
ssh -p 2222 root@<bar-ip-address> '/bin/sh /var/JBLRemote/uninstall.sh'
```

It restores its recorded hook if still recognized, stops its own process and
retains files for inspection. Later unknown hook edits are preserved. Keep
SSH usable while experimenting. Volume/mute and readback are hardware-tested;
Night Mode Off → On → Off and bass/Atmos cycles are hardware-tested too.
Rear changes need an undocked hardware test; power, input transitions and manual Standard remain experimental.

The UI test uses Node and Playwright on the development computer:

```sh
# First time only:
npm install --no-save --package-lock=false playwright
npx playwright install chromium
node tests/webremote-ui.mjs
```

`PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs` can reuse an
existing installation. Tests serve the real assets with delayed/failed mock
responses, cover queued/cancelled controls, holds, keyboard input, reconnects,
stale replies, standby, Bluetooth/Wi-Fi/streaming feedback and 320/390/1024px layouts. Go tests
cover LAN/hotspot/Host/CSRF boundaries, event streams, native Wi-Fi datagrams,
state confirmation, peer-name filtering, sound decoding, native Bonjour
registration and bounded read-only Cast framing/status queries.
UI fixtures do not substitute for hardware pairing or audible tests.

Live updates use Go’s standard HTTP server and the browser’s `EventSource`.
A single reader samples native state once per second only while subscribed;
unchanged state is suppressed and slow clients keep only the newest update.
Wi-Fi is sampled separately every five seconds; Cast metadata uses a bounded
loopback status query at most every two seconds while requested. Monotonic revisions prevent
an older stream/HTTP reply from undoing a newer observation. No third-party
Go module or database is needed. Both HTTP listeners share the same controls
and access checks; an occupied port 80 leaves 8080 usable.

On 20 October 2026 I confirmed live AirPlay and Wi-Fi readback, both HTTP
ports, a 22-second single browser event connection with zero state-poll requests,
Night Mode Off → On → Off and complete bass/Atmos cycles restored to their
original levels. The connected Bluetooth name and Playing → Paused updates were subsequently confirmed on my bar.
The observed YouTube session uses Chrome Audio Mirroring (`85CDB22F`): its receiver
keeps reporting PLAYING when playback pauses at the sender. I label this Audio
mirroring and do not claim sender pause detection. Normal receiver media-state
decoding has fixtures but no live validation. The deployed UI correctly identified that live mirroring session, with one
event connection and no automatic state requests over 22 seconds. Native hotspot
activation/access still needs an outage test. No Wi-Fi loss, reboot or factory reset was induced.

### Optional CLI

I use the standard-library Python 3 CLI with the same deployed Go API:

```sh
python3 tools/jbl.py status
python3 tools/jbl.py volume up
python3 tools/jbl.py night on
python3 tools/jbl.py smart off
python3 tools/jbl.py --host <private-ipv4[:port]> status --json
python3 tests/check_cli.py
```

`volume down`, `mute`, `power`, `source tv|hdmi|bluetooth` and available
`play`, `pause`, `next`, `previous`, `stop` commands are also supported.
`--timeout` accepts 1–30 seconds; JSON omits CSRF tokens. Night on/off and
Smart Mode off read the current state first and skip an unnecessary toggle.
Unknown state stays unknown, unsupported playback is refused, and failures
are never retried automatically. Manual Smart Mode off does not replace the
persistent wake watcher.

I took CLI/control ideas from [k1rnt/JBL Soundbar CLI at the reviewed commit](https://github.com/k1rnt/jbl-soundbar-cli/tree/7d10bdb1ebe0c5a77c9a4c14ebe4580bd3735309).
Its newer Linkplay/JBL One transport is separate; this CLI calls only my
fixed Bar 9.1 Go endpoints. [EQ exploration](research/README.md#eq-exploration--20-october-2026)
remains read-only research, without unverified sliders.
