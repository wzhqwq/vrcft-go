package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestApplicationConstructsComponentsInDependencyOrder(t *testing.T) {
	harness := newApplicationHarness(t, "", nil)

	app, err := newApplication(validApplicationConfig(t), harness.dependencies())
	if err != nil {
		t.Fatalf("newApplication() error = %v", err)
	}
	if app == nil {
		t.Fatal("newApplication() = nil")
	}

	want := []string{
		"construct.tracking",
		"construct.frame-sink",
		"construct.plugins",
		"construct.processing",
		"construct.avatar",
		"construct.osc.external",
		"construct.installer",
		"construct.coordinator",
	}
	if got := harness.trace.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("construction trace = %v, want %v", got, want)
	}
	if got := harness.lifecycleCalls(); got != 0 {
		t.Fatalf("construction lifecycle calls = %d, want zero", got)
	}
}

func TestApplicationConstructionFailureStopsWithoutLifecycleSideEffects(t *testing.T) {
	tests := []struct {
		factory string
		owner   string
	}{
		{factory: "tracking", owner: "tracking service"},
		{factory: "frame-sink", owner: "plugin frame sink"},
		{factory: "plugins", owner: "plugin manager"},
		{factory: "processing", owner: "processing pipeline"},
		{factory: "avatar", owner: "avatar planner"},
		{factory: "osc", owner: "OSC service"},
		{factory: "installer", owner: "plan installer"},
		{factory: "coordinator", owner: "coordinator"},
	}

	for _, test := range tests {
		t.Run(test.factory, func(t *testing.T) {
			wantErr := fmt.Errorf("%s unavailable", test.factory)
			harness := newApplicationHarness(t, test.factory, wantErr)

			app, err := newApplication(validApplicationConfig(t), harness.dependencies())
			if app != nil {
				t.Fatalf("newApplication() app = %#v, want nil", app)
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("newApplication() error = %v, want errors.Is(_, %v)", err, wantErr)
			}
			if !strings.Contains(err.Error(), test.owner) {
				t.Fatalf("newApplication() error = %q, want owner %q", err, test.owner)
			}
			if got := harness.lifecycleCalls(); got != 0 {
				t.Fatalf("construction failure lifecycle calls = %d, want zero", got)
			}
		})
	}
}

func TestApplicationProductionConstructionOwnsCompleteBackend(t *testing.T) {
	app, err := NewApp(validApplicationConfig(t))
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	if app == nil {
		t.Fatal("NewApp() = nil")
	}

	if err := app.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestApplicationStartSubscribesBeforeProducersAndStartsOSCLast(t *testing.T) {
	harness := newApplicationHarness(t, "", nil)
	app := harness.newApp(t)

	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = app.Close(context.Background()) })

	want := []string{
		"subscribe.avatar",
		"subscribe.osc",
		"subscribe.plugins",
		"subscribe.merged",
		"plugins.start",
		"coordinator.start",
		"coordinator.ready",
		"osc.start",
	}
	if got := harness.trace.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("start trace = %v, want %v", got, want)
	}
	if status := app.Status(); status.Lifecycle != LifecycleRunning {
		t.Fatalf("Status().Lifecycle = %q, want %q", status.Lifecycle, LifecycleRunning)
	}
}

func TestApplicationOSCStartFailureRollsBackCoordinatorThenPluginsAndJoinsErrors(t *testing.T) {
	oscStartErr := errors.New("OSC start failed")
	joinErr := errors.New("coordinator join failed")
	pluginCloseErr := errors.New("plugin close failed")
	harness := newApplicationHarness(t, "", nil)
	harness.osc.startErr = oscStartErr
	harness.coordinator.joinErr = joinErr
	harness.manager.closeErr = pluginCloseErr
	app := harness.newApp(t)

	err := app.Start(context.Background())
	for _, want := range []error{oscStartErr, joinErr, pluginCloseErr} {
		if !errors.Is(err, want) {
			t.Fatalf("Start() error = %v, want errors.Is(_, %v)", err, want)
		}
	}
	wantTail := []string{
		"osc.start",
		"coordinator.cancel",
		"coordinator.join",
		"plugins.close",
	}
	trace := harness.trace.snapshot()
	if len(trace) < len(wantTail) || !reflect.DeepEqual(trace[len(trace)-len(wantTail):], wantTail) {
		t.Fatalf("rollback trace = %v, want tail %v", trace, wantTail)
	}
}

func TestApplicationManagerStartFailureStartsNoOwnedComponent(t *testing.T) {
	wantErr := errors.New("manager start failed")
	harness := newApplicationHarness(t, "", nil)
	harness.manager.startErr = wantErr
	app := harness.newApp(t)

	err := app.Start(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Start() error = %v, want errors.Is(_, %v)", err, wantErr)
	}
	want := []string{
		"subscribe.avatar",
		"subscribe.osc",
		"subscribe.plugins",
		"subscribe.merged",
		"plugins.start",
	}
	if got := harness.trace.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("start failure trace = %v, want %v", got, want)
	}
}

func TestApplicationCoordinatorStartFailureClosesStartedManagerOnly(t *testing.T) {
	startErr := errors.New("coordinator start failed")
	pluginCloseErr := errors.New("plugin close failed")
	harness := newApplicationHarness(t, "", nil)
	harness.coordinator.startErr = startErr
	harness.manager.closeErr = pluginCloseErr
	app := harness.newApp(t)

	err := app.Start(context.Background())
	for _, want := range []error{startErr, pluginCloseErr} {
		if !errors.Is(err, want) {
			t.Fatalf("Start() error = %v, want errors.Is(_, %v)", err, want)
		}
	}
	wantTail := []string{"plugins.start", "coordinator.start", "plugins.close"}
	trace := harness.trace.snapshot()
	if len(trace) < len(wantTail) || !reflect.DeepEqual(trace[len(trace)-len(wantTail):], wantTail) {
		t.Fatalf("rollback trace = %v, want tail %v", trace, wantTail)
	}
}

func TestApplicationCloseIsReverseOrderedStableAndRejectsRestart(t *testing.T) {
	oscCloseErr := errors.New("OSC close failed")
	joinErr := errors.New("coordinator join failed")
	pluginCloseErr := errors.New("plugin close failed")
	harness := newApplicationHarness(t, "", nil)
	harness.osc.closeErr = oscCloseErr
	harness.coordinator.joinErr = joinErr
	harness.manager.closeErr = pluginCloseErr
	app := harness.newApp(t)
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	harness.trace.reset()

	first := app.Close(context.Background())
	for _, want := range []error{oscCloseErr, joinErr, pluginCloseErr} {
		if !errors.Is(first, want) {
			t.Fatalf("Close() error = %v, want errors.Is(_, %v)", first, want)
		}
	}
	want := []string{"osc.close", "coordinator.cancel", "coordinator.join", "plugins.close"}
	if got := harness.trace.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("close trace = %v, want %v", got, want)
	}

	second := app.Close(context.Background())
	if second != first {
		t.Fatalf("second Close() error = %v, want cached %v", second, first)
	}
	if got := harness.trace.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("repeated close trace = %v, want unchanged %v", got, want)
	}
	if err := app.Start(context.Background()); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("Start() after Close error = %v, want errors.Is(_, ErrInvalidLifecycle)", err)
	}
	if status := app.Status(); status.Lifecycle != LifecycleClosed {
		t.Fatalf("Status().Lifecycle = %q, want %q", status.Lifecycle, LifecycleClosed)
	}
}

func TestApplicationCloseBeforeStartClosesConstructedBoundaries(t *testing.T) {
	harness := newApplicationHarness(t, "", nil)
	app := harness.newApp(t)

	if err := app.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	want := []string{"osc.close", "plugins.close"}
	if got := harness.trace.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("close-before-start trace = %v, want %v", got, want)
	}
	if err := app.Start(context.Background()); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("Start() after Close error = %v, want errors.Is(_, ErrInvalidLifecycle)", err)
	}
}

func TestApplicationStatusAccessorsExposeOwnedLifecycleSnapshots(t *testing.T) {
	harness := newApplicationHarness(t, "", nil)
	app := harness.newApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	updates := app.SubscribeStatus(ctx)

	created := receiveApplicationStatus(t, updates)
	if created.Lifecycle != LifecycleCreated {
		t.Fatalf("initial lifecycle = %q, want %q", created.Lifecycle, LifecycleCreated)
	}
	created.PluginFailures = append(created.PluginFailures, PluginControlFailure{PluginID: "caller-owned"})
	if got := app.Status().PluginFailures; len(got) != 0 {
		t.Fatalf("Status().PluginFailures = %v, want owned empty slice", got)
	}

	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	running := awaitApplicationLifecycle(t, updates, LifecycleRunning)
	if running.Revision <= created.Revision {
		t.Fatalf("running revision = %d, want greater than created revision %d", running.Revision, created.Revision)
	}
	if err := app.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	closed := awaitApplicationLifecycle(t, updates, LifecycleClosed)
	if closed.Revision <= running.Revision {
		t.Fatalf("closed revision = %d, want greater than running revision %d", closed.Revision, running.Revision)
	}

	cancel()
	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("SubscribeStatus() delivered a value after cancellation, want closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("SubscribeStatus() did not close after cancellation")
	}
}

func receiveApplicationStatus(t *testing.T, updates <-chan Status) Status {
	t.Helper()
	select {
	case status, ok := <-updates:
		if !ok {
			t.Fatal("status subscription closed unexpectedly")
		}
		return status
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for application status")
		return Status{}
	}
}

func awaitApplicationLifecycle(t *testing.T, updates <-chan Status, lifecycle LifecycleState) Status {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case status, ok := <-updates:
			if !ok {
				t.Fatalf("status subscription closed before lifecycle %q", lifecycle)
			}
			if status.Lifecycle == lifecycle {
				return status
			}
		case <-deadline:
			t.Fatalf("timed out waiting for lifecycle %q", lifecycle)
		}
	}
}

type applicationHarness struct {
	trace       *applicationTrace
	failFactory string
	factoryErr  error
	tracking    *fakeApplicationTracking
	manager     *fakeApplicationManager
	osc         *fakeApplicationOSC
	coordinator *fakeApplicationCoordinator
	pipeline    *fakeApplicationPipeline
	planner     *fakeApplicationPlanner
}

func newApplicationHarness(t *testing.T, failFactory string, factoryErr error) *applicationHarness {
	t.Helper()
	trace := &applicationTrace{}
	return &applicationHarness{
		trace:       trace,
		failFactory: failFactory,
		factoryErr:  factoryErr,
		tracking:    &fakeApplicationTracking{trace: trace, merged: make(chan tracking.MergedFrame)},
		manager:     &fakeApplicationManager{trace: trace, events: make(chan plugins.Event)},
		osc: &fakeApplicationOSC{
			trace:         trace,
			avatarChanges: make(chan osc.AvatarChange),
			events:        make(chan osc.ControllerEvent),
		},
		coordinator: &fakeApplicationCoordinator{trace: trace},
		pipeline:    &fakeApplicationPipeline{},
		planner:     &fakeApplicationPlanner{},
	}
}

func (h *applicationHarness) newApp(t *testing.T) *Application {
	t.Helper()
	app, err := newApplication(validApplicationConfig(t), h.dependencies())
	if err != nil {
		t.Fatalf("newApplication() error = %v", err)
	}
	h.trace.reset()
	return app
}

func (h *applicationHarness) dependencies() applicationDependencies {
	failed := func(name string) error {
		if h.failFactory == name {
			return h.factoryErr
		}
		return nil
	}
	return applicationDependencies{
		newTracking: func() (applicationTracking, error) {
			h.trace.add("construct.tracking")
			return h.tracking, failed("tracking")
		},
		newFrameSink: func(applicationTracking) (plugins.FrameSink, error) {
			h.trace.add("construct.frame-sink")
			if err := failed("frame-sink"); err != nil {
				return nil, err
			}
			return fakeApplicationFrameSink{}, nil
		},
		newPluginManager: func(normalizedConfig, plugins.FrameSink) (applicationPluginManager, error) {
			h.trace.add("construct.plugins")
			return h.manager, failed("plugins")
		},
		newPipeline: func(processing.Config) (framePipeline, error) {
			h.trace.add("construct.processing")
			return h.pipeline, failed("processing")
		},
		newPlanner: func(avatar.PlannerConfig) (activationPlanner, error) {
			h.trace.add("construct.avatar")
			return h.planner, failed("avatar")
		},
		newOSC: func(config osc.ControllerConfig) (applicationOSC, error) {
			if config.CatalogMode == osc.CatalogExternal {
				h.trace.add("construct.osc.external")
			} else {
				h.trace.add("construct.osc.wrong-mode")
			}
			return h.osc, failed("osc")
		},
		newInstaller: func(applicationPluginManager, applicationTracking, applicationOSC, time.Duration) (*planInstaller, error) {
			h.trace.add("construct.installer")
			if err := failed("installer"); err != nil {
				return nil, err
			}
			return &planInstaller{}, nil
		},
		newCoordinator: func(activationPlanner, *planInstaller, framePipeline, applicationTracking, applicationOSC, *statusStore, func() time.Time) (applicationCoordinator, error) {
			h.trace.add("construct.coordinator")
			return h.coordinator, failed("coordinator")
		},
		newTicker: func(time.Duration) applicationTicker {
			return &fakeApplicationTicker{ticks: make(chan time.Time)}
		},
		now: func() time.Time { return time.Unix(100, 0) },
	}
}

func (h *applicationHarness) lifecycleCalls() int {
	count := 0
	for _, operation := range h.trace.snapshot() {
		if strings.Contains(operation, ".start") || strings.Contains(operation, ".close") || strings.Contains(operation, ".cancel") || strings.Contains(operation, ".join") {
			count++
		}
	}
	return count
}

type applicationTrace struct {
	mu         sync.Mutex
	operations []string
}

func (t *applicationTrace) add(operation string) {
	t.mu.Lock()
	t.operations = append(t.operations, operation)
	t.mu.Unlock()
}

func (t *applicationTrace) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.operations...)
}

func (t *applicationTrace) reset() {
	t.mu.Lock()
	t.operations = nil
	t.mu.Unlock()
}

type fakeApplicationTracking struct {
	trace  *applicationTrace
	merged chan tracking.MergedFrame
}

func (*fakeApplicationTracking) Submit(string, uint64, trackingmodel.TrackingFrame) error { return nil }
func (*fakeApplicationTracking) SetGeneration(uint64) error                               { return nil }
func (*fakeApplicationTracking) RemoveSource(string)                                      {}
func (t *fakeApplicationTracking) SubscribeMerged(context.Context) <-chan tracking.MergedFrame {
	t.trace.add("subscribe.merged")
	return t.merged
}

type fakeApplicationFrameSink struct{}

func (fakeApplicationFrameSink) Submit(string, uint64, trackingmodel.TrackingFrame) {}

type fakeApplicationManager struct {
	trace    *applicationTrace
	events   chan plugins.Event
	startErr error
	closeErr error
}

func (m *fakeApplicationManager) Start(context.Context) error {
	m.trace.add("plugins.start")
	return m.startErr
}

func (m *fakeApplicationManager) Close(context.Context) error {
	m.trace.add("plugins.close")
	return m.closeErr
}

func (*fakeApplicationManager) List() []plugins.RuntimeSnapshot { return nil }
func (*fakeApplicationManager) SetActive(context.Context, string, bool) error {
	return nil
}
func (*fakeApplicationManager) UpdateSubscription(context.Context, string, pluginapi.Subscription) error {
	return nil
}
func (m *fakeApplicationManager) Subscribe(context.Context) <-chan plugins.Event {
	m.trace.add("subscribe.plugins")
	return m.events
}

type fakeApplicationOSC struct {
	trace         *applicationTrace
	avatarChanges chan osc.AvatarChange
	events        chan osc.ControllerEvent
	startErr      error
	closeErr      error
}

func (o *fakeApplicationOSC) Start(context.Context) error {
	o.trace.add("osc.start")
	return o.startErr
}

func (o *fakeApplicationOSC) Close(context.Context) error {
	o.trace.add("osc.close")
	return o.closeErr
}

func (o *fakeApplicationOSC) Events() <-chan osc.ControllerEvent {
	o.trace.add("subscribe.osc")
	return o.events
}

func (o *fakeApplicationOSC) AvatarChanges(context.Context) <-chan osc.AvatarChange {
	o.trace.add("subscribe.avatar")
	return o.avatarChanges
}

func (*fakeApplicationOSC) ClearRuntime()                         {}
func (*fakeApplicationOSC) InstallCatalog(*osc.Catalog) error     { return nil }
func (*fakeApplicationOSC) Publish(uint64, osc.ValueSource) error { return nil }
func (*fakeApplicationOSC) Status() osc.OSCStatus                 { return osc.OSCStatus{} }

type fakeApplicationCoordinator struct {
	trace    *applicationTrace
	startErr error
	joinErr  error
	mu       sync.Mutex
	ready    chan struct{}
	done     chan struct{}
}

func (c *fakeApplicationCoordinator) Start(ctx context.Context, _ coordinatorInputs) error {
	c.trace.add("coordinator.start")
	if c.startErr != nil {
		return c.startErr
	}
	c.mu.Lock()
	c.ready = make(chan struct{})
	c.done = make(chan struct{})
	ready := c.ready
	done := c.done
	c.mu.Unlock()
	go func() {
		c.trace.add("coordinator.ready")
		close(ready)
		<-ctx.Done()
		c.trace.add("coordinator.cancel")
		close(done)
	}()
	return nil
}

func (c *fakeApplicationCoordinator) Ready() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ready
}

func (*fakeApplicationCoordinator) Reconcile([]plugins.RuntimeSnapshot) {}

func (c *fakeApplicationCoordinator) Join(ctx context.Context) error {
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.trace.add("coordinator.join")
	return c.joinErr
}

type fakeApplicationPipeline struct{}

func (*fakeApplicationPipeline) ProcessAt(tracking.MergedFrame, int64) (processing.CanonicalFrame, error) {
	return processing.CanonicalFrame{}, nil
}

type fakeApplicationPlanner struct{}

func (*fakeApplicationPlanner) Activate(string) activation { return activation{} }

type fakeApplicationTicker struct {
	ticks chan time.Time
}

func (t *fakeApplicationTicker) C() <-chan time.Time { return t.ticks }
func (*fakeApplicationTicker) Stop()                 {}
