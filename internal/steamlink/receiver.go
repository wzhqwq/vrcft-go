package steamlink

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type datagram struct {
	Bytes      []byte
	ReceivedAt time.Time
	Epoch      uint64
}

type receiver struct {
	conn net.PacketConn

	dropped atomic.Uint64

	closeOnce sync.Once
	closeErr  error
}

func listenReceiver(address string) (*receiver, error) {
	addr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	return &receiver{conn: conn}, nil
}

func (r *receiver) localAddr() *net.UDPAddr {
	addr := r.conn.LocalAddr().(*net.UDPAddr)
	return &net.UDPAddr{IP: append([]byte(nil), addr.IP...), Port: addr.Port, Zone: addr.Zone}
}

func (r *receiver) run(ctx context.Context, epoch *atomic.Uint64, now func() time.Time, out chan<- datagram) error {
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = r.close()
		case <-finished:
		}
	}()
	defer close(finished)

	buffer := make([]byte, 65535)
	for {
		n, _, err := r.conn.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		receivedAt := now()
		if n > maxDatagramBytes {
			continue
		}

		packet := datagram{
			Bytes:      append([]byte(nil), buffer[:n]...),
			ReceivedAt: receivedAt,
			Epoch:      epoch.Load(),
		}
		select {
		case out <- packet:
		default:
			r.dropped.Add(1)
		}
	}
}

func (r *receiver) close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.conn.Close()
	})
	return r.closeErr
}

func (r *receiver) takeDropped() uint64 {
	return r.dropped.Swap(0)
}
