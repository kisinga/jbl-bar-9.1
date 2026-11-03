# Browser remote

Go service and embedded browser UI for the JBL Bar Toolkit. The current native
control implementation targets the reviewed Bar 9.1 firmware only; moving the
application here does not add other model support.

From the repository root:

```sh
python3 tools/build_web.py --allowed-cidr <trusted-private-subnet>
(cd apps/remote && go test -race ./... && go vet ./...)
```

The [combined USB builder](../../docs/models/bar91/USB-TOOLKIT.md) packages the
same service. Model-owned lifecycle scripts and C helper sources are under
[`models/bar91/remote/`](../../models/bar91/remote/). Optional native helpers
remain owner-built and reuse guarded vendor libraries on the bar.

`profile_bar91_generated.go` embeds identities from the reviewed
[model profile](../../models/bar91/profile.json). Use
`python3 tools/check_profiles.py` to check drift; do not hand-edit the generated
file. Source-specific memory addresses, actions and restoration guards remain
part of the native Go implementation.

The [component guide](../../docs/COMPONENTS.md) describes the working boundaries:

- `internal/server/`: shared HTTP, access protection, serialized actions and SSE.
  It also tracks the control holding the action gate (`state.control` for states
  implementing `server.ControlState`) and serves the latest state while it runs,
  so reads and new streams never wait for a control. Background writers take the
  gate through `TryHold`; Bar 9.1 reports its automatic Standard Mode gesture as
  `auto_standard` and keeps its short monitors unreported.
- `internal/extension/`: compiled feature actions, pure projections and optional
  workers, joined before native cleanup.
- `backend_bar91.go` / `lifecycle_bar91.go`: existing native composition and policy.
- `www/components/`: scoped UI factories; the shell owns one stream/action queue.
- `cmd/preview` / `internal/preview`: a second server consumer with no hardware access.
- `cmd/inspect` / `internal/jblone`: two fixed read-only family getters, private
  capture records and conservative fixtures for the same preview/UI; no setters.

```sh
./jbl dev
./jbl dev --fixture research/bar-family/fixtures/remote-state.example.json
./jbl inspect --host <private-bar-ip>
./jbl test
./jbl test --full
./jbl build --model bar91
```

`server.Backend[S]` supplies `Snapshot`, `Execute` and `Features`. The native
implementation remains explicitly Bar 9.1; a future family backend implements
that public contract without importing the native application. Existing v1 JSON
fields, `eq_apply`, `eq_reset`, `center_next` and `gains_db` remain compatible.
New clients can use namespaced actions and typed `parameters`.

`/api/features` advertises build inclusion and known local UI component IDs.
`state.extensions` supplies feature state. Inclusion does not establish current
firmware/readiness: every native action retains its existing guards. The browser
maps compatible component state/actions from any declared feature namespace;
metadata never downloads code. Extensions compile into the static ARM executable.

Upstream through `3a52f58` is incorporated: center-wake (#7), EQ/power (#8), center
labels (#9), and advanced controls (#5). The native audio-sync/setup source and
tests are byte-identical to upstream. `components/advanced.js` owns the audio-sync
draft and setup display; the shell owns its debounce and request queue. Legacy
`av_sync_set` with `value` stays compatible. The native composition supplies
18/60-second action budgets; the generic server caps budgets and extends the reply
deadline without interpreting model action names.
Native feature files retain their existing structure for contributors. The
[architecture](../../docs/ARCHITECTURE.md) and [model development guide](../../docs/models/bar91/DEVELOPMENT.md)
cover qualification and native policy. Software updates remain a separate milestone.
