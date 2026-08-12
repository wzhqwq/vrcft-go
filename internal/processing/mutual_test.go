package processing

import (
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestMutualLargestAbsoluteValueWins(t *testing.T) {
	pipeline, jaw, closed := mustMutualPipeline(t)
	frame := mutualExpressionFrame(1, 100, "face", 0.6, -0.8)
	got, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	assertExpression(t, got, trackingmodel.ExpressionJawOpen, 0, true)
	assertExpression(t, got, trackingmodel.ExpressionMouthClosed, -0.8, true)
	_ = jaw
	_ = closed
}

func TestMutualAbsoluteTieChoosesSmallerChannelID(t *testing.T) {
	pipeline, jaw, closed := mustMutualPipeline(t)
	if jaw >= closed {
		t.Fatalf("test requires JawOpen channel %d smaller than MouthClosed channel %d", jaw, closed)
	}
	frame := mutualExpressionFrame(1, 100, "face", 0.5, -0.5)
	got, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	assertExpression(t, got, trackingmodel.ExpressionJawOpen, 0.5, true)
	assertExpression(t, got, trackingmodel.ExpressionMouthClosed, 0, true)
}

func TestMutualInvalidChannelDoesNotWin(t *testing.T) {
	pipeline, _, _ := mustMutualPipeline(t)
	frame := expressionChannelsFrame(1, 100, "face", map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionMouthClosed: 0.7,
	})
	got, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	assertExpression(t, got, trackingmodel.ExpressionJawOpen, 0, false)
	assertExpression(t, got, trackingmodel.ExpressionMouthClosed, 0.7, true)
}

func TestMutualLoserRemainsValidZero(t *testing.T) {
	pipeline, _, _ := mustMutualPipeline(t)
	frame := mutualExpressionFrame(1, 100, "face", 0.8, 0.3)
	got, err := pipeline.ProcessAt(frame, 100)
	if err != nil {
		t.Fatal(err)
	}
	assertExpression(t, got, trackingmodel.ExpressionJawOpen, 0.8, true)
	assertExpression(t, got, trackingmodel.ExpressionMouthClosed, 0, true)
}

func TestMutualHeldCandidateCannotCoexistWithFreshWinner(t *testing.T) {
	pipeline, _, _ := mustMutualPipeline(t)
	old := expressionChannelsFrame(1, 100, "face", map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: 0.8,
	})
	if _, err := pipeline.ProcessAt(old, 100); err != nil {
		t.Fatal(err)
	}
	fresh := expressionChannelsFrame(2, 105, "face", map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionMouthClosed: 1,
	})
	got, err := pipeline.ProcessAt(fresh, 105)
	if err != nil {
		t.Fatal(err)
	}
	assertExpression(t, got, trackingmodel.ExpressionJawOpen, 0, true)
	assertExpression(t, got, trackingmodel.ExpressionMouthClosed, 1, true)
}

func TestMutualProjectionDoesNotOverwriteIndependentHistory(t *testing.T) {
	config := dropoutTestConfig()
	jaw, ok := ExpressionChannel(trackingmodel.ExpressionJawOpen)
	if !ok {
		t.Fatal("JawOpen channel missing")
	}
	closed, ok := ExpressionChannel(trackingmodel.ExpressionMouthClosed)
	if !ok {
		t.Fatal("MouthClosed channel missing")
	}
	config.MutualExclusion = [][]ChannelID{{jaw, closed}}
	jawConfig := config.DefaultChannel
	jawConfig.Dropout = DropoutPolicy{StaleAfter: 100 * time.Nanosecond}
	closedConfig := config.DefaultChannel
	closedConfig.Dropout = DropoutPolicy{StaleAfter: time.Nanosecond}
	config.Overrides[jaw] = jawConfig
	config.Overrides[closed] = closedConfig
	pipeline := mustPipeline(t, config)

	old := expressionChannelsFrame(1, 100, "face", map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen: 0.8,
	})
	if _, err := pipeline.ProcessAt(old, 100); err != nil {
		t.Fatal(err)
	}
	fresh := expressionChannelsFrame(2, 105, "face", map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionMouthClosed: 1,
	})
	if got, err := pipeline.ProcessAt(fresh, 105); err != nil || expressionValue(got, trackingmodel.ExpressionJawOpen) != 0 {
		t.Fatalf("fresh winner output = %#v,%v; want projected JawOpen zero", got, err)
	}

	got, err := pipeline.ProcessAt(fresh, 107)
	if err != nil {
		t.Fatal(err)
	}
	assertExpression(t, got, trackingmodel.ExpressionJawOpen, 0.8, true)
	assertExpression(t, got, trackingmodel.ExpressionMouthClosed, 0, true)
}

func mustMutualPipeline(t *testing.T) (*Pipeline, ChannelID, ChannelID) {
	t.Helper()
	config := dropoutTestConfig()
	jaw, ok := ExpressionChannel(trackingmodel.ExpressionJawOpen)
	if !ok {
		t.Fatal("JawOpen channel missing")
	}
	closed, ok := ExpressionChannel(trackingmodel.ExpressionMouthClosed)
	if !ok {
		t.Fatal("MouthClosed channel missing")
	}
	config.MutualExclusion = [][]ChannelID{{jaw, closed}}
	return mustPipeline(t, config), jaw, closed
}

func mutualExpressionFrame(revision uint64, atNS int64, source string, jaw, closed float32) tracking.MergedFrame {
	return expressionChannelsFrame(revision, atNS, source, map[trackingmodel.ExpressionID]float32{
		trackingmodel.ExpressionJawOpen:     jaw,
		trackingmodel.ExpressionMouthClosed: closed,
	})
}

func expressionChannelsFrame(revision uint64, atNS int64, source string, values map[trackingmodel.ExpressionID]float32) tracking.MergedFrame {
	frame := tracking.MergedFrame{
		Generation:            1,
		Sequence:              revision,
		UpdatedAtNS:           atNS,
		Capabilities:          trackingmodel.CapabilityExpression,
		ExpressionSourceID:    source,
		ExpressionUpdatedAtNS: atNS,
	}
	for id, value := range values {
		frame.Expressions.Set(id, value)
	}
	return frame
}

func assertExpression(t *testing.T, frame CanonicalFrame, id trackingmodel.ExpressionID, want float32, wantValid bool) {
	t.Helper()
	got, valid := frame.Expressions.Get(id)
	if got != want || valid != wantValid {
		t.Fatalf("expression %d = %v,%t; want %v,%t", id, got, valid, want, wantValid)
	}
}
