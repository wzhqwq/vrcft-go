package processing

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

// ChannelID identifies one scalar value processed by the pipeline.
type ChannelID uint16

const (
	ChannelEyeLeftGazeX ChannelID = iota + 1
	ChannelEyeLeftGazeY
	ChannelEyeRightGazeX
	ChannelEyeRightGazeY
	ChannelEyeLeftOpenness
	ChannelEyeRightOpenness
	ChannelEyeLeftPupilDiameter
	ChannelEyeRightPupilDiameter
	ChannelEyeLeftPupilDilation
	ChannelEyeRightPupilDilation
	channelExpressionBase
)

const channelCount = int(channelExpressionBase) + int(trackingmodel.ExpressionCount) - 1

// ExpressionChannel returns the channel corresponding to id.
func ExpressionChannel(id trackingmodel.ExpressionID) (ChannelID, bool) {
	if id >= trackingmodel.ExpressionCount {
		return 0, false
	}
	return channelExpressionBase + ChannelID(id), true
}

// ExpressionID returns the expression corresponding to this channel.
func (id ChannelID) ExpressionID() (trackingmodel.ExpressionID, bool) {
	if id < channelExpressionBase || id >= ChannelID(channelCount+1) {
		return 0, false
	}
	return trackingmodel.ExpressionID(id - channelExpressionBase), true
}

// AllChannels returns every stable processing channel in ID order.
func AllChannels() []ChannelID {
	channels := make([]ChannelID, channelCount)
	for i := range channels {
		channels[i] = ChannelID(i + 1)
	}
	return channels
}

func knownChannel(id ChannelID) bool {
	return id >= ChannelEyeLeftGazeX && id <= ChannelID(channelCount)
}
