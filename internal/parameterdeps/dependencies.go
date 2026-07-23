// Package parameterdeps describes which tracking inputs are required to
// produce each OSC float or tracking-active Boolean parameter. It records dependency and operation
// metadata; it does not evaluate parameter values.
package parameterdeps

import (
	"fmt"
	"strings"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

// EyeField identifies a concrete EyeSample value used by a parameter.
type EyeField uint8

const (
	EyeFieldLeftGazeX EyeField = iota + 1
	EyeFieldLeftGazeY
	EyeFieldRightGazeX
	EyeFieldRightGazeY
	EyeFieldLeftOpenness
	EyeFieldRightOpenness
	EyeFieldLeftPupilDiameter
	EyeFieldRightPupilDiameter
	EyeFieldLeftPupilDilation
	EyeFieldRightPupilDilation
	eyeFieldCount
)

// EyeFields is a set of concrete eye-field leaves.
type EyeFields uint16

func EyeFieldsOf(fields ...EyeField) EyeFields {
	var result EyeFields
	for _, field := range fields {
		if field > 0 && field < eyeFieldCount {
			result |= EyeFields(1) << (field - 1)
		}
	}
	return result
}

func (f EyeFields) Has(field EyeField) bool {
	if field == 0 || field >= eyeFieldCount {
		return false
	}
	return f&(EyeFields(1)<<(field-1)) != 0
}

// EyeValid converts concrete field requirements to the coarser validity bits
// carried by tracking subscriptions.
func (f EyeFields) EyeValid() trackingmodel.EyeValid {
	var valid trackingmodel.EyeValid
	if f.Has(EyeFieldLeftGazeX) || f.Has(EyeFieldLeftGazeY) {
		valid |= trackingmodel.EyeValidLeftGaze
	}
	if f.Has(EyeFieldRightGazeX) || f.Has(EyeFieldRightGazeY) {
		valid |= trackingmodel.EyeValidRightGaze
	}
	if f.Has(EyeFieldLeftOpenness) {
		valid |= trackingmodel.EyeValidLeftOpenness
	}
	if f.Has(EyeFieldRightOpenness) {
		valid |= trackingmodel.EyeValidRightOpenness
	}
	if f.Has(EyeFieldLeftPupilDiameter) || f.Has(EyeFieldLeftPupilDilation) {
		valid |= trackingmodel.EyeValidLeftPupil
	}
	if f.Has(EyeFieldRightPupilDiameter) || f.Has(EyeFieldRightPupilDilation) {
		valid |= trackingmodel.EyeValidRightPupil
	}
	return valid
}

// ActiveState identifies a tracking-active Boolean source leaf.
type ActiveState uint8

const (
	ActiveStateEyeTracking ActiveState = iota + 1
	ActiveStateExpressionTracking
	ActiveStateLipTracking
	activeStateCount
)

// ActiveStates is a set of tracking-active source leaves.
type ActiveStates uint8

func ActiveStatesOf(states ...ActiveState) ActiveStates {
	var result ActiveStates
	for _, state := range states {
		if state > 0 && state < activeStateCount {
			result |= ActiveStates(1) << (state - 1)
		}
	}
	return result
}

func (s ActiveStates) Has(state ActiveState) bool {
	if state == 0 || state >= activeStateCount {
		return false
	}
	return s&(ActiveStates(1)<<(state-1)) != 0
}

type Inputs struct {
	Eye         EyeFields
	Expressions trackingmodel.ExpressionMask
	Active      ActiveStates
}

func (i Inputs) IsZero() bool {
	return i.Eye == 0 && i.Expressions.IsZero() && i.Active == 0
}

func (i Inputs) RequiredEyeValid() trackingmodel.EyeValid {
	return i.Eye.EyeValid()
}

type Operation uint8

const (
	OperationDirect Operation = iota + 1
	OperationAverage
	OperationMax
	OperationSignedPair
	OperationSumClamp
)

type DependencyPlan struct {
	Inputs    Inputs
	DependsOn []parameters.ParameterID
	Operation Operation
}

var dependencyPlans = buildPlans()

func Plan(id parameters.ParameterID) (DependencyPlan, bool) {
	plan, ok := dependencyPlans[id]
	if !ok {
		return DependencyPlan{}, false
	}
	plan.DependsOn = append([]parameters.ParameterID(nil), plan.DependsOn...)
	return plan, true
}

func ResolveLeaves(id parameters.ParameterID) (Inputs, error) {
	return resolveLeaves(id, dependencyPlans)
}

func RequiredInputs(ids []parameters.ParameterID) (Inputs, error) {
	var required Inputs
	for _, id := range ids {
		leaves, err := ResolveLeaves(id)
		if err != nil {
			return Inputs{}, err
		}
		required = union(required, leaves)
	}
	return required, nil
}

func resolveLeaves(id parameters.ParameterID, plans map[parameters.ParameterID]DependencyPlan) (Inputs, error) {
	const (
		unvisited uint8 = iota
		visiting
		visited
	)
	state := make(map[parameters.ParameterID]uint8)
	cache := make(map[parameters.ParameterID]Inputs)
	stack := make([]parameters.ParameterID, 0)

	var visit func(parameters.ParameterID, *parameters.ParameterID) (Inputs, error)
	visit = func(current parameters.ParameterID, parent *parameters.ParameterID) (Inputs, error) {
		plan, ok := plans[current]
		if !ok {
			if parent == nil {
				return Inputs{}, fmt.Errorf("unknown parameter %s", parameterIdentity(current))
			}
			return Inputs{}, fmt.Errorf("parameter %s depends on missing parameter %s", parameterIdentity(*parent), parameterIdentity(current))
		}

		switch state[current] {
		case visiting:
			start := 0
			for stack[start] != current {
				start++
			}
			cycle := append(append([]parameters.ParameterID(nil), stack[start:]...), current)
			identities := make([]string, len(cycle))
			for index, cycleID := range cycle {
				identities[index] = parameterIdentity(cycleID)
			}
			return Inputs{}, fmt.Errorf("parameter dependency cycle: %s", strings.Join(identities, " -> "))
		case visited:
			return cache[current], nil
		}

		state[current] = visiting
		stack = append(stack, current)
		leaves := plan.Inputs
		for _, dependency := range plan.DependsOn {
			dependencyLeaves, err := visit(dependency, &current)
			if err != nil {
				return Inputs{}, err
			}
			leaves = union(leaves, dependencyLeaves)
		}
		stack = stack[:len(stack)-1]
		state[current] = visited
		cache[current] = leaves
		return leaves, nil
	}

	return visit(id, nil)
}

func buildPlans() map[parameters.ParameterID]DependencyPlan {
	plans := make(map[parameters.ParameterID]DependencyPlan)
	directEye := func(id parameters.ParameterID, fields ...EyeField) {
		plans[id] = DependencyPlan{Inputs: Inputs{Eye: EyeFieldsOf(fields...)}, Operation: OperationDirect}
	}
	directEye(parameters.ParameterEyeLeftX, EyeFieldLeftGazeX)
	directEye(parameters.ParameterEyeLeftY, EyeFieldLeftGazeY)
	directEye(parameters.ParameterEyeRightX, EyeFieldRightGazeX)
	directEye(parameters.ParameterEyeRightY, EyeFieldRightGazeY)
	directEye(parameters.ParameterEyeLidRight, EyeFieldRightOpenness)
	directEye(parameters.ParameterEyeLidLeft, EyeFieldLeftOpenness)
	directEye(parameters.ParameterPupilDilation, EyeFieldLeftPupilDilation, EyeFieldRightPupilDilation)
	directEye(parameters.ParameterPupilDiameterRight, EyeFieldRightPupilDiameter)
	directEye(parameters.ParameterPupilDiameterLeft, EyeFieldLeftPupilDiameter)

	directActive := func(id parameters.ParameterID, state ActiveState) {
		plans[id] = DependencyPlan{
			Inputs:    Inputs{Active: ActiveStatesOf(state)},
			Operation: OperationDirect,
		}
	}
	directActive(parameters.ParameterEyeTrackingActive, ActiveStateEyeTracking)
	directActive(parameters.ParameterExpressionTrackingActive, ActiveStateExpressionTracking)
	directActive(parameters.ParameterLipTrackingActive, ActiveStateLipTracking)

	expressionIDs := make(map[string]trackingmodel.ExpressionID, trackingmodel.ExpressionCount)
	for id, name := range trackingmodel.ExpressionNames() {
		expressionIDs[name] = trackingmodel.ExpressionID(id)
	}
	for _, definition := range parameters.Definitions {
		if expressionID, ok := expressionIDs[definition.Name]; ok {
			plans[definition.ID] = DependencyPlan{
				Inputs:    Inputs{Expressions: trackingmodel.ExpressionMaskOf(expressionID)},
				Operation: OperationDirect,
			}
		}
	}

	derived := func(id parameters.ParameterID, operation Operation, dependencies ...parameters.ParameterID) {
		plans[id] = DependencyPlan{DependsOn: dependencies, Operation: operation}
	}
	derived(parameters.ParameterEyeLid, OperationAverage, parameters.ParameterEyeLidRight, parameters.ParameterEyeLidLeft)
	derived(parameters.ParameterEyeSquint, OperationAverage, parameters.ParameterEyeSquintRight, parameters.ParameterEyeSquintLeft)
	derived(parameters.ParameterPupilDiameter, OperationAverage, parameters.ParameterPupilDiameterRight, parameters.ParameterPupilDiameterLeft)

	derived(parameters.ParameterEyeX, OperationAverage, parameters.ParameterEyeLeftX, parameters.ParameterEyeRightX)
	derived(parameters.ParameterEyeY, OperationAverage, parameters.ParameterEyeLeftY, parameters.ParameterEyeRightY)
	derived(parameters.ParameterBrowDownRight, OperationMax, parameters.ParameterBrowPinchRight, parameters.ParameterBrowLowererRight)
	derived(parameters.ParameterBrowDownLeft, OperationMax, parameters.ParameterBrowPinchLeft, parameters.ParameterBrowLowererLeft)
	derived(parameters.ParameterBrowOuterUp, OperationAverage, parameters.ParameterBrowOuterUpRight, parameters.ParameterBrowOuterUpLeft)
	derived(parameters.ParameterBrowInnerUp, OperationAverage, parameters.ParameterBrowInnerUpRight, parameters.ParameterBrowInnerUpLeft)
	derived(parameters.ParameterBrowUp, OperationMax, parameters.ParameterBrowOuterUp, parameters.ParameterBrowInnerUp)
	derived(parameters.ParameterBrowExpressionRight, OperationSignedPair, parameters.ParameterBrowInnerUpRight, parameters.ParameterBrowOuterUpRight, parameters.ParameterBrowDownRight)
	derived(parameters.ParameterBrowExpressionLeft, OperationSignedPair, parameters.ParameterBrowInnerUpLeft, parameters.ParameterBrowOuterUpLeft, parameters.ParameterBrowDownLeft)
	derived(parameters.ParameterBrowExpression, OperationAverage, parameters.ParameterBrowExpressionRight, parameters.ParameterBrowExpressionLeft)
	derived(parameters.ParameterMouthX, OperationAverage, parameters.ParameterMouthUpperX, parameters.ParameterMouthLowerX)
	derived(parameters.ParameterMouthUpperUp, OperationAverage, parameters.ParameterMouthUpperUpRight, parameters.ParameterMouthUpperUpLeft)
	derived(parameters.ParameterMouthLowerDown, OperationAverage, parameters.ParameterMouthLowerDownRight, parameters.ParameterMouthLowerDownLeft)
	derived(parameters.ParameterMouthOpen, OperationSumClamp, parameters.ParameterMouthUpperUp, parameters.ParameterMouthLowerDown)
	derived(parameters.ParameterMouthSmileRight, OperationMax, parameters.ParameterMouthCornerPullRight, parameters.ParameterMouthCornerSlantRight)
	derived(parameters.ParameterMouthSmileLeft, OperationMax, parameters.ParameterMouthCornerPullLeft, parameters.ParameterMouthCornerSlantLeft)
	derived(parameters.ParameterMouthSadRight, OperationMax, parameters.ParameterMouthFrownRight, parameters.ParameterMouthStretchRight)
	derived(parameters.ParameterMouthSadLeft, OperationMax, parameters.ParameterMouthFrownLeft, parameters.ParameterMouthStretchLeft)
	derived(parameters.ParameterSmileFrownRight, OperationSignedPair, parameters.ParameterMouthCornerPullRight, parameters.ParameterMouthCornerSlantRight, parameters.ParameterMouthFrownRight)
	derived(parameters.ParameterSmileFrownLeft, OperationSignedPair, parameters.ParameterMouthCornerPullLeft, parameters.ParameterMouthCornerSlantLeft, parameters.ParameterMouthFrownLeft)
	derived(parameters.ParameterSmileFrown, OperationAverage, parameters.ParameterSmileFrownRight, parameters.ParameterSmileFrownLeft)
	derived(parameters.ParameterSmileSadRight, OperationSignedPair, parameters.ParameterMouthSmileRight, parameters.ParameterMouthSadRight)
	derived(parameters.ParameterSmileSadLeft, OperationSignedPair, parameters.ParameterMouthSmileLeft, parameters.ParameterMouthSadLeft)
	derived(parameters.ParameterSmileSad, OperationAverage, parameters.ParameterSmileSadRight, parameters.ParameterSmileSadLeft)
	derived(parameters.ParameterLipSuckUpper, OperationAverage, parameters.ParameterLipSuckUpperRight, parameters.ParameterLipSuckUpperLeft)
	derived(parameters.ParameterLipSuckLower, OperationAverage, parameters.ParameterLipSuckLowerRight, parameters.ParameterLipSuckLowerLeft)
	derived(parameters.ParameterLipSuck, OperationAverage, parameters.ParameterLipSuckUpper, parameters.ParameterLipSuckLower)
	derived(parameters.ParameterLipFunnelUpper, OperationAverage, parameters.ParameterLipFunnelUpperRight, parameters.ParameterLipFunnelUpperLeft)
	derived(parameters.ParameterLipFunnelLower, OperationAverage, parameters.ParameterLipFunnelLowerRight, parameters.ParameterLipFunnelLowerLeft)
	derived(parameters.ParameterLipFunnel, OperationAverage, parameters.ParameterLipFunnelUpper, parameters.ParameterLipFunnelLower)
	derived(parameters.ParameterLipPuckerUpper, OperationAverage, parameters.ParameterLipPuckerUpperRight, parameters.ParameterLipPuckerUpperLeft)
	derived(parameters.ParameterLipPuckerLower, OperationAverage, parameters.ParameterLipPuckerLowerRight, parameters.ParameterLipPuckerLowerLeft)
	derived(parameters.ParameterLipPucker, OperationAverage, parameters.ParameterLipPuckerUpper, parameters.ParameterLipPuckerLower)
	derived(parameters.ParameterNoseSneer, OperationAverage, parameters.ParameterNoseSneerRight, parameters.ParameterNoseSneerLeft)
	derived(parameters.ParameterCheekSquint, OperationAverage, parameters.ParameterCheekSquintRight, parameters.ParameterCheekSquintLeft)
	derived(parameters.ParameterCheekPuffSuck, OperationAverage, parameters.ParameterCheekPuffSuckRight, parameters.ParameterCheekPuffSuckLeft)

	return plans
}

func union(left, right Inputs) Inputs {
	left.Eye |= right.Eye
	left.Active |= right.Active
	for index := range left.Expressions.Words {
		left.Expressions.Words[index] |= right.Expressions.Words[index]
	}
	return left
}

func parameterIdentity(id parameters.ParameterID) string {
	if definition, ok := parameters.Definition(id); ok {
		return definition.OSCName
	}
	return fmt.Sprintf("id %d", id)
}
