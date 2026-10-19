# Validation record

## Owner hardware test: 2026-10-19

Revision 5 completed installation, survived a new kernel boot, waited in standby and selected Standard after delayed manual power-on. The owner reported normal operation and success. The requested test removed USB before restarting; the returned logs themselves do not independently record USB absence.

| Uptime | Watcher event |
| --- | --- |
| 8.13 s | `START_REV5` |
| 9.01 s | Startup state, Smart Mode |
| 34.15 s | Standby, Smart Mode |
| 134.67 s | Awake detected, Smart Mode |
| 144.75 s | Applying Standard |
| 169.22 s | `STANDARD_THIS_WAKE` |

Action logs confirmed `MODE_BEFORE=3`, `MODE_AFTER=0`, command completion, `DOWN=0 COMBO=0 REPEAT=0`, `KEY_RELEASE_AND_COMBO_IDLE=1` and `CONSOLE_RESTORED=1`. The installed files and hook passed verification on export. No disabled marker was exported. The action finished 34.55 seconds after first logged awake detection, not necessarily after the exact button-press instant.

The first installation attempt stopped on `LONG_PRESS_NOT_CONFIRMED`; a later attempt succeeded. This is a real reliability caveat, not omitted from the record. Only one post-install awake period is demonstrated in this capture. Repeated cold boots, standby cycles, CEC/STR, other units/builds and long-term behavior remain open.

## Offline checks

Development validation comprised 86 host scenarios across controller (31), bounded runner (10), installation/removal (16), supervisor (16), upgrade (9) and draining active work (4). Sixteen descriptor regression cases included expected failures for the old prefix and successful continuation for revision 5, as well as refusal on inspection failure. Earlier extracted-ARM/QEMU IPC and descriptor checks supported the startup hypothesis.

The repository's portable suite recreates these scenarios without vendor binaries. It substitutes synthetic executable identities **only in temporary test copies**, models procfs/state/CLI behavior, and uses host BusyBox. Descriptor tests can additionally use an externally supplied extracted ARM BusyBox and QEMU. The release's device bytes are checked against the tested revision 5 SHA-256 baseline. Portable host tests do not substitute for the original ARM tests or real device testing.

Raw returned backups, process maps, full firmware and local analysis databases are not published. The local research archive retains them for follow-up; this document summarizes the relevant observations without exposing device-specific dumps.
