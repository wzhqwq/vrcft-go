package application

import (
	"math"
	"time"
)

// monotonicClock adapts wall time to the caller-serialized Host clock used by
// processing. It clamps rollback and can require strict progress for a new
// processing action.
type monotonicClock struct {
	wall    func() time.Time
	last    int64
	hasLast bool
}

func newMonotonicClock(wall func() time.Time) *monotonicClock {
	if wall == nil {
		wall = time.Now
	}
	return &monotonicClock{wall: wall}
}

func (clock *monotonicClock) now(strict bool) int64 {
	return clock.sample(clock.wall(), strict)
}

func (clock *monotonicClock) observe(wall time.Time) int64 {
	return clock.sample(wall, false)
}

func (clock *monotonicClock) advance(wall time.Time) int64 {
	return clock.sample(wall, true)
}

func (clock *monotonicClock) sample(wall time.Time, strict bool) int64 {
	nowNS := wall.UnixNano()
	if !clock.hasLast {
		clock.last = nowNS
		clock.hasLast = true
		return nowNS
	}
	if nowNS < clock.last {
		nowNS = clock.last
	}
	if strict && nowNS <= clock.last && clock.last < math.MaxInt64 {
		nowNS = clock.last + 1
	}
	clock.last = nowNS
	return nowNS
}
