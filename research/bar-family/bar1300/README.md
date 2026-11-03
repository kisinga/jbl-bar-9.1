# Bar 1300 hardware work

Bar 1300 is the next target. The project owner reports a tester who owned the
device in 2023, with a conventional rather than capsule-shaped subwoofer. This
identifies the original generation from the reported timeline; exact 1300 versus
1300X, installed firmware and reachable interfaces remain unverified. The
[target record](target.json) preserves that distinction without inventing a
hardware result. Existing historical OTA1 findings describe the original family
and do not qualify the tester's firmware.

The first deliverable is a computer-hosted remote with verified state and a
small set of controls. Root access and an on-bar installer are separate work.

## Identify the device without developer tools

Ask the tester to open JBL One, select their soundbar and find **Product
Information**. JBL documents that screen as the place to read the current
[software version](https://support.jbl.com/gb/en/howto/jbl-one-app-updating-products-through-the-app-emea/000037146.html).
Copy the complete version string. For the exact model, copy the printed product
name/model code from the bar's label or packaging, including any `X` and `MK2`.
The app's user-editable device name alone is insufficient.

This is enough to start; the tester does not need SSH, a USB stick or a firmware
download. Send this small form:

```text
Model/model code from label:
Software version from JBL One > Product Information:
Country/region:
Connected by Wi-Fi or Ethernet:
```

JBL groups the original 1300 and 1300X in one set of
[release notes](https://support.harmanaudio.com/howto/bar-1300-1300x-software-update-release-notes-us/000043772.html),
and publishes a separate set for
[1300/1300X MK2](https://support.jbl.com/howto/bar-1300-1300x-mk2-software-update-release-notes-mgpqwr14hob-us/000052975.html).
That supports investigating overlap within each generation; it does not prove
identical hardware, firmware bytes or network commands. Record variant and
firmware as device facts. A variant does not automatically need another package,
backend or UI. These official identification/release references were checked on
2026-11-03.

## First session

Copy [session-template.json](session-template.json) to a new directory under
`research/private/bar-family/bar1300/<session-id>/`. Fill it from observations;
null means not observed. The template is a research record, not a runtime config
or a fixture accepted by `./jbl dev`.

1. Record the exact product/region label and firmware displayed by the JBL app,
   app version, network connection, and connected inputs/rears. Keep serial
   numbers, IP addresses and raw captures private.
2. Record a baseline using the stock app, display and physical remote: power,
   input, volume, mute and any visible sound-mode indicators. Mark which source
   supplied each observation. Distinguish standby from an unreachable device.
3. On that device, identify available discovery and read interfaces using the
   pinned [community and firmware references](../README.md). Validate response
   semantics against manual volume, mute and input changes. Record the actual
   request, response, timestamp and visible state; endpoint names and successful
   replies alone do not establish a getter. Do not assume a particular port,
   authentication policy or current-firmware command from historical images.
4. Once readback works, exercise one reversible control at a time at the
   tester's chosen listening level. Capture before, request result and fresh
   readback. Start with volume/mute; then qualify source and standby/wake. Record
   offline and timeout behavior. Never automatically retry an ambiguous toggle.
5. Test Smart Mode separately only after identifying a trustworthy observation
   of its state. An unverified getter or accepted toggle leaves the state
   unknown. Direct surround and rear connectivity are different features.

For every operation, distinguish unsupported, untested, unavailable and verified.
A missing response is not proof that a feature is unsupported. Record source and
volume conditions for behavior that depends on them. Capture wake/reconnect
results separately from normal operation.

## Collect read-only observations

On a computer on the tester's LAN, with Go and Python installed, use the bar's
private IPv4 address from their router's connected-device list. Substitute that
address for the example below:

```sh
./jbl inspect --host 192.168.1.50
```

This experimental developer command creates a new timestamped directory under
`research/private/bar-family/sessions/` and prints its path. It makes two fixed
SOAP getter requests on port 59152: `GetControlDeviceInfo` and `GetInfoEx`. It
does not scan, retry, send setters, download firmware or install anything. It
uses no vendor certificates or credentials. These getter shapes come from the
[pinned community coordinator](https://github.com/MrBearPresident/JBL_Soundbar/blob/9976c48755ad9d6725e281396c977a813cb9153b/custom_components/jbl_integration/coordinator.py),
not a qualification result for this device. The Go implementation is independent.

The session contains:

| File | Purpose |
| --- | --- |
| `report.json` | Timestamped request/response records, per-getter errors and explicitly returned model/hardware/firmware fields, when present |
| `remote-state.json` | Read-only UI fixture; directly reported volume/mute only, with power, input mapping and Smart Mode unknown |

Responses are bounded to 128 KiB each; captures retain at most that prefix.
Redirects are refused and each request has a five-second timeout. A getter can
fail while the other succeeds. Reports are saved in either case; a partial
failure exits nonzero. A timeout or missing field does not establish that the
model lacks the feature. In particular, the metadata getter might not expose
firmware, so the JBL One screen remains the simple fallback.

Keep the raw report private: it may include identifiers, names and media URIs.
Directories are created with mode 0700 and files with mode 0600. Existing
session directories are never overwritten. Use `--output-dir` to choose another
new directory under `research/private/`. No probe has yet run on this tester's
device; all automated checks use synthetic loopback responses.

## Turn observations into implementation

Normalize a single observed state using
[the example fixture](../fixtures/remote-state.example.json), keeping missing
values absent/null or `Unknown`. Record where each field came from in the
session record; a raw vendor reply is not automatically this public state shape.
Preview it locally:

```sh
./jbl dev --fixture research/private/bar-family/bar1300/<session-id>/remote-state.json
./jbl test
```

Fixture preview remains read-only and advertises no actions. Sanitized response
fixtures and evidence summaries can enter this directory once checked; raw
captures and credentials stay under `research/private/`.

Use these boundaries when adding the first runtime implementation:

| Piece | Starting point |
| --- | --- |
| Protocol and backend | `apps/remote/internal/jblone/` now contains the fixed read-only collector and protocol tests. Extend it with the `server.Backend[S]` adapter after actual readback is qualified; a separate abstraction for client and backend is unnecessary initially. |
| Executable | Add a small computer-hosted composition under `apps/remote/cmd/` when the backend works. The current `./jbl dev` remains a simulation/fixture preview. |
| UI | Reuse playback and system components for compatible observed fields. Add a compiled extension and bundled component for genuinely distinct controls. |
| Model facts | Keep identity, capabilities, tuning and device evidence specific to the tested variant/firmware. Use data for compatible variants; add a code branch only for an observed behavioral difference. Share the family backend with Bar 1000 as response semantics are confirmed. |
| Installation | Establish entry point, persistent storage, service startup and independent recovery before creating an executable model profile or package. |

These are intended locations, not empty packages to create in advance. Keep one
Go module and ordinary imports/compiled extension registration. The second real
model should reveal which protocol operations are shared; it does not need a
dynamic Go plugin loader or a universal hardware interface.

The first slice adds one family package and a small `cmd/inspect` composition.
Its normalized snapshot uses the existing preview server and browser shell. A
live network backend remains to be implemented and qualified. If it requires
copying HTTP/security/SSE code or the whole UI, revisit that boundary. Shared UI
changes for actual source choices, level ranges or unknown state are reasonable;
avoid checks such as `model === 'bar1300'` throughout components. See the
[extensibility check](../../../docs/COMPONENTS.md#the-first-real-extensibility-check)
for the current limits and expected changes.

The existing five-band front EQ and center-level UI have Bar 9.1-specific ranges
and behavior. Stock JBL app EQ does not establish compatibility with those
components. Historical product archives also select different 1000/1300 DSP and
ALSA configurations; do not reuse native offsets, coefficients or boot hooks.

## Completion criteria

Network-control support requires reproducible identity/state observations,
readback after each advertised action, bounded failure handling, reconnect and
standby/wake results, and regression fixtures for the tested firmware. Record
limitations per feature rather than claiming the whole model is supported.
Jailbreak/install support additionally requires its own installation and recovery
evidence. Neither milestone has been completed on the tester's device yet.
