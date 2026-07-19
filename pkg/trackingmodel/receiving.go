package trackingmodel

type ReceivedFrame struct {
	PluginID   string
	ReceivedAt int64

	Frame TrackingFrame
}
