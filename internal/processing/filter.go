package processing

type FilterMode string

const (
	FilterNone    FilterMode = "none"
	FilterEMA     FilterMode = "ema"
	FilterOneEuro FilterMode = "one_euro"
)

// FilterConfig contains the parameters for one supported filter mode.
type FilterConfig struct {
	Mode             FilterMode
	EMAAlpha         float32
	MinCutoff        float32
	Beta             float32
	DerivativeCutoff float32
}
