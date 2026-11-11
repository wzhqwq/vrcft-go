package picoconnect

import (
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestStreamStateExpiresFieldsIndependently(t *testing.T) {
	start := time.Now()
	state := newStreamState(start)
	state.reset(true, pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression})
	var eye shapePacket
	eye.present[shapeEyeBlinkL] = true
	eye.values[shapeEyeBlinkL] = .2
	state.observe(eye, start.Add(time.Millisecond))
	first, ok := state.next(start.Add(10 * time.Millisecond))
	if !ok || first.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 {
		t.Fatal("missing first eye frame")
	}
	var face shapePacket
	face.present[shapeJawOpen] = true
	face.values[shapeJawOpen] = .9
	state.observe(face, start.Add(200*time.Millisecond))
	second, ok := state.next(start.Add(260 * time.Millisecond))
	if !ok || second.Eye.Valid != 0 || !second.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
		t.Fatalf("stale eye retained: %+v", second)
	}
	third, ok := state.next(start.Add(451 * time.Millisecond))
	if !ok || third.Eye.Valid != 0 || !third.Expressions.Valid.IsZero() {
		t.Fatalf("expiry not published: %+v", third)
	}
	if _, ok := state.next(start.Add(500 * time.Millisecond)); ok {
		t.Fatal("repeated empty frame")
	}
}

func TestStreamStateResetDropsPriorGeneration(t *testing.T) {
	start := time.Now()
	state := newStreamState(start)
	sub := pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression}
	state.reset(true, sub)
	var packet shapePacket
	packet.present[shapeJawOpen] = true
	state.observe(packet, start)
	if _, ok := state.next(start.Add(time.Millisecond)); !ok {
		t.Fatal("first frame absent")
	}
	sub.Generation = 2
	state.reset(true, sub)
	if _, ok := state.next(start.Add(2 * time.Millisecond)); ok {
		t.Fatal("old generation survived reset")
	}
}
