package userconfig

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/processing"
)

func TestProcessingDefaultRoundTripAndStableNames(t *testing.T) {
	wire, err := processingToWire(processing.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	got, err := processingFromWire(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, processing.DefaultConfig()) {
		t.Fatalf("round trip = %#v", got)
	}
	if len(channelNames()) != len(processing.AllChannels()) {
		t.Fatalf("channel names = %d, channels = %d", len(channelNames()), len(processing.AllChannels()))
	}
	for _, channel := range processing.AllChannels() {
		if _, ok := channelNames()[channel]; !ok {
			t.Fatalf("missing stable name for %d", channel)
		}
	}
}

func TestProcessingRejectsInvalidWireValues(t *testing.T) {
	base, err := processingToWire(processing.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Processing)
		want   error
	}{
		{"unknown override", func(p *Processing) { p.Overrides = []ProcessingOverride{{Name: "unknown", Channel: p.DefaultChannel}} }, processing.ErrUnknownChannel},
		{"duplicate override", func(p *Processing) {
			p.Overrides = []ProcessingOverride{{Name: "eye.left_gaze_x", Channel: p.DefaultChannel}, {Name: "eye.left_gaze_x", Channel: p.DefaultChannel}}
		}, processing.ErrInvalidConfig},
		{"invalid milliseconds", func(p *Processing) { p.ActiveStaleAfterMs = 0 }, processing.ErrInvalidDropout},
		{"duplicate group membership", func(p *Processing) {
			p.MutualExclusion = [][]string{{"eye.left_gaze_x", "eye.right_gaze_x"}, {"eye.left_gaze_x", "eye.left_gaze_y"}}
		}, processing.ErrInvalidMutualExclusion},
		{"non finite", func(p *Processing) { p.DefaultChannel.Tuning.Gain = float32(math.NaN()) }, processing.ErrInvalidTuning},
		{"non finite calibration", func(p *Processing) { p.DefaultChannel.Calibration.Gain = float32(math.Inf(1)) }, processing.ErrInvalidCalibration},
		{"non finite filter", func(p *Processing) { p.DefaultChannel.Filter.Beta = float32(math.NaN()) }, processing.ErrInvalidFilter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := base.Clone()
			tt.mutate(&value)
			_, err := processingFromWire(value)
			if !errors.Is(err, tt.want) {
				t.Fatalf("processingFromWire() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestProcessingWireOutputSortsOverrides(t *testing.T) {
	config := processing.DefaultConfig()
	config.Overrides[processing.ChannelEyeRightGazeX] = config.DefaultChannel
	config.Overrides[processing.ChannelEyeLeftGazeX] = config.DefaultChannel
	wire, err := processingToWire(config)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := []string{wire.Overrides[0].Name, wire.Overrides[1].Name}, []string{"eye.left_gaze_x", "eye.right_gaze_x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("override names = %#v, want %#v", got, want)
	}
}
