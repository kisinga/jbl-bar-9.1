# JBL Bar Toolkit

**Control your JBL Bar 9.1 from your phone or computer.** Change the volume,
choose what is playing, and adjust the sound in a browser on your home network.
No cloud account or app to install.

![Browser remote showing example sound and bass controls](docs/web-remote.png)

*Screenshot uses sample data, not a connection to a real soundbar.*

**Still experimental.** Bar 9.1 is the only model with an installation package.
Individual features have been tested on a bar; the complete public USB install,
restart and recovery process still needs hardware testing.

## What you need

- A **JBL Bar 9.1 True Wireless Surround** on your home network.
- A computer and a spare USB stick formatted as **FAT32**. Formatting erases the
  stick, so copy anything you want to keep first.
- Your phone or computer connected to the same home network as the bar.

Already using an older installation? **Keep its original USB and recovery ZIP.**
Installing a new release over an older one is not supported yet. See the
[existing installation guide](models/bar91/README.md) before making changes.

## Get started

1. **Download two files.** Open [Downloads / Releases](https://github.com/kisinga/jbl-bar-toolkit/releases)
   and choose a release with **jbl-bar-toolkit** files. Older standalone
   Smart Mode releases use different installers. Under **Assets**, get
   `jbl-bar-toolkit-v…-usb.zip` and the matching `jbl-bar-toolkit-v…-recovery.zip`.
   Keep the recovery file on your computer in case you need to undo the install.
   Choose the toolkit ZIPs rather than GitHub's “Source code” downloads.
2. **Put the files on your USB.** Extract the **usb.zip** directly onto the stick.
   When you open the stick, you should see `AutoScript`, `JBLToolkit` and
   `SETUP.html` immediately, rather than inside another folder.
3. **Open `SETUP.html`.** Double-click it on the USB. The page helps you choose
   your home network and features. Leave **Browser remote** on and **SSH** off
   for a first install. Leave the action on **Check compatibility first**.
   Click **Download config.txt**, then move that file into the USB's
   `JBLToolkit` folder, replacing the file already there.
4. **Check your bar.** Safely eject the USB from your computer. Turn the bar on,
   plug in the stick, and wait **five minutes**. Bring it back to your computer
   and open `JBLToolkit/STATUS.txt`. Continue only if it says
   `CHECK_PASSED_READY_FOR_INSTALL_OR_CONFIGURE`.
5. **Install.** Open `SETUP.html` again, choose **Install selected features**,
   download and replace `config.txt` again. Safely eject the stick, put it back
   in the powered-on bar, and wait five minutes. Read `STATUS.txt` afterward;
   continue only if it says `INSTALLED_PENDING_REBOOT_TEST`. Otherwise follow
   the help below before continuing.
6. **Use your remote.** Remove the USB. Open **[jbl.local](http://jbl.local)** on
   your phone or computer. Unplug the bar from power for **60 seconds**, then
   reconnect it, let it finish starting, and turn it on. Keep the USB out.
   Check that the browser remote and your normal audio controls still work.
7. **Confirm the restart.** On your computer, open `SETUP.html`, choose
   **Verify after a restart without USB**, and replace `config.txt` as before.
   Put the USB in the powered-on bar, wait five minutes, then read `STATUS.txt`
   on your computer. Expect `VERIFIED_CURRENT_BOOT_PENDING_USER_TESTS`; check
   normal playback and volume yourself, then remove and keep the USB.

You do not need Python, Go, a terminal or an SSH connection to use the downloaded
packages. Your setup page runs locally; your network details stay on your USB.
Keep that configured stick private.

**No toolkit release available yet?** Development packages are under **Artifacts** on a
successful [Test and package toolkit run](https://github.com/kisinga/jbl-bar-toolkit/actions/workflows/package.yml).
Sign in to GitHub and download **jbl-bar-toolkit-usb-development**. Extract
GitHub's outer ZIP first, then follow the steps above with the USB/recovery ZIPs
inside. These downloads expire after 30 days.

## Make the sound yours

| Control | What it does |
| --- | --- |
| Volume, mute and source | Everyday controls for TV/ARC, HDMI and Bluetooth. |
| Playback | Play, pause and skip when the connected source supports them. |
| Bass **1–5** | Pick a bass level directly. The toolkit handles the steps and confirms the result. |
| Bass offset **(experimental)** | Fine-tune all five bass levels from **−3 to +3 dB**, in half-decibel steps. Pick an offset, then press **Save offset**. **0 dB** restores stock bass on the next supported playback route. |
| Center **1–6** | Boost the dialogue channel. **1** is the original level; higher levels add gain. |
| Rear speakers and Atmos height | Adjust the rear speaker level and the height effect for Atmos audio. |
| Standard / Smart | Choose the sound mode. During setup you can also enable automatic Standard Mode after wake, or remember the mode you chose before standby. |
| Night mode | Toggle it when the bar reports support. |

Bass offset is saved for restarts and applied during supported, stable
**Standard Mode** playback. It may wait or pause on other audio routes. Start
at a comfortable volume; extra bass or center gain can distort at high volume.
[How bass offset works](docs/BASS_OFFSET.md).

Open **Advanced** for audio delay (lip sync), rear speaker calibration,
and subwoofer pairing. The separate **Front EQ** panel offers experimental
front-speaker tuning. Controls appear or become available
according to the device, firmware and included helpers; some advanced controls
need a separately prepared build. Front EQ is limited to supported Standard
playback routes. The system monitor shows device health and rear speaker battery
levels when available. [Feature and setup details](docs/models/bar91/USB-TOOLKIT.md).

![Advanced controls with sample data](docs/web-remote-advanced.png)

## Need help?

- **The remote page will not open:** try the bar's IP address from your router's
  connected-device list, such as `http://192.168.1.50`. If needed, try
  `http://192.168.1.50:8080`. Your phone and bar must be on the same home network.
- **The USB check is blocked:** open `JBLToolkit/CHECKS.txt`, `PREFLIGHT.txt` and
  `HARDWARE.txt` on the stick. They explain what failed. Use the
  [installation and recovery guide](docs/models/bar91/USB-TOOLKIT.md) for next steps.
- **Need to undo an installation:** use the matching recovery ZIP and the
  [recovery instructions](docs/models/bar91/USB-TOOLKIT.md). Keep the original files
  for your installed version.
- **Have another model:** Bar 800, 1000, 1300/1300X, MK2 and other models do not
  have installers yet. See [supported hardware](docs/HARDWARE.md).

## For contributors

```sh
./jbl dev                         # Local UI preview with sample data
./jbl test                        # Fast Go tests
./jbl test --full                 # Host integration, race checks and vet
./jbl build                       # Public USB and recovery ZIPs
./jbl build --model bar91         # Model-specific packages
./jbl inspect --host <bar-ip>     # Read-only research; Go/Python required
```

[Contributing](CONTRIBUTING.md) · [Changelog](CHANGELOG.md) ·
[Releasing](docs/RELEASING.md) · [Code layout and components](docs/COMPONENTS.md) ·
[Migration notes](docs/MIGRATION.md) · [Bar 1300 research](research/bar-family/bar1300/README.md)

Independent project; not affiliated with JBL. MIT covers project code and
documentation. Vendor firmware and proprietary libraries are not bundled.
[Credits](docs/models/bar91/STANDALONE.md#credits).
