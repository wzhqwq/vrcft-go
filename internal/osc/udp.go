package osc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type UDPTransport struct {
	conn *net.UDPConn

	targetMu sync.RWMutex
	target   *net.UDPAddr
}

func ListenUDP(address string) (*UDPTransport, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("resolve OSC UDP bind address: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen OSC UDP: %w", err)
	}
	return &UDPTransport{conn: conn}, nil
}

func (t *UDPTransport) LocalAddr() *net.UDPAddr {
	if t == nil || t.conn == nil {
		return nil
	}
	addr, _ := t.conn.LocalAddr().(*net.UDPAddr)
	return addr
}

func (t *UDPTransport) SetTarget(addr *net.UDPAddr) {
	t.targetMu.Lock()
	if addr == nil {
		t.target = nil
	} else {
		copyAddr := *addr
		copyAddr.IP = append(net.IP(nil), addr.IP...)
		t.target = &copyAddr
	}
	t.targetMu.Unlock()
}

func (t *UDPTransport) Target() *net.UDPAddr {
	t.targetMu.RLock()
	defer t.targetMu.RUnlock()
	if t.target == nil {
		return nil
	}
	copyAddr := *t.target
	copyAddr.IP = append(net.IP(nil), t.target.IP...)
	return &copyAddr
}

func (t *UDPTransport) Send(packet []byte) error {
	if len(packet) == 0 {
		return nil
	}
	target := t.Target()
	if target == nil {
		return errors.New("OSC target is not configured")
	}
	_, err := t.conn.WriteToUDP(packet, target)
	if err != nil {
		return fmt.Errorf("send OSC UDP packet: %w", err)
	}
	return nil
}

func (t *UDPTransport) Serve(ctx context.Context, handler func(Message, *net.UDPAddr)) error {
	if handler == nil {
		return errors.New("OSC UDP handler is nil")
	}

	buffer := make([]byte, 64*1024)
	for {
		if deadline, ok := ctx.Deadline(); ok {
			_ = t.conn.SetReadDeadline(deadline)
		} else {
			_ = t.conn.SetReadDeadline(time.Now().Add(time.Second))
		}

		n, remote, err := t.conn.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			return fmt.Errorf("read OSC UDP packet: %w", err)
		}
		messages, err := UnmarshalPacket(buffer[:n])
		if err != nil {
			// Malformed or unsupported datagrams must not terminate the receiver.
			continue
		}
		for _, message := range messages {
			handler(message, remote)
		}
	}
}

func (t *UDPTransport) Close() error {
	if t == nil || t.conn == nil {
		return nil
	}
	return t.conn.Close()
}
