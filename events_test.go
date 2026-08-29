package main

import (
	"context"
	"reflect"
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
	calls   chan emittedEvent
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (emitter *blockingEventEmitter) Emit(_ context.Context, name string, values ...any) {
	emitter.calls <- emittedEvent{name: name, values: append([]any(nil), values...)}
	emitter.once.Do(func() {
		close(emitter.entered)
		<-emitter.release
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
	case <-time.After(20 * time.Millisecond):
	}
}

func TestEventBridgeStopJoinsBlockedEmitAndSuppressesPendingAndFutureEvents(t *testing.T) {
	runtimeAPI := newRuntimeAPI(true, time.Now)
	emitter := &blockingEventEmitter{
		calls:   make(chan emittedEvent, 4),
		entered: make(chan struct{}),
		release: make(chan struct{}),
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
	case <-stopped:
		t.Fatal("event bridge stop returned while synchronous emitter remained blocked")
	case <-time.After(20 * time.Millisecond):
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
	case <-time.After(20 * time.Millisecond):
	}
	runtimeAPI.setPhase(runtimePhaseClosed)
	select {
	case event := <-emitter.calls:
		t.Fatalf("future event emitted after joined stop: %+v", event)
	case <-time.After(20 * time.Millisecond):
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
	case <-time.After(20 * time.Millisecond):
	}
}

func TestEventBridgeStopJoinsModuleSubscriptionAdapters(t *testing.T) {
	runtimeAPI := newRuntimeAPI(true, time.Now)
	emitter := &recordingEventEmitter{calls: make(chan emittedEvent, 2)}
	forwarders := startEventForwarders(context.Background(), emitter, runtimeAPI, nil, nil)
	_ = receiveEmittedEvent(t, emitter.calls)

	runtimeAPI.mu.Lock()
	stopped := make(chan struct{})
	go func() {
		forwarders.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		runtimeAPI.mu.Unlock()
		t.Fatal("event bridge stop returned before its module subscription adapter exited")
	case <-time.After(20 * time.Millisecond):
	}
	runtimeAPI.mu.Unlock()
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
