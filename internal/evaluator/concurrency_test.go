package evaluator

import (
	"fmt"
	"sync"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestConcurrentEvaluateOneImmutablePlan(t *testing.T) {
	plan, err := Compile([]parameters.ParameterID{
		parameters.ParameterEyeX,
		parameters.ParameterJawOpen,
		parameters.ParameterLipTrackingActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	testCases := [...]struct {
		leftX    float32
		rightX   float32
		jawOpen  float32
		lip      bool
		wantEyeX float32
	}{
		{leftX: -0.75, rightX: 0.25, jawOpen: 0.125, lip: false, wantEyeX: -0.25},
		{leftX: -0.5, rightX: 1, jawOpen: 0.25, lip: true, wantEyeX: 0.25},
		{leftX: 0, rightX: 0.5, jawOpen: 0.5, lip: false, wantEyeX: 0.25},
		{leftX: 0.25, rightX: 0.75, jawOpen: 0.875, lip: true, wantEyeX: 0.5},
	}

	const workerCount = 16
	const evaluationsPerWorker = 1000
	errs := make(chan error, workerCount)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func(worker int) {
			defer workers.Done()
			for index := 0; index < evaluationsPerWorker; index++ {
				current := testCases[(worker+index)%len(testCases)]
				var expressions trackingmodel.ExpressionSet
				expressions.Set(trackingmodel.ExpressionJawOpen, current.jawOpen)
				frame := processing.CanonicalFrame{
					Generation: uint64(worker*evaluationsPerWorker + index + 1),
					LipActive:  current.lip,
					Eye: trackingmodel.EyeSample{
						Valid:     trackingmodel.EyeValidLeftGaze | trackingmodel.EyeValidRightGaze,
						LeftGaze:  trackingmodel.Vec2{X: current.leftX},
						RightGaze: trackingmodel.Vec2{X: current.rightX},
					},
					Expressions: expressions,
				}
				snapshot := plan.Evaluate(frame)
				if got, ok := snapshot.Float(parameters.ParameterEyeX); !ok || got != current.wantEyeX {
					errs <- fmt.Errorf("worker %d evaluation %d EyeX = %v,%t, want %v,true", worker, index, got, ok, current.wantEyeX)
					return
				}
				if got, ok := snapshot.Float(parameters.ParameterJawOpen); !ok || got != current.jawOpen {
					errs <- fmt.Errorf("worker %d evaluation %d JawOpen = %v,%t, want %v,true", worker, index, got, ok, current.jawOpen)
					return
				}
				if got, ok := snapshot.Bool(parameters.ParameterLipTrackingActive); !ok || got != current.lip {
					errs <- fmt.Errorf("worker %d evaluation %d LipTrackingActive = %v,%t, want %v,true", worker, index, got, ok, current.lip)
					return
				}
			}
		}(worker)
	}
	workers.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
