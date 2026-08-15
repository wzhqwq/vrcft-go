package evaluator

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestCompileEmptyRequestProducesEmptyPlan(t *testing.T) {
	plan, err := Compile(nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		t.Fatal("Compile(nil) returned a nil plan")
	}
	if len(plan.instructions) != 0 || plan.requested != (parameterSet{}) {
		t.Fatalf("empty plan = %#v", plan)
	}
}

func TestCompileDeduplicatesRequestsAndUsesStableDFSTopology(t *testing.T) {
	requested := []parameters.ParameterID{
		parameters.ParameterMouthOpen,
		parameters.ParameterEyeX,
		parameters.ParameterMouthOpen,
	}
	plan, err := Compile(requested)
	if err != nil {
		t.Fatal(err)
	}

	wantIDs := []parameters.ParameterID{
		parameters.ParameterEyeLeftX,
		parameters.ParameterEyeRightX,
		parameters.ParameterEyeX,
		parameters.ParameterMouthUpperUpRight,
		parameters.ParameterMouthUpperUpLeft,
		parameters.ParameterMouthUpperUp,
		parameters.ParameterMouthLowerDownRight,
		parameters.ParameterMouthLowerDownLeft,
		parameters.ParameterMouthLowerDown,
		parameters.ParameterMouthOpen,
	}
	assertInstructionIDs(t, plan, wantIDs)
	if !plan.requested.has(parameters.ParameterEyeX) || !plan.requested.has(parameters.ParameterMouthOpen) {
		t.Fatalf("requested set omitted a requested parameter: %#v", plan.requested)
	}
	reordered, err := Compile([]parameters.ParameterID{parameters.ParameterEyeX, parameters.ParameterMouthOpen})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan, reordered) {
		t.Fatalf("request order changed compiled plan:\nfirst  = %#v\nsecond = %#v", plan, reordered)
	}
}

func TestCompileOrdersPrimitiveOperandsBeforeDependencies(t *testing.T) {
	plans := map[parameters.ParameterID]parameterdeps.DependencyPlan{
		parameters.ParameterEyeLid: {
			Inputs: parameterdeps.Inputs{
				Eye: parameterdeps.EyeFieldsOf(
					parameterdeps.EyeFieldRightGazeX,
					parameterdeps.EyeFieldLeftGazeX,
				),
				Expressions: trackingmodel.ExpressionMaskOf(
					trackingmodel.ExpressionBrowPinchRight,
					trackingmodel.ExpressionEyeSquintRight,
				),
			},
			DependsOn: []parameters.ParameterID{
				parameters.ParameterPupilDiameterRight,
				parameters.ParameterPupilDiameterLeft,
			},
			Operation: parameterdeps.OperationAverage,
		},
		parameters.ParameterPupilDiameterRight: {
			Inputs:    parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldRightPupilDiameter)},
			Operation: parameterdeps.OperationDirect,
		},
		parameters.ParameterPupilDiameterLeft: {
			Inputs:    parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftPupilDiameter)},
			Operation: parameterdeps.OperationDirect,
		},
	}

	plan, err := compileWithPlans([]parameters.ParameterID{parameters.ParameterEyeLid}, mapLookup(plans))
	if err != nil {
		t.Fatal(err)
	}
	assertInstructionIDs(t, plan, []parameters.ParameterID{
		parameters.ParameterPupilDiameterRight,
		parameters.ParameterPupilDiameterLeft,
		parameters.ParameterEyeLid,
	})

	wantOperands := []operand{
		{kind: operandEye, eye: parameterdeps.EyeFieldLeftGazeX},
		{kind: operandEye, eye: parameterdeps.EyeFieldRightGazeX},
		{kind: operandExpression, expression: trackingmodel.ExpressionEyeSquintRight},
		{kind: operandExpression, expression: trackingmodel.ExpressionBrowPinchRight},
		{kind: operandParameter, parameter: parameters.ParameterPupilDiameterRight},
		{kind: operandParameter, parameter: parameters.ParameterPupilDiameterLeft},
	}
	if got := plan.instructions[2].operands; !reflect.DeepEqual(got, wantOperands) {
		t.Fatalf("target operands = %#v, want %#v", got, wantOperands)
	}
}

func TestCompileAcceptsEveryGeneratedParameter(t *testing.T) {
	for id := parameters.ParameterID(0); id < parameters.ParameterCount; id++ {
		if _, err := Compile([]parameters.ParameterID{id}); err != nil {
			t.Fatalf("Compile(%d): %v", id, err)
		}
	}
}

func TestCompileDoesNotRetainCallerRequestSlice(t *testing.T) {
	requested := []parameters.ParameterID{parameters.ParameterEyeX}
	plan, err := Compile(requested)
	if err != nil {
		t.Fatal(err)
	}
	requested[0] = parameters.ParameterMouthOpen

	if !plan.requested.has(parameters.ParameterEyeX) || plan.requested.has(parameters.ParameterMouthOpen) {
		t.Fatalf("caller mutation changed requested set: %#v", plan.requested)
	}
	if got := plan.instructions[len(plan.instructions)-1].parameter; got != parameters.ParameterEyeX {
		t.Fatalf("caller mutation changed final instruction to %d", got)
	}
}

func TestCompileOwnsLookupDependsOnBackingSliceAcrossMutationAndReuse(t *testing.T) {
	dependencies := []parameters.ParameterID{
		parameters.ParameterEyeLidRight,
		parameters.ParameterEyeLidLeft,
	}
	leafPlans := map[parameters.ParameterID]parameterdeps.DependencyPlan{
		parameters.ParameterEyeLidRight: {
			Inputs:    parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldRightOpenness)},
			Operation: parameterdeps.OperationDirect,
		},
		parameters.ParameterEyeLidLeft: {
			Inputs:    parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftOpenness)},
			Operation: parameterdeps.OperationDirect,
		},
		parameters.ParameterEyeLeftX: {
			Inputs:    parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftGazeX)},
			Operation: parameterdeps.OperationDirect,
		},
		parameters.ParameterEyeRightX: {
			Inputs:    parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldRightGazeX)},
			Operation: parameterdeps.OperationDirect,
		},
	}
	lookup := func(id parameters.ParameterID) (parameterdeps.DependencyPlan, bool) {
		if id == parameters.ParameterEyeLid {
			return parameterdeps.DependencyPlan{DependsOn: dependencies, Operation: parameterdeps.OperationAverage}, true
		}
		plan, ok := leafPlans[id]
		return plan, ok
	}

	first, err := compileWithPlans([]parameters.ParameterID{parameters.ParameterEyeLid}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	dependencies[0] = parameters.ParameterEyeLeftX
	dependencies[1] = parameters.ParameterEyeRightX
	second, err := compileWithPlans([]parameters.ParameterID{parameters.ParameterEyeLid}, lookup)
	if err != nil {
		t.Fatal(err)
	}

	frame := processing.CanonicalFrame{Eye: trackingmodel.EyeSample{
		Valid:         trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze | trackingmodel.EyeValidLeftOpenness | trackingmodel.EyeValidRightOpenness,
		LeftGaze:      trackingmodel.Vec2{X: 0.125},
		RightGaze:     trackingmodel.Vec2{X: 0.375},
		LeftOpenness:  0.25,
		RightOpenness: 0.75,
	}}
	if got, ok := first.Evaluate(frame).Float(parameters.ParameterEyeLid); !ok || got != 0.5 {
		t.Fatalf("first compiled plan after backing-slice reuse = %v,%t, want 0.5,true", got, ok)
	}
	if got, ok := second.Evaluate(frame).Float(parameters.ParameterEyeLid); !ok || got != 0.25 {
		t.Fatalf("second compiled plan from reused backing slice = %v,%t, want 0.25,true", got, ok)
	}
}

func TestCompileRejectsUnknownParameter(t *testing.T) {
	_, err := Compile([]parameters.ParameterID{parameters.ParameterCount})
	if !errors.Is(err, ErrUnknownParameter) {
		t.Fatalf("Compile(unknown) error = %v, want ErrUnknownParameter", err)
	}
}

func TestCompileRejectsMalformedPlans(t *testing.T) {
	validEye := parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftGazeX)}
	twoEyes := parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(
		parameterdeps.EyeFieldLeftGazeX,
		parameterdeps.EyeFieldRightGazeX,
	)}
	active := parameterdeps.Inputs{Active: parameterdeps.ActiveStatesOf(parameterdeps.ActiveStateEyeTracking)}

	tests := []struct {
		name      string
		requested parameters.ParameterID
		plans     map[parameters.ParameterID]parameterdeps.DependencyPlan
		want      error
	}{
		{
			name:      "missing root plan",
			requested: parameters.ParameterEyeLeftX,
			plans:     map[parameters.ParameterID]parameterdeps.DependencyPlan{},
			want:      ErrMissingPlan,
		},
		{
			name:      "missing dependency plan",
			requested: parameters.ParameterEyeLid,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLid: {
					DependsOn: []parameters.ParameterID{parameters.ParameterEyeLidLeft},
					Operation: parameterdeps.OperationAverage,
				},
			},
			want: ErrMissingPlan,
		},
		{
			name:      "dependency cycle",
			requested: parameters.ParameterEyeLid,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLid: {
					DependsOn: []parameters.ParameterID{parameters.ParameterEyeSquint},
					Operation: parameterdeps.OperationAverage,
				},
				parameters.ParameterEyeSquint: {
					DependsOn: []parameters.ParameterID{parameters.ParameterEyeLid},
					Operation: parameterdeps.OperationAverage,
				},
			},
			want: ErrDependencyCycle,
		},
		{
			name:      "direct has no operand",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Operation: parameterdeps.OperationDirect},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "direct has two operands",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Inputs: twoEyes, Operation: parameterdeps.OperationDirect},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "direct float parameter operand",
			requested: parameters.ParameterEyeLid,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLid: {
					DependsOn: []parameters.ParameterID{parameters.ParameterEyeLidLeft},
					Operation: parameterdeps.OperationDirect,
				},
				parameters.ParameterEyeLidLeft: {
					Inputs:    validEye,
					Operation: parameterdeps.OperationDirect,
				},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "direct bool parameter operand",
			requested: parameters.ParameterExpressionTrackingActive,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterExpressionTrackingActive: {
					DependsOn: []parameters.ParameterID{parameters.ParameterEyeTrackingActive},
					Operation: parameterdeps.OperationDirect,
				},
				parameters.ParameterEyeTrackingActive: {
					Inputs:    active,
					Operation: parameterdeps.OperationDirect,
				},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "signed pair has one operand",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Inputs: validEye, Operation: parameterdeps.OperationSignedPair},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "average has no operand",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Operation: parameterdeps.OperationAverage},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "max has no operand",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Operation: parameterdeps.OperationMax},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "sum clamp has no operand",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Operation: parameterdeps.OperationSumClamp},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "unsupported operation",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Inputs: validEye, Operation: parameterdeps.Operation(255)},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "bool parameter has float operand",
			requested: parameters.ParameterEyeTrackingActive,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeTrackingActive: {Inputs: validEye, Operation: parameterdeps.OperationDirect},
			},
			want: ErrInvalidOperation,
		},
		{
			name:      "float parameter has bool operand",
			requested: parameters.ParameterEyeLeftX,
			plans: map[parameters.ParameterID]parameterdeps.DependencyPlan{
				parameters.ParameterEyeLeftX: {Inputs: active, Operation: parameterdeps.OperationDirect},
			},
			want: ErrInvalidOperation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compileWithPlans([]parameters.ParameterID{tt.requested}, mapLookup(tt.plans))
			if !errors.Is(err, tt.want) {
				t.Fatalf("compileWithPlans() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func assertInstructionIDs(t *testing.T, plan *Plan, want []parameters.ParameterID) {
	t.Helper()
	if len(plan.instructions) != len(want) {
		t.Fatalf("instruction count = %d, want %d: %#v", len(plan.instructions), len(want), plan.instructions)
	}
	for index, wantID := range want {
		if got := plan.instructions[index].parameter; got != wantID {
			t.Fatalf("instruction %d parameter = %d, want %d", index, got, wantID)
		}
	}
}

func mapLookup(plans map[parameters.ParameterID]parameterdeps.DependencyPlan) func(parameters.ParameterID) (parameterdeps.DependencyPlan, bool) {
	return func(id parameters.ParameterID) (parameterdeps.DependencyPlan, bool) {
		plan, ok := plans[id]
		return plan, ok
	}
}
