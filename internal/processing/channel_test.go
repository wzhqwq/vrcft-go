package processing

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestExpressionChannelsRoundTripEveryID(t *testing.T) {
	seen := map[ChannelID]struct{}{}
	for id := trackingmodel.ExpressionID(0); id < trackingmodel.ExpressionCount; id++ {
		channel, ok := ExpressionChannel(id)
		if !ok {
			t.Fatalf("ExpressionChannel(%d) invalid", id)
		}
		got, ok := channel.ExpressionID()
		if !ok || got != id {
			t.Fatalf("round trip = %d,%t", got, ok)
		}
		if _, duplicate := seen[channel]; duplicate {
			t.Fatalf("duplicate channel %d", channel)
		}
		seen[channel] = struct{}{}
	}

	if _, ok := ExpressionChannel(trackingmodel.ExpressionCount); ok {
		t.Fatal("ExpressionChannel(ExpressionCount) valid")
	}
	if _, ok := ChannelID(0).ExpressionID(); ok {
		t.Fatal("ChannelID(0).ExpressionID valid")
	}
}

func TestEyeChannelsHaveStableValues(t *testing.T) {
	tests := []struct {
		name string
		got  ChannelID
		want ChannelID
	}{
		{"left gaze x", ChannelEyeLeftGazeX, 1},
		{"left gaze y", ChannelEyeLeftGazeY, 2},
		{"right gaze x", ChannelEyeRightGazeX, 3},
		{"right gaze y", ChannelEyeRightGazeY, 4},
		{"left openness", ChannelEyeLeftOpenness, 5},
		{"right openness", ChannelEyeRightOpenness, 6},
		{"left pupil diameter", ChannelEyeLeftPupilDiameter, 7},
		{"right pupil diameter", ChannelEyeRightPupilDiameter, 8},
		{"left pupil dilation", ChannelEyeLeftPupilDilation, 9},
		{"right pupil dilation", ChannelEyeRightPupilDilation, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("channel = %d, want %d", tt.got, tt.want)
			}
		})
	}
}

func TestAllChannelsAreCompleteUniqueAndCallerOwned(t *testing.T) {
	channels := AllChannels()
	wantCount := 10 + int(trackingmodel.ExpressionCount)
	if len(channels) != wantCount {
		t.Fatalf("len(AllChannels()) = %d, want %d", len(channels), wantCount)
	}

	seen := make(map[ChannelID]struct{}, wantCount)
	for _, channel := range channels {
		if _, duplicate := seen[channel]; duplicate {
			t.Fatalf("AllChannels duplicate %d", channel)
		}
		seen[channel] = struct{}{}
	}
	for _, channel := range []ChannelID{
		ChannelEyeLeftGazeX,
		ChannelEyeLeftGazeY,
		ChannelEyeRightGazeX,
		ChannelEyeRightGazeY,
		ChannelEyeLeftOpenness,
		ChannelEyeRightOpenness,
		ChannelEyeLeftPupilDiameter,
		ChannelEyeRightPupilDiameter,
		ChannelEyeLeftPupilDilation,
		ChannelEyeRightPupilDilation,
	} {
		if _, found := seen[channel]; !found {
			t.Fatalf("AllChannels missing eye channel %d", channel)
		}
	}

	channels[0] = 0
	if AllChannels()[0] != ChannelEyeLeftGazeX {
		t.Fatal("AllChannels returned shared backing storage")
	}
}
