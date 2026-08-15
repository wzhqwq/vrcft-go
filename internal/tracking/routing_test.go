package tracking

import (
	"errors"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func mustSetGeneration(t *testing.T, s *service, generation uint64) {
	t.Helper()
	if err := s.SetGeneration(generation); err != nil {
		t.Fatalf("SetGeneration(%d) error = %v", generation, err)
	}
}

func mustSubmit(t *testing.T, s *service, pluginID string, generation uint64, frame trackingmodel.TrackingFrame) {
	t.Helper()
	if err := s.Submit(pluginID, generation, frame); err != nil {
		t.Fatalf("Submit(%q) error = %v", pluginID, err)
	}
}

func eyeFrame(sequence uint64, openness float32) trackingmodel.TrackingFrame {
	return trackingmodel.TrackingFrame{
		Sequence:     sequence,
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: openness,
		},
	}
}

func expressionFrame(sequence uint64, expression trackingmodel.ExpressionID, value float32) trackingmodel.TrackingFrame {
	frame := trackingmodel.TrackingFrame{
		Sequence:     sequence,
		Capabilities: trackingmodel.CapabilityExpression,
	}
	frame.Expressions.Set(expression, value)
	return frame
}

func lipFrame(sequence uint64) trackingmodel.TrackingFrame {
	return trackingmodel.TrackingFrame{
		Sequence:     sequence,
		Capabilities: trackingmodel.CapabilityLip,
	}
}

func latestMerged(t *testing.T, s *service) MergedFrame {
	t.Helper()
	merged, ok := s.LatestMerged()
	if !ok {
		t.Fatal("LatestMerged() ok = false")
	}
	return merged
}

func TestAutoRoutingHelpersAreStickyAndDeterministic(t *testing.T) {
	sources := map[string]sourceState{
		"vendor.z": {frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityEye}},
		"vendor.a": {frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityEye}},
		"vendor.m": {frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityExpression}},
		"vendor.l": {frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityLip}},
	}

	if got := chooseAutoSource("vendor.z", sources, trackingmodel.CapabilityEye); got != "vendor.z" {
		t.Fatalf("chooseAutoSource(sticky Eye) = %q, want %q", got, "vendor.z")
	}
	if got := chooseAutoSource("", sources, trackingmodel.CapabilityEye); got != "vendor.a" {
		t.Fatalf("chooseAutoSource(initial Eye) = %q, want %q", got, "vendor.a")
	}
	if got := chooseAutoSource("vendor.z", sources, trackingmodel.CapabilityExpression); got != "vendor.m" {
		t.Fatalf("chooseAutoSource(Expression) = %q, want %q", got, "vendor.m")
	}
	if got := chooseAutoSource("", sources, trackingmodel.CapabilityLip); got != "vendor.l" {
		t.Fatalf("chooseAutoSource(Lip) = %q, want %q", got, "vendor.l")
	}
}

func TestManualRoutingHelperRequiresConfiguredCapability(t *testing.T) {
	sources := map[string]sourceState{
		"eye.plugin": {frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityEye}},
	}
	manualEye := SourceSelection{PluginID: "eye.plugin"}
	missing := SourceSelection{PluginID: "missing.plugin"}

	if got := resolveSource(manualEye, "fallback", sources, trackingmodel.CapabilityEye); got != "eye.plugin" {
		t.Fatalf("resolveSource(manual Eye) = %q, want %q", got, "eye.plugin")
	}
	if got := resolveSource(manualEye, "fallback", sources, trackingmodel.CapabilityExpression); got != "" {
		t.Fatalf("resolveSource(manual incapable) = %q, want empty", got)
	}
	if got := resolveSource(missing, "fallback", sources, trackingmodel.CapabilityEye); got != "" {
		t.Fatalf("resolveSource(manual missing) = %q, want empty", got)
	}
}

func TestManualRoutingSelectsAvailableAndNeverFallsBack(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 20 })
	mustSetGeneration(t, s, 3)
	mustSubmit(t, s, "eye.plugin", 3, eyeFrame(1, 0.25))
	mustSubmit(t, s, "expression.plugin", 3, expressionFrame(1, trackingmodel.ExpressionJawOpen, 0.75))

	config := RoutingConfig{
		Eye:        SourceSelection{PluginID: "eye.plugin"},
		Expression: SourceSelection{PluginID: "missing.plugin"},
		Lip:        SourceSelection{Auto: true},
	}
	if err := s.SetRouting(config); err != nil {
		t.Fatalf("SetRouting(available/missing) error = %v", err)
	}
	want := MergedFrame{
		Generation:     3,
		Sequence:       4,
		UpdatedAtNS:    22,
		EyeUpdatedAtNS: 21,
		Capabilities:   trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.25,
		},
		EyeSourceID: "eye.plugin",
	}
	if got := latestMerged(t, s); got != want {
		t.Fatalf("manual available/missing merged = %#v, want %#v", got, want)
	}

	config = RoutingConfig{
		Eye:        SourceSelection{PluginID: "expression.plugin"},
		Expression: SourceSelection{PluginID: "expression.plugin"},
		Lip:        SourceSelection{Auto: true},
	}
	if err := s.SetRouting(config); err != nil {
		t.Fatalf("SetRouting(incapable/available) error = %v", err)
	}
	want = MergedFrame{
		Generation:            3,
		Sequence:              5,
		UpdatedAtNS:           22,
		ExpressionUpdatedAtNS: 22,
		Capabilities:          trackingmodel.CapabilityExpression,
		Expressions:           expressionFrame(0, trackingmodel.ExpressionJawOpen, 0.75).Expressions,
		ExpressionSourceID:    "expression.plugin",
	}
	if got := latestMerged(t, s); got != want {
		t.Fatalf("manual incapable/available merged = %#v, want %#v", got, want)
	}
}

func TestManualRoutingChangedConfigRecomputesCachedFramesAndValidatesBeforeMutation(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	mustSetGeneration(t, s, 7)
	manualMissing := RoutingConfig{
		Eye:        SourceSelection{PluginID: "missing.eye"},
		Expression: SourceSelection{PluginID: "missing.expression"},
		Lip:        SourceSelection{PluginID: "missing.lip"},
	}
	if err := s.SetRouting(manualMissing); err != nil {
		t.Fatalf("SetRouting(manual missing) error = %v", err)
	}
	mustSubmit(t, s, "vendor.eye", 7, eyeFrame(1, 0.4))
	mustSubmit(t, s, "vendor.expression", 7, expressionFrame(1, trackingmodel.ExpressionJawOpen, 0.6))
	beforeAuto := latestMerged(t, s)

	auto := RoutingConfig{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}, Lip: SourceSelection{Auto: true}}
	if err := s.SetRouting(auto); err != nil {
		t.Fatalf("SetRouting(auto) error = %v", err)
	}
	afterAuto := latestMerged(t, s)
	if afterAuto.Sequence != beforeAuto.Sequence+1 || afterAuto.EyeSourceID != "vendor.eye" || afterAuto.ExpressionSourceID != "vendor.expression" {
		t.Fatalf("SetRouting(auto) merged = %#v, want cached sources and one revision after %#v", afterAuto, beforeAuto)
	}

	manualSameSources := RoutingConfig{
		Eye:        SourceSelection{PluginID: "vendor.eye"},
		Expression: SourceSelection{PluginID: "vendor.expression"},
		Lip:        SourceSelection{Auto: true},
	}
	if err := s.SetRouting(manualSameSources); err != nil {
		t.Fatalf("SetRouting(manual same sources) error = %v", err)
	}
	afterForced := latestMerged(t, s)
	if afterForced.Sequence != afterAuto.Sequence+1 || afterForced.EyeSourceID != afterAuto.EyeSourceID || afterForced.ExpressionSourceID != afterAuto.ExpressionSourceID {
		t.Fatalf("changed routing merged = %#v, want same content with one forced revision after %#v", afterForced, afterAuto)
	}

	beforeEqualCalls := clockCalls
	if err := s.SetRouting(manualSameSources); err != nil {
		t.Fatalf("SetRouting(equal) error = %v", err)
	}
	if got := latestMerged(t, s); got != afterForced || clockCalls != beforeEqualCalls {
		t.Fatalf("equal SetRouting state = (%#v, clock calls %d), want unchanged (%#v, %d)", got, clockCalls, afterForced, beforeEqualCalls)
	}

	invalid := RoutingConfig{
		Eye:        SourceSelection{Auto: true, PluginID: "invalid"},
		Expression: SourceSelection{Auto: true},
		Lip:        SourceSelection{Auto: true},
	}
	if err := s.SetRouting(invalid); !errors.Is(err, ErrInvalidRouting) {
		t.Fatalf("SetRouting(invalid) error = %v, want ErrInvalidRouting", err)
	}
	if got := s.Routing(); got != manualSameSources {
		t.Fatalf("Routing() after invalid config = %#v, want %#v", got, manualSameSources)
	}
	if got := latestMerged(t, s); got != afterForced || clockCalls != beforeEqualCalls {
		t.Fatalf("invalid SetRouting state = (%#v, clock calls %d), want unchanged (%#v, %d)", got, clockCalls, afterForced, beforeEqualCalls)
	}
}

func TestManualRoutingBeforeGenerationUpdatesConfigWithoutPublishing(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	manual := RoutingConfig{
		Eye:        SourceSelection{PluginID: "eye.plugin"},
		Expression: SourceSelection{PluginID: "expression.plugin"},
		Lip:        SourceSelection{PluginID: "lip.plugin"},
	}

	if err := s.SetRouting(manual); err != nil {
		t.Fatalf("SetRouting(manual) error = %v", err)
	}
	if got := s.Routing(); got != manual {
		t.Errorf("Routing() = %#v, want updated %#v", got, manual)
	}
	if s.mergedSequence != 0 {
		t.Errorf("mergedSequence = %d, want 0", s.mergedSequence)
	}
	if s.lastHostTimeNS != 0 || clockCalls != 0 {
		t.Errorf("Host time state = (%d, %d calls), want (0, 0 calls)", s.lastHostTimeNS, clockCalls)
	}
	if got, ok := s.LatestMerged(); ok || got != (MergedFrame{}) {
		t.Errorf("LatestMerged() = (%#v, %t), want (zero, false)", got, ok)
	}
	if t.Failed() {
		return
	}

	mustSetGeneration(t, s, 1)
	if got := latestMerged(t, s); got != (MergedFrame{Generation: 1, Sequence: 1, UpdatedAtNS: 10}) {
		t.Fatalf("generation snapshot = %#v, want one empty generation revision", got)
	}
	auto := RoutingConfig{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}, Lip: SourceSelection{Auto: true}}
	if err := s.SetRouting(auto); err != nil {
		t.Fatalf("SetRouting(auto after generation) error = %v", err)
	}
	if got := latestMerged(t, s); got != (MergedFrame{Generation: 1, Sequence: 2, UpdatedAtNS: 20}) {
		t.Fatalf("post-generation routing snapshot = %#v, want forced revision", got)
	}
}

func TestAutoRoutingFirstArrivalRemainsSticky(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 30 })
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.z", 1, eyeFrame(1, 0.1))
	mustSubmit(t, s, "vendor.a", 1, eyeFrame(1, 0.2))

	want := MergedFrame{
		Generation:     1,
		Sequence:       2,
		UpdatedAtNS:    31,
		EyeUpdatedAtNS: 31,
		Capabilities:   trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.1,
		},
		EyeSourceID: "vendor.z",
	}
	if got := latestMerged(t, s); got != want {
		t.Fatalf("sticky Auto merged = %#v, want %#v", got, want)
	}
}

func TestAutoRoutingInitialChoiceIsLexicographicallySmallest(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 40 })
	mustSetGeneration(t, s, 1)
	manual := RoutingConfig{
		Eye:        SourceSelection{PluginID: "missing.eye"},
		Expression: SourceSelection{PluginID: "missing.expression"},
		Lip:        SourceSelection{PluginID: "missing.lip"},
	}
	if err := s.SetRouting(manual); err != nil {
		t.Fatalf("SetRouting(manual) error = %v", err)
	}
	mustSubmit(t, s, "vendor.z", 1, eyeFrame(1, 0.1))
	mustSubmit(t, s, "vendor.a", 1, eyeFrame(1, 0.2))

	autoEye := RoutingConfig{
		Eye:        SourceSelection{Auto: true},
		Expression: SourceSelection{PluginID: "missing.expression"},
		Lip:        SourceSelection{PluginID: "missing.lip"},
	}
	if err := s.SetRouting(autoEye); err != nil {
		t.Fatalf("SetRouting(auto Eye) error = %v", err)
	}
	if got := latestMerged(t, s); got.EyeSourceID != "vendor.a" || got.Eye.LeftOpenness != 0.2 {
		t.Fatalf("initial Auto Eye = (%q, %v), want (%q, %v)", got.EyeSourceID, got.Eye.LeftOpenness, "vendor.a", float32(0.2))
	}
}

func TestAutoRoutingValidityDropoutDoesNotSwitchSource(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 50 })
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.z", 1, eyeFrame(1, 0.1))
	mustSubmit(t, s, "vendor.a", 1, eyeFrame(1, 0.2))
	mustSubmit(t, s, "vendor.z", 1, trackingmodel.TrackingFrame{
		Sequence:     2,
		Capabilities: trackingmodel.CapabilityEye,
	})

	want := MergedFrame{
		Generation:     1,
		Sequence:       3,
		UpdatedAtNS:    53,
		EyeUpdatedAtNS: 53,
		Capabilities:   trackingmodel.CapabilityEye,
		EyeSourceID:    "vendor.z",
	}
	if got := latestMerged(t, s); got != want {
		t.Fatalf("dropout Auto merged = %#v, want %#v", got, want)
	}
}

func TestAutoRoutingCapabilityLossReselectsSmallestCandidate(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 60 })
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.z", 1, eyeFrame(1, 0.1))
	mustSubmit(t, s, "vendor.m", 1, eyeFrame(1, 0.3))
	mustSubmit(t, s, "vendor.a", 1, eyeFrame(1, 0.2))
	mustSubmit(t, s, "vendor.z", 1, trackingmodel.TrackingFrame{Sequence: 2})

	if got := latestMerged(t, s); got.EyeSourceID != "vendor.a" || got.Eye.LeftOpenness != 0.2 || got.Sequence != 3 {
		t.Fatalf("capability-loss merged = %#v, want vendor.a value 0.2 at Sequence 3", got)
	}
}

func TestRemoveSourceReselectsAutoAndIgnoresNonSelectedOrUnknown(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.z", 1, eyeFrame(1, 0.1))
	mustSubmit(t, s, "vendor.a", 1, eyeFrame(1, 0.2))
	mustSubmit(t, s, "vendor.m", 1, eyeFrame(1, 0.3))
	before := latestMerged(t, s)
	beforeCalls := clockCalls

	s.RemoveSource("vendor.m")
	s.RemoveSource("unknown")
	s.RemoveSource("")
	if got := latestMerged(t, s); got != before || clockCalls != beforeCalls {
		t.Fatalf("idempotent/non-selected removals state = (%#v, clock calls %d), want (%#v, %d)", got, clockCalls, before, beforeCalls)
	}

	s.RemoveSource("vendor.z")
	after := latestMerged(t, s)
	if after.EyeSourceID != "vendor.a" || after.Eye.LeftOpenness != 0.2 || after.Sequence != before.Sequence+1 {
		t.Fatalf("selected Auto removal merged = %#v, want vendor.a value 0.2 one revision after %#v", after, before)
	}
}

func TestRemoveSourceManualSelectionBecomesUnavailableWithoutFallback(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 70 })
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "manual.eye", 1, eyeFrame(1, 0.1))
	mustSubmit(t, s, "fallback.eye", 1, eyeFrame(1, 0.2))
	manual := RoutingConfig{
		Eye:        SourceSelection{PluginID: "manual.eye"},
		Expression: SourceSelection{Auto: true},
		Lip:        SourceSelection{Auto: true},
	}
	if err := s.SetRouting(manual); err != nil {
		t.Fatalf("SetRouting(manual Eye) error = %v", err)
	}
	before := latestMerged(t, s)

	s.RemoveSource("manual.eye")
	want := MergedFrame{Generation: 1, Sequence: before.Sequence + 1, UpdatedAtNS: 72}
	if got := latestMerged(t, s); got != want {
		t.Fatalf("manual selected removal merged = %#v, want %#v", got, want)
	}
}

func TestManualRoutingReturnsValueCopy(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 80 })
	want := RoutingConfig{
		Eye:        SourceSelection{PluginID: "eye.plugin"},
		Expression: SourceSelection{PluginID: "expression.plugin"},
		Lip:        SourceSelection{PluginID: "lip.plugin"},
	}
	if err := s.SetRouting(want); err != nil {
		t.Fatalf("SetRouting() error = %v", err)
	}

	got := s.Routing()
	got.Eye.Auto = true
	got.Eye.PluginID = "mutated"
	got.Expression.PluginID = "mutated"
	got.Lip.PluginID = "mutated"
	if after := s.Routing(); after != want {
		t.Fatalf("Routing() after caller mutation = %#v, want %#v", after, want)
	}
}

func TestLipRoutingIsIndependentStickyAndManualHasNoFallback(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 90 })
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.lip.z", 1, lipFrame(1))
	mixed := trackingmodel.TrackingFrame{
		Sequence:     1,
		Capabilities: trackingmodel.CapabilityLip | trackingmodel.CapabilityExpression,
	}
	mixed.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.6)
	mustSubmit(t, s, "vendor.mixed", 1, mixed)
	mustSubmit(t, s, "vendor.lip.a", 1, lipFrame(1))

	sticky := latestMerged(t, s)
	if sticky.LipSourceID != "vendor.lip.z" || sticky.ExpressionSourceID != "vendor.mixed" || sticky.Expressions.Values[trackingmodel.ExpressionJawOpen] != 0.6 {
		t.Fatalf("mixed-capability independent groups = %#v, want sticky Lip vendor.lip.z and Expression vendor.mixed", sticky)
	}

	s.RemoveSource("vendor.lip.z")
	afterRemoval := latestMerged(t, s)
	if afterRemoval.LipSourceID != "vendor.lip.a" || afterRemoval.ExpressionSourceID != sticky.ExpressionSourceID || afterRemoval.Expressions != sticky.Expressions {
		t.Fatalf("Lip removal re-resolution = %#v, want vendor.lip.a without Expression mutation", afterRemoval)
	}

	manualMissing := RoutingConfig{
		Eye:        SourceSelection{Auto: true},
		Expression: SourceSelection{Auto: true},
		Lip:        SourceSelection{PluginID: "vendor.lip.z"},
	}
	if err := s.SetRouting(manualMissing); err != nil {
		t.Fatalf("SetRouting(manual missing Lip) error = %v", err)
	}
	missing := latestMerged(t, s)
	if missing.LipSourceID != "" || missing.Capabilities.Has(trackingmodel.CapabilityLip) {
		t.Fatalf("manual missing Lip = %#v, want unavailable without fallback", missing)
	}
	if missing.ExpressionSourceID != sticky.ExpressionSourceID || missing.Expressions != sticky.Expressions {
		t.Fatalf("manual Lip change mutated Expression: before %#v after %#v", sticky, missing)
	}

	mustSubmit(t, s, "vendor.lip.z", 1, lipFrame(1))
	selected := latestMerged(t, s)
	if selected.LipSourceID != "vendor.lip.z" || !selected.Capabilities.Has(trackingmodel.CapabilityLip) {
		t.Fatalf("manual Lip became available = %#v, want vendor.lip.z", selected)
	}
	mustSubmit(t, s, "vendor.lip.z", 1, trackingmodel.TrackingFrame{Sequence: 2})
	lost := latestMerged(t, s)
	if lost.LipSourceID != "" || lost.Capabilities.Has(trackingmodel.CapabilityLip) {
		t.Fatalf("manual Lip capability loss = %#v, want unavailable without fallback", lost)
	}
	if lost.ExpressionSourceID != sticky.ExpressionSourceID || lost.Expressions != sticky.Expressions {
		t.Fatalf("Lip capability loss mutated Expression: before %#v after %#v", sticky, lost)
	}
}
