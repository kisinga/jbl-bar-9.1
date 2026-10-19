# Research workspace

The public repository contains the implementation, reproducible tests and a concise evidence record in `docs/`. The working branch also carries research notes for continued latency and wake testing.

The original project materials have been consolidated into these **local-only, Git-ignored** directories:

| Directory | Contents |
| --- | --- |
| `private/firmware-workspace/` | Original firmware downloads, release candidates, REA exports, scripts, hardware captures and investigation reports |
| `private/spikes-jbl-firmware/` | Earlier firmware spike: extracted rootfs, Ghidra projects/scripts, binary analysis and tool installations |
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
