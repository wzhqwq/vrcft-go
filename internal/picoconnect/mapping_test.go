package picoconnect

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestMapShapesGazeBlinkAndExpression(t *testing.T) {
	var packet shapePacket
	for _, slot := range []int{0, 2, 31, 44, 28, 38, 7} {
		packet.present[slot] = true
	}
	packet.values[2] = .6 // left eye looks inward = rightward
	packet.values[28] = .25
	packet.values[38] = .5
	packet.values[7] = .75
	sub := pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression}
	got := mapShapes(packet, sub)
	if got.Eye.Valid&(trackingmodel.EyeValidLeftGaze|trackingmodel.EyeValidLeftOpenness|trackingmodel.EyeValidRightOpenness) != trackingmodel.EyeValidLeftGaze|trackingmodel.EyeValidLeftOpenness|trackingmodel.EyeValidRightOpenness {
		t.Fatalf("eye valid = %b", got.Eye.Valid)
	}
	if got.Eye.LeftGaze.X != .6 || got.Eye.LeftOpenness != .75 || got.Eye.RightOpenness != .5 {
		t.Fatalf("eye = %+v", got.Eye)
	}
	if value, ok := got.Expressions.Get(trackingmodel.ExpressionJawOpen); !ok || value != .75 {
		t.Fatalf("jaw = %v, %t", value, ok)
	}
	if got.Eye.Valid&trackingmodel.EyeValidLeftPupil != 0 {
		t.Fatal("invented pupil")
	}
}

func TestMapShapesTrimsSubscription(t *testing.T) {
	var packet shapePacket
	packet.present[7], packet.present[51] = true, true
	packet.values[7], packet.values[51] = .9, .7
	sub := pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionTongueOut)}
	got := mapShapes(packet, sub)
	if got.Eye.Valid != 0 || got.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
		t.Fatalf("unsubscribed data: %+v", got)
	}
	if value, ok := got.Expressions.Get(trackingmodel.ExpressionTongueOut); !ok || value != .7 {
		t.Fatalf("tongue = %v, %t", value, ok)
	}
}

func TestMapShapesLipRollAndMouthShrug(t *testing.T) {
	var packet shapePacket
	for _, slot := range []int{shapeMouthRollUpper, shapeMouthRollLower, shapeMouthShrugUpper, shapeMouthShrugLower, shapeMouthUpperUpL} {
		packet.present[slot] = true
	}
	packet.values[shapeMouthRollUpper] = .2
	packet.values[shapeMouthRollLower] = .3
	packet.values[shapeMouthShrugUpper] = .4
	packet.values[shapeMouthShrugLower] = .5
	packet.values[shapeMouthUpperUpL] = .6
	frame := mapShapes(packet, pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression})
	for _, pair := range []struct {
		id   trackingmodel.ExpressionID
		want float32
	}{{trackingmodel.ExpressionLipSuckUpperLeft, .2}, {trackingmodel.ExpressionLipSuckUpperRight, .2}, {trackingmodel.ExpressionLipSuckLowerLeft, .3}, {trackingmodel.ExpressionLipSuckLowerRight, .3}, {trackingmodel.ExpressionMouthRaiserUpper, .4}, {trackingmodel.ExpressionMouthRaiserLower, .5}, {trackingmodel.ExpressionMouthUpperDeepenLeft, .6}} {
		got, ok := frame.Expressions.Get(pair.id)
		if !ok || got != pair.want {
			t.Fatalf("expression %d = %v,%t want %v", pair.id, got, ok, pair.want)
		}
	}
}
