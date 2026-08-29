package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
)

func TestRuntimeAPIRootPhasesAndExactPublicSurface(t *testing.T) {
	now := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	api := newRuntimeAPI(true, func() time.Time {
		now = now.Add(time.Second)
		return now
	})

	initial := api.GetStatus()
	if initial.Revision != 1 || initial.Phase != "created" || !initial.PlatformSupported || initial.Application != nil || initial.Problem != nil {
		t.Fatalf("initial runtime response = %+v", initial)
	}
	if methods := reflect.TypeOf(api).NumMethod(); methods != 1 {
		t.Fatalf("RuntimeAPI exported method count = %d, want exactly GetStatus", methods)
	}
	if _, ok := reflect.TypeOf(api).MethodByName("GetStatus"); !ok {
		t.Fatal("RuntimeAPI does not expose GetStatus")
	}

	before := initial
	api.setPhase(runtimePhaseCreated)
	if got := api.GetStatus(); got.Revision != before.Revision || !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("equal phase changed snapshot: before=%+v after=%+v", before, got)
	}

	for index, phase := range []runtimePhase{
		runtimePhaseStarting,
		runtimePhaseRunning,
		runtimePhaseDiagnostic,
		runtimePhaseClosing,
		runtimePhaseClosed,
	} {
		api.setPhase(phase)
		got := api.GetStatus()
		if got.Phase != string(phase) || got.Revision != uint64(index+2) {
			t.Fatalf("phase %q response = %+v", phase, got)
		}
	}
}

func TestRuntimeAPIApplicationConversionIsBoundedOwnedAndPreservesRootState(t *testing.T) {
	api := newRuntimeAPI(false, time.Now)
	problem := &Problem{Code: ProblemUnsupportedPlatform, Message: "platform is not supported"}
	api.setRootState(runtimePhaseDiagnostic, problem)

	failures := make([]application.PluginControlFailure, 70)
	for index := range failures {
		failures[index] = application.PluginControlFailure{
			PluginID:  strings.Repeat("p", 600),
			Operation: strings.Repeat("o", 600),
			Message:   strings.Repeat("m", 600),
		}
	}
	status := application.Status{
		Revision:            41,
		UpdatedAt:           time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC),
		Lifecycle:           application.LifecycleDegraded,
		AvatarID:            "avtr_demo",
		PlanGeneration:      9,
		PlanStatus:          avatar.StatusReady,
		PlanSource:          avatar.SourceFallback,
		ConfigPath:          "C:/VRChat/OSC/avatar.json",
		ConfigID:            "config_demo",
		GenerationExhausted: true,
		OSC: osc.OSCStatus{
			Running: true, Connected: false, HasTarget: true,
			TargetMode: osc.TargetModeManual,
			Target:     osc.OSCTarget{Host: "127.0.0.1", Port: 9000},
			LastError:  strings.Repeat("x", 600),
		},
		PluginFailures: failures,
		PlanError:      strings.Repeat("q", 600),
		RuntimeError:   strings.Repeat("r", 600),
	}

	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan application.Status, 1)
	done := make(chan struct{})
	go func() {
		api.consumeStatus(ctx, updates)
		close(done)
	}()
	updates <- status
	got := waitRuntimeRevision(t, api, 3)

	if got.Phase != "diagnostic" || got.PlatformSupported || got.Problem == nil || got.Problem.Code != ProblemUnsupportedPlatform {
		t.Fatalf("Application status overwrote root state: %+v", got)
	}
	app := got.Application
	if app == nil || app.Lifecycle != "degraded" || app.PlanStatus != "ready" || app.PlanSource != "fallback" {
		t.Fatalf("Application enum conversion = %+v", app)
	}
	if !app.OSC.Running || app.OSC.Connected || !app.OSC.HasTarget || app.OSC.TargetMode != "manual" || app.OSC.Target.Host != "127.0.0.1" || app.OSC.Target.Port != 9000 {
		t.Fatalf("manual OSC conversion = %+v", app.OSC)
	}
	if len(app.PluginFailures) != 64 {
		t.Fatalf("PluginFailures length = %d, want bounded 64", len(app.PluginFailures))
	}
	for _, value := range []string{app.PluginFailures[0].PluginID, app.PluginFailures[0].Operation, app.PluginFailures[0].Message, app.OSC.LastError, app.PlanError, app.RuntimeError} {
		if len(value) > 512 {
			t.Fatalf("runtime diagnostic exceeds 512 bytes: %d", len(value))
		}
	}

	got.Application.PluginFailures[0].Message = "caller mutation"
	got.Application.OSC.Target.Host = "caller mutation"
	got.Problem.Message = "caller mutation"
	again := api.GetStatus()
	if again.Application.PluginFailures[0].Message == "caller mutation" || again.Application.OSC.Target.Host == "caller mutation" || again.Problem.Message == "caller mutation" {
		t.Fatalf("GetStatus returned aliased data: %+v", again)
	}

	beforeRevision := again.Revision
	status.Revision++
	status.UpdatedAt = status.UpdatedAt.Add(time.Hour)
	api.setApplicationStatus(status)
	if equal := api.GetStatus(); equal.Revision != beforeRevision {
		t.Fatalf("semantically equal Application status changed revision: %d -> %d", beforeRevision, equal.Revision)
	}

	status.OSC = osc.OSCStatus{
		Running: true, Connected: true, HasTarget: false,
		TargetMode: osc.TargetModeAuto,
		Target:     osc.OSCTarget{Host: "stale.invalid", Port: 1},
	}
	api.setApplicationStatus(status)
	changed := api.GetStatus()
	if !changed.Application.OSC.Connected || changed.Application.OSC.HasTarget || changed.Application.OSC.TargetMode != "auto" || changed.Application.OSC.Target != (OSCTargetDTO{}) {
		t.Fatalf("automatic discovery/target conversion = %+v", changed.Application.OSC)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RuntimeAPI status consumer did not stop after cancellation")
	}
	afterCancel := api.GetStatus().Revision
	updates <- application.Status{Lifecycle: application.LifecycleClosed, PlanStatus: avatar.Status(255), PlanSource: avatar.Source(255)}
	time.Sleep(20 * time.Millisecond)
	if revision := api.GetStatus().Revision; revision != afterCancel {
		t.Fatalf("canceled status consumer changed revision: %d -> %d", afterCancel, revision)
	}
}

func TestRuntimeAPIPlanEnumConversionUsesStableStrings(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     avatar.Status
		source     avatar.Source
		wantStatus string
		wantSource string
	}{
		{name: "absent", wantStatus: "none", wantSource: "none"},
		{name: "failed avatar config", status: avatar.StatusFailed, source: avatar.SourceAvatarConfig, wantStatus: "failed", wantSource: "avatar_config"},
		{name: "ready fallback", status: avatar.StatusReady, source: avatar.SourceFallback, wantStatus: "ready", wantSource: "fallback"},
		{name: "unknown", status: avatar.Status(255), source: avatar.Source(255), wantStatus: "unknown", wantSource: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := runtimeApplicationDTO(application.Status{PlanStatus: test.status, PlanSource: test.source})
			if got.PlanStatus != test.wantStatus || got.PlanSource != test.wantSource {
				t.Fatalf("plan enum conversion = %q, %q; want %q, %q", got.PlanStatus, got.PlanSource, test.wantStatus, test.wantSource)
			}
		})
	}
}

func TestRuntimeAPIProblemAndSubscriptionSuppressSemanticDuplicates(t *testing.T) {
	api := newRuntimeAPI(true, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	updates := api.subscribe(ctx)
	initial := receiveRuntimeResponse(t, updates)
	if cap(updates) != 1 || initial.Phase != "created" {
		t.Fatalf("initial runtime subscription = %+v, capacity=%d", initial, cap(updates))
	}

	problem := &Problem{Code: ProblemInternal, Message: "internal operation failed"}
	api.setProblem(problem)
	changed := receiveRuntimeResponse(t, updates)
	api.setProblem(&Problem{Code: ProblemInternal, Message: "internal operation failed"})
	select {
	case duplicate := <-updates:
		t.Fatalf("semantic duplicate published runtime response %+v", duplicate)
	case <-time.After(20 * time.Millisecond):
	}
	if got := api.GetStatus(); got.Revision != changed.Revision {
		t.Fatalf("semantic duplicate changed revision: %d -> %d", changed.Revision, got.Revision)
	}

	api.setProblem(nil)
	cleared := receiveRuntimeResponse(t, updates)
	if cleared.Problem != nil || cleared.Revision != changed.Revision+1 {
		t.Fatalf("cleared runtime problem = %+v", cleared)
	}
	cancel()
	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("runtime subscription remained open after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("runtime subscription did not close after cancellation")
	}
}

func waitRuntimeRevision(t *testing.T, api *RuntimeAPI, want uint64) RuntimeResponse {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		got := api.GetStatus()
		if got.Revision >= want {
			return got
		}
		select {
		case <-deadline:
			t.Fatalf("RuntimeAPI revision = %d, want at least %d", got.Revision, want)
		case <-time.After(time.Millisecond):
		}
	}
}

func receiveRuntimeResponse(t *testing.T, updates <-chan RuntimeResponse) RuntimeResponse {
	t.Helper()
	select {
	case response, ok := <-updates:
		if !ok {
			t.Fatal("runtime response stream closed unexpectedly")
		}
		return response
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runtime response")
		return RuntimeResponse{}
	}
}
