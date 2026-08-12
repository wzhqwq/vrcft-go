package tracking

import (
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestGenerationStartsUnset(t *testing.T) {
	s := newServiceWithClock(func() int64 {
		t.Fatal("an unset generation must not read the Host clock")
		return 0
	})

	if got := s.Generation(); got != 0 {
		t.Fatalf("Generation() = %d, want 0", got)
	}
	if got, ok := s.LatestMerged(); ok || got != (MergedFrame{}) {
		t.Fatalf("LatestMerged() = (%#v, %t), want (zero, false)", got, ok)
	}
	if s.sources == nil {
		t.Fatal("newServiceWithClock() sources map is nil, want initialized map")
	}
	if got := s.routing; got != (RoutingConfig{Eye: SourceSelection{Auto: true}, Expression: SourceSelection{Auto: true}, Lip: SourceSelection{Auto: true}}) {
		t.Fatalf("newServiceWithClock() routing = %#v, want both groups automatic", got)
	}
}

func TestGenerationNewServiceInitializesState(t *testing.T) {
	s := NewService()
	if s == nil {
		t.Fatal("NewService() = nil")
	}
	if got := s.Generation(); got != 0 {
		t.Fatalf("Generation() = %d, want 0", got)
	}
	if _, ok := s.LatestMerged(); ok {
		t.Fatal("LatestMerged() ok = true before generation is set")
	}
}

func TestSetGenerationRejectsZeroWithoutMutation(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return 20
	})

	err := s.SetGeneration(0)
	if !errors.Is(err, ErrGenerationZero) {
		t.Fatalf("SetGeneration(0) error = %v, want ErrGenerationZero", err)
	}
	if clockCalls != 0 {
		t.Fatalf("Host clock calls = %d, want 0", clockCalls)
	}
	if got := s.Generation(); got != 0 {
		t.Fatalf("Generation() = %d, want 0", got)
	}
	if _, ok := s.LatestMerged(); ok {
		t.Fatal("LatestMerged() ok = true after rejected generation")
	}
}

func TestSetGenerationAdvancesAndPublishesEmptyFrame(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return 50
	})

	if err := s.SetGeneration(7); err != nil {
		t.Fatalf("SetGeneration(7) error = %v", err)
	}
	if clockCalls != 1 {
		t.Fatalf("Host clock calls = %d, want 1", clockCalls)
	}
	if got := s.Generation(); got != 7 {
		t.Fatalf("Generation() = %d, want 7", got)
	}
	want := MergedFrame{Generation: 7, Sequence: 1, UpdatedAtNS: 50}
	if got, ok := s.LatestMerged(); !ok || got != want {
		t.Fatalf("LatestMerged() = (%#v, %t), want (%#v, true)", got, ok, want)
	}
}

func TestSetGenerationRejectsRegressionWithoutMutation(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	if err := s.SetGeneration(4); err != nil {
		t.Fatalf("SetGeneration(4) error = %v", err)
	}
	before, _ := s.LatestMerged()

	err := s.SetGeneration(3)
	if !errors.Is(err, ErrGenerationRegression) {
		t.Fatalf("SetGeneration(3) error = %v, want ErrGenerationRegression", err)
	}
	if clockCalls != 1 {
		t.Fatalf("Host clock calls = %d, want 1", clockCalls)
	}
	if got := s.Generation(); got != 4 {
		t.Fatalf("Generation() = %d, want 4", got)
	}
	if got, ok := s.LatestMerged(); !ok || got != before {
		t.Fatalf("LatestMerged() = (%#v, %t), want unchanged %#v", got, ok, before)
	}
}

func TestSetGenerationSameValueIsIdempotent(t *testing.T) {
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		clockCalls++
		return int64(clockCalls * 10)
	})
	if err := s.SetGeneration(5); err != nil {
		t.Fatalf("SetGeneration(5) error = %v", err)
	}
	if err := s.Submit("osc", 5, trackingmodel.TrackingFrame{Sequence: 3}); err != nil {
		t.Fatalf("Submit() setup error = %v", err)
	}
	before, _ := s.LatestMerged()

	if err := s.SetGeneration(5); err != nil {
		t.Fatalf("SetGeneration(5) idempotent error = %v", err)
	}
	if clockCalls != 2 {
		t.Fatalf("Host clock calls = %d, want 2", clockCalls)
	}
	if got, ok := s.LatestMerged(); !ok || got != before {
		t.Fatalf("LatestMerged() = (%#v, %t), want unchanged %#v", got, ok, before)
	}
	if err := s.Submit("osc", 5, trackingmodel.TrackingFrame{Sequence: 3}); !errors.Is(err, ErrSequenceNotIncreasing) {
		t.Fatalf("Submit() duplicate after idempotent SetGeneration error = %v, want ErrSequenceNotIncreasing", err)
	}
	if clockCalls != 2 {
		t.Fatalf("Host clock calls after rejected duplicate = %d, want 2", clockCalls)
	}
}

func TestSetGenerationSaturatesMergedSequence(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 80 })
	s.mergedSequence = math.MaxUint64

	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	got, ok := s.LatestMerged()
	if !ok {
		t.Fatal("LatestMerged() ok = false")
	}
	if got.Sequence != math.MaxUint64 {
		t.Fatalf("LatestMerged().Sequence = %d, want MaxUint64", got.Sequence)
	}
	if s.mergedSequence != math.MaxUint64 {
		t.Fatalf("mergedSequence = %d, want MaxUint64", s.mergedSequence)
	}
}

func TestSetGenerationClampsRegressingHostClock(t *testing.T) {
	times := []int64{100, 90}
	clockCalls := 0
	s := newServiceWithClock(func() int64 {
		got := times[clockCalls]
		clockCalls++
		return got
	})

	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	first, _ := s.LatestMerged()
	if err := s.SetGeneration(2); err != nil {
		t.Fatalf("SetGeneration(2) error = %v", err)
	}
	second, _ := s.LatestMerged()

	if first.UpdatedAtNS != 100 || second.UpdatedAtNS != 100 {
		t.Fatalf("UpdatedAtNS values = (%d, %d), want (100, 100)", first.UpdatedAtNS, second.UpdatedAtNS)
	}
	if second.Sequence != 2 {
		t.Fatalf("second Sequence = %d, want 2", second.Sequence)
	}
}

func TestGenerationAdvanceReplacesSources(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 100 })
	if err := s.SetGeneration(1); err != nil {
		t.Fatalf("SetGeneration(1) error = %v", err)
	}
	if err := s.Submit("osc", 1, trackingmodel.TrackingFrame{Sequence: 9}); err != nil {
		t.Fatalf("Submit() setup error = %v", err)
	}
	oldSources := s.sources

	if err := s.SetGeneration(2); err != nil {
		t.Fatalf("SetGeneration(2) error = %v", err)
	}
	if len(s.sources) != 0 {
		t.Fatalf("new generation sources length = %d, want 0", len(s.sources))
	}
	if reflect.ValueOf(oldSources).Pointer() == reflect.ValueOf(s.sources).Pointer() {
		t.Fatal("SetGeneration() retained the old sources map")
	}
}

func TestGenerationAndLatestMergedAreSafeForConcurrentAccess(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 100 })
	const generations = 100

	start := make(chan struct{})
	done := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for {
				select {
				case <-done:
					return
				default:
					s.Generation()
					s.LatestMerged()
				}
			}
		}()
	}
	close(start)
	for generation := uint64(1); generation <= generations; generation++ {
		if err := s.SetGeneration(generation); err != nil {
			t.Fatalf("SetGeneration(%d) error = %v", generation, err)
		}
	}
	close(done)
	readers.Wait()

	if got := s.Generation(); got != generations {
		t.Fatalf("Generation() = %d, want %d", got, generations)
	}
}

func TestLatestMergedReturnsValueCopy(t *testing.T) {
	s := newServiceWithClock(func() int64 { return 101 })
	if err := s.SetGeneration(3); err != nil {
		t.Fatalf("SetGeneration(3) error = %v", err)
	}

	first, ok := s.LatestMerged()
	if !ok {
		t.Fatal("LatestMerged() ok = false")
	}
	first.Generation = 99
	first.Eye.LeftOpenness = 0.75
	first.Expressions.Values[trackingmodel.ExpressionJawOpen] = 0.5
	first.EyeSourceID = "mutated"
	first.ExpressionSourceID = "mutated"

	want := MergedFrame{Generation: 3, Sequence: 1, UpdatedAtNS: 101}
	if got, ok := s.LatestMerged(); !ok || got != want {
		t.Fatalf("LatestMerged() after caller mutation = (%#v, %t), want (%#v, true)", got, ok, want)
	}
}
