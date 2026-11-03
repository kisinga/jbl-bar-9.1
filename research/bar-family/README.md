# JBL One-family investigation

This extends the toolkit beyond its Bar 9.1 implementation through firmware
evidence and controlled offline replay. It does not qualify another installer.
The initial targets are the original Bar 800, 1000 and 1300; MK2 is separate.

**Next hardware target: original-generation Bar 1300.** The project owner reports
access to a tester's device owned in 2023. Exact 1300/1300X variant and installed
firmware still need identification; no live results are claimed. The
[Bar 1300 test plan](bar1300/README.md) connects this
family research to the shared remote and defines the first evidence to collect.
`./jbl inspect --host <private-bar-ip>` now collects two fixed community-observed
getters and a conservative fixture; the live family backend remains unimplemented.

## Folder ownership

Keep reusable investigation code and reviewed findings here. Keep vendor bytes,
decompilation, full logs and acquired third-party sources under Git-ignored
`research/private/bar-family/`:

```text
research/bar-family/                      # tracked tools, findings, identities
research/private/bar-family/
  downloads/                              # original archives + acquisition records
  firmware/<models>/<version>/
    package/                              # checksum, signed update ZIP, release notes
    images/                               # system/recovery/boot and companion images
    rootfs/                               # extracted system image, never host root
    evidence/                             # extraction log and checked identities
  evidence/                               # cross-version research and REA/replay results
  tools/                                  # private analysis dependencies
```

The older `private/firmware-workspace`, `private/spikes-jbl-firmware` and
`private/local-ssh` retain the established 9.1 evidence. Do not reshuffle those
archives or move production controls into this research directory. The local
`shared-ota1` compatibility symlink resolves to the versioned firmware directory.

## Acquired targets

Exact identities and official pointers are in [artifacts.json](artifacts.json).
Both retrieved archives passed ZIP CRC checks and the packaged update ZIP's MD5.
The detached signatures were retained but not cryptographically verified.

| Target | Version | Result |
| --- | --- | --- |
| 800/1000/1300 shared OTA1 | 23.16.21.80.00 | Downloaded, extracted, decompiled; selected code replayed |
| Earlier 800/1000 | 22.34.55.80.00 | Downloaded, extracted; identities and command anchors compared |
| Separate 1300 RTS | 22.48.53.80.00 | Official pointer saved; payload unavailable |

JBL's live historical download links returned 404. The payloads came from
archived vendor CDN responses. The shared archive was located through
[fj016's firsthand research](https://fj016.fr/blog/exploring-iot-jbl-wifi-speakers/).
These historical images do not establish behavior on current firmware or 1300X.

## Findings that affect our implementation

| Area | Evidence | Consequence for this repo |
| --- | --- | --- |
| Control stack | ARM32 Linux, `setup-wamp`, `audio-ui`, `dsp-client`, WAMP and an app HTTP dispatcher | Add a separate transport/profile if qualified; existing `bdpprog` memory offsets do not apply |
| App controller | Native `OnSendAppController` accepts a fixed key map; it lacks `surround` in OTA1 | Modern client commands require version/capability validation |
| Smart Mode key | `Key_AudioEffectiveMode` toggles its state field between 0 and 1 | An accepted key request is not an idempotent desired-state setter |
| Mode publication | Native helper requests fade, 200 ms sleep, GPIO mute value 1200, then publishes `com.harman.audio.audmodeChanged` | Avoid bypassing this path with an assumed DSP-only equivalent |
| Mode reset | `restore_non_save_audio_setting` assigns enabled state and publishes mode 1 | Reset behavior is explicit; callers/wake timing still need qualification |
| DSP subscriber | Mode 1 selects source-dependent automatic EQ; Standard selects EQ 0 and a DAP-disable path | Smart Mode is distinct from the direct surround mixer RPC |
| Direct surround | Setter accepts 0/1 but ignores the mixer-command result; getter reads `HM SURROUND MODE` | Neither a true reply nor this getter verifies Smart Mode |
| Readback | Exact `getSmartMode` absent in both historical rootfs trees; OTA1 `OnGetSmartSoundState` is a constant-return stub | Preserve unknown state; this is not a verified getter |
| EQ | `HandleSetEQ` handles structured payloads and forwards toward `com.harman.setup.setEQ`; its GET-style setter path rejects the request | Trace the full pipeline and bounds before exposing writes |
| Model configuration | Product IDs select different model archives; 1000 and 1300 have different ALSA/DSP routing and coefficients | Shared firmware does not mean shared tuning or channel/register mapping |
| Persistence | `/data` persists; `/var` is tmpfs | The 9.1 `/var/AutoScript.TSS` installation design needs independent replacement |
| USB | Signed updater and mount handling exist | No arbitrary AutoScript or on-bar installation route established |

The exported name `Key_SmartTrigger` is a Moments/music action, and
`Key_setSurroundOnOFF` handles wireless rear connection state. Neither name alone
identifies the Smart Mode toggle. This distinction avoids two misleading paths.

The two releases have different `audio-ui` and `rootApp` hashes. All native
addresses below apply only to the pinned shared OTA1 binary.

## Community work and improvements to carry forward

Our existing CLI reference is still
[k1rnt/jbl-soundbar-cli at 7d10bdb](https://github.com/k1rnt/jbl-soundbar-cli/tree/7d10bdb1ebe0c5a77c9a4c14ebe4580bd3735309).
It lists Bar 1000 and 1300. Reuse discovery/control concepts through our own
implementation, retaining our state guards, error handling and readback policy.
Its `getSurroundMode`/`setSurroundMode` JSON assumptions are not validated by this
firmware investigation. A Cargo MIT declaration was found, but no root license
file was present in the inspected revision; implementation code was not vendored.

The [Home Assistant integration at 9976c48](https://github.com/MrBearPresident/JBL_Soundbar/tree/9976c48755ad9d6725e281396c977a813cb9153b)
provides a useful second reference with an MIT license. Its current Smart Mode
switch queries `getSmartMode` but both on/off methods send the same `surround`
toggle. The original [Smart Mode PR](https://github.com/MrBearPresident/JBL_Soundbar/pull/6)
was tested on Bar 1000. This does not validate the same command on OTA1.

For a future family backend, carry forward this repo's contract: read fresh
state, compare, mutate once, and verify. Unknown state stays unknown. Do not
replay a toggle after an ambiguous timeout. Detect supported commands rather
than treating version strings, fallback defaults or JSON acceptance as proof.
Standard app EQ is also separate from our 9.1 front-only EQ and center gain.

## Reproduce locally

Supply your own downloaded archive. Extraction creates a new directory, rejects
unknown/traversing/duplicate members, checks integrity, and runs debugfs in a
network-isolated bubblewrap namespace. It never mounts a filesystem or boots
firmware. Ownership warnings are expected; inspect all other diagnostics.

```sh
python3 research/bar-family/firmware_workspace.py \
  research/private/bar-family/downloads/JBL_Bar_800_1000_1300_OTA1_ver_23.16.21.80.00.zip \
  --output research/private/bar-family/firmware/800-1000-1300/23.16.21.80.00 \
  --extract-filesystem
python3 research/bar-family/test_workspace.py
```

Use a fresh output path if the version is already extracted. Device-absolute
symlinks into `/data` are inventory, not host paths: never dereference them on
the host. Model DSP libraries are inside the model tarballs, not necessarily at
the symlink target in the uninitialized system root.

Replay the pinned vendor product-loader source under host bash, with synthetic
product archives, fake procfs and isolated writable data:

```sh
python3 research/bar-family/replay_product_loader.py \
  research/private/bar-family/firmware/800-1000-1300/23.16.21.80.00/rootfs \
  --output research/private/bar-family/evidence/product-loader-replay-new
```

The seven cases cover 1000, 1300, lowercase ID, 800, unknown fallback, cached
configuration, and a separate-image boot. This proves the selected script's
behavior in the fixture, not that a soundbar boot reaches it.

Replay the original ARM instructions using `unicorn==2.1.4` in your chosen Python
environment:

```sh
python3 research/bar-family/replay_audio_mode.py \
  research/private/bar-family/firmware/800-1000-1300/23.16.21.80.00/rootfs/usr/bin/audio-ui \
  --dsp-binary research/private/bar-family/firmware/800-1000-1300/23.16.21.80.00/rootfs/usr/bin/dsp-client \
  --output research/private/bar-family/evidence/audio-mode-replay-new.json
```

Nine native cases passed: two toggle-prefix, two publisher, four DSP subscriber
combinations (Smart/Standard and local/nonlocal source), and one wrong-type
MessagePack rejection. The toggle stops after its native state write; the
publisher and valid subscriber cases run through return. C++ construction,
logging, sleep, EQ/DAP operations and external services are explicit stubs.
This does not validate real WAMP delivery, audible DSP effect, wake timing or
persistence. Omit `--dsp-binary` for the four audio-ui cases alone.

For repeatable decompilation, [rea_batch.py](rea_batch.py) accepts a JSON plan of
`{name, arguments, label}` operations, an explicit local `--rea` executable,
`--ghidra` installation and a new `--output` directory. It retains one analysis
session across queries, saves full results privately, restricts operation names
to offline tools, and closes its session. It does not edit client configuration.

## Evidence ledger and remaining work

Full records remain in `research/private/bar-family/evidence/`:

| Evidence | Private record | Scope |
| --- | --- | --- |
| Archive inventory | `archive-rea.json`, inspection `ev_4d1d8da749f294e3c5d081de5f84a0e9515ab9d107eba120e155b6a2a66160ff` | ZIP metadata and content identity |
| App key map | `setup-wamp-rea/function-0xb87c4.json`, `ev_73dcf35647ffdbdbf10041d0559271747b29c1ce8adb20f09bc5454937dc7c08` | Native decompilation |
| Getter stub | `setup-wamp-rea/function-0xa3a4c.json`, `ev_a5acf26d74256f14d6b69e2b99396dc0ad2adb7ebbd513054475ebbdcc20fb44` | Native constant return |
| Toggle | `audio-ui-mode-rea/func-0xccd7c.json`, `ev_3f09ebbb4665548938865c91076fc6a66e0c5b9db308f2854224f86da7471f6d` | Native decompilation; prefix replay |
| Mode publication | `audio-ui-mode-rea/func-0xcc8d4.json`, `ev_bedb2e5693d2b49a4a6a37d92dad5569d17ef7e8e7b1c597181d5ad4347f27ad` | Native decompilation; stubbed replay |
| Reset | `audio-ui-smart-rea/func-0x153f58.json`, `ev_52e70837d991e8b9dd265f87477cd93f447f865e70b7a25c1df38122a09a0cd2` | Static reset path; callers unresolved |
| DSP subscriber | `dsp-mode-rea/func-0xb2fd4.json`, `ev_e330fd9a9d74b5b8f32695a3c16cc7d951797451c8434c865feb517721ccfefb` | Native decompilation and stubbed replay |
| Surround separation | `dsp-mode.md`, `dsp-mode-bundle.json` | Mixer setter/getter differ from Smart Mode |
| Model/startup | `startup-inventory.md`, `product-loader-replay-v3/results.json` | Hashed files and controlled shell replay |
| Native replay | `audio-mode-and-dsp-replay.json` | Nine bounded ARM cases with explicit stubs |
| Version comparison | `earlier-comparison.md` | Two historical system images |
| Community sources | `protocol-reference.md`, `source-research.json` | Pinned sources and acquisition boundaries |

Still required before another model is supported: resolve HTTP dispatch and
authentication on the target firmware; verify real state readback; qualify model
tuning and trace reset callers; qualify the external transport
on hardware; establish installation/recovery if running on-bar. Native replay
and static evidence do not substitute for those results.

## Remote integration during research

Use `./jbl dev --fixture research/bar-family/fixtures/remote-state.example.json`
to replay normalized observations read-only through the shared server and UI.
The checked-in example intentionally contains Unknown/null values and is not a
hardware result. Raw vendor responses stay under `research/private/`; normalize
only supported observations. A future family backend can implement the shared
`server.Backend[S]` contract without importing the Bar 9.1 native app or enabling
its USB installer. See [components](../../docs/COMPONENTS.md) for feature and UI
contracts, local testing and capability semantics.
