package evaluator

import (
	"math"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestOperationsEvaluateHandCalculatedResults(t *testing.T) {
	tests := []struct {
		name  string
		id    parameters.ParameterID
		frame processing.CanonicalFrame
		want  float32
	}{
		{
			name: "direct JawOpen",
			id:   parameters.ParameterJawOpen,
			frame: frameWithExpressions(map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionJawOpen: 0.625,
			}),
			want: 0.625,
		},
		{
			name: "primitive average PupilDilation",
			id:   parameters.ParameterPupilDilation,
			frame: processing.CanonicalFrame{Eye: trackingmodel.EyeSample{
				Valid:              trackingmodel.EyeValidLeftPupil | trackingmodel.EyeValidRightPupil,
				LeftPupilDilation:  0.25,
				RightPupilDilation: 0.75,
			}},
			want: 0.5,
		},
		{
			name: "dependency average EyeX",
			id:   parameters.ParameterEyeX,
			frame: processing.CanonicalFrame{Eye: trackingmodel.EyeSample{
				Valid:     trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze,
				LeftGaze:  trackingmodel.Vec2{X: -0.5},
				RightGaze: trackingmodel.Vec2{X: 0.75},
			}},
			want: 0.125,
		},
		{
			name: "max BrowDownRight",
			id:   parameters.ParameterBrowDownRight,
			frame: frameWithExpressions(map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionBrowPinchRight:   0.25,
				trackingmodel.ExpressionBrowLowererRight: 0.75,
			}),
			want: 0.75,
		},
		{
			name: "two operand SignedPair SmileSadRight",
			id:   parameters.ParameterSmileSadRight,
			frame: frameWithExpressions(map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionMouthCornerPullRight:  0.7,
				trackingmodel.ExpressionMouthCornerSlantRight: 0.6,
				trackingmodel.ExpressionMouthFrownRight:       0.2,
				trackingmodel.ExpressionMouthStretchRight:     0.1,
			}),
			want: 0.5,
		},
		{
			name: "three operand SignedPair SmileFrownRight",
			id:   parameters.ParameterSmileFrownRight,
			frame: frameWithExpressions(map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionMouthCornerPullRight:  0.7,
				trackingmodel.ExpressionMouthCornerSlantRight: 0.6,
				trackingmodel.ExpressionMouthFrownRight:       0.2,
			}),
			want: 0.5,
		},
		{
			name: "SumClamp MouthOpen",
			id:   parameters.ParameterMouthOpen,
			frame: frameWithExpressions(map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionMouthUpperUpRight:   0.8,
				trackingmodel.ExpressionMouthUpperUpLeft:    0.6,
				trackingmodel.ExpressionMouthLowerDownRight: 0.5,
				trackingmodel.ExpressionMouthLowerDownLeft:  0.3,
			}),
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Compile([]parameters.ParameterID{tt.id})
			if err != nil {
				t.Fatal(err)
			}
			got, ok := plan.Evaluate(tt.frame).Float(tt.id)
			if !ok || got != tt.want {
				t.Fatalf("Float(%d) = %v,%t, want %v,true", tt.id, got, ok, tt.want)
			}
		})
	}
}

func TestSignedPairPlansEvaluateOrderSensitiveHandCalculatedResults(t *testing.T) {
	tests := []struct {
		name   string
		id     parameters.ParameterID
		values map[trackingmodel.ExpressionID]float32
		want   float32
	}{
		{
			name: "brow expression right",
			id:   parameters.ParameterBrowExpressionRight,
			values: map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionBrowInnerUpRight: 0.25,
				trackingmodel.ExpressionBrowOuterUpRight: 0.75,
				trackingmodel.ExpressionBrowPinchRight:   0.5,
				trackingmodel.ExpressionBrowLowererRight: 0.125,
			},
			want: 0.25,
		},
		{
			name: "brow expression left",
			id:   parameters.ParameterBrowExpressionLeft,
			values: map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionBrowInnerUpLeft: 0.875,
				trackingmodel.ExpressionBrowOuterUpLeft: 0.25,
				trackingmodel.ExpressionBrowPinchLeft:   0.125,
				trackingmodel.ExpressionBrowLowererLeft: 0.375,
			},
			want: 0.5,
		},
		{
			name: "smile frown right",
			id:   parameters.ParameterSmileFrownRight,
			values: map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionMouthCornerPullRight:  0.25,
				trackingmodel.ExpressionMouthCornerSlantRight: 0.875,
				trackingmodel.ExpressionMouthFrownRight:       0.5,
			},
			want: 0.375,
		},
		{
			name: "smile frown left",
			id:   parameters.ParameterSmileFrownLeft,
			values: map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionMouthCornerPullLeft:  0.625,
				trackingmodel.ExpressionMouthCornerSlantLeft: 0.125,
				trackingmodel.ExpressionMouthFrownLeft:       0.25,
			},
			want: 0.375,
		},
		{
			name: "smile sad right",
			id:   parameters.ParameterSmileSadRight,
			values: map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionMouthCornerPullRight:  0.875,
				trackingmodel.ExpressionMouthCornerSlantRight: 0.25,
				trackingmodel.ExpressionMouthFrownRight:       0.5,
				trackingmodel.ExpressionMouthStretchRight:     0.125,
			},
			want: 0.375,
		},
		{
			name: "smile sad left",
			id:   parameters.ParameterSmileSadLeft,
			values: map[trackingmodel.ExpressionID]float32{
				trackingmodel.ExpressionMouthCornerPullLeft:  0.625,
				trackingmodel.ExpressionMouthCornerSlantLeft: 0.25,
				trackingmodel.ExpressionMouthFrownLeft:       0.125,
				trackingmodel.ExpressionMouthStretchLeft:     0.375,
			},
			want: 0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Compile([]parameters.ParameterID{tt.id})
			if err != nil {
				t.Fatal(err)
			}
			got, ok := plan.Evaluate(frameWithExpressions(tt.values)).Float(tt.id)
			if !ok || got != tt.want {
				t.Fatalf("Float(%d) = %v,%t, want %v,true", tt.id, got, ok, tt.want)
			}
		})
	}
}

func TestValidityRequiresEveryLeaf(t *testing.T) {
	frame := frameWithExpressions(map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionMouthUpperUpRight:   0.8,
		trackingmodel.ExpressionMouthUpperUpLeft:    0.6,
		trackingmodel.ExpressionMouthLowerDownRight: 0.5,
	})
	plan, err := Compile([]parameters.ParameterID{parameters.ParameterMouthOpen})
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := plan.Evaluate(frame).Float(parameters.ParameterMouthOpen); ok || got != 0 {
		t.Fatalf("MouthOpen with missing lower-left leaf = %v,true, want 0,false", got)
	}
}

func TestValidityContainsNonFiniteValues(t *testing.T) {
	frame := frameWithExpressions(map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: float32(math.NaN()),
	})
	plan, err := Compile([]parameters.ParameterID{parameters.ParameterJawOpen})
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := plan.Evaluate(frame).Float(parameters.ParameterJawOpen); ok || got != 0 {
		t.Fatalf("JawOpen from NaN = %v,true, want 0,false", got)
	}
}

func TestEvaluateClampsEveryFloatResultToGeneratedRange(t *testing.T) {
	frame := frameWithExpressions(map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: 1.5,
	})
	plan, err := Compile([]parameters.ParameterID{parameters.ParameterJawOpen})
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := plan.Evaluate(frame).Float(parameters.ParameterJawOpen); !ok || got != 1 {
		t.Fatalf("JawOpen = %v,%t, want 1,true", got, ok)
	}
}

func TestEvaluateLipActiveIsIndependentFromExpressionActive(t *testing.T) {
	plan, err := Compile([]parameters.ParameterID{
		parameters.ParameterExpressionTrackingActive,
		parameters.ParameterLipTrackingActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := plan.Evaluate(processing.CanonicalFrame{ExpressionActive: false, LipActive: true})

	if got, ok := snapshot.Bool(parameters.ParameterExpressionTrackingActive); !ok || got {
		t.Fatalf("ExpressionTrackingActive = %v,%t, want false,true", got, ok)
	}
	if got, ok := snapshot.Bool(parameters.ParameterLipTrackingActive); !ok || !got {
		t.Fatalf("LipTrackingActive = %v,%t, want true,true", got, ok)
	}
}

func TestEvaluateTrackingActiveParametersAreOneHotAcrossAllThreeStates(t *testing.T) {
	ids := []parameters.ParameterID{
		parameters.ParameterEyeTrackingActive,
		parameters.ParameterExpressionTrackingActive,
		parameters.ParameterLipTrackingActive,
	}
	plan, err := Compile(ids)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		frame processing.CanonicalFrame
		want  [3]bool
	}{
		{name: "eye", frame: processing.CanonicalFrame{EyeActive: true}, want: [3]bool{true, false, false}},
		{name: "expression", frame: processing.CanonicalFrame{ExpressionActive: true}, want: [3]bool{false, true, false}},
		{name: "lip", frame: processing.CanonicalFrame{LipActive: true}, want: [3]bool{false, false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := plan.Evaluate(tt.frame)
			for index, id := range ids {
				got, ok := snapshot.Bool(id)
				if !ok || got != tt.want[index] {
					t.Fatalf("Bool(%d) = %v,%t, want %v,true", id, got, ok, tt.want[index])
				}
			}
		})
	}
}

func frameWithExpressions(values map[trackingmodel.ExpressionID]float32) processing.CanonicalFrame {
	var frame processing.CanonicalFrame
	for id, value := range values {
		frame.Expressions.Set(id, value)
	}
	return frame
}
