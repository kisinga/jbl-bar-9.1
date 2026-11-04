# Host and browser checks

Run from the repository root:

```sh
python3 tools/test.py
(cd apps/remote && go test -race ./... && go vet ./...)
node tests/webremote-ui.mjs
node tests/setup-ui.mjs
node tests/component-ui.mjs
node tests/bass-offset-ui.mjs
python3 research/bar-family/test_workspace.py
```

`tools/test.py` discovers shared `check_*.py` files here and model suites under
`bar91/`. Model fixtures and original payload hash baselines are under
`bar91/fixtures/`. Reports stay in ignored `build/test-results/`.

The CLI checks exercise the current local API. Profile checks reject unqualified
model substitutions and disagreement between profile metadata and native guards.
Project checks exercise the public ARM ZIP, credential-free recovery, single
model entry point and independent crypt/WPA2 vectors for the offline setup page.
The Bar 9.1 suite includes the combined USB parser, ownership, partial failures,
configuration and recovery, USB staging/readback, atomic storage checks, native
inspection, installed integrity and new-boot verification. Firmware/SSH operations
remain explicit fixture boundaries. Go tests live beside their implementations.

Browser fixtures need Playwright and Chromium; `PLAYWRIGHT_MODULE` can select an
existing local installation. Research replay and archive checks retain their
own dependencies and evidence boundaries.

Host fixtures, browser simulations and native replay do not establish physical
device installation, acoustics, standby behavior or recovery.

`check_hardware.py` checks automatic selection, unknown/ambiguous identities,
bootstrap integrity and firmware-independent owned recovery. `check_project.py`
also checks the CI matrix and distinct default/per-model public assets.

`./jbl test` runs the fast Go suite without BusyBox or device access.
`./jbl test --full` also runs host integration, Go race checks and vet.
`component-ui.mjs` builds the real preview composition, checks read-only replay,
and tests optional UI components with a synthetic feature namespace. The full
UI suite now drives state/history through snapshots and public component APIs,
without requiring globals from the application shell.

`internal/jblone` Go tests exercise the inspection collector against synthetic
loopback SOAP replies, including faults, wrong responses, truncation, redirects,
cancellation and unknown values. Project checks validate inspection target/output
refusals and launcher arguments without contacting a device. These checks do not
establish getter availability on the tester's firmware.

The contributed advanced-control regression tests are retained beside the native
implementation. Browser checks cover setup/audio-sync behavior and center labels.
Shared-server tests cover composition-defined action budgets, bounded reply
deadlines and legacy numeric values. Component browser checks also cover
capability discovery, unknown state and cancelling an unsent sync draft.

`bass-offset-ui.mjs` checks typed extension requests, explicit saves, draft
preservation, standby, calibration, failures, capability inclusion and mobile
layout using synthetic state. It never contacts a soundbar.

`check_release.py` checks changelog validation, complete target/checksum coverage,
source provenance, immutable publication, retry guards and upload failures without
writing to GitHub. `webremote-ui.mjs` also produces sample README screenshots at
`build/readme-remote.png` and `build/readme-advanced.png`. To refresh the public
images after reviewing them, copy those to `docs/web-remote.png` and
`docs/web-remote-advanced.png` respectively.
