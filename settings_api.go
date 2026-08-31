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

	gate      chan struct{}
	loaded    userconfig.Loaded
	hasLoaded bool
	closed    atomic.Bool
	lifecycle context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once

	subscriptionsMu sync.Mutex
	subscriptions   map[uint64]context.CancelFunc
	nextSubscriber  uint64
	subscriptionsWG sync.WaitGroup
}

func newSettingsAPI(backend settingsBackend, defaults userconfig.Candidate, now func() time.Time) *SettingsAPI {
	lifecycle, cancel := context.WithCancel(context.Background())
	api := &SettingsAPI{
		backend: backend,
		store: newModuleStore(settingsSnapshot{
			settings: defaults.Clone(),
		}, cloneSettingsSnapshot, now),
		lifecycle:     lifecycle,
		cancel:        cancel,
		done:          make(chan struct{}),
		subscriptions: make(map[uint64]context.CancelFunc),
	}
	api.gate = make(chan struct{}, 1)
	api.gate <- struct{}{}
	return api
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
	if err := userconfig.ValidateCandidateBounds(candidate); err != nil {
		envelope := api.store.snapshot()
		problem := sanitizeProblem(err, envelope.Revision)
		return settingsValidationResponse(envelope, envelope.Value.settings, &problem)
	}
	if err := api.admit(context.Background()); err != nil {
		envelope := api.store.snapshot()
		return settingsValidationResponse(envelope, envelope.Value.settings, unavailableSettingsProblem(envelope.Revision))
	}
	defer api.releaseAdmission()
	envelope := api.store.snapshot()
	if api.backend == nil {
		return settingsValidationResponse(envelope, envelope.Value.settings, unavailableSettingsProblem(envelope.Revision))
	}
	normalized, err := api.backend.Validate(candidate.Clone())
	if api.closed.Load() {
		return settingsValidationResponse(envelope, envelope.Value.settings, unavailableSettingsProblem(envelope.Revision))
	}
	if err != nil {
		problem := sanitizeProblem(err, envelope.Revision)
		return settingsValidationResponse(envelope, envelope.Value.settings, &problem)
	}
	if err := userconfig.ValidateCandidateBounds(normalized); err != nil {
		problem := sanitizeProblem(err, envelope.Revision)
		return settingsValidationResponse(envelope, envelope.Value.settings, &problem)
	}
	return settingsValidationResponse(envelope, normalized, nil)
}

// Save persists a normalized candidate for the next process start. It never
// reconstructs or mutates the currently running Application.
func (api *SettingsAPI) Save(expectedRevision uint64, candidate userconfig.Candidate) SettingsSaveResponse {
	if err := userconfig.ValidateCandidateBounds(candidate); err != nil {
		envelope := api.store.snapshot()
		problem := sanitizeProblem(err, envelope.Revision)
		return settingsSaveResponse(envelope, false, &problem)
	}
	if err := api.admit(context.Background()); err != nil {
		envelope := api.store.snapshot()
		return settingsSaveResponse(envelope, false, unavailableSettingsProblem(envelope.Revision))
	}
	defer api.releaseAdmission()
	envelope := api.store.snapshot()
	if api.backend == nil || !api.hasLoaded {
		return settingsSaveResponse(envelope, false, unavailableSettingsProblem(envelope.Revision))
	}
	if expectedRevision != envelope.Revision {
		problem := sanitizeProblem(userconfig.ErrConflict, envelope.Revision)
		return settingsSaveResponse(envelope, false, &problem)
	}

	ctx, cancel := api.operationContext(context.Background())
	defer cancel()
	result, err := api.backend.Save(ctx, cloneSettingsLoadedForAPI(api.loaded), candidate.Clone())
	if err != nil {
		if api.closed.Load() || ctx.Err() != nil {
			return settingsSaveResponse(envelope, false, unavailableSettingsProblem(envelope.Revision))
		}
		problem := sanitizeProblem(err, envelope.Revision)
		return settingsSaveResponse(envelope, false, &problem)
	}
	if err := validateSettingsLoadedBounds(result.Loaded); err != nil {
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
	if err := api.admit(ctx); err != nil {
		return userconfig.Loaded{}, err
	}
	defer api.releaseAdmission()
	if api.backend == nil {
		return userconfig.Loaded{}, errors.New("settings API is unavailable")
	}
	operationCtx, cancel := api.operationContext(ctx)
	defer cancel()
	loaded, err := api.backend.LoadOrCreate(operationCtx)
	if err := operationCtx.Err(); err != nil {
		return userconfig.Loaded{}, err
	}
	if err == nil {
		err = validateSettingsLoadedBounds(loaded)
	}
	if err != nil {
		envelope := api.store.snapshot()
		problem := sanitizeProblem(err, nextModuleRevision(envelope.Revision))
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
		mapped := sanitizeProblem(loaded.Diagnostic, nextModuleRevision(envelope.Revision))
		problem = &mapped
	}
	api.store.update(settingsSnapshotFromLoaded(loaded), problem)
	return cloneSettingsLoadedForAPI(loaded), nil
}

// subscribe exposes an owned, capacity-one latest-only stream for the root
// event forwarder. It is deliberately unexported and never Wails-bound.
func (api *SettingsAPI) subscribe(ctx context.Context) <-chan SettingsResponse {
	updates := make(chan SettingsResponse, 1)
	if ctx == nil {
		ctx = context.Background()
	}
	subscriptionCtx, cancel := context.WithCancel(ctx)
	api.subscriptionsMu.Lock()
	if api.closed.Load() {
		api.subscriptionsMu.Unlock()
		cancel()
		close(updates)
		return updates
	}
	api.nextSubscriber++
	id := api.nextSubscriber
	api.subscriptions[id] = cancel
	api.subscriptionsWG.Add(1)
	api.subscriptionsMu.Unlock()
	source := api.store.subscribe(subscriptionCtx)
	go func() {
		defer cancel()
		defer api.removeSubscription(id)
		defer api.subscriptionsWG.Done()
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
	api.closeOnce.Do(func() {
		api.closed.Store(true)
		api.cancel()
		// Save may already have durably replaced the file when cancellation
		// arrives. Acquiring the gate waits for that admitted operation to
		// reconcile its private token and (when changed) its one publication.
		<-api.gate
		api.subscriptionsMu.Lock()
		close(api.done)
		for _, cancel := range api.subscriptions {
			cancel()
		}
		api.subscriptionsMu.Unlock()
		api.subscriptionsWG.Wait()
	})
}

func (api *SettingsAPI) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(api.lifecycle, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (api *SettingsAPI) admit(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	if api.closed.Load() {
		return errors.New("settings API is unavailable")
	}
	select {
	case <-parent.Done():
		return parent.Err()
	case <-api.lifecycle.Done():
		return errors.New("settings API is unavailable")
	case <-api.gate:
	}
	if api.closed.Load() {
		api.releaseAdmission()
		return errors.New("settings API is unavailable")
	}
	if err := parent.Err(); err != nil {
		api.releaseAdmission()
		return err
	}
	return nil
}

func (api *SettingsAPI) releaseAdmission() { api.gate <- struct{}{} }

func (api *SettingsAPI) removeSubscription(id uint64) {
	api.subscriptionsMu.Lock()
	delete(api.subscriptions, id)
	api.subscriptionsMu.Unlock()
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

func validateSettingsLoadedBounds(loaded userconfig.Loaded) error {
	if loaded.Settings == nil {
		return userconfig.ValidateCandidateBounds(loaded.Defaults)
	}
	return userconfig.ValidateCandidateBounds(userconfig.Candidate{
		Avatar:     loaded.Settings.Avatar,
		Plugins:    loaded.Settings.Plugins,
		Processing: loaded.Settings.Processing,
		OSC:        loaded.Settings.OSC,
	})
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
		Settings:     boundedSettingsCandidate(envelope.Value.settings),
		Problem:      cloneProblem(problem),
	}
}

func settingsValidationResponse(envelope moduleEnvelope[settingsSnapshot], settings userconfig.Candidate, problem *Problem) SettingsValidationResponse {
	return SettingsValidationResponse{
		Revision:  envelope.Revision,
		UpdatedAt: envelope.UpdatedAt,
		Settings:  boundedSettingsCandidate(settings),
		Problem:   cloneProblem(problem),
	}
}

func settingsSaveResponse(envelope moduleEnvelope[settingsSnapshot], restartRequired bool, problem *Problem) SettingsSaveResponse {
	return SettingsSaveResponse{
		Revision:        envelope.Revision,
		UpdatedAt:       envelope.UpdatedAt,
		FileRevision:    envelope.Value.fileRevision,
		Settings:        boundedSettingsCandidate(envelope.Value.settings),
		RestartRequired: restartRequired,
		Problem:         cloneProblem(problem),
	}
}

// boundedSettingsCandidate is a final response guard for snapshots supplied
// by an injected backend. Caller candidates are admitted before cloning; this
// guard also keeps a faulty backend from making a Wails response unbounded.
func boundedSettingsCandidate(candidate userconfig.Candidate) userconfig.Candidate {
	if err := userconfig.ValidateCandidateBounds(candidate); err != nil {
		return userconfig.Candidate{}
	}
	return candidate.Clone()
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
