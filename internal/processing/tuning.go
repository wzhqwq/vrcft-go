package processing

type ChannelTuning struct {
	Deadzone     float32
	Gain         float32
	Exponent     float32
	ClampEnabled bool
	ClampMin     float32
	ClampMax     float32
}
