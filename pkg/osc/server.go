package osc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// HandlerFunc receives one decoded OSC message and the address of its sender.
// It is called synchronously in packet wire order.
type HandlerFunc func(Message, *net.UDPAddr)

// Server receives and sends OSC packets through one UDP socket.
type Server struct {
	conn      *net.UDPConn
	local     *net.UDPAddr
	mu        sync.Mutex
	serving   bool
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

// ListenUDP resolves address and returns a Server bound to it. The Server owns
// the socket until Close is called.
func ListenUDP(address string) (*Server, error) {
	local, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("resolve OSC UDP address %q: %w", address, err)
	}
	conn, err := net.ListenUDP("udp", local)
	if err != nil {
		return nil, fmt.Errorf("listen for OSC UDP packets on %s: %w", local, err)
	}
	return &Server{conn: conn, local: cloneUDPAddr(conn.LocalAddr().(*net.UDPAddr))}, nil
}

// LocalAddr returns an independently owned copy of the Server's bound address.
func (s *Server) LocalAddr() *net.UDPAddr {
	if s == nil {
		return nil
	}
	return cloneUDPAddr(s.local)
}

// Serve receives and synchronously dispatches decoded OSC messages until ctx
// is cancelled or the Server is closed. Only one Serve call may be active.
func (s *Server) Serve(ctx context.Context, handler HandlerFunc) error {
	if ctx == nil || handler == nil {
		return fmt.Errorf("serve OSC UDP packets: %w", ErrInvalidArgument)
	}
	if s == nil {
		return ErrServerClosed
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrServerClosed
	}
	if s.serving {
		s.mu.Unlock()
		return ErrServerRunning
	}
	s.serving = true
	s.mu.Unlock()

	cancellationDone := make(chan struct{})
	stopCancellationWakeup := context.AfterFunc(ctx, func() {
		_ = s.conn.SetReadDeadline(time.Now())
		close(cancellationDone)
	})
	defer func() {
		if !stopCancellationWakeup() {
			<-cancellationDone
		}
		_ = s.conn.SetReadDeadline(time.Time{})
		s.mu.Lock()
		s.serving = false
		s.mu.Unlock()
	}()

	buffer := make([]byte, 65_535)
	for {
		count, remote, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil || s.isClosed() {
				return nil
			}
			return fmt.Errorf("read OSC UDP packet: %w", err)
		}
		messages, err := UnmarshalPacket(buffer[:count])
		if err != nil {
			continue
		}
		for _, message := range messages {
			handler(message, cloneUDPAddr(remote))
		}
	}
}

// SendTo sends packet through the Server's bound socket to target.
func (s *Server) SendTo(packet []byte, target *net.UDPAddr) error {
	if len(packet) == 0 {
		return fmt.Errorf("send OSC UDP packet: %w", ErrMalformedPacket)
	}
	if target == nil {
		return fmt.Errorf("send OSC UDP packet: %w", ErrInvalidArgument)
	}
	if s == nil || s.isClosed() {
		return ErrServerClosed
	}

	if _, err := s.conn.WriteToUDP(packet, cloneUDPAddr(target)); err != nil {
		if errors.Is(err, net.ErrClosed) || s.isClosed() {
			return ErrServerClosed
		}
		return fmt.Errorf("send OSC UDP packet to %s: %w", target, err)
	}
	return nil
}

// Close permanently closes the Server's UDP socket. It is safe to call more
// than once and concurrently with Serve and SendTo.
func (s *Server) Close() error {
	if s == nil {
		return ErrServerClosed
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()

		err := s.conn.Close()
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		s.mu.Lock()
		s.closeErr = err
		s.mu.Unlock()
	})
	s.mu.Lock()
	err := s.closeErr
	s.mu.Unlock()
	return err
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func cloneUDPAddr(address *net.UDPAddr) *net.UDPAddr {
	if address == nil {
		return nil
	}
	clone := *address
	clone.IP = append(net.IP(nil), address.IP...)
	return &clone
}
