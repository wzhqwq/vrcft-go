package userconfig

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A missing configuration must be distinguished from corrupt user data: only
// the former is safe to create during startup.
func TestStoreLoadCreatesRevisionOneDefaultsOnlyWhenFileIsMissing(t *testing.T) {
	paths := testStorePaths(t)
	paths.SettingsDir = filepath.Join(paths.SettingsDir, "settings")
	paths.SettingsFile = filepath.Join(paths.SettingsDir, "config.json")
	paths.PluginStoreFile = filepath.Join(paths.SettingsDir, "plugins.json")
	store, err := NewStore(paths)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if loaded.Invalid || loaded.Diagnostic != nil {
		t.Fatalf("LoadOrCreate() invalid result = %#v", loaded)
	}
	if loaded.Settings == nil || loaded.Settings.SchemaVersion != SchemaVersion || loaded.Settings.Revision != 1 {
		t.Fatalf("LoadOrCreate() settings = %#v, want schema %d revision 1", loaded.Settings, SchemaVersion)
	}
	if loaded.Settings.Avatar.OSCRoot != paths.DefaultOSCRoot {
		t.Fatalf("default osc root = %q, want %q", loaded.Settings.Avatar.OSCRoot, paths.DefaultOSCRoot)
	}
	data, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatalf("ReadFile(default) error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("LoadOrCreate() created an empty configuration")
	}
}

func TestNewStoreRejectsPathsThatCannotProduceDefaults(t *testing.T) {
	paths := testStorePaths(t)
	paths.DefaultOSCRoot = ""
	if store, err := NewStore(paths); err == nil || store != nil {
		t.Fatalf("NewStore(blank default root) = %v, %v; want validation error", store, err)
	}
}

// Each case names a malformed document that must remain authoritative until
// an explicit revision-checked repair succeeds.
func TestStoreLoadPreservesInvalidDocuments(t *testing.T) {
	valid := validSettingsJSON(t)
	tooLarge := append([]byte(`{"schemaVersion":1,"revision":1,"avatar":`), make([]byte, MaxSettingsBytes+1)...)
	tests := []struct {
		name string
		data []byte
	}{
		{"unknown field", appendJSONField(valid, `"unknown":true`)},
		{"duplicate nested field", duplicateOSCRoot(valid)},
		{"required object null", replaceJSONText(valid, `"avatar":{`, `"avatar":null,"ignored":{`)},
		{"trailing value", append(append([]byte(nil), valid...), []byte(" true")...)},
		{"invalid utf8", append(append([]byte(nil), valid...), 0xff)},
		{"zero schema", replaceJSONText(valid, `"schemaVersion":1`, `"schemaVersion":0`)},
		{"unknown schema", replaceJSONText(valid, `"schemaVersion":1`, `"schemaVersion":2`)},
		{"zero revision", replaceJSONText(valid, `"revision":1`, `"revision":0`)},
		{"over byte limit", tooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := testStorePaths(t)
			if err := os.WriteFile(paths.SettingsFile, test.data, 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			store, err := NewStore(paths)
			if err != nil {
				t.Fatalf("NewStore() error = %v", err)
			}

			loaded, err := store.LoadOrCreate(context.Background())
			if err != nil {
				t.Fatalf("LoadOrCreate() error = %v", err)
			}
			if !loaded.Invalid || loaded.Diagnostic == nil || loaded.Settings != nil {
				t.Fatalf("LoadOrCreate() = %#v, want invalid diagnostic defaults", loaded)
			}
			if normalized, err := Normalize(loaded.Defaults); err != nil || normalized.Avatar.OSCRoot != paths.DefaultOSCRoot {
				t.Fatalf("invalid defaults = %#v, %v", loaded.Defaults, err)
			}
			got, err := os.ReadFile(paths.SettingsFile)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if string(got) != string(test.data) {
				t.Fatalf("invalid file changed\n got: %q\nwant: %q", got, test.data)
			}
		})
	}
}

// A save of equivalent intent must not churn the revision or touch the file;
// this catches accidental comparison of the caller's unnormalized DTO.
func TestStoreSaveNoOpDoesNotWriteOrIncrement(t *testing.T) {
	paths := testStorePaths(t)
	store, err := NewStore(paths)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	before, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}

	result, err := store.Save(context.Background(), loaded, candidateFromSettings(*loaded.Settings))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if result.Changed {
		t.Fatal("Save() marked an equivalent candidate changed")
	}
	if result.Loaded.Settings == nil || result.Loaded.Settings.Revision != 1 {
		t.Fatalf("Save() revision = %#v, want 1", result.Loaded.Settings)
	}
	after, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("no-op changed settings bytes\n got: %s\nwant: %s", after, before)
	}
}

func TestStoreSavePersistsOneNormalizedRevisionIncrement(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "next")

	result, err := store.Save(context.Background(), loaded, candidate)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !result.Changed || result.Loaded.Settings == nil || result.Loaded.Settings.Revision != 2 {
		t.Fatalf("Save() = %#v, want changed revision 2", result)
	}
	if result.Loaded.Settings.Avatar.FallbackPath != filepath.Join(paths.SettingsDir, "next") {
		t.Fatalf("saved fallback = %q", result.Loaded.Settings.Avatar.FallbackPath)
	}
	result.Loaded.Settings.Avatar.FallbackPath = "mutated"
	fresh, err := store.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() after save error = %v", err)
	}
	if fresh.Settings == nil || fresh.Settings.Revision != 2 || fresh.Settings.Avatar.FallbackPath != filepath.Join(paths.SettingsDir, "next") {
		t.Fatalf("persisted settings = %#v", fresh.Settings)
	}
}

// A stale token is a conflict even when the old in-memory revision still
// matches, because another process owns the authoritative file too.
func TestStoreSaveRejectsStaleTokenAfterExternalEdit(t *testing.T) {
	paths := testStorePaths(t)
	store, err := NewStore(paths)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	external := appendJSONField(validSettingsJSON(t), `"external":true`)
	if err := os.WriteFile(paths.SettingsFile, external, 0o600); err != nil {
		t.Fatalf("WriteFile(external) error = %v", err)
	}
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "changed")
	if _, err := store.Save(context.Background(), loaded, candidate); !errors.Is(err, ErrConflict) {
		t.Fatalf("Save() error = %v, want ErrConflict", err)
	}
	got, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(external) {
		t.Fatalf("stale save overwrote external document\n got: %s\nwant: %s", got, external)
	}
}

func TestStoreSaveRejectsRevisionExhaustion(t *testing.T) {
	paths := testStorePaths(t)
	candidate, err := Normalize(DefaultCandidate(paths))
	if err != nil {
		t.Fatalf("Normalize(default) error = %v", err)
	}
	data, err := encodeSettingsDocument(Settings{SchemaVersion: SchemaVersion, Revision: math.MaxUint64, Avatar: candidate.Avatar, Plugins: candidate.Plugins, Processing: candidate.Processing, OSC: candidate.OSC})
	if err != nil {
		t.Fatalf("encodeSettingsDocument() error = %v", err)
	}
	if err := os.WriteFile(paths.SettingsFile, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := NewStore(paths)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if loaded.Settings == nil || loaded.Invalid {
		t.Fatalf("LoadOrCreate() invalid = %v (%#v)", loaded.Diagnostic, loaded)
	}
	next := candidateFromSettings(*loaded.Settings)
	next.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "changed")
	if _, err := store.Save(context.Background(), loaded, next); !errors.Is(err, ErrRevisionExhausted) {
		t.Fatalf("Save() error = %v, want ErrRevisionExhausted", err)
	}
}

// Repair must preserve the latest bad bytes durably before they can be
// replaced, so an interrupted repair never destroys the only evidence.
func TestStoreRepairInstallsBackupBeforeReplacement(t *testing.T) {
	paths := testStorePaths(t)
	invalid := []byte(`{"schemaVersion":0}`)
	if err := os.WriteFile(paths.SettingsFile, invalid, 0o600); err != nil {
		t.Fatalf("WriteFile(invalid) error = %v", err)
	}
	store, err := NewStore(paths)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if !loaded.Invalid {
		t.Fatal("LoadOrCreate() accepted invalid document")
	}
	var replacements []string
	replace := store.ops.replace
	store.ops.replace = func(oldPath, newPath string) error {
		replacements = append(replacements, newPath)
		return replace(oldPath, newPath)
	}
	candidate := loaded.Defaults.Clone()
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "repair")
	result, err := store.Save(context.Background(), loaded, candidate)
	if err != nil {
		t.Fatalf("Save(repair) error = %v", err)
	}
	if !result.Changed || result.Loaded.Settings == nil || result.Loaded.Settings.Revision != 1 {
		t.Fatalf("Save(repair) = %#v", result)
	}
	want := []string{paths.SettingsFile + ".invalid.bak", paths.SettingsFile}
	if strings.Join(replacements, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("replacement order = %#v, want %#v", replacements, want)
	}
	backup, err := os.ReadFile(paths.SettingsFile + ".invalid.bak")
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(backup) != string(invalid) {
		t.Fatalf("backup = %q, want %q", backup, invalid)
	}
}

func TestStoreSavePreservesAuthoritativeFileWhenWriteLifecycleFails(t *testing.T) {
	for _, stage := range []string{"open", "read", "stat", "mkdir", "chmod", "create", "write", "sync", "close", "replace"} {
		t.Run(stage, func(t *testing.T) {
			paths := testStorePaths(t)
			store, loaded := loadedStore(t, paths)
			old, err := os.ReadFile(paths.SettingsFile)
			if err != nil {
				t.Fatalf("ReadFile(before) error = %v", err)
			}
			installStoreFailure(store, stage)
			candidate := candidateFromSettings(*loaded.Settings)
			candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "changed")
			if _, err := store.Save(context.Background(), loaded, candidate); err == nil {
				t.Fatalf("Save() succeeded with injected %s failure", stage)
			}
			got, err := os.ReadFile(paths.SettingsFile)
			if err != nil {
				t.Fatalf("ReadFile(after) error = %v", err)
			}
			if string(got) != string(old) {
				t.Fatalf("%s failure changed authoritative bytes", stage)
			}
			entries, err := os.ReadDir(paths.SettingsDir)
			if err != nil {
				t.Fatalf("ReadDir() error = %v", err)
			}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".tmp") {
					t.Fatalf("temporary file leaked after %s failure: %s", stage, entry.Name())
				}
			}
		})
	}
}

func TestStoreHonorsCanceledContextBeforeIOAndWhileWaiting(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Save(ctx, loaded, candidateFromSettings(*loaded.Settings)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save(canceled) error = %v", err)
	}

	<-store.lock
	waiting, cancelWaiting := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := store.Save(waiting, loaded, candidateFromSettings(*loaded.Settings))
		result <- err
	}()
	cancelWaiting()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Save(waiting canceled) error = %v", err)
	}
	store.lock <- struct{}{}
}

func loadedStore(t *testing.T, paths Paths) (*Store, Loaded) {
	t.Helper()
	store, err := NewStore(paths)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	return store, loaded
}

func installStoreFailure(store *Store, stage string) {
	fail := errors.New("injected " + stage + " failure")
	switch stage {
	case "open":
		store.ops.open = func(string) (storeFile, error) { return nil, fail }
	case "read":
		store.ops.readAll = func(io.Reader, int64) ([]byte, error) { return nil, fail }
	case "stat":
		store.ops.stat = func(string) (os.FileInfo, error) { return nil, fail }
	case "mkdir":
		store.ops.mkdirAll = func(string, os.FileMode) error { return fail }
	case "chmod":
		store.ops.chmod = func(string, os.FileMode) error { return fail }
	case "create":
		store.ops.createTemp = func(string, string) (storeFile, error) { return nil, fail }
	case "write", "sync", "close":
		create := store.ops.createTemp
		store.ops.createTemp = func(dir, pattern string) (storeFile, error) {
			file, err := create(dir, pattern)
			if err != nil {
				return nil, err
			}
			return &failingStoreFile{storeFile: file, stage: stage, err: fail}, nil
		}
	case "replace":
		store.ops.replace = func(string, string) error { return fail }
	}
}

type failingStoreFile struct {
	storeFile
	stage string
	err   error
}

func (f *failingStoreFile) Write(data []byte) (int, error) {
	if f.stage == "write" {
		return 0, f.err
	}
	return f.storeFile.Write(data)
}

func (f *failingStoreFile) Sync() error {
	if f.stage == "sync" {
		return f.err
	}
	return f.storeFile.Sync()
}

func (f *failingStoreFile) Close() error {
	if f.stage == "close" {
		if err := f.storeFile.Close(); err != nil {
			return err
		}
		return f.err
	}
	return f.storeFile.Close()
}

func testStorePaths(t *testing.T) Paths {
	t.Helper()
	dir := t.TempDir()
	return Paths{
		SettingsDir:     dir,
		SettingsFile:    filepath.Join(dir, "config.json"),
		DefaultOSCRoot:  filepath.Join(dir, "osc"),
		PluginStoreFile: filepath.Join(dir, "plugins.json"),
	}
}

func validSettingsJSON(t *testing.T) []byte {
	t.Helper()
	paths := testStorePaths(t)
	candidate, err := Normalize(DefaultCandidate(paths))
	if err != nil {
		t.Fatalf("Normalize(default) error = %v", err)
	}
	data, err := encodeSettingsDocument(Settings{SchemaVersion: SchemaVersion, Revision: 1, Avatar: candidate.Avatar, Plugins: candidate.Plugins, Processing: candidate.Processing, OSC: candidate.OSC})
	if err != nil {
		t.Fatalf("encodeSettingsDocument(valid settings) error = %v", err)
	}
	return data
}

func appendJSONField(data []byte, field string) []byte {
	return append(append(append([]byte(nil), data[:len(data)-1]...), ','), append([]byte(field), '}')...)
}

func duplicateOSCRoot(data []byte) []byte {
	return replaceJSONText(data, `"oscRoot":"`, `"oscRoot":"duplicate","oscRoot":"`)
}

func replaceJSONText(data []byte, old, replacement string) []byte {
	return []byte(strings.Replace(string(data), old, replacement, 1))
}
