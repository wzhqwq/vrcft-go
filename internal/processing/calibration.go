package processing

type ChannelCalibration struct {
	Neutral float32
	Min     float32
	Max     float32
	Gain    float32
	Invert  bool
}
