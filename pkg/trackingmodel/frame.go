package trackingmodel

type Vec2 struct {
	X float32
	Y float32
}

type Vec3 struct {
	X float32
	Y float32
	Z float32
}

type EyeValid uint16

const (
	EyeValidLeftGaze EyeValid = 1 << iota
	EyeValidRightGaze
	EyeValidLeftOpenness
	EyeValidRightOpenness
	EyeValidLeftPupil
	EyeValidRightPupil
)

type TrackingFrame struct {
	Sequence      uint64
	TimestampNS   int64
	Capabilities  Capability
	SourceClockNS int64

	Eye EyeSample

	Expressions ExpressionSet
}

type EyeSample struct {
	Valid EyeValid

	LeftGaze  Vec2
	RightGaze Vec2

	LeftOpenness  float32
	RightOpenness float32

	LeftPupilDiameterMM  float32
	RightPupilDiameterMM float32

	LeftPupilDilation  float32
	RightPupilDilation float32
}
