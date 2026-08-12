package evaluator

import (
	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type operandKind uint8

const (
	operandEye operandKind = iota + 1
	operandExpression
	operandActive
	operandParameter
)

type operand struct {
	kind       operandKind
	eye        parameterdeps.EyeField
	expression trackingmodel.ExpressionID
	active     parameterdeps.ActiveState
	parameter  parameters.ParameterID
}
