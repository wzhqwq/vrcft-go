package tracking

import (
	"math"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type MergedFrame struct {
	Generation         uint64
	Sequence           uint64
	UpdatedAtNS        int64
	Capabilities       trackingmodel.Capability
	Eye                trackingmodel.EyeSample
	Expressions        trackingmodel.ExpressionSet
	EyeSourceID        string
	ExpressionSourceID string
}

func mergedContentEqual(left, right MergedFrame) bool {
	return left.Generation == right.Generation &&
		left.Capabilities == right.Capabilities &&
		left.Eye == right.Eye &&
		left.Expressions == right.Expressions &&
		left.EyeSourceID == right.EyeSourceID &&
		left.ExpressionSourceID == right.ExpressionSourceID
}

func (s *service) advanceMergedSequenceLocked() {
	if s.mergedSequence < math.MaxUint64 {
		s.mergedSequence++
	}
}

func (s *service) recomputeMergedLocked(force bool) bool {
	nextEyeSourceID := resolveSource(s.routing.Eye, s.eyeSourceID, s.sources, trackingmodel.CapabilityEye)
	nextExpressionSourceID := resolveSource(s.routing.Expression, s.expressionSourceID, s.sources, trackingmodel.CapabilityExpression)
	selectionChanged := nextEyeSourceID != s.eyeSourceID || nextExpressionSourceID != s.expressionSourceID
	s.eyeSourceID = nextEyeSourceID
	s.expressionSourceID = nextExpressionSourceID

	next := MergedFrame{
		Generation:         s.generation,
		EyeSourceID:        nextEyeSourceID,
		ExpressionSourceID: nextExpressionSourceID,
	}
	if nextEyeSourceID != "" {
		next.Capabilities |= trackingmodel.CapabilityEye
		next.Eye = s.sources[nextEyeSourceID].frame.Eye
	}
	if nextExpressionSourceID != "" {
		next.Capabilities |= trackingmodel.CapabilityExpression
		next.Expressions = s.sources[nextExpressionSourceID].frame.Expressions
	}

	if !force && !selectionChanged && s.hasLatest && mergedContentEqual(next, s.latestMerged) {
		return false
	}

	s.advanceMergedSequenceLocked()
	next.Sequence = s.mergedSequence
	next.UpdatedAtNS = s.nextTimeLocked()
	s.latestMerged = next
	s.hasLatest = true
	return true
}
