package processing

import (
	"errors"
	"math"
	"testing"
)

func TestTuning(t *testing.T) {
	tuning := ChannelTuning{Deadzone: 0.2, Gain: 2, Exponent: 2, ClampEnabled: true, ClampMin: -1, ClampMax: 1}
	got, err := applyTuning(tuning, 0.6) // 0.6 -> 0.5 -> 1 -> 1 -> 1
	if err != nil || got != 1 {
		t.Fatalf("got %v,%v", got, err)
	}
}

func TestTuningDeadzoneAndSign(t *testing.T) {
	tuning := ChannelTuning{Deadzone: 0.2, Gain: 1, Exponent: 1}
	for _, test := range []struct{ input, want float32 }{{0.2, 0}, {-0.6, -0.5}, {0.2008, 0.001}} {
		got, err := applyTuning(tuning, test.input)
		if err != nil || math.Abs(float64(got-test.want)) > 0.000001 {
			t.Fatalf("input %v = %v,%v; want %v,nil", test.input, got, err, test.want)
		}
	}
}

func TestTuningCanLeaveValuesUnclamped(t *testing.T) {
	tuning := ChannelTuning{Deadzone: 0, Gain: 2, Exponent: 1}
	got, err := applyTuning(tuning, 0.75)
	if err != nil || got != 1.5 {
		t.Fatalf("applyTuning() = %v, %v; want 1.5, nil", got, err)
	}
}

func TestTuningRejectsInvalidConfigurationAndInput(t *testing.T) {
	valid := ChannelTuning{Gain: 1, Exponent: 1}
	for _, test := range []struct {
		name   string
		tuning ChannelTuning
		input  float32
	}{
		{"negative deadzone", ChannelTuning{Deadzone: -0.1, Gain: 1, Exponent: 1}, 0},
		{"deadzone one", ChannelTuning{Deadzone: 1, Gain: 1, Exponent: 1}, 0},
		{"nonpositive gain", ChannelTuning{Gain: 0, Exponent: 1}, 0},
		{"nonpositive exponent", ChannelTuning{Gain: 1, Exponent: 0}, 0},
		{"nonfinite deadzone", ChannelTuning{Deadzone: float32(math.NaN()), Gain: 1, Exponent: 1}, 0},
		{"nonfinite gain", ChannelTuning{Gain: float32(math.Inf(1)), Exponent: 1}, 0},
		{"nonfinite exponent", ChannelTuning{Gain: 1, Exponent: float32(math.NaN())}, 0},
		{"equal clamp bounds", ChannelTuning{Gain: 1, Exponent: 1, ClampEnabled: true, ClampMin: 0, ClampMax: 0}, 0},
		{"reversed clamp bounds", ChannelTuning{Gain: 1, Exponent: 1, ClampEnabled: true, ClampMin: 1, ClampMax: 0}, 0},
		{"nonfinite clamp minimum", ChannelTuning{Gain: 1, Exponent: 1, ClampEnabled: true, ClampMin: float32(math.Inf(-1)), ClampMax: 1}, 0},
		{"nonfinite clamp maximum", ChannelTuning{Gain: 1, Exponent: 1, ClampEnabled: true, ClampMin: -1, ClampMax: float32(math.NaN())}, 0},
		{"nonfinite input", valid, float32(math.Inf(1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := applyTuning(test.tuning, test.input)
			if !errors.Is(err, ErrInvalidTuning) {
				t.Fatalf("applyTuning() error = %v; want errors.Is(_, %v)", err, ErrInvalidTuning)
			}
		})
	}
}
