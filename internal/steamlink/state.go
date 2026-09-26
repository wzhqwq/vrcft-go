package steamlink

import (
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type streamState struct {
	raw       rawSnapshot
	selection rawSelection
	sub       pluginapi.Subscription
	active    bool
	dirty     bool

	previous      trackingmodel.TrackingFrame
	hasPrevious   bool
	startedAt     time.Time
	sequence      uint64
	lastTimestamp int64
}

func newStreamState(startedAt time.Time) *streamState {
	return &streamState{startedAt: startedAt}
}

func (s *streamState) reset(active bool, sub pluginapi.Subscription) {
	s.raw = rawSnapshot{}
	s.selection = rawSelection{}
	s.sub = sub.Normalize()
	s.active = active
	s.dirty = false
	s.previous = trackingmodel.TrackingFrame{}
	s.hasPrevious = false

	if s.active && s.sub.Generation > 0 {
		s.selection = requiredFields(s.sub)
	}
}

func (s *streamState) observe(values []observation, receivedAt time.Time) {
	if !s.active || s.sub.Generation == 0 {
		return
	}
	for _, value := range values {
		if value.ID >= rawCount || !s.selection[value.ID] {
			continue
		}
		s.raw[value.ID] = rawSample{Values: value.Values, ReceivedAt: receivedAt, Seen: true}
		s.dirty = true
	}
}

func (s *streamState) next(now time.Time) (trackingmodel.TrackingFrame, bool) {
	if !s.active || s.sub.Generation == 0 {
		return trackingmodel.TrackingFrame{}, false
	}

	current := mapSnapshot(s.raw, s.sub, now)
	publish := s.dirty || (s.hasPrevious && !samePayload(current, s.previous))
	s.dirty = false
	if !publish || (!s.hasPrevious && emptyPayload(current)) {
		return trackingmodel.TrackingFrame{}, false
	}

	s.previous = current
	s.hasPrevious = true
	s.sequence++
	timestamp := now.Sub(s.startedAt).Nanoseconds()
	if timestamp < 1 {
		timestamp = 1
	}
	if timestamp <= s.lastTimestamp {
		timestamp = s.lastTimestamp + 1
	}
	s.lastTimestamp = timestamp
	current.Sequence = s.sequence
	current.TimestampNS = timestamp
	current.SourceClockNS = 0
	return current, true
}

func emptyPayload(frame trackingmodel.TrackingFrame) bool {
	return frame.Eye.Valid == 0 && frame.Expressions.Valid.IsZero()
}

func samePayload(left, right trackingmodel.TrackingFrame) bool {
	left.Sequence = 0
	left.TimestampNS = 0
	left.SourceClockNS = 0
	right.Sequence = 0
	right.TimestampNS = 0
	right.SourceClockNS = 0
	return left == right
}
