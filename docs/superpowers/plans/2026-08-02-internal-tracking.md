# Internal Tracking Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete M3 with a deterministic, generation-aware Host tracking ingest service that validates plugin frames, performs stable Eye/Expression source routing, and publishes bounded merged snapshots and diagnostics.

**Architecture:** A single mutex-protected state machine owns the current generation, one latest frame per plugin, routing selections, diagnostics, and subscriber registries. Mutations are synchronous and linearizable; externally visible frames and summaries are value copies published through capacity-one latest-value channels. A transport-neutral wrapper structurally satisfies `plugins.FrameSink` without adding a production dependency from `internal/tracking` to `internal/plugins`.

**Tech Stack:** Go standard library (`context`, `errors`, `sync`, `time`), `pkg/trackingmodel`, table-driven Go tests, race detector, existing project-status generator.

## Global Constraints

- The authoritative design is `docs/superpowers/specs/2026-08-02-internal-tracking-design.md`.
- Production `internal/tracking` may import only the Go standard library and `pkg/trackingmodel`; it must not import `internal/plugins`.
- Delete `RoutingConfig.Head`; implement only Eye and Expression.
- `Submit` takes an owned `trackingmodel.TrackingFrame` value and never retains transport or shared-memory views.
- The Host explicitly advances one positive global generation; old-generation state is cleared atomically.
- Auto routing is sticky and reselects the lexicographically smallest capable plugin only when the current source becomes unavailable.
- Empty validity is dropout data and must not trigger Auto reselection while the capability remains advertised.
- Producers must never block on merged or Summary subscribers; both channels have capacity one and latest-value replacement.
- No frame history, unbounded ingest queue, calibration, filtering, avatar resolution, Application wiring, Head model, or shared-memory protocol is added.
- Use strict TDD: establish the intended RED against unchanged production behavior, then make the smallest owning change.
- Every temporary artifact must stay below ignored `.superpowers/tmp/`: `TMP`, `TEMP`, `GOCACHE`, build outputs, SDD ledger, task briefs, reports, and review packages. Use a unique `.superpowers/tmp/2026-08-02-internal-tracking/<task-name>` directory and set `GOTOOLCHAIN=local`.
- Subagents do not stage, commit, download, install, or request permissions. The primary agent inspects and verifies each diff, owns commits, and owns project-status generation.
- Preserve unrelated user changes. Do not push or open a PR unless the user explicitly requests it.

## File Structure

- Modify `internal/tracking/frame.go`: normative `MergedFrame` value model.
- Modify `internal/tracking/routing.go`: Eye/Expression routing types, defaults, and validation.
- Modify `internal/tracking/service.go`: final Service interface, concrete service construction, generation/control orchestration.
- Create `internal/tracking/errors.go`: stable sentinel errors and rejection-reason mapping.
- Create `internal/tracking/summary.go`: fixed diagnostic value types and saturating counters.
- Create `internal/tracking/source.go`: per-plugin current-generation frame and ordering state.
- Create `internal/tracking/subscription.go`: capacity-one latest-value subscriber registry and cancellation.
- Create `internal/tracking/sink.go`: transport-neutral `PluginFrameSink` wrapper.
- Create focused tests beside each responsibility; use `package tracking_test` only for the cross-package `plugins.FrameSink` compatibility test.
- Modify `docs/project/packages/internal-tracking.md`: completed responsibilities, interfaces, error and concurrency contract.
- Regenerate `docs/project/status.md` only after a clean source commit and commit the generated status separately.

---

### Task 1: Define Supported Routing Contracts

**Files:**
- Modify: `internal/tracking/routing.go`
- Create: `internal/tracking/errors.go`
- Create: `internal/tracking/contracts_test.go`

**Interfaces:**
- Consumes: the existing `SourceSelection` and `RoutingConfig` placeholders.
- Produces: Head-free `SourceSelection`, Head-free `RoutingConfig`, default Auto routing, `ErrInvalidRouting`, and exact empty/non-empty routing validation used by Task 3.

- [ ] **Step 1: Write contract RED tests**

Create `contracts_test.go` with routing validation tests. Reflection is limited
to the explicit removal contract for the obsolete Head field; it must not
assert the total field count or freeze unrelated future fields.

```go
func TestRoutingConfigContainsOnlySupportedGroups(t *testing.T) {
	typ := reflect.TypeOf(RoutingConfig{})
	if _, ok := typ.FieldByName("Head"); ok {
		t.Fatal("RoutingConfig still exposes unsupported Head routing")
	}
}

func TestSourceSelectionValidation(t *testing.T) {
	tests := []struct {
		name string
		selection SourceSelection
		wantErr bool
	}{
		{name: "auto", selection: SourceSelection{Auto: true}},
		{name: "auto with plugin", selection: SourceSelection{Auto: true, PluginID: "vendor.eye"}, wantErr: true},
		{name: "manual", selection: SourceSelection{PluginID: "vendor.eye"}},
		{name: "manual empty", selection: SourceSelection{}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.selection.validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-02-internal-tracking\task-01'
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'
New-Item -ItemType Directory -Force $env:TMP,$env:GOCACHE | Out-Null
go test ./internal/tracking -run 'Test(RoutingConfigContainsOnlySupportedGroups|SourceSelectionValidation)' -count=1
```

Expected: FAIL because Head still exists and routing validation/default behavior does not exist.

- [ ] **Step 3: Implement the exact routing contract**

Delete Head, retain the existing JSON shape for Eye and Expression, and
implement the exact empty/non-empty validation contract:

```go
type SourceSelection struct {
	Auto     bool   `json:"auto"`
	PluginID string `json:"pluginId,omitempty"`
}

type RoutingConfig struct {
	Eye        SourceSelection `json:"eye"`
	Expression SourceSelection `json:"expression"`
}

func defaultRouting() RoutingConfig {
	return RoutingConfig{
		Eye: SourceSelection{Auto: true},
		Expression: SourceSelection{Auto: true},
	}
}

func (s SourceSelection) validate() error {
	if s.Auto {
		if s.PluginID != "" {
			return ErrInvalidRouting
		}
		return nil
	}
	if s.PluginID == "" {
		return ErrInvalidRouting
	}
	return nil
}

func (c RoutingConfig) validate() error {
	return errors.Join(c.Eye.validate(), c.Expression.validate())
}
```
Define the one sentinel this behavior uses:

```go
var ErrInvalidRouting = errors.New("tracking: invalid routing")
```

- [ ] **Step 4: Run formatting and focused GREEN**

Run:

```powershell
gofmt -w internal/tracking/routing.go internal/tracking/errors.go internal/tracking/contracts_test.go
go test ./internal/tracking -run 'Test(RoutingConfigContainsOnlySupportedGroups|SourceSelectionValidation)' -count=20
go test ./internal/tracking -count=1
```

Expected: all commands exit 0.

- [ ] **Step 5: Primary-agent review and commit**

Verify `git diff --check`, ensure only Task 1 files changed, then the primary agent commits:

```bash
git add internal/tracking/routing.go internal/tracking/errors.go internal/tracking/contracts_test.go
git commit -m "feat(tracking): define supported routing"
```

Run an independent task review. Fix every Critical or Important finding before Task 2.

---

### Task 2: Implement Generation and Validated Latest-Source Ingest

**Files:**
- Modify: `internal/tracking/frame.go`
- Modify: `internal/tracking/errors.go`
- Modify: `internal/tracking/service.go`
- Create: `internal/tracking/source.go`
- Create: `internal/tracking/service_test.go`
- Create: `internal/tracking/source_test.go`

**Interfaces:**
- Consumes: Task 1 routing contracts and `TrackingFrame.Canonicalize() (TrackingFrame, error)`.
- Produces: `MergedFrame`, generation/ingest sentinel errors, an interim `NewService() *service`, linearizable `SetGeneration`, `Generation`, `Submit`, and `LatestMerged`; one current-generation `sourceState` per plugin. Task 4 adds diagnostics and changes the constructor return type to the final `Service`.

- [ ] **Step 1: Write generation RED tests**

Test unset, zero, regression, same-generation idempotence, advance, empty current snapshot, and old/future submissions:

```go
func TestServiceGenerationControlsCurrentState(t *testing.T) {
	service := NewService()
	if _, ok := service.LatestMerged(); ok {
		t.Fatal("LatestMerged before generation = true")
	}
	if err := service.SetGeneration(0); !errors.Is(err, ErrGenerationZero) {
		t.Fatalf("SetGeneration(0) error = %v", err)
	}
	if err := service.SetGeneration(4); err != nil {
		t.Fatal(err)
	}
	merged, ok := service.LatestMerged()
	if !ok || merged.Generation != 4 || merged.Sequence == 0 {
		t.Fatalf("LatestMerged = (%+v, %v), want generation 4 empty snapshot", merged, ok)
	}
	before := merged
	if err := service.SetGeneration(4); err != nil {
		t.Fatal(err)
	}
	if after, _ := service.LatestMerged(); after != before {
		t.Fatalf("idempotent generation changed snapshot: before=%+v after=%+v", before, after)
	}
	if err := service.SetGeneration(3); !errors.Is(err, ErrGenerationRegression) {
		t.Fatalf("regression error = %v", err)
	}
}
```

Use an internal deterministic clock constructor in tests so generation advance timestamps are asserted without sleeping.

- [ ] **Step 2: Write ingest validation and ownership RED tests**

Use table tests for empty plugin ID, unset/zero/stale/future generation, negative timestamps, malformed frames, duplicate Sequence, and each non-zero time regression. Include zero-time baseline behavior and generation reset:

```go
func TestSubmitOrdersEachPluginWithinGeneration(t *testing.T) {
	service := newServiceWithClock(sequenceClock(100, 101, 102, 103))
	mustSetGeneration(t, service, 7)
	mustSubmit(t, service, "vendor.eye", 7, trackingmodel.TrackingFrame{
		Sequence: 5, TimestampNS: 20, SourceClockNS: 30,
	})

	if err := service.Submit("vendor.eye", 7, trackingmodel.TrackingFrame{Sequence: 5, TimestampNS: 21, SourceClockNS: 31}); !errors.Is(err, ErrSequenceNotIncreasing) {
		t.Fatalf("duplicate sequence error = %v", err)
	}
	if err := service.Submit("vendor.eye", 7, trackingmodel.TrackingFrame{Sequence: 6, TimestampNS: 19, SourceClockNS: 31}); !errors.Is(err, ErrTimestampRegression) {
		t.Fatalf("timestamp regression error = %v", err)
	}
	if err := service.Submit("vendor.eye", 7, trackingmodel.TrackingFrame{Sequence: 6, TimestampNS: 21, SourceClockNS: 29}); !errors.Is(err, ErrSourceClockRegression) {
		t.Fatalf("source clock regression error = %v", err)
	}
}
```

After an accepted frame, mutate the caller's frame variable and assert the stored `sourceState.frame` remains the canonical owned value.

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-02-internal-tracking\task-02'
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'
New-Item -ItemType Directory -Force $env:TMP,$env:GOCACHE | Out-Null
go test ./internal/tracking -run 'Test(ServiceGeneration|Submit)' -count=1
```

Expected: FAIL because `NewService`, concrete generation state, and validated source ingest do not exist.

- [ ] **Step 4: Implement the minimal generation state machine**

The RED tests drive the merged snapshot value and stable generation/ingest
errors at the point they first have behavior:

```go
type MergedFrame struct {
	Generation  uint64
	Sequence    uint64
	UpdatedAtNS int64

	Capabilities trackingmodel.Capability
	Eye          trackingmodel.EyeSample
	Expressions  trackingmodel.ExpressionSet

	EyeSourceID        string
	ExpressionSourceID string
}

var (
	ErrGenerationUnset = errors.New("tracking: generation is unset")
	ErrGenerationZero = errors.New("tracking: generation must be positive")
	ErrGenerationRegression = errors.New("tracking: generation regression")
	ErrStaleGeneration = errors.New("tracking: stale generation")
	ErrFutureGeneration = errors.New("tracking: future generation")
	ErrInvalidPluginID = errors.New("tracking: invalid plugin ID")
	ErrInvalidFrame = errors.New("tracking: invalid frame")
	ErrSequenceNotIncreasing = errors.New("tracking: sequence is not increasing")
	ErrTimestampRegression = errors.New("tracking: timestamp regression")
	ErrSourceClockRegression = errors.New("tracking: source clock regression")
)
```

Create a concrete `service` with this state ownership:

```go
type service struct {
	mu sync.Mutex
	now func() int64
	lastHostTimeNS int64
	generation uint64
	routing RoutingConfig
	sources map[string]sourceState
	merged MergedFrame
	hasMerged bool
	mergedSequence uint64
}

type sourceState struct {
	frame trackingmodel.TrackingFrame
	receivedAtNS int64
	lastSequence uint64
	lastTimestampNS int64
	lastSourceClockNS int64
}
```

At this task boundary, `NewService` returns `*service` because routing and
subscription methods are introduced by Tasks 3 and 4. Do not add inert method
stubs merely to satisfy the final Service interface:

```go
func NewService() *service {
	return newServiceWithClock(func() int64 { return time.Now().UnixNano() })
}

func newServiceWithClock(now func() int64) *service {
	return &service{
		now: now,
		routing: defaultRouting(),
		sources: make(map[string]sourceState),
	}
}
```

Implement generation advance atomically:

```go
func (s *service) SetGeneration(next uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if next == 0 { return ErrGenerationZero }
	if next < s.generation { return ErrGenerationRegression }
	if next == s.generation { return nil }
	s.generation = next
	s.sources = make(map[string]sourceState)
	s.advanceMergedSequenceLocked()
	s.merged = MergedFrame{Generation: next, Sequence: s.mergedSequence, UpdatedAtNS: s.nextTimeLocked()}
	s.hasMerged = true
	return nil
}
```

Implement revision saturation explicitly and add a focused test that seeds the
private counter at `math.MaxUint64`, advances generation, and observes that it
does not wrap:

```go
func (s *service) advanceMergedSequenceLocked() {
	if s.mergedSequence < math.MaxUint64 {
		s.mergedSequence++
	}
}

func (s *service) nextTimeLocked() int64 {
	next := s.now()
	if next < s.lastHostTimeNS {
		return s.lastHostTimeNS
	}
	s.lastHostTimeNS = next
	return next
}
```

Implement `Submit` validation in the exact design order. Update ordering
baselines only after every validation succeeds. Wrap canonicalization and
negative-time failures with `ErrInvalidFrame` while preserving the safe cause.

```go
func (s *service) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pluginID == "" { return ErrInvalidPluginID }
	if s.generation == 0 { return ErrGenerationUnset }
	if generation == 0 { return ErrGenerationZero }
	if generation < s.generation { return ErrStaleGeneration }
	if generation > s.generation { return ErrFutureGeneration }

	canonical, err := frame.Canonicalize()
	if err != nil { return fmt.Errorf("%w: %v", ErrInvalidFrame, err) }
	previous, exists := s.sources[pluginID]
	if exists && canonical.Sequence <= previous.lastSequence { return ErrSequenceNotIncreasing }
	if canonical.TimestampNS < 0 || canonical.SourceClockNS < 0 { return ErrInvalidFrame }
	if exists && canonical.TimestampNS != 0 && previous.lastTimestampNS != 0 && canonical.TimestampNS < previous.lastTimestampNS { return ErrTimestampRegression }
	if exists && canonical.SourceClockNS != 0 && previous.lastSourceClockNS != 0 && canonical.SourceClockNS < previous.lastSourceClockNS { return ErrSourceClockRegression }

	next := sourceState{
		frame: canonical,
		receivedAtNS: s.nextTimeLocked(),
		lastSequence: canonical.Sequence,
		lastTimestampNS: previous.lastTimestampNS,
		lastSourceClockNS: previous.lastSourceClockNS,
	}
	if canonical.TimestampNS != 0 { next.lastTimestampNS = canonical.TimestampNS }
	if canonical.SourceClockNS != 0 { next.lastSourceClockNS = canonical.SourceClockNS }
	s.sources[pluginID] = next
	return nil
}
```

- [ ] **Step 5: Run focused stress and full package GREEN**

Run:

```powershell
gofmt -w internal/tracking/frame.go internal/tracking/errors.go internal/tracking/service.go internal/tracking/source.go internal/tracking/service_test.go internal/tracking/source_test.go
go test ./internal/tracking -run 'Test(ServiceGeneration|Submit)' -count=50
go test ./internal/tracking -count=1
go test -race ./internal/tracking -run 'Test(ServiceGeneration|Submit)' -count=10
```

Expected: all commands exit 0 with no race report.

- [ ] **Step 6: Primary-agent review and commit**

Verify Task 2's RED report, inspect ownership and mutation order, run `git diff --check`, then commit:

```bash
git add internal/tracking/frame.go internal/tracking/errors.go internal/tracking/service.go internal/tracking/source.go internal/tracking/service_test.go internal/tracking/source_test.go
git commit -m "feat(tracking): validate generation-tagged frames"
```

Run an independent task review. In particular, mutation must not precede complete validation.

---

### Task 3: Implement Sticky Routing and Deterministic Merge

**Files:**
- Modify: `internal/tracking/service.go`
- Modify: `internal/tracking/routing.go`
- Modify: `internal/tracking/frame.go`
- Create: `internal/tracking/routing_test.go`
- Create: `internal/tracking/merge_test.go`

**Interfaces:**
- Consumes: Task 2 `service.sources`, accepted canonical source frames, deterministic clock, and generation snapshot.
- Produces: working `SetRouting`, `Routing`, `RemoveSource`, sticky source IDs, split-group `MergedFrame`, and merge revision behavior.

- [ ] **Step 1: Write split merge and manual-routing RED tests**

Create frames with distinct Eye and Expression values and prove groups can be merged from different plugins. Then set a missing manual source and prove no fallback occurs:

```go
func TestServiceMergesGroupsFromDifferentSources(t *testing.T) {
	service := newServiceWithClock(sequenceClock(10, 11, 12, 13))
	mustSetGeneration(t, service, 2)
	mustSubmit(t, service, "eye.plugin", 2, eyeFrame(1, 0.25))
	mustSubmit(t, service, "expression.plugin", 2, expressionFrame(1, trackingmodel.ExpressionJawOpen, 0.75))

	merged, ok := service.LatestMerged()
	if !ok || merged.EyeSourceID != "eye.plugin" || merged.ExpressionSourceID != "expression.plugin" {
		t.Fatalf("merged sources = (%q, %q), ok=%v", merged.EyeSourceID, merged.ExpressionSourceID, ok)
	}
	if merged.Eye.LeftOpenness != 0.25 || merged.Expressions.Values[trackingmodel.ExpressionJawOpen] != 0.75 {
		t.Fatalf("merged values = %+v", merged)
	}
}
```

- [ ] **Step 2: Write Auto stickiness, dropout, removal, and revision RED tests**

Cover these deterministic cases with channel-free tests:

```go
func TestAutoRoutingIsStickyUntilSourceUnavailable(t *testing.T) {
	service := newServiceWithClock(sequenceClock(20, 21, 22, 23, 24, 25))
	mustSetGeneration(t, service, 1)
	mustSubmit(t, service, "vendor.z", 1, eyeFrame(1, 0.1))
	mustSubmit(t, service, "vendor.a", 1, eyeFrame(1, 0.2))
	assertEyeSource(t, service, "vendor.z")

	mustSubmit(t, service, "vendor.z", 1, trackingmodel.TrackingFrame{
		Sequence: 2,
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{},
	})
	assertEyeSource(t, service, "vendor.z")

	service.RemoveSource("vendor.z")
	assertEyeSource(t, service, "vendor.a")
}
```

Also assert:

- while routing is manual, cache two capable candidates, switch to Auto, and assert the lexicographically smallest ID wins;
- a selected source that drops the capability triggers reselection;
- a manual unavailable source produces zero capability/data/source ID;
- changing routing recomputes cached frames immediately;
- an accepted non-selected frame does not advance merged Sequence;
- an accepted selected-source frame advances Sequence even when its sample values equal the previous frame;
- removing an unknown or empty ID is idempotent;
- generation advance clears sticky selections.

- [ ] **Step 3: Run routing tests and verify RED**

Run:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-02-internal-tracking\task-03'
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'
New-Item -ItemType Directory -Force $env:TMP,$env:GOCACHE | Out-Null
go test ./internal/tracking -run 'Test(ServiceMerges|AutoRouting|ManualRouting|MergedSequence|RemoveSource)' -count=1
```

Expected: FAIL because accepted sources are cached but not selected or merged.

- [ ] **Step 4: Implement selection and merge recomputation**

Track `eyeSourceID` and `expressionSourceID`. Use one helper per group that preserves the current capable source and otherwise scans for the smallest capable ID:

```go
func chooseAutoSource(current string, sources map[string]sourceState, capability trackingmodel.Capability) string {
	if source, ok := sources[current]; ok && source.frame.Capabilities.Has(capability) {
		return current
	}
	selected := ""
	for id, source := range sources {
		if !source.frame.Capabilities.Has(capability) {
			continue
		}
		if selected == "" || id < selected {
			selected = id
		}
	}
	return selected
}
```

Manual selection returns the configured plugin only when its current frame
advertises the requested capability. Build a fresh merged value with zeroed
unavailable groups. Advance `mergedSequence` and clamp Host time only when a
caller-supplied `force` is true or the merged payload/source IDs differ.

```go
func resolveSource(selection SourceSelection, current string, sources map[string]sourceState, capability trackingmodel.Capability) string {
	if selection.Auto {
		return chooseAutoSource(current, sources, capability)
	}
	source, ok := sources[selection.PluginID]
	if ok && source.frame.Capabilities.Has(capability) {
		return selection.PluginID
	}
	return ""
}

func mergedContentEqual(left, right MergedFrame) bool {
	return left.Generation == right.Generation &&
		left.Capabilities == right.Capabilities &&
		left.Eye == right.Eye &&
		left.Expressions == right.Expressions &&
		left.EyeSourceID == right.EyeSourceID &&
		left.ExpressionSourceID == right.ExpressionSourceID
}

func (s *service) recomputeMergedLocked(force bool) bool {
	nextEye := resolveSource(s.routing.Eye, s.eyeSourceID, s.sources, trackingmodel.CapabilityEye)
	nextExpression := resolveSource(s.routing.Expression, s.expressionSourceID, s.sources, trackingmodel.CapabilityExpression)
	selectionChanged := nextEye != s.eyeSourceID || nextExpression != s.expressionSourceID
	s.eyeSourceID, s.expressionSourceID = nextEye, nextExpression

	next := MergedFrame{Generation: s.generation, EyeSourceID: nextEye, ExpressionSourceID: nextExpression}
	if nextEye != "" {
		next.Capabilities |= trackingmodel.CapabilityEye
		next.Eye = s.sources[nextEye].frame.Eye
	}
	if nextExpression != "" {
		next.Capabilities |= trackingmodel.CapabilityExpression
		next.Expressions = s.sources[nextExpression].frame.Expressions
	}
	if !force && !selectionChanged && mergedContentEqual(next, s.merged) { return false }
	s.advanceMergedSequenceLocked()
	next.Sequence = s.mergedSequence
	next.UpdatedAtNS = s.nextTimeLocked()
	s.merged, s.hasMerged = next, true
	return true
}
```

Call recomputation after accepted Submit, changed RoutingConfig, and source
removal. For Submit, force recomputation when the submitted plugin is selected
after selection or when selection changes. Generation advance continues to
create exactly one empty snapshot through `SetGeneration` and additionally
clears both selected source IDs; do not call recomputation a second time.

- [ ] **Step 5: Run focused stress, package tests, and race tests**

Run:

```powershell
gofmt -w internal/tracking/service.go internal/tracking/routing.go internal/tracking/frame.go internal/tracking/routing_test.go internal/tracking/merge_test.go
go test ./internal/tracking -run 'Test(ServiceMerges|AutoRouting|ManualRouting|MergedSequence|RemoveSource)' -count=100
go test ./internal/tracking -count=1
go test -race ./internal/tracking -run 'Test(ServiceMerges|AutoRouting|ManualRouting|MergedSequence|RemoveSource)' -count=20
```

Expected: all commands exit 0.

- [ ] **Step 6: Primary-agent review and commit**

Inspect deterministic selection, dropout semantics, zeroing, and revision rules. Commit only after independent review has no open Critical/Important findings:

```bash
git add internal/tracking/service.go internal/tracking/routing.go internal/tracking/frame.go internal/tracking/routing_test.go internal/tracking/merge_test.go
git commit -m "feat(tracking): route and merge plugin sources"
```

---

### Task 4: Add Saturating Diagnostics and Bounded Subscriptions

**Files:**
- Modify: `internal/tracking/service.go`
- Modify: `internal/tracking/source.go`
- Create: `internal/tracking/summary.go`
- Create: `internal/tracking/subscription.go`
- Create: `internal/tracking/summary_test.go`
- Create: `internal/tracking/subscription_test.go`

**Interfaces:**
- Consumes: every Task 2 Submit acceptance/rejection path and every Task 3 externally observable state change.
- Produces: exact Summary counters/reasons, `SubscribeMerged(context.Context)`, and `SubscribeSummary(context.Context)` with immediate snapshots, capacity one, replacement, cancellation, and channel closure.

- [ ] **Step 1: Write diagnostic classification and saturation RED tests**

Submit one frame for every rejection category, then assert fixed fields and last rejection. Directly exercise the private saturating helper at MaxUint64:

```go
func TestSummaryCountsAcceptedAndRejectedFrames(t *testing.T) {
	service := newServiceWithClock(sequenceClock(1, 2, 3, 4, 5, 6))
	if err := service.Submit("vendor.eye", 1, trackingmodel.TrackingFrame{}); !errors.Is(err, ErrGenerationUnset) {
		t.Fatalf("unset generation error = %v", err)
	}
	mustSetGeneration(t, service, 2)
	if err := service.Submit("vendor.eye", 1, trackingmodel.TrackingFrame{}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale error = %v", err)
	}
	mustSubmit(t, service, "vendor.eye", 2, eyeFrame(1, 0.4))

	summary := currentSummaryForTest(service)
	if summary.AcceptedFrames != 1 || summary.RejectedFrames != 2 || summary.Rejected.GenerationUnset != 1 || summary.Rejected.StaleGeneration != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestSaturatingAddDoesNotWrap(t *testing.T) {
	if got := saturatingAdd(math.MaxUint64, 1); got != math.MaxUint64 {
		t.Fatalf("saturatingAdd(MaxUint64, 1) = %d", got)
	}
}
```

- [ ] **Step 2: Write subscription RED tests**

Prove immediate values, capacity one, latest replacement, cancellation closure, and nonblocking producers. Coordinate only with channels and contexts, never sleeps:

```go
func TestMergedSubscriberReceivesLatestValueWithoutBlockingProducer(t *testing.T) {
	service := newServiceWithClock(incrementingClock(100))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeMerged(ctx)
	mustSetGeneration(t, service, 1)
	for sequence := uint64(1); sequence <= 100; sequence++ {
		mustSubmit(t, service, "vendor.eye", 1, eyeFrame(sequence, float32(sequence)))
	}
	latest := <-updates
	if latest.Sequence == 0 || latest.Eye.LeftOpenness != 100 {
		t.Fatalf("latest merged = %+v", latest)
	}
}
```

Register and cancel subscribers concurrently with publication in a dedicated test suitable for `go test -race`.

- [ ] **Step 3: Run tests and verify RED**

Run:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-02-internal-tracking\task-04'
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'
New-Item -ItemType Directory -Force $env:TMP,$env:GOCACHE | Out-Null
go test ./internal/tracking -run 'Test(Summary|Saturating|MergedSubscriber|SummarySubscriber|SubscriberCancellation)' -count=1
```

Expected: FAIL because subscription methods and diagnostic counters do not yet exist.

- [ ] **Step 4: Implement counters and immutable Summary construction**

The RED tests now drive the fixed diagnostic values. Move the obsolete empty
`Summary` declaration out of `service.go` and define these in `summary.go`:

```go
type RejectionReason uint8

const (
	RejectionNone RejectionReason = iota
	RejectionGenerationUnset
	RejectionGenerationZero
	RejectionStaleGeneration
	RejectionFutureGeneration
	RejectionInvalidPluginID
	RejectionInvalidFrame
	RejectionSequenceNotIncreasing
	RejectionTimestampRegression
	RejectionSourceClockRegression
)

type RejectionCounts struct {
	GenerationUnset       uint64
	GenerationZero        uint64
	StaleGeneration       uint64
	FutureGeneration      uint64
	InvalidPluginID       uint64
	InvalidFrame          uint64
	SequenceNotIncreasing uint64
	TimestampRegression   uint64
	SourceClockRegression uint64
}

type Rejection struct {
	PluginID   string
	Generation uint64
	Reason     RejectionReason
}

type Summary struct {
	Generation uint64
	Routing    RoutingConfig
	SourceCount int

	EyeSourceID         string
	EyeAvailable        bool
	ExpressionSourceID  string
	ExpressionAvailable bool

	AcceptedFrames uint64
	RejectedFrames uint64
	Rejected       RejectionCounts
	LastRejection  Rejection
}
```

Use a single rejection helper after acquiring the service mutex:

```go
func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

func (c *RejectionCounts) increment(reason RejectionReason) {
	switch reason {
	case RejectionGenerationUnset:
		c.GenerationUnset = saturatingAdd(c.GenerationUnset, 1)
	case RejectionGenerationZero:
		c.GenerationZero = saturatingAdd(c.GenerationZero, 1)
	case RejectionStaleGeneration:
		c.StaleGeneration = saturatingAdd(c.StaleGeneration, 1)
	case RejectionFutureGeneration:
		c.FutureGeneration = saturatingAdd(c.FutureGeneration, 1)
	case RejectionInvalidPluginID:
		c.InvalidPluginID = saturatingAdd(c.InvalidPluginID, 1)
	case RejectionInvalidFrame:
		c.InvalidFrame = saturatingAdd(c.InvalidFrame, 1)
	case RejectionSequenceNotIncreasing:
		c.SequenceNotIncreasing = saturatingAdd(c.SequenceNotIncreasing, 1)
	case RejectionTimestampRegression:
		c.TimestampRegression = saturatingAdd(c.TimestampRegression, 1)
	case RejectionSourceClockRegression:
		c.SourceClockRegression = saturatingAdd(c.SourceClockRegression, 1)
	}
}

func (s *service) rejectLocked(pluginID string, generation uint64, reason RejectionReason, err error) error {
	s.rejectedFrames = saturatingAdd(s.rejectedFrames, 1)
	s.rejected.increment(reason)
	s.lastRejection = Rejection{PluginID: pluginID, Generation: generation, Reason: reason}
	s.publishSummaryLocked()
	return err
}
```

Every Submit error path maps to exactly one reason. AcceptedFrames increments
only after the source state is committed. `Summary` is assembled as a complete
value under lock; `SourceCount` is `len(s.sources)`, and availability is based
on selected source capability rather than validity bits.

- [ ] **Step 5: Implement latest-value registries and cancellation**

Store writable channels in private subscriber maps. Use only nonblocking
operations when replacing a buffered value:

```go
func offerLatest[T any](ch chan T, value T) {
	select {
	case ch <- value:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- value:
	default:
	}
}
```

Subscription creation, publication, removal, and close all use `service.mu`.
If `ctx.Err()` is already non-nil, return an already-closed capacity-one
channel without registering it. Otherwise register, offer the immediate value
when required, and launch one goroutine that waits for `ctx.Done`, locks,
verifies the subscriber identity is still registered, removes it, and closes
the channel.

After both subscription methods exist, replace the obsolete Service interface
in `service.go`, change `NewService` to return that final interface, and add a
compile-time assertion:

```go
type Service interface {
	Submit(string, uint64, trackingmodel.TrackingFrame) error
	SetGeneration(uint64) error
	Generation() uint64

	SetRouting(RoutingConfig) error
	Routing() RoutingConfig
	RemoveSource(string)

	LatestMerged() (MergedFrame, bool)
	SubscribeMerged(context.Context) <-chan MergedFrame
	SubscribeSummary(context.Context) <-chan Summary
}

func NewService() Service {
	return newServiceWithClock(func() int64 { return time.Now().UnixNano() })
}

var _ Service = (*service)(nil)
```

- [ ] **Step 6: Run focused stress and race verification**

Run:

```powershell
gofmt -w internal/tracking/service.go internal/tracking/source.go internal/tracking/summary.go internal/tracking/subscription.go internal/tracking/summary_test.go internal/tracking/subscription_test.go
go test ./internal/tracking -run 'Test(Summary|Saturating|MergedSubscriber|SummarySubscriber|SubscriberCancellation)' -count=100
go test ./internal/tracking -count=1
go test -race ./internal/tracking -run 'Test(Summary|MergedSubscriber|SummarySubscriber|SubscriberCancellation)' -count=30
```

Expected: all commands exit 0 and producers finish while subscribers remain unread.

- [ ] **Step 7: Primary-agent review and commit**

Review every Submit return path against one counter reason, check that no send can block, and verify channel close is serialized with publication. Then commit:

```bash
git add internal/tracking/service.go internal/tracking/source.go internal/tracking/summary.go internal/tracking/subscription.go internal/tracking/summary_test.go internal/tracking/subscription_test.go
git commit -m "feat(tracking): publish bounded merge diagnostics"
```

Run an independent review and fix all Critical/Important findings before Task 5.

---

### Task 5: Add Plugin FrameSink Adapter and Concurrency Proof

**Files:**
- Create: `internal/tracking/sink.go`
- Create: `internal/tracking/sink_test.go`
- Create: `internal/tracking/sink_external_test.go`
- Create: `internal/tracking/concurrency_test.go`
- Modify: focused implementation files only if the race test proves an owning defect.

**Interfaces:**
- Consumes: final `Service.Submit(string, uint64, trackingmodel.TrackingFrame) error`.
- Produces: `FrameSubmitter`, `PluginFrameSink`, `NewPluginFrameSink(FrameSubmitter) PluginFrameSink`; compile-time compatibility with `plugins.FrameSink`.

- [ ] **Step 1: Write adapter compatibility and forwarding RED tests**

Use an external test package for the production dependency-direction assertion:

```go
package tracking_test

import (
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
)

var _ plugins.FrameSink = tracking.PluginFrameSink{}
```

In `package tracking`, use a recording submitter and assert exact plugin ID,
generation, and frame value. Return a sentinel error and prove the no-result
adapter remains non-panicking; the Service owns diagnostics for that rejection.

- [ ] **Step 2: Write concurrent state-machine RED tests**

Run concurrent goroutines for Submit, SetRouting, SetGeneration, RemoveSource,
LatestMerged, Routing, Generation, and subscription create/cancel. Use a
start barrier and WaitGroup:

```go
func TestServiceConcurrentOperationsRemainConsistent(t *testing.T) {
	service := NewService()
	mustSetGeneration(t, service, 1)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			pluginID := fmt.Sprintf("vendor.%02d", worker)
			for sequence := uint64(1); sequence <= 100; sequence++ {
				_ = service.Submit(pluginID, service.Generation(), eyeFrame(sequence, float32(worker)))
				_, _ = service.LatestMerged()
				_ = service.Routing()
			}
		}()
	}
	close(start)
	workers.Wait()
}
```

Add separate control goroutines that only advance generation monotonically and
that create/cancel subscriptions. Assertions after the join must verify a
well-formed current-generation snapshot, canonical capability/data agreement,
and internally consistent Summary totals.

- [ ] **Step 3: Run tests and verify RED**

Run:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-02-internal-tracking\task-05'
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'
New-Item -ItemType Directory -Force $env:TMP,$env:GOCACHE | Out-Null
go test ./internal/tracking -run 'Test(PluginFrameSink|ServiceConcurrent)' -count=1
```

Expected: adapter tests fail to compile before `sink.go`; concurrency tests may compile and pass normally but are not considered proven until the race command in Step 5.

- [ ] **Step 4: Implement the minimal transport-neutral adapter**

```go
type FrameSubmitter interface {
	Submit(string, uint64, trackingmodel.TrackingFrame) error
}

type PluginFrameSink struct {
	target FrameSubmitter
}

func NewPluginFrameSink(target FrameSubmitter) PluginFrameSink {
	return PluginFrameSink{target: target}
}

func (s PluginFrameSink) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) {
	if s.target != nil {
		_ = s.target.Submit(pluginID, generation, frame)
	}
}
```

The zero adapter is a safe no-op. Do not import `internal/plugins` in production.

- [ ] **Step 5: Run full focused normal/race stress**

Run:

```powershell
gofmt -w internal/tracking/sink.go internal/tracking/sink_test.go internal/tracking/sink_external_test.go internal/tracking/concurrency_test.go
go test ./internal/tracking -count=50
go test -race ./internal/tracking -count=20
go test ./internal/plugins ./internal/tracking -count=1
```

Expected: all commands exit 0 with no race report and no import cycle.

- [ ] **Step 6: Primary-agent review and commit**

Inspect the full package for pointer/transport retention and dependency
direction. Commit only adapter/tests and any race-proven owning fix:

```bash
git add internal/tracking/sink.go internal/tracking/sink_test.go internal/tracking/sink_external_test.go internal/tracking/concurrency_test.go
git commit -m "test(tracking): verify plugin sink concurrency"
```

If the race test exposes an owning defect in an earlier implementation file,
stop this commit, record the failing interleaving, add a focused regression
test, and fix it in a separately reviewed commit with exact pathspecs. Run an
independent whole-package review after the adapter commit.

---

### Task 6: Complete Package Specification and M3 Evidence

**Files:**
- Modify: `docs/project/packages/internal-tracking.md`
- Modify: `docs/project/status.md` through `cmd/projectstatus -write`
- Create ignored execution evidence only under `.superpowers/tmp/2026-08-02-internal-tracking/sdd/`.

**Interfaces:**
- Consumes: reviewed Task 1–5 implementation and tests.
- Produces: accurate package responsibilities, completion definition, generated M3 status, final verification evidence, and handoff record.

- [ ] **Step 1: Update the package specification**

Replace the outdated empty-implementation description with the implemented
contract. The document must explicitly cover:

```text
Purpose: generation-aware Host ingest, routing, and stable merge
Responsibilities: validation, canonicalization, ordering, generation rejection,
  sticky Eye/Expression selection, merge, latest-value publication, diagnostics
Non-responsibilities: plugin supervision, processing/filtering, avatar planning,
  Application wiring, Head data, shared-memory synchronization
Interfaces: Service, PluginFrameSink, RoutingConfig, MergedFrame, Summary
Owned data: current generation, one latest frame per plugin, selections,
  immutable latest snapshots, service-lifetime counters
Concurrency: linearizable mutex state machine; capacity-one nonblocking subscribers
Errors: stable errors.Is sentinels; rejected frames never mutate merged state
Performance: O(1) ordinary Submit, O(P) reselection, no history/backlog
```

Update known gaps so it does not claim Service or MergedFrame is empty. Record
only genuine deferred work from the design.

- [ ] **Step 2: Run fresh final verification from a clean source commit**

First commit the package spec with the primary agent after `git diff --check`:

```bash
git add docs/project/packages/internal-tracking.md
git commit -m "docs: specify host tracking merge"
```

Then run with a fresh scoped cache:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-02-internal-tracking\final-verification'
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'
New-Item -ItemType Directory -Force $env:TMP,$env:GOCACHE | Out-Null
go test ./...
go test -race ./internal/tracking ./internal/plugins ./pkg/trackingmodel
go vet ./internal/tracking ./internal/plugins ./pkg/trackingmodel
```

Run formatting and repository checks:

```bash
gofmt -d internal/tracking/*.go
git diff --check
git status --short
```

Expected: tests/vet exit 0, gofmt emits no diff, and tracked worktree is clean.

- [ ] **Step 3: Run independent final review**

Review the complete range from `78ed7b7` through the source/spec HEAD against
every section of the design. The reviewer must explicitly re-prove generation
atomicity, mutation-after-validation, deterministic routing, dropout behavior,
nonblocking subscription close safety, ownership, and dependency direction.

Fix all Critical and Important findings with strict RED/GREEN cycles and scoped
re-review. Record non-blocking Minor findings in the handoff rather than hiding
them.

- [ ] **Step 4: Regenerate project status from the clean source commit**

Run:

```powershell
$taskTmp='F:\dev\vrcft-go\.superpowers\tmp\2026-08-02-internal-tracking\status-write'
$env:TMP=$taskTmp; $env:TEMP=$taskTmp; $env:GOCACHE="$taskTmp\gocache"; $env:GOTOOLCHAIN='local'
New-Item -ItemType Directory -Force $env:TMP,$env:GOCACHE | Out-Null
go run ./cmd/projectstatus -write
```

Exit 1 is acceptable only for unrelated incomplete milestones. Inspect the
diff: `docs/project/status.md` must record a clean source commit, Dirty false,
`internal-tracking` complete 100%, and M3 complete 100%. It must not conceal
other project blockers.

Commit generated status separately:

```bash
git add docs/project/status.md
git commit -m "docs: refresh tracking completion evidence"
```

- [ ] **Step 5: Recheck generated status and final tree**

Run `go run ./cmd/projectstatus -check` with a fresh scoped cache. It may exit 1
only because the global project remains blocked; output must not say status is
stale. Then run:

```bash
git status --short
git log -12 --oneline
```

Expected: no tracked changes. Update the ignored SDD ledger/handoff with commit
IDs, exact verification output, review verdict, and any deferred Minor items.

---

## Plan Completion Gate

Do not mark M3 complete merely because the status detector's placeholder checks
pass. Completion requires all six tasks, strict RED/GREEN evidence, focused
normal and race verification, an independent whole-package review with no open
Critical or Important findings, accurate package documentation, and generated
clean-source project status.
