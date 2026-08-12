package processing

import (
	"fmt"

	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const knownCapabilities = trackingmodel.CapabilityEye |
	trackingmodel.CapabilityExpression |
	trackingmodel.CapabilityLip

func validateMergedInput(frame tracking.MergedFrame, nowNS int64) error {
	if frame.Generation == 0 {
		return fmt.Errorf("generation must be positive: %w", ErrInvalidInput)
	}
	if frame.Sequence == 0 {
		return fmt.Errorf("revision must be positive: %w", ErrInvalidInput)
	}
	if nowNS < 0 {
		return fmt.Errorf("processing time must be non-negative: %w", ErrInvalidInput)
	}
	if frame.UpdatedAtNS < 0 || frame.UpdatedAtNS > nowNS {
		return fmt.Errorf("merged timestamp %d outside [0,%d]: %w", frame.UpdatedAtNS, nowNS, ErrInvalidInput)
	}
	if frame.Capabilities&^knownCapabilities != 0 {
		return fmt.Errorf("unknown capability bits %d: %w", frame.Capabilities&^knownCapabilities, ErrInvalidInput)
	}

	if err := validateGroup(
		"Eye", frame.Capabilities.Has(trackingmodel.CapabilityEye), frame.EyeSourceID,
		frame.EyeUpdatedAtNS, nowNS, frame.Eye != (trackingmodel.EyeSample{}),
	); err != nil {
		return err
	}
	if err := validateGroup(
		"Expression", frame.Capabilities.Has(trackingmodel.CapabilityExpression), frame.ExpressionSourceID,
		frame.ExpressionUpdatedAtNS, nowNS, frame.Expressions != (trackingmodel.ExpressionSet{}),
	); err != nil {
		return err
	}
	if err := validateGroup(
		"Lip", frame.Capabilities.Has(trackingmodel.CapabilityLip), frame.LipSourceID,
		frame.LipUpdatedAtNS, nowNS, false,
	); err != nil {
		return err
	}

	modelFrame := trackingmodel.TrackingFrame{
		Capabilities: frame.Capabilities,
		Eye:          frame.Eye,
		Expressions:  frame.Expressions,
	}
	if err := modelFrame.Validate(); err != nil {
		return fmt.Errorf("merged payload: %v: %w", err, ErrInvalidInput)
	}
	return nil
}

func validateGroup(name string, capable bool, source string, updatedAtNS, nowNS int64, hasData bool) error {
	if capable {
		if source == "" {
			return fmt.Errorf("%s capability requires source: %w", name, ErrInvalidInput)
		}
		if updatedAtNS <= 0 || updatedAtNS > nowNS {
			return fmt.Errorf("%s timestamp %d outside (0,%d]: %w", name, updatedAtNS, nowNS, ErrInvalidInput)
		}
		return nil
	}
	if source != "" || updatedAtNS != 0 || hasData {
		return fmt.Errorf("absent %s capability retained source, timestamp, or data: %w", name, ErrInvalidInput)
	}
	return nil
}
