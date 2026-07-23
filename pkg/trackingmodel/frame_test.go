package trackingmodel

import (
	"reflect"
	"strings"
	"testing"
)

func TestTrackingFrameValidateRejectsMalformedValidity(t *testing.T) {
	expressionTail := ExpressionMask{}
	expressionTail.Words[len(expressionTail.Words)-1] = uint64(1) << (ExpressionCount % 64)

	tests := []struct {
		name  string
		frame TrackingFrame
		field string
	}{
		{
			name:  "unknown capability",
			frame: TrackingFrame{Capabilities: Capability(1 << 20)},
			field: "Capabilities",
		},
		{
			name: "unknown eye validity",
			frame: TrackingFrame{
				Capabilities: CapabilityEye,
				Eye:          EyeSample{Valid: EyeValid(1 << 15)},
			},
			field: "Eye.Valid",
		},
		{
			name: "expression tail",
			frame: TrackingFrame{
				Capabilities: CapabilityExpression,
				Expressions:  ExpressionSet{Valid: expressionTail},
			},
			field: "Expressions.Valid",
		},
		{
			name:  "eye validity without capability",
			frame: TrackingFrame{Eye: EyeSample{Valid: EyeValidLeftGaze}},
			field: "Eye.Valid",
		},
		{
			name:  "expression validity without capability",
			frame: TrackingFrame{Expressions: ExpressionSet{Valid: ExpressionMaskOf(ExpressionJawOpen)}},
			field: "Expressions.Valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.frame.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate() error = %v, want %q error", err, tt.field)
			}
			if _, err := tt.frame.Canonicalize(); err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Canonicalize() error = %v, want %q error", err, tt.field)
			}
		})
	}
}

func TestTrackingFrameCanonicalizeZeroesDataWithoutValidity(t *testing.T) {
	frame := TrackingFrame{
		Sequence:     9,
		Capabilities: CapabilityEye | CapabilityExpression,
		Eye: EyeSample{
			Valid:                EyeValidLeftGaze,
			LeftGaze:             Vec2{X: 1, Y: 2},
			RightGaze:            Vec2{X: 3, Y: 4},
			LeftOpenness:         5,
			RightOpenness:        6,
			LeftPupilDiameterMM:  7,
			RightPupilDiameterMM: 8,
			LeftPupilDilation:    9,
			RightPupilDilation:   10,
		},
	}
	frame.Expressions.Valid = ExpressionMaskOf(ExpressionJawOpen)
	frame.Expressions.Values[ExpressionJawOpen] = 0.75
	frame.Expressions.Values[ExpressionMouthClosed] = 0.5
	original := frame

	got, err := frame.Canonicalize()
	if err != nil {
		t.Fatal(err)
	}
	if got.Eye.LeftGaze != frame.Eye.LeftGaze || got.Eye.RightGaze != (Vec2{}) {
		t.Fatalf("canonical eye gazes = %+v, want only valid left gaze", got.Eye)
	}
	if got.Eye.LeftOpenness != 0 || got.Eye.RightOpenness != 0 ||
		got.Eye.LeftPupilDiameterMM != 0 || got.Eye.RightPupilDiameterMM != 0 ||
		got.Eye.LeftPupilDilation != 0 || got.Eye.RightPupilDilation != 0 {
		t.Fatalf("canonical eye retained values without validity: %+v", got.Eye)
	}
	if got.Expressions.Values[ExpressionJawOpen] != 0.75 ||
		got.Expressions.Values[ExpressionMouthClosed] != 0 {
		t.Fatalf("canonical expressions retained invalid values: %+v", got.Expressions)
	}
	if !reflect.DeepEqual(frame, original) {
		t.Fatalf("Canonicalize() mutated receiver: got %+v, want %+v", frame, original)
	}
}

func TestTrackingFrameValidDropoutIsCanonical(t *testing.T) {
	frame := TrackingFrame{Sequence: 10, TimestampNS: 20, SourceClockNS: 30}
	if err := frame.Validate(); err != nil {
		t.Fatalf("Validate(dropout) error = %v", err)
	}
	got, err := frame.Canonicalize()
	if err != nil {
		t.Fatalf("Canonicalize(dropout) error = %v", err)
	}
	if got != frame {
		t.Fatalf("Canonicalize(dropout) = %+v, want %+v", got, frame)
	}
}
