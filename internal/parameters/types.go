package parameters

import (
	"fmt"
	"strconv"
	"strings"
)

type ParameterID uint16

type ParameterKind uint8

const (
	KindDetailed ParameterKind = iota + 1
	KindSimplified
	KindTrackingActive
)

type ValueType uint8

const (
	ValueFloat ValueType = iota + 1
	ValueBool
)

type Encoding uint8

const (
	EncodingFloat Encoding = 1 << iota
	EncodingBinary
	EncodingBool
)

func (e Encoding) Has(flag Encoding) bool { return e&flag != 0 }

type ValueRange struct {
	Min float32
	Max float32
}

type SemanticRange struct {
	Min     float32
	Max     float32
	Meaning string
}

type BoolSemantics struct {
	True  string
	False string
}

type ParameterDefinition struct {
	ID         ParameterID
	Name       string
	OSCName    string
	Group      string
	Kind       ParameterKind
	ValueType  ValueType
	Encodings  Encoding
	Range      ValueRange
	HasRange   bool
	Unit       string
	Semantics  []SemanticRange
	Bool       BoolSemantics
	SendPolicy string
}

func Definition(id ParameterID) (ParameterDefinition, bool) {
	if id >= ParameterCount {
		return ParameterDefinition{}, false
	}
	return Definitions[id], true
}

func LookupOSCName(name string) (ParameterID, bool) {
	id, ok := ParameterByOSCName[name]
	return id, ok
}

// ResolveAddress accepts either a canonical parameter name such as v2/JawOpen,
// a prefixed name such as Face/v2/JawOpen, or a complete VRChat address such as
// /avatar/parameters/Face/v2/JawOpen.
func ResolveAddress(address string) (id ParameterID, prefix string, ok bool) {
	normalized := normalizeAddress(address)
	if normalized == "" {
		return 0, "", false
	}

	if id, ok := LookupOSCName(normalized); ok {
		return id, "", true
	}

	parts := strings.Split(normalized, "/")
	for i := 1; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], "/")
		if id, ok := LookupOSCName(suffix); ok {
			return id, strings.Join(parts[:i], "/"), true
		}
	}
	return 0, "", false
}

type BinaryPart struct {
	Parameter ParameterID
	Prefix    string
	Negative  bool
	Weight    uint32
}

// ResolveBinaryAddress resolves documented binary forms such as
// Face/v2/JawXNegative and Face/v2/JawX8. Weight is zero for Negative.
func ResolveBinaryAddress(address string) (BinaryPart, bool) {
	normalized := normalizeAddress(address)
	parts := strings.Split(normalized, "/")

	for i := 0; i < len(parts); i++ {
		candidate := strings.Join(parts[i:], "/")
		prefix := strings.Join(parts[:i], "/")

		for _, def := range Definitions {
			if def.ValueType != ValueFloat || !def.Encodings.Has(EncodingBinary) {
				continue
			}

			if candidate == def.OSCName+"Negative" {
				return BinaryPart{Parameter: def.ID, Prefix: prefix, Negative: true}, true
			}
			if !strings.HasPrefix(candidate, def.OSCName) {
				continue
			}

			suffix := strings.TrimPrefix(candidate, def.OSCName)
			if suffix == "" {
				continue
			}
			weight, err := strconv.ParseUint(suffix, 10, 32)
			if err != nil || weight == 0 || weight&(weight-1) != 0 {
				continue
			}
			return BinaryPart{Parameter: def.ID, Prefix: prefix, Weight: uint32(weight)}, true
		}
	}

	return BinaryPart{}, false
}

func CanonicalAddress(id ParameterID, prefix string) (string, error) {
	def, ok := Definition(id)
	if !ok {
		return "", fmt.Errorf("unknown parameter id %d", id)
	}
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "/avatar/parameters/" + def.OSCName, nil
	}
	return "/avatar/parameters/" + prefix + "/" + def.OSCName, nil
}

func Clamp(id ParameterID, value float32) (float32, bool) {
	def, ok := Definition(id)
	if !ok || !def.HasRange {
		return value, ok
	}
	if value < def.Range.Min {
		return def.Range.Min, true
	}
	if value > def.Range.Max {
		return def.Range.Max, true
	}
	return value, true
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	address = strings.TrimPrefix(address, "/avatar/parameters/")
	return strings.Trim(address, "/")
}
