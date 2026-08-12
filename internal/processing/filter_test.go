package processing

import (
	"errors"
	"math"
	"testing"
)

func TestEMAInitializesFiltersAndResets(t *testing.T) {
	var state filterState
	config := FilterConfig{Mode: FilterEMA, EMAAlpha: 0.25}

	got, err := state.apply(config, 8, 1_000_000_000)
	if err != nil || got != 8 {
		t.Fatalf("first = %v,%v; want 8,nil", got, err)
	}
	got, err = state.apply(config, 4, 2_000_000_000)
	if err != nil || got != 7 {
		t.Fatalf("second = %v,%v; want 7,nil", got, err)
	}

	state.reset()
	got, err = state.apply(config, 2, 3_000_000_000)
	if err != nil || got != 2 {
		t.Fatalf("reset = %v,%v; want 2,nil", got, err)
	}
}

func TestFilterNonePreservesIdentityAndRequiresIncreasingTimes(t *testing.T) {
	var state filterState
	config := FilterConfig{Mode: FilterNone}

	for _, sample := range []struct {
		value float32
		atNS  int64
	}{{3, 1_000_000_000}, {-2, 2_000_000_000}} {
		got, err := state.apply(config, sample.value, sample.atNS)
		if err != nil || got != sample.value {
			t.Fatalf("apply(%v, %d) = %v,%v; want %v,nil", sample.value, sample.atNS, got, err, sample.value)
		}
	}

	if _, err := state.apply(config, 4, 2_000_000_000); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("nonincreasing time error = %v; want errors.Is(_, %v)", err, ErrInvalidFilter)
	}
}

func TestOneEuroMatchesHandCalculatedConstantCutoff(t *testing.T) {
	var state filterState
	config := FilterConfig{Mode: FilterOneEuro, MinCutoff: 1, Beta: 0, DerivativeCutoff: 1}

	if got, err := state.apply(config, 0, 0); err != nil || got != 0 {
		t.Fatalf("first = %v,%v; want 0,nil", got, err)
	}
	got, err := state.apply(config, 1, 1_000_000_000)
	alpha := 1 / (1 + 1/(2*math.Pi))
	if err != nil || math.Abs(float64(got)-alpha) > 1e-6 {
		t.Fatalf("second = %v,%v; want %v within 1e-6,nil", got, err, alpha)
	}
}

func TestOneEuroBetaMatchesIndependentReference(t *testing.T) {
	config := FilterConfig{Mode: FilterOneEuro, MinCutoff: 1, Beta: 0.75, DerivativeCutoff: 1.5}
	samples := []filterSample{
		{value: 0, atNS: 0},
		{value: 1, atNS: 250_000_000},
		{value: -0.25, atNS: 750_000_000},
		{value: 2, atNS: 1_500_000_000},
	}
	want := oneEuroReference(t, config, samples)

	var state filterState
	for i, sample := range samples {
		got, err := state.apply(config, sample.value, sample.atNS)
		if err != nil || math.Abs(float64(got-want[i])) > 1e-6 {
			t.Fatalf("sample %d = %v,%v; want %v within 1e-6,nil", i, got, err, want[i])
		}
	}
}

func TestFilterRejectsInvalidConfigurationAndValues(t *testing.T) {
	validEMA := FilterConfig{Mode: FilterEMA, EMAAlpha: 0.5}
	for _, test := range []struct {
		name   string
		config FilterConfig
		value  float32
	}{
		{"invalid EMA configuration", FilterConfig{Mode: FilterEMA, EMAAlpha: 0}, 1},
		{"invalid One Euro configuration", FilterConfig{Mode: FilterOneEuro, MinCutoff: 1, Beta: float32(math.NaN()), DerivativeCutoff: 1}, 1},
		{"nonfinite value", validEMA, float32(math.Inf(1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			var state filterState
			if _, err := state.apply(test.config, test.value, 0); !errors.Is(err, ErrInvalidFilter) {
				t.Fatalf("apply() error = %v; want errors.Is(_, %v)", err, ErrInvalidFilter)
			}
		})
	}
}

type filterSample struct {
	value float32
	atNS  int64
}

func oneEuroReference(t *testing.T, config FilterConfig, samples []filterSample) []float32 {
	t.Helper()
	results := make([]float32, len(samples))
	var lastRaw, filteredValue, filteredDerivative float64
	for i, sample := range samples {
		value := float64(sample.value)
		if i == 0 {
			lastRaw = value
			filteredValue = value
			results[i] = sample.value
			continue
		}

		dt := float64(samples[i].atNS-samples[i-1].atNS) / 1_000_000_000
		derivativeAlpha := 1 / (1 + 1/(2*math.Pi*float64(config.DerivativeCutoff)*dt))
		derivative := (value - lastRaw) / dt
		filteredDerivative = derivativeAlpha*derivative + (1-derivativeAlpha)*filteredDerivative
		cutoff := float64(config.MinCutoff) + float64(config.Beta)*math.Abs(filteredDerivative)
		valueAlpha := 1 / (1 + 1/(2*math.Pi*cutoff*dt))
		filteredValue = valueAlpha*value + (1-valueAlpha)*filteredValue
		lastRaw = value
		results[i] = float32(filteredValue)
	}
	return results
}
