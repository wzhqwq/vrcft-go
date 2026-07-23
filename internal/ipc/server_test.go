package ipc

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type acceptOutcome struct {
	conn net.Conn
	err  error
}

type controlledListener struct {
	outcomes chan acceptOutcome
	closed   chan struct{}
	once     sync.Once
	accepts  atomic.Int32
}

func newControlledListener() *controlledListener {
	return &controlledListener{
		outcomes: make(chan acceptOutcome, 1),
		closed:   make(chan struct{}),
	}
}

func (l *controlledListener) Accept() (net.Conn, error) {
	l.accepts.Add(1)
	select {
	case outcome := <-l.outcomes:
		return outcome.conn, outcome.err
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *controlledListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *controlledListener) Addr() net.Addr { return testAddr("controlled") }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

func TestOneShotListenerAcceptsExactlyOneConnection(t *testing.T) {
	raw := newControlledListener()
	listener := newOneShotListener(raw)
	t.Cleanup(func() { _ = listener.Close() })
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close(); _ = client.Close() })
	raw.outcomes <- acceptOutcome{conn: server}

	conn, err := listener.Accept(context.Background())
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := listener.Accept(context.Background()); !errors.Is(err, ErrListenerConsumed) {
		t.Fatalf("second Accept() error = %v, want ErrListenerConsumed", err)
	}
	select {
	case <-raw.closed:
	default:
		t.Fatal("underlying listener remained open after successful accept")
	}
	received := make(chan error, 1)
	go func() {
		_, err := readFrame(client)
		received <- err
	}()
	if err := conn.Send(context.Background(), testHeartbeatMessage(t)); err != nil {
		t.Fatalf("accepted connection was closed with listener: %v", err)
	}
	if err := <-received; err != nil {
		t.Fatalf("client read error = %v", err)
	}
}

func TestOneShotListenerCanceledWaitCanBeResumed(t *testing.T) {
	raw := newControlledListener()
	listener := newOneShotListener(raw)
	t.Cleanup(func() { _ = listener.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listener.Accept(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Accept(canceled) error = %v", err)
	}
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close(); _ = client.Close() })
	raw.outcomes <- acceptOutcome{conn: server}
	conn, err := listener.Accept(context.Background())
	if err != nil {
		t.Fatalf("Accept(after cancellation) error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if got := raw.accepts.Load(); got != 1 {
		t.Fatalf("underlying Accept calls = %d, want 1", got)
	}
}

func TestOneShotListenerCloseWakesAcceptAndIsIdempotent(t *testing.T) {
	raw := newControlledListener()
	listener := newOneShotListener(raw)
	result := make(chan error, 1)
	go func() {
		_, err := listener.Accept(context.Background())
		result <- err
	}()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept(after Close) error = %v, want net.ErrClosed", err)
		}
	case <-time.After(ipcTestTimeout):
		t.Fatal("Close() did not wake Accept")
	}
}

func TestOneShotListenerConcurrentAcceptHasSingleWinner(t *testing.T) {
	raw := newControlledListener()
	listener := newOneShotListener(raw)
	t.Cleanup(func() { _ = listener.Close() })
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close(); _ = client.Close() })

	const callers = 8
	start := make(chan struct{})
	results := make(chan error, callers)
	var accepted atomic.Int32
	for range callers {
		go func() {
			<-start
			conn, err := listener.Accept(context.Background())
			if err == nil {
				accepted.Add(1)
				_ = conn.Close()
			}
			results <- err
		}()
	}
	close(start)
	raw.outcomes <- acceptOutcome{conn: server}
	consumed := 0
	for range callers {
		err := <-results
		switch {
		case err == nil:
		case errors.Is(err, ErrListenerConsumed):
			consumed++
		default:
			t.Fatalf("Accept() error = %v", err)
		}
	}
	if got := accepted.Load(); got != 1 || consumed != callers-1 {
		t.Fatalf("accepted = %d, consumed = %d", got, consumed)
	}
}
