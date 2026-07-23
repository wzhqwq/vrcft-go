package pluginruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

var (
	ErrControlBackpressure = errors.New("pluginruntime: control event queue full")
	ErrAlreadyRun          = errors.New("pluginruntime: runtime already run")
	ErrRuntimeAlreadyRun   = ErrAlreadyRun
	ErrShutdownTimeout     = errors.New("pluginruntime: driver shutdown timed out")
)

type RuntimeConfig struct {
	Token             string
	ControlQueue      int
	LogQueue          int
	HeartbeatInterval time.Duration
	ShutdownTimeout   time.Duration
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		ControlQueue:      32,
		LogQueue:          256,
		HeartbeatInterval: time.Second,
		ShutdownTimeout:   5 * time.Second,
	}
}

type Runtime struct {
	driver     pluginapi.Driver
	descriptor pluginapi.Descriptor
	conn       protocol.Conn
	config     RuntimeConfig
	run        atomic.Bool
}

type runtimeConnectionCloser struct {
	conn protocol.Conn
	once sync.Once
	err  error
}

func (c *runtimeConnectionCloser) Close() error {
	c.once.Do(func() {
		c.err = c.conn.Close()
	})
	return c.err
}

func New(driver pluginapi.Driver, conn protocol.Conn, cfg RuntimeConfig) (*Runtime, error) {
	if nilInterface(driver) {
		return nil, errors.New("pluginruntime: driver is nil")
	}
	if nilInterface(conn) {
		return nil, errors.New("pluginruntime: connection is nil")
	}
	descriptor := driver.Descriptor()
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("pluginruntime: invalid driver descriptor: %w", err)
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("pluginruntime: token must be nonblank")
	}
	if cfg.ControlQueue <= 0 {
		return nil, errors.New("pluginruntime: control queue capacity must be positive")
	}
	if cfg.LogQueue <= 0 {
		return nil, errors.New("pluginruntime: log queue capacity must be positive")
	}
	if cfg.HeartbeatInterval <= 0 {
		return nil, errors.New("pluginruntime: heartbeat interval must be positive")
	}
	if cfg.ShutdownTimeout <= 0 {
		return nil, errors.New("pluginruntime: shutdown timeout must be positive")
	}
	return &Runtime{driver: driver, descriptor: descriptor, conn: conn, config: cfg}, nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (r *Runtime) Run(ctx context.Context) (result error) {
	if !r.run.CompareAndSwap(false, true) {
		return ErrRuntimeAlreadyRun
	}
	closer := &runtimeConnectionCloser{conn: r.conn}
	defer func() {
		if closeErr := closer.Close(); closeErr != nil && !errors.Is(result, closeErr) {
			result = errors.Join(result, closeErr)
		}
	}()

	if err := ctx.Err(); err != nil {
		return err
	}
	hello := protocol.Hello{
		Token:       r.config.Token,
		Descriptor:  r.descriptor,
		ProtocolMin: protocol.Version,
		ProtocolMax: protocol.Version,
	}
	if err := r.send(ctx, hello); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		r.sendErrorNotice("protocol_error", err)
		return err
	}

	message, err := r.conn.Receive(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		r.sendErrorNotice("protocol_error", err)
		return err
	}
	if err := message.Validate(); err != nil {
		err = fmt.Errorf("pluginruntime: invalid initialization message: %w", err)
		r.sendErrorNotice("protocol_error", err)
		return err
	}
	initialize, ok := message.Payload.(protocol.Initialize)
	if !ok {
		err = fmt.Errorf("pluginruntime: first message must be Initialize, got %T", message.Payload)
		r.sendErrorNotice("protocol_error", err)
		return err
	}

	startup := cloneStartup(initialize.Startup)
	startup.Subscription = startup.Subscription.Normalize()
	host := newRuntimeHost(startup, r.config.ControlQueue, r.config.LogQueue)
	defer host.stop()
	if err := r.send(ctx, protocol.Ready{}); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		r.sendErrorNotice("protocol_error", err)
		return err
	}

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	terminal := newTerminalState()
	terminalDone := make(chan terminalEvent, 3)
	writerStarted := time.Now()
	go func() {
		terminalDone <- terminalEvent{worker: terminalWriter, writerErr: r.writeOutbound(childCtx, host, writerStarted)}
	}()
	go func() {
		terminalDone <- terminalEvent{worker: terminalDriver, driver: terminal.driverReturned(r.driver.Run(childCtx, host))}
	}()
	go func() {
		terminalDone <- terminalEvent{worker: terminalReader, reader: r.readControls(childCtx, host, terminal)}
	}()

	var first terminalEvent
	var hasFirst bool
	select {
	case <-ctx.Done():
	case first = <-terminalDone:
		hasFirst = true
	}
	externalErr := ctx.Err()
	cancel()

	outcome := terminalOutcome{}
	if hasFirst {
		outcome.primary = first.worker
		outcome.record(first, false)
	}
	outcome = r.collectTerminalResults(terminalDone, outcome, closer)
	return r.finishTerminal(externalErr, outcome)
}

func (r *Runtime) sendShutdownAck(result error) error {
	ackCtx, cancel := context.WithTimeout(context.Background(), r.config.ShutdownTimeout)
	defer cancel()
	if err := r.send(ackCtx, protocol.ShutdownAck{}); err != nil {
		r.sendErrorNotice("protocol_error", err)
		result = errors.Join(result, err)
	}
	return result
}

type terminalWorker uint8

const (
	terminalDriver terminalWorker = iota + 1
	terminalReader
	terminalWriter
)

type terminalEvent struct {
	worker    terminalWorker
	driver    driverResult
	reader    readerResult
	writerErr error
}

type terminalOutcome struct {
	primary terminalWorker

	driver             driverResult
	driverStopped      bool
	reader             readerResult
	readerStopped      bool
	readerAfterClose   bool
	writerErr          error
	writerStopped      bool
	writerAfterClose   bool
	timedOut           bool
	connectionClosed   bool
	earlyConnectionErr error
}

func (o *terminalOutcome) record(event terminalEvent, afterClose bool) {
	switch event.worker {
	case terminalDriver:
		if !o.driverStopped {
			o.driver = event.driver
			o.driverStopped = true
		}
	case terminalReader:
		if !o.readerStopped {
			o.reader = event.reader
			o.readerStopped = true
			o.readerAfterClose = afterClose
		}
	case terminalWriter:
		if !o.writerStopped {
			o.writerErr = event.writerErr
			o.writerStopped = true
			o.writerAfterClose = afterClose
		}
	}
}

func (o terminalOutcome) stoppedCount() int {
	count := 0
	if o.driverStopped {
		count++
	}
	if o.readerStopped {
		count++
	}
	if o.writerStopped {
		count++
	}
	return count
}

func (r *Runtime) collectTerminalResults(
	terminalDone <-chan terminalEvent,
	outcome terminalOutcome,
	closer *runtimeConnectionCloser,
) terminalOutcome {
	timer := time.NewTimer(r.config.ShutdownTimeout)
	defer timer.Stop()

	for outcome.stoppedCount() < 3 {
		select {
		case event := <-terminalDone:
			outcome.record(event, false)
		case <-timer.C:
			outcome.timedOut = true
			if !outcome.readerStopped {
				outcome.connectionClosed = true
				outcome.earlyConnectionErr = closer.Close()
				outcome = r.collectReaderAfterClose(terminalDone, outcome)
			}
			for {
				select {
				case event := <-terminalDone:
					outcome.record(event, outcome.connectionClosed)
				default:
					return outcome
				}
			}
		}
	}
	return outcome
}

func (r *Runtime) collectReaderAfterClose(
	terminalDone <-chan terminalEvent,
	outcome terminalOutcome,
) terminalOutcome {
	timer := time.NewTimer(r.config.ShutdownTimeout)
	defer timer.Stop()
	for !outcome.readerStopped {
		select {
		case event := <-terminalDone:
			outcome.record(event, true)
		case <-timer.C:
			return outcome
		}
	}
	return outcome
}

func (r *Runtime) finishTerminal(externalErr error, outcome terminalOutcome) error {
	driverErr := substantiveDriverError(externalErr, outcome)
	readerErr := substantiveReaderError(externalErr, outcome)
	writerErr := substantiveWriterError(outcome)

	result := errors.Join(externalErr, driverErr, readerErr, writerErr)
	if outcome.timedOut {
		result = errors.Join(result, ErrShutdownTimeout)
	}
	if outcome.earlyConnectionErr != nil {
		result = errors.Join(result, outcome.earlyConnectionErr)
	}

	// Terminal notices and acknowledgements are serialized after the outbound
	// writer stops, preserving the Conn.Send single-caller guarantee.
	if outcome.writerStopped {
		if driverErr != nil {
			r.sendErrorNotice("driver_error", driverErr)
		}
		if readerErr != nil {
			r.sendErrorNotice("protocol_error", readerErr)
		}
		if writerErr != nil {
			r.sendErrorNotice("protocol_error", writerErr)
		}
		if (outcome.readerStopped && outcome.reader.shutdown) ||
			(outcome.driverStopped && outcome.driver.shutdownAccepted) {
			result = r.sendShutdownAck(result)
		}
	}
	return result
}

func substantiveDriverError(externalErr error, outcome terminalOutcome) error {
	if !outcome.driverStopped || outcome.driver.err == nil {
		return nil
	}
	if !errors.Is(outcome.driver.err, context.Canceled) {
		return outcome.driver.err
	}
	if externalErr == nil && outcome.primary == terminalDriver &&
		!outcome.driver.shutdownAccepted &&
		!(outcome.readerStopped && outcome.reader.shutdown) {
		return outcome.driver.err
	}
	return nil
}

func substantiveReaderError(externalErr error, outcome terminalOutcome) error {
	if !outcome.readerStopped {
		return nil
	}
	if outcome.reader.err == nil {
		if outcome.primary == terminalReader && !outcome.reader.shutdown {
			return errors.New("pluginruntime: control reader stopped unexpectedly")
		}
		return nil
	}
	if outcome.readerAfterClose &&
		(errors.Is(outcome.reader.err, io.EOF) || errors.Is(outcome.reader.err, io.ErrClosedPipe)) {
		return nil
	}
	if errors.Is(outcome.reader.err, context.Canceled) {
		if externalErr == nil && outcome.primary == terminalReader && !outcome.reader.shutdown {
			return outcome.reader.err
		}
		return nil
	}
	return outcome.reader.err
}

func substantiveWriterError(outcome terminalOutcome) error {
	if !outcome.writerStopped {
		return nil
	}
	if outcome.writerErr == nil {
		if outcome.primary == terminalWriter {
			return errors.New("pluginruntime: outbound writer stopped unexpectedly")
		}
		return nil
	}
	if errors.Is(outcome.writerErr, context.Canceled) {
		return nil
	}
	if outcome.writerAfterClose &&
		(errors.Is(outcome.writerErr, io.EOF) || errors.Is(outcome.writerErr, io.ErrClosedPipe)) {
		return nil
	}
	return outcome.writerErr
}

func (r *Runtime) writeOutbound(ctx context.Context, host *runtimeHost, started time.Time) error {
	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-host.frames.Notify():
			pending, ok := host.frames.Load()
			if !ok {
				continue
			}
			payload := protocol.TrackingFrame{
				Generation: pending.Generation,
				Frame:      pending.Subscription.TrimFrame(pending.Frame),
			}
			if err := r.send(ctx, payload); err != nil {
				return fmt.Errorf("pluginruntime: send tracking frame: %w", err)
			}

		case <-host.statusNotify:
			status, ok := host.loadStatus()
			if !ok {
				continue
			}
			if err := r.send(ctx, protocol.Status{Status: status}); err != nil {
				return fmt.Errorf("pluginruntime: send status: %w", err)
			}

		case entry := <-host.logs:
			dropped := host.droppedLogs.Swap(0)
			entry.Dropped = dropped
			if err := r.send(ctx, entry); err != nil {
				if dropped != 0 {
					host.droppedLogs.Add(dropped)
				}
				return fmt.Errorf("pluginruntime: send log: %w", err)
			}

		case <-ticker.C:
			elapsed := time.Since(started).Milliseconds()
			if elapsed < 0 {
				elapsed = 0
			}
			if err := r.send(ctx, protocol.Heartbeat{UptimeMS: uint64(elapsed)}); err != nil {
				return fmt.Errorf("pluginruntime: send heartbeat: %w", err)
			}
		}
	}
}

func (r *Runtime) send(ctx context.Context, payload any) error {
	message, err := protocol.NewMessage(payload)
	if err != nil {
		return err
	}
	return r.conn.Send(ctx, message)
}

func (r *Runtime) sendErrorNotice(code string, cause error) {
	message := cause.Error()
	if strings.TrimSpace(message) == "" {
		message = "runtime failure"
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.config.ShutdownTimeout)
	defer cancel()
	_ = r.send(ctx, protocol.Error{Code: code, Message: message})
}

type readerResult struct {
	err      error
	shutdown bool
}

type driverResult struct {
	err              error
	shutdownAccepted bool
}

type terminalState struct {
	mu               sync.Mutex
	shutdownAccepted bool
}

func newTerminalState() *terminalState { return &terminalState{} }

func (s *terminalState) driverReturned(err error) driverResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return driverResult{err: err, shutdownAccepted: s.shutdownAccepted}
}

func (s *terminalState) deliverShutdown(events chan<- pluginapi.ControlEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case events <- pluginapi.ShutdownRequested{}:
		s.shutdownAccepted = true
		return nil
	default:
		return ErrControlBackpressure
	}
}

func (r *Runtime) readControls(ctx context.Context, host *runtimeHost, terminal *terminalState) readerResult {
	for {
		message, err := r.conn.Receive(ctx)
		if err != nil {
			return readerResult{err: err}
		}
		if err := message.Validate(); err != nil {
			return readerResult{err: fmt.Errorf("pluginruntime: invalid control message: %w", err)}
		}

		var event pluginapi.ControlEvent
		shutdown := false
		switch payload := message.Payload.(type) {
		case protocol.ConfigChanged:
			event, err = host.applyConfig(payload.Config)
		case protocol.SubscriptionChanged:
			event, err = host.applySubscription(payload.Subscription)
		case protocol.ActiveChanged:
			event = host.applyActive(payload.Active)
		case protocol.Shutdown:
			if err := terminal.deliverShutdown(host.events); err != nil {
				return readerResult{err: err}
			}
			shutdown = true
		default:
			err = fmt.Errorf("pluginruntime: unexpected control message %T", message.Payload)
		}
		if err != nil {
			return readerResult{err: err}
		}
		if event != nil {
			select {
			case host.events <- event:
			default:
				return readerResult{err: ErrControlBackpressure}
			}
		}
		if shutdown {
			return readerResult{shutdown: true}
		}
	}
}

type runtimeHost struct {
	mu            sync.RWMutex
	initial       pluginapi.Startup
	current       pluginapi.Startup
	events        chan pluginapi.ControlEvent
	frames        *LatestFrameSlot
	statusNotify  chan struct{}
	status        pluginapi.DeviceStatus
	statusPending bool
	logs          chan protocol.Log
	droppedLogs   atomic.Uint64
	stopped       bool
	closed        sync.Once
}

func newRuntimeHost(startup pluginapi.Startup, controlCapacity, logCapacity int) *runtimeHost {
	return &runtimeHost{
		initial:      cloneStartup(startup),
		current:      cloneStartup(startup),
		events:       make(chan pluginapi.ControlEvent, controlCapacity),
		frames:       NewLatestFrameSlot(),
		statusNotify: make(chan struct{}, 1),
		logs:         make(chan protocol.Log, logCapacity),
	}
}

func cloneStartup(startup pluginapi.Startup) pluginapi.Startup {
	startup.Config = startup.Config.Clone()
	return startup
}

func (h *runtimeHost) Startup() pluginapi.Startup {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneStartup(h.initial)
}

func (h *runtimeHost) Events() <-chan pluginapi.ControlEvent { return h.events }

func (h *runtimeHost) PublishFrame(frame trackingmodel.TrackingFrame) bool {
	canonical, err := frame.Canonicalize()
	if err != nil {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	subscription := h.current.Subscription
	if h.stopped || !h.current.Active || subscription.Generation == 0 || subscription.Capabilities == 0 {
		return false
	}
	return h.frames.Store(pendingFrame{
		Generation:   subscription.Generation,
		Subscription: subscription,
		Frame:        canonical,
	})
}

func (h *runtimeHost) PublishStatus(status pluginapi.DeviceStatus) {
	if status.Validate() != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return
	}
	h.status = status
	h.statusPending = true
	select {
	case h.statusNotify <- struct{}{}:
	default:
	}
}

func (h *runtimeHost) loadStatus() (pluginapi.DeviceStatus, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.statusPending {
		return pluginapi.DeviceStatus{}, false
	}
	status := h.status
	h.status = pluginapi.DeviceStatus{}
	h.statusPending = false
	return status, true
}

func (h *runtimeHost) Log(level pluginapi.LogLevel, message string) {
	if level.Validate() != nil || strings.TrimSpace(message) == "" {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.stopped {
		return
	}
	select {
	case h.logs <- protocol.Log{Level: level, Message: message}:
	default:
		h.droppedLogs.Add(1)
	}
}

func (h *runtimeHost) applyConfig(next pluginapi.Config) (pluginapi.ControlEvent, error) {
	next = next.Clone()
	h.mu.Lock()
	defer h.mu.Unlock()
	current := h.current.Config
	switch {
	case next.Revision < current.Revision:
		return nil, fmt.Errorf("pluginruntime: config revision regressed from %d to %d", current.Revision, next.Revision)
	case next.Revision == current.Revision:
		if reflect.DeepEqual(next.Data, current.Data) {
			return nil, nil
		}
		return nil, fmt.Errorf("pluginruntime: config revision %d conflicts with current data", next.Revision)
	default:
		h.current.Config = next.Clone()
		return pluginapi.ConfigChanged{Config: next.Clone()}, nil
	}
}

func (h *runtimeHost) applySubscription(next pluginapi.Subscription) (pluginapi.ControlEvent, error) {
	next = next.Normalize()
	h.mu.Lock()
	defer h.mu.Unlock()
	current := h.current.Subscription
	switch {
	case next.Generation < current.Generation:
		return nil, fmt.Errorf("pluginruntime: subscription generation regressed from %d to %d", current.Generation, next.Generation)
	case next.Generation == current.Generation:
		if next == current {
			return nil, nil
		}
		return nil, fmt.Errorf("pluginruntime: subscription generation %d conflicts with current subscription", next.Generation)
	default:
		h.current.Subscription = next
		h.frames.ClearBefore(next.Generation)
		return pluginapi.SubscriptionChanged{Subscription: next}, nil
	}
}

func (h *runtimeHost) applyActive(active bool) pluginapi.ControlEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	if active == h.current.Active {
		return nil
	}
	h.current.Active = active
	if !active {
		h.frames.Clear()
	}
	return pluginapi.ActiveChanged{Active: active}
}

func (h *runtimeHost) stop() {
	h.mu.Lock()
	h.stopped = true
	h.frames.Clear()
	h.status = pluginapi.DeviceStatus{}
	h.statusPending = false
	select {
	case <-h.statusNotify:
	default:
	}
	h.mu.Unlock()
	h.closed.Do(func() { close(h.events) })
}

var _ pluginapi.Host = (*runtimeHost)(nil)
