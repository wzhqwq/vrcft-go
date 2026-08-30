package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
	pluginmanager "github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func TestM7BackendPrerequisiteInterfacesEndToEnd(t *testing.T) {
	temp := t.TempDir()
	environment := userconfig.Environment{
		GOOS:        "windows",
		RoamingDir:  filepath.Join(temp, "Users", "Fixture", "AppData", "Roaming"),
		UserProfile: filepath.Join(temp, "Users", "Fixture"),
		Executable:  filepath.Join(temp, "Program Files", "vrcft-go", "vrcft-go.exe"),
	}
	paths, err := userconfig.ResolvePaths(environment)
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if _, err := os.Stat(paths.SettingsFile); !os.IsNotExist(err) {
		t.Fatalf("pre-start settings stat error = %v, want missing file", err)
	}

	backend := newM7IntegrationBackend()
	emitter := newM7IntegrationEmitter()
	var factoryMu sync.Mutex
	var factoryCalls int
	var constructedConfig application.Config
	var realStore *userconfig.Store
	app := newAppWithDependencies(appDependencies{
		goos: "windows",
		environment: func() (userconfig.Environment, error) {
			return environment, nil
		},
		resolvePaths: userconfig.ResolvePaths,
		newStore: func(got userconfig.Paths) (settingsBackend, error) {
			if got != paths {
				t.Fatalf("root store paths = %+v, want %+v", got, paths)
			}
			store, err := userconfig.NewStore(got)
			if err == nil {
				realStore = store
			}
			return store, err
		},
		applicationConfig: userconfig.ApplicationConfig,
		newBackend: func(config application.Config) (*application.Application, backendOperations, error) {
			factoryMu.Lock()
			defer factoryMu.Unlock()
			factoryCalls++
			constructedConfig = config
			return nil, backend, nil
		},
		now:             time.Now,
		emitter:         emitter,
		shutdownTimeout: time.Second,
	})
	t.Cleanup(func() { app.shutdown(context.Background()) })

	app.startup(context.Background())
	status := app.runtime.GetStatus()
	if status.Phase != "running" || status.Application == nil || status.Application.Lifecycle != "running" {
		t.Fatalf("runtime after startup = %+v problem=%+v, want running root and backend", status, status.Problem)
	}
	if realStore == nil {
		t.Fatal("root did not construct the real userconfig Store")
	}
	if _, err := os.Stat(paths.SettingsFile); err != nil {
		t.Fatalf("first-run settings file was not created: %v", err)
	}
	loaded, err := realStore.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("reload first-run settings: %v", err)
	}
	if loaded.Settings == nil || loaded.Settings.Revision != 1 || loaded.Settings.Avatar.OSCRoot != paths.DefaultOSCRoot {
		t.Fatalf("first-run settings = %+v, want revision-1 defaults rooted at %q", loaded.Settings, paths.DefaultOSCRoot)
	}
	factoryMu.Lock()
	if factoryCalls != 1 || constructedConfig.Avatar.OSCRoot != paths.DefaultOSCRoot || constructedConfig.PluginStorePath != paths.PluginStoreFile {
		factoryMu.Unlock()
		t.Fatalf("backend construction = calls %d config %+v", factoryCalls, constructedConfig)
	}
	factoryMu.Unlock()
	if starts, statusSubscriptions, pluginSubscriptions := backend.startupCounts(); starts != 1 || statusSubscriptions != 1 || pluginSubscriptions != 1 {
		t.Fatalf("backend startup/subscriptions = %d/%d/%d, want 1/1/1", starts, statusSubscriptions, pluginSubscriptions)
	}

	// Observe each module's final startup snapshot before taking event-count
	// baselines. Once these values have reached the serial module workers, no
	// older startup value can be emitted afterward.
	emitter.waitAfter(t, 0, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(RuntimeResponse)
		return ok && event.name == eventRuntimeStatus && response.Phase == "running"
	})
	emitter.waitAfter(t, 0, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(SettingsResponse)
		return ok && event.name == eventSettingsChanged && response.FileRevision == 1
	})
	emitter.waitAfter(t, 0, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(PluginListResponse)
		return ok && event.name == eventPluginsChanged && response.Problem == nil && len(response.Plugins) == 1 && response.Plugins[0].ID == "fixture.bootstrap"
	})
	if counts := emitter.counts(); counts.other != 0 {
		t.Fatalf("startup emitted non-versioned event names: %+v", counts)
	}

	plugin := pluginmanager.RuntimeSnapshot{
		ID: "vendor.alpha", Name: "Alpha", Description: "integration plugin", Version: "1.0.0",
		State: pluginmanager.StateStopped, ConfigRevision: 1,
	}
	beforePluginEvent := emitter.counts()
	backend.replacePlugin(plugin, pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)})
	emitter.waitAfter(t, beforePluginEvent.total, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(PluginListResponse)
		return ok && event.name == eventPluginsChanged && response.Problem == nil && len(response.Plugins) == 1 && response.Plugins[0].ID == plugin.ID
	})
	afterPluginEvent := emitter.counts()
	if afterPluginEvent.plugins != beforePluginEvent.plugins+1 || afterPluginEvent.runtime != beforePluginEvent.runtime || afterPluginEvent.settings != beforePluginEvent.settings || afterPluginEvent.other != beforePluginEvent.other {
		t.Fatalf("plugin snapshot event delta = before %+v after %+v, want one plugin event only", beforePluginEvent, afterPluginEvent)
	}

	enabled := app.plugins.SetEnabled(plugin.ID, true)
	if enabled.Problem != nil {
		t.Fatalf("SetEnabled problem = %+v", enabled.Problem)
	}
	emitter.waitAfter(t, afterPluginEvent.total, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(PluginListResponse)
		return ok && len(response.Plugins) == 1 && response.Plugins[0].ID == plugin.ID && response.Plugins[0].Enabled
	})
	updated := app.plugins.UpdateConfig(plugin.ID, 1, `{"gain":2}`)
	if updated.Problem != nil {
		t.Fatalf("UpdateConfig problem = %+v", updated.Problem)
	}
	config := app.plugins.GetConfig(plugin.ID)
	if config.Problem != nil || config.ConfigRevision != 2 || config.Data != `{"gain":2}` {
		t.Fatalf("updated plugin config = %+v, want immediate revision 2 JSON", config)
	}
	if setCalls, updateCalls, snapshot, stored := backend.mutationState(plugin.ID); setCalls != 1 || updateCalls != 1 || !snapshot.Enabled || snapshot.ConfigRevision != 2 || stored.Revision != 2 || string(stored.Data) != `{"gain":2}` {
		t.Fatalf("backend immediate plugin state = set/update %d/%d snapshot %+v config %+v", setCalls, updateCalls, snapshot, stored)
	}
	emitter.waitAfter(t, afterPluginEvent.total, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(PluginListResponse)
		return ok && len(response.Plugins) == 1 && response.Plugins[0].ID == plugin.ID && response.Plugins[0].Enabled && response.Plugins[0].ConfigRevision == 2
	})

	settingsBefore := app.settings.Get()
	candidate := settingsBefore.Settings.Clone()
	candidate.Avatar.FallbackPath = filepath.Join(temp, "fallback", "avatar.json")
	saved := app.settings.Save(settingsBefore.Revision, candidate)
	if saved.Problem != nil || !saved.RestartRequired || saved.FileRevision != 2 || saved.Settings.Avatar.FallbackPath != candidate.Avatar.FallbackPath {
		t.Fatalf("construction settings save = %+v, want revision 2 and restartRequired", saved)
	}
	emitter.waitAfter(t, 0, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(SettingsResponse)
		return ok && response.FileRevision == 2 && response.Settings.Avatar.FallbackPath == candidate.Avatar.FallbackPath
	})
	factoryMu.Lock()
	if factoryCalls != 1 {
		factoryMu.Unlock()
		t.Fatalf("settings save constructed %d backends, want original backend only", factoryCalls)
	}
	factoryMu.Unlock()
	if starts, _, _ := backend.startupCounts(); starts != 1 {
		t.Fatalf("settings save started backend %d times, want once", starts)
	}

	// Settle the latest plugin event before isolating the Runtime publication.
	emitter.waitAfter(t, 0, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(PluginListResponse)
		return ok && len(response.Plugins) == 1 && response.Plugins[0].ConfigRevision == 2
	})
	beforeRuntimeEvent := emitter.counts()
	backend.publishStatus(application.Status{
		Revision: 2, Lifecycle: application.LifecycleRunning, AvatarID: "avtr_integration",
	})
	emitter.waitAfter(t, beforeRuntimeEvent.total, func(event m7IntegrationEvent) bool {
		response, ok := event.value.(RuntimeResponse)
		return ok && event.name == eventRuntimeStatus && response.Application != nil && response.Application.AvatarID == "avtr_integration"
	})
	afterRuntimeEvent := emitter.counts()
	if afterRuntimeEvent.runtime != beforeRuntimeEvent.runtime+1 || afterRuntimeEvent.plugins != beforeRuntimeEvent.plugins || afterRuntimeEvent.settings != beforeRuntimeEvent.settings || afterRuntimeEvent.other != beforeRuntimeEvent.other {
		t.Fatalf("runtime status event delta = before %+v after %+v, want one runtime event only", beforeRuntimeEvent, afterRuntimeEvent)
	}

	app.shutdown(context.Background())
	app.shutdown(context.Background())
	if closes := backend.closeCount(); closes != 1 {
		t.Fatalf("backend Close calls = %d, want one", closes)
	}
	closed := app.runtime.GetStatus()
	if closed.Phase != "closed" || closed.Application == nil || closed.Application.Lifecycle != "closed" {
		t.Fatalf("runtime after shutdown = %+v, want closed root and backend", closed)
	}
	beforePostClose := emitter.counts()
	backend.publishStatus(application.Status{Revision: 3, Lifecycle: application.LifecycleRunning, AvatarID: "post-close"})
	backend.publishPlugins([]pluginmanager.RuntimeSnapshot{{ID: "post-close", State: pluginmanager.StateRunning}})
	if afterPostClose := emitter.counts(); afterPostClose != beforePostClose {
		t.Fatalf("post-close backend updates emitted events: before %+v after %+v", beforePostClose, afterPostClose)
	}
}

type m7IntegrationBackend struct {
	mu                  sync.Mutex
	status              application.Status
	plugins             []pluginmanager.RuntimeSnapshot
	configs             map[string]pluginapi.Config
	statusUpdates       chan application.Status
	pluginUpdates       chan []pluginmanager.RuntimeSnapshot
	startCalls          int
	closeCalls          int
	statusSubscriptions int
	pluginSubscriptions int
	setEnabledCalls     int
	updateConfigCalls   int
}

func newM7IntegrationBackend() *m7IntegrationBackend {
	return &m7IntegrationBackend{
		status: application.Status{Revision: 1, Lifecycle: application.LifecycleRunning},
		plugins: []pluginmanager.RuntimeSnapshot{{
			ID: "fixture.bootstrap", Name: "Bootstrap", Version: "1.0.0", State: pluginmanager.StateStopped,
		}},
		configs:       make(map[string]pluginapi.Config),
		statusUpdates: make(chan application.Status, 1),
		pluginUpdates: make(chan []pluginmanager.RuntimeSnapshot, 1),
	}
}

func (backend *m7IntegrationBackend) Start(context.Context) error {
	backend.mu.Lock()
	backend.startCalls++
	backend.mu.Unlock()
	return nil
}

func (backend *m7IntegrationBackend) Close(context.Context) error {
	backend.mu.Lock()
	backend.closeCalls++
	backend.status.Lifecycle = application.LifecycleClosed
	backend.status.Revision++
	backend.mu.Unlock()
	return nil
}

func (backend *m7IntegrationBackend) Status() application.Status {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return cloneM7IntegrationStatus(backend.status)
}

func (backend *m7IntegrationBackend) SubscribeStatus(context.Context) <-chan application.Status {
	backend.mu.Lock()
	backend.statusSubscriptions++
	status := cloneM7IntegrationStatus(backend.status)
	backend.mu.Unlock()
	m7OfferLatest(backend.statusUpdates, status)
	return backend.statusUpdates
}

func (backend *m7IntegrationBackend) Plugins() []pluginmanager.RuntimeSnapshot {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return cloneM7IntegrationPlugins(backend.plugins)
}

func (backend *m7IntegrationBackend) PluginConfig(id string) (pluginapi.Config, bool) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	config, ok := backend.configs[id]
	return config.Clone(), ok
}

func (backend *m7IntegrationBackend) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	for index := range backend.plugins {
		if backend.plugins[index].ID == id {
			backend.setEnabledCalls++
			backend.plugins[index].Enabled = enabled
			return nil
		}
	}
	return pluginmanager.ErrUnknownPlugin
}

func (backend *m7IntegrationBackend) UpdatePluginConfig(ctx context.Context, id string, config pluginapi.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	current, ok := backend.configs[id]
	if !ok {
		return pluginmanager.ErrUnknownPlugin
	}
	if config.Revision != current.Revision+1 {
		return pluginmanager.ErrConfigRevisionConflict
	}
	backend.updateConfigCalls++
	backend.configs[id] = config.Clone()
	for index := range backend.plugins {
		if backend.plugins[index].ID == id {
			backend.plugins[index].ConfigRevision = config.Revision
		}
	}
	return nil
}

func (backend *m7IntegrationBackend) SubscribePlugins(context.Context) <-chan []pluginmanager.RuntimeSnapshot {
	backend.mu.Lock()
	backend.pluginSubscriptions++
	plugins := cloneM7IntegrationPlugins(backend.plugins)
	backend.mu.Unlock()
	m7OfferLatest(backend.pluginUpdates, plugins)
	return backend.pluginUpdates
}

func (backend *m7IntegrationBackend) replacePlugin(snapshot pluginmanager.RuntimeSnapshot, config pluginapi.Config) {
	backend.mu.Lock()
	backend.plugins = []pluginmanager.RuntimeSnapshot{snapshot}
	backend.configs = map[string]pluginapi.Config{snapshot.ID: config.Clone()}
	plugins := cloneM7IntegrationPlugins(backend.plugins)
	backend.mu.Unlock()
	m7OfferLatest(backend.pluginUpdates, plugins)
}

func (backend *m7IntegrationBackend) publishStatus(status application.Status) {
	backend.mu.Lock()
	backend.status = cloneM7IntegrationStatus(status)
	owned := cloneM7IntegrationStatus(backend.status)
	backend.mu.Unlock()
	m7OfferLatest(backend.statusUpdates, owned)
}

func (backend *m7IntegrationBackend) publishPlugins(snapshots []pluginmanager.RuntimeSnapshot) {
	backend.mu.Lock()
	backend.plugins = cloneM7IntegrationPlugins(snapshots)
	owned := cloneM7IntegrationPlugins(backend.plugins)
	backend.mu.Unlock()
	m7OfferLatest(backend.pluginUpdates, owned)
}

func (backend *m7IntegrationBackend) startupCounts() (int, int, int) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.startCalls, backend.statusSubscriptions, backend.pluginSubscriptions
}

func (backend *m7IntegrationBackend) mutationState(id string) (int, int, pluginmanager.RuntimeSnapshot, pluginapi.Config) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	snapshot, _ := findM7IntegrationPlugin(backend.plugins, id)
	return backend.setEnabledCalls, backend.updateConfigCalls, snapshot, backend.configs[id].Clone()
}

func (backend *m7IntegrationBackend) closeCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.closeCalls
}

func cloneM7IntegrationStatus(status application.Status) application.Status {
	status.PluginFailures = append([]application.PluginControlFailure(nil), status.PluginFailures...)
	return status
}

func cloneM7IntegrationPlugins(values []pluginmanager.RuntimeSnapshot) []pluginmanager.RuntimeSnapshot {
	return append([]pluginmanager.RuntimeSnapshot(nil), values...)
}

func findM7IntegrationPlugin(values []pluginmanager.RuntimeSnapshot, id string) (pluginmanager.RuntimeSnapshot, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return pluginmanager.RuntimeSnapshot{}, false
}

func m7OfferLatest[T any](out chan T, value T) {
	select {
	case <-out:
	default:
	}
	select {
	case out <- value:
	default:
	}
}

type m7IntegrationEvent struct {
	name  string
	value any
}

type m7IntegrationEventCounts struct {
	total    int
	runtime  int
	plugins  int
	settings int
	other    int
}

type m7IntegrationEmitter struct {
	mu      sync.Mutex
	events  []m7IntegrationEvent
	changed chan struct{}
}

func newM7IntegrationEmitter() *m7IntegrationEmitter {
	return &m7IntegrationEmitter{changed: make(chan struct{})}
}

func (emitter *m7IntegrationEmitter) Emit(_ context.Context, name string, values ...any) {
	var value any
	if len(values) == 1 {
		value = values[0]
	}
	emitter.mu.Lock()
	emitter.events = append(emitter.events, m7IntegrationEvent{name: name, value: value})
	close(emitter.changed)
	emitter.changed = make(chan struct{})
	emitter.mu.Unlock()
}

func (emitter *m7IntegrationEmitter) waitAfter(t *testing.T, start int, predicate func(m7IntegrationEvent) bool) m7IntegrationEvent {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		emitter.mu.Lock()
		for index := start; index < len(emitter.events); index++ {
			event := emitter.events[index]
			if predicate(event) {
				emitter.mu.Unlock()
				return event
			}
		}
		start = len(emitter.events)
		changed := emitter.changed
		emitter.mu.Unlock()
		select {
		case <-changed:
		case <-timer.C:
			t.Fatalf("timed out waiting for integration event after sequence %d", start)
			return m7IntegrationEvent{}
		}
	}
}

func (emitter *m7IntegrationEmitter) counts() m7IntegrationEventCounts {
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	counts := m7IntegrationEventCounts{total: len(emitter.events)}
	for _, event := range emitter.events {
		switch event.name {
		case eventRuntimeStatus:
			counts.runtime++
		case eventPluginsChanged:
			counts.plugins++
		case eventSettingsChanged:
			counts.settings++
		default:
			counts.other++
		}
	}
	return counts
}
