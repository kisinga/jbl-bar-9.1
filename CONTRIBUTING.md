# Development

Work from the repository root. Support belongs to a model and a qualified firmware
profile. The current production control implementation is Bar 9.1; the original
800/1000/1300 family remains research.

See the [brief migration notes](docs/MIGRATION.md) for this breaking source reorganization.

## Build and test

Use Python 3.10+, BusyBox at `/usr/bin/busybox`, and Go 1.22+ on Linux.

Start with `./jbl dev` for a loopback preview and `./jbl test` for fast Go tests.
Neither needs a soundbar, credentials, BusyBox or firmware. The full host suite
below needs BusyBox; `./jbl test --full` runs it alongside race checks and vet.
Use `./jbl build --model bar91` to build a public package without owner credentials.

```sh
python3 tools/test.py
(cd apps/remote && go test -race ./... && go vet ./...)
python3 research/bar-family/test_workspace.py
```

The host runner discovers `tests/check_*.py` and model suites below
`tests/<model>/`. It checks the reviewed profile and pinned release bytes, builds
the ARM remote in the USB integration suite, and exercises installation,
configuration and recovery using temporary host fixtures. No soundbar is needed.
These checks do not establish device installation, sound or wake behavior.

The runner reports counts by suite and checks required reports are present and
nonempty. Adding a regression case does not require changing count assertions.
Historical payload identities remain pinned independently. The research
archive boundary suite adds six checks. Browser fixtures use Playwright:

```sh
node tests/webremote-ui.mjs
node tests/component-ui.mjs
node tests/setup-ui.mjs
```

Use `PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs` to reuse an
installation. Browser setup and detailed behavior contracts are in the
[Bar 9.1 development guide](docs/models/bar91/DEVELOPMENT.md#state-and-interface).

## Ownership and layout

Read [the architecture](docs/ARCHITECTURE.md) before moving controls between
models or changing installation ownership. Current source locations are listed
in [the root README](README.md#repository).

- `models/bar91/` owns 9.1 installation scripts, native helpers and qualification.
- `apps/remote/` owns the Go application and embedded UI; its native Go code is
  still explicitly pinned to the Bar 9.1 profile.
- `tools/` keeps stable command names and resolves source locations through
  `repo_layout.py`.
- `research/` keeps experimental work separate from executable eligibility.

Keep credentials, private packages, firmware, full captures and tool caches in
ignored directories. Preserve modified working files and unknown device hooks.

## Firmware profiles

`models/bar91/profile.json` supplies executable identities, helper-library hashes
and the target architecture. The runtime embeds generated Go identities:

```sh
python3 tools/check_profiles.py
# After a reviewed profile/code change:
python3 tools/check_profiles.py --write
```

The generator refuses disagreement with the pinned standalone shell payloads,
combined USB preflight and native HDSP guard. It never regenerates historical
shell payloads. Editing metadata does not qualify a new firmware: offsets, ABI,
state readback, action restoration and device recovery still require evidence.

## Model development

[Bar 9.1 development](docs/models/bar91/DEVELOPMENT.md) covers all existing build
commands, optional Bluetooth/EQ helpers, automation ownership and native control
constraints. The [combined USB guide](docs/models/bar91/USB-TOOLKIT.md) remains the
installation/configuration/recovery contract. Keep archive paths and device paths
stable when moving repository files.

`./jbl` is the host entry point. `python3 tools/build_distribution.py` builds the
public ARM USB and recovery ZIPs with offline setup. See
[releasing](docs/RELEASING.md) before changing artifact names or qualification.
Optional runtime features use `apps/remote/internal/extension`; the model
composition registers their typed actions inside the existing HTTP protections.
The [component guide](docs/COMPONENTS.md) covers backend contracts, feature workers,
UI ownership and normalized research replay. Keep mechanical moves separate from
behavior changes; preserve contributed regression tests and existing entry points.
The reorganized runtime includes upstream through `3a52f58`: center-wake (#7),
EQ/power (#8), center labels (#9), and advanced controls (#5), with contributed
regression tests retained. Advanced native source/tests remain byte-identical to
upstream; their UI lives in the existing components. Native qualification notes
are in [Bar 9.1 development](docs/models/bar91/DEVELOPMENT.md#advanced-controls).

For another model, start with [the model registry](models/README.md) and the
[family investigation](research/bar-family/README.md). Qualify request semantics,
unknown/readback handling, firmware eligibility and recovery before exposing
controls. A surround mixer enum is not automatically a Smart Mode setting; app EQ
is not automatically equivalent to the 9.1 front-only DSP path.
