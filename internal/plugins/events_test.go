package plugins

import (
	"context"
	"sync"
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
	status := &pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: "original"}
	log := &pluginapi.LogEntry{PluginID: "camera", Level: pluginapi.LogInfo, Message: "original"}
	if !hub.Publish(Event{Type: EventPluginStateChanged, PluginID: "camera", Snapshot: snapshot}) ||
		!hub.Publish(Event{Type: EventPluginStatus, PluginID: "camera", Status: status}) ||
		!hub.Publish(Event{Type: EventPluginLog, PluginID: "camera", Log: log}) {
		t.Fatal("Publish rejected events with available capacity")
	}
	snapshot.Name = "mutated"
	status.Message = "mutated"
	log.Message = "mutated"

	first := receiveEvent(t, events)
	second := receiveEvent(t, events)
	third := receiveEvent(t, events)
	if first.Sequence == 0 || second.Sequence != first.Sequence+1 || third.Sequence != second.Sequence+1 {
		t.Fatalf("sequences = %d, %d, %d, want adjacent manager-wide sequence", first.Sequence, second.Sequence, third.Sequence)
	}
	if first.Snapshot == nil || first.Snapshot.Name != "original" {
		t.Fatalf("snapshot = %+v, want immutable published copy", first.Snapshot)
	}
	if second.Status == nil || second.Status.Message != "original" || second.Status == status {
		t.Fatalf("status = %+v, want immutable published copy", second.Status)
	}
	if third.Log == nil || third.Log.Message != "original" {
		t.Fatalf("log = %+v, want immutable published copy", third.Log)
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

func TestEventHubOnlyReportsDroppedLogsOnTheNextAcceptedLog(t *testing.T) {
	// Catches treating discovery/removal entries as logs: their bounded loss
	// must not contaminate the next actual log's dropped count.
	hub := newEventHub(2)
	t.Cleanup(hub.Close)
	events := hub.Subscribe(context.Background())
	for _, event := range []Event{
		{Type: EventPluginDiscovered, PluginID: "one"},
		{Type: EventPluginRemoved, PluginID: "two"},
		{Type: EventPluginDiscovered, PluginID: "three"},
	} {
		hub.Publish(event)
	}
	first := receiveEvent(t, events)
	second := receiveEvent(t, events)
	if first.Type != EventPluginDiscovered || second.Type != EventPluginRemoved {
		t.Fatalf("non-log order = %s, %s, want discovered then removed", first.Type, second.Type)
	}

	hub.Publish(Event{Type: EventPluginLog, PluginID: "camera", Log: &pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "after non-log loss"}})
	if event := receiveEvent(t, events); event.Dropped != 0 {
		t.Fatalf("log after non-log loss dropped = %d, want 0", event.Dropped)
	}

	for _, message := range []string{"one", "two", "three"} {
		hub.Publish(Event{Type: EventPluginLog, PluginID: "camera", Log: &pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: message}})
	}
	_ = receiveEvent(t, events)
	_ = receiveEvent(t, events)
	hub.Publish(Event{Type: EventPluginLog, PluginID: "camera", Log: &pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "four"}})
	if event := receiveEvent(t, events); event.Dropped != 1 {
		t.Fatalf("next accepted log dropped = %d, want 1", event.Dropped)
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
	assertEventChannelClosedNow(t, second)
	hub.Close()
}

func TestEventHubConcurrentCloseReturnsAfterSubscribersAreClosed(t *testing.T) {
	hub := newEventHub(2)
	subscribers := []<-chan Event{
		hub.Subscribe(context.Background()),
		hub.Subscribe(context.Background()),
		hub.Subscribe(context.Background()),
	}
	start := make(chan struct{})
	var closers sync.WaitGroup
	for range 8 {
		closers.Add(1)
		go func() {
			defer closers.Done()
			<-start
			hub.Close()
		}()
	}
	close(start)
	closers.Wait()
	for _, subscriber := range subscribers {
		assertEventChannelClosedNow(t, subscriber)
	}
}

func TestEventHubSustainedPublishDoesNotStarveDeliveryOrCancellation(t *testing.T) {
	// Catches an unbounded publish-drain loop: continuously accepted work must
	// still leave the hub a chance to deliver and process cancellation.
	hub := newEventHub(1)
	t.Cleanup(hub.Close)
	ctx, cancel := context.WithCancel(context.Background())
	events := hub.Subscribe(ctx)

	stop := make(chan struct{})
	ready := make(chan struct{}, 4)
	var publishers sync.WaitGroup
	for range 4 {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			ready <- struct{}{}
			for {
				select {
				case <-stop:
					return
				default:
					hub.Publish(Event{Type: EventPluginStatus, PluginID: "camera", Snapshot: &RuntimeSnapshot{ID: "camera"}})
				}
			}
		}()
	}
	for range 4 {
		<-ready
	}

	_ = receiveEvent(t, events)
	cancel()
	waitEventuallyClosed(t, events)
	hub.Close()
	close(stop)
	publishers.Wait()
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

func assertEventChannelClosedNow(t *testing.T, events <-chan Event) {
	t.Helper()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("event channel produced an event after Close returned")
		}
	default:
		t.Fatal("event channel was not closed when Close returned")
	}
}

func waitEventuallyClosed(t *testing.T, events <-chan Event) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for event channel close")
		}
	}
}
