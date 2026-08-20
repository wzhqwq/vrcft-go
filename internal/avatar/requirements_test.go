package avatar

import (
	"reflect"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestRequirementsFromInputsDerivesCapabilitiesAndDetails(t *testing.T) {
	tests := []struct {
		name   string
		inputs parameterdeps.Inputs
		want   trackingRequirements
	}{
		{
			name: "numeric eye and expression",
			inputs: parameterdeps.Inputs{
				Eye:         parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftGazeX),
				Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
			},
			want: trackingRequirements{
				capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
				eye:          trackingmodel.EyeValidLeftGaze,
				expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
			},
		},
		{
			name: "all active states",
			inputs: parameterdeps.Inputs{Active: parameterdeps.ActiveStatesOf(
				parameterdeps.ActiveStateEyeTracking,
				parameterdeps.ActiveStateExpressionTracking,
				parameterdeps.ActiveStateLipTracking,
			)},
			want: trackingRequirements{
				capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip,
			},
		},
		{
			name: "zero inputs",
			want: trackingRequirements{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := requirementsFromInputs(test.inputs); got != test.want {
				t.Fatalf("requirementsFromInputs(%#v) = %#v, want %#v", test.inputs, got, test.want)
			}
		})
	}
}

func TestPlanSubscriptionForIntersectsCapabilities(t *testing.T) {
	inputs := parameterdeps.Inputs{
		Eye:         parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftGazeX),
		Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
		Active:      parameterdeps.ActiveStatesOf(parameterdeps.ActiveStateLipTracking),
	}
	plan := &Plan{
		generation: 17,
		status:     StatusReady,
		inputs:     inputs,
		required:   requirementsFromInputs(inputs),
	}

	tests := []struct {
		name       string
		advertised trackingmodel.Capability
		want       pluginapi.Subscription
		wantOK     bool
	}{
		{
			name:       "full match",
			advertised: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip,
			want: pluginapi.Subscription{
				Generation:   17,
				Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip,
				Eye:          trackingmodel.EyeValidLeftGaze,
				Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
			},
			wantOK: true,
		},
		{
			name:       "partial eye and lip match",
			advertised: trackingmodel.CapabilityEye | trackingmodel.CapabilityLip,
			want: pluginapi.Subscription{
				Generation:   17,
				Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityLip,
				Eye:          trackingmodel.EyeValidLeftGaze,
			},
			wantOK: true,
		},
		{
			name:       "lip only",
			advertised: trackingmodel.CapabilityLip,
			want:       pluginapi.Subscription{Generation: 17, Capabilities: trackingmodel.CapabilityLip},
			wantOK:     true,
		},
		{
			name:       "unknown advertised bits ignored",
			advertised: trackingmodel.CapabilityExpression | trackingmodel.Capability(1<<12),
			want: pluginapi.Subscription{
				Generation:   17,
				Capabilities: trackingmodel.CapabilityExpression,
				Expressions:  trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
			},
			wantOK: true,
		},
		{
			name:       "no intersection",
			advertised: trackingmodel.Capability(1 << 12),
			want:       pluginapi.Subscription{},
			wantOK:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := plan.SubscriptionFor(test.advertised)
			if ok != test.wantOK || got != test.want {
				t.Fatalf("SubscriptionFor(%#x) = %#v, %t; want %#v, %t", test.advertised, got, ok, test.want, test.wantOK)
			}
			if ok {
				if err := got.Validate(true); err != nil {
					t.Fatalf("SubscriptionFor(%#x) produced invalid subscription: %v", test.advertised, err)
				}
			}
		})
	}
}

func TestPlanSubscriptionForUsesWholeGroupForActiveOnlyRequirements(t *testing.T) {
	tests := []struct {
		name       string
		inputs     parameterdeps.Inputs
		capability trackingmodel.Capability
	}{
		{
			name:       "eye",
			inputs:     parameterdeps.Inputs{Active: parameterdeps.ActiveStatesOf(parameterdeps.ActiveStateEyeTracking)},
			capability: trackingmodel.CapabilityEye,
		},
		{
			name:       "expression",
			inputs:     parameterdeps.Inputs{Active: parameterdeps.ActiveStatesOf(parameterdeps.ActiveStateExpressionTracking)},
			capability: trackingmodel.CapabilityExpression,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := &Plan{generation: 8, status: StatusReady, inputs: test.inputs, required: requirementsFromInputs(test.inputs)}
			subscription, ok := plan.SubscriptionFor(test.capability)
			if !ok || subscription.Generation != 8 || subscription.Capabilities != test.capability || subscription.Eye != 0 || !subscription.Expressions.IsZero() {
				t.Fatalf("active-only SubscriptionFor() = %#v, %t", subscription, ok)
			}
			if err := subscription.Validate(true); err != nil {
				t.Fatalf("active-only subscription invalid: %v", err)
			}
		})
	}
}

func TestPlanOwnsConstructorInputsAndAccessorResults(t *testing.T) {
	ids := []parameters.ParameterID{parameters.ParameterEyeLeftX}
	catalog := &osc.Catalog{
		Bindings: map[parameters.ParameterID]osc.ParameterBinding{
			parameters.ParameterEyeLeftX: {Direct: []osc.Endpoint{{Address: "/avatar/parameters/v2/EyeLeftX", Type: "f"}}},
		},
		RawMethods: []osc.Endpoint{{Address: "/avatar/parameters/v2/EyeLeftX", Type: "f"}},
	}
	evaluatorPlan, err := evaluator.Compile(ids)
	if err != nil {
		t.Fatal(err)
	}
	inputs := parameterdeps.Inputs{Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftGazeX)}
	plan := newReadyPlan(5, "avtr_demo", "config_demo", "C:/avatar.json", SourceAvatarConfig, ids, catalog, evaluatorPlan, inputs)

	ids[0] = parameters.ParameterJawOpen
	catalog.Bindings[parameters.ParameterEyeLeftX] = osc.ParameterBinding{}
	catalog.RawMethods[0].Address = "/mutated"

	firstIDs := plan.ParameterIDs()
	secondIDs := plan.ParameterIDs()
	if !reflect.DeepEqual(firstIDs, []parameters.ParameterID{parameters.ParameterEyeLeftX}) || len(firstIDs) == 0 || &firstIDs[0] == &secondIDs[0] {
		t.Fatalf("ParameterIDs() did not return independent owned copies: first=%#v second=%#v", firstIDs, secondIDs)
	}
	firstIDs[0] = parameters.ParameterJawOpen
	if got := plan.ParameterIDs(); !reflect.DeepEqual(got, []parameters.ParameterID{parameters.ParameterEyeLeftX}) {
		t.Fatalf("ParameterIDs() retained caller mutation: %#v", got)
	}

	firstCatalog := plan.Catalog()
	secondCatalog := plan.Catalog()
	if firstCatalog == nil || secondCatalog == nil || firstCatalog == secondCatalog {
		t.Fatalf("Catalog() = %p, %p; want independent non-nil clones", firstCatalog, secondCatalog)
	}
	firstCatalog.Bindings[parameters.ParameterEyeLeftX] = osc.ParameterBinding{}
	firstCatalog.RawMethods[0].Address = "/accessor-mutation"
	if got := plan.Catalog(); len(got.Bindings) != 1 || got.RawMethods[0].Address != "/avatar/parameters/v2/EyeLeftX" {
		t.Fatalf("Catalog() retained caller mutation: %#v", got)
	}
}

func TestPlanFailedAndEmptyReadyStatesAreInert(t *testing.T) {
	failed := newFailedPlan(9, "avtr_failed", SourceFallback, "C:/fallback.json", "fallback-id")
	if failed.Generation() != 9 || failed.Status() != StatusFailed || failed.AvatarID() != "avtr_failed" || failed.ConfigID() != "fallback-id" || failed.ConfigPath() != "C:/fallback.json" || failed.Source() != SourceFallback {
		t.Fatalf("failed diagnostics = %#v", failed)
	}
	if failed.Catalog() != nil || failed.Evaluator() != nil || len(failed.ParameterIDs()) != 0 || !failed.RequiredInputs().IsZero() {
		t.Fatalf("failed plan retained operational state: %#v", failed)
	}
	if subscription, ok := failed.SubscriptionFor(trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip); ok || subscription != (pluginapi.Subscription{}) {
		t.Fatalf("failed subscription = %#v, %t; want zero, false", subscription, ok)
	}

	emptyEvaluator, err := evaluator.Compile(nil)
	if err != nil {
		t.Fatal(err)
	}
	empty := newReadyPlan(10, "avtr_empty", "config-empty", "C:/empty.json", SourceAvatarConfig, nil, &osc.Catalog{}, emptyEvaluator, parameterdeps.Inputs{})
	if empty.Status() != StatusReady || empty.Catalog() == nil || empty.Evaluator() == nil {
		t.Fatalf("empty ready plan = %#v", empty)
	}
	if subscription, ok := empty.SubscriptionFor(trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression | trackingmodel.CapabilityLip); ok || subscription != (pluginapi.Subscription{}) {
		t.Fatalf("empty ready subscription = %#v, %t; want zero, false", subscription, ok)
	}
}
