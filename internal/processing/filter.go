package processing

import (
	"fmt"
	"math"
)

type FilterMode string

const (
	FilterNone    FilterMode = "none"
	FilterEMA     FilterMode = "ema"
	FilterOneEuro FilterMode = "one_euro"
)

// FilterConfig contains the parameters for one supported filter mode.
type FilterConfig struct {
	Mode             FilterMode
	EMAAlpha         float32
	MinCutoff        float32
	Beta             float32
	DerivativeCutoff float32
}

// filterState holds the mutable state for one filtered channel.
type filterState struct {
	initialized        bool
	lastAtNS           int64
	lastRaw            float64
	filteredValue      float64
	filteredDerivative float64
}

func (state *filterState) reset() {
	*state = filterState{}
}

func (state *filterState) apply(config FilterConfig, value float32, atNS int64) (float32, error) {
	if err := validateFilter(config); err != nil || !finite(value) {
		return 0, fmt.Errorf("apply filter: %w", ErrInvalidFilter)
	}

	input := float64(value)
	if !finite64(input) {
		return 0, fmt.Errorf("apply filter input: %w", ErrInvalidFilter)
	}
	if !state.initialized {
		state.initialized = true
		state.lastAtNS = atNS
		state.lastRaw = input
		state.filteredValue = input
		state.filteredDerivative = 0
		return value, nil
	}
	if atNS <= state.lastAtNS {
		return 0, fmt.Errorf("apply filter time delta: %w", ErrInvalidFilter)
	}

	dt := float64(uint64(atNS)-uint64(state.lastAtNS)) / 1_000_000_000
	if !finite64(dt) || dt <= 0 {
		return 0, fmt.Errorf("apply filter time delta: %w", ErrInvalidFilter)
	}

	filteredValue := input
	filteredDerivative := 0.0
	switch config.Mode {
	case FilterNone:
	case FilterEMA:
		alpha := float64(config.EMAAlpha)
		filteredValue = alpha*input + (1-alpha)*state.filteredValue
	case FilterOneEuro:
		rawDerivative := (input - state.lastRaw) / dt
		if !finite64(rawDerivative) {
			return 0, fmt.Errorf("apply filter derivative: %w", ErrInvalidFilter)
		}

		derivativeAlpha := lowPassAlpha(float64(config.DerivativeCutoff), dt)
		if !finite64(derivativeAlpha) {
			return 0, fmt.Errorf("apply filter derivative alpha: %w", ErrInvalidFilter)
		}
		filteredDerivative = derivativeAlpha*rawDerivative + (1-derivativeAlpha)*state.filteredDerivative
		if !finite64(filteredDerivative) {
			return 0, fmt.Errorf("apply filter filtered derivative: %w", ErrInvalidFilter)
		}

		cutoff := float64(config.MinCutoff) + float64(config.Beta)*math.Abs(filteredDerivative)
		if !finite64(cutoff) || cutoff <= 0 {
			return 0, fmt.Errorf("apply filter cutoff: %w", ErrInvalidFilter)
		}
		valueAlpha := lowPassAlpha(cutoff, dt)
		if !finite64(valueAlpha) {
			return 0, fmt.Errorf("apply filter value alpha: %w", ErrInvalidFilter)
		}
		filteredValue = valueAlpha*input + (1-valueAlpha)*state.filteredValue
	default:
		return 0, fmt.Errorf("apply filter mode: %w", ErrInvalidFilter)
	}
	if !finite64(filteredValue) {
		return 0, fmt.Errorf("apply filter value: %w", ErrInvalidFilter)
	}

	result := float32(filteredValue)
	if !finite(result) {
		return 0, fmt.Errorf("apply filter result: %w", ErrInvalidFilter)
	}
	state.lastAtNS = atNS
	state.lastRaw = input
	state.filteredValue = filteredValue
	state.filteredDerivative = filteredDerivative
	return result, nil
}

func lowPassAlpha(cutoff, dt float64) float64 {
	tau := 1 / (2 * math.Pi * cutoff)
	return 1 / (1 + tau/dt)
}

func finite64(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
