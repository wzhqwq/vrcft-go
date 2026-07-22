package pluginapi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const allEyeValid = trackingmodel.EyeValidLeftGaze |
	trackingmodel.EyeValidRightGaze |
	trackingmodel.EyeValidLeftOpenness |
	trackingmodel.EyeValidRightOpenness |
	trackingmodel.EyeValidLeftPupil |
	trackingmodel.EyeValidRightPupil

func TestSubscriptionNormalizeClearsDisabledDetailsAndExpressionTail(t *testing.T) {
	invalidTail := trackingmodel.ExpressionMask{}
	invalidTail.Words[len(invalidTail.Words)-1] = uint64(1) << (trackingmodel.ExpressionCount % 64)

	subscription := Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityExpression,
		Eye:          trackingmodel.EyeValidLeftGaze,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
	subscription.Expressions.Words[len(subscription.Expressions.Words)-1] |= invalidTail.Words[len(invalidTail.Words)-1]

	normalized := subscription.Normalize()
	if normalized.Eye != 0 {
		t.Fatalf("Normalize().Eye = %#x, want 0", normalized.Eye)
	}
	if !normalized.Expressions.Has(trackingmodel.ExpressionJawOpen) {
		t.Fatal("Normalize() cleared a valid expression bit")
	}
	if normalized.Expressions.Words[len(normalized.Expressions.Words)-1]&invalidTail.Words[len(invalidTail.Words)-1] != 0 {
		t.Fatal("Normalize() retained an expression tail bit")
	}
	if !reflect.DeepEqual(subscription.Eye, trackingmodel.EyeValidLeftGaze) || subscription.Expressions.Words[len(subscription.Expressions.Words)-1]&invalidTail.Words[len(invalidTail.Words)-1] == 0 {
		t.Fatal("Normalize() mutated its receiver")
	}

	disabledExpressions := Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye, Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)}
	if got := disabledExpressions.Normalize().Expressions; !got.IsZero() {
		t.Fatalf("Normalize().Expressions = %#v, want zero when Expression is disabled", got)
	}
}

func TestSubscriptionValidate(t *testing.T) {
	valid := Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye}
	invalidTail := trackingmodel.ExpressionMask{}
	invalidTail.Words[len(invalidTail.Words)-1] = uint64(1) << (trackingmodel.ExpressionCount % 64)

	tests := []struct {
		name   string
		sub    Subscription
		active bool
		field  string
	}{
		{"inactive empty initial subscription", Subscription{}, false, ""},
		{"inactive retained future subscription", valid, false, ""},
		{"active positive subscription", valid, true, ""},
		{"active zero generation", Subscription{}, true, "Generation"},
		{"zero generation has capabilities", Subscription{Capabilities: trackingmodel.CapabilityEye}, false, "Generation"},
		{"zero generation has eye detail", Subscription{Eye: trackingmodel.EyeValidLeftGaze}, false, "Generation"},
		{"zero generation has expression detail", Subscription{Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)}, false, "Generation"},
		{"positive generation has no capability", Subscription{Generation: 1}, false, "Capabilities"},
		{"active subscription has no capability", Subscription{Generation: 1}, true, "Capabilities"},
		{"unknown capability", Subscription{Generation: 1, Capabilities: trackingmodel.Capability(1 << 10)}, false, "Capabilities"},
		{"mixed unknown capability", Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye | trackingmodel.Capability(1<<10)}, false, "Capabilities"},
		{"unknown eye bit", Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye, Eye: trackingmodel.EyeValid(1 << 10)}, false, "Eye"},
		{"expression tail bit", Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression, Expressions: invalidTail}, false, "Expressions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sub.Validate(tt.active)
			if tt.field == "" {
				if err != nil {
					t.Fatalf("Validate(%t) error = %v, want nil", tt.active, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate(%t) error = %v, want %q error", tt.active, err, tt.field)
			}
		})
	}
}

func TestSubscriptionMembership(t *testing.T) {
	wholeGroups := Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression}
	if !wholeGroups.IncludesEye(trackingmodel.EyeValidRightPupil) {
		t.Fatal("zero Eye mask did not include a valid eye field")
	}
	if !wholeGroups.IncludesExpression(trackingmodel.ExpressionJawOpen) {
		t.Fatal("zero Expression mask did not include a valid expression")
	}

	exact := Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye:          trackingmodel.EyeValidLeftGaze,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
	if !exact.IncludesEye(trackingmodel.EyeValidLeftGaze) || exact.IncludesEye(trackingmodel.EyeValidRightGaze) {
		t.Fatal("exact Eye mask membership is incorrect")
	}
	if !exact.IncludesExpression(trackingmodel.ExpressionJawOpen) || exact.IncludesExpression(trackingmodel.ExpressionMouthClosed) {
		t.Fatal("exact Expression mask membership is incorrect")
	}
	if exact.IncludesEye(0) || exact.IncludesEye(trackingmodel.EyeValidLeftGaze|trackingmodel.EyeValidRightGaze) || exact.IncludesEye(trackingmodel.EyeValid(1<<10)) {
		t.Fatal("IncludesEye accepted an invalid or composite query")
	}
	if exact.IncludesExpression(trackingmodel.ExpressionCount) {
		t.Fatal("IncludesExpression accepted ExpressionCount")
	}
	if (Subscription{}).IncludesEye(trackingmodel.EyeValidLeftGaze) || (Subscription{}).IncludesExpression(trackingmodel.ExpressionJawOpen) {
		t.Fatal("disabled group reported membership")
	}
}

func TestSubscriptionTrimFrameClearsUnsubscribedGroups(t *testing.T) {
	frame := populatedFrame()

	eyeOnly := Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye}.TrimFrame(frame)
	if !eyeOnly.Capabilities.Has(trackingmodel.CapabilityEye) || eyeOnly.Capabilities.Has(trackingmodel.CapabilityExpression) {
		t.Fatalf("eye-only frame capabilities = %#x", eyeOnly.Capabilities)
	}
	if eyeOnly.Expressions != (trackingmodel.ExpressionSet{}) {
		t.Fatalf("eye-only frame retained expressions: %#v", eyeOnly.Expressions)
	}

	expressionOnly := Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression}.TrimFrame(frame)
	if expressionOnly.Capabilities.Has(trackingmodel.CapabilityEye) || !expressionOnly.Capabilities.Has(trackingmodel.CapabilityExpression) {
		t.Fatalf("expression-only frame capabilities = %#x", expressionOnly.Capabilities)
	}
	if expressionOnly.Eye != (trackingmodel.EyeSample{}) {
		t.Fatalf("expression-only frame retained eye data: %#v", expressionOnly.Eye)
	}
}

func TestSubscriptionTrimFrameExactEyeMaskZeroesRejectedFields(t *testing.T) {
	frame := populatedFrame()
	subscription := Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye, Eye: trackingmodel.EyeValidLeftGaze}

	trimmed := subscription.TrimFrame(frame)
	if trimmed.Eye.Valid != trackingmodel.EyeValidLeftGaze || trimmed.Eye.LeftGaze != frame.Eye.LeftGaze {
		t.Fatalf("trimmed eye = %#v, want only left gaze", trimmed.Eye)
	}
	if trimmed.Eye.RightGaze != (trackingmodel.Vec2{}) || trimmed.Eye.LeftOpenness != 0 || trimmed.Eye.RightOpenness != 0 ||
		trimmed.Eye.LeftPupilDiameterMM != 0 || trimmed.Eye.LeftPupilDilation != 0 || trimmed.Eye.RightPupilDiameterMM != 0 || trimmed.Eye.RightPupilDilation != 0 {
		t.Fatalf("trimmed EyeSample retained rejected values: %#v", trimmed.Eye)
	}
}

func TestSubscriptionTrimFrameExactExpressionMaskZeroesRejectedValues(t *testing.T) {
	frame := populatedFrame()
	subscription := Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityExpression,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}

	trimmed := subscription.TrimFrame(frame)
	if !trimmed.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) || trimmed.Expressions.Values[trackingmodel.ExpressionJawOpen] != frame.Expressions.Values[trackingmodel.ExpressionJawOpen] {
		t.Fatalf("trimmed expressions did not retain selected value: %#v", trimmed.Expressions)
	}
	if trimmed.Expressions.Valid.Has(trackingmodel.ExpressionMouthClosed) || trimmed.Expressions.Values[trackingmodel.ExpressionMouthClosed] != 0 {
		t.Fatalf("trimmed expressions retained rejected value: %#v", trimmed.Expressions)
	}
}

func TestSubscriptionTrimFrameZeroDetailMaskPreservesFullGroup(t *testing.T) {
	frame := populatedFrame()
	subscription := Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression}

	trimmed := subscription.TrimFrame(frame)
	if trimmed.Eye != frame.Eye || trimmed.Expressions != frame.Expressions {
		t.Fatal("zero detail masks did not preserve complete subscribed groups")
	}
}

func TestSubscriptionTrimFrameDoesNotMutateInputsAndPreservesMetadata(t *testing.T) {
	frame := populatedFrame()
	originalFrame := frame
	subscription := Subscription{
		Generation:   42,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye:          trackingmodel.EyeValidLeftGaze,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
	originalSubscription := subscription

	trimmed := subscription.TrimFrame(frame)
	if !reflect.DeepEqual(frame, originalFrame) {
		t.Fatalf("TrimFrame() mutated input frame: got %#v, want %#v", frame, originalFrame)
	}
	if !reflect.DeepEqual(subscription, originalSubscription) {
		t.Fatalf("TrimFrame() mutated subscription: got %#v, want %#v", subscription, originalSubscription)
	}
	if trimmed.Sequence != frame.Sequence || trimmed.TimestampNS != frame.TimestampNS || trimmed.SourceClockNS != frame.SourceClockNS {
		t.Fatalf("TrimFrame() metadata = %#v, want sequence and timestamps preserved", trimmed)
	}
}

func populatedFrame() trackingmodel.TrackingFrame {
	frame := trackingmodel.TrackingFrame{
		Sequence:      7,
		TimestampNS:   11,
		SourceClockNS: 13,
		Capabilities:  trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.Capability(1<<10),
		Eye: trackingmodel.EyeSample{
			Valid:                allEyeValid,
			LeftGaze:             trackingmodel.Vec2{X: 1, Y: 2},
			RightGaze:            trackingmodel.Vec2{X: 3, Y: 4},
			LeftOpenness:         5,
			RightOpenness:        6,
			LeftPupilDiameterMM:  7,
			RightPupilDiameterMM: 8,
			LeftPupilDilation:    9,
			RightPupilDilation:   10,
		},
	}
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.5)
	frame.Expressions.Set(trackingmodel.ExpressionMouthClosed, 0.25)
	return frame
}
