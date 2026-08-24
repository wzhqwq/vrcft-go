package application

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestStatusStoreCreatedSnapshotAndUpdatesAreOwned(t *testing.T) {
	createdAt := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Second)
	clock := []time.Time{createdAt, updatedAt}
	store := newStatusStore(func() time.Time {
		now := clock[0]
		clock = clock[1:]
		return now
	})

	created := store.snapshot()
	if created.Revision != 1 || !created.UpdatedAt.Equal(createdAt) || created.Lifecycle != LifecycleCreated {
		t.Fatalf("created status = %+v, want revision 1, created time, and created lifecycle", created)
	}

	updated := store.update(func(status *Status) {
		status.PluginFailures = []PluginControlFailure{{PluginID: "plugin.one", Operation: "activate", Message: "failed"}}
	})
	if updated.Revision != 2 || !updated.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated status = %+v, want revision 2 and updated time", updated)
	}
	updated.PluginFailures[0].PluginID = "mutated"
	if got := store.snapshot().PluginFailures[0].PluginID; got != "plugin.one" {
		t.Fatalf("snapshot PluginFailures alias update result: got %q, want plugin.one", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subscriber := store.subscribe(ctx)
	fromSubscriber := receiveStatus(t, subscriber)
	fromSubscriber.PluginFailures[0].PluginID = "subscriber mutation"
	if got := store.snapshot().PluginFailures[0].PluginID; got != "plugin.one" {
		t.Fatalf("snapshot PluginFailures alias subscriber value: got %q, want plugin.one", got)
	}
}

func TestStatusStoreSubscribersReceiveCreatedAndLatestValuesAndCancelIndependently(t *testing.T) {
	now := time.Date(2026, 8, 24, 4, 5, 6, 0, time.UTC)
	store := newStatusStore(func() time.Time {
		now = now.Add(time.Second)
		return now
	})

	firstContext, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	secondContext, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	first := store.subscribe(firstContext)
	second := store.subscribe(secondContext)
	if got := receiveStatus(t, first); got.Revision != 1 || got.Lifecycle != LifecycleCreated {
		t.Fatalf("first immediate status = %+v, want created revision 1", got)
	}
	if got := receiveStatus(t, second); got.Revision != 1 || got.Lifecycle != LifecycleCreated {
		t.Fatalf("second immediate status = %+v, want created revision 1", got)
	}

	for revision := uint64(2); revision <= 5; revision++ {
		revision := revision
		store.update(func(status *Status) { status.PlanGeneration = revision })
	}
	if got := receiveStatus(t, first); got.Revision != 5 || got.PlanGeneration != 5 {
		t.Fatalf("first latest status = %+v, want revision and generation 5", got)
	}
	if got := receiveStatus(t, second); got.Revision != 5 || got.PlanGeneration != 5 {
		t.Fatalf("second latest status = %+v, want revision and generation 5", got)
	}

	cancelFirst()
	assertStatusChannelClosed(t, first)
	store.update(func(status *Status) { status.PlanGeneration = 6 })
	if got := receiveStatus(t, second); got.Revision != 6 || got.PlanGeneration != 6 {
		t.Fatalf("live subscriber status = %+v, want revision and generation 6", got)
	}
}

func TestStatusStoreRevisionSaturatesAtMaximumPositiveValue(t *testing.T) {
	store := newStatusStore(time.Now)
	store.current.Revision = 0
	if got := store.update(func(*Status) {}).Revision; got != 1 {
		t.Fatalf("zero revision update = %d, want 1", got)
	}
	store.current.Revision = math.MaxUint64
	if got := store.update(func(*Status) {}).Revision; got != math.MaxUint64 {
		t.Fatalf("saturated revision = %d, want %d", got, uint64(math.MaxUint64))
	}
}

func receiveStatus(t *testing.T, values <-chan Status) Status {
	t.Helper()
	select {
	case status, ok := <-values:
		if !ok {
			t.Fatal("status subscription closed before a value arrived")
		}
		return status
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for status value")
		return Status{}
	}
}

func assertStatusChannelClosed(t *testing.T, values <-chan Status) {
	t.Helper()
	select {
	case _, ok := <-values:
		if ok {
			t.Fatal("status subscription remained open after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancelled status subscription to close")
	}
}
