package processing

import (
	"fmt"
	"math"
	"time"
)

// Config describes the pipeline's processing behavior before it is compiled.
type Config struct {
	DefaultChannel   ChannelConfig
	Overrides        map[ChannelID]ChannelConfig
	ActiveStaleAfter time.Duration
	MutualExclusion  [][]ChannelID
}

// ChannelConfig describes processing behavior for one scalar channel.
type ChannelConfig struct {
	Calibration ChannelCalibration
	Tuning      ChannelTuning
	Filter      FilterConfig
	Dropout     DropoutPolicy
}

// DefaultConfig returns independent, identity processing settings.
func DefaultConfig() Config {
	return Config{
		DefaultChannel: ChannelConfig{
			Calibration: ChannelCalibration{Gain: 1},
			Tuning:      ChannelTuning{Gain: 1, Exponent: 1},
			Filter:      FilterConfig{Mode: FilterNone},
			Dropout: DropoutPolicy{
				StaleAfter:    500 * time.Millisecond,
				HoldDuration:  100 * time.Millisecond,
				DecayDuration: 300 * time.Millisecond,
			},
		},
		Overrides:        make(map[ChannelID]ChannelConfig),
		ActiveStaleAfter: 500 * time.Millisecond,
	}
}

// compiledConfig stores the validated, allocation-free per-channel settings
// needed by the evaluator. Its contents never alias Config-owned collections.
type compiledConfig struct {
	channels         [channelCount]ChannelConfig
	activeStaleAfter time.Duration
	mutualExclusion  [][]ChannelID
}

func (config compiledConfig) channelConfig(id ChannelID) ChannelConfig {
	return config.channels[id-1]
}

func compileConfig(config Config) (compiledConfig, error) {
	if config.ActiveStaleAfter <= 0 {
		return compiledConfig{}, fmt.Errorf("active stale after: %w", ErrInvalidDropout)
	}
	if err := validateChannelConfig(config.DefaultChannel); err != nil {
		return compiledConfig{}, err
	}

	compiled := compiledConfig{activeStaleAfter: config.ActiveStaleAfter}
	for i := range compiled.channels {
		compiled.channels[i] = config.DefaultChannel
	}
	for id, channel := range config.Overrides {
		if !knownChannel(id) {
			return compiledConfig{}, fmt.Errorf("override channel %d: %w", id, ErrUnknownChannel)
		}
		if err := validateChannelConfig(channel); err != nil {
			return compiledConfig{}, fmt.Errorf("override channel %d: %w", id, err)
		}
		compiled.channels[id-1] = channel
	}

	groups, err := compileMutualExclusion(config.MutualExclusion)
	if err != nil {
		return compiledConfig{}, err
	}
	compiled.mutualExclusion = groups
	return compiled, nil
}

func validateChannelConfig(config ChannelConfig) error {
	if err := validateCalibration(config.Calibration); err != nil {
		return err
	}
	if err := validateTuning(config.Tuning); err != nil {
		return err
	}
	if err := validateFilter(config.Filter); err != nil {
		return err
	}
	if err := validateDropout(config.Dropout); err != nil {
		return err
	}
	return nil
}

func validateCalibration(config ChannelCalibration) error {
	if !config.Enabled {
		return nil
	}
	if !finite(config.Min) || !finite(config.Neutral) || !finite(config.Max) || !finite(config.Gain) ||
		config.Min > config.Neutral || config.Neutral > config.Max ||
		(config.Min == config.Neutral && config.Neutral == config.Max) || config.Gain <= 0 {
		return ErrInvalidCalibration
	}
	return nil
}

func validateTuning(config ChannelTuning) error {
	if !finite(config.Deadzone) || !finite(config.Gain) || !finite(config.Exponent) ||
		config.Deadzone < 0 || config.Deadzone >= 1 || config.Gain <= 0 || config.Exponent <= 0 {
		return ErrInvalidTuning
	}
	if config.ClampEnabled && (!finite(config.ClampMin) || !finite(config.ClampMax) || config.ClampMin >= config.ClampMax) {
		return ErrInvalidTuning
	}
	return nil
}

func validateFilter(config FilterConfig) error {
	switch config.Mode {
	case FilterNone:
		return nil
	case FilterEMA:
		if finite(config.EMAAlpha) && config.EMAAlpha > 0 && config.EMAAlpha <= 1 {
			return nil
		}
	case FilterOneEuro:
		if finite(config.MinCutoff) && finite(config.Beta) && finite(config.DerivativeCutoff) &&
			config.MinCutoff > 0 && config.Beta >= 0 && config.DerivativeCutoff > 0 {
			return nil
		}
	}
	return ErrInvalidFilter
}

func validateDropout(config DropoutPolicy) error {
	if config.StaleAfter <= 0 || config.HoldDuration < 0 || config.DecayDuration < 0 {
		return ErrInvalidDropout
	}
	return nil
}

func compileMutualExclusion(groups [][]ChannelID) ([][]ChannelID, error) {
	compiled := make([][]ChannelID, len(groups))
	seen := make(map[ChannelID]struct{})
	for groupIndex, group := range groups {
		if len(group) < 2 {
			return nil, fmt.Errorf("group %d: %w", groupIndex, ErrInvalidMutualExclusion)
		}
		compiled[groupIndex] = make([]ChannelID, len(group))
		for channelIndex, channel := range group {
			if !knownChannel(channel) {
				return nil, fmt.Errorf("group %d channel %d: %w", groupIndex, channel, ErrUnknownChannel)
			}
			if _, duplicate := seen[channel]; duplicate {
				return nil, fmt.Errorf("group %d channel %d: %w", groupIndex, channel, ErrInvalidMutualExclusion)
			}
			seen[channel] = struct{}{}
			compiled[groupIndex][channelIndex] = channel
		}
	}
	return compiled, nil
}

func finite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
