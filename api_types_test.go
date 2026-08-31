package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
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

func TestPluginListRejectsMalformedAndOversizedPublicSnapshots(t *testing.T) {
	validID := strings.Repeat("a", 256)
	validVersion := "1.2.3+" + strings.Repeat("a", 250)
	valid := plugins.RuntimeSnapshot{
		ID: validID, Name: strings.Repeat("n", 256), Description: strings.Repeat("d", 4096),
		Version: validVersion, Capabilities: trackingmodel.CapabilityEye,
	}
	api := attachedPluginsAPI(t, context.Background(), &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{valid}, configs: map[string]pluginapi.Config{},
	})
	if got := api.List(); got.Problem != nil || len(got.Plugins) != 1 || got.Plugins[0].ID != validID {
		t.Fatalf("exact public descriptor bounds = %+v", got)
	}

	api = attachedPluginsAPI(t, context.Background(), &fakePluginsBackend{
		snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.alpha", Name: strings.Repeat("n", 257)}}, configs: map[string]pluginapi.Config{},
	})
	if got := api.List(); got.Problem == nil || got.Problem.Code != ProblemValidation || len(got.Plugins) != 0 {
		t.Fatalf("oversized snapshot = %+v", got)
	}
}

func TestPluginListCapsOversizedSnapshotListsDeterministically(t *testing.T) {
	snapshots := make([]plugins.RuntimeSnapshot, 1025)
	for index := range snapshots {
		snapshots[index] = plugins.RuntimeSnapshot{ID: fmt.Sprintf("vendor.%04d", 1024-index), Name: "Plugin", Version: "1.0.0"}
	}
	api := attachedPluginsAPI(t, context.Background(), &fakePluginsBackend{snapshots: snapshots, configs: map[string]pluginapi.Config{}})
	got := api.List()
	if got.Problem == nil || got.Problem.Code != ProblemValidation || len(got.Plugins) != 1024 {
		t.Fatalf("oversized plugin list = %+v", got)
	}
	if got.Plugins[0].ID != "vendor.0000" || got.Plugins[len(got.Plugins)-1].ID != "vendor.1023" {
		t.Fatalf("plugin list cap was not deterministic: first=%q last=%q", got.Plugins[0].ID, got.Plugins[len(got.Plugins)-1].ID)
	}
}

func TestPluginListRejectsAllTextBoundariesAndDoesNotChurnIdenticalDiagnostics(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	for _, test := range []struct {
		name     string
		snapshot plugins.RuntimeSnapshot
	}{
		{name: "ID plus one", snapshot: plugins.RuntimeSnapshot{ID: strings.Repeat("a", 257)}},
		{name: "name plus one", snapshot: plugins.RuntimeSnapshot{ID: "vendor.alpha", Name: strings.Repeat("n", 257)}},
		{name: "description plus one", snapshot: plugins.RuntimeSnapshot{ID: "vendor.alpha", Description: strings.Repeat("d", 4097)}},
		{name: "version plus one", snapshot: plugins.RuntimeSnapshot{ID: "vendor.alpha", Version: strings.Repeat("v", 257)}},
		{name: "invalid UTF-8", snapshot: plugins.RuntimeSnapshot{ID: "vendor.alpha", Name: invalidUTF8}},
		{name: "unknown state", snapshot: plugins.RuntimeSnapshot{ID: "vendor.alpha", State: plugins.State("future-state")}},
		{name: "unencodable time", snapshot: plugins.RuntimeSnapshot{ID: "vendor.alpha", StartedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakePluginsBackend{snapshots: []plugins.RuntimeSnapshot{{ID: "vendor.ok"}, test.snapshot}, configs: map[string]pluginapi.Config{}}
			api := attachedPluginsAPI(t, context.Background(), fake)
			first := api.List()
			if first.Problem == nil || first.Problem.Code != ProblemValidation || first.Problem.Field != "plugins" || len(first.Plugins) != 1 {
				t.Fatalf("bounded snapshot list = %+v", first)
			}
			if _, err := json.Marshal(first); err != nil {
				t.Fatalf("bounded snapshot response cannot be encoded: %v", err)
			}
			api.refreshBackend(fake, api.generation, false)
			if got := api.List(); got.Revision != first.Revision {
				t.Fatalf("identical invalid aggregate churned revision: %d -> %d", first.Revision, got.Revision)
			}
		})
	}

	got := pluginDTO(plugins.RuntimeSnapshot{ID: "vendor.alpha", LastError: invalidUTF8 + strings.Repeat("界", 300)})
	if !utf8.ValidString(got.LastError) || len(got.LastError) > 512 {
		t.Fatalf("last error public boundary = %q", got.LastError)
	}
}
