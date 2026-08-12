package processing

import "errors"

var (
	ErrInvalidConfig          = errors.New("invalid processing config")
	ErrUnknownChannel         = errors.New("unknown processing channel")
	ErrInvalidCalibration     = errors.New("invalid channel calibration")
	ErrInvalidTuning          = errors.New("invalid channel tuning")
	ErrInvalidFilter          = errors.New("invalid channel filter")
	ErrInvalidDropout         = errors.New("invalid channel dropout policy")
	ErrInvalidMutualExclusion = errors.New("invalid mutual exclusion group")
	ErrInvalidInput           = errors.New("invalid processing input")
	ErrGenerationRegression   = errors.New("processing generation regression")
	ErrRevisionRegression     = errors.New("processing revision regression")
	ErrRevisionConflict       = errors.New("processing revision conflict")
	ErrTimeRegression         = errors.New("processing time regression")
)
