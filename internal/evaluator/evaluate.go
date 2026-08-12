package evaluator

import (
	"math"

	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func (p *Plan) Evaluate(frame processing.CanonicalFrame) Snapshot {
	var floats [parameters.ParameterCount]float32
	var bools [parameters.ParameterCount]bool
	var floatValid [validityWordCount]uint64
	var boolValid [validityWordCount]uint64

	for _, current := range p.instructions {
		switch current.valueType {
		case parameters.ValueFloat:
			value, ok := evaluateFloat(current, &frame, &floats, &floatValid)
			if !ok {
				continue
			}
			floats[current.parameter] = value
			setValid(&floatValid, current.parameter)
		case parameters.ValueBool:
			value, ok := boolOperandValue(current.operands[0], &frame, &bools, &boolValid)
			if !ok {
				continue
			}
			bools[current.parameter] = value
			setValid(&boolValid, current.parameter)
		}
	}

	var snapshot Snapshot
	for id := parameters.ParameterID(0); id < parameters.ParameterCount; id++ {
		if !p.requested.has(id) {
			continue
		}
		switch parameters.Definitions[id].ValueType {
		case parameters.ValueFloat:
			if valid(&floatValid, id) {
				snapshot.floats[id] = floats[id]
				setValid(&snapshot.floatValid, id)
			}
		case parameters.ValueBool:
			if valid(&boolValid, id) {
				snapshot.bools[id] = bools[id]
				setValid(&snapshot.boolValid, id)
			}
		}
	}
	return snapshot
}

func evaluateFloat(
	current instruction,
	frame *processing.CanonicalFrame,
	floats *[parameters.ParameterCount]float32,
	floatValid *[validityWordCount]uint64,
) (float32, bool) {
	var result float32
	for index, currentOperand := range current.operands {
		value, ok := floatOperandValue(currentOperand, frame, floats, floatValid)
		if !ok || !finite(value) {
			return 0, false
		}

		switch current.operation {
		case parameterdeps.OperationDirect:
			result = value
		case parameterdeps.OperationAverage, parameterdeps.OperationSumClamp:
			result += value
		case parameterdeps.OperationMax:
			if index == 0 || value > result {
				result = value
			}
		case parameterdeps.OperationSignedPair:
			if index == len(current.operands)-1 {
				result -= value
			} else if index == 0 || value > result {
				result = value
			}
		}
	}

	if current.operation == parameterdeps.OperationAverage {
		result /= float32(len(current.operands))
	}
	if !finite(result) {
		return 0, false
	}
	result, ok := parameters.Clamp(current.parameter, result)
	if !ok || !finite(result) {
		return 0, false
	}
	return result, true
}

func floatOperandValue(
	current operand,
	frame *processing.CanonicalFrame,
	floats *[parameters.ParameterCount]float32,
	floatValid *[validityWordCount]uint64,
) (float32, bool) {
	switch current.kind {
	case operandEye:
		return eyeValue(frame.Eye, current.eye)
	case operandExpression:
		return frame.Expressions.Get(current.expression)
	case operandParameter:
		if !valid(floatValid, current.parameter) {
			return 0, false
		}
		return floats[current.parameter], true
	default:
		return 0, false
	}
}

func boolOperandValue(
	current operand,
	frame *processing.CanonicalFrame,
	bools *[parameters.ParameterCount]bool,
	boolValid *[validityWordCount]uint64,
) (bool, bool) {
	switch current.kind {
	case operandActive:
		switch current.active {
		case parameterdeps.ActiveStateEyeTracking:
			return frame.EyeActive, true
		case parameterdeps.ActiveStateExpressionTracking:
			return frame.ExpressionActive, true
		case parameterdeps.ActiveStateLipTracking:
			return frame.LipActive, true
		default:
			return false, false
		}
	case operandParameter:
		if !valid(boolValid, current.parameter) {
			return false, false
		}
		return bools[current.parameter], true
	default:
		return false, false
	}
}

func eyeValue(eye trackingmodel.EyeSample, field parameterdeps.EyeField) (float32, bool) {
	switch field {
	case parameterdeps.EyeFieldLeftGazeX:
		return eye.LeftGaze.X, eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	case parameterdeps.EyeFieldLeftGazeY:
		return eye.LeftGaze.Y, eye.Valid&trackingmodel.EyeValidLeftGaze != 0
	case parameterdeps.EyeFieldRightGazeX:
		return eye.RightGaze.X, eye.Valid&trackingmodel.EyeValidRightGaze != 0
	case parameterdeps.EyeFieldRightGazeY:
		return eye.RightGaze.Y, eye.Valid&trackingmodel.EyeValidRightGaze != 0
	case parameterdeps.EyeFieldLeftOpenness:
		return eye.LeftOpenness, eye.Valid&trackingmodel.EyeValidLeftOpenness != 0
	case parameterdeps.EyeFieldRightOpenness:
		return eye.RightOpenness, eye.Valid&trackingmodel.EyeValidRightOpenness != 0
	case parameterdeps.EyeFieldLeftPupilDiameter:
		return eye.LeftPupilDiameterMM, eye.Valid&trackingmodel.EyeValidLeftPupil != 0
	case parameterdeps.EyeFieldRightPupilDiameter:
		return eye.RightPupilDiameterMM, eye.Valid&trackingmodel.EyeValidRightPupil != 0
	case parameterdeps.EyeFieldLeftPupilDilation:
		return eye.LeftPupilDilation, eye.Valid&trackingmodel.EyeValidLeftPupil != 0
	case parameterdeps.EyeFieldRightPupilDilation:
		return eye.RightPupilDilation, eye.Valid&trackingmodel.EyeValidRightPupil != 0
	default:
		return 0, false
	}
}

func finite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
