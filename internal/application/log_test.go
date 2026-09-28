package application

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func TestPluginLogEventReachesApplicationLogger(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logPluginEvent(logger, plugins.Event{Type: plugins.EventPluginLog, PluginID: "eye", Log: &pluginapi.LogEntry{Level: pluginapi.LogError, Message: "camera disconnected"}, Dropped: 3})
	for _, want := range []string{"camera disconnected", "ERROR", "eye", "3 plugin log records omitted"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q: %s", want, output.String())
		}
	}
}

func TestPluginStateLoggingIgnoresTelemetryButKeepsTransitions(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	states := make(map[string]pluginLogState)
	state := plugins.RuntimeSnapshot{ID: "steamlink", State: plugins.StateRunning, SessionID: 1}
	for i := 0; i < 3; i++ {
		state.FrameRate = float64(i)
		logPluginState(logger, plugins.Event{Type: plugins.EventPluginStateChanged, PluginID: "steamlink", Snapshot: &state}, states)
	}
	state.SessionID = 2
	logPluginState(logger, plugins.Event{Type: plugins.EventPluginStateChanged, PluginID: "steamlink", Snapshot: &state}, states)
	state.LastError = "device lost"
	logPluginState(logger, plugins.Event{Type: plugins.EventPluginStateChanged, PluginID: "steamlink", Snapshot: &state}, states)
	if got := strings.Count(output.String(), `"stage":"plugin_state_changed"`); got != 3 {
		t.Fatalf("state log count = %d: %s", got, output.String())
	}
	if !strings.Contains(output.String(), "device lost") {
		t.Fatalf("lost state error: %s", output.String())
	}
}
