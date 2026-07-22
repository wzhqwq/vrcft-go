package pluginruntime

import (
	"sync"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type pendingFrame struct {
	Generation   uint64
	Subscription pluginapi.Subscription
	Frame        trackingmodel.TrackingFrame
}

// LatestFrameSlot is bounded storage for the most recently published frame.
type LatestFrameSlot struct {
	mu      sync.Mutex
	frame   pendingFrame
	pending bool
	notify  chan struct{}
}

func NewLatestFrameSlot() *LatestFrameSlot {
	return &LatestFrameSlot{notify: make(chan struct{}, 1)}
}

// Store replaces any pending frame without waiting for a consumer.
func (s *LatestFrameSlot) Store(frame pendingFrame) bool {
	if frame.Generation == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frame = frame
	s.pending = true
	select {
	case s.notify <- struct{}{}:
	default:
	}
	return true
}

// Load consumes and returns the pending frame, if any.
func (s *LatestFrameSlot) Load() (pendingFrame, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pending {
		return pendingFrame{}, false
	}
	frame := s.frame
	s.frame = pendingFrame{}
	s.pending = false
	s.drainNotifyLocked()
	return frame, true
}

// ClearBefore removes a pending frame only when it belongs to an older generation.
func (s *LatestFrameSlot) ClearBefore(generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending && s.frame.Generation < generation {
		s.clearLocked()
	}
}

// Clear removes any pending frame.
func (s *LatestFrameSlot) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
}

func (s *LatestFrameSlot) clearLocked() {
	s.frame = pendingFrame{}
	s.pending = false
	s.drainNotifyLocked()
}

func (s *LatestFrameSlot) drainNotifyLocked() {
	select {
	case <-s.notify:
	default:
	}
}

func (s *LatestFrameSlot) Notify() <-chan struct{} { return s.notify }
