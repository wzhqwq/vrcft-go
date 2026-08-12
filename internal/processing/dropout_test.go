package processing

import (
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestDropoutTimelineHoldsDecaysAndRemainsValidNeutral(t *testing.T) {
	pipeline := mustPipeline(t, dropoutTestConfig())
	frame := eyeFrame(1, 1, 100, "eye", 0.8)
	if _, err := pipeline.ProcessAt(frame, 100); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		now        int64
		want       float32
		wantActive bool
	}{
		{now: 110, want: 0.8, wantActive: true},
		{now: 115, want: 0.8, wantActive: false},
		{now: 120, want: 0.4, wantActive: false},
		{now: 125, want: 0, wantActive: false},
		{now: 200, want: 0, wantActive: false},
	}
	for _, test := range tests {
		got, err := pipeline.ProcessAt(frame, test.now)
		if err != nil {
			t.Fatalf("now %d: %v", test.now, err)
		}
		if got.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 ||
			got.Eye.LeftOpenness != test.want || got.EyeActive != test.wantActive {
			t.Fatalf("now %d: openness = %v valid=%#x active=%t; want %v valid and active=%t", test.now, got.Eye.LeftOpenness, got.Eye.Valid, got.EyeActive, test.want, test.wantActive)
		}
	}
}

func TestDropoutNeverSeenChannelRemainsInvalid(t *testing.T) {
	pipeline := mustPipeline(t, dropoutTestConfig())
	frame := eyeFrame(1, 1, 100, "eye", 0.8)
	got, err := pipeline.ProcessAt(frame, 200)
	if err != nil {
		t.Fatal(err)
	}
	if got.Eye.Valid&trackingmodel.EyeValidRightOpenness != 0 {
		t.Fatalf("right openness validity = %#x; want invalid", got.Eye.Valid)
	}
}

func TestDropoutCapabilityRemovalDeactivatesImmediatelyWhileHoldingValue(t *testing.T) {
	pipeline := mustPipeline(t, dropoutTestConfig())
	if _, err := pipeline.ProcessAt(eyeFrame(1, 1, 100, "eye", 0.8), 100); err != nil {
		t.Fatal(err)
	}

	removed := tracking.MergedFrame{Generation: 1, Sequence: 2, UpdatedAtNS: 110}
	got, err := pipeline.ProcessAt(removed, 110)
	if err != nil {
		t.Fatal(err)
	}
	if got.EyeActive || got.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 || got.Eye.LeftOpenness != 0.8 {
		t.Fatalf("removed output = %#v; want inactive held openness 0.8", got)
	}
}

func TestDropoutUnavailableSnapshotTimestampStartsDecayBeforeProcessing(t *testing.T) {
	pipeline := mustPipeline(t, dropoutTestConfig())
	if _, err := pipeline.ProcessAt(eyeFrame(1, 1, 100, "eye", 0.8), 100); err != nil {
		t.Fatal(err)
	}

	removed := tracking.MergedFrame{Generation: 1, Sequence: 2, UpdatedAtNS: 110}
	got, err := pipeline.ProcessAt(removed, 120)
	if err != nil {
		t.Fatal(err)
	}
	if got.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 || got.Eye.LeftOpenness != 0.4 {
		t.Fatalf("delayed removal output = %#v; want valid decay midpoint 0.4", got)
	}
}

func TestDropoutFreshRecoveryExitsNeutralState(t *testing.T) {
	pipeline := mustPipeline(t, dropoutTestConfig())
	stale := eyeFrame(1, 1, 100, "eye", 0.8)
	if _, err := pipeline.ProcessAt(stale, 100); err != nil {
		t.Fatal(err)
	}
	if got, err := pipeline.ProcessAt(stale, 125); err != nil || got.Eye.LeftOpenness != 0 {
		t.Fatalf("neutral output = %#v,%v; want zero", got, err)
	}

	fresh := eyeFrame(1, 2, 126, "eye", 0.6)
	got, err := pipeline.ProcessAt(fresh, 126)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EyeActive || got.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 || got.Eye.LeftOpenness != 0.6 {
		t.Fatalf("recovered output = %#v; want active valid openness 0.6", got)
	}
}

func dropoutTestConfig() Config {
	config := DefaultConfig()
	config.DefaultChannel.Dropout = DropoutPolicy{
		StaleAfter:    10 * time.Nanosecond,
		HoldDuration:  5 * time.Nanosecond,
		DecayDuration: 10 * time.Nanosecond,
	}
	config.ActiveStaleAfter = 10 * time.Nanosecond
	return config
}
