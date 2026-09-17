package osc

import (
	"context"
	"errors"
	"net"
	"sync"

	pkgosc "github.com/wzhqwq/vrcft-go/pkg/osc"
)

type UDPTransport struct {
	server *pkgosc.Server

	targetMu sync.RWMutex
	target   *net.UDPAddr
}

func ListenUDP(address string) (*UDPTransport, error) {
	server, err := pkgosc.ListenUDP(address)
	if err != nil {
		return nil, err
	}
	return &UDPTransport{server: server}, nil
}

func (t *UDPTransport) LocalAddr() *net.UDPAddr {
	if t == nil || t.server == nil {
		return nil
	}
	return t.server.LocalAddr()
}

func (t *UDPTransport) SetTarget(addr *net.UDPAddr) {
	if t == nil {
		return
	}
	t.targetMu.Lock()
	if addr == nil {
		t.target = nil
	} else {
		t.target = cloneUDPAddr(addr)
	}
	t.targetMu.Unlock()
}

func (t *UDPTransport) Target() *net.UDPAddr {
	if t == nil {
		return nil
	}
	t.targetMu.RLock()
	defer t.targetMu.RUnlock()
	return cloneUDPAddr(t.target)
}

func (t *UDPTransport) Send(packet []byte) error {
	if len(packet) == 0 {
		return nil
	}
	target := t.Target()
	if target == nil {
		return errors.New("OSC target is not configured")
	}
	if t == nil || t.server == nil {
		return errors.New("OSC UDP transport is not started")
	}
	return t.server.SendTo(packet, target)
}

func (t *UDPTransport) Serve(ctx context.Context, handler func(Message, *net.UDPAddr)) error {
	if t == nil || t.server == nil {
		return errors.New("OSC UDP transport is not started")
	}
	return t.server.Serve(ctx, handler)
}

func (t *UDPTransport) Close() error {
	if t == nil || t.server == nil {
		return nil
	}
	return t.server.Close()
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	copyAddr := *addr
	copyAddr.IP = append(net.IP(nil), addr.IP...)
	return &copyAddr
}
