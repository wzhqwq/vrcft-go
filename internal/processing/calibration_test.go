package processing

import (
	"errors"
	"math"
	"testing"
)

func TestCalibration(t *testing.T) {
	cal := ChannelCalibration{Enabled: true, Min: -2, Neutral: 0, Max: 4, Gain: 2}
	for _, test := range []struct{ raw, want float32 }{{-3, -2}, {-1, -1}, {0, 0}, {2, 1}, {5, 2}} {
		got, err := applyCalibration(cal, test.raw)
		if err != nil || got != test.want {
			t.Fatalf("raw %v = %v,%v", test.raw, got, err)
		}
	}
}

func TestCalibrationDisabledIsIdentity(t *testing.T) {
	got, err := applyCalibration(ChannelCalibration{}, 0.75)
	if err != nil || got != 0.75 {
		t.Fatalf("applyCalibration() = %v, %v; want 0.75, nil", got, err)
	}
}

func TestCalibrationInvertsOutput(t *testing.T) {
	cal := ChannelCalibration{Enabled: true, Min: -1, Neutral: 0, Max: 1, Gain: 1, Invert: true}
	got, err := applyCalibration(cal, 0.5)
	if err != nil || got != -0.5 {
		t.Fatalf("applyCalibration() = %v, %v; want -0.5, nil", got, err)
	}
}

func TestCalibrationSupportsOneSidedRanges(t *testing.T) {
	for _, test := range []struct {
		name string
		cal  ChannelCalibration
		raw  float32
		want float32
	}{
		{"positive", ChannelCalibration{Enabled: true, Min: 0, Neutral: 0, Max: 2, Gain: 1}, 1, 0.5},
		{"negative", ChannelCalibration{Enabled: true, Min: -2, Neutral: 0, Max: 0, Gain: 1}, -1, -0.5},
		{"negative neutral", ChannelCalibration{Enabled: true, Min: -2, Neutral: 0, Max: 0, Gain: 1}, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := applyCalibration(test.cal, test.raw)
			if err != nil || got != test.want {
				t.Fatalf("applyCalibration() = %v, %v; want %v, nil", got, err, test.want)
			}
		})
	}
}

func TestCalibrationRejectsInvalidConfigurationAndInput(t *testing.T) {
	valid := ChannelCalibration{Enabled: true, Min: -1, Neutral: 0, Max: 1, Gain: 1}
	for _, test := range []struct {
		name string
		cal  ChannelCalibration
		raw  float32
	}{
		{"all equal", ChannelCalibration{Enabled: true, Min: 1, Neutral: 1, Max: 1, Gain: 1}, 0},
		{"minimum above neutral", ChannelCalibration{Enabled: true, Min: 1, Neutral: 0, Max: 2, Gain: 1}, 0},
		{"neutral above maximum", ChannelCalibration{Enabled: true, Min: -1, Neutral: 2, Max: 1, Gain: 1}, 0},
		{"nonpositive gain", ChannelCalibration{Enabled: true, Min: -1, Neutral: 0, Max: 1, Gain: 0}, 0},
		{"nonfinite minimum", ChannelCalibration{Enabled: true, Min: float32(math.Inf(1)), Neutral: 0, Max: 1, Gain: 1}, 0},
		{"nonfinite neutral", ChannelCalibration{Enabled: true, Min: -1, Neutral: float32(math.NaN()), Max: 1, Gain: 1}, 0},
		{"nonfinite maximum", ChannelCalibration{Enabled: true, Min: -1, Neutral: 0, Max: float32(math.Inf(1)), Gain: 1}, 0},
		{"nonfinite gain", ChannelCalibration{Enabled: true, Min: -1, Neutral: 0, Max: 1, Gain: float32(math.NaN())}, 0},
		{"nonfinite input", valid, float32(math.Inf(1))},
		{"disabled nonfinite input", ChannelCalibration{}, float32(math.NaN())},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := applyCalibration(test.cal, test.raw)
			if !errors.Is(err, ErrInvalidCalibration) {
				t.Fatalf("applyCalibration() error = %v; want errors.Is(_, %v)", err, ErrInvalidCalibration)
			}
		})
	}
}

func TestCalibrationRejectsIntermediateSpanOverflow(t *testing.T) {
	calibration := ChannelCalibration{
		Enabled: true,
		Min:     -math.MaxFloat32,
		Neutral: -math.MaxFloat32 / 2,
		Max:     math.MaxFloat32,
		Gain:    1,
	}
	_, err := applyCalibration(calibration, -math.MaxFloat32)
	if !errors.Is(err, ErrInvalidCalibration) {
		t.Fatalf("applyCalibration() error = %v; want errors.Is(_, %v)", err, ErrInvalidCalibration)
	}
}
