package pluginruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const testTimeout = 2 * time.Second

type memoryConn struct {
	toRuntime   chan protocol.Message
	fromRuntime chan protocol.Message
	closed      chan struct{}
	closeOnce   sync.Once
	closeCalls  atomic.Int32
	activeSends atomic.Int32
	maxSends    atomic.Int32

	postReadySendStarted chan struct{}
	sendStartedOnce      sync.Once

	mu                   sync.Mutex
	sendCalls            int
	receiveCalls         int
	sendErrAt            int
	sendErr              error
	sendErrAfterCancelAt int
	sendErrAfterCancel   error
	receiveErr           error
	closeErr             error
	sendDelay            time.Duration
}

type orderedTerminalConn struct {
	*memoryConn
	receiveCalls  atomic.Int32
	readerStarted chan struct{}
	releaseReader chan struct{}
	readerErr     error
	afterCancel   bool
}

func (c *orderedTerminalConn) Receive(ctx context.Context) (protocol.Message, error) {
	if c.receiveCalls.Add(1) == 1 {
		return c.memoryConn.Receive(ctx)
	}
	close(c.readerStarted)
	if c.afterCancel {
		<-ctx.Done()
	} else {
		select {
		case <-c.releaseReader:
		case <-c.closed:
			return protocol.Message{}, io.EOF
		}
	}
	return protocol.Message{}, c.readerErr
}

type closeOnlyReceiveConn struct {
	*memoryConn
	receiveCalls  atomic.Int32
	readerStarted chan struct{}
}

func (c *closeOnlyReceiveConn) Receive(ctx context.Context) (protocol.Message, error) {
	if c.receiveCalls.Add(1) == 1 {
		return c.memoryConn.Receive(ctx)
	}
	close(c.readerStarted)
	<-c.closed
	return protocol.Message{}, io.EOF
}

func newMemoryConn(capacity int) *memoryConn {
	return &memoryConn{
		toRuntime:   make(chan protocol.Message, capacity),
		fromRuntime: make(chan protocol.Message, capacity),
		closed:      make(chan struct{}),
	}
}

func (c *memoryConn) Send(ctx context.Context, message protocol.Message) error {
	active := c.activeSends.Add(1)
	defer c.activeSends.Add(-1)
	for {
		maximum := c.maxSends.Load()
		if active <= maximum || c.maxSends.CompareAndSwap(maximum, active) {
			break
		}
	}

	c.mu.Lock()
	c.sendCalls++
	call := c.sendCalls
	errAt, sendErr := c.sendErrAt, c.sendErr
	errAfterCancelAt, errAfterCancel := c.sendErrAfterCancelAt, c.sendErrAfterCancel
	sendDelay := c.sendDelay
	c.mu.Unlock()
	if call >= 3 && c.postReadySendStarted != nil {
		c.sendStartedOnce.Do(func() { close(c.postReadySendStarted) })
	}
	if errAt != 0 && call == errAt {
		return sendErr
	}
	if errAfterCancelAt != 0 && call == errAfterCancelAt {
		<-ctx.Done()
		return errAfterCancel
	}
	if sendDelay > 0 {
		timer := time.NewTimer(sendDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closed:
			return io.ErrClosedPipe
		}
	}
	select {
	case c.fromRuntime <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return io.ErrClosedPipe
	}
}

func (c *memoryConn) Receive(ctx context.Context) (protocol.Message, error) {
	c.mu.Lock()
	c.receiveCalls++
	err := c.receiveErr
	c.receiveErr = nil
	c.mu.Unlock()
	if err != nil {
		return protocol.Message{}, err
	}
	select {
	case message := <-c.toRuntime:
		return message, nil
	case <-ctx.Done():
		return protocol.Message{}, ctx.Err()
	case <-c.closed:
		return protocol.Message{}, io.EOF
	}
}

func (c *memoryConn) Close() error {
	c.closeCalls.Add(1)
	c.closeOnce.Do(func() { close(c.closed) })
	return c.closeErr
}

func (c *memoryConn) calls() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendCalls, c.receiveCalls
}

func (c *memoryConn) failNextReceive(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.receiveErr = err
}

type testDriver struct {
	descriptor pluginapi.Descriptor
	run        func(context.Context, pluginapi.Host) error
	runCalls   atomic.Int32
}

func (d *testDriver) Descriptor() pluginapi.Descriptor { return d.descriptor }

func (d *testDriver) Run(ctx context.Context, host pluginapi.Host) error {
	d.runCalls.Add(1)
	if d.run == nil {
		return nil
	}
	return d.run(ctx, host)
}

func validTestDescriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		APIVersion:   pluginapi.APIVersion,
		ID:           "test.runtime",
		Name:         "Runtime Test",
		Version:      "1.2.3",
		Description:  "runtime test driver",
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
	}
}

func validTestStartup() pluginapi.Startup {
	return pluginapi.Startup{
		Active: true,
		Config: pluginapi.Config{Revision: 1, Data: json.RawMessage(`{"gain":1}`)},
		Subscription: pluginapi.Subscription{
			Generation:   1,
			Capabilities: trackingmodel.CapabilityEye,
			Eye:          trackingmodel.EyeValidLeftGaze,
		},
	}
}

func testConfig() RuntimeConfig {
	cfg := DefaultRuntimeConfig()
	cfg.Token = "runtime-token"
	cfg.HeartbeatInterval = time.Hour
	cfg.ShutdownTimeout = 250 * time.Millisecond
	return cfg
}

func mustMessage(t *testing.T, payload any) protocol.Message {
	t.Helper()
	message, err := protocol.NewMessage(payload)
	if err != nil {
		t.Fatalf("protocol.NewMessage(%T) error = %v", payload, err)
	}
	return message
}

func receiveMessage(t *testing.T, c *memoryConn) protocol.Message {
	t.Helper()
	select {
	case message := <-c.fromRuntime:
		return message
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for runtime message")
		return protocol.Message{}
	}
}

func startHandshake(t *testing.T, runtime *Runtime, c *memoryConn, startup pluginapi.Startup) <-chan error {
	t.Helper()
	return startHandshakeContext(t, runtime, c, startup, context.Background())
}

func startHandshakeContext(t *testing.T, runtime *Runtime, c *memoryConn, startup pluginapi.Startup, ctx context.Context) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	if got := receiveMessage(t, c); got.Type != protocol.MessageHello {
		t.Fatalf("first message type = %v, want Hello", got.Type)
	}
	c.toRuntime <- mustMessage(t, protocol.Initialize{Startup: startup})
	if got := receiveMessage(t, c); got.Type != protocol.MessageReady {
		t.Fatalf("second message type = %v, want Ready", got.Type)
	}
	return done
}

func runtimeNotices(t *testing.T, c *memoryConn) []protocol.Error {
	t.Helper()
	var notices []protocol.Error
	for {
		select {
		case message := <-c.fromRuntime:
			notice, ok := message.Payload.(protocol.Error)
			if !ok {
				t.Fatalf("terminal payload = %T, want protocol.Error", message.Payload)
			}
			notices = append(notices, notice)
		default:
			return notices
		}
	}
}

func requireNotice(t *testing.T, notices []protocol.Error, code, messagePart string, want int) {
	t.Helper()
	count := 0
	for _, notice := range notices {
		if notice.Code == code && strings.Contains(notice.Message, messagePart) {
			count++
		}
	}
	if count != want {
		t.Fatalf("notices matching code=%q message~%q = %d, want %d; all=%+v", code, messagePart, count, want, notices)
	}
}

func awaitResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(testTimeout):
		t.Fatal("runtime did not return")
		return nil
	}
}

func awaitClosed(t *testing.T, events <-chan pluginapi.ControlEvent) {
	t.Helper()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("event channel remained open")
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for event channel close")
	}
}

func TestLatestFrameSlotRejectsZeroGeneration(t *testing.T) {
	slot := NewLatestFrameSlot()
	if slot.Store(pendingFrame{}) {
		t.Fatal("Store(zero generation) = true")
	}
	if _, ok := slot.Load(); ok {
		t.Fatal("Load() found a zero-generation frame")
	}
	select {
	case <-slot.Notify():
		t.Fatal("zero-generation Store signaled notification")
	default:
	}
}

func TestLatestFrameSlotNotifiesOverwritesAndConsumes(t *testing.T) {
	slot := NewLatestFrameSlot()
	first := pendingFrame{
		Generation:   1,
		Subscription: validTestStartup().Subscription,
		Frame:        trackingmodel.TrackingFrame{Sequence: 1},
	}
	second := pendingFrame{
		Generation: 2,
		Subscription: pluginapi.Subscription{
			Generation:   2,
			Capabilities: trackingmodel.CapabilityExpression,
		},
		Frame: trackingmodel.TrackingFrame{Sequence: 2},
	}
	if !slot.Store(first) || !slot.Store(second) {
		t.Fatal("Store(valid frame) = false")
	}
	first.Frame.Sequence = 99
	second.Frame.Sequence = 99
	second.Subscription.Generation = 99

	if got := cap(slot.Notify()); got != 1 {
		t.Fatalf("notification capacity = %d, want 1", got)
	}
	select {
	case <-slot.Notify():
	default:
		t.Fatal("Store did not signal notification")
	}
	select {
	case <-slot.Notify():
		t.Fatal("overwrite queued more than one notification")
	default:
	}
	got, ok := slot.Load()
	if !ok || got.Generation != 2 || got.Subscription.Generation != 2 || got.Frame.Sequence != 2 {
		t.Fatalf("Load() = (%+v, %v), want copied newest frame", got, ok)
	}
	if _, ok := slot.Load(); ok {
		t.Fatal("second Load() found already-consumed frame")
	}
}

func TestLatestFrameSlotClearAndClearBefore(t *testing.T) {
	makeFrame := func(generation uint64) pendingFrame {
		return pendingFrame{Generation: generation, Subscription: pluginapi.Subscription{Generation: generation}}
	}

	t.Run("clear older generation", func(t *testing.T) {
		slot := NewLatestFrameSlot()
		slot.Store(makeFrame(2))
		slot.ClearBefore(3)
		if _, ok := slot.Load(); ok {
			t.Fatal("ClearBefore retained an older generation")
		}
	})

	t.Run("retain same or newer generation", func(t *testing.T) {
		slot := NewLatestFrameSlot()
		slot.Store(makeFrame(3))
		slot.ClearBefore(3)
		if got, ok := slot.Load(); !ok || got.Generation != 3 {
			t.Fatalf("Load() = (%+v, %v), want generation 3", got, ok)
		}
		select {
		case <-slot.Notify():
			t.Fatal("Load left a stale notification")
		default:
		}
	})

	t.Run("clear all", func(t *testing.T) {
		slot := NewLatestFrameSlot()
		slot.Store(makeFrame(4))
		slot.Clear()
		if _, ok := slot.Load(); ok {
			t.Fatal("Clear retained pending frame")
		}
	})
}

func TestRuntimeHostPublishFrameUsesAtomicSubscriptionSnapshot(t *testing.T) {
	startup := validTestStartup()
	host := newRuntimeHost(startup, 1, 1)
	frame := trackingmodel.TrackingFrame{Sequence: 7, Capabilities: trackingmodel.CapabilityEye}

	result := make(chan bool, 1)
	go func() { result <- host.PublishFrame(frame) }()
	select {
	case accepted := <-result:
		if !accepted {
			t.Fatal("PublishFrame(valid active frame) = false")
		}
	case <-time.After(testTimeout):
		t.Fatal("PublishFrame blocked")
	}

	pending, ok := host.frames.Load()
	if !ok || pending.Generation != startup.Subscription.Generation || pending.Subscription != startup.Subscription || pending.Frame != frame {
		t.Fatalf("pending frame = (%+v, %v), want generation-tagged snapshot", pending, ok)
	}
}

func TestRuntimeHostPublishFrameRejectsUnavailableStates(t *testing.T) {
	tests := []struct {
		name    string
		startup pluginapi.Startup
		stop    bool
	}{
		{"inactive", func() pluginapi.Startup { s := validTestStartup(); s.Active = false; return s }(), false},
		{"zero generation", func() pluginapi.Startup { s := validTestStartup(); s.Subscription.Generation = 0; return s }(), false},
		{"empty capabilities", func() pluginapi.Startup { s := validTestStartup(); s.Subscription.Capabilities = 0; return s }(), false},
		{"stopped", validTestStartup(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := newRuntimeHost(tt.startup, 1, 1)
			if tt.stop {
				host.stop()
			}
			if host.PublishFrame(trackingmodel.TrackingFrame{}) {
				t.Fatal("PublishFrame(unavailable host) = true")
			}
			if _, ok := host.frames.Load(); ok {
				t.Fatal("rejected frame was stored")
			}
		})
	}
}

func TestRuntimeHostPublishFrameRejectsMalformedFramesBeforeSlot(t *testing.T) {
	expressionTail := trackingmodel.ExpressionMask{}
	expressionTail.Words[len(expressionTail.Words)-1] = uint64(1) << (trackingmodel.ExpressionCount % 64)
	tests := []struct {
		name  string
		frame trackingmodel.TrackingFrame
	}{
		{
			name:  "unknown capability",
			frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.Capability(1 << 20)},
		},
		{
			name: "unknown eye validity",
			frame: trackingmodel.TrackingFrame{
				Capabilities: trackingmodel.CapabilityEye,
				Eye:          trackingmodel.EyeSample{Valid: trackingmodel.EyeValid(1 << 15)},
			},
		},
		{
			name: "expression tail",
			frame: trackingmodel.TrackingFrame{
				Capabilities: trackingmodel.CapabilityExpression,
				Expressions:  trackingmodel.ExpressionSet{Valid: expressionTail},
			},
		},
		{
			name:  "eye validity in disabled group",
			frame: trackingmodel.TrackingFrame{Eye: trackingmodel.EyeSample{Valid: trackingmodel.EyeValidLeftGaze}},
		},
		{
			name:  "expression validity in disabled group",
			frame: trackingmodel.TrackingFrame{Expressions: trackingmodel.ExpressionSet{Valid: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := newRuntimeHost(validTestStartup(), 1, 1)
			if host.PublishFrame(tt.frame) {
				t.Fatal("PublishFrame(malformed frame) = true")
			}
			if _, ok := host.frames.Load(); ok {
				t.Fatal("malformed frame entered LatestFrameSlot")
			}
		})
	}
}

func TestRuntimeHostPublishFrameStoresCanonicalFrameAndAcceptsDropout(t *testing.T) {
	host := newRuntimeHost(validTestStartup(), 1, 1)
	frame := trackingmodel.TrackingFrame{
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftGaze,
			LeftGaze:     trackingmodel.Vec2{X: 1, Y: 2},
			RightGaze:    trackingmodel.Vec2{X: 3, Y: 4},
			LeftOpenness: 0.5,
		},
	}
	if !host.PublishFrame(frame) {
		t.Fatal("PublishFrame(valid frame) = false")
	}
	pending, ok := host.frames.Load()
	if !ok {
		t.Fatal("valid frame was not stored")
	}
	if pending.Frame.Eye.RightGaze != (trackingmodel.Vec2{}) || pending.Frame.Eye.LeftOpenness != 0 {
		t.Fatalf("stored frame was not canonicalized: %+v", pending.Frame)
	}

	if !host.PublishFrame(trackingmodel.TrackingFrame{Sequence: 2}) {
		t.Fatal("PublishFrame(valid dropout) = false")
	}
	pending, ok = host.frames.Load()
	if !ok || pending.Frame.Sequence != 2 {
		t.Fatalf("stored dropout = (%+v, %t), want sequence 2", pending, ok)
	}
}

func TestRuntimeRejectsInvalidFrameWithoutTerminating(t *testing.T) {
	c := newMemoryConn(8)
	hostReady := make(chan pluginapi.Host, 1)
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		hostReady <- host
		<-ctx.Done()
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	host := <-hostReady
	invalid := trackingmodel.TrackingFrame{
		Capabilities: trackingmodel.CapabilityEye,
		Eye:          trackingmodel.EyeSample{Valid: trackingmodel.EyeValid(1 << 15)},
	}
	if host.PublishFrame(invalid) {
		t.Fatal("PublishFrame(invalid frame) = true")
	}
	select {
	case err := <-done:
		t.Fatalf("runtime terminated after invalid frame: %v", err)
	case message := <-c.fromRuntime:
		t.Fatalf("invalid frame produced wire message %#v", message)
	case <-time.After(30 * time.Millisecond):
	}

	c.toRuntime <- mustMessage(t, protocol.Shutdown{})
	if err := awaitResult(t, done); err != nil {
		t.Fatalf("Run() after later shutdown error = %v", err)
	}
	if got := receiveMessage(t, c); got.Type != protocol.MessageShutdownAck {
		t.Fatalf("terminal message = %#v, want ShutdownAck", got)
	}
}

func TestRuntimeHostSubscriptionAndActivationClearPendingFrames(t *testing.T) {
	t.Run("new generation clears older frame", func(t *testing.T) {
		host := newRuntimeHost(validTestStartup(), 1, 1)
		if !host.PublishFrame(trackingmodel.TrackingFrame{Sequence: 1}) {
			t.Fatal("PublishFrame() = false")
		}
		next := pluginapi.Subscription{Generation: 2, Capabilities: trackingmodel.CapabilityExpression}
		if _, err := host.applySubscription(next); err != nil {
			t.Fatal(err)
		}
		if _, ok := host.frames.Load(); ok {
			t.Fatal("new subscription retained older pending frame")
		}
	})

	t.Run("deactivation clears all frames", func(t *testing.T) {
		host := newRuntimeHost(validTestStartup(), 1, 1)
		if !host.PublishFrame(trackingmodel.TrackingFrame{Sequence: 1}) {
			t.Fatal("PublishFrame() = false")
		}
		host.applyActive(false)
		if _, ok := host.frames.Load(); ok {
			t.Fatal("deactivation retained pending frame")
		}
	})
}

func TestRuntimeHostTreatsNilAndEmptyConfigDataAsSameRevisionDuplicate(t *testing.T) {
	startup := validTestStartup()
	startup.Config = pluginapi.Config{Revision: 2}
	host := newRuntimeHost(startup, 1, 1)

	event, err := host.applyConfig(pluginapi.Config{Revision: 2, Data: json.RawMessage{}})
	if err != nil {
		t.Fatalf("applyConfig(empty duplicate) error = %v", err)
	}
	if event != nil {
		t.Fatalf("applyConfig(empty duplicate) event = %#v, want nil", event)
	}
	if host.current.Config.Data != nil {
		t.Fatalf("current Config.Data = %#v, want canonical nil", host.current.Config.Data)
	}
}

func TestRuntimeHostSubscriptionRaceCannotSendOldGeneration(t *testing.T) {
	sent := 0
	for attempt := 0; attempt < 100; attempt++ {
		host := newRuntimeHost(validTestStartup(), 1, 1)
		start := make(chan struct{})
		published := make(chan bool, 1)
		applied := make(chan error, 1)
		go func() {
			<-start
			published <- host.PublishFrame(trackingmodel.TrackingFrame{Sequence: 1})
		}()
		next := pluginapi.Subscription{Generation: 2, Capabilities: trackingmodel.CapabilityExpression}
		go func() {
			<-start
			_, err := host.applySubscription(next)
			applied <- err
		}()
		close(start)
		if err := <-applied; err != nil {
			t.Fatal(err)
		}
		<-published
		if len(host.frames.Notify()) == 0 {
			continue
		}

		c := newMemoryConn(1)
		runtime := &Runtime{conn: c, config: testConfig()}
		cancel, writerDone := startOutboundWriter(t, runtime, host)
		message := receiveMessage(t, c)
		cancel()
		_ = awaitResult(t, writerDone)
		frame, ok := message.Payload.(protocol.TrackingFrame)
		if !ok || frame.Generation != 2 {
			t.Fatalf("attempt %d sent %#v, want generation 2 or no frame", attempt, message.Payload)
		}
		sent++
	}
	if sent == 0 {
		t.Fatal("subscription race never exercised the outbound send path")
	}
}

func TestRuntimeHostStatusIsValidatedAndLatestOnly(t *testing.T) {
	host := newRuntimeHost(validTestStartup(), 1, 1)
	host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady, Message: "old"})
	host.PublishStatus(pluginapi.DeviceStatus{State: "invalid", Message: "ignored"})
	want := pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: "camera lost"}
	host.PublishStatus(want)

	select {
	case <-host.statusNotify:
	default:
		t.Fatal("valid status did not signal notification")
	}
	select {
	case <-host.statusNotify:
		t.Fatal("status overwrite queued more than one notification")
	default:
	}
	if got, ok := host.loadStatus(); !ok || got != want {
		t.Fatalf("loadStatus() = (%+v, %v), want latest valid status", got, ok)
	}
	if _, ok := host.loadStatus(); ok {
		t.Fatal("loadStatus() returned consumed status")
	}

	host.stop()
	host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady})
	if _, ok := host.loadStatus(); ok {
		t.Fatal("post-stop status was retained")
	}
}

func TestRuntimeHostLogIsBoundedNonblockingAndCountsDrops(t *testing.T) {
	host := newRuntimeHost(validTestStartup(), 1, 1)
	host.Log(pluginapi.LogInfo, "queued")
	done := make(chan struct{})
	go func() {
		host.Log(pluginapi.LogError, "dropped")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("Log blocked on a full queue")
	}

	select {
	case got := <-host.logs:
		if got.Level != pluginapi.LogInfo || got.Message != "queued" {
			t.Fatalf("queued log = %+v", got)
		}
	default:
		t.Fatal("first log was not queued")
	}
	if got := host.droppedLogs.Load(); got != 1 {
		t.Fatalf("dropped count = %d, want 1", got)
	}

	host.Log(pluginapi.LogLevel("trace"), "invalid level")
	host.Log(pluginapi.LogInfo, " \t")
	host.stop()
	host.Log(pluginapi.LogInfo, "after stop")
	select {
	case got := <-host.logs:
		t.Fatalf("invalid or post-stop log queued: %+v", got)
	default:
	}
	if got := host.droppedLogs.Load(); got != 1 {
		t.Fatalf("invalid/post-stop calls changed dropped count to %d", got)
	}
}

func startOutboundWriter(t *testing.T, runtime *Runtime, host *runtimeHost) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.writeOutbound(ctx, host, time.Now()) }()
	return cancel, done
}

func TestOutboundWriterTrimsFrameWithStoredSubscription(t *testing.T) {
	c := newMemoryConn(8)
	cfg := testConfig()
	runtime := &Runtime{conn: c, config: cfg}
	startup := validTestStartup()
	startup.Subscription = pluginapi.Subscription{
		Generation:   9,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye:          trackingmodel.EyeValidLeftGaze,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
	host := newRuntimeHost(startup, 1, 1)
	frame := trackingmodel.TrackingFrame{
		Sequence:     17,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye: trackingmodel.EyeSample{
			Valid:     trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze,
			LeftGaze:  trackingmodel.Vec2{X: 1, Y: 2},
			RightGaze: trackingmodel.Vec2{X: 3, Y: 4},
		},
	}
	frame.Expressions.Valid = trackingmodel.ExpressionMaskOf(
		trackingmodel.ExpressionJawOpen,
		trackingmodel.ExpressionBrowPinchRight,
	)
	frame.Expressions.Values[trackingmodel.ExpressionJawOpen] = 0.75
	frame.Expressions.Values[trackingmodel.ExpressionBrowPinchRight] = 0.5
	if !host.PublishFrame(frame) {
		t.Fatal("PublishFrame() = false")
	}
	current := startup.Subscription
	current.Eye = trackingmodel.EyeValidRightGaze
	current.Expressions = trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionBrowPinchRight)
	host.mu.Lock()
	host.current.Subscription = current
	host.mu.Unlock()
	if got := host.current.Subscription; got != current {
		t.Fatalf("current host subscription = %+v, want changed selection %+v", got, current)
	}

	cancel, writerDone := startOutboundWriter(t, runtime, host)
	message := receiveMessage(t, c)
	cancel()
	if err := awaitResult(t, writerDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("writer error = %v, want context.Canceled", err)
	}
	payload, ok := message.Payload.(protocol.TrackingFrame)
	if !ok {
		t.Fatalf("payload = %T, want protocol.TrackingFrame", message.Payload)
	}
	if payload.Generation != 9 || payload.Frame.Sequence != 17 {
		t.Fatalf("tracking frame metadata = %+v", payload)
	}
	if payload.Frame.Eye.Valid != trackingmodel.EyeValidLeftGaze || payload.Frame.Eye.LeftGaze != frame.Eye.LeftGaze || payload.Frame.Eye.RightGaze != (trackingmodel.Vec2{}) {
		t.Fatalf("trimmed eye = %+v", payload.Frame.Eye)
	}
	if !payload.Frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) || payload.Frame.Expressions.Valid.Has(trackingmodel.ExpressionBrowPinchRight) {
		t.Fatalf("trimmed expression validity = %+v", payload.Frame.Expressions.Valid)
	}
	if payload.Frame.Expressions.Values[trackingmodel.ExpressionJawOpen] != 0.75 || payload.Frame.Expressions.Values[trackingmodel.ExpressionBrowPinchRight] != 0 {
		t.Fatalf("trimmed expression values = %+v", payload.Frame.Expressions.Values)
	}
}

func TestOutboundWriterSendsLatestStatus(t *testing.T) {
	c := newMemoryConn(4)
	runtime := &Runtime{conn: c, config: testConfig()}
	host := newRuntimeHost(validTestStartup(), 1, 1)
	host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceInitializing})
	want := pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: "camera unavailable"}
	host.PublishStatus(want)

	cancel, writerDone := startOutboundWriter(t, runtime, host)
	message := receiveMessage(t, c)
	cancel()
	_ = awaitResult(t, writerDone)
	if got, ok := message.Payload.(protocol.Status); !ok || got.Status != want {
		t.Fatalf("status payload = %#v, want %#v", message.Payload, want)
	}
}

func TestOutboundWriterReportsDroppedLogsOnNextEntry(t *testing.T) {
	c := newMemoryConn(4)
	runtime := &Runtime{conn: c, config: testConfig()}
	host := newRuntimeHost(validTestStartup(), 1, 1)
	host.Log(pluginapi.LogInfo, "retained")
	host.Log(pluginapi.LogWarn, "dropped")

	cancel, writerDone := startOutboundWriter(t, runtime, host)
	message := receiveMessage(t, c)
	cancel()
	_ = awaitResult(t, writerDone)
	if got, ok := message.Payload.(protocol.Log); !ok || got.Level != pluginapi.LogInfo || got.Message != "retained" || got.Dropped != 1 {
		t.Fatalf("log payload = %#v", message.Payload)
	}
}

func TestOutboundWriterRestoresDroppedLogsWhenSendFails(t *testing.T) {
	c := newMemoryConn(1)
	sendErr := errors.New("log send failed")
	c.sendErrAt, c.sendErr = 1, sendErr
	runtime := &Runtime{conn: c, config: testConfig()}
	host := newRuntimeHost(validTestStartup(), 1, 1)
	host.Log(pluginapi.LogInfo, "retained")
	host.Log(pluginapi.LogWarn, "dropped")

	_, writerDone := startOutboundWriter(t, runtime, host)
	if err := awaitResult(t, writerDone); !errors.Is(err, sendErr) {
		t.Fatalf("writer error = %v, want send error", err)
	}
	if got := host.droppedLogs.Load(); got != 1 {
		t.Fatalf("restored dropped count = %d, want 1", got)
	}
}

func TestOutboundWriterHeartbeatCadenceAndMonotonicUptime(t *testing.T) {
	c := newMemoryConn(8)
	cfg := testConfig()
	cfg.HeartbeatInterval = 5 * time.Millisecond
	runtime := &Runtime{conn: c, config: cfg}
	host := newRuntimeHost(validTestStartup(), 1, 1)
	cancel, writerDone := startOutboundWriter(t, runtime, host)

	var previous uint64
	for i := 0; i < 3; i++ {
		message := receiveMessage(t, c)
		heartbeat, ok := message.Payload.(protocol.Heartbeat)
		if !ok {
			t.Fatalf("payload %d = %T, want protocol.Heartbeat", i, message.Payload)
		}
		if i > 0 && heartbeat.UptimeMS <= previous {
			t.Fatalf("heartbeat uptime %d = %d, want greater than %d", i, heartbeat.UptimeMS, previous)
		}
		previous = heartbeat.UptimeMS
	}
	cancel()
	if err := awaitResult(t, writerDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("writer error = %v, want context.Canceled", err)
	}
}

func TestDefaultRuntimeConfig(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	if cfg.ControlQueue != 32 || cfg.LogQueue != 256 || cfg.HeartbeatInterval != time.Second || cfg.ShutdownTimeout != 5*time.Second {
		t.Fatalf("DefaultRuntimeConfig() = %+v", cfg)
	}
}

func TestNewValidationAndNoIOBeforeRun(t *testing.T) {
	validDriver := &testDriver{descriptor: validTestDescriptor()}
	validConn := newMemoryConn(1)
	validCfg := testConfig()
	var typedNilDriver *testDriver
	var typedNilConn *memoryConn

	tests := []struct {
		name   string
		driver pluginapi.Driver
		conn   protocol.Conn
		cfg    RuntimeConfig
	}{
		{"nil driver", nil, validConn, validCfg},
		{"typed nil driver", typedNilDriver, validConn, validCfg},
		{"nil conn", validDriver, nil, validCfg},
		{"typed nil conn", validDriver, typedNilConn, validCfg},
		{"blank token", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.Token = " \t"; return c }()},
		{"zero control queue", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.ControlQueue = 0; return c }()},
		{"zero log queue", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.LogQueue = 0; return c }()},
		{"zero heartbeat", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.HeartbeatInterval = 0; return c }()},
		{"zero shutdown timeout", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.ShutdownTimeout = 0; return c }()},
		{"negative control queue", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.ControlQueue = -1; return c }()},
		{"negative log queue", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.LogQueue = -1; return c }()},
		{"negative heartbeat", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.HeartbeatInterval = -time.Second; return c }()},
		{"negative shutdown timeout", validDriver, validConn, func() RuntimeConfig { c := validCfg; c.ShutdownTimeout = -time.Second; return c }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.driver, tt.conn, tt.cfg); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}

	badDriver := &testDriver{descriptor: validTestDescriptor()}
	badDriver.descriptor.ID = "INVALID"
	if _, err := New(badDriver, validConn, validCfg); err == nil {
		t.Fatal("New(invalid descriptor) error = nil")
	}

	c := newMemoryConn(1)
	d := &testDriver{descriptor: validTestDescriptor()}
	if _, err := New(d, c, validCfg); err != nil {
		t.Fatalf("New(valid) error = %v", err)
	}
	if sends, receives := c.calls(); sends != 0 || receives != 0 || d.runCalls.Load() != 0 {
		t.Fatalf("before Run: sends=%d receives=%d driver runs=%d", sends, receives, d.runCalls.Load())
	}
}

func TestRuntimeHandshakeOrderAndValues(t *testing.T) {
	c := newMemoryConn(4)
	driverStarted := make(chan pluginapi.Host, 1)
	d := &testDriver{descriptor: validTestDescriptor(), run: func(_ context.Context, host pluginapi.Host) error {
		driverStarted <- host
		return nil
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runtime.Run(context.Background()) }()

	helloMessage := receiveMessage(t, c)
	hello, ok := helloMessage.Payload.(protocol.Hello)
	if !ok {
		t.Fatalf("first payload = %T, want protocol.Hello", helloMessage.Payload)
	}
	if hello.Token != "runtime-token" || hello.Descriptor != d.descriptor || hello.ProtocolMin != protocol.Version || hello.ProtocolMax != protocol.Version {
		t.Fatalf("hello = %+v", hello)
	}
	select {
	case <-driverStarted:
		t.Fatal("driver started before Initialize and Ready")
	default:
	}
	c.toRuntime <- mustMessage(t, protocol.Initialize{Startup: validTestStartup()})
	if got := receiveMessage(t, c); got.Type != protocol.MessageReady {
		t.Fatalf("message after Initialize = %v, want Ready", got.Type)
	}
	select {
	case <-driverStarted:
	case <-time.After(testTimeout):
		t.Fatal("driver did not start after Ready")
	}
	if err := awaitResult(t, done); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if c.closeCalls.Load() != 1 {
		t.Fatalf("Close calls = %d, want 1", c.closeCalls.Load())
	}
}

func TestRuntimeStartupSnapshotIsImmutableAndCurrentStateDrivesPublication(t *testing.T) {
	c := newMemoryConn(8)
	hostReady := make(chan pluginapi.Host, 1)
	release := make(chan struct{})
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		hostReady <- host
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	startup := validTestStartup()
	wantInitial := cloneStartup(startup)
	done := startHandshake(t, runtime, c, startup)
	host := <-hostReady
	startup.Config.Data[2] = 'X'
	first := host.Startup()
	if string(first.Config.Data) != `{"gain":1}` {
		t.Fatalf("Startup config = %q", first.Config.Data)
	}
	first.Config.Data[2] = 'Y'
	if got := string(host.Startup().Config.Data); got != `{"gain":1}` {
		t.Fatalf("defensive snapshot config = %q", got)
	}

	update := pluginapi.Config{Revision: 2, Data: json.RawMessage(`{"gain":2}`)}
	c.toRuntime <- mustMessage(t, protocol.ConfigChanged{Config: update})
	currentSubscription := pluginapi.Subscription{
		Generation:   2,
		Capabilities: trackingmodel.CapabilityExpression,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: false})
	c.toRuntime <- mustMessage(t, protocol.SubscriptionChanged{Subscription: currentSubscription})
	for i := 0; i < 3; i++ {
		select {
		case <-host.Events():
		case <-time.After(testTimeout):
			t.Fatalf("control event %d not delivered", i)
		}
	}
	update.Data[2] = 'Z'
	if got := host.Startup(); !reflect.DeepEqual(got, wantInitial) {
		t.Fatalf("Startup() after controls = %+v, want immutable initialization snapshot %+v", got, wantInitial)
	}
	if host.PublishFrame(trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityExpression}) {
		t.Fatal("PublishFrame() accepted a frame while current active state was false")
	}

	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: true})
	select {
	case <-host.Events():
	case <-time.After(testTimeout):
		t.Fatal("reactivation event not delivered")
	}
	frame := trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityExpression}
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.75)
	if !host.PublishFrame(frame) {
		t.Fatal("PublishFrame() rejected a frame after current state was reactivated")
	}
	pending, ok := host.(*runtimeHost).frames.Load()
	if !ok || pending.Generation != currentSubscription.Generation || pending.Subscription != currentSubscription || pending.Frame != frame {
		t.Fatalf("pending frame = (%+v, %t), want current subscription snapshot and frame", pending, ok)
	}
	if got := host.Startup(); !reflect.DeepEqual(got, wantInitial) {
		t.Fatalf("Startup() after publication = %+v, want immutable initialization snapshot %+v", got, wantInitial)
	}
	close(release)
	if err := awaitResult(t, done); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRuntimeDeliversControlsInWireOrderAndSuppressesDuplicates(t *testing.T) {
	c := newMemoryConn(16)
	hostReady := make(chan pluginapi.Host, 1)
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		hostReady <- host
		<-ctx.Done()
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	host := <-hostReady

	config := pluginapi.Config{Revision: 2, Data: json.RawMessage(`{"gain":2}`)}
	subscription := pluginapi.Subscription{
		Generation: 2, Capabilities: trackingmodel.CapabilityEye,
		Eye:         trackingmodel.EyeValidRightGaze,
		Expressions: trackingmodel.ExpressionMaskOf(1), // normalized away.
	}
	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: false})
	c.toRuntime <- mustMessage(t, protocol.ConfigChanged{Config: config})
	c.toRuntime <- mustMessage(t, protocol.SubscriptionChanged{Subscription: subscription})
	// Idempotent duplicates must not produce events.
	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: false})
	c.toRuntime <- mustMessage(t, protocol.ConfigChanged{Config: config.Clone()})
	normalizedEquivalent := subscription
	normalizedEquivalent.Expressions = trackingmodel.ExpressionMaskOf(2)
	c.toRuntime <- mustMessage(t, protocol.SubscriptionChanged{Subscription: normalizedEquivalent})

	want := []pluginapi.ControlEvent{
		pluginapi.ActiveChanged{Active: false},
		pluginapi.ConfigChanged{Config: config},
		pluginapi.SubscriptionChanged{Subscription: subscription.Normalize()},
	}
	for i, expected := range want {
		select {
		case got := <-host.Events():
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("event %d = %#v, want %#v", i, got, expected)
			}
		case <-time.After(testTimeout):
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
	select {
	case extra := <-host.Events():
		t.Fatalf("unexpected duplicate event %#v", extra)
	case <-time.After(30 * time.Millisecond):
	}
	c.toRuntime <- mustMessage(t, protocol.Shutdown{})
	if got := <-host.Events(); !reflect.DeepEqual(got, pluginapi.ShutdownRequested{}) {
		t.Fatalf("shutdown event = %#v", got)
	}
	if err := awaitResult(t, done); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRuntimeRejectsConflictingAndRegressiveVersions(t *testing.T) {
	tests := []struct {
		name    string
		payload any
	}{
		{"conflicting config", protocol.ConfigChanged{Config: pluginapi.Config{Revision: 1, Data: json.RawMessage(`{"other":1}`)}}},
		{"regressive config", protocol.ConfigChanged{Config: pluginapi.Config{Revision: 0}}},
		{"conflicting subscription", protocol.SubscriptionChanged{Subscription: pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye, Eye: trackingmodel.EyeValidRightGaze}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMemoryConn(8)
			d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, _ pluginapi.Host) error { <-ctx.Done(); return ctx.Err() }}
			runtime, err := New(d, c, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			done := startHandshake(t, runtime, c, validTestStartup())
			// Construct the regressive config manually because protocol validation forbids revision zero updates.
			if tt.name == "regressive config" {
				c.toRuntime <- protocol.Message{Version: protocol.Version, Type: protocol.MessageConfigChanged, Payload: tt.payload}
			} else {
				c.toRuntime <- mustMessage(t, tt.payload)
			}
			err = awaitResult(t, done)
			if err == nil {
				t.Fatal("Run() error = nil")
			}
			if got := receiveMessage(t, c); got.Type != protocol.MessageError || got.Payload.(protocol.Error).Code != "protocol_error" {
				t.Fatalf("error message = %#v", got)
			}
		})
	}

	t.Run("regressive subscription", func(t *testing.T) {
		c := newMemoryConn(8)
		hostReady := make(chan pluginapi.Host, 1)
		d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
			hostReady <- host
			<-ctx.Done()
			return ctx.Err()
		}}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		done := startHandshake(t, runtime, c, validTestStartup())
		host := <-hostReady
		up := pluginapi.Subscription{Generation: 2, Capabilities: trackingmodel.CapabilityEye}
		c.toRuntime <- mustMessage(t, protocol.SubscriptionChanged{Subscription: up})
		<-host.Events()
		c.toRuntime <- mustMessage(t, protocol.SubscriptionChanged{Subscription: validTestStartup().Subscription})
		if err := awaitResult(t, done); err == nil {
			t.Fatal("Run() error = nil")
		}
	})
}

func TestRuntimeRejectsUnexpectedMessages(t *testing.T) {
	t.Run("first message", func(t *testing.T) {
		c := newMemoryConn(4)
		d := &testDriver{descriptor: validTestDescriptor()}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- runtime.Run(context.Background()) }()
		_ = receiveMessage(t, c)
		c.toRuntime <- mustMessage(t, protocol.Ready{})
		if err := awaitResult(t, done); err == nil {
			t.Fatal("Run() error = nil")
		}
		if d.runCalls.Load() != 0 {
			t.Fatal("driver ran after invalid handshake")
		}
	})

	t.Run("after ready", func(t *testing.T) {
		c := newMemoryConn(8)
		d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, _ pluginapi.Host) error { <-ctx.Done(); return ctx.Err() }}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		done := startHandshake(t, runtime, c, validTestStartup())
		c.toRuntime <- mustMessage(t, protocol.Heartbeat{})
		if err := awaitResult(t, done); err == nil {
			t.Fatal("Run() error = nil")
		}
	})
}

func TestRuntimeControlBackpressure(t *testing.T) {
	c := newMemoryConn(8)
	cfg := testConfig()
	cfg.ControlQueue = 1
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, _ pluginapi.Host) error { <-ctx.Done(); return ctx.Err() }}
	runtime, err := New(d, c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: false})
	c.toRuntime <- mustMessage(t, protocol.ConfigChanged{Config: pluginapi.Config{Revision: 2}})
	if err := awaitResult(t, done); !errors.Is(err, ErrControlBackpressure) {
		t.Fatalf("Run() error = %v, want ErrControlBackpressure", err)
	}
}

func TestRuntimeShutdownLifecycle(t *testing.T) {
	c := newMemoryConn(8)
	hostReady := make(chan pluginapi.Host, 1)
	canceled := make(chan struct{})
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		hostReady <- host
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	host := <-hostReady
	c.toRuntime <- mustMessage(t, protocol.Shutdown{})
	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: false})
	select {
	case got := <-host.Events():
		if !reflect.DeepEqual(got, pluginapi.ShutdownRequested{}) {
			t.Fatalf("shutdown event = %#v", got)
		}
	case <-time.After(testTimeout):
		t.Fatal("shutdown event not delivered")
	}
	select {
	case <-canceled:
	case <-time.After(testTimeout):
		t.Fatal("driver context not canceled")
	}
	if err := awaitResult(t, done); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := receiveMessage(t, c); got.Type != protocol.MessageShutdownAck {
		t.Fatalf("final message = %v, want ShutdownAck", got.Type)
	}
	awaitClosed(t, host.Events())
	if c.closeCalls.Load() != 1 {
		t.Fatalf("Close calls = %d, want 1", c.closeCalls.Load())
	}
	if _, receives := c.calls(); receives != 2 {
		t.Fatalf("Receive calls = %d, want initialization plus shutdown only", receives)
	}
}

func TestRuntimeAcceptedShutdownOwnsCompletion(t *testing.T) {
	// Multiple independent runs amplify the terminal race without timing sleeps:
	// every driver return is causally after it consumes ShutdownRequested.
	for attempt := 0; attempt < 100; attempt++ {
		c := newMemoryConn(8)
		d := &testDriver{descriptor: validTestDescriptor(), run: func(_ context.Context, host pluginapi.Host) error {
			if event := <-host.Events(); !reflect.DeepEqual(event, pluginapi.ShutdownRequested{}) {
				return errors.New("driver received non-shutdown event")
			}
			return nil
		}}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		done := startHandshake(t, runtime, c, validTestStartup())
		c.toRuntime <- mustMessage(t, protocol.Shutdown{})
		if err := awaitResult(t, done); err != nil {
			t.Fatalf("attempt %d Run() error = %v", attempt, err)
		}
		select {
		case message := <-c.fromRuntime:
			if message.Type != protocol.MessageShutdownAck {
				t.Fatalf("attempt %d final message = %#v, want ShutdownAck", attempt, message)
			}
		default:
			t.Fatalf("attempt %d accepted shutdown returned without ShutdownAck", attempt)
		}
	}
}

func TestRuntimeShutdownAckSendFailureReportsProtocolError(t *testing.T) {
	c := newMemoryConn(8)
	ackErr := errors.New("shutdown ack send failed")
	c.sendErrAt, c.sendErr = 3, ackErr
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		if event := <-host.Events(); !reflect.DeepEqual(event, pluginapi.ShutdownRequested{}) {
			return errors.New("driver received non-shutdown event")
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	c.toRuntime <- mustMessage(t, protocol.Shutdown{})
	if err := awaitResult(t, done); !errors.Is(err, ackErr) {
		t.Fatalf("Run() error = %v, want ack send error", err)
	}
	got := receiveMessage(t, c)
	if got.Type != protocol.MessageError || got.Payload.(protocol.Error).Code != "protocol_error" {
		t.Fatalf("ack failure notice = %#v, want protocol_error", got)
	}
}

func TestRuntimePostReadyReceiveFailureCancelsDriver(t *testing.T) {
	c := newMemoryConn(8)
	receiveErr := errors.New("control receive failed")
	driverCanceled := make(chan struct{})
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, _ pluginapi.Host) error {
		<-ctx.Done()
		close(driverCanceled)
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	c.failNextReceive(receiveErr)
	// Unblock an already-waiting Receive so it observes the injected failure on
	// its next iteration without relying on a sleep.
	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: false})
	if err := awaitResult(t, done); !errors.Is(err, receiveErr) {
		t.Fatalf("Run() error = %v, want receive error", err)
	}
	select {
	case <-driverCanceled:
	case <-time.After(testTimeout):
		t.Fatal("post-Ready receive failure did not cancel driver")
	}
}

func TestRuntimeSpontaneousContextCanceledIsDriverError(t *testing.T) {
	c := newMemoryConn(8)
	d := &testDriver{descriptor: validTestDescriptor(), run: func(context.Context, pluginapi.Host) error {
		return context.Canceled
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	if err := awaitResult(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want spontaneous context.Canceled", err)
	}
	got := receiveMessage(t, c)
	if got.Type != protocol.MessageError || got.Payload.(protocol.Error).Code != "driver_error" {
		t.Fatalf("driver notice = %#v, want driver_error", got)
	}
}

func TestRuntimeReadySendFailureDoesNotStartDriver(t *testing.T) {
	c := newMemoryConn(8)
	readyErr := errors.New("ready send failed")
	c.sendErrAt, c.sendErr = 2, readyErr
	d := &testDriver{descriptor: validTestDescriptor()}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runtime.Run(context.Background()) }()
	if got := receiveMessage(t, c); got.Type != protocol.MessageHello {
		t.Fatalf("first message = %v, want Hello", got.Type)
	}
	c.toRuntime <- mustMessage(t, protocol.Initialize{Startup: validTestStartup()})
	if err := awaitResult(t, done); !errors.Is(err, readyErr) {
		t.Fatalf("Run() error = %v, want Ready send error", err)
	}
	if d.runCalls.Load() != 0 {
		t.Fatalf("driver Run calls = %d, want 0", d.runCalls.Load())
	}
	got := receiveMessage(t, c)
	if got.Type != protocol.MessageError || got.Payload.(protocol.Error).Code != "protocol_error" {
		t.Fatalf("Ready failure notice = %#v, want protocol_error", got)
	}
}

func TestRuntimeOutboundFailuresCancelDriverAndReportProtocolError(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*RuntimeConfig)
		publish   func(pluginapi.Host)
	}{
		{
			name: "frame",
			publish: func(host pluginapi.Host) {
				if !host.PublishFrame(trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityEye}) {
					panic("test frame was rejected")
				}
			},
		},
		{name: "status", publish: func(host pluginapi.Host) { host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady}) }},
		{name: "log", publish: func(host pluginapi.Host) { host.Log(pluginapi.LogInfo, "writer failure") }},
		{name: "heartbeat", configure: func(cfg *RuntimeConfig) { cfg.HeartbeatInterval = time.Millisecond }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMemoryConn(8)
			sendErr := errors.New(tt.name + " send failed")
			c.sendErrAt, c.sendErr = 3, sendErr
			driverCanceled := make(chan struct{})
			d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
				if tt.publish != nil {
					tt.publish(host)
				}
				<-ctx.Done()
				close(driverCanceled)
				return ctx.Err()
			}}
			cfg := testConfig()
			if tt.configure != nil {
				tt.configure(&cfg)
			}
			runtime, err := New(d, c, cfg)
			if err != nil {
				t.Fatal(err)
			}
			done := startHandshake(t, runtime, c, validTestStartup())
			if err := awaitResult(t, done); !errors.Is(err, sendErr) {
				t.Fatalf("Run() error = %v, want outbound send error", err)
			}
			select {
			case <-driverCanceled:
			case <-time.After(testTimeout):
				t.Fatal("outbound failure did not cancel driver")
			}
			message := receiveMessage(t, c)
			if message.Type != protocol.MessageError || message.Payload.(protocol.Error).Code != "protocol_error" {
				t.Fatalf("terminal notice = %#v, want protocol_error", message)
			}
		})
	}
}

func TestRuntimeTypedConnRejectsOversizedOutboundPayloadBeforeSend(t *testing.T) {
	c := newMemoryConn(8)
	fallbackErr := errors.New("typed connection accepted oversized payload")
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		host.Log(pluginapi.LogInfo, strings.Repeat("x", protocol.MaxPayloadSize))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return fallbackErr
		}
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	err = awaitResult(t, done)
	if errors.Is(err, fallbackErr) || err == nil || !strings.Contains(err.Error(), "encoded payload size") {
		t.Fatalf("Run() error = %v, want typed payload size rejection", err)
	}
	message := receiveMessage(t, c)
	if message.Type != protocol.MessageError || message.Payload.(protocol.Error).Code != "protocol_error" {
		t.Fatalf("terminal message = %#v, want protocol_error without oversized Log send", message)
	}
}

func TestRuntimeExternalCancellationPreservesAndReportsConcurrentWriterError(t *testing.T) {
	c := newMemoryConn(8)
	c.postReadySendStarted = make(chan struct{})
	writerErr := errors.New("writer failed while external cancellation won")
	c.sendErrAfterCancelAt, c.sendErrAfterCancel = 3, writerErr
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		host.Log(pluginapi.LogInfo, "block writer until cancellation")
		<-ctx.Done()
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := startHandshakeContext(t, runtime, c, validTestStartup(), ctx)
	select {
	case <-c.postReadySendStarted:
	case <-time.After(testTimeout):
		t.Fatal("writer send did not start")
	}
	cancel()
	err = awaitResult(t, done)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, writerErr) {
		t.Fatalf("Run() error = %v, want external cancellation joined with writer error", err)
	}
	notices := runtimeNotices(t, c)
	requireNotice(t, notices, "protocol_error", writerErr.Error(), 1)
	if got := c.maxSends.Load(); got != 1 {
		t.Fatalf("maximum concurrent Conn.Send calls = %d, want 1", got)
	}
}

func TestRuntimeReaderFailurePreservesAndReportsConcurrentWriterError(t *testing.T) {
	c := newMemoryConn(8)
	c.postReadySendStarted = make(chan struct{})
	writerErr := errors.New("writer failed after reader cancellation")
	readerErr := errors.New("reader failed while writer was blocked")
	c.sendErrAfterCancelAt, c.sendErrAfterCancel = 3, writerErr
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		host.Log(pluginapi.LogInfo, "block writer until reader fails")
		<-ctx.Done()
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	select {
	case <-c.postReadySendStarted:
	case <-time.After(testTimeout):
		t.Fatal("writer send did not start")
	}
	c.failNextReceive(readerErr)
	c.toRuntime <- mustMessage(t, protocol.ActiveChanged{Active: false})
	err = awaitResult(t, done)
	if !errors.Is(err, readerErr) || !errors.Is(err, writerErr) {
		t.Fatalf("Run() error = %v, want reader and writer errors", err)
	}
	notices := runtimeNotices(t, c)
	requireNotice(t, notices, "protocol_error", readerErr.Error(), 1)
	requireNotice(t, notices, "protocol_error", writerErr.Error(), 1)
	if got := c.maxSends.Load(); got != 1 {
		t.Fatalf("maximum concurrent Conn.Send calls = %d, want 1", got)
	}
}

func TestRuntimeCollectsDriverAndReaderFailuresInBothTerminalOrders(t *testing.T) {
	tests := []struct {
		name        string
		driverFirst bool
	}{
		{name: "driver selected first", driverFirst: true},
		{name: "reader selected first", driverFirst: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := newMemoryConn(16)
			driverErr := errors.New("simultaneous driver failure")
			readerErr := errors.New("simultaneous reader failure")
			conn := &orderedTerminalConn{
				memoryConn:    base,
				readerStarted: make(chan struct{}),
				releaseReader: make(chan struct{}),
				readerErr:     readerErr,
				afterCancel:   tt.driverFirst,
			}
			releaseDriver := make(chan struct{})
			d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, _ pluginapi.Host) error {
				if tt.driverFirst {
					<-conn.readerStarted
					<-releaseDriver
				} else {
					<-ctx.Done()
				}
				return driverErr
			}}
			runtime, err := New(d, conn, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			done := startHandshake(t, runtime, base, validTestStartup())
			<-conn.readerStarted
			if tt.driverFirst {
				close(releaseDriver)
			} else {
				close(conn.releaseReader)
			}

			err = awaitResult(t, done)
			if !errors.Is(err, driverErr) || !errors.Is(err, readerErr) {
				t.Fatalf("Run() error = %v, want joined driver and reader failures", err)
			}
			notices := runtimeNotices(t, base)
			requireNotice(t, notices, "driver_error", driverErr.Error(), 1)
			requireNotice(t, notices, "protocol_error", readerErr.Error(), 1)
		})
	}
}

func TestRuntimeWriterFailurePreservesAndReportsConcurrentDriverError(t *testing.T) {
	c := newMemoryConn(8)
	writerErr := errors.New("writer failed before canceling driver")
	driverErr := errors.New("driver failed during writer cancellation")
	c.sendErrAt, c.sendErr = 3, writerErr
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		host.Log(pluginapi.LogInfo, "fail writer")
		<-ctx.Done()
		return driverErr
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	err = awaitResult(t, done)
	if !errors.Is(err, writerErr) || !errors.Is(err, driverErr) {
		t.Fatalf("Run() error = %v, want writer primary joined with driver error", err)
	}
	notices := runtimeNotices(t, c)
	requireNotice(t, notices, "protocol_error", writerErr.Error(), 1)
	requireNotice(t, notices, "driver_error", driverErr.Error(), 1)
	if got := c.maxSends.Load(); got != 1 {
		t.Fatalf("maximum concurrent Conn.Send calls = %d, want 1", got)
	}
}

func TestRuntimeWriterPrimaryCollectsAndReportsConcurrentReaderFailure(t *testing.T) {
	c := newMemoryConn(8)
	runtime := &Runtime{conn: c, config: testConfig()}
	writerErr := errors.New("writer primary failure")
	readerErr := errors.New("reader failed during writer shutdown")
	outcome := terminalOutcome{primary: terminalWriter}
	outcome.record(terminalEvent{worker: terminalWriter, writerErr: writerErr}, false)
	outcome.record(terminalEvent{worker: terminalDriver, driver: driverResult{err: context.Canceled}}, false)
	outcome.record(terminalEvent{worker: terminalReader, reader: readerResult{err: readerErr}}, false)

	err := runtime.finishTerminal(nil, outcome)
	if !errors.Is(err, writerErr) || !errors.Is(err, readerErr) {
		t.Fatalf("finishTerminal() error = %v, want writer primary joined with reader error", err)
	}
	notices := runtimeNotices(t, c)
	requireNotice(t, notices, "protocol_error", writerErr.Error(), 1)
	requireNotice(t, notices, "protocol_error", readerErr.Error(), 1)
	if got := c.maxSends.Load(); got != 1 {
		t.Fatalf("maximum concurrent Conn.Send calls = %d, want 1", got)
	}
}

func TestRuntimeWriterPrimaryHonorsAcceptedShutdown(t *testing.T) {
	c := newMemoryConn(8)
	runtime := &Runtime{conn: c, config: testConfig()}
	writerErr := errors.New("writer failed during accepted shutdown")
	outcome := terminalOutcome{primary: terminalWriter}
	outcome.record(terminalEvent{worker: terminalWriter, writerErr: writerErr}, false)
	outcome.record(terminalEvent{worker: terminalDriver, driver: driverResult{err: context.Canceled, shutdownAccepted: true}}, false)
	outcome.record(terminalEvent{worker: terminalReader, reader: readerResult{shutdown: true}}, false)

	err := runtime.finishTerminal(nil, outcome)
	if !errors.Is(err, writerErr) {
		t.Fatalf("finishTerminal() error = %v, want writer error", err)
	}
	var writerNotices, acknowledgements int
	for {
		select {
		case message := <-c.fromRuntime:
			switch payload := message.Payload.(type) {
			case protocol.Error:
				if payload.Code == "protocol_error" && strings.Contains(payload.Message, writerErr.Error()) {
					writerNotices++
				}
			case protocol.ShutdownAck:
				acknowledgements++
			default:
				t.Fatalf("terminal payload = %T, want writer notice or ShutdownAck", payload)
			}
		default:
			if writerNotices != 1 || acknowledgements != 1 {
				t.Fatalf("writer notices=%d acknowledgements=%d, want exactly one each", writerNotices, acknowledgements)
			}
			if got := c.maxSends.Load(); got != 1 {
				t.Fatalf("maximum concurrent Conn.Send calls = %d, want 1", got)
			}
			return
		}
	}
}

func TestRuntimeNeverCallsConnSendConcurrently(t *testing.T) {
	c := newMemoryConn(8)
	c.sendDelay = 100 * time.Millisecond
	c.postReadySendStarted = make(chan struct{})
	driverErr := errors.New("driver stopped after writer send began")
	missedWriter := errors.New("writer did not send")
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, host pluginapi.Host) error {
		host.Log(pluginapi.LogInfo, "force a writer send")
		select {
		case <-c.postReadySendStarted:
			return driverErr
		case <-time.After(50 * time.Millisecond):
			return missedWriter
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	if err := awaitResult(t, done); !errors.Is(err, driverErr) {
		t.Fatalf("Run() error = %v, want driver error after writer started", err)
	}
	if got := c.maxSends.Load(); got != 1 {
		t.Fatalf("maximum concurrent Conn.Send calls = %d, want 1", got)
	}
}

func TestRuntimeShutdownTimeoutIsBounded(t *testing.T) {
	c := newMemoryConn(8)
	release := make(chan struct{})
	hostReady := make(chan pluginapi.Host, 1)
	d := &testDriver{descriptor: validTestDescriptor(), run: func(_ context.Context, host pluginapi.Host) error { hostReady <- host; <-release; return nil }}
	cfg := testConfig()
	cfg.ShutdownTimeout = 40 * time.Millisecond
	runtime, err := New(d, c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	<-hostReady
	started := time.Now()
	c.toRuntime <- mustMessage(t, protocol.Shutdown{})
	err = awaitResult(t, done)
	close(release)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("Run() error = %v, want ErrShutdownTimeout", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("shutdown took %v", elapsed)
	}
	if got := receiveMessage(t, c); got.Type != protocol.MessageShutdownAck {
		t.Fatalf("message = %v, want ShutdownAck", got.Type)
	}
}

func TestRuntimeClosesOnceToBoundReceiveThatIgnoresCancellation(t *testing.T) {
	base := newMemoryConn(8)
	closeErr := errors.New("close after shutdown deadline failed")
	base.closeErr = closeErr
	conn := &closeOnlyReceiveConn{
		memoryConn:    base,
		readerStarted: make(chan struct{}),
	}
	driverErr := errors.New("driver stopped while reader ignored cancellation")
	d := &testDriver{descriptor: validTestDescriptor(), run: func(context.Context, pluginapi.Host) error {
		<-conn.readerStarted
		return driverErr
	}}
	cfg := testConfig()
	cfg.ShutdownTimeout = 40 * time.Millisecond
	runtime, err := New(d, conn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, base, validTestStartup())
	started := time.Now()
	select {
	case err = <-done:
	case <-time.After(500 * time.Millisecond):
		_ = base.Close()
		t.Fatal("Run() did not close a Receive implementation that ignored context cancellation")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Run() took %v, want bounded shutdown", elapsed)
	}
	if !errors.Is(err, driverErr) || !errors.Is(err, ErrShutdownTimeout) || !errors.Is(err, closeErr) {
		t.Fatalf("Run() error = %v, want driver, shutdown-timeout, and close errors", err)
	}
	if got := base.closeCalls.Load(); got != 1 {
		t.Fatalf("Close calls = %d, want exactly 1", got)
	}
}

func TestRuntimeDriverReturns(t *testing.T) {
	realErr := errors.New("driver exploded")
	tests := []struct {
		name       string
		run        func(context.Context, pluginapi.Host) error
		wantErr    error
		wantNotice bool
	}{
		{"nil", func(context.Context, pluginapi.Host) error { return nil }, nil, false},
		{"context canceled after coordinated cancellation", func(ctx context.Context, _ pluginapi.Host) error { <-ctx.Done(); return context.Canceled }, nil, false},
		{"real error", func(context.Context, pluginapi.Host) error { return realErr }, realErr, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMemoryConn(8)
			d := &testDriver{descriptor: validTestDescriptor(), run: tt.run}
			runtime, err := New(d, c, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			done := startHandshake(t, runtime, c, validTestStartup())
			if tt.name == "context canceled after coordinated cancellation" {
				c.toRuntime <- mustMessage(t, protocol.Shutdown{})
			}
			err = awaitResult(t, done)
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantNotice {
				if got := receiveMessage(t, c); got.Type != protocol.MessageError || got.Payload.(protocol.Error).Code != "driver_error" {
					t.Fatalf("notice = %#v", got)
				}
			}
		})
	}
}

func TestRuntimeConnectionFailuresAndPrimaryPreservation(t *testing.T) {
	t.Run("receive failure", func(t *testing.T) {
		c := newMemoryConn(8)
		receiveErr := errors.New("receive failed")
		c.receiveErr = receiveErr
		d := &testDriver{descriptor: validTestDescriptor()}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		err = runtime.Run(context.Background())
		if !errors.Is(err, receiveErr) {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("send failure", func(t *testing.T) {
		c := newMemoryConn(8)
		sendErr := errors.New("send failed")
		c.sendErrAt, c.sendErr = 1, sendErr
		d := &testDriver{descriptor: validTestDescriptor()}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		err = runtime.Run(context.Background())
		if !errors.Is(err, sendErr) {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("close joins primary", func(t *testing.T) {
		c := newMemoryConn(8)
		primary := errors.New("receive failed")
		closeErr := errors.New("close failed")
		c.receiveErr, c.closeErr = primary, closeErr
		d := &testDriver{descriptor: validTestDescriptor()}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		err = runtime.Run(context.Background())
		if !errors.Is(err, primary) || !errors.Is(err, closeErr) {
			t.Fatalf("Run() error = %v, want joined errors", err)
		}
	})

	t.Run("close alone", func(t *testing.T) {
		c := newMemoryConn(8)
		closeErr := errors.New("close failed")
		c.closeErr = closeErr
		d := &testDriver{descriptor: validTestDescriptor()}
		runtime, err := New(d, c, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		done := startHandshake(t, runtime, c, validTestStartup())
		if err := awaitResult(t, done); !errors.Is(err, closeErr) {
			t.Fatalf("Run() error = %v", err)
		}
	})
}

func TestRuntimeExternalCancellationAndSingleUse(t *testing.T) {
	c := newMemoryConn(8)
	driverStarted := make(chan struct{})
	d := &testDriver{descriptor: validTestDescriptor(), run: func(ctx context.Context, _ pluginapi.Host) error {
		close(driverStarted)
		<-ctx.Done()
		return ctx.Err()
	}}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { first <- runtime.Run(ctx) }()
	_ = receiveMessage(t, c)
	c.toRuntime <- mustMessage(t, protocol.Initialize{Startup: validTestStartup()})
	_ = receiveMessage(t, c)
	<-driverStarted
	second := make(chan error, 1)
	go func() { second <- runtime.Run(context.Background()) }()
	if err := awaitResult(t, second); !errors.Is(err, ErrRuntimeAlreadyRun) {
		t.Fatalf("second Run error = %v", err)
	}
	cancel()
	if err := awaitResult(t, first); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Run error = %v", err)
	}
	if err := runtime.Run(context.Background()); !errors.Is(err, ErrRuntimeAlreadyRun) {
		t.Fatalf("third Run error = %v", err)
	}
}

func TestMainFactoryPaths(t *testing.T) {
	original := connect
	t.Cleanup(func() { connect = original })

	d := &testDriver{descriptor: validTestDescriptor()}
	connect = func(context.Context) (protocol.Conn, string, error) { return nil, "", ErrConnectionUnavailable }
	if err := Main(d); !errors.Is(err, ErrConnectionUnavailable) {
		t.Fatalf("Main() error = %v", err)
	}

	factoryErr := errors.New("factory failed")
	connect = func(context.Context) (protocol.Conn, string, error) { return nil, "", factoryErr }
	if err := Main(d); !errors.Is(err, factoryErr) {
		t.Fatalf("Main() error = %v", err)
	}

	c := newMemoryConn(8)
	c.toRuntime <- mustMessage(t, protocol.Initialize{Startup: validTestStartup()})
	d = &testDriver{descriptor: validTestDescriptor()}
	connect = func(context.Context) (protocol.Conn, string, error) { return c, "factory-token", nil }
	if err := Main(d); err != nil {
		t.Fatalf("Main() error = %v", err)
	}
	hello := (<-c.fromRuntime).Payload.(protocol.Hello)
	if hello.Token != "factory-token" {
		t.Fatalf("hello token = %q", hello.Token)
	}
	if got := (<-c.fromRuntime).Type; got != protocol.MessageReady {
		t.Fatalf("message = %v, want Ready", got)
	}
}

func TestRuntimePostStopPublicationMethodsAreConcurrencySafe(t *testing.T) {
	c := newMemoryConn(8)
	hostReady := make(chan pluginapi.Host, 1)
	release := make(chan struct{})
	d := &testDriver{descriptor: validTestDescriptor(), run: func(_ context.Context, host pluginapi.Host) error { hostReady <- host; <-release; return nil }}
	runtime, err := New(d, c, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := startHandshake(t, runtime, c, validTestStartup())
	host := <-hostReady

	const publishers = 32
	var started sync.WaitGroup
	started.Add(publishers)
	var wg sync.WaitGroup
	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !host.PublishFrame(trackingmodel.TrackingFrame{}) {
				t.Error("PublishFrame was rejected before stop began")
				started.Done()
				return
			}
			started.Done()
			for {
				host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady})
				host.Log(pluginapi.LogInfo, "overlap stop transition")
				_ = host.Startup()
				if !host.PublishFrame(trackingmodel.TrackingFrame{}) {
					if _, ok := <-host.Events(); ok {
						t.Error("event channel remained open after stopped state became visible")
					}
					host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady})
					host.Log(pluginapi.LogInfo, "post-stop no-op")
					return
				}
				goruntime.Gosched()
			}
		}()
	}
	started.Wait()
	close(release)
	if err := awaitResult(t, done); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wg.Wait()
	if host.PublishFrame(trackingmodel.TrackingFrame{}) {
		t.Fatal("post-stop PublishFrame returned true")
	}
	if got := c.maxSends.Load(); got != 1 {
		t.Fatalf("maximum concurrent Conn.Send calls = %d, want 1", got)
	}
}

func TestErrorsAreNamedAndDescriptive(t *testing.T) {
	for _, err := range []error{ErrControlBackpressure, ErrRuntimeAlreadyRun, ErrShutdownTimeout, ErrConnectionUnavailable} {
		if err == nil || !strings.Contains(err.Error(), "pluginruntime:") {
			t.Fatalf("named error = %v", err)
		}
	}
}
