package picoconnect

import (
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

// Slot indices follow PICO's 52-shape stream order as documented by
// UnifiedPicoModule/Core/Contracts.cs. The mapping uses only measured slots.
const (
	shapeEyeDownL        = 0
	shapeNoseSneerL      = 1
	shapeEyeInL          = 2
	shapeBrowInner       = 3
	shapeBrowDownR       = 4
	shapeMouthClose      = 5
	shapeMouthLowerDownR = 6
	shapeJawOpen         = 7
	shapeMouthUpperUpR   = 8
	shapeMouthShrugUpper = 9
	shapeMouthFunnel     = 10
	shapeEyeInR          = 11
	shapeEyeDownR        = 12
	shapeNoseSneerR      = 13
	shapeMouthRollUpper  = 14
	shapeJawRight        = 15
	shapeBrowDownL       = 16
	shapeMouthShrugLower = 17
	shapeMouthRollLower  = 18
	shapeMouthSmileL     = 19
	shapeMouthPressL     = 20
	shapeMouthSmileR     = 21
	shapeMouthPressR     = 22
	shapeMouthDimpleR    = 23
	shapeMouthLeft       = 24
	shapeJawForward      = 25
	shapeEyeSquintL      = 26
	shapeMouthFrownL     = 27
	shapeEyeBlinkL       = 28
	shapeCheekSquintL    = 29
	shapeBrowOuterL      = 30
	shapeEyeUpL          = 31
	shapeJawLeft         = 32
	shapeMouthStretchL   = 33
	shapeMouthPucker     = 34
	shapeEyeUpR          = 35
	shapeBrowOuterR      = 36
	shapeCheekSquintR    = 37
	shapeEyeBlinkR       = 38
	shapeMouthUpperUpL   = 39
	shapeMouthFrownR     = 40
	shapeEyeSquintR      = 41
	shapeMouthStretchR   = 42
	shapeCheekPuff       = 43
	shapeEyeOutL         = 44
	shapeEyeOutR         = 45
	shapeEyeWideR        = 46
	shapeEyeWideL        = 47
	shapeMouthRight      = 48
	shapeMouthDimpleL    = 49
	shapeMouthLowerDownL = 50
	shapeTongueOut       = 51
)

type shapeRule struct {
	slot   int
	target trackingmodel.ExpressionID
}

var directShapeRules = [...]shapeRule{
	{shapeNoseSneerL, trackingmodel.ExpressionNoseSneerLeft},
	{shapeNoseSneerR, trackingmodel.ExpressionNoseSneerRight},
	{shapeBrowDownL, trackingmodel.ExpressionBrowLowererLeft},
	{shapeBrowDownR, trackingmodel.ExpressionBrowLowererRight},
	{shapeBrowDownL, trackingmodel.ExpressionBrowPinchLeft},
	{shapeBrowDownR, trackingmodel.ExpressionBrowPinchRight},
	{shapeBrowInner, trackingmodel.ExpressionBrowInnerUpLeft},
	{shapeBrowInner, trackingmodel.ExpressionBrowInnerUpRight},
	{shapeBrowOuterL, trackingmodel.ExpressionBrowOuterUpLeft},
	{shapeBrowOuterR, trackingmodel.ExpressionBrowOuterUpRight},
	{shapeEyeSquintL, trackingmodel.ExpressionEyeSquintLeft},
	{shapeEyeSquintR, trackingmodel.ExpressionEyeSquintRight},
	{shapeCheekSquintL, trackingmodel.ExpressionCheekSquintLeft},
	{shapeCheekSquintR, trackingmodel.ExpressionCheekSquintRight},
	{shapeJawOpen, trackingmodel.ExpressionJawOpen},
	{shapeJawForward, trackingmodel.ExpressionJawZ},
	{shapeMouthClose, trackingmodel.ExpressionMouthClosed},
	{shapeMouthLowerDownL, trackingmodel.ExpressionMouthLowerDownLeft},
	{shapeMouthLowerDownR, trackingmodel.ExpressionMouthLowerDownRight},
	{shapeMouthUpperUpL, trackingmodel.ExpressionMouthUpperUpLeft},
	{shapeMouthUpperUpR, trackingmodel.ExpressionMouthUpperUpRight},
	{shapeMouthUpperUpL, trackingmodel.ExpressionMouthUpperDeepenLeft},
	{shapeMouthUpperUpR, trackingmodel.ExpressionMouthUpperDeepenRight},
	{shapeMouthRollUpper, trackingmodel.ExpressionLipSuckUpperLeft},
	{shapeMouthRollUpper, trackingmodel.ExpressionLipSuckUpperRight},
	{shapeMouthRollLower, trackingmodel.ExpressionLipSuckLowerLeft},
	{shapeMouthRollLower, trackingmodel.ExpressionLipSuckLowerRight},
	{shapeMouthShrugUpper, trackingmodel.ExpressionMouthRaiserUpper},
	{shapeMouthShrugLower, trackingmodel.ExpressionMouthRaiserLower},
	{shapeMouthSmileL, trackingmodel.ExpressionMouthCornerPullLeft},
	{shapeMouthSmileR, trackingmodel.ExpressionMouthCornerPullRight},
	{shapeMouthSmileL, trackingmodel.ExpressionMouthCornerSlantLeft},
	{shapeMouthSmileR, trackingmodel.ExpressionMouthCornerSlantRight},
	{shapeMouthPressL, trackingmodel.ExpressionMouthPressLeft},
	{shapeMouthPressR, trackingmodel.ExpressionMouthPressRight},
	{shapeMouthDimpleL, trackingmodel.ExpressionMouthDimpleLeft},
	{shapeMouthDimpleR, trackingmodel.ExpressionMouthDimpleRight},
	{shapeMouthFrownL, trackingmodel.ExpressionMouthFrownLeft},
	{shapeMouthFrownR, trackingmodel.ExpressionMouthFrownRight},
	{shapeMouthStretchL, trackingmodel.ExpressionMouthStretchLeft},
	{shapeMouthStretchR, trackingmodel.ExpressionMouthStretchRight},
	{shapeTongueOut, trackingmodel.ExpressionTongueOut},
}

func clamp(value, low, high float32) float32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func eyeOpenness(packet shapePacket, blinkSlot, wideSlot int) float32 {
	wide := float32(0)
	if packet.present[wideSlot] {
		wide = clamp(packet.values[wideSlot], 0, 1)
	}
	return (1 - clamp(packet.values[blinkSlot], 0, 1)) * (0.75 + 0.25*wide)
}

func mapShapes(packet shapePacket, sub pluginapi.Subscription) trackingmodel.TrackingFrame {
	frame := trackingmodel.TrackingFrame{Capabilities: sub.Capabilities & (trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression)}
	if frame.Capabilities.Has(trackingmodel.CapabilityEye) {
		if packet.present[shapeEyeInL] && packet.present[shapeEyeOutL] && packet.present[shapeEyeUpL] && packet.present[shapeEyeDownL] {
			frame.Eye.LeftGaze = trackingmodel.Vec2{X: clamp(packet.values[shapeEyeInL]-packet.values[shapeEyeOutL], -1, 1), Y: clamp(packet.values[shapeEyeUpL]-packet.values[shapeEyeDownL], -1, 1)}
			frame.Eye.Valid |= trackingmodel.EyeValidLeftGaze
		}
		if packet.present[shapeEyeInR] && packet.present[shapeEyeOutR] && packet.present[shapeEyeUpR] && packet.present[shapeEyeDownR] {
			frame.Eye.RightGaze = trackingmodel.Vec2{X: clamp(packet.values[shapeEyeOutR]-packet.values[shapeEyeInR], -1, 1), Y: clamp(packet.values[shapeEyeUpR]-packet.values[shapeEyeDownR], -1, 1)}
			frame.Eye.Valid |= trackingmodel.EyeValidRightGaze
		}
		if packet.present[shapeEyeBlinkL] {
			frame.Eye.LeftOpenness = eyeOpenness(packet, shapeEyeBlinkL, shapeEyeWideL)
			frame.Eye.Valid |= trackingmodel.EyeValidLeftOpenness
		}
		if packet.present[shapeEyeBlinkR] {
			frame.Eye.RightOpenness = eyeOpenness(packet, shapeEyeBlinkR, shapeEyeWideR)
			frame.Eye.Valid |= trackingmodel.EyeValidRightOpenness
		}
	}
	if frame.Capabilities.Has(trackingmodel.CapabilityExpression) {
		for _, rule := range directShapeRules {
			if packet.present[rule.slot] {
				frame.Expressions.Set(rule.target, clamp(packet.values[rule.slot], 0, 1))
			}
		}
		if packet.present[shapeJawRight] && packet.present[shapeJawLeft] {
			frame.Expressions.Set(trackingmodel.ExpressionJawX, clamp(packet.values[shapeJawRight]-packet.values[shapeJawLeft], -1, 1))
		}
		if packet.present[shapeMouthRight] && packet.present[shapeMouthLeft] {
			value := clamp(packet.values[shapeMouthRight]-packet.values[shapeMouthLeft], -1, 1)
			frame.Expressions.Set(trackingmodel.ExpressionMouthUpperX, value)
			frame.Expressions.Set(trackingmodel.ExpressionMouthLowerX, value)
		}
		if packet.present[shapeCheekPuff] {
			value := clamp(packet.values[shapeCheekPuff], 0, 1)
			frame.Expressions.Set(trackingmodel.ExpressionCheekPuffSuckLeft, value)
			frame.Expressions.Set(trackingmodel.ExpressionCheekPuffSuckRight, value)
		}
		if packet.present[shapeMouthPucker] {
			value := clamp(packet.values[shapeMouthPucker], 0, 1)
			for _, id := range []trackingmodel.ExpressionID{trackingmodel.ExpressionLipPuckerUpperLeft, trackingmodel.ExpressionLipPuckerUpperRight, trackingmodel.ExpressionLipPuckerLowerLeft, trackingmodel.ExpressionLipPuckerLowerRight} {
				frame.Expressions.Set(id, value)
			}
		}
		if packet.present[shapeMouthFunnel] {
			value := clamp(packet.values[shapeMouthFunnel], 0, 1)
			for _, id := range []trackingmodel.ExpressionID{trackingmodel.ExpressionLipFunnelUpperLeft, trackingmodel.ExpressionLipFunnelUpperRight, trackingmodel.ExpressionLipFunnelLowerLeft, trackingmodel.ExpressionLipFunnelLowerRight} {
				frame.Expressions.Set(id, value)
			}
		}
	}
	return sub.TrimFrame(frame)
}
