# Development

On `work/faster-wake`, the builder pins an **untested revision 6 candidate** and emits `v0.2.0-dev.1` archives. The original revision 5 baseline remains in the fixtures and on `main`/`v0.1.0`. See [candidate notes below](#faster-wake-candidate--revision-6); descriptions of the release I tested below refer to revision 5.

Keep the foundation small: readable scripts, reproducible packages and working recovery. Smart Mode, SSH and the browser remote are separate features. Preserve the working behavior and firmware identity guards. Do not add support for a build by merely changing its hash: its fields, event ABI and restoration behavior need validation.

## Build and test

On Linux, install Python 3.10+ and BusyBox (`/usr/bin/busybox`). On Debian/Ubuntu the packages are `python3` and `busybox`. These are developer requirements; users only need the ZIP.

```sh
python3 tools/build_release.py
python3 tools/test.py
```

Archives and `SHA256SUMS` appear in `dist/`. Rebuilding the same inputs produces identical ZIP bytes. The build verifies payload hashes, embedded expected hashes, exact tested device files and shell syntax. It refuses stale or modified device code against the pinned baseline. No firmware image or external Python package is needed.

The tests execute adapted temporary copies with fake process files and CLI behavior. They never connect to a soundbar or modify the real USB. The synthetic identity replacement is confined to those copies. Results go in ignored `build/test-results/`.

Optional descriptor validation against a legally obtained extracted ARM BusyBox uses `JBL_ARM_BUSYBOX=/absolute/path/to/busybox python3 tests/check_descriptors.py`, with `qemu-arm` and registered ARM binfmt available for direct applet execution. That binary is not bundled.

## Layout

- `usb/`: exact installable device source, with CRLF-preserved launcher/hook.
- `README.md`: installation, removal, compatibility, troubleshooting, evidence and credits.
- `COMMUNITY.md`: discussion links and a first-person draft.
- `USB-README.txt` / `ROLLBACK.txt`: short guides included in the ZIPs.
- `tools/`: package builders and test runner.
- `ssh/`: portable SSH setup templates; generated passwords, Wi-Fi configuration and device keys stay out of Git.
- `webremote/`: small Go server and embedded browser UI; root README records which controls have hardware evidence.
- `tests/`: portable fixture suite and pinned revision 5 file hashes.
- `research/`: public index plus ignored local-only artifacts; see its README.

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
Go 1.25.5). The bar runs the resulting static ARM binary; no Go runtime or
third-party Go modules are needed on it.

```sh
python3 tools/build_web.py --allowed-cidr <trusted-lan-subnet>
cd webremote && go test ./...
```

Use the actual trusted IPv4 subnet, for example `192.168.1.0/24`. The output
directory is `dist/webremote-private/`; its config contains the listener and
local subnet. The browser remote requires no login. SSH keeps its own password.
The builder strips local source paths and VCS metadata. Identical sources
and Go toolchain produce the same server binary; owner settings differ.

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

Open `http://<bar-ip-address>:8080` directly; there is no web login.
Anyone on the configured trusted subnet can use these controls. Use the bar’s
IP address in the URL; DNS hostnames are rejected to prevent rebinding.
The server accepts requests only from the configured subnet. Fixed control
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
power, input transitions and manual Standard selection remain experimental.

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
stale polls, standby, Bluetooth feedback and 320/390/1024px layouts. Go tests
cover network/Host/CSRF boundaries, state confirmation and Bluetooth decoding.
UI fixtures do not substitute for hardware pairing or audible tests.
