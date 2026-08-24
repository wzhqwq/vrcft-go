# M6 OSC End-to-End Loop Design

## Summary

M6 composes the implemented plugin, tracking, processing, evaluator, avatar,
and OSC components into one cancellable backend lifecycle. A single
Application-owned coordinator serializes avatar-plan activation and frame
evaluation so a previous avatar generation cannot publish through a newly
installed OSC catalog.

M6 is a backend milestone. It does not wire the root Wails bootstrap, choose or
persist user paths, expose frontend bindings, or implement M7 UI and release
work. `internal/application` accepts explicit configuration so M7 can construct
it later without M6 inventing configuration defaults.

## Goals

- Consume `/avatar/change` notifications and compile them through the M5
  `avatar.Planner`.
- Install every allocated plan, including failed and ready-but-empty plans, as
  a fail-closed generation transition.
- Route authenticated plugin frames through tracking merge, processing,
  selective evaluation, and the current local-avatar OSC bindings.
- Make the M5 local Avatar Plan the exclusive owner of output bindings in the
  composed Application.
- Prevent stale frames, evaluator results, catalogs, or plugin subscriptions
  from crossing an avatar generation boundary.
- Start, roll back, and stop the composed backend in a deterministic order.
- Expose bounded, payload-free runtime status for later M7 presentation.
- Prove the complete composition with deterministic normal and race tests.

## Non-Goals

- Root Wails startup, shutdown hooks, or frontend bindings.
- Persistence or UI selection of OSC root, fallback path, plugin roots,
  processing settings, OSC target preferences, or other Application config.
- Plugin installation, updates, signing, or marketplace behavior.
- Numeric Lip payloads or Expression-to-Lip mapping.
- Replacing M5 local JSON discovery with OSCQuery, or merging the two binding
  sources.
- Retaining every intermediate avatar in an unbounded burst queue.

## Chosen Architecture

M6 uses one Application coordinator rather than independent plan and frame
workers or component restart on every avatar. The coordinator owns the current
immutable plan, the stateful processing pipeline, and evaluator calls. Avatar
notifications, merged-frame updates, plugin lifecycle events, and a processing
clock enter this one serialized loop.

```text
plugins -- generation frames --> tracking.Service
                                      | merged latest
                                      v
/avatar/change --> Application coordinator --> processing.Pipeline
       |                    |                         |
       v                    | current evaluator.Plan |
 avatar.Planner             v                         v
       |             plugin subscriptions      evaluator.Snapshot
       |                                              |
       +-- catalog + generation                       v
                                           OSC generation source
                                                     |
                                                     v
                                              VRChat UDP output
```

Planning is a low-frequency synchronous control operation. It may briefly
delay frame processing, while the capacity-one merged-frame subscription keeps
the latest value. This makes switch ordering explicit and avoids the more
complex generation fences required by multiple evaluation workers.

## Package Responsibilities

### `internal/application`

The package constructs and owns the process-lifetime backend components:

- plugin Manager and its `tracking.PluginFrameSink`;
- tracking Service;
- processing Pipeline;
- avatar Planner;
- external-catalog OSC runtime;
- the serialized coordinator and its cancellation/join state; and
- the latest immutable Application status plus bounded subscribers.

It owns plan-install ordering and joins component errors, but it does not
reimplement package algorithms or persist configuration.

The Application code is split by responsibility rather than placed in
one large `app.go`:

- `config.go` validates explicit construction and timing configuration;
- `app.go` owns construction and Start/Close state;
- `coordinator.go` owns the event loop and frame schedule;
- `install.go` implements one fail-closed plan transition;
- `status.go` owns immutable diagnostics and latest-value subscribers; and
- focused tests and one external integration fixture cover each boundary.

### `internal/osc`

OSC continues to own discovery of VRChat/OSCQuery services, target selection,
UDP receive/send, packet encoding, change suppression, and `/avatar/change`
parsing. It gains an external-catalog mode used by M6.

In external mode:

- OSCQuery never compiles, refreshes, or overwrites the output catalog;
- OSCQuery remains available for identifying VRChat and its UDP target;
- avatar notifications are published through a dedicated bounded latest-value
  control stream, separate from best-effort diagnostic events;
- the Application explicitly clears and installs the send runtime; and
- published evaluator sources must match the installed catalog generation.

The existing OSCQuery-owned catalog mode remains the default for existing
standalone Controller behavior and tests. OSC must not depend on avatar,
tracking, processing, evaluator, plugins, or application.

## Public and Internal Shape

Application construction is explicit. The exported config and lifecycle API
are:

```go
type Config struct {
    Avatar              avatar.PlannerConfig
    PluginCatalog       plugins.DirectoryCatalogConfig
    PluginStorePath     string
    PluginStoreMaxBytes int64
    PluginOptions       plugins.Options
    Processing          processing.Config
    OSC                 osc.ControllerConfig
    FrameInterval       time.Duration
    PluginControlTimeout time.Duration
}

const (
    DefaultFrameInterval         = 10 * time.Millisecond
    DefaultPluginControlTimeout = 2 * time.Second
)

func NewApp(Config) (*Application, error)
func (*Application) Start(context.Context) error
func (*Application) Close(context.Context) error
func (*Application) Status() Status
func (*Application) SubscribeStatus(context.Context) <-chan Status
```

All filesystem roots and files are supplied by the caller. A zero
`FrameInterval` selects `DefaultFrameInterval`; a zero
`PluginControlTimeout` selects `DefaultPluginControlTimeout`. Other nested
configs retain their owning package's documented zero-value behavior. M6 does
not guess filesystem paths. Invalid negative durations, invalid component
configs, or failed component construction make `NewApp` fail without starting
goroutines or processes.

Tests use an unexported dependency bundle for fake Manager, Planner, OSC
runtime, ticker, and clock injection. There is no second exported dependency
injection API.

The OSC application-facing interface is:

```go
type CatalogMode uint8

const (
    CatalogOSCQuery CatalogMode = iota
    CatalogExternal
)

type AvatarChange struct {
    Revision uint64
    AvatarID string
}

type OSCStatus struct {
    Running   bool
    Connected bool
    HasTarget bool
    Target    OSCTarget
    LastError string
}

type OSCService interface {
    Start(context.Context) error
    Close(context.Context) error
    Events() <-chan ControllerEvent
    AvatarChanges(context.Context) <-chan AvatarChange
    ClearRuntime()
    InstallCatalog(*Catalog) error
    Publish(uint64, ValueSource) error
    Status() OSCStatus
}
```

`ControllerConfig` gains a `CatalogMode CatalogMode` field. The zero value is
`CatalogOSCQuery`, preserving current behavior. `NewOSCService` changes from a
parameterless constructor that can terminate the process to
`NewOSCService(ControllerConfig) (OSCService, error)`; construction errors are
returned to Application. The placeholder `SetEnabled` and `SetTarget` methods
are removed from `OSCService`; M6 always follows the Controller's discovered
target while it is started. Later user override controls require a separate M7
design.

`InstallCatalog` clones caller-owned catalog data. `Publish` accepts an
immutable source and rejects zero or a generation different from the installed
catalog. External runtime mutation is rejected in OSCQuery mode.

The send loop observes catalog and source as one synchronized runtime. A send
holds the read side of that boundary through transport use; `ClearRuntime`
holds the write side and returns only after an in-progress old-runtime send has
finished. A catalog may be installed with no source, which sends nothing until
a same-generation source is published.

## Avatar Notification Mailbox

`/avatar/change` is a control signal and must not depend on the best-effort
diagnostic event channel. The Controller assigns every valid string message a
positive monotonic revision and offers the latest `AvatarChange` to a
capacity-one subscriber.

If planning is idle, the coordinator handles the notification immediately. If
one or more notifications arrive while planning or installing a plan, the
mailbox retains the newest Avatar ID and revision. Multiple pending messages
may coalesce, but a repeated ID still changes the revision and therefore
causes at least one reload after the current activation. The design guarantees
eventual processing of the latest state without an unbounded queue or blocking
the UDP receive goroutine; it does not promise to replay every intermediate
avatar in an extreme burst.

## Fail-Closed Plan Installation

For every non-nil plan returned by `Planner.Activate`, the coordinator performs
one serialized transition:

1. Call `OSCService.ClearRuntime`. After it returns, the old catalog and old
   evaluator source can no longer start a send.
2. Call `tracking.Service.SetGeneration(plan.Generation())`. This atomically
   clears retained sources/selections and makes old plugin frames stale.
3. Make the plan the coordinator's current diagnostic plan, but do not expose
   its catalog yet.
4. Call `SetActive(false)` for every known plugin. Each call has its own bounded
   control context.
5. For every plugin whose advertised capabilities intersect the plan, call
   `UpdateSubscription` and activate it only after that update succeeds.
6. For a ready plan with a non-empty catalog, install its cloned catalog with
   no value source. Failed and ready-but-empty plans retain an empty runtime.
7. Publish one new Application Status revision, including the plan diagnostic
   and any plugin control failures.

The coordinator iterates plugins in stable plugin-ID order. It uses advertised
capabilities from Manager snapshots and never changes the persisted Enabled
preference. A disabled plugin may receive desired avatar-session controls, but
its supervisor remains governed by its user preference.

Plugin controls cannot be a cross-process transaction. A failure leaves that
plugin inactive, is recorded, and does not prevent safe controls for other
plugins. The old plan is never restored. The new tracking generation already
rejects old frames, so even a plugin that fails to observe deactivation cannot
feed the current pipeline with its prior generation.

If generation advance fails, the coordinator retains an empty OSC runtime,
attempts to deactivate all plugins, does not install the plan catalog, and
records an internal consistency error. Generation exhaustion is the only
activation error with a nil plan; it follows the same clear/deactivate policy
and places Application status in a terminal exhausted state. Future avatar
notifications cannot restore an older plan.

`Result.Err` is reported only after its non-nil failed plan has been installed.
Configuration failure is not an early-return path.

## Plugin Lifecycle Integration

The Manager is constructed with `tracking.NewPluginFrameSink(trackingService)`
so authenticated session frames preserve plugin identity and subscription
generation at the ingest boundary.

The coordinator consumes bounded Manager lifecycle events. When a plugin is no
longer Running, becomes inactive, is disabled, crashes, or otherwise loses its
current session, the coordinator calls `tracking.RemoveSource(pluginID)`.
Unknown removals are idempotent. Manager desired subscription/active state is
retained by its supervisor, so a later restart handshakes with the current
avatar-session controls without Application manufacturing a second state
store.

## Frame Scheduling and Evaluation

The coordinator subscribes to tracking's capacity-one merged stream and keeps
the latest frame. It processes a current-generation frame in two cases:

- immediately when a new merged value arrives; and
- on every configurable frame tick, default 10 milliseconds, even when the
  merged revision did not change.

The repeated tick is required for M4 dropout hold/decay and active-staleness
transitions. A Host clock adapter supplies non-regressing nanoseconds; wall
clock rollback is clamped so it cannot create `processing.ErrTimeRegression`.
All calls to the caller-serialized Pipeline occur in the coordinator.

A failed plan has no evaluator and does not process frames. A ready-but-empty
plan also skips processing/evaluation because it has no requested output and
publishes no OSC source. For an ordinary ready plan:

1. process the merged frame through `Pipeline.ProcessAt`;
2. evaluate the resulting canonical frame with the current immutable
   evaluator Plan; and
3. publish the immutable evaluator snapshot with the plan generation.

Frames whose generation differs from the current plan are ignored. OSC also
checks generation on publish, providing a second fence against late results.

## Runtime Error and Recovery Policy

A processing error or generation invariant violation clears the OSC runtime
and records a degraded status; it must not keep sending the last snapshot. The
current ready plan remains available. A later valid frame can reinstall the
same plan catalog, reset sender change detection, publish a same-generation
snapshot, and return the frame path to running state.

OSC network errors and target disconnects update status but do not restore old
plans. A nil target sends nothing. External catalog/source state remains
associated with the current generation while disconnected. Reconnection
resets change detection so current values are sent again rather than suppressed
as unchanged.

Diagnostic/event backpressure never blocks the frame or UDP paths. Application
Status is the authoritative bounded summary; verbose component logs remain
component-owned.

## Application Status

The exported status contract is:

```go
type LifecycleState string

const (
    LifecycleCreated  LifecycleState = "created"
    LifecycleStarting LifecycleState = "starting"
    LifecycleRunning  LifecycleState = "running"
    LifecycleDegraded LifecycleState = "degraded"
    LifecycleClosing  LifecycleState = "closing"
    LifecycleClosed   LifecycleState = "closed"
)

type PluginControlFailure struct {
    PluginID  string
    Operation string
    Message   string
}

type Status struct {
    Revision  uint64
    UpdatedAt time.Time
    Lifecycle LifecycleState

    AvatarID           string
    PlanGeneration     uint64
    PlanStatus         avatar.Status
    PlanSource         avatar.Source
    ConfigPath         string
    ConfigID           string
    GenerationExhausted bool

    OSC            osc.OSCStatus
    PluginFailures []PluginControlFailure
    PlanError      string
    RuntimeError   string
}
```

It contains:

- a lifecycle state (`created`, `starting`, `running`, `degraded`, `closing`,
  or `closed`);
- monotonic status revision and update time;
- current Avatar ID, plan generation/status/source/path/config ID;
- terminal generation-exhausted state when applicable;
- current OSC connection/target state;
- stable plugin IDs that failed the latest control transition and a sanitized
  aggregate error summary; and
- the most recent planning, processing, runtime, or lifecycle error summary.

It contains no tracking values, plugin configuration contents, session token,
transport buffer, or unbounded history. Path diagnostics already exposed by
the M5 Plan are retained. `Status()` returns a deep-owned value snapshot,
including a fresh `PluginFailures` slice. `SubscribeStatus` immediately offers
the current value and then uses capacity-one latest-value replacement;
cancellation removes and closes the subscriber without racing publication.
Before the first plan, the zero `avatar.Status` and `avatar.Source` values mean
"no plan installed"; they are not reported as ready or failed.

## Lifecycle

`Start` is a single-use transition. It creates a child context and all
subscriptions before a producer starts. The production start sequence is:

1. start the plugin Manager while plugins have empty subscription and inactive
   avatar-session state;
2. start the coordinator and wait until its event loop is ready; and
3. start OSC last so an Avatar notification cannot precede its consumer.

After the coordinator is ready, it reconciles plugin diagnostics from
`Manager.List`; correctness does not depend on retaining every discovery event
published during Manager startup.

If Manager start fails, no OSC worker starts. If coordinator startup or OSC
startup fails, cancellation stops and joins the coordinator and the already
started Manager is closed. Rollback errors are joined with the owning startup
error.

`Close` is idempotent and bounded by the caller context. It:

1. closes OSC first, preventing new Avatar events and network sends;
2. cancels the coordinator, clears its runtime, and waits for it to exit; and
3. closes the plugin Manager.

Tracking and processing are value/state services without independent Close
methods. Close errors are joined and the stable result is returned to repeated
callers. Start after closing is rejected.

## Concurrency and Ownership

The coordinator is the only owner that mutates current-plan, processing, and
recovery state. Tracking and plugin Manager retain their existing concurrency
contracts. Status has its own short critical section and never holds its lock
while invoking component methods.

Plans, evaluator snapshots, merged frames, canonical frames, and status values
are retained as owned immutable values. OSC clones installed catalogs and does
not expose the live send runtime. No caller slice/map or component event payload
is retained without the producing package's documented ownership guarantee.

All Application queues are bounded: avatar latest value is capacity one,
tracking merged is capacity one, Status is capacity one per subscriber, and
existing component event bounds remain unchanged. The Application introduces
no frame history or unbounded error queue.

## Error Model

Application errors wrap stable component errors with operation context and use
`errors.Join` where multiple independent cleanup/control errors matter. Error
text never contains avatar JSON, plugin config, session credentials, or frame
payloads.

Construction errors identify the invalid config/component. Startup and
shutdown errors identify lifecycle stage. Plan status retains M5's
`errors.Is`-compatible classification. Per-plugin failures retain stable plugin
IDs in deterministic order without preventing other controls. Frame-path
errors clear output before becoming observable.

## Testing Strategy

### OSC external runtime

`internal/osc` tests prove:

- OSCQuery mode preserves existing catalog refresh behavior;
- external mode never builds or overwrites an output catalog from OSCQuery;
- clear waits for an in-flight send and no old runtime begins sending after it
  returns;
- catalog installation owns a deep clone;
- source publication rejects zero, stale, and future generations;
- installed catalog without a source sends nothing;
- target reconnect resets change detection and resends current values; and
- repeated identical Avatar IDs still advance mailbox revision while bursts
  retain the latest notification.

### Plan installation

Application tests with deterministic fakes assert the exact call order:

```text
clear OSC
advance tracking generation
deactivate every plugin by sorted ID
update matching subscriptions
activate only successful matches
install ready non-empty catalog
publish status
```

They cover ready, ready-empty, failed, partial plugin-control failure, tracking
generation failure, and generation exhaustion. Every failure proves that the
old catalog/source is absent and no rollback activates an old subscription.

### Coordinator and frame loop

Tests cover immediate merged-frame processing, timer-only dropout/decay,
non-regressing Host time, old/future frame rejection, same-generation recovery
after a processing error, stale publish rejection, rapid Avatar latest-wins,
and repeated-ID reload after an in-flight activation.

### Lifecycle and diagnostics

Tests prove construction has no side effects, producer subscriptions precede
OSC start, partial-start rollback order, reverse shutdown, repeated Close,
Start-after-Close rejection, plugin-loss source removal, bounded status
replacement, subscriber cancellation, and coordinator goroutine exit.

### End-to-end composition

`internal/application/app_test.go` uses a real avatar Planner, tracking Service,
processing Pipeline, evaluator Plan, parameter definitions, and OSC binding
semantics with an in-memory plugin Manager and OSC transport/runtime boundary.
It proves a generation-bearing plugin frame reaches only parameters requested
by the current Avatar config. Switching avatars changes bindings and
subscriptions; malformed or missing configuration clears output; late old
frames and snapshots cannot reappear.

Existing plugin handshake/process tests and OSC wire tests remain the owning
evidence for those component boundaries. The Application integration fixture
does not spawn an external plugin process or require a real VRChat instance.

Normal, repeated, race, vet, and full-repository checks use the fixed absolute
repository-local `.go-gocache`.

## Project Evidence

M6 updates the package specifications for `internal/application` and
`internal/osc`, the end-to-end subsystem contract, and adjacent package gap
text where Application composition is no longer deferred. Acceptance checks
must name executable tests for:

- Application lifecycle and atomic installation;
- Avatar-aware end-to-end output;
- OSC external-runtime ownership/generation fencing; and
- Application race safety.

Generated `docs/project/status.md` is refreshed only after implementation,
task reviews, full normal/race/vet verification, and a clean whole-range review.
The report may leave M7 frontend/configuration gaps open but must show M6
complete without claiming Wails root integration.

## Completion Definition

M6 is complete when:

1. the configured backend Application constructs and owns plugin, tracking,
   processing, avatar, evaluator, and external-catalog OSC composition;
2. each delivered/coalesced Avatar notification installs a new ready or
   fail-closed generation without restoring old state;
3. old frames, evaluations, catalogs, and subscriptions cannot cross a plan
   generation boundary;
4. current-generation plugin data reaches only the current Avatar's requested
   OSC bindings through processing and selective evaluation;
5. plugin/process, processing, and OSC failures clear or degrade output under
   the documented recovery policy;
6. startup rollback, reverse shutdown, diagnostics, and all bounded queues pass
   normal and race tests;
7. package/subsystem specifications and generated evidence report M6 complete;
   and
8. root Wails construction, persisted configuration, frontend diagnostics,
   numeric Lip payloads, and Expression-to-Lip mapping remain explicitly
   outside M6.
