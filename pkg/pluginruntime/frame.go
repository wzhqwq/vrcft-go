package pluginruntime

import (
	"sync"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type LatestFrameSlot struct {
	mu      sync.Mutex
	frame   trackingmodel.TrackingFrame
	pending bool
	notify  chan struct{}
}

func (s *LatestFrameSlot) Store(frame trackingmodel.TrackingFrame) {
	s.mu.Lock()
	s.frame = frame
	shouldNotify := !s.pending
	s.pending = true
	s.mu.Unlock()

	if shouldNotify {
		select {
		case s.notify <- struct{}{}:
		default:
		}
	}
}
