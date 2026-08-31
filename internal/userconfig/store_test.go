package userconfig

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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

func TestStoreLoadAcceptsExactMaximumAndRejectsMaximumPlusOne(t *testing.T) {
	valid := validSettingsJSON(t)
	if len(valid) >= MaxSettingsBytes {
		t.Fatalf("valid fixture length = %d, need less than %d", len(valid), MaxSettingsBytes)
	}
	for _, test := range []struct {
		name    string
		size    int
		invalid bool
	}{
		{name: "exact maximum", size: MaxSettingsBytes},
		{name: "maximum plus one", size: MaxSettingsBytes + 1, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths := testStorePaths(t)
			data := append([]byte(nil), valid...)
			data = append(data, make([]byte, test.size-len(data))...)
			for index := len(valid); index < len(data); index++ {
				data[index] = ' '
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
			if loaded.Invalid != test.invalid {
				t.Fatalf("LoadOrCreate() invalid = %v, want %v", loaded.Invalid, test.invalid)
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

func TestStoreSaveDerivesRevisionAndNoOpFromAuthoritativeDocument(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "authoritative-change")

	loaded.Invalid = true
	loaded.Settings.Revision = math.MaxUint64
	loaded.Settings.Avatar.FallbackPath = candidate.Avatar.FallbackPath
	result, err := store.Save(context.Background(), loaded, candidate)
	if err != nil {
		t.Fatalf("Save(tampered loaded) error = %v", err)
	}
	if !result.Changed || result.Loaded.Settings == nil || result.Loaded.Settings.Revision != 2 {
		t.Fatalf("Save(tampered loaded) = %#v, want authoritative revision 2", result)
	}
	if _, err := os.Stat(paths.SettingsFile + ".invalid.bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Save(tampered loaded) installed repair backup: %v", err)
	}
}

func TestStoreSaveDoesNotAcceptTamperedLoadedCandidateAsNoOp(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "actual-change")
	loaded.Settings.Avatar.FallbackPath = candidate.Avatar.FallbackPath

	result, err := store.Save(context.Background(), loaded, candidate)
	if err != nil {
		t.Fatalf("Save(tampered candidate) error = %v", err)
	}
	if !result.Changed || result.Loaded.Settings == nil || result.Loaded.Settings.Revision != 2 {
		t.Fatalf("Save(tampered candidate) = %#v, want authoritative changed revision 2", result)
	}
}

func TestStoreSaveTreatsNilAndEmptyRequiredSlicesAsSemanticNoOp(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Plugins.DevRoots = []string{}
	candidate.Processing.Overrides = []ProcessingOverride{}
	candidate.Processing.MutualExclusion = [][]string{}

	result, err := store.Save(context.Background(), loaded, candidate)
	if err != nil {
		t.Fatalf("Save(nil/empty equivalent) error = %v", err)
	}
	if result.Changed || result.Loaded.Settings == nil || result.Loaded.Settings.Revision != 1 {
		t.Fatalf("Save(nil/empty equivalent) = %#v, want unchanged revision 1", result)
	}
}

func TestStoreRepairCopiesEntireOversizedDocumentToBackup(t *testing.T) {
	paths := testStorePaths(t)
	marker := []byte("<complete-invalid-tail>")
	invalid := append([]byte(`{"schemaVersion":1,"revision":1,"avatar":`), make([]byte, MaxSettingsBytes+1)...)
	invalid = append(invalid, marker...)
	if err := os.WriteFile(paths.SettingsFile, invalid, 0o600); err != nil {
		t.Fatalf("WriteFile(oversized) error = %v", err)
	}
	store, err := NewStore(paths)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil || !loaded.Invalid {
		t.Fatalf("LoadOrCreate() = %#v, %v", loaded, err)
	}
	if _, err := store.Save(context.Background(), loaded, loaded.Defaults); err != nil {
		t.Fatalf("Save(repair oversized) error = %v", err)
	}
	backup, err := os.ReadFile(paths.SettingsFile + ".invalid.bak")
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(backup) != string(invalid) {
		t.Fatalf("backup lost oversized bytes: got %d bytes, want %d", len(backup), len(invalid))
	}
}

func TestStoreSaveSucceedsWhenPostReplaceReadWouldFail(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	open := store.ops.open
	calls := 0
	store.ops.open = func(path string) (storeFile, error) {
		calls++
		if calls == 3 {
			return nil, errors.New("post-replace read must not occur")
		}
		return open(path)
	}
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "saved")
	result, err := store.Save(context.Background(), loaded, candidate)
	if err != nil {
		t.Fatalf("Save() after replacement error = %v", err)
	}
	if !result.Changed || result.Loaded.Settings == nil || result.Loaded.Settings.Revision != 2 {
		t.Fatalf("Save() result = %#v", result)
	}
}

func TestStoreSaveRejectsSameBytesFromReplacedFileIdentity(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	data, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	info, err := os.Stat(paths.SettingsFile)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	replacement, err := os.CreateTemp(paths.SettingsDir, ".identity-*.tmp")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	replacementPath := replacement.Name()
	defer os.Remove(replacementPath)
	if _, err := replacement.Write(data); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Chtimes(replacementPath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	if err := replaceSettingsFile(replacementPath, paths.SettingsFile); err != nil {
		t.Fatalf("replaceSettingsFile() error = %v", err)
	}
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "changed")
	if _, err := store.Save(context.Background(), loaded, candidate); !errors.Is(err, ErrConflict) {
		t.Fatalf("Save() error = %v, want ErrConflict after same-byte replacement", err)
	}
}

func TestStoreTemporaryPermissionsAndDirectorySync(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	var directoryModes []os.FileMode
	chmod := store.ops.chmod
	store.ops.chmod = func(path string, mode os.FileMode) error {
		directoryModes = append(directoryModes, mode)
		return chmod(path, mode)
	}
	var temporaryModes []os.FileMode
	create := store.ops.createTemp
	store.ops.createTemp = func(dir, pattern string) (storeFile, error) {
		file, err := create(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &modeStoreFile{storeFile: file, modes: &temporaryModes}, nil
	}
	syncs := 0
	store.ops.syncDirectory = func(string) error { syncs++; return nil }
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "changed")
	if _, err := store.Save(context.Background(), loaded, candidate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if len(directoryModes) == 0 || directoryModes[0] != 0o700 {
		t.Fatalf("directory chmod calls = %#v, want 0700", directoryModes)
	}
	if len(temporaryModes) == 0 || temporaryModes[0] != 0o600 {
		t.Fatalf("temporary chmod calls = %#v, want 0600", temporaryModes)
	}
	if syncs != 1 {
		t.Fatalf("directory sync calls = %d, want 1", syncs)
	}
}

func TestStoreTempChmodFailureCleansTemporaryFile(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	create := store.ops.createTemp
	store.ops.createTemp = func(dir, pattern string) (storeFile, error) {
		file, err := create(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &failingTempChmodFile{storeFile: file}, nil
	}
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "changed")
	if _, err := store.Save(context.Background(), loaded, candidate); err == nil {
		t.Fatal("Save() succeeded despite temporary chmod failure")
	}
	entries, err := os.ReadDir(paths.SettingsDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary file leaked after chmod failure: %s", entry.Name())
		}
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

func TestStoreReadCurrentStopsAfterCanceledStreamChunkAndClosesFile(t *testing.T) {
	paths := testStorePaths(t)
	store, _ := loadedStore(t, paths)
	entered := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	open := store.ops.open
	store.ops.open = func(path string) (storeFile, error) {
		file, err := open(path)
		if err != nil {
			return nil, err
		}
		return &closeSignalStoreFile{storeFile: file, closed: closed}, nil
	}
	read := store.ops.read
	calls := 0
	store.ops.read = func(file storeFile, data []byte) (int, error) {
		calls++
		switch calls {
		case 1:
			count, _ := read(file, data[:1])
			return count, nil
		case 2:
			close(entered)
			<-release
			return 0, nil
		default:
			return 0, errors.New("read resumed after cancellation")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := store.LoadOrCreate(ctx)
		result <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := waitStoreError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadOrCreate(canceled stream) error = %v, want context.Canceled", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("LoadOrCreate(canceled stream) leaked its file handle")
	}
}

func TestStoreRepairBackupStopsAfterCanceledStreamChunkAndCleansTemporary(t *testing.T) {
	paths := testStorePaths(t)
	invalid := []byte(`{"schemaVersion":0}`)
	if err := os.WriteFile(paths.SettingsFile, invalid, 0o600); err != nil {
		t.Fatalf("WriteFile(invalid) error = %v", err)
	}
	store, err := NewStore(paths)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadOrCreate(context.Background())
	if err != nil || !loaded.Invalid {
		t.Fatalf("LoadOrCreate() = %#v, %v; want invalid document", loaded, err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	read := store.ops.read
	authoritativeReadComplete := false
	backupCalls := 0
	store.ops.read = func(file storeFile, data []byte) (int, error) {
		if !authoritativeReadComplete {
			count, readErr := read(file, data)
			if readErr == io.EOF {
				authoritativeReadComplete = true
			}
			return count, readErr
		}
		backupCalls++
		switch backupCalls {
		case 1:
			count, _ := read(file, data[:1])
			return count, nil
		case 2:
			close(entered)
			<-release
			return 0, nil
		default:
			return 0, errors.New("backup read resumed after cancellation")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := store.Save(ctx, loaded, loaded.Defaults)
		result <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := waitStoreError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save(canceled backup stream) error = %v, want context.Canceled", err)
	}
	assertStoreTemporaryCleanup(t, paths)
	if got, err := os.ReadFile(paths.SettingsFile); err != nil || string(got) != string(invalid) {
		t.Fatalf("canceled repair changed authoritative settings: %q, %v", got, err)
	}
	if _, err := os.Stat(paths.SettingsFile + ".invalid.bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled repair installed backup: %v", err)
	}
}

func TestStoreTemporaryWriteStopsAfterCanceledChunkAndCleansTemporary(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	old, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	create := store.ops.createTemp
	store.ops.createTemp = func(dir, pattern string) (storeFile, error) {
		file, err := create(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &blockingWriteStoreFile{storeFile: file, entered: entered, release: release}, nil
	}
	candidate := candidateFromSettings(*loaded.Settings)
	candidate.Avatar.FallbackPath = filepath.Join(paths.SettingsDir, "changed")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := store.Save(ctx, loaded, candidate)
		result <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := waitStoreError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save(canceled temporary write) error = %v, want context.Canceled", err)
	}
	assertStoreTemporaryCleanup(t, paths)
	if got, err := os.ReadFile(paths.SettingsFile); err != nil || string(got) != string(old) {
		t.Fatalf("canceled temporary write changed authoritative settings: %q, %v", got, err)
	}
}

func TestStoreReplaceStopsWhenContextCancelsAfterFinalAuthoritativeRead(t *testing.T) {
	paths := testStorePaths(t)
	store, loaded := loadedStore(t, paths)
	temporary, err := os.CreateTemp(paths.SettingsDir, ".replace-cancel-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	temporaryPath := temporary.Name()
	if _, err := temporary.WriteString("replacement"); err != nil {
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	open := store.ops.open
	store.ops.open = func(path string) (storeFile, error) {
		file, err := open(path)
		if err != nil {
			return nil, err
		}
		return &cancelOnCloseStoreFile{storeFile: file, cancel: cancel}, nil
	}
	replaced := false
	store.ops.replace = func(string, string) error {
		replaced = true
		return nil
	}

	err = store.replaceTemporary(ctx, temporaryPath, paths.SettingsFile, loaded.Token)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("replaceTemporary(canceled after read) error = %v, want context.Canceled", err)
	}
	if replaced {
		t.Fatal("replaceTemporary mutated destination after cancellation")
	}
	if _, err := os.Stat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled replacement leaked temporary file: %v", err)
	}
}

func waitStoreError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("canceled store operation did not return after its blocked operation was released")
		return nil
	}
}

func assertStoreTemporaryCleanup(t *testing.T, paths Paths) {
	t.Helper()
	entries, err := os.ReadDir(paths.SettingsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary file leaked after cancellation: %s", entry.Name())
		}
	}
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
		store.ops.read = func(storeFile, []byte) (int, error) { return 0, fail }
	case "stat":
		store.ops.stat = func(storeFile) (os.FileInfo, error) { return nil, fail }
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

type closeSignalStoreFile struct {
	storeFile
	closed chan struct{}
	once   sync.Once
}

type cancelOnCloseStoreFile struct {
	storeFile
	cancel context.CancelFunc
	once   sync.Once
}

func (f *cancelOnCloseStoreFile) Close() error {
	err := f.storeFile.Close()
	f.once.Do(f.cancel)
	return err
}

func (f *cancelOnCloseStoreFile) SyscallConn() (syscall.RawConn, error) {
	return f.storeFile.(interface {
		SyscallConn() (syscall.RawConn, error)
	}).SyscallConn()
}

func (f *closeSignalStoreFile) Close() error {
	err := f.storeFile.Close()
	f.once.Do(func() { close(f.closed) })
	return err
}

func (f *closeSignalStoreFile) SyscallConn() (syscall.RawConn, error) {
	return f.storeFile.(interface {
		SyscallConn() (syscall.RawConn, error)
	}).SyscallConn()
}

type blockingWriteStoreFile struct {
	storeFile
	entered chan struct{}
	release chan struct{}
	writes  int
}

func (f *blockingWriteStoreFile) Write(data []byte) (int, error) {
	f.writes++
	if f.writes > 1 {
		return 0, errors.New("write resumed after cancellation")
	}
	close(f.entered)
	<-f.release
	if len(data) == 0 {
		return 0, nil
	}
	return f.storeFile.Write(data[:1])
}

func (f *blockingWriteStoreFile) SyscallConn() (syscall.RawConn, error) {
	return f.storeFile.(interface {
		SyscallConn() (syscall.RawConn, error)
	}).SyscallConn()
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

func (f *failingStoreFile) SyscallConn() (syscall.RawConn, error) {
	return f.storeFile.(interface {
		SyscallConn() (syscall.RawConn, error)
	}).SyscallConn()
}

type modeStoreFile struct {
	storeFile
	modes *[]os.FileMode
}

func (f *modeStoreFile) Chmod(mode os.FileMode) error {
	*f.modes = append(*f.modes, mode)
	return f.storeFile.Chmod(mode)
}

func (f *modeStoreFile) SyscallConn() (syscall.RawConn, error) {
	return f.storeFile.(interface {
		SyscallConn() (syscall.RawConn, error)
	}).SyscallConn()
}

type failingTempChmodFile struct{ storeFile }

func (f *failingTempChmodFile) Chmod(os.FileMode) error {
	return errors.New("injected temporary chmod failure")
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
