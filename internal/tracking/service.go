package tracking

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type Service interface {
	Submit(string, uint64, trackingmodel.TrackingFrame) error
	SetGeneration(uint64) error
	Generation() uint64
	SetRouting(RoutingConfig) error
	Routing() RoutingConfig
	RemoveSource(string)
	LatestMerged() (MergedFrame, bool)
	SubscribeMerged(context.Context) <-chan MergedFrame
	SubscribeSummary(context.Context) <-chan Summary
}

type service struct {
	mu sync.Mutex

	now func() int64

	generation         uint64
	routing            RoutingConfig
	sources            map[string]sourceState
	eyeSourceID        string
	expressionSourceID string
	lipSourceID        string
	mergedSequence     uint64
	lastHostTimeNS     int64
	latestMerged       MergedFrame
	hasLatest          bool

	acceptedFrames uint64
	rejectedFrames uint64
	rejected       RejectionCounts
	lastRejection  Rejection

	mergedSubscribers  map[chan MergedFrame]struct{}
	summarySubscribers map[chan Summary]struct{}
}

var _ Service = (*service)(nil)

func NewService() Service {
	return newServiceWithClock(func() int64 { return time.Now().UnixNano() })
}

func newServiceWithClock(now func() int64) *service {
	return &service{
		now:                now,
		routing:            defaultRouting(),
		sources:            make(map[string]sourceState),
		mergedSubscribers:  make(map[chan MergedFrame]struct{}),
		summarySubscribers: make(map[chan Summary]struct{}),
	}
}

func (s *service) SetGeneration(generation uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if generation == 0 {
		return ErrGenerationZero
	}
	if generation < s.generation {
		return ErrGenerationRegression
	}
	if generation == s.generation {
		return nil
	}

	s.generation = generation
	s.sources = make(map[string]sourceState)
	s.eyeSourceID = ""
	s.expressionSourceID = ""
	s.lipSourceID = ""
	s.advanceMergedSequenceLocked()
	s.latestMerged = MergedFrame{
		Generation:  generation,
		Sequence:    s.mergedSequence,
		UpdatedAtNS: s.nextTimeLocked(),
	}
	s.hasLatest = true
	s.publishMergedLocked()
	s.publishSummaryLocked()
	return nil
}

func (s *service) Generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.generation
}

func (s *service) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pluginID == "" {
		return s.rejectLocked(pluginID, generation, RejectionInvalidPluginID, ErrInvalidPluginID)
	}
	if s.generation == 0 {
		return s.rejectLocked(pluginID, generation, RejectionGenerationUnset, ErrGenerationUnset)
	}
	if generation == 0 {
		return s.rejectLocked(pluginID, generation, RejectionGenerationZero, ErrGenerationZero)
	}
	if generation < s.generation {
		return s.rejectLocked(pluginID, generation, RejectionStaleGeneration, ErrStaleGeneration)
	}
	if generation > s.generation {
		return s.rejectLocked(pluginID, generation, RejectionFutureGeneration, ErrFutureGeneration)
	}

	canonical, err := frame.Canonicalize()
	if err != nil {
		return s.rejectLocked(pluginID, generation, RejectionInvalidFrame, fmt.Errorf("%w: %v", ErrInvalidFrame, err))
	}

	previous, exists := s.sources[pluginID]
	if exists && canonical.Sequence <= previous.lastSequence {
		return s.rejectLocked(pluginID, generation, RejectionSequenceNotIncreasing, ErrSequenceNotIncreasing)
	}
	if canonical.TimestampNS < 0 || canonical.SourceClockNS < 0 {
		return s.rejectLocked(pluginID, generation, RejectionInvalidFrame, ErrInvalidFrame)
	}
	if exists && canonical.TimestampNS != 0 && previous.lastTimestampNS != 0 && canonical.TimestampNS < previous.lastTimestampNS {
		return s.rejectLocked(pluginID, generation, RejectionTimestampRegression, ErrTimestampRegression)
	}
	if exists && canonical.SourceClockNS != 0 && previous.lastSourceClockNS != 0 && canonical.SourceClockNS < previous.lastSourceClockNS {
		return s.rejectLocked(pluginID, generation, RejectionSourceClockRegression, ErrSourceClockRegression)
	}

	lastTimestampNS := previous.lastTimestampNS
	if canonical.TimestampNS != 0 {
		lastTimestampNS = canonical.TimestampNS
	}
	lastSourceClockNS := previous.lastSourceClockNS
	if canonical.SourceClockNS != 0 {
		lastSourceClockNS = canonical.SourceClockNS
	}
	if s.lastHostTimeNS == math.MaxInt64 {
		return s.rejectLocked(pluginID, generation, RejectionTimestampRegression, ErrTimestampRegression)
	}
	selected := pluginID == s.eyeSourceID || pluginID == s.expressionSourceID || pluginID == s.lipSourceID
	s.sources[pluginID] = sourceState{
		frame:             canonical,
		receivedAtNS:      s.nextReceiptTimeLocked(),
		lastSequence:      canonical.Sequence,
		lastTimestampNS:   lastTimestampNS,
		lastSourceClockNS: lastSourceClockNS,
	}
	if s.recomputeMergedLocked(selected) {
		s.publishMergedLocked()
	}
	s.acceptedFrames = saturatingAdd(s.acceptedFrames, 1)
	s.publishSummaryLocked()
	return nil
}

func (s *service) LatestMerged() (MergedFrame, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.latestMerged, s.hasLatest
}

func (s *service) nextTimeLocked() int64 {
	now := s.now()
	if now < s.lastHostTimeNS {
		return s.lastHostTimeNS
	}
	s.lastHostTimeNS = now
	return now
}

func (s *service) nextReceiptTimeLocked() int64 {
	now := s.now()
	if now <= s.lastHostTimeNS {
		if s.lastHostTimeNS < math.MaxInt64 {
			s.lastHostTimeNS++
		}
		return s.lastHostTimeNS
	}
	s.lastHostTimeNS = now
	return now
}
