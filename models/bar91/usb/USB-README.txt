JBL BAR 9.1 TOOLKIT — EXPERIMENTAL COMBINED INSTALLER

Host-tested; a clean combined install and recovery on hardware are still required.
Supported firmware identities and the full guide: docs/models/bar91/USB-TOOLKIT.md in the repo.
https://github.com/kisinga/jbl-bar-toolkit

CHECK FIRST
Open SETUP.html in your browser. Choose the home network and features, download
config.txt and use it to REPLACE JBLToolkit/config.txt. No Go or Python needed
for a prebuilt release. SSH and optional Wi-Fi credentials can be in this config;
the page derives them locally. Keep the configured USB private.
Set action=check in JBLToolkit/config.txt. Insert with the bar fully on and wait
FIVE MINUTES. Read CHECKS.txt, PREFLIGHT.txt, DIAGNOSTICS.txt, native-check.json and STATUS.txt on the computer.
Require CHECK_PASSED_READY_FOR_INSTALL_OR_CONFIGURE. Recognized installations
are checked for update or migration; do not delete them to bypass ownership checks.
The check does not install services, change networking or press controls.

INSTALL / UPDATE
1. Keep the matching recovery ZIP on your computer.
2. Extract the install ZIP directly to a clean FAT32 USB root.
   Expect AutoScript/AutoScript and JBLToolkit/config.txt (no enclosing folder).
3. In config.txt set the actual trusted LAN subnet and selected features.
   Choose Install / update everything (action=update). SETUP.html can prepare
   credentials locally in config.txt; private builds can bundle them separately.
4. Turn the bar fully on and wait for normal playback. Insert USB, leave controls
   alone and wait FIVE MINUTES. Audio may briefly mute if auto_standard=true.
5. Return USB to the computer. Require JBLToolkit/STATUS.txt to say
   INSTALLED_PENDING_REBOOT_TEST. Read RESULT.txt and logs/connection.txt.
6. Remove USB, disconnect mains for 60 seconds, reconnect, let LOAD finish,
   turn on and verify web/SSH/Standard Mode and normal controls.
7. Set action=verify and reinsert. Read VERIFY.txt and require
   VERIFIED_CURRENT_BOOT_PENDING_USER_TESTS. Machine checks require a new kernel
   boot with the managed services ready, intact runtime and matching native state.
   Audio, SSH/SFTP, USB-free boot, standby/CEC and recovery still need user tests.
   SSH: verify logs/host_ed25519.pub and the fingerprint in connection.txt.

CHANGES AND LOGS
Reinsert with action=install to export logs without changing features.
For explicit changes, edit feature/network settings and use action=configure.
For newer toolkit packages, choose action=update. Existing toolkit settings,
SSH credentials/host keys and saved sound settings are retained. Bundled app and
scripts are replaced; JBL firmware is not flashed. Reinsertions of the same
package only export status. Unknown/modified installs stop before replacement.
Credentials edited in config.txt apply only with action=configure; existing
sessions remain until closed/rebooted. Keep your current config before changing it.
Configuration actions restart managed services. Existing SSH sessions may remain.
Do not combine standalone Smart Mode, SSH or web installers with this package.

LEGACY MIGRATION (ADVANCED)
Normal install preserves existing standalone stores and refuses adoption.
Install / update automatically migrates the exact reviewed rev6 + SSH + remote
chain. The explicit action=migrate remains available for advanced use.
Read MIGRATION_CHECK.txt and the repository guide first. It verifies owner
markers, exact scripts and hook snapshots, stops recognized services, retains
original files inactive in /var/JBLToolkitLegacy and installs the toolkit.
It preserves the SSH host key. Unknown scripts/hooks/processes are not adopted.
Recovery deactivates the toolkit; it does not reactivate the old chain.

RECOVERY
Set action=uninstall, or create an empty JBLToolkit/RECOVER file (no extension).
The RECOVER marker works even if config.txt or the application payload is broken.
Alternatively extract the matching recovery ZIP onto a clean FAT32 USB drive.
Insert with the bar on; wait TWO MINUTES. Require RECOVERED_REBOOT_REQUIRED in
JBLToolkit/STATUS.txt. Remove USB and restart. Files and Wi-Fi settings are kept.
An unknown/changed startup hook is preserved and reported as incomplete recovery.
If unresponsive, restart with recovery USB inserted; inspect the result after LOAD.
Removing USB alone does not uninstall. Never interrupt a normal installation.

PRIVACY
The install ZIP/USB may contain a password hash and derived Wi-Fi key. Keep it
private. Exported logs can identify your network/device; review before sharing.
