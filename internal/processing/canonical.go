package processing

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type CanonicalFrame struct {
	Sequence    uint64
	TimestampNS int64

	EyeSource        string
	ExpressionSource string

	Eye         trackingmodel.EyeSample
	Expressions trackingmodel.ExpressionSet
}
