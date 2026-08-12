package tracking

import (
	"math"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type RejectionReason uint8

const (
	RejectionNone RejectionReason = iota
	RejectionGenerationUnset
	RejectionGenerationZero
	RejectionStaleGeneration
	RejectionFutureGeneration
	RejectionInvalidPluginID
	RejectionInvalidFrame
	RejectionSequenceNotIncreasing
	RejectionTimestampRegression
	RejectionSourceClockRegression
)

type RejectionCounts struct {
	GenerationUnset       uint64
	GenerationZero        uint64
	StaleGeneration       uint64
	FutureGeneration      uint64
	InvalidPluginID       uint64
	InvalidFrame          uint64
	SequenceNotIncreasing uint64
	TimestampRegression   uint64
	SourceClockRegression uint64
}

type Rejection struct {
	PluginID   string
	Generation uint64
	Reason     RejectionReason
}

type Summary struct {
	Generation  uint64
	Routing     RoutingConfig
	SourceCount int

	EyeSourceID         string
	EyeAvailable        bool
	ExpressionSourceID  string
	ExpressionAvailable bool
	LipSourceID         string
	LipAvailable        bool

	AcceptedFrames uint64
	RejectedFrames uint64
	Rejected       RejectionCounts
	LastRejection  Rejection
}

func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

func (counts *RejectionCounts) increment(reason RejectionReason) {
	switch reason {
	case RejectionGenerationUnset:
		counts.GenerationUnset = saturatingAdd(counts.GenerationUnset, 1)
	case RejectionGenerationZero:
		counts.GenerationZero = saturatingAdd(counts.GenerationZero, 1)
	case RejectionStaleGeneration:
		counts.StaleGeneration = saturatingAdd(counts.StaleGeneration, 1)
	case RejectionFutureGeneration:
		counts.FutureGeneration = saturatingAdd(counts.FutureGeneration, 1)
	case RejectionInvalidPluginID:
		counts.InvalidPluginID = saturatingAdd(counts.InvalidPluginID, 1)
	case RejectionInvalidFrame:
		counts.InvalidFrame = saturatingAdd(counts.InvalidFrame, 1)
	case RejectionSequenceNotIncreasing:
		counts.SequenceNotIncreasing = saturatingAdd(counts.SequenceNotIncreasing, 1)
	case RejectionTimestampRegression:
		counts.TimestampRegression = saturatingAdd(counts.TimestampRegression, 1)
	case RejectionSourceClockRegression:
		counts.SourceClockRegression = saturatingAdd(counts.SourceClockRegression, 1)
	}
}

func (s *service) rejectLocked(pluginID string, generation uint64, reason RejectionReason, err error) error {
	s.rejectedFrames = saturatingAdd(s.rejectedFrames, 1)
	s.rejected.increment(reason)
	s.lastRejection = Rejection{PluginID: pluginID, Generation: generation, Reason: reason}
	s.publishSummaryLocked()
	return err
}

func (s *service) currentSummaryLocked() Summary {
	return Summary{
		Generation:          s.generation,
		Routing:             s.routing,
		SourceCount:         len(s.sources),
		EyeSourceID:         s.eyeSourceID,
		EyeAvailable:        s.sourceHasCapabilityLocked(s.eyeSourceID, trackingmodel.CapabilityEye),
		ExpressionSourceID:  s.expressionSourceID,
		ExpressionAvailable: s.sourceHasCapabilityLocked(s.expressionSourceID, trackingmodel.CapabilityExpression),
		LipSourceID:         s.lipSourceID,
		LipAvailable:        s.sourceHasCapabilityLocked(s.lipSourceID, trackingmodel.CapabilityLip),
		AcceptedFrames:      s.acceptedFrames,
		RejectedFrames:      s.rejectedFrames,
		Rejected:            s.rejected,
		LastRejection:       s.lastRejection,
	}
}

func (s *service) sourceHasCapabilityLocked(pluginID string, capability trackingmodel.Capability) bool {
	if pluginID == "" {
		return false
	}
	source, ok := s.sources[pluginID]
	return ok && source.frame.Capabilities.Has(capability)
}
