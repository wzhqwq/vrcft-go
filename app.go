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

	a.mu.Lock()
	if a.lifecycle != rootCreated {
		a.mu.Unlock()
		return
	}
	a.lifecycle = rootStarting
	a.processCtx, a.processCancel = context.WithCancel(parent)
	processCtx := a.processCtx
	a.runtime.setRootState(runtimePhaseStarting, nil)
	a.forwarders = startEventForwarders(processCtx, a.deps.emitter, a.runtime, a.plugins, a.settings)
	a.mu.Unlock()

	if err := processCtx.Err(); err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if a.deps.goos != "windows" {
		a.setPlatformSupported(false)
		a.enterDiagnostic(userconfig.ErrUnsupportedPlatform, nil)
		return
	}
	if a.deps.environment == nil || a.deps.resolvePaths == nil || a.deps.newStore == nil || a.deps.applicationConfig == nil || a.deps.newBackend == nil {
		a.enterDiagnostic(errors.New("root application dependencies are incomplete"), nil)
		return
	}

	environment, err := a.deps.environment()
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if !a.startupActive(processCtx) {
		return
	}
	paths, err := a.deps.resolvePaths(environment)
	if err != nil {
		if errors.Is(err, userconfig.ErrUnsupportedPlatform) {
			a.setPlatformSupported(false)
		}
		a.enterDiagnostic(err, nil)
		return
	}
	if !a.startupActive(processCtx) {
		return
	}
	store, err := a.deps.newStore(paths)
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if !a.startupActive(processCtx) {
		return
	}
	if err := a.settingsIO.attach(store); err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if !a.startupActive(processCtx) {
		return
	}
	loaded, err := a.settings.loadForStartup(processCtx)
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if !a.startupActive(processCtx) {
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
	config, err := a.deps.applicationConfig(loaded.Settings.Clone(), paths)
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if !a.startupActive(processCtx) {
		return
	}

	// Construction and ownership publication share the root lock. Shutdown can
	// therefore never observe a construction window without also observing the
	// operations value that must be closed.
	a.mu.Lock()
	if a.lifecycle != rootStarting || processCtx.Err() != nil {
		a.mu.Unlock()
		return
	}
	backend, operations, err := a.deps.newBackend(config)
	if err == nil {
		a.backend = backend
		a.backendOps = operations
	}
	a.mu.Unlock()
	if err != nil {
		a.enterDiagnostic(err, nil)
		return
	}
	if operations == nil {
		a.enterDiagnostic(errors.New("backend factory returned no operations"), nil)
		return
	}
	if !a.startupActive(processCtx) {
		return
	}
	if err := operations.Start(processCtx); err != nil {
		a.enterDiagnostic(err, operations)
		return
	}
	if err := processCtx.Err(); err != nil {
		a.enterDiagnostic(err, operations)
		return
	}
	a.attachRunningConsumers(processCtx, operations)
}

func (a *App) startupActive(processCtx context.Context) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lifecycle == rootStarting && processCtx.Err() == nil
}

func (a *App) attachRunningConsumers(processCtx context.Context, operations backendOperations) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lifecycle != rootStarting || processCtx.Err() != nil || a.backendOps != operations {
		return
	}
	consumerCtx, cancel := context.WithCancel(processCtx)
	a.consumerCancel = cancel
	a.runtime.setApplicationStatus(operations.Status())
	a.plugins.attach(consumerCtx, operations)
	a.plugins.consumeSnapshots(consumerCtx, operations.SubscribePlugins(consumerCtx))
	statusUpdates := operations.SubscribeStatus(consumerCtx)
	a.consumerWG.Add(1)
	go func() {
		defer a.consumerWG.Done()
		a.runtime.consumeStatus(consumerCtx, statusUpdates)
	}()
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
	processCancel := a.processCancel
	consumerCancel := a.consumerCancel
	forwarders := a.forwarders
	operations := a.backendOps
	a.mu.Unlock()

	if processCancel != nil {
		processCancel()
	}
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
