package main

import (
	"context"
	"math"
	"testing"
	"time"
)

type storeValue struct{ Items []string }

func cloneStoreValue(value storeValue) storeValue {
	value.Items = append([]string(nil), value.Items...)
	return value
}

func TestModuleStoreStartsAtRevisionOne(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	store := newModuleStore(storeValue{Items: []string{"initial"}}, cloneStoreValue, func() time.Time { return now })

	got := store.snapshot()
	if got.Revision != 1 || !got.UpdatedAt.Equal(now) || got.Value.Items[0] != "initial" {
		t.Fatalf("initial snapshot = %#v", got)
	}
}

func TestModuleStoreClampsClockAndSaturatesRevision(t *testing.T) {
	times := []time.Time{
		time.Date(2026, 8, 26, 12, 0, 1, 0, time.UTC),
		time.Date(2026, 8, 26, 11, 59, 59, 0, time.UTC),
		time.Date(2026, 8, 26, 12, 0, 2, 0, time.UTC),
	}
	index := 0
	store := newModuleStore(storeValue{}, cloneStoreValue, func() time.Time {
		value := times[index]
		index++
		return value
	})

	first := store.snapshot()
	second := store.update(storeValue{Items: []string{"next"}}, nil)
	if second.Revision != 2 || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("clamped update = %#v after %#v", second, first)
	}

	store.current.Revision = math.MaxUint64
	saturated := store.update(storeValue{Items: []string{"saturated"}}, nil)
	if saturated.Revision != math.MaxUint64 {
		t.Fatalf("saturated Revision = %d, want MaxUint64", saturated.Revision)
	}
}

func TestModuleStoreOwnsSnapshotsAndLatestSubscription(t *testing.T) {
	store := newModuleStore(storeValue{Items: []string{"initial"}}, cloneStoreValue, time.Now)
	read := store.snapshot()
	read.Value.Items[0] = "mutated read"
	if got := store.snapshot().Value.Items[0]; got != "initial" {
		t.Fatalf("snapshot mutation leaked into store: %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := store.subscribe(ctx)
	initial := <-updates
	initial.Value.Items[0] = "mutated event"
	if got := store.snapshot().Value.Items[0]; got != "initial" {
		t.Fatalf("subscription mutation leaked into store: %q", got)
	}

	store.update(storeValue{Items: []string{"one"}}, nil)
	store.update(storeValue{Items: []string{"two"}}, nil)
	latest := <-updates
	if cap(updates) != 1 || latest.Value.Items[0] != "two" {
		t.Fatalf("latest update = %#v, channel capacity = %d", latest, cap(updates))
	}

	cancel()
	if _, ok := <-updates; ok {
		t.Fatal("subscription remained open after cancellation")
	}
}
