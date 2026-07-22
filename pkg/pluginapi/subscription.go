package pluginapi

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type Subscription struct {
	Generation   uint64
	Capabilities trackingmodel.Capability
	Eye          trackingmodel.EyeValid
	Expressions  trackingmodel.ExpressionMask
}
