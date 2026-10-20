# JBL Bar 9.1 — browser remote, Smart Mode fix and root SSH

I’m turning my JBL Bar 9.1 into something I can control and tinker with. The browser remote runs on the bar itself; the Smart Mode helper and root SSH provide the foundation underneath it. Everything here is optional, reversible and specific to the firmware I tested.

| Tool | Source | What I use it for |
| --- | --- | --- |
| [Browser remote](#local-browser-remote) | [`webremote/`](webremote/) | Phone-friendly controls, Bluetooth devices, Wi-Fi status and sound settings, plus an optional CLI. Written in Go. |
| [Smart Mode off](#smart-mode-off) | [`smart-mode/`](smart-mode/) | Install once from USB, then select Standard Mode automatically after wake. |
| [Root SSH](#root-ssh) | [`ssh/`](ssh/) | USB-free terminal access, SFTP and a base for my own experiments. |

**Experimental: tested on my own bar and one firmware build.** I am not affiliated with JBL. The original repository name was `jbl-bar9-standard-mode`; existing release filenames stay the same.

I install features in this order: **Smart Mode → SSH → browser remote**, and remove them in reverse. Each installer preserves the startup hook it found. Extract only one USB package at a time; they share the AutoScript entry point.

## Local browser remote

![JBL Bar 9.1 browser remote with example settings](docs/web-remote.png)

I open **[http://jbl.local](http://jbl.local)** from a phone or computer on my configured LAN. I register that alias with the bar’s existing Bonjour daemon. It needs local multicast and mDNS support; if the name conflicts or discovery is blocked, I use **`http://<bar-ip-address>`**. Port 80 needs no suffix; `:8080` remains available. There is no web login. SSH retains its separate password. [Build and install instructions](CONTRIBUTING.md#browser-remote) produce one static ARM Go executable with embedded assets and no external Go modules or ongoing computer service. Bluetooth playback adds an optional owner-built native helper.

- **Controls:** volume, mute, power, input selection and Standard Mode, with live state. I can tap repeatedly, hold +/−, use the arrow keys or press M to mute. Queued changes show **Adjusting…** until confirmed and can be cleared.
- **Bluetooth:** the bar’s name, connected device name, native streaming/suspended state and pairing status. **Pair a device** opens pairing; I finish connecting in the phone’s Bluetooth settings. The playback card stays visible when paused. A device name comes from the matching Bluetooth cache entry and may lag a rename.
- **Wi-Fi:** current network, address, signal and band. This is read-only; credentials are never exposed. I read the native control socket directly from Go.
- **Details:** uptime, Smart Mode automation status and read-only Cast tone settings when reported.
- **Sound:** Night Mode, Bass Levels 1–5, Atmos height Low/Mid/High and rear Low/Mid/High/Muted. Rear controls are blocked while either speaker is docked. Atmos height is a setting, not proof the current source is decoding Atmos. [Full EQ remains research](research/README.md#eq-exploration--20-october-2026).
- **Playback:** one Play/Pause button, Next/Previous and Stop appear when supported. Bluetooth needs the optional native helper; I confirmed audible Pause, but not Next/Previous/Stop. The toggle follows my successful requests because a sending app can pause while A2DP stays streaming. Native stream changes, source changes and disconnects reset that intent. Normal Cast controls follow receiver capabilities and still need hardware validation; mirroring exposes no transport controls.
- **Streaming:** AirPlay session activity (which does not distinguish paused audio) and Chromecast receiver information. Chrome Audio Mirroring appears as **Audio mirroring**: pausing YouTube at the sender can leave the receiver reporting PLAYING, so sender pause is unavailable. Track title, artist, album and progress appear when supplied by a normal Cast receiver. Normal receiver Playing/Paused support has fixtures but has not been verified live.

The layout puts listening controls first, sound settings alongside them and connection details below. Short transitions respect reduced-motion settings. The Equalizer section is a preview with no adjustable controls.

Go sends changed state over one event connection while the page is visible. Feedback is immediate, but the bar still takes roughly **1–2 seconds per volume step** to confirm a change. Unsent actions are discarded after an error or connection loss; controls are never replayed automatically.

I confirmed volume/mute, Bluetooth pairing, the connected phone’s name and native Playing → Paused updates, Wi-Fi readback, Night Mode Off → On → Off and complete bass/height cycles, restoring my starting settings. The page and SSH returned after a USB-free reboot. Power/input transitions and manual Standard selection remain experimental; rear adjustment still needs an undocked hardware test. The server used about 10 MiB resident memory with Cast status enabled in my latest check.

Access is restricted to my configured LAN, with fixed control names, firmware identity checks and same-origin/CSRF protection. When the bar’s native setup hotspot is already active, clients can also open **`http://192.168.255.249`** on that interface. Automatic hotspot activation after Wi-Fi loss and captive-portal opening remain unconfirmed; I have not changed Wi-Fi setup policy.

I can also use the optional Python 3 CLI against the same Go API, with no Python packages to install:

```sh
python3 tools/jbl.py status
python3 tools/jbl.py night on
python3 tools/jbl.py smart off
```

`volume up` / `volume down`, `mute`, `power`, `source tv|hdmi|bluetooth` and supported playback commands are available too. I use `--host <private-ipv4[:port]>` if discovery is unavailable, or `--json` for token-free state output. Night and Smart Mode commands check the current state before changing it. Manual `smart off` leaves the existing automatic wake watcher in place.

## Smart Mode off

Install once from USB, then remove the drive: the helper waits indefinitely in standby and selects Standard Mode after wake, including after the mains restart I tested. Repeated standby/CEC wakes and other builds remain unverified.

### Download

- **[Install ZIP](https://github.com/kisinga/jbl-bar-9.1/releases/download/v0.1.0/jbl-bar9-standard-mode-v0.1.0-install.zip)**
- **[Uninstall ZIP](https://github.com/kisinga/jbl-bar-9.1/releases/download/v0.1.0/jbl-bar9-standard-mode-v0.1.0-uninstall.zip)** — keep this available.
- [Release notes and SHA-256 checksums](https://github.com/kisinga/jbl-bar-9.1/releases/tag/v0.1.0)

Use these ready-to-copy ZIPs, rather than GitHub's source archive. No configuration is needed. The separate [`work/faster-wake` branch](https://github.com/kisinga/jbl-bar-9.1/tree/work/faster-wake) is an **untested hardware candidate**.

### Requirements

- **JBL Bar 9.1 True Wireless Surround** with the [supported executables](#compatibility). The recorded firmware label is **23.06.41.80**; I have not independently verified the display version.
- A working **FAT32 USB drive**, first partition on the first USB disk (`/mnt/sda1`). I tested a 512 MiB partition; that size is not an established requirement. Back up files before formatting.
- A computer to extract ZIPs/read logs, and the soundbar remote.

No firmware download, soldering, developer tools or ongoing computer/network service is required.

### Install

1. Download both ZIPs, keeping the uninstall package on your computer.
2. Extract the install ZIP's **contents directly to a clean USB root**, without an extra enclosing folder or another AutoScript package.
3. Fully power on the bar and wait until it plays normally. Insert USB and leave controls alone for **2 minutes**. The automated mute sequence briefly interrupts audio.
4. Expect **SMART MODE OFF** if Smart Mode was enabled. Return USB to the computer: `JBLInstall/INSTALL_STATUS.txt` should say `INSTALLED_PENDING_REBOOT_TEST` or `ALREADY_INSTALLED_LOGS_EXPORTED`. Otherwise see [troubleshooting](#troubleshooting).
5. With USB removed, disconnect mains for **60 seconds**, reconnect, let LOAD finish, and turn the bar on whenever you want. Allow **60 seconds** for the change; check audio, volume, mute and standby.

The USB root must contain `AutoScript/AutoScript`, `JBLInstall/`, `README.txt`, `ROLLBACK.txt` and `manifest.json` (plus the license).

**There is no standby deadline.** Already in Standard Mode means no action or message is needed. Once a wake is handled, manually selecting Smart Mode is respected until the next observed standby/wake. Very short standby transitions may be missed.

### Remove or recover

If I also installed the browser remote or SSH autostart, I remove **browser remote → SSH → Smart Mode**, in that order, before changing the earlier startup hook. See [browser removal](CONTRIBUTING.md#browser-remote) and [SSH rollback](CONTRIBUTING.md#ssh-package).

**Removing USB does not uninstall the helper.** Extract the matching uninstall ZIP to a clean FAT32 USB root, insert into the powered bar and wait **100 seconds**. Check `JBLInstall/UNINSTALL_STATUS.txt` for `UNINSTALLED` or `ALREADY_UNINSTALLED`, then remove USB and restart.

Alternatively, add an empty `JBLInstall/ROLLBACK` file to the install USB, **without a `.txt` extension**. Removal waits for active work, backs up installed files to `JBLInstall/uninstalled-backup`, and preserves unknown hooks.

For `HOOK_DISABLED_REBOOT_TO_FINISH_UNINSTALL`, reboot with rollback USB connected; once fully on, remove/reinsert it, wait 100 seconds and check again.

If LOAD persists or controls stop responding, insert the rollback USB and allow removal to finish. If controls remain unresponsive, disconnect mains and reconnect with rollback USB inserted, then check removal logs. Do not interrupt a normal install/uninstall. If rollback fails to restore normal boot, stop repeated experiments and seek device-specific recovery help.

### Compatibility

The installer checks these exact SHA-256 identities; a matching version label alone is insufficient:

| Executable | SHA-256 |
| --- | --- |
| `bdpprog` | `86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63` |
| `cli` | `8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f` |

It also requires the expected writable `/var` environment. Do not bypass identity guards or assume other JBL models work. Firmware updates may remove the helper or invalidate its checks. v0.1.0 supports fresh installation, reinsertion of its exact installed version, and the recognized previous revision 4; other versions/hooks are preserved.

### Troubleshooting

No new `install.log` or `update.log` usually means the USB launcher did not run: check FAT32, root layout and that `JBLInstall/ROLLBACK` is absent. Start installation only once the bar plays normally.

| Result | Next step |
| --- | --- |
| `TRIAL_NOT_SUCCESSFUL_NO_INSTALL` | No new helper installed; read `trial/STATUS.txt` and `trial/latest.log`. |
| `STOPPED_LONG_PRESS_NOT_CONFIRMED` | Happened once on my bar. If controls work, let it settle and retry insertion once, leaving controls alone. Stop if repeated. |
| `FIRMWARE_MISMATCH` / `CLI_MISMATCH` | Unsupported build; keep the guards intact. |
| `EXISTING_AUTOSCRIPT_PRESERVED_NO_INSTALL` / `INSTALLED_VERSION_CONFLICT` | Another hook/version exists; do not overwrite blindly. |
| `runtime/STATUS.txt`: `STANDARD_SELECTED` | Mode change confirmed; check its boot ID. |
| `runtime/WATCH_STATUS.txt`: `STANDARD_THIS_WAKE` | This wake was handled successfully. |
| `runtime/DISABLED` | A guard stopped further action. Collect logs and uninstall. Reinsertion does not clear it. |

`UPDATE_STATUS.txt=SETUP_FINISHED` alone is not success: check `INSTALL_STATUS.txt` and `install.log`'s `EXIT=`. An upgrade's uninstall status can refer to the previous version.

For wake logs, reinsert the same install USB for **15 seconds**. Read `JBLInstall/runtime/watch.log`, `latest.log`, `last-attempt/result.txt` and `last-attempt/JBLTest/result.txt`. Compare boot IDs/uptime; the wall clock may be wrong. Older component header numbers can be normal. Reinsertion exports logs without replaying the sequence.

Report model, displayed firmware (or unknown), timing, control behavior and cold-boot/standby/CEC results in [Issues](https://github.com/kisinga/jbl-bar-9.1/issues). Attach relevant text logs after reviewing them; omit full dumps and personal/network information.

### How it works

I install `/var/JBLStandard` and the `/var/AutoScript.TSS` startup hook. The watcher checks readiness and mode, temporarily enables existing console fields in RAM, sends the normal mute-hold/volume-up sequence, verifies Standard Mode and released keys, then restores those fields. It does not flash firmware or patch executable instructions.

This persists the automation, **not JBL's saved default**. It runs with system privileges. Earlier development versions caused a startup stall or unresponsive controls; the confirmed revision includes inherited-socket cleanup and handling for disappearing process descriptors. Use the packaged release and keep its rollback ZIP.

### Smart Mode validation

On **19 October 2026**, I confirmed a mains restart followed by delayed power-on. The watcher started at uptime 8.13 seconds, detected awake at 134.67, and completed at 169.22: **34.55 seconds**. Logs recorded mode `3 → 0`, released keys and `CONSOLE_RESTORED=1`. I removed USB for this test; logs alone do not independently establish its absence.

My first installation needed a retry. This evidence covers one post-install wake, not every standby/CEC path. **102 host checks passed**, and anonymous release downloads matched a reproducible fresh-clone build. Host checks do not replace hardware testing.

## Root SSH

I confirmed root password login, an interactive shell, SFTP and startup after a reboot with **USB removed** on 20 October 2026. Wi-Fi reconnected and normal controls worked. The portable installer still needs an independent clean-install test; the ready-made Smart Mode ZIPs do **not** install SSH.

[Build my SSH package](CONTRIBUTING.md#ssh-package) with Python 3 on Linux, choosing a password and the one computer IPv4 address permitted to connect. There is no shared default password. The installer reuses the bar’s native OpenSSH on port **2222**, generates a host key and can reuse existing Wi-Fi or configure it during setup. I verify the exported host-key fingerprint before logging in:

```sh
ssh -p 2222 root@<bar-ip-address>
```

The setup keeps its account files and startup scripts in writable `/var`, with a private loader/library view; it does not replace firmware binaries. Keep the matching recovery package and a stable computer LAN address. [Installation, validation and rollback](CONTRIBUTING.md#ssh-package) are documented together.

### What I can run

This is **32-bit ARM Linux** with Android-derived services and Chromecast components, about **448 MiB RAM** and **25 MiB free writable storage** on my bar. Compatible native Linux programs can run within those limits. I found no APK package manager or normal Android app runtime.

## Credits

- **[touch MAX’s 8 January 2024 article](https://touch-max.ru/vzlom/zapusk-ssh-na-proshivke-jbl-9-1) and [almirus/jbl9.1](https://github.com/almirus/jbl9.1/tree/master/AutoScript):** the published USB entry point, persistent-storage groundwork and invitation to automate Smart Mode. I built on their work; I did not discover AutoScript. The article is credited to `admin` and links that GitHub handle, without establishing their personal identity. It also acknowledges earlier MediaTek/Blu-ray AutoScript work.
- **JBL:** [documents the manual sequence and Smart Mode reset on power-on](https://support.jbl.com/howto/jbl-bar-9-1-lacking-dialogue-too-much-surround-us/000017755.html).
- **[k1rnt/JBL Soundbar CLI](https://github.com/k1rnt/jbl-soundbar-cli/tree/7d10bdb1ebe0c5a77c9a4c14ebe4580bd3735309):** I reviewed this commit for CLI layout and sound-control ideas. Its newer Linkplay/JBL One protocol is separate from the native Bar 9.1 API I use here; I did not port that protocol or replace the wake watcher.
- **SimenCodes:** [related Smart Mode automation in JBL_Soundbar](https://github.com/MrBearPresident/JBL_Soundbar/pull/6), merged 11 November 2024 and tested on Bar 1000. This is separate network-based prior work, not evidence this USB package supports that model.

I added build-specific mode/readiness analysis, guarded key automation, release/readback checks, console restoration and persistent standby handling, using REA/Ghidra and shell tests. I claim no world first. The MIT license covers my distributed code/docs, not vendor firmware or linked upstream material; vendor firmware and upstream proprietary binaries are not bundled.

[Development and reproducible builds](CONTRIBUTING.md) · [Community links and my post draft](COMMUNITY.md) · [Research archive index](research/README.md)
