package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestControlStateConfigMonotonicityAndOwnership(t *testing.T) {
	state := controlState{Config: pluginapi.Config{Revision: 2, Data: json.RawMessage(`{"gain":2}`)}}

	for _, test := range []struct {
		name        string
		config      pluginapi.Config
		changed     bool
		wantErr     error
		wantInvalid bool
	}{
		{"lower", pluginapi.Config{Revision: 1, Data: json.RawMessage(`{"gain":1}`)}, false, ErrConfigRevisionRegression, false},
		{"same equal", pluginapi.Config{Revision: 2, Data: json.RawMessage(`{"gain":2}`)}, false, nil, false},
		{"same conflicting", pluginapi.Config{Revision: 2, Data: json.RawMessage(`{"gain":3}`)}, false, ErrConfigRevisionConflict, false},
		{"higher invalid", pluginapi.Config{Revision: 3, Data: json.RawMessage(`{`)}, false, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := state.Config.Clone()
			changed, err := state.applyConfig(test.config)
			if changed != test.changed || (!test.wantInvalid && !errors.Is(err, test.wantErr)) || (test.wantInvalid && err == nil) {
				t.Fatalf("applyConfig() = (%v, %v), want (%v, %v)", changed, err, test.changed, test.wantErr)
			}
			if !reflect.DeepEqual(state.Config, before) {
				t.Fatalf("applyConfig() mutated state on rejected/idempotent update: got %#v, want %#v", state.Config, before)
			}
		})
	}

	input := pluginapi.Config{Revision: 3, Data: json.RawMessage(`{"gain":3}`)}
	changed, err := state.applyConfig(input)
	if err != nil || !changed {
		t.Fatalf("applyConfig(higher) = (%v, %v), want (true, nil)", changed, err)
	}
	input.Data[8] = '9'
	if got := string(state.Config.Data); got != `{"gain":3}` {
		t.Fatalf("state.Config.Data = %q after caller mutation, want owned data", got)
	}
}

func TestControlStateSubscriptionMonotonicity(t *testing.T) {
	initial := testSubscription(2, trackingmodel.CapabilityEye)
	state := controlState{Active: true, Subscription: initial}

	conflicting := initial
	conflicting.Eye = trackingmodel.EyeValidLeftGaze
	invalid := pluginapi.Subscription{Generation: 3}
	for _, test := range []struct {
		name         string
		subscription pluginapi.Subscription
		changed      bool
		wantErr      error
		wantInvalid  bool
	}{
		{"lower", testSubscription(1, trackingmodel.CapabilityEye), false, ErrSubscriptionGenerationRegression, false},
		{"same equal", initial, false, nil, false},
		{"same conflicting", conflicting, false, ErrSubscriptionGenerationConflict, false},
		{"higher invalid", invalid, false, nil, true},
		{"higher", testSubscription(3, trackingmodel.CapabilityExpression), true, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed, err := state.applySubscription(test.subscription)
			if changed != test.changed || (!test.wantInvalid && !errors.Is(err, test.wantErr)) || (test.wantInvalid && err == nil) {
				t.Fatalf("applySubscription() = (%v, %v), want (%v, %v)", changed, err, test.changed, test.wantErr)
			}
		})
	}
}

func TestControlStateActiveIsIdempotent(t *testing.T) {
	state := controlState{Active: false}
	if state.applyActive(false) {
		t.Fatal("applyActive(false) changed an already inactive state")
	}
	if !state.applyActive(true) || !state.Active {
		t.Fatal("applyActive(true) did not activate state")
	}
}

func TestSessionWriterBuildsExactTypedMessagesAndSkipsIdempotentUpdates(t *testing.T) {
	conn := newRecordingControlConn()
	initial := controlState{
		Active:       false,
		Config:       pluginapi.Config{Revision: 1, Data: json.RawMessage(`{}`)},
		Subscription: testSubscription(1, trackingmodel.CapabilityEye),
	}
	writer := newSessionWriter(conn, initial, 4)
	defer stopSessionWriter(t, writer)

	requests := []controlRequest{
		{kind: controlConfig, state: controlState{Config: initial.Config.Clone()}},
		{kind: controlActive, state: controlState{Active: true}},
		{kind: controlConfig, state: controlState{Config: pluginapi.Config{Revision: 2, Data: json.RawMessage(`{"gain":2}`)}}},
		{kind: controlSubscription, state: controlState{Subscription: testSubscription(2, trackingmodel.CapabilityExpression)}},
	}
	for _, request := range requests {
		if err := writer.Control(context.Background(), request); err != nil {
			t.Fatalf("Control(%d) error = %v", request.kind, err)
		}
	}

	got := conn.messages()
	wantPayloads := []any{
		protocol.ActiveChanged{Active: true},
		protocol.ConfigChanged{Config: requests[2].state.Config},
		protocol.SubscriptionChanged{Subscription: requests[3].state.Subscription},
	}
	if len(got) != len(wantPayloads) {
		t.Fatalf("sent %d messages, want %d: %#v", len(got), len(wantPayloads), got)
	}
	for i, payload := range wantPayloads {
		if got[i].Version != protocol.Version || !reflect.DeepEqual(got[i].Payload, payload) {
			t.Errorf("message[%d] = %#v, want payload %#v", i, got[i], payload)
		}
	}
}

func TestSessionWriterSerializesAcceptedControlsAndPreservesOrder(t *testing.T) {
	conn := newRecordingControlConn()
	conn.block = make(chan struct{})
	conn.entered = make(chan struct{}, 4)
	writer := newSessionWriter(conn, controlState{}, 4)
	defer stopSessionWriter(t, writer)

	first := asyncControl(writer, controlRequest{kind: controlActive, state: controlState{Active: true}})
	awaitSignal(t, conn.entered)
	second := asyncControl(writer, controlRequest{kind: controlConfig, state: controlState{Config: pluginapi.Config{Revision: 1, Data: json.RawMessage(`{}`)}}})
	waitForQueueLength(t, writer, 1)
	close(conn.block)
	if err := <-first; err != nil {
		t.Fatalf("first Control() error = %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second Control() error = %v", err)
	}

	if conn.maxConcurrent != 1 {
		t.Fatalf("maximum concurrent Send calls = %d, want 1", conn.maxConcurrent)
	}
	got := conn.messages()
	if len(got) != 2 ||
		got[0].Type != protocol.MessageActiveChanged ||
		got[1].Type != protocol.MessageConfigChanged {
		t.Fatalf("message order = %#v, want ActiveChanged then ConfigChanged", got)
	}
}

func TestSessionWriterBackpressureAndCancellationBeforeAcceptance(t *testing.T) {
	conn := newRecordingControlConn()
	conn.block = make(chan struct{})
	conn.entered = make(chan struct{}, 4)
	writer := newSessionWriter(conn, controlState{}, 1)
	defer stopSessionWriter(t, writer)

	first := asyncControl(writer, controlRequest{kind: controlActive, state: controlState{Active: true}})
	awaitSignal(t, conn.entered)
	second := asyncControl(writer, controlRequest{kind: controlConfig, state: controlState{Config: pluginapi.Config{Revision: 1}}})
	waitForQueueLength(t, writer, 1)

	err := writer.Control(context.Background(), controlRequest{
		kind:  controlSubscription,
		state: controlState{Subscription: testSubscription(1, trackingmodel.CapabilityEye)},
	})
	if !errors.Is(err, ErrControlBackpressure) {
		t.Fatalf("Control(full queue) error = %v, want ErrControlBackpressure", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writer.Control(ctx, controlRequest{kind: controlActive}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Control(canceled) error = %v, want context.Canceled", err)
	}

	close(conn.block)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
}

func TestSessionWriterShutdownFollowsAcceptedControlsAndBlocksLaterControls(t *testing.T) {
	conn := newRecordingControlConn()
	conn.block = make(chan struct{})
	conn.entered = make(chan struct{}, 4)
	writer := newSessionWriter(conn, controlState{}, 2)

	first := asyncControl(writer, controlRequest{kind: controlActive, state: controlState{Active: true}})
	awaitSignal(t, conn.entered)
	shutdown := asyncControl(writer, controlRequest{kind: controlShutdown})
	waitForQueueLength(t, writer, 1)
	if err := writer.Control(context.Background(), controlRequest{kind: controlActive}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Control(after shutdown accepted) error = %v, want ErrInvalidState", err)
	}
	close(conn.block)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
	select {
	case <-writer.Done():
	case <-time.After(time.Second):
		t.Fatal("writer Done did not close after Shutdown")
	}
	got := conn.messages()
	if len(got) != 2 || got[0].Type != protocol.MessageActiveChanged || got[1].Type != protocol.MessageShutdown {
		t.Fatalf("message order = %#v, want ActiveChanged then Shutdown", got)
	}
}

func TestSessionWriterSendFailureCompletesOwnerAndTerminates(t *testing.T) {
	sendErr := errors.New("send failed")
	conn := newRecordingControlConn()
	conn.sendErr = sendErr
	writer := newSessionWriter(conn, controlState{}, 1)

	err := writer.Control(context.Background(), controlRequest{kind: controlActive, state: controlState{Active: true}})
	if !errors.Is(err, sendErr) {
		t.Fatalf("Control() error = %v, want send failure", err)
	}
	select {
	case <-writer.Done():
	case <-time.After(time.Second):
		t.Fatal("writer did not terminate after Send failure")
	}
	if err := writer.Control(context.Background(), controlRequest{kind: controlActive}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Control(after failure) error = %v, want ErrInvalidState", err)
	}
}

func testSubscription(generation uint64, capabilities trackingmodel.Capability) pluginapi.Subscription {
	return pluginapi.Subscription{Generation: generation, Capabilities: capabilities}
}

type recordingControlConn struct {
	mu            sync.Mutex
	sent          []protocol.Message
	concurrent    int
	maxConcurrent int
	block         chan struct{}
	entered       chan struct{}
	sendErr       error
}

func newRecordingControlConn() *recordingControlConn {
	return &recordingControlConn{}
}

func (c *recordingControlConn) Send(ctx context.Context, message protocol.Message) error {
	c.mu.Lock()
	c.concurrent++
	if c.concurrent > c.maxConcurrent {
		c.maxConcurrent = c.concurrent
	}
	c.sent = append(c.sent, message)
	c.mu.Unlock()

	if c.entered != nil {
		c.entered <- struct{}{}
	}
	if c.block != nil {
		select {
		case <-c.block:
		case <-ctx.Done():
		}
	}

	c.mu.Lock()
	c.concurrent--
	c.mu.Unlock()
	return c.sendErr
}

func (c *recordingControlConn) Receive(context.Context) (protocol.Message, error) {
	panic("unexpected Receive")
}

func (c *recordingControlConn) Close() error { return nil }

func (c *recordingControlConn) messages() []protocol.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]protocol.Message(nil), c.sent...)
}

func asyncControl(writer *sessionWriter, request controlRequest) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- writer.Control(context.Background(), request)
	}()
	return result
}

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Send")
	}
}

func waitForQueueLength(t *testing.T, writer *sessionWriter, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(writer.requests) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue length = %d, want %d", len(writer.requests), want)
}

func stopSessionWriter(t *testing.T, writer *sessionWriter) {
	t.Helper()
	select {
	case <-writer.Done():
		return
	default:
	}
	if err := writer.Control(context.Background(), controlRequest{kind: controlShutdown}); err != nil {
		t.Errorf("shutdown Control() error = %v", err)
		return
	}
	select {
	case <-writer.Done():
	case <-time.After(time.Second):
		t.Error("writer did not terminate during cleanup")
	}
}
