package tracking

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type sourceState struct {
	frame             trackingmodel.TrackingFrame
	receivedAtNS      int64
	lastSequence      uint64
	lastTimestampNS   int64
	lastSourceClockNS int64
}
