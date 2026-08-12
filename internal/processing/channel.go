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

func rawChannelValue(frame trackingmodel.EyeSample, expressions trackingmodel.ExpressionSet, id ChannelID) (float32, bool) {
	switch id {
	case ChannelEyeLeftGazeX:
		return frame.LeftGaze.X, frame.Valid&trackingmodel.EyeValidLeftGaze != 0
	case ChannelEyeLeftGazeY:
		return frame.LeftGaze.Y, frame.Valid&trackingmodel.EyeValidLeftGaze != 0
	case ChannelEyeRightGazeX:
		return frame.RightGaze.X, frame.Valid&trackingmodel.EyeValidRightGaze != 0
	case ChannelEyeRightGazeY:
		return frame.RightGaze.Y, frame.Valid&trackingmodel.EyeValidRightGaze != 0
	case ChannelEyeLeftOpenness:
		return frame.LeftOpenness, frame.Valid&trackingmodel.EyeValidLeftOpenness != 0
	case ChannelEyeRightOpenness:
		return frame.RightOpenness, frame.Valid&trackingmodel.EyeValidRightOpenness != 0
	case ChannelEyeLeftPupilDiameter:
		return frame.LeftPupilDiameterMM, frame.Valid&trackingmodel.EyeValidLeftPupil != 0
	case ChannelEyeRightPupilDiameter:
		return frame.RightPupilDiameterMM, frame.Valid&trackingmodel.EyeValidRightPupil != 0
	case ChannelEyeLeftPupilDilation:
		return frame.LeftPupilDilation, frame.Valid&trackingmodel.EyeValidLeftPupil != 0
	case ChannelEyeRightPupilDilation:
		return frame.RightPupilDilation, frame.Valid&trackingmodel.EyeValidRightPupil != 0
	default:
		expression, ok := id.ExpressionID()
		if !ok {
			return 0, false
		}
		return expressions.Get(expression)
	}
}
