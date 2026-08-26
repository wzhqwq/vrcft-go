package userconfig

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

var (
	channelByName map[string]processing.ChannelID
	nameByChannel map[processing.ChannelID]string
)

func init() {
	channelByName, nameByChannel = buildChannelNames()
}

func buildChannelNames() (map[string]processing.ChannelID, map[processing.ChannelID]string) {
	byName := make(map[string]processing.ChannelID)
	byChannel := make(map[processing.ChannelID]string)
	register := func(name string, channel processing.ChannelID) { byName[name], byChannel[channel] = channel, name }
	register("eye.left_gaze_x", processing.ChannelEyeLeftGazeX)
	register("eye.left_gaze_y", processing.ChannelEyeLeftGazeY)
	register("eye.right_gaze_x", processing.ChannelEyeRightGazeX)
	register("eye.right_gaze_y", processing.ChannelEyeRightGazeY)
	register("eye.left_openness", processing.ChannelEyeLeftOpenness)
	register("eye.right_openness", processing.ChannelEyeRightOpenness)
	register("eye.left_pupil_diameter", processing.ChannelEyeLeftPupilDiameter)
	register("eye.right_pupil_diameter", processing.ChannelEyeRightPupilDiameter)
	register("eye.left_pupil_dilation", processing.ChannelEyeLeftPupilDilation)
	register("eye.right_pupil_dilation", processing.ChannelEyeRightPupilDilation)
	for id, name := range trackingmodel.ExpressionNames() {
		channel, ok := processing.ExpressionChannel(trackingmodel.ExpressionID(id))
		if !ok {
			panic("userconfig: expression channel table is inconsistent")
		}
		register("expression:"+name, channel)
	}
	return byName, byChannel
}

func channelNames() map[processing.ChannelID]string {
	copy := make(map[processing.ChannelID]string, len(nameByChannel))
	for id, name := range nameByChannel {
		copy[id] = name
	}
	return copy
}

func processingToWire(config processing.Config) (Processing, error) {
	if _, err := processing.NewPipeline(config); err != nil {
		return Processing{}, err
	}
	defaultChannel, err := channelToWire(config.DefaultChannel)
	if err != nil {
		return Processing{}, err
	}
	active, err := durationToMilliseconds(config.ActiveStaleAfter, false)
	if err != nil {
		return Processing{}, err
	}
	wire := Processing{DefaultChannel: defaultChannel, ActiveStaleAfterMs: active}
	if config.MutualExclusion != nil {
		wire.MutualExclusion = make([][]string, len(config.MutualExclusion))
	}
	for i, group := range config.MutualExclusion {
		wire.MutualExclusion[i] = make([]string, len(group))
		for j, channel := range group {
			name, ok := nameByChannel[channel]
			if !ok {
				return Processing{}, processing.ErrUnknownChannel
			}
			wire.MutualExclusion[i][j] = name
		}
	}
	keys := make([]processing.ChannelID, 0, len(config.Overrides))
	for id := range config.Overrides {
		keys = append(keys, id)
	}
	sort.Slice(keys, func(i, j int) bool { return nameByChannel[keys[i]] < nameByChannel[keys[j]] })
	wire.Overrides = make([]ProcessingOverride, len(keys))
	for i, id := range keys {
		name, ok := nameByChannel[id]
		if !ok {
			return Processing{}, processing.ErrUnknownChannel
		}
		channel, err := channelToWire(config.Overrides[id])
		if err != nil {
			return Processing{}, err
		}
		wire.Overrides[i] = ProcessingOverride{Name: name, Channel: channel}
	}
	return wire, nil
}

func processingFromWire(wire Processing) (processing.Config, error) {
	defaultChannel, err := channelFromWire(wire.DefaultChannel)
	if err != nil {
		return processing.Config{}, err
	}
	active, err := millisecondsToDuration(wire.ActiveStaleAfterMs, false)
	if err != nil {
		return processing.Config{}, err
	}
	config := processing.Config{DefaultChannel: defaultChannel, Overrides: make(map[processing.ChannelID]processing.ChannelConfig, len(wire.Overrides)), ActiveStaleAfter: active}
	if wire.MutualExclusion != nil {
		config.MutualExclusion = make([][]processing.ChannelID, len(wire.MutualExclusion))
	}
	for i, override := range wire.Overrides {
		channel, ok := channelByName[override.Name]
		if !ok {
			return processing.Config{}, fmt.Errorf("override %d: %w", i, processing.ErrUnknownChannel)
		}
		if _, duplicate := config.Overrides[channel]; duplicate {
			return processing.Config{}, fmt.Errorf("override %d: %w", i, processing.ErrInvalidConfig)
		}
		value, err := channelFromWire(override.Channel)
		if err != nil {
			return processing.Config{}, fmt.Errorf("override %d: %w", i, err)
		}
		config.Overrides[channel] = value
	}
	for i, group := range wire.MutualExclusion {
		config.MutualExclusion[i] = make([]processing.ChannelID, len(group))
		for j, name := range group {
			channel, ok := channelByName[name]
			if !ok {
				return processing.Config{}, fmt.Errorf("group %d channel %d: %w", i, j, processing.ErrUnknownChannel)
			}
			config.MutualExclusion[i][j] = channel
		}
	}
	if _, err := processing.NewPipeline(config); err != nil {
		return processing.Config{}, err
	}
	return config, nil
}

func channelToWire(channel processing.ChannelConfig) (ProcessingChannel, error) {
	if err := ensureFiniteChannel(channel); err != nil {
		return ProcessingChannel{}, err
	}
	hold, err := durationToMilliseconds(channel.Dropout.HoldDuration, true)
	if err != nil {
		return ProcessingChannel{}, err
	}
	decay, err := durationToMilliseconds(channel.Dropout.DecayDuration, true)
	if err != nil {
		return ProcessingChannel{}, err
	}
	stale, err := durationToMilliseconds(channel.Dropout.StaleAfter, false)
	if err != nil {
		return ProcessingChannel{}, err
	}
	return ProcessingChannel{Calibration: Calibration{channel.Calibration.Enabled, channel.Calibration.Neutral, channel.Calibration.Min, channel.Calibration.Max, channel.Calibration.Gain, channel.Calibration.Invert}, Tuning: Tuning{channel.Tuning.Deadzone, channel.Tuning.Gain, channel.Tuning.Exponent, channel.Tuning.ClampEnabled, channel.Tuning.ClampMin, channel.Tuning.ClampMax}, Filter: Filter{string(channel.Filter.Mode), channel.Filter.EMAAlpha, channel.Filter.MinCutoff, channel.Filter.Beta, channel.Filter.DerivativeCutoff}, Dropout: Dropout{hold, decay, stale}}, nil
}

func channelFromWire(channel ProcessingChannel) (processing.ChannelConfig, error) {
	for _, value := range []float32{channel.Calibration.Neutral, channel.Calibration.Min, channel.Calibration.Max, channel.Calibration.Gain} {
		if !finite(value) {
			return processing.ChannelConfig{}, processing.ErrInvalidCalibration
		}
	}
	for _, value := range []float32{channel.Tuning.Deadzone, channel.Tuning.Gain, channel.Tuning.Exponent, channel.Tuning.ClampMin, channel.Tuning.ClampMax} {
		if !finite(value) {
			return processing.ChannelConfig{}, processing.ErrInvalidTuning
		}
	}
	for _, value := range []float32{channel.Filter.EMAAlpha, channel.Filter.MinCutoff, channel.Filter.Beta, channel.Filter.DerivativeCutoff} {
		if !finite(value) {
			return processing.ChannelConfig{}, processing.ErrInvalidFilter
		}
	}
	hold, err := millisecondsToDuration(channel.Dropout.HoldDurationMs, true)
	if err != nil {
		return processing.ChannelConfig{}, err
	}
	decay, err := millisecondsToDuration(channel.Dropout.DecayDurationMs, true)
	if err != nil {
		return processing.ChannelConfig{}, err
	}
	stale, err := millisecondsToDuration(channel.Dropout.StaleAfterMs, false)
	if err != nil {
		return processing.ChannelConfig{}, err
	}
	result := processing.ChannelConfig{Calibration: processing.ChannelCalibration{Enabled: channel.Calibration.Enabled, Neutral: channel.Calibration.Neutral, Min: channel.Calibration.Min, Max: channel.Calibration.Max, Gain: channel.Calibration.Gain, Invert: channel.Calibration.Invert}, Tuning: processing.ChannelTuning{Deadzone: channel.Tuning.Deadzone, Gain: channel.Tuning.Gain, Exponent: channel.Tuning.Exponent, ClampEnabled: channel.Tuning.ClampEnabled, ClampMin: channel.Tuning.ClampMin, ClampMax: channel.Tuning.ClampMax}, Filter: processing.FilterConfig{Mode: processing.FilterMode(channel.Filter.Mode), EMAAlpha: channel.Filter.EMAAlpha, MinCutoff: channel.Filter.MinCutoff, Beta: channel.Filter.Beta, DerivativeCutoff: channel.Filter.DerivativeCutoff}, Dropout: processing.DropoutPolicy{HoldDuration: hold, DecayDuration: decay, StaleAfter: stale}}
	return result, nil
}

func ensureFiniteChannel(channel processing.ChannelConfig) error {
	_, err := channelFromWire(ProcessingChannel{Calibration: Calibration{Neutral: channel.Calibration.Neutral, Min: channel.Calibration.Min, Max: channel.Calibration.Max, Gain: channel.Calibration.Gain}, Tuning: Tuning{Deadzone: channel.Tuning.Deadzone, Gain: channel.Tuning.Gain, Exponent: channel.Tuning.Exponent, ClampMin: channel.Tuning.ClampMin, ClampMax: channel.Tuning.ClampMax}, Filter: Filter{EMAAlpha: channel.Filter.EMAAlpha, MinCutoff: channel.Filter.MinCutoff, Beta: channel.Filter.Beta, DerivativeCutoff: channel.Filter.DerivativeCutoff}, Dropout: Dropout{HoldDurationMs: 0, DecayDurationMs: 0, StaleAfterMs: 1}})
	return err
}
func finite(value float32) bool { return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) }
func durationToMilliseconds(value time.Duration, zeroAllowed bool) (int64, error) {
	if value < 0 || !zeroAllowed && value == 0 || value%time.Millisecond != 0 {
		return 0, processing.ErrInvalidDropout
	}
	return int64(value / time.Millisecond), nil
}
func millisecondsToDuration(value int64, zeroAllowed bool) (time.Duration, error) {
	if value < 0 || !zeroAllowed && value == 0 || value > int64(math.MaxInt64/int64(time.Millisecond)) {
		return 0, processing.ErrInvalidDropout
	}
	return time.Duration(value) * time.Millisecond, nil
}
