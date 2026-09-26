package steamlink

import (
	"math"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestMappingJawDifferenceRequiresBothInputs(t *testing.T) {
	now := time.Unix(100, 0)
	sub := pluginapi.Subscription{Generation: 1,
		Capabilities: trackingmodel.CapabilityExpression,
		Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawX)}
	var raw rawSnapshot
	raw[rawJawSidewaysRight] = rawSample{Values: [3]float32{0.8}, ReceivedAt: now, Seen: true}
	first := mapSnapshot(raw, sub, now)
	if first.Expressions.Valid.Has(trackingmodel.ExpressionJawX) {
		t.Fatal("missing input treated as zero")
	}
	raw[rawJawSidewaysLeft] = rawSample{Values: [3]float32{0.3}, ReceivedAt: now, Seen: true}
	second := mapSnapshot(raw, sub, now)
	value, valid := second.Expressions.Get(trackingmodel.ExpressionJawX)
	if !valid || math.Abs(float64(value-0.5)) > 1e-6 {
		t.Fatalf("got %v, %v", value, valid)
	}
}

func TestMappingEveryExpressionRule(t *testing.T) {
	now := time.Unix(100, 0)
	type source struct {
		id    rawID
		value float32
	}
	tests := []struct {
		name    string
		target  trackingmodel.ExpressionID
		sources []source
		want    float32
	}{
		{"left squint", trackingmodel.ExpressionEyeSquintLeft, []source{{rawLidTightenerL, .11}}, .11},
		{"right squint", trackingmodel.ExpressionEyeSquintRight, []source{{rawLidTightenerR, .12}}, .12},
		{"left inner brow", trackingmodel.ExpressionBrowInnerUpLeft, []source{{rawInnerBrowRaiserL, .13}}, .13},
		{"right inner brow", trackingmodel.ExpressionBrowInnerUpRight, []source{{rawInnerBrowRaiserR, .14}}, .14},
		{"left outer brow", trackingmodel.ExpressionBrowOuterUpLeft, []source{{rawOuterBrowRaiserL, .15}}, .15},
		{"right outer brow", trackingmodel.ExpressionBrowOuterUpRight, []source{{rawOuterBrowRaiserR, .16}}, .16},
		{"left brow lower", trackingmodel.ExpressionBrowLowererLeft, []source{{rawBrowLowererL, .17}}, .17},
		{"right brow lower", trackingmodel.ExpressionBrowLowererRight, []source{{rawBrowLowererR, .18}}, .18},
		{"left brow pinch", trackingmodel.ExpressionBrowPinchLeft, []source{{rawBrowLowererL, .19}}, .19},
		{"right brow pinch", trackingmodel.ExpressionBrowPinchRight, []source{{rawBrowLowererR, .20}}, .20},
		{"left nose", trackingmodel.ExpressionNoseSneerLeft, []source{{rawNoseWrinklerL, .21}}, .21},
		{"right nose", trackingmodel.ExpressionNoseSneerRight, []source{{rawNoseWrinklerR, .22}}, .22},
		{"left cheek", trackingmodel.ExpressionCheekSquintLeft, []source{{rawCheekRaiserL, .23}}, .23},
		{"right cheek", trackingmodel.ExpressionCheekSquintRight, []source{{rawCheekRaiserR, .24}}, .24},
		{"left cheek difference", trackingmodel.ExpressionCheekPuffSuckLeft, []source{{rawCheekPuffL, .75}, {rawCheekSuckL, .25}}, .50},
		{"right cheek difference", trackingmodel.ExpressionCheekPuffSuckRight, []source{{rawCheekPuffR, .20}, {rawCheekSuckR, .70}}, -.50},
		{"jaw open", trackingmodel.ExpressionJawOpen, []source{{rawJawDrop, .25}}, .25},
		{"mouth closed", trackingmodel.ExpressionMouthClosed, []source{{rawLipsToward, .26}}, .26},
		{"jaw x", trackingmodel.ExpressionJawX, []source{{rawJawSidewaysRight, .80}, {rawJawSidewaysLeft, .30}}, .50},
		{"jaw z", trackingmodel.ExpressionJawZ, []source{{rawJawThrust, .27}}, .27},
		{"mouth upper x", trackingmodel.ExpressionMouthUpperX, []source{{rawMouthRight, .90}, {rawMouthLeft, .40}}, .50},
		{"mouth lower x", trackingmodel.ExpressionMouthLowerX, []source{{rawMouthRight, .10}, {rawMouthLeft, .60}}, -.50},
		{"upper raiser", trackingmodel.ExpressionMouthRaiserUpper, []source{{rawChinRaiserT, .28}}, .28},
		{"lower raiser", trackingmodel.ExpressionMouthRaiserLower, []source{{rawChinRaiserB, .29}}, .29},
		{"left dimple", trackingmodel.ExpressionMouthDimpleLeft, []source{{rawDimplerL, .30}}, .30},
		{"right dimple", trackingmodel.ExpressionMouthDimpleRight, []source{{rawDimplerR, .31}}, .31},
		{"left corner pull", trackingmodel.ExpressionMouthCornerPullLeft, []source{{rawLipCornerPullerL, .32}}, .32},
		{"right corner pull", trackingmodel.ExpressionMouthCornerPullRight, []source{{rawLipCornerPullerR, .33}}, .33},
		{"left corner slant", trackingmodel.ExpressionMouthCornerSlantLeft, []source{{rawLipCornerPullerL, .34}}, .34},
		{"right corner slant", trackingmodel.ExpressionMouthCornerSlantRight, []source{{rawLipCornerPullerR, .35}}, .35},
		{"left frown", trackingmodel.ExpressionMouthFrownLeft, []source{{rawLipCornerDepressorL, .36}}, .36},
		{"right frown", trackingmodel.ExpressionMouthFrownRight, []source{{rawLipCornerDepressorR, .37}}, .37},
		{"left lower lip", trackingmodel.ExpressionMouthLowerDownLeft, []source{{rawLowerLipDepressorL, .38}}, .38},
		{"right lower lip", trackingmodel.ExpressionMouthLowerDownRight, []source{{rawLowerLipDepressorR, .39}}, .39},
		{"left upper lip", trackingmodel.ExpressionMouthUpperUpLeft, []source{{rawUpperLipRaiserL, .40}}, .40},
		{"right upper lip", trackingmodel.ExpressionMouthUpperUpRight, []source{{rawUpperLipRaiserR, .41}}, .41},
		{"left tightener", trackingmodel.ExpressionMouthTightenerLeft, []source{{rawLipTightenerL, .42}}, .42},
		{"right tightener", trackingmodel.ExpressionMouthTightenerRight, []source{{rawLipTightenerR, .43}}, .43},
		{"left press", trackingmodel.ExpressionMouthPressLeft, []source{{rawLipPressorL, .44}}, .44},
		{"right press", trackingmodel.ExpressionMouthPressRight, []source{{rawLipPressorR, .45}}, .45},
		{"left stretch", trackingmodel.ExpressionMouthStretchLeft, []source{{rawLipStretcherL, .46}}, .46},
		{"right stretch", trackingmodel.ExpressionMouthStretchRight, []source{{rawLipStretcherR, .47}}, .47},
		{"left pucker upper", trackingmodel.ExpressionLipPuckerUpperLeft, []source{{rawLipPuckerL, .48}}, .48},
		{"left pucker lower", trackingmodel.ExpressionLipPuckerLowerLeft, []source{{rawLipPuckerL, .49}}, .49},
		{"right pucker upper", trackingmodel.ExpressionLipPuckerUpperRight, []source{{rawLipPuckerR, .50}}, .50},
		{"right pucker lower", trackingmodel.ExpressionLipPuckerLowerRight, []source{{rawLipPuckerR, .51}}, .51},
		{"funnel upper left", trackingmodel.ExpressionLipFunnelUpperLeft, []source{{rawLipFunnelerLT, .52}}, .52},
		{"funnel upper right", trackingmodel.ExpressionLipFunnelUpperRight, []source{{rawLipFunnelerRT, .53}}, .53},
		{"funnel lower left", trackingmodel.ExpressionLipFunnelLowerLeft, []source{{rawLipFunnelerLB, .54}}, .54},
		{"funnel lower right", trackingmodel.ExpressionLipFunnelLowerRight, []source{{rawLipFunnelerRB, .55}}, .55},
		{"suck upper left", trackingmodel.ExpressionLipSuckUpperLeft, []source{{rawLipSuckLT, .56}}, .56},
		{"suck upper right", trackingmodel.ExpressionLipSuckUpperRight, []source{{rawLipSuckRT, .57}}, .57},
		{"suck lower left", trackingmodel.ExpressionLipSuckLowerLeft, []source{{rawLipSuckLB, .58}}, .58},
		{"suck lower right", trackingmodel.ExpressionLipSuckLowerRight, []source{{rawLipSuckRB, .59}}, .59},
		{"tongue", trackingmodel.ExpressionTongueOut, []source{{rawTongueOut, .60}}, .60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw rawSnapshot
			for _, source := range tt.sources {
				raw[source.id] = rawSample{Values: [3]float32{source.value}, ReceivedAt: now, Seen: true}
			}
			frame := mapSnapshot(raw, expressionSubscription(tt.target), now)
			got, ok := frame.Expressions.Get(tt.target)
			if !ok || math.Abs(float64(got-tt.want)) > 1e-6 {
				t.Fatalf("%v, %v; want %v, true", got, ok, tt.want)
			}
		})
	}
}

func TestMappingFanOutSharesRawValues(t *testing.T) {
	now := time.Unix(100, 0)
	tests := []struct {
		name    string
		source  rawID
		targets []trackingmodel.ExpressionID
	}{
		{"brow", rawBrowLowererL, []trackingmodel.ExpressionID{trackingmodel.ExpressionBrowLowererLeft, trackingmodel.ExpressionBrowPinchLeft}},
		{"corner", rawLipCornerPullerR, []trackingmodel.ExpressionID{trackingmodel.ExpressionMouthCornerPullRight, trackingmodel.ExpressionMouthCornerSlantRight}},
		{"pucker", rawLipPuckerL, []trackingmodel.ExpressionID{trackingmodel.ExpressionLipPuckerUpperLeft, trackingmodel.ExpressionLipPuckerLowerLeft}},
		{"mouth x", rawMouthRight, []trackingmodel.ExpressionID{trackingmodel.ExpressionMouthUpperX, trackingmodel.ExpressionMouthLowerX}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw rawSnapshot
			raw[tt.source] = rawSample{Values: [3]float32{.6}, ReceivedAt: now, Seen: true}
			if tt.source == rawMouthRight {
				raw[rawMouthLeft] = rawSample{Values: [3]float32{.1}, ReceivedAt: now, Seen: true}
			}
			frame := mapSnapshot(raw, pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionMaskOf(tt.targets...)}, now)
			for _, target := range tt.targets {
				want := float32(.6)
				if tt.source == rawMouthRight {
					want = .5
				}
				if got, ok := frame.Expressions.Get(target); !ok || got != want {
					t.Fatalf("%v = %v, %v; want %v, true", target, got, ok, want)
				}
			}
		})
	}
}

func TestMappingFreshnessAndUnsupportedTargets(t *testing.T) {
	now := time.Unix(100, 0)
	sub := expressionSubscription(trackingmodel.ExpressionJawOpen, trackingmodel.ExpressionTongueRoll)
	var raw rawSnapshot
	raw[rawJawDrop] = rawSample{Values: [3]float32{0}, ReceivedAt: now, Seen: true}
	frame := mapSnapshot(raw, sub, now)
	if value, ok := frame.Expressions.Get(trackingmodel.ExpressionJawOpen); !ok || value != 0 {
		t.Fatalf("valid zero = %v, %v", value, ok)
	}
	if frame.Expressions.Valid.Has(trackingmodel.ExpressionTongueRoll) {
		t.Fatal("unsupported target is valid")
	}
	raw[rawJawDrop].ReceivedAt = now.Add(-fieldTTL)
	if mapSnapshot(raw, sub, now).Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
		t.Fatal("expired input is valid")
	}
	raw[rawJawDrop].ReceivedAt = now.Add(time.Millisecond)
	if mapSnapshot(raw, sub, now).Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
		t.Fatal("future input is valid")
	}
}

func TestMappingEyeGazeAndLids(t *testing.T) {
	now := time.Unix(100, 0)
	sub := pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye}
	var raw rawSnapshot
	raw[rawGazePoint] = rawSample{Values: [3]float32{0, 0, -1}, ReceivedAt: now, Seen: true}
	forward := mapSnapshot(raw, sub, now)
	if forward.Eye.LeftGaze != (trackingmodel.Vec2{}) || forward.Eye.RightGaze != (trackingmodel.Vec2{}) || forward.Eye.Valid&(trackingmodel.EyeValidLeftGaze|trackingmodel.EyeValidRightGaze) == 0 {
		t.Fatalf("forward gaze = %+v", forward.Eye)
	}
	raw[rawGazePoint] = rawSample{Values: [3]float32{1, -1, -1}, ReceivedAt: now, Seen: true}
	raw[rawEyesClosedL] = rawSample{Values: [3]float32{.2}, ReceivedAt: now, Seen: true}
	raw[rawUpperLidRaiserL] = rawSample{Values: [3]float32{1}, ReceivedAt: now, Seen: true}
	raw[rawEyesClosedR] = rawSample{Values: [3]float32{.4}, ReceivedAt: now, Seen: true}
	frame := mapSnapshot(raw, sub, now)
	if frame.Eye.LeftGaze != (trackingmodel.Vec2{X: 1, Y: -1}) || frame.Eye.RightGaze != frame.Eye.LeftGaze {
		t.Fatalf("shared +/-45 gaze = %+v, %+v", frame.Eye.LeftGaze, frame.Eye.RightGaze)
	}
	if !floatClose(frame.Eye.LeftOpenness, .8) || !floatClose(frame.Eye.RightOpenness, .45) {
		t.Fatalf("lids = %v, %v", frame.Eye.LeftOpenness, frame.Eye.RightOpenness)
	}
	if frame.Eye.Valid&(trackingmodel.EyeValidLeftPupil|trackingmodel.EyeValidRightPupil) != 0 {
		t.Fatal("pupils synthesized")
	}
	raw[rawGazePoint].Values = [3]float32{10, -10, -1}
	frame = mapSnapshot(raw, sub, now)
	if frame.Eye.LeftGaze != (trackingmodel.Vec2{X: 1, Y: -1}) {
		t.Fatalf("saturated gaze = %+v", frame.Eye.LeftGaze)
	}
	raw[rawUpperLidRaiserL].ReceivedAt = now.Add(-fieldTTL)
	frame = mapSnapshot(raw, sub, now)
	if frame.Eye.LeftOpenness != .6 || frame.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 {
		t.Fatalf("expired wide = %v, %v", frame.Eye.LeftOpenness, frame.Eye.Valid)
	}
	raw[rawEyesClosedR].ReceivedAt = now.Add(-fieldTTL)
	frame = mapSnapshot(raw, sub, now)
	if frame.Eye.Valid&trackingmodel.EyeValidRightOpenness != 0 {
		t.Fatal("expired closed eye is valid")
	}
	var wideOnly rawSnapshot
	wideOnly[rawUpperLidRaiserL] = rawSample{Values: [3]float32{1}, ReceivedAt: now, Seen: true}
	if mapSnapshot(wideOnly, sub, now).Eye.Valid&trackingmodel.EyeValidLeftOpenness != 0 {
		t.Fatal("wide-only eyelid is valid")
	}
}

func TestRequiredFieldsReflectSubscription(t *testing.T) {
	tests := []struct {
		name string
		sub  pluginapi.Subscription
		want []rawID
	}{
		{"eye left lid", pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye, Eye: trackingmodel.EyeValidLeftOpenness}, []rawID{rawEyesClosedL, rawUpperLidRaiserL}},
		{"jaw x", expressionSubscription(trackingmodel.ExpressionJawX), []rawID{rawJawSidewaysRight, rawJawSidewaysLeft}},
		{"expression only", expressionSubscription(trackingmodel.ExpressionJawOpen), []rawID{rawJawDrop}},
		{"lip only", pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityLip}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requiredFields(tt.sub)
			for id := rawID(0); id < rawCount; id++ {
				want := false
				for _, expected := range tt.want {
					if id == expected {
						want = true
					}
				}
				if got[id] != want {
					t.Fatalf("required[%d] = %v, want %v", id, got[id], want)
				}
			}
		})
	}
	full := requiredFields(pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression})
	for id := rawID(0); id < rawCount; id++ {
		if !full[id] {
			t.Fatalf("full-group selection omitted %d", id)
		}
	}
}

func TestMappingLipOnlyProducesNoNumericOutput(t *testing.T) {
	now := time.Unix(100, 0)
	var raw rawSnapshot
	raw[rawGazePoint] = rawSample{Values: [3]float32{0, 0, -1}, ReceivedAt: now, Seen: true}
	raw[rawJawDrop] = rawSample{Values: [3]float32{.5}, ReceivedAt: now, Seen: true}
	frame := mapSnapshot(raw, pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityLip}, now)
	if frame.Capabilities != 0 || frame.Eye.Valid != 0 || !frame.Expressions.Valid.IsZero() {
		t.Fatalf("lip-only frame = %+v", frame)
	}
}

func expressionSubscription(ids ...trackingmodel.ExpressionID) pluginapi.Subscription {
	return pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionMaskOf(ids...)}
}

func floatClose(got, want float32) bool { return math.Abs(float64(got-want)) < 1e-6 }
