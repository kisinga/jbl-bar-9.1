JBL BAR 9.1 — AUTOMATIC STANDARD MODE, v0.2.0-dev.1 (device revision 6)
EXPERIMENTAL — wake timing tested on one bar. Published v0.1.0 is unchanged.
Full guide: https://github.com/kisinga/jbl-bar-9.1

REQUIREMENTS
JBL Bar 9.1, supported executable build (recorded label 23.06.41.80),
working FAT32 USB, computer to unzip/copy files, remote. No network or tools.
Executable hashes are checked automatically; other builds are refused.

INSTALL ZIP
Extract contents directly to the USB root, so AutoScript and JBLInstall are
root folders. Use a clean USB without another AutoScript project.
Fully power on the bar, insert USB and wait 2 minutes without touching buttons.
SMART MODE OFF appears if Smart was on. Check JBLInstall/INSTALL_STATUS.txt on
your computer: INSTALLED_PENDING_REBOOT_TEST or ALREADY_INSTALLED_LOGS_EXPORTED
means success. Any other status: read the online troubleshooting guide.
An initial long-press failure was observed once; a later insertion succeeded.

Remove USB, disconnect mains 60 seconds, reconnect and let LOAD finish.
Power on whenever you want; allow 60 seconds for Smart Mode to switch off.
The helper waits indefinitely in standby. No two-minute standby wait is needed.
Revision6 measured about 18 seconds after the bar was observed awake.
Revision5 measured about 35 seconds. All key-release checks remain unchanged.
Check audio, volume, mute and standby. CEC/cold-boot coverage remains limited.
An unexplained power-off/reboot occurred during testing; see the online record.
If already Standard, no action/message is needed. USB need not stay attached.

LOGS
Reinsert this install USB for at least 15 seconds to export logs to
JBLInstall/runtime. Check watch.log, latest.log and STATUS.txt.
Reinsertion of this exact installed version only exports logs.

REMOVAL
See ROLLBACK.txt and keep the uninstall ZIP available. Removing USB does not
uninstall. This installs /var/JBLStandard and /var/AutoScript.TSS and temporarily
changes command-console fields in RAM before restoring them. No firmware flash.
Do not bypass identity checks or copy this to another soundbar model.
