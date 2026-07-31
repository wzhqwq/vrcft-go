package plugins

import (
	"math"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func TestSaturatingAddUint64NeverWraps(t *testing.T) {
	for _, test := range []struct {
		name        string
		left, right uint64
		want        uint64
	}{
		{name: "below limit", left: math.MaxUint64 - 2, right: 1, want: math.MaxUint64 - 1},
		{name: "reaches limit", left: math.MaxUint64 - 2, right: 2, want: math.MaxUint64},
		{name: "saturates past limit", left: math.MaxUint64 - 2, right: 3, want: math.MaxUint64},
		{name: "stays saturated", left: math.MaxUint64, right: 1, want: math.MaxUint64},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := saturatingAddUint64(test.left, test.right); got != test.want {
				t.Fatalf("saturatingAddUint64(%d, %d) = %d, want %d", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestLogLossCompositionSaturatesAcrossBoundaries(t *testing.T) {
	const pluginID = "camera"
	hub := &eventHub{publish: make(chan Event, 1), done: make(chan struct{})}
	manager := &pluginManager{
		options:     DefaultOptions(),
		events:      hub,
		lifecycle:   managerStarted,
		supervisors: map[string]pluginSupervisor{pluginID: nil},
	}
	managerPublish := manager.supervisorConfig(
		InstalledPlugin{Manifest: Manifest{ID: pluginID}},
		PluginPreference{},
	).PublishLog
	supervisor := &serializedPluginSupervisor{
		config:   pluginSupervisorConfig{PublishLog: managerPublish},
		callback: make(chan supervisorCallback, 1),
		done:     make(chan struct{}),
	}
	callbacks := supervisor.sessionCallbacks(1)
	state := supervisorLoopState{
		snapshot:   RuntimeSnapshot{State: StateRunning},
		session:    newSupervisorTestSession(),
		instanceID: 1,
	}

	supervisor.callback <- supervisorCallback{kind: supervisorHeartbeat, instanceID: 1}
	callbacks.Log(1, observedPluginLog{
		Entry:   pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "lost at supervisor"},
		Dropped: math.MaxUint64 - 10,
	})
	<-supervisor.callback
	hub.publish <- Event{Type: EventPluginStatus, PluginID: pluginID}
	callbacks.Log(1, observedPluginLog{
		Entry:   pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "lost at manager"},
		Dropped: 5,
	})
	supervisor.handleCallback(&state, <-supervisor.callback)
	<-hub.publish
	callbacks.Log(1, observedPluginLog{
		Entry:   pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "lost at subscriber"},
		Dropped: 2,
	})
	supervisor.handleCallback(&state, <-supervisor.callback)
	managerEvent := <-hub.publish
	managerEvent.Sequence = 2

	subscriber := &eventSubscriber{states: make(map[string]Event)}
	subscriber.enqueue(Event{
		Sequence: 1,
		Type:     EventPluginLog,
		PluginID: pluginID,
		Log:      &pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "retained"},
	}, 1)
	subscriber.enqueue(managerEvent, 1)
	retained, ok := subscriber.next()
	if !ok {
		t.Fatal("subscriber retained no first log")
	}
	subscriber.pop(retained)
	subscriber.enqueue(Event{
		Sequence: 3,
		Type:     EventPluginLog,
		PluginID: pluginID,
		Log:      &pluginapi.LogEntry{Level: pluginapi.LogInfo, Message: "delivered"},
		Dropped:  5,
	}, 1)
	delivered, ok := subscriber.next()
	if !ok || delivered.Dropped != math.MaxUint64 {
		t.Fatalf("composed Dropped = %d, want MaxUint64", delivered.Dropped)
	}
}
