package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

// TestApplicationAvatarAwareOSCEndToEnd catches any bypass of the real avatar
// planner, tracking merge, processing pipeline, evaluator, or compiled binding
// catalog at the Application boundary. Only process, network, and time
// boundaries are deterministic in-memory fakes.
func TestApplicationAvatarAwareOSCEndToEnd(t *testing.T) {
	const (
		pluginID  = "vendor.expression"
		avatarOne = "avtr_one"
		avatarTwo = "avtr_two"
	)

	fixture := newAvatarAwareEndToEndFixture(t)
	fixture.writeAvatarConfig(t, avatarOne, `{
		"id": "avtr_one",
		"name": "Generation One",
		"parameters": [
			{"name": "Face/v2/JawOpen", "input": {"address": "/avatar/parameters/Face/v2/JawOpen", "type": "Float"}},
			{"name": "ExpressionTrackingActive", "input": {"address": "/avatar/parameters/ExpressionTrackingActive", "type": "Bool"}}
		]
	}`)
	avatarTwoJSON := `{
		"id": "avtr_two",
		"name": "Generation Two",
		"parameters": [
			{"name": "Face/v2/MouthClosed", "input": {"address": "/avatar/parameters/Face/v2/MouthClosed", "type": "Float"}},
			{"name": "ExpressionTrackingActive", "input": {"address": "/avatar/parameters/ExpressionTrackingActive", "type": "Bool"}}
		]
	}`
	fixture.writeAvatarConfig(t, avatarTwo, avatarTwoJSON)
	fixture.start(t)

	fixture.runtime.offerAvatarChange(osc.AvatarChange{Revision: 1, AvatarID: avatarOne})
	statusOne := awaitEndToEndStatus(t, fixture.app, func(status Status) bool {
		return status.PlanGeneration == 1 && status.PlanStatus == avatar.StatusReady
	})
	if statusOne.AvatarID != avatarOne || statusOne.ConfigID != avatarOne || statusOne.PlanSource != avatar.SourceAvatarConfig || statusOne.PlanError != "" || statusOne.RuntimeError != "" {
		t.Fatalf("generation 1 status = %#v, want ready avatar config without errors", statusOne)
	}
	subscriptionOne := awaitEndToEndPlugin(t, fixture.manager, func(state endToEndPluginState) bool {
		return state.active && state.subscription.Generation == 1
	})
	assertEndToEndSubscription(t, subscriptionOne.subscription, pluginapi.Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityExpression,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	})

	frameOne := trackingmodel.TrackingFrame{
		Sequence:     1,
		Capabilities: trackingmodel.CapabilityExpression,
	}
	frameOne.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.75)
	fixture.sink.Submit(pluginID, 1, frameOne)
	outputOne := awaitEndToEndRuntime(t, fixture.runtime, func(state endToEndRuntimeState) bool {
		return state.catalog != nil && state.catalog.Generation == 1 &&
			endToEndFloatEquals(state.source, parameters.ParameterJawOpen, 0.75) &&
			endToEndBoolEquals(state.source, parameters.ParameterExpressionTrackingActive, true)
	})
	assertEndToEndCatalogIDs(t, outputOne.catalog,
		parameters.ParameterJawOpen,
		parameters.ParameterExpressionTrackingActive,
	)
	assertEndToEndFloat(t, outputOne.source, parameters.ParameterJawOpen, 0.75, true)
	assertEndToEndBool(t, outputOne.source, parameters.ParameterExpressionTrackingActive, true, true)
	assertEndToEndFloat(t, outputOne.source, parameters.ParameterMouthClosed, 0, false)
	generationOneSource := outputOne.source

	fixture.runtime.offerAvatarChange(osc.AvatarChange{Revision: 2, AvatarID: avatarTwo})
	statusTwo := awaitEndToEndStatus(t, fixture.app, func(status Status) bool {
		return status.PlanGeneration == 2 && status.PlanStatus == avatar.StatusReady
	})
	if statusTwo.AvatarID != avatarTwo || statusTwo.ConfigID != avatarTwo || statusTwo.PlanError != "" || statusTwo.RuntimeError != "" {
		t.Fatalf("generation 2 status = %#v, want ready replacement plan", statusTwo)
	}
	subscriptionTwo := awaitEndToEndPlugin(t, fixture.manager, func(state endToEndPluginState) bool {
		return state.active && state.subscription.Generation == 2
	})
	assertEndToEndSubscription(t, subscriptionTwo.subscription, pluginapi.Subscription{
		Generation:   2,
		Capabilities: trackingmodel.CapabilityExpression,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionMouthClosed),
	})

	frameTwo := trackingmodel.TrackingFrame{
		Sequence:     1,
		Capabilities: trackingmodel.CapabilityExpression,
	}
	frameTwo.Expressions.Set(trackingmodel.ExpressionMouthClosed, 0.25)
	fixture.sink.Submit(pluginID, 2, frameTwo)
	outputTwo := awaitEndToEndRuntime(t, fixture.runtime, func(state endToEndRuntimeState) bool {
		return state.catalog != nil && state.catalog.Generation == 2 &&
			endToEndFloatEquals(state.source, parameters.ParameterMouthClosed, 0.25) &&
			endToEndBoolEquals(state.source, parameters.ParameterExpressionTrackingActive, true)
	})
	assertEndToEndCatalogIDs(t, outputTwo.catalog,
		parameters.ParameterMouthClosed,
		parameters.ParameterExpressionTrackingActive,
	)
	assertEndToEndFloat(t, outputTwo.source, parameters.ParameterMouthClosed, 0.25, true)
	assertEndToEndBool(t, outputTwo.source, parameters.ParameterExpressionTrackingActive, true, true)
	assertEndToEndFloat(t, outputTwo.source, parameters.ParameterJawOpen, 0, false)

	lateFrame := trackingmodel.TrackingFrame{
		Sequence:     2,
		Capabilities: trackingmodel.CapabilityExpression,
	}
	lateFrame.Expressions.Set(trackingmodel.ExpressionJawOpen, 1)
	trackingBeforeStale, ok := fixture.tracking.LatestMerged()
	if !ok {
		t.Fatal("generation 2 LatestMerged() unavailable before stale submission")
	}
	beforeStale := fixture.runtime.snapshot()
	publicationMarker := fixture.runtime.publicationCount()
	if err := fixture.tracking.Submit(pluginID, 1, lateFrame); !errors.Is(err, tracking.ErrStaleGeneration) {
		t.Fatalf("late generation 1 tracking Submit() error = %v, want ErrStaleGeneration", err)
	}
	trackingAfterStale, ok := fixture.tracking.LatestMerged()
	if !ok || trackingAfterStale != trackingBeforeStale {
		t.Fatalf("late generation 1 tracking changed merged state from %#v to %#v,%t", trackingBeforeStale, trackingAfterStale, ok)
	}
	if err := fixture.runtime.Publish(1, generationOneSource); !errors.Is(err, osc.ErrRuntimeGeneration) {
		t.Fatalf("late generation 1 OSC Publish() error = %v, want ErrRuntimeGeneration", err)
	}

	barrierFrame := trackingmodel.TrackingFrame{
		Sequence:     2,
		Capabilities: trackingmodel.CapabilityExpression,
	}
	barrierFrame.Expressions.Set(trackingmodel.ExpressionMouthClosed, 0.5)
	fixture.sink.Submit(pluginID, 2, barrierFrame)
	barrierOutput := awaitEndToEndRuntime(t, fixture.runtime, func(state endToEndRuntimeState) bool {
		return state.revision > beforeStale.revision &&
			endToEndFloatEquals(state.source, parameters.ParameterMouthClosed, 0.5) &&
			endToEndBoolEquals(state.source, parameters.ParameterExpressionTrackingActive, true)
	})
	publications := fixture.runtime.publicationsFrom(publicationMarker)
	if len(publications) != 1 {
		t.Fatalf("OSC publications after stale submission = %#v, want exactly the current-generation barrier", publications)
	}
	barrierPublication := publications[0]
	if barrierPublication.revision != barrierOutput.revision || barrierPublication.generation != 2 {
		t.Fatalf("barrier publication = %#v, want runtime revision %d and generation 2", barrierPublication, barrierOutput.revision)
	}
	assertEndToEndCatalogIDs(t, barrierPublication.catalog,
		parameters.ParameterMouthClosed,
		parameters.ParameterExpressionTrackingActive,
	)
	assertEndToEndFloat(t, barrierPublication.source, parameters.ParameterMouthClosed, 0.5, true)
	assertEndToEndBool(t, barrierPublication.source, parameters.ParameterExpressionTrackingActive, true, true)
	assertEndToEndFloat(t, barrierPublication.source, parameters.ParameterJawOpen, 0, false)

	trackingAfterBarrier, ok := fixture.tracking.LatestMerged()
	barrierValue, barrierValueOK := trackingAfterBarrier.Expressions.Get(trackingmodel.ExpressionMouthClosed)
	if !ok || trackingAfterBarrier.Generation != 2 || trackingAfterBarrier.Sequence <= trackingBeforeStale.Sequence || !barrierValueOK || barrierValue != 0.5 {
		t.Fatalf("generation 2 tracking barrier = %#v,%t, MouthClosed=%v,%t", trackingAfterBarrier, ok, barrierValue, barrierValueOK)
	}

	fixture.writeAvatarConfig(t, avatarTwo, `{"id":`)
	fixture.runtime.offerAvatarChange(osc.AvatarChange{Revision: 3, AvatarID: avatarTwo})
	statusThree := awaitEndToEndStatus(t, fixture.app, func(status Status) bool {
		return status.PlanGeneration == 3 && status.PlanStatus == avatar.StatusFailed
	})
	if statusThree.AvatarID != avatarTwo || statusThree.PlanError == "" || statusThree.Lifecycle != LifecycleDegraded {
		t.Fatalf("generation 3 status = %#v, want failed/degraded plan", statusThree)
	}
	pluginThree := fixture.manager.snapshot()
	if pluginThree.active {
		t.Fatal("generation 3 malformed plan left expression plugin active")
	}
	runtimeThree := fixture.runtime.snapshot()
	if runtimeThree.catalog != nil || runtimeThree.source != nil || runtimeThree.generation != 0 {
		t.Fatalf("generation 3 malformed plan runtime = %#v, want cleared catalog/source", runtimeThree)
	}

	fixture.writeAvatarConfig(t, avatarTwo, avatarTwoJSON)
	fixture.runtime.offerAvatarChange(osc.AvatarChange{Revision: 4, AvatarID: avatarTwo})
	statusFour := awaitEndToEndStatus(t, fixture.app, func(status Status) bool {
		return status.PlanGeneration == 4 && status.PlanStatus == avatar.StatusReady
	})
	if statusFour.AvatarID != avatarTwo || statusFour.PlanError != "" || statusFour.RuntimeError != "" || statusFour.Lifecycle != LifecycleRunning {
		t.Fatalf("generation 4 status = %#v, want recovered ready plan", statusFour)
	}
	subscriptionFour := awaitEndToEndPlugin(t, fixture.manager, func(state endToEndPluginState) bool {
		return state.active && state.subscription.Generation == 4
	})
	assertEndToEndSubscription(t, subscriptionFour.subscription, pluginapi.Subscription{
		Generation:   4,
		Capabilities: trackingmodel.CapabilityExpression,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionMouthClosed),
	})

	frameFour := trackingmodel.TrackingFrame{
		Sequence:     1,
		Capabilities: trackingmodel.CapabilityExpression,
	}
	frameFour.Expressions.Set(trackingmodel.ExpressionMouthClosed, 0.4)
	fixture.sink.Submit(pluginID, 4, frameFour)
	outputFour := awaitEndToEndRuntime(t, fixture.runtime, func(state endToEndRuntimeState) bool {
		return state.catalog != nil && state.catalog.Generation == 4 &&
			endToEndFloatEquals(state.source, parameters.ParameterMouthClosed, 0.4) &&
			endToEndBoolEquals(state.source, parameters.ParameterExpressionTrackingActive, true)
	})
	assertEndToEndCatalogIDs(t, outputFour.catalog,
		parameters.ParameterMouthClosed,
		parameters.ParameterExpressionTrackingActive,
	)

	merged, ok := fixture.tracking.LatestMerged()
	if !ok || merged.Generation != 4 || merged.ExpressionUpdatedAtNS == 0 {
		t.Fatalf("generation 4 LatestMerged() = %#v,%t, want expression freshness", merged, ok)
	}
	tickAt := time.Unix(0, merged.ExpressionUpdatedAtNS).Add(fixture.config.Processing.ActiveStaleAfter + time.Nanosecond)
	fixture.clock.set(tickAt)
	fixture.ticker.tick(t, tickAt)
	timedOut := awaitEndToEndRuntime(t, fixture.runtime, func(state endToEndRuntimeState) bool {
		return state.revision > outputFour.revision &&
			endToEndBoolEquals(state.source, parameters.ParameterExpressionTrackingActive, false)
	})
	assertEndToEndBool(t, timedOut.source, parameters.ParameterExpressionTrackingActive, false, true)
	assertEndToEndFloat(t, timedOut.source, parameters.ParameterMouthClosed, 0.4, true)
	assertEndToEndFloat(t, timedOut.source, parameters.ParameterJawOpen, 0, false)
}

type avatarAwareEndToEndFixture struct {
	app      *Application
	config   Config
	oscRoot  string
	manager  *endToEndManager
	runtime  *endToEndRuntime
	tracking tracking.Service
	sink     plugins.FrameSink
	ticker   *endToEndTicker
	clock    *endToEndClock
}

func newAvatarAwareEndToEndFixture(t *testing.T) *avatarAwareEndToEndFixture {
	t.Helper()
	config := validApplicationConfig(t)
	config.Avatar.OSCRoot = filepath.Join(t.TempDir(), "OSC")
	// The test clock starts one hour ahead of the real tracking receipt clock.
	// A two-hour activity window therefore keeps ordinary submissions active,
	// while a manual tick can cross the boundary without waiting in real time.
	config.Processing.ActiveStaleAfter = 2 * time.Hour
	config.Processing.DefaultChannel.Dropout.StaleAfter = 4 * time.Hour

	fixture := &avatarAwareEndToEndFixture{
		config:  config,
		oscRoot: config.Avatar.OSCRoot,
		manager: newEndToEndManager(),
		runtime: newEndToEndRuntime(),
		ticker:  newEndToEndTicker(),
		clock:   newEndToEndClock(time.Now().Add(time.Hour)),
	}
	dependencies := productionApplicationDependencies()
	dependencies.newTracking = func() (applicationTracking, error) {
		fixture.tracking = tracking.NewService()
		return fixture.tracking, nil
	}
	dependencies.newPluginManager = func(_ normalizedConfig, sink plugins.FrameSink) (applicationPluginManager, error) {
		fixture.sink = sink
		return fixture.manager, nil
	}
	dependencies.newOSC = func(config osc.ControllerConfig) (applicationOSC, error) {
		if config.CatalogMode != osc.CatalogExternal {
			return nil, errors.New("end-to-end OSC runtime requires external catalog mode")
		}
		return fixture.runtime, nil
	}
	dependencies.newTicker = func(time.Duration) applicationTicker { return fixture.ticker }
	dependencies.now = fixture.clock.now
	fixture.app = newApplicationForTest(t, config, dependencies)
	if fixture.tracking == nil || fixture.sink == nil {
		t.Fatal("end-to-end construction did not retain real tracking service and frame sink")
	}
	return fixture
}

func (fixture *avatarAwareEndToEndFixture) writeAvatarConfig(t *testing.T, avatarID, content string) {
	t.Helper()
	path := filepath.Join(fixture.oscRoot, "usr_test", "Avatars", avatarID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func (fixture *avatarAwareEndToEndFixture) start(t *testing.T) {
	t.Helper()
	if err := fixture.app.Start(context.Background()); err != nil {
		t.Fatalf("Application.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := fixture.app.Close(ctx); err != nil {
			t.Errorf("Application.Close() error = %v", err)
		}
	})
}

type endToEndPluginState struct {
	active       bool
	subscription pluginapi.Subscription
}

type endToEndManager struct {
	mu           sync.Mutex
	active       bool
	subscription pluginapi.Subscription
	started      bool
	closed       bool
	events       chan plugins.Event
	changed      chan struct{}
}

func newEndToEndManager() *endToEndManager {
	return &endToEndManager{
		events:  make(chan plugins.Event),
		changed: make(chan struct{}, 1),
	}
}

func (manager *endToEndManager) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("end-to-end manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	manager.mu.Lock()
	manager.started = true
	manager.mu.Unlock()
	signalEndToEndChange(manager.changed)
	return nil
}

func (manager *endToEndManager) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("end-to-end manager close context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	manager.mu.Lock()
	manager.closed = true
	manager.active = false
	manager.mu.Unlock()
	signalEndToEndChange(manager.changed)
	return nil
}

func (manager *endToEndManager) List() []plugins.RuntimeSnapshot {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return []plugins.RuntimeSnapshot{{
		ID:                     "vendor.expression",
		Name:                   "Expression Fixture",
		Capabilities:           trackingmodel.CapabilityExpression,
		Enabled:                true,
		Active:                 manager.active,
		State:                  plugins.StateRunning,
		PID:                    1,
		SubscriptionGeneration: manager.subscription.Generation,
	}}
}

func (manager *endToEndManager) SetActive(ctx context.Context, pluginID string, active bool) error {
	if ctx == nil {
		return errors.New("end-to-end manager control context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if pluginID != "vendor.expression" {
		return plugins.ErrUnknownPlugin
	}
	manager.mu.Lock()
	if !manager.started || manager.closed {
		manager.mu.Unlock()
		return plugins.ErrManagerNotStarted
	}
	manager.active = active
	manager.mu.Unlock()
	signalEndToEndChange(manager.changed)
	return nil
}

func (manager *endToEndManager) UpdateSubscription(ctx context.Context, pluginID string, subscription pluginapi.Subscription) error {
	if ctx == nil {
		return errors.New("end-to-end manager subscription context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if pluginID != "vendor.expression" {
		return plugins.ErrUnknownPlugin
	}
	if err := subscription.Validate(false); err != nil {
		return err
	}
	manager.mu.Lock()
	if !manager.started || manager.closed {
		manager.mu.Unlock()
		return plugins.ErrManagerNotStarted
	}
	manager.subscription = subscription
	manager.mu.Unlock()
	signalEndToEndChange(manager.changed)
	return nil
}

func (manager *endToEndManager) Subscribe(context.Context) <-chan plugins.Event {
	return manager.events
}

func (manager *endToEndManager) snapshot() endToEndPluginState {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return endToEndPluginState{
		active:       manager.active,
		subscription: manager.subscription,
	}
}

type endToEndRuntimeState struct {
	revision   uint64
	generation uint64
	catalog    *osc.Catalog
	source     osc.ValueSource
}

type endToEndSourceSnapshot struct {
	floats     [parameters.ParameterCount]float32
	floatValid [parameters.ParameterCount]bool
	bools      [parameters.ParameterCount]bool
	boolValid  [parameters.ParameterCount]bool
}

func snapshotEndToEndSource(source osc.ValueSource) endToEndSourceSnapshot {
	var snapshot endToEndSourceSnapshot
	if source == nil {
		return snapshot
	}
	for id := parameters.ParameterID(0); id < parameters.ParameterCount; id++ {
		snapshot.floats[id], snapshot.floatValid[id] = source.Float(id)
		snapshot.bools[id], snapshot.boolValid[id] = source.Bool(id)
	}
	return snapshot
}

func (snapshot endToEndSourceSnapshot) Float(id parameters.ParameterID) (float32, bool) {
	if id >= parameters.ParameterCount {
		return 0, false
	}
	return snapshot.floats[id], snapshot.floatValid[id]
}

func (snapshot endToEndSourceSnapshot) Bool(id parameters.ParameterID) (bool, bool) {
	if id >= parameters.ParameterCount {
		return false, false
	}
	return snapshot.bools[id], snapshot.boolValid[id]
}

type endToEndPublication struct {
	revision   uint64
	generation uint64
	catalog    *osc.Catalog
	source     endToEndSourceSnapshot
}

type endToEndRuntime struct {
	mu           sync.Mutex
	running      bool
	revision     uint64
	generation   uint64
	catalog      *osc.Catalog
	source       osc.ValueSource
	publications []endToEndPublication
	changes      chan osc.AvatarChange
	events       chan osc.ControllerEvent
	stateChange  chan struct{}
}

func newEndToEndRuntime() *endToEndRuntime {
	return &endToEndRuntime{
		changes:     make(chan osc.AvatarChange, 8),
		events:      make(chan osc.ControllerEvent),
		stateChange: make(chan struct{}, 1),
	}
}

func (runtime *endToEndRuntime) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("end-to-end OSC context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.running = true
	runtime.mu.Unlock()
	signalEndToEndChange(runtime.stateChange)
	return nil
}

func (runtime *endToEndRuntime) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("end-to-end OSC close context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.running = false
	runtime.mu.Unlock()
	signalEndToEndChange(runtime.stateChange)
	return nil
}

func (runtime *endToEndRuntime) Events() <-chan osc.ControllerEvent { return runtime.events }

func (runtime *endToEndRuntime) AvatarChanges(context.Context) <-chan osc.AvatarChange {
	return runtime.changes
}

func (runtime *endToEndRuntime) ClearRuntime() {
	runtime.mu.Lock()
	runtime.revision++
	runtime.generation = 0
	runtime.catalog = nil
	runtime.source = nil
	runtime.mu.Unlock()
	signalEndToEndChange(runtime.stateChange)
}

func (runtime *endToEndRuntime) InstallCatalog(catalog *osc.Catalog) error {
	if catalog == nil {
		return osc.ErrRuntimeCatalog
	}
	if catalog.Generation == 0 {
		return osc.ErrRuntimeGeneration
	}
	runtime.mu.Lock()
	runtime.revision++
	runtime.generation = catalog.Generation
	runtime.catalog = catalog.Clone()
	runtime.source = nil
	runtime.mu.Unlock()
	signalEndToEndChange(runtime.stateChange)
	return nil
}

func (runtime *endToEndRuntime) Publish(generation uint64, source osc.ValueSource) error {
	if source == nil {
		return osc.ErrRuntimeCatalog
	}
	runtime.mu.Lock()
	if generation == 0 || runtime.generation == 0 || generation != runtime.generation {
		runtime.mu.Unlock()
		return osc.ErrRuntimeGeneration
	}
	sourceSnapshot := snapshotEndToEndSource(source)
	runtime.revision++
	runtime.source = sourceSnapshot
	runtime.publications = append(runtime.publications, endToEndPublication{
		revision:   runtime.revision,
		generation: generation,
		catalog:    runtime.catalog.Clone(),
		source:     sourceSnapshot,
	})
	runtime.mu.Unlock()
	signalEndToEndChange(runtime.stateChange)
	return nil
}

func (runtime *endToEndRuntime) Status() osc.OSCStatus {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return osc.OSCStatus{Running: runtime.running}
}

func (runtime *endToEndRuntime) offerAvatarChange(change osc.AvatarChange) {
	runtime.changes <- change
}

func (runtime *endToEndRuntime) snapshot() endToEndRuntimeState {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return endToEndRuntimeState{
		revision:   runtime.revision,
		generation: runtime.generation,
		catalog:    runtime.catalog.Clone(),
		source:     runtime.source,
	}
}

func (runtime *endToEndRuntime) publicationCount() int {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return len(runtime.publications)
}

func (runtime *endToEndRuntime) publicationsFrom(marker int) []endToEndPublication {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if marker < 0 {
		marker = 0
	}
	if marker >= len(runtime.publications) {
		return nil
	}
	publications := make([]endToEndPublication, len(runtime.publications)-marker)
	for index, publication := range runtime.publications[marker:] {
		publications[index] = publication
		publications[index].catalog = publication.catalog.Clone()
	}
	return publications
}

type endToEndTicker struct {
	ticks chan time.Time
}

func newEndToEndTicker() *endToEndTicker {
	return &endToEndTicker{ticks: make(chan time.Time, 1)}
}

func (ticker *endToEndTicker) C() <-chan time.Time { return ticker.ticks }
func (*endToEndTicker) Stop()                      {}

func (ticker *endToEndTicker) tick(t *testing.T, at time.Time) {
	t.Helper()
	select {
	case ticker.ticks <- at:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out offering end-to-end processing tick")
	}
}

type endToEndClock struct {
	mu      sync.Mutex
	current time.Time
}

func newEndToEndClock(current time.Time) *endToEndClock {
	return &endToEndClock{current: current}
}

func (clock *endToEndClock) now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.current
}

func (clock *endToEndClock) set(current time.Time) {
	clock.mu.Lock()
	clock.current = current
	clock.mu.Unlock()
}

func awaitEndToEndStatus(t *testing.T, app *Application, matches func(Status) bool) Status {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	updates := app.SubscribeStatus(ctx)
	for {
		select {
		case status, ok := <-updates:
			if !ok {
				t.Fatal("Application status subscription closed before expected state")
			}
			if matches(status) {
				return status
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for Application status; latest = %#v", app.Status())
		}
	}
}

func awaitEndToEndPlugin(t *testing.T, manager *endToEndManager, matches func(endToEndPluginState) bool) endToEndPluginState {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		state := manager.snapshot()
		if matches(state) {
			return state
		}
		select {
		case <-manager.changed:
		case <-timer.C:
			t.Fatalf("timed out waiting for plugin state; latest = %#v", state)
		}
	}
}

func awaitEndToEndRuntime(t *testing.T, runtime *endToEndRuntime, matches func(endToEndRuntimeState) bool) endToEndRuntimeState {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		state := runtime.snapshot()
		if matches(state) {
			return state
		}
		select {
		case <-runtime.stateChange:
		case <-timer.C:
			t.Fatalf("timed out waiting for OSC runtime state; latest = %#v", state)
		}
	}
}

func assertEndToEndSubscription(t *testing.T, got, want pluginapi.Subscription) {
	t.Helper()
	if got != want {
		t.Fatalf("plugin subscription = %#v, want %#v", got, want)
	}
}

func assertEndToEndCatalogIDs(t *testing.T, catalog *osc.Catalog, want ...parameters.ParameterID) {
	t.Helper()
	if catalog == nil {
		t.Fatal("OSC catalog = nil")
	}
	if len(catalog.Bindings) != len(want) {
		t.Fatalf("OSC catalog binding count = %d, want %d; bindings = %#v", len(catalog.Bindings), len(want), catalog.Bindings)
	}
	wanted := make(map[parameters.ParameterID]struct{}, len(want))
	for _, id := range want {
		wanted[id] = struct{}{}
		if _, ok := catalog.Bindings[id]; !ok {
			t.Errorf("OSC catalog missing parameter ID %d", id)
		}
	}
	for id := range catalog.Bindings {
		if _, ok := wanted[id]; !ok {
			t.Errorf("OSC catalog contains unexpected parameter ID %d", id)
		}
	}
}

func assertEndToEndFloat(t *testing.T, source osc.ValueSource, id parameters.ParameterID, want float32, wantValid bool) {
	t.Helper()
	var got float32
	var valid bool
	if source != nil {
		got, valid = source.Float(id)
	}
	if got != want || valid != wantValid {
		t.Fatalf("OSC source Float(%d) = %v,%t, want %v,%t", id, got, valid, want, wantValid)
	}
}

func assertEndToEndBool(t *testing.T, source osc.ValueSource, id parameters.ParameterID, want, wantValid bool) {
	t.Helper()
	var got bool
	var valid bool
	if source != nil {
		got, valid = source.Bool(id)
	}
	if got != want || valid != wantValid {
		t.Fatalf("OSC source Bool(%d) = %t,%t, want %t,%t", id, got, valid, want, wantValid)
	}
}

func endToEndFloatEquals(source osc.ValueSource, id parameters.ParameterID, want float32) bool {
	if source == nil {
		return false
	}
	got, ok := source.Float(id)
	return ok && got == want
}

func endToEndBoolEquals(source osc.ValueSource, id parameters.ParameterID, want bool) bool {
	if source == nil {
		return false
	}
	got, ok := source.Bool(id)
	return ok && got == want
}

func signalEndToEndChange(changed chan struct{}) {
	select {
	case changed <- struct{}{}:
	default:
	}
}

var _ applicationPluginManager = (*endToEndManager)(nil)
var _ applicationOSC = (*endToEndRuntime)(nil)
var _ applicationTicker = (*endToEndTicker)(nil)
