//go:build windows

package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/wzhqwq/vrcft-go/internal/ipc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/pluginruntime"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const (
	integrationPluginID    = "test.integration"
	integrationHelperEnv   = "VRCFT_TEST_PLUGIN_HELPER"
	integrationBehaviorEnv = "VRCFT_TEST_PLUGIN_BEHAVIOR"
)

func TestPluginHelperProcess(t *testing.T) {
	if os.Getenv(integrationHelperEnv) != "1" {
		return
	}
	os.Exit(runHelperPlugin())
}

func TestWindowsPluginIntegrationHappyPath(t *testing.T) {
	harness := newWindowsIntegrationHarness(t, "normal", 3)
	events := harness.manager.Subscribe(harness.ctx)

	harness.startAndEnable()
	running := harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateRunning && snapshot.PID > 0
	})
	if running.RestartCount != 0 {
		t.Fatalf("initial running snapshot RestartCount = %d, want 0", running.RestartCount)
	}
	harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return !snapshot.LastHeartbeatAt.IsZero()
	})

	config := pluginapi.Config{Revision: 1, Data: []byte(`{"gain":0.75}`)}
	subscription := pluginapi.Subscription{
		Generation:   7,
		Capabilities: trackingmodel.CapabilityEye,
		Eye:          trackingmodel.EyeValidLeftGaze,
	}
	if err := harness.manager.UpdateConfig(harness.ctx, integrationPluginID, config); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if err := harness.manager.UpdateSubscription(harness.ctx, integrationPluginID, subscription); err != nil {
		t.Fatalf("UpdateSubscription() error = %v", err)
	}
	if err := harness.manager.SetActive(harness.ctx, integrationPluginID, true); err != nil {
		t.Fatalf("SetActive() error = %v", err)
	}

	frame := harness.sink.wait(harness.ctx)
	if frame.pluginID != integrationPluginID || frame.generation != 7 ||
		frame.frame.Sequence != 42 ||
		frame.frame.Capabilities != trackingmodel.CapabilityEye ||
		frame.frame.Eye.Valid != trackingmodel.EyeValidLeftGaze ||
		frame.frame.Eye.LeftGaze != (trackingmodel.Vec2{X: 0.25, Y: -0.5}) {
		t.Fatalf("FrameSink delivery = %+v, want helper frame with plugin ID and generation 7", frame)
	}
	waitIntegrationTelemetry(t, harness.ctx, events)

	if err := harness.manager.Disable(harness.ctx, integrationPluginID); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateDisabled && snapshot.PID == 0
	})
	harness.waitAllExited()
	harness.assertPipesReleased()
}

func TestWindowsPluginIntegrationDescriptorMismatchIsIncompatible(t *testing.T) {
	harness := newWindowsIntegrationHarness(t, "descriptor-mismatch", 3)
	harness.startAndEnable()

	snapshot := harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateIncompatible
	})
	if snapshot.RestartCount != 0 || snapshot.PID != 0 ||
		!strings.Contains(snapshot.LastError, ErrDescriptorMismatch.Error()) {
		t.Fatalf("descriptor mismatch snapshot = %+v, want incompatible without restart", snapshot)
	}
	harness.waitAllExited()
	harness.assertPipesReleased()
}

func TestWindowsPluginIntegrationCrashUsesFiniteRestartBudget(t *testing.T) {
	harness := newWindowsIntegrationHarness(t, "crash", 2)
	harness.startAndEnable()

	snapshot := harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateCrashed &&
			snapshot.ConsecutiveFailures == 2
	})
	if snapshot.RestartCount != 1 || snapshot.LastError != ErrRestartLimitReached.Error() {
		t.Fatalf("finite restart snapshot = %+v, want one restart then exhausted budget", snapshot)
	}
	if got := harness.launcher.startCount(); got != 2 {
		t.Fatalf("process starts = %d, want 2", got)
	}
	harness.waitAllExited()
	harness.assertPipesReleased()
}

func TestWindowsPluginIntegrationHeartbeatHangKillsAndRestarts(t *testing.T) {
	harness := newWindowsIntegrationHarness(t, "hang", 2)
	harness.startAndEnable()

	first := harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateRunning && snapshot.PID > 0
	})

	harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateUnresponsive && snapshot.PID == first.PID
	})
	restarted := harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateRunning && snapshot.PID > 0 &&
			snapshot.PID != first.PID && snapshot.RestartCount == 1
	})
	if restarted.ConsecutiveFailures != 1 {
		t.Fatalf("restart snapshot ConsecutiveFailures = %d, want 1", restarted.ConsecutiveFailures)
	}
	if err := harness.launcher.waitPIDExited(harness.ctx, first.PID); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsPluginIntegrationManagerCloseReapsProcessAndListener(t *testing.T) {
	harness := newWindowsIntegrationHarness(t, "normal", 3)
	harness.startAndEnable()
	harness.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateRunning && snapshot.PID > 0
	})

	if err := harness.manager.Close(harness.ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	harness.closed = true
	harness.waitAllExited()
	harness.assertPipesReleased()
}

type windowsIntegrationHarness struct {
	t        *testing.T
	ctx      context.Context
	cancel   context.CancelFunc
	manager  Manager
	launcher *integrationLauncher
	sink     *integrationFrameSink
	closed   bool
}

func newWindowsIntegrationHarness(t *testing.T, behavior string, maxFailures int) *windowsIntegrationHarness {
	t.Helper()
	t.Setenv(integrationHelperEnv, "1")
	t.Setenv(integrationBehaviorEnv, behavior)

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	plugin := InstalledPlugin{
		Manifest: Manifest{
			SchemaVersion: 1,
			ID:            integrationPluginID,
			Name:          "Windows Integration Helper",
			Version:       "1.0.0",
			Description:   "Real process and named pipe integration helper.",
			ProtocolMin:   protocol.Version,
			ProtocolMax:   protocol.Version,
			Entrypoint:    "unused-by-static-test-catalog.exe",
			Capabilities:  trackingmodel.CapabilityEye,
		},
		RootDir:    t.TempDir(),
		Executable: executable,
		Source:     SourceDev,
	}
	catalog := &managerTestCatalog{plugins: []InstalledPlugin{plugin}}
	store := newManagerTestStore(emptyPluginSettings())
	launcher := newIntegrationLauncher()
	sink := &integrationFrameSink{frames: make(chan integrationFrame, 8)}
	options := DefaultOptions()
	options.HandshakeTimeout = 2 * time.Second
	options.HeartbeatTimeout = 1500 * time.Millisecond
	options.GracefulTimeout = 1500 * time.Millisecond
	options.KillTimeout = 1500 * time.Millisecond
	options.Restart = RestartPolicy{
		InitialBackoff: 20 * time.Millisecond,
		Multiplier:     1,
		MaxBackoff:     20 * time.Millisecond,
		MaxFailures:    maxFailures,
		StableWindow:   time.Minute,
	}
	manager, err := NewManager(catalog, store, launcher, sink, options)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	harness := &windowsIntegrationHarness{
		t:        t,
		ctx:      ctx,
		cancel:   cancel,
		manager:  manager,
		launcher: launcher,
		sink:     sink,
	}
	t.Cleanup(func() {
		if !harness.closed {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = harness.manager.Close(closeCtx)
			closeCancel()
		}
		cancel()
	})
	return harness
}

func (h *windowsIntegrationHarness) startAndEnable() {
	h.t.Helper()
	if err := h.manager.Start(h.ctx); err != nil {
		h.t.Fatalf("Start() error = %v", err)
	}
	if err := h.manager.Enable(h.ctx, integrationPluginID); err != nil {
		h.t.Fatalf("Enable() error = %v", err)
	}
}

func (h *windowsIntegrationHarness) waitSnapshot(match func(RuntimeSnapshot) bool) RuntimeSnapshot {
	h.t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, ok := h.manager.Get(integrationPluginID)
		if ok && match(snapshot) {
			return snapshot
		}
		select {
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for plugin snapshot: %v; last = %+v", h.ctx.Err(), snapshot)
		case <-ticker.C:
		}
	}
}

func (h *windowsIntegrationHarness) assertPipesReleased() {
	h.t.Helper()
	for _, pipeName := range h.launcher.pipeNames() {
		listener, err := ipc.Listen(ipc.ServerConfig{PipeName: pipeName})
		if err != nil {
			h.t.Fatalf("named pipe %q remained owned after process shutdown: %v", pipeName, err)
		}
		if err := listener.Close(); err != nil {
			h.t.Fatalf("close probe listener %q: %v", pipeName, err)
		}
	}
}

func (h *windowsIntegrationHarness) waitAllExited() {
	h.t.Helper()
	if err := h.launcher.waitAllExited(h.ctx); err != nil {
		h.t.Fatal(err)
	}
}

type integrationFrame struct {
	pluginID   string
	generation uint64
	frame      trackingmodel.TrackingFrame
}

type integrationFrameSink struct {
	frames chan integrationFrame
}

func (s *integrationFrameSink) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) {
	select {
	case s.frames <- integrationFrame{pluginID: pluginID, generation: generation, frame: frame}:
	default:
	}
}

func (s *integrationFrameSink) wait(ctx context.Context) integrationFrame {
	select {
	case frame := <-s.frames:
		return frame
	case <-ctx.Done():
		return integrationFrame{}
	}
}

type integrationLauncher struct {
	real ProcessLauncher
	mu   sync.Mutex
	runs []*integrationProcess
}

func newIntegrationLauncher() *integrationLauncher {
	return &integrationLauncher{real: NewProcessLauncher()}
}

func (l *integrationLauncher) Start(ctx context.Context, spec ProcessSpec) (Process, error) {
	spec.Args = append(spec.Args, "-test.run=^TestPluginHelperProcess$", "-test.v=false")
	process, err := l.real.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	tracked := &integrationProcess{
		Process:  process,
		pipeName: integrationEnvValue(spec.Env, "VRCFT_PIPE_NAME"),
		exited:   make(chan struct{}),
	}
	l.mu.Lock()
	l.runs = append(l.runs, tracked)
	l.mu.Unlock()
	return tracked, nil
}

func (l *integrationLauncher) startCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.runs)
}

func (l *integrationLauncher) pipeNames() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, len(l.runs))
	for index, run := range l.runs {
		names[index] = run.pipeName
	}
	return names
}

func (l *integrationLauncher) waitAllExited(ctx context.Context) error {
	l.mu.Lock()
	runs := append([]*integrationProcess(nil), l.runs...)
	l.mu.Unlock()
	for _, run := range runs {
		select {
		case <-run.exited:
		case <-ctx.Done():
			return fmt.Errorf("waiting for helper PID %d to exit: %w", run.PID(), ctx.Err())
		}
	}
	return nil
}

func (l *integrationLauncher) waitPIDExited(ctx context.Context, pid int) error {
	l.mu.Lock()
	runs := append([]*integrationProcess(nil), l.runs...)
	l.mu.Unlock()
	for _, run := range runs {
		if run.PID() != pid {
			continue
		}
		select {
		case <-run.exited:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("waiting for helper PID %d to exit: %w", pid, ctx.Err())
		}
	}
	return fmt.Errorf("helper PID %d was not recorded", pid)
}

type integrationProcess struct {
	Process
	pipeName string
	exited   chan struct{}
	waitOnce sync.Once
	waitErr  error
}

func (p *integrationProcess) Wait() error {
	p.waitOnce.Do(func() {
		p.waitErr = p.Process.Wait()
		close(p.exited)
	})
	return p.waitErr
}

func integrationEnvValue(environment []string, key string) string {
	prefix := key + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func waitIntegrationEvent(t *testing.T, ctx context.Context, events <-chan Event, match func(Event) bool) Event {
	t.Helper()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("manager event stream closed before expected event")
			}
			if match(event) {
				return event
			}
		case <-ctx.Done():
			t.Fatalf("waiting for manager event: %v", ctx.Err())
		}
	}
}

func waitIntegrationTelemetry(t *testing.T, ctx context.Context, events <-chan Event) {
	t.Helper()
	var statusSeen, logSeen bool
	for !statusSeen || !logSeen {
		event := waitIntegrationEvent(t, ctx, events, func(event Event) bool {
			return event.PluginID == integrationPluginID &&
				(event.Type == EventPluginStatus || event.Type == EventPluginLog)
		})
		switch event.Type {
		case EventPluginStatus:
			statusSeen = event.Status != nil && event.Status.State == pluginapi.DeviceReady
		case EventPluginLog:
			logSeen = event.Log != nil && event.Log.Level == pluginapi.LogInfo &&
				event.Log.Message == "integration helper ready"
		}
	}
}

func suspendCurrentIntegrationProcess() error {
	procedure := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSuspendProcess")
	status, _, _ := procedure.Call(uintptr(windows.CurrentProcess()))
	if status != 0 {
		return fmt.Errorf("NtSuspendProcess status = %#x", status)
	}
	return nil
}

type integrationDriver struct {
	behavior string
}

func (d integrationDriver) Descriptor() pluginapi.Descriptor {
	id := integrationPluginID
	if d.behavior == "descriptor-mismatch" {
		id = "test.integration.mismatch"
	}
	return pluginapi.Descriptor{
		APIVersion:   pluginapi.APIVersion,
		ID:           id,
		Name:         "Windows Integration Helper",
		Version:      "1.0.0",
		Description:  "Real process and named pipe integration helper.",
		Capabilities: trackingmodel.CapabilityEye,
	}
}

func (d integrationDriver) Run(ctx context.Context, host pluginapi.Host) error {
	if d.behavior == "crash" {
		return errors.New("integration helper crash")
	}
	host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady})
	host.Log(pluginapi.LogInfo, "integration helper ready")
	if d.behavior == "hang" {
		return suspendCurrentIntegrationProcess()
	}

	var configSeen, subscriptionSeen, active bool
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-host.Events():
			if !ok {
				return nil
			}
			switch event := event.(type) {
			case pluginapi.ConfigChanged:
				configSeen = event.Config.Revision == 1
			case pluginapi.SubscriptionChanged:
				subscriptionSeen = event.Subscription.Generation == 7
			case pluginapi.ActiveChanged:
				active = event.Active
			case pluginapi.ShutdownRequested:
				return nil
			}
			if configSeen && subscriptionSeen && active {
				host.PublishFrame(trackingmodel.TrackingFrame{
					Sequence:     42,
					Capabilities: trackingmodel.CapabilityEye,
					Eye: trackingmodel.EyeSample{
						Valid:    trackingmodel.EyeValidLeftGaze,
						LeftGaze: trackingmodel.Vec2{X: 0.25, Y: -0.5},
					},
				})
				configSeen = false
			}
		}
	}
}

func runHelperPlugin() int {
	behavior := os.Getenv(integrationBehaviorEnv)
	if behavior == "bad-token" {
		_ = os.Setenv(pluginruntime.SessionTokenEnv, "invalid-test-token")
	}
	if err := pluginruntime.Main(integrationDriver{behavior: behavior}); err != nil {
		return 1
	}
	return 0
}
