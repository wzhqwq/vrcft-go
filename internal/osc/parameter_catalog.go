package osc

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

var (
	ErrInvalidParameterID   = errors.New("invalid parameter ID")
	ErrDuplicateParameterID = errors.New("duplicate parameter ID")
	ErrDuplicateOSCName     = errors.New("duplicate OSC name")
	ErrEmptyOSCName         = errors.New("empty OSC name")
	ErrInvalidOSCName       = errors.New("invalid OSC name")
	ErrUnsupportedValueType = errors.New("unsupported value type")
	ErrUnsupportedEncoding  = errors.New("unsupported encoding")
	ErrInvalidRange         = errors.New("invalid value range")
	ErrBinaryRequiresFloat  = errors.New("binary encoding requires float value")
	ErrBinaryRequiresRange  = errors.New("binary encoding requires numeric range")
)

type DefinitionError struct {
	Index   int
	ID      parameters.ParameterID
	OSCName string
	Field   string
	Err     error
}

func (e *DefinitionError) Error() string {
	return fmt.Sprintf(
		"invalid parameter definition at index %d, id=%d, oscName=%q, field=%s: %v",
		e.Index,
		e.ID,
		e.OSCName,
		e.Field,
		e.Err,
	)
}

func (e *DefinitionError) Unwrap() error { return e.Err }

type ParameterSpec struct {
	ID        parameters.ParameterID
	OSCName   string
	Kind      parameters.ParameterKind
	ValueType parameters.ValueType
	Encodings parameters.Encoding
	Range     parameters.ValueRange
	HasRange  bool
	Unit      string
}

func (s ParameterSpec) SupportsDirect() bool {
	switch s.ValueType {
	case parameters.ValueFloat:
		return s.Encodings.Has(parameters.EncodingFloat)
	case parameters.ValueBool:
		return s.Encodings.Has(parameters.EncodingBool)
	default:
		return false
	}
}

func (s ParameterSpec) SupportsBinary() bool {
	return s.ValueType == parameters.ValueFloat && s.Encodings.Has(parameters.EncodingBinary)
}

func (s ParameterSpec) Signed() bool {
	return s.HasRange && s.Range.Min < 0
}

func (s ParameterSpec) Clamp(value float32) float32 {
	if !s.HasRange {
		return value
	}
	if value < s.Range.Min {
		return s.Range.Min
	}
	if value > s.Range.Max {
		return s.Range.Max
	}
	return value
}

type AddressMatch struct {
	ID     parameters.ParameterID
	Prefix string
}

type BinaryAddressMatch struct {
	ID       parameters.ParameterID
	Prefix   string
	Negative bool
	Weight   uint32
}

type ParameterCatalog struct {
	byID      []ParameterSpec
	byOSCName map[string]parameters.ParameterID
}

func NewParameterCatalog(definitions []parameters.ParameterDefinition) (*ParameterCatalog, error) {
	catalog := &ParameterCatalog{
		byID:      make([]ParameterSpec, len(definitions)),
		byOSCName: make(map[string]parameters.ParameterID, len(definitions)),
	}
	seenIDs := make([]bool, len(definitions))

	for index, definition := range definitions {
		idIndex := int(definition.ID)
		if idIndex < 0 || idIndex >= len(definitions) {
			return nil, definitionError(index, definition, "ID", ErrInvalidParameterID)
		}
		if seenIDs[idIndex] {
			return nil, definitionError(index, definition, "ID", ErrDuplicateParameterID)
		}
		if idIndex != index {
			return nil, definitionError(index, definition, "ID", fmt.Errorf(
				"%w: expected %d at index %d, got %d",
				ErrInvalidParameterID,
				index,
				index,
				idIndex,
			))
		}

		spec, err := compileParameterDefinition(definition)
		if err != nil {
			return nil, definitionError(index, definition, "definition", err)
		}
		if previous, exists := catalog.byOSCName[spec.OSCName]; exists {
			return nil, definitionError(index, definition, "OSCName", fmt.Errorf(
				"%w: already used by parameter ID %d",
				ErrDuplicateOSCName,
				previous,
			))
		}

		seenIDs[idIndex] = true
		catalog.byID[idIndex] = spec
		catalog.byOSCName[spec.OSCName] = spec.ID
	}

	return catalog, nil
}

func (c *ParameterCatalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.byID)
}

func (c *ParameterCatalog) Spec(id parameters.ParameterID) (ParameterSpec, bool) {
	if c == nil {
		return ParameterSpec{}, false
	}
	index := int(id)
	if index < 0 || index >= len(c.byID) {
		return ParameterSpec{}, false
	}
	return c.byID[index], true
}

func (c *ParameterCatalog) LookupOSCName(name string) (parameters.ParameterID, bool) {
	if c == nil {
		return 0, false
	}
	id, ok := c.byOSCName[strings.Trim(name, "/")]
	return id, ok
}

func (c *ParameterCatalog) Specs() []ParameterSpec {
	if c == nil {
		return nil
	}
	result := make([]ParameterSpec, len(c.byID))
	copy(result, c.byID)
	return result
}

func (c *ParameterCatalog) ResolveAddress(address string) (AddressMatch, bool) {
	normalized := normalizeParameterAddress(address)
	if normalized == "" {
		return AddressMatch{}, false
	}

	parts := strings.Split(normalized, "/")
	for index := 0; index < len(parts); index++ {
		candidate := strings.Join(parts[index:], "/")
		if id, ok := c.byOSCName[candidate]; ok {
			return AddressMatch{
				ID:     id,
				Prefix: strings.Join(parts[:index], "/"),
			}, true
		}
	}
	return AddressMatch{}, false
}

func (c *ParameterCatalog) ResolveBinaryAddress(address string) (BinaryAddressMatch, bool) {
	normalized := normalizeParameterAddress(address)
	if normalized == "" {
		return BinaryAddressMatch{}, false
	}

	parts := strings.Split(normalized, "/")
	for index := 0; index < len(parts); index++ {
		candidate := strings.Join(parts[index:], "/")
		prefix := strings.Join(parts[:index], "/")

		if strings.HasSuffix(candidate, "Negative") {
			name := strings.TrimSuffix(candidate, "Negative")
			if id, ok := c.byOSCName[name]; ok {
				spec := c.byID[int(id)]
				if spec.SupportsBinary() && spec.Signed() {
					return BinaryAddressMatch{ID: id, Prefix: prefix, Negative: true}, true
				}
			}
		}

		name, weight, ok := splitBinaryWeight(candidate)
		if !ok {
			continue
		}
		id, exists := c.byOSCName[name]
		if !exists || !c.byID[int(id)].SupportsBinary() {
			continue
		}
		return BinaryAddressMatch{ID: id, Prefix: prefix, Weight: weight}, true
	}

	return BinaryAddressMatch{}, false
}

func compileParameterDefinition(definition parameters.ParameterDefinition) (ParameterSpec, error) {
	oscName := strings.TrimSpace(definition.OSCName)
	oscName = strings.Trim(oscName, "/")
	if oscName == "" {
		return ParameterSpec{}, ErrEmptyOSCName
	}
	if strings.Contains(oscName, "//") || strings.HasPrefix(oscName, "avatar/parameters/") {
		return ParameterSpec{}, ErrInvalidOSCName
	}

	spec := ParameterSpec{
		ID:        definition.ID,
		OSCName:   oscName,
		Kind:      definition.Kind,
		ValueType: definition.ValueType,
		Encodings: definition.Encodings,
		Range:     definition.Range,
		HasRange:  definition.HasRange,
		Unit:      definition.Unit,
	}

	if err := validateParameterSpec(spec); err != nil {
		return ParameterSpec{}, err
	}
	return spec, nil
}

func validateParameterSpec(spec ParameterSpec) error {
	switch spec.ValueType {
	case parameters.ValueFloat:
		if spec.Encodings.Has(parameters.EncodingBool) {
			return fmt.Errorf("%w: float parameter cannot use bool encoding", ErrUnsupportedEncoding)
		}
		if !spec.Encodings.Has(parameters.EncodingFloat) && !spec.Encodings.Has(parameters.EncodingBinary) {
			return ErrUnsupportedEncoding
		}
	case parameters.ValueBool:
		if !spec.Encodings.Has(parameters.EncodingBool) || spec.Encodings.Has(parameters.EncodingFloat) || spec.Encodings.Has(parameters.EncodingBinary) {
			return fmt.Errorf("%w: bool parameter requires only bool encoding", ErrUnsupportedEncoding)
		}
	default:
		return ErrUnsupportedValueType
	}

	if spec.HasRange {
		if math.IsNaN(float64(spec.Range.Min)) || math.IsNaN(float64(spec.Range.Max)) ||
			math.IsInf(float64(spec.Range.Min), 0) || math.IsInf(float64(spec.Range.Max), 0) ||
			spec.Range.Min > spec.Range.Max {
			return ErrInvalidRange
		}
	}

	if spec.Encodings.Has(parameters.EncodingBinary) {
		if spec.ValueType != parameters.ValueFloat {
			return ErrBinaryRequiresFloat
		}
		if !spec.HasRange {
			return ErrBinaryRequiresRange
		}
	}
	return nil
}

func splitBinaryWeight(candidate string) (string, uint32, bool) {
	index := len(candidate)
	for index > 0 && candidate[index-1] >= '0' && candidate[index-1] <= '9' {
		index--
	}
	if index == len(candidate) || index == 0 {
		return "", 0, false
	}
	parsed, err := strconv.ParseUint(candidate[index:], 10, 32)
	if err != nil || parsed == 0 {
		return "", 0, false
	}
	weight := uint32(parsed)
	if weight&(weight-1) != 0 {
		return "", 0, false
	}
	return candidate[:index], weight, true
}

func normalizeParameterAddress(address string) string {
	address = strings.TrimSpace(address)
	address = strings.TrimPrefix(address, "/avatar/parameters/")
	return strings.Trim(address, "/")
}

func definitionError(index int, definition parameters.ParameterDefinition, field string, err error) error {
	return &DefinitionError{
		Index:   index,
		ID:      definition.ID,
		OSCName: definition.OSCName,
		Field:   field,
		Err:     err,
	}
}
