package processing

import "fmt"

type ChannelCalibration struct {
	Enabled bool
	Neutral float32
	Min     float32
	Max     float32
	Gain    float32
	Invert  bool
}

func applyCalibration(calibration ChannelCalibration, raw float32) (float32, error) {
	if err := validateCalibration(calibration); err != nil || !finite(raw) {
		return 0, fmt.Errorf("apply calibration: %w", ErrInvalidCalibration)
	}
	if !calibration.Enabled {
		return raw, nil
	}
	negativeSpan := calibration.Neutral - calibration.Min
	if !finite(negativeSpan) {
		return 0, fmt.Errorf("apply calibration negative span: %w", ErrInvalidCalibration)
	}
	positiveSpan := calibration.Max - calibration.Neutral
	if !finite(positiveSpan) {
		return 0, fmt.Errorf("apply calibration positive span: %w", ErrInvalidCalibration)
	}

	value := raw
	if value < calibration.Min {
		value = calibration.Min
	} else if value > calibration.Max {
		value = calibration.Max
	}
	if !finite(value) {
		return 0, fmt.Errorf("apply calibration clamp: %w", ErrInvalidCalibration)
	}

	value -= calibration.Neutral
	if !finite(value) {
		return 0, fmt.Errorf("apply calibration center: %w", ErrInvalidCalibration)
	}

	scale := positiveSpan
	if value < 0 || calibration.Max == calibration.Neutral {
		scale = negativeSpan
	}
	if !finite(scale) || scale == 0 {
		return 0, fmt.Errorf("apply calibration scale: %w", ErrInvalidCalibration)
	}
	value /= scale
	if !finite(value) {
		return 0, fmt.Errorf("apply calibration normalize: %w", ErrInvalidCalibration)
	}
	if calibration.Invert {
		value = -value
		if !finite(value) {
			return 0, fmt.Errorf("apply calibration invert: %w", ErrInvalidCalibration)
		}
	}
	value *= calibration.Gain
	if !finite(value) {
		return 0, fmt.Errorf("apply calibration gain: %w", ErrInvalidCalibration)
	}
	return value, nil
}
