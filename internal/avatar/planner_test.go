package avatar

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestPlannerCompilesAndReplacesReadyPlan(t *testing.T) {
	const avatarID = "avtr_planner"
	root := filepath.Join(t.TempDir(), "OSC")
	path := plannerAvatarPath(root, avatarID)
	writePlannerConfig(t, path, plannerConfigJSON(avatarID,
		plannerInput("/avatar/parameters/Face/v2/JawOpen", "Float"),
		plannerInput("/avatar/parameters/Face/v2/EyeX", "Float"),
		plannerInput("/avatar/parameters/Face/LipTrackingActive", "Bool"),
	))

	planner, err := NewPlanner(PlannerConfig{OSCRoot: root})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	first := planner.Activate(avatarID)
	if first.Err != nil {
		t.Fatalf("Activate() error = %v", first.Err)
	}
	wantIDs := []parameters.ParameterID{
		parameters.ParameterJawOpen,
		parameters.ParameterEyeX,
		parameters.ParameterLipTrackingActive,
	}
	assertReadyPlannerResult(t, first, 1, avatarID, avatarID, path, SourceAvatarConfig, wantIDs)
	if got := first.Plan.RequiredInputs(); !reflect.DeepEqual(got, (parameterdeps.Inputs{
		Eye:         parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftGazeX, parameterdeps.EyeFieldRightGazeX),
		Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
		Active:      parameterdeps.ActiveStatesOf(parameterdeps.ActiveStateLipTracking),
	})) {
		t.Fatalf("RequiredInputs() = %#v, want exact Eye/Expression/Lip leaves", got)
	}
	wantSubscription := pluginapi.Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip,
		Eye:          trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
	if got, ok := first.Plan.SubscriptionFor(wantSubscription.Capabilities); !ok || got != wantSubscription {
		t.Fatalf("SubscriptionFor(all) = %#v, %t; want %#v, true", got, ok, wantSubscription)
	} else if err := got.Validate(true); err != nil {
		t.Fatalf("SubscriptionFor(all) validation error = %v", err)
	}

	writePlannerConfig(t, path, plannerConfigJSON(avatarID,
		plannerInput("/avatar/parameters/Face/v2/MouthClosed", "Float"),
	))
	second := planner.Activate(avatarID)
	if second.Err != nil {
		t.Fatalf("second Activate() error = %v", second.Err)
	}
	assertReadyPlannerResult(t, second, 2, avatarID, avatarID, path, SourceAvatarConfig, []parameters.ParameterID{parameters.ParameterMouthClosed})
	for _, oldID := range wantIDs {
		if _, ok := second.Plan.Catalog().Bindings[oldID]; ok {
			t.Errorf("generation 2 catalog retained generation 1 ID %d", oldID)
		}
	}
	if got := second.Plan.RequiredInputs(); got.Eye != 0 || got.Active != 0 || got.Expressions.Has(trackingmodel.ExpressionJawOpen) || !got.Expressions.Has(trackingmodel.ExpressionMouthClosed) {
		t.Fatalf("generation 2 requirements retained generation 1 state: %#v", got)
	}
}

func TestPlannerFailClosedTransitions(t *testing.T) {
	const avatarID = "avtr_transitions"
	root := filepath.Join(t.TempDir(), "OSC")
	currentPath := plannerAvatarPath(root, avatarID)
	fallbackPath := filepath.Join(t.TempDir(), "fallback.json")
	writePlannerConfig(t, fallbackPath, plannerConfigJSON("fallback_config",
		plannerInput("/avatar/parameters/v2/EyeLeftX", "Float"),
	))
	writePlannerConfig(t, currentPath, plannerConfigJSON(avatarID,
		plannerInput("/avatar/parameters/v2/JawOpen", "Float"),
	))

	planner, err := NewPlanner(PlannerConfig{OSCRoot: root, FallbackPath: fallbackPath})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}

	ready := planner.Activate(avatarID)
	if ready.Err != nil {
		t.Fatalf("generation 1 Activate() error = %v", ready.Err)
	}
	assertReadyPlannerResult(t, ready, 1, avatarID, avatarID, currentPath, SourceAvatarConfig, []parameters.ParameterID{parameters.ParameterJawOpen})

	writePlannerConfig(t, currentPath, `{"id":`)
	malformed := planner.Activate(avatarID)
	assertFailedPlannerResult(t, malformed, 2, avatarID, SourceAvatarConfig, currentPath, "", ErrInvalidJSON)

	if err := os.Remove(currentPath); err != nil {
		t.Fatalf("Remove(%q): %v", currentPath, err)
	}
	fallback := planner.Activate(avatarID)
	if fallback.Err != nil {
		t.Fatalf("generation 3 Activate() error = %v", fallback.Err)
	}
	assertReadyPlannerResult(t, fallback, 3, avatarID, "fallback_config", fallbackPath, SourceFallback, []parameters.ParameterID{parameters.ParameterEyeLeftX})

	writePlannerConfig(t, currentPath, plannerConfigJSON("different_config",
		plannerInput("/avatar/parameters/v2/JawOpen", "Float"),
	))
	mismatch := planner.Activate(avatarID)
	assertFailedPlannerResult(t, mismatch, 4, avatarID, SourceAvatarConfig, currentPath, "different_config", ErrConfigIDMismatch)

	writePlannerConfig(t, currentPath, plannerConfigJSON(avatarID,
		plannerInput("/avatar/parameters/v2/JawOpen", "Float"),
		plannerInput("/avatar/parameters/v2/JawOpen", "Int"),
	))
	conflict := planner.Activate(avatarID)
	assertFailedPlannerResult(t, conflict, 5, avatarID, SourceAvatarConfig, currentPath, avatarID, ErrBindingCompilation)
	if !errors.Is(conflict.Err, osc.ErrConflictingOSCAddress) {
		t.Fatalf("generation 5 error = %v, want retained osc.ErrConflictingOSCAddress", conflict.Err)
	}
}

func TestPlannerGenerationExhaustionNeverWraps(t *testing.T) {
	const avatarID = "avtr_exhaustion"
	root := filepath.Join(t.TempDir(), "OSC")
	writePlannerConfig(t, plannerAvatarPath(root, avatarID), plannerConfigJSON(avatarID))
	planner, err := NewPlanner(PlannerConfig{OSCRoot: root})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	planner.generation = math.MaxUint64 - 1

	last := planner.Activate(avatarID)
	if last.Err != nil {
		t.Fatalf("Activate() at final generation error = %v", last.Err)
	}
	if last.Plan == nil || last.Plan.Generation() != math.MaxUint64 {
		t.Fatalf("final plan = %#v, want generation %d", last.Plan, uint64(math.MaxUint64))
	}

	for attempt := 0; attempt < 2; attempt++ {
		exhausted := planner.Activate(avatarID)
		if exhausted.Plan != nil || !errors.Is(exhausted.Err, ErrGenerationExhausted) {
			t.Fatalf("exhausted Activate() = %#v, want nil plan and ErrGenerationExhausted", exhausted)
		}
		if planner.generation != math.MaxUint64 {
			t.Fatalf("generation wrapped to %d after exhaustion", planner.generation)
		}
	}
}

func TestPlannerConcurrentActivateAllocatesDistinctGenerations(t *testing.T) {
	const (
		avatarID = "avtr_concurrent"
		workers  = 32
	)
	root := filepath.Join(t.TempDir(), "OSC")
	writePlannerConfig(t, plannerAvatarPath(root, avatarID), plannerConfigJSON(avatarID,
		plannerInput("/avatar/parameters/v2/JawOpen", "Float"),
	))
	planner, err := NewPlanner(PlannerConfig{OSCRoot: root})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}

	results := make(chan Result, workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			results <- planner.Activate(avatarID)
		}()
	}

	generations := make([]uint64, 0, workers)
	for worker := 0; worker < workers; worker++ {
		result := <-results
		if result.Err != nil || result.Plan == nil || result.Plan.Status() != StatusReady {
			t.Errorf("concurrent Activate() = %#v, want ready result", result)
			continue
		}
		generations = append(generations, result.Plan.Generation())
	}
	if len(generations) != workers {
		t.Fatalf("ready generation count = %d, want %d", len(generations), workers)
	}
	sort.Slice(generations, func(i, j int) bool { return generations[i] < generations[j] })
	for index, generation := range generations {
		if want := uint64(index + 1); generation != want {
			t.Fatalf("sorted generation[%d] = %d, want %d; all = %v", index, generation, want, generations)
		}
	}
}

func TestPlannerConstructorValidatesAndNormalizesPaths(t *testing.T) {
	if planner, err := NewPlanner(PlannerConfig{}); planner != nil || !errors.Is(err, ErrInvalidPlannerConfig) {
		t.Fatalf("NewPlanner(empty) = %#v, %v; want nil and ErrInvalidPlannerConfig", planner, err)
	}

	base := t.TempDir()
	root := filepath.Join(base, "root", "..", "OSC")
	fallback := filepath.Join(base, "fallback-dir", "..", "fallback.json")
	planner, err := NewPlanner(PlannerConfig{OSCRoot: root, FallbackPath: fallback})
	if err != nil {
		t.Fatalf("NewPlanner(paths) error = %v", err)
	}
	if planner.oscRoot != mustAbsoluteCleanPath(t, root) || planner.fallbackPath != mustAbsoluteCleanPath(t, fallback) {
		t.Fatalf("normalized paths = %q, %q", planner.oscRoot, planner.fallbackPath)
	}
	if planner.specs == nil {
		t.Fatal("NewPlanner() did not compile parameter catalog")
	}
}

func assertReadyPlannerResult(t *testing.T, result Result, generation uint64, avatarID, configID, path string, source Source, ids []parameters.ParameterID) {
	t.Helper()
	if result.Plan == nil {
		t.Fatal("ready Result.Plan is nil")
	}
	plan := result.Plan
	if plan.Generation() != generation || plan.Status() != StatusReady || plan.AvatarID() != avatarID || plan.ConfigID() != configID || plan.ConfigPath() != mustAbsoluteCleanPath(t, path) || plan.Source() != source {
		t.Fatalf("ready plan diagnostics = generation %d, status %d, avatar %q, config %q, path %q, source %d", plan.Generation(), plan.Status(), plan.AvatarID(), plan.ConfigID(), plan.ConfigPath(), plan.Source())
	}
	if got := plan.ParameterIDs(); !reflect.DeepEqual(got, ids) {
		t.Fatalf("ParameterIDs() = %v, want numeric order %v", got, ids)
	}
	catalog := plan.Catalog()
	if catalog == nil || catalog.Generation != generation {
		t.Fatalf("Catalog() = %#v, want generation %d", catalog, generation)
	}
	if plan.Evaluator() == nil {
		t.Fatal("Evaluator() is nil for ready plan")
	}
}

func assertFailedPlannerResult(t *testing.T, result Result, generation uint64, avatarID string, source Source, path, configID string, wantErr error) {
	t.Helper()
	if !errors.Is(result.Err, wantErr) {
		t.Fatalf("failed result error = %v, want errors.Is(_, %v)", result.Err, wantErr)
	}
	if result.Plan == nil {
		t.Fatal("ordinary failure returned nil plan")
	}
	plan := result.Plan
	if plan.Generation() != generation || plan.Status() != StatusFailed || plan.AvatarID() != avatarID || plan.Source() != source || plan.ConfigID() != configID {
		t.Fatalf("failed plan diagnostics = generation %d, status %d, avatar %q, source %d, config %q", plan.Generation(), plan.Status(), plan.AvatarID(), plan.Source(), plan.ConfigID())
	}
	if path == "" {
		if plan.ConfigPath() != "" {
			t.Fatalf("failed ConfigPath() = %q, want empty", plan.ConfigPath())
		}
	} else if plan.ConfigPath() != mustAbsoluteCleanPath(t, path) {
		t.Fatalf("failed ConfigPath() = %q, want %q", plan.ConfigPath(), mustAbsoluteCleanPath(t, path))
	}
	if plan.Catalog() != nil || plan.Evaluator() != nil || len(plan.ParameterIDs()) != 0 || !plan.RequiredInputs().IsZero() {
		t.Fatalf("failed plan retained operational state: %#v", plan)
	}
	if subscription, ok := plan.SubscriptionFor(trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip); ok || subscription != (pluginapi.Subscription{}) {
		t.Fatalf("failed SubscriptionFor(all) = %#v, %t; want zero, false", subscription, ok)
	}
}

func plannerAvatarPath(root, avatarID string) string {
	return filepath.Join(root, "usr_test", "Avatars", avatarID+".json")
}

func writePlannerConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func plannerConfigJSON(id string, inputs ...string) string {
	return fmt.Sprintf(`{"id":%q,"parameters":[%s]}`, id, joinPlannerInputs(inputs))
}

func plannerInput(address, typ string) string {
	return fmt.Sprintf(`{"input":{"address":%q,"type":%q}}`, address, typ)
}

func joinPlannerInputs(inputs []string) string {
	result := ""
	for index, input := range inputs {
		if index != 0 {
			result += ","
		}
		result += input
	}
	return result
}
