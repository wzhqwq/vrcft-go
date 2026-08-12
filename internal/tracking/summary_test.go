package tracking

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestSummaryRejectionReasonValuesAreStable(t *testing.T) {
	// Mutation target: reordering a public rejection reason changes its fixed numeric value.
	got := []RejectionReason{
		RejectionNone,
		RejectionGenerationUnset,
		RejectionGenerationZero,
		RejectionStaleGeneration,
		RejectionFutureGeneration,
		RejectionInvalidPluginID,
		RejectionInvalidFrame,
		RejectionSequenceNotIncreasing,
		RejectionTimestampRegression,
		RejectionSourceClockRegression,
	}
	want := []RejectionReason{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("rejection reason at index %d = %d, want %d", index, got[index], want[index])
		}
	}
}

func TestSummaryClassifiesEverySubmitRejectionExactlyOnce(t *testing.T) {
	tests := []struct {
		name       string
		pluginID   string
		generation uint64
		frame      trackingmodel.TrackingFrame
		setup      func(*testing.T, *service)
		wantErr    error
		wantReason RejectionReason
		wantCounts RejectionCounts
	}{
		{
			name:       "generation unset wins over submitted zero",
			pluginID:   "vendor.eye",
			generation: 0,
			wantErr:    ErrGenerationUnset,
			wantReason: RejectionGenerationUnset,
			wantCounts: RejectionCounts{GenerationUnset: 1},
		},
		{
			name:       "generation zero",
			pluginID:   "vendor.eye",
			generation: 0,
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
			},
			wantErr:    ErrGenerationZero,
			wantReason: RejectionGenerationZero,
			wantCounts: RejectionCounts{GenerationZero: 1},
		},
		{
			name:       "stale generation",
			pluginID:   "vendor.eye",
			generation: 1,
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
			},
			wantErr:    ErrStaleGeneration,
			wantReason: RejectionStaleGeneration,
			wantCounts: RejectionCounts{StaleGeneration: 1},
		},
		{
			name:       "future generation",
			pluginID:   "vendor.eye",
			generation: 3,
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
			},
			wantErr:    ErrFutureGeneration,
			wantReason: RejectionFutureGeneration,
			wantCounts: RejectionCounts{FutureGeneration: 1},
		},
		{
			name:       "invalid plugin ID",
			generation: 2,
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
			},
			wantErr:    ErrInvalidPluginID,
			wantReason: RejectionInvalidPluginID,
			wantCounts: RejectionCounts{InvalidPluginID: 1},
		},
		{
			name:       "canonicalization failure",
			pluginID:   "vendor.eye",
			generation: 2,
			frame: trackingmodel.TrackingFrame{
				Capabilities: trackingmodel.Capability(1 << 31),
			},
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
			},
			wantErr:    ErrInvalidFrame,
			wantReason: RejectionInvalidFrame,
			wantCounts: RejectionCounts{InvalidFrame: 1},
		},
		{
			name:       "negative timestamp",
			pluginID:   "vendor.eye",
			generation: 2,
			frame: trackingmodel.TrackingFrame{
				Sequence:    1,
				TimestampNS: -1,
			},
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
			},
			wantErr:    ErrInvalidFrame,
			wantReason: RejectionInvalidFrame,
			wantCounts: RejectionCounts{InvalidFrame: 1},
		},
		{
			name:       "sequence not increasing",
			pluginID:   "vendor.eye",
			generation: 2,
			frame:      trackingmodel.TrackingFrame{Sequence: 1},
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
				mustSubmit(t, service, "vendor.eye", 2, trackingmodel.TrackingFrame{Sequence: 1})
			},
			wantErr:    ErrSequenceNotIncreasing,
			wantReason: RejectionSequenceNotIncreasing,
			wantCounts: RejectionCounts{SequenceNotIncreasing: 1},
		},
		{
			name:       "timestamp regression",
			pluginID:   "vendor.eye",
			generation: 2,
			frame: trackingmodel.TrackingFrame{
				Sequence:    2,
				TimestampNS: 9,
			},
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
				mustSubmit(t, service, "vendor.eye", 2, trackingmodel.TrackingFrame{Sequence: 1, TimestampNS: 10})
			},
			wantErr:    ErrTimestampRegression,
			wantReason: RejectionTimestampRegression,
			wantCounts: RejectionCounts{TimestampRegression: 1},
		},
		{
			name:       "source clock regression",
			pluginID:   "vendor.eye",
			generation: 2,
			frame: trackingmodel.TrackingFrame{
				Sequence:      2,
				SourceClockNS: 9,
			},
			setup: func(t *testing.T, service *service) {
				mustSetGeneration(t, service, 2)
				mustSubmit(t, service, "vendor.eye", 2, trackingmodel.TrackingFrame{Sequence: 1, SourceClockNS: 10})
			},
			wantErr:    ErrSourceClockRegression,
			wantReason: RejectionSourceClockRegression,
			wantCounts: RejectionCounts{SourceClockRegression: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mutation target: mapping this Submit branch to zero or the wrong reason.
			service := newServiceWithClock(func() int64 { return 10 })
			if tt.setup != nil {
				tt.setup(t, service)
			}

			err := service.Submit(tt.pluginID, tt.generation, tt.frame)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Submit() error = %v, want errors.Is(error, %v)", err, tt.wantErr)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			summary := receiveSummary(t, service.SubscribeSummary(ctx))
			if summary.RejectedFrames != 1 {
				t.Fatalf("RejectedFrames = %d, want 1", summary.RejectedFrames)
			}
			if summary.Rejected != tt.wantCounts {
				t.Fatalf("Rejected = %+v, want %+v", summary.Rejected, tt.wantCounts)
			}
			if totalRejectionCounts(summary.Rejected) != 1 {
				t.Fatalf("sum of rejection reasons = %d, want exactly 1", totalRejectionCounts(summary.Rejected))
			}
			wantLast := Rejection{PluginID: tt.pluginID, Generation: tt.generation, Reason: tt.wantReason}
			if summary.LastRejection != wantLast {
				t.Fatalf("LastRejection = %+v, want %+v", summary.LastRejection, wantLast)
			}
		})
	}
}

func TestSummaryAcceptedFrameDoesNotClearLastRejection(t *testing.T) {
	// Mutation target: clearing LastRejection on a successful Submit.
	service := newServiceWithClock(func() int64 { return 20 })
	if err := service.Submit("vendor.eye", 0, trackingmodel.TrackingFrame{}); !errors.Is(err, ErrGenerationUnset) {
		t.Fatalf("unset Submit() error = %v, want ErrGenerationUnset", err)
	}
	mustSetGeneration(t, service, 1)
	mustSubmit(t, service, "vendor.eye", 1, trackingmodel.TrackingFrame{Sequence: 1})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	summary := receiveSummary(t, service.SubscribeSummary(ctx))
	if summary.AcceptedFrames != 1 || summary.RejectedFrames != 1 {
		t.Fatalf("frame totals = (%d accepted, %d rejected), want (1, 1)", summary.AcceptedFrames, summary.RejectedFrames)
	}
	want := Rejection{PluginID: "vendor.eye", Generation: 0, Reason: RejectionGenerationUnset}
	if summary.LastRejection != want {
		t.Fatalf("LastRejection = %+v, want stable %+v", summary.LastRejection, want)
	}
}

func TestSaturatingAddDoesNotWrap(t *testing.T) {
	// Mutation target: ordinary uint64 addition wraps diagnostics at MaxUint64.
	tests := []struct {
		left  uint64
		right uint64
		want  uint64
	}{
		{left: 7, right: 9, want: 16},
		{left: math.MaxUint64, right: 0, want: math.MaxUint64},
		{left: math.MaxUint64, right: 1, want: math.MaxUint64},
		{left: math.MaxUint64 - 1, right: 2, want: math.MaxUint64},
	}
	for _, tt := range tests {
		if got := saturatingAdd(tt.left, tt.right); got != tt.want {
			t.Fatalf("saturatingAdd(%d, %d) = %d, want %d", tt.left, tt.right, got, tt.want)
		}
	}
}

func TestSummaryTotalsAndPerReasonSaturateAtMaxUint64(t *testing.T) {
	// Mutation target: accepted, rejected, or per-reason counters wrapping independently.
	service := newServiceWithClock(func() int64 { return 30 })
	mustSetGeneration(t, service, 1)
	service.acceptedFrames = math.MaxUint64
	service.rejectedFrames = math.MaxUint64
	service.rejected.InvalidPluginID = math.MaxUint64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeSummary(ctx)
	_ = receiveSummary(t, updates)

	mustSubmit(t, service, "vendor.eye", 1, trackingmodel.TrackingFrame{Sequence: 1})
	afterAccepted := receiveSummary(t, updates)
	if afterAccepted.AcceptedFrames != math.MaxUint64 {
		t.Fatalf("AcceptedFrames = %d, want MaxUint64", afterAccepted.AcceptedFrames)
	}
	if err := service.Submit("", 1, trackingmodel.TrackingFrame{Sequence: 2}); !errors.Is(err, ErrInvalidPluginID) {
		t.Fatalf("invalid plugin Submit() error = %v, want ErrInvalidPluginID", err)
	}
	afterRejected := receiveSummary(t, updates)
	if afterRejected.RejectedFrames != math.MaxUint64 || afterRejected.Rejected.InvalidPluginID != math.MaxUint64 {
		t.Fatalf("saturated rejection counters = (%d total, %d invalid plugin), want MaxUint64 for both",
			afterRejected.RejectedFrames, afterRejected.Rejected.InvalidPluginID)
	}
}

func TestSummaryGenerationAdvancePreservesDiagnosticsAndClearsSources(t *testing.T) {
	// Mutation target: generation advance resetting lifetime counters or retaining current-generation sources.
	service := newServiceWithClock(func() int64 { return 40 })
	if err := service.Submit("vendor.eye", 0, trackingmodel.TrackingFrame{}); !errors.Is(err, ErrGenerationUnset) {
		t.Fatalf("unset Submit() error = %v, want ErrGenerationUnset", err)
	}
	mustSetGeneration(t, service, 1)
	mustSubmit(t, service, "vendor.eye", 1, eyeFrame(1, 0.5))
	mustSubmit(t, service, "vendor.expression", 1, expressionFrame(1, trackingmodel.ExpressionJawOpen, 0.6))
	mustSetGeneration(t, service, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	summary := receiveSummary(t, service.SubscribeSummary(ctx))
	wantLast := Rejection{PluginID: "vendor.eye", Generation: 0, Reason: RejectionGenerationUnset}
	if summary.Generation != 2 || summary.SourceCount != 0 {
		t.Fatalf("generation/source count = (%d, %d), want (2, 0)", summary.Generation, summary.SourceCount)
	}
	if summary.EyeSourceID != "" || summary.EyeAvailable || summary.ExpressionSourceID != "" || summary.ExpressionAvailable || summary.LipSourceID != "" || summary.LipAvailable {
		t.Fatalf("availability after generation advance = %+v, want all unavailable", summary)
	}
	if summary.AcceptedFrames != 2 || summary.RejectedFrames != 1 || summary.Rejected.GenerationUnset != 1 || summary.LastRejection != wantLast {
		t.Fatalf("diagnostics after generation advance = %+v, want lifetime counters and LastRejection preserved", summary)
	}
}

func TestSummaryCapabilityAvailabilityIgnoresEmptyValidityMasks(t *testing.T) {
	// Mutation target: deriving availability from sample validity masks instead of declared capabilities.
	service := newServiceWithClock(func() int64 { return 50 })
	mustSetGeneration(t, service, 1)
	mustSubmit(t, service, "vendor.both", 1, trackingmodel.TrackingFrame{
		Sequence:     1,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	summary := receiveSummary(t, service.SubscribeSummary(ctx))
	if summary.EyeSourceID != "vendor.both" || !summary.EyeAvailable {
		t.Fatalf("Eye summary = (%q, %t), want vendor.both available", summary.EyeSourceID, summary.EyeAvailable)
	}
	if summary.ExpressionSourceID != "vendor.both" || !summary.ExpressionAvailable {
		t.Fatalf("Expression summary = (%q, %t), want vendor.both available", summary.ExpressionSourceID, summary.ExpressionAvailable)
	}
	if summary.LipSourceID != "vendor.both" || !summary.LipAvailable {
		t.Fatalf("Lip summary = (%q, %t), want vendor.both available", summary.LipSourceID, summary.LipAvailable)
	}
}

func TestSummarySubscriberReceivesInitialAndFrameUpdates(t *testing.T) {
	// Mutation target: omitting the immediate Summary or accepted/rejected Submit publications.
	service := newServiceWithClock(func() int64 { return 60 })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeSummary(ctx)
	if cap(updates) != 1 {
		t.Fatalf("SubscribeSummary() channel capacity = %d, want 1", cap(updates))
	}
	initial := receiveSummary(t, updates)
	wantRouting := RoutingConfig{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}, Lip: SourceSelection{Auto: true}}
	if initial != (Summary{Routing: wantRouting}) {
		t.Fatalf("initial Summary = %+v, want only default routing %+v", initial, wantRouting)
	}

	mustSetGeneration(t, service, 3)
	afterGeneration := receiveSummary(t, updates)
	if afterGeneration.Generation != 3 || afterGeneration.Routing != wantRouting {
		t.Fatalf("generation Summary = %+v, want generation 3 and default routing", afterGeneration)
	}
	mustSubmit(t, service, "vendor.eye", 3, eyeFrame(1, 0.25))
	afterAccepted := receiveSummary(t, updates)
	if afterAccepted.AcceptedFrames != 1 || afterAccepted.SourceCount != 1 || afterAccepted.EyeSourceID != "vendor.eye" || !afterAccepted.EyeAvailable {
		t.Fatalf("accepted Summary = %+v, want accepted selected eye source", afterAccepted)
	}
	if err := service.Submit("vendor.eye", 3, eyeFrame(1, 0.5)); !errors.Is(err, ErrSequenceNotIncreasing) {
		t.Fatalf("duplicate Submit() error = %v, want ErrSequenceNotIncreasing", err)
	}
	afterRejected := receiveSummary(t, updates)
	if afterRejected.AcceptedFrames != 1 || afterRejected.RejectedFrames != 1 || afterRejected.Rejected.SequenceNotIncreasing != 1 {
		t.Fatalf("rejected Summary = %+v, want one accepted and one sequence rejection", afterRejected)
	}
}

func TestSummarySubscriberReceivesChangedControlsAndExistingRemoval(t *testing.T) {
	// Mutation target: failing to publish changed routing/generation or a removed non-selected source.
	service := newServiceWithClock(func() int64 { return 70 })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeSummary(ctx)
	_ = receiveSummary(t, updates)

	manualMissing := RoutingConfig{
		Eye:        SourceSelection{PluginID: "missing.eye"},
		Expression: SourceSelection{PluginID: "missing.expression"},
		Lip:        SourceSelection{PluginID: "missing.lip"},
	}
	if err := service.SetRouting(manualMissing); err != nil {
		t.Fatalf("pre-generation SetRouting() error = %v", err)
	}
	if got := receiveSummary(t, updates); got.Generation != 0 || got.Routing != manualMissing {
		t.Fatalf("pre-generation routing Summary = %+v, want generation 0 and changed routing", got)
	}
	mustSetGeneration(t, service, 1)
	if got := receiveSummary(t, updates); got.Generation != 1 || got.Routing != manualMissing {
		t.Fatalf("generation Summary = %+v, want generation 1 and retained routing", got)
	}

	mustSubmit(t, service, "vendor.z", 1, eyeFrame(1, 0.2))
	_ = receiveSummary(t, updates)
	mustSubmit(t, service, "vendor.a", 1, eyeFrame(1, 0.4))
	if got := receiveSummary(t, updates); got.SourceCount != 2 {
		t.Fatalf("two-source Summary SourceCount = %d, want 2", got.SourceCount)
	}
	service.RemoveSource("vendor.a")
	if got := receiveSummary(t, updates); got.SourceCount != 1 {
		t.Fatalf("non-selected removal Summary SourceCount = %d, want 1", got.SourceCount)
	}

	auto := RoutingConfig{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}, Lip: SourceSelection{Auto: true}}
	if err := service.SetRouting(auto); err != nil {
		t.Fatalf("post-generation SetRouting() error = %v", err)
	}
	if got := receiveSummary(t, updates); got.Routing != auto || got.EyeSourceID != "vendor.z" || !got.EyeAvailable {
		t.Fatalf("post-generation routing Summary = %+v, want auto-selected vendor.z", got)
	}
}

func TestSummarySubscriberIgnoresIdempotentInvalidAndUnknownControls(t *testing.T) {
	// Mutation target: publishing Summary for a control operation that leaves observable state unchanged.
	service := newServiceWithClock(func() int64 { return 80 })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeSummary(ctx)
	_ = receiveSummary(t, updates)

	auto := RoutingConfig{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}, Lip: SourceSelection{Auto: true}}
	if err := service.SetRouting(auto); err != nil {
		t.Fatalf("equal SetRouting() error = %v", err)
	}
	invalid := RoutingConfig{Eye: SourceSelection{Auto: true, PluginID: "invalid"}, Expression: SourceSelection{Auto: true}, Lip: SourceSelection{Auto: true}}
	if err := service.SetRouting(invalid); !errors.Is(err, ErrInvalidRouting) {
		t.Fatalf("invalid SetRouting() error = %v, want ErrInvalidRouting", err)
	}
	service.RemoveSource("")
	service.RemoveSource("unknown")
	if err := service.SetGeneration(0); !errors.Is(err, ErrGenerationZero) {
		t.Fatalf("SetGeneration(0) error = %v, want ErrGenerationZero", err)
	}
	assertNoSummary(t, updates)

	mustSetGeneration(t, service, 1)
	_ = receiveSummary(t, updates)
	if err := service.SetGeneration(1); err != nil {
		t.Fatalf("equal SetGeneration() error = %v", err)
	}
	if err := service.SetGeneration(0); !errors.Is(err, ErrGenerationZero) {
		t.Fatalf("regressing SetGeneration(0) error = %v, want ErrGenerationZero", err)
	}
	service.RemoveSource("still-unknown")
	assertNoSummary(t, updates)
}

func TestSummaryPublicConstructorReturnsFinalService(t *testing.T) {
	// Mutation target: returning a constructor value that lacks any final Service method.
	var service Service = NewService()
	if service == nil {
		t.Fatal("NewService() = nil")
	}
	if err := service.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	if service.Generation() != 1 {
		t.Fatalf("Generation() = %d, want 1", service.Generation())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if got := receiveSummary(t, service.SubscribeSummary(ctx)); got.Generation != 1 {
		t.Fatalf("constructor Summary Generation = %d, want 1", got.Generation)
	}
	if got := receiveMerged(t, service.SubscribeMerged(ctx)); got.Generation != 1 {
		t.Fatalf("constructor merged Generation = %d, want 1", got.Generation)
	}
}

func totalRejectionCounts(counts RejectionCounts) uint64 {
	return counts.GenerationUnset +
		counts.GenerationZero +
		counts.StaleGeneration +
		counts.FutureGeneration +
		counts.InvalidPluginID +
		counts.InvalidFrame +
		counts.SequenceNotIncreasing +
		counts.TimestampRegression +
		counts.SourceClockRegression
}
