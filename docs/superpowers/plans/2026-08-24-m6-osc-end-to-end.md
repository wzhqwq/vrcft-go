# M6 OSC End-to-End Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compose the existing plugin, tracking, processing, evaluator, avatar-planning, and OSC packages into one fail-closed, generation-safe backend Application lifecycle.

**Architecture:** A single `internal/application` coordinator serializes Avatar activation, plugin controls, processing, and evaluator publication. `internal/osc` gains an external-catalog runtime whose catalog/source pair is generation-fenced and synchronized with the send loop; OSCQuery remains responsible for VRChat discovery and targeting but cannot overwrite Application-installed bindings.

**Tech Stack:** Go 1.24, existing internal package contracts, contexts/channels/mutexes, table-driven tests, race detector, repository project-status generator.

**Spec:** `docs/superpowers/specs/2026-08-24-m6-osc-end-to-end-design.md`

## Global Constraints

- Work directly from the user-selected `master` checkout unless the user changes that decision before execution.
- Set `GOCACHE` to the absolute repository-local `.go-gocache/` before every Go build, test, race, vet, run, or generate command; never clear the cache without a recorded reproducible corruption error.
- Do not modify the root Wails bootstrap, frontend, configuration persistence, or M7 bindings.
- In the composed Application, the M5 local Avatar Plan is the exclusive output-catalog source; OSCQuery discovers VRChat and its target but never merges with or overwrites that plan.
- Apply every non-nil M5 plan before reporting its error. Failed and ready-but-empty plans clear output and advance tracking generation.
- Generation exhaustion is the only activation result with a nil plan; it clears output, deactivates plugins, and is terminal.
- Clear OSC before advancing tracking or changing plugin controls. Never roll back to an old plan after a partial control failure.
- Update a matching plugin's subscription before activating it; nonmatching or failed plugins stay inactive. User Enabled preferences remain plugin-owned and unmodified.
- Avatar bursts use bounded capacity-one latest-value replacement. Repeated identical IDs retain one pending reload via a new revision; intermediate burst values need not all be replayed.
- Process new merged frames immediately and reprocess the latest frame every 10 ms by default for dropout/active timing.
- Preserve the dependency direction: evaluator, processing, and tracking must not import OSC or Application.
- Numeric Lip payloads and Expression-to-Lip mapping remain out of scope.
- All new queues/subscriptions are bounded, all caller-visible slices are owned copies, and no status contains frame payloads, plugin config contents, or session credentials.
- Run the narrowest tests during each task, then each task's required normal/race/vet checks before review and commit.

---

### Task 1: Build the Generation-Fenced OSC Send Runtime

**Files:**
- Create: `internal/osc/runtime.go`
- Create: `internal/osc/runtime_test.go`
- Modify: `internal/osc/sender.go`

**Interfaces:**
- Consumes: `ParameterSender`, `Catalog.Clone`, and `ValueSource`.
- Produces: exported `CatalogMode` with zero/default `CatalogOSCQuery` and explicit `CatalogExternal` values used by the runtime and Controller.
- Produces: package-private `sendRuntime` with `clear`, `installQuery`, `installExternal`, `publish`, `resetChangeDetection`, and `send` methods used by Task 2.
- Produces: `ErrRuntimeMode`, `ErrRuntimeCatalog`, and `ErrRuntimeGeneration` sentinels.

- [ ] **Step 1: Write ownership and generation RED tests**

Create table-driven tests that compile a generation-7 catalog and assert:

```go
runtime := newSendRuntime(sender, CatalogExternal, nil)
if err := runtime.installExternal(catalog); err != nil {
    t.Fatal(err)
}
mutateEveryCatalogLayer(catalog)
if got := runtime.catalog(); !reflect.DeepEqual(got, originalCatalog) {
    t.Fatalf("installed catalog changed: %#v", got)
}

if err := runtime.publish(6, source); !errors.Is(err, ErrRuntimeGeneration) {
    t.Fatalf("stale publish error = %v", err)
}
if err := runtime.publish(8, source); !errors.Is(err, ErrRuntimeGeneration) {
    t.Fatalf("future publish error = %v", err)
}
if err := runtime.publish(7, source); err != nil {
    t.Fatal(err)
}
```

Also assert nil catalog/source, generation zero, and external mutation in
`CatalogOSCQuery` return the exact sentinels. Verify a catalog with no source
sends no packet and query mode sends only from its fixed constructor source.

- [ ] **Step 2: Write the in-flight clear RED test**

Use a blocking `packetSender`. Install a catalog/source, enter `send` until the
transport blocks, then call `clear` from another goroutine. Assert `clear` does
not return before the transport is released, does return afterward, and a
subsequent `send` emits nothing:

```go
go func() { sendDone <- runtime.send() }()
<-transport.entered
go func() { runtime.clear(); close(clearDone) }()
assertNotClosed(t, clearDone)
close(transport.release)
assertClosed(t, clearDone)
```

- [ ] **Step 3: Run the runtime tests to verify RED**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/osc -run 'TestSendRuntime' -count=1
```

Expected: build failure because `sendRuntime` and its sentinels do not exist.

- [ ] **Step 4: Implement the synchronized runtime**

Define the mode before the runtime so this task is independently buildable:

```go
type CatalogMode uint8

const (
    CatalogOSCQuery CatalogMode = iota
    CatalogExternal
)
```

Then use this shape:

```go
type sendRuntime struct {
    mu         sync.RWMutex
    mode       CatalogMode
    sender     *ParameterSender
    fixed      ValueSource
    generation uint64
    source     ValueSource
}

func newSendRuntime(*ParameterSender, CatalogMode, ValueSource) *sendRuntime
func (*sendRuntime) clear()
func (*sendRuntime) installQuery(*Catalog) error
func (*sendRuntime) installExternal(*Catalog) error
func (*sendRuntime) publish(uint64, ValueSource) error
func (*sendRuntime) resetChangeDetection()
func (*sendRuntime) catalog() *Catalog
func (*sendRuntime) send() error
```

`send` holds `RLock` through `ParameterSender.Send`. All mutations hold `Lock`.
External installation clones and installs the catalog, records its positive
generation, and clears the source. Query installation uses the fixed source.
`clear` removes catalog/source/generation and resets sender caches. Keep
`ParameterSender`'s existing mutex and deep-cloning `Catalog()` accessor; do
not expose `sendRuntime` publicly.

- [ ] **Step 5: Run stress and race GREEN**

```powershell
gofmt -w internal/osc/runtime.go internal/osc/runtime_test.go internal/osc/sender.go
go test ./internal/osc -run 'TestSendRuntime' -count=200
go test -race ./internal/osc -run 'TestSendRuntime' -count=50
go test ./internal/osc -count=20
go vet ./internal/osc
git diff --check
```

- [ ] **Step 6: Commit the runtime**

```powershell
git add internal/osc/runtime.go internal/osc/runtime_test.go internal/osc/sender.go
git commit -m "feat(osc): add generation-fenced send runtime"
```

---

### Task 2: Add External Catalog Mode and Avatar Mailbox

**Files:**
- Create: `internal/osc/avatar_changes.go`
- Create: `internal/osc/avatar_changes_test.go`
- Modify: `internal/osc/controller.go`
- Modify: `internal/osc/controller_test.go`
- Modify: `internal/osc/service.go`
- Create: `internal/osc/service_test.go`

**Interfaces:**
- Consumes: Task 1 `CatalogMode` and `sendRuntime`.
- Produces: exported `AvatarChange`, revised `OSCStatus`, revised `OSCService`, and `NewOSCService(ControllerConfig) (OSCService, error)` from the approved spec.
- Produces: `Controller.ClearRuntime`, `InstallCatalog`, `Publish`, `AvatarChanges`, and `Status` through the service facade.

- [ ] **Step 1: Write Avatar mailbox RED tests**

Subscribe before Controller start through a package-private publish seam. Assert
the first event has revision 1, identical IDs receive increasing revisions,
and a full capacity-one channel replaces its pending value with the newest ID:

```go
changes := controller.AvatarChanges(ctx)
controller.publishAvatarChange("avtr_a")
first := <-changes
controller.publishAvatarChange("avtr_a")
controller.publishAvatarChange("avtr_b")
latest := <-changes
if first.Revision != 1 || latest.AvatarID != "avtr_b" || latest.Revision != 3 {
    t.Fatalf("changes = %#v, %#v", first, latest)
}
```

Cover cancellation closure, Controller Close closure, multiple subscribers, and
revision saturation without wrap.

- [ ] **Step 2: Write external/query mode RED tests**

Add tests proving:

```text
zero/default CatalogOSCQuery -> existing refresh compiles OSCQuery catalog
CatalogExternal              -> refresh probes health but never changes catalog
external InstallCatalog      -> Catalog returns an owned matching clone
external Publish mismatch    -> ErrRuntimeGeneration
query Install/Publish        -> ErrRuntimeMode
external disconnect          -> target nil, installed runtime retained
external reconnect           -> current runtime retained and change cache reset
```

Use the existing fake query client/browser/transport seams; do not bind a real
network port in these unit tests.

- [ ] **Step 3: Run targeted tests to verify RED**

```powershell
go test ./internal/osc -run 'Test(AvatarChanges|ControllerExternalCatalog|OSCService)' -count=1
```

Expected: build failures for the new mode, mailbox, facade methods, and
constructor signature.

- [ ] **Step 4: Implement the mailbox and Controller mode**

Define:

```go
type AvatarChange struct {
    Revision uint64
    AvatarID string
}
```

Implement a mutex-owned subscriber registry with capacity-one latest-value
replacement. Publish it from `/avatar/change` before the best-effort diagnostic
event. Saturate revision at `math.MaxUint64`; even at saturation, publishing a
new message replaces the pending channel value.

Add `CatalogMode` to `ControllerConfig`. Replace Controller's direct sender
catalog/source manipulation with Task 1 runtime calls. In query mode retain
current refresh/retry behavior. In external mode, polling verifies VRChat
health but never compiles a catalog; disconnect clears the target but retains
the external runtime; reconnect resets change detection.

- [ ] **Step 5: Replace the service facade safely**

Make `NewOSCService(config ControllerConfig) (OSCService, error)` return
construction errors instead of calling `log.Fatal`. Define the exact interface:

```go
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

Remove the panicking `SetEnabled`/`SetTarget` facade methods. Extend
`OSCStatus` with `Running`, `Connected`, `HasTarget`, owned `Target`, and
sanitized `LastError`. Ensure all existing Controller tests construct query mode
explicitly only where the zero default is unclear.

- [ ] **Step 6: Run package, race, and compatibility GREEN**

```powershell
gofmt -w internal/osc/avatar_changes.go internal/osc/avatar_changes_test.go internal/osc/controller.go internal/osc/controller_test.go internal/osc/service.go internal/osc/service_test.go
go test ./internal/osc -run 'Test(AvatarChanges|ControllerExternalCatalog|OSCService)' -count=100
go test -race ./internal/osc -count=20
go test ./... -count=1
go vet ./internal/osc
git diff --check
```

- [ ] **Step 7: Commit external mode**

```powershell
git add internal/osc/avatar_changes.go internal/osc/avatar_changes_test.go internal/osc/controller.go internal/osc/controller_test.go internal/osc/service.go internal/osc/service_test.go
git commit -m "feat(osc): support application-owned avatar catalogs"
```

---

### Task 3: Define Application Configuration and Bounded Status

**Files:**
- Create: `internal/application/config.go`
- Create: `internal/application/config_test.go`
- Create: `internal/application/status.go`
- Create: `internal/application/status_test.go`

**Interfaces:**
- Consumes: approved package config types and Task 2 `osc.OSCStatus`.
- Produces: exported `Config`, duration defaults, `LifecycleState`, `PluginControlFailure`, and `Status`.
- Produces: package-private `normalizedConfig` and `statusStore` for Tasks 4–6.

- [ ] **Step 1: Write Config RED tests**

Table-test exact rules:

```text
zero FrameInterval          -> 10ms
zero PluginControlTimeout   -> 2s
negative duration           -> ErrInvalidConfig
empty Avatar.OSCRoot        -> ErrInvalidConfig wrapping avatar error
empty plugin builtin root   -> ErrInvalidConfig wrapping plugin error
empty store path/max bytes  -> ErrInvalidConfig
invalid processing config   -> ErrInvalidConfig wrapping processing error
caller maps/slices          -> normalized config owns copies
OSC CatalogMode input       -> normalized to CatalogExternal
```

Construction tests must prove config validation starts no goroutine, process,
listener, or filesystem write.

- [ ] **Step 2: Write Status RED tests**

Assert `Status()` and every subscriber value own a fresh
`PluginFailures` slice. Subscribe two contexts, require immediate created
status, publish more revisions than channel capacity without reading, and
assert each receives only the newest revision. Cancel one subscriber and prove
its channel closes while the other remains live.

- [ ] **Step 3: Run tests to verify RED**

```powershell
go test ./internal/application -run 'Test(Config|Status)' -count=1
```

Expected: build failure because config/status types do not exist.

- [ ] **Step 4: Implement exact exported values**

Copy the `Config`, defaults, lifecycle constants, `PluginControlFailure`, and
`Status` declarations verbatim from the approved design. Add:

```go
var ErrInvalidConfig = errors.New("application: invalid config")

func normalizeConfig(Config) (normalizedConfig, error)

type statusStore struct {
    mu          sync.Mutex
    now         func() time.Time
    current     Status
    subscribers map[chan Status]struct{}
}

func newStatusStore(now func() time.Time) *statusStore
func (*statusStore) snapshot() Status
func (*statusStore) update(func(*Status)) Status
func (*statusStore) subscribe(context.Context) <-chan Status
```

The created snapshot has revision 1 and the injected current time. Every update
increments a saturating positive revision, obtains a fresh time from `now`,
deep-copies failure slices, and offers latest without blocking. Cancellation
and send/close use the same mutex.

- [ ] **Step 5: Run stress and race GREEN**

```powershell
gofmt -w internal/application/config.go internal/application/config_test.go internal/application/status.go internal/application/status_test.go
go test ./internal/application -run 'Test(Config|Status)' -count=200
go test -race ./internal/application -run 'TestStatus' -count=50
go vet ./internal/application
git diff --check
```

- [ ] **Step 6: Commit config/status**

```powershell
git add internal/application/config.go internal/application/config_test.go internal/application/status.go internal/application/status_test.go
git commit -m "feat(application): define M6 config and status"
```

---

### Task 4: Implement Fail-Closed Plan Installation

**Files:**
- Create: `internal/application/plan_adapter.go`
- Create: `internal/application/install.go`
- Create: `internal/application/install_test.go`

**Interfaces:**
- Consumes: M5 `avatar.Result`/`Plan`, plugin Manager controls, tracking generation control, Task 2 OSC runtime, and Task 3 status values.
- Produces: testable `activationPlanner`, `planView`, `activation`, `planInstaller`, and `installOutcome` used by Task 5.

- [ ] **Step 1: Write exact-order ready-plan RED tests**

Use a fake `planView` with generation 9, a two-binding catalog, evaluator,
and capability projections. Supply plugin snapshots in unsorted order:

```text
vendor.eye        Eye
vendor.expression Expression
vendor.none       Lip
```

Record every fake call and require:

```text
osc.clear
tracking.generation:9
plugin.active:vendor.eye:false
plugin.active:vendor.expression:false
plugin.active:vendor.none:false
plugin.subscription:vendor.eye:9
plugin.active:vendor.eye:true
plugin.subscription:vendor.expression:9
plugin.active:vendor.expression:true
osc.install:9
```

The trace must be sorted by plugin ID regardless of Manager order. Verify no
Enabled preference method is called.

- [ ] **Step 2: Write fail-closed RED tables**

Cover:

```text
failed non-nil plan       -> clear, generation advance, deactivate all, no install
ready-empty plan          -> clear, generation advance, deactivate all, no install
subscription error        -> plugin stays inactive, later plugins continue, catalog installs
activation error          -> failure recorded, later plugins continue, catalog installs
tracking generation error -> clear, deactivate all, no subscription/activation/install
nil exhausted plan        -> clear, deactivate all, terminal outcome, no generation/install
```

Give every fake plugin control a context assertion and a blocked-control row
that ends at `PluginControlTimeout` without blocking the next plugin.

- [ ] **Step 3: Run installer tests to verify RED**

```powershell
go test ./internal/application -run 'TestPlanInstaller' -count=1
```

Expected: build failure for missing installer symbols.

- [ ] **Step 4: Implement the adapter and narrow seams**

Define:

```go
type planView interface {
    Generation() uint64
    Status() avatar.Status
    AvatarID() string
    ConfigID() string
    ConfigPath() string
    Source() avatar.Source
    ParameterIDs() []parameters.ParameterID
    Catalog() *osc.Catalog
    Evaluator() *evaluator.Plan
    SubscriptionFor(trackingmodel.Capability) (pluginapi.Subscription, bool)
}

type activation struct {
    plan planView
    err  error
}

type activationPlanner interface { Activate(string) activation }
```

Wrap the real `avatar.Planner` without copying plan internals. Define narrowed
plugin, tracking, and OSC control interfaces so tests do not implement unrelated
methods.

- [ ] **Step 5: Implement deterministic installation**

Define:

```go
type installOutcome struct {
    plan           planView
    planErr        error
    runtimeErr     error
    pluginFailures []PluginControlFailure
    exhausted      bool
    catalogReady   bool
}

func (i *planInstaller) install(context.Context, activation) installOutcome
```

Clear first. For nil plans, deactivate all and mark exhausted. For non-nil
plans, advance generation before any plugin controls. On advance failure,
deactivate and return. Otherwise deactivate every stable-sorted plugin, then
project/update/activate matches. Give each control its own timeout context.
Install only ready, non-empty catalogs after all controls. Preserve the
original plan error separately from runtime/control errors.

- [ ] **Step 6: Run installer stress/race GREEN**

```powershell
gofmt -w internal/application/plan_adapter.go internal/application/install.go internal/application/install_test.go
go test ./internal/application -run 'TestPlanInstaller' -count=200
go test -race ./internal/application -run 'TestPlanInstaller' -count=50
go vet ./internal/application
git diff --check
```

- [ ] **Step 7: Commit installer**

```powershell
git add internal/application/plan_adapter.go internal/application/install.go internal/application/install_test.go
git commit -m "feat(application): install avatar plans fail closed"
```

---

### Task 5: Build the Serialized Coordinator and Frame Loop

**Files:**
- Create: `internal/application/coordinator.go`
- Create: `internal/application/coordinator_test.go`
- Create: `internal/application/clock.go`
- Create: `internal/application/clock_test.go`

**Interfaces:**
- Consumes: Task 4 installer/plan seams, tracking merged/plugin events, processing Pipeline, evaluator snapshots, and Task 2 OSC publish/runtime APIs.
- Produces: `coordinator.run(context.Context, coordinatorInputs)`, ready/done synchronization, recovery state, and monotonic Host clock used by Task 6.

- [ ] **Step 1: Write monotonic clock RED tests**

Inject wall times `100`, `90`, `100`, `101` and require output
`100`, `100`, `100`, `101`; after a new processing action requiring strict
advance, require `last+1` unless saturated. Cover `math.MaxInt64` without wrap.

- [ ] **Step 2: Write frame RED tests**

Use channels for merged values and manual ticks plus fake pipeline/runtime.
Assert:

- a new same-generation merged frame processes immediately;
- a tick reprocesses the latest identical frame at a later monotonic time;
- old/future/no-plan frames do not call processing or publish;
- failed and ready-empty plans do not process;
- success calls evaluator then `Publish(planGeneration, snapshot)`;
- processing/publish failure clears runtime and marks degraded;
- the next successful frame reinstalls the current catalog before publish and
  clears the recoverable runtime error.

- [ ] **Step 3: Write control-event RED tests**

Block the first fake planner activation. Offer `avtr_a`, then while blocked
replace the mailbox with `avtr_b`, repeated `avtr_b`, and `avtr_c`. Release the
first call and require activation order `avtr_a`, `avtr_c`. In a second case,
offer the same ID during the in-flight call and require two activations.

Publish plugin lifecycle events and assert `tracking.RemoveSource` for
non-running or inactive snapshots, but not for a running active snapshot.

- [ ] **Step 4: Run coordinator tests to verify RED**

```powershell
go test ./internal/application -run 'Test(Coordinator|MonotonicClock)' -count=1
```

Expected: build failure for coordinator/clock symbols.

- [ ] **Step 5: Implement the single event loop**

Define:

```go
type coordinatorInputs struct {
    avatarChanges <-chan osc.AvatarChange
    oscEvents     <-chan osc.ControllerEvent
    pluginEvents  <-chan plugins.Event
    merged        <-chan tracking.MergedFrame
    ticks         <-chan time.Time
}

type coordinator struct {
    planner   activationPlanner
    installer *planInstaller
    pipeline  framePipeline
    tracking  sourceRemover
    runtime   runtimePublisher
    status    *statusStore
    clock     *monotonicClock
    current   planView
    latest    tracking.MergedFrame
    hasLatest bool
    suspended bool
}

func (c *coordinator) run(ctx context.Context, inputs coordinatorInputs, ready chan<- struct{})
```

Use one `select` loop only. Planning/installation runs synchronously. Because
the Avatar channel is capacity one, pending events coalesce while the loop is
busy. On frame/tick, require current ready non-empty plan and exact generation;
call Pipeline, evaluator, and OSC in order. On recovery, reinstall the current
catalog before publishing. Consume OSC events only to update status; never use
`EventCatalogUpdated` as an Application binding source.

- [ ] **Step 6: Verify timing, stress, race, and cancellation**

```powershell
gofmt -w internal/application/coordinator.go internal/application/coordinator_test.go internal/application/clock.go internal/application/clock_test.go
go test ./internal/application -run 'Test(Coordinator|MonotonicClock)' -count=200
go test -race ./internal/application -run 'TestCoordinator' -count=50
go vet ./internal/application
git diff --check
```

Assert cancellation closes `done`, no callback occurs afterward, and all test
goroutines use buffered result channels rather than `testing.T` calls.

- [ ] **Step 7: Commit the coordinator**

```powershell
git add internal/application/coordinator.go internal/application/coordinator_test.go internal/application/clock.go internal/application/clock_test.go
git commit -m "feat(application): run avatar-aware frame coordinator"
```

---

### Task 6: Compose Production Construction and Lifecycle

**Files:**
- Modify: `internal/application/app.go`
- Create: `internal/application/app_test.go`

**Interfaces:**
- Consumes: Tasks 2–5 and existing production constructors.
- Produces: complete `NewApp(Config) (*Application, error)`, `Start`, `Close`, `Status`, and `SubscribeStatus` behavior.

- [ ] **Step 1: Write construction RED tests**

Use an unexported `applicationDependencies` factory seam. Assert `NewApp`
constructs, in order, real-compatible tracking, frame sink, plugin Manager,
processing Pipeline, avatar Planner, external-mode OSC service, installer, and
coordinator. Force failure at every factory and assert no Start/Close method or
goroutine ran and the error names the owning component.

- [ ] **Step 2: Write lifecycle RED tests**

With fakes, assert the exact successful trace:

```text
subscribe.avatar
subscribe.osc
subscribe.plugins
subscribe.merged
plugins.start
coordinator.start
coordinator.ready
osc.start
```

Assert OSC-start failure produces `coordinator.cancel`, `coordinator.join`,
then `plugins.close`, with joined rollback errors. Add Manager-start and
coordinator-start failures. Close must trace `osc.close`,
`coordinator.cancel`, `coordinator.join`, `plugins.close`; repeated Close
returns the same joined error, and Start after close returns
`ErrInvalidLifecycle`.

- [ ] **Step 3: Run lifecycle tests to verify RED**

```powershell
go test ./internal/application -run 'TestApplication' -count=1
```

Expected: failures because the current Application only constructs/starts OSC
and has no config, coordinator, rollback, or status lifecycle.

- [ ] **Step 4: Implement production factories and Application state**

Construct:

```go
trackingService := tracking.NewService()
sink := tracking.NewPluginFrameSink(trackingService)
catalog, err := plugins.NewDirectoryCatalog(config.PluginCatalog)
store, err := plugins.NewJSONStore(config.PluginStorePath, config.PluginStoreMaxBytes)
manager, err := plugins.NewManager(catalog, store, plugins.NewProcessLauncher(), sink, config.PluginOptions)
pipeline, err := processing.NewPipeline(config.Processing)
planner, err := avatar.NewPlanner(config.Avatar)
config.OSC.CatalogMode = osc.CatalogExternal
oscService, err := osc.NewOSCService(config.OSC)
```

Store only successfully constructed owned values. Construction starts no
workers. Keep the dependency seam package-private and compile-time assert real
types satisfy its narrow interfaces.

- [ ] **Step 5: Implement Start/Close and status accessors**

Create the child context/subscriptions before Manager Start. Start Manager,
then coordinator and wait for ready, then OSC. Reconcile initial plugin
snapshots after coordinator readiness. On every failure, unwind only started
components in reverse order and join errors. Close OSC, coordinator, then
Manager; cache its result for idempotent callers. Forward `Status` and
`SubscribeStatus` to Task 3's store.

- [ ] **Step 6: Run lifecycle stress/race and cross-package GREEN**

```powershell
gofmt -w internal/application/app.go internal/application/app_test.go
go test ./internal/application -run 'TestApplication' -count=200
go test -race ./internal/application -run 'TestApplication' -count=50
go test ./internal/application ./internal/plugins ./internal/tracking ./internal/processing ./internal/evaluator ./internal/avatar ./internal/osc -count=10
go vet ./internal/application
git diff --check
```

- [ ] **Step 7: Commit lifecycle composition**

```powershell
git add internal/application/app.go internal/application/app_test.go
git commit -m "feat(application): compose M6 backend lifecycle"
```

---

### Task 7: Prove Avatar-Aware End-to-End Output

**Files:**
- Modify: `internal/application/app_test.go`
- Create: `internal/application/integration_test.go`

**Interfaces:**
- Consumes: the public Application behavior and the package-private deterministic dependency seam.
- Produces: executable evidence that current-generation plugin data reaches only current-avatar bindings without a real VRChat or external plugin process.

- [ ] **Step 1: Build the real-component integration fixture**

Use real `avatar.Planner`, `tracking.Service`, `tracking.PluginFrameSink`,
`processing.Pipeline`, evaluator plans, parameter definitions, and compiled OSC
catalogs. Use an in-memory Manager exposing one enabled running Expression
plugin and an in-memory OSC runtime that records installed catalogs and
published `ValueSource`s.

Create `OSC/usr_test/Avatars/avtr_one.json` with JawOpen and
ExpressionTrackingActive bindings. Start Application, offer Avatar change 1,
wait for subscription generation 1 and active true, then submit:

```go
frame := trackingmodel.TrackingFrame{
    Sequence:     1,
    Capabilities: trackingmodel.CapabilityExpression,
}
frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.75)
sink.Submit("vendor.expression", 1, frame)
```

Assert the captured generation-1 source exposes JawOpen `0.75` and active
true, and no unbound parameter.

- [ ] **Step 2: Add switch, failure, and late-data assertions**

Create `avtr_two.json` with a different requested expression/binding. Switch to
generation 2 and prove the installed catalog/source has only generation-2 IDs.
Submit a late generation-1 frame and directly attempt a stale generation-1 OSC
publish; assert tracking/runtime rejection and no captured old output.

Replace generation-2 config with malformed JSON and repeat the Avatar event.
Require generation 3 failed status, OSC clear, plugin inactive, and no catalog
or source. Restore valid JSON, repeat again, and prove generation 4 recovers.

- [ ] **Step 3: Add active-timeout/ticker proof**

Drive the injected clock/tick beyond `Processing.ActiveStaleAfter` without a new
plugin frame. Assert the next published snapshot changes
ExpressionTrackingActive to false while preserving only values allowed by M4
dropout policy.

- [ ] **Step 4: Run integration and race GREEN**

```powershell
gofmt -w internal/application/app_test.go internal/application/integration_test.go
go test ./internal/application -run 'Test.*Avatar.*OSC|Test.*EndToEnd' -count=100
go test -race ./internal/application ./internal/osc ./internal/tracking -count=20
go test ./... -count=1
go vet ./...
git diff --check
```

- [ ] **Step 5: Review dependency direction and commit**

```powershell
go list -deps ./internal/evaluator | Select-String 'github.com/wzhqwq/vrcft-go/internal/osc|github.com/wzhqwq/vrcft-go/internal/application'
go list -deps ./internal/processing | Select-String 'github.com/wzhqwq/vrcft-go/internal/osc|github.com/wzhqwq/vrcft-go/internal/application'
rg -n 'frontend|wails|numeric.*lip|Expression.*Lip' internal/application internal/osc
```

Expected: dependency commands have no matches; source inspection contains no
Wails/frontend imports or new numeric Lip mapping.

```powershell
git add internal/application/app_test.go internal/application/integration_test.go
git commit -m "test(application): prove avatar-aware OSC pipeline"
```

---

### Task 8: Register M6 Ownership and Executable Evidence

**Files:**
- Modify: `docs/project/packages/internal-application.md`
- Modify: `docs/project/packages/internal-osc.md`
- Modify: `docs/project/subsystems/end-to-end.md`
- Modify: `docs/project/packages/internal-plugins.md`
- Modify: `docs/project/packages/internal-tracking.md`
- Modify: `docs/project/packages/internal-processing.md`
- Modify: `docs/project/packages/internal-evaluator.md`

**Interfaces:**
- Consumes: reviewed Tasks 1–7 and exact test symbols.
- Produces: authoritative M6 completion checks without claiming root Wails/M7 completion.

- [ ] **Step 1: Update internal-application checks and contract**

Replace the placeholder build-only evidence with required checks equivalent to:

```yaml
checks:
  - id: package-tests
    description: Application composition tests pass
    type: command
    command: go-test
    args: [./internal/application]
    weight: 3
    required: true
  - id: package-race-tests
    description: Application coordinator is race-free
    type: command
    command: go-test-race
    args: [./internal/application]
    weight: 2
    required: true
  - id: tracking-wired
    description: Application runs the tracking pipeline
    type: symbol
    path: internal/application/coordinator.go
    pattern: '(?m)^func \(c \*coordinator\) run\('
    weight: 3
    required: true
```

Document explicit config, external catalog ownership, fail-closed installation,
coordinator scheduling, lifecycle rollback, bounded status, and the remaining
M7 root/config/UI gap.

- [ ] **Step 2: Update OSC and end-to-end checks**

Add a required structural test check to `internal-osc.md` for
`TestControllerExternalCatalogModeDoesNotRefreshCatalog` and describe the
generation-fenced runtime/mailbox. In `end-to-end.md`, keep the existing
component aggregate and make `integration-test` match the exact Task 7 test
name. State that M6 composition is implemented while real Wails construction
and persisted config remain M7.

- [ ] **Step 3: Close adjacent package gap text**

Update only stale M6 sentences in plugins, tracking, processing, and evaluator
specs. Preserve their non-responsibilities and state that Application wiring is
implemented without moving algorithm ownership or claiming numeric Lip work.

- [ ] **Step 4: Run catalog and symbol verification**

```powershell
go test ./internal/projectstatus -count=10
go run ./cmd/projectstatus
rg -n 'func \(c \*coordinator\) run|func TestControllerExternalCatalogModeDoesNotRefreshCatalog|func Test.*Avatar.*OSC|func Test.*EndToEnd' internal/application internal/osc
git diff --check
```

Expected: M6 checks pass in the live report; global state may remain incomplete
only because M7 root/frontend/configuration checks remain.

- [ ] **Step 5: Commit authoritative specs**

```powershell
git add docs/project/packages/internal-application.md docs/project/packages/internal-osc.md docs/project/subsystems/end-to-end.md docs/project/packages/internal-plugins.md docs/project/packages/internal-tracking.md docs/project/packages/internal-processing.md docs/project/packages/internal-evaluator.md
git commit -m "docs: specify M6 end-to-end composition"
```

---

### Task 9: Complete Verification, Whole-Range Review, and Generated Status

**Files:**
- Modify: production/tests/specs only for a focused review finding proven by RED.
- Modify: `docs/project/status.md` only through `go run ./cmd/projectstatus -write` after source review closes.

**Interfaces:**
- Consumes: clean reviewed Tasks 1–8.
- Produces: final formatting/normal/race/vet evidence, whole-range review closure, generated M6 status, and a clean tracked worktree.

- [ ] **Step 1: Run focused formatting and M6 verification**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
$files = Get-ChildItem internal/application,internal/osc -Filter '*.go' -File
gofmt -d ($files.FullName)
go test ./internal/application ./internal/osc -count=20
go test -race ./internal/application ./internal/osc -count=10
go vet ./internal/application ./internal/osc
git diff --check
```

Expected: no gofmt output; all commands exit 0.

- [ ] **Step 2: Run complete repository verification**

```powershell
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/projectstatus
git status --short
```

Expected: Go commands exit 0; projectstatus reports M6 complete while M7 may
keep the global state incomplete; tracked worktree is clean before review. If
restricted Windows plugin/process tests fail only with access denied, rerun the
exact command outside the sandbox and record both results.

- [ ] **Step 3: Request one whole-range code review**

Use `superpowers:requesting-code-review` from design commit `b300b5a` through
the Task 8 implementation/spec HEAD. The reviewer must explicitly check:

- external mode never accepts OSCQuery catalog ownership;
- Clear synchronizes with sends and generation fences every published source;
- Avatar latest-value behavior preserves a repeated reload and final latest ID;
- plan installation ordering and partial plugin failures are fail-closed;
- generation exhaustion and tracking-advance failure cannot restore output;
- plugin loss removes tracking sources;
- immediate/tick processing and recovery obey M4 state semantics;
- startup rollback/reverse shutdown and status ownership are race-free;
- end-to-end tests use real planning/tracking/processing/evaluation/bindings;
- dependency direction and M7 scope remain intact; and
- project specs do not overclaim Wails/configuration/frontend completion.

For each Critical or Important finding, write a focused failing test, prove RED,
make the smallest owning fix, run affected normal/race checks, commit, and
request scoped re-review. Finish with no open Critical or Important finding.

- [ ] **Step 4: Generate status from the reviewed clean source commit**

Confirm `git status --short` is empty, then run:

```powershell
go run ./cmd/projectstatus -write
git diff -- docs/project/status.md
```

The report must identify the reviewed source commit, say `Dirty: false`, show
M6 and its Application/end-to-end/OSC checks complete, retain only accurate M7
gaps, and contain no stale M6 wording.

- [ ] **Step 5: Commit evidence and check freshness**

```powershell
git add docs/project/status.md
git commit -m "docs: refresh M6 completion evidence"
go run ./cmd/projectstatus -check
git status --short
git log -14 --oneline --decorate
```

The freshness output must not say `stale`. The final tracked worktree must be
clean. Perform a final task-scoped review of the generated status diff; do not
repeat the already closed whole-range code review.

---

## Plan Completion Gate

Do not mark M6 complete from symbol checks alone. Completion requires all nine
tasks, task-level RED/GREEN evidence, fixed-cache normal/race/vet verification,
one whole-range review with no open Critical or Important finding, accurate
package/subsystem specs, generated clean-source status showing M6 complete, and
a clean tracked worktree. Root Wails construction, persisted configuration,
frontend diagnostics, numeric Lip payloads, and Expression-to-Lip mapping remain
M7 or later work.
