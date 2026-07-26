package plugins

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

const handshakeTestTimeout = time.Second

type handshakeCall struct {
	result handshakeResult
	err    error
}

// handshakeMemoryConn is an in-memory protocol.Conn endpoint.  It transports
// protocol messages between two independent endpoints, rather than recording
// calls made by hostHandshake.
type handshakeMemoryConn struct {
	incoming <-chan protocol.Message
	outgoing chan<- protocol.Message

	closeOnce sync.Once
	closed    chan struct{}

	receiveErr error
}

func newHandshakeMemoryPair() (*handshakeMemoryConn, *handshakeMemoryConn) {
	toHost := make(chan protocol.Message, 4)
	toPlugin := make(chan protocol.Message, 4)
	hostClosed := make(chan struct{})
	pluginClosed := make(chan struct{})
	return &handshakeMemoryConn{incoming: toHost, outgoing: toPlugin, closed: hostClosed},
		&handshakeMemoryConn{incoming: toPlugin, outgoing: toHost, closed: pluginClosed}
}

func (c *handshakeMemoryConn) Send(ctx context.Context, message protocol.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-c.closed:
		return io.ErrClosedPipe
	case <-ctx.Done():
		return ctx.Err()
	case c.outgoing <- message:
		return nil
	}
}

func (c *handshakeMemoryConn) Receive(ctx context.Context) (protocol.Message, error) {
	if c.receiveErr != nil {
		return protocol.Message{}, c.receiveErr
	}
	if err := ctx.Err(); err != nil {
		return protocol.Message{}, err
	}
	select {
	case <-c.closed:
		return protocol.Message{}, io.ErrClosedPipe
	case <-ctx.Done():
		return protocol.Message{}, ctx.Err()
	case message, ok := <-c.incoming:
		if !ok {
			return protocol.Message{}, io.EOF
		}
		return message, nil
	}
}

func (c *handshakeMemoryConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func runHostHandshake(ctx context.Context, conn protocol.Conn, manifest Manifest, token string, startup pluginapi.Startup) <-chan handshakeCall {
	result := make(chan handshakeCall, 1)
	go func() {
		got, err := hostHandshake(ctx, conn, manifest, token, startup)
		result <- handshakeCall{result: got, err: err}
	}()
	return result
}

func receiveHandshakeMessage(t *testing.T, conn protocol.Conn) protocol.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), handshakeTestTimeout)
	defer cancel()
	message, err := conn.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	return message
}

func awaitHandshake(t *testing.T, call <-chan handshakeCall) handshakeCall {
	t.Helper()
	select {
	case result := <-call:
		return result
	case <-time.After(handshakeTestTimeout):
		t.Fatal("hostHandshake did not return")
		return handshakeCall{}
	}
}

func handshakeToken(byteValue byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{byteValue}, 32))
}

func validHandshakeDescriptor() pluginapi.Descriptor {
	manifest := validManifest()
	return pluginapi.Descriptor{
		APIVersion:   pluginapi.APIVersion,
		ID:           manifest.ID,
		Name:         "Runtime Device",
		Version:      manifest.Version,
		Description:  "Runtime description",
		Capabilities: manifest.Capabilities,
	}
}

func validHandshakeStartup() pluginapi.Startup {
	return pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: []byte(`{"gain":0.5}`)}}
}

func validHello(token string) protocol.Message {
	return protocol.Message{
		Version: protocol.Version,
		Type:    protocol.MessageHello,
		Payload: protocol.Hello{
			Token:       token,
			Descriptor:  validHandshakeDescriptor(),
			ProtocolMin: protocol.Version,
			ProtocolMax: protocol.Version,
		},
	}
}

func TestHostHandshakeHelloInitializeReady(t *testing.T) {
	host, plugin := newHandshakeMemoryPair()
	t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
	token := handshakeToken(1)
	call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
	if err := plugin.Send(context.Background(), validHello(token)); err != nil {
		t.Fatal(err)
	}

	initialize := receiveHandshakeMessage(t, plugin)
	if initialize.Type != protocol.MessageInitialize {
		t.Fatalf("host message type = %v, want Initialize", initialize.Type)
	}
	if err := plugin.Send(context.Background(), protocol.Message{Version: protocol.Version, Type: protocol.MessageReady, Payload: protocol.Ready{}}); err != nil {
		t.Fatal(err)
	}
	result := awaitHandshake(t, call)
	if result.err != nil {
		t.Fatalf("hostHandshake() error = %v", result.err)
	}
	if result.result.Descriptor != validHandshakeDescriptor() {
		t.Fatalf("Descriptor = %#v, want %#v", result.result.Descriptor, validHandshakeDescriptor())
	}
}

func TestHostHandshakeInitializeOwnsStartupSnapshot(t *testing.T) {
	host, plugin := newHandshakeMemoryPair()
	t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
	token := handshakeToken(2)
	startup := validHandshakeStartup()
	call := runHostHandshake(context.Background(), host, validManifest(), token, startup)
	if err := plugin.Send(context.Background(), validHello(token)); err != nil {
		t.Fatal(err)
	}

	message := receiveHandshakeMessage(t, plugin)
	initialize, ok := message.Payload.(protocol.Initialize)
	if !ok {
		t.Fatalf("Initialize payload = %T", message.Payload)
	}
	copy(startup.Config.Data, []byte(`{"gain":9.9}`))
	if got := string(initialize.Startup.Config.Data); got != `{"gain":0.5}` {
		t.Fatalf("Initialize Config.Data after caller mutation = %q", got)
	}
	if err := plugin.Send(context.Background(), protocol.Message{Version: protocol.Version, Type: protocol.MessageReady, Payload: protocol.Ready{}}); err != nil {
		t.Fatal(err)
	}
	if result := awaitHandshake(t, call); result.err != nil {
		t.Fatalf("hostHandshake() error = %v", result.err)
	}
}

func TestHostHandshakeRejectsInvalidToken(t *testing.T) {
	for _, test := range []struct {
		name  string
		token string
	}{
		{"wrong", handshakeToken(4)},
		{"blank", ""},
		{"wrong decoded length", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 31))},
	} {
		t.Run(test.name, func(t *testing.T) {
			host, plugin := newHandshakeMemoryPair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			expected := handshakeToken(3)
			call := runHostHandshake(context.Background(), host, validManifest(), expected, validHandshakeStartup())
			if err := plugin.Send(context.Background(), validHello(test.token)); err != nil {
				t.Fatal(err)
			}
			result := awaitHandshake(t, call)
			if !errors.Is(result.err, ErrAuthenticationFailed) {
				t.Fatalf("hostHandshake() error = %v, want ErrAuthenticationFailed", result.err)
			}
			if strings.Contains(result.err.Error(), expected) || (test.token != "" && strings.Contains(result.err.Error(), test.token)) {
				t.Fatalf("hostHandshake() error exposes a token: %v", result.err)
			}
		})
	}
}

func TestHostHandshakeRejectsIncompatibleProtocolRange(t *testing.T) {
	for _, test := range []struct {
		name string
		min  uint16
		max  uint16
	}{
		{"below host", 0, 0},
		{"above host", protocol.Version + 1, protocol.Version + 1},
		{"inverted", protocol.Version + 1, protocol.Version},
	} {
		t.Run(test.name, func(t *testing.T) {
			host, plugin := newHandshakeMemoryPair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			token := handshakeToken(5)
			message := validHello(token)
			hello := message.Payload.(protocol.Hello)
			hello.ProtocolMin, hello.ProtocolMax = test.min, test.max
			message.Payload = hello
			call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
			if err := plugin.Send(context.Background(), message); err != nil {
				t.Fatal(err)
			}
			if result := awaitHandshake(t, call); !errors.Is(result.err, ErrProtocolIncompatible) {
				t.Fatalf("hostHandshake() error = %v, want ErrProtocolIncompatible", result.err)
			}
		})
	}
}

func TestHostHandshakeRejectsInvalidOrMismatchedDescriptor(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*pluginapi.Descriptor)
	}{
		{"invalid", func(d *pluginapi.Descriptor) { d.Name = "" }},
		{"ID", func(d *pluginapi.Descriptor) { d.ID = "another.device" }},
		{"version", func(d *pluginapi.Descriptor) { d.Version = "2.0.0" }},
		{"capabilities", func(d *pluginapi.Descriptor) { d.Capabilities = 2 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			host, plugin := newHandshakeMemoryPair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			token := handshakeToken(6)
			message := validHello(token)
			hello := message.Payload.(protocol.Hello)
			test.mutate(&hello.Descriptor)
			message.Payload = hello
			call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
			if err := plugin.Send(context.Background(), message); err != nil {
				t.Fatal(err)
			}
			if result := awaitHandshake(t, call); !errors.Is(result.err, ErrDescriptorMismatch) {
				t.Fatalf("hostHandshake() error = %v, want ErrDescriptorMismatch", result.err)
			}
		})
	}
}

func TestHostHandshakeAdoptsRuntimeDisplayDescriptor(t *testing.T) {
	host, plugin := newHandshakeMemoryPair()
	t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
	token := handshakeToken(7)
	message := validHello(token)
	hello := message.Payload.(protocol.Hello)
	hello.Descriptor.Name = "Live Runtime Name"
	hello.Descriptor.Description = "Live runtime description"
	message.Payload = hello
	call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
	if err := plugin.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	_ = receiveHandshakeMessage(t, plugin)
	if err := plugin.Send(context.Background(), protocol.Message{Version: protocol.Version, Type: protocol.MessageReady, Payload: protocol.Ready{}}); err != nil {
		t.Fatal(err)
	}
	result := awaitHandshake(t, call)
	if result.err != nil {
		t.Fatalf("hostHandshake() error = %v", result.err)
	}
	if got := result.result.Descriptor; got.Name != hello.Descriptor.Name || got.Description != hello.Descriptor.Description {
		t.Fatalf("display Descriptor = %#v, want runtime display values %#v", got, hello.Descriptor)
	}
}

func TestHostHandshakeRejectsWrongPhaseMessages(t *testing.T) {
	t.Run("ready before hello", func(t *testing.T) {
		host, plugin := newHandshakeMemoryPair()
		t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
		call := runHostHandshake(context.Background(), host, validManifest(), handshakeToken(8), validHandshakeStartup())
		if err := plugin.Send(context.Background(), protocol.Message{Version: protocol.Version, Type: protocol.MessageReady, Payload: protocol.Ready{}}); err != nil {
			t.Fatal(err)
		}
		if result := awaitHandshake(t, call); !errors.Is(result.err, ErrProtocolViolation) {
			t.Fatalf("hostHandshake() error = %v, want ErrProtocolViolation", result.err)
		}
	})

	wrongPhase := []protocol.Message{
		{Version: protocol.Version, Type: protocol.MessageInitialize, Payload: protocol.Initialize{}},
		{Version: protocol.Version, Type: protocol.MessageHeartbeat, Payload: protocol.Heartbeat{}},
		{Version: protocol.Version, Type: protocol.MessageTrackingFrame, Payload: protocol.TrackingFrame{}},
		{Version: protocol.Version, Type: protocol.MessageStatus, Payload: protocol.Status{}},
		{Version: protocol.Version, Type: protocol.MessageLog, Payload: protocol.Log{}},
		{Version: protocol.Version, Type: protocol.MessageConfigChanged, Payload: protocol.ConfigChanged{}},
		{Version: protocol.Version, Type: protocol.MessageSubscriptionChanged, Payload: protocol.SubscriptionChanged{}},
		{Version: protocol.Version, Type: protocol.MessageActiveChanged, Payload: protocol.ActiveChanged{}},
		{Version: protocol.Version, Type: protocol.MessageShutdown, Payload: protocol.Shutdown{}},
		{Version: protocol.Version, Type: protocol.MessageShutdownAck, Payload: protocol.ShutdownAck{}},
		{Version: protocol.Version, Type: protocol.MessageError, Payload: protocol.Error{}},
	}
	for index, message := range wrongPhase {
		t.Run("before hello message "+string(rune('a'+index)), func(t *testing.T) {
			host, plugin := newHandshakeMemoryPair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			call := runHostHandshake(context.Background(), host, validManifest(), handshakeToken(9), validHandshakeStartup())
			if err := plugin.Send(context.Background(), message); err != nil {
				t.Fatal(err)
			}
			if result := awaitHandshake(t, call); !errors.Is(result.err, ErrProtocolViolation) {
				t.Fatalf("hostHandshake() error = %v, want ErrProtocolViolation", result.err)
			}
		})
	}

	t.Run("duplicate hello", func(t *testing.T) {
		host, plugin := newHandshakeMemoryPair()
		t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
		token := handshakeToken(10)
		call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
		if err := plugin.Send(context.Background(), validHello(token)); err != nil {
			t.Fatal(err)
		}
		_ = receiveHandshakeMessage(t, plugin)
		if err := plugin.Send(context.Background(), validHello(token)); err != nil {
			t.Fatal(err)
		}
		if result := awaitHandshake(t, call); !errors.Is(result.err, ErrProtocolViolation) {
			t.Fatalf("hostHandshake() error = %v, want ErrProtocolViolation", result.err)
		}
	})

	for index, message := range wrongPhase {
		t.Run("after initialize message "+string(rune('a'+index)), func(t *testing.T) {
			host, plugin := newHandshakeMemoryPair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			token := handshakeToken(11)
			call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
			if err := plugin.Send(context.Background(), validHello(token)); err != nil {
				t.Fatal(err)
			}
			_ = receiveHandshakeMessage(t, plugin)
			if err := plugin.Send(context.Background(), message); err != nil {
				t.Fatal(err)
			}
			if result := awaitHandshake(t, call); !errors.Is(result.err, ErrProtocolViolation) {
				t.Fatalf("hostHandshake() error = %v, want ErrProtocolViolation", result.err)
			}
		})
	}
}

func TestHostHandshakeTimesOutWaitingForHelloOrReady(t *testing.T) {
	t.Run("hello", func(t *testing.T) {
		host, plugin := newHandshakeMemoryPair()
		t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		result := awaitHandshake(t, runHostHandshake(ctx, host, validManifest(), handshakeToken(12), validHandshakeStartup()))
		if !errors.Is(result.err, ErrHandshakeTimeout) || !errors.Is(result.err, context.DeadlineExceeded) {
			t.Fatalf("hostHandshake() error = %v, want joined timeout", result.err)
		}
	})

	t.Run("ready", func(t *testing.T) {
		host, plugin := newHandshakeMemoryPair()
		t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
		token := handshakeToken(13)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		call := runHostHandshake(ctx, host, validManifest(), token, validHandshakeStartup())
		if err := plugin.Send(context.Background(), validHello(token)); err != nil {
			t.Fatal(err)
		}
		_ = receiveHandshakeMessage(t, plugin)
		result := awaitHandshake(t, call)
		if !errors.Is(result.err, ErrHandshakeTimeout) || !errors.Is(result.err, context.DeadlineExceeded) {
			t.Fatalf("hostHandshake() error = %v, want joined timeout", result.err)
		}
	})
}

func TestHostHandshakeJoinsSafeConnectionFailureWithoutSecrets(t *testing.T) {
	host, plugin := newHandshakeMemoryPair()
	t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
	failure := errors.New("connection reset")
	host.receiveErr = failure
	token := handshakeToken(14)
	configSecret := `{"private":"config-secret"}`
	startup := pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: []byte(configSecret)}}
	result := awaitHandshake(t, runHostHandshake(context.Background(), host, validManifest(), token, startup))
	if !errors.Is(result.err, ErrProtocolViolation) || !errors.Is(result.err, failure) {
		t.Fatalf("hostHandshake() error = %v, want joined protocol failure", result.err)
	}
	if got := result.err.Error(); strings.Contains(got, token) || strings.Contains(got, configSecret) {
		t.Fatalf("hostHandshake() error exposes secret data: %v", result.err)
	}
}
