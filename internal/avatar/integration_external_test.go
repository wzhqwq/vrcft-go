package avatar_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestAvatarPlanDrivesRequestedEvaluatorOutputs(t *testing.T) {
	const avatarID = "avtr_demo"
	root := filepath.Join(t.TempDir(), "OSC")
	configPath := filepath.Join(root, "usr_test", "Avatars", avatarID+".json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"id": "avtr_demo",
		"parameters": [
			{"name": "Face/v2/JawOpen", "input": {"address": "/avatar/parameters/Face/v2/JawOpen", "type": "Float"}},
			{"name": "ExpressionTrackingActive", "input": {"address": "/avatar/parameters/ExpressionTrackingActive", "type": "Bool"}}
		]
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	planner, err := avatar.NewPlanner(avatar.PlannerConfig{OSCRoot: root})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	result := planner.Activate(avatarID)
	if result.Err != nil {
		t.Fatalf("Activate() error = %v", result.Err)
	}
	if result.Plan == nil {
		t.Fatal("Activate() returned nil plan")
	}
	if got := result.Plan.Status(); got != avatar.StatusReady {
		t.Fatalf("Status() = %v, want StatusReady", got)
	}
	if got := result.Plan.Generation(); got != 1 {
		t.Fatalf("Generation() = %d, want 1", got)
	}

	var frame processing.CanonicalFrame
	frame.Generation = 1
	frame.ExpressionActive = true
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.75)
	snapshot := result.Plan.Evaluator().Evaluate(frame)
	var source osc.ValueSource = snapshot
	if value, ok := source.Float(parameters.ParameterJawOpen); !ok || value != 0.75 {
		t.Fatalf("JawOpen = %v,%t", value, ok)
	}
	if value, ok := source.Bool(parameters.ParameterExpressionTrackingActive); !ok || !value {
		t.Fatalf("ExpressionTrackingActive = %v,%t", value, ok)
	}
	if _, ok := source.Float(parameters.ParameterJawX); ok {
		t.Fatal("unbound JawX became externally visible")
	}

	wantIDs := []parameters.ParameterID{
		parameters.ParameterJawOpen,
		parameters.ParameterExpressionTrackingActive,
	}
	wantEndpoints := map[parameters.ParameterID]osc.Endpoint{
		parameters.ParameterJawOpen: {
			Address: "/avatar/parameters/Face/v2/JawOpen",
			Type:    "f",
		},
		parameters.ParameterExpressionTrackingActive: {
			Address: "/avatar/parameters/ExpressionTrackingActive",
			Type:    "T",
		},
	}
	assertPlanBindings(t, result.Plan.ParameterIDs(), result.Plan.Catalog(), wantIDs, wantEndpoints, 1)

	// Each exported reference layer belongs to the returned values. These
	// mutations must not alter the immutable plan retained by the planner.
	returnedIDs := result.Plan.ParameterIDs()
	for index := range returnedIDs {
		returnedIDs[index] = parameters.ParameterJawX
	}
	returnedCatalog := result.Plan.Catalog()
	returnedCatalog.Generation = 0
	returnedCatalog.UpdatedAt = time.Time{}
	returnedCatalog.Hash = 0
	for index := range returnedCatalog.RawMethods {
		returnedCatalog.RawMethods[index] = osc.Endpoint{Address: "/mutated/raw", Type: "i"}
	}
	returnedCatalog.Outputs = returnedCatalog.Outputs[:0]
	for id, binding := range returnedCatalog.Bindings {
		for index := range binding.Direct {
			binding.Direct[index] = osc.Endpoint{Address: "/mutated/direct", Type: "i"}
		}
		binding.Binary = append(binding.Binary, osc.BinaryBinding{
			Negative: &osc.Endpoint{Address: "/mutated/negative", Type: "T"},
			Bits:     []osc.BinaryBit{{Endpoint: osc.Endpoint{Address: "/mutated/bit", Type: "T"}, Weight: 1}},
		})
		returnedCatalog.Bindings[id] = binding
	}
	delete(returnedCatalog.Bindings, parameters.ParameterJawOpen)
	returnedCatalog.Bindings[parameters.ParameterJawX] = osc.ParameterBinding{
		Direct: []osc.Endpoint{{Address: "/mutated/map", Type: "f"}},
	}
	assertPlanBindings(t, result.Plan.ParameterIDs(), result.Plan.Catalog(), wantIDs, wantEndpoints, 1)

	// A repeated avatar change deliberately recompiles the selected file. It
	// must not reuse caller-mutated data from the first returned plan.
	second := planner.Activate(avatarID)
	if second.Err != nil {
		t.Fatalf("second Activate() error = %v", second.Err)
	}
	if second.Plan == nil {
		t.Fatal("second Activate() returned nil plan")
	}
	assertPlanBindings(t, second.Plan.ParameterIDs(), second.Plan.Catalog(), wantIDs, wantEndpoints, 2)
}

func assertPlanBindings(
	t testing.TB,
	gotIDs []parameters.ParameterID,
	catalog *osc.Catalog,
	wantIDs []parameters.ParameterID,
	wantEndpoints map[parameters.ParameterID]osc.Endpoint,
	wantGeneration uint64,
) {
	t.Helper()
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("ParameterIDs() length = %d, want %d", len(gotIDs), len(wantIDs))
	}
	for index, want := range wantIDs {
		if got := gotIDs[index]; got != want {
			t.Fatalf("ParameterIDs()[%d] = %d, want %d", index, got, want)
		}
	}
	if catalog == nil {
		t.Fatal("Catalog() = nil")
	}
	if got := catalog.Generation; got != wantGeneration {
		t.Fatalf("Catalog().Generation = %d, want %d", got, wantGeneration)
	}
	if got := len(catalog.Bindings); got != len(wantEndpoints) {
		t.Fatalf("Catalog().Bindings length = %d, want %d", got, len(wantEndpoints))
	}
	if got := len(catalog.RawMethods); got != len(wantEndpoints) {
		t.Fatalf("Catalog().RawMethods length = %d, want %d", got, len(wantEndpoints))
	}
	if got := len(catalog.Outputs); got != len(wantEndpoints) {
		t.Fatalf("Catalog().Outputs length = %d, want %d", got, len(wantEndpoints))
	}
	for id, want := range wantEndpoints {
		binding, ok := catalog.Bindings[id]
		if !ok {
			t.Fatalf("Catalog() missing binding for %d", id)
		}
		if len(binding.Direct) != 1 || binding.Direct[0] != want {
			t.Fatalf("Catalog() binding for %d = %#v, want one direct endpoint %#v", id, binding, want)
		}
		if len(binding.Binary) != 0 {
			t.Fatalf("Catalog() binding for %d has %d binary bindings, want 0", id, len(binding.Binary))
		}
	}
	for _, endpoint := range catalog.RawMethods {
		matched := false
		for _, want := range wantEndpoints {
			if endpoint == want {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("Catalog().RawMethods contains unexpected endpoint %#v", endpoint)
		}
	}
}
