package pluginapi

import (
	"errors"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const knownSubscriptionEye = trackingmodel.EyeValidLeftGaze |
	trackingmodel.EyeValidRightGaze |
	trackingmodel.EyeValidLeftOpenness |
	trackingmodel.EyeValidRightOpenness |
	trackingmodel.EyeValidLeftPupil |
	trackingmodel.EyeValidRightPupil

const knownSubscriptionCapabilities = trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression

type Subscription struct {
	Generation   uint64
	Capabilities trackingmodel.Capability
	Eye          trackingmodel.EyeValid
	Expressions  trackingmodel.ExpressionMask
}

// Normalize removes detail selections for disabled groups and unused expression bits.
func (s Subscription) Normalize() Subscription {
	if !s.Capabilities.Has(trackingmodel.CapabilityEye) {
		s.Eye = 0
	}
	if !s.Capabilities.Has(trackingmodel.CapabilityExpression) {
		s.Expressions = trackingmodel.ExpressionMask{}
	} else {
		s.Expressions = s.Expressions.Normalize()
	}
	return s
}

// Validate reports whether s is a valid subscription for the supplied runtime state.
func (s Subscription) Validate(active bool) error {
	if s.Capabilities&^knownSubscriptionCapabilities != 0 {
		return errors.New("Subscription.Capabilities contains unknown capability bits")
	}
	if s.Eye&^knownSubscriptionEye != 0 {
		return errors.New("Subscription.Eye contains unknown eye bits")
	}
	if s.Expressions != s.Expressions.Normalize() {
		return errors.New("Subscription.Expressions contains expression tail bits")
	}

	if s.Generation == 0 {
		if active || s.Capabilities != 0 || s.Eye != 0 || !s.Expressions.IsZero() {
			return errors.New("Subscription.Generation zero requires an inactive empty subscription")
		}
		return nil
	}

	if s.Capabilities&knownSubscriptionCapabilities == 0 {
		return errors.New("Subscription.Capabilities must include a known capability for positive generations")
	}
	return nil
}

// IncludesEye reports whether an exact known eye field is selected.
func (s Subscription) IncludesEye(bit trackingmodel.EyeValid) bool {
	if bit == 0 || bit&^knownSubscriptionEye != 0 || bit&(bit-1) != 0 {
		return false
	}
	if !s.Capabilities.Has(trackingmodel.CapabilityEye) {
		return false
	}
	return s.Eye == 0 || s.Eye&bit != 0
}

// IncludesExpression reports whether an exact known expression is selected.
func (s Subscription) IncludesExpression(id trackingmodel.ExpressionID) bool {
	if id >= trackingmodel.ExpressionCount || !s.Capabilities.Has(trackingmodel.CapabilityExpression) {
		return false
	}
	return s.Expressions.IsZero() || s.Expressions.Has(id)
}

// TrimFrame returns a frame copy containing only the groups and fields selected by s.
func (s Subscription) TrimFrame(frame trackingmodel.TrackingFrame) trackingmodel.TrackingFrame {
	trimmed := frame
	trimmed.Capabilities &= knownSubscriptionCapabilities & s.Capabilities

	if !trimmed.Capabilities.Has(trackingmodel.CapabilityEye) {
		trimmed.Eye = trackingmodel.EyeSample{}
	} else {
		trimmed.Eye.Valid &= knownSubscriptionEye
		if s.Eye != 0 {
			trimmed.Eye.Valid &= s.Eye
		}
		zeroUnselectedEyeValues(&trimmed.Eye)
	}

	if !trimmed.Capabilities.Has(trackingmodel.CapabilityExpression) {
		trimmed.Expressions = trackingmodel.ExpressionSet{}
	} else {
		trimmed.Expressions.Valid = trimmed.Expressions.Valid.Normalize()
		if !s.Expressions.IsZero() {
			trimmed.Expressions.Valid = trimmed.Expressions.Valid.Intersect(s.Expressions)
		}
		zeroInvalidExpressionValues(&trimmed.Expressions)
	}

	return trimmed
}

func zeroUnselectedEyeValues(eye *trackingmodel.EyeSample) {
	if eye.Valid&trackingmodel.EyeValidLeftGaze == 0 {
		eye.LeftGaze = trackingmodel.Vec2{}
	}
	if eye.Valid&trackingmodel.EyeValidRightGaze == 0 {
		eye.RightGaze = trackingmodel.Vec2{}
	}
	if eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 {
		eye.LeftOpenness = 0
	}
	if eye.Valid&trackingmodel.EyeValidRightOpenness == 0 {
		eye.RightOpenness = 0
	}
	if eye.Valid&trackingmodel.EyeValidLeftPupil == 0 {
		eye.LeftPupilDiameterMM = 0
		eye.LeftPupilDilation = 0
	}
	if eye.Valid&trackingmodel.EyeValidRightPupil == 0 {
		eye.RightPupilDiameterMM = 0
		eye.RightPupilDilation = 0
	}
}

func zeroInvalidExpressionValues(expressions *trackingmodel.ExpressionSet) {
	for id := trackingmodel.ExpressionID(0); id < trackingmodel.ExpressionCount; id++ {
		if !expressions.Valid.Has(id) {
			expressions.Values[id] = 0
		}
	}
}
