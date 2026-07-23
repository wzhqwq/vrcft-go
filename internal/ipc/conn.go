package ipc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type streamConn struct {
	conn net.Conn

	readMu  sync.Mutex
	writeMu sync.Mutex

	closeOnce sync.Once
	closeErr  error
}

var _ protocol.Conn = (*streamConn)(nil)

func newConn(conn net.Conn) protocol.Conn {
	return &streamConn{conn: conn}
}

func (c *streamConn) Send(ctx context.Context, message protocol.Message) error {
	if ctx == nil {
		return errors.New("ipc: Send context must not be nil")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	cleanup, err := armDeadline(ctx, c.conn.SetWriteDeadline)
	if err != nil {
		return fmt.Errorf("ipc: set write deadline: %w", err)
	}
	err = writeFrame(c.conn, message)
	cleanup()
	return c.classifyOperationError(ctx, err)
}

func (c *streamConn) Receive(ctx context.Context) (protocol.Message, error) {
	if ctx == nil {
		return protocol.Message{}, errors.New("ipc: Receive context must not be nil")
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if err := ctx.Err(); err != nil {
		return protocol.Message{}, err
	}
	cleanup, err := armDeadline(ctx, c.conn.SetReadDeadline)
	if err != nil {
		return protocol.Message{}, fmt.Errorf("ipc: set read deadline: %w", err)
	}
	message, err := readFrame(c.conn)
	cleanup()
	return message, c.classifyOperationError(ctx, err)
}

func (c *streamConn) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.conn.Close()
	})
	return c.closeErr
}

func (c *streamConn) classifyOperationError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var fatal *fatalStreamError
	isFatal := errors.As(err, &fatal)
	if isFatal && fatal.progressed {
		_ = c.Close()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if !isFatal || !fatal.progressed {
			return ctxErr
		}
		return ctxErr
	}
	if isFatal || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		_ = c.Close()
	}
	return err
}

func armDeadline(ctx context.Context, setDeadline func(time.Time) error) (func(), error) {
	deadline := time.Time{}
	if value, ok := ctx.Deadline(); ok {
		deadline = value
	}
	if err := setDeadline(deadline); err != nil {
		return nil, err
	}
	if ctx.Done() == nil {
		return func() {
			_ = setDeadline(time.Time{})
		}, nil
	}

	callbackDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = setDeadline(time.Now())
		close(callbackDone)
	})
	return func() {
		if !stop() {
			<-callbackDone
		}
		_ = setDeadline(time.Time{})
	}, nil
}
