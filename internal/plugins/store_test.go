package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func testConfig(revision uint64, data string) pluginapi.Config {
	return pluginapi.Config{Revision: revision, Data: json.RawMessage(data)}
}

func newTestJSONStore(t *testing.T, maxBytes int64) (Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plugins.json")
	store, err := NewJSONStore(path, maxBytes)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	return store, path
}

func TestJSONStoreLoadMissingFileReturnsEmptySettings(t *testing.T) {
	store, _ := newTestJSONStore(t, 4096)

	settings, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(settings.Plugins) != 0 {
		t.Fatalf("Load().Plugins = %+v, want empty", settings.Plugins)
	}
}

func TestJSONStoreRoundTripsEnabledConfigAndUnknownIDs(t *testing.T) {
	store, _ := newTestJSONStore(t, 4096)
	want := PluginSettings{Plugins: map[string]PluginPreference{
		"available.device": {Enabled: true, Config: testConfig(7, `{"gain":0.75}`)},
		"missing.device":   {Enabled: false, Config: testConfig(2, `{"mode":"safe"}`)},
	}}

	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for id, preference := range want.Plugins {
		actual, ok := got.Plugins[id]
		if !ok {
			t.Fatalf("Load().Plugins missing %q", id)
		}
		if actual.Enabled != preference.Enabled || actual.Config.Revision != preference.Config.Revision || string(actual.Config.Data) != string(preference.Config.Data) {
			t.Fatalf("Load().Plugins[%q] = %+v, want %+v", id, actual, preference)
		}
	}
}

func TestJSONStoreReplacesExistingSettingsWithCompleteNewVersion(t *testing.T) {
	store, path := newTestJSONStore(t, 4096)
	first := PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.device": {Enabled: true, Config: testConfig(1, `{"mode":"first"}`)},
	}}
	second := PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.device": {Enabled: false, Config: testConfig(2, `{"mode":"second"}`)},
	}}
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	preference := loaded.Plugins["vendor.device"]
	if preference.Enabled || preference.Config.Revision != 2 || string(preference.Config.Data) != `{"mode":"second"}` {
		t.Fatalf("Load() after replacement = %+v, want complete second version", preference)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "first") || !strings.Contains(string(data), "second") {
		t.Fatalf("replacement file = %s, want only complete second version", data)
	}
}

func TestJSONStoreRejectsUnknownFields(t *testing.T) {
	store, path := newTestJSONStore(t, 4096)
	data := []byte(`{"plugins":[{"id":"vendor.device","enabled":true,"config":{"Revision":1,"Data":{"gain":1}},"unexpected":true}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("Load() error = nil, want unknown field rejection")
	}
}

func TestJSONStoreRejectsInvalidConfigWithoutLeakingData(t *testing.T) {
	store, _ := newTestJSONStore(t, 4096)
	err := store.Save(context.Background(), PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.device": {Config: testConfig(0, `{"credential":"do-not-leak"}`)},
	}})
	if err == nil {
		t.Fatal("Save() error = nil, want invalid Config rejection")
	}
	if strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("Save() error leaked Config.Data: %v", err)
	}
}

func TestJSONStoreEnforcesFileAndConfigSizeLimits(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		store, _ := newTestJSONStore(t, 20)
		err := store.Save(context.Background(), PluginSettings{Plugins: map[string]PluginPreference{
			"vendor.device": {Config: testConfig(1, `{"value":"this is too large"}`)},
		}})
		if err == nil {
			t.Fatal("Save() error = nil, want config size rejection")
		}
	})
	t.Run("file", func(t *testing.T) {
		store, path := newTestJSONStore(t, 90)
		data := []byte(`{"plugins":[{"id":"one","enabled":true,"config":{"Revision":1,"Data":{}}},{"id":"two","enabled":true,"config":{"Revision":1,"Data":{}}}]}`)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(context.Background()); err == nil {
			t.Fatal("Load() error = nil, want file size rejection")
		}
	})
}

func TestJSONStoreDefensivelyOwnsConfigData(t *testing.T) {
	store, _ := newTestJSONStore(t, 4096)
	input := testConfig(1, `{"mode":"original"}`)
	if err := store.Save(context.Background(), PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.device": {Enabled: true, Config: input},
	}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	input.Data[9] = 'X'

	first, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first.Plugins["vendor.device"].Config.Data[9] = 'Y'
	second, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(second.Plugins["vendor.device"].Config.Data); got != `{"mode":"original"}` {
		t.Fatalf("subsequent Load() Config.Data = %q, want original", got)
	}
}

func TestJSONStoreWritesPluginIDsInOrderWithoutSessionFields(t *testing.T) {
	store, path := newTestJSONStore(t, 4096)
	if err := store.Save(context.Background(), PluginSettings{Plugins: map[string]PluginPreference{
		"zeta.device":  {Enabled: true, Config: testConfig(1, `{}`)},
		"alpha.device": {Enabled: false, Config: testConfig(2, `{}`)},
	}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Plugins []struct {
			ID string `json:"id"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Plugins) != 2 || wire.Plugins[0].ID != "alpha.device" || wire.Plugins[1].ID != "zeta.device" {
		t.Fatalf("written plugin order = %+v, want alpha.device then zeta.device", wire.Plugins)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"active": true, "subscription": true, "pipe": true, "token": true, "pid": true, "runtime": true}
	assertNoForbiddenJSONKeys(t, document, forbidden)
}

func TestJSONStorePreservesOldFileWhenRenameFails(t *testing.T) {
	store, path := newTestJSONStore(t, 4096)
	old := []byte(`{"plugins":[]}`)
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	jsonStore := store.(*jsonStore)
	originalReplace := jsonStore.ops.replace
	jsonStore.ops.replace = func(string, string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { jsonStore.ops.replace = originalReplace })

	err := store.Save(context.Background(), PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.device": {Enabled: true, Config: testConfig(1, `{}`)},
	}})
	if err == nil {
		t.Fatal("Save() error = nil, want injected rename failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(old) {
		t.Fatalf("destination after failed replacement = %s, want original %s", got, old)
	}
}

func TestJSONStoreCleansTemporaryFileAndPreservesDestinationOnWriteLifecycleFailure(t *testing.T) {
	store, path := newTestJSONStore(t, 4096)
	old := []byte(`{"plugins":[]}`)
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	settings := PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.device": {Enabled: true, Config: testConfig(1, `{}`)},
	}}

	for _, stage := range []string{"write", "sync", "close"} {
		t.Run(stage, func(t *testing.T) {
			jsonStore := store.(*jsonStore)
			originalCreateTemp := jsonStore.ops.createTemp
			jsonStore.ops.createTemp = func(dir, pattern string) (jsonStoreFile, error) {
				file, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				return &failingJSONStoreFile{File: file, stage: stage}, nil
			}
			t.Cleanup(func() { jsonStore.ops.createTemp = originalCreateTemp })

			if err := store.Save(context.Background(), settings); err == nil {
				t.Fatal("Save() error = nil, want injected lifecycle failure")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(old) {
				t.Fatalf("destination after failed %s = %s, want original %s", stage, got, old)
			}
			temporary, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".plugins-*.tmp"))
			if err != nil {
				t.Fatal(err)
			}
			if len(temporary) != 0 {
				t.Fatalf("temporary files after failed %s = %v, want none", stage, temporary)
			}
		})
	}
}

type failingJSONStoreFile struct {
	*os.File
	stage string
}

func (f *failingJSONStoreFile) Write(data []byte) (int, error) {
	if f.stage == "write" {
		return 0, errors.New("injected write failure")
	}
	return f.File.Write(data)
}

func (f *failingJSONStoreFile) Sync() error {
	if f.stage == "sync" {
		return errors.New("injected sync failure")
	}
	return f.File.Sync()
}

func (f *failingJSONStoreFile) Close() error {
	err := f.File.Close()
	if err != nil {
		return err
	}
	if f.stage == "close" {
		return errors.New("injected close failure")
	}
	return nil
}

func TestJSONStoreHonorsCanceledContextBeforeIO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "plugins.json")
	store, err := NewJSONStore(path, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want context.Canceled", err)
	}
	if err := store.Save(ctx, PluginSettings{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}
}

func assertNoForbiddenJSONKeys(t *testing.T, value any, forbidden map[string]bool) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if forbidden[strings.ToLower(key)] {
				t.Fatalf("persisted JSON contains forbidden field %q", key)
			}
			assertNoForbiddenJSONKeys(t, child, forbidden)
		}
	case []any:
		for _, child := range value {
			assertNoForbiddenJSONKeys(t, child, forbidden)
		}
	}
}
