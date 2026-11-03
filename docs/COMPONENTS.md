# Components and compiled extensions

The starting point is one Go module, one on-bar remote executable and plain
browser modules. Shared code is extracted where there is an actual consumer;
the loopback preview is the second consumer of the server and UI. There is no
dynamic plugin loader, generated UI framework or runtime package manager.

## Boundaries that exist today

| Location | Owns | Dependencies |
| --- | --- | --- |
| `apps/remote/internal/server/` | HTTP protection, bounded requests, serialization, public assets and SSE | Standard library and feature metadata; no device paths or model code |
| `apps/remote/internal/extension/` | Typed action registry, snapshot projections, optional workers | Standard library; no HTTP routes or native policy |
| `apps/remote/backend_bar91.go` | Existing v1 aliases, feature composition and native action dispatch | Bar 9.1 device implementation and shared contracts |
| `apps/remote/lifecycle_bar91.go` | Native worker startup, joined shutdown and final restoration | Bar 9.1 implementation |
| `apps/remote/www/components/` | Scoped UI renderers and local draft/display state | Supplied snapshots and bundled modules |
| `apps/remote/www/app.js` | One state connection, one action queue, session state and page controls | Component factories |
| `apps/remote/internal/preview/` | Safe simulated state or normalized read-only fixtures | Shared server contract |
| `apps/remote/internal/jblone/` | Fixed read-only getter collection and conservative fixture normalization | Standard library; community-observed wire shapes, no native paths or setters |

The existing native feature files remain in the application package to preserve
contributor workflows. Extracting the shared server does not make those files
generic. The local adapter uses their existing methods; a future backend can
implement the public shared contract without importing them.

## Backend contract

`server.Backend[S]` has three operations:

```go
Snapshot() S
Execute(context.Context, server.Command) (S, error)
Features() server.Features
```

The snapshot type implements `StateRevision()`, `PublicState(token)` and
`ChangeState()`. The latter two return detached serializable values without
performing I/O. The server supplies the session token; comparison omits volatile
fields such as uptime, token and revision. This preserves the existing v1 JSON
shape while allowing another backend to use its own typed state.

Keep common JSON fields (`available`, `power`, `volume`, `muted`, `input`,
`revision`) compatible. Missing observations stay absent/null or Unknown.
`Execute` returns current observed state with an error when a change cannot be
confirmed. An accepted request is not automatically an observed change.

`Features` supplies model/name, declared core actions and extension metadata.
Build inclusion is distinct from current readiness. Native firmware identity,
source restrictions and fresh-state checks remain in every model's actions.

The server owns its operation gate during `Execute`; a handler must not acquire
that same gate again. Pass the same gate to background writers. Snapshot reads
retain backend-owned locking. The Bar 9.1 monitors already follow this contract.

## Register an optional feature

The model's composition registers an `extension.Module[S]` containing:

- A namespaced ID, local component identifier and inclusion/experimental metadata.
- Typed action handlers accepting bounded JSON parameters.
- An optional pure projection of an already captured snapshot.
- An optional `Run(context.Context)` worker for monitoring/reapplication.

`extensions_bar91.go` contains the working Front EQ, center-level and bass-offset examples.
HTTP and extension actions use the same LAN, origin/CSRF, timeout and operation
gate. Registry workers run only for included modules. They must honor cancellation
after completing bounded native restoration; shutdown joins them before closing
native helpers. Worker scheduling and hardware coordination remain model policy.

Features such as EQ, bass and center gain may share DSP/amplifier state. Registering
separate modules does not permit concurrent writes. Keep the existing guarded
native paths and restoration order. The merged center-wake and EQ/power fixes
and tests are retained in this foundation.

The legacy `eq_apply`, `eq_reset`, `center_next` and `gains_db` requests remain
compatible. New feature clients use namespaced actions and `parameters`.

## Browser components

Each component is a small factory bound to an HTML section. `render(context)`
receives the current snapshot, readiness, pending action and session context.
Playback owns sender intent; EQ owns its unapplied draft; System owns its bounded
history of real samples. Components do not open connections, retry controls,
create independent action queues or read another component's private variables.
The shell supplies the shared timer to components that need one.

`bass-offset.js` owns its selector draft and typed save parameters. It requires
an advertised `bass-offset` component and projects namespaced extension state;
older servers keep the control hidden. Its monitor shares the model action gate
and is joined by the extension lifecycle before native helper cleanup.

`advanced.js` owns the contributed audio-sync draft and calibration/subwoofer
display. The shell debounces taps into one legacy `av_sync_set` request with
`value`, cancels unsent drafts on disconnect/session changes, and keeps one action
queue. The Bar 9.1 composition supplies longer setup/audio-sync budgets through
`Options.ActionTimeout`; the server caps these at 60 seconds and extends that
reply's write deadline for bounded restoration. Other backends retain the default
15 seconds. These native controls do not establish family protocol compatibility.

HTML remains a readable static scaffold with existing IDs, accessibility labels
and CSS. This keeps contributors' UI changes straightforward and requires no
Node build step. Component ownership follows existing panels instead of creating
a separate abstraction for every label or button.

`GET /api/features` selects optional panels through known bundled `component`
identifiers. Metadata never supplies executable code or script URLs. Unsupported
components are hidden; unavailable supported components use their state-based
disabled/error presentation. Older v1 servers without discovery retain existing
behavior. Discovery refreshes when the session token changes after a restart.

The initial optional UI contracts are:

| Component | Projected state | Required namespaced actions |
| --- | --- | --- |
| `front-eq` | `available`, `enabled`, five `gains_db` values, `frequencies_hz`, `status`, optional `error` | `<feature-id>.apply` with `parameters.gains_db`; `<feature-id>.reset` |
| `center-level` | `level`, optional `error` | `<feature-id>.next` |

Front EQ currently represents the existing five-band, cuts-only −6..0 dB controls.
Center represents the existing 0..5 requested levels and +0..12 dB display. Reuse
these components only when semantics/ranges agree. A different DSP feature gets
its own local component; a similar marketing name does not establish equivalence.

The capability adapter reads `state.extensions[feature-id]` and translates local
UI intents to the advertised action namespace. A synthetic different namespace
is exercised by `tests/component-ui.mjs`, without claiming another supported bar.

## Research to implementation

```sh
./jbl dev
./jbl dev --fixture research/bar-family/fixtures/remote-state.example.json
./jbl inspect --host <private-bar-ip>
./jbl test
./jbl test --full
./jbl build --model bar91
```

The default preview permits simulated volume/mute/power changes in memory. A
fixture preview is read-only, advertises no actions, and keeps missing values
unknown. It listens only on 127.0.0.1. Use normalized observations; retain raw
vendor captures and private data under `research/private/`.

Bar 1300 is the next hardware target, with original generation inferred from
reported ownership in 2023. Follow its
[test plan](../research/bar-family/bar1300/README.md): begin with normalized
observations, then implement the backend contract as verified control behavior
arrives. Bar 1000 can follow using the same contracts and family research.
Share a family protocol client when actual commands and response semantics agree.
Model identity, topology, DSP tuning and qualification remain separate.
Installation is an independent capability: a network backend must not implicitly
enable a jailbreak, storage path or boot hook.

## The first real extensibility check

The loopback preview proves that the server and embedded assets run independently
of Bar 9.1. Synthetic feature tests prove namespace binding. Neither proves that
all browser state and control semantics fit a second real bar. Bar 1300 is that
test; its exact variant can remain unknown while shared work proceeds. The first
collector fixture already exposed incorrect standby/startup and mute defaults;
the shared UI now displays unknown power/mute explicitly, with browser regressions.

Extend the existing app in these steps:

1. Use `./jbl inspect --host <private-bar-ip>` to collect the two fixed getters
   and a read-only UI fixture. `internal/jblone/` contains the bounded collector
   with synthetic protocol tests; it does not yet implement a live backend.
   Add real captured-response fixtures, then the adapter in the same package.
   Use ordinary Go functions and `server.Backend[S]`; introduce another interface
   only if a concrete second implementation needs it.
2. Add a small computer-hosted composition using the existing server and
   embedded assets. Leave the current native entry point and contributor file
   layout intact. Running the remote on a computer is useful before a jailbreak
   exists; hosting location is not a reason to duplicate the app.
3. Normalize common observed state into the existing public fields. Advertise
   only tested actions. Represent unreadable values as unknown. Keep native
   product/source numeric IDs in the backend, rather than teaching the shared
   shell every model's identifiers.
4. Adapt a shared component where observations reveal a real difference. The
   current sound component assumes five bass levels, three Atmos levels and a
   particular rear-level sequence. Playback still contains a native source-ID
   condition. These are remaining Bar 9.1 assumptions, not universal contracts.
   Replace them with verified semantic state or narrowly scoped capability data
   when the second backend needs it; preserve existing behavior with fixtures.
5. Use an extension for a distinct optional feature, such as an EQ with different
   scope/ranges. Core volume/mute/source support should remain ordinary backend
   actions. A new bar does not need to become a plugin, and every button does not
   need its own module.

Success means the second real backend needs no copy of HTTP protection, SSE,
the browser connection/action queue or the whole page. Shared components may
need small changes for verified differences. A variant with identical semantics
should need identity/capability data and fixtures, not another backend class.
Install/recovery remains model-owned even when the runtime backend is shared.

Keep source ownership predictable:

| Location | Rule |
| --- | --- |
| `apps/remote/internal/` | Reusable Go runtime packages; `jblone/` starts with inspection and can grow into the family backend |
| `apps/remote/www/components/` | Bundled browser components; capability/semantic behavior instead of scattered model-name checks |
| `models/<model>/` | Qualified install assets, native helpers, profile and device validation; no duplicate copy of the app |
| `research/bar-family/` | Shared research tools and findings |
| `research/bar-family/bar1300/` | Target-specific test plan and reviewed evidence while support is being developed |
| `research/private/` | Raw device captures, firmware and private session records |

The Bar 9.1 native Go files currently remain at `apps/remote/` as a deliberate
incremental exception. They are not shared merely because of that path. Move
them together only when a concrete consumer or separate mechanical cleanup
justifies the contributor disruption. Do not create empty `bar1000`, `bar1300x`
or `mk2` package trees in anticipation of differences.

## Next lifecycle milestone

The shared USB/SSH updater remains a separate implementation milestone. It must
work independently of the remote, preserve owner state, and validate storage and
release/config compatibility before stopping services. USB bootstrap checks must
be separated from ongoing release application. This component foundation adds
no working `update` command and does not relax the current package mismatch guard.
See the USB guide for current installation and recovery behavior.
