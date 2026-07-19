package parameters

const FloatParameterCount = 124

type ParameterID uint16

type ParameterSnapshot struct {
	Sequence    uint64
	TimestampNS int64

	Values [FloatParameterCount]float32
	Valid  [2]uint64

	Active TrackingActiveState
}

type TrackingActiveState struct {
	Eye        bool
	Expression bool
	Lip        bool
}
