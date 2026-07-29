package plugins

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

type RestartPolicy struct {
	InitialBackoff time.Duration
	Multiplier     uint
	MaxBackoff     time.Duration
	MaxFailures    int
	StableWindow   time.Duration
}

func DefaultRestartPolicy() RestartPolicy {
	return RestartPolicy{
		InitialBackoff: time.Second,
		Multiplier:     2,
		MaxBackoff:     30 * time.Second,
		MaxFailures:    5,
		StableWindow:   time.Minute,
	}
}

type supervisorCommandKind uint8

const (
	supervisorEnable supervisorCommandKind = iota + 1
	supervisorDisable
	supervisorActive
	supervisorConfig
	supervisorSubscription
	supervisorRestart
	supervisorClose
)

func (kind supervisorCommandKind) String() string {
	switch kind {
	case supervisorEnable:
		return "enable"
	case supervisorDisable:
		return "disable"
	case supervisorActive:
		return "active"
	case supervisorConfig:
		return "config"
	case supervisorSubscription:
		return "subscription"
	case supervisorRestart:
		return "restart"
	case supervisorClose:
		return "close"
	default:
		return "unknown"
	}
}

type supervisorCommand struct {
	kind         supervisorCommandKind
	config       pluginapi.Config
	subscription pluginapi.Subscription
	active       bool
	reply        chan error
}

type pluginSupervisor interface {
	Command(context.Context, supervisorCommand) error
	Snapshot() RuntimeSnapshot
	Close(context.Context) error
}

type supervisorTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type supervisorSessionCallbacks struct {
	ProcessStarted func(uint64, int)
	Ready          func(uint64)
	Heartbeat      func(uint64, time.Time)
	Frame          func(uint64, time.Time, float64)
	Unresponsive   func(uint64)
	Status         func(uint64, pluginapi.DeviceStatus)
	Log            func(uint64, pluginapi.LogEntry)
}

type supervisorSessionFactory func(
	context.Context,
	uint64,
	pluginapi.Startup,
	supervisorSessionCallbacks,
) pluginSession

type pluginSupervisorConfig struct {
	Plugin             InstalledPlugin
	Preference         PluginPreference
	Active             bool
	Subscription       pluginapi.Subscription
	Restart            RestartPolicy
	NewSession         supervisorSessionFactory
	Now                func() time.Time
	NewTimer           func(time.Duration) supervisorTimer
	Publish            func(RuntimeSnapshot)
	PublishStatus      func(pluginapi.DeviceStatus)
	PublishLog         func(pluginapi.LogEntry)
	SupervisorCapacity int
}

type supervisorCommandEnvelope struct {
	ctx     context.Context
	command supervisorCommand
}

type supervisorSessionOutcome struct {
	instanceID uint64
	result     sessionResult
}

type supervisorControlOutcome struct {
	instanceID  uint64
	operationID uint64
	err         error
}

type supervisorStopOutcome struct {
	instanceID uint64
	err        error
}

type supervisorCallbackKind uint8

const (
	supervisorProcessStarted supervisorCallbackKind = iota + 1
	supervisorReady
	supervisorHeartbeat
	supervisorFrame
	supervisorUnresponsive
	supervisorStatus
	supervisorLog
)

type supervisorCallback struct {
	kind       supervisorCallbackKind
	instanceID uint64
	pid        int
	at         time.Time
	frameRate  float64
	status     pluginapi.DeviceStatus
	log        pluginapi.LogEntry
}

type supervisorStopIntent uint8

const (
	supervisorStopNone supervisorStopIntent = iota
	supervisorStopDisable
	supervisorStopRestart
	supervisorStopClose
)

type serializedPluginSupervisor struct {
	config           pluginSupervisorConfig
	commands         chan supervisorCommandEnvelope
	results          chan supervisorSessionOutcome
	controlResults   chan supervisorControlOutcome
	stops            chan supervisorStopOutcome
	criticalCallback chan supervisorCallback
	callback         chan supervisorCallback
	done             chan struct{}
	snapshot         atomic.Value
}

type supervisorQueuedControl struct {
	envelope      supervisorCommandEnvelope
	request       controlRequest
	durableConfig bool
}

type supervisorInFlightControl struct {
	instanceID    uint64
	operationID   uint64
	envelope      supervisorCommandEnvelope
	request       controlRequest
	next          controlState
	durableConfig bool
	cancel        context.CancelFunc
}

type supervisorLoopState struct {
	snapshot RuntimeSnapshot
	control  controlState

	session        pluginSession
	instanceID     uint64
	launches       int
	timer          supervisorTimer
	timerC         <-chan time.Time
	stableTimer    supervisorTimer
	stableTimerC   <-chan time.Time
	stableInstance uint64
	stopIntent     supervisorStopIntent
	disableReply   chan error
	restartReply   chan error
	closeReply     chan error
	sessionOutcome *sessionResult
	closing        bool

	controlQueue []supervisorQueuedControl
	controlID    uint64
	inFlight     *supervisorInFlightControl
}

func newPluginSupervisor(config pluginSupervisorConfig) (pluginSupervisor, error) {
	if err := validateRestartPolicy(config.Restart); err != nil {
		return nil, err
	}
	if config.NewSession == nil {
		return nil, errors.New("plugins: supervisor session factory is required")
	}
	if err := config.Preference.Config.Validate(); err != nil {
		return nil, errors.New("plugins: supervisor config is invalid")
	}
	if err := config.Subscription.Validate(config.Active); err != nil {
		return nil, errors.New("plugins: supervisor subscription is invalid")
	}
	config.Preference.Config = config.Preference.Config.Clone()
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.NewTimer == nil {
		config.NewTimer = func(delay time.Duration) supervisorTimer {
			return realSupervisorTimer{Timer: time.NewTimer(delay)}
		}
	}
	capacity := config.SupervisorCapacity
	if capacity < 1 {
		capacity = 32
	}
	config.SupervisorCapacity = capacity
	supervisor := &serializedPluginSupervisor{
		config:           config,
		commands:         make(chan supervisorCommandEnvelope, capacity),
		results:          make(chan supervisorSessionOutcome, capacity),
		controlResults:   make(chan supervisorControlOutcome, capacity),
		stops:            make(chan supervisorStopOutcome, capacity),
		criticalCallback: make(chan supervisorCallback, 16),
		callback:         make(chan supervisorCallback, capacity),
		done:             make(chan struct{}),
	}
	initial := RuntimeSnapshot{
		ID:                     config.Plugin.Manifest.ID,
		Name:                   config.Plugin.Manifest.Name,
		Version:                config.Plugin.Manifest.Version,
		Capabilities:           config.Plugin.Manifest.Capabilities,
		Enabled:                config.Preference.Enabled,
		Active:                 config.Active,
		State:                  StateStopped,
		ConfigRevision:         config.Preference.Config.Revision,
		SubscriptionGeneration: config.Subscription.Generation,
	}
	supervisor.snapshot.Store(initial)
	go supervisor.run(initial)
	return supervisor, nil
}

func (s *serializedPluginSupervisor) Command(ctx context.Context, command supervisorCommand) error {
	if ctx == nil {
		return errors.New("plugins: supervisor command context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if command.kind == supervisorConfig {
		command.config = command.config.Clone()
	}
	command.reply = make(chan error, 1)
	envelope := supervisorCommandEnvelope{ctx: ctx, command: command}
	select {
	case <-s.done:
		return ErrManagerClosed
	case s.commands <- envelope:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-command.reply:
		return err
	case <-s.done:
		select {
		case err := <-command.reply:
			return err
		default:
			return ErrManagerClosed
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *serializedPluginSupervisor) Snapshot() RuntimeSnapshot {
	return s.snapshot.Load().(RuntimeSnapshot).clone()
}

func (s *serializedPluginSupervisor) Close(ctx context.Context) error {
	return s.Command(ctx, supervisorCommand{kind: supervisorClose})
}

func (s *serializedPluginSupervisor) run(initial RuntimeSnapshot) {
	state := supervisorLoopState{
		snapshot: initial,
		control: controlState{
			Active:       initial.Active,
			Config:       s.config.Preference.Config.Clone(),
			Subscription: s.config.Subscription,
		},
	}
	s.publish(&state)
	if initial.Enabled {
		s.startSession(&state)
	} else {
		state.snapshot.State = StateDisabled
		s.publish(&state)
	}

	for {
		select {
		case envelope := <-s.commands:
			if s.handleCommand(&state, envelope) {
				close(s.done)
				return
			}
		case outcome := <-s.results:
			s.handleSessionResult(&state, outcome)
		case outcome := <-s.controlResults:
			s.handleControlResult(&state, outcome)
		case outcome := <-s.stops:
			if s.handleStopResult(&state, outcome) {
				close(s.done)
				return
			}
		case callback := <-s.criticalCallback:
			s.handleCallback(&state, callback)
		case callback := <-s.callback:
			s.handleCallback(&state, callback)
		case <-state.timerC:
			state.timer = nil
			state.timerC = nil
			if state.snapshot.Enabled && !state.closing && state.session == nil {
				s.startSession(&state)
			}
		case <-state.stableTimerC:
			instanceID := state.stableInstance
			state.stableTimer = nil
			state.stableTimerC = nil
			state.stableInstance = 0
			if state.session != nil && state.instanceID == instanceID &&
				state.snapshot.State == StateRunning &&
				state.snapshot.ConsecutiveFailures != 0 {
				state.snapshot.ConsecutiveFailures = 0
				s.publish(&state)
			}
		}
	}
}

func (s *serializedPluginSupervisor) handleCommand(
	state *supervisorLoopState,
	envelope supervisorCommandEnvelope,
) bool {
	command := envelope.command
	if err := envelope.ctx.Err(); err != nil {
		command.reply <- err
		return false
	}
	if state.closing {
		command.reply <- ErrManagerClosed
		return false
	}
	if state.stopIntent != supervisorStopNone && command.kind != supervisorClose {
		command.reply <- ErrInvalidState
		return false
	}
	switch command.kind {
	case supervisorEnable:
		if state.snapshot.Enabled {
			if state.snapshot.State == StateStopped && state.session == nil {
				s.startSession(state)
			}
			command.reply <- nil
			return false
		}
		state.snapshot.Enabled = true
		state.snapshot.LastError = ""
		s.publish(state)
		if state.session == nil && state.stopIntent == supervisorStopNone {
			s.cancelTimer(state)
			s.startSession(state)
		}
		command.reply <- nil
	case supervisorDisable:
		state.snapshot.Enabled = false
		s.cancelTimer(state)
		if state.session == nil {
			state.snapshot.State = StateDisabled
			clearRuntimeMetrics(&state.snapshot)
			state.snapshot.NextRestartAt = time.Time{}
			s.publish(state)
			command.reply <- nil
			return false
		}
		if state.stopIntent != supervisorStopNone {
			command.reply <- ErrInvalidState
			return false
		}
		s.beginStop(state, supervisorStopDisable, command.reply)
	case supervisorRestart:
		state.snapshot.ConsecutiveFailures = 0
		state.snapshot.LastError = ""
		s.cancelTimer(state)
		if !state.snapshot.Enabled {
			state.snapshot.State = StateDisabled
			clearRuntimeMetrics(&state.snapshot)
			s.publish(state)
			command.reply <- nil
			return false
		}
		if state.session == nil {
			s.startSession(state)
			command.reply <- nil
			return false
		}
		if state.stopIntent != supervisorStopNone {
			command.reply <- ErrInvalidState
			return false
		}
		s.beginStop(state, supervisorStopRestart, command.reply)
	case supervisorClose:
		state.closing = true
		state.snapshot.Enabled = false
		s.cancelTimer(state)
		s.retireControls(state)
		if state.stopIntent != supervisorStopNone {
			switch state.stopIntent {
			case supervisorStopDisable:
				state.stopIntent = supervisorStopClose
				state.closeReply = command.reply
			case supervisorStopRestart:
				state.restartReply <- ErrInvalidState
				state.restartReply = nil
				state.stopIntent = supervisorStopClose
				state.closeReply = command.reply
			}
			return false
		}
		if state.session == nil {
			state.snapshot.State = StateDisabled
			clearRuntimeMetrics(&state.snapshot)
			s.publish(state)
			command.reply <- nil
			return true
		}
		s.beginStop(state, supervisorStopClose, command.reply)
	default:
		s.handleControlCommand(state, envelope)
	}
	return false
}

func (s *serializedPluginSupervisor) handleControlCommand(
	state *supervisorLoopState,
	envelope supervisorCommandEnvelope,
) {
	command := envelope.command
	if state.closing {
		command.reply <- ErrManagerClosed
		return
	}
	if state.stopIntent != supervisorStopNone || state.snapshot.State == StateStopping {
		command.reply <- ErrInvalidState
		return
	}
	if command.kind == supervisorConfig {
		next, request, changed, err := prepareSupervisorControl(state.control, command)
		if err != nil || !changed {
			command.reply <- err
			return
		}
		// Config is durable user intent by the time it reaches the supervisor.
		// Own it immediately so a failed, canceled, or retired runtime delivery
		// cannot make a later session start with stale persisted state.
		s.commitControl(state, next)
		if state.session == nil {
			command.reply <- nil
			return
		}
		if len(state.controlQueue) >= s.config.SupervisorCapacity &&
			s.config.SupervisorCapacity > 0 {
			command.reply <- ErrControlBackpressure
			return
		}
		state.controlQueue = append(state.controlQueue, supervisorQueuedControl{
			envelope:      envelope,
			request:       request,
			durableConfig: true,
		})
		s.dispatchNextControl(state)
		return
	}
	if state.session != nil &&
		len(state.controlQueue) >= s.config.SupervisorCapacity &&
		s.config.SupervisorCapacity > 0 {
		command.reply <- ErrControlBackpressure
		return
	}
	if state.session == nil {
		next, _, changed, err := prepareSupervisorControl(state.control, command)
		if err != nil || !changed {
			command.reply <- err
			return
		}
		s.commitControl(state, next)
		command.reply <- nil
		return
	}
	state.controlQueue = append(state.controlQueue, supervisorQueuedControl{envelope: envelope})
	s.dispatchNextControl(state)
}

func prepareSupervisorControl(
	current controlState,
	command supervisorCommand,
) (controlState, controlRequest, bool, error) {
	next := current
	next.Config = next.Config.Clone()
	request := controlRequest{state: next}
	var (
		changed bool
		err     error
	)
	switch command.kind {
	case supervisorConfig:
		request.kind = controlConfig
		changed, err = next.applyConfig(command.config)
		request.state.Config = next.Config.Clone()
	case supervisorSubscription:
		request.kind = controlSubscription
		changed, err = next.applySubscription(command.subscription)
		request.state.Subscription = next.Subscription
	case supervisorActive:
		request.kind = controlActive
		changed = next.applyActive(command.active)
		request.state.Active = next.Active
	default:
		err = ErrInvalidState
	}
	return next, request, changed, err
}

func (s *serializedPluginSupervisor) dispatchNextControl(state *supervisorLoopState) {
	for state.inFlight == nil && state.session != nil && len(state.controlQueue) != 0 {
		queued := state.controlQueue[0]
		state.controlQueue = state.controlQueue[1:]
		if err := queued.envelope.ctx.Err(); err != nil {
			queued.envelope.command.reply <- err
			continue
		}
		next := state.control
		request := queued.request
		if !queued.durableConfig {
			var (
				changed bool
				err     error
			)
			next, request, changed, err = prepareSupervisorControl(state.control, queued.envelope.command)
			if err != nil || !changed {
				queued.envelope.command.reply <- err
				continue
			}
		}
		state.controlID++
		operationID := state.controlID
		instanceID := state.instanceID
		controlCtx, cancel := context.WithCancel(queued.envelope.ctx)
		state.inFlight = &supervisorInFlightControl{
			instanceID:    instanceID,
			operationID:   operationID,
			envelope:      queued.envelope,
			request:       request,
			next:          next,
			durableConfig: queued.durableConfig,
			cancel:        cancel,
		}
		session := state.session
		go func() {
			err := session.Control(controlCtx, request)
			select {
			case s.controlResults <- supervisorControlOutcome{
				instanceID:  instanceID,
				operationID: operationID,
				err:         err,
			}:
			case <-s.done:
			}
		}()
	}
}

func (s *serializedPluginSupervisor) handleControlResult(
	state *supervisorLoopState,
	outcome supervisorControlOutcome,
) {
	inFlight := state.inFlight
	if inFlight == nil || outcome.instanceID != inFlight.instanceID ||
		outcome.operationID != inFlight.operationID {
		return
	}
	inFlight.cancel()
	state.inFlight = nil
	if outcome.err == nil {
		if inFlight.durableConfig {
			state.snapshot.LastError = ""
			s.publish(state)
		} else {
			// A durable Config may be accepted while this non-Config runtime
			// control is in flight. Never let its older aggregate state roll
			// the desired Config back.
			inFlight.next.Config = state.control.Config.Clone()
			s.commitControl(state, inFlight.next)
		}
	} else if inFlight.durableConfig {
		state.snapshot.LastError = sanitizedSupervisorError(outcome.err)
		s.publish(state)
	}
	inFlight.envelope.command.reply <- outcome.err
	s.dispatchNextControl(state)
}

func (s *serializedPluginSupervisor) commitControl(
	state *supervisorLoopState,
	next controlState,
) {
	state.control = next
	state.control.Config = next.Config.Clone()
	state.snapshot.Active = next.Active
	state.snapshot.ConfigRevision = next.Config.Revision
	state.snapshot.SubscriptionGeneration = next.Subscription.Generation
	s.publish(state)
}

func (s *serializedPluginSupervisor) retireControls(state *supervisorLoopState) {
	if state.inFlight != nil {
		state.inFlight.cancel()
		state.inFlight.envelope.command.reply <- ErrInvalidState
		state.inFlight = nil
	}
	for _, queued := range state.controlQueue {
		queued.envelope.command.reply <- ErrInvalidState
	}
	state.controlQueue = nil
	state.controlID++
}

func (s *serializedPluginSupervisor) beginStop(
	state *supervisorLoopState,
	intent supervisorStopIntent,
	reply chan error,
) {
	s.retireControls(state)
	s.cancelStableTimer(state)
	state.stopIntent = intent
	switch intent {
	case supervisorStopDisable:
		state.disableReply = reply
	case supervisorStopRestart:
		state.restartReply = reply
	case supervisorStopClose:
		state.closeReply = reply
	}
	state.snapshot.State = StateStopping
	clearRuntimeMetrics(&state.snapshot)
	s.publish(state)
	instanceID := state.instanceID
	session := state.session
	go func() {
		err := session.Stop(context.Background())
		select {
		case s.stops <- supervisorStopOutcome{instanceID: instanceID, err: err}:
		case <-s.done:
		}
	}()
}

func (s *serializedPluginSupervisor) handleStopResult(
	state *supervisorLoopState,
	outcome supervisorStopOutcome,
) bool {
	if outcome.instanceID != state.instanceID || state.stopIntent == supervisorStopNone {
		return false
	}
	intent := state.stopIntent
	state.stopIntent = supervisorStopNone
	state.session = nil
	state.sessionOutcome = nil
	clearRuntimeMetrics(&state.snapshot)
	switch intent {
	case supervisorStopDisable:
		state.snapshot.State = StateDisabled
	case supervisorStopRestart:
		if state.snapshot.Enabled {
			state.snapshot.State = StateStopped
			s.publish(state)
			s.startSession(state)
		} else {
			state.snapshot.State = StateDisabled
		}
	case supervisorStopClose:
		state.snapshot.State = StateDisabled
	}
	s.publish(state)
	if state.disableReply != nil {
		state.disableReply <- outcome.err
		state.disableReply = nil
	}
	if state.restartReply != nil {
		state.restartReply <- outcome.err
		state.restartReply = nil
	}
	if state.closeReply != nil {
		state.closeReply <- outcome.err
		state.closeReply = nil
	}
	return intent == supervisorStopClose
}

func (s *serializedPluginSupervisor) startSession(state *supervisorLoopState) {
	s.cancelStableTimer(state)
	state.snapshot.State = StateStarting
	clearRuntimeMetrics(&state.snapshot)
	state.snapshot.NextRestartAt = time.Time{}
	if state.launches > 0 {
		state.snapshot.RestartCount++
	}
	state.launches++
	state.instanceID++
	instanceID := state.instanceID
	s.publish(state)

	callbacks := s.sessionCallbacks(instanceID)
	startup := pluginapi.Startup{
		Active:       state.control.Active,
		Config:       state.control.Config.Clone(),
		Subscription: state.control.Subscription,
	}
	session := s.config.NewSession(context.Background(), instanceID, startup, callbacks)
	if session == nil {
		state.snapshot.State = StateIncompatible
		state.snapshot.LastError = sanitizedSupervisorError(ErrInvalidState)
		clearRuntimeMetrics(&state.snapshot)
		s.publish(state)
		return
	}
	state.session = session
	go func() {
		result, ok := <-session.Done()
		if !ok {
			result = sessionResult{Err: ErrProtocolViolation, Retryable: false}
		}
		select {
		case s.results <- supervisorSessionOutcome{instanceID: instanceID, result: result}:
		case <-s.done:
		}
	}()
}

func (s *serializedPluginSupervisor) handleSessionResult(
	state *supervisorLoopState,
	outcome supervisorSessionOutcome,
) {
	if outcome.instanceID != state.instanceID || state.session == nil {
		return
	}
	if state.stopIntent != supervisorStopNone {
		result := outcome.result
		state.sessionOutcome = &result
		return
	}
	s.cancelStableTimer(state)
	s.retireControls(state)
	state.session = nil
	clearRuntimeMetrics(&state.snapshot)
	result := outcome.result
	if result.Err == nil {
		state.snapshot.State = StateStopped
		state.snapshot.LastError = ""
		s.publish(state)
		return
	}
	if !result.Retryable {
		state.snapshot.State = StateIncompatible
		state.snapshot.LastError = sanitizedSupervisorError(result.Err)
		state.snapshot.NextRestartAt = time.Time{}
		clearRuntimeMetrics(&state.snapshot)
		s.publish(state)
		return
	}
	if result.StableFor >= s.config.Restart.StableWindow {
		state.snapshot.ConsecutiveFailures = 0
	}
	state.snapshot.ConsecutiveFailures++
	state.snapshot.LastError = sanitizedSupervisorError(result.Err)
	clearRuntimeMetrics(&state.snapshot)
	if !state.snapshot.Enabled || state.snapshot.ConsecutiveFailures >= s.config.Restart.MaxFailures {
		if state.snapshot.ConsecutiveFailures >= s.config.Restart.MaxFailures {
			state.snapshot.LastError = ErrRestartLimitReached.Error()
		}
		state.snapshot.State = StateCrashed
		s.publish(state)
		return
	}
	delay := restartDelay(s.config.Restart, state.snapshot.ConsecutiveFailures)
	state.timer = s.config.NewTimer(delay)
	state.timerC = state.timer.C()
	state.snapshot.State = StateBackoff
	state.snapshot.NextRestartAt = s.config.Now().Add(delay)
	s.publish(state)
}

func (s *serializedPluginSupervisor) sessionCallbacks(instanceID uint64) supervisorSessionCallbacks {
	sendTelemetry := func(callback supervisorCallback) {
		select {
		case s.callback <- callback:
		case <-s.done:
		default:
		}
	}
	sendCritical := func(callback supervisorCallback) {
		select {
		case s.criticalCallback <- callback:
		case <-s.done:
		}
	}
	return supervisorSessionCallbacks{
		ProcessStarted: func(_ uint64, pid int) {
			sendCritical(supervisorCallback{kind: supervisorProcessStarted, instanceID: instanceID, pid: pid})
		},
		Ready: func(_ uint64) {
			sendCritical(supervisorCallback{kind: supervisorReady, instanceID: instanceID})
		},
		Heartbeat: func(_ uint64, at time.Time) {
			sendTelemetry(supervisorCallback{kind: supervisorHeartbeat, instanceID: instanceID, at: at})
		},
		Frame: func(_ uint64, at time.Time, frameRate float64) {
			sendTelemetry(supervisorCallback{kind: supervisorFrame, instanceID: instanceID, at: at, frameRate: frameRate})
		},
		Unresponsive: func(_ uint64) {
			sendCritical(supervisorCallback{kind: supervisorUnresponsive, instanceID: instanceID})
		},
		Status: func(_ uint64, status pluginapi.DeviceStatus) {
			sendTelemetry(supervisorCallback{kind: supervisorStatus, instanceID: instanceID, status: status})
		},
		Log: func(_ uint64, entry pluginapi.LogEntry) {
			sendTelemetry(supervisorCallback{kind: supervisorLog, instanceID: instanceID, log: entry})
		},
	}
}

func (s *serializedPluginSupervisor) handleCallback(
	state *supervisorLoopState,
	callback supervisorCallback,
) {
	if callback.instanceID != state.instanceID || state.session == nil ||
		state.stopIntent != supervisorStopNone {
		return
	}
	switch callback.kind {
	case supervisorProcessStarted:
		if state.snapshot.State != StateStarting {
			return
		}
		state.snapshot.PID = callback.pid
		state.snapshot.State = StateHandshaking
	case supervisorReady:
		if state.snapshot.State != StateHandshaking {
			return
		}
		state.snapshot.State = StateRunning
		state.snapshot.StartedAt = s.config.Now()
		s.armStableTimer(state)
	case supervisorHeartbeat:
		if state.snapshot.State != StateRunning {
			return
		}
		state.snapshot.LastHeartbeatAt = callback.at
	case supervisorFrame:
		if state.snapshot.State != StateRunning {
			return
		}
		state.snapshot.LastFrameAt = callback.at
		state.snapshot.FrameRate = callback.frameRate
	case supervisorUnresponsive:
		if state.snapshot.State != StateRunning {
			return
		}
		s.cancelStableTimer(state)
		state.snapshot.State = StateUnresponsive
	case supervisorStatus:
		if s.config.PublishStatus != nil {
			s.config.PublishStatus(callback.status)
		}
	case supervisorLog:
		if s.config.PublishLog != nil {
			s.config.PublishLog(callback.log)
		}
	default:
		return
	}
	s.publish(state)
}

func (s *serializedPluginSupervisor) cancelTimer(state *supervisorLoopState) {
	if state.timer != nil {
		state.timer.Stop()
	}
	state.timer = nil
	state.timerC = nil
	state.snapshot.NextRestartAt = time.Time{}
}

func (s *serializedPluginSupervisor) armStableTimer(state *supervisorLoopState) {
	s.cancelStableTimer(state)
	if state.snapshot.ConsecutiveFailures == 0 {
		return
	}
	state.stableTimer = s.config.NewTimer(s.config.Restart.StableWindow)
	state.stableTimerC = state.stableTimer.C()
	state.stableInstance = state.instanceID
}

func (s *serializedPluginSupervisor) cancelStableTimer(state *supervisorLoopState) {
	if state.stableTimer != nil {
		state.stableTimer.Stop()
	}
	state.stableTimer = nil
	state.stableTimerC = nil
	state.stableInstance = 0
}

func clearRuntimeMetrics(snapshot *RuntimeSnapshot) {
	snapshot.PID = 0
	snapshot.StartedAt = time.Time{}
	snapshot.LastHeartbeatAt = time.Time{}
	snapshot.LastFrameAt = time.Time{}
	snapshot.FrameRate = 0
}

func (s *serializedPluginSupervisor) publish(state *supervisorLoopState) {
	snapshot := state.snapshot.clone()
	s.snapshot.Store(snapshot)
	if s.config.Publish != nil {
		s.config.Publish(snapshot)
	}
}

func validateRestartPolicy(policy RestartPolicy) error {
	if policy.InitialBackoff <= 0 || policy.Multiplier < 1 ||
		policy.MaxBackoff <= 0 || policy.InitialBackoff > policy.MaxBackoff ||
		policy.MaxFailures < 1 || policy.StableWindow <= 0 {
		return errors.New("plugins: invalid restart policy")
	}
	return nil
}

func restartDelay(policy RestartPolicy, failures int) time.Duration {
	if failures <= 1 {
		return policy.InitialBackoff
	}
	delay := policy.InitialBackoff
	for count := 1; count < failures; count++ {
		if delay >= policy.MaxBackoff {
			return policy.MaxBackoff
		}
		if policy.Multiplier > uint(policy.MaxBackoff/delay) {
			return policy.MaxBackoff
		}
		delay *= time.Duration(policy.Multiplier)
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}

func sanitizedSupervisorError(err error) string {
	for _, known := range []error{
		ErrAuthenticationFailed,
		ErrDescriptorMismatch,
		ErrProtocolIncompatible,
		ErrProtocolViolation,
		ErrInvalidManifest,
		ErrInvalidEntrypoint,
		ErrConfigRevisionRegression,
		ErrConfigRevisionConflict,
		ErrSubscriptionGenerationRegression,
		ErrSubscriptionGenerationConflict,
		ErrHeartbeatTimeout,
		ErrGracefulShutdownTimeout,
		ErrKillTimeout,
		ErrRestartLimitReached,
	} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return "plugins: session failed"
}

type realSupervisorTimer struct {
	*time.Timer
}

func (timer realSupervisorTimer) C() <-chan time.Time { return timer.Timer.C }
