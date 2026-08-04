# M4 Processing and Selective Parameter Evaluation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the tracking path with independent Lip activity metadata, then build a deterministic processing Pipeline and compiled selective parameter evaluator that complete M4 without introducing Application wiring or background services.

**Architecture:** Existing plugin frames continue through M3, which gains independent Lip routing and per-group Host freshness. A caller-serialized `processing.Pipeline` transforms each `tracking.MergedFrame` into a value-only `CanonicalFrame`; an immutable `evaluator.Plan` computes only requested parameters into a dense Snapshot that structurally satisfies OSC's source interface.

**Tech Stack:** Go, fixed-size `pkg/trackingmodel` values, `internal/tracking`, `internal/parameterdeps`, `internal/parameters`, table-driven tests, Go race detector, and generated project-status checks.

## Global Constraints

- Work in the existing checkout on `master`; the user previously declined worktree isolation.
- Every temporary artifact, `TMP`, `TEMP`, `GOCACHE`, build output, brief, report, and review package must stay below `F:\dev\vrcft-go\.superpowers\tmp\2026-08-04-m4-processing-evaluator\`.
- Subagents must not run Git commands, stage, commit, request permissions, use the network, install/download tools, or write outside the repository. The primary agent owns Git and permission-bearing operations.
- Use strict RED/GREEN TDD. Create the complete focused test for a behavior before changing its production implementation; record the exact failing command and failure cause.
- Use `GOTOOLCHAIN=local` and `GOTELEMETRY=off` for every Go command.
- Do not add Application construction, subscription loops, tick scheduling, shutdown, avatar loading/plans, OSC network wiring, config persistence/UI, dynamic evaluator operations, numeric Lip payloads, Expression-to-Lip mappings, or shared-memory protocol work.
- `CapabilityEye == 1` and `CapabilityExpression == 2` must not change; `CapabilityLip == 4`.
- Pipeline instances are caller-serialized and start no goroutine. Evaluator Plans are immutable and safe for concurrent evaluation.
- Every task ends with primary-agent verification, an independent spec/quality review, fixes for all Critical/Important findings, and a scoped primary-agent commit.

Use this task-local PowerShell preamble for Go commands, replacing `NN` with the task number:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-04-m4-processing-evaluator\task-NN'
New-Item -ItemType Directory -Force -Path $taskTmp,"$taskTmp\gocache" | Out-Null
$env:TMP=$taskTmp
$env:TEMP=$taskTmp
$env:GOCACHE="$taskTmp\gocache"
$env:GOTOOLCHAIN='local'
$env:GOTELEMETRY='off'
```

## File Map

- Shared Lip contract: capability/model files in `pkg/trackingmodel`, validation/trimming in `pkg/pluginapi`, and compatibility tests in protocol/runtime/plugins.
- M3 extension: `internal/tracking/routing.go`, `frame.go`, `service.go`, `summary.go`, and their existing tests.
- Processing: focused files for channel catalog, immutable config, scalar transforms, filters, validation, Pipeline, dropout, and mutual exclusion.
- Evaluator: `internal/parameterdeps` metadata correction plus new `internal/evaluator` plan, operand, execution, snapshot, and tests.
- Evidence: affected package specs, new evaluator spec, ignored SDD records, and generated `docs/project/status.md`.

---

### Task 1: Add the Shared Metadata-Only Lip Capability

**Files:**
- Modify: `pkg/trackingmodel/capacity.go`
- Modify: `pkg/trackingmodel/frame.go`
- Modify: `pkg/trackingmodel/frame_test.go`
- Modify: `pkg/pluginapi/describer.go`
- Modify: `pkg/pluginapi/subscription.go`
- Modify: `pkg/pluginapi/plugin_test.go`
- Modify: `pkg/pluginapi/subscription_test.go`
- Modify: `pkg/protocol/message_test.go`
- Modify: `pkg/pluginruntime/runtime_test.go`
- Modify: `pkg/pluginruntime/integration_test.go`
- Modify: `internal/plugins/manifest_test.go`
- Modify: `internal/plugins/handshake_test.go`

**Interfaces:**
- Consumes: existing numeric `trackingmodel.Capability`, `TrackingFrame`, descriptor, subscription, and protocol fields.
- Produces: `trackingmodel.CapabilityLip == 4`, accepted by all capability boundaries without a Lip payload.

- [ ] **Step 1: Write model and API RED tests**

```go
func TestCapabilityValuesIncludeStableLipBit(t *testing.T) {
    if CapabilityEye != 1 || CapabilityExpression != 2 || CapabilityLip != 4 {
        t.Fatalf("capabilities = (%d,%d,%d), want (1,2,4)", CapabilityEye, CapabilityExpression, CapabilityLip)
    }
}

func TestTrackingFrameLipOnlyIsCanonicalWithoutPayload(t *testing.T) {
    frame := TrackingFrame{Sequence: 1, Capabilities: CapabilityLip}
    got, err := frame.Canonicalize()
    if err != nil { t.Fatal(err) }
    if got.Capabilities != CapabilityLip || got.Eye != (EyeSample{}) || got.Expressions != (ExpressionSet{}) {
        t.Fatalf("canonical Lip frame = %#v", got)
    }
}
```

In `subscription_test.go`, validate a Lip-only positive generation, then trim a frame containing Lip+Expression through a Lip-only subscription and require exactly Lip capability with an empty ExpressionSet.

- [ ] **Step 2: Write wire/runtime/Host RED tests**

```go
message, err := NewMessage(TrackingFrame{
    Generation: 3,
    Frame: trackingmodel.TrackingFrame{Sequence: 9, Capabilities: trackingmodel.CapabilityLip},
})
if err != nil { t.Fatal(err) }
data, err := json.Marshal(message)
if err != nil { t.Fatal(err) }
var decoded Message
if err := json.Unmarshal(data, &decoded); err != nil { t.Fatal(err) }
payload := decoded.Payload.(TrackingFrame)
if payload.Frame.Capabilities != trackingmodel.CapabilityLip {
    t.Fatalf("capabilities = %d", payload.Frame.Capabilities)
}
```

Add descriptor, manifest, handshake, and runtime publication cases whose only capability is Lip. Preserve unknown-bit cases using `1<<10` or `1<<30`.

- [ ] **Step 3: Run focused RED**

```powershell
go test ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime ./internal/plugins -run 'Test.*Lip' -count=1
```

Expected: build failure because `trackingmodel.CapabilityLip` is undefined. Confirm no production file changed before this run.

- [ ] **Step 4: Implement the minimum shared bit propagation**

```go
const (
    CapabilityEye Capability = 1 << iota
    CapabilityExpression
    CapabilityLip
)
```

Add `CapabilityLip` to `knownCapabilities`, `knownDescriptorCapabilities`, and `knownSubscriptionCapabilities`. Do not add a value field or validity mask. Existing protocol conversion code remains shape-compatible.

- [ ] **Step 5: Run focused and affected verification**

```powershell
go test ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime ./internal/plugins -run 'Test.*Lip' -count=50
go test ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime ./internal/plugins -count=1
go test -race ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime ./internal/plugins -run 'Test.*Lip' -count=10
```

Expected: all commands exit 0 and Lip never introduces Expression validity.

- [ ] **Step 6: Review and commit**

After primary verification and independent review:

```bash
git add pkg/trackingmodel/capacity.go pkg/trackingmodel/frame.go pkg/trackingmodel/frame_test.go pkg/pluginapi/describer.go pkg/pluginapi/subscription.go pkg/pluginapi/plugin_test.go pkg/pluginapi/subscription_test.go pkg/protocol/message_test.go pkg/pluginruntime/runtime_test.go pkg/pluginruntime/integration_test.go internal/plugins/manifest_test.go internal/plugins/handshake_test.go
git commit -m "feat(trackingmodel): add Lip capability metadata"
```

---

### Task 2: Extend M3 with Lip Routing and Group Freshness

**Files:**
- Modify: `internal/tracking/routing.go`
- Modify: `internal/tracking/frame.go`
- Modify: `internal/tracking/service.go`
- Modify: `internal/tracking/summary.go`
- Modify: `internal/tracking/contracts_test.go`
- Modify: `internal/tracking/routing_test.go`
- Modify: `internal/tracking/merge_test.go`
- Modify: `internal/tracking/service_test.go`
- Modify: `internal/tracking/source_test.go`
- Modify: `internal/tracking/summary_test.go`
- Modify: `internal/tracking/subscription_test.go`
- Modify: `internal/tracking/concurrency_test.go`

**Interfaces:**
- Consumes: Task 1 `trackingmodel.CapabilityLip` and existing `sourceState.receivedAtNS`.
- Produces: `RoutingConfig.Lip`, Lip source/availability, and Eye/Expression/Lip group update timestamps.

- [ ] **Step 1: Write routing and freshness RED tests**

```go
func TestLipRoutingIsIndependentAndSticky(t *testing.T) {
    now := int64(100)
    s := newServiceWithClock(func() int64 { now++; return now })
    mustSetGeneration(t, s, 1)
    mustSubmit(t, s, "vendor.z", 1, trackingmodel.TrackingFrame{Sequence:1, Capabilities:trackingmodel.CapabilityLip})
    mustSubmit(t, s, "vendor.a", 1, trackingmodel.TrackingFrame{Sequence:1, Capabilities:trackingmodel.CapabilityLip|trackingmodel.CapabilityExpression})
    got := latestMerged(t, s)
    if got.LipSourceID != "vendor.z" || got.ExpressionSourceID != "vendor.a" {
        t.Fatalf("sources = (%q,%q)", got.LipSourceID, got.ExpressionSourceID)
    }
}
```

With different selected plugins, submit unchanged Eye, Expression, and Lip frames in sequence and assert only the corresponding group timestamp changes. Add manual no-fallback, capability loss, removal, and generation-reset cases. Update every valid RoutingConfig literal to set Lip, normally `SourceSelection{Auto:true}`.

- [ ] **Step 2: Write merged, Summary, and subscription RED tests**

Assert a selected Lip-only update advances merged sequence/publication despite having no numeric payload. Assert non-selected Lip updates publish Summary only. Extend concurrency consistency checks so `LipAvailable` matches capability/source metadata.

- [ ] **Step 3: Run focused RED**

```powershell
go test ./internal/tracking -run 'Test(Lip|GroupFreshness|DefaultRouting|RoutingValidation)' -count=1
```

Expected: build failures for missing Lip and freshness fields.

- [ ] **Step 4: Implement routing and merged metadata**

```go
type RoutingConfig struct {
    Eye SourceSelection `json:"eye"`
    Expression SourceSelection `json:"expression"`
    Lip SourceSelection `json:"lip"`
}
```

Add `LipSourceID`, `EyeUpdatedAtNS`, `ExpressionUpdatedAtNS`, and `LipUpdatedAtNS` to MergedFrame; add Lip source/availability to Summary; add `lipSourceID` to service. Resolve all groups independently and copy the selected source's `receivedAtNS`. Include Lip in the selected-source predicate and include all source/timestamp fields in merged equality. Generation advance clears all three selections/timestamps.

- [ ] **Step 5: Run stress verification**

```powershell
go test ./internal/tracking -run 'Test(Lip|GroupFreshness|Routing|Merge|Summary|Subscriber)' -count=100
go test ./internal/tracking -count=20
go test -race ./internal/tracking -count=20
```

- [ ] **Step 6: Review and commit**

Review must prove Lip loss cannot mutate Expression data/source and same-value selected updates refresh only their group. Then:

```bash
git add internal/tracking/routing.go internal/tracking/frame.go internal/tracking/service.go internal/tracking/summary.go internal/tracking/contracts_test.go internal/tracking/routing_test.go internal/tracking/merge_test.go internal/tracking/service_test.go internal/tracking/source_test.go internal/tracking/summary_test.go internal/tracking/subscription_test.go internal/tracking/concurrency_test.go
git commit -m "feat(tracking): route Lip activity metadata"
```

---

### Task 3: Define Processing Channels and Immutable Configuration

**Files:**
- Create: `internal/processing/channel.go`
- Create: `internal/processing/config.go`
- Create: `internal/processing/errors.go`
- Modify: `internal/processing/calibration.go`
- Delete: `internal/processing/tunning.go`
- Create: `internal/processing/tuning.go`
- Modify: `internal/processing/filter.go`
- Modify: `internal/processing/dropout.go`
- Create: `internal/processing/channel_test.go`
- Create: `internal/processing/config_test.go`

**Interfaces:**
- Consumes: fixed Eye scalar fields and `trackingmodel.ExpressionCount`.
- Produces: stable ChannelID catalog, `Config`, `ChannelConfig`, `FilterConfig`, `DefaultConfig`, and immutable compiled configuration.

- [ ] **Step 1: Write ChannelID coverage RED tests**

```go
func TestExpressionChannelsRoundTripEveryID(t *testing.T) {
    seen := map[ChannelID]struct{}{}
    for id := trackingmodel.ExpressionID(0); id < trackingmodel.ExpressionCount; id++ {
        channel, ok := ExpressionChannel(id)
        if !ok { t.Fatalf("ExpressionChannel(%d) invalid", id) }
        got, ok := channel.ExpressionID()
        if !ok || got != id { t.Fatalf("round trip = %d,%t", got, ok) }
        if _, duplicate := seen[channel]; duplicate { t.Fatalf("duplicate channel %d", channel) }
        seen[channel] = struct{}{}
    }
}
```

Enumerate all ten Eye scalar constants. Assert `AllChannels()` has exactly `10+int(ExpressionCount)` unique stable values and returns a caller-owned slice.

- [ ] **Step 2: Write config validation and ownership RED tests**

```go
tests := []struct{name string; mutate func(*Config); want error}{
    {"nonpositive active stale", func(c *Config){ c.ActiveStaleAfter = 0 }, ErrInvalidDropout},
    {"unknown override", func(c *Config){ c.Overrides = map[ChannelID]ChannelConfig{ChannelID(0xffff):c.DefaultChannel} }, ErrUnknownChannel},
    {"all equal calibration", func(c *Config){ c.DefaultChannel.Calibration = ChannelCalibration{Enabled:true, Min:1, Neutral:1, Max:1, Gain:1} }, ErrInvalidCalibration},
    {"deadzone one", func(c *Config){ c.DefaultChannel.Tuning.Deadzone = 1 }, ErrInvalidTuning},
    {"EMA alpha zero", func(c *Config){ c.DefaultChannel.Filter = FilterConfig{Mode:FilterEMA, EMAAlpha:0} }, ErrInvalidFilter},
    {"One Euro cutoff zero", func(c *Config){ c.DefaultChannel.Filter = FilterConfig{Mode:FilterOneEuro, MinCutoff:0, DerivativeCutoff:1} }, ErrInvalidFilter},
    {"negative hold", func(c *Config){ c.DefaultChannel.Dropout.HoldDuration = -time.Nanosecond }, ErrInvalidDropout},
    {"short exclusion", func(c *Config){ c.MutualExclusion = [][]ChannelID{{ChannelEyeLeftGazeX}} }, ErrInvalidMutualExclusion},
    {"duplicate across groups", func(c *Config){ c.MutualExclusion = [][]ChannelID{{ChannelEyeLeftGazeX,ChannelEyeLeftGazeY},{ChannelEyeLeftGazeX,ChannelEyeRightGazeX}} }, ErrInvalidMutualExclusion},
}
```

After successful `compileConfig`, mutate the input override map and nested exclusion slices and assert the returned compiled configuration is unchanged.

Add duration cases proving ActiveStaleAfter/Channel StaleAfter must be positive,
HoldDuration/DecayDuration must be non-negative, and zero hold/decay are valid.

- [ ] **Step 3: Run RED**

```powershell
go test ./internal/processing -run 'Test(Channel|ExpressionChannels|DefaultConfig|Config)' -count=1
```

Expected: build failures for the missing contracts.

- [ ] **Step 4: Implement stable types and validation**

```go
type ChannelID uint16
const (
    ChannelEyeLeftGazeX ChannelID = iota + 1
    ChannelEyeLeftGazeY
    ChannelEyeRightGazeX
    ChannelEyeRightGazeY
    ChannelEyeLeftOpenness
    ChannelEyeRightOpenness
    ChannelEyeLeftPupilDiameter
    ChannelEyeRightPupilDiameter
    ChannelEyeLeftPupilDilation
    ChannelEyeRightPupilDilation
    channelExpressionBase
)

type Config struct {
    DefaultChannel ChannelConfig
    Overrides map[ChannelID]ChannelConfig
    ActiveStaleAfter time.Duration
    MutualExclusion [][]ChannelID
}

type ChannelConfig struct {
    Calibration ChannelCalibration
    Tuning ChannelTuning
    Filter FilterConfig
    Dropout DropoutPolicy
}

type ChannelCalibration struct {
    Enabled bool
    Neutral float32
    Min float32
    Max float32
    Gain float32
    Invert bool
}

type ChannelTuning struct {
    Deadzone float32
    Gain float32
    Exponent float32
    ClampEnabled bool
    ClampMin float32
    ClampMax float32
}

type FilterConfig struct {
    Mode FilterMode
    EMAAlpha float32
    MinCutoff float32
    Beta float32
    DerivativeCutoff float32
}
```

`ExpressionChannel(id)` maps valid IDs contiguously from `channelExpressionBase`. Add `Enabled` to calibration, `ClampEnabled` to tuning, and EMA/One-Euro parameters to `FilterConfig`. DefaultConfig returns identity transforms, no filter/groups, active/channel stale `500ms`, hold `100ms`, and decay `300ms`.

Compile maps and groups into fixed arrays/copies. Define stable sentinels `ErrInvalidConfig`, `ErrUnknownChannel`, `ErrInvalidCalibration`, `ErrInvalidTuning`, `ErrInvalidFilter`, `ErrInvalidDropout`, `ErrInvalidMutualExclusion`, `ErrInvalidInput`, `ErrGenerationRegression`, `ErrRevisionRegression`, `ErrRevisionConflict`, and `ErrTimeRegression` in `errors.go`; later tasks wrap rather than duplicate them.

- [ ] **Step 5: Verify**

```powershell
go test ./internal/processing -run 'Test(Channel|ExpressionChannels|DefaultConfig|Config)' -count=100
go test ./internal/processing -count=1
```

- [ ] **Step 6: Review and commit**

Review stable IDs, exact defaults, caller-owned config copies, and absence of hot-update APIs. Then:

```bash
git add internal/processing/channel.go internal/processing/config.go internal/processing/errors.go internal/processing/calibration.go internal/processing/tuning.go internal/processing/filter.go internal/processing/dropout.go internal/processing/channel_test.go internal/processing/config_test.go
git rm internal/processing/tunning.go
git commit -m "feat(processing): define compiled channel configuration"
```

---

### Task 4: Implement Calibration and Tuning Primitives

**Files:**
- Modify: `internal/processing/calibration.go`
- Modify: `internal/processing/tuning.go`
- Create: `internal/processing/calibration_test.go`
- Create: `internal/processing/tuning_test.go`

**Interfaces:**
- Consumes: Task 3 ChannelCalibration and ChannelTuning.
- Produces: finite pure scalar transforms used before filtering.

- [ ] **Step 1: Write exact calibration RED tests**

```go
cal := ChannelCalibration{Enabled:true, Min:-2, Neutral:0, Max:4, Gain:2}
tests := []struct{raw, want float32}{{-3,-2},{-1,-1},{0,0},{2,1},{5,2}}
for _, test := range tests {
    got, err := applyCalibration(cal, test.raw)
    if err != nil || got != test.want { t.Fatalf("raw %v = %v,%v", test.raw, got, err) }
}
```

Add disabled identity, invert, both one-sided forms, all-equal rejection, invalid ordering, nonpositive gain, and non-finite input/config cases.

- [ ] **Step 2: Write exact tuning RED tests**

```go
tuning := ChannelTuning{Deadzone:0.2, Gain:2, Exponent:2, ClampEnabled:true, ClampMin:-1, ClampMax:1}
got, err := applyTuning(tuning, 0.6) // 0.6 -> 0.5 -> 1 -> 1 -> 1
if err != nil || got != 1 { t.Fatalf("got %v,%v", got, err) }
```

Test `0.2 -> 0`, negative sign preservation, continuity immediately above deadzone, disabled clamp, and every invalid bound.

- [ ] **Step 3: Run RED**

```powershell
go test ./internal/processing -run 'Test(Calibration|Tuning)' -count=1
```

Expected: build failure for missing transforms.

- [ ] **Step 4: Implement pure transforms**

Calibration clamps, selects the nonzero side of Neutral, applies invert, then gain. Tuning computes:

```go
inputMagnitude := float32(math.Abs(float64(value)))
scaled := (inputMagnitude-tuning.Deadzone)/(1-tuning.Deadzone)
magnitude := float32(math.Pow(float64(scaled*tuning.Gain), float64(tuning.Exponent)))
value = float32(math.Copysign(float64(magnitude), float64(value)))
```

Check finiteness after every stage and wrap the matching stable sentinel.

- [ ] **Step 5: Verify and commit**

```powershell
go test ./internal/processing -run 'Test(Calibration|Tuning)' -count=100
go test ./internal/processing -count=1
```

After independent review:

```bash
git add internal/processing/calibration.go internal/processing/calibration_test.go internal/processing/tuning.go internal/processing/tuning_test.go
git commit -m "feat(processing): calibrate and tune channels"
```

---

### Task 5: Implement EMA and One Euro Filters

**Files:**
- Modify: `internal/processing/filter.go`
- Create: `internal/processing/filter_test.go`

**Interfaces:**
- Consumes: Task 3 FilterConfig and FilterMode.
- Produces: resettable `filterState.apply(config, value, atNS)` for Task 6.

- [ ] **Step 1: Write EMA and initialization RED tests**

```go
var state filterState
config := FilterConfig{Mode:FilterEMA, EMAAlpha:0.25}
got, err := state.apply(config, 8, 1_000_000_000)
if err != nil || got != 8 { t.Fatalf("first = %v,%v", got, err) }
got, err = state.apply(config, 4, 2_000_000_000)
if err != nil || got != 7 { t.Fatalf("second = %v,%v", got, err) }
state.reset()
got, err = state.apply(config, 2, 3_000_000_000)
if err != nil || got != 2 { t.Fatalf("reset = %v,%v", got, err) }
```

Test FilterNone identity and rejection of a nonpositive time delta for a new sample.

- [ ] **Step 2: Write a hand-calculated One Euro RED test**

Use `MinCutoff=1`, `Beta=0`, `DerivativeCutoff=1`, samples `0` at 0 and `1` at 1 second. Compute `alpha := 1/(1+1/(2*pi))` in the test and require the second result within `1e-6`. Add a Beta-positive sequence compared with a small test-only reference function.

- [ ] **Step 3: Run RED**

```powershell
go test ./internal/processing -run 'Test(Filter|EMA|OneEuro)' -count=1
```

Expected: build failure for missing filter state behavior.

- [ ] **Step 4: Implement fixed filter state**

Store initialization, last time/raw value, filtered value, and filtered derivative. Use:

```go
func lowPassAlpha(cutoff, dt float64) float64 {
    tau := 1 / (2 * math.Pi * cutoff)
    return 1 / (1 + tau/dt)
}
```

One Euro filters the derivative at DerivativeCutoff, computes `MinCutoff+Beta*abs(filteredDerivative)`, then filters the value. The first sample initializes exactly; reset zeroes the state.

- [ ] **Step 5: Verify and commit**

```powershell
go test ./internal/processing -run 'Test(Filter|EMA|OneEuro)' -count=100
go test ./internal/processing -count=1
```

After numerical review:

```bash
git add internal/processing/filter.go internal/processing/filter_test.go
git commit -m "feat(processing): filter channels with EMA and One Euro"
```

---

### Task 6: Build the Caller-Driven Pipeline State Machine

**Files:**
- Modify: `internal/processing/canonical.go`
- Modify: `internal/processing/validation.go`
- Modify: `internal/processing/channel.go`
- Create: `internal/processing/pipeline.go`
- Create: `internal/processing/pipeline_test.go`
- Create: `internal/processing/validation_test.go`

**Interfaces:**
- Consumes: Task 2 MergedFrame freshness and Tasks 3–5 transforms.
- Produces: `NewPipeline(Config)` and `(*Pipeline).ProcessAt(tracking.MergedFrame, int64) (CanonicalFrame, error)` with fresh-input transforms and reset rules.

- [ ] **Step 1: Write public shape and fresh transform RED tests**

```go
pipeline, err := NewPipeline(DefaultConfig())
if err != nil { t.Fatal(err) }
frame := tracking.MergedFrame{
    Generation:1, Sequence:1, UpdatedAtNS:100,
    Capabilities:trackingmodel.CapabilityEye|trackingmodel.CapabilityExpression,
    EyeSourceID:"eye", ExpressionSourceID:"face",
    EyeUpdatedAtNS:90, ExpressionUpdatedAtNS:95,
    Eye:trackingmodel.EyeSample{Valid:trackingmodel.EyeValidLeftOpenness, LeftOpenness:0.75},
}
frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.5)
got, err := pipeline.ProcessAt(frame, 100)
if err != nil { t.Fatal(err) }
if got.Generation != 1 || got.Revision != 1 || got.ProcessedAtNS != 100 || got.Eye.LeftOpenness != 0.75 {
    t.Fatalf("canonical = %#v", got)
}
```

Use one channel override to prove calibration → tuning → filter order.

- [ ] **Step 2: Write ordering, reset, and atomic-error RED tests**

Add literal tests for:

- identical revision at later now does not update EMA;
- revision regression returns ErrRevisionRegression;
- same unsaturated revision with changed content returns ErrRevisionConflict;
- generation increase resets every filter;
- Eye A→B replacement resets Eye only while Expression history continues;
- Eye A→empty preserves Eye numeric history for Task 7 dropout;
- negative or future group timestamps and NaN valid values return ErrInvalidInput;
- a capability without its source/timestamp, an absent capability with retained
  source/timestamp/data, and an unknown capability bit return ErrInvalidInput;
- time regression returns ErrTimeRegression;
- equal processing time is accepted for an identical repeated input but
  rejected for a new revision; and
- after each error, the next valid output equals a control Pipeline that never received the bad call.

- [ ] **Step 3: Run RED**

```powershell
go test ./internal/processing -run 'Test(Pipeline|ProcessAt|InputValidation|GenerationReset|SourceReset)' -count=1
```

Expected: build failures for missing Pipeline and CanonicalFrame fields.

- [ ] **Step 4: Implement value output and transactional state**

```go
type CanonicalFrame struct {
    Generation uint64
    Revision uint64
    ProcessedAtNS int64
    EyeSourceID string
    ExpressionSourceID string
    LipSourceID string
    EyeActive bool
    ExpressionActive bool
    LipActive bool
    Eye trackingmodel.EyeSample
    Expressions trackingmodel.ExpressionSet
}
```

Store fixed channel states and last input. Validate before mutation, then use a local value copy:

```go
next := *p
out, err := next.processAt(frame, nowNS)
if err != nil { return CanonicalFrame{}, err }
*p = next
return out, nil
```

Compiled config is immutable and may be shared; runtime arrays are values. Rebuild coarse Eye validity only when all scalar members are available. At `math.MaxUint64`, full input equality and non-regressing freshness distinguish a repeat from a new saturated snapshot.

Replace the unused `Validator` interface in `validation.go` with the
actual merged-input validation helpers; there is no released or in-repository
consumer requiring a compatibility alias. Add a caller-mutation test proving
Pipeline retained its own MergedFrame value after ProcessAt returns.

- [ ] **Step 5: Verify and commit**

```powershell
go test ./internal/processing -run 'Test(Pipeline|ProcessAt|InputValidation|GenerationReset|SourceReset)' -count=100
go test ./internal/processing -count=20
go test -race ./internal/processing -count=10
```

The race run checks accidental globals; Pipeline remains caller-serialized. After review:

```bash
git add internal/processing/canonical.go internal/processing/validation.go internal/processing/channel.go internal/processing/pipeline.go internal/processing/pipeline_test.go internal/processing/validation_test.go
git commit -m "feat(processing): process merged tracking frames"
```

---

### Task 7: Add Dropout, Active State, and Mutual Exclusion

**Files:**
- Modify: `internal/processing/dropout.go`
- Create: `internal/processing/mutual.go`
- Modify: `internal/processing/pipeline.go`
- Create: `internal/processing/dropout_test.go`
- Create: `internal/processing/mutual_test.go`
- Modify: `internal/processing/pipeline_test.go`

**Interfaces:**
- Consumes: Task 6 channel/filter history and group timestamps.
- Produces: stale/hold/decay/neutral output, explicit active flags, and post-dropout winner projection.

- [ ] **Step 1: Write exact dropout timeline RED tests**

Use `StaleAfter=10ns`, `HoldDuration=5ns`, and `DecayDuration=10ns`. Submit value `0.8` at time 100, repeat the same merged snapshot, and assert:

```text
now 110 -> 0.8 active boundary
now 115 -> 0.8 hold boundary
now 120 -> 0.4 decay midpoint
now 125 -> 0.0 valid neutral
now 200 -> 0.0 still valid neutral
```

Assert a never-seen channel remains invalid, capability removal makes Active false immediately while the number holds, and fresh recovery exits dropout.
For a removal snapshot with `UpdatedAtNS=110` first processed at `now=120`,
assert the output is already at the decay midpoint rather than beginning a new
hold at 120.

- [ ] **Step 2: Write active and source-change RED tests**

With ActiveStaleAfter `10ns`, prove Eye, Expression, and Lip become false independently. A Lip-only frame sets only LipActive. Lip removal changes no Expression value. Replacing A with B resets the numeric group, so a channel absent from B is invalid rather than held from A; changing A to no source preserves A's history and starts dropout.

- [ ] **Step 3: Write mutual-exclusion RED tests**

```go
jaw, ok := ExpressionChannel(trackingmodel.ExpressionJawOpen)
if !ok { t.Fatal("JawOpen channel missing") }
closed, ok := ExpressionChannel(trackingmodel.ExpressionMouthClosed)
if !ok { t.Fatal("MouthClosed channel missing") }
config.MutualExclusion = [][]ChannelID{{jaw, closed}}
```

Assert largest absolute value wins, a tie chooses the smaller ChannelID, invalid channels do not win, losers remain valid zero, and a held old candidate cannot coexist with a fresh new winner.

- [ ] **Step 4: Run RED**

```powershell
go test ./internal/processing -run 'Test(Dropout|Active|Mutual|SourceChange)' -count=1
```

Expected: behavioral failures because Task 6 emits only fresh transforms.

- [ ] **Step 5: Implement dropout before output projection**

Each channel stores seen, last fresh time/value, filter, and dropout origin/start. For stopped selected data, start at `lastFresh+StaleAfter`; for capability/source loss without replacement, preserve history and start at the unavailable merged snapshot's `UpdatedAtNS`. Hold, linearly interpolate to zero, then keep valid zero. A new non-empty source resets the affected group before ingesting its first sample.

Compute Active from source/capability and `now-groupUpdatedAtNS <= ActiveStaleAfter`. Run mutual exclusion on the complete candidate array without overwriting channel history.

- [ ] **Step 6: Stress, review, and commit**

```powershell
go test ./internal/processing -run 'Test(Dropout|Active|Mutual|SourceChange)' -count=200
go test ./internal/processing -count=50
go test -race ./internal/processing -count=20
```

After boundary review:

```bash
git add internal/processing/dropout.go internal/processing/dropout_test.go internal/processing/mutual.go internal/processing/mutual_test.go internal/processing/pipeline.go internal/processing/pipeline_test.go
git commit -m "feat(processing): handle dropout and mutual exclusion"
```

---

### Task 8: Compile Immutable Evaluator Instructions

**Files:**
- Modify: `internal/parameterdeps/dependencies.go`
- Modify: `internal/parameterdeps/dependencies_test.go`
- Create: `internal/evaluator/errors.go`
- Create: `internal/evaluator/operand.go`
- Create: `internal/evaluator/plan.go`
- Create: `internal/evaluator/plan_test.go`

**Interfaces:**
- Consumes: parameterdeps plans, generated parameter types, and processing Eye/Active leaves.
- Produces: `evaluator.Compile([]parameters.ParameterID) (*Plan, error)` with deterministic instructions and no requested-slice retention.

- [ ] **Step 1: Write PupilDilation metadata RED test**

```go
plan, ok := Plan(parameters.ParameterPupilDilation)
if !ok { t.Fatal("missing PupilDilation plan") }
if plan.Operation != OperationAverage || plan.Inputs.Eye != EyeFieldsOf(EyeFieldLeftPupilDilation, EyeFieldRightPupilDilation) {
    t.Fatalf("plan = %#v", plan)
}
```

Expected before production change: assertion failure because current metadata says Direct.

- [ ] **Step 2: Write evaluator compile RED tests**

Test an empty request producing an empty valid Plan, request deduplication, deterministic topology, all 127 generated IDs, caller-slice ownership, and compile errors. Define the internal seam as `compileWithPlans(requested []parameters.ParameterID, lookup func(parameters.ParameterID) (parameterdeps.DependencyPlan, bool)) (*Plan, error)`; feed it a test map to prove missing dependency, cycle, Direct arity != 1, SignedPair arity < 2, and unsupported operation. Production `Compile` passes `parameterdeps.Plan` as the lookup.

```go
for id := parameters.ParameterID(0); id < parameters.ParameterCount; id++ {
    if _, err := Compile([]parameters.ParameterID{id}); err != nil {
        t.Fatalf("Compile(%d): %v", id, err)
    }
}
```

- [ ] **Step 3: Run RED**

```powershell
go test ./internal/parameterdeps ./internal/evaluator -run 'Test(PupilDilation|Compile|Plan)' -count=1
```

Expected: PupilDilation failure and missing evaluator symbols.

- [ ] **Step 4: Correct metadata and implement compiler**

Change only PupilDilation to OperationAverage. Represent operands as:

```go
type operandKind uint8
const (
    operandEye operandKind = iota + 1
    operandExpression
    operandActive
    operandParameter
)
type operand struct {
    kind operandKind
    eye parameterdeps.EyeField
    expression trackingmodel.ExpressionID
    active parameterdeps.ActiveState
    parameter parameters.ParameterID
}
```

Enumerate primitive Inputs in stable EyeField, ExpressionID, and ActiveState order, followed by DependsOn order. DFS into deterministic instructions; validate operation arity and generated value type. Copy and deduplicate requests into a fixed bitset.

Define evaluator sentinels `ErrUnknownParameter`, `ErrMissingPlan`, `ErrDependencyCycle`, and `ErrInvalidOperation`. Unknown IDs wrap ErrUnknownParameter; dependency lookup failure wraps ErrMissingPlan; type/arity/unsupported-operation failures wrap ErrInvalidOperation.

- [ ] **Step 5: Verify and commit**

```powershell
go test ./internal/parameterdeps ./internal/evaluator -run 'Test(PupilDilation|Compile|Plan)' -count=100
go test ./internal/parameterdeps ./internal/evaluator -count=1
```

Review must prove evaluator has no PupilDilation ParameterID special case. Then:

```bash
git add internal/parameterdeps/dependencies.go internal/parameterdeps/dependencies_test.go internal/evaluator/errors.go internal/evaluator/operand.go internal/evaluator/plan.go internal/evaluator/plan_test.go
git commit -m "feat(evaluator): compile selective parameter plans"
```

---

### Task 9: Evaluate Typed Parameter Snapshots

**Files:**
- Create: `internal/evaluator/evaluate.go`
- Create: `internal/evaluator/snapshot.go`
- Create: `internal/evaluator/evaluate_test.go`
- Create: `internal/evaluator/snapshot_test.go`
- Create: `internal/evaluator/concurrency_test.go`

**Interfaces:**
- Consumes: Task 8 Plan and Task 7 CanonicalFrame.
- Produces: `(*Plan).Evaluate(processing.CanonicalFrame) Snapshot`, `Snapshot.Float`, and `Snapshot.Bool`.

- [ ] **Step 1: Write hand-calculated operation RED tests**

Use real IDs for Direct JawOpen, primitive Average PupilDilation, dependency Average EyeX, Max BrowDownRight, two/three-operand SignedPair, and SumClamp MouthOpen.

```go
frame.Expressions.Set(trackingmodel.ExpressionMouthCornerPullRight, 0.7)
frame.Expressions.Set(trackingmodel.ExpressionMouthCornerSlantRight, 0.6)
frame.Expressions.Set(trackingmodel.ExpressionMouthFrownRight, 0.2)
frame.Expressions.Set(trackingmodel.ExpressionMouthStretchRight, 0.1)
plan, err := Compile([]parameters.ParameterID{parameters.ParameterSmileSadRight})
if err != nil { t.Fatal(err) }
snapshot := plan.Evaluate(frame)
got, ok := snapshot.Float(parameters.ParameterSmileSadRight)
if !ok || got != 0.5 { t.Fatalf("SmileSadRight = %v,%t", got, ok) }
```

- [ ] **Step 2: Write validity and visibility RED tests**

Omit one leaf from a derived request and require invalid. Request only EyeX and require its EyeLeftX/EyeRightX dependencies remain externally invalid. Test final range clamp, NaN containment from a manually built CanonicalFrame, wrong-type access, unknown IDs, and Lip Active true while Expression Active false.

- [ ] **Step 3: Write immutable concurrent Plan RED test**

Run 16 goroutines, each evaluating 1,000 distinct frames through one Plan and comparing exact results. Workers send errors through a buffered channel and WaitGroup; they never call testing.T.

- [ ] **Step 4: Run RED**

```powershell
go test ./internal/evaluator -run 'Test(Evaluate|Snapshot|Operations|Validity|Concurrent)' -count=1
```

Expected: build failures for missing Evaluate/Snapshot.

- [ ] **Step 5: Implement fixed-size execution and Snapshot**

Use ParameterID-indexed arrays for temporary floats, bools, and validity. Resolve primitive operands from CanonicalFrame and parameter operands from earlier instructions. Canonical Active operands are always valid booleans; numeric operands use Eye/Expression validity. Require every operand valid. Apply fixed operations, mark non-finite results invalid, and clamp every float instruction to its generated range.

Snapshot stores dense arrays and fixed validity bitsets:

```go
const validityWordCount = (parameters.ParameterCount + 63) / 64
type Snapshot struct {
    floats [parameters.ParameterCount]float32
    bools [parameters.ParameterCount]bool
    floatValid [validityWordCount]uint64
    boolValid [validityWordCount]uint64
}
```

Copy only requested results. Float/Bool check ID range, generated type, and matching validity.

- [ ] **Step 6: Stress, race, review, and commit**

```powershell
go test ./internal/evaluator -run 'Test(Evaluate|Snapshot|Operations|Validity|Concurrent)' -count=200
go test ./internal/evaluator -count=50
go test -race ./internal/evaluator -count=20
```

After formula/ownership review:

```bash
git add internal/evaluator/evaluate.go internal/evaluator/snapshot.go internal/evaluator/evaluate_test.go internal/evaluator/snapshot_test.go internal/evaluator/concurrency_test.go
git commit -m "feat(evaluator): evaluate typed parameter snapshots"
```

---

### Task 10: Prove Cross-Package Compatibility and Bounds

**Files:**
- Create: `internal/evaluator/integration_external_test.go`
- Create: `internal/evaluator/evaluate_benchmark_test.go`
- Modify: `internal/processing/pipeline_test.go` only if an integration failure requires a processing-owned regression.

**Interfaces:**
- Consumes: final tracking, processing, evaluator, and OSC source contracts.
- Produces: dependency-direction proof and an in-memory M4 fixture.

- [ ] **Step 1: Write external structural compatibility**

```go
package evaluator_test

import (
    "github.com/wzhqwq/vrcft-go/internal/evaluator"
    "github.com/wzhqwq/vrcft-go/internal/osc"
)

var _ osc.ValueSource = evaluator.Snapshot{}
```

OSC must remain a test-only dependency.

- [ ] **Step 2: Write merged-to-snapshot fixture**

Create a Lip+Eye+Expression MergedFrame, process it at a literal Host time, compile one direct, one derived, and all three Active parameters, evaluate, then read through an `osc.ValueSource`. Assert exact values and an unrequested ID invalid. Do not construct transport or Application.

- [ ] **Step 3: Add allocation and scale proof**

Use `testing.AllocsPerRun(1000, func(){ _ = plan.Evaluate(frame) })` and require zero allocations after compilation. Add benchmarks for Evaluate and Pipeline with `b.ReportAllocs()` and no timing threshold.

- [ ] **Step 4: Run related normal/race/benchmark verification**

If the fixture exposes an owning defect, add a focused RED in that package before changing production.

```powershell
go test ./internal/tracking ./internal/processing ./internal/parameterdeps ./internal/evaluator ./internal/osc -count=50
go test -race ./internal/tracking ./internal/processing ./internal/parameterdeps ./internal/evaluator ./internal/osc -count=20
go test ./internal/evaluator -run '^$' -bench 'Benchmark(Evaluate|Pipeline)' -benchmem -count=3
```

- [ ] **Step 5: Review and commit**

Review production imports, frame ownership, and absence of timers/goroutines. Commit only proof files unless a separate owning fix was reviewed:

```bash
git add internal/evaluator/integration_external_test.go internal/evaluator/evaluate_benchmark_test.go
git commit -m "test(evaluator): prove M4 pipeline compatibility"
```

---

### Task 11: Complete Specs, Final Review, and Generated M4 Status

**Files:**
- Modify: `docs/project/packages/pkg-trackingmodel.md`
- Modify: `docs/project/packages/pkg-pluginapi.md`
- Modify: `docs/project/packages/pkg-pluginruntime.md`
- Modify: `docs/project/packages/pkg-protocol.md`
- Modify: `docs/project/packages/internal-plugins.md`
- Modify: `docs/project/packages/internal-tracking.md`
- Modify: `docs/project/packages/internal-processing.md`
- Modify: `docs/project/packages/internal-parameterdeps.md`
- Modify: `docs/project/packages/internal-application.md`
- Create: `docs/project/packages/internal-evaluator.md`
- Modify: `docs/project/subsystems/end-to-end.md`
- Modify: `docs/project/status.md` through `cmd/projectstatus -write`
- Create ignored evidence below `.superpowers/tmp/2026-08-04-m4-processing-evaluator/`.

**Interfaces:**
- Consumes: reviewed Tasks 1–10.
- Produces: accurate package ownership, final verification/review, and generated M4 evidence.

- [ ] **Step 1: Update package specifications**

Document only implemented facts:

- capability packages recognize metadata-only Lip bit 4;
- tracking owns independent Lip routing and per-group Host freshness;
- processing owns caller-serialized transforms, history, dropout, and mutual exclusion;
- parameterdeps owns generalized operands and PupilDilation average metadata;
- evaluator owns immutable selective plans and typed snapshots; and
- Application wiring, avatar planning, OSC networking, persistence/UI, numeric Lip payload, and Expression-to-Lip mapping remain deferred.

Create the evaluator spec with executable checks:

```yaml
id: internal-evaluator
kind: go-package
path: internal/evaluator
milestone: M4
depends_on: [internal-processing, internal-parameterdeps, internal-parameters]
checks:
  - id: package-tests
    description: Evaluator package tests pass
    type: command
    command: go-test
    args: [./internal/evaluator]
    weight: 3
    required: true
  - id: package-race-tests
    description: Evaluator package race tests pass
    type: command
    command: go-test-race
    args: [./internal/evaluator]
    weight: 2
    required: true
```

Change internal-processing dependencies to `[internal-tracking, pkg-trackingmodel]`, move its `pipeline-implemented` check to `internal/processing/pipeline.go` with pattern `type Pipeline struct`, and replace stale prose with the actual Pipeline contract. Add internal-evaluator to the planned Application/end-to-end dependency documentation and to the end-to-end component aggregate; do not add Application production wiring.

- [ ] **Step 2: Commit accurate specs before final verification**

Run `git diff --check`, compare every named symbol with production, obtain an independent docs review, then:

```bash
git add docs/project/packages/pkg-trackingmodel.md docs/project/packages/pkg-pluginapi.md docs/project/packages/pkg-pluginruntime.md docs/project/packages/pkg-protocol.md docs/project/packages/internal-plugins.md docs/project/packages/internal-tracking.md docs/project/packages/internal-processing.md docs/project/packages/internal-parameterdeps.md docs/project/packages/internal-application.md docs/project/packages/internal-evaluator.md docs/project/subsystems/end-to-end.md
git commit -m "docs: specify M4 processing evaluation"
```

- [ ] **Step 3: Run final verification from the clean spec commit**

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-04-m4-processing-evaluator\final-verification'
New-Item -ItemType Directory -Force -Path $taskTmp,"$taskTmp\gocache" | Out-Null
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'; $env:GOTELEMETRY='off'
go test ./...
go test -race ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime ./internal/plugins ./internal/tracking ./internal/processing ./internal/parameterdeps ./internal/evaluator ./internal/osc
go vet ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime ./internal/plugins ./internal/tracking ./internal/processing ./internal/parameterdeps ./internal/evaluator ./internal/osc
```

```powershell
$files = Get-ChildItem pkg/trackingmodel,pkg/pluginapi,pkg/protocol,pkg/pluginruntime,internal/plugins,internal/tracking,internal/processing,internal/parameterdeps,internal/evaluator -Filter '*.go' -File
gofmt -d ($files.FullName)
git diff --check
git status --short
```

Expected: tests/vet exit 0, gofmt emits no diff, and tracked worktree is clean.

- [ ] **Step 4: Run independent whole-range review**

Review from design commit `8d7ff9f` through source/spec HEAD. The reviewer explicitly re-proves:

- Lip never implies or mutates Expression data;
- existing capability bits and plugin/protocol compatibility remain stable;
- M3 routing/freshness and generation atomicity;
- Pipeline validate-before-mutate, reset boundaries, transform order, dropout/final neutral, and post-dropout mutual exclusion;
- immutable config ownership and no background lifecycle;
- evaluator operands, strict validity, hidden dependencies, clamping/non-finite handling, concurrency, and OSC dependency direction;
- fixed bounds/no unbounded history; and
- specs preserve M5/M6 deferrals.

Fix every Critical/Important finding through scoped RED/GREEN and re-review. Record non-blocking Minor findings in the handoff.

- [ ] **Step 5: Generate status from the reviewed clean source commit**

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-04-m4-processing-evaluator\status-write'
New-Item -ItemType Directory -Force -Path $taskTmp,"$taskTmp\gocache" | Out-Null
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'; $env:GOTELEMETRY='off'
go run ./cmd/projectstatus -write
```

Exit 1 is acceptable only for unrelated incomplete milestones. The generated report must name the clean source commit, Dirty false, internal-processing/internal-evaluator complete, and M4 complete 100%, while retaining M5/M6/frontend/release blockers.

```bash
git add docs/project/status.md
git commit -m "docs: refresh M4 completion evidence"
```

- [ ] **Step 6: Recheck status and finish the ignored handoff**

Run `go run ./cmd/projectstatus -check` with a new scoped cache. Exit 1 is acceptable only for global blocked state; output must not report stale status. Then:

```bash
git status --short
git log -15 --oneline
```

Update the ignored ledger/handoff with commit IDs, exact RED/GREEN and final outputs, final review verdict, generated status, and deferred Minor findings. Do not push or open a PR without explicit authorization.

---

## Plan Completion Gate

Do not mark M4 complete because a symbol or status check passes. Completion requires all eleven tasks, strict task-level RED/GREEN evidence, final normal/race/vet verification, independent whole-range review with no open Critical or Important findings, accurate package specs, clean-source generated status, and a clean tracked worktree.
