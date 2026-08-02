package tracking

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type MergedFrame struct {
	Generation         uint64
	Sequence           uint64
	UpdatedAtNS        int64
	Capabilities       trackingmodel.Capability
	Eye                trackingmodel.EyeSample
	Expressions        trackingmodel.ExpressionSet
	EyeSourceID        string
	ExpressionSourceID string
}
