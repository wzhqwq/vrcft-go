package trackingmodel

type ExpressionID uint16

const (
	ExpressionEyeSquintRight ExpressionID = iota
	ExpressionEyeSquintLeft
	ExpressionBrowPinchRight
	ExpressionBrowPinchLeft
	ExpressionBrowLowererRight
	ExpressionBrowLowererLeft
	ExpressionBrowInnerUpRight
	ExpressionBrowInnerUpLeft
	ExpressionBrowOuterUpRight
	ExpressionBrowOuterUpLeft
	ExpressionNoseSneerRight
	ExpressionNoseSneerLeft
	ExpressionNasalDilationRight
	ExpressionNasalDilationLeft
	ExpressionNasalConstrictRight
	ExpressionNasalConstrictLeft
	ExpressionCheekSquintRight
	ExpressionCheekSquintLeft
	ExpressionCheekPuffSuckRight
	ExpressionCheekPuffSuckLeft
	ExpressionJawOpen
	ExpressionMouthClosed
	ExpressionJawX
	ExpressionJawZ
	ExpressionJawClench
	ExpressionJawMandibleRaise
	ExpressionLipSuckUpperRight
	ExpressionLipSuckUpperLeft
	ExpressionLipSuckLowerRight
	ExpressionLipSuckLowerLeft
	ExpressionLipSuckCornerRight
	ExpressionLipSuckCornerLeft
	ExpressionLipFunnelUpperRight
	ExpressionLipFunnelUpperLeft
	ExpressionLipFunnelLowerRight
	ExpressionLipFunnelLowerLeft
	ExpressionLipPuckerUpperRight
	ExpressionLipPuckerUpperLeft
	ExpressionLipPuckerLowerRight
	ExpressionLipPuckerLowerLeft
	ExpressionMouthUpperUpRight
	ExpressionMouthUpperUpLeft
	ExpressionMouthLowerDownRight
	ExpressionMouthLowerDownLeft
	ExpressionMouthUpperDeepenRight
	ExpressionMouthUpperDeepenLeft
	ExpressionMouthUpperX
	ExpressionMouthLowerX
	ExpressionMouthCornerPullRight
	ExpressionMouthCornerPullLeft
	ExpressionMouthCornerSlantRight
	ExpressionMouthCornerSlantLeft
	ExpressionMouthDimpleRight
	ExpressionMouthDimpleLeft
	ExpressionMouthFrownRight
	ExpressionMouthFrownLeft
	ExpressionMouthStretchRight
	ExpressionMouthStretchLeft
	ExpressionMouthRaiserUpper
	ExpressionMouthRaiserLower
	ExpressionMouthPressRight
	ExpressionMouthPressLeft
	ExpressionMouthTightenerRight
	ExpressionMouthTightenerLeft
	ExpressionTongueOut
	ExpressionTongueX
	ExpressionTongueY
	ExpressionTongueRoll
	ExpressionTongueArchY
	ExpressionTongueShape
	ExpressionTongueTwistRight
	ExpressionTongueTwistLeft
	ExpressionSoftPalateClose
	ExpressionThroatSwallow
	ExpressionNeckFlexRight
	ExpressionNeckFlexLeft
	ExpressionCount
)

var expressionNames = [ExpressionCount]string{
	"EyeSquintRight", "EyeSquintLeft",
	"BrowPinchRight", "BrowPinchLeft", "BrowLowererRight", "BrowLowererLeft", "BrowInnerUpRight", "BrowInnerUpLeft", "BrowOuterUpRight", "BrowOuterUpLeft",
	"NoseSneerRight", "NoseSneerLeft", "NasalDilationRight", "NasalDilationLeft", "NasalConstrictRight", "NasalConstrictLeft",
	"CheekSquintRight", "CheekSquintLeft", "CheekPuffSuckRight", "CheekPuffSuckLeft",
	"JawOpen", "MouthClosed", "JawX", "JawZ", "JawClench", "JawMandibleRaise",
	"LipSuckUpperRight", "LipSuckUpperLeft", "LipSuckLowerRight", "LipSuckLowerLeft", "LipSuckCornerRight", "LipSuckCornerLeft",
	"LipFunnelUpperRight", "LipFunnelUpperLeft", "LipFunnelLowerRight", "LipFunnelLowerLeft",
	"LipPuckerUpperRight", "LipPuckerUpperLeft", "LipPuckerLowerRight", "LipPuckerLowerLeft",
	"MouthUpperUpRight", "MouthUpperUpLeft", "MouthLowerDownRight", "MouthLowerDownLeft", "MouthUpperDeepenRight", "MouthUpperDeepenLeft", "MouthUpperX", "MouthLowerX", "MouthCornerPullRight", "MouthCornerPullLeft", "MouthCornerSlantRight", "MouthCornerSlantLeft", "MouthDimpleRight", "MouthDimpleLeft", "MouthFrownRight", "MouthFrownLeft", "MouthStretchRight", "MouthStretchLeft", "MouthRaiserUpper", "MouthRaiserLower", "MouthPressRight", "MouthPressLeft", "MouthTightenerRight", "MouthTightenerLeft",
	"TongueOut", "TongueX", "TongueY", "TongueRoll", "TongueArchY", "TongueShape", "TongueTwistRight", "TongueTwistLeft", "SoftPalateClose", "ThroatSwallow", "NeckFlexRight", "NeckFlexLeft",
}

func ExpressionNames() []string {
	names := make([]string, len(expressionNames))
	copy(names, expressionNames[:])
	return names
}

type ExpressionSet struct {
	Values [ExpressionCount]float32
	Valid  ExpressionMask
}

func (s *ExpressionSet) Set(id ExpressionID, value float32) bool {
	if id >= ExpressionCount {
		return false
	}
	s.Values[id] = value
	s.Valid.Set(id)
	return true
}

func (s ExpressionSet) Get(id ExpressionID) (float32, bool) {
	if id >= ExpressionCount || !s.Valid.Has(id) {
		return 0, false
	}
	return s.Values[id], true
}

func (s *ExpressionSet) Clear(id ExpressionID) bool {
	if id >= ExpressionCount {
		return false
	}
	s.Values[id] = 0
	s.Valid.Words[id/64] &^= uint64(1) << (id % 64)
	return true
}
