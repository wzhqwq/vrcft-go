package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/userconfig"
)

type fakeSettingsBackend struct {
	mu sync.Mutex

	loaded        userconfig.Loaded
	loadErr       error
	loadFn        func(context.Context) (userconfig.Loaded, error)
	validate      userconfig.Candidate
	validateErr   error
	saveResult    userconfig.SaveResult
	saveErr       error
	saveFn        func(context.Context, userconfig.Loaded, userconfig.Candidate) (userconfig.SaveResult, error)
	loadCalls     int
	validateCalls int
	saveCalls     int
}

func (backend *fakeSettingsBackend) LoadOrCreate(ctx context.Context) (userconfig.Loaded, error) {
	backend.mu.Lock()
	backend.loadCalls++
	load := backend.loadFn
	loaded, err := cloneSettingsLoaded(backend.loaded), backend.loadErr
	backend.mu.Unlock()
	if load != nil {
		return load(ctx)
	}
	return loaded, err
}

func (backend *fakeSettingsBackend) Validate(candidate userconfig.Candidate) (userconfig.Candidate, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.validateCalls++
	return backend.validate.Clone(), backend.validateErr
}

func (backend *fakeSettingsBackend) Save(ctx context.Context, loaded userconfig.Loaded, candidate userconfig.Candidate) (userconfig.SaveResult, error) {
	backend.mu.Lock()
	backend.saveCalls++
	save := backend.saveFn
	result, err := cloneSettingsSaveResult(backend.saveResult), backend.saveErr
	useLoaded := backend.saveResult.Loaded.Settings == nil && !backend.saveResult.Changed
	backend.mu.Unlock()
	if save != nil {
		return save(ctx, cloneSettingsLoaded(loaded), candidate.Clone())
	}
	if useLoaded {
		return userconfig.SaveResult{Loaded: cloneSettingsLoaded(loaded)}, err
	}
	return result, err
}

func TestSettingsAPIGetReturnsOwnedValidSettings(t *testing.T) {
	settings := settingsAtRevision(7, "C:/osc")
	backend := &fakeSettingsBackend{loaded: userconfig.Loaded{Settings: &settings, Defaults: candidate("C:/default")}}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)

	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := api.Get()
	if got.Problem != nil || got.FileRevision != 7 || got.Settings.Avatar.OSCRoot != "C:/osc" {
		t.Fatalf("Get() = %+v, want valid loaded settings", got)
	}
	got.Settings.Plugins.DevRoots = append(got.Settings.Plugins.DevRoots, "C:/mutated")
	if again := api.Get(); len(again.Settings.Plugins.DevRoots) != 0 {
		t.Fatalf("Get() leaked settings ownership: %+v", again.Settings.Plugins.DevRoots)
	}
}

func TestSettingsAPIGetInvalidFileOffersDefaultsAndDiagnostic(t *testing.T) {
	backend := &fakeSettingsBackend{loaded: userconfig.Loaded{
		Defaults:   candidate("C:/repair"),
		Invalid:    true,
		Diagnostic: errors.New("bad file contains credential=secret"),
	}}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := api.Get()
	if got.Problem == nil || got.Problem.Code != ProblemInternal {
		t.Fatalf("Get().Problem = %+v, want sanitized internal diagnostic", got.Problem)
	}
	if got.Settings.Avatar.OSCRoot != "C:/repair" || got.FileRevision != 0 {
		t.Fatalf("Get() = %+v, want repair defaults with no file revision", got)
	}
}

func TestSettingsAPIValidateDoesNotWrite(t *testing.T) {
	settings := settingsAtRevision(1, "C:/osc")
	backend := &fakeSettingsBackend{loaded: userconfig.Loaded{Settings: &settings}, validate: candidate("C:/normalized")}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := api.Validate(candidate("C:/candidate"))
	if got.Problem != nil || got.Settings.Avatar.OSCRoot != "C:/normalized" {
		t.Fatalf("Validate() = %+v", got)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.saveCalls != 0 || backend.validateCalls != 1 {
		t.Fatalf("Validate calls = validate %d, save %d; want 1, 0", backend.validateCalls, backend.saveCalls)
	}
}

func TestSettingsAPISaveRejectsStaleModuleRevisionWithoutStoreIO(t *testing.T) {
	settings := settingsAtRevision(3, "C:/osc")
	backend := &fakeSettingsBackend{loaded: userconfig.Loaded{Settings: &settings}}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	current := api.Get()

	got := api.Save(current.Revision-1, candidate("C:/candidate"))
	if got.Problem == nil || got.Problem.Code != ProblemConflict || got.Problem.CurrentRevision != current.Revision {
		t.Fatalf("Save stale = %+v, want module conflict at %d", got, current.Revision)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.saveCalls != 0 {
		t.Fatalf("Store Save calls = %d, want 0", backend.saveCalls)
	}
}

func TestSettingsAPISaveChangedPublishesOnceAndRequiresRestart(t *testing.T) {
	before := settingsAtRevision(1, "C:/before")
	after := settingsAtRevision(2, "C:/after")
	backend := &fakeSettingsBackend{
		loaded:     userconfig.Loaded{Settings: &before},
		saveResult: userconfig.SaveResult{Changed: true, Loaded: userconfig.Loaded{Settings: &after}},
	}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	previous := api.Get()

	got := api.Save(previous.Revision, candidate("C:/after"))
	if got.Problem != nil || !got.RestartRequired || got.Revision != previous.Revision+1 || got.FileRevision != 2 || got.Settings.Avatar.OSCRoot != "C:/after" {
		t.Fatalf("Save changed = %+v, previous = %+v", got, previous)
	}
	if current := api.Get(); current.Revision != got.Revision {
		t.Fatalf("Get revision = %d, want %d", current.Revision, got.Revision)
	}
}

func TestSettingsAPISaveNoOpDoesNotPublishOrRequireRestart(t *testing.T) {
	settings := settingsAtRevision(5, "C:/same")
	backend := &fakeSettingsBackend{loaded: userconfig.Loaded{Settings: &settings}}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	previous := api.Get()

	got := api.Save(previous.Revision, candidate("C:/same"))
	if got.Problem != nil || got.RestartRequired || got.Revision != previous.Revision || got.FileRevision != previous.FileRevision {
		t.Fatalf("Save no-op = %+v, previous = %+v", got, previous)
	}
}

func TestSettingsAPISaveFailureDoesNotPublish(t *testing.T) {
	settings := settingsAtRevision(5, "C:/same")
	backend := &fakeSettingsBackend{loaded: userconfig.Loaded{Settings: &settings}, saveErr: errors.New("disk contains token=secret")}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	previous := api.Get()

	got := api.Save(previous.Revision, candidate("C:/changed"))
	if got.Problem == nil || got.Problem.Code != ProblemInternal || got.Revision != previous.Revision {
		t.Fatalf("Save failure = %+v, previous = %+v", got, previous)
	}
	if current := api.Get(); current.Revision != previous.Revision || current.Settings.Avatar.OSCRoot != "C:/same" {
		t.Fatalf("failed Save published %+v", current)
	}
}

func TestSettingsAPISaveAfterCloseIsUnavailable(t *testing.T) {
	settings := settingsAtRevision(1, "C:/osc")
	backend := &fakeSettingsBackend{loaded: userconfig.Loaded{Settings: &settings}}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	api.close()

	got := api.Save(api.Get().Revision, candidate("C:/candidate"))
	if got.Problem == nil || got.Problem.Code != ProblemUnavailable {
		t.Fatalf("Save after close = %+v", got)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.saveCalls != 0 {
		t.Fatalf("Store Save calls = %d, want 0", backend.saveCalls)
	}
}

func TestSettingsAPISerializesConcurrentCAS(t *testing.T) {
	before := settingsAtRevision(1, "C:/before")
	after := settingsAtRevision(2, "C:/after")
	backend := &fakeSettingsBackend{
		loaded:     userconfig.Loaded{Settings: &before},
		saveResult: userconfig.SaveResult{Changed: true, Loaded: userconfig.Loaded{Settings: &after}},
	}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	expected := api.Get().Revision

	responses := make(chan SettingsSaveResponse, 2)
	go func() { responses <- api.Save(expected, candidate("C:/after")) }()
	go func() { responses <- api.Save(expected, candidate("C:/after")) }()
	first, second := <-responses, <-responses
	var successes, conflicts int
	for _, response := range []SettingsSaveResponse{first, second} {
		switch {
		case response.Problem == nil && response.RestartRequired:
			successes++
		case response.Problem != nil && response.Problem.Code == ProblemConflict:
			conflicts++
		default:
			t.Fatalf("concurrent Save response = %+v", response)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent Save outcomes = %d successes, %d conflicts; want 1, 1", successes, conflicts)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.saveCalls != 1 {
		t.Fatalf("Store Save calls = %d, want 1", backend.saveCalls)
	}
}

func TestSettingsAPILoadStartupConflictProblemUsesPublishedRevision(t *testing.T) {
	backend := &fakeSettingsBackend{loadErr: userconfig.ErrConflict}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)

	if _, err := api.loadForStartup(context.Background()); !errors.Is(err, userconfig.ErrConflict) {
		t.Fatalf("loadForStartup() error = %v, want conflict", err)
	}
	got := api.Get()
	if got.Revision != 2 || got.Problem == nil || got.Problem.Code != ProblemConflict || got.Problem.CurrentRevision != got.Revision {
		t.Fatalf("Get() after startup conflict = %+v, want revision 2 and currentRevision 2", got)
	}
}

func TestSettingsAPICloseCancelsBlockedSaveWithoutPublication(t *testing.T) {
	settings := settingsAtRevision(1, "C:/before")
	entered := make(chan struct{})
	backend := &fakeSettingsBackend{
		loaded: userconfig.Loaded{Settings: &settings},
		saveFn: func(ctx context.Context, _ userconfig.Loaded, _ userconfig.Candidate) (userconfig.SaveResult, error) {
			close(entered)
			<-ctx.Done()
			return userconfig.SaveResult{}, ctx.Err()
		},
	}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	previous := api.Get()
	result := make(chan SettingsSaveResponse, 1)
	go func() { result <- api.Save(previous.Revision, candidate("C:/after")) }()
	<-entered

	closed := make(chan struct{})
	go func() {
		api.close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not return while Save was blocked")
	}
	select {
	case got := <-result:
		if got.Problem == nil || got.Problem.Code != ProblemUnavailable || got.Revision != previous.Revision {
			t.Fatalf("blocked Save result = %+v, want unavailable without publication", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Save did not return after close cancellation")
	}
	if got := api.Get(); got.Revision != previous.Revision || got.Settings.Avatar.OSCRoot != "C:/before" {
		t.Fatalf("blocked Save published %+v", got)
	}
}

func TestSettingsAPICloseCancelsBlockedStartupLoad(t *testing.T) {
	entered := make(chan struct{})
	backend := &fakeSettingsBackend{
		loadFn: func(ctx context.Context) (userconfig.Loaded, error) {
			close(entered)
			<-ctx.Done()
			return userconfig.Loaded{}, ctx.Err()
		},
	}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	result := make(chan error, 1)
	go func() {
		_, err := api.loadForStartup(context.Background())
		result <- err
	}()
	<-entered

	api.close()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("loadForStartup() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup load did not return after close cancellation")
	}
}

func TestSettingsAPISubscribeIsOwnedLatestOnlyAndCancelable(t *testing.T) {
	before := settingsAtRevision(1, "C:/before")
	after := settingsAtRevision(2, "C:/after")
	after.Plugins.DevRoots = []string{"C:/one"}
	backend := &fakeSettingsBackend{
		loaded:     userconfig.Loaded{Settings: &before},
		saveResult: userconfig.SaveResult{Changed: true, Loaded: userconfig.Loaded{Settings: &after}},
	}
	api := newSettingsAPI(backend, candidate("C:/initial"), nil)
	if _, err := api.loadForStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	updates := api.subscribe(ctx)
	initial := <-updates
	initial.Settings.Plugins.DevRoots = append(initial.Settings.Plugins.DevRoots, "C:/mutated")
	if current := api.Get(); len(current.Settings.Plugins.DevRoots) != 0 {
		t.Fatalf("subscription leaked initial ownership: %+v", current.Settings.Plugins.DevRoots)
	}
	if got := api.Save(initial.Revision, candidate("C:/after")); got.Problem != nil || !got.RestartRequired {
		t.Fatalf("Save() = %+v", got)
	}
	select {
	case updated := <-updates:
		if updated.FileRevision != 2 || len(updated.Settings.Plugins.DevRoots) != 1 || updated.Settings.Plugins.DevRoots[0] != "C:/one" {
			t.Fatalf("subscription update = %+v", updated)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription did not receive changed snapshot")
	}
	cancel()
	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("subscription remained open after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription did not close after cancellation")
	}
}

func candidate(root string) userconfig.Candidate {
	return userconfig.Candidate{Avatar: userconfig.Avatar{OSCRoot: root}, Plugins: userconfig.Plugins{DevRoots: []string{}}}
}

func settingsAtRevision(revision uint64, root string) userconfig.Settings {
	return userconfig.Settings{SchemaVersion: userconfig.SchemaVersion, Revision: revision, Avatar: userconfig.Avatar{OSCRoot: root}, Plugins: userconfig.Plugins{DevRoots: []string{}}}
}

func cloneSettingsLoaded(value userconfig.Loaded) userconfig.Loaded {
	clone := value
	clone.Defaults = value.Defaults.Clone()
	if value.Settings != nil {
		settings := value.Settings.Clone()
		clone.Settings = &settings
	}
	return clone
}

func cloneSettingsSaveResult(value userconfig.SaveResult) userconfig.SaveResult {
	return userconfig.SaveResult{Loaded: cloneSettingsLoaded(value.Loaded), Changed: value.Changed}
}
