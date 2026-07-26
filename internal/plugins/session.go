package plugins

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/ipc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type sessionConfig struct {
	Plugin           InstalledPlugin
	Startup          pluginapi.Startup
	HandshakeTimeout time.Duration
	HeartbeatTimeout time.Duration
	GracefulTimeout  time.Duration
	KillTimeout      time.Duration
	ControlCapacity  int
}

type sessionResult struct {
	StartedAt time.Time
	StableFor time.Duration
	Err       error
	Retryable bool
}

type pluginSession interface {
	Control(context.Context, controlRequest) error
	Stop(context.Context) error
	Done() <-chan sessionResult
}

type sessionDependencies struct {
	credentials    func() (string, string, error)
	listen         func(ipc.ServerConfig) (ipc.Listener, error)
	launcher       ProcessLauncher
	frameSink      FrameSink
	onHeartbeat    func(uint64, time.Time)
	onStatus       func(uint64, pluginapi.DeviceStatus)
	onLog          func(uint64, pluginapi.LogEntry)
	onUnresponsive func(uint64)
}

type sessionPhase uint8

const (
	sessionStarting sessionPhase = iota + 1
	sessionReplaying
	sessionReady
	sessionStopping
	sessionEnded
)

const (
	shutdownNotStarted uint32 = iota
	shutdownSending
	shutdownSendComplete
)

type pendingSessionControl struct {
	request controlRequest
	result  chan error
}

type sessionWorkers struct {
	readerCtx     context.Context
	cancelReader  context.CancelFunc
	readerResult  chan error
	processResult chan error
	writerResult  chan error
}

type processSession struct {
	config     sessionConfig
	instanceID uint64
	deps       sessionDependencies

	ctx    context.Context
	cancel context.CancelFunc
	done   chan sessionResult
	ended  chan struct{}

	mu             sync.Mutex
	phase          sessionPhase
	writer         *sessionWriter
	pending        []pendingSessionControl
	result         sessionResult
	stopOnce       sync.Once
	heartbeatPulse chan struct{}
	shutdownAck    chan struct{}
	shutdownState  atomic.Uint32
	ackQueued      atomic.Bool
}

func newPluginSession(parent context.Context, instanceID uint64, config sessionConfig, dependencies sessionDependencies) pluginSession {
	if parent == nil {
		parent = context.Background()
	}
	config.Startup = cloneStartup(config.Startup)
	ctx, cancel := context.WithCancel(parent)
	session := &processSession{
		config:         config,
		instanceID:     instanceID,
		deps:           dependencies,
		ctx:            ctx,
		cancel:         cancel,
		done:           make(chan sessionResult, 1),
		ended:          make(chan struct{}),
		heartbeatPulse: make(chan struct{}, 1),
		shutdownAck:    make(chan struct{}, 1),
		phase:          sessionStarting,
	}
	go session.run()
	return session
}

func (s *processSession) Done() <-chan sessionResult {
	return s.done
}

func (s *processSession) Control(ctx context.Context, request controlRequest) error {
	if ctx == nil {
		return errors.New("plugins: control context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	switch s.phase {
	case sessionStarting, sessionReplaying:
		capacity := s.config.ControlCapacity
		if capacity < 1 {
			capacity = 1
		}
		if len(s.pending) >= capacity {
			s.mu.Unlock()
			return ErrControlBackpressure
		}
		if request.kind == controlConfig {
			request.state.Config = request.state.Config.Clone()
		}
		pending := pendingSessionControl{request: request, result: make(chan error, 1)}
		s.pending = append(s.pending, pending)
		s.mu.Unlock()
		select {
		case err := <-pending.result:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case sessionReady:
		writer := s.writer
		s.mu.Unlock()
		return opaqueWriterControlError(writer, writer.Control(ctx, request))
	default:
		s.mu.Unlock()
		return ErrInvalidState
	}
}

func (s *processSession) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		if s.phase != sessionEnded {
			s.phase = sessionStopping
		}
		s.mu.Unlock()
		s.cancel()
	})
	select {
	case <-s.ended:
		s.mu.Lock()
		err := s.result.Err
		s.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *processSession) run() {
	result := s.startAndWait()
	s.mu.Lock()
	s.phase = sessionEnded
	s.result = result
	for _, pending := range s.pending {
		pending.result <- ErrInvalidState
	}
	s.pending = nil
	s.mu.Unlock()
	s.done <- result
	close(s.done)
	close(s.ended)
}

func (s *processSession) startAndWait() sessionResult {
	var result sessionResult
	credentials := s.deps.credentials
	if credentials == nil {
		credentials = newSessionCredentials
	}
	listen := s.deps.listen
	if listen == nil {
		listen = ipc.Listen
	}
	launcher := s.deps.launcher
	if launcher == nil {
		launcher = NewProcessLauncher()
	}

	pipeName, token, err := credentials()
	if err != nil {
		result.Err = errors.New("plugins: create session credentials")
		return result
	}
	listener, err := listen(ipc.ServerConfig{PipeName: pipeName})
	if err != nil {
		result.Err = errors.New("plugins: create session listener")
		result.Retryable = true
		return result
	}
	listener = &onceSessionListener{Listener: listener}
	defer listener.Close()

	environment, err := launchEnvironment(os.Environ(), pipeName, token)
	if err != nil {
		result.Err = err
		return result
	}
	process, err := launcher.Start(s.ctx, ProcessSpec{
		Executable: s.config.Plugin.Executable,
		WorkingDir: s.config.Plugin.RootDir,
		Env:        environment,
	})
	if err != nil {
		result.Err = err
		result.Retryable = true
		return result
	}

	handshakeCtx, cancelHandshake := context.WithTimeout(s.ctx, s.config.HandshakeTimeout)
	conn, err := listener.Accept(handshakeCtx)
	if err == nil {
		_ = listener.Close()
	}
	if err != nil {
		cancelHandshake()
		result.Err = errors.Join(
			handshakeConnectionError(handshakeCtx, err),
			s.cleanupStartedProcess(process),
		)
		result.Retryable = true
		return result
	}
	conn = &onceSessionConn{Conn: conn}
	defer conn.Close()

	_, err = hostHandshake(handshakeCtx, conn, s.config.Plugin.Manifest, token, s.config.Startup)
	cancelHandshake()
	if err != nil {
		result.Err = errors.Join(err, s.cleanupStartedProcess(process))
		result.Retryable = retryableSessionError(err)
		return result
	}

	result.StartedAt = time.Now()
	initial := controlState{
		Active:       s.config.Startup.Active,
		Config:       s.config.Startup.Config,
		Subscription: s.config.Startup.Subscription,
	}
	writer := newSessionWriter(conn, initial, s.config.ControlCapacity)
	s.mu.Lock()
	s.writer = writer
	if s.phase == sessionStarting {
		s.phase = sessionReplaying
	} else {
		for _, pending := range s.pending {
			pending.result <- ErrInvalidState
		}
		s.pending = nil
	}
	s.mu.Unlock()

	workers := s.startSessionWorkers(conn, process, writer)
	stopped, err := s.replayPendingControls(conn, process, writer, workers)
	if stopped {
		workers.cancelReader()
		result.StableFor = time.Since(result.StartedAt)
		result.Err = err
		result.Retryable = err != nil && retryableSessionError(err)
		return result
	}
	err = s.runRuntime(conn, process, writer, workers)
	result.StableFor = time.Since(result.StartedAt)
	if err != nil {
		result.Err = err
		result.Retryable = retryableSessionError(err)
	}
	return result
}

func (s *processSession) cleanupStartedProcess(process Process) error {
	killErr := process.Kill()
	waitResult := make(chan error, 1)
	go func() { waitResult <- process.Wait() }()
	timer := time.NewTimer(minimumSessionTimeout(s.config.KillTimeout))
	defer timer.Stop()
	select {
	case waitErr := <-waitResult:
		if killErr != nil {
			result := error(opaqueSessionCause{kind: "plugins: process kill failure", cause: killErr})
			if waitErr != nil {
				result = errors.Join(result,
					opaqueSessionCause{kind: "plugins: process cleanup wait failure", cause: waitErr})
			}
			return result
		}
		return nil
	case <-timer.C:
		if killErr != nil {
			return errors.Join(
				opaqueSessionCause{kind: "plugins: process kill failure", cause: killErr},
				ErrKillTimeout,
			)
		}
		return ErrKillTimeout
	}
}

func (s *processSession) startSessionWorkers(
	conn protocol.Conn,
	process Process,
	writer *sessionWriter,
) sessionWorkers {
	readerCtx, cancelReader := context.WithCancel(context.Background())
	workers := sessionWorkers{
		readerCtx:     readerCtx,
		cancelReader:  cancelReader,
		readerResult:  make(chan error, 1),
		processResult: make(chan error, 1),
		writerResult:  make(chan error, 1),
	}
	go func() { workers.readerResult <- s.readRuntime(readerCtx, conn) }()
	go func() { workers.processResult <- process.Wait() }()
	go func() { workers.writerResult <- writer.terminalError() }()
	return workers
}

func (s *processSession) replayPendingControls(
	conn protocol.Conn,
	process Process,
	writer *sessionWriter,
	workers sessionWorkers,
) (bool, error) {
	for {
		s.mu.Lock()
		if s.phase != sessionReplaying {
			s.rejectPendingLocked(ErrInvalidState)
			s.mu.Unlock()
			return true, s.shutdownProcess(
				conn, process, writer,
				workers.processResult, workers.readerResult, workers.writerResult,
				workers.cancelReader,
			)
		}
		if len(s.pending) == 0 {
			s.phase = sessionReady
			s.mu.Unlock()
			return false, nil
		}
		pending := s.pending[0]
		s.pending = s.pending[1:]
		s.mu.Unlock()

		controlResult := make(chan error, 1)
		go func() {
			controlResult <- writer.Control(context.Background(), pending.request)
		}()

		select {
		case err := <-controlResult:
			pending.result <- opaqueWriterControlError(writer, err)
			if terminalErr := writer.currentTerminalError(); terminalErr != nil {
				s.beginReplayStop()
				cleanupErr := s.shutdownProcess(
					conn, process, writer,
					workers.processResult, workers.readerResult, workers.writerResult,
					workers.cancelReader,
				)
				return true, errors.Join(
					opaqueSessionCause{kind: "plugins: IPC writer failure", cause: terminalErr},
					cleanupErr,
				)
			}
		case processErr := <-workers.processResult:
			s.beginReplayStop()
			_ = conn.Close()
			s.completeReplayControl(writer, pending, controlResult)
			return true, unexpectedProcessExit(processErr)
		case readerErr := <-workers.readerResult:
			s.beginReplayStop()
			shutdownErr := s.shutdownProcess(
				conn, process, writer,
				workers.processResult, workers.readerResult, workers.writerResult,
				workers.cancelReader,
			)
			s.completeReplayControl(writer, pending, controlResult)
			return true, errors.Join(readerErr, shutdownErr)
		case writerErr := <-workers.writerResult:
			s.beginReplayStop()
			shutdownErr := s.shutdownProcess(
				conn, process, writer,
				workers.processResult, workers.readerResult, workers.writerResult,
				workers.cancelReader,
			)
			s.completeReplayControl(writer, pending, controlResult)
			if writerErr == nil {
				writerErr = ErrProtocolViolation
			} else {
				writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: writerErr}
			}
			return true, errors.Join(writerErr, shutdownErr)
		case <-s.ctx.Done():
			s.beginReplayStop()
			shutdownErr := s.shutdownProcess(
				conn, process, writer,
				workers.processResult, workers.readerResult, workers.writerResult,
				workers.cancelReader,
			)
			s.completeReplayControl(writer, pending, controlResult)
			return true, shutdownErr
		}
	}
}

func (s *processSession) beginReplayStop() {
	s.mu.Lock()
	if s.phase != sessionEnded {
		s.phase = sessionStopping
	}
	s.rejectPendingLocked(ErrInvalidState)
	s.mu.Unlock()
}

func (s *processSession) rejectPendingLocked(err error) {
	for _, pending := range s.pending {
		pending.result <- err
	}
	s.pending = nil
}

func (s *processSession) completeReplayControl(
	writer *sessionWriter,
	pending pendingSessionControl,
	controlResult <-chan error,
) {
	select {
	case err := <-controlResult:
		pending.result <- opaqueWriterControlError(writer, err)
	case <-time.After(minimumSessionTimeout(s.config.KillTimeout)):
		pending.result <- ErrInvalidState
	}
}

func unexpectedProcessExit(err error) error {
	if err == nil {
		return errors.New("plugins: process exited unexpectedly")
	}
	return opaqueSessionCause{kind: "plugins: process exited unexpectedly", cause: err}
}

func (s *processSession) runRuntime(
	conn protocol.Conn,
	process Process,
	writer *sessionWriter,
	workers sessionWorkers,
) error {
	defer workers.cancelReader()
	readerResult := workers.readerResult
	processResult := workers.processResult
	writerResult := workers.writerResult
	watchdogResult := make(chan error, 1)
	stopWatchdog := make(chan struct{})
	defer close(stopWatchdog)
	go s.watchHeartbeat(stopWatchdog, watchdogResult)

	select {
	case processErr := <-processResult:
		if s.isSessionStopping() {
			return s.shutdownProcess(
				conn, process, writer, singleProcessResult(processErr),
				readerResult, writerResult, workers.cancelReader,
			)
		}
		return s.finishAfterProcessExit(conn, writer, workers.cancelReader, readerResult, writerResult, processErr)
	case readerErr := <-readerResult:
		if readerErr == nil {
			readerErr = ErrProtocolViolation
		}
		return errors.Join(readerErr,
			s.shutdownProcess(conn, process, writer, processResult, readerResult, writerResult, workers.cancelReader))
	case watchdogErr := <-watchdogResult:
		if s.deps.onUnresponsive != nil {
			s.deps.onUnresponsive(s.instanceID)
		}
		return errors.Join(watchdogErr,
			s.shutdownProcess(conn, process, writer, processResult, readerResult, writerResult, workers.cancelReader))
	case writerErr := <-writerResult:
		if writerErr == nil {
			writerErr = ErrProtocolViolation
		} else {
			writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: writerErr}
		}
		return errors.Join(writerErr,
			s.shutdownProcess(conn, process, writer, processResult, readerResult, writerResult, workers.cancelReader))
	case <-s.ctx.Done():
		select {
		case readerErr := <-readerResult:
			if readerErr != nil {
				return errors.Join(readerErr,
					s.shutdownProcess(conn, process, writer, processResult, readerResult, writerResult, workers.cancelReader))
			}
		case processErr := <-processResult:
			return s.shutdownProcess(
				conn, process, writer, singleProcessResult(processErr),
				readerResult, writerResult, workers.cancelReader,
			)
		default:
		}
		return s.shutdownProcess(conn, process, writer, processResult, readerResult, writerResult, workers.cancelReader)
	}
}

func (s *processSession) isSessionStopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase == sessionStopping
}

func singleProcessResult(err error) <-chan error {
	result := make(chan error, 1)
	result <- err
	return result
}

func (s *processSession) readRuntime(ctx context.Context, conn protocol.Conn) error {
	for {
		message, err := conn.Receive(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
				return nil
			}
			return opaqueSessionCause{kind: "plugins: IPC reader failure", cause: err}
		}
		switch payload := message.Payload.(type) {
		case protocol.Heartbeat:
			select {
			case s.heartbeatPulse <- struct{}{}:
			default:
			}
			if s.deps.onHeartbeat != nil {
				s.deps.onHeartbeat(s.instanceID, time.Now())
			}
		case protocol.TrackingFrame:
			if s.deps.frameSink != nil {
				s.deps.frameSink.Submit(s.config.Plugin.Manifest.ID, payload.Generation, payload.Frame)
			}
		case protocol.Status:
			if s.deps.onStatus != nil {
				s.deps.onStatus(s.instanceID, payload.Status)
			}
		case protocol.Log:
			if s.deps.onLog != nil {
				s.deps.onLog(s.instanceID, pluginapi.LogEntry{
					Time:     time.Now(),
					PluginID: s.config.Plugin.Manifest.ID,
					Level:    payload.Level,
					Message:  payload.Message,
				})
			}
		case protocol.ShutdownAck:
			switch s.shutdownState.Load() {
			case shutdownNotStarted:
				return ErrProtocolViolation
			case shutdownSending:
				s.ackQueued.Store(true)
				if s.shutdownState.Load() == shutdownSendComplete {
					s.signalShutdownAck()
				}
			case shutdownSendComplete:
				s.signalShutdownAck()
			}
		case protocol.Error:
			return errors.New("plugins: peer reported an error")
		default:
			return ErrProtocolViolation
		}
	}
}

func (s *processSession) watchHeartbeat(stop <-chan struct{}, result chan<- error) {
	timeout := minimumSessionTimeout(s.config.HeartbeatTimeout)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-s.heartbeatPulse:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case <-timer.C:
			result <- ErrHeartbeatTimeout
			return
		}
	}
}

func (s *processSession) signalShutdownAck() {
	select {
	case s.shutdownAck <- struct{}{}:
	default:
	}
}

func (s *processSession) finishAfterProcessExit(
	conn protocol.Conn,
	writer *sessionWriter,
	cancelReader context.CancelFunc,
	readerResult <-chan error,
	writerResult <-chan error,
	processErr error,
) error {
	var writerErr error
	if err := writer.currentTerminalError(); err != nil {
		writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: err}
	}
	var (
		readerErr       error
		readerCompleted bool
	)
	select {
	case readerErr = <-readerResult:
		readerCompleted = true
	case <-time.After(terminalDrainWindow(s.config.GracefulTimeout)):
	}
	cancelReader()
	_ = conn.Close()
	s.stopWriter(writer)
	if processErr != nil && writerErr == nil {
		select {
		case err := <-writerResult:
			if err != nil {
				writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: err}
			}
		case <-time.After(terminalDrainWindow(s.config.GracefulTimeout)):
		}
	}
	result := unexpectedProcessExit(processErr)
	if readerCompleted {
		result = errors.Join(result, readerErr)
	} else {
		select {
		case readerErr = <-readerResult:
			result = errors.Join(result, readerErr)
		case <-time.After(minimumSessionTimeout(s.config.KillTimeout)):
		}
	}
	return errors.Join(result, writerErr)
}

func (s *processSession) stopWriter(writer *sessionWriter) {
	ctx, cancel := context.WithTimeout(context.Background(), minimumSessionTimeout(s.config.GracefulTimeout))
	defer cancel()
	_ = writer.Control(ctx, controlRequest{kind: controlShutdown})
}

func (s *processSession) shutdownProcess(
	conn protocol.Conn,
	process Process,
	writer *sessionWriter,
	processResult <-chan error,
	readerResult <-chan error,
	writerResult <-chan error,
	cancelReader context.CancelFunc,
) error {
	graceful := minimumSessionTimeout(s.config.GracefulTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), graceful)
	defer cancel()
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- writer.Control(ctx, controlRequest{
			kind: controlShutdown,
			onSendAttempt: func() {
				s.shutdownState.Store(shutdownSending)
			},
		})
	}()
	timer := time.NewTimer(graceful)
	defer timer.Stop()
	var (
		acked        bool
		exited       bool
		shutdownSent bool
		processErr   error
		readerErr    error
		writerErr    error
	)
	for !(acked && exited && shutdownSent) {
		select {
		case err := <-shutdownResult:
			shutdownSent = err == nil
			if err == nil {
				s.shutdownState.Store(shutdownSendComplete)
				if s.ackQueued.Load() {
					s.signalShutdownAck()
				}
			}
			if err != nil {
				if errors.Is(err, ErrInvalidState) {
					if terminalErr := writer.currentTerminalError(); terminalErr != nil {
						writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: terminalErr}
						continue
					}
					select {
					case <-writer.Done():
						if terminalErr := writer.terminalError(); terminalErr != nil {
							writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: terminalErr}
						}
					default:
						// The writer-result worker will publish the terminal
						// transport cause once the owner has fully stopped.
					}
				} else {
					writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: err}
				}
			}
		case <-s.shutdownAck:
			acked = true
		case err := <-processResult:
			exited = true
			processErr = err
		case err := <-readerResult:
			if err != nil {
				readerErr = err
			}
		case err := <-writerResult:
			if err != nil {
				writerErr = opaqueSessionCause{kind: "plugins: IPC writer failure", cause: err}
			}
		case <-timer.C:
			goto timeout
		}
	}
	cancelReader()
	_ = conn.Close()
	if processErr != nil {
		processErr = opaqueSessionCause{kind: "plugins: process exited during shutdown", cause: processErr}
	}
	return errors.Join(writerErr, readerErr, processErr)

timeout:
	cancelReader()
	_ = conn.Close()
	select {
	case err := <-readerResult:
		if err != nil {
			readerErr = errors.Join(readerErr, err)
		}
	case <-time.After(terminalDrainWindow(s.config.GracefulTimeout)):
	}
	result := errors.Join(ErrGracefulShutdownTimeout, writerErr, readerErr)
	if exited {
		if processErr != nil {
			result = errors.Join(result,
				opaqueSessionCause{kind: "plugins: process exited during shutdown", cause: processErr})
		}
		return result
	}
	killErr := process.Kill()
	if killErr != nil {
		result = errors.Join(result,
			opaqueSessionCause{kind: "plugins: process kill failure", cause: killErr})
	}
	killTimeout := minimumSessionTimeout(s.config.KillTimeout)
	select {
	case <-processResult:
		return result
	case <-time.After(killTimeout):
		return errors.Join(result, ErrKillTimeout)
	}
}

func minimumSessionTimeout(value time.Duration) time.Duration {
	if value <= 0 {
		return time.Millisecond
	}
	return value
}

func terminalDrainWindow(value time.Duration) time.Duration {
	const maximum = 10 * time.Millisecond
	value = minimumSessionTimeout(value)
	if value > maximum {
		return maximum
	}
	return value
}

func retryableSessionError(err error) bool {
	return !errors.Is(err, ErrAuthenticationFailed) &&
		!errors.Is(err, ErrDescriptorMismatch) &&
		!errors.Is(err, ErrProtocolIncompatible)
}

func opaqueWriterControlError(writer *sessionWriter, err error) error {
	if err == nil || writer == nil {
		return err
	}
	terminalErr := writer.currentTerminalError()
	if terminalErr == nil || !errors.Is(err, terminalErr) {
		return err
	}
	return opaqueSessionCause{kind: "plugins: IPC writer failure", cause: terminalErr}
}

type opaqueSessionCause struct {
	kind  string
	cause error
}

func (e opaqueSessionCause) Error() string { return e.kind }
func (e opaqueSessionCause) Is(target error) bool {
	return errors.Is(e.cause, target)
}

type onceSessionListener struct {
	ipc.Listener
	once sync.Once
	err  error
}

func (l *onceSessionListener) Close() error {
	l.once.Do(func() { l.err = l.Listener.Close() })
	return l.err
}

type onceSessionConn struct {
	protocol.Conn
	once sync.Once
	err  error
}

func (c *onceSessionConn) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close() })
	return c.err
}
