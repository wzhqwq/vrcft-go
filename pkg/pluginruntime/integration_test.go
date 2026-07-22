package pluginruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type selectiveFrameDriver struct{}

func (selectiveFrameDriver) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		APIVersion:   pluginapi.APIVersion,
		ID:           "test.selective-frame",
		Name:         "Selective Frame Test",
		Version:      "1.0.0",
		Description:  "publishes selected and unselected tracking data",
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
	}
}

func (selectiveFrameDriver) Run(ctx context.Context, host pluginapi.Host) error {
	frame := trackingmodel.TrackingFrame{
		Sequence:     23,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye: trackingmodel.EyeSample{
			Valid:    trackingmodel.EyeValidLeftGaze,
			LeftGaze: trackingmodel.Vec2{X: 0.25, Y: -0.5},
		},
	}
	frame.Expressions.Valid = trackingmodel.ExpressionMaskOf(
		trackingmodel.ExpressionJawOpen,
		trackingmodel.ExpressionBrowPinchRight,
	)
	frame.Expressions.Values[trackingmodel.ExpressionJawOpen] = 0.8
	frame.Expressions.Values[trackingmodel.ExpressionBrowPinchRight] = 0.6
	if !host.PublishFrame(frame) {
		return errors.New("sample driver frame was rejected")
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestRuntimeIntegrationSendsOnlySubscribedExpression(t *testing.T) {
	c := newMemoryConn(8)
	runtime, err := New(selectiveFrameDriver{}, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()

	if got := receiveMessage(t, c); got.Type != protocol.MessageHello {
		t.Fatalf("first message = %v, want Hello", got.Type)
	}
	startup := pluginapi.Startup{
		Active: true,
		Subscription: pluginapi.Subscription{
			Generation:   41,
			Capabilities: trackingmodel.CapabilityExpression,
			Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
		},
	}
	c.toRuntime <- mustMessage(t, protocol.Initialize{Startup: startup})
	if got := receiveMessage(t, c); got.Type != protocol.MessageReady {
		t.Fatalf("second message = %v, want Ready", got.Type)
	}

	message := receiveMessage(t, c)
	payload, ok := message.Payload.(protocol.TrackingFrame)
	if !ok {
		t.Fatalf("wire payload = %T, want protocol.TrackingFrame", message.Payload)
	}
	if payload.Generation != 41 {
		t.Fatalf("wire generation = %d, want 41", payload.Generation)
	}
	if payload.Frame.Capabilities != trackingmodel.CapabilityExpression || payload.Frame.Eye != (trackingmodel.EyeSample{}) {
		t.Fatalf("unselected eye data reached wire: capabilities=%v eye=%+v", payload.Frame.Capabilities, payload.Frame.Eye)
	}
	if payload.Frame.Expressions.Valid != trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen) {
		t.Fatalf("wire expression validity = %+v, want JawOpen only", payload.Frame.Expressions.Valid)
	}
	if got := payload.Frame.Expressions.Values[trackingmodel.ExpressionJawOpen]; got != 0.8 {
		t.Fatalf("wire JawOpen = %v, want 0.8", got)
	}
	if got := payload.Frame.Expressions.Values[trackingmodel.ExpressionBrowPinchRight]; got != 0 {
		t.Fatalf("wire BrowPinchRight = %v, want cleared", got)
	}
	for id, value := range payload.Frame.Expressions.Values {
		if trackingmodel.ExpressionID(id) != trackingmodel.ExpressionJawOpen && value != 0 {
			t.Fatalf("unselected expression %d reached wire with value %v", id, value)
		}
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("runtime did not stop after integration cancellation")
	}
}
