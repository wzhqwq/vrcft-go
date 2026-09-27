package steamlink

import (
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type fakeHost struct {
	mu sync.Mutex

	startup pluginapi.Startup
	events  chan pluginapi.ControlEvent
	frames  []trackingmodel.TrackingFrame
	status  []pluginapi.DeviceStatus
	logs    []string
	accept  bool
}

func newFakeHost(startup pluginapi.Startup) *fakeHost {
	return &fakeHost{startup: cloneStartup(startup), events: make(chan pluginapi.ControlEvent, 16), accept: true}
}

func cloneStartup(startup pluginapi.Startup) pluginapi.Startup {
	startup.Config = startup.Config.Clone()
	return startup
}

func (h *fakeHost) Startup() pluginapi.Startup {
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneStartup(h.startup)
}

func (h *fakeHost) Events() <-chan pluginapi.ControlEvent { return h.events }

func (h *fakeHost) PublishFrame(frame trackingmodel.TrackingFrame) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = append(h.frames, frame)
	return h.accept
}

func (h *fakeHost) PublishStatus(status pluginapi.DeviceStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = append(h.status, status)
}

func (h *fakeHost) Log(_ pluginapi.LogLevel, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, message)
}

func (h *fakeHost) frameCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.frames)
}

func (h *fakeHost) setAccept(accept bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.accept = accept
}

func (h *fakeHost) framesCopy() []trackingmodel.TrackingFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]trackingmodel.TrackingFrame(nil), h.frames...)
}

func (h *fakeHost) statusesCopy() []pluginapi.DeviceStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]pluginapi.DeviceStatus(nil), h.status...)
}

func (h *fakeHost) logsCopy() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.logs...)
}

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*fakeTicker
}

func newFakeClock(now time.Time) *fakeClock { return &fakeClock{now: now} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTicker(period time.Duration) ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTicker{period: period, next: c.now.Add(period), ch: make(chan time.Time, 256)}
	c.tickers = append(c.tickers, t)
	return t
}

func (c *fakeClock) Advance(by time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(by)
	now := c.now
	tickers := append([]*fakeTicker(nil), c.tickers...)
	c.mu.Unlock()
	for _, t := range tickers {
		t.mu.Lock()
		for !t.stopped && !t.next.After(now) {
			select {
			case t.ch <- t.next:
			default:
			}
			t.next = t.next.Add(t.period)
		}
		t.mu.Unlock()
	}
}

type fakeTicker struct {
	mu      sync.Mutex
	period  time.Duration
	next    time.Time
	ch      chan time.Time
	stopped bool
}

func (t *fakeTicker) C() <-chan time.Time { return t.ch }
func (t *fakeTicker) Stop()               { t.mu.Lock(); t.stopped = true; t.mu.Unlock() }
func (t *fakeTicker) stoppedState() bool  { t.mu.Lock(); defer t.mu.Unlock(); return t.stopped }
