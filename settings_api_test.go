package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/userconfig"
)

type fakeSettingsBackend struct {
	mu sync.Mutex

	loaded        userconfig.Loaded
	loadErr       error
	validate      userconfig.Candidate
	validateErr   error
	saveResult    userconfig.SaveResult
	saveErr       error
	loadCalls     int
	validateCalls int
	saveCalls     int
}

func (backend *fakeSettingsBackend) LoadOrCreate(context.Context) (userconfig.Loaded, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.loadCalls++
	return cloneSettingsLoaded(backend.loaded), backend.loadErr
}

func (backend *fakeSettingsBackend) Validate(candidate userconfig.Candidate) (userconfig.Candidate, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.validateCalls++
	return backend.validate.Clone(), backend.validateErr
}

func (backend *fakeSettingsBackend) Save(_ context.Context, loaded userconfig.Loaded, candidate userconfig.Candidate) (userconfig.SaveResult, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.saveCalls++
	if backend.saveResult.Loaded.Settings == nil && !backend.saveResult.Changed {
		return userconfig.SaveResult{Loaded: cloneSettingsLoaded(loaded)}, backend.saveErr
	}
	return cloneSettingsSaveResult(backend.saveResult), backend.saveErr
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
