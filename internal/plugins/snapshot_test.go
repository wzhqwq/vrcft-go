package plugins

import (
	"reflect"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestRuntimeSnapshotPinsStatesAndFields(t *testing.T) {
	// Catches accidental state spelling changes and omission of an approved
	// observation field from the host-facing snapshot.
	wantStates := []State{
		"disabled",
		"stopped",
		"starting",
		"handshaking",
		"running",
		"stopping",
		"backoff",
		"crashed",
		"unresponsive",
		"incompatible",
	}
	gotStates := []State{
		StateDisabled,
		StateStopped,
		StateStarting,
		StateHandshaking,
		StateRunning,
		StateStopping,
		StateBackoff,
		StateCrashed,
		StateUnresponsive,
		StateIncompatible,
	}
	if !reflect.DeepEqual(gotStates, wantStates) {
		t.Fatalf("states = %#v, want %#v", gotStates, wantStates)
	}

	now := time.Unix(100, 0)
	snapshot := RuntimeSnapshot{
		ID:                     "camera",
		Name:                   "Camera",
		Description:            "Camera runtime",
		Version:                "1.2.3",
		Capabilities:           trackingmodel.CapabilityEye,
		Enabled:                true,
		Active:                 true,
		State:                  StateRunning,
		PID:                    42,
		ConfigRevision:         3,
		SubscriptionGeneration: 4,
		StartedAt:              now,
		LastHeartbeatAt:        now.Add(time.Second),
		LastFrameAt:            now.Add(2 * time.Second),
		NextRestartAt:          now.Add(3 * time.Second),
		FrameRate:              90.5,
		ConsecutiveFailures:    2,
		RestartCount:           7,
		LastError:              "sanitized",
	}
	if snapshot.ID != "camera" || snapshot.Name != "Camera" || snapshot.Description != "Camera runtime" ||
		snapshot.Version != "1.2.3" ||
		snapshot.Capabilities != trackingmodel.CapabilityEye || !snapshot.Enabled || !snapshot.Active ||
		snapshot.State != StateRunning || snapshot.PID != 42 || snapshot.ConfigRevision != 3 ||
		snapshot.SubscriptionGeneration != 4 || snapshot.StartedAt != now ||
		snapshot.LastHeartbeatAt != now.Add(time.Second) || snapshot.LastFrameAt != now.Add(2*time.Second) ||
		snapshot.NextRestartAt != now.Add(3*time.Second) || snapshot.FrameRate != 90.5 ||
		snapshot.ConsecutiveFailures != 2 || snapshot.RestartCount != 7 ||
		snapshot.LastError != "sanitized" {
		t.Fatalf("RuntimeSnapshot lost an approved field: %+v", snapshot)
	}
}

func TestRuntimeSnapshotEventContractContainsNoFrames(t *testing.T) {
	// Catches reintroducing high-frequency frame data into the bounded event
	// system, where it could retain frames or create unbounded pressure.
	eventType := reflect.TypeOf(Event{})
	framePointer := reflect.TypeOf((*trackingmodel.TrackingFrame)(nil))
	for i := 0; i < eventType.NumField(); i++ {
		field := eventType.Field(i)
		if field.Type == framePointer {
			t.Fatalf("Event field %q retains *trackingmodel.TrackingFrame", field.Name)
		}
	}
	statusField, exists := eventType.FieldByName("Status")
	if !exists || statusField.Type != reflect.TypeOf((*pluginapi.DeviceStatus)(nil)) {
		t.Fatalf("Event.Status field = %v/%v, want *pluginapi.DeviceStatus", statusField.Type, exists)
	}
	for _, event := range []EventType{
		EventPluginDiscovered,
		EventPluginRemoved,
		EventPluginStateChanged,
		EventPluginStatus,
		EventPluginLog,
	} {
		if event == "plugin_frame" {
			t.Fatal("frame event must not exist")
		}
	}
}
