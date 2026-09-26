package steamlink

import (
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestStateExpiryPublishesOneInvalidation(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression})
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start)
	first, ok := s.next(start.Add(time.Millisecond))
	if !ok || !first.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
		t.Fatal("missing fresh frame")
	}
	if _, ok := s.next(start.Add(249 * time.Millisecond)); ok {
		t.Fatal("cache republished")
	}
	expired, ok := s.next(start.Add(250 * time.Millisecond))
	if !ok || !expired.Expressions.Valid.IsZero() {
		t.Fatal("missing expiration frame")
	}
	if _, ok := s.next(start.Add(260 * time.Millisecond)); ok {
		t.Fatal("invalidation repeated")
	}
}

func TestStateExpiryDoesNotExtendOtherFields(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, pluginapi.Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye:          trackingmodel.EyeValidLeftGaze,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	})
	s.observe([]observation{
		{ID: rawGazePoint, Values: [3]float32{0, 0, -1}},
		{ID: rawJawDrop, Values: [3]float32{0.4}},
	}, start)
	if _, ok := s.next(start.Add(time.Millisecond)); !ok {
		t.Fatal("missing initial frame")
	}
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.4}}}, start.Add(200*time.Millisecond))
	frame, ok := s.next(start.Add(250 * time.Millisecond))
	if !ok || frame.Eye.Valid != 0 || !frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
		t.Fatalf("partial expiry frame = %#v, published = %v", frame, ok)
	}
}

func TestStateDifferenceInputExpiryInvalidatesTarget(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, expressionSubscription(trackingmodel.ExpressionJawX))
	s.observe([]observation{
		{ID: rawJawSidewaysRight, Values: [3]float32{0.8}},
		{ID: rawJawSidewaysLeft, Values: [3]float32{0.3}},
	}, start)
	if _, ok := s.next(start.Add(time.Millisecond)); !ok {
		t.Fatal("missing initial difference")
	}
	s.observe([]observation{{ID: rawJawSidewaysRight, Values: [3]float32{0.8}}}, start.Add(200*time.Millisecond))
	frame, ok := s.next(start.Add(250 * time.Millisecond))
	if !ok || frame.Expressions.Valid.Has(trackingmodel.ExpressionJawX) {
		t.Fatalf("difference expiry frame = %#v, published = %v", frame, ok)
	}
}

func TestStateSuppressesEmptyStartup(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	if _, ok := s.next(start); ok {
		t.Fatal("empty startup published")
	}
}

func TestStateRepeatedObservationPublishesSamePayload(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	value := observation{ID: rawJawDrop, Values: [3]float32{0.5}}
	s.observe([]observation{value}, start)
	first, ok := s.next(start.Add(time.Millisecond))
	if !ok {
		t.Fatal("missing initial frame")
	}
	s.observe([]observation{value}, start.Add(2*time.Millisecond))
	second, ok := s.next(start.Add(3 * time.Millisecond))
	if !ok || second.Expressions != first.Expressions {
		t.Fatalf("repeated observation frame = %#v, published = %v", second, ok)
	}
}

func TestStateOptionalWideExpiryChangesOpenPayload(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, pluginapi.Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityEye,
		Eye:          trackingmodel.EyeValidLeftOpenness,
	})
	s.observe([]observation{
		{ID: rawEyesClosedL, Values: [3]float32{0.2}},
		{ID: rawUpperLidRaiserL, Values: [3]float32{1}},
	}, start)
	if _, ok := s.next(start.Add(time.Millisecond)); !ok {
		t.Fatal("missing wide eyelid frame")
	}
	s.observe([]observation{{ID: rawEyesClosedL, Values: [3]float32{0.2}}}, start.Add(100*time.Millisecond))
	if _, ok := s.next(start.Add(101 * time.Millisecond)); !ok {
		t.Fatal("missing refreshed closed eyelid frame")
	}
	frame, ok := s.next(start.Add(250 * time.Millisecond))
	if !ok || frame.Eye.Valid != trackingmodel.EyeValidLeftOpenness || frame.Eye.LeftOpenness != 0.6 {
		t.Fatalf("wide expiry frame = %#v, published = %v", frame, ok)
	}
}

func TestStateIgnoresObservationsWithoutSubscription(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, pluginapi.Subscription{})
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start)
	if _, ok := s.next(start.Add(time.Millisecond)); ok {
		t.Fatal("unsubscribed observation published")
	}
}

func TestStatePauseResumeRequiresNewObservation(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	sub := expressionSubscription(trackingmodel.ExpressionJawOpen)
	s.reset(true, sub)
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start)
	if _, ok := s.next(start.Add(time.Millisecond)); !ok {
		t.Fatal("missing active frame")
	}
	s.reset(false, pluginapi.Subscription{})
	if _, ok := s.next(start.Add(2 * time.Millisecond)); ok {
		t.Fatal("paused state published")
	}
	s.reset(true, sub)
	if _, ok := s.next(start.Add(3 * time.Millisecond)); ok {
		t.Fatal("resumed cached frame published")
	}
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start.Add(4*time.Millisecond))
	if _, ok := s.next(start.Add(5 * time.Millisecond)); !ok {
		t.Fatal("resume did not publish new observation")
	}
}

func TestStateGenerationResetRequiresNewObservation(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start)
	if _, ok := s.next(start.Add(time.Millisecond)); !ok {
		t.Fatal("missing first generation frame")
	}
	s.reset(true, pluginapi.Subscription{Generation: 2, Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)})
	if _, ok := s.next(start.Add(2 * time.Millisecond)); ok {
		t.Fatal("previous generation observation published")
	}
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start.Add(3*time.Millisecond))
	if _, ok := s.next(start.Add(4 * time.Millisecond)); !ok {
		t.Fatal("new generation observation not published")
	}
}

func TestStateMetadataRemainsMonotonicAcrossResets(t *testing.T) {
	start := time.Unix(100, 0)
	s := newStreamState(start)
	s.reset(true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start)
	first, ok := s.next(start)
	if !ok || first.Sequence != 1 || first.TimestampNS != 1 || first.SourceClockNS != 0 {
		t.Fatalf("first metadata = %#v, published = %v", first, ok)
	}
	s.reset(true, pluginapi.Subscription{Generation: 2, Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)})
	s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start.Add(-time.Second))
	second, ok := s.next(start.Add(-time.Second))
	if !ok || second.Sequence != 2 || second.TimestampNS != 2 || second.SourceClockNS != 0 {
		t.Fatalf("reset metadata = %#v, published = %v", second, ok)
	}
}
