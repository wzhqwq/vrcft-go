package application

import (
	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type planView interface {
	Generation() uint64
	Status() avatar.Status
	AvatarID() string
	ConfigID() string
	ConfigPath() string
	Source() avatar.Source
	ParameterIDs() []parameters.ParameterID
	Catalog() *osc.Catalog
	Evaluator() *evaluator.Plan
	SubscriptionFor(trackingmodel.Capability) (pluginapi.Subscription, bool)
}

type activation struct {
	plan planView
	err  error
}

type activationPlanner interface {
	Activate(string) activation
}

type avatarPlannerAdapter struct {
	planner *avatar.Planner
}

func newActivationPlanner(planner *avatar.Planner) activationPlanner {
	return &avatarPlannerAdapter{planner: planner}
}

func (adapter *avatarPlannerAdapter) Activate(avatarID string) activation {
	result := adapter.planner.Activate(avatarID)
	return activation{plan: result.Plan, err: result.Err}
}

var _ planView = (*avatar.Plan)(nil)
var _ activationPlanner = (*avatarPlannerAdapter)(nil)
