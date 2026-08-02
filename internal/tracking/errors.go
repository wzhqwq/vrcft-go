package tracking

import "errors"

var (
	ErrInvalidRouting        = errors.New("tracking: invalid routing")
	ErrGenerationUnset       = errors.New("tracking: generation is unset")
	ErrGenerationZero        = errors.New("tracking: generation must be positive")
	ErrGenerationRegression  = errors.New("tracking: generation regression")
	ErrStaleGeneration       = errors.New("tracking: stale generation")
	ErrFutureGeneration      = errors.New("tracking: future generation")
	ErrInvalidPluginID       = errors.New("tracking: invalid plugin ID")
	ErrInvalidFrame          = errors.New("tracking: invalid frame")
	ErrSequenceNotIncreasing = errors.New("tracking: sequence is not increasing")
	ErrTimestampRegression   = errors.New("tracking: timestamp regression")
	ErrSourceClockRegression = errors.New("tracking: source clock regression")
)
