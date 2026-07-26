package plugins

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/ipc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const handshakeTestTimeout = time.Second

type handshakeCall struct {
	result handshakeResult
	err    error
}

type handshakeWireConn struct {
	protocol.Conn
	raw net.Conn
}

type failingHandshakeConn struct{ err error }

func (c failingHandshakeConn) Send(context.Context, protocol.Message) error { return c.err }
func (c failingHandshakeConn) Receive(context.Context) (protocol.Message, error) {
	return protocol.Message{}, c.err
}
func (failingHandshakeConn) Close() error { return nil }

func newHandshakeWirePair() (*handshakeWireConn, *handshakeWireConn) {
	hostRaw, pluginRaw := net.Pipe()
	return &handshakeWireConn{Conn: ipc.WrapConn(hostRaw), raw: hostRaw},
		&handshakeWireConn{Conn: ipc.WrapConn(pluginRaw), raw: pluginRaw}
}

// sendRawHandshakeJSON bypasses outbound policy validation only for malicious
// inbound-wire cases. The Host endpoint still receives through ipc.streamConn.
func sendRawHandshakeJSON(ctx context.Context, conn *handshakeWireConn, payload string) error {
	data := []byte(payload)
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame, uint32(len(data)))
	copy(frame[4:], data)
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.raw.SetWriteDeadline(deadline); err != nil {
			return err
		}
		defer conn.raw.SetWriteDeadline(time.Time{})
	}
	_, err := conn.raw.Write(frame)
	return err
}

func rawHelloJSON(t *testing.T, token string, descriptor pluginapi.Descriptor, protocolMin, protocolMax uint16) string {
	t.Helper()
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	descriptorJSON, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`{"version":%d,"type":%d,"payload":{"token":%s,"descriptor":%s,"protocolMin":%d,"protocolMax":%d}}`,
		protocol.Version, protocol.MessageHello, tokenJSON, descriptorJSON, protocolMin, protocolMax)
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

func mustHandshakeMessage(t *testing.T, payload any) protocol.Message {
	t.Helper()
	message, err := protocol.NewMessage(payload)
	if err != nil {
		t.Fatalf("protocol.NewMessage(%T) error = %v", payload, err)
	}
	return message
}

func TestHostHandshakeHelloInitializeReady(t *testing.T) {
	host, plugin := newHandshakeWirePair()
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
	host, plugin := newHandshakeWirePair()
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
			host, plugin := newHandshakeWirePair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			expected := handshakeToken(3)
			call := runHostHandshake(context.Background(), host, validManifest(), expected, validHandshakeStartup())
			var err error
			if test.token == "" {
				err = sendRawHandshakeJSON(context.Background(), plugin, rawHelloJSON(t, test.token, validHandshakeDescriptor(), protocol.Version, protocol.Version))
			} else {
				err = plugin.Send(context.Background(), validHello(test.token))
			}
			if err != nil {
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

func TestHostHandshakeRejectsNonCanonicalTokenText(t *testing.T) {
	token := handshakeToken(15)
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, token[len(token)-1])
	nonCanonical := token[:len(token)-1] + string(alphabet[(last&^3)|((last+1)&3)])

	for _, attack := range []string{token + "\r\n", nonCanonical} {
		t.Run("canonical attack", func(t *testing.T) {
			host, plugin := newHandshakeWirePair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
			if err := plugin.Send(context.Background(), validHello(attack)); err != nil {
				t.Fatal(err)
			}
			if result := awaitHandshake(t, call); !errors.Is(result.err, ErrAuthenticationFailed) {
				t.Fatalf("hostHandshake() error = %v, want ErrAuthenticationFailed", result.err)
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
			host, plugin := newHandshakeWirePair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			token := handshakeToken(5)
			message := validHello(token)
			hello := message.Payload.(protocol.Hello)
			hello.ProtocolMin, hello.ProtocolMax = test.min, test.max
			message.Payload = hello
			call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
			if err := sendRawHandshakeJSON(context.Background(), plugin, rawHelloJSON(t, hello.Token, hello.Descriptor, hello.ProtocolMin, hello.ProtocolMax)); err != nil {
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
			host, plugin := newHandshakeWirePair()
			t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
			token := handshakeToken(6)
			message := validHello(token)
			hello := message.Payload.(protocol.Hello)
			test.mutate(&hello.Descriptor)
			message.Payload = hello
			call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
			if err := sendRawHandshakeJSON(context.Background(), plugin, rawHelloJSON(t, hello.Token, hello.Descriptor, hello.ProtocolMin, hello.ProtocolMax)); err != nil {
				t.Fatal(err)
			}
			if result := awaitHandshake(t, call); !errors.Is(result.err, ErrDescriptorMismatch) {
				t.Fatalf("hostHandshake() error = %v, want ErrDescriptorMismatch", result.err)
			}
		})
	}
}

func TestHostHandshakeAdoptsRuntimeDisplayDescriptor(t *testing.T) {
	host, plugin := newHandshakeWirePair()
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
		host, plugin := newHandshakeWirePair()
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
		mustHandshakeMessage(t, protocol.Initialize{Startup: validHandshakeStartup()}),
		mustHandshakeMessage(t, protocol.Heartbeat{}),
		mustHandshakeMessage(t, protocol.TrackingFrame{Generation: 1, Frame: trackingmodel.TrackingFrame{}}),
		mustHandshakeMessage(t, protocol.Status{Status: pluginapi.DeviceStatus{State: pluginapi.DeviceReady}}),
		mustHandshakeMessage(t, protocol.Log{Level: pluginapi.LogInfo, Message: "wrong phase"}),
		mustHandshakeMessage(t, protocol.ConfigChanged{Config: pluginapi.Config{Revision: 1, Data: []byte(`{}`)}}),
		mustHandshakeMessage(t, protocol.SubscriptionChanged{Subscription: pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye}}),
		mustHandshakeMessage(t, protocol.ActiveChanged{}),
		mustHandshakeMessage(t, protocol.Shutdown{}),
		mustHandshakeMessage(t, protocol.ShutdownAck{}),
		mustHandshakeMessage(t, protocol.Error{Code: "peer_error", Message: "wrong phase"}),
	}
	for index, message := range wrongPhase {
		t.Run("before hello message "+string(rune('a'+index)), func(t *testing.T) {
			host, plugin := newHandshakeWirePair()
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
		host, plugin := newHandshakeWirePair()
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
			host, plugin := newHandshakeWirePair()
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
		host, plugin := newHandshakeWirePair()
		t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		result := awaitHandshake(t, runHostHandshake(ctx, host, validManifest(), handshakeToken(12), validHandshakeStartup()))
		if !errors.Is(result.err, ErrHandshakeTimeout) || !errors.Is(result.err, context.DeadlineExceeded) {
			t.Fatalf("hostHandshake() error = %v, want joined timeout", result.err)
		}
	})

	t.Run("ready", func(t *testing.T) {
		host, plugin := newHandshakeWirePair()
		t.Cleanup(func() { _ = host.Close(); _ = plugin.Close() })
		token := handshakeToken(13)
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
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
	failure := errors.New("connection reset")
	token := handshakeToken(14)
	configSecret := `{"private":"config-secret"}`
	startup := pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: []byte(configSecret)}}
	result := awaitHandshake(t, runHostHandshake(context.Background(), failingHandshakeConn{err: failure}, validManifest(), token, startup))
	if !errors.Is(result.err, ErrProtocolViolation) || !errors.Is(result.err, failure) {
		t.Fatalf("hostHandshake() error = %v, want joined protocol failure", result.err)
	}
	if got := result.err.Error(); strings.Contains(got, token) || strings.Contains(got, configSecret) {
		t.Fatalf("hostHandshake() error exposes secret data: %v", result.err)
	}
}

func TestHostHandshakePreservesConnectionCauseWithoutExposingItsText(t *testing.T) {
	token := handshakeToken(16)
	configSecret := `{"private":"config-secret"}`
	startup := pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: []byte(configSecret)}}
	for _, failure := range []error{
		errors.New("transport token fragment " + token[:12]),
		errors.New(`transport escaped config {\"private\":\"config-secret\"}`),
		errors.New("transport complete secrets " + token + " " + configSecret),
	} {
		t.Run("opaque cause", func(t *testing.T) {
			result := awaitHandshake(t, runHostHandshake(context.Background(), failingHandshakeConn{err: failure}, validManifest(), token, startup))
			if !errors.Is(result.err, ErrProtocolViolation) || !errors.Is(result.err, failure) {
				t.Fatalf("hostHandshake() error = %v, want discoverable protocol/cause errors", result.err)
			}
			const want = "plugins: protocol violation\nplugins: handshake connection failure"
			if got := result.err.Error(); got != want || strings.Contains(got, failure.Error()) {
				t.Fatalf("hostHandshake() error = %q, want opaque public text %q", got, want)
			}
		})
	}
}
