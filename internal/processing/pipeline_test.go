package processing

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestPipelinePublishesCanonicalValueSnapshot(t *testing.T) {
	pipeline := mustPipeline(t, DefaultConfig())
	frame := tracking.MergedFrame{
		Generation:            1,
		Sequence:              1,
		UpdatedAtNS:           100,
		Capabilities:          trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		EyeSourceID:           "eye",
		ExpressionSourceID:    "face",
		EyeUpdatedAtNS:        90,
		ExpressionUpdatedAtNS: 95,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.75,
		},
	}
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.5)

	got, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	jaw, jawValid := got.Expressions.Get(trackingmodel.ExpressionJawOpen)
	if got.Generation != 1 || got.Revision != 1 || got.ProcessedAtNS != 100 ||
		got.EyeSourceID != "eye" || got.ExpressionSourceID != "face" || got.LipSourceID != "" ||
		!got.EyeActive || !got.ExpressionActive || got.LipActive ||
		got.Eye.Valid != trackingmodel.EyeValidLeftOpenness || got.Eye.LeftOpenness != 0.75 ||
		!jawValid || jaw != 0.5 {
		t.Fatalf("canonical = %#v, jaw = %v,%t", got, jaw, jawValid)
	}
}

func TestActiveGroupsExpireIndependently(t *testing.T) {
	config := DefaultConfig()
	config.ActiveStaleAfter = 10
	pipeline := mustPipeline(t, config)
	frame := tracking.MergedFrame{
		Generation:            1,
		Sequence:              1,
		UpdatedAtNS:           109,
		Capabilities:          trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip,
		EyeSourceID:           "eye",
		ExpressionSourceID:    "face",
		LipSourceID:           "lip",
		EyeUpdatedAtNS:        100,
		ExpressionUpdatedAtNS: 105,
		LipUpdatedAtNS:        109,
	}
	if got, err := pipeline.ProcessAt(frame, 109); err != nil || !got.EyeActive || !got.ExpressionActive || !got.LipActive {
		t.Fatalf("initial active flags = (%t,%t,%t),%v; want all true", got.EyeActive, got.ExpressionActive, got.LipActive, err)
	}

	tests := []struct {
		now                  int64
		eye, expression, lip bool
	}{
		{now: 111, eye: false, expression: true, lip: true},
		{now: 116, eye: false, expression: false, lip: true},
		{now: 120, eye: false, expression: false, lip: false},
	}
	for _, test := range tests {
		got, err := pipeline.ProcessAt(frame, test.now)
		if err != nil {
			t.Fatalf("now %d: %v", test.now, err)
		}
		if got.EyeActive != test.eye || got.ExpressionActive != test.expression || got.LipActive != test.lip {
			t.Fatalf("now %d active flags = (%t,%t,%t); want (%t,%t,%t)", test.now, got.EyeActive, got.ExpressionActive, got.LipActive, test.eye, test.expression, test.lip)
		}
	}
}

func TestActiveLipOnlyFrameSetsOnlyLipActive(t *testing.T) {
	config := DefaultConfig()
	config.ActiveStaleAfter = 10
	pipeline := mustPipeline(t, config)
	frame := tracking.MergedFrame{
		Generation:     1,
		Sequence:       1,
		UpdatedAtNS:    100,
		Capabilities:   trackingmodel.CapabilityLip,
		LipSourceID:    "lip",
		LipUpdatedAtNS: 100,
	}
	got, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.EyeActive || got.ExpressionActive || !got.LipActive || got.Eye.Valid != 0 || got.Expressions != (trackingmodel.ExpressionSet{}) {
		t.Fatalf("Lip-only output = %#v; want only Lip active and no numeric inference", got)
	}
}

func TestActiveLipRemovalDoesNotChangeExpressionValue(t *testing.T) {
	config := DefaultConfig()
	config.ActiveStaleAfter = 10
	pipeline := mustPipeline(t, config)
	frame := expressionChannelsFrame(1, 100, "face", map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: 0.6,
	})
	frame.Capabilities |= trackingmodel.CapabilityLip
	frame.LipSourceID = "lip"
	frame.LipUpdatedAtNS = 100
	if _, err := pipeline.ProcessAt(frame, 100); err != nil {
		t.Fatal(err)
	}

	removed := frame
	removed.Sequence = 2
	removed.UpdatedAtNS = 105
	removed.Capabilities &^= trackingmodel.CapabilityLip
	removed.LipSourceID = ""
	removed.LipUpdatedAtNS = 0
	got, err := pipeline.ProcessAt(removed, 105)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ExpressionActive || got.LipActive {
		t.Fatalf("active flags = Expression %t Lip %t; want true,false", got.ExpressionActive, got.LipActive)
	}
	assertExpression(t, got, trackingmodel.ExpressionJawOpen, 0.6, true)
}

func TestSourceChangeReplacementResetsAbsentNumericChannel(t *testing.T) {
	pipeline := mustPipeline(t, dropoutTestConfig())
	first := eyeFrame(1, 1, 100, "eye-a", 0.8)
	if _, err := pipeline.ProcessAt(first, 100); err != nil {
		t.Fatal(err)
	}

	replacement := tracking.MergedFrame{
		Generation:     1,
		Sequence:       2,
		UpdatedAtNS:    105,
		Capabilities:   trackingmodel.CapabilityEye,
		EyeSourceID:    "eye-b",
		EyeUpdatedAtNS: 105,
		Eye: trackingmodel.EyeSample{
			Valid:         trackingmodel.EyeValidRightOpenness,
			RightOpenness: 0.3,
		},
	}
	got, err := pipeline.ProcessAt(replacement, 105)
	if err != nil {
		t.Fatal(err)
	}
	if got.Eye.Valid&trackingmodel.EyeValidLeftOpenness != 0 || got.Eye.Valid&trackingmodel.EyeValidRightOpenness == 0 || got.Eye.RightOpenness != 0.3 {
		t.Fatalf("replacement Eye = %#v; want absent left invalid and right 0.3 valid", got.Eye)
	}
}

func TestSourceChangeToEmptyPreservesHistoryAndStartsDropout(t *testing.T) {
	pipeline := mustPipeline(t, dropoutTestConfig())
	if _, err := pipeline.ProcessAt(eyeFrame(1, 1, 100, "eye-a", 0.8), 100); err != nil {
		t.Fatal(err)
	}

	empty := tracking.MergedFrame{Generation: 1, Sequence: 2, UpdatedAtNS: 110}
	got, err := pipeline.ProcessAt(empty, 120)
	if err != nil {
		t.Fatal(err)
	}
	if got.EyeSourceID != "" || got.EyeActive || got.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 || got.Eye.LeftOpenness != 0.4 {
		t.Fatalf("empty-source output = %#v; want inactive preserved history at decay midpoint 0.4", got)
	}
}

func TestPipelineAppliesCalibrationThenTuningThenFilter(t *testing.T) {
	config := DefaultConfig()
	config.Overrides[ChannelEyeLeftOpenness] = ChannelConfig{
		Calibration: ChannelCalibration{Enabled: true, Min: 0, Neutral: 0, Max: 1, Gain: 2},
		Tuning:      ChannelTuning{Deadzone: 0.5, Gain: 2, Exponent: 2},
		Filter:      FilterConfig{Mode: FilterEMA, EMAAlpha: 0.5},
		Dropout:     config.DefaultChannel.Dropout,
	}
	pipeline := mustPipeline(t, config)

	first := eyeFrame(1, 1, 100, "eye", 0.75)
	got, err := pipeline.ProcessAt(first, 100)
	if err != nil || got.Eye.LeftOpenness != 16 {
		t.Fatalf("first transformed openness = %v,%v; want 16,nil", got.Eye.LeftOpenness, err)
	}
	second := eyeFrame(1, 2, 200, "eye", 0.5)
	got, err = pipeline.ProcessAt(second, 200)
	if err != nil || got.Eye.LeftOpenness != 10 {
		t.Fatalf("second transformed openness = %v,%v; want 10,nil", got.Eye.LeftOpenness, err)
	}
}

func TestProcessAtIdenticalRevisionAtLaterTimeDoesNotDoubleIngestEMA(t *testing.T) {
	pipeline := mustEMAPipeline(t)
	first := eyeFrame(1, 1, 100, "eye", 8)
	if _, err := pipeline.ProcessAt(first, 100); err != nil {
		t.Fatal(err)
	}

	repeated, err := pipeline.ProcessAt(first, 150)
	if err != nil || repeated.ProcessedAtNS != 150 || repeated.Eye.LeftOpenness != 8 {
		t.Fatalf("repeated output = %#v,%v; want processed time 150 and openness 8", repeated, err)
	}
	second := eyeFrame(1, 2, 200, "eye", 4)
	got, err := pipeline.ProcessAt(second, 200)
	if err != nil || got.Eye.LeftOpenness != 7 {
		t.Fatalf("post-repeat EMA = %v,%v; want 7,nil", got.Eye.LeftOpenness, err)
	}
}

func TestGenerationResetClearsEveryFilterHistory(t *testing.T) {
	pipeline := mustEMAPipeline(t)
	first := eyeExpressionFrame(1, 1, 100, "eye-a", "face-a", 8, 8)
	second := eyeExpressionFrame(1, 2, 200, "eye-a", "face-a", 4, 4)
	if _, err := pipeline.ProcessAt(first, 100); err != nil {
		t.Fatal(err)
	}
	if got, err := pipeline.ProcessAt(second, 200); err != nil || got.Eye.LeftOpenness != 7 || expressionValue(got, trackingmodel.ExpressionJawOpen) != 7 {
		t.Fatalf("pre-reset EMA = %#v,%v; want both values 7", got, err)
	}

	reset := eyeExpressionFrame(2, 1, 300, "eye-a", "face-a", 2, 3)
	got, err := pipeline.ProcessAt(reset, 300)
	if err != nil || got.Eye.LeftOpenness != 2 || expressionValue(got, trackingmodel.ExpressionJawOpen) != 3 {
		t.Fatalf("generation reset output = %#v,%v; want initialized values 2 and 3", got, err)
	}
}

func TestSourceResetReinitializesEyeOnlyWhileExpressionHistoryContinues(t *testing.T) {
	pipeline := mustEMAPipeline(t)
	first := eyeExpressionFrame(1, 1, 100, "eye-a", "face-a", 8, 8)
	if _, err := pipeline.ProcessAt(first, 100); err != nil {
		t.Fatal(err)
	}

	replaced := eyeExpressionFrame(1, 2, 200, "eye-b", "face-a", 2, 4)
	got, err := pipeline.ProcessAt(replaced, 200)
	if err != nil || got.Eye.LeftOpenness != 2 || expressionValue(got, trackingmodel.ExpressionJawOpen) != 7 {
		t.Fatalf("source replacement output = %#v,%v; want Eye 2 and Expression 7", got, err)
	}
}

func TestSourceResetToEmptyPreservesEyeNumericHistory(t *testing.T) {
	pipeline := mustEMAPipeline(t)
	first := eyeFrame(1, 1, 100, "eye-a", 8)
	if _, err := pipeline.ProcessAt(first, 100); err != nil {
		t.Fatal(err)
	}

	empty := tracking.MergedFrame{Generation: 1, Sequence: 2, UpdatedAtNS: 200}
	got, err := pipeline.ProcessAt(empty, 200)
	if err != nil || got.EyeSourceID != "" || got.Eye.Valid != trackingmodel.EyeValidLeftOpenness || got.Eye.LeftOpenness != 8 {
		t.Fatalf("empty-source output = %#v,%v; want retained valid openness 8", got, err)
	}
}

func TestSourceResetRebuildsCoarseEyeValidityOnlyFromCompleteScalarGroups(t *testing.T) {
	pipeline := mustPipeline(t, DefaultConfig())
	frame := eyeFrame(1, 1, 100, "eye-a", 0.8)
	frame.Eye.Valid |= trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidLeftPupil
	frame.Eye.LeftGaze = trackingmodel.Vec2{X: 0.25, Y: -0.5}
	frame.Eye.LeftPupilDiameterMM = 4
	frame.Eye.LeftPupilDilation = 0.6
	got, err := pipeline.ProcessAt(frame, 100)
	if err != nil || got.Eye.Valid != trackingmodel.EyeValidLeftGaze|trackingmodel.EyeValidLeftOpenness|trackingmodel.EyeValidLeftPupil {
		t.Fatalf("complete eye groups = %#v,%v", got.Eye, err)
	}

	replacement := eyeFrame(1, 2, 200, "eye-b", 0.4)
	got, err = pipeline.ProcessAt(replacement, 200)
	if err != nil || got.Eye.Valid != trackingmodel.EyeValidLeftOpenness || got.Eye.LeftGaze != (trackingmodel.Vec2{}) || got.Eye.LeftPupilDiameterMM != 0 {
		t.Fatalf("replacement retained incomplete groups = %#v,%v", got.Eye, err)
	}
}

func TestProcessAtSaturatedRevisionUsesFullValueAndFreshnessToDetectNewSnapshot(t *testing.T) {
	pipeline := mustEMAPipeline(t)
	first := eyeFrame(1, math.MaxUint64, 100, "eye", 8)
	if _, err := pipeline.ProcessAt(first, 100); err != nil {
		t.Fatal(err)
	}
	if got, err := pipeline.ProcessAt(first, 110); err != nil || got.Eye.LeftOpenness != 8 {
		t.Fatalf("identical saturated repeat = %#v,%v; want unchanged 8", got, err)
	}

	second := eyeFrame(1, math.MaxUint64, 200, "eye", 4)
	got, err := pipeline.ProcessAt(second, 200)
	if err != nil || got.Eye.LeftOpenness != 7 {
		t.Fatalf("new saturated snapshot = %#v,%v; want EMA 7", got, err)
	}

	regressedFreshness := eyeFrame(1, math.MaxUint64, 190, "eye", 2)
	if _, err := pipeline.ProcessAt(regressedFreshness, 210); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("regressed saturated freshness error = %v; want errors.Is(_, %v)", err, ErrRevisionConflict)
	}
}

func TestPipelineRetainsMergedFrameValueAfterCallerMutation(t *testing.T) {
	pipeline := mustPipeline(t, DefaultConfig())
	frame := eyeFrame(1, 1, 100, "eye", 0.75)
	original := frame
	if _, err := pipeline.ProcessAt(frame, 100); err != nil {
		t.Fatal(err)
	}
	frame.Eye.LeftOpenness = 0.1
	frame.EyeSourceID = "mutated"
	frame.EyeUpdatedAtNS = 101

	got, err := pipeline.ProcessAt(original, 110)
	if err != nil || got.EyeSourceID != "eye" || got.Eye.LeftOpenness != 0.75 {
		t.Fatalf("repeat after caller mutation = %#v,%v; want retained original input", got, err)
	}
}

func mustPipeline(t *testing.T, config Config) *Pipeline {
	t.Helper()
	pipeline, err := NewPipeline(config)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

func mustEMAPipeline(t *testing.T) *Pipeline {
	t.Helper()
	config := DefaultConfig()
	config.DefaultChannel.Filter = FilterConfig{Mode: FilterEMA, EMAAlpha: 0.25}
	return mustPipeline(t, config)
}

func eyeFrame(generation, revision uint64, atNS int64, source string, openness float32) tracking.MergedFrame {
	return tracking.MergedFrame{
		Generation:     generation,
		Sequence:       revision,
		UpdatedAtNS:    atNS,
		Capabilities:   trackingmodel.CapabilityEye,
		EyeSourceID:    source,
		EyeUpdatedAtNS: atNS,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: openness,
		},
	}
}

func eyeExpressionFrame(generation, revision uint64, atNS int64, eyeSource, expressionSource string, openness, jaw float32) tracking.MergedFrame {
	frame := eyeFrame(generation, revision, atNS, eyeSource, openness)
	frame.Capabilities |= trackingmodel.CapabilityExpression
	frame.ExpressionSourceID = expressionSource
	frame.ExpressionUpdatedAtNS = atNS
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, jaw)
	return frame
}

func expressionValue(frame CanonicalFrame, id trackingmodel.ExpressionID) float32 {
	value, _ := frame.Expressions.Get(id)
	return value
}

func assertCanonicalEqual(t *testing.T, got, want CanonicalFrame) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical = %#v, want %#v", got, want)
	}
}
