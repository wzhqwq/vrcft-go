package trackingmodel

import (
	"errors"
	"fmt"
	"math"
)

type Vec2 struct {
	X float32
	Y float32
}

type Vec3 struct {
	X float32
	Y float32
	Z float32
}

type EyeValid uint16

const (
	EyeValidLeftGaze EyeValid = 1 << iota
	EyeValidRightGaze
	EyeValidLeftOpenness
	EyeValidRightOpenness
	EyeValidLeftPupil
	EyeValidRightPupil
)

const knownEyeValid = EyeValidLeftGaze |
	EyeValidRightGaze |
	EyeValidLeftOpenness |
	EyeValidRightOpenness |
	EyeValidLeftPupil |
	EyeValidRightPupil

type TrackingFrame struct {
	Sequence      uint64
	TimestampNS   int64
	Capabilities  Capability
	SourceClockNS int64

	Eye EyeSample

	Expressions ExpressionSet
}

type EyeSample struct {
	Valid EyeValid

	LeftGaze  Vec2
	RightGaze Vec2

	LeftOpenness  float32
	RightOpenness float32

	LeftPupilDiameterMM  float32
	RightPupilDiameterMM float32

	LeftPupilDilation  float32
	RightPupilDilation float32
}

// Validate rejects tracking metadata that cannot be represented by the v1
// capability and validity contracts.
func (f TrackingFrame) Validate() error {
	const knownCapabilities = CapabilityEye | CapabilityExpression | CapabilityLip
	if f.Capabilities&^knownCapabilities != 0 {
		return errors.New("TrackingFrame.Capabilities contains unknown capability bits")
	}
	if f.Eye.Valid&^knownEyeValid != 0 {
		return errors.New("TrackingFrame.Eye.Valid contains unknown eye bits")
	}
	if f.Expressions.Valid != f.Expressions.Valid.Normalize() {
		return errors.New("TrackingFrame.Expressions.Valid contains expression tail bits")
	}
	if !f.Capabilities.Has(CapabilityEye) && f.Eye.Valid != 0 {
		return errors.New("TrackingFrame.Eye.Valid requires CapabilityEye")
	}
	if !f.Capabilities.Has(CapabilityExpression) && !f.Expressions.Valid.IsZero() {
		return errors.New("TrackingFrame.Expressions.Valid requires CapabilityExpression")
	}
	if err := validateFiniteEye(f.Eye); err != nil {
		return err
	}
	for id := ExpressionID(0); id < ExpressionCount; id++ {
		if f.Expressions.Valid.Has(id) && !finite32(f.Expressions.Values[id]) {
			return fmt.Errorf("TrackingFrame.Expressions.Values[%d] must be finite", id)
		}
	}
	return nil
}

func validateFiniteEye(eye EyeSample) error {
	checks := []struct {
		valid EyeValid
		name  string
		value float32
	}{
		{EyeValidLeftGaze, "LeftGaze.X", eye.LeftGaze.X},
		{EyeValidLeftGaze, "LeftGaze.Y", eye.LeftGaze.Y},
		{EyeValidRightGaze, "RightGaze.X", eye.RightGaze.X},
		{EyeValidRightGaze, "RightGaze.Y", eye.RightGaze.Y},
		{EyeValidLeftOpenness, "LeftOpenness", eye.LeftOpenness},
		{EyeValidRightOpenness, "RightOpenness", eye.RightOpenness},
		{EyeValidLeftPupil, "LeftPupilDiameterMM", eye.LeftPupilDiameterMM},
		{EyeValidLeftPupil, "LeftPupilDilation", eye.LeftPupilDilation},
		{EyeValidRightPupil, "RightPupilDiameterMM", eye.RightPupilDiameterMM},
		{EyeValidRightPupil, "RightPupilDilation", eye.RightPupilDilation},
	}
	for _, check := range checks {
		if eye.Valid&check.valid != 0 && !finite32(check.value) {
			return fmt.Errorf("TrackingFrame.Eye.%s must be finite", check.name)
		}
	}
	return nil
}

func finite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// Canonicalize validates f and clears values that are not marked valid.
func (f TrackingFrame) Canonicalize() (TrackingFrame, error) {
	if err := f.Validate(); err != nil {
		return TrackingFrame{}, err
	}
	if !f.Capabilities.Has(CapabilityEye) {
		f.Eye = EyeSample{}
	} else {
		zeroInvalidEyeValues(&f.Eye)
	}
	if !f.Capabilities.Has(CapabilityExpression) {
		f.Expressions = ExpressionSet{}
	} else {
		for id := ExpressionID(0); id < ExpressionCount; id++ {
			if !f.Expressions.Valid.Has(id) {
				f.Expressions.Values[id] = 0
			}
		}
	}
	return f, nil
}

func zeroInvalidEyeValues(eye *EyeSample) {
	if eye.Valid&EyeValidLeftGaze == 0 {
		eye.LeftGaze = Vec2{}
	}
	if eye.Valid&EyeValidRightGaze == 0 {
		eye.RightGaze = Vec2{}
	}
	if eye.Valid&EyeValidLeftOpenness == 0 {
		eye.LeftOpenness = 0
	}
	if eye.Valid&EyeValidRightOpenness == 0 {
		eye.RightOpenness = 0
	}
	if eye.Valid&EyeValidLeftPupil == 0 {
		eye.LeftPupilDiameterMM = 0
		eye.LeftPupilDilation = 0
	}
	if eye.Valid&EyeValidRightPupil == 0 {
		eye.RightPupilDiameterMM = 0
		eye.RightPupilDilation = 0
	}
}
