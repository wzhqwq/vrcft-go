package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
)

// settingsBackend is the narrow persistence boundary used by SettingsAPI. It
// keeps the Wails-facing API independent from Store construction and file I/O.
type settingsBackend interface {
	LoadOrCreate(context.Context) (userconfig.Loaded, error)
	Validate(userconfig.Candidate) (userconfig.Candidate, error)
	Save(context.Context, userconfig.Loaded, userconfig.Candidate) (userconfig.SaveResult, error)
}

type settingsSnapshot struct {
	fileRevision uint64
	settings     userconfig.Candidate
}

// SettingsAPI is the Wails-safe settings surface. Persisted file revisions and
// module revisions intentionally use separate counters.
type SettingsAPI struct {
	backend settingsBackend
	store   *moduleStore[settingsSnapshot]

	admission sync.Mutex
	loaded    userconfig.Loaded
	hasLoaded bool
	closed    atomic.Bool
	done      chan struct{}
	closeOnce sync.Once
}

func newSettingsAPI(backend settingsBackend, defaults userconfig.Candidate, now func() time.Time) *SettingsAPI {
	return &SettingsAPI{
		backend: backend,
		store: newModuleStore(settingsSnapshot{
			settings: defaults.Clone(),
		}, cloneSettingsSnapshot, now),
		done: make(chan struct{}),
	}
}

// Get returns the most recently loaded authoritative settings snapshot.
func (api *SettingsAPI) Get() SettingsResponse {
	envelope := api.store.snapshot()
	if api.closed.Load() {
		return settingsResponse(envelope, unavailableSettingsProblem(envelope.Revision))
	}
	return settingsResponse(envelope, envelope.Problem)
}

// Validate normalizes a candidate without changing the loaded token, file, or
// module snapshot.
func (api *SettingsAPI) Validate(candidate userconfig.Candidate) SettingsValidationResponse {
	api.admission.Lock()
	defer api.admission.Unlock()

	envelope := api.store.snapshot()
	if api.closed.Load() || api.backend == nil {
		return settingsValidationResponse(envelope, candidate, unavailableSettingsProblem(envelope.Revision))
	}
	normalized, err := api.backend.Validate(candidate.Clone())
	if err != nil {
		problem := sanitizeProblem(err, envelope.Revision)
		return settingsValidationResponse(envelope, candidate, &problem)
	}
	return settingsValidationResponse(envelope, normalized, nil)
}

// Save persists a normalized candidate for the next process start. It never
// reconstructs or mutates the currently running Application.
func (api *SettingsAPI) Save(expectedRevision uint64, candidate userconfig.Candidate) SettingsSaveResponse {
	api.admission.Lock()
	defer api.admission.Unlock()

	envelope := api.store.snapshot()
	if api.closed.Load() || api.backend == nil || !api.hasLoaded {
		return settingsSaveResponse(envelope, false, unavailableSettingsProblem(envelope.Revision))
	}
	if expectedRevision != envelope.Revision {
		problem := sanitizeProblem(userconfig.ErrConflict, envelope.Revision)
		return settingsSaveResponse(envelope, false, &problem)
	}

	result, err := api.backend.Save(context.Background(), cloneSettingsLoadedForAPI(api.loaded), candidate.Clone())
	if err != nil {
		problem := sanitizeProblem(err, envelope.Revision)
		return settingsSaveResponse(envelope, false, &problem)
	}

	api.loaded = cloneSettingsLoadedForAPI(result.Loaded)
	api.hasLoaded = true
	if !result.Changed {
		return settingsSaveResponse(envelope, false, nil)
	}

	updated := api.store.update(settingsSnapshotFromLoaded(result.Loaded), nil)
	return settingsSaveResponse(updated, true, nil)
}

// loadForStartup loads the settings document and publishes the initial loaded
// state for Get and later repair. The returned Loaded value is independently
// owned so root lifecycle code can safely convert it to application.Config.
func (api *SettingsAPI) loadForStartup(ctx context.Context) (userconfig.Loaded, error) {
	api.admission.Lock()
	defer api.admission.Unlock()

	if api.closed.Load() || api.backend == nil {
		return userconfig.Loaded{}, errors.New("settings API is unavailable")
	}
	loaded, err := api.backend.LoadOrCreate(ctx)
	if err != nil {
		envelope := api.store.snapshot()
		problem := sanitizeProblem(err, envelope.Revision)
		api.store.update(envelope.Value, &problem)
		return userconfig.Loaded{}, err
	}

	api.loaded = cloneSettingsLoadedForAPI(loaded)
	api.hasLoaded = true
	envelope := api.store.snapshot()
	var problem *Problem
	if loaded.Invalid {
		if loaded.Diagnostic == nil {
			loaded.Diagnostic = errors.New("settings file is invalid")
		}
		mapped := sanitizeProblem(loaded.Diagnostic, envelope.Revision)
		problem = &mapped
	}
	api.store.update(settingsSnapshotFromLoaded(loaded), problem)
	return cloneSettingsLoadedForAPI(loaded), nil
}

// subscribe exposes an owned, capacity-one latest-only stream for the root
// event forwarder. It is deliberately unexported and never Wails-bound.
func (api *SettingsAPI) subscribe(ctx context.Context) <-chan SettingsResponse {
	updates := make(chan SettingsResponse, 1)
	if api.closed.Load() {
		close(updates)
		return updates
	}
	if ctx == nil {
		ctx = context.Background()
	}
	subscriptionCtx, cancel := context.WithCancel(ctx)
	source := api.store.subscribe(subscriptionCtx)
	go func() {
		defer cancel()
		defer close(updates)
		for {
			select {
			case <-api.done:
				return
			case <-subscriptionCtx.Done():
				return
			case envelope, ok := <-source:
				if !ok {
					return
				}
				offerSettingsResponse(updates, settingsResponse(envelope, envelope.Problem))
			}
		}
	}()
	return updates
}

// close rejects future validation and save admissions and stops root event
// subscriptions. It does not close the Store because root owns that lifetime.
func (api *SettingsAPI) close() {
	api.admission.Lock()
	api.closed.Store(true)
	api.admission.Unlock()
	api.closeOnce.Do(func() { close(api.done) })
}

func settingsSnapshotFromLoaded(loaded userconfig.Loaded) settingsSnapshot {
	if loaded.Settings == nil {
		return settingsSnapshot{settings: loaded.Defaults.Clone()}
	}
	return settingsSnapshot{
		fileRevision: loaded.Settings.Revision,
		settings:     candidateFromLoadedSettings(*loaded.Settings),
	}
}

func candidateFromLoadedSettings(settings userconfig.Settings) userconfig.Candidate {
	return userconfig.Candidate{
		Avatar:     settings.Avatar,
		Plugins:    settings.Plugins,
		Processing: settings.Processing,
		OSC:        settings.OSC,
	}.Clone()
}

func cloneSettingsSnapshot(value settingsSnapshot) settingsSnapshot {
	return settingsSnapshot{fileRevision: value.fileRevision, settings: value.settings.Clone()}
}

func cloneSettingsLoadedForAPI(value userconfig.Loaded) userconfig.Loaded {
	clone := value
	clone.Defaults = value.Defaults.Clone()
	if value.Settings != nil {
		settings := value.Settings.Clone()
		clone.Settings = &settings
	}
	return clone
}

func settingsResponse(envelope moduleEnvelope[settingsSnapshot], problem *Problem) SettingsResponse {
	return SettingsResponse{
		Revision:     envelope.Revision,
		UpdatedAt:    envelope.UpdatedAt,
		FileRevision: envelope.Value.fileRevision,
		Settings:     envelope.Value.settings.Clone(),
		Problem:      cloneProblem(problem),
	}
}

func settingsValidationResponse(envelope moduleEnvelope[settingsSnapshot], settings userconfig.Candidate, problem *Problem) SettingsValidationResponse {
	return SettingsValidationResponse{
		Revision:  envelope.Revision,
		UpdatedAt: envelope.UpdatedAt,
		Settings:  settings.Clone(),
		Problem:   cloneProblem(problem),
	}
}

func settingsSaveResponse(envelope moduleEnvelope[settingsSnapshot], restartRequired bool, problem *Problem) SettingsSaveResponse {
	return SettingsSaveResponse{
		Revision:        envelope.Revision,
		UpdatedAt:       envelope.UpdatedAt,
		FileRevision:    envelope.Value.fileRevision,
		Settings:        envelope.Value.settings.Clone(),
		RestartRequired: restartRequired,
		Problem:         cloneProblem(problem),
	}
}

func unavailableSettingsProblem(currentRevision uint64) *Problem {
	problem := sanitizeProblem(application.ErrInvalidLifecycle, currentRevision)
	return &problem
}

func offerSettingsResponse(out chan SettingsResponse, response SettingsResponse) {
	select {
	case <-out:
	default:
	}
	select {
	case out <- response:
	default:
	}
}
