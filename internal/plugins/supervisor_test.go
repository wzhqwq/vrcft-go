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

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestPluginSupervisorControlsStateAndOwnsStartup(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(100, 0))
	var publishedMu sync.Mutex
	var published []RuntimeSnapshot
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{},
		Restart:    DefaultRestartPolicy(),
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
		Publish: func(snapshot RuntimeSnapshot) {
			publishedMu.Lock()
			published = append(published, snapshot)
			publishedMu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)

	awaitSupervisorState(t, supervisor, StateDisabled)
	if got := supervisor.Snapshot(); got.State != StateDisabled || got.Enabled {
		t.Fatalf("initial snapshot = %+v", got)
	}
	configBytes := []byte(`{"threshold":1}`)
	config := pluginapi.Config{Revision: 1, Data: configBytes}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorConfig, config: config}); err != nil {
		t.Fatal(err)
	}
	configBytes[13] = '9'
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorActive, active: true}); err != nil {
		t.Fatal(err)
	}
	subscription := pluginapi.Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityEye,
		Eye:          trackingmodel.EyeValidLeftGaze,
	}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorSubscription, subscription: subscription}); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorEnable}); err != nil {
		t.Fatal(err)
	}

	launch := factory.awaitLaunch(t)
	if string(launch.startup.Config.Data) != `{"threshold":1}` || !launch.startup.Active ||
		launch.startup.Subscription != subscription {
		t.Fatalf("startup = %+v", launch.startup)
	}
	launch.startup.Config.Data[0] = '!'
	if got := supervisor.Snapshot(); got.ConfigRevision != 1 ||
		got.SubscriptionGeneration != 1 || !got.Active || !got.Enabled ||
		got.State != StateStarting {
		t.Fatalf("starting snapshot = %+v", got)
	}

	launch.callbacks.ProcessStarted(launch.instanceID, 4321)
	launch.callbacks.Heartbeat(launch.instanceID, clock.now())
	awaitSupervisorState(t, supervisor, StateHandshaking)
	if got := supervisor.Snapshot(); got.State != StateHandshaking || got.PID != 4321 {
		t.Fatalf("heartbeat before Ready changed lifecycle state: %+v", got)
	}
	launch.callbacks.Ready(launch.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	nextConfig := pluginapi.Config{Revision: 2, Data: []byte(`{"threshold":2}`)}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorConfig, config: nextConfig}); err != nil {
		t.Fatal(err)
	}
	if got := launch.session.awaitControl(t); got.kind != controlConfig ||
		string(got.state.Config.Data) != `{"threshold":2}` {
		t.Fatalf("runtime control = %+v", got)
	}
	nextConfig.Data[0] = '!'
	if got := supervisor.Snapshot(); got.ConfigRevision != 2 {
		t.Fatalf("snapshot after Config = %+v", got)
	}

	launch.session.blockStop()
	disableDone := make(chan error, 1)
	go func() {
		disableDone <- supervisor.Command(context.Background(), supervisorCommand{kind: supervisorDisable})
	}()
	launch.session.awaitStop(t)
	awaitSupervisorState(t, supervisor, StateStopping)
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorActive, active: false}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("control while stopping error = %v", err)
	}
	launch.session.releaseStop()
	if err := awaitSupervisorError(t, disableDone); err != nil {
		t.Fatal(err)
	}
	if got := supervisor.Snapshot(); got.State != StateDisabled || got.Enabled {
		t.Fatalf("disabled snapshot = %+v", got)
	}

	publishedMu.Lock()
	states := make(map[State]bool)
	for _, snapshot := range published {
		states[snapshot.State] = true
	}
	publishedMu.Unlock()
	for _, state := range []State{StateDisabled, StateStopped, StateStarting, StateHandshaking, StateRunning, StateStopping} {
		if !states[state] {
			t.Errorf("state %q was never published", state)
		}
	}
}

func TestPluginSupervisorUsesRuntimeDisplayMetadataOnlyForCurrentReadySession(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(125, 0))
	plugin := supervisorTestPlugin()
	restart := DefaultRestartPolicy()
	restart.MaxFailures = 2
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     plugin,
		Preference: PluginPreference{Enabled: true},
		Restart:    restart,
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)

	assertManifestDisplay := func(state State) {
		t.Helper()
		awaitSupervisorState(t, supervisor, state)
		snapshot := supervisor.Snapshot()
		if snapshot.Name != plugin.Manifest.Name || snapshot.Description != plugin.Manifest.Description {
			t.Fatalf("%s display metadata = %q/%q, want manifest %q/%q", state,
				snapshot.Name, snapshot.Description, plugin.Manifest.Name, plugin.Manifest.Description)
		}
	}
	assertRuntimeDisplay := func(state State, descriptor pluginapi.Descriptor) {
		t.Helper()
		awaitSupervisorState(t, supervisor, state)
		snapshot := supervisor.Snapshot()
		if snapshot.Name != descriptor.Name || snapshot.Description != descriptor.Description {
			t.Fatalf("%s display metadata = %q/%q, want runtime %q/%q", state,
				snapshot.Name, snapshot.Description, descriptor.Name, descriptor.Description)
		}
	}

	first := factory.awaitLaunch(t)
	assertManifestDisplay(StateStarting)
	first.callbacks.ProcessStarted(first.instanceID, 1)
	runtimeOne := validHandshakeDescriptor()
	runtimeOne.Name = "Runtime One"
	runtimeOne.Description = "First authenticated runtime"
	first.callbacks.Ready(first.instanceID, runtimeOne)
	assertRuntimeDisplay(StateRunning, runtimeOne)
	first.session.finish(sessionResult{})
	assertManifestDisplay(StateStopped)

	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart}); err != nil {
		t.Fatal(err)
	}
	second := factory.awaitLaunch(t)
	second.callbacks.ProcessStarted(second.instanceID, 2)
	stale := validHandshakeDescriptor()
	stale.Name = "Stale Runtime"
	stale.Description = "Retired session metadata"
	first.callbacks.Ready(first.instanceID, stale)
	assertManifestDisplay(StateHandshaking)
	runtimeTwo := validHandshakeDescriptor()
	runtimeTwo.Name = "Runtime Two"
	runtimeTwo.Description = "Second authenticated runtime"
	second.callbacks.Ready(second.instanceID, runtimeTwo)
	assertRuntimeDisplay(StateRunning, runtimeTwo)

	second.session.blockStop()
	disableDone := make(chan error, 1)
	go func() {
		disableDone <- supervisor.Command(context.Background(), supervisorCommand{kind: supervisorDisable})
	}()
	second.session.awaitStop(t)
	assertManifestDisplay(StateStopping)
	second.session.releaseStop()
	if err := awaitSupervisorError(t, disableDone); err != nil {
		t.Fatal(err)
	}
	assertManifestDisplay(StateDisabled)

	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorEnable}); err != nil {
		t.Fatal(err)
	}
	third := factory.awaitLaunch(t)
	third.callbacks.ProcessStarted(third.instanceID, 3)
	runtimeThree := validHandshakeDescriptor()
	runtimeThree.Name = "Runtime Three"
	runtimeThree.Description = "Third authenticated runtime"
	third.callbacks.Ready(third.instanceID, runtimeThree)
	assertRuntimeDisplay(StateRunning, runtimeThree)
	third.session.finish(sessionResult{Err: errors.New("retryable failure"), Retryable: true})
	assertManifestDisplay(StateBackoff)
	clock.fireNext(t)

	fourth := factory.awaitLaunch(t)
	assertManifestDisplay(StateStarting)
	fourth.callbacks.ProcessStarted(fourth.instanceID, 4)
	assertManifestDisplay(StateHandshaking)
	runtimeFour := validHandshakeDescriptor()
	runtimeFour.Name = "Runtime Four"
	runtimeFour.Description = "Fourth authenticated runtime"
	fourth.callbacks.Ready(fourth.instanceID, runtimeFour)
	assertRuntimeDisplay(StateRunning, runtimeFour)
	fourth.session.finish(sessionResult{Err: ErrDescriptorMismatch, Retryable: false})
	assertManifestDisplay(StateIncompatible)

	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart}); err != nil {
		t.Fatal(err)
	}
	fifth := factory.awaitLaunch(t)
	fifth.callbacks.ProcessStarted(fifth.instanceID, 5)
	runtimeFive := validHandshakeDescriptor()
	runtimeFive.Name = "Runtime Five"
	runtimeFive.Description = "Fifth authenticated runtime"
	fifth.callbacks.Ready(fifth.instanceID, runtimeFive)
	assertRuntimeDisplay(StateRunning, runtimeFive)
	fifth.session.finish(sessionResult{Err: errors.New("first crash"), Retryable: true})
	assertManifestDisplay(StateBackoff)
	clock.fireNext(t)

	sixth := factory.awaitLaunch(t)
	sixth.callbacks.ProcessStarted(sixth.instanceID, 6)
	runtimeSix := validHandshakeDescriptor()
	runtimeSix.Name = "Runtime Six"
	runtimeSix.Description = "Sixth authenticated runtime"
	sixth.callbacks.Ready(sixth.instanceID, runtimeSix)
	assertRuntimeDisplay(StateRunning, runtimeSix)
	sixth.session.finish(sessionResult{Err: errors.New("second crash"), Retryable: true})
	assertManifestDisplay(StateCrashed)
}

func TestPluginSupervisorFiniteRestartAndStableReset(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(200, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart: RestartPolicy{
			InitialBackoff: time.Second,
			Multiplier:     2,
			MaxBackoff:     30 * time.Second,
			MaxFailures:    5,
			StableWindow:   time.Minute,
		},
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)

	wantDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt, wantDelay := range wantDelays {
		launch := factory.awaitLaunch(t)
		launch.session.finish(sessionResult{Err: errors.New("crash"), Retryable: true})
		awaitSupervisorState(t, supervisor, StateBackoff)
		if got := clock.awaitTimer(t).delay; got != wantDelay {
			t.Fatalf("failure %d delay = %v, want %v", attempt+1, got, wantDelay)
		}
		clock.fireNext(t)
	}
	finalLaunch := factory.awaitLaunch(t)
	finalLaunch.session.finish(sessionResult{Err: errors.New("crash"), Retryable: true})
	awaitSupervisorState(t, supervisor, StateCrashed)
	if clock.timerCount() != 0 {
		t.Fatal("restart timer remained after reaching MaxFailures")
	}
	if got := supervisor.Snapshot(); got.ConsecutiveFailures != 5 ||
		got.RestartCount != 4 || got.LastError != ErrRestartLimitReached.Error() {
		t.Fatalf("restart-limit snapshot = %+v", got)
	}

	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart}); err != nil {
		t.Fatal(err)
	}
	stableLaunch := factory.awaitLaunch(t)
	if got := supervisor.Snapshot(); got.ConsecutiveFailures != 0 || got.RestartCount != 5 {
		t.Fatalf("manual restart snapshot = %+v", got)
	}
	stableLaunch.session.finish(sessionResult{
		StartedAt: clock.now().Add(-time.Minute),
		StableFor: time.Minute,
		Err:       errors.New("late crash"),
		Retryable: true,
	})
	awaitSupervisorState(t, supervisor, StateBackoff)
	if got := supervisor.Snapshot(); got.ConsecutiveFailures != 1 || got.RestartCount != 5 {
		t.Fatalf("stable reset snapshot = %+v", got)
	}
	stableTimer := clock.awaitTimer(t)
	if got := stableTimer.delay; got != time.Second {
		t.Fatalf("post-stable delay = %v", got)
	}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart}); err != nil {
		t.Fatal(err)
	}
	factory.awaitLaunch(t)
	if !stableTimer.stopped() {
		t.Fatal("manual Restart did not cancel backoff")
	}
	if got := supervisor.Snapshot(); got.ConsecutiveFailures != 0 || got.RestartCount != 6 {
		t.Fatalf("manual backoff restart snapshot = %+v", got)
	}
}

func TestPluginSupervisorStableWindowResetsWhileSessionIsRunning(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(225, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart: RestartPolicy{
			InitialBackoff: time.Second,
			Multiplier:     2,
			MaxBackoff:     30 * time.Second,
			MaxFailures:    2,
			StableWindow:   time.Minute,
		},
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)

	first := factory.awaitLaunch(t)
	first.session.finish(sessionResult{Err: errors.New("first crash"), Retryable: true})
	awaitSupervisorState(t, supervisor, StateBackoff)
	clock.fireNext(t)
	second := factory.awaitLaunch(t)
	second.callbacks.ProcessStarted(second.instanceID, 2)
	second.callbacks.Ready(second.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	stableTimer := clock.awaitTimer(t)
	if stableTimer.delay != time.Minute {
		t.Fatalf("stable timer delay = %v", stableTimer.delay)
	}
	clock.fireNext(t)
	awaitSupervisorFailures(t, supervisor, 0)

	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorDisable}); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorEnable}); err != nil {
		t.Fatal(err)
	}
	third := factory.awaitLaunch(t)
	third.session.finish(sessionResult{Err: errors.New("post-stable crash"), Retryable: true})
	awaitSupervisorState(t, supervisor, StateBackoff)
	if got := supervisor.Snapshot().ConsecutiveFailures; got != 1 {
		t.Fatalf("post-stable failures = %d", got)
	}
}

func TestPluginSupervisorStableTimerIsCanceledWhenSessionStops(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(230, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart:    DefaultRestartPolicy(),
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)
	first := factory.awaitLaunch(t)
	first.session.finish(sessionResult{Err: errors.New("first crash"), Retryable: true})
	awaitSupervisorState(t, supervisor, StateBackoff)
	clock.fireNext(t)
	second := factory.awaitLaunch(t)
	second.callbacks.ProcessStarted(second.instanceID, 2)
	second.callbacks.Ready(second.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	stableTimer := clock.awaitTimer(t)
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorDisable}); err != nil {
		t.Fatal(err)
	}
	if !stableTimer.stopped() {
		t.Fatal("Disable did not cancel stable timer")
	}
}

func TestPluginSupervisorRetryPublishesBackoffWithoutIntermediateCrash(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(235, 0))
	var mu sync.Mutex
	var states []State
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart:    DefaultRestartPolicy(),
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
		Publish: func(snapshot RuntimeSnapshot) {
			mu.Lock()
			states = append(states, snapshot.State)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)
	launch := factory.awaitLaunch(t)
	launch.session.finish(sessionResult{Err: errors.New("retryable"), Retryable: true})
	awaitSupervisorState(t, supervisor, StateBackoff)
	mu.Lock()
	defer mu.Unlock()
	for _, state := range states {
		if state == StateCrashed {
			t.Fatalf("under-limit failure published crashed: %v", states)
		}
	}
}

func TestPluginSupervisorHonorsSessionRetryClassification(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		retryable bool
		wantState State
		wantError string
	}{
		{name: "invalid entrypoint source classification", err: ErrInvalidEntrypoint, wantState: StateIncompatible, wantError: "plugins: invalid entrypoint"},
		{name: "missing executable source classification", err: fmt.Errorf("secret executable path: %w", os.ErrNotExist), wantState: StateIncompatible, wantError: "plugins: session failed"},
		{name: "permission source classification", err: fmt.Errorf("secret working directory: %w", os.ErrPermission), wantState: StateIncompatible, wantError: "plugins: session failed"},
		{name: "handshake transport overrides protocol-shaped error", err: errors.Join(ErrProtocolViolation, os.ErrNotExist), retryable: true, wantState: StateBackoff, wantError: "plugins: protocol violation"},
		{name: "runtime IPC permission remains retryable", err: os.ErrPermission, retryable: true, wantState: StateBackoff, wantError: "plugins: session failed"},
		{name: "explicit generic non-retryable", err: errors.New("secret incompatible detail"), wantState: StateIncompatible, wantError: "plugins: session failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			factory := newSupervisorTestFactory()
			clock := newSupervisorTestClock(time.Unix(240, 0))
			supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
				Plugin:     supervisorTestPlugin(),
				Preference: PluginPreference{Enabled: true},
				Restart:    DefaultRestartPolicy(),
				NewSession: factory.newSession,
				Now:        clock.now,
				NewTimer:   clock.newTimer,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer closeSupervisor(t, supervisor)
			launch := factory.awaitLaunch(t)
			launch.session.finish(sessionResult{Err: test.err, Retryable: test.retryable})
			awaitSupervisorState(t, supervisor, test.wantState)
			if got := supervisor.Snapshot().LastError; got != test.wantError ||
				strings.Contains(got, "secret") {
				t.Fatalf("sanitized startup error = %q, want %q", got, test.wantError)
			}
			if test.wantState == StateIncompatible && clock.timerCount() != 0 {
				t.Fatal("startup contract error scheduled restart")
			}
		})
	}
}

func TestPluginSupervisorStoppedControlsBecomeNextStartup(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(150, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart:    DefaultRestartPolicy(),
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)
	first := factory.awaitLaunch(t)
	first.session.finish(sessionResult{})
	awaitSupervisorState(t, supervisor, StateStopped)
	config := pluginapi.Config{Revision: 1, Data: []byte(`{"next":true}`)}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorConfig, config: config}); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorEnable}); err != nil {
		t.Fatal(err)
	}
	second := factory.awaitLaunch(t)
	if string(second.startup.Config.Data) != `{"next":true}` {
		t.Fatalf("next startup config = %s", second.startup.Config.Data)
	}
}

func TestPluginSupervisorBackoffCapAndOverflowSafety(t *testing.T) {
	defaultShape := RestartPolicy{
		InitialBackoff: time.Second,
		Multiplier:     2,
		MaxBackoff:     30 * time.Second,
		MaxFailures:    8,
		StableWindow:   time.Minute,
	}
	for failures, want := range []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second,
		30 * time.Second,
	} {
		if got := restartDelay(defaultShape, failures+1); got != want {
			t.Fatalf("failure %d delay = %v, want %v", failures+1, got, want)
		}
	}

	policy := RestartPolicy{
		InitialBackoff: 20 * time.Second,
		Multiplier:     ^uint(0),
		MaxBackoff:     30 * time.Second,
		MaxFailures:    10,
		StableWindow:   time.Minute,
	}
	if got := restartDelay(policy, 1); got != 20*time.Second {
		t.Fatalf("first delay = %v", got)
	}
	for _, failures := range []int{2, 3, 9, int(^uint(0) >> 1)} {
		if got := restartDelay(policy, failures); got != 30*time.Second {
			t.Fatalf("failure %d delay = %v", failures, got)
		}
	}
}

func TestPluginSupervisorBlockedControlDoesNotBlockClose(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(360, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:             supervisorTestPlugin(),
		Preference:         PluginPreference{Enabled: true},
		Restart:            DefaultRestartPolicy(),
		NewSession:         factory.newSession,
		Now:                clock.now,
		NewTimer:           clock.newTimer,
		SupervisorCapacity: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	launch := factory.awaitLaunch(t)
	launch.session.blockControl()
	controlDone := make(chan error, 1)
	go func() {
		controlDone <- supervisor.Command(context.Background(), supervisorCommand{
			kind:   supervisorConfig,
			config: pluginapi.Config{Revision: 1, Data: []byte(`{"blocked":true}`)},
		})
	}()
	launch.session.awaitControlStart(t)

	closeDone := make(chan error, 1)
	go func() { closeDone <- supervisor.Close(context.Background()) }()
	launch.session.awaitStop(t)
	if err := awaitSupervisorError(t, closeDone); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if err := awaitSupervisorError(t, controlDone); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("retired control error = %v", err)
	}
}

func TestPluginSupervisorSerializesAndBoundsAsyncControls(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(365, 0))
	supervisorAPI, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:             supervisorTestPlugin(),
		Preference:         PluginPreference{Enabled: true},
		Restart:            DefaultRestartPolicy(),
		NewSession:         factory.newSession,
		Now:                clock.now,
		NewTimer:           clock.newTimer,
		SupervisorCapacity: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisorAPI)
	supervisor := supervisorAPI.(*serializedPluginSupervisor)
	launch := factory.awaitLaunch(t)
	launch.session.blockControl()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- supervisor.Command(context.Background(), supervisorCommand{
			kind:   supervisorConfig,
			config: pluginapi.Config{Revision: 1, Data: []byte(`{"order":1}`)},
		})
	}()
	launch.session.awaitControlStart(t)

	secondReply := make(chan error, 1)
	supervisor.commands <- supervisorCommandEnvelope{
		ctx: context.Background(),
		command: supervisorCommand{
			kind:   supervisorConfig,
			config: pluginapi.Config{Revision: 2, Data: []byte(`{"order":2}`)},
			reply:  secondReply,
		},
	}
	awaitSupervisorCommandDrain(t, supervisor)
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorActive, active: true}); !errors.Is(err, ErrControlBackpressure) {
		t.Fatalf("control beyond capacity error = %v", err)
	}
	launch.session.releaseControl()
	if err := awaitSupervisorError(t, firstDone); err != nil {
		t.Fatal(err)
	}
	if err := awaitSupervisorError(t, secondReply); err != nil {
		t.Fatal(err)
	}
	first := launch.session.awaitControl(t)
	second := launch.session.awaitControl(t)
	if first.state.Config.Revision != 1 || second.state.Config.Revision != 2 {
		t.Fatalf("control order = %d, %d", first.state.Config.Revision, second.state.Config.Revision)
	}
}

func TestPluginSupervisorCloseTakesOverExistingStop(t *testing.T) {
	for _, test := range []struct {
		name           string
		first          supervisorCommandKind
		wantFirstError error
	}{
		{name: "disable", first: supervisorDisable},
		{name: "restart", first: supervisorRestart, wantFirstError: ErrInvalidState},
	} {
		t.Run(test.name, func(t *testing.T) {
			factory := newSupervisorTestFactory()
			clock := newSupervisorTestClock(time.Unix(370, 0))
			supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
				Plugin:     supervisorTestPlugin(),
				Preference: PluginPreference{Enabled: true},
				Restart:    DefaultRestartPolicy(),
				NewSession: factory.newSession,
				Now:        clock.now,
				NewTimer:   clock.newTimer,
			})
			if err != nil {
				t.Fatal(err)
			}
			launch := factory.awaitLaunch(t)
			launch.session.blockStop()
			firstDone := make(chan error, 1)
			go func() {
				firstDone <- supervisor.Command(context.Background(), supervisorCommand{kind: test.first})
			}()
			launch.session.awaitStop(t)
			closeDone := make(chan error, 1)
			go func() { closeDone <- supervisor.Close(context.Background()) }()
			awaitSupervisorRejectsCommands(t, supervisor)
			launch.session.releaseStop()
			if err := awaitSupervisorError(t, firstDone); !errors.Is(err, test.wantFirstError) {
				t.Fatalf("%s error = %v, want %v", test.name, err, test.wantFirstError)
			}
			if err := awaitSupervisorError(t, closeDone); err != nil {
				t.Fatalf("Close error = %v", err)
			}
			if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorEnable}); !errors.Is(err, ErrManagerClosed) {
				t.Fatalf("post-close command error = %v", err)
			}
		})
	}
}

func TestPluginSupervisorStopOutcomeRetiresLateSessionResult(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(380, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart:    DefaultRestartPolicy(),
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)
	launch := factory.awaitLaunch(t)
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorDisable}); err != nil {
		t.Fatal(err)
	}
	awaitSupervisorState(t, supervisor, StateDisabled)
	launch.session.finish(sessionResult{Err: errors.New("late crash"), Retryable: true})
	time.Sleep(10 * time.Millisecond)
	if got := supervisor.Snapshot(); got.State != StateDisabled || got.NextRestartAt != (time.Time{}) ||
		got.ConsecutiveFailures != 0 {
		t.Fatalf("late Done polluted disabled snapshot = %+v", got)
	}
	if clock.timerCount() != 0 {
		t.Fatal("late Done scheduled restart")
	}
}

func TestPluginSupervisorCriticalCallbacksSurviveTelemetryBackpressure(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(390, 0))
	supervisorAPI, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:             supervisorTestPlugin(),
		Preference:         PluginPreference{Enabled: true},
		Restart:            DefaultRestartPolicy(),
		NewSession:         factory.newSession,
		Now:                clock.now,
		NewTimer:           clock.newTimer,
		SupervisorCapacity: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisorAPI)
	supervisor := supervisorAPI.(*serializedPluginSupervisor)
	launch := factory.awaitLaunch(t)
	supervisor.callback <- supervisorCallback{
		kind:       supervisorHeartbeat,
		instanceID: launch.instanceID,
		at:         clock.now(),
	}
	launch.callbacks.ProcessStarted(launch.instanceID, 55)
	launch.callbacks.Ready(launch.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	if got := supervisor.Snapshot().PID; got != 55 {
		t.Fatalf("PID = %d", got)
	}
}

func TestPluginSupervisorTelemetryLogLossComposesOnNextPublishedLog(t *testing.T) {
	published := make(chan observedPluginLog, 1)
	supervisor := &serializedPluginSupervisor{
		config: pluginSupervisorConfig{PublishLog: func(log observedPluginLog) {
			published <- log
		}},
		callback: make(chan supervisorCallback, 1),
		done:     make(chan struct{}),
	}
	callbacks := supervisor.sessionCallbacks(91)
	supervisor.callback <- supervisorCallback{kind: supervisorHeartbeat, instanceID: 91}

	producerReturned := make(chan struct{})
	go func() {
		callbacks.Log(91, observedPluginLog{
			Entry:   pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "lost one"},
			Dropped: 4,
		})
		close(producerReturned)
	}()
	select {
	case <-producerReturned:
	case <-time.After(time.Second):
		t.Fatal("Log callback blocked on full telemetry queue")
	}
	callbacks.Log(91, observedPluginLog{
		Entry:   pluginapi.LogEntry{Level: pluginapi.LogWarn, Message: "lost two"},
		Dropped: 2,
	})
	callbacks.Heartbeat(91, time.Unix(1, 0))
	callbacks.Frame(91, time.Unix(2, 0), 60)
	callbacks.Status(91, pluginapi.DeviceStatus{State: pluginapi.DeviceReady})

	<-supervisor.callback
	callbacks.Log(91, observedPluginLog{
		Entry:   pluginapi.LogEntry{Level: pluginapi.LogError, Message: "delivered"},
		Dropped: 3,
	})
	accepted := <-supervisor.callback
	state := supervisorLoopState{
		snapshot:   RuntimeSnapshot{State: StateRunning},
		session:    newSupervisorTestSession(),
		instanceID: 91,
	}
	supervisor.handleCallback(&state, accepted)

	got := awaitValue(t, published)
	if got.Entry.Message != "delivered" || got.Dropped != 11 {
		t.Fatalf("published log = %+v, want delivered with Dropped 11", got)
	}
}

func TestPluginSupervisorClearsRuntimeMetricsAcrossRestart(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(395, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart:    DefaultRestartPolicy(),
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)
	launch := factory.awaitLaunch(t)
	launch.callbacks.ProcessStarted(launch.instanceID, 77)
	launch.callbacks.Ready(launch.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	launch.callbacks.Heartbeat(launch.instanceID, clock.now())
	launch.callbacks.Frame(launch.instanceID, clock.now(), 90)
	launch.session.finish(sessionResult{Err: errors.New("crash"), Retryable: true})
	awaitSupervisorState(t, supervisor, StateBackoff)
	assertSupervisorRuntimeMetricsZero(t, supervisor.Snapshot())
	clock.fireNext(t)
	factory.awaitLaunch(t)
	awaitSupervisorState(t, supervisor, StateStarting)
	assertSupervisorRuntimeMetricsZero(t, supervisor.Snapshot())
}

func TestPluginSupervisorSuppressesNonRetryableAndIgnoresStaleCallbacks(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		retryable bool
		wantError string
	}{
		{name: "authentication", err: ErrAuthenticationFailed, wantError: "plugins: authentication failed"},
		{name: "descriptor", err: ErrDescriptorMismatch, wantError: "plugins: descriptor mismatch"},
		{name: "API", err: ErrProtocolIncompatible, wantError: "plugins: protocol incompatible"},
		{name: "protocol", err: ErrProtocolViolation, wantError: "plugins: protocol violation"},
		{name: "manifest", err: ErrInvalidManifest, wantError: "plugins: invalid manifest"},
		{name: "config", err: ErrConfigRevisionConflict, wantError: "plugins: config revision conflict"},
		{name: "opaque", err: errors.New("token=do-not-publish"), wantError: "plugins: session failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			factory := newSupervisorTestFactory()
			clock := newSupervisorTestClock(time.Unix(300, 0))
			supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
				Plugin:     supervisorTestPlugin(),
				Preference: PluginPreference{Enabled: true},
				Restart:    DefaultRestartPolicy(),
				NewSession: factory.newSession,
				Now:        clock.now,
				NewTimer:   clock.newTimer,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer closeSupervisor(t, supervisor)
			launch := factory.awaitLaunch(t)
			launch.session.finish(sessionResult{Err: test.err, Retryable: test.retryable})
			awaitSupervisorState(t, supervisor, StateIncompatible)
			if clock.timerCount() != 0 || factory.launchCount() != 1 {
				t.Fatal("non-retryable result scheduled a restart")
			}
			if got := supervisor.Snapshot().LastError; got != test.wantError {
				t.Fatalf("sanitized error = %q, want %q", got, test.wantError)
			}

			if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart}); err != nil {
				t.Fatal(err)
			}
			newLaunch := factory.awaitLaunch(t)
			launch.callbacks.ProcessStarted(launch.instanceID, 1)
			launch.callbacks.Ready(launch.instanceID, validHandshakeDescriptor())
			launch.callbacks.Heartbeat(launch.instanceID, clock.now())
			if got := supervisor.Snapshot().State; got != StateStarting {
				t.Fatalf("stale callback changed state to %q", got)
			}
			newLaunch.callbacks.ProcessStarted(newLaunch.instanceID, 2)
			newLaunch.callbacks.Ready(newLaunch.instanceID, validHandshakeDescriptor())
			awaitSupervisorState(t, supervisor, StateRunning)
			newLaunch.callbacks.Unresponsive(newLaunch.instanceID)
			awaitSupervisorState(t, supervisor, StateUnresponsive)
		})
	}
}

func TestPluginSupervisorIgnoresStaleSessionResult(t *testing.T) {
	factory := newSupervisorTestFactory()
	clock := newSupervisorTestClock(time.Unix(350, 0))
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:     supervisorTestPlugin(),
		Preference: PluginPreference{Enabled: true},
		Restart:    DefaultRestartPolicy(),
		NewSession: factory.newSession,
		Now:        clock.now,
		NewTimer:   clock.newTimer,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeSupervisor(t, supervisor)
	old := factory.awaitLaunch(t)
	if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart}); err != nil {
		t.Fatal(err)
	}
	current := factory.awaitLaunch(t)
	old.session.finish(sessionResult{Err: errors.New("stale crash"), Retryable: true})
	current.callbacks.ProcessStarted(current.instanceID, 9)
	current.callbacks.Ready(current.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	if got := supervisor.Snapshot(); got.ConsecutiveFailures != 0 || got.RestartCount != 1 {
		t.Fatalf("stale result changed snapshot = %+v", got)
	}
}

func TestPluginSupervisorDisableCloseCancelBackoffAndCommandsHonorContext(t *testing.T) {
	for _, action := range []supervisorCommandKind{supervisorDisable, supervisorClose} {
		t.Run(action.String(), func(t *testing.T) {
			factory := newSupervisorTestFactory()
			clock := newSupervisorTestClock(time.Unix(400, 0))
			supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
				Plugin:     supervisorTestPlugin(),
				Preference: PluginPreference{Enabled: true},
				Restart:    DefaultRestartPolicy(),
				NewSession: factory.newSession,
				Now:        clock.now,
				NewTimer:   clock.newTimer,
			})
			if err != nil {
				t.Fatal(err)
			}
			launch := factory.awaitLaunch(t)
			launch.session.finish(sessionResult{Err: errors.New("crash"), Retryable: true})
			awaitSupervisorState(t, supervisor, StateBackoff)
			timer := clock.awaitTimer(t)
			if action == supervisorClose {
				if err := supervisor.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else if err := supervisor.Command(context.Background(), supervisorCommand{kind: action}); err != nil {
				t.Fatal(err)
			}
			if !timer.stopped() {
				t.Fatal("backoff timer was not stopped")
			}
			timer.fire()
			if got := factory.launchCount(); got != 1 {
				t.Fatalf("launch count after canceled timer = %d", got)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := supervisor.Command(ctx, supervisorCommand{kind: supervisorEnable}); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled command error = %v", err)
			}
			if action == supervisorClose {
				if err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorEnable}); !errors.Is(err, ErrManagerClosed) {
					t.Fatalf("post-close command error = %v", err)
				}
			} else {
				closeSupervisor(t, supervisor)
			}
		})
	}
}

func TestPluginSupervisorDefaultPolicy(t *testing.T) {
	policy := DefaultRestartPolicy()
	if policy.InitialBackoff != time.Second || policy.Multiplier != 2 ||
		policy.MaxBackoff != 30*time.Second || policy.MaxFailures != 5 ||
		policy.StableWindow != time.Minute {
		t.Fatalf("default policy = %+v", policy)
	}
}

func TestPluginSupervisorAdmissionCompletesOnCanceledAndClosedCommands(t *testing.T) {
	factory := newSupervisorTestFactory()
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin:       supervisorTestPlugin(),
		Preference:   PluginPreference{},
		Restart:      DefaultRestartPolicy(),
		NewSession:   factory.newSession,
		Subscription: pluginapi.Subscription{},
	})
	if err != nil {
		t.Fatalf("newPluginSupervisor() error = %v", err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledAdmission := newSupervisorAdmission()
	err = supervisor.Command(canceledCtx, supervisorCommand{
		kind:      supervisorRestart,
		admission: canceledAdmission,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Command error = %v, want context.Canceled", err)
	}
	if err := canceledAdmission.wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission error = %v, want context.Canceled", err)
	}

	if err := supervisor.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	closedAdmission := newSupervisorAdmission()
	err = supervisor.Command(context.Background(), supervisorCommand{
		kind:      supervisorRestart,
		admission: closedAdmission,
	})
	if !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("closed Command error = %v, want ErrManagerClosed", err)
	}
	if err := closedAdmission.wait(); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("closed admission error = %v, want ErrManagerClosed", err)
	}
}

func TestPluginSupervisorRetainsAcceptedConfigAcrossRetiredRuntimeDelivery(t *testing.T) {
	factory := newSupervisorTestFactory()
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin: managerTestPlugin("vendor.config"),
		Preference: PluginPreference{
			Enabled: true,
			Config:  pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)},
		},
		Restart:      DefaultRestartPolicy(),
		NewSession:   factory.newSession,
		Subscription: pluginapi.Subscription{},
	})
	if err != nil {
		t.Fatalf("newPluginSupervisor() error = %v", err)
	}
	defer closeSupervisor(t, supervisor)

	first := factory.awaitLaunch(t)
	first.callbacks.ProcessStarted(first.instanceID, 1001)
	first.callbacks.Ready(first.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	first.session.blockControl()

	configResult := make(chan error, 1)
	go func() {
		configResult <- supervisor.Command(context.Background(), supervisorCommand{
			kind:   supervisorConfig,
			config: pluginapi.Config{Revision: 2, Data: []byte(`{"gain":2}`)},
		})
	}()
	first.session.awaitControlStart(t)

	restartResult := make(chan error, 1)
	go func() {
		restartResult <- supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart})
	}()
	if err := awaitSupervisorError(t, configResult); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("retired Config error = %v, want ErrInvalidState", err)
	}
	second := factory.awaitLaunch(t)
	if got := second.startup.Config; got.Revision != 2 || string(got.Data) != `{"gain":2}` {
		t.Fatalf("next Startup.Config = %+v, want accepted persisted rev2", got)
	}
	if err := awaitSupervisorError(t, restartResult); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
}

func TestPluginSupervisorPublishesDurableConfigRuntimeFailureAndClearsItOnSuccess(t *testing.T) {
	factory := newSupervisorTestFactory()
	supervisor, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin: managerTestPlugin("vendor.config"),
		Preference: PluginPreference{
			Enabled: true,
			Config:  pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)},
		},
		Restart:      DefaultRestartPolicy(),
		NewSession:   factory.newSession,
		Subscription: pluginapi.Subscription{},
	})
	if err != nil {
		t.Fatalf("newPluginSupervisor() error = %v", err)
	}
	defer closeSupervisor(t, supervisor)

	launch := factory.awaitLaunch(t)
	launch.callbacks.ProcessStarted(launch.instanceID, 1001)
	launch.callbacks.Ready(launch.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	runtimeErr := errors.New("runtime delivery failed")
	launch.session.controlErr = runtimeErr

	err = supervisor.Command(context.Background(), supervisorCommand{
		kind:   supervisorConfig,
		config: pluginapi.Config{Revision: 2, Data: []byte(`{"gain":2}`)},
	})
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("Config rev2 error = %v, want runtime failure", err)
	}
	snapshot := supervisor.Snapshot()
	if snapshot.ConfigRevision != 2 || snapshot.LastError != "plugins: session failed" {
		t.Fatalf("snapshot after Config failure = %+v, want durable rev2 and sanitized error", snapshot)
	}

	launch.session.controlErr = nil
	if err := supervisor.Command(context.Background(), supervisorCommand{
		kind:   supervisorConfig,
		config: pluginapi.Config{Revision: 3, Data: []byte(`{"gain":3}`)},
	}); err != nil {
		t.Fatalf("Config rev3 error = %v", err)
	}
	snapshot = supervisor.Snapshot()
	if snapshot.ConfigRevision != 3 || snapshot.LastError != "" {
		t.Fatalf("snapshot after Config recovery = %+v, want rev3 and cleared error", snapshot)
	}
}

func TestPluginSupervisorRetainsDurableConfigWhenRuntimeQueueIsFull(t *testing.T) {
	factory := newSupervisorTestFactory()
	supervisorAPI, err := newPluginSupervisor(pluginSupervisorConfig{
		Plugin: managerTestPlugin("vendor.config"),
		Preference: PluginPreference{
			Enabled: true,
			Config:  pluginapi.Config{Revision: 1, Data: []byte(`{"gain":1}`)},
		},
		Restart:            DefaultRestartPolicy(),
		NewSession:         factory.newSession,
		Subscription:       pluginapi.Subscription{},
		SupervisorCapacity: 1,
	})
	if err != nil {
		t.Fatalf("newPluginSupervisor() error = %v", err)
	}
	defer closeSupervisor(t, supervisorAPI)
	supervisor := supervisorAPI.(*serializedPluginSupervisor)

	first := factory.awaitLaunch(t)
	first.callbacks.ProcessStarted(first.instanceID, 1001)
	first.callbacks.Ready(first.instanceID, validHandshakeDescriptor())
	awaitSupervisorState(t, supervisor, StateRunning)
	first.session.blockControl()

	inFlight := make(chan error, 1)
	go func() {
		inFlight <- supervisor.Command(context.Background(), supervisorCommand{
			kind:   supervisorActive,
			active: true,
		})
	}()
	first.session.awaitControlStart(t)

	queued := make(chan error, 1)
	supervisor.commands <- supervisorCommandEnvelope{
		ctx: context.Background(),
		command: supervisorCommand{
			kind:   supervisorActive,
			active: false,
			reply:  queued,
		},
	}
	awaitSupervisorCommandDrain(t, supervisor)

	err = supervisor.Command(context.Background(), supervisorCommand{
		kind:   supervisorConfig,
		config: pluginapi.Config{Revision: 2, Data: []byte(`{"gain":2}`)},
	})
	if !errors.Is(err, ErrControlBackpressure) {
		t.Fatalf("Config with full runtime queue error = %v, want ErrControlBackpressure", err)
	}
	if snapshot := supervisor.Snapshot(); snapshot.ConfigRevision != 2 {
		t.Fatalf("snapshot ConfigRevision = %d, want durable rev2 after backpressure", snapshot.ConfigRevision)
	}

	restart := make(chan error, 1)
	go func() {
		restart <- supervisor.Command(context.Background(), supervisorCommand{kind: supervisorRestart})
	}()
	if err := awaitSupervisorError(t, inFlight); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("retired in-flight control error = %v, want ErrInvalidState", err)
	}
	if err := awaitSupervisorError(t, queued); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("retired queued control error = %v, want ErrInvalidState", err)
	}
	second := factory.awaitLaunch(t)
	if got := second.startup.Config; got.Revision != 2 || string(got.Data) != `{"gain":2}` {
		t.Fatalf("next Startup.Config = %+v, want durable rev2 after backpressure", got)
	}
	if err := awaitSupervisorError(t, restart); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
}

type supervisorTestFactory struct {
	mu       sync.Mutex
	launches []*supervisorTestLaunch
	notify   chan struct{}
}

type supervisorTestLaunch struct {
	instanceID uint64
	startup    pluginapi.Startup
	callbacks  supervisorSessionCallbacks
	session    *supervisorTestSession
}

func newSupervisorTestFactory() *supervisorTestFactory {
	return &supervisorTestFactory{notify: make(chan struct{}, 32)}
}

func (f *supervisorTestFactory) newSession(
	_ context.Context,
	instanceID uint64,
	startup pluginapi.Startup,
	callbacks supervisorSessionCallbacks,
) pluginSession {
	session := newSupervisorTestSession()
	launch := &supervisorTestLaunch{
		instanceID: instanceID,
		startup:    cloneStartup(startup),
		callbacks:  callbacks,
		session:    session,
	}
	f.mu.Lock()
	f.launches = append(f.launches, launch)
	f.mu.Unlock()
	f.notify <- struct{}{}
	return session
}

func (f *supervisorTestFactory) awaitLaunch(t *testing.T) *supervisorTestLaunch {
	t.Helper()
	select {
	case <-f.notify:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for launch")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.launches[len(f.launches)-1]
}

func (f *supervisorTestFactory) launchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.launches)
}

type supervisorTestSession struct {
	done           chan sessionResult
	controls       chan controlRequest
	controlStarted chan struct{}
	controlGate    chan struct{}
	stopCall       chan struct{}
	stopGate       chan struct{}
	stopOnce       sync.Once
	controlErr     error
}

func newSupervisorTestSession() *supervisorTestSession {
	stopGate := make(chan struct{})
	close(stopGate)
	controlGate := make(chan struct{})
	close(controlGate)
	return &supervisorTestSession{
		done:           make(chan sessionResult, 1),
		controls:       make(chan controlRequest, 16),
		controlStarted: make(chan struct{}, 16),
		controlGate:    controlGate,
		stopCall:       make(chan struct{}),
		stopGate:       stopGate,
	}
}

func (s *supervisorTestSession) Control(ctx context.Context, request controlRequest) error {
	if request.kind == controlConfig {
		request.state.Config = request.state.Config.Clone()
	}
	select {
	case s.controls <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.controlStarted <- struct{}{}
	select {
	case <-s.controlGate:
		return s.controlErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *supervisorTestSession) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() { close(s.stopCall) })
	select {
	case <-s.stopGate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *supervisorTestSession) Done() <-chan sessionResult { return s.done }

func (s *supervisorTestSession) finish(result sessionResult) {
	s.done <- result
	close(s.done)
}

func (s *supervisorTestSession) blockStop() { s.stopGate = make(chan struct{}) }
func (s *supervisorTestSession) blockControl() {
	s.controlGate = make(chan struct{})
}
func (s *supervisorTestSession) releaseControl() {
	close(s.controlGate)
}
func (s *supervisorTestSession) releaseStop() {
	close(s.stopGate)
}

func (s *supervisorTestSession) awaitStop(t *testing.T) {
	t.Helper()
	select {
	case <-s.stopCall:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Stop")
	}
}

func (s *supervisorTestSession) awaitControlStart(t *testing.T) {
	t.Helper()
	select {
	case <-s.controlStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Control start")
	}
}

func (s *supervisorTestSession) awaitControl(t *testing.T) controlRequest {
	t.Helper()
	select {
	case request := <-s.controls:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for control")
		return controlRequest{}
	}
}

type supervisorTestClock struct {
	mu     sync.Mutex
	value  time.Time
	timers []*supervisorTestTimer
	notify chan struct{}
}

type supervisorTestTimer struct {
	delay time.Duration
	ch    chan time.Time
	mu    sync.Mutex
	stop  bool
}

func newSupervisorTestClock(now time.Time) *supervisorTestClock {
	return &supervisorTestClock{value: now, notify: make(chan struct{}, 32)}
}

func (c *supervisorTestClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *supervisorTestClock) newTimer(delay time.Duration) supervisorTimer {
	timer := &supervisorTestTimer{delay: delay, ch: make(chan time.Time, 1)}
	c.mu.Lock()
	c.timers = append(c.timers, timer)
	c.mu.Unlock()
	c.notify <- struct{}{}
	return timer
}

func (c *supervisorTestClock) awaitTimer(t *testing.T) *supervisorTestTimer {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case <-c.notify:
		case <-deadline:
			t.Fatal("timed out waiting for timer")
		}
		c.mu.Lock()
		c.removeStoppedTimersLocked()
		if len(c.timers) != 0 {
			timer := c.timers[0]
			c.mu.Unlock()
			return timer
		}
		c.mu.Unlock()
	}
}

func (c *supervisorTestClock) fireNext(t *testing.T) {
	t.Helper()
	select {
	case <-c.notify:
	default:
	}
	c.mu.Lock()
	c.removeStoppedTimersLocked()
	if len(c.timers) == 0 {
		c.mu.Unlock()
		t.Fatal("no timer to fire")
	}
	timer := c.timers[0]
	c.timers = c.timers[1:]
	c.mu.Unlock()
	timer.fire()
}

func (c *supervisorTestClock) removeStoppedTimersLocked() {
	for len(c.timers) != 0 && c.timers[0].stopped() {
		c.timers = c.timers[1:]
	}
}

func (c *supervisorTestClock) timerCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, timer := range c.timers {
		if !timer.stopped() {
			count++
		}
	}
	return count
}

func (t *supervisorTestTimer) C() <-chan time.Time { return t.ch }
func (t *supervisorTestTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.stop
	t.stop = true
	return wasActive
}
func (t *supervisorTestTimer) stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stop
}
func (t *supervisorTestTimer) fire() {
	t.mu.Lock()
	stopped := t.stop
	t.mu.Unlock()
	if !stopped {
		t.ch <- time.Now()
	}
}

func supervisorTestPlugin() InstalledPlugin {
	return InstalledPlugin{Manifest: Manifest{
		ID:           "vendor.device",
		Name:         "Device",
		Version:      "1.2.3",
		Capabilities: trackingmodel.CapabilityEye,
	}}
}

func awaitSupervisorState(t *testing.T, supervisor pluginSupervisor, want State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if supervisor.Snapshot().State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state = %q, want %q", supervisor.Snapshot().State, want)
}

func awaitSupervisorFailures(t *testing.T, supervisor pluginSupervisor, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if supervisor.Snapshot().ConsecutiveFailures == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("failures = %d, want %d", supervisor.Snapshot().ConsecutiveFailures, want)
}

func closeSupervisor(t *testing.T, supervisor pluginSupervisor) {
	t.Helper()
	if err := supervisor.Close(context.Background()); err != nil && !errors.Is(err, ErrManagerClosed) {
		t.Fatal(err)
	}
}

func awaitSupervisorError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command")
		return nil
	}
}

func awaitSupervisorRejectsCommands(t *testing.T, supervisor pluginSupervisor) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		err := supervisor.Command(context.Background(), supervisorCommand{kind: supervisorEnable})
		if errors.Is(err, ErrManagerClosed) {
			return
		}
		if err != nil && !errors.Is(err, ErrInvalidState) {
			t.Fatalf("command while awaiting Close = %v", err)
		}
	}
	t.Fatal("Close was not accepted")
}

func awaitSupervisorCommandDrain(t *testing.T, supervisor *serializedPluginSupervisor) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(supervisor.commands) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("supervisor did not drain command")
}

func assertSupervisorRuntimeMetricsZero(t *testing.T, snapshot RuntimeSnapshot) {
	t.Helper()
	if snapshot.PID != 0 || !snapshot.StartedAt.IsZero() ||
		!snapshot.LastHeartbeatAt.IsZero() || !snapshot.LastFrameAt.IsZero() ||
		snapshot.FrameRate != 0 {
		t.Fatalf("runtime metrics were retained: %+v", snapshot)
	}
}
