package pluginapi

import "time"

type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

type LogEntry struct {
	Sequence uint64
	Time     time.Time
	PluginID string
	Level    LogLevel
	Message  string
}

type LogStore interface {
	Append(entry LogEntry)
	List(pluginID string, after uint64, limit int) []LogEntry
}
