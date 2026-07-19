package tracking

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type Service interface {
	Submit(pluginID string, frame trackingmodel.TrackingFrame)

	SetRouting(config RoutingConfig)
	Routing() RoutingConfig

	LatestMerged() (MergedFrame, bool)

	SubscribeSummary() <-chan Summary
}

type Summary struct {
}
