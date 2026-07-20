package specparser

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Document struct {
	SchemaVersion int    `yaml:"schema_version"`
	Counts        Counts `yaml:"counts"`

	DetailedParameters       []ParameterSpec `yaml:"detailed_parameters"`
	SimplifiedParameters     []ParameterSpec `yaml:"simplified_parameters"`
	TrackingActiveParameters []ParameterSpec `yaml:"tracking_active_parameters"`
}

type Counts struct {
	DetailedFloatParameters   int `yaml:"detailed_float_parameters"`
	SimplifiedFloatParameters int `yaml:"simplified_float_parameters"`
	FloatParametersTotal      int `yaml:"float_parameters_total"`
	StatusBoolParameters      int `yaml:"status_bool_parameters"`
	AllParametersTotal        int `yaml:"all_parameters_total"`
}

type ValueRange struct {
	Min float32 `yaml:"min"`
	Max float32 `yaml:"max"`
}

type ParameterSpec struct {
	Name               string      `yaml:"name"`
	OSCName            string      `yaml:"osc_name"`
	Group              string      `yaml:"group"`
	ValueType          string      `yaml:"value_type"`
	SupportedEncodings []string    `yaml:"supported_encodings"`
	Range              *ValueRange `yaml:"range,omitempty"`
	Unit               string      `yaml:"unit,omitempty"`
	Semantics          yaml.Node   `yaml:"semantics"`
	SendPolicy         string      `yaml:"send_policy,omitempty"`
}

type SemanticRange struct {
	Range   [2]float32 `yaml:"range"`
	Meaning string     `yaml:"meaning"`
}

type BoolSemantics struct {
	True  string
	False string
}

type ClassifiedParameter struct {
	Spec ParameterSpec
	Kind string
}

func LoadFile(path string) (*Document, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	doc, err := Load(data)
	return doc, data, err
}

func Load(data []byte) (*Document, error) {
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse parameter YAML: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (d *Document) All() []ClassifiedParameter {
	result := make([]ClassifiedParameter, 0, d.Counts.AllParametersTotal)
	for _, item := range d.DetailedParameters {
		result = append(result, ClassifiedParameter{Spec: item, Kind: "detailed"})
	}
	for _, item := range d.SimplifiedParameters {
		result = append(result, ClassifiedParameter{Spec: item, Kind: "simplified"})
	}
	for _, item := range d.TrackingActiveParameters {
		result = append(result, ClassifiedParameter{Spec: item, Kind: "tracking_active"})
	}
	return result
}

func (d *Document) Validate() error {
	if d.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d", d.SchemaVersion)
	}

	actualDetailed := len(d.DetailedParameters)
	actualSimplified := len(d.SimplifiedParameters)
	actualStatus := len(d.TrackingActiveParameters)
	actualTotal := actualDetailed + actualSimplified + actualStatus

	if actualDetailed != d.Counts.DetailedFloatParameters ||
		actualSimplified != d.Counts.SimplifiedFloatParameters ||
		actualStatus != d.Counts.StatusBoolParameters ||
		actualDetailed+actualSimplified != d.Counts.FloatParametersTotal ||
		actualTotal != d.Counts.AllParametersTotal {
		return fmt.Errorf("declared parameter counts do not match content")
	}

	seen := make(map[string]struct{}, actualTotal)
	for _, classified := range d.All() {
		item := classified.Spec
		if item.Name == "" || item.OSCName == "" || item.Group == "" {
			return fmt.Errorf("parameter has missing identity fields: %+v", item)
		}
		if _, exists := seen[item.OSCName]; exists {
			return fmt.Errorf("duplicate osc_name %q", item.OSCName)
		}
		seen[item.OSCName] = struct{}{}

		switch item.ValueType {
		case "float":
			if item.Range == nil || item.Range.Min > item.Range.Max {
				return fmt.Errorf("float parameter %q has invalid range", item.OSCName)
			}
			if _, err := item.FloatSemantics(); err != nil {
				return fmt.Errorf("parameter %q: %w", item.OSCName, err)
			}
		case "bool":
			if _, err := item.ParseBoolSemantics(); err != nil {
				return fmt.Errorf("parameter %q: %w", item.OSCName, err)
			}
		default:
			return fmt.Errorf("parameter %q has unsupported value_type %q", item.OSCName, item.ValueType)
		}
	}
	return nil
}

func (p ParameterSpec) FloatSemantics() ([]SemanticRange, error) {
	if p.Semantics.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("float semantics must be a sequence")
	}
	var result []SemanticRange
	if err := p.Semantics.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode float semantics: %w", err)
	}
	return result, nil
}

func (p ParameterSpec) ParseBoolSemantics() (BoolSemantics, error) {
	if p.Semantics.Kind != yaml.MappingNode {
		return BoolSemantics{}, fmt.Errorf("bool semantics must be a mapping")
	}
	values := make(map[string]string)
	if err := p.Semantics.Decode(&values); err != nil {
		return BoolSemantics{}, fmt.Errorf("decode bool semantics: %w", err)
	}
	result := BoolSemantics{True: values["true"], False: values["false"]}
	if result.True == "" || result.False == "" {
		return BoolSemantics{}, fmt.Errorf("bool semantics require true and false descriptions")
	}
	return result, nil
}
