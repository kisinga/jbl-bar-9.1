# Research workspace

I keep the public implementation, reproducible tests and concise evidence in the root [README](../README.md). The working branch also carries research notes for continued latency, SSH and browser-control work.

The original project materials have been consolidated into these **local-only, Git-ignored** directories:

| Directory | Contents |
| --- | --- |
| `private/firmware-workspace/` | Original firmware downloads, release candidates, REA exports, scripts, hardware captures and investigation reports |
| `private/spikes-jbl-firmware/` | Earlier firmware spike: extracted rootfs, Ghidra projects/scripts, binary analysis and tool installations |
| `private/local-ssh/` | Original Wi-Fi/SSH experiments, hardware outcomes, private configuration and boot rollback evidence |
| `private/migration.json` | Per-file SHA-256/size or symlink target, plus verified migration records |

Compatibility symlinks at the original locations preserve older scripts and analysis database paths. The original trees were moved, with file hashes checked before and after; they are not duplicate copies. These directories are not present in a public clone, Git history or release assets. Proprietary firmware, raw device dumps, local paths and large tool caches are deliberately kept local.

Useful entry points within the local firmware workspace:

- `INVESTIGATION-STATUS.txt`: chronological findings and hardware outcomes.
- `COMMUNITY-PRIOR-WORK.txt`: bounded prior-art search and source links.
- `analysis/reversible-smart-mode/runtime-socket-retry-1/ASSESSMENT.txt`: successful hardware evidence.
- `analysis/reversible-smart-mode/socket-retry/`: revision 5 implementation, validation and staging records.
- `analysis/reversible-smart-mode/startup-hang/`: IPC analysis, ARM-shell regressions and modeled socket-lifetime checks.
- `analysis/reversible-smart-mode/scripts/`: original development tests and REA automation.

Do not upload this private archive as a community support attachment. Use reviewed summaries and narrowly relevant text logs instead.

## Why closing sockets matters

An earlier resident watcher coincided with prolonged LOAD. Native analysis found differing client/daemon IPC transfer sizes (6168 versus 4220 bytes) and a shell child capable of retaining the accepted socket. A modeled AF_UNIX exchange with the extracted ARM shell reproduced a caller waiting while the child held that socket. Closing the inherited socket released the modeled caller. The exact live client-library hash was not captured, so the full native causal chain is not proven solely by these observations.

Revision 4 closed sockets but could exit before starting the watcher. Enumerating `/proc/$$/fd/*` can include the shell's own temporary directory descriptor. After low sockets close, that entry may have disappeared by the time `readlink` examines it. Revision 5 skips absent entries, while refusing to continue if an existing descriptor cannot be inspected. It preserves regular file descriptors, including the open script. This bug was reproduced under the extracted ARM shell; revision 5 subsequently booted and performed the action on hardware.


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

These decimal process addresses are meaningful only for the exact `bdpprog` SHA-256 in the [root compatibility section](../README.md#compatibility). Widths and meanings reflect source checks and analysis of that build. Do not transfer them to other firmware.

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

## SSH findings I reproduced

On 20 October 2026 I confirmed a fresh USB-free reboot followed by root password login, an interactive shell and SFTP. Wi-Fi reconnected, the Smart Mode watcher restarted and I confirmed normal power/remote behavior. The public portable installer is derived from this installation; independent clean-install evidence remains a separate requirement.

- The native OpenSSH tools need the vendor glibc 2.18 loader and libraries; the default runtime includes glibc 2.27. A library-path change alone did not fix loader incompatibility.
- A private mount namespace supplies account/NSS lookup and the compatible loader without overwriting the system view. Host-key generation must happen inside that view too.
- A session wrapper selects the compatible environment for commands, interactive shells and SFTP. Successful `sshd -t` alone did not prove these paths worked.
- A background startup hook closes inherited sockets and waits a bounded time for Wi-Fi. It preserves the Smart Mode command and has a matching rollback that preserves unknown later hook changes.
- I measured ARM32 userspace, Linux 4.9.134, about 448 MiB RAM and 25 MiB free writable storage. Android-derived services coexist with Chromecast; I found no package manager/framework for normal APK apps.

The local-only `local-ssh/README.txt` is chronological: later outcomes supersede earlier “unconfirmed” entries. It and the raw attempts contain personal configuration; publish reviewed findings and templates rather than those files.

## Remaining questions

- Can faster scheduling retain normal LOAD and control behavior on hardware?
- How reliably are remote/CEC/STR wake paths observed, including very short standby?
- Does more frequent state sampling materially affect device load?
- What caused the one transient installation long-press failure?
- Which other exact firmware identities can be supported after analysis and testing?

Keep the confirmed release available while investigating these questions. Do not replace verified evidence with local-model suggestions or infer success from a passing host fixture alone.
