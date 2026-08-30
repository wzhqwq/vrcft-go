package main

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func TestAppNewIsPassiveAndOwnsOnlyAPIs(t *testing.T) {
	harness := newRootAppHarness(t)
	app := newAppWithDependencies(harness.dependencies())

	if app == nil || app.runtime == nil || app.plugins == nil || app.settings == nil {
		t.Fatalf("newAppWithDependencies() = %#v, want three passive APIs", app)
	}
	if app.backend != nil || app.backendOps != nil {
		t.Fatalf("passive backend ownership = (%#v, %#v), want nil", app.backend, app.backendOps)
	}
	if app.forwarders != nil {
		t.Fatal("passive construction started event forwarders")
	}
	if got := harness.calls(); len(got) != 0 {
		t.Fatalf("passive construction dependency calls = %v, want none", got)
	}
}

func TestAppStartupUnsupportedPlatformEntersDiagnosticWithoutIO(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.goos = "linux"
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })

	status := app.runtime.GetStatus()
	if status.Phase != "diagnostic" || status.PlatformSupported || status.Problem == nil || status.Problem.Code != ProblemUnsupportedPlatform {
		t.Fatalf("runtime status = %+v, want unsupported diagnostic", status)
	}
	if app.forwarders == nil {
		t.Fatal("unsupported startup did not start diagnostic event forwarding")
	}
	if harness.environmentCalls != 0 || harness.storeFactoryCalls != 0 || harness.configCalls != 0 || harness.backendFactoryCalls != 0 {
		t.Fatalf("unsupported startup performed I/O: environment %d store %d config %d backend %d", harness.environmentCalls, harness.storeFactoryCalls, harness.configCalls, harness.backendFactoryCalls)
	}
}

func TestAppStartupMissingSettingsLoadsCreatesAndStartsExactlyOneBackend(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.store.missing = true
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })

	if got := app.runtime.GetStatus(); got.Phase != "running" || got.Problem != nil || got.Application == nil {
		t.Fatalf("runtime status = %+v, want running backend status", got)
	}
	if harness.store.loadCalls != 1 || !harness.store.createdDefaults {
		t.Fatalf("LoadOrCreate calls/default creation = %d/%t, want 1/true", harness.store.loadCalls, harness.store.createdDefaults)
	}
	if harness.backend.startCalls != 1 || harness.backendFactoryCalls != 1 {
		t.Fatalf("backend factory/start calls = %d/%d, want 1/1", harness.backendFactoryCalls, harness.backend.startCalls)
	}
	if harness.backend.statusSubscriptions != 1 || harness.backend.pluginSubscriptions != 1 {
		t.Fatalf("backend subscriptions = status %d plugins %d, want 1 each", harness.backend.statusSubscriptions, harness.backend.pluginSubscriptions)
	}
	app.startup(context.Background())
	if harness.backendFactoryCalls != 1 || harness.backend.startCalls != 1 {
		t.Fatalf("repeated startup constructed/started again: factory %d start %d", harness.backendFactoryCalls, harness.backend.startCalls)
	}
}

func TestAppStartupInvalidSettingsPreservesDocumentAndSkipsBackend(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.store.loaded.Settings = nil
	harness.store.loaded.Invalid = true
	harness.store.loaded.Diagnostic = errors.New("invalid settings containing private bytes")
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })

	if got := app.runtime.GetStatus(); got.Phase != "diagnostic" || got.Problem == nil {
		t.Fatalf("runtime status = %+v, want invalid-settings diagnostic", got)
	}
	if harness.store.saveCalls != 0 || harness.configCalls != 0 || harness.backendFactoryCalls != 0 {
		t.Fatalf("invalid settings mutated/converted/constructed: save %d config %d backend %d", harness.store.saveCalls, harness.configCalls, harness.backendFactoryCalls)
	}
	if got := app.settings.Get(); got.Problem == nil || got.Problem.Message == harness.store.loaded.Diagnostic.Error() {
		t.Fatalf("settings diagnostic = %+v, want sanitized problem", got.Problem)
	}
}

func TestAppStartupConfigConversionFailureStartsForwardersFirst(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.configErr = &userconfig.ValidationError{Field: "avatar.oscRoot", Err: errors.New("required")}
	var app *App
	harness.onConfig = func() {
		if app.forwarders == nil {
			t.Error("configuration conversion ran before event forwarders started")
		}
	}
	app = newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })

	if got := app.runtime.GetStatus(); got.Phase != "diagnostic" || got.Problem == nil || got.Problem.Code != ProblemValidation {
		t.Fatalf("runtime status = %+v, want validation diagnostic", got)
	}
	if harness.backendFactoryCalls != 0 {
		t.Fatalf("backend factory calls = %d, want zero", harness.backendFactoryCalls)
	}
}

func TestAppStartupBackendConstructionFailureEntersDiagnostic(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.backendErr = errors.New("constructor leaked private path")
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })

	status := app.runtime.GetStatus()
	if status.Phase != "diagnostic" || status.Problem == nil || status.Problem.Code != ProblemInternal || status.Problem.Message == harness.backendErr.Error() {
		t.Fatalf("runtime status = %+v, want sanitized construction diagnostic", status)
	}
	if harness.backendFactoryCalls != 1 || harness.backend.startCalls != 0 || app.backendOps != nil {
		t.Fatalf("construction failure ownership/calls = factory %d start %d ops %#v", harness.backendFactoryCalls, harness.backend.startCalls, app.backendOps)
	}
}

func TestAppStartupStartFailureRetainsBackendForShutdown(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.backend.startErr = errors.New("start failed with private detail")
	harness.backend.status.Lifecycle = application.LifecycleDegraded
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())

	status := app.runtime.GetStatus()
	if status.Phase != "diagnostic" || status.Application == nil || status.Application.Lifecycle != string(application.LifecycleDegraded) {
		t.Fatalf("runtime status = %+v, want retained degraded backend", status)
	}
	if app.backendOps != harness.backend || harness.backend.statusSubscriptions != 0 || harness.backend.pluginSubscriptions != 0 {
		t.Fatalf("failed Start ownership/subscriptions = ops %T status %d plugins %d", app.backendOps, harness.backend.statusSubscriptions, harness.backend.pluginSubscriptions)
	}
	app.shutdown(context.Background())
	if harness.backend.closeCalls != 1 {
		t.Fatalf("Close calls after failed Start = %d, want 1", harness.backend.closeCalls)
	}
}

func TestAppStartupDoesNotAttachConsumersUntilStartSucceeds(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.backend.onStart = func() {
		if harness.backend.statusSubscriptions != 0 || harness.backend.pluginSubscriptions != 0 {
			t.Errorf("subscriptions during Start = status %d plugins %d", harness.backend.statusSubscriptions, harness.backend.pluginSubscriptions)
		}
	}
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })

	if got := app.plugins.List(); got.Problem != nil {
		t.Fatalf("Plugins.List() after successful Start problem = %+v", got.Problem)
	}
	if harness.backend.statusSubscriptions != 1 || harness.backend.pluginSubscriptions != 1 {
		t.Fatalf("post-Start subscriptions = status %d plugins %d, want 1 each", harness.backend.statusSubscriptions, harness.backend.pluginSubscriptions)
	}
}

func TestAppShutdownBeforeStartupIsFinalAndPreventsAllWork(t *testing.T) {
	harness := newRootAppHarness(t)
	app := newAppWithDependencies(harness.dependencies())

	app.shutdown(context.Background())
	first := app.runtime.GetStatus()
	app.shutdown(context.Background())
	app.startup(context.Background())
	second := app.runtime.GetStatus()

	if first.Phase != "closed" || first.Problem != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated pre-start shutdown statuses = %+v then %+v, want identical closed success", first, second)
	}
	if got := harness.calls(); len(got) != 0 {
		t.Fatalf("startup after shutdown dependency calls = %v, want none", got)
	}
}

func TestAppShutdownStopsAdmissionForwardersAndConsumersBeforeClose(t *testing.T) {
	harness := newRootAppHarness(t)
	app := newAppWithDependencies(harness.dependencies())
	harness.backend.onClose = func(context.Context) {
		if got := app.plugins.List(); got.Problem == nil || got.Problem.Code != ProblemUnavailable {
			t.Errorf("plugin admission remained available during Close: %+v", got)
		}
		select {
		case <-app.forwarders.done:
		default:
			t.Error("backend Close ran before event forwarders joined")
		}
	}
	app.startup(context.Background())
	app.shutdown(context.Background())

	if calls := harness.backend.closeCallCount(); calls != 1 {
		t.Fatalf("Close calls = %d, want 1", calls)
	}
	if got := app.runtime.GetStatus(); got.Phase != "closed" || got.Problem != nil {
		t.Fatalf("final runtime status = %+v, want closed success", got)
	}
	if got := app.settings.Get(); got.Problem == nil || got.Problem.Code != ProblemUnavailable {
		t.Fatalf("final settings status = %+v, want unavailable", got)
	}
}

func TestAppShutdownUsesFreshBackgroundTimeoutAfterWailsCancellation(t *testing.T) {
	harness := newRootAppHarness(t)
	dependencies := harness.dependencies()
	dependencies.shutdownTimeout = 80 * time.Millisecond
	app := newAppWithDependencies(dependencies)
	wailsCtx, cancelWails := context.WithCancel(context.Background())
	app.startup(wailsCtx)
	cancelWails()

	harness.backend.closeFunc = func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			t.Errorf("Close context started canceled: %v", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("Close context has no fixed deadline")
		} else if remaining := time.Until(deadline); remaining <= 0 || remaining > dependencies.shutdownTimeout {
			t.Errorf("Close deadline remaining = %v, want (0,%v]", remaining, dependencies.shutdownTimeout)
		}
		return nil
	}
	app.shutdown(wailsCtx)

	if calls := harness.backend.closeCallCount(); calls != 1 {
		t.Fatalf("Close calls = %d, want 1", calls)
	}
}

func TestAppShutdownTimeoutIsSanitizedAndRecordedOnce(t *testing.T) {
	harness := newRootAppHarness(t)
	dependencies := harness.dependencies()
	dependencies.shutdownTimeout = 20 * time.Millisecond
	app := newAppWithDependencies(dependencies)
	app.startup(context.Background())
	harness.backend.closeFunc = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}

	app.shutdown(context.Background())
	first := app.runtime.GetStatus()
	app.shutdown(context.Background())
	second := app.runtime.GetStatus()

	if first.Phase != "closed" || first.Problem == nil || first.Problem.Code != ProblemTimeout || !reflect.DeepEqual(first, second) {
		t.Fatalf("timeout shutdown statuses = %+v then %+v, want identical timeout result", first, second)
	}
	if calls := harness.backend.closeCallCount(); calls != 1 {
		t.Fatalf("timeout Close calls = %d, want 1", calls)
	}
}

func TestAppConcurrentRepeatedShutdownClosesAtMostOnce(t *testing.T) {
	harness := newRootAppHarness(t)
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())

	const callers = 16
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			app.shutdown(context.Background())
		}()
	}
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("concurrent shutdown callers deadlocked")
	}
	if calls := harness.backend.closeCallCount(); calls != 1 {
		t.Fatalf("concurrent Close calls = %d, want 1", calls)
	}
}

func TestAppShutdownWaitsForAdmittedStartupBeforePublishingClosed(t *testing.T) {
	harness := newRootAppHarness(t)
	dependencies := harness.dependencies()
	environmentEntered := make(chan struct{})
	releaseEnvironment := make(chan struct{})
	baseEnvironment := dependencies.environment
	dependencies.environment = func() (userconfig.Environment, error) {
		close(environmentEntered)
		<-releaseEnvironment
		return baseEnvironment()
	}
	app := newAppWithDependencies(dependencies)
	startupDone := make(chan struct{})
	go func() {
		app.startup(context.Background())
		close(startupDone)
	}()
	select {
	case <-environmentEntered:
	case <-time.After(time.Second):
		t.Fatal("startup did not reach environment dependency")
	}

	startupCtx := app.processCtx
	shutdownDone := make(chan struct{})
	go func() {
		app.shutdown(context.Background())
		close(shutdownDone)
	}()
	select {
	case <-startupCtx.Done():
	case <-time.After(time.Second):
		close(releaseEnvironment)
		t.Fatal("shutdown did not cancel the admitted startup")
	}
	select {
	case <-shutdownDone:
		close(releaseEnvironment)
		t.Fatal("shutdown returned before admitted startup exited")
	default:
	}
	if got := app.runtime.GetStatus(); got.Phase == "closed" {
		close(releaseEnvironment)
		t.Fatal("root published closed before admitted startup exited")
	}
	close(releaseEnvironment)
	select {
	case <-startupDone:
	case <-time.After(time.Second):
		t.Fatal("startup did not return after shutdown")
	}
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after admitted startup exited")
	}
	if harness.storeFactoryCalls != 0 || harness.configCalls != 0 || harness.backendFactoryCalls != 0 {
		t.Fatalf("post-close startup work = store %d config %d backend %d, want zero", harness.storeFactoryCalls, harness.configCalls, harness.backendFactoryCalls)
	}
	if got := app.runtime.GetStatus(); got.Phase != "closed" {
		t.Fatalf("final runtime phase = %q, want closed", got.Phase)
	}
}

func TestAppShutdownCancelsWhileFactoryRunsThenClosesDeliveredBackendWithoutStart(t *testing.T) {
	harness := newRootAppHarness(t)
	dependencies := harness.dependencies()
	baseFactory := dependencies.newBackend
	factoryEntered := make(chan struct{})
	releaseFactory := make(chan struct{})
	dependencies.newBackend = func(config application.Config) (*application.Application, backendOperations, error) {
		close(factoryEntered)
		<-releaseFactory
		return baseFactory(config)
	}
	app := newAppWithDependencies(dependencies)
	startupDone := make(chan struct{})
	go func() {
		app.startup(context.Background())
		close(startupDone)
	}()
	select {
	case <-factoryEntered:
	case <-time.After(time.Second):
		t.Fatal("startup did not reach backend factory")
	}

	startupCtx := app.processCtx
	shutdownDone := make(chan struct{})
	go func() {
		app.shutdown(context.Background())
		close(shutdownDone)
	}()
	select {
	case <-startupCtx.Done():
	case <-time.After(100 * time.Millisecond):
		close(releaseFactory)
		<-startupDone
		<-shutdownDone
		t.Fatal("blocking factory held the root lock and prevented shutdown cancellation")
	}
	rootLockAvailable := make(chan struct{})
	go func() {
		app.mu.Lock()
		close(rootLockAvailable)
		app.mu.Unlock()
	}()
	select {
	case <-rootLockAvailable:
	case <-time.After(time.Second):
		close(releaseFactory)
		<-startupDone
		<-shutdownDone
		t.Fatal("blocking factory held the root lifecycle lock")
	}
	select {
	case <-shutdownDone:
		close(releaseFactory)
		t.Fatal("shutdown returned before factory ownership delivery")
	default:
	}
	close(releaseFactory)
	select {
	case <-startupDone:
	case <-time.After(time.Second):
		t.Fatal("startup did not finish after factory returned")
	}
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after factory ownership delivery")
	}
	if harness.backend.startCallCount() != 0 || harness.backend.closeCallCount() != 1 {
		t.Fatalf("factory/shutdown lifecycle = Start %d Close %d, want 0/1", harness.backend.startCallCount(), harness.backend.closeCallCount())
	}
	if app.backendOps != harness.backend {
		t.Fatalf("delivered backend operations = %T, want retained fake backend", app.backendOps)
	}
	if got := app.runtime.GetStatus(); got.Phase != "closed" {
		t.Fatalf("final runtime phase = %q, want closed", got.Phase)
	}
}

func TestAppShutdownJoinsAdmittedUnsupportedPublicationBeforeClosed(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.goos = "linux"
	dependencies := harness.dependencies()
	barrier := newRootStartupBarrier("unsupported")
	dependencies.startupBoundary = barrier.wait
	app := newAppWithDependencies(dependencies)
	startupDone := make(chan struct{})
	go func() {
		app.startup(context.Background())
		close(startupDone)
	}()
	barrier.waitUntilEntered(t)
	startupCtx := app.processCtx
	shutdownDone := startRootShutdown(app)
	waitRootContextCanceled(t, startupCtx)
	assertRootShutdownWaiting(t, shutdownDone, "unsupported publication")

	barrier.release()
	waitRootOperationDone(t, startupDone, "unsupported startup")
	waitRootOperationDone(t, shutdownDone, "shutdown")
	first := app.runtime.GetStatus()
	second := app.runtime.GetStatus()
	if first.Phase != "closed" || first.Revision != second.Revision {
		t.Fatalf("post-join runtime statuses = %+v then %+v, want stable closed revision", first, second)
	}
}

func TestAppShutdownJoinsAdmittedSettingsAttachBeforeClosed(t *testing.T) {
	harness := newRootAppHarness(t)
	dependencies := harness.dependencies()
	barrier := newRootStartupBarrier("settings_attach")
	dependencies.startupBoundary = barrier.wait
	app := newAppWithDependencies(dependencies)
	startupDone := make(chan struct{})
	go func() {
		app.startup(context.Background())
		close(startupDone)
	}()
	barrier.waitUntilEntered(t)
	shutdownDone := startRootShutdown(app)
	waitRootContextCanceled(t, app.processCtx)
	assertRootShutdownWaiting(t, shutdownDone, "settings attach")

	barrier.release()
	waitRootOperationDone(t, startupDone, "settings startup")
	waitRootOperationDone(t, shutdownDone, "shutdown")
	if _, err := app.settingsIO.current(); err != nil {
		t.Fatalf("admitted settings attach did not deliver its store before shutdown: %v", err)
	}
	if harness.store.loadCalls != 0 || harness.configCalls != 0 || harness.backendFactoryCalls != 0 {
		t.Fatalf("post-cancel work after settings attach = load %d config %d backend %d, want zero", harness.store.loadCalls, harness.configCalls, harness.backendFactoryCalls)
	}
	if got := app.runtime.GetStatus(); got.Phase != "closed" {
		t.Fatalf("final runtime phase = %q, want closed", got.Phase)
	}
}

func TestAppShutdownJoinsAdmittedConfigConversionBeforeClosed(t *testing.T) {
	harness := newRootAppHarness(t)
	dependencies := harness.dependencies()
	barrier := newRootStartupBarrier("application_config")
	dependencies.startupBoundary = barrier.wait
	app := newAppWithDependencies(dependencies)
	startupDone := make(chan struct{})
	go func() {
		app.startup(context.Background())
		close(startupDone)
	}()
	barrier.waitUntilEntered(t)
	shutdownDone := startRootShutdown(app)
	waitRootContextCanceled(t, app.processCtx)
	assertRootShutdownWaiting(t, shutdownDone, "config conversion")

	barrier.release()
	waitRootOperationDone(t, startupDone, "config startup")
	waitRootOperationDone(t, shutdownDone, "shutdown")
	if harness.configCalls != 1 || harness.backendFactoryCalls != 0 {
		t.Fatalf("admitted config/post-cancel backend calls = %d/%d, want 1/0", harness.configCalls, harness.backendFactoryCalls)
	}
	if got := app.runtime.GetStatus(); got.Phase != "closed" {
		t.Fatalf("final runtime phase = %q, want closed", got.Phase)
	}
}

func TestAppShutdownDuringBackendStartCancelsAndClosesRetainedBackend(t *testing.T) {
	harness := newRootAppHarness(t)
	startEntered := make(chan struct{})
	startCanceled := make(chan struct{})
	allowStartReturn := make(chan struct{})
	startReturned := make(chan struct{})
	harness.backend.startFunc = func(ctx context.Context) error {
		close(startEntered)
		<-ctx.Done()
		close(startCanceled)
		<-allowStartReturn
		close(startReturned)
		return ctx.Err()
	}
	harness.backend.onClose = func(context.Context) {
		select {
		case <-startReturned:
		default:
			t.Error("backend Close ran before admitted Start returned")
		}
	}
	app := newAppWithDependencies(harness.dependencies())
	startupDone := make(chan struct{})
	go func() {
		app.startup(context.Background())
		close(startupDone)
	}()
	select {
	case <-startEntered:
	case <-time.After(time.Second):
		t.Fatal("startup did not reach backend Start")
	}

	shutdownDone := startRootShutdown(app)
	select {
	case <-startCanceled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel backend Start")
	}
	if calls := harness.backend.closeCallCount(); calls != 0 {
		close(allowStartReturn)
		waitRootOperationDone(t, startupDone, "startup")
		waitRootOperationDone(t, shutdownDone, "shutdown")
		t.Fatalf("Close calls while Start remained admitted = %d, want zero", calls)
	}
	assertRootShutdownWaiting(t, shutdownDone, "backend Start")
	close(allowStartReturn)
	select {
	case <-startupDone:
	case <-time.After(time.Second):
		t.Fatal("canceled startup did not return")
	}
	waitRootOperationDone(t, shutdownDone, "shutdown")
	if harness.backendFactoryCalls != 1 || harness.backend.closeCallCount() != 1 {
		t.Fatalf("startup/shutdown ownership = factory %d Close %d, want 1/1", harness.backendFactoryCalls, harness.backend.closeCallCount())
	}
	if harness.backend.statusSubscriptionCount() != 0 || harness.backend.pluginSubscriptionCount() != 0 {
		t.Fatalf("failed concurrent Start attached consumers: status %d plugins %d", harness.backend.statusSubscriptionCount(), harness.backend.pluginSubscriptionCount())
	}
	if got := app.runtime.GetStatus(); got.Phase != "closed" {
		t.Fatalf("final runtime phase = %q, want closed", got.Phase)
	}
}

type rootAppHarness struct {
	t          *testing.T
	mu         sync.Mutex
	trace      []string
	goos       string
	paths      userconfig.Paths
	store      *fakeRootSettingsStore
	backend    *fakeRootBackend
	configErr  error
	backendErr error
	onConfig   func()

	environmentCalls    int
	storeFactoryCalls   int
	configCalls         int
	backendFactoryCalls int
}

func newRootAppHarness(t *testing.T) *rootAppHarness {
	t.Helper()
	paths := userconfig.Paths{
		SettingsDir:      `C:\Users\Tester\AppData\Roaming\vrcft-go`,
		SettingsFile:     `C:\Users\Tester\AppData\Roaming\vrcft-go\config.json`,
		PluginStoreFile:  `C:\Users\Tester\AppData\Roaming\vrcft-go\plugins.json`,
		BuiltinPluginDir: `C:\Program Files\vrcft-go\plugins`,
		DefaultOSCRoot:   `C:\Users\Tester\AppData\LocalLow\VRChat\VRChat\OSC`,
	}
	candidate, err := userconfig.Normalize(userconfig.DefaultCandidate(paths))
	if err != nil {
		t.Fatal(err)
	}
	settings := userconfig.Settings{
		SchemaVersion: userconfig.SchemaVersion,
		Revision:      1,
		Avatar:        candidate.Avatar,
		Plugins:       candidate.Plugins,
		Processing:    candidate.Processing,
		OSC:           candidate.OSC,
	}
	return &rootAppHarness{
		t:     t,
		goos:  "windows",
		paths: paths,
		store: &fakeRootSettingsStore{loaded: userconfig.Loaded{Settings: &settings, Defaults: candidate}},
		backend: &fakeRootBackend{
			status:        application.Status{Lifecycle: application.LifecycleRunning},
			statusUpdates: make(chan application.Status, 4),
			pluginUpdates: make(chan []plugins.RuntimeSnapshot, 4),
		},
	}
}

func (h *rootAppHarness) dependencies() appDependencies {
	return appDependencies{
		goos: h.goos,
		environment: func() (userconfig.Environment, error) {
			h.record("environment")
			h.environmentCalls++
			return userconfig.Environment{GOOS: h.goos, RoamingDir: `C:\Users\Tester\AppData\Roaming`, UserProfile: `C:\Users\Tester`, Executable: `C:\Program Files\vrcft-go\vrcft-go.exe`}, nil
		},
		resolvePaths: func(userconfig.Environment) (userconfig.Paths, error) {
			h.record("paths")
			return h.paths, nil
		},
		newStore: func(userconfig.Paths) (settingsBackend, error) {
			h.record("store")
			h.storeFactoryCalls++
			return h.store, nil
		},
		applicationConfig: func(userconfig.Settings, userconfig.Paths) (application.Config, error) {
			h.record("config")
			h.configCalls++
			if h.onConfig != nil {
				h.onConfig()
			}
			return application.Config{}, h.configErr
		},
		newBackend: func(application.Config) (*application.Application, backendOperations, error) {
			h.record("backend")
			h.backendFactoryCalls++
			if h.backendErr != nil {
				return nil, nil, h.backendErr
			}
			return nil, h.backend, nil
		},
		now:             func() time.Time { return time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC) },
		emitter:         rootDiscardEmitter{record: func() { h.record("emitter") }},
		shutdownTimeout: 100 * time.Millisecond,
	}
}

func (h *rootAppHarness) record(call string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if call == "emitter" {
		for _, existing := range h.trace {
			if existing == call {
				return
			}
		}
	}
	h.trace = append(h.trace, call)
}

func (h *rootAppHarness) calls() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.trace...)
}

type rootStartupBarrier struct {
	target   string
	entered  chan struct{}
	releaseC chan struct{}
	once     sync.Once
}

func newRootStartupBarrier(target string) *rootStartupBarrier {
	return &rootStartupBarrier{target: target, entered: make(chan struct{}), releaseC: make(chan struct{})}
}

func (barrier *rootStartupBarrier) wait(boundary string) {
	if boundary != barrier.target {
		return
	}
	barrier.once.Do(func() { close(barrier.entered) })
	<-barrier.releaseC
}

func (barrier *rootStartupBarrier) waitUntilEntered(t *testing.T) {
	t.Helper()
	select {
	case <-barrier.entered:
	case <-time.After(time.Second):
		t.Fatalf("startup did not reach %q boundary", barrier.target)
	}
}

func (barrier *rootStartupBarrier) release() { close(barrier.releaseC) }

func startRootShutdown(app *App) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		app.shutdown(context.Background())
		close(done)
	}()
	return done
}

func waitRootContextCanceled(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel startup context")
	}
}

func assertRootShutdownWaiting(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-done:
		t.Fatalf("shutdown returned before admitted %s completed", operation)
	default:
	}
}

func waitRootOperationDone(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s did not finish", operation)
	}
}

type rootDiscardEmitter struct{ record func() }

func (emitter rootDiscardEmitter) Emit(context.Context, string, ...any) {
	if emitter.record != nil {
		emitter.record()
	}
}

type fakeRootSettingsStore struct {
	loaded          userconfig.Loaded
	loadErr         error
	loadCalls       int
	validateCalls   int
	saveCalls       int
	missing         bool
	createdDefaults bool
}

func (store *fakeRootSettingsStore) LoadOrCreate(context.Context) (userconfig.Loaded, error) {
	store.loadCalls++
	if store.missing && store.loadErr == nil {
		store.createdDefaults = true
		store.missing = false
	}
	return cloneRootLoaded(store.loaded), store.loadErr
}

func (store *fakeRootSettingsStore) Validate(candidate userconfig.Candidate) (userconfig.Candidate, error) {
	store.validateCalls++
	return candidate.Clone(), nil
}

func (store *fakeRootSettingsStore) Save(context.Context, userconfig.Loaded, userconfig.Candidate) (userconfig.SaveResult, error) {
	store.saveCalls++
	return userconfig.SaveResult{}, nil
}

func cloneRootLoaded(value userconfig.Loaded) userconfig.Loaded {
	clone := value
	clone.Defaults = value.Defaults.Clone()
	if value.Settings != nil {
		settings := value.Settings.Clone()
		clone.Settings = &settings
	}
	return clone
}

type fakeRootBackend struct {
	mu                  sync.Mutex
	startErr            error
	closeErr            error
	status              application.Status
	plugins             []plugins.RuntimeSnapshot
	statusUpdates       chan application.Status
	pluginUpdates       chan []plugins.RuntimeSnapshot
	onStart             func()
	onClose             func(context.Context)
	startFunc           func(context.Context) error
	closeFunc           func(context.Context) error
	startCalls          int
	closeCalls          int
	statusSubscriptions int
	pluginSubscriptions int
}

func (backend *fakeRootBackend) Start(ctx context.Context) error {
	backend.mu.Lock()
	backend.startCalls++
	onStart := backend.onStart
	startFunc := backend.startFunc
	err := backend.startErr
	backend.mu.Unlock()
	if onStart != nil {
		onStart()
	}
	if startFunc != nil {
		return startFunc(ctx)
	}
	return err
}

func (backend *fakeRootBackend) Close(ctx context.Context) error {
	backend.mu.Lock()
	backend.closeCalls++
	onClose := backend.onClose
	closeFunc := backend.closeFunc
	err := backend.closeErr
	backend.mu.Unlock()
	if onClose != nil {
		onClose(ctx)
	}
	if closeFunc != nil {
		return closeFunc(ctx)
	}
	return err
}

func (backend *fakeRootBackend) closeCallCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.closeCalls
}

func (backend *fakeRootBackend) startCallCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.startCalls
}

func (backend *fakeRootBackend) statusSubscriptionCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.statusSubscriptions
}

func (backend *fakeRootBackend) pluginSubscriptionCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.pluginSubscriptions
}

func (backend *fakeRootBackend) Status() application.Status {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.status
}

func (backend *fakeRootBackend) SubscribeStatus(context.Context) <-chan application.Status {
	backend.mu.Lock()
	backend.statusSubscriptions++
	backend.mu.Unlock()
	return backend.statusUpdates
}

func (backend *fakeRootBackend) Plugins() []plugins.RuntimeSnapshot {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return append([]plugins.RuntimeSnapshot(nil), backend.plugins...)
}

func (backend *fakeRootBackend) PluginConfig(string) (pluginapi.Config, bool) {
	return pluginapi.Config{}, false
}

func (backend *fakeRootBackend) SetPluginEnabled(context.Context, string, bool) error { return nil }

func (backend *fakeRootBackend) UpdatePluginConfig(context.Context, string, pluginapi.Config) error {
	return nil
}

func (backend *fakeRootBackend) SubscribePlugins(context.Context) <-chan []plugins.RuntimeSnapshot {
	backend.mu.Lock()
	backend.pluginSubscriptions++
	backend.mu.Unlock()
	return backend.pluginUpdates
}
