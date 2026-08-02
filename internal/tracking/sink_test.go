package tracking

import (
	"errors"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type recordingFrameSubmitter struct {
	calls      int
	pluginID   string
	generation uint64
	frame      trackingmodel.TrackingFrame
	err        error
}

func (r *recordingFrameSubmitter) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) error {
	r.calls++
	r.pluginID = pluginID
	r.generation = generation
	r.frame = frame
	return r.err
}

func TestPluginFrameSinkForwardsExactSubmissionAndIgnoresTargetError(t *testing.T) {
	target := &recordingFrameSubmitter{err: errors.New("rejected by service")}
	sink := NewPluginFrameSink(target)
	frame := trackingmodel.TrackingFrame{
		Sequence:      42,
		TimestampNS:   100,
		SourceClockNS: 90,
		Capabilities:  trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.75,
		},
	}
	frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.5)
	wantFrame := frame

	sink.Submit("vendor.tracker", 7, frame)

	if target.calls != 1 {
		t.Fatalf("target Submit calls = %d, want 1", target.calls)
	}
	if target.pluginID != "vendor.tracker" || target.generation != 7 || target.frame != wantFrame {
		t.Fatalf("forwarded submission = (%q, %d, %#v), want (%q, %d, %#v)", target.pluginID, target.generation, target.frame, "vendor.tracker", 7, wantFrame)
	}

	frame.Sequence = 99
	frame.Eye.LeftOpenness = 0.1
	frame.Expressions.Values[trackingmodel.ExpressionJawOpen] = 0.2
	if target.frame != wantFrame {
		t.Fatalf("recorded frame after caller mutation = %#v, want owned value %#v", target.frame, wantFrame)
	}
}

func TestPluginFrameSinkZeroAndNilTargetAreSafeNoOps(t *testing.T) {
	frame := trackingmodel.TrackingFrame{Sequence: 1}

	var zero PluginFrameSink
	zero.Submit("vendor.zero", 1, frame)

	withNilTarget := NewPluginFrameSink(nil)
	withNilTarget.Submit("vendor.nil", 2, frame)
}
