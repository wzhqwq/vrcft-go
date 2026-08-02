package tracking

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestServiceConcurrentOperationsRemainConsistent(t *testing.T) {
	const (
		pluginCount       = 4
		framesPerPlugin   = 100
		finalGeneration   = 20
		subscriptionCount = 25
	)

	service := NewService()
	if err := service.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) setup error = %v", err)
	}

	pluginIDs := make([]string, pluginCount)
	knownPlugin := make(map[string]struct{}, pluginCount)
	for worker := range pluginCount {
		pluginID := fmt.Sprintf("vendor.%02d", worker)
		pluginIDs[worker] = pluginID
		knownPlugin[pluginID] = struct{}{}
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	var errorOnce sync.Once
	var workerError error
	recordError := func(err error) {
		if err != nil {
			errorOnce.Do(func() { workerError = err })
		}
	}

	for worker, pluginID := range pluginIDs {
		worker := worker
		pluginID := pluginID
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for sequence := uint64(1); sequence <= framesPerPlugin; sequence++ {
				generation := service.Generation()
				_ = service.Submit(pluginID, generation, concurrentTrackingFrame(sequence, worker))
			}
		}()
	}

	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		for generation := uint64(2); generation <= finalGeneration; generation++ {
			recordError(service.SetGeneration(generation))
		}
	}()

	routingConfigs := []RoutingConfig{
		{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}},
		{Eye: SourceSelection{PluginID: pluginIDs[0]}, Expression: SourceSelection{PluginID: pluginIDs[0]}},
		{Eye: SourceSelection{PluginID: "vendor.missing"}, Expression: SourceSelection{PluginID: "vendor.missing"}},
		{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{PluginID: pluginIDs[1]}},
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		for iteration := 0; iteration < 100; iteration++ {
			recordError(service.SetRouting(routingConfigs[iteration%len(routingConfigs)]))
		}
	}()

	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		removals := append(append([]string{}, pluginIDs...), "", "vendor.unknown")
		for iteration := 0; iteration < 100; iteration++ {
			service.RemoveSource(removals[iteration%len(removals)])
		}
	}()

	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		for range 250 {
			_, _ = service.LatestMerged()
			_ = service.Routing()
			_ = service.Generation()
		}
	}()

	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		for range subscriptionCount {
			ctx, cancel := context.WithCancel(context.Background())
			merged := service.SubscribeMerged(ctx)
			summaries := service.SubscribeSummary(ctx)
			cancel()

			closeCtx, stopWaiting := context.WithTimeout(context.Background(), 5*time.Second)
			if err := waitForConcurrentChannelClose(closeCtx, merged); err != nil {
				recordError(fmt.Errorf("merged subscription did not close: %w", err))
			}
			if err := waitForConcurrentChannelClose(closeCtx, summaries); err != nil {
				recordError(fmt.Errorf("Summary subscription did not close: %w", err))
			}
			stopWaiting()
		}
	}()

	close(start)
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelWait()
	if err := waitForConcurrentWorkers(waitCtx, &workers); err != nil {
		t.Fatal(err)
	}
	if workerError != nil {
		t.Fatal(workerError)
	}

	if got := service.Generation(); got != finalGeneration {
		t.Fatalf("Generation() = %d, want %d", got, finalGeneration)
	}
	merged, ok := service.LatestMerged()
	if !ok {
		t.Fatal("LatestMerged() ok = false after generation was set")
	}
	if merged.Generation != finalGeneration || merged.Sequence == 0 {
		t.Fatalf("LatestMerged() metadata = (generation %d, sequence %d), want generation %d and nonzero sequence", merged.Generation, merged.Sequence, finalGeneration)
	}

	summary := receiveConcurrentSummary(t, service)
	if summary.Generation != finalGeneration {
		t.Fatalf("Summary.Generation = %d, want %d", summary.Generation, finalGeneration)
	}
	if summary.Routing != service.Routing() {
		t.Fatalf("Summary.Routing = %#v, want current routing %#v", summary.Routing, service.Routing())
	}
	const attemptedSubmissions = uint64(pluginCount * framesPerPlugin)
	if got := summary.AcceptedFrames + summary.RejectedFrames; got != attemptedSubmissions {
		t.Fatalf("accounted submissions = %d accepted + %d rejected = %d, want %d", summary.AcceptedFrames, summary.RejectedFrames, got, attemptedSubmissions)
	}
	if got := totalConcurrentRejectionCounts(summary.Rejected); got != summary.RejectedFrames {
		t.Fatalf("rejection reason total = %d, want RejectedFrames %d", got, summary.RejectedFrames)
	}
	if summary.RejectedFrames != summary.Rejected.StaleGeneration {
		t.Fatalf("concurrent rejection counts = %+v, want only stale-generation rejections", summary.Rejected)
	}

	assertConcurrentMergedConsistency(t, merged, summary, knownPlugin)
}

func concurrentTrackingFrame(sequence uint64, worker int) trackingmodel.TrackingFrame {
	value := float32(worker+1) / 10
	frame := trackingmodel.TrackingFrame{
		Sequence:     sequence,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: value,
		},
	}
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, value)
	return frame
}

func waitForConcurrentWorkers(ctx context.Context, workers *sync.WaitGroup) error {
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for concurrent workers: %w", ctx.Err())
	}
}

func waitForConcurrentChannelClose[T any](ctx context.Context, updates <-chan T) error {
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func receiveConcurrentSummary(t *testing.T, service Service) Summary {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	updates := service.SubscribeSummary(ctx)
	receiveCtx, stopWaiting := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopWaiting()

	var summary Summary
	select {
	case got, ok := <-updates:
		if !ok {
			t.Fatal("Summary subscription closed before its immediate snapshot")
		}
		summary = got
	case <-receiveCtx.Done():
		t.Fatal("timed out waiting for immediate Summary snapshot")
	}

	cancel()
	if err := waitForConcurrentChannelClose(receiveCtx, updates); err != nil {
		t.Fatalf("Summary subscription did not close after cancellation: %v", err)
	}
	return summary
}

func totalConcurrentRejectionCounts(counts RejectionCounts) uint64 {
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

func assertConcurrentMergedConsistency(t *testing.T, merged MergedFrame, summary Summary, knownPlugin map[string]struct{}) {
	t.Helper()
	if merged.EyeSourceID != summary.EyeSourceID || merged.ExpressionSourceID != summary.ExpressionSourceID {
		t.Fatalf("merged sources = (%q, %q), Summary sources = (%q, %q)", merged.EyeSourceID, merged.ExpressionSourceID, summary.EyeSourceID, summary.ExpressionSourceID)
	}

	eyeAvailable := merged.Capabilities.Has(trackingmodel.CapabilityEye)
	if eyeAvailable != summary.EyeAvailable {
		t.Fatalf("merged Eye capability = %t, Summary EyeAvailable = %t", eyeAvailable, summary.EyeAvailable)
	}
	if eyeAvailable {
		if merged.EyeSourceID == "" || merged.Eye.Valid != trackingmodel.EyeValidLeftOpenness {
			t.Fatalf("available Eye group = (source %q, sample %#v), want known source and canonical validity", merged.EyeSourceID, merged.Eye)
		}
		if _, ok := knownPlugin[merged.EyeSourceID]; !ok {
			t.Fatalf("EyeSourceID = %q, want a submitted plugin", merged.EyeSourceID)
		}
	} else if merged.EyeSourceID != "" || merged.Eye != (trackingmodel.EyeSample{}) {
		t.Fatalf("unavailable Eye group retained source/data: source %q sample %#v", merged.EyeSourceID, merged.Eye)
	}

	expressionAvailable := merged.Capabilities.Has(trackingmodel.CapabilityExpression)
	if expressionAvailable != summary.ExpressionAvailable {
		t.Fatalf("merged Expression capability = %t, Summary ExpressionAvailable = %t", expressionAvailable, summary.ExpressionAvailable)
	}
	if expressionAvailable {
		if merged.ExpressionSourceID == "" || !merged.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
			t.Fatalf("available Expression group = (source %q, sample %#v), want known source and JawOpen validity", merged.ExpressionSourceID, merged.Expressions)
		}
		if _, ok := knownPlugin[merged.ExpressionSourceID]; !ok {
			t.Fatalf("ExpressionSourceID = %q, want a submitted plugin", merged.ExpressionSourceID)
		}
	} else if merged.ExpressionSourceID != "" || merged.Expressions != (trackingmodel.ExpressionSet{}) {
		t.Fatalf("unavailable Expression group retained source/data: source %q sample %#v", merged.ExpressionSourceID, merged.Expressions)
	}

	if (merged.EyeSourceID != "" || merged.ExpressionSourceID != "") && summary.SourceCount == 0 {
		t.Fatal("available merged source with Summary.SourceCount = 0")
	}
}
