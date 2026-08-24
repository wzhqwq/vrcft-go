package osc

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestAvatarChangesRetainsLatestValueAndRevisionsRepeatedIDs(t *testing.T) {
	controller := newUnstartedController(t, CatalogOSCQuery)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := controller.AvatarChanges(ctx)

	controller.publishAvatarChange("avtr_a")
	first := receiveAvatarChange(t, changes)
	controller.publishAvatarChange("avtr_a")
	controller.publishAvatarChange("avtr_b")
	latest := receiveAvatarChange(t, changes)

	if first.Revision != 1 || first.AvatarID != "avtr_a" || latest.Revision != 3 || latest.AvatarID != "avtr_b" {
		t.Fatalf("changes = %#v, %#v", first, latest)
	}
}

func TestAvatarChangesPublishesToMultipleSubscribers(t *testing.T) {
	controller := newUnstartedController(t, CatalogOSCQuery)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := controller.AvatarChanges(ctx)
	second := controller.AvatarChanges(ctx)

	controller.publishAvatarChange("avtr_shared")
	for index, changes := range []<-chan AvatarChange{first, second} {
		if got := receiveAvatarChange(t, changes); got != (AvatarChange{Revision: 1, AvatarID: "avtr_shared"}) {
			t.Fatalf("subscriber %d change = %#v", index, got)
		}
	}
}

func TestAvatarChangesCancellationClosesSubscription(t *testing.T) {
	controller := newUnstartedController(t, CatalogOSCQuery)
	ctx, cancel := context.WithCancel(context.Background())
	changes := controller.AvatarChanges(ctx)
	cancel()
	assertAvatarChangesClosed(t, changes)
}

func TestAvatarChangesControllerCloseClosesSubscriptions(t *testing.T) {
	controller := newUnstartedController(t, CatalogOSCQuery)
	changes := controller.AvatarChanges(context.Background())
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertAvatarChangesClosed(t, changes)
}

func TestAvatarChangesRevisionSaturatesAndStillReplacesLatest(t *testing.T) {
	controller := newUnstartedController(t, CatalogOSCQuery)
	changes := controller.AvatarChanges(context.Background())
	controller.avatarChanges.mu.Lock()
	controller.avatarChanges.revision = math.MaxUint64 - 1
	controller.avatarChanges.mu.Unlock()

	controller.publishAvatarChange("avtr_before_max")
	controller.publishAvatarChange("avtr_at_max")

	if got := receiveAvatarChange(t, changes); got != (AvatarChange{Revision: math.MaxUint64, AvatarID: "avtr_at_max"}) {
		t.Fatalf("change = %#v, want saturated latest value", got)
	}
}

func receiveAvatarChange(t *testing.T, changes <-chan AvatarChange) AvatarChange {
	t.Helper()
	select {
	case change, ok := <-changes:
		if !ok {
			t.Fatal("avatar changes closed before value")
		}
		return change
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for avatar change")
		return AvatarChange{}
	}
}

func assertAvatarChangesClosed(t *testing.T, changes <-chan AvatarChange) {
	t.Helper()
	select {
	case _, ok := <-changes:
		if ok {
			t.Fatal("avatar changes remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for avatar changes to close")
	}
}
