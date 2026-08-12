package evaluator

import "errors"

var (
	ErrUnknownParameter = errors.New("unknown parameter")
	ErrMissingPlan      = errors.New("missing parameter plan")
	ErrDependencyCycle  = errors.New("parameter dependency cycle")
	ErrInvalidOperation = errors.New("invalid parameter operation")
)
