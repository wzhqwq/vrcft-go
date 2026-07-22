package plugins

import (
	"context"
	"encoding/json"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type Manager interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error

	List() []RuntimeSnapshot
	Get(id string) (RuntimeSnapshot, bool)

	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) error

	UpdateConfig(
		ctx context.Context,
		id string,
		config json.RawMessage,
	) error

	Subscribe() <-chan Event
}

type EventType string

const (
	EventPluginDiscovered EventType = "plugin_discovered"
	EventPluginChanged    EventType = "plugin_changed"
	EventPluginRemoved    EventType = "plugin_removed"
	EventPluginLog        EventType = "plugin_log"
	EventPluginFrame      EventType = "plugin_frame"
)

type Event struct {
	Sequence uint64
	Time     time.Time

	Type     EventType
	PluginID string

	Snapshot *RuntimeSnapshot
	Log      *pluginapi.LogEntry
	Frame    *trackingmodel.TrackingFrame
}

type FrameSink interface {
	Submit(pluginID string, frame trackingmodel.TrackingFrame)
}
