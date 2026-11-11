package picoconnect

import (
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const fieldTTL = 250 * time.Millisecond

type streamState struct {
	values        [shapeCount]float32
	seenAt        [shapeCount]time.Time
	sub           pluginapi.Subscription
	active        bool
	dirty         bool
	previous      trackingmodel.TrackingFrame
	hasPrevious   bool
	startedAt     time.Time
	sequence      uint64
	lastTimestamp int64
}

func newStreamState(startedAt time.Time) *streamState { return &streamState{startedAt: startedAt} }

func (s *streamState) reset(active bool, sub pluginapi.Subscription) {
	s.values = [shapeCount]float32{}
	s.seenAt = [shapeCount]time.Time{}
	s.sub, s.active, s.dirty = sub.Normalize(), active, false
	s.previous, s.hasPrevious = trackingmodel.TrackingFrame{}, false
}

func (s *streamState) observe(packet shapePacket, at time.Time) {
	if !s.active || s.sub.Generation == 0 {
		return
	}
	for i, present := range packet.present {
		if present {
			s.values[i], s.seenAt[i] = packet.values[i], at
			s.dirty = true
		}
	}
}

func (s *streamState) next(now time.Time) (trackingmodel.TrackingFrame, bool) {
	if !s.active || s.sub.Generation == 0 {
		return trackingmodel.TrackingFrame{}, false
	}
	var packet shapePacket
	for i, at := range s.seenAt {
		if !at.IsZero() && !now.Before(at) && now.Sub(at) < fieldTTL {
			packet.values[i], packet.present[i] = s.values[i], true
		}
	}
	frame := mapShapes(packet, s.sub)
	publish := s.dirty || (s.hasPrevious && !equalFramePayload(frame, s.previous))
	s.dirty = false
	if !publish || (!s.hasPrevious && frame.Eye.Valid == 0 && frame.Expressions.Valid.IsZero()) {
		return trackingmodel.TrackingFrame{}, false
	}
	s.sequence++
	timestamp := now.Sub(s.startedAt).Nanoseconds()
	if timestamp < 1 {
		timestamp = 1
	}
	if timestamp <= s.lastTimestamp {
		timestamp = s.lastTimestamp + 1
	}
	s.lastTimestamp = timestamp
	frame.Sequence, frame.TimestampNS = s.sequence, timestamp
	s.previous, s.hasPrevious = frame, true
	return frame, true
}

func equalFramePayload(a, b trackingmodel.TrackingFrame) bool {
	a.Sequence, a.TimestampNS, a.SourceClockNS = 0, 0, 0
	b.Sequence, b.TimestampNS, b.SourceClockNS = 0, 0, 0
	return a == b
}
