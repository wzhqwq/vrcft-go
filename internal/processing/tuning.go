package processing

import (
	"fmt"
	"math"
)

type ChannelTuning struct {
	Deadzone     float32
	Gain         float32
	Exponent     float32
	ClampEnabled bool
	ClampMin     float32
	ClampMax     float32
}

func applyTuning(tuning ChannelTuning, value float32) (float32, error) {
	if err := validateTuning(tuning); err != nil || !finite(value) {
		return 0, fmt.Errorf("apply tuning: %w", ErrInvalidTuning)
	}

	inputMagnitude := float32(math.Abs(float64(value)))
	if !finite(inputMagnitude) {
		return 0, fmt.Errorf("apply tuning magnitude: %w", ErrInvalidTuning)
	}
	if inputMagnitude <= tuning.Deadzone {
		return 0, nil
	}

	scaled := (inputMagnitude - tuning.Deadzone) / (1 - tuning.Deadzone)
	if !finite(scaled) {
		return 0, fmt.Errorf("apply tuning deadzone: %w", ErrInvalidTuning)
	}
	magnitude := scaled * tuning.Gain
	if !finite(magnitude) {
		return 0, fmt.Errorf("apply tuning gain: %w", ErrInvalidTuning)
	}
	magnitude = float32(math.Pow(float64(magnitude), float64(tuning.Exponent)))
	if !finite(magnitude) {
		return 0, fmt.Errorf("apply tuning exponent: %w", ErrInvalidTuning)
	}

	value = float32(math.Copysign(float64(magnitude), float64(value)))
	if !finite(value) {
		return 0, fmt.Errorf("apply tuning sign: %w", ErrInvalidTuning)
	}
	if tuning.ClampEnabled {
		if value < tuning.ClampMin {
			value = tuning.ClampMin
		} else if value > tuning.ClampMax {
			value = tuning.ClampMax
		}
		if !finite(value) {
			return 0, fmt.Errorf("apply tuning clamp: %w", ErrInvalidTuning)
		}
	}
	return value, nil
}
