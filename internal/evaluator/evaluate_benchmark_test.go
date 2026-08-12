package evaluator_test

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/processing"
)

var (
	benchmarkSnapshot evaluator.Snapshot
	benchmarkFrame    processing.CanonicalFrame
)

func TestEvaluateAllocationsAfterCompile(t *testing.T) {
	plan, frame := compiledIntegrationFixture(t)

	allocations := testing.AllocsPerRun(1000, func() {
		_ = plan.Evaluate(frame)
	})
	if allocations != 0 {
		t.Fatalf("Plan.Evaluate allocations = %v, want 0", allocations)
	}
}

func BenchmarkEvaluate(b *testing.B) {
	plan, frame := compiledIntegrationFixture(b)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkSnapshot = plan.Evaluate(frame)
	}
}

func BenchmarkPipeline(b *testing.B) {
	pipeline, err := processing.NewPipeline(processing.DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	merged := integrationMergedFrame()
	nowNS := int64(4_000_000_000)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		merged.Sequence++
		merged.UpdatedAtNS++
		merged.EyeUpdatedAtNS++
		merged.ExpressionUpdatedAtNS++
		merged.LipUpdatedAtNS++
		nowNS++
		benchmarkFrame, err = pipeline.ProcessAt(merged, nowNS)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func compiledIntegrationFixture(tb testing.TB) (*evaluator.Plan, processing.CanonicalFrame) {
	tb.Helper()
	pipeline, err := processing.NewPipeline(processing.DefaultConfig())
	if err != nil {
		tb.Fatal(err)
	}
	frame, err := pipeline.ProcessAt(integrationMergedFrame(), 4_000_000_000)
	if err != nil {
		tb.Fatal(err)
	}
	plan, err := evaluator.Compile(integrationParameterIDs())
	if err != nil {
		tb.Fatal(err)
	}
	return plan, frame
}
