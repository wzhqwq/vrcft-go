package plugins

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/ipc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type framingTestPhase uint8

const (
	framingBeforeHello framingTestPhase = iota + 1
	framingBeforeReady
	framingAfterReady
)

type framingTestInjection uint8

const (
	framingMalformed framingTestInjection = iota + 1
	framingOversized
	framingEOF
	framingTransport
)

func TestHostHandshakeClassifiesFramingErrorsAtBothReceiveSites(t *testing.T) {
	for _, test := range []struct {
		name      string
		phase     framingTestPhase
		injection framingTestInjection
	}{
		{name: "malformed before Hello", phase: framingBeforeHello, injection: framingMalformed},
		{name: "oversized before Hello", phase: framingBeforeHello, injection: framingOversized},
		{name: "malformed before Ready", phase: framingBeforeReady, injection: framingMalformed},
		{name: "oversized before Ready", phase: framingBeforeReady, injection: framingOversized},
	} {
		t.Run(test.name, func(t *testing.T) {
			host, peer := newHandshakeWirePair()
			t.Cleanup(func() { _ = host.Close(); _ = peer.Close() })
			token := handshakeToken(81)
			call := runHostHandshake(context.Background(), host, validManifest(), token, validHandshakeStartup())
			if test.phase == framingBeforeReady {
				if err := peer.Send(context.Background(), validHello(token)); err != nil {
					t.Fatal(err)
				}
				_ = receiveHandshakeMessage(t, peer)
			}
			if err := writeFramingTestInput(peer.raw, test.injection); err != nil {
				t.Fatal(err)
			}

			result := awaitHandshake(t, call)
			if !errors.Is(result.err, ErrProtocolViolation) {
				t.Fatalf("hostHandshake() error = %v, want ErrProtocolViolation", result.err)
			}
			if got := result.err.Error(); got != ErrProtocolViolation.Error() || strings.Contains(got, "framing-secret") {
				t.Fatalf("hostHandshake() error = %q, want sanitized protocol violation", got)
			}
		})
	}
}

func TestHostHandshakeDoesNotClassifySendSentinelAsPeerFraming(t *testing.T) {
	token := handshakeToken(83)
	conn := &framingSendFailureConn{hello: validHello(token)}
	result := awaitHandshake(t, runHostHandshake(
		context.Background(),
		conn,
		validManifest(),
		token,
		validHandshakeStartup(),
	))
	if errors.Is(result.err, ErrProtocolViolation) || !errors.Is(result.err, ipc.ErrMalformedFrame) {
		t.Fatalf("hostHandshake() send error = %v, want opaque transport sentinel without protocol violation", result.err)
	}
	if got := result.err.Error(); got != "plugins: handshake connection failure" {
		t.Fatalf("hostHandshake() send error text = %q, want sanitized connection failure", got)
	}
}

func TestPluginSupervisorClassifiesRealFramingViolations(t *testing.T) {
	for _, test := range []struct {
		name      string
		phase     framingTestPhase
		injection framingTestInjection
	}{
		{name: "malformed before Hello", phase: framingBeforeHello, injection: framingMalformed},
		{name: "oversized before Hello", phase: framingBeforeHello, injection: framingOversized},
		{name: "malformed after Ready", phase: framingAfterReady, injection: framingMalformed},
		{name: "oversized after Ready", phase: framingAfterReady, injection: framingOversized},
	} {
		t.Run(test.name, func(t *testing.T) {
			plugin := newSessionTestInstalledPlugin(t)
			factory := newFramingSupervisorFactory(plugin, test.phase, test.injection)
			clock := newSupervisorTestClock(time.Unix(500, 0))
			supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
				Plugin:     plugin,
				Preference: PluginPreference{Enabled: true},
				Restart:    DefaultRestartPolicy(),
				NewSession: factory.newSession,
				Now:        clock.now,
				NewTimer:   clock.newTimer,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer closeSupervisor(t, supervisor)

			result := awaitValue(t, factory.results)
			if result.Retryable || !errors.Is(result.Err, ErrProtocolViolation) {
				t.Fatalf("session result = %+v, want non-retryable ErrProtocolViolation", result)
			}
			if strings.Contains(result.Err.Error(), "framing-secret") {
				t.Fatalf("session result exposed raw framing: %v", result.Err)
			}
			if wireErr := awaitValue(t, factory.wireErrors); wireErr != nil {
				t.Fatalf("peer wire action error = %v", wireErr)
			}
			awaitSupervisorState(t, supervisor, StateIncompatible)
			if got := supervisor.Snapshot().LastError; got != ErrProtocolViolation.Error() {
				t.Fatalf("supervisor LastError = %q, want protocol violation", got)
			}
			if clock.timerCount() != 0 || factory.sessions.Load() != 1 {
				t.Fatalf("protocol violation restart state = timers %d sessions %d, want 0/1", clock.timerCount(), factory.sessions.Load())
			}
		})
	}
}

func TestPluginSupervisorKeepsEOFAndOrdinaryTransportRetryable(t *testing.T) {
	for _, test := range []struct {
		name      string
		injection framingTestInjection
		want      error
	}{
		{name: "EOF", injection: framingEOF, want: io.EOF},
		{name: "ordinary transport", injection: framingTransport, want: os.ErrPermission},
	} {
		t.Run(test.name, func(t *testing.T) {
			plugin := newSessionTestInstalledPlugin(t)
			factory := newFramingSupervisorFactory(plugin, framingAfterReady, test.injection)
			clock := newSupervisorTestClock(time.Unix(510, 0))
			supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
				Plugin:     plugin,
				Preference: PluginPreference{Enabled: true},
				Restart:    DefaultRestartPolicy(),
				NewSession: factory.newSession,
				Now:        clock.now,
				NewTimer:   clock.newTimer,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer closeSupervisor(t, supervisor)

			result := awaitValue(t, factory.results)
			causeMatches := errors.Is(result.Err, test.want)
			if test.injection == framingEOF {
				causeMatches = causeMatches || errors.Is(result.Err, io.ErrClosedPipe)
			}
			if !result.Retryable || errors.Is(result.Err, ErrProtocolViolation) || !causeMatches {
				t.Fatalf("session result = %+v, want retryable %v without protocol violation", result, test.want)
			}
			if wireErr := awaitValue(t, factory.wireErrors); wireErr != nil {
				t.Fatalf("peer wire action error = %v", wireErr)
			}
			awaitSupervisorState(t, supervisor, StateBackoff)
			if clock.timerCount() != 1 || factory.sessions.Load() != 1 {
				t.Fatalf("transport restart state = timers %d sessions %d, want 1/1 before timer fires", clock.timerCount(), factory.sessions.Load())
			}
		})
	}
}

type framingSendFailureConn struct {
	hello protocol.Message
	read  bool
}

func (c *framingSendFailureConn) Send(context.Context, protocol.Message) error {
	return ipc.ErrMalformedFrame
}

func (c *framingSendFailureConn) Receive(context.Context) (protocol.Message, error) {
	if c.read {
		return protocol.Message{}, io.EOF
	}
	c.read = true
	return c.hello, nil
}

func (*framingSendFailureConn) Close() error { return nil }

type framingSupervisorFactory struct {
	plugin     InstalledPlugin
	phase      framingTestPhase
	injection  framingTestInjection
	results    chan sessionResult
	wireErrors chan error
	sessions   atomic.Int32
}

func newFramingSupervisorFactory(
	plugin InstalledPlugin,
	phase framingTestPhase,
	injection framingTestInjection,
) *framingSupervisorFactory {
	return &framingSupervisorFactory{
		plugin:     plugin,
		phase:      phase,
		injection:  injection,
		results:    make(chan sessionResult, 1),
		wireErrors: make(chan error, 1),
	}
}

func (f *framingSupervisorFactory) newSession(
	ctx context.Context,
	instanceID uint64,
	startup pluginapi.Startup,
	callbacks supervisorSessionCallbacks,
) pluginSession {
	f.sessions.Add(1)
	hostRaw, peerRaw := net.Pipe()
	hostBase := ipc.WrapConn(hostRaw)
	var hostConn protocol.Conn = hostBase
	if f.injection == framingTransport {
		hostConn = &sessionRuntimeFailureConn{Conn: hostBase, err: os.ErrPermission}
	}
	peerConn := ipc.WrapConn(peerRaw)
	process := newSessionTestProcess()
	token := handshakeToken(82)
	realSession := newPluginSession(ctx, instanceID, sessionConfig{
		Plugin:           f.plugin,
		Startup:          startup,
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  20 * time.Millisecond,
		KillTimeout:      20 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "framing-supervisor", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				defer peerConn.Close()
				f.wireErrors <- runFramingPeer(peerRaw, peerConn, token, f.phase, f.injection)
			}()
			return process, nil
		}},
		onProcessStarted: callbacks.ProcessStarted,
		onReady:          callbacks.Ready,
		onHeartbeat:      callbacks.Heartbeat,
		onFrame:          callbacks.Frame,
		onUnresponsive:   callbacks.Unresponsive,
		onStatus:         callbacks.Status,
		onLog:            callbacks.Log,
	})
	supervisorDone := make(chan sessionResult, 1)
	go func() {
		result := <-realSession.Done()
		f.results <- result
		supervisorDone <- result
		close(supervisorDone)
	}()
	return &framingObservedSession{pluginSession: realSession, done: supervisorDone}
}

type framingObservedSession struct {
	pluginSession
	done <-chan sessionResult
}

func (s *framingObservedSession) Done() <-chan sessionResult { return s.done }

func runFramingPeer(
	raw net.Conn,
	conn protocol.Conn,
	token string,
	phase framingTestPhase,
	injection framingTestInjection,
) error {
	if phase == framingBeforeHello {
		return writeFramingTestInput(raw, injection)
	}
	if err := conn.Send(context.Background(), validHello(token)); err != nil {
		return err
	}
	if _, err := conn.Receive(context.Background()); err != nil {
		return err
	}
	if phase == framingBeforeReady {
		return writeFramingTestInput(raw, injection)
	}
	ready, err := protocol.NewMessage(protocol.Ready{})
	if err != nil {
		return err
	}
	if err := conn.Send(context.Background(), ready); err != nil {
		return err
	}
	switch injection {
	case framingEOF:
		return conn.Close()
	case framingTransport:
		return nil
	default:
		return writeFramingTestInput(raw, injection)
	}
}

func writeFramingTestInput(conn net.Conn, injection framingTestInjection) error {
	var frame []byte
	switch injection {
	case framingMalformed:
		payload := []byte(`{"wire":"framing-secret"`)
		frame = make([]byte, 4+len(payload))
		binary.BigEndian.PutUint32(frame, uint32(len(payload)))
		copy(frame[4:], payload)
	case framingOversized:
		frame = make([]byte, 4)
		binary.BigEndian.PutUint32(frame, uint32(protocol.MaxMessageSize+1))
	default:
		return errors.New("test framing injection is not a frame")
	}
	_, err := conn.Write(frame)
	return err
}
