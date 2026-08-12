package tracking

import (
	"math"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type MergedFrame struct {
	Generation            uint64
	Sequence              uint64
	UpdatedAtNS           int64
	Capabilities          trackingmodel.Capability
	Eye                   trackingmodel.EyeSample
	Expressions           trackingmodel.ExpressionSet
	EyeSourceID           string
	ExpressionSourceID    string
	LipSourceID           string
	EyeUpdatedAtNS        int64
	ExpressionUpdatedAtNS int64
	LipUpdatedAtNS        int64
}

func mergedContentEqual(left, right MergedFrame) bool {
	return left.Generation == right.Generation &&
		left.Capabilities == right.Capabilities &&
		left.Eye == right.Eye &&
		left.Expressions == right.Expressions &&
		left.EyeSourceID == right.EyeSourceID &&
		left.ExpressionSourceID == right.ExpressionSourceID &&
		left.LipSourceID == right.LipSourceID &&
		left.EyeUpdatedAtNS == right.EyeUpdatedAtNS &&
		left.ExpressionUpdatedAtNS == right.ExpressionUpdatedAtNS &&
		left.LipUpdatedAtNS == right.LipUpdatedAtNS
}

func (s *service) advanceMergedSequenceLocked() {
	if s.mergedSequence < math.MaxUint64 {
		s.mergedSequence++
	}
}

func (s *service) recomputeMergedLocked(force bool) bool {
	nextEyeSourceID := resolveSource(s.routing.Eye, s.eyeSourceID, s.sources, trackingmodel.CapabilityEye)
	nextExpressionSourceID := resolveSource(s.routing.Expression, s.expressionSourceID, s.sources, trackingmodel.CapabilityExpression)
	nextLipSourceID := resolveSource(s.routing.Lip, s.lipSourceID, s.sources, trackingmodel.CapabilityLip)
	selectionChanged := nextEyeSourceID != s.eyeSourceID || nextExpressionSourceID != s.expressionSourceID || nextLipSourceID != s.lipSourceID
	s.eyeSourceID = nextEyeSourceID
	s.expressionSourceID = nextExpressionSourceID
	s.lipSourceID = nextLipSourceID

	next := MergedFrame{
		Generation:         s.generation,
		EyeSourceID:        nextEyeSourceID,
		ExpressionSourceID: nextExpressionSourceID,
		LipSourceID:        nextLipSourceID,
	}
	if nextEyeSourceID != "" {
		source := s.sources[nextEyeSourceID]
		next.Capabilities |= trackingmodel.CapabilityEye
		next.Eye = source.frame.Eye
		next.EyeUpdatedAtNS = source.receivedAtNS
	}
	if nextExpressionSourceID != "" {
		source := s.sources[nextExpressionSourceID]
		next.Capabilities |= trackingmodel.CapabilityExpression
		next.Expressions = source.frame.Expressions
		next.ExpressionUpdatedAtNS = source.receivedAtNS
	}
	if nextLipSourceID != "" {
		source := s.sources[nextLipSourceID]
		next.Capabilities |= trackingmodel.CapabilityLip
		next.LipUpdatedAtNS = source.receivedAtNS
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
