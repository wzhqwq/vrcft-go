package tracking

import (
	"errors"
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
		Generation:            2,
		Sequence:              3,
		UpdatedAtNS:           12,
		EyeUpdatedAtNS:        11,
		ExpressionUpdatedAtNS: 12,
		Capabilities:          trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
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
	if afterSelected.EyeUpdatedAtNS <= beforeNonSelected.EyeUpdatedAtNS {
		t.Fatalf("selected identical Eye freshness = %d, want greater than %d", afterSelected.EyeUpdatedAtNS, beforeNonSelected.EyeUpdatedAtNS)
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
	if s.eyeSourceID != "" || s.expressionSourceID != "" || s.lipSourceID != "" {
		t.Fatalf("sticky sources after generation advance = (%q, %q, %q), want empty", s.eyeSourceID, s.expressionSourceID, s.lipSourceID)
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
		Lip:        SourceSelection{Auto: true},
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
		Lip:        SourceSelection{Auto: true},
	}
	if err := s.SetRouting(manual); err != nil {
		t.Fatalf("SetRouting(manual) error = %v", err)
	}
	third := latestMerged(t, s)

	if first.UpdatedAtNS != 100 || second.UpdatedAtNS != 101 || third.UpdatedAtNS != 101 {
		t.Fatalf("UpdatedAtNS values = (%d, %d, %d), want (100, 101, 101)", first.UpdatedAtNS, second.UpdatedAtNS, third.UpdatedAtNS)
	}
	if first.Sequence != 1 || second.Sequence != 2 || third.Sequence != 3 {
		t.Fatalf("Sequence values = (%d, %d, %d), want (1, 2, 3)", first.Sequence, second.Sequence, third.Sequence)
	}
}

func TestSelectedGroupFreshnessAdvancesSafelyToHostTimeSaturation(t *testing.T) {
	tests := []struct {
		name  string
		value float32
	}{
		{name: "changed value", value: 0.75},
		{name: "same value", value: 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const start = math.MaxInt64 - 3
			s := newServiceWithClock(func() int64 { return start })
			mustSetGeneration(t, s, 1)

			wantFreshness := []int64{math.MaxInt64 - 2, math.MaxInt64 - 1, math.MaxInt64}
			for index, want := range wantFreshness {
				mustSubmit(t, s, "eye.plugin", 1, eyeFrame(uint64(index+1), 0.5))
				got := latestMerged(t, s)
				if got.EyeUpdatedAtNS != want || got.UpdatedAtNS != want {
					t.Fatalf("update %d timestamps = (%d group, %d merged), want (%d, %d)", index+1, got.EyeUpdatedAtNS, got.UpdatedAtNS, want, want)
				}
			}

			beforeSource := s.sources["eye.plugin"]
			beforeMerged := latestMerged(t, s)
			beforeSequence := s.mergedSequence
			beforeHostTime := s.lastHostTimeNS
			beforeAccepted := s.acceptedFrames
			beforeRejected := s.rejectedFrames
			beforeRejections := s.rejected
			beforeLastRejection := s.lastRejection
			err := s.Submit("eye.plugin", 1, eyeFrame(4, tt.value))
			if !errors.Is(err, ErrTimestampRegression) {
				t.Fatalf("saturated Submit() error = %v, want ErrTimestampRegression", err)
			}

			if got := s.sources["eye.plugin"]; got != beforeSource {
				t.Fatalf("source after saturated rejection = %#v, want unchanged %#v", got, beforeSource)
			}
			if got := latestMerged(t, s); got != beforeMerged || s.mergedSequence != beforeSequence || s.lastHostTimeNS != beforeHostTime {
				t.Fatalf("merged state after saturated rejection = %#v sequence %d host time %d, want unchanged %#v sequence %d host time %d", got, s.mergedSequence, s.lastHostTimeNS, beforeMerged, beforeSequence, beforeHostTime)
			}
			if s.acceptedFrames != beforeAccepted {
				t.Fatalf("AcceptedFrames = %d, want unchanged %d", s.acceptedFrames, beforeAccepted)
			}
			wantRejections := beforeRejections
			wantRejections.TimestampRegression++
			if s.rejectedFrames != beforeRejected+1 || s.rejected != wantRejections {
				t.Fatalf("rejection diagnostics = (%d, %+v), want one TimestampRegression after (%d, %+v)", s.rejectedFrames, s.rejected, beforeRejected, beforeRejections)
			}
			wantLast := Rejection{PluginID: "eye.plugin", Generation: 1, Reason: RejectionTimestampRegression}
			if s.lastRejection != wantLast || s.lastRejection == beforeLastRejection {
				t.Fatalf("LastRejection = %+v, want %+v", s.lastRejection, wantLast)
			}
		})
	}
}

func TestReceiptTimeSaturationRejectsBeforeInitialSelection(t *testing.T) {
	s := newServiceWithClock(func() int64 { return math.MaxInt64 })
	mustSetGeneration(t, s, 1)
	before := latestMerged(t, s)

	err := s.Submit("eye.plugin", 1, eyeFrame(1, 0.5))
	if !errors.Is(err, ErrTimestampRegression) {
		t.Fatalf("saturated initial Submit() error = %v, want ErrTimestampRegression", err)
	}
	if len(s.sources) != 0 || s.eyeSourceID != "" {
		t.Fatalf("saturated initial rejection retained source state: sources=%#v EyeSourceID=%q", s.sources, s.eyeSourceID)
	}
	if got := latestMerged(t, s); got != before || s.mergedSequence != before.Sequence || s.lastHostTimeNS != math.MaxInt64 {
		t.Fatalf("state after saturated initial rejection = %#v sequence=%d host=%d, want unchanged %#v", got, s.mergedSequence, s.lastHostTimeNS, before)
	}
	if s.acceptedFrames != 0 || s.rejectedFrames != 1 || s.rejected != (RejectionCounts{TimestampRegression: 1}) {
		t.Fatalf("diagnostics after saturated initial rejection = accepted %d rejected %d counts %+v", s.acceptedFrames, s.rejectedFrames, s.rejected)
	}
}

func TestGroupFreshnessTracksSelectedSourceReceiptsIndependently(t *testing.T) {
	now := int64(0)
	s := newServiceWithClock(func() int64 {
		now += 10
		return now
	})
	mustSetGeneration(t, s, 1)
	mustSubmit(t, s, "vendor.eye", 1, eyeFrame(1, 0.25))
	afterEye := latestMerged(t, s)
	if afterEye.EyeUpdatedAtNS != 20 || afterEye.ExpressionUpdatedAtNS != 0 || afterEye.LipUpdatedAtNS != 0 {
		t.Fatalf("freshness after Eye = (%d, %d, %d), want (20, 0, 0)", afterEye.EyeUpdatedAtNS, afterEye.ExpressionUpdatedAtNS, afterEye.LipUpdatedAtNS)
	}

	mustSubmit(t, s, "vendor.expression", 1, expressionFrame(1, trackingmodel.ExpressionJawOpen, 0.5))
	afterExpression := latestMerged(t, s)
	if afterExpression.EyeUpdatedAtNS != 20 || afterExpression.ExpressionUpdatedAtNS != 40 || afterExpression.LipUpdatedAtNS != 0 {
		t.Fatalf("freshness after Expression = (%d, %d, %d), want (20, 40, 0)", afterExpression.EyeUpdatedAtNS, afterExpression.ExpressionUpdatedAtNS, afterExpression.LipUpdatedAtNS)
	}

	mustSubmit(t, s, "vendor.lip", 1, lipFrame(1))
	afterLip := latestMerged(t, s)
	if afterLip.EyeUpdatedAtNS != 20 || afterLip.ExpressionUpdatedAtNS != 40 || afterLip.LipUpdatedAtNS != 60 {
		t.Fatalf("freshness after Lip = (%d, %d, %d), want (20, 40, 60)", afterLip.EyeUpdatedAtNS, afterLip.ExpressionUpdatedAtNS, afterLip.LipUpdatedAtNS)
	}
	if afterLip.LipSourceID != "vendor.lip" || !afterLip.Capabilities.Has(trackingmodel.CapabilityLip) {
		t.Fatalf("Lip merged metadata = %#v, want selected and available", afterLip)
	}

	mustSubmit(t, s, "vendor.eye", 1, eyeFrame(2, 0.25))
	afterSameEye := latestMerged(t, s)
	if afterSameEye.EyeUpdatedAtNS != 80 || afterSameEye.ExpressionUpdatedAtNS != 40 || afterSameEye.LipUpdatedAtNS != 60 {
		t.Fatalf("freshness after same Eye = (%d, %d, %d), want (80, 40, 60)", afterSameEye.EyeUpdatedAtNS, afterSameEye.ExpressionUpdatedAtNS, afterSameEye.LipUpdatedAtNS)
	}
	if afterSameEye.Sequence != afterLip.Sequence+1 || afterSameEye.Eye != afterLip.Eye {
		t.Fatalf("same Eye update = %#v, want unchanged payload and one revision after %#v", afterSameEye, afterLip)
	}

	mustSubmit(t, s, "vendor.expression", 1, expressionFrame(2, trackingmodel.ExpressionJawOpen, 0.5))
	afterSameExpression := latestMerged(t, s)
	if afterSameExpression.EyeUpdatedAtNS != 80 || afterSameExpression.ExpressionUpdatedAtNS != 100 || afterSameExpression.LipUpdatedAtNS != 60 {
		t.Fatalf("freshness after same Expression = (%d, %d, %d), want (80, 100, 60)", afterSameExpression.EyeUpdatedAtNS, afterSameExpression.ExpressionUpdatedAtNS, afterSameExpression.LipUpdatedAtNS)
	}
	if afterSameExpression.Sequence != afterSameEye.Sequence+1 ||
		afterSameExpression.Eye != afterSameEye.Eye || afterSameExpression.Expressions != afterSameEye.Expressions ||
		afterSameExpression.EyeSourceID != afterSameEye.EyeSourceID || afterSameExpression.ExpressionSourceID != afterSameEye.ExpressionSourceID || afterSameExpression.LipSourceID != afterSameEye.LipSourceID ||
		afterSameExpression.Capabilities != afterSameEye.Capabilities {
		t.Fatalf("same Expression update = %#v, want only Expression freshness and ordering changed from %#v", afterSameExpression, afterSameEye)
	}

	mustSubmit(t, s, "vendor.lip", 1, lipFrame(2))
	afterSameLip := latestMerged(t, s)
	if afterSameLip.EyeUpdatedAtNS != 80 || afterSameLip.ExpressionUpdatedAtNS != 100 || afterSameLip.LipUpdatedAtNS != 120 {
		t.Fatalf("freshness after same Lip = (%d, %d, %d), want (80, 100, 120)", afterSameLip.EyeUpdatedAtNS, afterSameLip.ExpressionUpdatedAtNS, afterSameLip.LipUpdatedAtNS)
	}
	if afterSameLip.Sequence != afterSameExpression.Sequence+1 {
		t.Fatalf("same Lip Sequence = %d, want %d", afterSameLip.Sequence, afterSameExpression.Sequence+1)
	}

	mustSetGeneration(t, s, 2)
	afterGeneration := latestMerged(t, s)
	if afterGeneration.EyeSourceID != "" || afterGeneration.ExpressionSourceID != "" || afterGeneration.LipSourceID != "" ||
		afterGeneration.EyeUpdatedAtNS != 0 || afterGeneration.ExpressionUpdatedAtNS != 0 || afterGeneration.LipUpdatedAtNS != 0 {
		t.Fatalf("generation reset retained selections or freshness: %#v", afterGeneration)
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
