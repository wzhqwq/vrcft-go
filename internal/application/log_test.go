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
