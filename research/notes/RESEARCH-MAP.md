# Consolidated research map

This branch retains public summaries for continued work. The complete local dump is in the Git-ignored `research/private/` tree; firmware images, rootfs, Ghidra projects, tools and original captures were moved there and hash-verified. The migration inventory records every regular file and symlink. Compatibility symlinks keep previous absolute paths working.

## Most useful archived material

| Topic | Local path beneath `research/private/` | Why keep it |
| --- | --- | --- |
| Confirmed cold-boot success | `firmware-workspace/analysis/reversible-smart-mode/runtime-socket-retry-1/` | Original checksummed logs and assessment |
| Earlier LOAD incident and recovery | `firmware-workspace/analysis/reversible-smart-mode/runtime-wake-rollback-1/` | Confirms rollback, waiting-only watcher and recovery |
| Revision 4 early exit | `firmware-workspace/analysis/reversible-smart-mode/runtime-detached-retry-1/` | Intact install but absent watcher logs |
| IPC/shell startup analysis | `firmware-workspace/analysis/reversible-smart-mode/startup-hang/` | REA exports, library/daemon identities, modeled socket-lifetime fixtures |
| Original development automation | `firmware-workspace/analysis/reversible-smart-mode/scripts/` | REA helpers and historical tests; portable release tests are in `tests/` |
| Firmware and previous releases | `firmware-workspace/` | Downloaded vendor packages, clean candidate ZIPs and chronological status |
| Earlier spike | `spikes-jbl-firmware/` | Rootfs, Ghidra databases/scripts, disassembly and other investigated routes |
| Extra exploratory note | `other-notes/XOR_ANALYSIS.md` | Earlier XOR/container analysis, not an implemented release technique |

Earlier exploit, Bluetooth and signature-bypass ideas in the archive are exploratory records, **not prerequisites or claims of working techniques**. They are not part of the USB release. The relevant solution uses the published AutoScript mechanism plus the build-specific controller and startup fixes.

## Verified-build field map

These decimal process addresses are meaningful only for the exact `bdpprog` SHA-256 in the compatibility guide. Widths and meanings reflect source checks and analysis of that build. Do not transfer them to other firmware.

| Address | Bytes | Used as |
| --- | --- | --- |
| 12962676 | 4 | System state; awake predicate requires 0 |
| 13378532 | 4 | Sound mode: Standard 0, Smart 3 |
| 14191893 | 1 | Power predicate requires 1 |
| 13822200 | 4 | Input adapter type |
| 13822224 | 1 | Key-down state |
| 14200904 | 4 | Combination state |
| 14200898 | 1 | Repeat / long-press indication |
| 15427780 | 1 | Console input setting, temporarily changed/restored |
| 15427784 | 4 | Console gate setting, temporarily changed/restored |

The key route is `mw.iom.se`, with mute event `0x6f002` and volume-up `0x6f000`. Success requires observed mode readback and released input, not just a successful CLI write. Source in `usb/JBLInstall/payload/` is authoritative for all guard conditions and restoration order.

## Remaining questions

- Can faster scheduling retain normal LOAD and control behavior on hardware?
- How reliably are remote/CEC/STR wake paths observed, including very short standby?
- Does more frequent state sampling materially affect device load?
- What caused the one transient installation long-press failure?
- Which other exact firmware identities can be supported after analysis and testing?

Keep the confirmed release available while investigating these questions. Do not replace verified evidence with local-model suggestions or infer success from a passing host fixture alone.
