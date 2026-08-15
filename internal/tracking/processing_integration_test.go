package tracking_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestSelectedNumericUpdatesRemainFreshAcrossNonadvancingHostClock(t *testing.T) {
	tests := []struct {
		name            string
		secondEye       float32
		secondJaw       float32
		finalNowNS      int64
		wantEye         float32
		wantJaw         float32
		wantGroupActive bool
	}{
		{
			name:            "changed values cross the tracking processing boundary",
			secondEye:       0.25,
			secondJaw:       0.125,
			finalNowNS:      102,
			wantEye:         0.25,
			wantJaw:         0.125,
			wantGroupActive: true,
		},
		{
			name:            "same values renew downstream freshness",
			secondEye:       0.75,
			secondJaw:       0.625,
			finalNowNS:      103,
			wantEye:         0.75,
			wantJaw:         0.625,
			wantGroupActive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostTimes := []int64{100, 100, 90, 80, 70}
			clockIndex := 0
			service := tracking.NewServiceWithClockForTest(func() int64 {
				value := hostTimes[clockIndex]
				clockIndex++
				return value
			})
			if err := service.SetGeneration(1); err != nil {
				t.Fatal(err)
			}

			config := processing.DefaultConfig()
			config.ActiveStaleAfter = 2 * time.Nanosecond
			config.DefaultChannel.Dropout = processing.DropoutPolicy{
				StaleAfter: 2 * time.Nanosecond,
			}
			pipeline, err := processing.NewPipeline(config)
			if err != nil {
				t.Fatal(err)
			}

			if err := service.Submit("vendor.combined", 1, numericFrame(1, 0.75, 0.625)); err != nil {
				t.Fatal(err)
			}
			first, ok := service.LatestMerged()
			if !ok {
				t.Fatal("first selected update did not publish a merged frame")
			}
			if _, err := pipeline.ProcessAt(first, 101); err != nil {
				t.Fatal(err)
			}

			if err := service.Submit("vendor.combined", 1, numericFrame(2, tt.secondEye, tt.secondJaw)); err != nil {
				t.Fatal(err)
			}
			second, ok := service.LatestMerged()
			if !ok {
				t.Fatal("second selected update did not publish a merged frame")
			}
			if second.EyeUpdatedAtNS <= first.EyeUpdatedAtNS || second.ExpressionUpdatedAtNS <= first.ExpressionUpdatedAtNS {
				t.Fatalf("selected group freshness did not advance: first=(%d,%d) second=(%d,%d)",
					first.EyeUpdatedAtNS, first.ExpressionUpdatedAtNS,
					second.EyeUpdatedAtNS, second.ExpressionUpdatedAtNS)
			}
			got, err := pipeline.ProcessAt(second, 102)
			if err != nil {
				t.Fatal(err)
			}
			if tt.finalNowNS != 102 {
				got, err = pipeline.ProcessAt(second, tt.finalNowNS)
				if err != nil {
					t.Fatal(err)
				}
			}

			jaw, jawValid := got.Expressions.Get(trackingmodel.ExpressionJawOpen)
			if got.Eye.Valid&trackingmodel.EyeValidLeftOpenness == 0 || got.Eye.LeftOpenness != tt.wantEye ||
				!jawValid || jaw != tt.wantJaw || got.EyeActive != tt.wantGroupActive || got.ExpressionActive != tt.wantGroupActive {
				t.Fatalf("processed selected update = %#v, jaw=%v,%t; want Eye=%v Jaw=%v active=%t",
					got, jaw, jawValid, tt.wantEye, tt.wantJaw, tt.wantGroupActive)
			}
		})
	}
}

func TestSelectedNumericUpdateIsRejectedAtHostTimeSaturation(t *testing.T) {
	tests := []struct {
		name     string
		thirdEye float32
		thirdJaw float32
	}{
		{name: "changed values are not accepted invisibly", thirdEye: 0.25, thirdJaw: 0.125},
		{name: "same values are not accepted as a false refresh", thirdEye: 0.75, thirdJaw: 0.625},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := tracking.NewServiceWithClockForTest(func() int64 { return math.MaxInt64 - 2 })
			if err := service.SetGeneration(1); err != nil {
				t.Fatal(err)
			}
			pipeline, err := processing.NewPipeline(processing.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}

			if err := service.Submit("vendor.combined", 1, numericFrame(1, 0.5, 0.375)); err != nil {
				t.Fatal(err)
			}
			first, _ := service.LatestMerged()
			if _, err := pipeline.ProcessAt(first, math.MaxInt64-1); err != nil {
				t.Fatal(err)
			}
			if err := service.Submit("vendor.combined", 1, numericFrame(2, 0.75, 0.625)); err != nil {
				t.Fatal(err)
			}
			lastAccepted, _ := service.LatestMerged()
			got, err := pipeline.ProcessAt(lastAccepted, math.MaxInt64)
			if err != nil {
				t.Fatal(err)
			}

			err = service.Submit("vendor.combined", 1, numericFrame(3, tt.thirdEye, tt.thirdJaw))
			if !errors.Is(err, tracking.ErrTimestampRegression) {
				t.Fatalf("saturated Submit() error = %v, want ErrTimestampRegression", err)
			}
			afterRejected, _ := service.LatestMerged()
			if afterRejected != lastAccepted {
				t.Fatalf("tracking published rejected update: got %#v, want unchanged %#v", afterRejected, lastAccepted)
			}
			jaw, jawValid := got.Expressions.Get(trackingmodel.ExpressionJawOpen)
			if got.Eye.LeftOpenness != 0.75 || !jawValid || jaw != 0.625 {
				t.Fatalf("processing state = %#v, jaw=%v,%t; want last accepted Eye=0.75 Jaw=0.625", got, jaw, jawValid)
			}
		})
	}
}

func numericFrame(sequence uint64, eye, jaw float32) trackingmodel.TrackingFrame {
	frame := trackingmodel.TrackingFrame{
		Sequence:     sequence,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: eye,
		},
	}
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, jaw)
	return frame
}
