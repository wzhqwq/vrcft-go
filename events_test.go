package main

import (
	"context"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/userconfig"
)

type emittedEvent struct {
	name   string
	values []any
}

type recordingEventEmitter struct {
	calls chan emittedEvent
}

func (emitter *recordingEventEmitter) Emit(_ context.Context, name string, values ...any) {
	owned := append([]any(nil), values...)
	emitter.calls <- emittedEvent{name: name, values: owned}
}

type blockingEventEmitter struct {
	calls        chan emittedEvent
	entered      chan struct{}
	release      chan struct{}
	canceled     chan struct{}
	beforeReturn func()
	once         sync.Once
}

func (emitter *blockingEventEmitter) Emit(ctx context.Context, name string, values ...any) {
	emitter.calls <- emittedEvent{name: name, values: append([]any(nil), values...)}
	emitter.once.Do(func() {
		close(emitter.entered)
		if emitter.canceled == nil {
			<-emitter.release
		} else {
			select {
			case <-emitter.release:
			case <-ctx.Done():
				close(emitter.canceled)
				<-emitter.release
			}
		}
		if emitter.beforeReturn != nil {
			emitter.beforeReturn()
		}
	})
}

func TestEventBridgeUsesExactVersionedNamesAndCompleteModuleResponses(t *testing.T) {
	if eventRuntimeStatus != "vrcft:v1:runtime-status" || eventPluginsChanged != "vrcft:v1:plugins-changed" || eventSettingsChanged != "vrcft:v1:settings-changed" {
		t.Fatalf("event names = %q, %q, %q", eventRuntimeStatus, eventPluginsChanged, eventSettingsChanged)
	}

	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	runtimeAPI := newRuntimeAPI(true, func() time.Time { return now })
	pluginsAPI := newPluginsAPI(func() time.Time { return now })
	settingsAPI := newSettingsAPI(nil, userconfig.Candidate{}, func() time.Time { return now })
	t.Cleanup(pluginsAPI.close)
	t.Cleanup(settingsAPI.close)
	emitter := &recordingEventEmitter{calls: make(chan emittedEvent, 6)}

	forwarders := startEventForwarders(context.Background(), emitter, runtimeAPI, pluginsAPI, settingsAPI)
	t.Cleanup(forwarders.stop)

	got := make(map[string]emittedEvent)
	for len(got) < 3 {
		select {
		case event := <-emitter.calls:
			got[event.name] = event
		case <-time.After(time.Second):
			t.Fatalf("initial event names = %v, want all three modules", reflect.ValueOf(got).MapKeys())
		}
	}
	assertSingleEventValue(t, got[eventRuntimeStatus], runtimeAPI.GetStatus())
	assertSingleEventValue(t, got[eventPluginsChanged], pluginsAPI.List())
	assertSingleEventValue(t, got[eventSettingsChanged], settingsAPI.Get())
}

func TestEventBridgeBlockedEmitterSendsFirstInflightThenNewestPending(t *testing.T) {
	runtimeAPI := newRuntimeAPI(true, time.Now)
	emitter := &blockingEventEmitter{
		calls:   make(chan emittedEvent, 4),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	forwarders := startEventForwarders(context.Background(), emitter, runtimeAPI, nil, nil)
	t.Cleanup(forwarders.stop)

	select {
	case <-emitter.entered:
	case <-time.After(time.Second):
		t.Fatal("runtime emitter did not receive initial response")
	}
	runtimeAPI.setPhase(runtimePhaseStarting)
	runtimeAPI.setPhase(runtimePhaseRunning)
	runtimeAPI.setPhase(runtimePhaseDiagnostic)
	close(emitter.release)

	first := receiveEmittedEvent(t, emitter.calls)
	latest := receiveEmittedEvent(t, emitter.calls)
	if first.name != eventRuntimeStatus || first.values[0].(RuntimeResponse).Phase != "created" {
		t.Fatalf("first in-flight event = %+v", first)
	}
	if latest.name != eventRuntimeStatus || latest.values[0].(RuntimeResponse).Phase != "diagnostic" {
		t.Fatalf("newest pending event = %+v", latest)
	}
	select {
	case stale := <-emitter.calls:
		t.Fatalf("event bridge retained stale pending response %+v", stale)
	default:
	}
}

func TestEventBridgeBlockedPluginsEmitterSkipsIntermediatePendingSnapshot(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousProcs)
	pluginsAPI := newPluginsAPI(time.Now)
	t.Cleanup(pluginsAPI.close)
	emitter := &blockingEventEmitter{
		calls:   make(chan emittedEvent, 4),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	forwarders := startEventForwarders(context.Background(), emitter, nil, pluginsAPI, nil)
	t.Cleanup(forwarders.stop)
	waitEmitterEntered(t, emitter.entered)

	pluginsAPI.store.update([]PluginDTO{{ID: "intermediate"}}, nil)
	yieldEventPipeline()
	emitter.beforeReturn = func() {
		pluginsAPI.store.update([]PluginDTO{{ID: "latest"}}, nil)
	}
	close(emitter.release)

	first := receiveEmittedEvent(t, emitter.calls)
	latest := receiveEmittedEvent(t, emitter.calls)
	if got := first.values[0].(PluginListResponse); len(got.Plugins) != 0 {
		t.Fatalf("first plugin event = %+v, want initial empty list", got)
	}
	if got := latest.values[0].(PluginListResponse); len(got.Plugins) != 1 || got.Plugins[0].ID != "latest" {
		t.Fatalf("plugin event after blocked burst = %+v, want only latest", got)
	}
}

func TestEventBridgeBlockedSettingsEmitterSkipsIntermediatePendingSnapshot(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousProcs)
	settingsAPI := newSettingsAPI(nil, userconfig.Candidate{}, time.Now)
	t.Cleanup(settingsAPI.close)
	emitter := &blockingEventEmitter{
		calls:   make(chan emittedEvent, 4),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	forwarders := startEventForwarders(context.Background(), emitter, nil, nil, settingsAPI)
	t.Cleanup(forwarders.stop)
	waitEmitterEntered(t, emitter.entered)

	settingsAPI.store.update(settingsSnapshot{fileRevision: 2, settings: userconfig.Candidate{Avatar: userconfig.Avatar{OSCRoot: "C:/intermediate"}}}, nil)
	yieldEventPipeline()
	emitter.beforeReturn = func() {
		settingsAPI.store.update(settingsSnapshot{fileRevision: 3, settings: userconfig.Candidate{Avatar: userconfig.Avatar{OSCRoot: "C:/latest"}}}, nil)
	}
	close(emitter.release)

	first := receiveEmittedEvent(t, emitter.calls)
	latest := receiveEmittedEvent(t, emitter.calls)
	if got := first.values[0].(SettingsResponse); got.FileRevision != 0 {
		t.Fatalf("first settings event = %+v, want initial revision zero", got)
	}
	if got := latest.values[0].(SettingsResponse); got.FileRevision != 3 || got.Settings.Avatar.OSCRoot != "C:/latest" {
		t.Fatalf("settings event after blocked burst = %+v, want only latest", got)
	}
}

func TestEventBridgeStopJoinsBlockedEmitAndSuppressesPendingAndFutureEvents(t *testing.T) {
	runtimeAPI := newRuntimeAPI(true, time.Now)
	emitter := &blockingEventEmitter{
		calls:    make(chan emittedEvent, 4),
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	forwarders := startEventForwarders(context.Background(), emitter, runtimeAPI, nil, nil)
	select {
	case <-emitter.entered:
	case <-time.After(time.Second):
		t.Fatal("runtime emitter did not block")
	}
	runtimeAPI.setPhase(runtimePhaseRunning)

	stopped := make(chan struct{})
	go func() {
		forwarders.stop()
		close(stopped)
	}()
	select {
	case <-emitter.canceled:
	case <-time.After(time.Second):
		t.Fatal("blocked emitter did not observe event bridge cancellation")
	}
	select {
	case <-stopped:
		t.Fatal("event bridge stop returned while synchronous emitter remained blocked")
	default:
	}
	close(emitter.release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("event bridge stop did not join after emitter returned")
	}

	_ = receiveEmittedEvent(t, emitter.calls)
	select {
	case event := <-emitter.calls:
		t.Fatalf("event emitted after cancellation: %+v", event)
	default:
	}
	runtimeAPI.setPhase(runtimePhaseClosed)
	select {
	case event := <-emitter.calls:
		t.Fatalf("future event emitted after joined stop: %+v", event)
	default:
	}

	secondStop := make(chan struct{})
	go func() {
		forwarders.stop()
		close(secondStop)
	}()
	select {
	case <-secondStop:
	case <-time.After(time.Second):
		t.Fatal("repeated event bridge stop did not return")
	}
}

func TestEventBridgeParentCancellationStopsAllForwarders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runtimeAPI := newRuntimeAPI(true, time.Now)
	pluginsAPI := newPluginsAPI(time.Now)
	settingsAPI := newSettingsAPI(nil, userconfig.Candidate{}, time.Now)
	t.Cleanup(pluginsAPI.close)
	t.Cleanup(settingsAPI.close)
	emitter := &recordingEventEmitter{calls: make(chan emittedEvent, 12)}
	forwarders := startEventForwarders(ctx, emitter, runtimeAPI, pluginsAPI, settingsAPI)

	for range 3 {
		_ = receiveEmittedEvent(t, emitter.calls)
	}
	cancel()
	forwarders.stop()
	runtimeAPI.setPhase(runtimePhaseClosed)
	select {
	case event := <-emitter.calls:
		t.Fatalf("event emitted after parent cancellation joined: %+v", event)
	default:
	}
}

func TestEventBridgeClosedSourceStopsWorkerWithoutEmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := make(chan int)
	close(source)
	emitter := &recordingEventEmitter{calls: make(chan emittedEvent, 1)}
	var workers sync.WaitGroup
	startEventForwarder(ctx, &workers, emitter, eventRuntimeStatus, source, func(value int) any { return value })
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event worker did not stop after its source closed")
	}
	select {
	case event := <-emitter.calls:
		t.Fatalf("closed source emitted event: %+v", event)
	default:
	}
}

func TestEventBridgeConcurrentRepeatedStopIsSafe(t *testing.T) {
	runtimeAPI := newRuntimeAPI(true, time.Now)
	emitter := &recordingEventEmitter{calls: make(chan emittedEvent, 2)}
	forwarders := startEventForwarders(context.Background(), emitter, runtimeAPI, nil, nil)
	_ = receiveEmittedEvent(t, emitter.calls)

	const callers = 16
	var stopped sync.WaitGroup
	stopped.Add(callers)
	for range callers {
		go func() {
			defer stopped.Done()
			forwarders.stop()
		}()
	}
	done := make(chan struct{})
	go func() {
		stopped.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("concurrent repeated event bridge stops did not return")
	}

	runtimeAPI.setPhase(runtimePhaseClosed)
	select {
	case event := <-emitter.calls:
		t.Fatalf("event emitted after concurrent stops joined: %+v", event)
	default:
	}
}

func TestEventBridgeStopJoinsModuleSubscriptionAdapters(t *testing.T) {
	runtimeAPI := newRuntimeAPI(true, time.Now)
	emitter := &recordingEventEmitter{calls: make(chan emittedEvent, 2)}
	forwarders := startEventForwarders(context.Background(), emitter, runtimeAPI, nil, nil)
	_ = receiveEmittedEvent(t, emitter.calls)

	runtimeAPI.store.mu.Lock()
	stopped := make(chan struct{})
	go func() {
		forwarders.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		runtimeAPI.store.mu.Unlock()
		t.Fatal("event bridge stop returned before its owned module source cleanup exited")
	case <-time.After(20 * time.Millisecond):
	}
	runtimeAPI.store.mu.Unlock()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("event bridge stop did not join module subscription after cleanup unblocked")
	}
}

func assertSingleEventValue(t *testing.T, event emittedEvent, want any) {
	t.Helper()
	if len(event.values) != 1 || !reflect.DeepEqual(event.values[0], want) {
		t.Fatalf("event %q values = %#v, want %#v", event.name, event.values, want)
	}
}

func receiveEmittedEvent(t *testing.T, events <-chan emittedEvent) emittedEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for emitted event")
		return emittedEvent{}
	}
}

func waitEmitterEntered(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("event emitter was not entered")
	}
}

func yieldEventPipeline() {
	for range 100 {
		runtime.Gosched()
	}
}
