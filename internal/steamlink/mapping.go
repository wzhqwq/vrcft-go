package steamlink

import (
	"math"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const fieldTTL = 250 * time.Millisecond

type rawSample struct {
	Values     [3]float32
	ReceivedAt time.Time
	Seen       bool
}

type rawSnapshot [rawCount]rawSample
type rawSelection [rawCount]bool

type mappingOperation uint8

const (
	mappingCopy mappingOperation = iota
	mappingSubtract
)

type mappingRule struct {
	target    trackingmodel.ExpressionID
	sources   []rawID
	operation mappingOperation
}

var mappingRules = []mappingRule{
	{trackingmodel.ExpressionEyeSquintLeft, []rawID{rawLidTightenerL}, mappingCopy},
	{trackingmodel.ExpressionEyeSquintRight, []rawID{rawLidTightenerR}, mappingCopy},
	{trackingmodel.ExpressionBrowInnerUpLeft, []rawID{rawInnerBrowRaiserL}, mappingCopy},
	{trackingmodel.ExpressionBrowInnerUpRight, []rawID{rawInnerBrowRaiserR}, mappingCopy},
	{trackingmodel.ExpressionBrowOuterUpLeft, []rawID{rawOuterBrowRaiserL}, mappingCopy},
	{trackingmodel.ExpressionBrowOuterUpRight, []rawID{rawOuterBrowRaiserR}, mappingCopy},
	{trackingmodel.ExpressionBrowLowererLeft, []rawID{rawBrowLowererL}, mappingCopy},
	{trackingmodel.ExpressionBrowLowererRight, []rawID{rawBrowLowererR}, mappingCopy},
	{trackingmodel.ExpressionBrowPinchLeft, []rawID{rawBrowLowererL}, mappingCopy},
	{trackingmodel.ExpressionBrowPinchRight, []rawID{rawBrowLowererR}, mappingCopy},
	{trackingmodel.ExpressionNoseSneerLeft, []rawID{rawNoseWrinklerL}, mappingCopy},
	{trackingmodel.ExpressionNoseSneerRight, []rawID{rawNoseWrinklerR}, mappingCopy},
	{trackingmodel.ExpressionCheekSquintLeft, []rawID{rawCheekRaiserL}, mappingCopy},
	{trackingmodel.ExpressionCheekSquintRight, []rawID{rawCheekRaiserR}, mappingCopy},
	{trackingmodel.ExpressionCheekPuffSuckLeft, []rawID{rawCheekPuffL, rawCheekSuckL}, mappingSubtract},
	{trackingmodel.ExpressionCheekPuffSuckRight, []rawID{rawCheekPuffR, rawCheekSuckR}, mappingSubtract},
	{trackingmodel.ExpressionJawOpen, []rawID{rawJawDrop}, mappingCopy},
	{trackingmodel.ExpressionMouthClosed, []rawID{rawLipsToward}, mappingCopy},
	{trackingmodel.ExpressionJawX, []rawID{rawJawSidewaysRight, rawJawSidewaysLeft}, mappingSubtract},
	{trackingmodel.ExpressionJawZ, []rawID{rawJawThrust}, mappingCopy},
	{trackingmodel.ExpressionMouthUpperX, []rawID{rawMouthRight, rawMouthLeft}, mappingSubtract},
	{trackingmodel.ExpressionMouthLowerX, []rawID{rawMouthRight, rawMouthLeft}, mappingSubtract},
	{trackingmodel.ExpressionMouthRaiserUpper, []rawID{rawChinRaiserT}, mappingCopy},
	{trackingmodel.ExpressionMouthRaiserLower, []rawID{rawChinRaiserB}, mappingCopy},
	{trackingmodel.ExpressionMouthDimpleLeft, []rawID{rawDimplerL}, mappingCopy},
	{trackingmodel.ExpressionMouthDimpleRight, []rawID{rawDimplerR}, mappingCopy},
	{trackingmodel.ExpressionMouthCornerPullLeft, []rawID{rawLipCornerPullerL}, mappingCopy},
	{trackingmodel.ExpressionMouthCornerPullRight, []rawID{rawLipCornerPullerR}, mappingCopy},
	{trackingmodel.ExpressionMouthCornerSlantLeft, []rawID{rawLipCornerPullerL}, mappingCopy},
	{trackingmodel.ExpressionMouthCornerSlantRight, []rawID{rawLipCornerPullerR}, mappingCopy},
	{trackingmodel.ExpressionMouthFrownLeft, []rawID{rawLipCornerDepressorL}, mappingCopy},
	{trackingmodel.ExpressionMouthFrownRight, []rawID{rawLipCornerDepressorR}, mappingCopy},
	{trackingmodel.ExpressionMouthLowerDownLeft, []rawID{rawLowerLipDepressorL}, mappingCopy},
	{trackingmodel.ExpressionMouthLowerDownRight, []rawID{rawLowerLipDepressorR}, mappingCopy},
	{trackingmodel.ExpressionMouthUpperUpLeft, []rawID{rawUpperLipRaiserL}, mappingCopy},
	{trackingmodel.ExpressionMouthUpperUpRight, []rawID{rawUpperLipRaiserR}, mappingCopy},
	{trackingmodel.ExpressionMouthTightenerLeft, []rawID{rawLipTightenerL}, mappingCopy},
	{trackingmodel.ExpressionMouthTightenerRight, []rawID{rawLipTightenerR}, mappingCopy},
	{trackingmodel.ExpressionMouthPressLeft, []rawID{rawLipPressorL}, mappingCopy},
	{trackingmodel.ExpressionMouthPressRight, []rawID{rawLipPressorR}, mappingCopy},
	{trackingmodel.ExpressionMouthStretchLeft, []rawID{rawLipStretcherL}, mappingCopy},
	{trackingmodel.ExpressionMouthStretchRight, []rawID{rawLipStretcherR}, mappingCopy},
	{trackingmodel.ExpressionLipPuckerUpperLeft, []rawID{rawLipPuckerL}, mappingCopy},
	{trackingmodel.ExpressionLipPuckerLowerLeft, []rawID{rawLipPuckerL}, mappingCopy},
	{trackingmodel.ExpressionLipPuckerUpperRight, []rawID{rawLipPuckerR}, mappingCopy},
	{trackingmodel.ExpressionLipPuckerLowerRight, []rawID{rawLipPuckerR}, mappingCopy},
	{trackingmodel.ExpressionLipFunnelUpperLeft, []rawID{rawLipFunnelerLT}, mappingCopy},
	{trackingmodel.ExpressionLipFunnelUpperRight, []rawID{rawLipFunnelerRT}, mappingCopy},
	{trackingmodel.ExpressionLipFunnelLowerLeft, []rawID{rawLipFunnelerLB}, mappingCopy},
	{trackingmodel.ExpressionLipFunnelLowerRight, []rawID{rawLipFunnelerRB}, mappingCopy},
	{trackingmodel.ExpressionLipSuckUpperLeft, []rawID{rawLipSuckLT}, mappingCopy},
	{trackingmodel.ExpressionLipSuckUpperRight, []rawID{rawLipSuckRT}, mappingCopy},
	{trackingmodel.ExpressionLipSuckLowerLeft, []rawID{rawLipSuckLB}, mappingCopy},
	{trackingmodel.ExpressionLipSuckLowerRight, []rawID{rawLipSuckRB}, mappingCopy},
	{trackingmodel.ExpressionTongueOut, []rawID{rawTongueOut}, mappingCopy},
}

func requiredFields(sub pluginapi.Subscription) rawSelection {
	var required rawSelection
	if sub.IncludesEye(trackingmodel.EyeValidLeftGaze) || sub.IncludesEye(trackingmodel.EyeValidRightGaze) {
		required[rawGazePoint] = true
	}
	if sub.IncludesEye(trackingmodel.EyeValidLeftOpenness) {
		required[rawEyesClosedL] = true
		required[rawUpperLidRaiserL] = true
	}
	if sub.IncludesEye(trackingmodel.EyeValidRightOpenness) {
		required[rawEyesClosedR] = true
		required[rawUpperLidRaiserR] = true
	}
	for _, rule := range mappingRules {
		if sub.IncludesExpression(rule.target) {
			for _, source := range rule.sources {
				required[source] = true
			}
		}
	}
	return required
}

func mapSnapshot(raw rawSnapshot, sub pluginapi.Subscription, now time.Time) trackingmodel.TrackingFrame {
	frame := trackingmodel.TrackingFrame{Capabilities: sub.Capabilities & (trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression)}
	if fresh(raw[rawGazePoint], now) {
		gaze := normalizedGaze(raw[rawGazePoint].Values[0], raw[rawGazePoint].Values[1], raw[rawGazePoint].Values[2])
		frame.Eye.LeftGaze = gaze
		frame.Eye.RightGaze = gaze
		frame.Eye.Valid |= trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze
	}
	mapOpenness(&frame.Eye, raw, now, rawEyesClosedL, rawUpperLidRaiserL, trackingmodel.EyeValidLeftOpenness)
	mapOpenness(&frame.Eye, raw, now, rawEyesClosedR, rawUpperLidRaiserR, trackingmodel.EyeValidRightOpenness)
	for _, rule := range mappingRules {
		if !sub.IncludesExpression(rule.target) || !allFresh(raw, rule.sources, now) {
			continue
		}
		value := raw[rule.sources[0]].Values[0]
		if rule.operation == mappingSubtract {
			value -= raw[rule.sources[1]].Values[0]
		}
		frame.Expressions.Set(rule.target, value)
	}
	return sub.TrimFrame(frame)
}

func fresh(sample rawSample, now time.Time) bool {
	age := now.Sub(sample.ReceivedAt)
	return sample.Seen && age >= 0 && age < fieldTTL
}

func allFresh(raw rawSnapshot, sources []rawID, now time.Time) bool {
	for _, source := range sources {
		if !fresh(raw[source], now) {
			return false
		}
	}
	return true
}

func mapOpenness(eye *trackingmodel.EyeSample, raw rawSnapshot, now time.Time, closedID, wideID rawID, valid trackingmodel.EyeValid) {
	if !fresh(raw[closedID], now) {
		return
	}
	wide := float32(0)
	if fresh(raw[wideID], now) {
		wide = raw[wideID].Values[0]
	}
	if valid == trackingmodel.EyeValidLeftOpenness {
		eye.LeftOpenness = openness(raw[closedID].Values[0], wide)
	} else {
		eye.RightOpenness = openness(raw[closedID].Values[0], wide)
	}
	eye.Valid |= valid
}

func normalizedGaze(x, y, z float32) trackingmodel.Vec2 {
	clamp := func(value float64) float32 { return float32(math.Max(-1, math.Min(1, value))) }
	return trackingmodel.Vec2{
		X: clamp(math.Atan2(float64(x), -float64(z)) / (math.Pi / 4)),
		Y: clamp(math.Atan2(float64(y), -float64(z)) / (math.Pi / 4)),
	}
}

func openness(closed, wide float32) float32 { return (1 - closed) * (0.75 + 0.25*wide) }
