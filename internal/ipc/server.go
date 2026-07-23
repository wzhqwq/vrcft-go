package ipc

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type listenerResult struct {
	conn net.Conn
	err  error
}

type oneShotListener struct {
	listener net.Listener
	ready    chan struct{}

	mu       sync.Mutex
	result   listenerResult
	consumed bool

	closeOnce sync.Once
	closeErr  error

	shutdownOnce sync.Once
	shutdownErr  error
}

var _ Listener = (*oneShotListener)(nil)

func newOneShotListener(listener net.Listener) Listener {
	result := &oneShotListener{
		listener: listener,
		ready:    make(chan struct{}),
	}
	go result.accept()
	return result
}

func validateServerConfig(config ServerConfig) error {
	return validatePipeName(config.PipeName)
}

func (l *oneShotListener) accept() {
	conn, err := l.listener.Accept()
	if err == nil {
		_ = l.closeUnderlying()
	}
	l.mu.Lock()
	l.result = listenerResult{conn: conn, err: err}
	l.mu.Unlock()
	close(l.ready)
}

func (l *oneShotListener) Accept(ctx context.Context) (protocol.Conn, error) {
	if ctx == nil {
		panic("ipc: Listener.Accept context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.ready:
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.result.err != nil {
		return nil, l.result.err
	}
	if l.consumed {
		return nil, ErrListenerConsumed
	}
	l.consumed = true
	return newConn(l.result.conn), nil
}

func (l *oneShotListener) Close() error {
	l.shutdownOnce.Do(func() {
		listenerErr := l.closeUnderlying()
		<-l.ready

		l.mu.Lock()
		var pending net.Conn
		if l.result.err == nil && l.result.conn != nil && !l.consumed {
			pending = l.result.conn
			l.result = listenerResult{err: net.ErrClosed}
		}
		l.mu.Unlock()

		var pendingErr error
		if pending != nil {
			pendingErr = pending.Close()
		}
		l.shutdownErr = errors.Join(listenerErr, pendingErr)
	})
	return l.shutdownErr
}

func (l *oneShotListener) closeUnderlying() error {
	l.closeOnce.Do(func() {
		l.closeErr = l.listener.Close()
	})
	return l.closeErr
}
