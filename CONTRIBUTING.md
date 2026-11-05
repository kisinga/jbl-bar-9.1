# Development

The [root README](README.md) is the setup guide. Repository setup builds current
source; downloaded platform executables use published packages without Go or
installation. Bar 9.1 is the supported implementation; other models remain research.

Both launch modes offer USB setup and **Network update (SSH)**. Network updates
resolve `jbl.local` with native multicast DNS or accept a private IPv4 address;
Go handles SSH directly. Passwords are request-only, and verified public host keys
are saved in the user configuration directory. The same model updater runs from
a private RAM staging directory, with managed-installation and firmware guards.

## Requirements and commands

Use Go 1.25.7 or newer and a browser for local setup and public builds:

```sh
go run ./apps/usbsetup --repo .
go run ./apps/build
```

The shared Go builder produces ARM USB/recovery ZIPs, six platform executables
and offline bundles under `dist/`. Local setup retains its matching files under
`build/setup-payload-…/`. Restart setup to include source edits.

Contributor commands under `./jbl` require Python 3.10+. The independent host
fixtures also need BusyBox; Linux FAT validation uses `fsck.fat` when available.
Browser tests require Node, Playwright and Chromium. Set `PLAYWRIGHT_MODULE` to
reuse an existing installation. None of these checks writes to attached USBs.

```sh
go test -race ./apps/build ./apps/usbsetup ./internal/buildtool ./internal/usbsetup ./packaging
go vet ./apps/build ./apps/usbsetup ./internal/buildtool ./internal/usbsetup ./packaging
(cd apps/remote && go test -race ./... && go vet ./...)
python3 tools/test.py
node tests/webremote-ui.mjs
node tests/component-ui.mjs
node tests/bass-offset-ui.mjs
node tests/setup-ui.mjs
node tests/setup-local.mjs
```

`./jbl dev` previews the remote with sample data; `./jbl test --full` runs the
Go and host checks. The Python host runner discovers `tests/check_*.py` and
model fixtures, writing reports to ignored `build/test-results/`. It independently
compares Go-built payloads with the legacy builder. Setup tests use temporary
folders, sparse disk images and mocked native commands. Browser fixtures check
production assets, capabilities, drafts, disclosure defaults and device states.
These checks do not qualify physical installation, sound, standby or recovery.
SSH fixtures use a loopback server with real authentication and encrypted channels;
device transaction fixtures test setting/key retention and failure handling without a bar.

## Screenshots

`node tests/webremote-ui.mjs` generates deterministic production UI captures:

| Generated image | GitHub image | Contents |
| --- | --- | --- |
| `build/readme-remote.png` | `docs/web-remote.png` | 1200px full page; Audio sync, Bass offset and System monitor open |
| `build/readme-advanced.png` | `docs/web-remote-advanced.png` | Both Sound columns and expanded Room setup |

Review readability, complete controls and sample states, then replace the two
GitHub images. Never capture private device data. The compact [UI feature map](docs/REMOTE-UI.md)
and frozen `tests/fixtures/remote-ui-baseline.json` preserve semantic coverage.

## Ownership and qualification

The root Go module owns setup/build tooling; `apps/remote` is a separate Go
module for the device application. `models/bar91` owns installation, recovery,
native helpers and qualification. Keep research outside executable eligibility.
See [architecture](docs/ARCHITECTURE.md), [components](docs/COMPONENTS.md),
[model status](models/README.md) and [releasing](docs/RELEASING.md).

Default packages retain Front EQ and Bluetooth through verified project-owned
ARM helper assets; Go checks source, builder, dependency and binary hashes.
Maintainer regeneration uses `tools/check_native.py --write` with a private
rootfs and ARM GCC; normal setup and CI require neither.

`models/bar91/profile.json` supplies exact executable identities and architecture:

```sh
python3 tools/check_profiles.py
# After a reviewed profile/code change:
python3 tools/check_profiles.py --write
```

The generator checks pinned standalone payloads, USB preflight and native guards;
it does not regenerate historical scripts or qualify firmware. Preserve unknown
hooks, modified files, contributed regressions and recovery ownership. Keep
credentials, private packages, firmware and captures in ignored directories.

[Bar 9.1 development](docs/models/bar91/DEVELOPMENT.md) covers native behavior;
[installation reference](docs/models/bar91/USB-TOOLKIT.md) covers qualification,
configuration and recovery. Optional features use the existing extension/action
protections. Additional models need evidence for semantics, unknown states,
firmware eligibility and recovery before exposing controls.
