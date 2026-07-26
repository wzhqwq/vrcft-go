package ipc

import (
	"bytes"
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

const ipcTestTimeout = 2 * time.Second

type deadlineLagContext struct {
	deadline time.Time
	done     chan struct{}
}

func (c deadlineLagContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c deadlineLagContext) Done() <-chan struct{}       { return c.done }
func (deadlineLagContext) Err() error                    { return nil }
func (deadlineLagContext) Value(any) any                 { return nil }

type testTimeoutError struct{}

func (testTimeoutError) Error() string   { return "test timeout" }
func (testTimeoutError) Timeout() bool   { return true }
func (testTimeoutError) Temporary() bool { return true }

func receiveAsync(conn protocol.Conn) <-chan struct {
	message protocol.Message
	err     error
} {
	result := make(chan struct {
		message protocol.Message
		err     error
	}, 1)
	go func() {
		message, err := conn.Receive(context.Background())
		result <- struct {
			message protocol.Message
			err     error
		}{message: message, err: err}
	}()
	return result
}

func TestStreamConnBidirectionalExchange(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, right := newConn(leftRaw), newConn(rightRaw)
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })

	leftMessage := testHeartbeatMessage(t)
	rightMessage, err := protocol.NewMessage(protocol.Ready{})
	if err != nil {
		t.Fatal(err)
	}
	leftReceive := receiveAsync(left)
	rightReceive := receiveAsync(right)
	if err := left.Send(context.Background(), leftMessage); err != nil {
		t.Fatal(err)
	}
	if err := right.Send(context.Background(), rightMessage); err != nil {
		t.Fatal(err)
	}
	if result := <-rightReceive; result.err != nil || !reflect.DeepEqual(result.message, leftMessage) {
		t.Fatalf("right Receive() = (%#v, %v)", result.message, result.err)
	}
	if result := <-leftReceive; result.err != nil || !reflect.DeepEqual(result.message, rightMessage) {
		t.Fatalf("left Receive() = (%#v, %v)", result.message, result.err)
	}
}

func TestWrapConnExposesStreamProtocolConn(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, right := WrapConn(leftRaw), WrapConn(rightRaw)
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })

	received := receiveAsync(right)
	if err := left.Send(context.Background(), testHeartbeatMessage(t)); err != nil {
		t.Fatal(err)
	}
	if result := <-received; result.err != nil || result.message.Type != protocol.MessageHeartbeat {
		t.Fatalf("wrapped Receive() = (%#v, %v)", result.message, result.err)
	}
}

func TestStreamConnReceiveCancellationDoesNotPoisonNextRead(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, right := newConn(leftRaw), newConn(rightRaw)
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := left.Receive(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Receive(canceled) error = %v", err)
	}

	message := testHeartbeatMessage(t)
	received := receiveAsync(left)
	if err := right.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if result := <-received; result.err != nil || !reflect.DeepEqual(result.message, message) {
		t.Fatalf("Receive(after cancellation) = (%#v, %v)", result.message, result.err)
	}
}

func TestStreamConnBlockedOperationsHonorContext(t *testing.T) {
	t.Run("receive", func(t *testing.T) {
		leftRaw, rightRaw := net.Pipe()
		conn := newConn(leftRaw)
		t.Cleanup(func() { _ = conn.Close(); _ = rightRaw.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if _, err := conn.Receive(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Receive() error = %v, want deadline exceeded", err)
		}
	})

	t.Run("send", func(t *testing.T) {
		leftRaw, rightRaw := net.Pipe()
		conn := newConn(leftRaw)
		t.Cleanup(func() { _ = conn.Close(); _ = rightRaw.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := conn.Send(ctx, testHeartbeatMessage(t)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Send() error = %v, want deadline exceeded", err)
		}
	})
}

func TestStreamConnMapsExpiredContextDeadlineBeforeContextTimerFires(t *testing.T) {
	conn := &streamConn{}
	ctx := deadlineLagContext{
		deadline: time.Now().Add(-time.Millisecond),
		done:     make(chan struct{}),
	}
	if err := conn.classifyOperationError(ctx, testTimeoutError{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("classifyOperationError() = %v, want context.DeadlineExceeded", err)
	}
}

func TestStreamConnCloseUnblocksReceiveAndIsIdempotent(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	conn := newConn(leftRaw)
	defer rightRaw.Close()
	result := receiveAsync(conn)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	select {
	case received := <-result:
		if received.err == nil {
			t.Fatal("blocked Receive() error = nil")
		}
	case <-time.After(ipcTestTimeout):
		t.Fatal("Close() did not unblock Receive()")
	}
}

func TestStreamConnCloseUnblocksSend(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	conn := newConn(leftRaw)
	defer rightRaw.Close()
	message := testHeartbeatMessage(t)
	result := make(chan error, 1)
	go func() {
		result <- conn.Send(context.Background(), message)
	}()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("blocked Send() error = nil")
		}
	case <-time.After(ipcTestTimeout):
		t.Fatal("Close() did not unblock Send()")
	}
}

func TestStreamConnSerializesConcurrentSends(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, right := newConn(leftRaw), newConn(rightRaw)
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })

	messages := make([]protocol.Message, 2)
	for index := range messages {
		message, err := protocol.NewMessage(protocol.Heartbeat{UptimeMS: uint64(index + 1)})
		if err != nil {
			t.Fatal(err)
		}
		messages[index] = message
	}
	var wait sync.WaitGroup
	wait.Add(2)
	errorsOut := make(chan error, 2)
	for _, message := range messages {
		go func() {
			defer wait.Done()
			errorsOut <- left.Send(context.Background(), message)
		}()
	}
	received := []protocol.Message{}
	for range 2 {
		message, err := right.Receive(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		received = append(received, message)
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range messages {
		found := false
		for _, got := range received {
			found = found || reflect.DeepEqual(got, want)
		}
		if !found {
			t.Fatalf("received = %#v, missing %#v", received, want)
		}
	}
}

func TestStreamConnSerializesConcurrentReceives(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, right := newConn(leftRaw), newConn(rightRaw)
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })

	results := []<-chan struct {
		message protocol.Message
		err     error
	}{receiveAsync(right), receiveAsync(right)}
	for index := range 2 {
		message, err := protocol.NewMessage(protocol.Heartbeat{UptimeMS: uint64(index + 1)})
		if err != nil {
			t.Fatal(err)
		}
		if err := left.Send(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[uint64]bool{}
	for _, resultChannel := range results {
		result := <-resultChannel
		if result.err != nil {
			t.Fatal(result.err)
		}
		seen[result.message.Payload.(protocol.Heartbeat).UptimeMS] = true
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("received uptimes = %v, want 1 and 2", seen)
	}
}

func TestStreamConnOutboundValidationFailureLeavesConnectionUsable(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, right := newConn(leftRaw), newConn(rightRaw)
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
	if err := left.Send(context.Background(), protocol.Message{}); err == nil {
		t.Fatal("Send(invalid) error = nil")
	}
	message := testHeartbeatMessage(t)
	received := receiveAsync(right)
	if err := left.Send(context.Background(), message); err != nil {
		t.Fatalf("Send(valid after invalid) error = %v", err)
	}
	if result := <-received; result.err != nil || !reflect.DeepEqual(result.message, message) {
		t.Fatalf("Receive() = (%#v, %v)", result.message, result.err)
	}
}

func TestStreamConnInboundFramingFailureClosesStream(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	conn := newConn(leftRaw)
	defer rightRaw.Close()
	go func() {
		_, _ = rightRaw.Write(rawFrame([]byte(`not-json`)))
	}()
	if _, err := conn.Receive(context.Background()); !errors.Is(err, ErrMalformedFrame) {
		t.Fatalf("Receive(malformed) error = %v", err)
	}
	if err := conn.Send(context.Background(), testHeartbeatMessage(t)); err == nil {
		t.Fatal("Send(after malformed frame) error = nil")
	}
}

func TestStreamConnSendOwnsEncodedData(t *testing.T) {
	leftRaw, rightRaw := net.Pipe()
	left, right := newConn(leftRaw), newConn(rightRaw)
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })

	data := []byte(`{"gain":0.5}`)
	message, err := protocol.NewMessage(protocol.ConfigChanged{
		Config: testConfigWithData(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	received := receiveAsync(right)
	if err := left.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	copy(data, bytes.Repeat([]byte{'x'}, len(data)))
	result := <-received
	if result.err != nil {
		t.Fatal(result.err)
	}
	got := result.message.Payload.(protocol.ConfigChanged).Config.Data
	if string(got) != `{"gain":0.5}` {
		t.Fatalf("received config data = %s", got)
	}
}

func testConfigWithData(data []byte) pluginapi.Config {
	return pluginapi.Config{Revision: 1, Data: data}
}
