package plugins

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestManagerStartupBuildsFixedSortedRegistryAndPreservesUnavailablePreferences(t *testing.T) {
	catalog := &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.beta"),
		managerTestPlugin("vendor.alpha"),
	}}
	store := newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.alpha": {
			Enabled: true,
			Config:  pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)},
		},
		"vendor.unavailable": {
			Enabled: true,
			Config:  pluginapi.Config{Revision: 9, Data: []byte(`{"kept":true}`)},
		},
	}})
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, catalog, store, factory)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })

	if catalog.scans != 1 || store.loads != 1 {
		t.Fatalf("startup calls = scan %d, load %d; want one each", catalog.scans, store.loads)
	}
	snapshots := manager.List()
	if got := managerSnapshotIDs(snapshots); !reflect.DeepEqual(got, []string{"vendor.alpha", "vendor.beta"}) {
		t.Fatalf("List IDs = %v, want sorted fixed registry", got)
	}
	if !snapshots[0].Enabled || snapshots[0].State != StateStarting {
		t.Fatalf("alpha snapshot = %+v, want enabled starting", snapshots[0])
	}
	if snapshots[1].Enabled || snapshots[1].State != StateDisabled {
		t.Fatalf("beta snapshot = %+v, want disabled", snapshots[1])
	}
	snapshots[0].Name = "caller mutation"
	if fresh, _ := manager.Get("vendor.alpha"); fresh.Name == "caller mutation" {
		t.Fatal("List returned storage shared with Manager snapshots")
	}
	if got := factory.preference("vendor.beta"); got.Enabled || got.Config.Revision != 0 {
		t.Fatalf("missing beta preference = %+v, want disabled zero default", got)
	}
	if err := manager.Start(context.Background()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second Start() error = %v, want ErrInvalidState", err)
	}

	if err := manager.Enable(context.Background(), "vendor.beta"); err != nil {
		t.Fatalf("Enable(beta) error = %v", err)
	}
	saved := store.latest()
	if got := saved.Plugins["vendor.unavailable"]; !got.Enabled || got.Config.Revision != 9 ||
		string(got.Config.Data) != `{"kept":true}` {
		t.Fatalf("unavailable preference after Save = %+v, want preserved", got)
	}

	catalog.plugins = append(catalog.plugins, managerTestPlugin("vendor.late"))
	if _, exists := manager.Get("vendor.late"); exists {
		t.Fatal("registry changed after successful startup scan")
	}
}

func TestManagerStartupRollsBackAndCanRetry(t *testing.T) {
	catalog := &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
		managerTestPlugin("vendor.beta"),
	}}
	store := newManagerTestStore(emptyPluginSettings())
	factory := newManagerTestSupervisorFactory()
	factory.failID = "vendor.beta"
	manager := newManagerForTest(t, catalog, store, factory)

	if err := manager.Start(context.Background()); err == nil {
		t.Fatal("Start() error = nil, want supervisor construction failure")
	}
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("List() after rollback = %+v, want empty", got)
	}
	if !factory.closed("vendor.alpha") {
		t.Fatal("startup rollback did not close already-created supervisor")
	}
	if err := manager.Enable(context.Background(), "vendor.alpha"); !errors.Is(err, ErrManagerNotStarted) {
		t.Fatalf("Enable() after rollback error = %v, want ErrManagerNotStarted", err)
	}

	factory.failID = ""
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("retry Start() error = %v", err)
	}
	if got := managerSnapshotIDs(manager.List()); !reflect.DeepEqual(got, []string{"vendor.alpha", "vendor.beta"}) {
		t.Fatalf("retry List IDs = %v", got)
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestManagerRejectsInvalidDependenciesAndOptions(t *testing.T) {
	validCatalog := &managerTestCatalog{}
	validStore := newManagerTestStore(emptyPluginSettings())
	validLauncher := managerTestLauncher{}
	validSink := managerTestFrameSink{}
	valid := DefaultOptions()

	tests := []struct {
		name     string
		catalog  Catalog
		store    Store
		launcher ProcessLauncher
		sink     FrameSink
		options  Options
	}{
		{name: "catalog", store: validStore, launcher: validLauncher, sink: validSink, options: valid},
		{name: "store", catalog: validCatalog, launcher: validLauncher, sink: validSink, options: valid},
		{name: "launcher", catalog: validCatalog, store: validStore, sink: validSink, options: valid},
		{name: "sink", catalog: validCatalog, store: validStore, launcher: validLauncher, options: valid},
		{name: "handshake timeout", catalog: validCatalog, store: validStore, launcher: validLauncher, sink: validSink, options: mutateManagerOptions(valid, func(o *Options) { o.HandshakeTimeout = 0 })},
		{name: "heartbeat timeout", catalog: validCatalog, store: validStore, launcher: validLauncher, sink: validSink, options: mutateManagerOptions(valid, func(o *Options) { o.HeartbeatTimeout = 0 })},
		{name: "graceful timeout", catalog: validCatalog, store: validStore, launcher: validLauncher, sink: validSink, options: mutateManagerOptions(valid, func(o *Options) { o.GracefulTimeout = 0 })},
		{name: "kill timeout", catalog: validCatalog, store: validStore, launcher: validLauncher, sink: validSink, options: mutateManagerOptions(valid, func(o *Options) { o.KillTimeout = 0 })},
		{name: "control capacity", catalog: validCatalog, store: validStore, launcher: validLauncher, sink: validSink, options: mutateManagerOptions(valid, func(o *Options) { o.ControlCapacity = 0 })},
		{name: "event capacity", catalog: validCatalog, store: validStore, launcher: validLauncher, sink: validSink, options: mutateManagerOptions(valid, func(o *Options) { o.EventCapacity = 0 })},
		{name: "restart policy", catalog: validCatalog, store: validStore, launcher: validLauncher, sink: validSink, options: mutateManagerOptions(valid, func(o *Options) { o.Restart.MaxFailures = 0 })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if manager, err := NewManager(test.catalog, test.store, test.launcher, test.sink, test.options); err == nil || manager != nil {
				t.Fatalf("NewManager() = (%T, %v), want nil error result", manager, err)
			}
		})
	}
}

func TestManagerDefaultOptions(t *testing.T) {
	options := DefaultOptions()
	if options.HandshakeTimeout != 5*time.Second ||
		options.HeartbeatTimeout != 5*time.Second ||
		options.GracefulTimeout != 2*time.Second ||
		options.KillTimeout != 2*time.Second ||
		options.ControlCapacity != 32 ||
		options.EventCapacity != 64 ||
		options.Restart != DefaultRestartPolicy() {
		t.Fatalf("DefaultOptions() = %+v, want v1 defaults", options)
	}
}

func TestManagerCommandsPersistBeforeRuntimeAndRetainSavedIntentOnRuntimeFailure(t *testing.T) {
	store := newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.alpha": {
			Enabled: true,
			Config:  pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)},
		},
	}})
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, store, factory)

	if err := manager.UpdateConfig(context.Background(), "vendor.alpha", pluginapi.Config{}); !errors.Is(err, ErrManagerNotStarted) {
		t.Fatalf("UpdateConfig before Start error = %v, want ErrManagerNotStarted", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	if err := manager.Enable(context.Background(), "vendor.unknown"); !errors.Is(err, ErrUnknownPlugin) {
		t.Fatalf("Enable unknown error = %v, want ErrUnknownPlugin", err)
	}

	runtimeErr := errors.New("runtime config failed")
	supervisor := factory.supervisor("vendor.alpha")
	supervisor.commandHook = func(command supervisorCommand) error {
		if command.kind == supervisorConfig {
			if got := store.latest().Plugins["vendor.alpha"].Config.Revision; got != command.config.Revision {
				t.Errorf("runtime observed Config revision %d before persisted revision %d", command.config.Revision, got)
			}
			return runtimeErr
		}
		return nil
	}
	config := pluginapi.Config{Revision: 2, Data: []byte(`{"gain":2}`)}
	if err := manager.UpdateConfig(context.Background(), "vendor.alpha", config); !errors.Is(err, runtimeErr) {
		t.Fatalf("UpdateConfig runtime error = %v, want %v", err, runtimeErr)
	}
	config.Data[8] = '9'
	saved := store.latest()
	if got := saved.Plugins["vendor.alpha"].Config; got.Revision != 2 || string(got.Data) != `{"gain":2}` {
		t.Fatalf("saved config after runtime failure = %+v, want owned rev2 intent", got)
	}
	if snapshot, _ := manager.Get("vendor.alpha"); snapshot.ConfigRevision != 2 ||
		snapshot.LastError != "plugins: session failed" {
		t.Fatalf("snapshot after runtime failure = %+v, want persisted revision and runtime error", snapshot)
	}

	saveErr := errors.New("save failed")
	store.saveErr = saveErr
	supervisor.commandHook = nil
	if err := manager.UpdateConfig(context.Background(), "vendor.alpha",
		pluginapi.Config{Revision: 3, Data: []byte(`{"gain":3}`)}); !errors.Is(err, saveErr) {
		t.Fatalf("UpdateConfig save error = %v, want %v", err, saveErr)
	}
	if got := supervisor.commandCount(supervisorConfig); got != 1 {
		t.Fatalf("runtime Config commands = %d, want no command after failed Save", got)
	}
	store.saveErr = nil
	if err := manager.Disable(context.Background(), "vendor.alpha"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if got := store.latest().Plugins["vendor.alpha"].Config.Revision; got != 2 {
		t.Fatalf("Config revision after failed-save rollback = %d, want 2", got)
	}
}

func TestManagerRoutesRuntimeControlsAndHonorsCancellationAndBackpressure(t *testing.T) {
	store := newManagerTestStore(emptyPluginSettings())
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, store, factory)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	supervisor := factory.supervisor("vendor.alpha")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Enable(ctx, "vendor.alpha"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Enable(canceled) error = %v, want context.Canceled", err)
	}
	if store.saves != 0 {
		t.Fatalf("Save calls after pre-canceled command = %d, want 0", store.saves)
	}

	subscription := pluginapi.Subscription{}
	if err := manager.UpdateSubscription(context.Background(), "vendor.alpha", subscription); err != nil {
		t.Fatalf("UpdateSubscription() error = %v", err)
	}
	if err := manager.SetActive(context.Background(), "vendor.alpha", false); err != nil {
		t.Fatalf("SetActive() error = %v", err)
	}
	if err := manager.Restart(context.Background(), "vendor.alpha"); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	for _, kind := range []supervisorCommandKind{supervisorSubscription, supervisorActive, supervisorRestart} {
		if got := supervisor.commandCount(kind); got != 1 {
			t.Fatalf("%s command count = %d, want 1", kind, got)
		}
	}

	supervisor.commandHook = func(supervisorCommand) error { return ErrControlBackpressure }
	if err := manager.SetActive(context.Background(), "vendor.alpha", true); !errors.Is(err, ErrControlBackpressure) {
		t.Fatalf("SetActive backpressure error = %v, want ErrControlBackpressure", err)
	}
}

func TestManagerStatusEventCarriesOwnedStatusAndCurrentSnapshot(t *testing.T) {
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, newManagerTestStore(emptyPluginSettings()), factory)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	events := manager.Subscribe(context.Background())

	status := pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: "camera disconnected"}
	factory.supervisor("vendor.alpha").publishStatus(status)
	status.Message = "caller mutation"
	event := receiveManagerEventMatching(t, events, func(event Event) bool {
		return event.Type == EventPluginStatus && event.PluginID == "vendor.alpha"
	})
	if event.Status == nil || event.Status.State != pluginapi.DeviceError ||
		event.Status.Message != "camera disconnected" {
		t.Fatalf("status event payload = %+v, want owned original status", event.Status)
	}
	if event.Snapshot == nil || event.Snapshot.ID != "vendor.alpha" {
		t.Fatalf("status event snapshot = %+v, want current plugin snapshot", event.Snapshot)
	}
	if event.Snapshot.LastError != "" {
		t.Fatalf("status message was copied into LastError: %+v", event.Snapshot)
	}
}

func TestManagerPersistentRuntimeCommandsDoNotBlockOtherPlugins(t *testing.T) {
	store := newManagerTestStore(emptyPluginSettings())
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
		managerTestPlugin("vendor.beta"),
	}}, store, factory)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })

	alphaStarted := make(chan struct{})
	releaseAlpha := make(chan struct{})
	factory.supervisor("vendor.alpha").commandHook = func(command supervisorCommand) error {
		if command.kind == supervisorConfig {
			close(alphaStarted)
			<-releaseAlpha
		}
		return nil
	}
	alphaResult := make(chan error, 1)
	go func() {
		alphaResult <- manager.UpdateConfig(context.Background(), "vendor.alpha",
			pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)})
	}()
	awaitManagerSignal(t, alphaStarted)

	betaResult := make(chan error, 1)
	go func() { betaResult <- manager.Enable(context.Background(), "vendor.beta") }()
	select {
	case err := <-betaResult:
		if err != nil {
			t.Fatalf("Enable(beta) error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		close(releaseAlpha)
		t.Fatal("blocked alpha runtime command held Manager persistence serialization")
	}
	close(releaseAlpha)
	if err := awaitManagerError(t, alphaResult); err != nil {
		t.Fatalf("UpdateConfig(alpha) error = %v", err)
	}
}

func TestManagerSavedIntentReachesSupervisorAfterCallerCancellation(t *testing.T) {
	store := newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.alpha": {
			Enabled: true,
			Config:  pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)},
		},
	}})
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, store, factory)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	store.afterSave = cancel
	err := manager.UpdateConfig(ctx, "vendor.alpha",
		pluginapi.Config{Revision: 2, Data: []byte(`{"gain":2}`)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateConfig() error = %v, want context.Canceled after successful Save", err)
	}
	supervisor := factory.supervisor("vendor.alpha")
	deadline := time.Now().Add(time.Second)
	for supervisor.commandCount(supervisorConfig) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := supervisor.commandCount(supervisorConfig); got != 1 {
		t.Fatalf("supervisor Config commands = %d, want saved intent admitted despite caller cancellation", got)
	}
}

func TestManagerMultiPluginCrashRestartDoesNotInterruptPeerFramesOrControls(t *testing.T) {
	catalog := &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
		managerTestPlugin("vendor.beta"),
	}}
	store := newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.alpha": {Enabled: true},
		"vendor.beta":  {Enabled: true},
	}})
	sessions := newManagerScriptedSessionFactory()
	sink := &managerRecordingFrameSink{}
	options := DefaultOptions()
	options.Restart.InitialBackoff = time.Millisecond
	options.Restart.MaxBackoff = time.Millisecond
	manager, err := newManager(
		catalog,
		store,
		managerTestLauncher{},
		sink,
		options,
		managerDependencies{newSession: sessions.create},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })

	alpha := sessions.await(t, "vendor.alpha", 1)
	beta := sessions.await(t, "vendor.beta", 1)
	alpha.ready()
	beta.ready()
	awaitManagerState(t, manager, "vendor.alpha", StateRunning)
	awaitManagerState(t, manager, "vendor.beta", StateRunning)

	alpha.finish(sessionResult{Err: errors.New("alpha crashed"), Retryable: true})
	restartedAlpha := sessions.await(t, "vendor.alpha", 2)
	restartedAlpha.ready()
	awaitManagerState(t, manager, "vendor.alpha", StateRunning)

	if err := manager.SetActive(context.Background(), "vendor.beta", true); err != nil {
		t.Fatalf("SetActive(beta) during alpha restart error = %v", err)
	}
	if request := beta.awaitControl(t); request.kind != controlActive || !request.state.Active {
		t.Fatalf("beta control = %+v, want Active(true)", request)
	}
	beta.sendFrame(7, trackingmodel.TrackingFrame{})
	if got := sink.count("vendor.beta"); got != 1 {
		t.Fatalf("beta frames = %d, want 1 while alpha restarts", got)
	}
	if got := sessions.count("vendor.beta"); got != 1 {
		t.Fatalf("beta launches = %d, want unaffected original session", got)
	}
}

func TestManagerCloseRejectsControlsStopsConcurrentlyJoinsErrorsAndIsIdempotent(t *testing.T) {
	store := newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.alpha": {Enabled: true},
		"vendor.beta":  {Enabled: true},
	}})
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
		managerTestPlugin("vendor.beta"),
	}}, store, factory)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	events := manager.Subscribe(context.Background())

	alphaErr := errors.New("alpha close")
	betaErr := errors.New("beta close")
	alpha := factory.supervisor("vendor.alpha")
	beta := factory.supervisor("vendor.beta")
	alpha.closeErr, beta.closeErr = alphaErr, betaErr
	alpha.closeGate, beta.closeGate = make(chan struct{}), make(chan struct{})

	closeResult := make(chan error, 1)
	go func() { closeResult <- manager.Close(context.Background()) }()
	awaitManagerSignal(t, alpha.closeStarted)
	awaitManagerSignal(t, beta.closeStarted)
	if err := manager.Restart(context.Background(), "vendor.alpha"); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Restart during Close error = %v, want ErrManagerClosed", err)
	}
	if store.saves != 0 {
		t.Fatalf("Manager Close persisted Enabled=false with %d Save calls", store.saves)
	}

	close(alpha.closeGate)
	close(beta.closeGate)
	err := awaitManagerError(t, closeResult)
	if !errors.Is(err, alphaErr) || !errors.Is(err, betaErr) {
		t.Fatalf("Close() error = %v, want joined alpha and beta errors", err)
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("event subscription remains open after Manager Close")
		}
	case <-time.After(time.Second):
		t.Fatal("event subscription did not close")
	}
	if second := manager.Close(context.Background()); !errors.Is(second, alphaErr) || !errors.Is(second, betaErr) {
		t.Fatalf("second Close() error = %v, want stable joined result", second)
	}
	if alpha.closeCalls != 1 || beta.closeCalls != 1 {
		t.Fatalf("Close calls = alpha %d beta %d, want one each", alpha.closeCalls, beta.closeCalls)
	}
	if err := manager.Enable(context.Background(), "vendor.alpha"); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Enable after Close error = %v, want ErrManagerClosed", err)
	}
	if got := store.latest().Plugins["vendor.alpha"].Enabled; !got {
		t.Fatal("Manager Close changed persisted Enabled preference")
	}
}

func TestManagerCloseObeysCallerContextWhileShutdownContinues(t *testing.T) {
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, newManagerTestStore(emptyPluginSettings()), factory)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	supervisor := factory.supervisor("vendor.alpha")
	supervisor.closeGate = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Close(canceled) error = %v, want context.Canceled", err)
	}
	awaitManagerSignal(t, supervisor.closeStarted)
	close(supervisor.closeGate)
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close after shutdown completes error = %v", err)
	}
}

type managerTestCatalog struct {
	plugins []InstalledPlugin
	err     error
	scans   int
}

func (c *managerTestCatalog) Scan(ctx context.Context) ([]InstalledPlugin, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.scans++
	return append([]InstalledPlugin(nil), c.plugins...), c.err
}

type managerTestStore struct {
	mu        sync.Mutex
	settings  PluginSettings
	loadErr   error
	saveErr   error
	loads     int
	saves     int
	afterSave func()
}

func newManagerTestStore(settings PluginSettings) *managerTestStore {
	return &managerTestStore{settings: clonePluginSettings(settings)}
}

func (s *managerTestStore) Load(ctx context.Context) (PluginSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return PluginSettings{}, err
	}
	s.loads++
	return clonePluginSettings(s.settings), s.loadErr
}

func (s *managerTestStore) Save(ctx context.Context, settings PluginSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.settings = clonePluginSettings(settings)
	if s.afterSave != nil {
		s.afterSave()
	}
	return nil
}

func (s *managerTestStore) latest() PluginSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePluginSettings(s.settings)
}

type managerTestSupervisorFactory struct {
	mu          sync.Mutex
	supervisors map[string]*managerTestSupervisor
	preferences map[string]PluginPreference
	failID      string
}

func newManagerTestSupervisorFactory() *managerTestSupervisorFactory {
	return &managerTestSupervisorFactory{
		supervisors: make(map[string]*managerTestSupervisor),
		preferences: make(map[string]PluginPreference),
	}
}

func (f *managerTestSupervisorFactory) create(config pluginSupervisorConfig) (pluginSupervisor, error) {
	id := config.Plugin.Manifest.ID
	if id == f.failID {
		return nil, errors.New("test supervisor construction failure")
	}
	state := StateDisabled
	if config.Preference.Enabled {
		state = StateStarting
	}
	supervisor := &managerTestSupervisor{
		snapshot: RuntimeSnapshot{
			ID:             id,
			Name:           config.Plugin.Manifest.Name,
			Version:        config.Plugin.Manifest.Version,
			Capabilities:   config.Plugin.Manifest.Capabilities,
			Enabled:        config.Preference.Enabled,
			State:          state,
			ConfigRevision: config.Preference.Config.Revision,
		},
		publish:      config.Publish,
		status:       config.PublishStatus,
		closeStarted: make(chan struct{}, 1),
	}
	f.mu.Lock()
	f.supervisors[id] = supervisor
	f.preferences[id] = PluginPreference{
		Enabled: config.Preference.Enabled,
		Config:  config.Preference.Config.Clone(),
	}
	f.mu.Unlock()
	if config.Publish != nil {
		config.Publish(supervisor.snapshot)
	}
	return supervisor, nil
}

func (f *managerTestSupervisorFactory) supervisor(id string) *managerTestSupervisor {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.supervisors[id]
}

func (f *managerTestSupervisorFactory) preference(id string) PluginPreference {
	f.mu.Lock()
	defer f.mu.Unlock()
	preference := f.preferences[id]
	preference.Config = preference.Config.Clone()
	return preference
}

func (f *managerTestSupervisorFactory) closed(id string) bool {
	supervisor := f.supervisor(id)
	return supervisor != nil && supervisor.closeCalls != 0
}

type managerTestSupervisor struct {
	mu           sync.Mutex
	snapshot     RuntimeSnapshot
	commands     []supervisorCommand
	commandHook  func(supervisorCommand) error
	publish      func(RuntimeSnapshot)
	status       func(pluginapi.DeviceStatus)
	closeStarted chan struct{}
	closeGate    chan struct{}
	closeErr     error
	closeCalls   int
}

func (s *managerTestSupervisor) Command(ctx context.Context, command supervisorCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if command.kind == supervisorConfig {
		command.config = command.config.Clone()
	}
	s.commands = append(s.commands, command)
	hook := s.commandHook
	s.mu.Unlock()
	if hook != nil {
		err := hook(command)
		if err != nil {
			s.mu.Lock()
			if command.kind == supervisorConfig {
				s.snapshot.ConfigRevision = command.config.Revision
			}
			s.snapshot.LastError = sanitizedSupervisorError(err)
			snapshot := s.snapshot
			s.mu.Unlock()
			if s.publish != nil {
				s.publish(snapshot)
			}
			return err
		}
	}
	s.mu.Lock()
	switch command.kind {
	case supervisorEnable:
		s.snapshot.Enabled = true
	case supervisorDisable:
		s.snapshot.Enabled = false
		s.snapshot.State = StateDisabled
	case supervisorConfig:
		s.snapshot.ConfigRevision = command.config.Revision
	case supervisorSubscription:
		s.snapshot.SubscriptionGeneration = command.subscription.Generation
	case supervisorActive:
		s.snapshot.Active = command.active
	}
	snapshot := s.snapshot
	s.mu.Unlock()
	if s.publish != nil {
		s.publish(snapshot)
	}
	return nil
}

func (s *managerTestSupervisor) Snapshot() RuntimeSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

func (s *managerTestSupervisor) Close(context.Context) error {
	s.mu.Lock()
	s.closeCalls++
	gate := s.closeGate
	err := s.closeErr
	s.mu.Unlock()
	select {
	case s.closeStarted <- struct{}{}:
	default:
	}
	if gate != nil {
		<-gate
	}
	return err
}

func (s *managerTestSupervisor) commandCount(kind supervisorCommandKind) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, command := range s.commands {
		if command.kind == kind {
			count++
		}
	}
	return count
}

func (s *managerTestSupervisor) publishStatus(status pluginapi.DeviceStatus) {
	s.mu.Lock()
	publish := s.status
	s.mu.Unlock()
	publish(status)
}

type managerTestLauncher struct{}

func (managerTestLauncher) Start(context.Context, ProcessSpec) (Process, error) {
	return nil, errors.New("manager test launcher is not used")
}

type managerTestFrameSink struct{}

func (managerTestFrameSink) Submit(string, uint64, trackingFrameForManagerTest) {}

// Keep the fake FrameSink method tied to the production type without carrying
// frame values through Manager tests.
type trackingFrameForManagerTest = trackingmodel.TrackingFrame

type managerRecordingFrameSink struct {
	mu     sync.Mutex
	frames map[string]int
}

func (s *managerRecordingFrameSink) Submit(id string, _ uint64, _ trackingmodel.TrackingFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frames == nil {
		s.frames = make(map[string]int)
	}
	s.frames[id]++
}

func (s *managerRecordingFrameSink) count(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frames[id]
}

type managerScriptedSessionFactory struct {
	mu       sync.Mutex
	sessions map[string][]*managerScriptedSession
	notify   chan struct{}
}

func newManagerScriptedSessionFactory() *managerScriptedSessionFactory {
	return &managerScriptedSessionFactory{
		sessions: make(map[string][]*managerScriptedSession),
		notify:   make(chan struct{}, 16),
	}
}

func (f *managerScriptedSessionFactory) create(
	_ context.Context,
	instanceID uint64,
	config sessionConfig,
	dependencies sessionDependencies,
) pluginSession {
	session := &managerScriptedSession{
		instanceID:   instanceID,
		dependencies: dependencies,
		done:         make(chan sessionResult, 1),
		controls:     make(chan controlRequest, 8),
	}
	f.mu.Lock()
	id := config.Plugin.Manifest.ID
	f.sessions[id] = append(f.sessions[id], session)
	f.mu.Unlock()
	f.notify <- struct{}{}
	return session
}

func (f *managerScriptedSessionFactory) await(
	t *testing.T,
	id string,
	count int,
) *managerScriptedSession {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		f.mu.Lock()
		sessions := f.sessions[id]
		if len(sessions) >= count {
			session := sessions[count-1]
			f.mu.Unlock()
			return session
		}
		f.mu.Unlock()
		select {
		case <-f.notify:
		case <-deadline:
			t.Fatalf("timed out waiting for %s session %d", id, count)
		}
	}
}

func (f *managerScriptedSessionFactory) count(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions[id])
}

type managerScriptedSession struct {
	instanceID   uint64
	dependencies sessionDependencies
	done         chan sessionResult
	controls     chan controlRequest
	finishOnce   sync.Once
}

func (s *managerScriptedSession) Control(ctx context.Context, request controlRequest) error {
	select {
	case s.controls <- request:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *managerScriptedSession) Stop(context.Context) error {
	s.finish(sessionResult{})
	return nil
}

func (s *managerScriptedSession) Done() <-chan sessionResult { return s.done }

func (s *managerScriptedSession) ready() {
	s.dependencies.onProcessStarted(s.instanceID, 1234)
	s.dependencies.onReady(s.instanceID)
}

func (s *managerScriptedSession) finish(result sessionResult) {
	s.finishOnce.Do(func() {
		s.done <- result
		close(s.done)
	})
}

func (s *managerScriptedSession) sendFrame(
	generation uint64,
	frame trackingmodel.TrackingFrame,
) {
	s.dependencies.frameSink.Submit("vendor.beta", generation, frame)
}

func (s *managerScriptedSession) awaitControl(t *testing.T) controlRequest {
	t.Helper()
	select {
	case request := <-s.controls:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scripted control")
		return controlRequest{}
	}
}

func newManagerForTest(
	t *testing.T,
	catalog Catalog,
	store Store,
	factory *managerTestSupervisorFactory,
) Manager {
	t.Helper()
	manager, err := newManager(
		catalog,
		store,
		managerTestLauncher{},
		managerTestFrameSink{},
		DefaultOptions(),
		managerDependencies{newSupervisor: factory.create},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}
	return manager
}

func managerTestPlugin(id string) InstalledPlugin {
	manifest := validManifest()
	manifest.ID = id
	manifest.Name = id
	return InstalledPlugin{
		Manifest:   manifest,
		RootDir:    `C:\plugins\` + id,
		Executable: `C:\plugins\` + id + `\plugin.exe`,
	}
}

func managerSnapshotIDs(snapshots []RuntimeSnapshot) []string {
	ids := make([]string, len(snapshots))
	for index, snapshot := range snapshots {
		ids[index] = snapshot.ID
	}
	return ids
}

func mutateManagerOptions(options Options, mutate func(*Options)) Options {
	mutate(&options)
	return options
}

func awaitManagerSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for manager signal")
	}
}

func awaitManagerError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for manager result")
		return nil
	}
}

func awaitManagerState(t *testing.T, manager Manager, id string, want State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if snapshot, exists := manager.Get(id); exists && snapshot.State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	snapshot, _ := manager.Get(id)
	t.Fatalf("%s state = %q, want %q", id, snapshot.State, want)
}

func receiveManagerEventMatching(
	t *testing.T,
	events <-chan Event,
	match func(Event) bool,
) Event {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("event channel closed before matching event")
			}
			if match(event) {
				return event
			}
		case <-deadline:
			t.Fatal("timed out waiting for matching Manager event")
			return Event{}
		}
	}
}
