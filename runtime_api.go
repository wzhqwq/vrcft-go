package main

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/avatar"
)

type runtimePhase string

const (
	runtimePhaseCreated    runtimePhase = "created"
	runtimePhaseStarting   runtimePhase = "starting"
	runtimePhaseRunning    runtimePhase = "running"
	runtimePhaseDiagnostic runtimePhase = "diagnostic"
	runtimePhaseClosing    runtimePhase = "closing"
	runtimePhaseClosed     runtimePhase = "closed"
)

const maxRuntimePluginFailures = 64

type runtimeSnapshot struct {
	phase             runtimePhase
	platformSupported bool
	application       *RuntimeApplicationDTO
}

// RuntimeAPI owns the Wails-safe runtime module snapshot. Root lifecycle
// setters and Application consumers stay unexported so GetStatus is the only
// bound method.
type RuntimeAPI struct {
	mu          sync.Mutex
	store       *moduleStore[runtimeSnapshot]
	subscribers map[chan RuntimeResponse]struct{}
}

func newRuntimeAPI(platformSupported bool, now func() time.Time) *RuntimeAPI {
	return &RuntimeAPI{
		store: newModuleStore(runtimeSnapshot{
			phase:             runtimePhaseCreated,
			platformSupported: platformSupported,
		}, cloneRuntimeSnapshot, now),
		subscribers: make(map[chan RuntimeResponse]struct{}),
	}
}

// GetStatus returns the latest complete owned runtime snapshot.
func (api *RuntimeAPI) GetStatus() RuntimeResponse {
	return runtimeResponse(api.store.snapshot())
}

func (api *RuntimeAPI) setPhase(phase runtimePhase) {
	api.mu.Lock()
	defer api.mu.Unlock()
	envelope := api.store.snapshot()
	next := cloneRuntimeSnapshot(envelope.Value)
	next.phase = phase
	api.updateLocked(envelope, next, envelope.Problem)
}

func (api *RuntimeAPI) setProblem(problem *Problem) {
	api.mu.Lock()
	defer api.mu.Unlock()
	envelope := api.store.snapshot()
	api.updateLocked(envelope, envelope.Value, problem)
}

func (api *RuntimeAPI) setRootState(phase runtimePhase, problem *Problem) {
	api.mu.Lock()
	defer api.mu.Unlock()
	envelope := api.store.snapshot()
	next := cloneRuntimeSnapshot(envelope.Value)
	next.phase = phase
	api.updateLocked(envelope, next, problem)
}

func (api *RuntimeAPI) setApplicationStatus(status application.Status) {
	api.mu.Lock()
	defer api.mu.Unlock()
	envelope := api.store.snapshot()
	next := cloneRuntimeSnapshot(envelope.Value)
	converted := runtimeApplicationDTO(status)
	next.application = &converted
	api.updateLocked(envelope, next, envelope.Problem)
}

func (api *RuntimeAPI) updateLocked(current moduleEnvelope[runtimeSnapshot], next runtimeSnapshot, problem *Problem) {
	if reflect.DeepEqual(current.Value, next) && reflect.DeepEqual(current.Problem, problem) {
		return
	}
	envelope := api.store.update(next, problem)
	for subscriber := range api.subscribers {
		offerRuntimeResponse(subscriber, runtimeResponse(envelope))
	}
}

func (api *RuntimeAPI) consumeStatus(ctx context.Context, updates <-chan application.Status) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case status, ok := <-updates:
			if !ok {
				return
			}
			if ctx.Err() == nil {
				api.setApplicationStatus(status)
			}
		}
	}
}

func (api *RuntimeAPI) subscribe(ctx context.Context) <-chan RuntimeResponse {
	if ctx == nil {
		ctx = context.Background()
	}
	updates := make(chan RuntimeResponse, 1)
	api.mu.Lock()
	api.subscribers[updates] = struct{}{}
	offerRuntimeResponse(updates, runtimeResponse(api.store.snapshot()))
	api.mu.Unlock()
	go func() {
		<-ctx.Done()
		api.mu.Lock()
		if _, ok := api.subscribers[updates]; ok {
			delete(api.subscribers, updates)
			close(updates)
		}
		api.mu.Unlock()
	}()
	return updates
}

func runtimeApplicationDTO(status application.Status) RuntimeApplicationDTO {
	result := RuntimeApplicationDTO{
		Lifecycle:           boundedMessage(string(status.Lifecycle)),
		AvatarID:            boundedMessage(status.AvatarID),
		PlanGeneration:      status.PlanGeneration,
		PlanStatus:          runtimePlanStatus(status.PlanStatus),
		PlanSource:          runtimePlanSource(status.PlanSource),
		ConfigPath:          boundedMessage(status.ConfigPath),
		ConfigID:            boundedMessage(status.ConfigID),
		GenerationExhausted: status.GenerationExhausted,
		OSC: RuntimeOSCDTO{
			Running:    status.OSC.Running,
			Connected:  status.OSC.Connected,
			HasTarget:  status.OSC.HasTarget,
			TargetMode: boundedMessage(string(status.OSC.TargetMode)),
			LastError:  boundedMessage(status.OSC.LastError),
		},
		PlanError:    boundedMessage(status.PlanError),
		RuntimeError: boundedMessage(status.RuntimeError),
	}
	if status.OSC.HasTarget {
		result.OSC.Target = OSCTargetDTO{Host: boundedMessage(status.OSC.Target.Host), Port: status.OSC.Target.Port}
	}
	failureCount := len(status.PluginFailures)
	if failureCount > maxRuntimePluginFailures {
		failureCount = maxRuntimePluginFailures
	}
	result.PluginFailures = make([]PluginControlFailureDTO, failureCount)
	for index := range failureCount {
		failure := status.PluginFailures[index]
		result.PluginFailures[index] = PluginControlFailureDTO{
			PluginID:  boundedMessage(failure.PluginID),
			Operation: boundedMessage(failure.Operation),
			Message:   boundedMessage(failure.Message),
		}
	}
	return result
}

func runtimePlanStatus(status avatar.Status) string {
	switch status {
	case avatar.StatusReady:
		return "ready"
	case avatar.StatusFailed:
		return "failed"
	case 0:
		return "none"
	default:
		return "unknown"
	}
}

func runtimePlanSource(source avatar.Source) string {
	switch source {
	case avatar.SourceAvatarConfig:
		return "avatar_config"
	case avatar.SourceFallback:
		return "fallback"
	case avatar.SourceNone, 0:
		return "none"
	default:
		return "unknown"
	}
}

func cloneRuntimeSnapshot(value runtimeSnapshot) runtimeSnapshot {
	if value.application != nil {
		application := cloneRuntimeApplicationDTO(*value.application)
		value.application = &application
	}
	return value
}

func cloneRuntimeApplicationDTO(value RuntimeApplicationDTO) RuntimeApplicationDTO {
	value.PluginFailures = append([]PluginControlFailureDTO(nil), value.PluginFailures...)
	return value
}

func runtimeResponse(envelope moduleEnvelope[runtimeSnapshot]) RuntimeResponse {
	response := RuntimeResponse{
		Revision:          envelope.Revision,
		UpdatedAt:         envelope.UpdatedAt,
		Phase:             string(envelope.Value.phase),
		PlatformSupported: envelope.Value.platformSupported,
		Problem:           cloneProblem(envelope.Problem),
	}
	if envelope.Value.application != nil {
		application := cloneRuntimeApplicationDTO(*envelope.Value.application)
		response.Application = &application
	}
	return response
}

func offerRuntimeResponse(out chan RuntimeResponse, response RuntimeResponse) {
	select {
	case <-out:
	default:
	}
	select {
	case out <- response:
	default:
	}
}
