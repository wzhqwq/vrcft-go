package processing

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultConfigProvidesIndependentIdentityConfiguration(t *testing.T) {
	first := DefaultConfig()
	if first.ActiveStaleAfter != 500*time.Millisecond {
		t.Fatalf("ActiveStaleAfter = %v, want 500ms", first.ActiveStaleAfter)
	}
	if first.DefaultChannel.Dropout.StaleAfter != 500*time.Millisecond {
		t.Fatalf("channel StaleAfter = %v, want 500ms", first.DefaultChannel.Dropout.StaleAfter)
	}
	if first.DefaultChannel.Dropout.HoldDuration != 100*time.Millisecond {
		t.Fatalf("HoldDuration = %v, want 100ms", first.DefaultChannel.Dropout.HoldDuration)
	}
	if first.DefaultChannel.Dropout.DecayDuration != 300*time.Millisecond {
		t.Fatalf("DecayDuration = %v, want 300ms", first.DefaultChannel.Dropout.DecayDuration)
	}
	if first.DefaultChannel.Calibration.Enabled || first.DefaultChannel.Calibration.Gain != 1 || first.DefaultChannel.Calibration.Invert {
		t.Fatalf("calibration = %+v, want disabled identity", first.DefaultChannel.Calibration)
	}
	if first.DefaultChannel.Tuning.Deadzone != 0 || first.DefaultChannel.Tuning.Gain != 1 || first.DefaultChannel.Tuning.Exponent != 1 || first.DefaultChannel.Tuning.ClampEnabled {
		t.Fatalf("tuning = %+v, want unclamped identity", first.DefaultChannel.Tuning)
	}
	if first.DefaultChannel.Filter.Mode != FilterNone {
		t.Fatalf("filter mode = %q, want %q", first.DefaultChannel.Filter.Mode, FilterNone)
	}
	if len(first.Overrides) != 0 || len(first.MutualExclusion) != 0 {
		t.Fatalf("collections = overrides %d, exclusion groups %d, want empty", len(first.Overrides), len(first.MutualExclusion))
	}

	first.Overrides[ChannelEyeLeftGazeX] = first.DefaultChannel
	second := DefaultConfig()
	if len(second.Overrides) != 0 {
		t.Fatal("DefaultConfig returned a shared override map")
	}
}

func TestCompileConfigRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   error
	}{
		{"nonpositive active stale", func(c *Config) { c.ActiveStaleAfter = 0 }, ErrInvalidDropout},
		{"unknown override", func(c *Config) { c.Overrides = map[ChannelID]ChannelConfig{ChannelID(0xffff): c.DefaultChannel} }, ErrUnknownChannel},
		{"all equal calibration", func(c *Config) {
			c.DefaultChannel.Calibration = ChannelCalibration{Enabled: true, Min: 1, Neutral: 1, Max: 1, Gain: 1}
		}, ErrInvalidCalibration},
		{"deadzone one", func(c *Config) { c.DefaultChannel.Tuning.Deadzone = 1 }, ErrInvalidTuning},
		{"EMA alpha zero", func(c *Config) { c.DefaultChannel.Filter = FilterConfig{Mode: FilterEMA, EMAAlpha: 0} }, ErrInvalidFilter},
		{"One Euro cutoff zero", func(c *Config) {
			c.DefaultChannel.Filter = FilterConfig{Mode: FilterOneEuro, MinCutoff: 0, DerivativeCutoff: 1}
		}, ErrInvalidFilter},
		{"negative hold", func(c *Config) { c.DefaultChannel.Dropout.HoldDuration = -time.Nanosecond }, ErrInvalidDropout},
		{"short exclusion", func(c *Config) { c.MutualExclusion = [][]ChannelID{{ChannelEyeLeftGazeX}} }, ErrInvalidMutualExclusion},
		{"duplicate across groups", func(c *Config) {
			c.MutualExclusion = [][]ChannelID{{ChannelEyeLeftGazeX, ChannelEyeLeftGazeY}, {ChannelEyeLeftGazeX, ChannelEyeRightGazeX}}
		}, ErrInvalidMutualExclusion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.mutate(&config)
			if _, err := compileConfig(config); !errors.Is(err, tt.want) {
				t.Fatalf("compileConfig() error = %v, want errors.Is(_, %v)", err, tt.want)
			}
		})
	}
}

func TestCompileConfigValidatesCalibrationOrdering(t *testing.T) {
	tests := []struct {
		name        string
		calibration ChannelCalibration
		want        error
	}{
		{"minimum equals neutral", ChannelCalibration{Enabled: true, Min: 0, Neutral: 0, Max: 1, Gain: 1}, nil},
		{"neutral equals maximum", ChannelCalibration{Enabled: true, Min: -1, Neutral: 1, Max: 1, Gain: 1}, nil},
		{"all equal", ChannelCalibration{Enabled: true, Min: 1, Neutral: 1, Max: 1, Gain: 1}, ErrInvalidCalibration},
		{"minimum above neutral", ChannelCalibration{Enabled: true, Min: 1, Neutral: 0, Max: 2, Gain: 1}, ErrInvalidCalibration},
		{"neutral above maximum", ChannelCalibration{Enabled: true, Min: -1, Neutral: 2, Max: 1, Gain: 1}, ErrInvalidCalibration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.DefaultChannel.Calibration = tt.calibration
			_, err := compileConfig(config)
			if !errors.Is(err, tt.want) {
				t.Fatalf("compileConfig() error = %v, want errors.Is(_, %v)", err, tt.want)
			}
		})
	}
}

func TestCompileConfigDurationValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   error
	}{
		{"negative active stale", func(c *Config) { c.ActiveStaleAfter = -time.Nanosecond }, ErrInvalidDropout},
		{"zero channel stale", func(c *Config) { c.DefaultChannel.Dropout.StaleAfter = 0 }, ErrInvalidDropout},
		{"negative channel stale", func(c *Config) { c.DefaultChannel.Dropout.StaleAfter = -time.Nanosecond }, ErrInvalidDropout},
		{"negative decay", func(c *Config) { c.DefaultChannel.Dropout.DecayDuration = -time.Nanosecond }, ErrInvalidDropout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.mutate(&config)
			if _, err := compileConfig(config); !errors.Is(err, tt.want) {
				t.Fatalf("compileConfig() error = %v, want errors.Is(_, %v)", err, tt.want)
			}
		})
	}

	config := DefaultConfig()
	config.DefaultChannel.Dropout.HoldDuration = 0
	config.DefaultChannel.Dropout.DecayDuration = 0
	if _, err := compileConfig(config); err != nil {
		t.Fatalf("compileConfig() with zero hold and decay: %v", err)
	}
}

func TestCompileConfigCopiesCallerOwnedCollections(t *testing.T) {
	config := DefaultConfig()
	override := config.DefaultChannel
	override.Tuning.Gain = 2
	config.Overrides = map[ChannelID]ChannelConfig{ChannelEyeLeftGazeX: override}
	config.MutualExclusion = [][]ChannelID{{ChannelEyeLeftGazeX, ChannelEyeRightGazeX}}

	compiled, err := compileConfig(config)
	if err != nil {
		t.Fatalf("compileConfig(): %v", err)
	}
	if got := compiled.channels[ChannelEyeLeftGazeX-1].Tuning.Gain; got != 2 {
		t.Fatalf("compiled override gain = %v, want 2", got)
	}
	if got := compiled.mutualExclusion[0][0]; got != ChannelEyeLeftGazeX {
		t.Fatalf("compiled group first channel = %d, want %d", got, ChannelEyeLeftGazeX)
	}

	config.Overrides[ChannelEyeLeftGazeX] = config.DefaultChannel
	config.MutualExclusion[0][0] = ChannelEyeLeftGazeY
	if got := compiled.channels[ChannelEyeLeftGazeX-1].Tuning.Gain; got != 2 {
		t.Fatalf("compiled override changed after input mutation: %v", got)
	}
	if got := compiled.mutualExclusion[0][0]; got != ChannelEyeLeftGazeX {
		t.Fatalf("compiled group changed after input mutation: %d", got)
	}
}
