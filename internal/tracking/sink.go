package tracking

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type FrameSubmitter interface {
	Submit(string, uint64, trackingmodel.TrackingFrame) error
}

type PluginFrameSink struct {
	target FrameSubmitter
}

func NewPluginFrameSink(target FrameSubmitter) PluginFrameSink {
	return PluginFrameSink{target: target}
}

func (s PluginFrameSink) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) {
	if s.target != nil {
		_ = s.target.Submit(pluginID, generation, frame)
	}
}
