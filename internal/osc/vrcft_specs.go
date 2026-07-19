package osc

// VRCFTParameterSpecs returns the public OSC parameters documented by VRCFaceTracking.
// Keys intentionally equal the matched OSC suffixes so a tracking snapshot can use the
// same stable identifiers without knowing the actual avatar prefix/address.
func VRCFTParameterSpecs() []ParameterSpec {
	unsigned := []string{
		"v2/EyeLidRight", "v2/EyeLidLeft", "v2/EyeLid",
		"v2/EyeSquintRight", "v2/EyeSquintLeft", "v2/EyeSquint",
		"v2/PupilDilation",
		"v2/BrowPinchRight", "v2/BrowPinchLeft",
		"v2/BrowLowererRight", "v2/BrowLowererLeft",
		"v2/BrowInnerUpRight", "v2/BrowInnerUpLeft",
		"v2/BrowOuterUpRight", "v2/BrowOuterUpLeft",
		"v2/NoseSneerRight", "v2/NoseSneerLeft",
		"v2/NasalDilationRight", "v2/NasalDilationLeft",
		"v2/NasalConstrictRight", "v2/NasalConstrictLeft",
		"v2/CheekSquintRight", "v2/CheekSquintLeft",
		"v2/JawOpen", "v2/MouthClosed", "v2/JawClench", "v2/JawMandibleRaise",
		"v2/LipSuckUpperRight", "v2/LipSuckUpperLeft",
		"v2/LipSuckLowerRight", "v2/LipSuckLowerLeft",
		"v2/LipSuckCornerRight", "v2/LipSuckCornerLeft",
		"v2/LipFunnelUpperRight", "v2/LipFunnelUpperLeft",
		"v2/LipFunnelLowerRight", "v2/LipFunnelLowerLeft",
		"v2/LipPuckerUpperRight", "v2/LipPuckerUpperLeft",
		"v2/LipPuckerLowerRight", "v2/LipPuckerLowerLeft",
		"v2/MouthUpperUpRight", "v2/MouthUpperUpLeft",
		"v2/MouthLowerDownRight", "v2/MouthLowerDownLeft",
		"v2/MouthUpperDeepenRight", "v2/MouthUpperDeepenLeft",
		"v2/MouthCornerPullRight", "v2/MouthCornerPullLeft",
		"v2/MouthCornerSlantRight", "v2/MouthCornerSlantLeft",
		"v2/MouthDimpleRight", "v2/MouthDimpleLeft",
		"v2/MouthFrownRight", "v2/MouthFrownLeft",
		"v2/MouthStretchRight", "v2/MouthStretchLeft",
		"v2/MouthRaiserUpper", "v2/MouthRaiserLower",
		"v2/MouthPressRight", "v2/MouthPressLeft",
		"v2/MouthTightenerRight", "v2/MouthTightenerLeft",
		"v2/TongueOut", "v2/TongueRoll", "v2/TongueTwistRight", "v2/TongueTwistLeft",
		"v2/SoftPalateClose", "v2/ThroatSwallow", "v2/NeckFlexRight", "v2/NeckFlexLeft",

		// Simplified parameters.
		"v2/BrowDownRight", "v2/BrowDownLeft", "v2/BrowOuterUp", "v2/BrowInnerUp", "v2/BrowUp",
		"v2/MouthUpperUp", "v2/MouthLowerDown", "v2/MouthOpen",
		"v2/MouthSmileRight", "v2/MouthSmileLeft",
		"v2/MouthSadRight", "v2/MouthSadLeft",
		"v2/LipSuckUpper", "v2/LipSuckLower", "v2/LipSuck",
		"v2/LipFunnelUpper", "v2/LipFunnelLower", "v2/LipFunnel",
		"v2/LipPuckerUpper", "v2/LipPuckerLower", "v2/LipPucker",
		"v2/NoseSneer", "v2/CheekSquint",
	}

	signed := []string{
		"v2/EyeLeftX", "v2/EyeLeftY", "v2/EyeRightX", "v2/EyeRightY",
		"v2/CheekPuffSuckRight", "v2/CheekPuffSuckLeft",
		"v2/JawX", "v2/JawZ",
		"v2/MouthUpperX", "v2/MouthLowerX",
		"v2/TongueX", "v2/TongueY", "v2/TongueArchY", "v2/TongueShape",

		// Simplified parameters.
		"v2/EyeX", "v2/EyeY",
		"v2/BrowExpressionRight", "v2/BrowExpressionLeft", "v2/BrowExpression",
		"v2/MouthX",
		"v2/SmileFrownRight", "v2/SmileFrownLeft", "v2/SmileFrown",
		"v2/SmileSadRight", "v2/SmileSadLeft", "v2/SmileSad",
		"v2/CheekPuffSuck",
	}

	unbounded := []string{
		"v2/PupilDiameterRight", "v2/PupilDiameterLeft", "v2/PupilDiameter",
	}

	result := make([]ParameterSpec, 0, len(unsigned)+len(signed)+len(unbounded)+3)
	for _, name := range unsigned {
		result = append(result, ParameterSpec{Key: name, Suffix: name, Class: ParameterFloat})
	}
	for _, name := range signed {
		result = append(result, ParameterSpec{Key: name, Suffix: name, Class: ParameterFloat, Signed: true})
	}
	for _, name := range unbounded {
		result = append(result, ParameterSpec{Key: name, Suffix: name, Class: ParameterFloat, Unbounded: true})
	}
	for _, name := range []string{"EyeTrackingActive", "ExpressionTrackingActive", "LipTrackingActive"} {
		result = append(result, ParameterSpec{Key: name, Suffix: name, Class: ParameterBool})
	}
	return result
}
