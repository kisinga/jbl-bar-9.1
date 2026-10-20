# JBL Bar 9.1 — EQ, browser remote, Smart Mode fix and root SSH

Control playback and tune Front EQ from a browser, disable Smart Mode automatically after wake, or enable root SSH for further development. Each tool runs on the bar; no ongoing computer service is required.

| Tool | Source | Function |
| --- | --- | --- |
| [Browser remote](#local-browser-remote) | [`webremote/`](webremote/) | Local Go interface, Front EQ, playback and sound controls, plus an optional CLI. |
| [Smart Mode off](#smart-mode-off) | [`smart-mode/`](smart-mode/) | Install once from USB; select Standard Mode automatically after wake. |
| [Root SSH](#root-ssh) | [`ssh/`](ssh/) | USB-free root terminal access and SFTP. |

**Experimental: hardware validation covers one Bar 9.1 and the [exact firmware identities below](#compatibility).** This project is not affiliated with JBL. The former repository name was `jbl-bar9-standard-mode`; existing release filenames remain unchanged.

Install **Smart Mode → SSH → browser remote**; remove in reverse order. Each installer preserves the startup hook it finds. Use only one AutoScript USB package at a time.

## Local browser remote

![JBL Bar 9.1 browser remote with example settings](docs/web-remote.png)

Open **[http://jbl.local](http://jbl.local)** on the configured LAN, or **`http://<bar-ip-address>`** if mDNS is unavailable or the name conflicts. Port `8080` also works. No web login is required; SSH keeps its separate password. [Build and install the remote](CONTRIBUTING.md#browser-remote).

- **Listening:** volume, mute, power, source and Standard Mode. Tap or hold +/−; use ↑/↓ or M on a keyboard. Clear queued taps to cancel unsent changes.
- **Playback:** a single Play/Pause button, Previous/Next and Stop when supported. Bluetooth requires the optional native helper. Pause has audible confirmation; Next/Previous/Stop still need hardware validation. A2DP can stay streaming while an app is paused, so the toggle follows successful requests and resets on source, peer or stream changes. Normal Cast controls require receiver capabilities; mirroring provides no transport controls.
- **Bluetooth:** connected device name, native stream activity, pairing status and a pairing button. Finish pairing in the sending device’s Bluetooth settings. Cached names can lag a rename; track metadata is not currently available over Bluetooth.
- **Sound:** Night Mode, five bass levels, three Atmos height levels and rear level/mute. Undock both rear speakers to adjust them. Atmos height does not identify the current audio format.
- **Front EQ (experimental):** choose Original, Less bass, Clear dialogue, Softer treble, Warm, Bright or Vocal focus, then press **Apply EQ**. Adjust five sliders for manual tuning; dragging only previews changes. Cuts range from −6 to 0 dB relative to original front-channel tuning, in 0.5 dB steps. Reset restores the native preset. [Build with the optional EQ helper](CONTRIBUTING.md#optional-front-eq-helper).
- **System monitor:** CPU/RAM history, storage, Wi-Fi status/traffic, load averages, uptime and process count. Updates every two seconds with up to two minutes of browser-local history; missing sensors stay unavailable.
- **Streaming and details:** AirPlay session activity, Cast application/track metadata when supplied, automation status and read-only Cast tone settings. AirPlay cannot distinguish pause. Chrome Audio Mirroring can remain PLAYING when its sending app pauses; the page labels that limitation.

[Optional Go wake automation](CONTRIBUTING.md#optional-go-wake-automation) shares the web controls’ lock and readback, replacing the shell poller while the service runs. It retains the verified button gesture and restores the shell helper when stopped or uninstalled. Enable it with `--auto-standard`; three controlled wakes measured **7.77–7.82 seconds** on the test bar ([device record](research/validation/go-automation-device.json)). Device details show the active Go state.

Live updates share one event connection while the page is visible. No database is required. Short transitions respect reduced-motion preferences. Volume feedback is immediate, but native confirmation takes roughly **1–2 seconds per step**. Errors and connection loss discard unsent controls; actions are never replayed automatically.

Confirmed hardware behavior includes volume/mute, Bluetooth pairing/name/stream readback, audible Pause, Night Mode Off → On → Off, bass/height cycles, Wi-Fi readback and USB-free web/SSH startup. Rear adjustment needs an undocked test. Power/source transitions, manual Standard selection and normal Cast transport controls remain experimental. Front EQ has audible confirmation over Bluetooth; other sources and channel layouts need listening tests. It uses the bar’s custom DSP mode, resets on restart, and resets before web power/source/sound changes. It is not a whole-system EQ. Browser fixtures do not replace hardware tests.

Access is limited to the configured LAN, with fixed commands, firmware checks and same-origin/CSRF protection. When the native setup hotspot is already active, use **`http://192.168.255.249`** from that network. Automatic hotspot activation after Wi-Fi loss and captive-portal opening remain unconfirmed.

For terminal control, use Python 3 without additional packages:

```sh
python3 tools/jbl.py status
python3 tools/jbl.py night on
python3 tools/jbl.py smart off
```

Also available: `volume up|down`, `mute`, `power`, `source tv|hdmi|bluetooth` and supported playback commands. Use `--host <private-ipv4[:port]>` for an IP address or `--json` for token-free state. Night and Smart Mode commands check state before changing it; manual `smart off` preserves the automatic watcher.

## Smart Mode off

Install once, then remove USB. The helper waits indefinitely in standby and selects Standard Mode after wake. It automates JBL’s normal button sequence; it does not change the saved firmware default or flash firmware.

### Download

- **[Install ZIP](https://github.com/kisinga/jbl-bar-9.1/releases/download/v0.1.0/jbl-bar9-standard-mode-v0.1.0-install.zip)**
- **[Uninstall ZIP](https://github.com/kisinga/jbl-bar-9.1/releases/download/v0.1.0/jbl-bar9-standard-mode-v0.1.0-uninstall.zip)** — keep a copy.
- [Release notes and SHA-256 checksums](https://github.com/kisinga/jbl-bar-9.1/releases/tag/v0.1.0)

Use the release ZIPs, rather than GitHub’s source archive. No configuration is needed. Source builds now contain revision 6: faster timing measured at about **18 seconds** on the test bar, versus about 35 seconds for revision 5. The linked v0.1.0 ZIPs remain unchanged. [Build and qualification details](CONTRIBUTING.md#faster-wake--revision-6).

### Requirements

- **JBL Bar 9.1 True Wireless Surround** matching the supported executables. The recorded firmware label is **23.06.41.80**; its display version has not been independently verified.
- Working **FAT32 USB**, first partition of the first USB disk (`/mnt/sda1`). A 512 MiB partition was tested; that size is not a requirement. Back up files before formatting.
- Computer for ZIP extraction/logs, and the soundbar remote.

No firmware download, soldering, developer tools or ongoing network service is needed.

### Install

1. Download both ZIPs; keep the uninstall package on the computer.
2. Extract the install ZIP’s contents directly to a clean USB root. Check for `AutoScript/AutoScript`, `JBLInstall/`, `README.txt`, `ROLLBACK.txt`, `manifest.json` and the license, with no enclosing folder or another AutoScript package.
3. Turn the bar fully on and wait for normal playback. Insert USB and leave controls alone for **2 minutes**. Audio briefly mutes during the sequence.
4. Expect **SMART MODE OFF** if Smart Mode was enabled. Return USB to the computer. `JBLInstall/INSTALL_STATUS.txt` must say `INSTALLED_PENDING_REBOOT_TEST` or `ALREADY_INSTALLED_LOGS_EXPORTED`; otherwise follow [troubleshooting](#troubleshooting).
5. Remove USB, disconnect mains for **60 seconds**, reconnect and let LOAD finish. Turn the bar on whenever convenient. Allow **60 seconds** for Standard selection; verify audio, volume, mute and standby.

There is **no standby deadline**. If Standard is already selected, no action/message is needed. After a wake is handled, manual Smart Mode selection is respected until the next observed standby/wake. Very short standby transitions may be missed.

### Remove or recover

Remove **browser remote → SSH → Smart Mode** before changing an earlier startup hook. Follow [browser removal](CONTRIBUTING.md#browser-remote) and [SSH rollback](CONTRIBUTING.md#ssh-package).

Removing USB does **not** uninstall the helper. Extract the matching uninstall ZIP to a clean FAT32 USB root, insert into the powered bar and wait **100 seconds**. Require `JBLInstall/UNINSTALL_STATUS.txt` to show `UNINSTALLED` or `ALREADY_UNINSTALLED`; remove USB and restart.

Alternatively, add an empty `JBLInstall/ROLLBACK` file to the install USB, with **no `.txt` extension**. Removal waits for active work, saves files to `JBLInstall/uninstalled-backup` and preserves unknown hooks. For `HOOK_DISABLED_REBOOT_TO_FINISH_UNINSTALL`, reboot with rollback USB connected; once fully on, remove/reinsert it, wait 100 seconds and check again.

If LOAD persists or controls stop responding, insert rollback USB and allow removal to finish. If still unresponsive, disconnect mains and reconnect with rollback USB inserted, then inspect removal logs. Do not interrupt a normal installation/removal. If rollback does not restore normal boot, stop repeated experiments and seek device-specific recovery help.

### Compatibility

Match these exact SHA-256 identities; a version label alone is insufficient:

| Executable | SHA-256 |
| --- | --- |
| `bdpprog` | `86ec83472a10dd4278285123f8ecbfb96bdd97444dd98c3a56d5d87441c90b63` |
| `cli` | `8910fb852d2f994a519615fede404fbb036e3a8f5264cc69cdff57038af3c78f` |

The expected writable `/var` environment is also required. Keep identity guards intact; other models/builds are unsupported without validation. Firmware updates can remove the helper or invalidate its checks. v0.1.0 accepts fresh installation, its exact installed version and recognized revision 4; unknown versions/hooks are preserved.

### Troubleshooting

No new `install.log`/`update.log`: check FAT32, USB root layout and absence of `JBLInstall/ROLLBACK`. Start only after normal playback.

| Result | Action |
| --- | --- |
| `TRIAL_NOT_SUCCESSFUL_NO_INSTALL` | Read `trial/STATUS.txt` and `trial/latest.log`; no new helper was installed. |
| `STOPPED_LONG_PRESS_NOT_CONFIRMED` | If controls work, let the bar settle and retry insertion once without touching controls. Stop if repeated. |
| `FIRMWARE_MISMATCH` / `CLI_MISMATCH` | Keep guards intact; the build is unsupported. |
| `EXISTING_AUTOSCRIPT_PRESERVED_NO_INSTALL` / `INSTALLED_VERSION_CONFLICT` | Preserve the existing hook/version; do not overwrite blindly. |
| `runtime/STATUS.txt`: `STANDARD_SELECTED` | Confirmed mode change; check the boot ID. |
| `runtime/WATCH_STATUS.txt`: `STANDARD_THIS_WAKE` | This wake was handled. |
| `runtime/DISABLED` | Collect logs and uninstall. Reinsertion does not clear the guard. |

`UPDATE_STATUS.txt=SETUP_FINISHED` alone is insufficient: check `INSTALL_STATUS.txt` and the install log’s `EXIT=`. An upgrade’s uninstall status may refer to the previous version.

To export wake logs, reinsert the same install USB for **15 seconds**. Inspect `JBLInstall/runtime/watch.log`, `latest.log`, `last-attempt/result.txt` and `last-attempt/JBLTest/result.txt`. Compare boot IDs/uptime; wall-clock time and older component headers can mislead. Reinsertion exports logs without replaying the sequence.

[Report an issue](https://github.com/kisinga/jbl-bar-9.1/issues) with model, displayed firmware (or unknown), timing, normal control behavior and cold-boot/standby/CEC results. Review text logs before attaching them; omit full dumps and personal/network information.

### Implementation and validation

`/var/JBLStandard` and `/var/AutoScript.TSS` run a guarded watcher. It checks readiness/mode, temporarily changes two console fields in RAM, sends mute-hold/volume-up, verifies Standard and released keys, then restores the fields. Executable instructions remain unchanged. Revision 5 includes inherited-socket cleanup and disappearing-descriptor handling; preserve both when modifying startup scripts.

The confirmed 19 October 2026 cold boot detected awake at uptime 134.67 seconds and completed at 169.22: **34.55 seconds**, with mode `3 → 0`, released keys and `CONSOLE_RESTORED=1`. USB removal was reported for that test; logs alone do not establish its absence. The first installation required a retry. Repeated standby/CEC paths remain unverified. The original release passed **102 host checks** and matched reproducible fresh-clone/anonymous downloads; current tools pass **141 host checks**.

## Root SSH

[Build a private SSH package](CONTRIBUTING.md#ssh-package) using Python 3 on Linux. Choose a password and the single computer IPv4 address allowed to connect. No shared default password is provided. The native OpenSSH daemon uses port **2222**; setup can reuse configured Wi-Fi or provision it interactively.

Verify the exported host-key fingerprint, then connect:

```sh
ssh -p 2222 root@<bar-ip-address>
```

USB-free startup, password login, an interactive shell and SFTP were confirmed on 20 October 2026, with Wi-Fi and normal controls restored after reboot. The portable installer still needs an independent clean-install test. Smart Mode release ZIPs do **not** install SSH.

Account files, startup scripts and the private compatible loader/library view stay in writable `/var`; firmware binaries are not replaced. Keep the matching recovery ZIP and a stable computer LAN address. Remove the browser remote before rolling back SSH.

Use compatible **32-bit ARM Linux** programs within the device’s limits: approximately **448 MiB RAM** and **25 MiB free writable storage** in the recorded capture. Android-derived/Chromecast services are present; no normal APK package manager/runtime was found.

## Credits

- **[touch MAX’s 8 January 2024 article](https://touch-max.ru/vzlom/zapusk-ssh-na-proshivke-jbl-9-1) and [almirus/jbl9.1](https://github.com/almirus/jbl9.1/tree/master/AutoScript):** published USB entry point and persistent-storage foundation. The article is credited to `admin` and links that GitHub handle; it does not establish their personal identity. It also acknowledges earlier MediaTek/Blu-ray AutoScript work.
- **JBL:** [manual Smart Mode sequence and reset on power-on](https://support.jbl.com/howto/jbl-bar-9-1-lacking-dialogue-too-much-surround-us/000017755.html).
- **[k1rnt/JBL Soundbar CLI](https://github.com/k1rnt/jbl-soundbar-cli/tree/7d10bdb1ebe0c5a77c9a4c14ebe4580bd3735309):** CLI/control/discovery inspiration. Its newer Linkplay/JBL One transport is separate from the native Bar 9.1 implementation and does not supply this persistent wake watcher.
- **SimenCodes:** [related Smart Mode automation](https://github.com/MrBearPresident/JBL_Soundbar/pull/6), merged 11 November 2024 and tested on Bar 1000. That separate network-based implementation does not establish support for that model here.

Build-specific mode/readiness analysis, guarded automation, console restoration and standby handling extend the published foundation; no world-first claim is made. MIT covers the distributed code/docs, not vendor firmware or linked upstream material. Vendor firmware and proprietary binaries are not bundled.

[Development and builds](CONTRIBUTING.md) · [Community links and post draft](COMMUNITY.md) · [Research archive](research/README.md)
