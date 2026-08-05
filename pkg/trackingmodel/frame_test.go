package trackingmodel

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestCapabilityLipUsesNextStableBit(t *testing.T) {
	if CapabilityEye != 1 || CapabilityExpression != 2 || CapabilityLip != 4 {
		t.Fatalf("capability bits = (Eye=%d, Expression=%d, Lip=%d), want (1, 2, 4)", CapabilityEye, CapabilityExpression, CapabilityLip)
	}
}

func TestTrackingFrameLipOnlyIsCanonical(t *testing.T) {
	frame := TrackingFrame{
		Sequence:      7,
		TimestampNS:   11,
		Capabilities:  CapabilityLip,
		SourceClockNS: 13,
		Eye: EyeSample{
			LeftGaze:     Vec2{X: 1, Y: 2},
			LeftOpenness: 0.5,
		},
	}
	frame.Expressions.Values[ExpressionJawOpen] = 0.75

	if err := frame.Validate(); err != nil {
		t.Fatalf("Validate(Lip-only) error = %v", err)
	}
	got, err := frame.Canonicalize()
	if err != nil {
		t.Fatalf("Canonicalize(Lip-only) error = %v", err)
	}
	if got.Capabilities != CapabilityLip || got.Sequence != 7 || got.TimestampNS != 11 || got.SourceClockNS != 13 {
		t.Fatalf("Canonicalize(Lip-only) metadata = %+v, want Lip capability and preserved frame metadata", got)
	}
	if got.Eye != (EyeSample{}) || got.Expressions != (ExpressionSet{}) {
		t.Fatalf("Canonicalize(Lip-only) retained numeric payload: Eye=%+v Expressions=%+v", got.Eye, got.Expressions)
	}
}

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

func TestTrackingFrameValidateRejectsNonFiniteValidValues(t *testing.T) {
	tests := []struct {
		name  string
		frame TrackingFrame
	}{
		{"eye NaN", TrackingFrame{Capabilities: CapabilityEye, Eye: EyeSample{Valid: EyeValidLeftGaze, LeftGaze: Vec2{X: float32(math.NaN())}}}},
		{"eye positive infinity", TrackingFrame{Capabilities: CapabilityEye, Eye: EyeSample{Valid: EyeValidRightPupil, RightPupilDiameterMM: float32(math.Inf(1))}}},
		{"eye negative infinity", TrackingFrame{Capabilities: CapabilityEye, Eye: EyeSample{Valid: EyeValidLeftOpenness, LeftOpenness: float32(math.Inf(-1))}}},
		{"expression NaN", func() TrackingFrame {
			f := TrackingFrame{Capabilities: CapabilityExpression}
			f.Expressions.Set(ExpressionJawOpen, float32(math.NaN()))
			return f
		}()},
		{"expression positive infinity", func() TrackingFrame {
			f := TrackingFrame{Capabilities: CapabilityExpression}
			f.Expressions.Set(ExpressionJawOpen, float32(math.Inf(1)))
			return f
		}()},
		{"expression negative infinity", func() TrackingFrame {
			f := TrackingFrame{Capabilities: CapabilityExpression}
			f.Expressions.Set(ExpressionJawOpen, float32(math.Inf(-1)))
			return f
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.frame.Validate(); err == nil || !strings.Contains(err.Error(), "finite") {
				t.Fatalf("Validate() error = %v, want finite-value error", err)
			}
		})
	}
}

func TestTrackingFrameCanonicalizeClearsNonFiniteInvalidValues(t *testing.T) {
	frame := TrackingFrame{Capabilities: CapabilityEye | CapabilityExpression}
	frame.Eye.LeftGaze.X = float32(math.NaN())
	frame.Expressions.Values[ExpressionJawOpen] = float32(math.Inf(1))
	got, err := frame.Canonicalize()
	if err != nil {
		t.Fatal(err)
	}
	if got.Eye.LeftGaze != (Vec2{}) || got.Expressions.Values[ExpressionJawOpen] != 0 {
		t.Fatalf("Canonicalize() retained invalid non-finite values: %+v", got)
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
