package evaluator

import (
	"fmt"

	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const parameterWordCount = (parameters.ParameterCount + 63) / 64

type parameterSet [parameterWordCount]uint64

func (s *parameterSet) add(id parameters.ParameterID) {
	s[id/64] |= uint64(1) << (id % 64)
}

func (s parameterSet) has(id parameters.ParameterID) bool {
	return id < parameters.ParameterCount && s[id/64]&(uint64(1)<<(id%64)) != 0
}

type instruction struct {
	parameter parameters.ParameterID
	valueType parameters.ValueType
	operation parameterdeps.Operation
	operands  []operand
}

type Plan struct {
	requested    parameterSet
	instructions []instruction
}

type planLookup func(parameters.ParameterID) (parameterdeps.DependencyPlan, bool)

func Compile(requested []parameters.ParameterID) (*Plan, error) {
	return compileWithPlans(requested, parameterdeps.Plan)
}

func compileWithPlans(requested []parameters.ParameterID, lookup planLookup) (*Plan, error) {
	var requestedSet parameterSet
	for _, id := range requested {
		if _, ok := parameters.Definition(id); !ok {
			return nil, fmt.Errorf("%w: id %d", ErrUnknownParameter, id)
		}
		requestedSet.add(id)
	}

	compiled := &Plan{
		requested:    requestedSet,
		instructions: make([]instruction, 0, parameters.ParameterCount),
	}
	if len(requested) == 0 {
		return compiled, nil
	}

	const (
		unvisited uint8 = iota
		visiting
		visited
	)
	var states [parameters.ParameterCount]uint8
	stack := make([]parameters.ParameterID, 0, parameters.ParameterCount)

	var visit func(parameters.ParameterID) error
	visit = func(id parameters.ParameterID) error {
		definition, ok := parameters.Definition(id)
		if !ok {
			return fmt.Errorf("%w: id %d", ErrUnknownParameter, id)
		}

		switch states[id] {
		case visiting:
			return fmt.Errorf("%w: %s", ErrDependencyCycle, formatCycle(stack, id))
		case visited:
			return nil
		}

		dependencyPlan, ok := lookup(id)
		if !ok {
			return fmt.Errorf("%w: parameter %d", ErrMissingPlan, id)
		}

		states[id] = visiting
		stack = append(stack, id)
		for _, dependency := range dependencyPlan.DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}

		operands := enumerateOperands(dependencyPlan)
		if err := validateInstruction(definition, dependencyPlan.Operation, operands); err != nil {
			return fmt.Errorf("parameter %d: %w", id, err)
		}
		compiled.instructions = append(compiled.instructions, instruction{
			parameter: id,
			valueType: definition.ValueType,
			operation: dependencyPlan.Operation,
			operands:  operands,
		})
		stack = stack[:len(stack)-1]
		states[id] = visited
		return nil
	}

	for id := parameters.ParameterID(0); id < parameters.ParameterCount; id++ {
		if requestedSet.has(id) {
			if err := visit(id); err != nil {
				return nil, err
			}
		}
	}
	return compiled, nil
}

func enumerateOperands(plan parameterdeps.DependencyPlan) []operand {
	operands := make([]operand, 0, primitiveCount(plan.Inputs)+len(plan.DependsOn))
	for field := parameterdeps.EyeFieldLeftGazeX; field <= parameterdeps.EyeFieldRightPupilDilation; field++ {
		if plan.Inputs.Eye.Has(field) {
			operands = append(operands, operand{kind: operandEye, eye: field})
		}
	}
	for expression := trackingmodel.ExpressionID(0); expression < trackingmodel.ExpressionCount; expression++ {
		if plan.Inputs.Expressions.Has(expression) {
			operands = append(operands, operand{kind: operandExpression, expression: expression})
		}
	}
	for active := parameterdeps.ActiveStateEyeTracking; active <= parameterdeps.ActiveStateLipTracking; active++ {
		if plan.Inputs.Active.Has(active) {
			operands = append(operands, operand{kind: operandActive, active: active})
		}
	}
	for _, dependency := range plan.DependsOn {
		operands = append(operands, operand{kind: operandParameter, parameter: dependency})
	}
	return operands
}

func primitiveCount(inputs parameterdeps.Inputs) int {
	count := 0
	for field := parameterdeps.EyeFieldLeftGazeX; field <= parameterdeps.EyeFieldRightPupilDilation; field++ {
		if inputs.Eye.Has(field) {
			count++
		}
	}
	for expression := trackingmodel.ExpressionID(0); expression < trackingmodel.ExpressionCount; expression++ {
		if inputs.Expressions.Has(expression) {
			count++
		}
	}
	for active := parameterdeps.ActiveStateEyeTracking; active <= parameterdeps.ActiveStateLipTracking; active++ {
		if inputs.Active.Has(active) {
			count++
		}
	}
	return count
}

func validateInstruction(definition parameters.ParameterDefinition, operation parameterdeps.Operation, operands []operand) error {
	switch definition.ValueType {
	case parameters.ValueFloat, parameters.ValueBool:
	default:
		return fmt.Errorf("%w: unsupported generated value type %d", ErrInvalidOperation, definition.ValueType)
	}

	switch operation {
	case parameterdeps.OperationDirect:
		if len(operands) != 1 {
			return fmt.Errorf("%w: direct requires exactly one operand, got %d", ErrInvalidOperation, len(operands))
		}
	case parameterdeps.OperationAverage, parameterdeps.OperationMax, parameterdeps.OperationSumClamp:
		if len(operands) == 0 {
			return fmt.Errorf("%w: operation %d requires at least one operand", ErrInvalidOperation, operation)
		}
	case parameterdeps.OperationSignedPair:
		if len(operands) < 2 {
			return fmt.Errorf("%w: signed pair requires at least two operands, got %d", ErrInvalidOperation, len(operands))
		}
	default:
		return fmt.Errorf("%w: unsupported operation %d", ErrInvalidOperation, operation)
	}

	if definition.ValueType == parameters.ValueBool && operation != parameterdeps.OperationDirect {
		return fmt.Errorf("%w: bool output requires direct operation", ErrInvalidOperation)
	}
	for _, current := range operands {
		operandType, ok := valueTypeOf(current)
		if !ok || operandType != definition.ValueType {
			return fmt.Errorf("%w: operand type %d does not match generated type %d", ErrInvalidOperation, operandType, definition.ValueType)
		}
	}
	return nil
}

func valueTypeOf(current operand) (parameters.ValueType, bool) {
	switch current.kind {
	case operandEye, operandExpression:
		return parameters.ValueFloat, true
	case operandActive:
		return parameters.ValueBool, true
	case operandParameter:
		definition, ok := parameters.Definition(current.parameter)
		if !ok {
			return 0, false
		}
		return definition.ValueType, true
	default:
		return 0, false
	}
}

func formatCycle(stack []parameters.ParameterID, repeated parameters.ParameterID) string {
	start := 0
	for start < len(stack) && stack[start] != repeated {
		start++
	}
	cycle := ""
	for index := start; index < len(stack); index++ {
		if cycle != "" {
			cycle += " -> "
		}
		cycle += fmt.Sprint(stack[index])
	}
	if cycle != "" {
		cycle += " -> "
	}
	return cycle + fmt.Sprint(repeated)
}
