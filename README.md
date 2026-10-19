# JBL Bar 9.1: automatic Standard Mode

Install once from USB. The soundbar then switches Smart Mode off automatically after it wakes, including after a mains restart. You can remove the USB after installation.

**Experimental, owner-tested on one supported firmware build.** Cold boot followed by delayed power-on is confirmed. Repeated standby/CEC wakes and other firmware builds are not yet validated. This is an independent community project, not a JBL product.

## Download

- **[Install ZIP](https://github.com/kisinga/jbl-bar9-standard-mode/releases/download/v0.1.0/jbl-bar9-standard-mode-v0.1.0-install.zip)**
- **[Uninstall ZIP](https://github.com/kisinga/jbl-bar9-standard-mode/releases/download/v0.1.0/jbl-bar9-standard-mode-v0.1.0-uninstall.zip)** — keep this available.
- [Release notes and SHA-256 checksums](https://github.com/kisinga/jbl-bar9-standard-mode/releases/tag/v0.1.0)

Use the **install ZIP above**, rather than GitHub's “Source code” archive. It already has the correct USB layout. No edits, account, IP address or device-specific configuration are needed.

## What you need

- A **JBL Bar 9.1 True Wireless Surround** on the supported build. The recorded firmware label is **23.06.41.80**; the installer checks exact executable hashes automatically. The display version was not independently verified. See [compatibility](docs/COMPATIBILITY.md).
- A working **FAT32 USB flash drive**, with one partition as the first USB disk. A 512 MiB FAT32 partition worked in our test; that size is not a requirement established by testing. Back up files before formatting a drive.
- A computer to unzip/copy files and read status files, plus the soundbar remote.

You do not need to download firmware, run commands, solder, install developer tools, or leave a computer or network service running.

## Install

1. Download **both ZIPs**, keeping the uninstall ZIP on your computer.
2. Extract the **contents of the install ZIP** directly to the USB root. Do not put them inside an extra folder. Use a clean drive without another AutoScript package.
3. Fully power on the bar and wait until it is playing normally. Insert the USB. Leave the remote/buttons alone for **2 minutes**; the automated mute sequence can briefly interrupt audio.
4. You should see **SMART MODE OFF** if Smart Mode was on. Remove the USB and check `JBLInstall/INSTALL_STATUS.txt` on your computer: success is `INSTALLED_PENDING_REBOOT_TEST` or `ALREADY_INSTALLED_LOGS_EXPORTED`. If it says anything else, follow [troubleshooting](docs/TROUBLESHOOTING.md) before continuing.
5. With USB removed, disconnect mains for **60 seconds**, reconnect, let LOAD finish, and turn the bar on whenever you want. Allow **60 seconds** for Smart Mode to switch off. Check audio, volume, mute and standby.

USB layout:

```text
USB root/
├── AutoScript/
│   └── AutoScript
├── JBLInstall/
│   ├── install.sh
│   ├── setup.sh
│   ├── uninstall.sh
│   └── payload/…
├── README.txt
├── ROLLBACK.txt
└── manifest.json
```

**The standby wait is indefinite.** There is no requirement to leave it in standby for two minutes: that was a validation test. Turn it on immediately or hours later. The confirmed release took about **35 seconds from detected power-on** to completing the change. It waits for the bar to be ready and checks that simulated keys are released.

If already in Standard Mode, it does nothing and may show no message. Once it has handled a wake, manually selecting Smart Mode is respected until the next observed standby/wake cycle. Very short standby transitions may be missed.

## Remove it

Removing the USB **does not uninstall** the helper.

1. Extract the **uninstall ZIP** to a clean FAT32 USB root.
2. Insert it into the powered bar and wait **100 seconds**.
3. Return USB to the computer. Confirm `JBLInstall/UNINSTALL_STATUS.txt` says `UNINSTALLED` or `ALREADY_UNINSTALLED`.
4. Remove USB and restart the bar. Smart Mode returns to JBL's normal behavior.

Alternatively, create an empty file named `ROLLBACK` **without an extension** inside `JBLInstall` on the installation USB. [Full removal and recovery instructions](docs/TROUBLESHOOTING.md#remove-or-recover).

## What it changes

The installer adds a small helper under `/var/JBLStandard` and its startup hook `/var/AutoScript.TSS`. It waits for an awake state, checks the current mode, temporarily enables the existing command handler, sends the normal mute-hold/volume-up sequence, verifies Standard Mode and key release, then restores the command-handler settings. It uses temporary process-memory writes for those settings; it does not flash a firmware image or patch executable instructions.

This is a persistent automation workaround: it reapplies Standard after wake. It does not make JBL's own saved default permanent. Firmware updates may remove it or cause the build checks to refuse operation. It runs with the device's system privileges; keep the uninstall package available. Earlier development revisions caused a startup stall or unresponsive controls, so use the release package rather than old experimental files.

## Documentation and contributions

- [Compatibility and exact supported hashes](docs/COMPATIBILITY.md)
- [Troubleshooting, status files and recovery](docs/TROUBLESHOOTING.md)
- [How it works and where RE findings helped](docs/DESIGN.md)
- [What has actually been tested](docs/VALIDATION.md)
- [Development, tests and reproducible ZIPs](CONTRIBUTING.md)
- [Credits and prior work](docs/CREDITS.md)

Report your model, firmware label, cold-boot/standby results and relevant **text status/log files** through [Issues](https://github.com/kisinga/jbl-bar9-standard-mode/issues). Do not upload full firmware, backups, memory dumps or personal/network information.

JBL documents the manual key sequence and reset to Smart Mode on every power-on in its [support article](https://support.jbl.com/howto/jbl-bar-9-1-lacking-dialogue-too-much-surround-us/000017755.html). The USB execution mechanism builds on [almirus/touch MAX's published work](https://github.com/almirus/jbl9.1). This project contributes the guarded Bar 9.1 automation and startup handling; it does not claim to have discovered AutoScript.
