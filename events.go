package main

import (
	"context"
	"reflect"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	eventRuntimeStatus      = "vrcft:v1:runtime-status"
	eventPluginsChanged     = "vrcft:v1:plugins-changed"
	eventSettingsChanged    = "vrcft:v1:settings-changed"
	pluginBroadcastInterval = time.Second
)

type eventEmitter interface {
	Emit(context.Context, string, ...any)
}

type wailsEmitter struct{}

func (wailsEmitter) Emit(ctx context.Context, name string, values ...any) {
	wailsruntime.EventsEmit(ctx, name, values...)
}

type eventForwarders struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// startEventForwarders is the explicit root-startup seam. API constructors do
// not call it, so passive root construction starts no forwarding goroutines.
func startEventForwarders(
	parent context.Context,
	emitter eventEmitter,
	runtimeAPI *RuntimeAPI,
	pluginsAPI *PluginsAPI,
	settingsAPI *SettingsAPI,
) *eventForwarders {
	if parent == nil {
		parent = context.Background()
	}
	if emitter == nil {
		emitter = wailsEmitter{}
	}
	ctx, cancel := context.WithCancel(parent)
	forwarders := &eventForwarders{cancel: cancel, done: make(chan struct{})}
	var workers sync.WaitGroup
	if runtimeAPI != nil {
		startEventForwarder(ctx, &workers, emitter, eventRuntimeStatus, runtimeAPI.store.subscribe(ctx), func(envelope moduleEnvelope[runtimeSnapshot]) any {
			return runtimeResponse(envelope)
		})
	}
	if pluginsAPI != nil {
		startPluginEventForwarder(ctx, &workers, emitter, pluginsAPI.store.subscribe(ctx), func(envelope moduleEnvelope[[]PluginDTO]) any {
			problem := envelope.Problem
			if unavailable := pluginsAPI.unavailableProblem(envelope.Revision); unavailable != nil {
				problem = unavailable
			}
			return pluginListResponse(envelope, problem)
		})
	}
	if settingsAPI != nil {
		startEventForwarder(ctx, &workers, emitter, eventSettingsChanged, settingsAPI.store.subscribe(ctx), func(envelope moduleEnvelope[settingsSnapshot]) any {
			problem := envelope.Problem
			if settingsAPI.closed.Load() {
				problem = unavailableSettingsProblem(envelope.Revision)
			}
			return settingsResponse(envelope, problem)
		})
	}
	go func() {
		workers.Wait()
		close(forwarders.done)
	}()
	return forwarders
}

func startPluginEventForwarder(ctx context.Context, workers *sync.WaitGroup, emitter eventEmitter, source <-chan moduleEnvelope[[]PluginDTO], response func(moduleEnvelope[[]PluginDTO]) any) {
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(pluginBroadcastInterval)
		defer ticker.Stop()
		var lastEmitted moduleEnvelope[[]PluginDTO]
		var pending moduleEnvelope[[]PluginDTO]
		hasPending := false
		emittedInitial := false
		for {
			select {
			case <-ctx.Done():
				drainEventSource(source)
				return
			case value, ok := <-source:
				if !ok || ctx.Err() != nil {
					if ok {
						drainEventSource(source)
					}
					return
				}
				if !emittedInitial {
					emitter.Emit(ctx, eventPluginsChanged, response(value))
					lastEmitted = value
					emittedInitial = true
					continue
				}
				if !pluginTelemetryOnlyChange(lastEmitted, value) {
					emitter.Emit(ctx, eventPluginsChanged, response(value))
					lastEmitted = value
					hasPending = false
					ticker.Reset(pluginBroadcastInterval)
					continue
				}
				pending = value
				hasPending = true
			case <-ticker.C:
				if !hasPending {
					continue
				}
				emitter.Emit(ctx, eventPluginsChanged, response(pending))
				lastEmitted = pending
				hasPending = false
			}
		}
	}()
}

func pluginTelemetryOnlyChange(previous, current moduleEnvelope[[]PluginDTO]) bool {
	if !reflect.DeepEqual(previous.Problem, current.Problem) || len(previous.Value) != len(current.Value) {
		return false
	}
	for index := range previous.Value {
		before := previous.Value[index]
		after := current.Value[index]
		before.FrameRate = 0
		before.LastFrameAt = nil
		after.FrameRate = 0
		after.LastFrameAt = nil
		if !reflect.DeepEqual(before, after) {
			return false
		}
	}
	return true
}

func startEventForwarder[T any](ctx context.Context, workers *sync.WaitGroup, emitter eventEmitter, name string, source <-chan T, response func(T) any) {
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			select {
			case <-ctx.Done():
				drainEventSource(source)
				return
			case value, ok := <-source:
				if !ok || ctx.Err() != nil {
					if ok {
						drainEventSource(source)
					}
					return
				}
				// Emit is intentionally synchronous: this bounds each module to
				// one in-flight value while its capacity-one source replaces the
				// pending value. Shutdown joins an in-flight Emit rather than
				// abandoning a goroutine; production EventsEmit is expected to
				// return, and a non-cooperative injected emitter violates this seam.
				emitter.Emit(ctx, name, response(value))
				if ctx.Err() != nil {
					drainEventSource(source)
					return
				}
			}
		}
	}()
}

func drainEventSource[T any](source <-chan T) {
	for range source {
	}
}

// stop cancels every source subscription and joins all forwarding workers.
// It is safe for repeated and concurrent callers.
func (forwarders *eventForwarders) stop() {
	if forwarders == nil {
		return
	}
	forwarders.cancel()
	<-forwarders.done
}
