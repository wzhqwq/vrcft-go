package plugins

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type controlState struct {
	Active       bool
	Config       pluginapi.Config
	Subscription pluginapi.Subscription
}

func (s *controlState) applyConfig(next pluginapi.Config) (bool, error) {
	if err := next.Validate(); err != nil {
		return false, fmt.Errorf("plugins: invalid config: %w", err)
	}
	switch {
	case next.Revision < s.Config.Revision:
		return false, ErrConfigRevisionRegression
	case next.Revision == s.Config.Revision:
		if bytes.Equal(next.Data, s.Config.Data) {
			return false, nil
		}
		return false, ErrConfigRevisionConflict
	default:
		s.Config = next.Clone()
		return true, nil
	}
}

func (s *controlState) applySubscription(next pluginapi.Subscription) (bool, error) {
	if err := next.Validate(s.Active); err != nil {
		return false, fmt.Errorf("plugins: invalid subscription: %w", err)
	}
	switch {
	case next.Generation < s.Subscription.Generation:
		return false, ErrSubscriptionGenerationRegression
	case next.Generation == s.Subscription.Generation:
		if next == s.Subscription {
			return false, nil
		}
		return false, ErrSubscriptionGenerationConflict
	default:
		s.Subscription = next
		return true, nil
	}
}

func (s *controlState) applyActive(next bool) bool {
	if s.Active == next {
		return false
	}
	s.Active = next
	return true
}

type controlKind uint8

const (
	controlConfig controlKind = iota + 1
	controlSubscription
	controlActive
	controlShutdown
)

type controlRequest struct {
	kind  controlKind
	state controlState
	reply chan error
}

type sessionWriter struct {
	conn     protocol.Conn
	requests chan controlRequest
	done     chan struct{}

	mu          sync.Mutex
	accepting   bool
	terminalErr error
}

func newSessionWriter(conn protocol.Conn, initial controlState, capacity int) *sessionWriter {
	if capacity < 1 {
		capacity = 1
	}
	writer := &sessionWriter{
		conn:      conn,
		requests:  make(chan controlRequest, capacity),
		done:      make(chan struct{}),
		accepting: true,
	}
	initial.Config = initial.Config.Clone()
	go writer.run(initial)
	return writer
}

func (w *sessionWriter) Control(ctx context.Context, request controlRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	request.reply = make(chan error, 1)

	w.mu.Lock()
	if !w.accepting {
		w.mu.Unlock()
		return ErrInvalidState
	}
	if err := ctx.Err(); err != nil {
		w.mu.Unlock()
		return err
	}
	if request.kind == controlConfig {
		request.state.Config = request.state.Config.Clone()
	}
	select {
	case w.requests <- request:
		if request.kind == controlShutdown {
			w.accepting = false
		}
	default:
		w.mu.Unlock()
		return ErrControlBackpressure
	}
	w.mu.Unlock()
	return <-request.reply
}

func (w *sessionWriter) Done() <-chan struct{} {
	return w.done
}

func (w *sessionWriter) terminalError() error {
	<-w.done
	return w.currentTerminalError()
}

func (w *sessionWriter) currentTerminalError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.terminalErr
}

func (w *sessionWriter) run(state controlState) {
	defer close(w.done)
	for {
		request := <-w.requests
		message, changed, shutdown, err := applyControl(&state, request)
		if err != nil {
			request.reply <- err
			continue
		}
		if !changed {
			request.reply <- nil
			continue
		}
		if err := w.conn.Send(context.Background(), message); err != nil {
			w.mu.Lock()
			w.terminalErr = err
			w.mu.Unlock()
			request.reply <- err
			w.stopAndRejectQueued()
			return
		}
		request.reply <- nil
		if shutdown {
			w.stopAndRejectQueued()
			return
		}
	}
}

func (w *sessionWriter) stopAndRejectQueued() {
	w.mu.Lock()
	w.accepting = false
	w.mu.Unlock()
	for {
		select {
		case request := <-w.requests:
			request.reply <- ErrInvalidState
		default:
			return
		}
	}
}

func applyControl(state *controlState, request controlRequest) (protocol.Message, bool, bool, error) {
	var (
		payload  any
		changed  bool
		shutdown bool
		err      error
	)
	switch request.kind {
	case controlConfig:
		changed, err = state.applyConfig(request.state.Config)
		payload = protocol.ConfigChanged{Config: state.Config.Clone()}
	case controlSubscription:
		changed, err = state.applySubscription(request.state.Subscription)
		payload = protocol.SubscriptionChanged{Subscription: state.Subscription}
	case controlActive:
		changed = state.applyActive(request.state.Active)
		payload = protocol.ActiveChanged{Active: state.Active}
	case controlShutdown:
		changed = true
		shutdown = true
		payload = protocol.Shutdown{}
	default:
		return protocol.Message{}, false, false, ErrInvalidState
	}
	if err != nil || !changed {
		return protocol.Message{}, changed, shutdown, err
	}
	message, err := protocol.NewMessage(payload)
	if err != nil {
		return protocol.Message{}, false, false, fmt.Errorf("plugins: create control message: %w", err)
	}
	return message, true, shutdown, nil
}
