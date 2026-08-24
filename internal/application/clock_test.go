package application

import (
	"math"
	"testing"
	"time"
)

func TestMonotonicClockClampsWallClockRollback(t *testing.T) {
	clock := newMonotonicClock(nil)
	walls := []int64{100, 90, 100, 101}
	want := []int64{100, 100, 100, 101}

	for index, wall := range walls {
		if got := clock.observe(time.Unix(0, wall)); got != want[index] {
			t.Fatalf("observe(%d) = %d, want %d", wall, got, want[index])
		}
	}
}

func TestMonotonicClockAdvancesStrictActionsWithoutOverflow(t *testing.T) {
	clock := newMonotonicClock(nil)
	if got := clock.observe(time.Unix(0, 100)); got != 100 {
		t.Fatalf("first observe = %d, want 100", got)
	}
	if got := clock.advance(time.Unix(0, 90)); got != 101 {
		t.Fatalf("advance after rollback = %d, want 101", got)
	}
	if got := clock.advance(time.Unix(0, math.MaxInt64)); got != math.MaxInt64 {
		t.Fatalf("advance to saturation = %d, want MaxInt64", got)
	}
	if got := clock.advance(time.Unix(0, 1)); got != math.MaxInt64 {
		t.Fatalf("advance after saturation = %d, want MaxInt64", got)
	}
}

func TestMonotonicClockUsesInjectedWallTime(t *testing.T) {
	walls := []time.Time{time.Unix(0, 100), time.Unix(0, 90)}
	clock := newMonotonicClock(func() time.Time {
		next := walls[0]
		walls = walls[1:]
		return next
	})

	if got := clock.now(false); got != 100 {
		t.Fatalf("first now = %d, want 100", got)
	}
	if got := clock.now(true); got != 101 {
		t.Fatalf("strict now after rollback = %d, want 101", got)
	}
}
