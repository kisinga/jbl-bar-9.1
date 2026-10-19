# Troubleshooting

## No message after insertion

Check the USB on your computer. `AutoScript` and `JBLInstall` must be at the root, not inside the ZIP name or repository directory. The drive must be readable as FAT32 by the bar. `JBLInstall/ROLLBACK` must be absent for installation. If no new `install.log` or `update.log` appears, the launcher may not have run: investigate USB layout/detection first.

Wait until the bar is fully on before inserting. Leave remote/buttons alone during the two-minute installation. Standard Mode already selected can produce no display message.

| File / result | Meaning / next step |
| --- | --- |
| `INSTALL_STATUS.txt`: `INSTALLED_PENDING_REBOOT_TEST` | Installed; now test a reboot without USB. |
| `ALREADY_INSTALLED_LOGS_EXPORTED` | Exact installation verified; logs copied to `JBLInstall/runtime`. No key sequence replayed by reinsertion. |
| `TRIAL_NOT_SUCCESSFUL_NO_INSTALL` | The trial did not qualify, so a new helper was not installed. Read `trial/STATUS.txt` and `trial/latest.log`. |
| `STOPPED_LONG_PRESS_NOT_CONFIRMED` | The key handler did not confirm the long press. This occurred once before a successful retry on the test device. With controls normal, allow the bar to settle, remove/reinsert USB and wait two minutes without touching remote/buttons. Do not loop retries if it keeps failing. |
| `FIRMWARE_MISMATCH`, `CLI_MISMATCH`, or variants ending `_NO_CHANGE` | Unsupported executable. Stop; keep identity guards intact. |
| `EXISTING_AUTOSCRIPT_PRESERVED_NO_INSTALL` / `INSTALLED_VERSION_CONFLICT` | Another hook/version exists. Do not overwrite it blindly. |
| `runtime/STATUS.txt`: `STANDARD_SELECTED` | The action confirmed Standard Mode. Check the boot ID in `latest.log`. |
| `runtime/WATCH_STATUS.txt`: `STANDARD_THIS_WAKE` | The watcher handled this wake successfully. |
| `runtime/DISABLED` | A guard stopped the helper after a failure. Collect logs and uninstall before further experimentation. Reinsertion only exports logs; it does not clear this marker. |

`UPDATE_STATUS.txt=SETUP_FINISHED` only means the dispatcher completed that step. Inspect `install.log`'s `EXIT=` and `INSTALL_STATUS.txt` for success. An upgrade can leave `UNINSTALL_STATUS.txt=UNINSTALLED` from removing the previous revision while the new revision is installed; use the newest installation log too.

## No message after reboot / wake

Let LOAD clear, turn the bar fully on, play audio and allow 60 seconds. The helper waits indefinitely in standby; there is no two-minute deadline. The confirmed release needed roughly 35 seconds after detecting an awake bar. No message is needed if already Standard.

Reinsert the **same install USB** for at least 15 seconds to export logs, then read:

- `JBLInstall/runtime/watch.log`: `START_REV5`, waiting/awake transitions, `APPLYING_STANDARD`, `STANDARD_THIS_WAKE`.
- `JBLInstall/runtime/latest.log`: action result and boot ID.
- `JBLInstall/runtime/last-attempt/result.txt`: temporary console restoration.
- `JBLInstall/runtime/last-attempt/JBLTest/result.txt`: mode before/after and key release.

The bar's wall clock may be wrong. Compare boot IDs and uptime. Older revision numbers in unchanged component headers are expected; the watcher revision and payload checksums identify the complete package. Reinsertion does not reinstall or toggle an already installed package.

## Remove or recover

Use the release's uninstall ZIP on a clean FAT32 USB, or add `JBLInstall/ROLLBACK` (empty file, **no `.txt` extension**) to its install USB. On Windows enable “File name extensions” in Explorer before creating/checking it. On macOS ensure the editor does not silently append `.txt`.

Insert in the powered bar and wait 100 seconds. Confirm `UNINSTALL_STATUS.txt` reads `UNINSTALLED` or `ALREADY_UNINSTALLED`, remove USB, then restart normally. The uninstaller disables new work, removes only the recognized hook, waits for active work to finish, and copies installed files to `JBLInstall/uninstalled-backup` before removing them.

If `HOOK_DISABLED_REBOOT_TO_FINISH_UNINSTALL` appears, reboot with the rollback USB connected; once fully on, remove/reinsert it and wait 100 seconds, then check the status again. Unknown/modified hooks are preserved rather than deleted.

If LOAD lasts unusually long or buttons stop responding, stop the test. Prepare the rollback USB on your computer. If the bar can accept USB while powered, insert it and allow removal to complete. If controls remain unresponsive, disconnect mains and reconnect with the rollback USB inserted, then check removal logs. Removing the USB alone does not undo installation. Do not interrupt power during a normal install/uninstall. If rollback does not restore normal boot, stop repeated experiments and seek device-specific recovery help; factory reset and reflashing are not steps supplied by this project.

## Reporting results

Attach only the relevant text statuses and logs, after reviewing them for personal information. Include exact model, displayed firmware (or unknown), whether LOAD/controls were normal, time to the message, whether USB was absent at boot, and whether the test was mains restart, remote standby or CEC wake. Full dumps and Wi-Fi settings are unnecessary.
