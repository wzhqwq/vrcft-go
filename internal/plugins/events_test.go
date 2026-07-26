package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func TestEventHubSequencesEventsAndCopiesPublishedValues(t *testing.T) {
	// Catches per-subscriber sequence assignment and retaining caller-owned
	// snapshot/log pointers after Publish returns.
	hub := newEventHub(4)
	t.Cleanup(hub.Close)
	events := hub.Subscribe(context.Background())

	snapshot := &RuntimeSnapshot{ID: "camera", Name: "original", State: StateRunning}
	log := &pluginapi.LogEntry{PluginID: "camera", Level: pluginapi.LogInfo, Message: "original"}
	if !hub.Publish(Event{Type: EventPluginStateChanged, PluginID: "camera", Snapshot: snapshot}) ||
		!hub.Publish(Event{Type: EventPluginLog, PluginID: "camera", Log: log}) {
		t.Fatal("Publish rejected events with available capacity")
	}
	snapshot.Name = "mutated"
	log.Message = "mutated"

	first := receiveEvent(t, events)
	second := receiveEvent(t, events)
	if first.Sequence == 0 || second.Sequence != first.Sequence+1 {
		t.Fatalf("sequences = %d, %d, want adjacent manager-wide sequence", first.Sequence, second.Sequence)
	}
	if first.Snapshot == nil || first.Snapshot.Name != "original" {
		t.Fatalf("snapshot = %+v, want immutable published copy", first.Snapshot)
	}
	if second.Log == nil || second.Log.Message != "original" {
		t.Fatalf("log = %+v, want immutable published copy", second.Log)
	}
}

func TestEventHubSlowSubscriberDoesNotBlockPublishAndCoalescesLatest(t *testing.T) {
	// Catches a fanout loop that blocks on a slow subscriber and a bounded
	// queue that drops the newest state/status rather than coalescing it.
	hub := newEventHub(2)
	t.Cleanup(hub.Close)
	slow := hub.Subscribe(context.Background())
	fast := hub.Subscribe(context.Background())

	published := make(chan struct{})
	go func() {
		for i := 1; i <= 3; i++ {
			hub.Publish(Event{
				Type:     EventPluginStatus,
				PluginID: "camera",
				Snapshot: &RuntimeSnapshot{ID: "camera", ConfigRevision: uint64(i)},
			})
		}
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked behind a slow subscriber")
	}

	gotFast := receiveLatestRevision(t, fast, 3)
	gotSlow := receiveLatestRevision(t, slow, 3)
	if gotFast != 3 || gotSlow != 3 {
		t.Fatalf("latest revisions = fast %d, slow %d, want 3", gotFast, gotSlow)
	}
}

func TestEventHubLogsStayOrderedAndReportDrops(t *testing.T) {
	// Catches unbounded log growth, log reordering, and silently discarded
	// logs that never surface a dropped count.
	hub := newEventHub(2)
	t.Cleanup(hub.Close)
	events := hub.Subscribe(context.Background())

	for _, message := range []string{"one", "two", "three"} {
		hub.Publish(Event{
			Type:     EventPluginLog,
			PluginID: "camera",
			Log:      &pluginapi.LogEntry{PluginID: "camera", Level: pluginapi.LogInfo, Message: message},
		})
	}
	first := receiveEvent(t, events)
	second := receiveEvent(t, events)
	if first.Log.Message != "one" || second.Log.Message != "two" {
		t.Fatalf("retained logs = %q, %q, want one, two", first.Log.Message, second.Log.Message)
	}

	hub.Publish(Event{
		Type:     EventPluginLog,
		PluginID: "camera",
		Log:      &pluginapi.LogEntry{PluginID: "camera", Level: pluginapi.LogInfo, Message: "four"},
	})
	report := receiveEvent(t, events)
	if report.Log.Message != "four" || report.Dropped != 1 {
		t.Fatalf("drop report = message %q dropped %d, want four/1", report.Log.Message, report.Dropped)
	}
}

func TestEventHubCancellationAndCloseAreIndependentAndIdempotent(t *testing.T) {
	// Catches shared subscriber cancellation, leaked subscription channels,
	// and Close panics/deadlocks on repeated calls.
	hub := newEventHub(2)
	firstContext, cancelFirst := context.WithCancel(context.Background())
	first := hub.Subscribe(firstContext)
	second := hub.Subscribe(context.Background())

	cancelFirst()
	waitClosed(t, first)
	hub.Publish(Event{Type: EventPluginDiscovered, PluginID: "camera"})
	if event := receiveEvent(t, second); event.PluginID != "camera" {
		t.Fatalf("second subscriber event = %+v", event)
	}

	hub.Close()
	waitClosed(t, second)
	hub.Close()
}

func receiveEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("event channel closed before event")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func receiveLatestRevision(t *testing.T, events <-chan Event, want uint64) uint64 {
	t.Helper()
	deadline := time.After(time.Second)
	var latest uint64
	for latest != want {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("event channel closed before latest state")
			}
			if event.Snapshot != nil {
				latest = event.Snapshot.ConfigRevision
			}
		case <-deadline:
			t.Fatalf("latest revision = %d, want %d", latest, want)
		}
	}
	return latest
}

func waitClosed(t *testing.T, events <-chan Event) {
	t.Helper()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("event channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event channel close")
	}
}
