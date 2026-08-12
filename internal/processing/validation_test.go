package processing

import (
	"errors"
	"math"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestInputValidationRejectsMalformedMergedGroupsWithoutMutatingPipeline(t *testing.T) {
	base := eyeFrame(2, 2, 100, "eye", 8)
	next := eyeFrame(2, 3, 300, "eye", 4)

	tests := []struct {
		name    string
		nowNS   int64
		wantErr error
		bad     func(tracking.MergedFrame) tracking.MergedFrame
	}{
		{"generation regression", 200, ErrGenerationRegression, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Generation = 1
			frame.Sequence = 3
			frame.UpdatedAtNS = 200
			frame.EyeUpdatedAtNS = 200
			return frame
		}},
		{"revision regression", 200, ErrRevisionRegression, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 1
			return frame
		}},
		{"same unsaturated revision changed content", 200, ErrRevisionConflict, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Eye.LeftOpenness = 7
			return frame
		}},
		{"negative merged timestamp", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.UpdatedAtNS = -1
			return frame
		}},
		{"future merged timestamp", 150, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.UpdatedAtNS = 151
			return frame
		}},
		{"negative group timestamp", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.EyeUpdatedAtNS = -1
			return frame
		}},
		{"future group timestamp", 150, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.EyeUpdatedAtNS = 151
			return frame
		}},
		{"NaN valid eye value", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.Eye.LeftOpenness = float32(math.NaN())
			return frame
		}},
		{"NaN valid expression value", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.Capabilities |= trackingmodel.CapabilityExpression
			frame.ExpressionSourceID = "face"
			frame.ExpressionUpdatedAtNS = 200
			frame.Expressions.Set(trackingmodel.ExpressionJawOpen, float32(math.NaN()))
			return frame
		}},
		{"capability without source", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.EyeSourceID = ""
			return frame
		}},
		{"capability without timestamp", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.EyeUpdatedAtNS = 0
			return frame
		}},
		{"absent capability with source", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.Capabilities = 0
			frame.EyeSourceID = "eye"
			frame.EyeUpdatedAtNS = 0
			frame.Eye = trackingmodel.EyeSample{}
			return frame
		}},
		{"absent capability with timestamp", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.Capabilities = 0
			frame.EyeSourceID = ""
			frame.EyeUpdatedAtNS = 100
			frame.Eye = trackingmodel.EyeSample{}
			return frame
		}},
		{"absent capability with data", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.Capabilities = 0
			frame.EyeSourceID = ""
			frame.EyeUpdatedAtNS = 0
			return frame
		}},
		{"unknown capability", 200, ErrInvalidInput, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			frame.Capabilities |= trackingmodel.Capability(1 << 31)
			return frame
		}},
		{"equal time for new revision", 100, ErrTimeRegression, func(frame tracking.MergedFrame) tracking.MergedFrame {
			frame.Sequence = 3
			return frame
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipeline := mustEMAPipeline(t)
			control := mustEMAPipeline(t)
			if _, err := pipeline.ProcessAt(base, 100); err != nil {
				t.Fatal(err)
			}
			if _, err := control.ProcessAt(base, 100); err != nil {
				t.Fatal(err)
			}

			if _, err := pipeline.ProcessAt(test.bad(base), test.nowNS); !errors.Is(err, test.wantErr) {
				t.Fatalf("bad call error = %v; want errors.Is(_, %v)", err, test.wantErr)
			}
			got, gotErr := pipeline.ProcessAt(next, 300)
			want, wantErr := control.ProcessAt(next, 300)
			if gotErr != nil || wantErr != nil {
				t.Fatalf("next valid errors = %v,%v", gotErr, wantErr)
			}
			assertCanonicalEqual(t, got, want)
		})
	}
}

func TestProcessAtTimeRegressionDoesNotMutatePipeline(t *testing.T) {
	base := eyeFrame(1, 1, 100, "eye", 8)
	next := eyeFrame(1, 2, 300, "eye", 4)
	pipeline := mustEMAPipeline(t)
	control := mustEMAPipeline(t)
	if _, err := pipeline.ProcessAt(base, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := control.ProcessAt(base, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.ProcessAt(base, 150); !errors.Is(err, ErrTimeRegression) {
		t.Fatalf("regressed caller time error = %v; want errors.Is(_, %v)", err, ErrTimeRegression)
	}
	got, gotErr := pipeline.ProcessAt(next, 300)
	want, wantErr := control.ProcessAt(next, 300)
	if gotErr != nil || wantErr != nil {
		t.Fatalf("next valid errors = %v,%v", gotErr, wantErr)
	}
	assertCanonicalEqual(t, got, want)
}

func TestInputValidationRequiresPositiveGenerationAndRevision(t *testing.T) {
	for _, test := range []struct {
		name  string
		frame tracking.MergedFrame
	}{
		{"zero generation", eyeFrame(0, 1, 100, "eye", 1)},
		{"zero revision", eyeFrame(1, 0, 100, "eye", 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			pipeline := mustPipeline(t, DefaultConfig())
			if _, err := pipeline.ProcessAt(test.frame, 100); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ProcessAt() error = %v; want errors.Is(_, %v)", err, ErrInvalidInput)
			}
		})
	}
}

func TestProcessAtEqualTimeAcceptsOnlyIdenticalRepeatedInput(t *testing.T) {
	pipeline := mustPipeline(t, DefaultConfig())
	frame := eyeFrame(1, 1, 100, "eye", 0.75)
	first, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatalf("identical equal-time repeat error = %v", err)
	}
	assertCanonicalEqual(t, repeated, first)

	newRevision := eyeFrame(1, 2, 100, "eye", 0.5)
	if _, err := pipeline.ProcessAt(newRevision, 100); !errors.Is(err, ErrTimeRegression) {
		t.Fatalf("new equal-time revision error = %v; want errors.Is(_, %v)", err, ErrTimeRegression)
	}
}
