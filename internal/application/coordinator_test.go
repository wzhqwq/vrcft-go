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
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func TestCoordinatorProcessesNewFrameImmediatelyAndLatestFrameOnTick(t *testing.T) {
	harness := newCoordinatorHarness(t)
	harness.coordinator.current = coordinatorReadyPlan(t, 7)
	harness.runtime.seedCatalog(harness.coordinator.current.Catalog())
	harness.pipeline.result = processing.CanonicalFrame{Generation: 7, EyeActive: true}
	harness.start()

	frame := tracking.MergedFrame{Generation: 7, Sequence: 1, UpdatedAtNS: 50}
	harness.merged <- frame
	harness.runtime.awaitPublishes(t, 1)
	harness.ticks <- time.Unix(0, 101)
	harness.runtime.awaitPublishes(t, 2)

	calls := harness.pipeline.snapshotCalls()
	if len(calls) != 2 || calls[0].frame != frame || calls[1].frame != frame {
		t.Fatalf("pipeline calls = %#v, want same frame twice", calls)
	}
	if calls[0].nowNS != 100 || calls[1].nowNS != 101 {
		t.Fatalf("pipeline times = %d, %d, want 100, 101", calls[0].nowNS, calls[1].nowNS)
	}
	publications := harness.runtime.snapshotPublications()
	if len(publications) != 2 || publications[0].generation != 7 || publications[1].generation != 7 {
		t.Fatalf("publications = %#v, want two generation-7 snapshots", publications)
	}
	for index, publication := range publications {
		if got, ok := publication.source.Bool(parameters.ParameterEyeTrackingActive); !ok || !got {
			t.Fatalf("publication %d EyeTrackingActive = (%t, %t), want (true, true)", index, got, ok)
		}
	}
}

func TestCoordinatorSkipsFramesWithoutUsableExactGenerationPlan(t *testing.T) {
	tests := []struct {
		name  string
		plan  planView
		frame tracking.MergedFrame
	}{
		{name: "no plan", frame: tracking.MergedFrame{Generation: 7, Sequence: 1}},
		{name: "old generation", plan: coordinatorReadyPlan(t, 7), frame: tracking.MergedFrame{Generation: 6, Sequence: 1}},
		{name: "future generation", plan: coordinatorReadyPlan(t, 7), frame: tracking.MergedFrame{Generation: 8, Sequence: 1}},
		{name: "failed plan", plan: &fakeInstallPlan{generation: 7, status: avatar.StatusFailed}, frame: tracking.MergedFrame{Generation: 7, Sequence: 1}},
		{name: "ready empty plan", plan: &fakeInstallPlan{generation: 7, status: avatar.StatusReady, catalog: &osc.Catalog{Generation: 7, Bindings: map[parameters.ParameterID]osc.ParameterBinding{}}, evaluator: &evaluator.Plan{}}, frame: tracking.MergedFrame{Generation: 7, Sequence: 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newCoordinatorHarness(t)
			harness.coordinator.current = test.plan
			if test.plan != nil && test.plan.Status() == avatar.StatusReady && len(test.plan.Catalog().Bindings) != 0 {
				harness.runtime.seedCatalog(test.plan.Catalog())
			}
			harness.start()

			harness.merged <- test.frame
			harness.sync(t)

			if got := len(harness.pipeline.snapshotCalls()); got != 0 {
				t.Fatalf("pipeline call count = %d, want zero", got)
			}
			if got := len(harness.runtime.snapshotPublications()); got != 0 {
				t.Fatalf("publication count = %d, want zero", got)
			}
		})
	}
}

func TestCoordinatorFrameFailureClearsAndRecoversCatalogBeforePublish(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*coordinatorHarness)
	}{
		{
			name: "processing failure",
			configure: func(h *coordinatorHarness) {
				h.pipeline.errs = []error{errors.New("bad processing input")}
			},
		},
		{
			name: "processed generation invariant",
			configure: func(h *coordinatorHarness) {
				h.pipeline.results = []processing.CanonicalFrame{{Generation: 99}}
			},
		},
		{
			name: "publish failure",
			configure: func(h *coordinatorHarness) {
				h.runtime.publishErrs = []error{errors.New("publish rejected")}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newCoordinatorHarness(t)
			harness.coordinator.current = coordinatorReadyPlan(t, 7)
			harness.runtime.seedCatalog(harness.coordinator.current.Catalog())
			harness.pipeline.result = processing.CanonicalFrame{Generation: 7, EyeActive: true}
			test.configure(harness)
			harness.start()

			harness.merged <- tracking.MergedFrame{Generation: 7, Sequence: 1}
			harness.runtime.awaitClears(t, 1)
			degraded := harness.awaitRuntimeError(t)
			if degraded.Lifecycle != LifecycleDegraded || degraded.RuntimeError == "" {
				t.Fatalf("degraded status = %+v, want lifecycle degraded and runtime error", degraded)
			}

			harness.ticks <- time.Unix(0, 101)
			harness.runtime.awaitPublishes(t, 1)
			harness.awaitRuntimeRecovery(t)
			operations := harness.runtime.snapshotOperations()
			if !reflect.DeepEqual(operations[len(operations)-2:], []string{"install:7", "publish:7"}) {
				t.Fatalf("recovery tail = %v, want [install:7 publish:7]", operations)
			}
			recovered := harness.status.snapshot()
			if recovered.Lifecycle != LifecycleRunning || recovered.RuntimeError != "" {
				t.Fatalf("recovered status = %+v, want running without runtime error", recovered)
			}
		})
	}
}

func TestCoordinatorRecoveryInstallFailureRemainsClosedAndDegraded(t *testing.T) {
	harness := newCoordinatorHarness(t)
	harness.coordinator.current = coordinatorReadyPlan(t, 7)
	harness.runtime.seedCatalog(harness.coordinator.current.Catalog())
	harness.pipeline.result = processing.CanonicalFrame{Generation: 7, EyeActive: true}
	harness.runtime.publishErrs = []error{errors.New("first publish failed")}
	harness.start()

	harness.merged <- tracking.MergedFrame{Generation: 7, Sequence: 1}
	harness.runtime.awaitClears(t, 1)
	harness.runtime.installErrs = []error{errors.New("catalog unavailable")}
	harness.ticks <- time.Unix(0, 101)
	harness.runtime.awaitClears(t, 2)
	harness.sync(t)

	if got := len(harness.runtime.snapshotPublications()); got != 0 {
		t.Fatalf("successful publications = %d, want zero", got)
	}
	status := harness.status.snapshot()
	if status.Lifecycle != LifecycleDegraded || !strings.Contains(status.RuntimeError, "catalog unavailable") {
		t.Fatalf("status = %+v, want catalog recovery failure", status)
	}
}

func TestCoordinatorAvatarMailboxCoalescesLatestWhileActivationRuns(t *testing.T) {
	planner := newBlockingActivationPlanner()
	harness := newCoordinatorHarnessWithPlanner(t, planner)
	harness.start()
	avatarChanges := harness.avatarChanges

	offerAvatarChange(avatarChanges, osc.AvatarChange{Revision: 1, AvatarID: "avtr_a"})
	planner.awaitStart(t)
	offerAvatarChange(avatarChanges, osc.AvatarChange{Revision: 2, AvatarID: "avtr_b"})
	offerAvatarChange(avatarChanges, osc.AvatarChange{Revision: 3, AvatarID: "avtr_b"})
	offerAvatarChange(avatarChanges, osc.AvatarChange{Revision: 4, AvatarID: "avtr_c"})
	close(planner.releaseFirst)
	planner.awaitCalls(t, 2)
	harness.sync(t)

	if got := planner.snapshotCalls(); !reflect.DeepEqual(got, []string{"avtr_a", "avtr_c"}) {
		t.Fatalf("activation order = %v, want [avtr_a avtr_c]", got)
	}
}

func TestCoordinatorRepeatedAvatarIDDuringActivationReloads(t *testing.T) {
	planner := newBlockingActivationPlanner()
	harness := newCoordinatorHarnessWithPlanner(t, planner)
	harness.start()
	avatarChanges := harness.avatarChanges

	offerAvatarChange(avatarChanges, osc.AvatarChange{Revision: 1, AvatarID: "avtr_same"})
	planner.awaitStart(t)
	offerAvatarChange(avatarChanges, osc.AvatarChange{Revision: 2, AvatarID: "avtr_same"})
	close(planner.releaseFirst)
	planner.awaitCalls(t, 2)
	harness.sync(t)

	if got := planner.snapshotCalls(); !reflect.DeepEqual(got, []string{"avtr_same", "avtr_same"}) {
		t.Fatalf("activation order = %v, want repeated avatar reload", got)
	}
}

func TestCoordinatorPluginLifecycleLossRemovesTrackingSource(t *testing.T) {
	harness := newCoordinatorHarness(t)
	harness.start()
	pluginEvents := harness.pluginEvents

	pluginEvents <- plugins.Event{Type: plugins.EventPluginStateChanged, PluginID: "stopped", Snapshot: &plugins.RuntimeSnapshot{ID: "stopped", State: plugins.StateStopped, Active: true}}
	harness.tracking.awaitRemoval(t)
	pluginEvents <- plugins.Event{Type: plugins.EventPluginStateChanged, PluginID: "inactive", Snapshot: &plugins.RuntimeSnapshot{ID: "inactive", State: plugins.StateRunning, Active: false}}
	harness.tracking.awaitRemoval(t)
	pluginEvents <- plugins.Event{Type: plugins.EventPluginRemoved, PluginID: "removed"}
	harness.tracking.awaitRemoval(t)
	pluginEvents <- plugins.Event{Type: plugins.EventPluginStateChanged, PluginID: "healthy", Snapshot: &plugins.RuntimeSnapshot{ID: "healthy", State: plugins.StateRunning, Active: true}}
	harness.sync(t)

	if got := harness.tracking.snapshotRemovals(); !reflect.DeepEqual(got, []string{"stopped", "inactive", "removed"}) {
		t.Fatalf("source removals = %v, want stopped, inactive, removed", got)
	}
}

func TestCoordinatorOSCDiagnosticsUpdateStatusWithoutOwningCatalog(t *testing.T) {
	harness := newCoordinatorHarness(t)
	harness.runtime.oscStatus = osc.OSCStatus{Running: true, Connected: true, HasTarget: true, Target: osc.OSCTarget{Host: "127.0.0.1", Port: 9000}}
	harness.start()
	before := harness.status.snapshot().Revision

	harness.oscEvents <- osc.ControllerEvent{
		Kind:    osc.EventCatalogUpdated,
		Catalog: &osc.Catalog{Generation: 999, Bindings: map[parameters.ParameterID]osc.ParameterBinding{parameters.ParameterJawOpen: {}}},
	}
	harness.awaitStatusRevision(t, before+1)

	if got := harness.runtime.snapshotOperations(); len(got) != 0 {
		t.Fatalf("OSC diagnostic mutated runtime: %v", got)
	}
	if got := harness.status.snapshot().OSC; got != harness.runtime.oscStatus {
		t.Fatalf("status OSC = %+v, want %+v", got, harness.runtime.oscStatus)
	}
}

func TestCoordinatorCancellationReturnsAndStopsCallbacks(t *testing.T) {
	harness := newCoordinatorHarness(t)
	harness.coordinator.current = coordinatorReadyPlan(t, 7)
	harness.runtime.seedCatalog(harness.coordinator.current.Catalog())
	harness.pipeline.result = processing.CanonicalFrame{Generation: 7, EyeActive: true}
	harness.start()

	harness.cancel()
	harness.awaitDone(t)
	beforeCalls := len(harness.pipeline.snapshotCalls())
	beforePublishes := len(harness.runtime.snapshotPublications())
	offerMerged(harness.merged, tracking.MergedFrame{Generation: 7, Sequence: 1})
	offerTick(harness.ticks, time.Unix(0, 101))
	time.Sleep(5 * time.Millisecond)

	if got := len(harness.pipeline.snapshotCalls()); got != beforeCalls {
		t.Fatalf("pipeline calls after cancellation = %d, want %d", got, beforeCalls)
	}
	if got := len(harness.runtime.snapshotPublications()); got != beforePublishes {
		t.Fatalf("publications after cancellation = %d, want %d", got, beforePublishes)
	}
}

func TestCoordinatorSignalsReadyAndPreservesClosingLifecycle(t *testing.T) {
	ready := make(chan struct{})
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	c := &coordinator{clock: newMonotonicClock(nil)}
	go func() {
		defer close(done)
		c.run(ctx, coordinatorInputs{}, ready)
	}()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coordinator readiness")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coordinator cancellation")
	}

	harness := newCoordinatorHarness(t)
	harness.coordinator.current = coordinatorReadyPlan(t, 7)
	harness.runtime.seedCatalog(harness.coordinator.current.Catalog())
	harness.pipeline.result = processing.CanonicalFrame{Generation: 7, EyeActive: true}
	harness.status.update(func(status *Status) { status.Lifecycle = LifecycleClosing })
	harness.merged <- tracking.MergedFrame{Generation: 7, Sequence: 1}
	harness.runtime.awaitPublishes(t, 1)

	if got := harness.status.snapshot().Lifecycle; got != LifecycleClosing {
		t.Fatalf("lifecycle after in-flight publication = %q, want closing", got)
	}
}

func TestCoordinatorStatusErrorsStayValidAndBounded(t *testing.T) {
	message := strings.Repeat("a", 511) + "\xff\xff"
	got := coordinatorErrorMessage(errors.New(message))
	if len(got) > 512 {
		t.Fatalf("sanitized error length = %d, want at most 512 bytes", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("sanitized error is not valid UTF-8: %q", got)
	}
}

type coordinatorHarness struct {
	coordinator   *coordinator
	inputs        coordinatorInputs
	pipeline      *fakeFramePipeline
	runtime       *fakeRuntimePublisher
	tracking      *fakeCoordinatorTracking
	status        *statusStore
	avatarChanges chan osc.AvatarChange
	oscEvents     chan osc.ControllerEvent
	pluginEvents  chan plugins.Event
	merged        chan tracking.MergedFrame
	ticks         chan time.Time
	cancel        context.CancelFunc
	done          chan struct{}
}

func newCoordinatorHarness(t *testing.T) *coordinatorHarness {
	t.Helper()
	return newCoordinatorHarnessWithPlanner(t, &fixedActivationPlanner{})
}

func newCoordinatorHarnessWithPlanner(t *testing.T, planner activationPlanner) *coordinatorHarness {
	t.Helper()
	runtime := newFakeRuntimePublisher()
	trackingControl := newFakeCoordinatorTracking()
	status := newStatusStore(func() time.Time { return time.Unix(0, 1) })
	pipeline := &fakeFramePipeline{notify: make(chan struct{}, 32)}
	pluginControls := &coordinatorPluginControls{}
	installer := &planInstaller{
		plugins:              pluginControls,
		tracking:             trackingControl,
		osc:                  runtime,
		pluginControlTimeout: time.Second,
	}
	walls := []time.Time{time.Unix(0, 100), time.Unix(0, 100), time.Unix(0, 102), time.Unix(0, 103)}
	clock := newMonotonicClock(func() time.Time {
		if len(walls) == 0 {
			return time.Unix(0, 200)
		}
		next := walls[0]
		walls = walls[1:]
		return next
	})
	ctx, cancel := context.WithCancel(context.Background())
	avatarChanges := make(chan osc.AvatarChange, 1)
	oscEvents := make(chan osc.ControllerEvent)
	pluginEvents := make(chan plugins.Event)
	merged := make(chan tracking.MergedFrame, 1)
	ticks := make(chan time.Time, 1)
	harness := &coordinatorHarness{
		pipeline:      pipeline,
		runtime:       runtime,
		tracking:      trackingControl,
		status:        status,
		avatarChanges: avatarChanges,
		oscEvents:     oscEvents,
		pluginEvents:  pluginEvents,
		merged:        merged,
		ticks:         ticks,
		cancel:        cancel,
		done:          make(chan struct{}),
		inputs: coordinatorInputs{
			avatarChanges: avatarChanges,
			oscEvents:     oscEvents,
			pluginEvents:  pluginEvents,
			merged:        merged,
			ticks:         ticks,
		},
	}
	harness.coordinator = &coordinator{
		planner:   planner,
		installer: installer,
		pipeline:  pipeline,
		tracking:  trackingControl,
		runtime:   runtime,
		status:    status,
		clock:     clock,
	}
	go func() {
		defer close(harness.done)
		harness.coordinator.run(ctx, harness.inputs, nil)
	}()
	t.Cleanup(func() {
		cancel()
		harness.awaitDone(t)
	})
	return harness
}

func (h *coordinatorHarness) start() {
	// The coordinator goroutine is already running. This method documents the
	// point at which the complete fixture is ready for externally visible input.
}

func (h *coordinatorHarness) sync(t *testing.T) {
	t.Helper()
	h.pluginEvents <- plugins.Event{Type: plugins.EventPluginRemoved, PluginID: "sync"}
	for {
		if h.tracking.awaitRemoval(t) == "sync" {
			h.tracking.discardSyncRemoval()
			return
		}
	}
}

func (h *coordinatorHarness) awaitStatusRevision(t *testing.T, revision uint64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.status.snapshot().Revision >= revision {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("status revision = %d, want at least %d", h.status.snapshot().Revision, revision)
}

func (h *coordinatorHarness) awaitRuntimeError(t *testing.T) Status {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := h.status.snapshot()
		if status.RuntimeError != "" {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	status := h.status.snapshot()
	t.Fatalf("status = %+v, want runtime error", status)
	return Status{}
}

func (h *coordinatorHarness) awaitRuntimeRecovery(t *testing.T) Status {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := h.status.snapshot()
		if status.RuntimeError == "" && status.Lifecycle == LifecycleRunning {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	status := h.status.snapshot()
	t.Fatalf("status = %+v, want recovered runtime", status)
	return Status{}
}

func (h *coordinatorHarness) awaitDone(t *testing.T) {
	t.Helper()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coordinator exit")
	}
}

func coordinatorReadyPlan(t *testing.T, generation uint64) planView {
	t.Helper()
	evaluatorPlan, err := evaluator.Compile([]parameters.ParameterID{parameters.ParameterEyeTrackingActive})
	if err != nil {
		t.Fatal(err)
	}
	return &fakeInstallPlan{
		generation:   generation,
		status:       avatar.StatusReady,
		avatarID:     "avtr_test",
		parameterIDs: []parameters.ParameterID{parameters.ParameterEyeTrackingActive},
		catalog: &osc.Catalog{
			Generation: generation,
			Bindings: map[parameters.ParameterID]osc.ParameterBinding{
				parameters.ParameterEyeTrackingActive: {},
			},
		},
		evaluator: evaluatorPlan,
	}
}

type pipelineCall struct {
	frame tracking.MergedFrame
	nowNS int64
}

type fakeFramePipeline struct {
	mu      sync.Mutex
	calls   []pipelineCall
	result  processing.CanonicalFrame
	results []processing.CanonicalFrame
	errs    []error
	notify  chan struct{}
}

func (p *fakeFramePipeline) ProcessAt(frame tracking.MergedFrame, nowNS int64) (processing.CanonicalFrame, error) {
	p.mu.Lock()
	p.calls = append(p.calls, pipelineCall{frame: frame, nowNS: nowNS})
	result := p.result
	if len(p.results) != 0 {
		result = p.results[0]
		p.results = p.results[1:]
	}
	var err error
	if len(p.errs) != 0 {
		err = p.errs[0]
		p.errs = p.errs[1:]
	}
	p.mu.Unlock()
	select {
	case p.notify <- struct{}{}:
	default:
	}
	return result, err
}

func (p *fakeFramePipeline) snapshotCalls() []pipelineCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]pipelineCall(nil), p.calls...)
}

type runtimePublication struct {
	generation uint64
	source     osc.ValueSource
}

type fakeRuntimePublisher struct {
	mu            sync.Mutex
	catalog       *osc.Catalog
	publications  []runtimePublication
	operations    []string
	installErrs   []error
	publishErrs   []error
	clearCount    int
	clearNotify   chan struct{}
	publishNotify chan struct{}
	oscStatus     osc.OSCStatus
}

func newFakeRuntimePublisher() *fakeRuntimePublisher {
	return &fakeRuntimePublisher{clearNotify: make(chan struct{}, 32), publishNotify: make(chan struct{}, 32)}
}

func (r *fakeRuntimePublisher) seedCatalog(catalog *osc.Catalog) {
	r.mu.Lock()
	r.catalog = catalog.Clone()
	r.mu.Unlock()
}

func (r *fakeRuntimePublisher) ClearRuntime() {
	r.mu.Lock()
	r.catalog = nil
	r.clearCount++
	r.operations = append(r.operations, "clear")
	r.mu.Unlock()
	select {
	case r.clearNotify <- struct{}{}:
	default:
	}
}

func (r *fakeRuntimePublisher) InstallCatalog(catalog *osc.Catalog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, fmt.Sprintf("install:%d", catalog.Generation))
	if len(r.installErrs) != 0 {
		err := r.installErrs[0]
		r.installErrs = r.installErrs[1:]
		return err
	}
	r.catalog = catalog.Clone()
	return nil
}

func (r *fakeRuntimePublisher) Publish(generation uint64, source osc.ValueSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, fmt.Sprintf("publish:%d", generation))
	if len(r.publishErrs) != 0 {
		err := r.publishErrs[0]
		r.publishErrs = r.publishErrs[1:]
		return err
	}
	if r.catalog == nil || r.catalog.Generation != generation {
		return fmt.Errorf("catalog generation unavailable")
	}
	r.publications = append(r.publications, runtimePublication{generation: generation, source: source})
	select {
	case r.publishNotify <- struct{}{}:
	default:
	}
	return nil
}

func (r *fakeRuntimePublisher) Status() osc.OSCStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.oscStatus
}

func (r *fakeRuntimePublisher) snapshotOperations() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.operations...)
}

func (r *fakeRuntimePublisher) snapshotPublications() []runtimePublication {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runtimePublication(nil), r.publications...)
}

func (r *fakeRuntimePublisher) awaitPublishes(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if len(r.snapshotPublications()) >= count {
			return
		}
		select {
		case <-r.publishNotify:
		case <-deadline:
			t.Fatalf("publication count = %d, want at least %d", len(r.snapshotPublications()), count)
		}
	}
}

func (r *fakeRuntimePublisher) awaitClears(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		r.mu.Lock()
		got := r.clearCount
		r.mu.Unlock()
		if got >= count {
			return
		}
		select {
		case <-r.clearNotify:
		case <-deadline:
			t.Fatalf("clear count = %d, want at least %d", got, count)
		}
	}
}

type fakeCoordinatorTracking struct {
	mu         sync.Mutex
	generation uint64
	removals   []string
	removed    chan string
}

func newFakeCoordinatorTracking() *fakeCoordinatorTracking {
	return &fakeCoordinatorTracking{removed: make(chan string, 32)}
}

func (t *fakeCoordinatorTracking) SetGeneration(generation uint64) error {
	t.mu.Lock()
	t.generation = generation
	t.mu.Unlock()
	return nil
}

func (t *fakeCoordinatorTracking) RemoveSource(pluginID string) {
	t.mu.Lock()
	t.removals = append(t.removals, pluginID)
	t.mu.Unlock()
	t.removed <- pluginID
}

func (t *fakeCoordinatorTracking) awaitRemoval(testingT *testing.T) string {
	testingT.Helper()
	select {
	case pluginID := <-t.removed:
		return pluginID
	case <-time.After(time.Second):
		testingT.Fatal("timed out waiting for tracking source removal")
		return ""
	}
}

func (t *fakeCoordinatorTracking) snapshotRemovals() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.removals...)
}

func (t *fakeCoordinatorTracking) discardSyncRemoval() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for index := len(t.removals) - 1; index >= 0; index-- {
		if t.removals[index] == "sync" {
			t.removals = append(t.removals[:index], t.removals[index+1:]...)
			return
		}
	}
}

type coordinatorPluginControls struct{}

func (*coordinatorPluginControls) List() []plugins.RuntimeSnapshot { return nil }
func (*coordinatorPluginControls) SetActive(context.Context, string, bool) error {
	return nil
}
func (*coordinatorPluginControls) UpdateSubscription(context.Context, string, pluginapi.Subscription) error {
	return nil
}

type fixedActivationPlanner struct{}

func (*fixedActivationPlanner) Activate(string) activation { return activation{} }

type blockingActivationPlanner struct {
	mu           sync.Mutex
	calls        []string
	started      chan struct{}
	releaseFirst chan struct{}
}

func newBlockingActivationPlanner() *blockingActivationPlanner {
	return &blockingActivationPlanner{started: make(chan struct{}, 8), releaseFirst: make(chan struct{})}
}

func (p *blockingActivationPlanner) Activate(avatarID string) activation {
	p.mu.Lock()
	p.calls = append(p.calls, avatarID)
	call := len(p.calls)
	p.mu.Unlock()
	p.started <- struct{}{}
	if call == 1 {
		<-p.releaseFirst
	}
	return activation{plan: &fakeInstallPlan{
		generation: uint64(call),
		status:     avatar.StatusReady,
		avatarID:   avatarID,
		catalog:    &osc.Catalog{Generation: uint64(call), Bindings: map[parameters.ParameterID]osc.ParameterBinding{}},
		evaluator:  &evaluator.Plan{},
	}}
}

func (p *blockingActivationPlanner) awaitStart(t *testing.T) {
	t.Helper()
	select {
	case <-p.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for planner activation")
	}
}

func (p *blockingActivationPlanner) awaitCalls(t *testing.T, count int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if len(p.snapshotCalls()) >= count {
			return
		}
		select {
		case <-p.started:
		case <-deadline:
			t.Fatalf("planner calls = %v, want at least %d", p.snapshotCalls(), count)
		}
	}
}

func (p *blockingActivationPlanner) snapshotCalls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func offerAvatarChange(ch chan osc.AvatarChange, change osc.AvatarChange) {
	select {
	case ch <- change:
	default:
		select {
		case <-ch:
		default:
		}
		ch <- change
	}
}

func offerMerged(ch chan tracking.MergedFrame, frame tracking.MergedFrame) {
	select {
	case ch <- frame:
	default:
	}
}

func offerTick(ch chan time.Time, tick time.Time) {
	select {
	case ch <- tick:
	default:
	}
}
