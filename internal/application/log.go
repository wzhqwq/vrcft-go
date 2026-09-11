package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

// logPluginEvent runs on control events only. The root handler retains memory
// records and queues disk output without blocking the coordinator on file I/O.
func logPluginEvent(logger *slog.Logger, event plugins.Event) {
	if logger == nil {
		return
	}
	logger = logger.With("component", "plugin", "stage", string(event.Type))
	if event.Dropped > 0 {
		logger.Warn(fmt.Sprintf("%s: %d plugin log records omitted", event.PluginID, event.Dropped))
	}
	if event.Type == plugins.EventPluginLog && event.Log != nil {
		level := slog.LevelInfo
		switch event.Log.Level {
		case pluginapi.LogDebug:
			level = slog.LevelDebug
		case pluginapi.LogWarn:
			level = slog.LevelWarn
		case pluginapi.LogError:
			level = slog.LevelError
		}
		logger.Log(context.Background(), level, event.PluginID+": "+event.Log.Message)
	} else if event.Type == plugins.EventPluginStateChanged && event.Snapshot != nil {
		message := fmt.Sprintf("%s: %s", event.PluginID, event.Snapshot.State)
		if event.Snapshot.LastError != "" {
			logger.Error(message + ": " + event.Snapshot.LastError)
		} else {
			logger.Info(message)
		}
	}
}
