# Model status

Model directories contain executable implementation and its reviewed profile.
Research findings live under `research/`; they do not create supported profiles.

`./jbl models` reads [catalog.json](catalog.json) to expose this status at the
entry point. The catalog is descriptive: editing it cannot authorize another
model's installer or select the 9.1 memory layout.

| Target | Implementation | Qualification |
| --- | --- | --- |
| [Bar 9.1](bar91/README.md) | [Profile](bar91/profile.json), single-USB coordinator, SSH, standalone Smart Mode, remote lifecycle and native helpers | Individual features have one-device evidence; combined USB flow is host-tested and awaits complete device qualification |
| Original Bar 800/1000/1300 | [Family research](../research/bar-family/README.md) | Historical artifacts, static traces and controlled replay; no qualified installer or runtime backend |
| MK2 and other models | No implementation | Independently identify firmware and qualify behavior |

Bar 1300 is the next hardware target. Reported ownership in 2023 identifies the
original generation; exact 1300/1300X variant, installed firmware and runtime
behavior remain unverified. The read-only collector is available for research.
Follow the [Bar 1300 test plan](../research/bar-family/bar1300/README.md).
Bar 800/1000 continue to share family research without inheriting qualification.

## Adding a model

Record hardware identity, exact firmware hashes, storage/startup assumptions,
request semantics and recovery requirements. Keep unsupported capabilities
unavailable, and represent unreadable state as unknown. Validation must identify
whether evidence came from source, shipped firmware, controlled replay or a
physical device.

Create an installation package only after establishing that model's entry point,
persistence and recovery. Shared UI or network controls can be useful before an
on-bar installation route exists. A new profile must not reuse another model's
memory offsets, DSP coefficients, amplifier registers or boot hook by assumption.
