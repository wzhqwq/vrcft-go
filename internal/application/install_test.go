package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestPlanInstallerAvatarPlannerAdapterPreservesResult(t *testing.T) {
	planner, err := avatar.NewPlanner(avatar.PlannerConfig{
		OSCRoot: filepath.Join(t.TempDir(), "missing-osc-root"),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := newActivationPlanner(planner).Activate("avtr_adapter")

	if _, ok := got.plan.(*avatar.Plan); !ok {
		t.Fatalf("adapter plan type = %T, want *avatar.Plan", got.plan)
	}
	if got.plan.Generation() != 1 || got.plan.Status() != avatar.StatusFailed {
		t.Fatalf("adapter plan generation/status = (%d, %d), want (1, failed)", got.plan.Generation(), got.plan.Status())
	}
	if !errors.Is(got.err, avatar.ErrConfigNotFound) {
		t.Fatalf("adapter error = %v, want errors.Is(_, avatar.ErrConfigNotFound)", got.err)
	}
}

func TestPlanInstallerReadyPlanUsesFailClosedDeterministicOrder(t *testing.T) {
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	plan := readyInstallPlan(9)
	installer := newTestPlanInstaller(recorder, pluginControls)

	outcome := installer.install(context.Background(), activation{plan: plan})

	wantTrace := []string{
		"osc.clear",
		"tracking.generation:9",
		"plugin.active:vendor.expression:false",
		"plugin.active:vendor.eye:false",
		"plugin.active:vendor.none:false",
		"plugin.subscription:vendor.expression:9",
		"plugin.active:vendor.expression:true",
		"plugin.subscription:vendor.eye:9",
		"plugin.active:vendor.eye:true",
		"osc.install:9",
	}
	assertInstallTrace(t, recorder, wantTrace)
	if outcome.plan != plan {
		t.Fatal("install outcome did not retain the activated plan")
	}
	if outcome.planErr != nil || outcome.runtimeErr != nil || outcome.exhausted {
		t.Fatalf("install outcome errors/exhausted = (%v, %v, %t), want nil, nil, false", outcome.planErr, outcome.runtimeErr, outcome.exhausted)
	}
	if !outcome.catalogReady {
		t.Fatal("install outcome catalogReady = false, want true")
	}
	if len(outcome.pluginFailures) != 0 {
		t.Fatalf("install outcome plugin failures = %#v, want none", outcome.pluginFailures)
	}
	if pluginControls.preferenceCalls != 0 {
		t.Fatalf("persisted Enabled preference calls = %d, want zero", pluginControls.preferenceCalls)
	}
	pluginControls.assertIndependentBoundedContexts(t, DefaultPluginControlTimeout)
}

func TestPlanInstallerFailedAndEmptyPlansStayClosed(t *testing.T) {
	planFailure := errors.New("avatar configuration failed")
	tests := []struct {
		name       string
		activation activation
		generation uint64
		planErr    error
	}{
		{
			name: "failed non-nil plan",
			activation: activation{
				plan: &fakeInstallPlan{generation: 10, status: avatar.StatusFailed},
				err:  planFailure,
			},
			generation: 10,
			planErr:    planFailure,
		},
		{
			name: "ready empty plan",
			activation: activation{plan: &fakeInstallPlan{
				generation: 11,
				status:     avatar.StatusReady,
				catalog:    &osc.Catalog{Generation: 11, Bindings: map[parameters.ParameterID]osc.ParameterBinding{}},
			}},
			generation: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &installRecorder{}
			pluginControls := newFakePlanPluginControls(recorder)
			installer := newTestPlanInstaller(recorder, pluginControls)

			outcome := installer.install(context.Background(), tt.activation)

			assertInstallTrace(t, recorder, []string{
				"osc.clear",
				fmt.Sprintf("tracking.generation:%d", tt.generation),
				"plugin.active:vendor.expression:false",
				"plugin.active:vendor.eye:false",
				"plugin.active:vendor.none:false",
			})
			if !errors.Is(outcome.planErr, tt.planErr) {
				t.Fatalf("install outcome planErr = %v, want errors.Is(_, %v)", outcome.planErr, tt.planErr)
			}
			if outcome.runtimeErr != nil || outcome.exhausted || outcome.catalogReady {
				t.Fatalf("install outcome runtime/exhausted/catalog = (%v, %t, %t), want nil, false, false", outcome.runtimeErr, outcome.exhausted, outcome.catalogReady)
			}
			if len(outcome.pluginFailures) != 0 {
				t.Fatalf("install outcome plugin failures = %#v, want none", outcome.pluginFailures)
			}
			pluginControls.assertIndependentBoundedContexts(t, DefaultPluginControlTimeout)
		})
	}
}

func TestPlanInstallerPluginFailureLeavesThatPluginInactiveAndContinues(t *testing.T) {
	tests := []struct {
		name           string
		configure      func(*fakePlanPluginControls)
		wantFailure    PluginControlFailure
		missingCall    string
		wantTrace      []string
		controlTimeout time.Duration
	}{
		{
			name: "subscription failure",
			configure: func(controls *fakePlanPluginControls) {
				controls.subscriptionErrors["vendor.eye"] = errors.New("subscription rejected")
			},
			wantFailure: PluginControlFailure{PluginID: "vendor.eye", Operation: "subscription", Message: "subscription rejected"},
			missingCall: "plugin.active:vendor.eye:true",
			wantTrace: []string{
				"osc.clear",
				"tracking.generation:9",
				"plugin.active:vendor.expression:false",
				"plugin.active:vendor.eye:false",
				"plugin.active:vendor.none:false",
				"plugin.subscription:vendor.expression:9",
				"plugin.active:vendor.expression:true",
				"plugin.subscription:vendor.eye:9",
				"osc.install:9",
			},
		},
		{
			name: "activation failure",
			configure: func(controls *fakePlanPluginControls) {
				controls.activeErrors[activeControlKey("vendor.eye", true)] = errors.New("activation rejected")
			},
			wantFailure: PluginControlFailure{PluginID: "vendor.eye", Operation: "activate", Message: "activation rejected"},
			wantTrace: []string{
				"osc.clear",
				"tracking.generation:9",
				"plugin.active:vendor.expression:false",
				"plugin.active:vendor.eye:false",
				"plugin.active:vendor.none:false",
				"plugin.subscription:vendor.expression:9",
				"plugin.active:vendor.expression:true",
				"plugin.subscription:vendor.eye:9",
				"plugin.active:vendor.eye:true",
				"plugin.active:vendor.eye:false",
				"osc.install:9",
			},
		},
		{
			name: "deactivation timeout",
			configure: func(controls *fakePlanPluginControls) {
				controls.blockActive[activeControlKey("vendor.eye", false)] = true
			},
			wantFailure:    PluginControlFailure{PluginID: "vendor.eye", Operation: "deactivate", Message: context.DeadlineExceeded.Error()},
			missingCall:    "plugin.subscription:vendor.eye:9",
			controlTimeout: 2 * time.Millisecond,
			wantTrace: []string{
				"osc.clear",
				"tracking.generation:9",
				"plugin.active:vendor.expression:false",
				"plugin.active:vendor.eye:false",
				"plugin.active:vendor.none:false",
				"plugin.subscription:vendor.expression:9",
				"plugin.active:vendor.expression:true",
				"osc.install:9",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &installRecorder{}
			pluginControls := newFakePlanPluginControls(recorder)
			tt.configure(pluginControls)
			installer := newTestPlanInstaller(recorder, pluginControls)
			if tt.controlTimeout != 0 {
				installer.pluginControlTimeout = tt.controlTimeout
			}

			outcome := installer.install(context.Background(), activation{plan: readyInstallPlan(9)})

			assertInstallTrace(t, recorder, tt.wantTrace)
			if tt.missingCall != "" && recorder.contains(tt.missingCall) {
				t.Fatalf("trace contains unsafe call %q: %v", tt.missingCall, recorder.snapshot())
			}
			if !reflect.DeepEqual(outcome.pluginFailures, []PluginControlFailure{tt.wantFailure}) {
				t.Fatalf("install outcome plugin failures = %#v, want %#v", outcome.pluginFailures, []PluginControlFailure{tt.wantFailure})
			}
			if outcome.runtimeErr != nil || !outcome.catalogReady {
				t.Fatalf("install outcome runtimeErr/catalogReady = (%v, %t), want nil, true", outcome.runtimeErr, outcome.catalogReady)
			}
			wantTimeout := DefaultPluginControlTimeout
			if tt.controlTimeout != 0 {
				wantTimeout = tt.controlTimeout
			}
			pluginControls.assertIndependentBoundedContexts(t, wantTimeout)
		})
	}
}

func TestPlanInstallerActivationTimeoutCompensatesBeforeContinuing(t *testing.T) {
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	pluginControls.blockActive[activeControlKey("vendor.expression", true)] = true
	pluginControls.commitActiveOnError["vendor.expression"] = true
	installer := newTestPlanInstaller(recorder, pluginControls)
	installer.pluginControlTimeout = 2 * time.Millisecond

	outcome := installer.install(context.Background(), activation{plan: readyInstallPlan(9)})

	assertInstallTrace(t, recorder, []string{
		"osc.clear",
		"tracking.generation:9",
		"plugin.active:vendor.expression:false",
		"plugin.active:vendor.eye:false",
		"plugin.active:vendor.none:false",
		"plugin.subscription:vendor.expression:9",
		"plugin.active:vendor.expression:true",
		"plugin.active:vendor.expression:false",
		"plugin.subscription:vendor.eye:9",
		"plugin.active:vendor.eye:true",
		"osc.install:9",
	})
	if pluginControls.activeState["vendor.expression"] {
		t.Fatal("vendor.expression remained active after successful compensating deactivation")
	}
	if !pluginControls.activeState["vendor.eye"] {
		t.Fatal("later vendor.eye plugin was not activated")
	}
	wantFailures := []PluginControlFailure{{
		PluginID:  "vendor.expression",
		Operation: "activate",
		Message:   context.DeadlineExceeded.Error(),
	}}
	if !reflect.DeepEqual(outcome.pluginFailures, wantFailures) {
		t.Fatalf("install outcome plugin failures = %#v, want %#v", outcome.pluginFailures, wantFailures)
	}
	if !outcome.catalogReady {
		t.Fatal("catalog was not installed after compensating and continuing")
	}
	pluginControls.assertIndependentBoundedContexts(t, installer.pluginControlTimeout)
}

func TestPlanInstallerFailedActivationCompensationIsRecordedAndLaterPluginsContinue(t *testing.T) {
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	pluginControls.activeErrors[activeControlKey("vendor.expression", true)] = errors.New("activation result lost")
	pluginControls.commitActiveOnError["vendor.expression"] = true
	pluginControls.compensationErrors["vendor.expression"] = errors.New("compensation rejected")
	installer := newTestPlanInstaller(recorder, pluginControls)

	outcome := installer.install(context.Background(), activation{plan: readyInstallPlan(9)})

	assertInstallTrace(t, recorder, []string{
		"osc.clear",
		"tracking.generation:9",
		"plugin.active:vendor.expression:false",
		"plugin.active:vendor.eye:false",
		"plugin.active:vendor.none:false",
		"plugin.subscription:vendor.expression:9",
		"plugin.active:vendor.expression:true",
		"plugin.active:vendor.expression:false",
		"plugin.subscription:vendor.eye:9",
		"plugin.active:vendor.eye:true",
		"osc.install:9",
	})
	wantFailures := []PluginControlFailure{
		{PluginID: "vendor.expression", Operation: "activate", Message: "activation result lost"},
		{PluginID: "vendor.expression", Operation: "deactivate", Message: "compensation rejected"},
	}
	if !reflect.DeepEqual(outcome.pluginFailures, wantFailures) {
		t.Fatalf("install outcome plugin failures = %#v, want %#v", outcome.pluginFailures, wantFailures)
	}
	if !pluginControls.activeState["vendor.expression"] {
		t.Fatal("fake did not preserve uncertain active state after failed compensation")
	}
	if !pluginControls.activeState["vendor.eye"] || !outcome.catalogReady {
		t.Fatal("later plugin controls or catalog installation did not continue")
	}
	pluginControls.assertIndependentBoundedContexts(t, installer.pluginControlTimeout)
}

func TestPlanInstallerPluginFailureMessageIsSingleLineBoundedUTF8(t *testing.T) {
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	pluginControls.subscriptionErrors["vendor.eye"] = errors.New(
		"  first\r\n second\t" + string([]byte{0xff}) + "  " + strings.Repeat("x", 600),
	)
	installer := newTestPlanInstaller(recorder, pluginControls)

	outcome := installer.install(context.Background(), activation{plan: readyInstallPlan(9)})

	if len(outcome.pluginFailures) != 1 {
		t.Fatalf("plugin failures = %#v, want one", outcome.pluginFailures)
	}
	message := outcome.pluginFailures[0].Message
	want := "first second � " + strings.Repeat("x", 495)
	if message != want {
		t.Fatalf("sanitized message = %q, want %q", message, want)
	}
	if len(message) != 512 {
		t.Fatalf("sanitized message byte length = %d, want 512", len(message))
	}
	if !utf8.ValidString(message) {
		t.Fatalf("sanitized message is not valid UTF-8: %q", message)
	}
	if strings.ContainsAny(message, "\r\n\t") {
		t.Fatalf("sanitized message contains line/control whitespace: %q", message)
	}
}

func TestPlanInstallerGenerationFailureDeactivatesAllAndDoesNotExposeCatalog(t *testing.T) {
	generationFailure := errors.New("generation advance failed")
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	trackingControls := &fakeGenerationControl{recorder: recorder, err: generationFailure}
	installer := newTestPlanInstaller(recorder, pluginControls)
	installer.tracking = trackingControls

	outcome := installer.install(context.Background(), activation{plan: readyInstallPlan(12)})

	assertInstallTrace(t, recorder, []string{
		"osc.clear",
		"tracking.generation:12",
		"plugin.active:vendor.expression:false",
		"plugin.active:vendor.eye:false",
		"plugin.active:vendor.none:false",
	})
	if !errors.Is(outcome.runtimeErr, generationFailure) {
		t.Fatalf("install outcome runtimeErr = %v, want errors.Is(_, %v)", outcome.runtimeErr, generationFailure)
	}
	if outcome.catalogReady {
		t.Fatal("install outcome catalogReady = true after generation failure")
	}
	pluginControls.assertIndependentBoundedContexts(t, DefaultPluginControlTimeout)
}

func TestPlanInstallerNilPlanClearsDeactivatesAndMarksExhausted(t *testing.T) {
	exhausted := errors.New("generation exhausted")
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	installer := newTestPlanInstaller(recorder, pluginControls)

	outcome := installer.install(context.Background(), activation{err: exhausted})

	assertInstallTrace(t, recorder, []string{
		"osc.clear",
		"plugin.active:vendor.expression:false",
		"plugin.active:vendor.eye:false",
		"plugin.active:vendor.none:false",
	})
	if outcome.plan != nil || !errors.Is(outcome.planErr, exhausted) {
		t.Fatalf("install outcome plan/planErr = (%v, %v), want nil and errors.Is(_, %v)", outcome.plan, outcome.planErr, exhausted)
	}
	if !outcome.exhausted || outcome.catalogReady || outcome.runtimeErr != nil {
		t.Fatalf("install outcome exhausted/catalog/runtime = (%t, %t, %v), want true, false, nil", outcome.exhausted, outcome.catalogReady, outcome.runtimeErr)
	}
	pluginControls.assertIndependentBoundedContexts(t, DefaultPluginControlTimeout)
}

func TestPlanInstallerCatalogInstallFailureRemainsClosed(t *testing.T) {
	installFailure := errors.New("catalog rejected")
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	installer := newTestPlanInstaller(recorder, pluginControls)
	installer.osc = &fakeCatalogControl{recorder: recorder, installErr: installFailure}

	outcome := installer.install(context.Background(), activation{plan: readyInstallPlan(13)})

	if !errors.Is(outcome.runtimeErr, installFailure) {
		t.Fatalf("install outcome runtimeErr = %v, want errors.Is(_, %v)", outcome.runtimeErr, installFailure)
	}
	if outcome.catalogReady {
		t.Fatal("install outcome catalogReady = true after catalog install failure")
	}
	if got := recorder.snapshot()[len(recorder.snapshot())-1]; got != "osc.install:13" {
		t.Fatalf("last call = %q, want osc.install:13", got)
	}
}

func TestPlanInstallerPreservesPlanErrorAlongsideRuntimeFailure(t *testing.T) {
	planFailure := errors.New("planning failed")
	generationFailure := errors.New("generation failed")
	recorder := &installRecorder{}
	pluginControls := newFakePlanPluginControls(recorder)
	installer := newTestPlanInstaller(recorder, pluginControls)
	installer.tracking = &fakeGenerationControl{recorder: recorder, err: generationFailure}

	outcome := installer.install(context.Background(), activation{
		plan: &fakeInstallPlan{generation: 14, status: avatar.StatusFailed},
		err:  planFailure,
	})

	if !errors.Is(outcome.planErr, planFailure) {
		t.Fatalf("install outcome planErr = %v, want errors.Is(_, %v)", outcome.planErr, planFailure)
	}
	if !errors.Is(outcome.runtimeErr, generationFailure) {
		t.Fatalf("install outcome runtimeErr = %v, want errors.Is(_, %v)", outcome.runtimeErr, generationFailure)
	}
}

func newTestPlanInstaller(recorder *installRecorder, pluginControls *fakePlanPluginControls) *planInstaller {
	return &planInstaller{
		plugins:              pluginControls,
		tracking:             &fakeGenerationControl{recorder: recorder},
		osc:                  &fakeCatalogControl{recorder: recorder},
		pluginControlTimeout: DefaultPluginControlTimeout,
	}
}

func readyInstallPlan(generation uint64) *fakeInstallPlan {
	return &fakeInstallPlan{
		generation: generation,
		status:     avatar.StatusReady,
		catalog: &osc.Catalog{
			Generation: generation,
			Bindings: map[parameters.ParameterID]osc.ParameterBinding{
				0: {},
				1: {},
			},
		},
		evaluator: &evaluator.Plan{},
		subscriptions: map[trackingmodel.Capability]pluginapi.Subscription{
			trackingmodel.CapabilityEye: {
				Generation:   generation,
				Capabilities: trackingmodel.CapabilityEye,
			},
			trackingmodel.CapabilityExpression: {
				Generation:   generation,
				Capabilities: trackingmodel.CapabilityExpression,
			},
		},
	}
}

type fakeInstallPlan struct {
	generation    uint64
	status        avatar.Status
	avatarID      string
	configID      string
	configPath    string
	source        avatar.Source
	parameterIDs  []parameters.ParameterID
	catalog       *osc.Catalog
	evaluator     *evaluator.Plan
	subscriptions map[trackingmodel.Capability]pluginapi.Subscription
}

func (p *fakeInstallPlan) Generation() uint64         { return p.generation }
func (p *fakeInstallPlan) Status() avatar.Status      { return p.status }
func (p *fakeInstallPlan) AvatarID() string           { return p.avatarID }
func (p *fakeInstallPlan) ConfigID() string           { return p.configID }
func (p *fakeInstallPlan) ConfigPath() string         { return p.configPath }
func (p *fakeInstallPlan) Source() avatar.Source      { return p.source }
func (p *fakeInstallPlan) Evaluator() *evaluator.Plan { return p.evaluator }
func (p *fakeInstallPlan) ParameterIDs() []parameters.ParameterID {
	return append([]parameters.ParameterID(nil), p.parameterIDs...)
}
func (p *fakeInstallPlan) Catalog() *osc.Catalog { return p.catalog.Clone() }
func (p *fakeInstallPlan) SubscriptionFor(capability trackingmodel.Capability) (pluginapi.Subscription, bool) {
	for advertised, subscription := range p.subscriptions {
		if capability.Has(advertised) {
			return subscription, true
		}
	}
	return pluginapi.Subscription{}, false
}

type installRecorder struct {
	mu    sync.Mutex
	trace []string
}

func (r *installRecorder) add(call string) {
	r.mu.Lock()
	r.trace = append(r.trace, call)
	r.mu.Unlock()
}

func (r *installRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.trace...)
}

func (r *installRecorder) contains(call string) bool {
	for _, current := range r.snapshot() {
		if current == call {
			return true
		}
	}
	return false
}

type fakeGenerationControl struct {
	recorder *installRecorder
	err      error
}

func (f *fakeGenerationControl) SetGeneration(generation uint64) error {
	f.recorder.add(fmt.Sprintf("tracking.generation:%d", generation))
	return f.err
}

type fakeCatalogControl struct {
	recorder   *installRecorder
	installErr error
}

func (f *fakeCatalogControl) ClearRuntime() { f.recorder.add("osc.clear") }
func (f *fakeCatalogControl) InstallCatalog(catalog *osc.Catalog) error {
	f.recorder.add(fmt.Sprintf("osc.install:%d", catalog.Generation))
	return f.installErr
}

type fakePlanPluginControls struct {
	recorder            *installRecorder
	snapshots           []plugins.RuntimeSnapshot
	activeErrors        map[string]error
	subscriptionErrors  map[string]error
	blockActive         map[string]bool
	commitActiveOnError map[string]bool
	compensationErrors  map[string]error
	activeState         map[string]bool
	preferenceCalls     int
	contexts            []context.Context
}

func newFakePlanPluginControls(recorder *installRecorder) *fakePlanPluginControls {
	return &fakePlanPluginControls{
		recorder: recorder,
		snapshots: []plugins.RuntimeSnapshot{
			{ID: "vendor.none", Capabilities: trackingmodel.CapabilityLip},
			{ID: "vendor.expression", Capabilities: trackingmodel.CapabilityExpression},
			{ID: "vendor.eye", Capabilities: trackingmodel.CapabilityEye},
		},
		activeErrors:        make(map[string]error),
		subscriptionErrors:  make(map[string]error),
		blockActive:         make(map[string]bool),
		commitActiveOnError: make(map[string]bool),
		compensationErrors:  make(map[string]error),
		activeState:         make(map[string]bool),
	}
}

func (f *fakePlanPluginControls) List() []plugins.RuntimeSnapshot {
	return append([]plugins.RuntimeSnapshot(nil), f.snapshots...)
}

func (f *fakePlanPluginControls) SetActive(ctx context.Context, id string, active bool) error {
	f.captureContext(ctx)
	f.recorder.add(fmt.Sprintf("plugin.active:%s:%t", id, active))
	key := activeControlKey(id, active)
	var err error
	if f.blockActive[key] {
		<-ctx.Done()
		err = ctx.Err()
	} else {
		err = f.activeErrors[key]
	}
	if err != nil {
		if active && f.commitActiveOnError[id] {
			f.activeState[id] = true
		}
		return err
	}
	if !active && f.activeState[id] {
		if err := f.compensationErrors[id]; err != nil {
			return err
		}
	}
	f.activeState[id] = active
	return nil
}

func (f *fakePlanPluginControls) UpdateSubscription(ctx context.Context, id string, subscription pluginapi.Subscription) error {
	f.captureContext(ctx)
	f.recorder.add(fmt.Sprintf("plugin.subscription:%s:%d", id, subscription.Generation))
	return f.subscriptionErrors[id]
}

func (f *fakePlanPluginControls) Enable(context.Context, string) error {
	f.preferenceCalls++
	return nil
}

func (f *fakePlanPluginControls) Disable(context.Context, string) error {
	f.preferenceCalls++
	return nil
}

func (f *fakePlanPluginControls) captureContext(ctx context.Context) {
	f.contexts = append(f.contexts, ctx)
}

func (f *fakePlanPluginControls) assertIndependentBoundedContexts(t *testing.T, timeout time.Duration) {
	t.Helper()
	if len(f.contexts) == 0 {
		t.Fatal("plugin controls received no contexts")
	}
	for index, ctx := range f.contexts {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("plugin control context %d has no deadline", index)
		}
		if index > 0 && ctx == f.contexts[index-1] {
			t.Fatalf("plugin control contexts %d and %d are the same context", index-1, index)
		}
		if ctx.Err() == nil {
			t.Fatalf("plugin control context %d was not canceled when its call returned (timeout %v)", index, timeout)
		}
	}
}

func activeControlKey(id string, active bool) string {
	return fmt.Sprintf("%s:%t", id, active)
}

func assertInstallTrace(t *testing.T, recorder *installRecorder, want []string) {
	t.Helper()
	if got := recorder.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("install trace mismatch\n got: %v\nwant: %v", got, want)
	}
}
