# Plugin API v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a validated single-device plugin API v1 whose mixed-granularity subscriptions pass through protocol v1 and the plugin runtime, while proving every float OSC parameter has a valid tracking-input dependency path.

**Architecture:** `pkg/trackingmodel` owns primitive tracking values and masks; new `internal/parameterdeps` maps the YAML parameter catalog onto those primitives without an import cycle. `pkg/pluginapi` defines the driver-facing lifecycle, `pkg/protocol` defines typed process messages, and `pkg/pluginruntime` maps the protocol to a bounded concurrent host environment and trims frames before IPC.

**Tech Stack:** Go 1.25.6, standard library contexts/channels/JSON, `gopkg.in/yaml.v3`, table-driven tests, Go race detector.

## Global Constraints

- Breaking the provisional Go API is allowed; the project has no release or third-party compatibility commitment.
- One plugin process represents one device and one logical tracking source.
- Capability bits provide coarse subscription; a zero detail mask means the whole enabled group.
- Frame publication is non-blocking and latest-only.
- Control events are ordered, bounded, and never silently discarded.
- Every YAML float parameter resolves acyclically to valid primitive tracking inputs.
- Concrete IPC transport, host supervision, avatar loading, planning, merge, processing, and OSC evaluation remain out of scope.
- Use red-green-refactor TDD and a focused commit for every task.

## File Map

- `pkg/trackingmodel/expression.go`: complete expression IDs and set accessors.
- `pkg/trackingmodel/mask.go`: fixed-width expression mask.
- `internal/parameterdeps/dependencies.go`: OSC parameter dependency graph.
- `pkg/pluginapi/plugin.go`, `describer.go`, `command.go`, `log.go`: public v1 lifecycle and values.
- `pkg/pluginapi/subscription.go`: normalization, membership, and frame trimming.
- `pkg/protocol/message.go`, `connection.go`: typed protocol v1.
- `pkg/pluginruntime/runtime.go`, `frame.go`, `main.go`: plugin-side runtime.
- Matching `_test.go` files: behavioral, compatibility, integration, and race evidence.

---

### Task 1: Complete primitive expressions and masks

**Files:**
- Modify: `pkg/trackingmodel/frame.go`
- Create: `pkg/trackingmodel/expression.go`
- Create: `pkg/trackingmodel/mask.go`
- Create: `pkg/trackingmodel/expression_test.go`

**Interfaces:**
- Produces: `ExpressionID`, `ExpressionCount`, `ExpressionMask`, and safe `ExpressionSet` accessors.
- Removes: unused duplicate `MaxExpressionCount`, `ExpressionData`, `EyeData`, `HeadValid`, and `HeadData`.

- [ ] **Step 1: Write failing expression identity and mask tests**

```go
func TestExpressionMaskAndSet(t *testing.T) {
    var mask ExpressionMask
    if !mask.Set(ExpressionJawOpen) || !mask.Has(ExpressionJawOpen) {
        t.Fatal("JawOpen was not retained")
    }
    if mask.Set(ExpressionCount) { t.Fatal("accepted ExpressionCount") }

    var set ExpressionSet
    if !set.Set(ExpressionJawOpen, .75) { t.Fatal("rejected JawOpen") }
    got, ok := set.Get(ExpressionJawOpen)
    if !ok || got != .75 { t.Fatalf("Get = %v, %v", got, ok) }
    set.Clear(ExpressionJawOpen)
    if _, ok := set.Get(ExpressionJawOpen); ok { t.Fatal("Clear retained value") }
}
```

Also require `ExpressionNames()` to contain exactly `ExpressionCount` unique,
nonempty names, with representative IDs from every facial group.

- [ ] **Step 2: Verify the incomplete model fails**

Run: `go test ./pkg/trackingmodel -run TestExpression`

Expected: FAIL because mask/set methods and most constants do not exist.

- [ ] **Step 3: Implement the complete primitive enum**

Copy the primitive detailed names in YAML order. Exclude eye gaze, eyelid, pupil,
and combined outputs because those live in `EyeSample` or are derived. Begin:

```go
type ExpressionID uint16
const (
    ExpressionEyeSquintRight ExpressionID = iota
    ExpressionEyeSquintLeft
    ExpressionBrowPinchRight
    ExpressionBrowPinchLeft
    ExpressionBrowLowererRight
    ExpressionBrowLowererLeft
    ExpressionBrowInnerUpRight
    ExpressionBrowInnerUpLeft
    ExpressionBrowOuterUpRight
    ExpressionBrowOuterUpLeft
    ExpressionNoseSneerRight
    ExpressionNoseSneerLeft
    ExpressionNasalDilationRight
    ExpressionNasalDilationLeft
    ExpressionNasalConstrictRight
    ExpressionNasalConstrictLeft
    ExpressionCheekSquintRight
    ExpressionCheekSquintLeft
    ExpressionCheekPuffSuckRight
    ExpressionCheekPuffSuckLeft
    ExpressionJawOpen
    ExpressionMouthClosed
    ExpressionJawX
    ExpressionJawZ
    ExpressionJawClench
    ExpressionJawMandibleRaise
    ExpressionLipSuckUpperRight
    ExpressionLipSuckUpperLeft
    ExpressionLipSuckLowerRight
    ExpressionLipSuckLowerLeft
    ExpressionLipSuckCornerRight
    ExpressionLipSuckCornerLeft
    ExpressionLipFunnelUpperRight
    ExpressionLipFunnelUpperLeft
    ExpressionLipFunnelLowerRight
    ExpressionLipFunnelLowerLeft
    ExpressionLipPuckerUpperRight
    ExpressionLipPuckerUpperLeft
    ExpressionLipPuckerLowerRight
    ExpressionLipPuckerLowerLeft
    ExpressionMouthUpperUpRight
    ExpressionMouthUpperUpLeft
    ExpressionMouthLowerDownRight
    ExpressionMouthLowerDownLeft
    ExpressionMouthUpperDeepenRight
    ExpressionMouthUpperDeepenLeft
    ExpressionMouthUpperX
    ExpressionMouthLowerX
    ExpressionMouthCornerPullRight
    ExpressionMouthCornerPullLeft
    ExpressionMouthCornerSlantRight
    ExpressionMouthCornerSlantLeft
    ExpressionMouthDimpleRight
    ExpressionMouthDimpleLeft
    ExpressionMouthFrownRight
    ExpressionMouthFrownLeft
    ExpressionMouthStretchRight
    ExpressionMouthStretchLeft
    ExpressionMouthRaiserUpper
    ExpressionMouthRaiserLower
    ExpressionMouthPressRight
    ExpressionMouthPressLeft
    ExpressionMouthTightenerRight
    ExpressionMouthTightenerLeft
    ExpressionTongueOut
    ExpressionTongueX
    ExpressionTongueY
    ExpressionTongueRoll
    ExpressionTongueArchY
    ExpressionTongueShape
    ExpressionTongueTwistRight
    ExpressionTongueTwistLeft
    ExpressionSoftPalateClose
    ExpressionThroatSwallow
    ExpressionNeckFlexRight
    ExpressionNeckFlexLeft
    ExpressionCount
)
```

Keep an adjacent name array so ID/name stability is testable. Do not retain
`EyeWideRight/Left`; the approved YAML dependency catalog has no direct output
that consumes them.

- [ ] **Step 4: Implement fixed-width masks and bounds-safe set access**

```go
type ExpressionMask struct { Words [(ExpressionCount+63)/64]uint64 }
func ExpressionMaskOf(ids ...ExpressionID) ExpressionMask
func (m ExpressionMask) Has(id ExpressionID) bool
func (m *ExpressionMask) Set(id ExpressionID) bool
func (m ExpressionMask) IsZero() bool
func (m ExpressionMask) Intersect(ExpressionMask) ExpressionMask
func (m ExpressionMask) Normalize() ExpressionMask

type ExpressionSet struct {
    Values [ExpressionCount]float32
    Valid ExpressionMask
}
func (s *ExpressionSet) Set(ExpressionID, float32) bool
func (s ExpressionSet) Get(ExpressionID) (float32, bool)
func (s *ExpressionSet) Clear(ExpressionID) bool
```

- [ ] **Step 5: Verify and commit**

Run: `go test ./pkg/trackingmodel`

Expected: PASS.

```powershell
git add pkg/trackingmodel
git commit -m "feat(trackingmodel): complete expression primitives"
```

---

### Task 2: Prove YAML parameter dependency coverage

**Files:**
- Create: `internal/parameterdeps/dependencies.go`
- Create: `internal/parameterdeps/dependencies_test.go`

**Interfaces:**
- Produces: `Plan(parameters.ParameterID)`, `ResolveLeaves`, and `RequiredInputs` for later avatar planning.

- [ ] **Step 1: Write the failing YAML-wide coverage test**

```go
func TestEveryFloatParameterHasPrimitiveDependencies(t *testing.T) {
    doc, _, err := specparser.LoadFile("../../spec/vrcft_osc_parameters.yaml")
    if err != nil { t.Fatal(err) }
    all := append(doc.DetailedParameters, doc.SimplifiedParameters...)
    for _, item := range all {
        id, ok := parameters.LookupOSCName(item.OSCName)
        if !ok { t.Errorf("%s: generated ID missing", item.Name); continue }
        leaves, err := ResolveLeaves(id)
        if err != nil { t.Errorf("%s: %v", item.Name, err); continue }
        if leaves.IsZero() { t.Errorf("%s: no primitive inputs", item.Name) }
    }
}
```

Add injected-graph tests for missing references and cycles, plus an orphan test
that requires every expression ID to be used by at least one parameter.

- [ ] **Step 2: Verify the missing package fails**

Run: `go test ./internal/parameterdeps`

Expected: FAIL because no dependency catalog exists.

- [ ] **Step 3: Implement dependency graph types**

```go
type Inputs struct {
    Eye trackingmodel.EyeValid
    Expressions trackingmodel.ExpressionMask
}
func (i Inputs) IsZero() bool

type Operation uint8
const (
    OperationDirect Operation = iota + 1
    OperationAverage
    OperationMax
    OperationSignedPair
    OperationSumClamp
)
type DependencyPlan struct {
    Inputs Inputs
    DependsOn []parameters.ParameterID
    Operation Operation
}
func Plan(parameters.ParameterID) (DependencyPlan, bool)
func ResolveLeaves(parameters.ParameterID) (Inputs, error)
func RequiredInputs([]parameters.ParameterID) (Inputs, error)
```

- [ ] **Step 4: Populate all 124 float mappings**

Direct eye plans map gaze/openness/pupil parameters to `EyeValid`; direct
expression plans map one-to-one to the completed enum. Combined detailed and
all 36 simplified outputs reference other parameter IDs. Fix representative
semantics explicitly:

```go
parameters.ParameterEyeLid: {
    DependsOn: []parameters.ParameterID{parameters.ParameterEyeLidRight, parameters.ParameterEyeLidLeft},
    Operation: OperationAverage,
},
parameters.ParameterEyeSquint: {
    DependsOn: []parameters.ParameterID{parameters.ParameterEyeSquintRight, parameters.ParameterEyeSquintLeft},
    Operation: OperationAverage,
},
parameters.ParameterSmileFrownRight: {
    DependsOn: []parameters.ParameterID{
        parameters.ParameterMouthCornerPullRight,
        parameters.ParameterMouthCornerSlantRight,
        parameters.ParameterMouthFrownRight,
    }, Operation: OperationSignedPair,
},
```

Use average for left/right combined values, max for smile composites,
signed-pair for positive/negative outputs, and sum-clamp for `MouthOpen`.
Numeric evaluation remains outside this plan; this task establishes exact input
closure and operation metadata.

- [ ] **Step 5: Verify exact catalog coverage and commit**

Run: `go test ./internal/parameterdeps`

Expected: PASS for 88 detailed plus 36 simplified entries, no cycles, no missing
leaves, and no orphan primitive IDs.

```powershell
git add internal/parameterdeps
git commit -m "feat(parameters): map OSC outputs to tracking inputs"
```

---

### Task 3: Replace the provisional plugin API with v1

**Files:**
- Replace: `pkg/pluginapi/plugin.go`
- Replace: `pkg/pluginapi/describer.go`
- Replace: `pkg/pluginapi/command.go`
- Replace: `pkg/pluginapi/log.go`
- Create: `pkg/pluginapi/plugin_test.go`
- Create: `pkg/pluginapi/example_test.go`

**Interfaces:**
- Produces: `Driver`, `Host`, `Startup`, typed control events, and validated descriptor/config/status/log values.

- [ ] **Step 1: Write failing contract tests**

Cover invalid descriptors, unknown capabilities, semantic version syntax,
config revision rules, JSON deep-copy ownership, device states, log levels, and
compilation of this minimal driver:

```go
type exampleDriver struct{}
func (exampleDriver) Descriptor() pluginapi.Descriptor { return validDescriptor }
func (exampleDriver) Run(ctx context.Context, host pluginapi.Host) error {
    _ = host.Startup()
    for {
        select {
        case <-ctx.Done(): return ctx.Err()
        case event, ok := <-host.Events():
            if !ok { return nil }
            if _, shutdown := event.(pluginapi.ShutdownRequested); shutdown { return nil }
        }
    }
}
```

- [ ] **Step 2: Verify the old contract fails**

Run: `go test ./pkg/pluginapi`

Expected: FAIL because `Host`, `Startup`, validation, and typed events are absent.

- [ ] **Step 3: Implement API v1**

Define `APIVersion = 1`, the approved `Driver`, `Host`, `Startup`, and sealed
`ControlEvent` interface with `ActiveChanged`, `ConfigChanged`,
`SubscriptionChanged`, and `ShutdownRequested`. Implement `Config.Clone` and
field-specific validation. Remove `Command`, `CommandType`, `Environment`, and
`LogStore`.

- [ ] **Step 4: Verify and commit**

Run: `go test ./pkg/pluginapi`

Expected: PASS.

```powershell
git add pkg/pluginapi
git commit -m "feat(pluginapi): define validated v1 driver contract"
```

---

### Task 4: Add mixed-granularity subscriptions and trimming

**Files:**
- Create: `pkg/pluginapi/subscription.go`
- Create: `pkg/pluginapi/subscription_test.go`

**Interfaces:**
- Produces: `Subscription`, normalization, membership, validation, and frame trimming.

- [ ] **Step 1: Write failing subscription tests**

Cover disabled-group mask clearing, zero-mask whole-group behavior, exact masks,
generation-zero rules, unknown bits, capability removal, validity intersection,
and zeroing every rejected scalar/vector/array value.

- [ ] **Step 2: Verify failure**

Run: `go test ./pkg/pluginapi -run 'TestSubscription|TestTrimFrame'`

Expected: FAIL because the subscription implementation is absent.

- [ ] **Step 3: Implement the approved API**

```go
type Subscription struct {
    Generation uint64
    Capabilities trackingmodel.Capability
    Eye trackingmodel.EyeValid
    Expressions trackingmodel.ExpressionMask
}
func (s Subscription) Normalize() Subscription
func (s Subscription) Validate(active bool) error
func (s Subscription) IncludesEye(trackingmodel.EyeValid) bool
func (s Subscription) IncludesExpression(trackingmodel.ExpressionID) bool
func (s Subscription) TrimFrame(trackingmodel.TrackingFrame) trackingmodel.TrackingFrame
```

Trimming returns a value copy. Empty detail masks retain all valid fields of an
enabled group; nonempty masks retain only their intersection.

- [ ] **Step 4: Verify with race detector and commit**

Run: `go test -race ./pkg/pluginapi`

Expected: PASS.

```powershell
git add pkg/pluginapi/subscription.go pkg/pluginapi/subscription_test.go
git commit -m "feat(pluginapi): add selective tracking subscriptions"
```

---

### Task 5: Define typed protocol v1

**Files:**
- Replace: `pkg/protocol/message.go`
- Replace: `pkg/protocol/connection.go`
- Create: `pkg/protocol/message_test.go`

**Interfaces:**
- Produces: protocol v1 messages, validation, JSON round trips, and context-aware `Conn`.

- [ ] **Step 1: Write failing message matrix tests**

Round-trip hello, initialize, ready, heartbeat, tracking frame, status, log,
config, subscription, active, shutdown, shutdown ack, and error. Reject unknown
types, mismatched payloads, version != 1, payloads above 1 MiB, invalid public
values, and missing tracking generation.

- [ ] **Step 2: Verify failure**

Run: `go test ./pkg/protocol`

Expected: FAIL because typed messages and `Conn` are incomplete.

- [ ] **Step 3: Implement envelope and connection contract**

```go
const Version uint16 = 1
const MaxPayloadSize = 1024 * 1024
type Message struct { Version uint16; Type MessageType; Payload any }
type TrackingFrame struct { Generation uint64; Frame trackingmodel.TrackingFrame }
type Conn interface {
    Send(context.Context, Message) error
    Receive(context.Context) (Message, error)
    Close() error
}
```

Use an internal `json.RawMessage` envelope so each message type decodes to
exactly one concrete payload. Keep concrete framing/transport out of this package.

- [ ] **Step 4: Verify and commit**

Run: `go test ./pkg/protocol`

Expected: PASS.

```powershell
git add pkg/protocol
git commit -m "feat(protocol): define typed plugin protocol v1"
```

---

### Task 6: Implement runtime lifecycle and ordered controls

**Files:**
- Replace: `pkg/pluginruntime/runtime.go`
- Replace: `pkg/pluginruntime/main.go`
- Create: `pkg/pluginruntime/runtime_test.go`

**Interfaces:**
- Produces: `RuntimeConfig`, `New`, `Run`, runtime-managed `pluginapi.Host`, and `Main`.

- [ ] **Step 1: Write failing lifecycle tests**

Build a test-only bounded `memoryConn`. Test descriptor validation,
hello/initialize/ready order, atomic startup, ordered controls, duplicate
revision/generation suppression, regressive update errors, one shutdown event,
driver cancellation/error, connection failure, and control queue exhaustion.

- [ ] **Step 2: Verify failure**

Run: `go test ./pkg/pluginruntime -run 'TestRuntime.*(Handshake|Control|Shutdown|Failure)'`

Expected: FAIL because runtime execution is absent.

- [ ] **Step 3: Implement bounded lifecycle**

```go
type RuntimeConfig struct {
    ControlQueue int
    LogQueue int
    HeartbeatInterval time.Duration
    ShutdownTimeout time.Duration
}
func DefaultRuntimeConfig() RuntimeConfig
func New(pluginapi.Driver, protocol.Conn, RuntimeConfig) (*Runtime, error)
func (r *Runtime) Run(context.Context) error
```

Defaults are control 32, logs 256, heartbeat 1 second, shutdown 5 seconds. Use
one cancellation scope. The reader is the only control producer; a full queue
returns `ErrControlBackpressure`. Deep-copy config bytes. Close events once.
Treat expected cancellation as success and propagate all other failures.

- [ ] **Step 4: Implement `Main` boundary**

Use an injectable connection factory for tests. Missing process connection
configuration must produce a nonzero exit, never a silent no-op.

- [ ] **Step 5: Verify with race detector and commit**

Run: `go test -race ./pkg/pluginruntime -run 'TestRuntime.*(Handshake|Control|Shutdown|Failure)'`

Expected: PASS.

```powershell
git add pkg/pluginruntime
git commit -m "feat(pluginruntime): run driver lifecycle and controls"
```

---

### Task 7: Implement selective frames and telemetry

**Files:**
- Replace: `pkg/pluginruntime/frame.go`
- Modify: `pkg/pluginruntime/runtime.go`
- Modify: `pkg/pluginruntime/runtime_test.go`
- Create: `pkg/pluginruntime/integration_test.go`

**Interfaces:**
- Produces: generation-tagged latest-frame delivery, heartbeat, latest status, and bounded logs.

- [ ] **Step 1: Write failing data-path tests**

Test overwrite, nonblocking publication, inactive/empty suppression, exact wire
trimming, stale-generation clearing, heartbeat, latest-only status, log queue
saturation, dropped-log reporting, and send failure. The integration driver
publishes JawOpen plus BrowPinch under a JawOpen-only subscription; only JawOpen
may appear on the wire and the envelope generation must match.

- [ ] **Step 2: Verify failure**

Run: `go test ./pkg/pluginruntime -run 'Test(RuntimeFrame|RuntimeHeartbeat|RuntimeStatus|RuntimeLog|Selective)'`

Expected: FAIL because the slot has no load/generation behavior.

- [ ] **Step 3: Implement generation-tagged slot and writers**

```go
type pendingFrame struct { Generation uint64; Frame trackingmodel.TrackingFrame }
func NewLatestFrameSlot() *LatestFrameSlot
func (s *LatestFrameSlot) Store(pendingFrame) bool
func (s *LatestFrameSlot) Load() (pendingFrame, bool)
func (s *LatestFrameSlot) ClearBefore(uint64)
func (s *LatestFrameSlot) Notify() <-chan struct{}
```

Trim immediately before send. Status uses one latest-value slot. Logs use the
configured queue and attach a dropped count to the next sent record. Runtime,
not driver, emits heartbeat. Any send error cancels the runtime.

- [ ] **Step 4: Run all scoped verification**

Run: `go test ./pkg/trackingmodel ./internal/parameterdeps ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime`

Run: `go test -race ./pkg/trackingmodel ./internal/parameterdeps ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime`

Run: `go vet ./pkg/trackingmodel ./internal/parameterdeps ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime`

Expected: all exit 0.

- [ ] **Step 5: Commit data path**

```powershell
git add pkg/pluginruntime
git commit -m "feat(pluginruntime): deliver selectively trimmed frames"
```

---

### Task 8: Correct package specifications and repository integration

**Files:**
- Modify: `docs/project/packages/pkg-trackingmodel.md`
- Modify: `docs/project/packages/internal-parameters.md`
- Modify: `docs/project/packages/pkg-pluginapi.md`
- Modify: `docs/project/packages/pkg-protocol.md`
- Modify: `docs/project/packages/pkg-pluginruntime.md`
- Modify: `internal/plugins/manager.go`

**Interfaces:**
- Produces: honest executable completion checks and a compiling adjacent consumer.

- [ ] **Step 1: Fix the adjacent log type reference**

Import `pkg/pluginapi` in `internal/plugins/manager.go` and change the event
field to `Log *pluginapi.LogEntry`. Do not implement host supervision here.

- [ ] **Step 2: Replace symbol checks with behavioral evidence**

Require YAML dependency coverage, package tests, race tests, protocol round
trips, example-driver compilation, and selective runtime integration. Remove
the current `Driver`-symbol-only completion evidence.

- [ ] **Step 3: Run final verification**

Run: `go test -race ./pkg/trackingmodel ./internal/parameterdeps ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime`

Run: `go vet ./pkg/trackingmodel ./internal/parameterdeps ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime`

Run: `go test ./...`

Expected: every command exits 0 before completion is claimed.

- [ ] **Step 4: Regenerate project evidence**

Run: `go run ./cmd/projectstatus -write`

Expected: it writes `docs/project/status.md`; exit 1 is acceptable while
unrelated milestones remain incomplete, but no catalog/tool error is allowed.

- [ ] **Step 5: Check and commit**

Run: `git diff --check`

Expected: no whitespace errors.

```powershell
git add internal/plugins/manager.go docs/project
git commit -m "docs: require behavioral plugin API evidence"
```

## Final Acceptance

- All 88 detailed and 36 simplified float parameters resolve to valid primitive inputs.
- No dependency cycle, missing mapping, or orphan expression ID remains.
- A sample vendor driver compiles only against public tracking/plugin packages.
- Protocol v1 rejects malformed, oversized, regressive, and type-mismatched messages.
- Runtime controls are ordered and bounded; frames are latest-only and precisely trimmed.
- Subscription generation prevents old-avatar frames from being treated as current.
- Scoped race tests, vet, and full `go test ./...` pass.
