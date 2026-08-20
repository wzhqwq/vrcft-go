package avatar

import "errors"

var (
	ErrInvalidPlannerConfig   = errors.New("avatar: invalid planner configuration")
	ErrInvalidAvatarID        = errors.New("avatar: invalid avatar ID")
	ErrConfigNotFound         = errors.New("avatar: configuration not found")
	ErrInvalidConfigPath      = errors.New("avatar: invalid configuration path")
	ErrConfigTooLarge         = errors.New("avatar: configuration too large")
	ErrInvalidJSON            = errors.New("avatar: invalid configuration JSON")
	ErrConfigIDMismatch       = errors.New("avatar: configuration ID mismatch")
	ErrTooManyParameters      = errors.New("avatar: too many parameters")
	ErrInvalidInputEndpoint   = errors.New("avatar: invalid input endpoint")
	ErrBindingCompilation     = errors.New("avatar: binding compilation failed")
	ErrRequirementCompilation = errors.New("avatar: requirement compilation failed")
	ErrGenerationExhausted    = errors.New("avatar: generation exhausted")
)
