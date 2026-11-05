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

Contributor commands under `./jbl` require Python 3.10+. Full checks also need
Linux, BusyBox, dosfstools and Node 20+ (CI uses Node 24). Install browser tools
once with `npm ci` and `npm run test:ui:install`; on Linux, install missing browser
libraries with `npx playwright install-deps chromium`. `PLAYWRIGHT_MODULE` can
reuse an existing Playwright installation. Checks never write to attached USBs.

```sh
./jbl test                                      # Fast Go checks; no build integration
./jbl test --full                               # All host, race/vet and browser checks
npm run test:ui -- --suite remote                # Remote browser checks only
npm run test:ui -- --suite setup                 # Setup browser/source-launch checks
python3 tools/test.py --suite usb --suite setup  # Selected host fixtures
```

`./jbl dev` previews the remote with sample data; `./jbl test --ui` runs all browser
checks. Windows contributors can use `python tools/project.py test` or `npm run
test:ui` directly; the full shell fixtures run on Linux. The host runner discovers
`tests/check_*.py` and model fixtures, runs four at a time (`--jobs` overrides), and
writes reports and suite timings to ignored `build/test-results/`. It independently
compares Go-built payloads with the legacy builder. Setup tests use temporary
folders, sparse disk images and mocked native commands. Browser fixtures check
production assets, capabilities, drafts, disclosure defaults and device states.
These checks do not qualify physical installation, sound, standby or recovery.
SSH fixtures use a loopback server with real authentication and encrypted channels;
device transaction fixtures test setting/key retention and failure handling without a bar.

PR checks follow changed paths: documentation needs preflight only; interface
changes run the relevant browser suites, with remote race/vet and an ARM build
for remote assets. Shared, unknown and tooling paths run everything. Pushes to
`main`, tags and manual runs always run the complete suite, including native
Windows/macOS checks. CI uploads the exact public packages verified by the host
fixtures, rather than rebuilding them. One `test` check gates all selected jobs.
Preview routing locally with `python3 tools/ci.py --base origin/main`.

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
