package pluginruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
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

	mu           sync.Mutex
	sendCalls    int
	receiveCalls int
	sendErrAt    int
	sendErr      error
	receiveErr   error
	closeErr     error
}

func newMemoryConn(capacity int) *memoryConn {
	return &memoryConn{
		toRuntime:   make(chan protocol.Message, capacity),
		fromRuntime: make(chan protocol.Message, capacity),
		closed:      make(chan struct{}),
	}
}

func (c *memoryConn) Send(ctx context.Context, message protocol.Message) error {
	c.mu.Lock()
	c.sendCalls++
	call := c.sendCalls
	errAt, sendErr := c.sendErrAt, c.sendErr
	c.mu.Unlock()
	if errAt != 0 && call == errAt {
		return sendErr
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
	done := make(chan error, 1)
	go func() { done <- runtime.Run(context.Background()) }()
	if got := receiveMessage(t, c); got.Type != protocol.MessageHello {
		t.Fatalf("first message type = %v, want Hello", got.Type)
	}
	c.toRuntime <- mustMessage(t, protocol.Initialize{Startup: startup})
	if got := receiveMessage(t, c); got.Type != protocol.MessageReady {
		t.Fatalf("second message type = %v, want Ready", got.Type)
	}
	return done
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

	tests := []struct {
		name   string
		driver pluginapi.Driver
		conn   protocol.Conn
		cfg    RuntimeConfig
	}{
		{"nil driver", nil, validConn, validCfg},
		{"nil conn", validDriver, nil, validCfg},
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

func TestRuntimeStartupSnapshotsAreDefensiveAndAtomic(t *testing.T) {
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
	select {
	case <-host.Events():
	case <-time.After(testTimeout):
		t.Fatal("config event not delivered")
	}
	update.Data[2] = 'Z'
	if got := host.Startup(); got.Config.Revision != 2 || string(got.Config.Data) != `{"gain":2}` {
		t.Fatalf("updated Startup = %+v", got)
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

func TestTransitionalPublicationMethodsAreConcurrencySafe(t *testing.T) {
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
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if host.PublishFrame(trackingmodel.TrackingFrame{}) {
				t.Error("transitional PublishFrame returned true")
			}
			host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady})
			host.Log(pluginapi.LogInfo, "safe transitional no-op")
			_ = host.Startup()
			_ = host.Events()
		}()
	}
	wg.Wait()
	close(release)
	if err := awaitResult(t, done); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sends, _ := c.calls(); sends != 2 {
		t.Fatalf("transitional publications sent messages: total sends=%d, want handshake-only 2", sends)
	}
}

func TestErrorsAreNamedAndDescriptive(t *testing.T) {
	for _, err := range []error{ErrControlBackpressure, ErrRuntimeAlreadyRun, ErrShutdownTimeout, ErrConnectionUnavailable} {
		if err == nil || !strings.Contains(err.Error(), "pluginruntime:") {
			t.Fatalf("named error = %v", err)
		}
	}
}
