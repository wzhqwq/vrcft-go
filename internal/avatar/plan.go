package avatar

import (
	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type Status uint8

const (
	StatusReady Status = iota + 1
	StatusFailed
)

type Result struct {
	Plan *Plan
	Err  error
}

// Plan is the immutable control-plane state compiled for one avatar change.
type Plan struct {
	generation uint64
	status     Status
	avatarID   string
	configID   string
	configPath string
	source     Source
	ids        []parameters.ParameterID
	catalog    *osc.Catalog
	evaluator  *evaluator.Plan
	inputs     parameterdeps.Inputs
	required   trackingRequirements
}

func (p *Plan) Generation() uint64 { return p.generation }

func (p *Plan) Status() Status { return p.status }

func (p *Plan) AvatarID() string { return p.avatarID }

func (p *Plan) ConfigID() string { return p.configID }

func (p *Plan) ConfigPath() string { return p.configPath }

func (p *Plan) Source() Source { return p.source }

func (p *Plan) ParameterIDs() []parameters.ParameterID {
	return append([]parameters.ParameterID(nil), p.ids...)
}

func (p *Plan) Catalog() *osc.Catalog {
	return p.catalog.Clone()
}

func (p *Plan) Evaluator() *evaluator.Plan { return p.evaluator }

func (p *Plan) RequiredInputs() parameterdeps.Inputs { return p.inputs }

// SubscriptionFor projects this plan's requirements onto a plugin's advertised
// capabilities. It returns false instead of constructing the invalid positive-
// generation empty subscription when no known capability intersects.
func (p *Plan) SubscriptionFor(advertised trackingmodel.Capability) (pluginapi.Subscription, bool) {
	if p.status != StatusReady || p.generation == 0 {
		return pluginapi.Subscription{}, false
	}

	capabilities := p.required.capabilities & advertised & knownCapabilities
	if capabilities == 0 {
		return pluginapi.Subscription{}, false
	}

	subscription := pluginapi.Subscription{
		Generation:   p.generation,
		Capabilities: capabilities,
	}
	if capabilities.Has(trackingmodel.CapabilityEye) {
		subscription.Eye = p.required.eye
	}
	if capabilities.Has(trackingmodel.CapabilityExpression) {
		subscription.Expressions = p.required.expressions
	}
	return subscription.Normalize(), true
}

func newReadyPlan(generation uint64, avatarID, configID, configPath string, source Source, ids []parameters.ParameterID, catalog *osc.Catalog, evaluatorPlan *evaluator.Plan, inputs parameterdeps.Inputs) *Plan {
	return &Plan{
		generation: generation,
		status:     StatusReady,
		avatarID:   avatarID,
		configID:   configID,
		configPath: configPath,
		source:     source,
		ids:        append([]parameters.ParameterID(nil), ids...),
		catalog:    catalog.Clone(),
		evaluator:  evaluatorPlan,
		inputs:     inputs,
		required:   requirementsFromInputs(inputs),
	}
}

func newFailedPlan(generation uint64, avatarID string, source Source, configPath, configID string) *Plan {
	return &Plan{
		generation: generation,
		status:     StatusFailed,
		avatarID:   avatarID,
		configID:   configID,
		configPath: configPath,
		source:     source,
	}
}
