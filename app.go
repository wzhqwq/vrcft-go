package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

const defaultRootShutdownTimeout = 5 * time.Second

type backendOperations interface {
	Start(context.Context) error
	Close(context.Context) error
	Status() application.Status
	SubscribeStatus(context.Context) <-chan application.Status
	Plugins() []plugins.RuntimeSnapshot
	PluginConfig(string) (pluginapi.Config, bool)
	SetPluginEnabled(context.Context, string, bool) error
	UpdatePluginConfig(context.Context, string, pluginapi.Config) error
	SubscribePlugins(context.Context) <-chan []plugins.RuntimeSnapshot
}

type ownedBackendFactory func(application.Config) (*application.Application, backendOperations, error)

func productionOwnedBackend(config application.Config) (*application.Application, backendOperations, error) {
	backend, err := application.NewApp(config)
	return backend, backend, err
}

type appDependencies struct {
	goos              string
	environment       func() (userconfig.Environment, error)
	resolvePaths      func(userconfig.Environment) (userconfig.Paths, error)
	newStore          func(userconfig.Paths) (settingsBackend, error)
	applicationConfig func(userconfig.Settings, userconfig.Paths) (application.Config, error)
	newBackend        ownedBackendFactory
	now               func() time.Time
	emitter           eventEmitter
	shutdownTimeout   time.Duration
	startupBoundary   func(string)
}

type rootLifecycle uint8

const (
	rootCreated rootLifecycle = iota
	rootStarting
	rootRunning
	rootDiagnostic
	rootClosing
	rootClosed
)

type rootStartupOperation struct {
	token  uint64
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	result rootLifecycle
}

// App owns the one concrete production Application and the narrower
// operations seam used by lifecycle tests. NewApp only creates passive values;
// startup owns all environment, storage, backend, and goroutine work.
type App struct {
	mu        sync.Mutex
	lifecycle rootLifecycle
	deps      appDependencies

	backend    *application.Application
	backendOps backendOperations
	runtime    *RuntimeAPI
	plugins    *PluginsAPI
	settings   *SettingsAPI
	settingsIO *rootSettingsBackend

	processCtx     context.Context
	processCancel  context.CancelFunc
	consumerCancel context.CancelFunc
	consumerWG     sync.WaitGroup
	forwarders     *eventForwarders
	startupToken   uint64
	startupOp      *rootStartupOperation
	shutdownDone   chan struct{}
}

// NewApp constructs the passive production root object.
func NewApp() *App {
	return newAppWithDependencies(productionAppDependencies())
}

func newAppWithDependencies(dependencies appDependencies) *App {
	if dependencies.goos == "" {
		dependencies.goos = runtime.GOOS
	}
	if dependencies.now == nil {
		dependencies.now = time.Now
	}
	if dependencies.shutdownTimeout <= 0 {
		dependencies.shutdownTimeout = defaultRootShutdownTimeout
	}
	settingsIO := &rootSettingsBackend{}
	return &App{
		lifecycle:  rootCreated,
		deps:       dependencies,
		runtime:    newRuntimeAPI(dependencies.goos == "windows", dependencies.now),
		plugins:    newPluginsAPI(dependencies.now),
		settings:   newSettingsAPI(settingsIO, userconfig.Candidate{}, dependencies.now),
		settingsIO: settingsIO,
	}
}

func productionAppDependencies() appDependencies {
	return appDependencies{
		goos: runtime.GOOS,
		environment: func() (userconfig.Environment, error) {
			executable, err := os.Executable()
			if err != nil {
				return userconfig.Environment{}, fmt.Errorf("resolve executable: %w", err)
			}
			roaming, err := os.UserConfigDir()
			if err != nil {
				return userconfig.Environment{}, fmt.Errorf("resolve user config directory: %w", err)
			}
			profile, err := os.UserHomeDir()
			if err != nil {
				return userconfig.Environment{}, fmt.Errorf("resolve user profile: %w", err)
			}
			return userconfig.Environment{GOOS: runtime.GOOS, RoamingDir: roaming, UserProfile: profile, Executable: executable}, nil
		},
		resolvePaths: userconfig.ResolvePaths,
		newStore: func(paths userconfig.Paths) (settingsBackend, error) {
			return userconfig.NewStore(paths)
		},
		applicationConfig: userconfig.ApplicationConfig,
		newBackend:        productionOwnedBackend,
		now:               time.Now,
		emitter:           wailsEmitter{},
		shutdownTimeout:   defaultRootShutdownTimeout,
	}
}

// startup is the Wails OnStartup callback. Wails cannot receive its error, so
// each failure becomes a sanitized diagnostic module snapshot.
func (a *App) startup(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}
	op := a.admitStartup(parent)
	if op == nil {
		return
	}
	defer a.finishStartup(op)

	if !a.startupActive(op) {
		return
	}
	a.runtime.setRootState(runtimePhaseStarting, nil)
	if !a.startupActive(op) {
		return
	}
	forwarders := startEventForwarders(op.ctx, a.deps.emitter, a.runtime, a.plugins, a.settings)
	a.mu.Lock()
	if a.startupOp == op {
		a.forwarders = forwarders
	}
	a.mu.Unlock()
	if !a.startupActive(op) {
		return
	}

	if err := op.ctx.Err(); err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if a.deps.goos != "windows" {
		a.reachStartupBoundary("unsupported")
		a.setPlatformSupported(false)
		a.enterDiagnostic(userconfig.ErrUnsupportedPlatform, nil)
		return
	}
	if a.deps.environment == nil || a.deps.resolvePaths == nil || a.deps.newStore == nil || a.deps.applicationConfig == nil || a.deps.newBackend == nil {
		a.enterDiagnostic(errors.New("root application dependencies are incomplete"), nil)
		return
	}

	if !a.startupActive(op) {
		return
	}
	a.reachStartupBoundary("environment")
	environment, err := a.deps.environment()
	if !a.startupActive(op) {
		return
	}
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	a.reachStartupBoundary("resolve_paths")
	paths, err := a.deps.resolvePaths(environment)
	if !a.startupActive(op) {
		return
	}
	if err != nil {
		if errors.Is(err, userconfig.ErrUnsupportedPlatform) {
			a.setPlatformSupported(false)
		}
		a.enterDiagnostic(err, nil)
		return
	}
	a.reachStartupBoundary("new_store")
	store, err := a.deps.newStore(paths)
	if !a.startupActive(op) {
		return
	}
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	a.reachStartupBoundary("settings_attach")
	err = a.settingsIO.attach(store)
	if !a.startupActive(op) {
		return
	}
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	a.reachStartupBoundary("settings_load")
	loaded, err := a.settings.loadForStartup(op.ctx)
	if !a.startupActive(op) {
		return
	}
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if loaded.Invalid {
		diagnostic := loaded.Diagnostic
		if diagnostic == nil {
			diagnostic = errors.New("settings file is invalid")
		}
		a.enterDiagnostic(diagnostic, nil)
		return
	}
	if loaded.Settings == nil {
		a.enterDiagnostic(errors.New("settings load returned no document"), nil)
		return
	}
	if !a.startupActive(op) {
		return
	}
	a.reachStartupBoundary("application_config")
	config, err := a.deps.applicationConfig(loaded.Settings.Clone(), paths)
	if !a.startupActive(op) {
		return
	}
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}

	// Factory construction is synchronous and bounded by contract, but it does
	// not receive a context. Keep it outside the root lock so shutdown can mark
	// closing and cancel the admitted startup while construction is in flight.
	a.reachStartupBoundary("new_backend")
	backend, operations, err := a.deps.newBackend(config)
	a.mu.Lock()
	if backend != nil || operations != nil {
		a.backend = backend
		a.backendOps = operations
	}
	activeAfterFactory := a.startupActiveLocked(op)
	a.mu.Unlock()
	if err != nil {
		if activeAfterFactory {
			a.enterDiagnostic(err, nil)
		}
		return
	}
	if operations == nil {
		a.enterDiagnostic(errors.New("backend factory returned no operations"), nil)
		return
	}
	if !activeAfterFactory {
		return
	}
	a.reachStartupBoundary("backend_start")
	if err := operations.Start(op.ctx); err != nil {
		if !a.startupActive(op) {
			return
		}
		a.enterDiagnostic(err, operations)
		return
	}
	if !a.startupActive(op) {
		return
	}
	if err := op.ctx.Err(); err != nil {
		a.enterDiagnostic(err, operations)
		return
	}
	a.attachRunningConsumers(op, operations)
}

func (a *App) admitStartup(parent context.Context) *rootStartupOperation {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lifecycle != rootCreated {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	a.startupToken++
	op := &rootStartupOperation{token: a.startupToken, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	a.startupOp = op
	a.processCtx = ctx
	a.processCancel = cancel
	a.lifecycle = rootStarting
	return op
}

func (a *App) finishStartup(op *rootStartupOperation) {
	a.mu.Lock()
	if a.startupOp == op {
		op.result = a.lifecycle
		close(op.done)
	}
	a.mu.Unlock()
}

func (a *App) startupActive(op *rootStartupOperation) bool {
	if op == nil || op.ctx.Err() != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.startupActiveLocked(op)
}

func (a *App) startupActiveLocked(op *rootStartupOperation) bool {
	return op != nil && a.startupOp == op && a.lifecycle == rootStarting && op.ctx.Err() == nil
}

func (a *App) reachStartupBoundary(boundary string) {
	if a.deps.startupBoundary != nil {
		a.deps.startupBoundary(boundary)
	}
}

func (a *App) attachRunningConsumers(op *rootStartupOperation, operations backendOperations) {
	if !a.startupActive(op) {
		return
	}
	consumerCtx, cancel := context.WithCancel(op.ctx)
	a.mu.Lock()
	if !a.startupActiveLocked(op) || a.backendOps != operations {
		a.mu.Unlock()
		cancel()
		return
	}
	a.consumerCancel = cancel
	a.mu.Unlock()

	status := operations.Status()
	if !a.startupActive(op) {
		return
	}
	a.runtime.setApplicationStatus(status)
	if !a.startupActive(op) {
		return
	}
	a.plugins.attach(consumerCtx, operations)
	if !a.startupActive(op) {
		return
	}
	pluginUpdates := operations.SubscribePlugins(consumerCtx)
	if !a.startupActive(op) {
		return
	}
	a.plugins.consumeSnapshots(consumerCtx, pluginUpdates)
	if !a.startupActive(op) {
		return
	}
	statusUpdates := operations.SubscribeStatus(consumerCtx)
	if !a.startupActive(op) {
		return
	}
	a.consumerWG.Add(1)
	go func() {
		defer a.consumerWG.Done()
		a.runtime.consumeStatus(consumerCtx, statusUpdates)
	}()
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.startupActiveLocked(op) {
		return
	}
	a.lifecycle = rootRunning
	a.runtime.setRootState(runtimePhaseRunning, nil)
}

func (a *App) enterDiagnostic(err error, operations backendOperations) {
	var status *application.Status
	if operations != nil {
		value := operations.Status()
		status = &value
	}
	problem := sanitizeProblem(err, a.runtime.GetStatus().Revision)

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lifecycle != rootStarting {
		return
	}
	if status != nil {
		a.runtime.setApplicationStatus(*status)
	}
	a.lifecycle = rootDiagnostic
	a.runtime.setRootState(runtimePhaseDiagnostic, &problem)
}

func (a *App) setPlatformSupported(supported bool) {
	a.runtime.mu.Lock()
	defer a.runtime.mu.Unlock()
	envelope := a.runtime.store.snapshot()
	next := cloneRuntimeSnapshot(envelope.Value)
	next.platformSupported = supported
	a.runtime.updateLocked(envelope, next, envelope.Problem)
}

// shutdown is the Wails OnShutdown callback. One caller performs shutdown;
// repeated and concurrent callers wait for and observe the same final result.
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	if a.shutdownDone != nil {
		done := a.shutdownDone
		a.mu.Unlock()
		<-done
		return
	}
	a.shutdownDone = make(chan struct{})
	done := a.shutdownDone
	a.lifecycle = rootClosing
	startupOp := a.startupOp
	if startupOp != nil {
		startupOp.cancel()
	} else if a.processCancel != nil {
		a.processCancel()
	}
	a.mu.Unlock()

	if startupOp != nil {
		<-startupOp.done
	}
	a.mu.Lock()
	consumerCancel := a.consumerCancel
	forwarders := a.forwarders
	operations := a.backendOps
	a.mu.Unlock()

	a.plugins.close()
	if consumerCancel != nil {
		consumerCancel()
	}
	a.consumerWG.Wait()
	forwarders.stop()
	a.settings.close()
	a.runtime.setRootState(runtimePhaseClosing, nil)

	var closeErr error
	if operations != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), a.deps.shutdownTimeout)
		closeErr = operations.Close(closeCtx)
		cancel()
		a.runtime.setApplicationStatus(operations.Status())
	}
	var problem *Problem
	if closeErr != nil {
		mapped := sanitizeProblem(closeErr, a.runtime.GetStatus().Revision)
		problem = &mapped
	}
	a.runtime.setRootState(runtimePhaseClosed, problem)

	a.mu.Lock()
	a.lifecycle = rootClosed
	close(done)
	a.mu.Unlock()
}

// rootSettingsBackend lets main bind a passive SettingsAPI before startup has
// resolved user-specific paths and constructed the real Store.
type rootSettingsBackend struct {
	mu      sync.RWMutex
	backend settingsBackend
}

func (backend *rootSettingsBackend) attach(store settingsBackend) error {
	if store == nil {
		return errors.New("settings store is unavailable")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.backend != nil {
		return errors.New("settings store is already attached")
	}
	backend.backend = store
	return nil
}

func (backend *rootSettingsBackend) current() (settingsBackend, error) {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	if backend.backend == nil {
		return nil, application.ErrInvalidLifecycle
	}
	return backend.backend, nil
}

func (backend *rootSettingsBackend) LoadOrCreate(ctx context.Context) (userconfig.Loaded, error) {
	store, err := backend.current()
	if err != nil {
		return userconfig.Loaded{}, err
	}
	return store.LoadOrCreate(ctx)
}

func (backend *rootSettingsBackend) Validate(candidate userconfig.Candidate) (userconfig.Candidate, error) {
	store, err := backend.current()
	if err != nil {
		return userconfig.Candidate{}, err
	}
	return store.Validate(candidate)
}

func (backend *rootSettingsBackend) Save(ctx context.Context, loaded userconfig.Loaded, candidate userconfig.Candidate) (userconfig.SaveResult, error) {
	store, err := backend.current()
	if err != nil {
		return userconfig.SaveResult{}, err
	}
	return store.Save(ctx, loaded, candidate)
}
