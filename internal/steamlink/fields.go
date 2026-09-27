package steamlink

type rawID uint16

const (
	rawGazePoint rawID = iota
	rawEyesClosedL
	rawEyesClosedR
	rawUpperLidRaiserL
	rawUpperLidRaiserR
	rawLidTightenerL
	rawLidTightenerR
	rawInnerBrowRaiserL
	rawInnerBrowRaiserR
	rawOuterBrowRaiserL
	rawOuterBrowRaiserR
	rawBrowLowererL
	rawBrowLowererR
	rawNoseWrinklerL
	rawNoseWrinklerR
	rawCheekRaiserL
	rawCheekRaiserR
	rawCheekPuffL
	rawCheekPuffR
	rawCheekSuckL
	rawCheekSuckR
	rawJawDrop
	rawLipsToward
	rawJawSidewaysRight
	rawJawSidewaysLeft
	rawJawThrust
	rawMouthRight
	rawMouthLeft
	rawChinRaiserT
	rawChinRaiserB
	rawDimplerL
	rawDimplerR
	rawLipCornerPullerL
	rawLipCornerPullerR
	rawLipCornerDepressorL
	rawLipCornerDepressorR
	rawLowerLipDepressorL
	rawLowerLipDepressorR
	rawUpperLipRaiserL
	rawUpperLipRaiserR
	rawLipTightenerL
	rawLipTightenerR
	rawLipPressorL
	rawLipPressorR
	rawLipStretcherL
	rawLipStretcherR
	rawLipPuckerL
	rawLipPuckerR
	rawLipFunnelerLT
	rawLipFunnelerRT
	rawLipFunnelerLB
	rawLipFunnelerRB
	rawLipSuckLT
	rawLipSuckRT
	rawLipSuckLB
	rawLipSuckRB
	rawTongueOut
	rawCount
)
