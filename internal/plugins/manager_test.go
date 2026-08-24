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

func TestManagerConstructionDefersEventHubUntilFirstSubscription(t *testing.T) {
	managerAPI, err := newManager(
		&managerTestCatalog{},
		newManagerTestStore(emptyPluginSettings()),
		managerTestLauncher{},
		managerTestFrameSink{},
		DefaultOptions(),
		managerDependencies{},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}
	manager := managerAPI.(*pluginManager)
	select {
	case <-manager.events.started:
		t.Fatal("event hub started during Manager construction")
	default:
	}

	ctx, cancel := context.WithCancel(context.Background())
	events := manager.Subscribe(ctx)
	select {
	case <-manager.events.started:
	case <-time.After(time.Second):
		t.Fatal("event hub did not start for first subscription")
	}
	cancel()
	waitClosed(t, events)
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestManagerSubscriberBeforeStartReceivesStartupSnapshot(t *testing.T) {
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, newManagerTestStore(emptyPluginSettings()), factory)
	eventCtx, cancelEvents := context.WithCancel(context.Background())
	t.Cleanup(cancelEvents)
	events := manager.Subscribe(eventCtx)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	event := receiveManagerEventMatching(t, events, func(event Event) bool {
		return event.Type == EventPluginDiscovered && event.PluginID == "vendor.alpha"
	})
	if event.Snapshot == nil || event.Snapshot.ID != "vendor.alpha" {
		t.Fatalf("startup discovery snapshot = %+v, want vendor.alpha", event.Snapshot)
	}
}

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

func TestManagerLogIngressLossComposesPerPluginWithoutBlocking(t *testing.T) {
	alpha := managerTestPlugin("vendor.alpha")
	beta := managerTestPlugin("vendor.beta")
	hub := &eventHub{
		publish: make(chan Event, 1),
		done:    make(chan struct{}),
	}
	manager := &pluginManager{
		options:   DefaultOptions(),
		events:    hub,
		lifecycle: managerStarted,
		supervisors: map[string]pluginSupervisor{
			alpha.Manifest.ID: &managerTestSupervisor{},
			beta.Manifest.ID:  &managerTestSupervisor{},
		},
	}
	alphaLog := manager.supervisorConfig(alpha, PluginPreference{}).PublishLog
	betaLog := manager.supervisorConfig(beta, PluginPreference{}).PublishLog

	hub.publish <- Event{Type: EventPluginStatus, PluginID: alpha.Manifest.ID}
	producerReturned := make(chan struct{})
	go func() {
		alphaLog(observedPluginLog{
			Entry:   pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "lost alpha"},
			Dropped: 4,
		})
		close(producerReturned)
	}()
	select {
	case <-producerReturned:
	case <-time.After(time.Second):
		t.Fatal("Manager PublishLog blocked on full event-hub ingress")
	}
	<-hub.publish
	alphaLog(observedPluginLog{
		Entry:   pluginapi.LogEntry{Level: pluginapi.LogWarn, Message: "delivered alpha"},
		Dropped: 2,
	})
	alphaEvent := <-hub.publish
	if alphaEvent.PluginID != alpha.Manifest.ID || alphaEvent.Log.Message != "delivered alpha" || alphaEvent.Dropped != 7 {
		t.Fatalf("alpha event = %+v, want delivered alpha with Dropped 7", alphaEvent)
	}

	hub.publish <- Event{Type: EventPluginStatus, PluginID: beta.Manifest.ID}
	betaLog(observedPluginLog{
		Entry:   pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "lost beta"},
		Dropped: 1,
	})
	<-hub.publish
	alphaLog(observedPluginLog{Entry: pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "alpha independent"}})
	if got := <-hub.publish; got.Dropped != 0 {
		t.Fatalf("alpha independent Dropped = %d, want 0", got.Dropped)
	}
	betaLog(observedPluginLog{Entry: pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "delivered beta"}})
	if got := <-hub.publish; got.Dropped != 2 {
		t.Fatalf("beta composed Dropped = %d, want 2", got.Dropped)
	}

	close(hub.done)
	closedReturned := make(chan struct{})
	go func() {
		var calls sync.WaitGroup
		for _, publish := range []func(observedPluginLog){alphaLog, betaLog} {
			calls.Add(1)
			go func(publish func(observedPluginLog)) {
				defer calls.Done()
				publish(observedPluginLog{Entry: pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "during close"}})
			}(publish)
		}
		calls.Wait()
		close(closedReturned)
	}()
	select {
	case <-closedReturned:
	case <-time.After(time.Second):
		t.Fatal("Manager PublishLog blocked after event-hub close")
	}
}

func TestManagerLogIngressLossDoesNotCrossSessionInstances(t *testing.T) {
	plugin := managerTestPlugin("vendor.alpha")
	hub := &eventHub{publish: make(chan Event, 1), done: make(chan struct{})}
	manager := &pluginManager{
		options:     DefaultOptions(),
		events:      hub,
		lifecycle:   managerStarted,
		supervisors: map[string]pluginSupervisor{plugin.Manifest.ID: nil},
	}
	publishLog := manager.supervisorConfig(plugin, PluginPreference{}).PublishLog

	hub.publish <- Event{Type: EventPluginStatus, PluginID: plugin.Manifest.ID}
	publishLog(observedPluginLog{
		InstanceID: 1,
		Entry:      pluginapi.LogEntry{Level: pluginapi.LogWarn, Message: "lost old session"},
		Dropped:    4,
	})
	<-hub.publish
	publishLog(observedPluginLog{
		InstanceID: 2,
		Entry:      pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "current session"},
		Dropped:    2,
	})

	got := <-hub.publish
	if got.Log == nil || got.Log.Message != "current session" || got.Dropped != 2 {
		t.Fatalf("current-session Manager event = %+v, want Dropped 2 without old-session loss", got)
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

func TestManagerSameIDSupervisorRecreationUsesNewSessionIdentity(t *testing.T) {
	sessions := newManagerScriptedSessionFactory()
	failBeta := true
	manager, err := newManager(
		&managerTestCatalog{plugins: []InstalledPlugin{
			managerTestPlugin("vendor.alpha"),
			managerTestPlugin("vendor.beta"),
		}},
		newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
			"vendor.alpha": {Enabled: true},
		}}),
		managerTestLauncher{},
		&managerRecordingFrameSink{},
		DefaultOptions(),
		managerDependencies{
			newSession: sessions.create,
			newSupervisor: func(config pluginSupervisorConfig) (pluginSupervisor, error) {
				if config.Plugin.Manifest.ID == "vendor.beta" && failBeta {
					failBeta = false
					return nil, errors.New("construct beta once")
				}
				return newPluginSupervisor(config)
			},
		},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}

	if err := manager.Start(context.Background()); err == nil {
		t.Fatal("first Start() error = nil, want beta construction failure")
	}
	first := sessions.await(t, "vendor.alpha", 1)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("retry Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	second := sessions.await(t, "vendor.alpha", 2)

	if first.instanceID == 0 || second.instanceID <= first.instanceID {
		t.Fatalf("same-ID recreated session identities = %d then %d, want positive increase", first.instanceID, second.instanceID)
	}
}

func TestManagerStartupRollsBackWhenCallerCancelsDuringFinalSupervisorConstruction(t *testing.T) {
	catalog := &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
		managerTestPlugin("vendor.beta"),
	}}
	factory := newManagerTestSupervisorFactory()
	ctx, cancel := context.WithCancel(context.Background())
	manager, err := newManager(
		catalog,
		newManagerTestStore(emptyPluginSettings()),
		managerTestLauncher{},
		managerTestFrameSink{},
		DefaultOptions(),
		managerDependencies{newSupervisor: func(config pluginSupervisorConfig) (pluginSupervisor, error) {
			supervisor, err := factory.create(config)
			if config.Plugin.Manifest.ID == "vendor.beta" {
				cancel()
			}
			return supervisor, err
		}},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}

	if err := manager.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("List() after cancellation = %+v, want empty registry", got)
	}
	if !factory.closed("vendor.alpha") || !factory.closed("vendor.beta") {
		t.Fatal("startup cancellation did not close every constructed supervisor")
	}
	if err := manager.Enable(context.Background(), "vendor.alpha"); !errors.Is(err, ErrManagerNotStarted) {
		t.Fatalf("Enable() after cancellation error = %v, want ErrManagerNotStarted", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() with fresh context after cancellation error = %v", err)
	}
	if got := managerSnapshotIDs(manager.List()); !reflect.DeepEqual(got, []string{"vendor.alpha", "vendor.beta"}) {
		t.Fatalf("List() after retry = %v, want rebuilt registry", got)
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close() after retry error = %v", err)
	}
}

func TestManagerStartupCancellationJoinsRollbackError(t *testing.T) {
	catalog := &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
		managerTestPlugin("vendor.beta"),
	}}
	factory := newManagerTestSupervisorFactory()
	rollbackErr := errors.New("rollback close failure")
	ctx, cancel := context.WithCancel(context.Background())
	manager, err := newManager(
		catalog,
		newManagerTestStore(emptyPluginSettings()),
		managerTestLauncher{},
		managerTestFrameSink{},
		DefaultOptions(),
		managerDependencies{newSupervisor: func(config pluginSupervisorConfig) (pluginSupervisor, error) {
			supervisor, err := factory.create(config)
			if config.Plugin.Manifest.ID == "vendor.beta" {
				testSupervisor := supervisor.(*managerTestSupervisor)
				testSupervisor.mu.Lock()
				testSupervisor.closeErr = rollbackErr
				testSupervisor.mu.Unlock()
				cancel()
			}
			return supervisor, err
		}},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
		defer closeCancel()
		if err := manager.Close(closeCtx); err != nil {
			t.Errorf("Close() cleanup error = %v", err)
		}
	})

	err = manager.Start(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("Start() error = %v, want rollback error %v", err, rollbackErr)
	}
	if got := factory.closeCount("vendor.alpha"); got != 1 {
		t.Fatalf("alpha Close() calls = %d, want 1", got)
	}
	if got := factory.closeCount("vendor.beta"); got != 1 {
		t.Fatalf("beta Close() calls = %d, want 1", got)
	}
}

func TestManagerCloseDuringFinalStartupCancellationKeepsClosingLifecycle(t *testing.T) {
	catalog := &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
		managerTestPlugin("vendor.beta"),
	}}
	factory := newManagerTestSupervisorFactory()
	ctx, cancel := context.WithCancel(context.Background())
	finalConstructed := make(chan struct{})
	releaseFinalConstruction := make(chan struct{})
	var releaseFinalOnce sync.Once
	releaseFinal := func() { releaseFinalOnce.Do(func() { close(releaseFinalConstruction) }) }
	t.Cleanup(releaseFinal)
	rollbackCloseGate := make(chan struct{})
	var releaseOnce sync.Once
	releaseRollback := func() { releaseOnce.Do(func() { close(rollbackCloseGate) }) }
	t.Cleanup(releaseRollback)

	manager, err := newManager(
		catalog,
		newManagerTestStore(emptyPluginSettings()),
		managerTestLauncher{},
		managerTestFrameSink{},
		DefaultOptions(),
		managerDependencies{newSupervisor: func(config pluginSupervisorConfig) (pluginSupervisor, error) {
			supervisor, err := factory.create(config)
			if config.Plugin.Manifest.ID != "vendor.beta" {
				return supervisor, err
			}
			for _, id := range []string{"vendor.alpha", "vendor.beta"} {
				testSupervisor := factory.supervisor(id)
				testSupervisor.mu.Lock()
				testSupervisor.closeGate = rollbackCloseGate
				testSupervisor.mu.Unlock()
			}
			cancel()
			close(finalConstructed)
			<-releaseFinalConstruction
			return supervisor, err
		}},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}
	implementation := manager.(*pluginManager)
	implementation.events.Close()
	blockingEvents := &eventHub{
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
		started: make(chan struct{}),
	}
	blockingEvents.startOnce.Do(func() { close(blockingEvents.started) })
	implementation.events = blockingEvents
	var releaseEventsOnce sync.Once
	releaseEvents := func() { releaseEventsOnce.Do(func() { close(blockingEvents.stopped) }) }
	t.Cleanup(releaseEvents)
	startResult := make(chan error, 1)
	go func() { startResult <- manager.Start(ctx) }()
	awaitManagerSignal(t, finalConstructed)

	closeResult := make(chan error, 1)
	go func() { closeResult <- manager.Close(context.Background()) }()
	awaitManagerLifecycle(t, implementation, managerClosing)
	select {
	case err := <-closeResult:
		t.Fatalf("Close() returned before startup completed: %v", err)
	default:
	}

	releaseFinal()
	awaitManagerSignal(t, factory.supervisor("vendor.alpha").closeStarted)
	awaitManagerSignal(t, factory.supervisor("vendor.beta").closeStarted)
	awaitManagerLifecycle(t, implementation, managerClosing)
	select {
	case err := <-startResult:
		t.Fatalf("Start() returned before rollback Close() calls completed: %v", err)
	default:
	}

	releaseRollback()
	if err := awaitManagerError(t, startResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
	awaitManagerLifecycle(t, implementation, managerClosing)
	releaseEvents()
	if err := awaitManagerError(t, closeResult); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	awaitManagerLifecycle(t, implementation, managerClosed)
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("List() after concurrent shutdown = %+v, want empty registry", got)
	}
	if err := manager.Start(context.Background()); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Start() after Close error = %v, want ErrManagerClosed", err)
	}
	for _, id := range []string{"vendor.alpha", "vendor.beta"} {
		if got := factory.closeCount(id); got != 1 {
			t.Fatalf("%s Close() calls = %d, want exactly 1", id, got)
		}
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

func TestManagerFailedActivationCompensationReachesCurrentSession(t *testing.T) {
	sessions := newManagerScriptedSessionFactory()
	manager, err := newManager(
		&managerTestCatalog{plugins: []InstalledPlugin{managerTestPlugin("vendor.alpha")}},
		newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
			"vendor.alpha": {Enabled: true},
		}}),
		managerTestLauncher{},
		&managerRecordingFrameSink{},
		DefaultOptions(),
		managerDependencies{newSession: sessions.create},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })

	session := sessions.await(t, "vendor.alpha", 1)
	session.ready()
	awaitManagerState(t, manager, "vendor.alpha", StateRunning)

	activationErr := errors.New("activation acknowledgement lost")
	session.setControlError(activationErr)
	if err := manager.SetActive(context.Background(), "vendor.alpha", true); !errors.Is(err, activationErr) {
		t.Fatalf("SetActive(true) error = %v, want activation failure", err)
	}
	if request := session.awaitControl(t); request.kind != controlActive || !request.state.Active {
		t.Fatalf("activation request = %+v, want ActiveChanged(true)", request)
	}
	if snapshot, _ := manager.Get("vendor.alpha"); snapshot.Active {
		t.Fatalf("snapshot Active = true after failed activation: %+v", snapshot)
	}

	session.setControlError(nil)
	if err := manager.SetActive(context.Background(), "vendor.alpha", false); err != nil {
		t.Fatalf("compensating SetActive(false) error = %v", err)
	}
	if request := session.awaitControl(t); request.kind != controlActive || request.state.Active || !request.forceActive {
		t.Fatalf("compensating request = %+v, want forced ActiveChanged(false)", request)
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

func TestManagerSamePluginAdmissionDoesNotWaitForRuntimeDelivery(t *testing.T) {
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

	configRuntimeStarted := make(chan struct{})
	releaseConfigRuntime := make(chan struct{})
	restartAdmission := make(chan struct{})
	supervisor.beforeCommand = func(command supervisorCommand) {
		if command.kind == supervisorRestart {
			close(restartAdmission)
		}
	}
	supervisor.commandHook = func(command supervisorCommand) error {
		if command.kind == supervisorConfig {
			close(configRuntimeStarted)
			<-releaseConfigRuntime
		}
		return nil
	}
	configResult := make(chan error, 1)
	go func() {
		configResult <- manager.UpdateConfig(context.Background(), "vendor.alpha",
			pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)})
	}()
	awaitManagerSignal(t, configRuntimeStarted)

	restartResult := make(chan error, 1)
	go func() { restartResult <- manager.Restart(context.Background(), "vendor.alpha") }()
	awaitManagerSignal(t, restartAdmission)
	close(releaseConfigRuntime)
	if err := awaitManagerError(t, configResult); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if err := awaitManagerError(t, restartResult); err != nil {
		t.Fatalf("Restart() error = %v", err)
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

func TestManagerSavedConfigIsAdmittedBeforeConcurrentRestart(t *testing.T) {
	store := newManagerTestStore(PluginSettings{Plugins: map[string]PluginPreference{
		"vendor.alpha": {
			Enabled: true,
			Config:  pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)},
		},
	}})
	sessions := newManagerScriptedSessionFactory()
	admissionGate := make(chan struct{})
	admissionEntered := make(chan struct{})
	options := DefaultOptions()
	manager, err := newManager(
		&managerTestCatalog{plugins: []InstalledPlugin{managerTestPlugin("vendor.alpha")}},
		store,
		managerTestLauncher{},
		&managerRecordingFrameSink{},
		options,
		managerDependencies{
			newSession: sessions.create,
			newSupervisor: func(config pluginSupervisorConfig) (pluginSupervisor, error) {
				supervisor, err := newPluginSupervisor(config)
				if err != nil {
					return nil, err
				}
				return &managerBlockingAdmissionSupervisor{
					pluginSupervisor: supervisor,
					kind:             supervisorConfig,
					entered:          admissionEntered,
					gate:             admissionGate,
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("newManager() error = %v", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	first := sessions.await(t, "vendor.alpha", 1)
	first.ready()
	awaitManagerState(t, manager, "vendor.alpha", StateRunning)

	ctx, cancel := context.WithCancel(context.Background())
	store.afterSave = cancel
	configResult := make(chan error, 1)
	go func() {
		configResult <- manager.UpdateConfig(ctx, "vendor.alpha",
			pluginapi.Config{Revision: 2, Data: []byte(`{"gain":2}`)})
	}()
	awaitManagerSignal(t, admissionEntered)

	returnedBeforeAdmission := false
	select {
	case err := <-configResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("UpdateConfig() early error = %v, want context.Canceled", err)
		}
		returnedBeforeAdmission = true
	case <-time.After(100 * time.Millisecond):
	}

	restartResult := make(chan error, 1)
	go func() { restartResult <- manager.Restart(context.Background(), "vendor.alpha") }()
	var second *managerScriptedSession
	if returnedBeforeAdmission {
		second = sessions.await(t, "vendor.alpha", 2)
		close(admissionGate)
	} else {
		close(admissionGate)
		if err := awaitManagerError(t, configResult); !errors.Is(err, context.Canceled) {
			t.Fatalf("UpdateConfig() error = %v, want context.Canceled after admission", err)
		}
		second = sessions.await(t, "vendor.alpha", 2)
	}
	if got := second.startup.Config; got.Revision != 2 || string(got.Data) != `{"gain":2}` {
		t.Fatalf("Restart Startup.Config = %+v, want saved rev2 admitted before Restart", got)
	}
	if err := awaitManagerError(t, restartResult); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if returnedBeforeAdmission {
		t.Fatal("UpdateConfig returned after Save cancellation before supervisor admission")
	}
}

func TestManagerPersistentSaveOrderMatchesSamePluginAdmissionOrder(t *testing.T) {
	store := newManagerTestStore(emptyPluginSettings())
	store.saveEvents = make(chan PluginSettings, 4)
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, store, factory)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	supervisor := factory.supervisor("vendor.alpha")

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	supervisor.beforeCommand = func(command supervisorCommand) {
		if command.kind == supervisorEnable {
			close(firstEntered)
			<-releaseFirst
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.afterSave = cancel
	firstResult := make(chan error, 1)
	go func() { firstResult <- manager.Enable(ctx, "vendor.alpha") }()
	firstSave := awaitManagerSettings(t, store.saveEvents)
	if !firstSave.Plugins["vendor.alpha"].Enabled {
		t.Fatal("first Save did not persist Enabled=true")
	}
	awaitManagerSignal(t, firstEntered)

	secondResult := make(chan error, 1)
	go func() { secondResult <- manager.Disable(context.Background(), "vendor.alpha") }()
	overtook := false
	select {
	case secondSave := <-store.saveEvents:
		if secondSave.Plugins["vendor.alpha"].Enabled {
			t.Fatal("second Save did not persist Enabled=false")
		}
		overtook = true
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := awaitManagerError(t, firstResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("Enable() error = %v, want context.Canceled after admission", err)
	}
	if !overtook {
		secondSave := awaitManagerSettings(t, store.saveEvents)
		if secondSave.Plugins["vendor.alpha"].Enabled {
			t.Fatal("second Save did not persist Enabled=false")
		}
	}
	if err := awaitManagerError(t, secondResult); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if got := supervisor.commandKinds(); !reflect.DeepEqual(got, []supervisorCommandKind{
		supervisorEnable,
		supervisorDisable,
	}) {
		t.Fatalf("supervisor admission order = %v, want [enable disable]", got)
	}
	if overtook {
		t.Fatal("second persistent Save overtook first command admission")
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
	eventCtx, cancelEvents := context.WithCancel(context.Background())
	t.Cleanup(cancelEvents)
	events := manager.Subscribe(eventCtx)
	frameAt := time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC)
	beta.sendFrame(7, trackingmodel.TrackingFrame{}, frameAt, 100)
	if got := sink.count("vendor.beta"); got != 1 {
		t.Fatalf("beta frames = %d, want 1 while alpha restarts", got)
	}
	frameEvent := receiveManagerEventMatching(t, events, func(event Event) bool {
		return event.PluginID == "vendor.beta" &&
			event.Snapshot != nil &&
			event.Snapshot.LastFrameAt == frameAt
	})
	if frameEvent.Snapshot.FrameRate != 100 {
		t.Fatalf("beta frame rate = %v, want 100", frameEvent.Snapshot.FrameRate)
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
			t.Fatal("event subscription produced an event after Manager Close")
		}
	default:
		t.Fatal("event subscription was not closed when Manager Close returned")
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

func TestManagerPersistentCommandReturnsAdmissionTokenWhenCloseWinsSecondLifecycleCheck(
	t *testing.T,
) {
	factory := newManagerTestSupervisorFactory()
	manager := newManagerForTest(t, &managerTestCatalog{plugins: []InstalledPlugin{
		managerTestPlugin("vendor.alpha"),
	}}, newManagerTestStore(emptyPluginSettings()), factory).(*pluginManager)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })

	manager.mu.RLock()
	admissionToken := manager.admissions["vendor.alpha"]
	manager.mu.RUnlock()

	<-manager.persistToken
	persistTokenHeld := true
	releasePersistToken := func() {
		if persistTokenHeld {
			manager.persistToken <- struct{}{}
			persistTokenHeld = false
		}
	}
	defer releasePersistToken()

	commandResult := make(chan error, 1)
	go func() {
		commandResult <- manager.Enable(context.Background(), "vendor.alpha")
	}()
	awaitManagerTokenTaken(t, admissionToken)

	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	releasePersistToken()

	if err := awaitManagerError(t, commandResult); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Enable() error = %v, want ErrManagerClosed", err)
	}
	select {
	case <-admissionToken:
		admissionToken <- struct{}{}
	default:
		t.Fatal("persistent command did not return its original admission token")
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
	mu         sync.Mutex
	settings   PluginSettings
	loadErr    error
	saveErr    error
	loads      int
	saves      int
	afterSave  func()
	saveEvents chan PluginSettings
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
	if s.saveEvents != nil {
		s.saveEvents <- clonePluginSettings(settings)
	}
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
	return f.closeCount(id) != 0
}

func (f *managerTestSupervisorFactory) closeCount(id string) int {
	supervisor := f.supervisor(id)
	if supervisor == nil {
		return 0
	}
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	return supervisor.closeCalls
}

type managerTestSupervisor struct {
	mu            sync.Mutex
	snapshot      RuntimeSnapshot
	commands      []supervisorCommand
	commandHook   func(supervisorCommand) error
	beforeCommand func(supervisorCommand)
	publish       func(RuntimeSnapshot)
	status        func(pluginapi.DeviceStatus)
	closeStarted  chan struct{}
	closeGate     chan struct{}
	closeErr      error
	closeCalls    int
}

func (s *managerTestSupervisor) Command(ctx context.Context, command supervisorCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	before := s.beforeCommand
	s.mu.Unlock()
	if before != nil {
		before(command)
	}
	s.mu.Lock()
	if command.kind == supervisorConfig {
		command.config = command.config.Clone()
	}
	s.commands = append(s.commands, command)
	hook := s.commandHook
	s.mu.Unlock()
	command.signalAdmission(nil)
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

func (s *managerTestSupervisor) commandKinds() []supervisorCommandKind {
	s.mu.Lock()
	defer s.mu.Unlock()
	kinds := make([]supervisorCommandKind, len(s.commands))
	for index, command := range s.commands {
		kinds[index] = command.kind
	}
	return kinds
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
		descriptor: pluginapi.Descriptor{
			APIVersion:   pluginapi.APIVersion,
			ID:           config.Plugin.Manifest.ID,
			Name:         config.Plugin.Manifest.Name,
			Version:      config.Plugin.Manifest.Version,
			Description:  config.Plugin.Manifest.Description,
			Capabilities: config.Plugin.Manifest.Capabilities,
		},
		startup:  cloneStartup(config.Startup),
		done:     make(chan sessionResult, 1),
		controls: make(chan controlRequest, 8),
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
	mu           sync.Mutex
	instanceID   uint64
	dependencies sessionDependencies
	descriptor   pluginapi.Descriptor
	startup      pluginapi.Startup
	done         chan sessionResult
	controls     chan controlRequest
	finishOnce   sync.Once
	controlErr   error
}

func (s *managerScriptedSession) Control(ctx context.Context, request controlRequest) error {
	select {
	case s.controls <- request:
		s.mu.Lock()
		err := s.controlErr
		s.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *managerScriptedSession) setControlError(err error) {
	s.mu.Lock()
	s.controlErr = err
	s.mu.Unlock()
}

func (s *managerScriptedSession) Stop(context.Context) error {
	s.finish(sessionResult{})
	return nil
}

func (s *managerScriptedSession) Done() <-chan sessionResult { return s.done }

func (s *managerScriptedSession) ready() {
	s.dependencies.onProcessStarted(s.instanceID, 1234)
	s.dependencies.onReady(s.instanceID, s.descriptor)
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
	at time.Time,
	rate float64,
) {
	s.dependencies.frameSink.Submit("vendor.beta", generation, frame)
	if s.dependencies.onFrame != nil {
		s.dependencies.onFrame(s.instanceID, at, rate)
	}
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

func awaitManagerLifecycle(t *testing.T, manager *pluginManager, want managerLifecycle) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		got := manager.lifecycle
		manager.mu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	manager.mu.RLock()
	got := manager.lifecycle
	manager.mu.RUnlock()
	t.Fatalf("manager lifecycle = %d, want %d", got, want)
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

func awaitManagerSettings(t *testing.T, settings <-chan PluginSettings) PluginSettings {
	t.Helper()
	select {
	case value := <-settings:
		return clonePluginSettings(value)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for saved settings")
		return PluginSettings{}
	}
}

func awaitManagerTokenTaken(t *testing.T, token chan struct{}) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(token) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for manager admission token")
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

type managerBlockingAdmissionSupervisor struct {
	pluginSupervisor
	kind    supervisorCommandKind
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func (s *managerBlockingAdmissionSupervisor) Command(
	ctx context.Context,
	command supervisorCommand,
) error {
	if command.kind == s.kind {
		s.once.Do(func() { close(s.entered) })
		select {
		case <-s.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.pluginSupervisor.Command(ctx, command)
}
