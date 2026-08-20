package avatar_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type expectedBinding struct {
	direct []osc.Endpoint
	binary []osc.BinaryBinding
}

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
			{"name": "Face/v2/JawXNegative", "input": {"address": "/avatar/parameters/Face/v2/JawXNegative", "type": "Bool"}},
			{"name": "Face/v2/JawX1", "input": {"address": "/avatar/parameters/Face/v2/JawX1", "type": "Bool"}},
			{"name": "Face/v2/JawX2", "input": {"address": "/avatar/parameters/Face/v2/JawX2", "type": "Bool"}},
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
	if _, ok := source.Float(parameters.ParameterJawZ); ok {
		t.Fatal("unbound JawZ became externally visible")
	}

	wantIDs := []parameters.ParameterID{
		parameters.ParameterJawOpen,
		parameters.ParameterJawX,
		parameters.ParameterExpressionTrackingActive,
	}
	wantBindings := map[parameters.ParameterID]expectedBinding{
		parameters.ParameterJawOpen: {
			direct: []osc.Endpoint{{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"}},
		},
		parameters.ParameterJawX: {
			binary: []osc.BinaryBinding{{
				Negative: &osc.Endpoint{Address: "/avatar/parameters/Face/v2/JawXNegative", Type: "T"},
				Bits: []osc.BinaryBit{
					{Endpoint: osc.Endpoint{Address: "/avatar/parameters/Face/v2/JawX1", Type: "T"}, Weight: 1},
					{Endpoint: osc.Endpoint{Address: "/avatar/parameters/Face/v2/JawX2", Type: "T"}, Weight: 2},
				},
			}},
		},
		parameters.ParameterExpressionTrackingActive: {
			direct: []osc.Endpoint{{Address: "/avatar/parameters/ExpressionTrackingActive", Type: "T"}},
		},
	}
	assertPlanBindings(t, result.Plan.ParameterIDs(), result.Plan.Catalog(), wantIDs, wantBindings, 1)

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
		for binaryIndex := range binding.Binary {
			binary := &binding.Binary[binaryIndex]
			if binary.Negative == nil {
				t.Fatalf("Catalog() binary binding for %d has no negative endpoint", id)
			}
			binary.Negative.Address = "/mutated/negative"
			for bitIndex := range binary.Bits {
				binary.Bits[bitIndex].Endpoint.Address = "/mutated/bit"
			}
		}
		returnedCatalog.Bindings[id] = binding
	}
	delete(returnedCatalog.Bindings, parameters.ParameterJawOpen)
	returnedCatalog.Bindings[parameters.ParameterJawX] = osc.ParameterBinding{
		Direct: []osc.Endpoint{{Address: "/mutated/map", Type: "f"}},
	}
	assertPlanBindings(t, result.Plan.ParameterIDs(), result.Plan.Catalog(), wantIDs, wantBindings, 1)

	// A repeated avatar change deliberately recompiles the selected file. It
	// must use this changed file, not any caller-mutated first-plan data.
	if err := os.WriteFile(configPath, []byte(`{
		"id": "avtr_demo",
		"parameters": [
			{"name": "Face/v2/JawZ", "input": {"address": "/avatar/parameters/Face/v2/JawZ", "type": "Float"}},
			{"name": "EyeTrackingActive", "input": {"address": "/avatar/parameters/EyeTrackingActive", "type": "Bool"}}
		]
	}`), 0o600); err != nil {
		t.Fatalf("rewrite WriteFile() error = %v", err)
	}
	second := planner.Activate(avatarID)
	if second.Err != nil {
		t.Fatalf("second Activate() error = %v", second.Err)
	}
	if second.Plan == nil {
		t.Fatal("second Activate() returned nil plan")
	}
	secondIDs := []parameters.ParameterID{
		parameters.ParameterJawZ,
		parameters.ParameterEyeTrackingActive,
	}
	secondBindings := map[parameters.ParameterID]expectedBinding{
		parameters.ParameterJawZ: {
			direct: []osc.Endpoint{{Address: "/avatar/parameters/Face/v2/JawZ", Type: "f"}},
		},
		parameters.ParameterEyeTrackingActive: {
			direct: []osc.Endpoint{{Address: "/avatar/parameters/EyeTrackingActive", Type: "T"}},
		},
	}
	assertPlanBindings(t, second.Plan.ParameterIDs(), second.Plan.Catalog(), secondIDs, secondBindings, 2)

	var secondFrame processing.CanonicalFrame
	secondFrame.Generation = 2
	secondFrame.EyeActive = true
	secondFrame.Expressions.Set(trackingmodel.ExpressionJawZ, 0.25)
	secondSnapshot := second.Plan.Evaluator().Evaluate(secondFrame)
	var secondSource osc.ValueSource = secondSnapshot
	if value, ok := secondSource.Float(parameters.ParameterJawZ); !ok || value != 0.25 {
		t.Fatalf("second JawZ = %v,%t", value, ok)
	}
	if value, ok := secondSource.Bool(parameters.ParameterEyeTrackingActive); !ok || !value {
		t.Fatalf("second EyeTrackingActive = %v,%t", value, ok)
	}
	if _, ok := secondSource.Float(parameters.ParameterJawOpen); ok {
		t.Fatal("second plan retained old JawOpen output")
	}
	if _, ok := secondSource.Bool(parameters.ParameterExpressionTrackingActive); ok {
		t.Fatal("second plan retained old ExpressionTrackingActive output")
	}
}

func assertPlanBindings(
	t testing.TB,
	gotIDs []parameters.ParameterID,
	catalog *osc.Catalog,
	wantIDs []parameters.ParameterID,
	wantBindings map[parameters.ParameterID]expectedBinding,
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
	if got := len(catalog.Bindings); got != len(wantBindings) {
		t.Fatalf("Catalog().Bindings length = %d, want %d", got, len(wantBindings))
	}
	wantRawMethods := make(map[osc.Endpoint]struct{})
	wantOutputs := 0
	for _, want := range wantBindings {
		for _, endpoint := range want.direct {
			wantRawMethods[endpoint] = struct{}{}
			wantOutputs++
		}
		for _, binary := range want.binary {
			if binary.Negative != nil {
				wantRawMethods[*binary.Negative] = struct{}{}
				wantOutputs++
			}
			for _, bit := range binary.Bits {
				wantRawMethods[bit.Endpoint] = struct{}{}
				wantOutputs++
			}
		}
	}
	if got := len(catalog.RawMethods); got != len(wantRawMethods) {
		t.Fatalf("Catalog().RawMethods length = %d, want %d", got, len(wantRawMethods))
	}
	if got := len(catalog.Outputs); got != wantOutputs {
		t.Fatalf("Catalog().Outputs length = %d, want %d", got, wantOutputs)
	}
	for id, want := range wantBindings {
		binding, ok := catalog.Bindings[id]
		if !ok {
			t.Fatalf("Catalog() missing binding for %d", id)
		}
		if !reflect.DeepEqual(binding.Direct, want.direct) {
			t.Fatalf("Catalog() direct binding for %d = %#v, want %#v", id, binding.Direct, want.direct)
		}
		if !reflect.DeepEqual(binding.Binary, want.binary) {
			t.Fatalf("Catalog() binary binding for %d = %#v, want %#v", id, binding.Binary, want.binary)
		}
	}
	for _, endpoint := range catalog.RawMethods {
		if _, ok := wantRawMethods[endpoint]; !ok {
			t.Fatalf("Catalog().RawMethods contains unexpected endpoint %#v", endpoint)
		}
	}
}
