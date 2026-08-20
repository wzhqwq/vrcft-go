package avatar

import (
	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const knownCapabilities = trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip

type trackingRequirements struct {
	capabilities trackingmodel.Capability
	eye          trackingmodel.EyeValid
	expressions  trackingmodel.ExpressionMask
}

func requirementsFromInputs(inputs parameterdeps.Inputs) trackingRequirements {
	required := trackingRequirements{
		eye:         inputs.RequiredEyeValid(),
		expressions: inputs.Expressions.Normalize(),
	}
	if required.eye != 0 || inputs.Active.Has(parameterdeps.ActiveStateEyeTracking) {
		required.capabilities |= trackingmodel.CapabilityEye
	}
	if !required.expressions.IsZero() || inputs.Active.Has(parameterdeps.ActiveStateExpressionTracking) {
		required.capabilities |= trackingmodel.CapabilityExpression
	}
	if inputs.Active.Has(parameterdeps.ActiveStateLipTracking) {
		required.capabilities |= trackingmodel.CapabilityLip
	}
	return required
}
