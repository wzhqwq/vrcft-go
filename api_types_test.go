package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestDTOPluginSnapshotIsAllowlistedAndOwned(t *testing.T) {
	started := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	source := plugins.RuntimeSnapshot{
		ID: "vendor.alpha", Name: "Alpha", Description: "tracking", Version: "1.2.3",
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityLip,
		Enabled:      true, Active: true, State: plugins.StateRunning, PID: 42, SessionID: 99,
		ConfigRevision: 5, SubscriptionGeneration: 12, StartedAt: started,
		FrameRate: math.Inf(1), LastError: strings.Repeat("x", 600),
	}
	got := pluginDTO(source)
	if got.ID != "vendor.alpha" || got.State != "running" || got.FrameRate != 0 {
		t.Fatalf("pluginDTO() = %#v", got)
	}
	if strings.Join(got.Capabilities, ",") != "eye,lip" {
		t.Fatalf("Capabilities = %#v, want eye, lip", got.Capabilities)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(started) || len(got.LastError) > 512 {
		t.Fatalf("pluginDTO() timestamps/error = %#v", got)
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"pid", "session", "executable", "log", "subscription", "configData"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("plugin DTO leaked %q in %s", forbidden, data)
		}
	}
	got.Capabilities[0] = "mutated"
	if source.Capabilities != trackingmodel.CapabilityEye|trackingmodel.CapabilityLip {
		t.Fatal("DTO mutation changed source snapshot")
	}
}

func TestDTOResponsesAreConcreteAndSeparateSettingsRevisions(t *testing.T) {
	updated := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	response := SettingsResponse{
		Revision: 11, UpdatedAt: updated, FileRevision: 4,
		Settings: userconfig.Candidate{Plugins: userconfig.Plugins{DevRoots: []string{"C:/plugins"}}},
	}
	if response.Revision == response.FileRevision {
		t.Fatal("test fixture must distinguish module and persisted revisions")
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"revision\":11") || !strings.Contains(string(data), "\"fileRevision\":4") {
		t.Fatalf("SettingsResponse JSON = %s", data)
	}

	var _ RuntimeResponse
	var _ PluginListResponse
	var _ PluginConfigResponse
	var _ PluginMutationResponse
	var _ SettingsResponse
	var _ SettingsValidationResponse
	var _ SettingsSaveResponse
}
