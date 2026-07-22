package pluginruntime

import (
	"context"
	"errors"
	"fmt"
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
	defer func() {
		if closeErr := r.conn.Close(); closeErr != nil {
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
	driverDone := make(chan driverResult, 1)
	readerDone := make(chan readerResult, 1)
	writerDone := make(chan error, 1)
	writerStarted := time.Now()
	go func() { writerDone <- r.writeOutbound(childCtx, host, writerStarted) }()
	go func() { driverDone <- terminal.driverReturned(r.driver.Run(childCtx, host)) }()
	go func() { readerDone <- r.readControls(childCtx, host, terminal) }()

	select {
	case <-ctx.Done():
		cancel()
		writerErr, joined := r.awaitWriter(writerDone)
		_, _ = r.awaitStops(driverDone, readerDone)
		if !joined {
			return errors.Join(ctx.Err(), ErrShutdownTimeout)
		}
		return errors.Join(ctx.Err(), r.reportWriterError(writerErr))

	case driver := <-driverDone:
		if ctxErr := ctx.Err(); ctxErr != nil {
			cancel()
			writerErr, joined := r.awaitWriter(writerDone)
			<-readerDone
			if !joined {
				return errors.Join(ctxErr, ErrShutdownTimeout)
			}
			return errors.Join(ctxErr, r.reportWriterError(writerErr))
		}
		if driver.shutdownAccepted {
			cancel()
			writerErr, joined := r.awaitWriter(writerDone)
			reader := <-readerDone
			if !reader.shutdown {
				return errors.New("pluginruntime: accepted shutdown was not completed by control reader")
			}
			if !joined {
				return errors.Join(driver.err, ErrShutdownTimeout)
			}
			writerErr = r.reportWriterError(writerErr)
			return errors.Join(r.finishAcceptedShutdown(driver.err), writerErr)
		}
		// A driver return owns completion, so cancellation after observing the
		// result cannot turn a spontaneous context.Canceled into success.
		cancel()
		writerErr, joined := r.awaitWriter(writerDone)
		<-readerDone
		if !joined {
			return errors.Join(driver.err, ErrShutdownTimeout)
		}
		writerErr = r.reportWriterError(writerErr)
		if driver.err == nil {
			return writerErr
		}
		r.sendErrorNotice("driver_error", driver.err)
		return errors.Join(driver.err, writerErr)

	case reader := <-readerDone:
		cancel()
		writerErr, joined := r.awaitWriter(writerDone)
		if !joined {
			r.awaitDriver(driverDone)
			return errors.Join(reader.err, ErrShutdownTimeout)
		}
		writerErr = r.reportWriterError(writerErr)
		if ctxErr := ctx.Err(); ctxErr != nil {
			r.awaitDriver(driverDone)
			return errors.Join(ctxErr, writerErr)
		}
		if reader.shutdown {
			return errors.Join(r.finishShutdown(driverDone), writerErr)
		}
		r.awaitDriver(driverDone)
		if reader.err == nil {
			reader.err = errors.New("pluginruntime: control reader stopped unexpectedly")
		}
		r.sendErrorNotice("protocol_error", reader.err)
		return errors.Join(reader.err, writerErr)

	case writerErr := <-writerDone:
		cancel()
		driver, driverStopped := r.awaitStops(driverDone, readerDone)
		if writerErr == nil {
			writerErr = errors.New("pluginruntime: outbound writer stopped unexpectedly")
		}
		writerErr = r.reportWriterError(writerErr)
		var driverErr error
		if driverStopped && driver.err != nil && !errors.Is(driver.err, context.Canceled) {
			driverErr = driver.err
			r.sendErrorNotice("driver_error", driverErr)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(ctxErr, writerErr, driverErr)
		}
		return errors.Join(writerErr, driverErr)
	}
}

func (r *Runtime) finishShutdown(driverDone <-chan driverResult) error {
	timer := time.NewTimer(r.config.ShutdownTimeout)
	defer timer.Stop()

	var result error
	select {
	case driver := <-driverDone:
		result = r.classifyShutdownDriverError(driver.err)
	case <-timer.C:
		result = ErrShutdownTimeout
	}
	return r.sendShutdownAck(result)
}

func (r *Runtime) finishAcceptedShutdown(driverErr error) error {
	return r.sendShutdownAck(r.classifyShutdownDriverError(driverErr))
}

func (r *Runtime) classifyShutdownDriverError(driverErr error) error {
	if driverErr != nil && !errors.Is(driverErr, context.Canceled) {
		r.sendErrorNotice("driver_error", driverErr)
		return driverErr
	}
	return nil
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

func (r *Runtime) awaitStops(driverDone <-chan driverResult, readerDone <-chan readerResult) (driverResult, bool) {
	timer := time.NewTimer(r.config.ShutdownTimeout)
	defer timer.Stop()
	var driver driverResult
	driverStopped := false
	for driverDone != nil || readerDone != nil {
		select {
		case driver = <-driverDone:
			driverDone = nil
			driverStopped = true
		case <-readerDone:
			readerDone = nil
		case <-timer.C:
			return driver, driverStopped
		}
	}
	return driver, driverStopped
}

func (r *Runtime) awaitDriver(driverDone <-chan driverResult) {
	timer := time.NewTimer(r.config.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-driverDone:
	case <-timer.C:
	}
}

func (r *Runtime) awaitWriter(writerDone <-chan error) (error, bool) {
	timer := time.NewTimer(r.config.ShutdownTimeout)
	defer timer.Stop()
	select {
	case err := <-writerDone:
		return err, true
	case <-timer.C:
		return nil, false
	}
}

func relevantWriterError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (r *Runtime) reportWriterError(err error) error {
	err = relevantWriterError(err)
	if err != nil {
		r.sendErrorNotice("protocol_error", err)
	}
	return err
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
	startup       pluginapi.Startup
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
		startup:      cloneStartup(startup),
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
	return cloneStartup(h.startup)
}

func (h *runtimeHost) Events() <-chan pluginapi.ControlEvent { return h.events }

func (h *runtimeHost) PublishFrame(frame trackingmodel.TrackingFrame) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	subscription := h.startup.Subscription
	if h.stopped || !h.startup.Active || subscription.Generation == 0 || subscription.Capabilities == 0 {
		return false
	}
	return h.frames.Store(pendingFrame{
		Generation:   subscription.Generation,
		Subscription: subscription,
		Frame:        frame,
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
	current := h.startup.Config
	switch {
	case next.Revision < current.Revision:
		return nil, fmt.Errorf("pluginruntime: config revision regressed from %d to %d", current.Revision, next.Revision)
	case next.Revision == current.Revision:
		if reflect.DeepEqual(next.Data, current.Data) {
			return nil, nil
		}
		return nil, fmt.Errorf("pluginruntime: config revision %d conflicts with current data", next.Revision)
	default:
		h.startup.Config = next.Clone()
		return pluginapi.ConfigChanged{Config: next.Clone()}, nil
	}
}

func (h *runtimeHost) applySubscription(next pluginapi.Subscription) (pluginapi.ControlEvent, error) {
	next = next.Normalize()
	h.mu.Lock()
	defer h.mu.Unlock()
	current := h.startup.Subscription
	switch {
	case next.Generation < current.Generation:
		return nil, fmt.Errorf("pluginruntime: subscription generation regressed from %d to %d", current.Generation, next.Generation)
	case next.Generation == current.Generation:
		if next == current {
			return nil, nil
		}
		return nil, fmt.Errorf("pluginruntime: subscription generation %d conflicts with current subscription", next.Generation)
	default:
		h.startup.Subscription = next
		h.frames.ClearBefore(next.Generation)
		return pluginapi.SubscriptionChanged{Subscription: next}, nil
	}
}

func (h *runtimeHost) applyActive(active bool) pluginapi.ControlEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	if active == h.startup.Active {
		return nil
	}
	h.startup.Active = active
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
