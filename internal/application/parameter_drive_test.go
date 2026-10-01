package application

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestParameterDriveProbeRequiresFreshValidLeaves(t *testing.T) {
	ids := []parameters.ParameterID{parameters.ParameterEyeTrackingActive, parameters.ParameterMouthSmileRight, parameters.ParameterEyeX, parameters.ParameterEyeLeftX}
	probe, err := newParameterDriveProbe(ids)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := evaluator.Compile(ids)
	if err != nil {
		t.Fatal(err)
	}
	frame := tracking.MergedFrame{Generation: 1, Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression, Eye: trackingmodel.EyeSample{Valid: trackingmodel.EyeValidLeftGaze}, EyeSourceID: "eye", ExpressionSourceID: "face"}
	canonical := processing.CanonicalFrame{Generation: 1, EyeActive: true, Eye: frame.Eye, ExpressionActive: true}
	got := probe.Evaluate(frame, canonical, plan.Evaluate(canonical))
	if len(got) != 4 || got[0].Name != "v2/EyeLeftX" || !got[0].Driven {
		t.Fatalf("eye leaf = %+v", got)
	}
	if got[1].Name != "v2/EyeX" || got[1].Driven {
		t.Fatalf("combined eye needs both leaves: %+v", got)
	}
	if got[2].Name != "v2/MouthSmileRight" || got[2].Driven {
		t.Fatalf("combined expression missing leaves: %+v", got)
	}
	if got[3].Name != "EyeTrackingActive" || !got[3].Driven {
		t.Fatalf("active eye = %+v", got)
	}
	canonical.EyeActive = false // processing may still hold an old eye value
	stale := probe.Evaluate(frame, canonical, plan.Evaluate(canonical))
	if stale[0].Driven || stale[3].Driven {
		t.Fatalf("stale source drives held values: %+v", stale)
	}
}
