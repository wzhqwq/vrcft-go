package steamlink

import (
	"bytes"
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReceiverOwnsQueuedBytes(t *testing.T) {
	r := newTestReceiver(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var epoch atomic.Uint64
	epoch.Store(7)
	out := make(chan datagram, 2)
	done := startReceiver(ctx, r, &epoch, func() time.Time { return time.Unix(123, 0) }, out)

	first := []byte{1, 2, 3}
	second := []byte{4, 5, 6, 7}
	sendDatagram(t, r.localAddr(), first)
	sendDatagram(t, r.localAddr(), second)

	packets := []datagram{receiveDatagram(t, ctx, out), receiveDatagram(t, ctx, out)}
	if !(bytes.Equal(packets[0].Bytes, first) && bytes.Equal(packets[1].Bytes, second)) &&
		!(bytes.Equal(packets[0].Bytes, second) && bytes.Equal(packets[1].Bytes, first)) {
		t.Fatalf("queued bytes = %v, %v; want %v, %v", packets[0].Bytes, packets[1].Bytes, first, second)
	}
	for _, packet := range packets {
		if packet.ReceivedAt != time.Unix(123, 0) || packet.Epoch != 7 {
			t.Fatalf("packet metadata = %#v", packet)
		}
	}

	cancel()
	awaitReceiver(t, done)
}

func TestReceiverDropsOnFullQueue(t *testing.T) {
	r := newTestReceiver(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var epoch atomic.Uint64
	out := make(chan datagram, 1)
	out <- datagram{}
	read := make(chan struct{})
	var readOnce sync.Once
	done := startReceiver(ctx, r, &epoch, func() time.Time {
		readOnce.Do(func() { close(read) })
		return time.Now()
	}, out)

	sendDatagram(t, r.localAddr(), []byte{1})
	select {
	case <-read:
	case <-ctx.Done():
		t.Fatal("receiver did not read datagram")
	}
	if got := awaitDropped(t, ctx, r); got != 1 {
		t.Fatalf("dropped = %d, want 1", got)
	}
	if got := r.takeDropped(); got != 0 {
		t.Fatalf("dropped after take = %d, want 0", got)
	}

	cancel()
	awaitReceiver(t, done)
}

func TestReceiverCloseUnblocksRead(t *testing.T) {
	r := newTestReceiver(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var epoch atomic.Uint64
	done := startReceiver(ctx, r, &epoch, time.Now, make(chan datagram))
	if err := r.close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := r.close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	awaitReceiver(t, done)
}

func TestReceiverReturnsReadError(t *testing.T) {
	r := newTestReceiver(t)
	if err := r.conn.SetReadDeadline(time.Now()); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var epoch atomic.Uint64
	done := startReceiver(context.Background(), r, &epoch, time.Now, make(chan datagram))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("receiver accepted read timeout")
		}
	case <-ctx.Done():
		t.Fatal("receiver did not return read error")
	}
}

func newTestReceiver(t *testing.T) *receiver {
	t.Helper()
	r, err := listenReceiver("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen receiver: %v", err)
	}
	t.Cleanup(func() {
		if err := r.close(); err != nil {
			t.Errorf("close receiver: %v", err)
		}
	})
	return r
}

func startReceiver(ctx context.Context, r *receiver, epoch *atomic.Uint64, now func() time.Time, out chan<- datagram) <-chan error {
	done := make(chan error, 1)
	go func() { done <- r.run(ctx, epoch, now, out) }()
	return done
}

func sendDatagram(t *testing.T, address *net.UDPAddr, payload []byte) {
	t.Helper()
	conn, err := net.DialUDP("udp4", nil, address)
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("send datagram: %v", err)
	}
}

func receiveDatagram(t *testing.T, ctx context.Context, out <-chan datagram) datagram {
	t.Helper()
	select {
	case packet := <-out:
		return packet
	case <-ctx.Done():
		t.Fatal("timed out waiting for datagram")
		return datagram{}
	}
}

func awaitDropped(t *testing.T, ctx context.Context, r *receiver) uint64 {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if dropped := r.takeDropped(); dropped > 0 {
			return dropped
		}
		select {
		case <-ctx.Done():
			t.Fatal("receiver did not drop full-queue datagram")
			return 0
		case <-ticker.C:
		}
	}
}

func awaitReceiver(t *testing.T, done <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("receiver run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("receiver did not stop")
	}
}
