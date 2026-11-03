# JBL Bar Toolkit

Jailbreak your soundbar, open a local web remote, and add features without a cloud account.
One USB is the canonical installation entry point. Hardware selection is automatic.

![Browser remote with example settings](docs/web-remote.png)

**Development build:** Bar 9.1 is the only packaged hardware implementation.
Individual features have device evidence; the complete USB installation, restart
and recovery flow still needs hardware qualification. Other bars remain research.

## Install

1. Download **jbl-bar-toolkit-v…-usb.zip** and its matching **-recovery.zip** from
   [Releases](https://github.com/kisinga/jbl-bar-toolkit/releases). Before a release
   is published, use **jbl-bar-toolkit-usb-development** under a successful
   [package workflow](https://github.com/kisinga/jbl-bar-toolkit/actions/workflows/package.yml) run.
   GitHub wraps CI artifacts in another ZIP; extract that wrapper first.
2. Extract the USB ZIP onto a clean **FAT32** USB, with `AutoScript`, `JBLToolkit`
   and `SETUP.html` at its root. Keep the recovery ZIP on your computer.
3. Open **SETUP.html**. Choose your network and features, download `config.txt`,
   and replace `JBLToolkit/config.txt` on the USB. Start with **Check**.
4. Insert it into the fully powered-on bar. The dispatcher identifies the
   included hardware implementation; its installer checks firmware, storage and
   selected features. Read `JBLToolkit/STATUS.txt` and `HARDWARE.txt` afterward.
5. Once checks pass, choose **Install** in SETUP.html and repeat. Remove USB,
   open **http://jbl.local** (or the bar's IP), then verify a USB-free restart.

No Go or Python is needed to install a prebuilt ZIP. SSH and optional Wi-Fi
credentials are configured locally; your configured USB is private.
[Installation and recovery guide](docs/models/bar91/USB-TOOLKIT.md).

## Hardware

| Hardware | USB installation | Current work |
| --- | --- | --- |
| **Bar 9.1 True Wireless Surround** | Packaged; exact executable identities required | Web remote, sound/playback controls, automatic Standard Mode and optional root SSH; experimental Front EQ and center gain |
| Original **Bar 800 / 1000 / 1300 / 1300X** | Not implemented | Firmware research and read-only network inspection; original 1300 is the next device target |
| **MK2 and other models** | Not implemented | Separate identification and qualification required |

Use the [short hardware guide](docs/HARDWARE.md) if unsure. A model name alone
cannot authorize installation. Unsupported identities stop before an installer
runs. AutoScript startup itself is currently established only for Bar 9.1;
automatic selection does not make other bars execute USB scripts.

## Features and existing installations

The web remote provides local volume, mute, source, playback and sound controls.
Bar 9.1 also has audio sync, surround calibration, subwoofer pairing and audio
format reporting. Optional features compile into one application and appear
according to its capabilities.

Single USB is the canonical path for new installations. **Keep an existing
installation's original USB and recovery package.** Cross-version USB/SSH
updates are not implemented yet; packages refuse incompatible replacements.
Only the documented legacy chain has a migration path. Repository moves do not
rename installed directories. [Existing installation guidance](models/bar91/README.md).

## Contribute

```sh
./jbl dev                         # Local UI preview, no hardware needed
./jbl test                        # Fast Go tests
./jbl test --full                 # Host integration, race checks and vet
./jbl build                       # Public ZIP with automatic hardware selection
./jbl build --model bar91         # Hardware-specific distribution
./jbl inspect --host <bar-ip>     # Read-only family research; Go/Python required
```

| Directory | Purpose |
| --- | --- |
| `apps/remote/` | Go app, shared server, feature registry and browser components |
| `models/` | Model-specific profiles, installation, native helpers and validation |
| `packaging/` | Shared USB dispatcher, offline setup and release text |
| `tools/`, `tests/` | Host commands and integration fixtures; Go tests live beside code |
| `research/` | Firmware findings and reproduction tools; raw captures stay private |

[Contributing](CONTRIBUTING.md) · [Brief migration notes](docs/MIGRATION.md) ·
[Components](docs/COMPONENTS.md) · [Release packaging](docs/RELEASING.md) ·
[Bar 1300 research](research/bar-family/bar1300/README.md)

Independent project; not affiliated with JBL. MIT covers project code and
documentation. Vendor firmware and proprietary libraries are not bundled.
[Credits](docs/models/bar91/STANDALONE.md#credits).
