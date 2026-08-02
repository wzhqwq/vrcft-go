package tracking

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestSubmitErrorsHaveStableMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrGenerationUnset, "tracking: generation is unset"},
		{ErrGenerationZero, "tracking: generation must be positive"},
		{ErrGenerationRegression, "tracking: generation regression"},
		{ErrStaleGeneration, "tracking: stale generation"},
		{ErrFutureGeneration, "tracking: future generation"},
		{ErrInvalidPluginID, "tracking: invalid plugin ID"},
		{ErrInvalidFrame, "tracking: invalid frame"},
		{ErrSequenceNotIncreasing, "tracking: sequence is not increasing"},
		{ErrTimestampRegression, "tracking: timestamp regression"},
		{ErrSourceClockRegression, "tracking: source clock regression"},
	}

	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("sentinel error = %q, want %q", got, tt.want)
		}
	}
}

func TestSubmitRejectsInvalidPluginAndGeneration(t *testing.T) {
	tests := []struct {
		name          string
		setGeneration uint64
		pluginID      string
		generation    uint64
		wantErr       error
	}{
		{name: "empty plugin wins before unset generation", pluginID: "", generation: 0, wantErr: ErrInvalidPluginID},
		{name: "unset current generation", pluginID: "osc", generation: 0, wantErr: ErrGenerationUnset},
		{name: "zero submitted generation", setGeneration: 2, pluginID: "osc", generation: 0, wantErr: ErrGenerationZero},
		{name: "stale submitted generation", setGeneration: 2, pluginID: "osc", generation: 1, wantErr: ErrStaleGeneration},
		{name: "future submitted generation", setGeneration: 2, pluginID: "osc", generation: 3, wantErr: ErrFutureGeneration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clockCalls := 0
			s := newServiceWithClock(func() int64 {
				clockCalls++
				return 10
			})
			if tt.setGeneration != 0 {
				if err := s.SetGeneration(tt.setGeneration); err != nil {
					t.Fatalf("SetGeneration(%d) error = %v", tt.setGeneration, err)
				}
			}
			before, beforeOK := s.LatestMerged()
			beforeClockCalls := clockCalls

			err := s.Submit(tt.pluginID, tt.generation, trackingmodel.TrackingFrame{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Submit() error = %v, want %v", err, tt.wantErr)
			}
			if clockCalls != beforeClockCalls {
				t.Fatalf("Host clock calls = %d, want %d", clockCalls, beforeClockCalls)
			}
			if len(s.sources) != 0 {
				t.Fatalf("sources length = %d, want 0", len(s.sources))
			}
			if got, ok := s.LatestMerged(); ok != beforeOK || got != before {
				t.Fatalf("LatestMerged() = (%#v, %t), want unchanged (%#v, %t)", got, ok, before, beforeOK)
			}
		})
	}
}

func TestSubmitCanonicalizesUnselectedValuesBeforeStorage(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	frame := trackingmodel.TrackingFrame{
		Sequence:     4,
		TimestampNS:  30,
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:                trackingmodel.EyeValidLeftOpenness,
			LeftOpenness:         0.75,
			RightOpenness:        float32(math.NaN()),
			LeftPupilDiameterMM:  float32(math.Inf(1)),
			RightPupilDiameterMM: 6,
		},
	}
	frame.Expressions.Values[trackingmodel.ExpressionJawOpen] = float32(math.NaN())

	if err := s.Submit("osc", 1, frame); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	got, ok := s.sources["osc"]
	if !ok {
		t.Fatal("stored source is missing")
	}
	want := trackingmodel.TrackingFrame{
		Sequence:     4,
		TimestampNS:  30,
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.75,
		},
	}
	if got.frame != want {
		t.Fatalf("stored frame = %#v, want canonical %#v", got.frame, want)
	}
	if got.receivedAtNS != 20 || got.lastSequence != 4 || got.lastTimestampNS != 30 || got.lastSourceClockNS != 0 {
		t.Fatalf("stored source metadata = %#v, want received=20 sequence=4 timestamp=30 sourceClock=0", got)
	}
}

func TestSubmitRejectsMalformedValidityAndSelectedNonFiniteValues(t *testing.T) {
	tests := []struct {
		name  string
		frame trackingmodel.TrackingFrame
	}{
		{
			name: "unknown eye validity bit",
			frame: trackingmodel.TrackingFrame{
				Capabilities: trackingmodel.CapabilityEye,
				Eye:          trackingmodel.EyeSample{Valid: trackingmodel.EyeValid(1 << 15)},
			},
		},
		{
			name: "expression validity tail bit",
			frame: func() trackingmodel.TrackingFrame {
				frame := trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityExpression}
				frame.Expressions.Valid.Words[1] = uint64(1) << 63
				return frame
			}(),
		},
		{
			name: "selected eye value is NaN",
			frame: trackingmodel.TrackingFrame{
				Capabilities: trackingmodel.CapabilityEye,
				Eye: trackingmodel.EyeSample{
					Valid:        trackingmodel.EyeValidLeftOpenness,
					LeftOpenness: float32(math.NaN()),
				},
			},
		},
		{
			name: "selected expression value is infinite",
			frame: func() trackingmodel.TrackingFrame {
				frame := trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityExpression}
				frame.Expressions.Valid.Set(trackingmodel.ExpressionJawOpen)
				frame.Expressions.Values[trackingmodel.ExpressionJawOpen] = float32(math.Inf(1))
				return frame
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clockCalls := 0
			s := newServiceWithClock(func() int64 {
				clockCalls++
				return 10
			})
			if err := s.SetGeneration(1); err != nil {
				t.Fatalf("SetGeneration(1) error = %v", err)
			}
			tt.frame.Sequence = 424242

			err := s.Submit("osc", 1, tt.frame)
			if !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("Submit() error = %v, want ErrInvalidFrame", err)
			}
			if !strings.Contains(err.Error(), ErrInvalidFrame.Error()) {
				t.Fatalf("Submit() error = %q, want safe sentinel context", err)
			}
			if strings.Contains(err.Error(), "424242") || strings.Contains(err.Error(), "TrackingFrame{") {
				t.Fatalf("Submit() error leaked the submitted frame: %q", err)
			}
			if clockCalls != 1 {
				t.Fatalf("Host clock calls = %d, want 1", clockCalls)
			}
			if len(s.sources) != 0 {
				t.Fatalf("sources length = %d, want 0", len(s.sources))
			}
		})
	}
}

func TestSubmitAllowsFirstSequenceZeroAndRequiresStrictIncrease(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 10 })
	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 0}); err != nil {
		t.Fatalf("first Submit() with Sequence 0 error = %v", err)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 0}); !errors.Is(err, ErrSequenceNotIncreasing) {
		t.Fatalf("duplicate Submit() error = %v, want ErrSequenceNotIncreasing", err)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 2}); err != nil {
		t.Fatalf("increasing Submit() error = %v", err)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 1}); !errors.Is(err, ErrSequenceNotIncreasing) {
		t.Fatalf("lower Submit() error = %v, want ErrSequenceNotIncreasing", err)
	}
}

func TestSubmitRejectsNegativeTimestamps(t *testing.T) {
	tests := []struct {
		name  string
		frame trackingmodel.TrackingFrame
	}{
		{name: "negative timestamp", frame: trackingmodel.TrackingFrame{TimestampNS: -1}},
		{name: "negative source clock", frame: trackingmodel.TrackingFrame{SourceClockNS: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clockCalls := 0
			s := newServiceWithClock(func() int64 {
				clockCalls++
				return 10
			})
			if err := s.SetGeneration(1); err != nil {
				t.Fatalf("SetGeneration(1) error = %v", err)
			}

			err := s.Submit("osc", 1, tt.frame)
			if !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("Submit() error = %v, want ErrInvalidFrame", err)
			}
			if clockCalls != 1 {
				t.Fatalf("Host clock calls = %d, want 1", clockCalls)
			}
			if len(s.sources) != 0 {
				t.Fatalf("sources length = %d, want 0", len(s.sources))
			}
		})
	}
}

func TestSubmitEnforcesNonZeroTimestampBaselines(t *testing.T) {
	tests := []struct {
		name       string
		initial    trackingmodel.TrackingFrame
		zero       trackingmodel.TrackingFrame
		regressing trackingmodel.TrackingFrame
		equal      trackingmodel.TrackingFrame
		wantErr    error
	}{
		{
			name:       "timestamp",
			initial:    trackingmodel.TrackingFrame{Sequence: 1, TimestampNS: 100},
			zero:       trackingmodel.TrackingFrame{Sequence: 2, TimestampNS: 0},
			regressing: trackingmodel.TrackingFrame{Sequence: 3, TimestampNS: 99},
			equal:      trackingmodel.TrackingFrame{Sequence: 3, TimestampNS: 100},
			wantErr:    ErrTimestampRegression,
		},
		{
			name:       "source clock",
			initial:    trackingmodel.TrackingFrame{Sequence: 1, SourceClockNS: 100},
			zero:       trackingmodel.TrackingFrame{Sequence: 2, SourceClockNS: 0},
			regressing: trackingmodel.TrackingFrame{Sequence: 3, SourceClockNS: 99},
			equal:      trackingmodel.TrackingFrame{Sequence: 3, SourceClockNS: 100},
			wantErr:    ErrSourceClockRegression,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServiceWithClock(func() int64 { return 10 })
			if err := s.SetGeneration(1); err != nil {
				t.Fatalf("SetGeneration(1) error = %v", err)
			}
			if err := s.Submit("osc", 1, tt.initial); err != nil {
				t.Fatalf("initial Submit() error = %v", err)
			}
			if err := s.Submit("osc", 1, tt.zero); err != nil {
				t.Fatalf("zero-baseline Submit() error = %v", err)
			}
			if err := s.Submit("osc", 1, tt.regressing); !errors.Is(err, tt.wantErr) {
				t.Fatalf("regressing Submit() error = %v, want %v", err, tt.wantErr)
			}
			if err := s.Submit("osc", 1, tt.equal); err != nil {
				t.Fatalf("equal-baseline Submit() error = %v", err)
			}
		})
	}
}

func TestSubmitRejectedFrameDoesNotMutateOrConsumeOrderingState(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 5, TimestampNS: 50, SourceClockNS: 60}); err != nil {
		t.Fatalf("Submit() setup error = %v", err)
	}
	beforeSource := s.sources["osc"]
	beforeMerged, _ := s.LatestMerged()
	beforeMergedSequence := s.mergedSequence
	beforeHostTime := s.lastHostTimeNS

	err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 100, TimestampNS: 49, SourceClockNS: 70})
	if !errors.Is(err, ErrTimestampRegression) {
		t.Fatalf("rejected Submit() error = %v, want ErrTimestampRegression", err)
	}
	if clockCalls != 2 {
		t.Fatalf("Host clock calls = %d, want 2", clockCalls)
	}
	if got := s.sources["osc"]; got != beforeSource {
		t.Fatalf("source after rejection = %#v, want unchanged %#v", got, beforeSource)
	}
	if got, ok := s.LatestMerged(); !ok || got != beforeMerged {
		t.Fatalf("LatestMerged() after rejection = (%#v, %t), want unchanged %#v", got, ok, beforeMerged)
	}
	if s.mergedSequence != beforeMergedSequence || s.lastHostTimeNS != beforeHostTime {
		t.Fatalf("merged ordering state after rejection = (sequence %d, time %d), want (%d, %d)", s.mergedSequence, s.lastHostTimeNS, beforeMergedSequence, beforeHostTime)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 6, TimestampNS: 50, SourceClockNS: 60}); err != nil {
		t.Fatalf("Submit() after rejected high sequence error = %v", err)
	}
	if clockCalls != 3 {
		t.Fatalf("Host clock calls after accepted Submit() = %d, want 3", clockCalls)
	}
}

func TestSubmitTracksOrderingIndependentlyPerPlugin(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 10 })
	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	if err := s.Submit("high", 1, trackingmodel.TrackingFrame{Sequence: 100, TimestampNS: 1000, SourceClockNS: 2000}); err != nil {
		t.Fatalf("Submit(high) error = %v", err)
	}
	if err := s.Submit("low", 1, trackingmodel.TrackingFrame{Sequence: 0, TimestampNS: 1, SourceClockNS: 2}); err != nil {
		t.Fatalf("Submit(low) error = %v", err)
	}
	if got := s.sources["high"].lastSequence; got != 100 {
		t.Fatalf("high lastSequence = %d, want 100", got)
	}
	if got := s.sources["low"].lastSequence; got != 0 {
		t.Fatalf("low lastSequence = %d, want 0", got)
	}
}

func TestSubmitGenerationAdvanceResetsOrderingBaselines(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 10 })
	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 100, TimestampNS: 1000, SourceClockNS: 2000}); err != nil {
		t.Fatalf("Submit() setup error = %v", err)
	}
	if err := s.SetGeneration(2); err != nil {
		t.Fatalf("SetGeneration(2) error = %v", err)
	}

	frame := trackingmodel.TrackingFrame{Sequence: 0, TimestampNS: 1, SourceClockNS: 2}
	if err := s.Submit("osc", 2, frame); err != nil {
		t.Fatalf("Submit() after generation advance error = %v", err)
	}
	if got := s.sources["osc"].frame; got != frame {
		t.Fatalf("stored frame = %#v, want %#v", got, frame)
	}
}

func TestSubmitStoresOwnedCanonicalFrameValue(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 10 })
	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	frame := trackingmodel.TrackingFrame{
		Sequence:     1,
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.25,
		},
	}
	if err := s.Submit("osc", 1, frame); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	frame.Sequence = 99
	frame.Eye.LeftOpenness = 0.75
	want := trackingmodel.TrackingFrame{
		Sequence:     1,
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.25,
		},
	}
	if got := s.sources["osc"].frame; got != want {
		t.Fatalf("stored frame after caller mutation = %#v, want %#v", got, want)
	}
}
