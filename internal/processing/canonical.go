package processing

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type CanonicalFrame struct {
	Generation    uint64
	Revision      uint64
	ProcessedAtNS int64

	EyeSourceID        string
	ExpressionSourceID string
	LipSourceID        string

	EyeActive        bool
	ExpressionActive bool
	LipActive        bool

	Eye         trackingmodel.EyeSample
	Expressions trackingmodel.ExpressionSet
}
