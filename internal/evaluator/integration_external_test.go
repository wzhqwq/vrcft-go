package evaluator_test

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

var _ osc.ValueSource = evaluator.Snapshot{}

func TestMergedFrameFlowsThroughPipelineEvaluatorAndOSCValueSource(t *testing.T) {
	merged := integrationMergedFrame()
	pipeline, err := processing.NewPipeline(processing.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	frame, err := pipeline.ProcessAt(merged, 4_000_000_000)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := evaluator.Compile(integrationParameterIDs())
	if err != nil {
		t.Fatal(err)
	}
	var source osc.ValueSource = plan.Evaluate(frame)

	floatTests := []struct {
		name string
		id   parameters.ParameterID
		want float32
	}{
		{name: "direct JawOpen", id: parameters.ParameterJawOpen, want: 0.625},
		{name: "derived EyeX", id: parameters.ParameterEyeX, want: -0.25},
	}
	for _, tt := range floatTests {
		got, ok := source.Float(tt.id)
		if !ok || got != tt.want {
			t.Errorf("%s = %v,%t, want %v,true", tt.name, got, ok, tt.want)
		}
	}

	for _, id := range []parameters.ParameterID{
		parameters.ParameterEyeTrackingActive,
		parameters.ParameterExpressionTrackingActive,
		parameters.ParameterLipTrackingActive,
	} {
		got, ok := source.Bool(id)
		if !ok || !got {
			t.Errorf("Bool(%d) = %t,%t, want true,true", id, got, ok)
		}
	}

	if got, ok := source.Float(parameters.ParameterEyeY); ok || got != 0 {
		t.Fatalf("unrequested EyeY = %v,%t, want 0,false", got, ok)
	}
}

func integrationMergedFrame() tracking.MergedFrame {
	var expressions trackingmodel.ExpressionSet
	expressions.Set(trackingmodel.ExpressionJawOpen, 0.625)
	return tracking.MergedFrame{
		Generation:            7,
		Sequence:              11,
		UpdatedAtNS:           3_999_999_900,
		Capabilities:          trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip,
		EyeSourceID:           "eye-source",
		ExpressionSourceID:    "expression-source",
		LipSourceID:           "lip-source",
		EyeUpdatedAtNS:        3_999_999_900,
		ExpressionUpdatedAtNS: 3_999_999_900,
		LipUpdatedAtNS:        3_999_999_900,
		Eye: trackingmodel.EyeSample{
			Valid:     trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze,
			LeftGaze:  trackingmodel.Vec2{X: -0.75, Y: 0.125},
			RightGaze: trackingmodel.Vec2{X: 0.25, Y: -0.125},
		},
		Expressions: expressions,
	}
}

func integrationParameterIDs() []parameters.ParameterID {
	return []parameters.ParameterID{
		parameters.ParameterJawOpen,
		parameters.ParameterEyeX,
		parameters.ParameterEyeTrackingActive,
		parameters.ParameterExpressionTrackingActive,
		parameters.ParameterLipTrackingActive,
	}
}
