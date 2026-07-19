package trackingmodel

const MaxExpressionCount = 128

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

type EyeData struct {
	Valid EyeValid

	LeftGaze  Vec2
	RightGaze Vec2

	LeftOpenness  float32
	RightOpenness float32

	LeftPupilMM  float32
	RightPupilMM float32
}

type HeadValid uint8

const (
	HeadValidRotation HeadValid = 1 << iota
	HeadValidPosition
)

type HeadData struct {
	Valid HeadValid

	Rotation Vec3
	Position Vec3
}

type ExpressionData struct {
	Weights [MaxExpressionCount]float32

	Valid [2]uint64
}

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

type ExpressionID uint16

const (
	ExpressionEyeSquintRight ExpressionID = iota
	ExpressionEyeSquintLeft
	ExpressionEyeWideRight
	ExpressionEyeWideLeft

	ExpressionBrowPinchRight
	ExpressionBrowPinchLeft
	// ...

	ExpressionTongueTwistRight
	ExpressionTongueTwistLeft

	ExpressionSoftPalateClose
	ExpressionThroatSwallow
	ExpressionNeckFlexRight
	ExpressionNeckFlexLeft

	ExpressionCount
)

const expressionValidWords = (ExpressionCount + 63) / 64

type ExpressionSet struct {
	Values [ExpressionCount]float32
	Valid  [expressionValidWords]uint64
}
