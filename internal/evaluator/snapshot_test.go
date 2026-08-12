package evaluator

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestSnapshotExposesOnlyRequestedParameters(t *testing.T) {
	plan, err := Compile([]parameters.ParameterID{parameters.ParameterEyeX})
	if err != nil {
		t.Fatal(err)
	}
	frame := processing.CanonicalFrame{Eye: trackingmodel.EyeSample{
		Valid:     trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze,
		LeftGaze:  trackingmodel.Vec2{X: -0.5},
		RightGaze: trackingmodel.Vec2{X: 0.75},
	}}
	snapshot := plan.Evaluate(frame)

	if got, ok := snapshot.Float(parameters.ParameterEyeX); !ok || got != 0.125 {
		t.Fatalf("EyeX = %v,%t, want 0.125,true", got, ok)
	}
	for _, dependency := range []parameters.ParameterID{parameters.ParameterEyeLeftX, parameters.ParameterEyeRightX} {
		if got, ok := snapshot.Float(dependency); ok || got != 0 {
			t.Fatalf("dependency %d = %v,true, want 0,false", dependency, got)
		}
	}
}

func TestSnapshotRejectsWrongTypesAndUnknownIDs(t *testing.T) {
	plan, err := Compile([]parameters.ParameterID{
		parameters.ParameterJawOpen,
		parameters.ParameterEyeTrackingActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	frame := frameWithExpressions(map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: 0.5,
	})
	frame.EyeActive = true
	snapshot := plan.Evaluate(frame)

	if got, ok := snapshot.Bool(parameters.ParameterJawOpen); ok || got {
		t.Fatalf("Bool(JawOpen) = %v,true, want false,false", got)
	}
	if got, ok := snapshot.Float(parameters.ParameterEyeTrackingActive); ok || got != 0 {
		t.Fatalf("Float(EyeTrackingActive) = %v,true, want 0,false", got)
	}
	if got, ok := snapshot.Float(parameters.ParameterCount); ok || got != 0 {
		t.Fatalf("Float(unknown) = %v,true, want 0,false", got)
	}
	if got, ok := snapshot.Bool(parameters.ParameterCount + 1); ok || got {
		t.Fatalf("Bool(unknown) = %v,true, want false,false", got)
	}
}

func TestSnapshotOwnsValuesAcrossLaterEvaluations(t *testing.T) {
	plan, err := Compile([]parameters.ParameterID{parameters.ParameterJawOpen})
	if err != nil {
		t.Fatal(err)
	}
	first := plan.Evaluate(frameWithExpressions(map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: 0.25,
	}))
	second := plan.Evaluate(frameWithExpressions(map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: 0.75,
	}))

	if got, ok := first.Float(parameters.ParameterJawOpen); !ok || got != 0.25 {
		t.Fatalf("first JawOpen after second evaluation = %v,%t, want 0.25,true", got, ok)
	}
	if got, ok := second.Float(parameters.ParameterJawOpen); !ok || got != 0.75 {
		t.Fatalf("second JawOpen = %v,%t, want 0.75,true", got, ok)
	}
}
