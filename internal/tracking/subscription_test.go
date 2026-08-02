package tracking

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const subscriptionFailureTimeout = 2 * time.Second

func TestMergedSubscriberWaitsForGenerationThenReceivesSelectedFrame(t *testing.T) {
	// Mutation target: exposing a pre-generation merged value or omitting generation/frame publications.
	now := int64(99)
	service := newServiceWithClock(func() int64 {
		now++
		return now
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeMerged(ctx)
	if cap(updates) != 1 {
		t.Fatalf("SubscribeMerged() channel capacity = %d, want 1", cap(updates))
	}
	assertNoMerged(t, updates)

	mustSetGeneration(t, service, 1)
	if got := receiveMerged(t, updates); got != (MergedFrame{Generation: 1, Sequence: 1, UpdatedAtNS: 100}) {
		t.Fatalf("generation merged snapshot = %#v, want empty generation 1 snapshot", got)
	}
	mustSubmit(t, service, "vendor.eye", 1, eyeFrame(1, 0.5))
	got := receiveMerged(t, updates)
	if got.Generation != 1 || got.Sequence != 2 || got.EyeSourceID != "vendor.eye" || got.Eye.LeftOpenness != 0.5 {
		t.Fatalf("selected merged frame = %#v, want generation 1 sequence 2 from vendor.eye", got)
	}
}

func TestMergedSubscriberReceivesLatestValueWithoutBlockingProducer(t *testing.T) {
	// Mutation target: blocking on a full subscriber or retaining the first unread value.
	service := newServiceWithClock(func() int64 { return 110 })
	mustSetGeneration(t, service, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeMerged(ctx)
	if cap(updates) != 1 {
		t.Fatalf("SubscribeMerged() channel capacity = %d, want 1", cap(updates))
	}

	done := make(chan error, 1)
	go func() {
		for sequence := uint64(1); sequence <= 100; sequence++ {
			if err := service.Submit("vendor.eye", 1, eyeFrame(sequence, float32(sequence))); err != nil {
				done <- fmt.Errorf("Submit(sequence %d): %w", sequence, err)
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(subscriptionFailureTimeout):
		t.Fatal("100 merged publications blocked on an unread subscriber")
	}

	latest := receiveMerged(t, updates)
	if latest.Sequence != 101 || latest.EyeSourceID != "vendor.eye" || latest.Eye.LeftOpenness != 100 {
		t.Fatalf("latest merged = %#v, want sequence 101 with vendor.eye value 100", latest)
	}
}

func TestSummarySubscriberReceivesLatestValueWithoutBlockingProducer(t *testing.T) {
	// Mutation target: using an unbounded/blocking Summary queue or failing latest-value replacement.
	service := newServiceWithClock(func() int64 { return 120 })
	mustSetGeneration(t, service, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := service.SubscribeSummary(ctx)
	if cap(updates) != 1 {
		t.Fatalf("SubscribeSummary() channel capacity = %d, want 1", cap(updates))
	}

	done := make(chan error, 1)
	go func() {
		for sequence := uint64(1); sequence <= 100; sequence++ {
			if err := service.Submit("vendor.eye", 1, eyeFrame(sequence, float32(sequence))); err != nil {
				done <- fmt.Errorf("Submit(sequence %d): %w", sequence, err)
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(subscriptionFailureTimeout):
		t.Fatal("100 Summary publications blocked on an unread subscriber")
	}

	latest := receiveSummary(t, updates)
	if latest.AcceptedFrames != 100 || latest.SourceCount != 1 || latest.EyeSourceID != "vendor.eye" || !latest.EyeAvailable {
		t.Fatalf("latest Summary = %+v, want 100 accepted frames and selected vendor.eye", latest)
	}
}

func TestSummarySubscriberGetsNonSelectedFrameWhileMergedSubscriberDoesNot(t *testing.T) {
	// Mutation target: publishing merged for a non-selected frame or omitting its Summary update.
	service := newServiceWithClock(func() int64 { return 130 })
	mustSetGeneration(t, service, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mergedUpdates := service.SubscribeMerged(ctx)
	summaryUpdates := service.SubscribeSummary(ctx)
	_ = receiveMerged(t, mergedUpdates)
	_ = receiveSummary(t, summaryUpdates)

	mustSubmit(t, service, "vendor.z", 1, eyeFrame(1, 0.25))
	_ = receiveMerged(t, mergedUpdates)
	_ = receiveSummary(t, summaryUpdates)
	mustSubmit(t, service, "vendor.a", 1, eyeFrame(1, 0.75))
	summary := receiveSummary(t, summaryUpdates)
	if summary.AcceptedFrames != 2 || summary.SourceCount != 2 || summary.EyeSourceID != "vendor.z" {
		t.Fatalf("non-selected Summary = %+v, want two accepted sources with vendor.z selected", summary)
	}
	assertNoMerged(t, mergedUpdates)
}

func TestMergedSubscriberAndSummarySubscriberMultipleSubscribersAreIsolated(t *testing.T) {
	// Mutation target: sharing one channel/buffer between subscribers so one reader steals another's update.
	service := newServiceWithClock(func() int64 { return 140 })
	mustSetGeneration(t, service, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mergedA := service.SubscribeMerged(ctx)
	mergedB := service.SubscribeMerged(ctx)
	summaryA := service.SubscribeSummary(ctx)
	summaryB := service.SubscribeSummary(ctx)
	_ = receiveMerged(t, mergedA)
	_ = receiveMerged(t, mergedB)
	_ = receiveSummary(t, summaryA)
	_ = receiveSummary(t, summaryB)

	mustSubmit(t, service, "vendor.eye", 1, eyeFrame(1, 0.25))
	if got := receiveMerged(t, mergedB); got.Sequence != 2 {
		t.Fatalf("subscriber B first merged Sequence = %d, want 2", got.Sequence)
	}
	if got := receiveSummary(t, summaryB); got.AcceptedFrames != 1 {
		t.Fatalf("subscriber B first AcceptedFrames = %d, want 1", got.AcceptedFrames)
	}
	mustSubmit(t, service, "vendor.eye", 1, eyeFrame(2, 0.75))

	for name, updates := range map[string]<-chan MergedFrame{"A": mergedA, "B": mergedB} {
		got := receiveMerged(t, updates)
		if got.Sequence != 3 || got.Eye.LeftOpenness != 0.75 {
			t.Fatalf("merged subscriber %s latest = %#v, want sequence 3 value 0.75", name, got)
		}
	}
	for name, updates := range map[string]<-chan Summary{"A": summaryA, "B": summaryB} {
		got := receiveSummary(t, updates)
		if got.AcceptedFrames != 2 {
			t.Fatalf("Summary subscriber %s AcceptedFrames = %d, want 2", name, got.AcceptedFrames)
		}
	}
}

func TestSubscriberCancellationPreCanceledContextsReturnClosedWithoutRegistryLeak(t *testing.T) {
	// Mutation target: registering a pre-canceled context or returning a zero-capacity/open channel.
	service := newServiceWithClock(func() int64 { return 150 })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	merged := service.SubscribeMerged(ctx)
	summary := service.SubscribeSummary(ctx)
	if cap(merged) != 1 || cap(summary) != 1 {
		t.Fatalf("pre-canceled capacities = (%d merged, %d Summary), want (1, 1)", cap(merged), cap(summary))
	}
	if _, ok := <-merged; ok {
		t.Fatal("pre-canceled merged channel is open")
	}
	if _, ok := <-summary; ok {
		t.Fatal("pre-canceled Summary channel is open")
	}

	service.mu.Lock()
	mergedCount := len(service.mergedSubscribers)
	summaryCount := len(service.summarySubscribers)
	service.mu.Unlock()
	if mergedCount != 0 || summaryCount != 0 {
		t.Fatalf("pre-canceled registry sizes = (%d merged, %d Summary), want (0, 0)", mergedCount, summaryCount)
	}
}

func TestSubscriberCancellationClosesRegisteredChannels(t *testing.T) {
	// Mutation target: canceling without atomically deleting and closing the exact registered channel.
	service := newServiceWithClock(func() int64 { return 160 })
	mergedCtx, cancelMerged := context.WithCancel(context.Background())
	summaryCtx, cancelSummary := context.WithCancel(context.Background())
	merged := service.SubscribeMerged(mergedCtx)
	summary := service.SubscribeSummary(summaryCtx)
	_ = receiveSummary(t, summary)
	cancelMerged()
	cancelSummary()
	waitClosed(t, merged)
	waitClosed(t, summary)

	service.mu.Lock()
	mergedCount := len(service.mergedSubscribers)
	summaryCount := len(service.summarySubscribers)
	service.mu.Unlock()
	if mergedCount != 0 || summaryCount != 0 {
		t.Fatalf("canceled registry sizes = (%d merged, %d Summary), want (0, 0)", mergedCount, summaryCount)
	}
}

func TestSubscriberCancellationConcurrentPublicationIsSafe(t *testing.T) {
	// Mutation target: publishing and closing outside the same mutex, causing send/close races or panics.
	service := newServiceWithClock(func() int64 { return 170 })
	const subscriberCount = 8
	cancels := make([]context.CancelFunc, 0, subscriberCount)
	mergedChannels := make([]<-chan MergedFrame, 0, subscriberCount)
	summaryChannels := make([]<-chan Summary, 0, subscriberCount)
	for range subscriberCount {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		mergedChannels = append(mergedChannels, service.SubscribeMerged(ctx))
		summaryChannels = append(summaryChannels, service.SubscribeSummary(ctx))
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	publicationErrors := make(chan error, 1)
	go func() {
		defer workers.Done()
		<-start
		for generation := uint64(1); generation <= 100; generation++ {
			if err := service.SetGeneration(generation); err != nil {
				publicationErrors <- fmt.Errorf("SetGeneration(%d): %w", generation, err)
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for _, cancel := range cancels {
			cancel()
		}
	}()
	close(start)
	waitGroup(t, &workers)
	select {
	case err := <-publicationErrors:
		t.Fatal(err)
	default:
	}

	for _, updates := range mergedChannels {
		waitClosed(t, updates)
	}
	for _, updates := range summaryChannels {
		waitClosed(t, updates)
	}
	service.mu.Lock()
	mergedCount := len(service.mergedSubscribers)
	summaryCount := len(service.summarySubscribers)
	service.mu.Unlock()
	if mergedCount != 0 || summaryCount != 0 {
		t.Fatalf("post-race registry sizes = (%d merged, %d Summary), want (0, 0)", mergedCount, summaryCount)
	}
}

func TestSummarySubscriberSnapshotCannotMutateServiceState(t *testing.T) {
	// Mutation target: exposing pointers or reference-bearing mutable Summary state.
	service := newServiceWithClock(func() int64 { return 180 })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := receiveSummary(t, service.SubscribeSummary(ctx))
	first.Generation = 99
	first.Routing.Eye = SourceSelection{PluginID: "mutated"}
	first.EyeSourceID = "mutated"
	first.LastRejection.PluginID = "mutated"

	second := receiveSummary(t, service.SubscribeSummary(ctx))
	wantRouting := RoutingConfig{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}}
	if second != (Summary{Routing: wantRouting}) {
		t.Fatalf("Summary after caller mutation = %+v, want unchanged initial Summary", second)
	}
}

func TestMergedSubscriberSnapshotCannotMutateServiceState(t *testing.T) {
	// Mutation target: retaining a caller-mutable reference in a published MergedFrame.
	service := newServiceWithClock(func() int64 { return 190 })
	mustSetGeneration(t, service, 1)
	mustSubmit(t, service, "vendor.expression", 1, expressionFrame(1, trackingmodel.ExpressionJawOpen, 0.75))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := receiveMerged(t, service.SubscribeMerged(ctx))
	want := first
	first.Generation = 99
	first.Expressions.Values[trackingmodel.ExpressionJawOpen] = 0.1
	first.ExpressionSourceID = "mutated"

	second := receiveMerged(t, service.SubscribeMerged(ctx))
	if second != want {
		t.Fatalf("merged snapshot after caller mutation = %#v, want unchanged %#v", second, want)
	}
}

func receiveSummary(t *testing.T, updates <-chan Summary) Summary {
	t.Helper()
	select {
	case summary, ok := <-updates:
		if !ok {
			t.Fatal("Summary channel closed before expected value")
		}
		return summary
	case <-time.After(subscriptionFailureTimeout):
		t.Fatal("timed out waiting for Summary")
		return Summary{}
	}
}

func receiveMerged(t *testing.T, updates <-chan MergedFrame) MergedFrame {
	t.Helper()
	select {
	case merged, ok := <-updates:
		if !ok {
			t.Fatal("merged channel closed before expected value")
		}
		return merged
	case <-time.After(subscriptionFailureTimeout):
		t.Fatal("timed out waiting for merged frame")
		return MergedFrame{}
	}
}

func assertNoSummary(t *testing.T, updates <-chan Summary) {
	t.Helper()
	select {
	case summary, ok := <-updates:
		t.Fatalf("unexpected Summary update (%+v, open=%t)", summary, ok)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertNoMerged(t *testing.T, updates <-chan MergedFrame) {
	t.Helper()
	select {
	case merged, ok := <-updates:
		t.Fatalf("unexpected merged update (%+v, open=%t)", merged, ok)
	case <-time.After(20 * time.Millisecond):
	}
}

func waitClosed[T any](t *testing.T, updates <-chan T) {
	t.Helper()
	timer := time.NewTimer(subscriptionFailureTimeout)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("timed out waiting for subscriber channel closure")
		}
	}
}

func waitGroup(t *testing.T, group *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(subscriptionFailureTimeout):
		t.Fatal("timed out waiting for concurrent publisher/canceler")
	}
}
