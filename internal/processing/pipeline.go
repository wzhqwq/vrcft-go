package processing

import (
	"fmt"
	"math"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const eyeChannelCount = int(channelExpressionBase) - 1

type channelState struct {
	filter         filterState
	seen           bool
	lastFreshAtNS  int64
	lastFreshValue float32
	dropoutStartNS int64
}

// Pipeline is a caller-serialized processing state machine.
type Pipeline struct {
	config   compiledConfig
	channels [channelCount]channelState

	hasLast   bool
	lastInput tracking.MergedFrame
	lastNowNS int64
}

// NewPipeline validates and compiles config into a new pipeline with empty history.
func NewPipeline(config Config) (*Pipeline, error) {
	compiled, err := compileConfig(config)
	if err != nil {
		return nil, fmt.Errorf("new pipeline: %w: %w", ErrInvalidConfig, err)
	}
	return &Pipeline{config: compiled}, nil
}

// ProcessAt evaluates frame at the supplied Host time. Calls must be serialized
// by the caller. A rejected call leaves the receiver unchanged.
func (p *Pipeline) ProcessAt(frame tracking.MergedFrame, nowNS int64) (CanonicalFrame, error) {
	if p == nil {
		return CanonicalFrame{}, fmt.Errorf("nil pipeline: %w", ErrInvalidInput)
	}
	next := *p
	out, err := next.processAt(frame, nowNS)
	if err != nil {
		return CanonicalFrame{}, err
	}
	*p = next
	return out, nil
}

func (p *Pipeline) processAt(frame tracking.MergedFrame, nowNS int64) (CanonicalFrame, error) {
	if err := validateMergedInput(frame, nowNS); err != nil {
		return CanonicalFrame{}, err
	}

	newInput, generationReset, err := p.classifyInput(frame, nowNS)
	if err != nil {
		return CanonicalFrame{}, err
	}
	if !newInput {
		p.lastNowNS = nowNS
		return p.canonical(frame, nowNS), nil
	}

	previous := p.lastInput
	if generationReset {
		p.channels = [channelCount]channelState{}
	}

	eyeReset := generationReset || frame.EyeSourceID != "" && frame.EyeSourceID != previous.EyeSourceID
	expressionReset := generationReset || frame.ExpressionSourceID != "" && frame.ExpressionSourceID != previous.ExpressionSourceID
	if eyeReset {
		p.resetChannels(0, eyeChannelCount)
	}
	if expressionReset {
		p.resetChannels(eyeChannelCount, channelCount)
	}
	if !generationReset && previous.EyeSourceID != "" && frame.EyeSourceID == "" {
		p.markUnavailable(frame.UpdatedAtNS, 0, eyeChannelCount)
	}
	if !generationReset && previous.ExpressionSourceID != "" && frame.ExpressionSourceID == "" {
		p.markUnavailable(frame.UpdatedAtNS, eyeChannelCount, channelCount)
	}

	eyeFresh := frame.Capabilities.Has(trackingmodel.CapabilityEye) &&
		(eyeReset || !p.hasLast || frame.EyeUpdatedAtNS > previous.EyeUpdatedAtNS)
	if eyeFresh {
		if err := p.ingestRange(frame, frame.EyeUpdatedAtNS, 0, eyeChannelCount); err != nil {
			return CanonicalFrame{}, err
		}
	}
	expressionFresh := frame.Capabilities.Has(trackingmodel.CapabilityExpression) &&
		(expressionReset || !p.hasLast || frame.ExpressionUpdatedAtNS > previous.ExpressionUpdatedAtNS)
	if expressionFresh {
		if err := p.ingestRange(frame, frame.ExpressionUpdatedAtNS, eyeChannelCount, channelCount); err != nil {
			return CanonicalFrame{}, err
		}
	}

	p.hasLast = true
	p.lastInput = frame
	p.lastNowNS = nowNS
	return p.canonical(frame, nowNS), nil
}

func (p *Pipeline) classifyInput(frame tracking.MergedFrame, nowNS int64) (newInput, generationReset bool, err error) {
	if !p.hasLast {
		return true, true, nil
	}
	previous := p.lastInput
	if frame.Generation < previous.Generation {
		return false, false, fmt.Errorf("generation %d precedes %d: %w", frame.Generation, previous.Generation, ErrGenerationRegression)
	}
	if frame.Generation > previous.Generation {
		if nowNS <= p.lastNowNS {
			return false, false, fmt.Errorf("new generation processing time %d not after %d: %w", nowNS, p.lastNowNS, ErrTimeRegression)
		}
		return true, true, nil
	}
	if frame.Sequence < previous.Sequence {
		return false, false, fmt.Errorf("revision %d precedes %d: %w", frame.Sequence, previous.Sequence, ErrRevisionRegression)
	}

	if frame.Sequence == previous.Sequence {
		if frame == previous {
			if nowNS < p.lastNowNS {
				return false, false, fmt.Errorf("repeat processing time %d precedes %d: %w", nowNS, p.lastNowNS, ErrTimeRegression)
			}
			return false, false, nil
		}
		if frame.Sequence != math.MaxUint64 {
			return false, false, fmt.Errorf("revision %d changed content: %w", frame.Sequence, ErrRevisionConflict)
		}
		if !saturatedFreshnessNonregressing(previous, frame) {
			return false, false, fmt.Errorf("saturated revision freshness regressed: %w", ErrRevisionConflict)
		}
	} else if previous.Sequence == math.MaxUint64 {
		return false, false, fmt.Errorf("revision advanced beyond saturation: %w", ErrRevisionConflict)
	}

	if nowNS <= p.lastNowNS {
		return false, false, fmt.Errorf("new input processing time %d not after %d: %w", nowNS, p.lastNowNS, ErrTimeRegression)
	}
	return true, false, nil
}

func saturatedFreshnessNonregressing(previous, next tracking.MergedFrame) bool {
	if next.UpdatedAtNS < previous.UpdatedAtNS {
		return false
	}
	if previous.EyeSourceID != "" && previous.EyeSourceID == next.EyeSourceID && next.EyeUpdatedAtNS < previous.EyeUpdatedAtNS {
		return false
	}
	if previous.ExpressionSourceID != "" && previous.ExpressionSourceID == next.ExpressionSourceID && next.ExpressionUpdatedAtNS < previous.ExpressionUpdatedAtNS {
		return false
	}
	if previous.LipSourceID != "" && previous.LipSourceID == next.LipSourceID && next.LipUpdatedAtNS < previous.LipUpdatedAtNS {
		return false
	}
	return true
}

func (p *Pipeline) resetChannels(start, end int) {
	for index := start; index < end; index++ {
		p.channels[index] = channelState{}
	}
}

func (p *Pipeline) markUnavailable(atNS int64, start, end int) {
	for index := start; index < end; index++ {
		p.channels[index].recordUnavailable(atNS)
	}
}

func (p *Pipeline) ingestRange(frame tracking.MergedFrame, atNS int64, start, end int) error {
	for index := start; index < end; index++ {
		id := ChannelID(index + 1)
		raw, valid := rawChannelValue(frame.Eye, frame.Expressions, id)
		if !valid {
			continue
		}
		config := p.config.channelConfig(id)
		calibrated, err := applyCalibration(config.Calibration, raw)
		if err != nil {
			return fmt.Errorf("channel %d calibration: %w: %w", id, ErrInvalidInput, err)
		}
		tuned, err := applyTuning(config.Tuning, calibrated)
		if err != nil {
			return fmt.Errorf("channel %d tuning: %w: %w", id, ErrInvalidInput, err)
		}
		filtered, err := p.channels[index].filter.apply(config.Filter, tuned, atNS)
		if err != nil {
			return fmt.Errorf("channel %d filter: %w: %w", id, ErrInvalidInput, err)
		}
		p.channels[index].recordFresh(filtered, atNS, config.Dropout)
	}
	return nil
}

func (p *Pipeline) canonical(frame tracking.MergedFrame, nowNS int64) CanonicalFrame {
	out := CanonicalFrame{
		Generation:         frame.Generation,
		Revision:           frame.Sequence,
		ProcessedAtNS:      nowNS,
		EyeSourceID:        frame.EyeSourceID,
		ExpressionSourceID: frame.ExpressionSourceID,
		LipSourceID:        frame.LipSourceID,
		EyeActive:          groupActive(frame.Capabilities.Has(trackingmodel.CapabilityEye), frame.EyeSourceID, frame.EyeUpdatedAtNS, nowNS, p.config.activeStaleAfter),
		ExpressionActive:   groupActive(frame.Capabilities.Has(trackingmodel.CapabilityExpression), frame.ExpressionSourceID, frame.ExpressionUpdatedAtNS, nowNS, p.config.activeStaleAfter),
		LipActive:          groupActive(frame.Capabilities.Has(trackingmodel.CapabilityLip), frame.LipSourceID, frame.LipUpdatedAtNS, nowNS, p.config.activeStaleAfter),
	}

	var candidates [channelCount]channelCandidate
	for index, state := range p.channels {
		id := ChannelID(index + 1)
		candidates[index].value, candidates[index].valid = state.dropoutValue(p.config.channelConfig(id).Dropout, nowNS)
	}
	projectMutualExclusion(&candidates, p.config.mutualExclusion)

	leftGaze := candidates[ChannelEyeLeftGazeX-1].valid && candidates[ChannelEyeLeftGazeY-1].valid
	rightGaze := candidates[ChannelEyeRightGazeX-1].valid && candidates[ChannelEyeRightGazeY-1].valid
	leftPupil := candidates[ChannelEyeLeftPupilDiameter-1].valid && candidates[ChannelEyeLeftPupilDilation-1].valid
	rightPupil := candidates[ChannelEyeRightPupilDiameter-1].valid && candidates[ChannelEyeRightPupilDilation-1].valid
	if leftGaze {
		out.Eye.Valid |= trackingmodel.EyeValidLeftGaze
		out.Eye.LeftGaze.X = candidates[ChannelEyeLeftGazeX-1].value
		out.Eye.LeftGaze.Y = candidates[ChannelEyeLeftGazeY-1].value
	}
	if rightGaze {
		out.Eye.Valid |= trackingmodel.EyeValidRightGaze
		out.Eye.RightGaze.X = candidates[ChannelEyeRightGazeX-1].value
		out.Eye.RightGaze.Y = candidates[ChannelEyeRightGazeY-1].value
	}
	if candidates[ChannelEyeLeftOpenness-1].valid {
		out.Eye.Valid |= trackingmodel.EyeValidLeftOpenness
		out.Eye.LeftOpenness = candidates[ChannelEyeLeftOpenness-1].value
	}
	if candidates[ChannelEyeRightOpenness-1].valid {
		out.Eye.Valid |= trackingmodel.EyeValidRightOpenness
		out.Eye.RightOpenness = candidates[ChannelEyeRightOpenness-1].value
	}
	if leftPupil {
		out.Eye.Valid |= trackingmodel.EyeValidLeftPupil
		out.Eye.LeftPupilDiameterMM = candidates[ChannelEyeLeftPupilDiameter-1].value
		out.Eye.LeftPupilDilation = candidates[ChannelEyeLeftPupilDilation-1].value
	}
	if rightPupil {
		out.Eye.Valid |= trackingmodel.EyeValidRightPupil
		out.Eye.RightPupilDiameterMM = candidates[ChannelEyeRightPupilDiameter-1].value
		out.Eye.RightPupilDilation = candidates[ChannelEyeRightPupilDilation-1].value
	}

	for id := trackingmodel.ExpressionID(0); id < trackingmodel.ExpressionCount; id++ {
		channel, _ := ExpressionChannel(id)
		candidate := candidates[channel-1]
		if candidate.valid {
			out.Expressions.Set(id, candidate.value)
		}
	}
	return out
}

func groupActive(capable bool, source string, updatedAtNS, nowNS int64, staleAfter time.Duration) bool {
	return capable && source != "" && nowNS-updatedAtNS <= int64(staleAfter)
}
