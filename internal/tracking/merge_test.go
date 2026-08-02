package tracking

import (
	"math"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestServiceMergesGroupsFromDifferentSources(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 10 })
	mustSetGeneration(t, s, 2)
	mustSubmit(t, s, "eye.plugin", 2, eyeFrame(11, 0.25))
	mustSubmit(t, s, "expression.plugin", 2, expressionFrame(22, trackingmodel.ExpressionJawOpen, 0.75))

	wantExpressions := trackingmodel.ExpressionSet{}
	wantExpressions.Set(trackingmodel.ExpressionJawOpen, 0.75)
	want := MergedFrame{
		Generation:   2,
		Sequence:     3,
		UpdatedAtNS:  10,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.25,
		},
		Expressions:        wantExpressions,
		EyeSourceID:        "eye.plugin",
		ExpressionSourceID: "expression.plugin",
	}
	if got := latestMerged(t, s); got != want {
		t.Fatalf("LatestMerged() = %#v, want %#v", got, want)
	}
}

func TestMergedSequenceIgnoresNonSelectedFramesAndAdvancesForSelectedIdenticalData(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.z", 1, eyeFrame(1, 0.25))
	mustSubmit(t, s, "vendor.a", 1, eyeFrame(1, 0.5))
	beforeNonSelected := latestMerged(t, s)

	mustSubmit(t, s, "vendor.a", 1, eyeFrame(2, 0.75))
	afterNonSelected := latestMerged(t, s)
	if afterNonSelected != beforeNonSelected {
		t.Fatalf("non-selected Submit merged = %#v, want unchanged %#v", afterNonSelected, beforeNonSelected)
	}

	mustSubmit(t, s, "vendor.z", 1, eyeFrame(2, 0.25))
	afterSelected := latestMerged(t, s)
	if afterSelected.Sequence != beforeNonSelected.Sequence+1 || afterSelected.UpdatedAtNS <= beforeNonSelected.UpdatedAtNS {
		t.Fatalf("selected identical Submit merged ordering = (%d, %d), want Sequence %d and time > %d", afterSelected.Sequence, afterSelected.UpdatedAtNS, beforeNonSelected.Sequence+1, beforeNonSelected.UpdatedAtNS)
	}
	if afterSelected.Eye != beforeNonSelected.Eye || afterSelected.EyeSourceID != "vendor.z" {
		t.Fatalf("selected identical Submit content = %#v, want unchanged selected payload from vendor.z", afterSelected)
	}
}

func TestServiceMergesGenerationAdvanceClearsStickySourcesWithOneEmptyRevision(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		value := int64(10 + clockCalls)
		clockCalls++
		return value
	})
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.z", 1, eyeFrame(1, 0.1))
	mustSubmit(t, s, "vendor.a", 1, eyeFrame(1, 0.2))
	if s.eyeSourceID != "vendor.z" {
		t.Fatalf("eyeSourceID before generation advance = %q, want vendor.z", s.eyeSourceID)
	}
	if clockCalls != 4 {
		t.Fatalf("Host clock calls before generation advance = %d, want 4", clockCalls)
	}

	mustSetGeneration(t, s, 2)
	want := MergedFrame{Generation: 2, Sequence: 3, UpdatedAtNS: 14}
	if got := latestMerged(t, s); got != want {
		t.Fatalf("generation advance merged = %#v, want %#v", got, want)
	}
	if s.eyeSourceID != "" || s.expressionSourceID != "" {
		t.Fatalf("sticky sources after generation advance = (%q, %q), want empty", s.eyeSourceID, s.expressionSourceID)
	}
	if clockCalls != 5 {
		t.Fatalf("Host clock calls after generation advance = %d, want exactly 5", clockCalls)
	}
}

func TestMergedSequenceSaturatesAcrossRoutingSubmitAndRemovalChanges(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls)
	})
	s.mergedSequence = math.MaxUint64
	mustSetGeneration(t, s, 1)
	if got := latestMerged(t, s); got.Sequence != math.MaxUint64 {
		t.Fatalf("generation Sequence = %d, want MaxUint64", got.Sequence)
	}

	mustSubmit(t, s, "eye.plugin", 1, eyeFrame(1, 0.5))
	afterSubmit := latestMerged(t, s)
	if afterSubmit.Sequence != math.MaxUint64 || afterSubmit.EyeSourceID != "eye.plugin" {
		t.Fatalf("Submit saturated merged = %#v, want MaxUint64 selected eye.plugin", afterSubmit)
	}

	manual := RoutingConfig{
		Eye:        SourceSelection{PluginID: "eye.plugin"},
		Expression: SourceSelection{Auto: true},
	}
	if err := s.SetRouting(manual); err != nil {
		t.Fatalf("SetRouting(manual) error = %v", err)
	}
	afterRouting := latestMerged(t, s)
	if afterRouting.Sequence != math.MaxUint64 || afterRouting.UpdatedAtNS <= afterSubmit.UpdatedAtNS {
		t.Fatalf("routing saturated ordering = (%d, %d), want MaxUint64 and time > %d", afterRouting.Sequence, afterRouting.UpdatedAtNS, afterSubmit.UpdatedAtNS)
	}

	s.RemoveSource("eye.plugin")
	afterRemoval := latestMerged(t, s)
	if afterRemoval.Sequence != math.MaxUint64 || afterRemoval.Capabilities != 0 || afterRemoval.UpdatedAtNS <= afterRouting.UpdatedAtNS {
		t.Fatalf("removal saturated merged = %#v, want empty MaxUint64 with later time", afterRemoval)
	}
	if s.mergedSequence != math.MaxUint64 {
		t.Fatalf("mergedSequence = %d, want MaxUint64", s.mergedSequence)
	}
}

func TestMergedSequenceClampsRegressingHostClock(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		value := int64(100 - clockCalls*10)
		clockCalls++
		return value
	})
	mustSetGeneration(t, s, 1)
	first := latestMerged(t, s)
	mustSubmit(t, s, "eye.plugin", 1, eyeFrame(1, 0.5))
	second := latestMerged(t, s)
	manual := RoutingConfig{
		Eye:        SourceSelection{PluginID: "eye.plugin"},
		Expression: SourceSelection{Auto: true},
	}
	if err := s.SetRouting(manual); err != nil {
		t.Fatalf("SetRouting(manual) error = %v", err)
	}
	third := latestMerged(t, s)

	if first.UpdatedAtNS != 100 || second.UpdatedAtNS != 100 || third.UpdatedAtNS != 100 {
		t.Fatalf("UpdatedAtNS values = (%d, %d, %d), want (100, 100, 100)", first.UpdatedAtNS, second.UpdatedAtNS, third.UpdatedAtNS)
	}
	if first.Sequence != 1 || second.Sequence != 2 || third.Sequence != 3 {
		t.Fatalf("Sequence values = (%d, %d, %d), want (1, 2, 3)", first.Sequence, second.Sequence, third.Sequence)
	}
}

func TestServiceMergesLatestFrameAsOwnedValue(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 90 })
	mustSetGeneration(t, s, 4)
	mustSubmit(t, s, "eye.plugin", 4, eyeFrame(1, 0.25))
	mustSubmit(t, s, "expression.plugin", 4, expressionFrame(1, trackingmodel.ExpressionJawOpen, 0.75))
	want := latestMerged(t, s)

	mutated := want
	mutated.Generation = 99
	mutated.Eye.LeftOpenness = 1
	mutated.Expressions.Values[trackingmodel.ExpressionJawOpen] = 1
	mutated.EyeSourceID = "mutated"
	mutated.ExpressionSourceID = "mutated"
	if got := latestMerged(t, s); got != want {
		t.Fatalf("LatestMerged() after caller mutation = %#v, want %#v", got, want)
	}
}
