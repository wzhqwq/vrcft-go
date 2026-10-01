package application

import (
	"sort"

	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
	"github.com/wzhqwq/vrcft-go/internal/processing"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type ParameterDriveStatus struct {
	Name   string
	Driven bool
}

type driveEntry struct {
	id     parameters.ParameterID
	name   string
	inputs parameterdeps.Inputs
	value  parameters.ValueType
}

type parameterDriveProbe struct{ entries []driveEntry }

func newParameterDriveProbe(ids []parameters.ParameterID) (parameterDriveProbe, error) {
	ordered := append([]parameters.ParameterID(nil), ids...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	probe := parameterDriveProbe{entries: make([]driveEntry, 0, len(ordered))}
	for _, id := range ordered {
		if len(probe.entries) > 0 && probe.entries[len(probe.entries)-1].id == id {
			continue
		}
		inputs, err := parameterdeps.ResolveLeaves(id)
		if err != nil {
			return parameterDriveProbe{}, err
		}
		definition, ok := parameters.Definition(id)
		if !ok {
			return parameterDriveProbe{}, evaluator.ErrUnknownParameter
		}
		probe.entries = append(probe.entries, driveEntry{id: id, name: definition.OSCName, inputs: inputs, value: definition.ValueType})
	}
	return probe, nil
}

func (p parameterDriveProbe) Evaluate(frame tracking.MergedFrame, canonical processing.CanonicalFrame, values evaluator.Snapshot) []ParameterDriveStatus {
	result := make([]ParameterDriveStatus, len(p.entries))
	for index, entry := range p.entries {
		result[index] = ParameterDriveStatus{Name: entry.name}
		inputs := entry.inputs
		needEye := inputs.Eye != 0 || inputs.Active.Has(parameterdeps.ActiveStateEyeTracking)
		needExpression := !inputs.Expressions.IsZero() || inputs.Active.Has(parameterdeps.ActiveStateExpressionTracking)
		needLip := inputs.Active.Has(parameterdeps.ActiveStateLipTracking)
		if needEye && (!canonical.EyeActive || !frame.Capabilities.Has(trackingmodel.CapabilityEye) || frame.EyeSourceID == "") {
			continue
		}
		if needExpression && (!canonical.ExpressionActive || !frame.Capabilities.Has(trackingmodel.CapabilityExpression) || frame.ExpressionSourceID == "") {
			continue
		}
		if needLip && (!canonical.LipActive || !frame.Capabilities.Has(trackingmodel.CapabilityLip) || frame.LipSourceID == "") {
			continue
		}
		requiredEye := inputs.RequiredEyeValid()
		if frame.Eye.Valid&requiredEye != requiredEye {
			continue
		}
		if frame.Expressions.Valid.Intersect(inputs.Expressions) != inputs.Expressions {
			continue
		}
		switch entry.value {
		case parameters.ValueFloat:
			_, result[index].Driven = values.Float(entry.id)
		case parameters.ValueBool:
			_, result[index].Driven = values.Bool(entry.id)
		}
	}
	return result
}
