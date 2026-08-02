package tracking

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type Service interface {
	Submit(pluginID string, frame trackingmodel.TrackingFrame)

	SetRouting(config RoutingConfig)
	Routing() RoutingConfig

	LatestMerged() (MergedFrame, bool)

	SubscribeSummary() <-chan Summary
}

type Summary struct {
}

type service struct {
	mu sync.Mutex

	now func() int64

	generation     uint64
	routing        RoutingConfig
	sources        map[string]sourceState
	mergedSequence uint64
	lastHostTimeNS int64
	latestMerged   MergedFrame
	hasLatest      bool
}

func NewService() *service {
	return newServiceWithClock(func() int64 { return time.Now().UnixNano() })
}

func newServiceWithClock(now func() int64) *service {
	return &service{
		now:     now,
		routing: defaultRouting(),
		sources: make(map[string]sourceState),
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
	if s.mergedSequence < math.MaxUint64 {
		s.mergedSequence++
	}
	s.latestMerged = MergedFrame{
		Generation:  generation,
		Sequence:    s.mergedSequence,
		UpdatedAtNS: s.nextTimeLocked(),
	}
	s.hasLatest = true
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
		return ErrInvalidPluginID
	}
	if s.generation == 0 {
		return ErrGenerationUnset
	}
	if generation == 0 {
		return ErrGenerationZero
	}
	if generation < s.generation {
		return ErrStaleGeneration
	}
	if generation > s.generation {
		return ErrFutureGeneration
	}

	canonical, err := frame.Canonicalize()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}

	previous, exists := s.sources[pluginID]
	if exists && canonical.Sequence <= previous.lastSequence {
		return ErrSequenceNotIncreasing
	}
	if canonical.TimestampNS < 0 || canonical.SourceClockNS < 0 {
		return ErrInvalidFrame
	}
	if exists && canonical.TimestampNS != 0 && previous.lastTimestampNS != 0 && canonical.TimestampNS < previous.lastTimestampNS {
		return ErrTimestampRegression
	}
	if exists && canonical.SourceClockNS != 0 && previous.lastSourceClockNS != 0 && canonical.SourceClockNS < previous.lastSourceClockNS {
		return ErrSourceClockRegression
	}

	lastTimestampNS := previous.lastTimestampNS
	if canonical.TimestampNS != 0 {
		lastTimestampNS = canonical.TimestampNS
	}
	lastSourceClockNS := previous.lastSourceClockNS
	if canonical.SourceClockNS != 0 {
		lastSourceClockNS = canonical.SourceClockNS
	}
	s.sources[pluginID] = sourceState{
		frame:             canonical,
		receivedAtNS:      s.nextTimeLocked(),
		lastSequence:      canonical.Sequence,
		lastTimestampNS:   lastTimestampNS,
		lastSourceClockNS: lastSourceClockNS,
	}
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
