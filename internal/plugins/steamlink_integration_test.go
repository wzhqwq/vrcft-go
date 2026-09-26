//go:build windows

package plugins

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/ipc"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/osc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const steamLinkIntegrationPluginID = "steamlink"

func TestSteamLinkProcessPipeline(t *testing.T) {
	h := newSteamLinkIntegrationHarness(t)
	h.startConfigured(7, jawSubscription(7))

	packet := steamLinkPacket(t, false)
	got := h.sendUntil(packet, func(frame steamLinkFrame) bool {
		return frame.generation == 7 && frame.frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen)
	})
	if got.pluginID != steamLinkIntegrationPluginID || got.generation != 7 ||
		!got.frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) ||
		got.frame.Expressions.Values[trackingmodel.ExpressionJawOpen] != 0.5 {
		t.Fatalf("unexpected Steam Link frame: %+v", got)
	}

	service := tracking.NewService()
	if err := service.SetGeneration(got.generation); err != nil {
		t.Fatal(err)
	}
	if err := service.Submit(got.pluginID, got.generation, got.frame); err != nil {
		t.Fatal(err)
	}
	merged, ok := service.LatestMerged()
	if !ok {
		t.Fatal("missing merged frame")
	}
	pipeline, err := processing.NewPipeline(processing.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := pipeline.ProcessAt(merged, merged.UpdatedAtNS)
	if err != nil {
		t.Fatal(err)
	}
	if !canonical.ExpressionActive {
		t.Fatal("fresh expression group is inactive")
	}
	h.drainEvents()

	invalidated := h.waitFrame(func(frame steamLinkFrame) bool {
		return frame.generation == got.generation &&
			frame.frame.Sequence > got.frame.Sequence &&
			!frame.frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen)
	})
	if err := service.Submit(invalidated.pluginID, invalidated.generation, invalidated.frame); err != nil {
		t.Fatal(err)
	}
	merged, ok = service.LatestMerged()
	if !ok {
		t.Fatal("missing merged invalidation")
	}
	canonical, err = pipeline.ProcessAt(merged, merged.UpdatedAtNS+int64(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if jaw, _ := canonical.Expressions.Get(trackingmodel.ExpressionJawOpen); jaw != 0 {
		t.Fatalf("dropout JawOpen = %v; want neutral", jaw)
	}
	if canonical.ExpressionActive {
		t.Fatal("expired expression group remains active")
	}
	h.waitDisconnected()

	h.disableAndAssertReleased()
}

func TestSteamLinkProcessLifecycleAndSubscriptions(t *testing.T) {
	h := newSteamLinkIntegrationHarness(t)
	h.startConfigured(7, jawSubscription(7))
	first := h.sendUntil(steamLinkPacket(t, false), func(frame steamLinkFrame) bool {
		return frame.generation == 7 && frame.frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen)
	})
	if first.frame.Capabilities != trackingmodel.CapabilityExpression {
		t.Fatalf("JawOpen-only capabilities = %v", first.frame.Capabilities)
	}

	h.sink.drain()
	eye := pluginapi.Subscription{
		Generation:   8,
		Capabilities: trackingmodel.CapabilityEye,
		Eye:          trackingmodel.EyeValidLeftGaze,
	}
	if err := h.manager.UpdateSubscription(h.ctx, steamLinkIntegrationPluginID, eye); err != nil {
		t.Fatalf("UpdateSubscription(eye) error = %v", err)
	}
	eyeFrame := h.sendUntilGeneration(8, steamLinkPacket(t, true), func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})
	if eyeFrame.frame.Capabilities != trackingmodel.CapabilityEye ||
		eyeFrame.frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) ||
		eyeFrame.frame.Expressions.Values[trackingmodel.ExpressionJawOpen] != 0 {
		t.Fatalf("eye-only subscription delivered unselected data: %+v", eyeFrame.frame)
	}

	h.sink.drain()
	if err := h.manager.SetActive(h.ctx, steamLinkIntegrationPluginID, false); err != nil {
		t.Fatalf("SetActive(false) error = %v", err)
	}
	h.send(steamLinkPacket(t, true), h.port)
	h.assertNoMatchingFrame(150*time.Millisecond, func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})
	if err := h.manager.SetActive(h.ctx, steamLinkIntegrationPluginID, true); err != nil {
		t.Fatalf("SetActive(true) error = %v", err)
	}
	h.sendUntil(steamLinkPacket(t, true), func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})

	h.sink.drain()
	oldPort := h.port
	newPort := h.reservePort()
	if err := h.manager.UpdateConfig(h.ctx, steamLinkIntegrationPluginID, pluginapi.Config{
		Revision: 2,
		Data:     []byte(fmt.Sprintf(`{"listenPort":%d}`, newPort)),
	}); err != nil {
		t.Fatalf("UpdateConfig(rebind) error = %v", err)
	}
	h.port = newPort
	h.sendUntil(steamLinkPacket(t, true), func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})
	// A UDP datagram accepted before the receive transition can still be in the
	// receiver queue. Wait for the bounded freshness window to quiesce before
	// making the malformed-datagram assertion.
	h.waitForQuietMatching(300*time.Millisecond, func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})
	h.sink.drain()
	h.send(steamLinkPacket(t, true), oldPort)
	h.assertNoMatchingFrame(150*time.Millisecond, func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})

	h.sink.drain()
	running := h.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateRunning && snapshot.PID > 0
	})
	h.send([]byte{0, 1, 2, 3}, h.port)
	h.assertNoMatchingFrame(150*time.Millisecond, func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})
	h.waitSnapshot(func(snapshot RuntimeSnapshot) bool {
		return snapshot.State == StateRunning && snapshot.PID == running.PID
	})
	h.sendUntil(steamLinkPacket(t, true), func(frame steamLinkFrame) bool {
		return frame.generation == 8 && frame.frame.Eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	})
	h.disableAndAssertReleased()
}

type steamLinkFrame struct {
	pluginID   string
	generation uint64
	frame      trackingmodel.TrackingFrame
}

type steamLinkSink struct{ frames chan steamLinkFrame }

func (s *steamLinkSink) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) {
	select {
	case s.frames <- steamLinkFrame{pluginID: pluginID, generation: generation, frame: frame}:
	default:
	}
}

func (s *steamLinkSink) drain() {
	for {
		select {
		case <-s.frames:
		default:
			return
		}
	}
}

type steamLinkIntegrationHarness struct {
	t        *testing.T
	ctx      context.Context
	cancel   context.CancelFunc
	manager  Manager
	launcher *steamLinkLauncher
	sink     *steamLinkSink
	events   <-chan Event
	port     int
	ports    []int
	closed   bool
}

func newSteamLinkIntegrationHarness(t *testing.T) *steamLinkIntegrationHarness {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	root := t.TempDir()
	pluginRoot := filepath.Join(root, steamLinkIntegrationPluginID)
	if err := os.Mkdir(pluginRoot, 0o700); err != nil {
		t.Fatalf("create plugin root: %v", err)
	}
	repo := steamLinkRepositoryRoot(t)
	executable := filepath.Join(pluginRoot, "steamlink-plugin.exe")
	steamLinkBuildCommand(t, ctx, repo, executable)
	copySteamLinkManifest(t, filepath.Join(repo, "plugins", "steamlink", "manifest.json"), filepath.Join(pluginRoot, "manifest.json"))
	catalog, err := NewDirectoryCatalog(DirectoryCatalogConfig{BuiltinRoot: root})
	if err != nil {
		t.Fatalf("NewDirectoryCatalog() error = %v", err)
	}
	store, err := NewJSONStore(filepath.Join(root, "plugins.json"), 64*1024)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	launcher := newSteamLinkLauncher()
	sink := &steamLinkSink{frames: make(chan steamLinkFrame, 32)}
	options := DefaultOptions()
	options.HandshakeTimeout = 4 * time.Second
	options.HeartbeatTimeout = 4 * time.Second
	options.GracefulTimeout = 2 * time.Second
	options.KillTimeout = 2 * time.Second
	manager, err := NewManager(catalog, store, launcher, sink, options)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	h := &steamLinkIntegrationHarness{t: t, ctx: ctx, cancel: cancel, manager: manager, launcher: launcher, sink: sink, events: manager.Subscribe(ctx)}
	t.Cleanup(func() {
		defer cancel()
		if !h.closed {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := h.manager.Close(closeCtx)
			closeCancel()
			if err != nil {
				t.Errorf("cleanup Manager.Close() error = %v", err)
			}
		}
		if err := h.launcher.waitAllExited(context.Background(), 5*time.Second); err != nil {
			t.Errorf("cleanup process wait: %v", err)
		}
	})
	return h
}

func (h *steamLinkIntegrationHarness) startConfigured(generation uint64, subscription pluginapi.Subscription) {
	h.t.Helper()
	h.port = h.reservePort()
	if err := h.manager.Start(h.ctx); err != nil {
		h.t.Fatalf("Start() error = %v", err)
	}
	if err := h.manager.UpdateConfig(h.ctx, steamLinkIntegrationPluginID, pluginapi.Config{
		Revision: 1,
		Data:     []byte(fmt.Sprintf(`{"listenPort":%d}`, h.port)),
	}); err != nil {
		h.t.Fatalf("UpdateConfig() error = %v", err)
	}
	if err := h.manager.Enable(h.ctx, steamLinkIntegrationPluginID); err != nil {
		h.t.Fatalf("Enable() error = %v", err)
	}
	h.waitSnapshot(func(snapshot RuntimeSnapshot) bool { return snapshot.State == StateRunning && snapshot.PID > 0 })
	if subscription.Generation != generation {
		h.t.Fatalf("subscription generation %d, want %d", subscription.Generation, generation)
	}
	if err := h.manager.UpdateSubscription(h.ctx, steamLinkIntegrationPluginID, subscription); err != nil {
		h.t.Fatalf("UpdateSubscription() error = %v", err)
	}
	if err := h.manager.SetActive(h.ctx, steamLinkIntegrationPluginID, true); err != nil {
		h.t.Fatalf("SetActive() error = %v", err)
	}
}

func (h *steamLinkIntegrationHarness) reservePort() int {
	h.t.Helper()
	for attempt := 0; attempt < 4; attempt++ {
		listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
		if err != nil {
			continue
		}
		port := listener.LocalAddr().(*net.UDPAddr).Port
		if err := listener.Close(); err == nil && port != 0 && port != 9015 {
			h.ports = append(h.ports, port)
			return port
		}
	}
	h.t.Fatal("reserve non-production UDP port after bounded retries")
	return 0
}

func (h *steamLinkIntegrationHarness) waitSnapshot(match func(RuntimeSnapshot) bool) RuntimeSnapshot {
	h.t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, ok := h.manager.Get(steamLinkIntegrationPluginID)
		if ok && match(snapshot) {
			return snapshot
		}
		select {
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for Steam Link snapshot: %v; last = %+v", h.ctx.Err(), snapshot)
		case <-ticker.C:
		}
	}
}

func (h *steamLinkIntegrationHarness) sendUntil(packet []byte, match func(steamLinkFrame) bool) steamLinkFrame {
	return h.sendUntilFrame(packet, match, func(steamLinkFrame) {})
}

func (h *steamLinkIntegrationHarness) sendUntilGeneration(expectedGeneration uint64, packet []byte, match func(steamLinkFrame) bool) steamLinkFrame {
	h.t.Helper()
	return h.sendUntilFrame(packet, match, func(frame steamLinkFrame) {
		if frame.pluginID != steamLinkIntegrationPluginID || frame.generation > expectedGeneration {
			h.t.Fatalf("unexpected Steam Link frame after generation transition: %+v", frame)
		}
	})
}

func (h *steamLinkIntegrationHarness) sendUntilFrame(packet []byte, match func(steamLinkFrame) bool, observe func(steamLinkFrame)) steamLinkFrame {
	h.t.Helper()
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for {
		h.send(packet, h.port)
		for {
			select {
			case frame := <-h.sink.frames:
				observe(frame)
				if match(frame) {
					return frame
				}
			default:
				goto wait
			}
		}
	wait:
		select {
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for repeated Steam Link UDP frame: %v", h.ctx.Err())
		case <-ticker.C:
		}
	}
}

func (h *steamLinkIntegrationHarness) waitFrame(match func(steamLinkFrame) bool) steamLinkFrame {
	h.t.Helper()
	for {
		select {
		case frame := <-h.sink.frames:
			if match(frame) {
				return frame
			}
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for Steam Link frame: %v", h.ctx.Err())
		}
	}
}

func (h *steamLinkIntegrationHarness) send(packet []byte, port int) {
	h.t.Helper()
	connection, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		h.t.Fatalf("dial Steam Link UDP port %d: %v", port, err)
	}
	defer connection.Close()
	if _, err := connection.Write(packet); err != nil {
		h.t.Fatalf("write Steam Link UDP packet: %v", err)
	}
}

func (h *steamLinkIntegrationHarness) assertNoMatchingFrame(duration time.Duration, match func(steamLinkFrame) bool) {
	h.t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case frame := <-h.sink.frames:
			if match(frame) {
				h.t.Fatalf("unexpected Steam Link frame after transition: %+v", frame)
			}
		case <-timer.C:
			return
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for absent Steam Link frame: %v", h.ctx.Err())
		}
	}
}

func (h *steamLinkIntegrationHarness) drainEvents() {
	h.t.Helper()
	for {
		select {
		case <-h.events:
		default:
			return
		}
	}
}

func (h *steamLinkIntegrationHarness) waitDisconnected() {
	h.t.Helper()
	for {
		select {
		case event, ok := <-h.events:
			if !ok {
				h.t.Fatal("manager event stream closed before Steam Link disconnected")
			}
			if event.PluginID == steamLinkIntegrationPluginID && event.Type == EventPluginStatus &&
				event.Status != nil && event.Status.State == pluginapi.DeviceDisconnected {
				return
			}
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for Steam Link disconnected status: %v", h.ctx.Err())
		}
	}
}

func (h *steamLinkIntegrationHarness) waitForQuietMatching(duration time.Duration, match func(steamLinkFrame) bool) {
	h.t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case frame := <-h.sink.frames:
			if match(frame) {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(duration)
			}
		case <-timer.C:
			return
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for Steam Link transition to quiesce: %v", h.ctx.Err())
		}
	}
}

func (h *steamLinkIntegrationHarness) disableAndAssertReleased() {
	h.t.Helper()
	if err := h.manager.Disable(h.ctx, steamLinkIntegrationPluginID); err != nil {
		h.t.Fatalf("Disable() error = %v", err)
	}
	h.waitSnapshot(func(snapshot RuntimeSnapshot) bool { return snapshot.State == StateDisabled && snapshot.PID == 0 })
	if err := h.launcher.waitAllExited(h.ctx, 0); err != nil {
		h.t.Fatal(err)
	}
	for _, port := range h.ports {
		listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			h.t.Fatalf("UDP port %d remained owned after disable: %v", port, err)
		}
		_ = listener.Close()
	}
	for _, pipeName := range h.launcher.pipeNames() {
		listener, err := ipc.Listen(ipc.ServerConfig{PipeName: pipeName})
		if err != nil {
			h.t.Fatalf("named pipe %q remained owned after disable: %v", pipeName, err)
		}
		if err := listener.Close(); err != nil {
			h.t.Fatalf("close named pipe probe %q: %v", pipeName, err)
		}
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.manager.Close(closeCtx); err != nil {
		h.t.Fatalf("bounded Manager.Close() error = %v", err)
	}
	h.closed = true
}

func steamLinkPacket(t *testing.T, includeEye bool) []byte {
	t.Helper()
	messages := []osc.Message{{Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Float32(0.5)}}}
	if includeEye {
		messages = append(messages, osc.Message{Address: "/sl/eyeTrackedGazePoint", Args: []osc.Value{osc.Float32(0), osc.Float32(0), osc.Float32(-1)}})
	}
	elements := make([][]byte, len(messages))
	for index, message := range messages {
		packet, err := osc.MarshalMessage(message)
		if err != nil {
			t.Fatalf("MarshalMessage(%q): %v", message.Address, err)
		}
		elements[index] = packet
	}
	packet, err := osc.MarshalBundle(osc.Bundle{Elements: elements})
	if err != nil {
		t.Fatalf("MarshalBundle(): %v", err)
	}
	return packet
}

func jawSubscription(generation uint64) pluginapi.Subscription {
	return pluginapi.Subscription{
		Generation:   generation,
		Capabilities: trackingmodel.CapabilityExpression,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
}

func steamLinkRepositoryRoot(t *testing.T) string {
	t.Helper()
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func steamLinkBuildCommand(t *testing.T, ctx context.Context, repository, executable string) {
	t.Helper()
	cache := os.Getenv("GOCACHE")
	if cache == "" || !filepath.IsAbs(cache) {
		t.Fatal("Steam Link process test requires an inherited absolute GOCACHE")
	}
	if info, err := os.Stat(cache); err != nil || !info.IsDir() {
		t.Fatalf("Steam Link process test requires existing GOCACHE %q: %v", cache, err)
	}
	// This disposable worktree has no usable VCS stamping context in the Go
	// toolchain. The executable's runtime behavior is independent of build
	// metadata, so disable only that stamping step while retaining a real build.
	command := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", executable, "./cmd/steamlink-plugin")
	command.Dir = repository
	command.Env = os.Environ()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Steam Link command: %v\n%s", err, output)
	}
}

func copySteamLinkManifest(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read Steam Link manifest: %v", err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatalf("copy Steam Link manifest: %v", err)
	}
}

type steamLinkLauncher struct {
	real ProcessLauncher
	mu   sync.Mutex
	runs []*steamLinkProcess
}

func newSteamLinkLauncher() *steamLinkLauncher { return &steamLinkLauncher{real: NewProcessLauncher()} }

func (l *steamLinkLauncher) Start(ctx context.Context, spec ProcessSpec) (Process, error) {
	process, err := l.real.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	run := &steamLinkProcess{Process: process, pipeName: steamLinkEnvironmentValue(spec.Env, "VRCFT_PIPE_NAME"), exited: make(chan struct{})}
	l.mu.Lock()
	l.runs = append(l.runs, run)
	l.mu.Unlock()
	return run, nil
}

func (l *steamLinkLauncher) pipeNames() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, len(l.runs))
	for index, run := range l.runs {
		names[index] = run.pipeName
	}
	return names
}

func (l *steamLinkLauncher) waitAllExited(ctx context.Context, fallback time.Duration) error {
	if fallback > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, fallback)
		defer cancel()
	}
	l.mu.Lock()
	runs := append([]*steamLinkProcess(nil), l.runs...)
	l.mu.Unlock()
	for _, run := range runs {
		select {
		case <-run.exited:
		case <-ctx.Done():
			return fmt.Errorf("waiting for Steam Link process %d: %w", run.PID(), ctx.Err())
		}
	}
	return nil
}

type steamLinkProcess struct {
	Process
	pipeName string
	exited   chan struct{}
	waitOnce sync.Once
	waitErr  error
}

func (p *steamLinkProcess) Wait() error {
	p.waitOnce.Do(func() {
		p.waitErr = p.Process.Wait()
		close(p.exited)
	})
	return p.waitErr
}

func steamLinkEnvironmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
