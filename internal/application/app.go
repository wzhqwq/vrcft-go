package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

var ErrInvalidLifecycle = errors.New("application: invalid lifecycle")

type applicationTracking interface {
	Submit(string, uint64, trackingmodel.TrackingFrame) error
	SetGeneration(uint64) error
	RemoveSource(string)
	SubscribeMerged(context.Context) <-chan tracking.MergedFrame
}

type applicationPluginManager interface {
	Start(context.Context) error
	Close(context.Context) error
	List() []plugins.RuntimeSnapshot
	SetActive(context.Context, string, bool) error
	UpdateSubscription(context.Context, string, pluginapi.Subscription) error
	Subscribe(context.Context) <-chan plugins.Event
}

type applicationOSC interface {
	Start(context.Context) error
	Close(context.Context) error
	Events() <-chan osc.ControllerEvent
	AvatarChanges(context.Context) <-chan osc.AvatarChange
	runtimePublisher
}

type applicationCoordinator interface {
	Start(context.Context, coordinatorInputs) error
	Ready() <-chan struct{}
	Reconcile([]plugins.RuntimeSnapshot)
	Join(context.Context) error
}

type applicationTicker interface {
	C() <-chan time.Time
	Stop()
}

type applicationDependencies struct {
	newTracking      func() (applicationTracking, error)
	newFrameSink     func(applicationTracking) (plugins.FrameSink, error)
	newPluginManager func(normalizedConfig, plugins.FrameSink) (applicationPluginManager, error)
	newPipeline      func(processing.Config) (framePipeline, error)
	newPlanner       func(avatar.PlannerConfig) (activationPlanner, error)
	newOSC           func(osc.ControllerConfig) (applicationOSC, error)
	newInstaller     func(applicationPluginManager, applicationTracking, applicationOSC, time.Duration) (*planInstaller, error)
	newCoordinator   func(activationPlanner, *planInstaller, framePipeline, applicationTracking, applicationOSC, *statusStore, func() time.Time) (applicationCoordinator, error)
	newTicker        func(time.Duration) applicationTicker
	now              func() time.Time
}

type applicationLifecycle uint8

const (
	applicationCreated applicationLifecycle = iota
	applicationStarting
	applicationRunning
	applicationFailed
	applicationClosing
	applicationClosed
)

type Application struct {
	plugins     applicationPluginManager
	tracking    applicationTracking
	osc         applicationOSC
	coordinator applicationCoordinator
	status      *statusStore

	frameInterval time.Duration
	newTicker     func(time.Duration) applicationTicker

	mu                 sync.Mutex
	lifecycle          applicationLifecycle
	runCancel          context.CancelFunc
	ticker             applicationTicker
	startDone          chan struct{}
	managerClosed      bool
	coordinatorStarted bool
	oscClosed          bool
	closeDone          chan struct{}
	closeErr           error
}

func NewApp(config Config) (*Application, error) {
	return newApplication(config, productionApplicationDependencies())
}

func newApplication(config Config, dependencies applicationDependencies) (*Application, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}

	trackingService, err := dependencies.newTracking()
	if err != nil {
		return nil, fmt.Errorf("construct tracking service: %w", err)
	}
	frameSink, err := dependencies.newFrameSink(trackingService)
	if err != nil {
		return nil, fmt.Errorf("construct plugin frame sink: %w", err)
	}
	manager, err := dependencies.newPluginManager(normalized, frameSink)
	if err != nil {
		return nil, fmt.Errorf("construct plugin manager: %w", err)
	}
	pipeline, err := dependencies.newPipeline(normalized.processing)
	if err != nil {
		return nil, fmt.Errorf("construct processing pipeline: %w", err)
	}
	planner, err := dependencies.newPlanner(normalized.avatar)
	if err != nil {
		return nil, fmt.Errorf("construct avatar planner: %w", err)
	}
	normalized.osc.CatalogMode = osc.CatalogExternal
	oscService, err := dependencies.newOSC(normalized.osc)
	if err != nil {
		return nil, fmt.Errorf("construct OSC service: %w", err)
	}
	status := newStatusStore(dependencies.now)
	installer, err := dependencies.newInstaller(manager, trackingService, oscService, normalized.pluginControlTimeout)
	if err != nil {
		return nil, fmt.Errorf("construct plan installer: %w", err)
	}
	coordinator, err := dependencies.newCoordinator(
		planner,
		installer,
		pipeline,
		trackingService,
		oscService,
		status,
		dependencies.now,
	)
	if err != nil {
		return nil, fmt.Errorf("construct coordinator: %w", err)
	}

	startDone := make(chan struct{})
	close(startDone)
	return &Application{
		plugins:       manager,
		tracking:      trackingService,
		osc:           oscService,
		coordinator:   coordinator,
		status:        status,
		frameInterval: normalized.frameInterval,
		newTicker:     dependencies.newTicker,
		lifecycle:     applicationCreated,
		startDone:     startDone,
		closeDone:     make(chan struct{}),
	}, nil
}

func productionApplicationDependencies() applicationDependencies {
	return applicationDependencies{
		newTracking: func() (applicationTracking, error) {
			return tracking.NewService(), nil
		},
		newFrameSink: func(target applicationTracking) (plugins.FrameSink, error) {
			return tracking.NewPluginFrameSink(target), nil
		},
		newPluginManager: func(config normalizedConfig, sink plugins.FrameSink) (applicationPluginManager, error) {
			catalog, err := plugins.NewDirectoryCatalog(config.pluginCatalog)
			if err != nil {
				return nil, err
			}
			store, err := plugins.NewJSONStore(config.pluginStorePath, config.pluginStoreMaxBytes)
			if err != nil {
				return nil, err
			}
			return plugins.NewManager(catalog, store, plugins.NewProcessLauncher(), sink, config.pluginOptions)
		},
		newPipeline: func(config processing.Config) (framePipeline, error) {
			return processing.NewPipeline(config)
		},
		newPlanner: func(config avatar.PlannerConfig) (activationPlanner, error) {
			planner, err := avatar.NewPlanner(config)
			if err != nil {
				return nil, err
			}
			return newActivationPlanner(planner), nil
		},
		newOSC: func(config osc.ControllerConfig) (applicationOSC, error) {
			config.CatalogMode = osc.CatalogExternal
			return osc.NewOSCService(config)
		},
		newInstaller: func(manager applicationPluginManager, trackingService applicationTracking, runtime applicationOSC, timeout time.Duration) (*planInstaller, error) {
			return &planInstaller{
				plugins:              manager,
				tracking:             trackingService,
				osc:                  runtime,
				pluginControlTimeout: timeout,
			}, nil
		},
		newCoordinator: func(
			planner activationPlanner,
			installer *planInstaller,
			pipeline framePipeline,
			trackingService applicationTracking,
			runtime applicationOSC,
			status *statusStore,
			wall func() time.Time,
		) (applicationCoordinator, error) {
			return &coordinatorRunner{coordinator: &coordinator{
				planner:   planner,
				installer: installer,
				pipeline:  pipeline,
				tracking:  trackingService,
				runtime:   runtime,
				status:    status,
				clock:     newMonotonicClock(wall),
			}}, nil
		},
		newTicker: func(interval time.Duration) applicationTicker {
			return realApplicationTicker{Ticker: time.NewTicker(interval)}
		},
		now: time.Now,
	}
}

func (a *Application) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("start application: context is required")
	}

	a.mu.Lock()
	if a.lifecycle != applicationCreated {
		lifecycle := a.lifecycle
		a.mu.Unlock()
		return fmt.Errorf("%w: start from state %d", ErrInvalidLifecycle, lifecycle)
	}
	a.lifecycle = applicationStarting
	runCtx, cancel := context.WithCancel(ctx)
	startDone := make(chan struct{})
	a.runCancel = cancel
	a.startDone = startDone
	ticker := a.newTicker(a.frameInterval)
	a.ticker = ticker
	a.mu.Unlock()
	defer close(startDone)

	a.status.update(func(status *Status) {
		status.Lifecycle = LifecycleStarting
		status.RuntimeError = ""
	})

	inputs := coordinatorInputs{
		avatarChanges: a.osc.AvatarChanges(runCtx),
		oscEvents:     a.osc.Events(),
		pluginEvents:  a.plugins.Subscribe(runCtx),
		merged:        a.tracking.SubscribeMerged(runCtx),
		ticks:         ticker.C(),
	}

	if err := a.plugins.Start(runCtx); err != nil {
		cancel()
		ticker.Stop()
		return a.failStart(fmt.Errorf("start plugin manager: %w", err))
	}
	if err := a.coordinator.Start(runCtx, inputs); err != nil {
		startupErr := fmt.Errorf("start coordinator: %w", err)
		return a.failStart(a.rollback(ctx, startupErr, false))
	}
	a.setCoordinatorStarted(true)

	select {
	case <-a.coordinator.Ready():
	case <-runCtx.Done():
		startupErr := fmt.Errorf("wait for coordinator readiness: %w", runCtx.Err())
		return a.failStart(a.rollback(ctx, startupErr, true))
	}
	a.coordinator.Reconcile(a.plugins.List())

	if err := runCtx.Err(); err != nil {
		startupErr := fmt.Errorf("start application: %w", err)
		return a.failStart(a.rollback(ctx, startupErr, true))
	}
	if err := a.osc.Start(runCtx); err != nil {
		startupErr := fmt.Errorf("start OSC service: %w", err)
		return a.failStart(a.rollback(ctx, startupErr, true))
	}

	a.mu.Lock()
	if a.lifecycle != applicationStarting {
		a.mu.Unlock()
		startupErr := fmt.Errorf("%w: application closed during start", ErrInvalidLifecycle)
		return a.failStart(a.rollback(ctx, startupErr, true))
	}
	a.lifecycle = applicationRunning
	a.mu.Unlock()
	a.status.update(func(status *Status) {
		if status.Lifecycle != LifecycleClosing && status.Lifecycle != LifecycleClosed {
			status.Lifecycle = LifecycleRunning
			status.RuntimeError = ""
		}
	})
	return nil
}

func (a *Application) rollback(ctx context.Context, startupErr error, coordinatorStarted bool) error {
	a.mu.Lock()
	cancel := a.runCancel
	ticker := a.ticker
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if ticker != nil {
		ticker.Stop()
	}

	result := startupErr
	if coordinatorStarted {
		if err := a.coordinator.Join(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("join coordinator during startup rollback: %w", err))
		}
		a.setCoordinatorStarted(false)
	}
	if err := a.plugins.Close(ctx); err != nil {
		result = errors.Join(result, fmt.Errorf("close plugin manager during startup rollback: %w", err))
	}
	a.mu.Lock()
	a.managerClosed = true
	a.mu.Unlock()
	return result
}

func (a *Application) failStart(err error) error {
	a.mu.Lock()
	if a.lifecycle == applicationStarting {
		a.lifecycle = applicationFailed
	}
	a.mu.Unlock()
	a.status.update(func(status *Status) {
		if status.Lifecycle != LifecycleClosing && status.Lifecycle != LifecycleClosed {
			status.Lifecycle = LifecycleDegraded
			status.RuntimeError = coordinatorErrorMessage(err)
		}
	})
	return err
}

func (a *Application) Close(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("close application: context is required")
	}

	a.mu.Lock()
	switch a.lifecycle {
	case applicationClosed:
		err := a.closeErr
		a.mu.Unlock()
		return err
	case applicationClosing:
		done := a.closeDone
		a.mu.Unlock()
		return waitForApplicationClose(ctx, done, a.cachedCloseError)
	}
	wasStarting := a.lifecycle == applicationStarting
	a.lifecycle = applicationClosing
	startDone := a.startDone
	cancel := a.runCancel
	done := a.closeDone
	a.status.update(func(status *Status) {
		status.Lifecycle = LifecycleClosing
	})
	a.mu.Unlock()

	go a.finishClose(ctx, startDone, cancel, wasStarting)
	return waitForApplicationClose(ctx, done, a.cachedCloseError)
}

func (a *Application) finishClose(ctx context.Context, startDone <-chan struct{}, cancel context.CancelFunc, wasStarting bool) {
	result := error(nil)
	if wasStarting && cancel != nil {
		cancel()
	}
	if startDone != nil {
		select {
		case <-startDone:
		case <-ctx.Done():
			result = errors.Join(result, fmt.Errorf("wait for application start: %w", ctx.Err()))
		}
	}

	a.mu.Lock()
	oscClosed := a.oscClosed
	coordinatorStarted := a.coordinatorStarted
	managerClosed := a.managerClosed
	ticker := a.ticker
	a.mu.Unlock()

	if !oscClosed {
		if err := a.osc.Close(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("close OSC service: %w", err))
		}
		a.mu.Lock()
		a.oscClosed = true
		a.mu.Unlock()
	}
	if cancel != nil {
		cancel()
	}
	if ticker != nil {
		ticker.Stop()
	}
	if coordinatorStarted {
		if err := a.coordinator.Join(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("join coordinator: %w", err))
		}
		a.setCoordinatorStarted(false)
	}
	if !managerClosed {
		if err := a.plugins.Close(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("close plugin manager: %w", err))
		}
	}

	a.status.update(func(status *Status) {
		status.Lifecycle = LifecycleClosed
		if result != nil {
			status.RuntimeError = coordinatorErrorMessage(result)
		}
	})
	a.mu.Lock()
	a.managerClosed = true
	a.closeErr = result
	a.lifecycle = applicationClosed
	close(a.closeDone)
	a.mu.Unlock()
}

func waitForApplicationClose(ctx context.Context, done <-chan struct{}, result func() error) error {
	select {
	case <-done:
		return result()
	case <-ctx.Done():
		select {
		case <-done:
			return result()
		default:
			return ctx.Err()
		}
	}
}

func (a *Application) cachedCloseError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closeErr
}

func (a *Application) Status() Status {
	return a.status.snapshot()
}

func (a *Application) SubscribeStatus(ctx context.Context) <-chan Status {
	return a.status.subscribe(ctx)
}

func (a *Application) setCoordinatorStarted(started bool) {
	a.mu.Lock()
	a.coordinatorStarted = started
	a.mu.Unlock()
}

type coordinatorRunner struct {
	coordinator *coordinator

	mu      sync.Mutex
	started bool
	ready   chan struct{}
	done    chan struct{}
}

func (runner *coordinatorRunner) Start(ctx context.Context, inputs coordinatorInputs) error {
	if ctx == nil {
		return errors.New("application: coordinator context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runner.mu.Lock()
	if runner.started {
		runner.mu.Unlock()
		return ErrInvalidLifecycle
	}
	runner.started = true
	runner.ready = make(chan struct{})
	runner.done = make(chan struct{})
	ready := runner.ready
	done := runner.done
	runner.mu.Unlock()

	go func() {
		defer close(done)
		defer runner.coordinator.runtime.ClearRuntime()
		runner.coordinator.run(ctx, inputs, ready)
	}()
	return nil
}

func (runner *coordinatorRunner) Ready() <-chan struct{} {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.ready
}

func (runner *coordinatorRunner) Reconcile(snapshots []plugins.RuntimeSnapshot) {
	for index := range snapshots {
		snapshot := snapshots[index]
		runner.coordinator.observePlugin(plugins.Event{
			Type:     plugins.EventPluginStateChanged,
			PluginID: snapshot.ID,
			Snapshot: &snapshot,
		})
	}
}

func (runner *coordinatorRunner) Join(ctx context.Context) error {
	if ctx == nil {
		return errors.New("application: coordinator join context is required")
	}
	runner.mu.Lock()
	done := runner.done
	runner.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type realApplicationTicker struct {
	*time.Ticker
}

func (ticker realApplicationTicker) C() <-chan time.Time { return ticker.Ticker.C }

var _ applicationTracking = tracking.Service(nil)
var _ applicationPluginManager = plugins.Manager(nil)
var _ applicationOSC = osc.OSCService(nil)
var _ framePipeline = (*processing.Pipeline)(nil)
var _ plugins.FrameSink = tracking.PluginFrameSink{}
var _ applicationCoordinator = (*coordinatorRunner)(nil)
var _ applicationTicker = realApplicationTicker{}
