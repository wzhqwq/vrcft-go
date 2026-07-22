package parameterdeps

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/specparser"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestPlansCoverEveryFloatParameterAndExpressionPrimitive(t *testing.T) {
	doc, _, err := specparser.LoadFile(filepath.Join("..", "..", "spec", "vrcft_osc_parameters.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	floatIDs := make([]parameters.ParameterID, 0, len(doc.DetailedParameters)+len(doc.SimplifiedParameters))
	sections := []struct {
		name       string
		parameters []specparser.ParameterSpec
	}{
		{"detailed", doc.DetailedParameters},
		{"simplified", doc.SimplifiedParameters},
	}
	for _, section := range sections {
		for _, spec := range section.parameters {
			if spec.ValueType != "float" {
				continue
			}
			id, ok := parameters.LookupOSCName(spec.OSCName)
			if !ok {
				t.Fatalf("LookupOSCName(%q) failed", spec.OSCName)
			}
			plan, ok := Plan(id)
			if !ok {
				t.Errorf("Plan(%s) missing", spec.OSCName)
				continue
			}
			if plan.Operation == 0 {
				t.Errorf("Plan(%s) has no operation", spec.OSCName)
			}
			if plan.Operation == OperationDirect && plan.Inputs.IsZero() {
				t.Errorf("direct Plan(%s) has no primitive input", spec.OSCName)
			}
			if section.name == "simplified" && (len(plan.DependsOn) == 0 || !plan.Inputs.IsZero()) {
				t.Errorf("simplified Plan(%s) is not derived: %+v", spec.OSCName, plan)
			}
			leaves, err := ResolveLeaves(id)
			if err != nil {
				t.Errorf("ResolveLeaves(%s): %v", spec.OSCName, err)
				continue
			}
			if leaves.IsZero() {
				t.Errorf("ResolveLeaves(%s) is empty", spec.OSCName)
			}
			floatIDs = append(floatIDs, id)
		}
	}

	all, err := RequiredInputs(floatIDs)
	if err != nil {
		t.Fatal(err)
	}
	for id := trackingmodel.ExpressionID(0); id < trackingmodel.ExpressionCount; id++ {
		if !all.Expressions.Has(id) {
			t.Errorf("expression primitive %d (%s) is orphaned", id, trackingmodel.ExpressionNames()[id])
		}
	}
}

func TestRepresentativeDirectPlans(t *testing.T) {
	tests := []struct {
		name string
		id   parameters.ParameterID
		want Inputs
	}{
		{"left gaze", parameters.ParameterEyeLeftX, Inputs{Eye: trackingmodel.EyeValidLeftGaze}},
		{"right gaze", parameters.ParameterEyeRightY, Inputs{Eye: trackingmodel.EyeValidRightGaze}},
		{"left eyelid", parameters.ParameterEyeLidLeft, Inputs{Eye: trackingmodel.EyeValidLeftOpenness}},
		{"pupil dilation", parameters.ParameterPupilDilation, Inputs{Eye: trackingmodel.EyeValidLeftPupil | trackingmodel.EyeValidRightPupil}},
		{"expression", parameters.ParameterJawOpen, Inputs{Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, ok := Plan(tt.id)
			if !ok {
				t.Fatal("plan missing")
			}
			if plan.Operation != OperationDirect || len(plan.DependsOn) != 0 || plan.Inputs != tt.want {
				t.Fatalf("Plan(%d) = %+v, want direct %+v", tt.id, plan, tt.want)
			}
		})
	}
}

func TestRepresentativeDerivedPlans(t *testing.T) {
	tests := []struct {
		name      string
		id        parameters.ParameterID
		operation Operation
		dependsOn []parameters.ParameterID
	}{
		{"eyelid", parameters.ParameterEyeLid, OperationAverage, []parameters.ParameterID{parameters.ParameterEyeLidRight, parameters.ParameterEyeLidLeft}},
		{"eye squint", parameters.ParameterEyeSquint, OperationAverage, []parameters.ParameterID{parameters.ParameterEyeSquintRight, parameters.ParameterEyeSquintLeft}},
		{"smile frown right", parameters.ParameterSmileFrownRight, OperationSignedPair, []parameters.ParameterID{parameters.ParameterMouthCornerPullRight, parameters.ParameterMouthCornerSlantRight, parameters.ParameterMouthFrownRight}},
		{"mouth open", parameters.ParameterMouthOpen, OperationSumClamp, []parameters.ParameterID{parameters.ParameterMouthUpperUp, parameters.ParameterMouthLowerDown}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, ok := Plan(tt.id)
			if !ok {
				t.Fatal("plan missing")
			}
			if plan.Operation != tt.operation || !equalIDs(plan.DependsOn, tt.dependsOn) || !plan.Inputs.IsZero() {
				t.Fatalf("Plan(%d) = %+v, want operation %d and dependencies %v", tt.id, plan, tt.operation, tt.dependsOn)
			}
		})
	}
}

func TestResolveLeavesRepresentativeDerivedPlan(t *testing.T) {
	got, err := ResolveLeaves(parameters.ParameterSmileFrownRight)
	if err != nil {
		t.Fatal(err)
	}
	want := Inputs{Expressions: trackingmodel.ExpressionMaskOf(
		trackingmodel.ExpressionMouthCornerPullRight,
		trackingmodel.ExpressionMouthCornerSlantRight,
		trackingmodel.ExpressionMouthFrownRight,
	)}
	if got != want {
		t.Fatalf("ResolveLeaves(SmileFrownRight) = %+v, want %+v", got, want)
	}
}

func TestResolveLeavesReportsMissingReference(t *testing.T) {
	plans := map[parameters.ParameterID]DependencyPlan{
		parameters.ParameterEyeLid: {
			DependsOn: []parameters.ParameterID{parameters.ParameterEyeLidLeft},
			Operation: OperationAverage,
		},
	}
	_, err := resolveLeaves(parameters.ParameterEyeLid, plans)
	if err == nil || !strings.Contains(err.Error(), "v2/EyeLid") || !strings.Contains(err.Error(), "v2/EyeLidLeft") {
		t.Fatalf("resolveLeaves() error = %v, want missing reference identities", err)
	}
}

func TestResolveLeavesReportsCycle(t *testing.T) {
	plans := map[parameters.ParameterID]DependencyPlan{
		parameters.ParameterEyeLid: {
			DependsOn: []parameters.ParameterID{parameters.ParameterEyeSquint},
			Operation: OperationAverage,
		},
		parameters.ParameterEyeSquint: {
			DependsOn: []parameters.ParameterID{parameters.ParameterEyeLid},
			Operation: OperationAverage,
		},
	}
	_, err := resolveLeaves(parameters.ParameterEyeLid, plans)
	if err == nil || !strings.Contains(err.Error(), "cycle") || !strings.Contains(err.Error(), "v2/EyeLid") || !strings.Contains(err.Error(), "v2/EyeSquint") {
		t.Fatalf("resolveLeaves() error = %v, want cycle identities", err)
	}
}

func TestResolveLeavesRejectsUnknownID(t *testing.T) {
	id := parameters.ParameterCount + 10
	_, err := ResolveLeaves(id)
	if err == nil || !strings.Contains(err.Error(), "137") {
		t.Fatalf("ResolveLeaves(%d) error = %v, want identity", id, err)
	}
}

func TestRequiredInputsUnionsLeavesAndPropagatesErrors(t *testing.T) {
	got, err := RequiredInputs([]parameters.ParameterID{parameters.ParameterEyeLeftX, parameters.ParameterJawOpen})
	if err != nil {
		t.Fatal(err)
	}
	want := Inputs{
		Eye:         trackingmodel.EyeValidLeftGaze,
		Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	}
	if got != want {
		t.Fatalf("RequiredInputs() = %+v, want %+v", got, want)
	}
	if _, err := RequiredInputs([]parameters.ParameterID{parameters.ParameterCount}); err == nil {
		t.Fatal("RequiredInputs(unknown) returned nil error")
	}
}

func TestPlanReturnsDefensiveDependencyCopy(t *testing.T) {
	first, ok := Plan(parameters.ParameterEyeLid)
	if !ok || len(first.DependsOn) == 0 {
		t.Fatal("combined eyelid plan missing")
	}
	first.DependsOn[0] = parameters.ParameterJawOpen

	second, _ := Plan(parameters.ParameterEyeLid)
	if second.DependsOn[0] != parameters.ParameterEyeLidRight {
		t.Fatalf("package plan mutated through returned slice: %v", second.DependsOn)
	}
}

func equalIDs(a, b []parameters.ParameterID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
