package pluginapi

import (
	"errors"
	"strings"
	"time"
)

type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

func (l LogLevel) Validate() error {
	switch l {
	case LogDebug, LogInfo, LogWarn, LogError:
		return nil
	default:
		return errors.New("LogLevel is unknown")
	}
}

type LogEntry struct {
	Sequence uint64
	Time     time.Time
	PluginID string
	Level    LogLevel
	Message  string
}

func (e LogEntry) Validate() error {
	if err := e.Level.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(e.Message) == "" {
		return errors.New("LogEntry.Message must be nonblank")
	}
	return nil
}
