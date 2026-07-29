package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

type Options struct {
	HandshakeTimeout time.Duration
	HeartbeatTimeout time.Duration
	GracefulTimeout  time.Duration
	KillTimeout      time.Duration
	ControlCapacity  int
	EventCapacity    int
	Restart          RestartPolicy
}

func DefaultOptions() Options {
	// These are the host's v1 lifecycle and queue defaults. Applications with
	// different process or IPC budgets can override every value through Options.
	return Options{
		HandshakeTimeout: 5 * time.Second,
		HeartbeatTimeout: 5 * time.Second,
		GracefulTimeout:  2 * time.Second,
		KillTimeout:      2 * time.Second,
		ControlCapacity:  32,
		EventCapacity:    64,
		Restart:          DefaultRestartPolicy(),
	}
}

type managerDependencies struct {
	newSupervisor func(pluginSupervisorConfig) (pluginSupervisor, error)
	newSession    func(context.Context, uint64, sessionConfig, sessionDependencies) pluginSession
}

type managerLifecycle uint8

const (
	managerNotStarted managerLifecycle = iota
	managerStarting
	managerStarted
	managerClosing
	managerClosed
)

type pluginManager struct {
	catalog   Catalog
	store     Store
	launcher  ProcessLauncher
	frameSink FrameSink
	options   Options
	deps      managerDependencies
	events    *eventHub

	mu          sync.RWMutex
	lifecycle   managerLifecycle
	supervisors map[string]pluginSupervisor
	admissions  map[string]chan struct{}
	ids         []string
	settings    PluginSettings
	startDone   chan struct{}

	persistToken chan struct{}
	closeOnce    sync.Once
	closeDone    chan struct{}
	closeErr     error
}

func NewManager(
	catalog Catalog,
	store Store,
	launcher ProcessLauncher,
	frameSink FrameSink,
	options Options,
) (Manager, error) {
	return newManager(catalog, store, launcher, frameSink, options, managerDependencies{})
}

func newManager(
	catalog Catalog,
	store Store,
	launcher ProcessLauncher,
	frameSink FrameSink,
	options Options,
	dependencies managerDependencies,
) (Manager, error) {
	if catalog == nil {
		return nil, errors.New("plugins: manager catalog is required")
	}
	if store == nil {
		return nil, errors.New("plugins: manager store is required")
	}
	if launcher == nil {
		return nil, errors.New("plugins: manager process launcher is required")
	}
	if frameSink == nil {
		return nil, errors.New("plugins: manager frame sink is required")
	}
	if err := validateManagerOptions(options); err != nil {
		return nil, err
	}
	if dependencies.newSupervisor == nil {
		dependencies.newSupervisor = newPluginSupervisor
	}
	if dependencies.newSession == nil {
		dependencies.newSession = newPluginSession
	}
	startDone := make(chan struct{})
	close(startDone)
	manager := &pluginManager{
		catalog:      catalog,
		store:        store,
		launcher:     launcher,
		frameSink:    frameSink,
		options:      options,
		deps:         dependencies,
		events:       newEventHub(options.EventCapacity),
		supervisors:  make(map[string]pluginSupervisor),
		admissions:   make(map[string]chan struct{}),
		settings:     emptyPluginSettings(),
		startDone:    startDone,
		persistToken: make(chan struct{}, 1),
		closeDone:    make(chan struct{}),
	}
	manager.persistToken <- struct{}{}
	return manager, nil
}

func validateManagerOptions(options Options) error {
	if options.HandshakeTimeout <= 0 || options.HeartbeatTimeout <= 0 ||
		options.GracefulTimeout <= 0 || options.KillTimeout <= 0 ||
		options.ControlCapacity <= 0 || options.EventCapacity <= 0 {
		return errors.New("plugins: invalid manager options")
	}
	if err := validateRestartPolicy(options.Restart); err != nil {
		return errors.New("plugins: invalid manager options")
	}
	return nil
}

func (m *pluginManager) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("plugins: manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	switch m.lifecycle {
	case managerClosing, managerClosed:
		m.mu.Unlock()
		return ErrManagerClosed
	case managerNotStarted:
	default:
		m.mu.Unlock()
		return ErrInvalidState
	}
	m.lifecycle = managerStarting
	m.startDone = make(chan struct{})
	startDone := m.startDone
	m.mu.Unlock()

	plugins, err := m.catalog.Scan(ctx)
	if err != nil {
		m.finishFailedStart(startDone)
		return err
	}
	settings, err := m.store.Load(ctx)
	if err != nil {
		m.finishFailedStart(startDone)
		return err
	}
	settings = clonePluginSettings(settings)

	installed := make(map[string]InstalledPlugin, len(plugins))
	ids := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		id := plugin.Manifest.ID
		if _, exists := installed[id]; exists {
			m.finishFailedStart(startDone)
			return fmt.Errorf("%w: %s", ErrDuplicatePluginID, id)
		}
		installed[id] = plugin
		ids = append(ids, id)
	}
	sort.Strings(ids)

	supervisors := make(map[string]pluginSupervisor, len(ids))
	admissions := make(map[string]chan struct{}, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			rollbackErr := closePluginSupervisors(supervisors)
			m.finishFailedStart(startDone)
			return errors.Join(err, rollbackErr)
		}
		plugin := installed[id]
		preference := settings.Plugins[id]
		preference.Config = preference.Config.Clone()
		supervisor, err := m.deps.newSupervisor(m.supervisorConfig(plugin, preference))
		if err != nil {
			rollbackErr := closePluginSupervisors(supervisors)
			m.finishFailedStart(startDone)
			return errors.Join(err, rollbackErr)
		}
		supervisors[id] = supervisor
		admission := make(chan struct{}, 1)
		admission <- struct{}{}
		admissions[id] = admission
	}

	m.mu.Lock()
	if m.lifecycle != managerStarting {
		m.mu.Unlock()
		rollbackErr := closePluginSupervisors(supervisors)
		close(startDone)
		if rollbackErr != nil {
			return errors.Join(ErrManagerClosed, rollbackErr)
		}
		return ErrManagerClosed
	}
	m.supervisors = supervisors
	m.admissions = admissions
	m.ids = append([]string(nil), ids...)
	m.settings = settings
	m.lifecycle = managerStarted
	close(startDone)
	m.mu.Unlock()

	for _, id := range ids {
		snapshot := supervisors[id].Snapshot()
		m.events.Publish(Event{
			Type:     EventPluginDiscovered,
			PluginID: id,
			Snapshot: &snapshot,
		})
		m.publishSnapshot(id, snapshot)
	}
	return nil
}

func (m *pluginManager) finishFailedStart(startDone chan struct{}) {
	m.mu.Lock()
	if m.lifecycle == managerStarting {
		m.lifecycle = managerNotStarted
	}
	close(startDone)
	m.mu.Unlock()
}

func (m *pluginManager) supervisorConfig(
	plugin InstalledPlugin,
	preference PluginPreference,
) pluginSupervisorConfig {
	id := plugin.Manifest.ID
	return pluginSupervisorConfig{
		Plugin:       plugin,
		Preference:   preference,
		Restart:      m.options.Restart,
		Subscription: pluginapi.Subscription{},
		NewSession: func(
			ctx context.Context,
			instanceID uint64,
			startup pluginapi.Startup,
			callbacks supervisorSessionCallbacks,
		) pluginSession {
			return m.deps.newSession(ctx, instanceID, sessionConfig{
				Plugin:           plugin,
				Startup:          startup,
				HandshakeTimeout: m.options.HandshakeTimeout,
				HeartbeatTimeout: m.options.HeartbeatTimeout,
				GracefulTimeout:  m.options.GracefulTimeout,
				KillTimeout:      m.options.KillTimeout,
				ControlCapacity:  m.options.ControlCapacity,
			}, sessionDependencies{
				launcher:         m.launcher,
				frameSink:        m.frameSink,
				onProcessStarted: callbacks.ProcessStarted,
				onReady:          callbacks.Ready,
				onHeartbeat:      callbacks.Heartbeat,
				onUnresponsive:   callbacks.Unresponsive,
				onStatus:         callbacks.Status,
				onLog:            callbacks.Log,
			})
		},
		Publish: func(snapshot RuntimeSnapshot) {
			m.publishSnapshot(id, snapshot)
		},
		PublishStatus: func(status pluginapi.DeviceStatus) {
			snapshot, _ := m.Get(id)
			statusCopy := status
			m.publishIfStarted(Event{
				Type:     EventPluginStatus,
				PluginID: id,
				Snapshot: &snapshot,
				Status:   &statusCopy,
			})
		},
		PublishLog: func(entry pluginapi.LogEntry) {
			entry.PluginID = id
			m.publishIfStarted(Event{
				Type:     EventPluginLog,
				PluginID: id,
				Log:      &entry,
			})
		},
		SupervisorCapacity: m.options.ControlCapacity,
	}
}

func (m *pluginManager) publishSnapshot(id string, snapshot RuntimeSnapshot) {
	m.publishIfStarted(Event{
		Type:     EventPluginStateChanged,
		PluginID: id,
		Snapshot: &snapshot,
	})
}

func (m *pluginManager) publishIfStarted(event Event) {
	m.mu.RLock()
	_, exists := m.supervisors[event.PluginID]
	started := m.lifecycle == managerStarted && exists
	m.mu.RUnlock()
	if started {
		m.events.Publish(event)
	}
}

func (m *pluginManager) List() []RuntimeSnapshot {
	m.mu.RLock()
	ids := append([]string(nil), m.ids...)
	supervisors := make([]pluginSupervisor, len(ids))
	preferences := make([]PluginPreference, len(ids))
	for index, id := range ids {
		supervisors[index] = m.supervisors[id]
		preferences[index] = m.settings.Plugins[id]
	}
	m.mu.RUnlock()
	snapshots := make([]RuntimeSnapshot, len(ids))
	for index, supervisor := range supervisors {
		snapshots[index] = overlayPreference(supervisor.Snapshot(), preferences[index])
	}
	return snapshots
}

func (m *pluginManager) Get(id string) (RuntimeSnapshot, bool) {
	m.mu.RLock()
	supervisor, exists := m.supervisors[id]
	preference := m.settings.Plugins[id]
	m.mu.RUnlock()
	if !exists {
		return RuntimeSnapshot{}, false
	}
	return overlayPreference(supervisor.Snapshot(), preference), true
}

func overlayPreference(snapshot RuntimeSnapshot, preference PluginPreference) RuntimeSnapshot {
	snapshot.Enabled = preference.Enabled
	snapshot.ConfigRevision = preference.Config.Revision
	return snapshot.clone()
}

func (m *pluginManager) Enable(ctx context.Context, id string) error {
	return m.updateEnabled(ctx, id, true, supervisorEnable)
}

func (m *pluginManager) Disable(ctx context.Context, id string) error {
	return m.updateEnabled(ctx, id, false, supervisorDisable)
}

func (m *pluginManager) updateEnabled(
	ctx context.Context,
	id string,
	enabled bool,
	kind supervisorCommandKind,
) error {
	return m.persistAndCommand(ctx, id, func(settings *PluginSettings) (bool, error) {
		preference := settings.Plugins[id]
		if preference.Enabled == enabled {
			return false, nil
		}
		preference.Enabled = enabled
		preference.Config = preference.Config.Clone()
		settings.Plugins[id] = preference
		return true, nil
	}, supervisorCommand{kind: kind})
}

func (m *pluginManager) UpdateConfig(
	ctx context.Context,
	id string,
	config pluginapi.Config,
) error {
	config = config.Clone()
	return m.persistAndCommand(ctx, id, func(settings *PluginSettings) (bool, error) {
		preference := settings.Plugins[id]
		state := controlState{Config: preference.Config.Clone()}
		changed, err := state.applyConfig(config)
		if err != nil || !changed {
			return changed, err
		}
		preference.Config = state.Config.Clone()
		settings.Plugins[id] = preference
		return true, nil
	}, supervisorCommand{kind: supervisorConfig, config: config})
}

func (m *pluginManager) persistAndCommand(
	ctx context.Context,
	id string,
	update func(*PluginSettings) (bool, error),
	command supervisorCommand,
) error {
	if ctx == nil {
		return errors.New("plugins: manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := m.controlTarget(id)
	if err != nil {
		return err
	}
	admissionToken := target.admission
	if err := acquireManagerToken(ctx, admissionToken); err != nil {
		return err
	}
	admissionHeld := true
	releaseAdmission := func() {
		if admissionHeld {
			admissionToken <- struct{}{}
			admissionHeld = false
		}
	}
	defer releaseAdmission()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.persistToken:
	}
	tokenHeld := true
	releaseToken := func() {
		if tokenHeld {
			m.persistToken <- struct{}{}
			tokenHeld = false
		}
	}
	defer releaseToken()

	currentTarget, err := m.controlTarget(id)
	if err != nil {
		return err
	}
	supervisor := currentTarget.supervisor
	m.mu.RLock()
	next := clonePluginSettings(m.settings)
	m.mu.RUnlock()
	changed, err := update(&next)
	if err != nil {
		return err
	}
	if changed {
		if err := m.store.Save(ctx, next); err != nil {
			return err
		}
		m.mu.Lock()
		m.settings = clonePluginSettings(next)
		m.mu.Unlock()
	}
	// The whole-settings Save must be serialized, but a plugin's runtime
	// delivery must not hold that serialization and stall other plugins.
	releaseToken()
	result, admission := beginSupervisorCommand(
		context.WithoutCancel(ctx),
		supervisor,
		command,
	)
	admissionErr := admission.wait()
	releaseAdmission()
	return finishManagerCommand(ctx, result, admissionErr)
}

func (m *pluginManager) Restart(ctx context.Context, id string) error {
	return m.command(ctx, id, supervisorCommand{kind: supervisorRestart})
}

func (m *pluginManager) SetActive(ctx context.Context, id string, active bool) error {
	return m.command(ctx, id, supervisorCommand{kind: supervisorActive, active: active})
}

func (m *pluginManager) UpdateSubscription(
	ctx context.Context,
	id string,
	subscription pluginapi.Subscription,
) error {
	return m.command(ctx, id, supervisorCommand{
		kind:         supervisorSubscription,
		subscription: subscription,
	})
}

func (m *pluginManager) command(ctx context.Context, id string, command supervisorCommand) error {
	if ctx == nil {
		return errors.New("plugins: manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := m.controlTarget(id)
	if err != nil {
		return err
	}
	if err := acquireManagerToken(ctx, target.admission); err != nil {
		return err
	}
	result, admission := beginSupervisorCommand(ctx, target.supervisor, command)
	admissionErr := admission.wait()
	target.admission <- struct{}{}
	return finishManagerCommand(ctx, result, admissionErr)
}

type managerControlTarget struct {
	supervisor pluginSupervisor
	admission  chan struct{}
}

func (m *pluginManager) controlTarget(id string) (managerControlTarget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	switch m.lifecycle {
	case managerClosing, managerClosed:
		return managerControlTarget{}, ErrManagerClosed
	case managerStarted:
	default:
		return managerControlTarget{}, ErrManagerNotStarted
	}
	supervisor, exists := m.supervisors[id]
	if !exists {
		return managerControlTarget{}, ErrUnknownPlugin
	}
	return managerControlTarget{
		supervisor: supervisor,
		admission:  m.admissions[id],
	}, nil
}

func acquireManagerToken(ctx context.Context, token chan struct{}) error {
	select {
	case <-token:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func beginSupervisorCommand(
	ctx context.Context,
	supervisor pluginSupervisor,
	command supervisorCommand,
) (<-chan error, *supervisorAdmission) {
	admission := newSupervisorAdmission()
	command.admission = admission
	result := make(chan error, 1)
	go func() {
		err := supervisor.Command(ctx, command)
		admission.signal(err)
		result <- err
	}()
	return result, admission
}

func finishManagerCommand(
	ctx context.Context,
	result <-chan error,
	admissionErr error,
) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if admissionErr != nil {
		return admissionErr
	}
	select {
	case err := <-result:
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *pluginManager) Subscribe(ctx context.Context) <-chan Event {
	return m.events.Subscribe(ctx)
}

func (m *pluginManager) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("plugins: manager context is required")
	}
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.lifecycle = managerClosing
		startDone := m.startDone
		m.mu.Unlock()
		go m.finishClose(startDone)
	})
	select {
	case <-m.closeDone:
		m.mu.RLock()
		err := m.closeErr
		m.mu.RUnlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *pluginManager) finishClose(startDone <-chan struct{}) {
	<-startDone
	m.mu.RLock()
	supervisors := make(map[string]pluginSupervisor, len(m.supervisors))
	for id, supervisor := range m.supervisors {
		supervisors[id] = supervisor
	}
	m.mu.RUnlock()
	err := closePluginSupervisors(supervisors)
	m.events.Close()
	m.mu.Lock()
	m.closeErr = err
	m.lifecycle = managerClosed
	close(m.closeDone)
	m.mu.Unlock()
}

func closePluginSupervisors(supervisors map[string]pluginSupervisor) error {
	results := make(chan error, len(supervisors))
	for _, supervisor := range supervisors {
		go func(supervisor pluginSupervisor) {
			results <- supervisor.Close(context.Background())
		}(supervisor)
	}
	var result error
	for range supervisors {
		result = errors.Join(result, <-results)
	}
	return result
}
