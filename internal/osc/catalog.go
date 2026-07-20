package osc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

var (
	ErrConflictingOSCAddress = errors.New("conflicting OSC output address")
	ErrTooManyOSCOutputs     = errors.New("too many OSC outputs")
)

type Endpoint struct {
	Address string
	Type    string
}

type BinaryBit struct {
	Endpoint Endpoint
	Weight   uint32
}

type BinaryBinding struct {
	Negative *Endpoint
	Bits     []BinaryBit
}

type ParameterBinding struct {
	Spec   ParameterSpec
	Direct []Endpoint
	Binary []BinaryBinding
}

type outputOperation uint8

const (
	outputDirectFloat outputOperation = iota + 1
	outputDirectBool
	outputBinaryNegative
	outputBinaryBit
)

type outputBinding struct {
	Parameter   parameters.ParameterID
	Address     string
	Operation   outputOperation
	WireKind    scalarKind
	Weight      uint32
	QuantizeMax uint32
	Range       parameters.ValueRange
	HasRange    bool
	CacheIndex  uint16
}

type Catalog struct {
	Generation uint64
	UpdatedAt  time.Time
	Hash       uint64
	Bindings   map[parameters.ParameterID]ParameterBinding
	RawMethods []Endpoint
	Outputs    []outputBinding
}

func BuildCatalog(root *QueryNode, specs *ParameterCatalog, generation uint64) (*Catalog, error) {
	if root == nil {
		return nil, fmt.Errorf("nil OSCQuery root")
	}
	if specs == nil {
		return nil, fmt.Errorf("nil parameter catalog")
	}

	catalog := &Catalog{
		Generation: generation,
		UpdatedAt:  time.Now(),
		Bindings:   make(map[parameters.ParameterID]ParameterBinding),
	}
	groups := make(map[parameters.ParameterID]map[string]*binaryGroup)

	for _, method := range root.FlattenMethods() {
		if !isWritable(method) || !supportedParameterType(method.Type) {
			continue
		}
		endpoint := Endpoint{Address: method.FullPath, Type: method.Type}
		catalog.RawMethods = append(catalog.RawMethods, endpoint)

		if match, ok := specs.ResolveAddress(method.FullPath); ok {
			spec, exists := specs.Spec(match.ID)
			if exists && spec.SupportsDirect() {
				binding := catalog.Bindings[match.ID]
				binding.Spec = spec
				binding.Direct = append(binding.Direct, endpoint)
				catalog.Bindings[match.ID] = binding
			}
			continue
		}

		match, ok := specs.ResolveBinaryAddress(method.FullPath)
		if !ok {
			continue
		}
		spec, exists := specs.Spec(match.ID)
		if !exists || !spec.SupportsBinary() {
			continue
		}

		parameterGroups := groups[match.ID]
		if parameterGroups == nil {
			parameterGroups = make(map[string]*binaryGroup)
			groups[match.ID] = parameterGroups
		}
		group := parameterGroups[match.Prefix]
		if group == nil {
			group = &binaryGroup{}
			parameterGroups[match.Prefix] = group
		}
		if match.Negative {
			copyEndpoint := endpoint
			group.negative = &copyEndpoint
		} else {
			group.bits = append(group.bits, BinaryBit{Endpoint: endpoint, Weight: match.Weight})
		}

		binding := catalog.Bindings[match.ID]
		binding.Spec = spec
		catalog.Bindings[match.ID] = binding
	}

	sortEndpoints(catalog.RawMethods)
	for id, binding := range catalog.Bindings {
		sortEndpoints(binding.Direct)

		parameterGroups := groups[id]
		prefixes := make([]string, 0, len(parameterGroups))
		for prefix := range parameterGroups {
			prefixes = append(prefixes, prefix)
		}
		sort.Strings(prefixes)
		for _, prefix := range prefixes {
			group := parameterGroups[prefix]
			if len(group.bits) == 0 {
				continue
			}
			sort.Slice(group.bits, func(i, j int) bool {
				return group.bits[i].Weight < group.bits[j].Weight
			})
			binding.Binary = append(binding.Binary, BinaryBinding{
				Negative: group.negative,
				Bits:     append([]BinaryBit(nil), group.bits...),
			})
		}
		catalog.Bindings[id] = binding
	}

	outputs, err := compileOutputs(catalog)
	if err != nil {
		return nil, err
	}
	catalog.Outputs = outputs

	catalog.Hash = hashCatalog(catalog)
	return catalog, nil
}

func compileOutputs(catalog *Catalog) ([]outputBinding, error) {
	outputs := make([]outputBinding, 0, len(catalog.RawMethods))
	for id, binding := range catalog.Bindings {
		operation := outputDirectFloat
		if binding.Spec.ValueType == parameters.ValueBool {
			operation = outputDirectBool
		}
		for _, endpoint := range binding.Direct {
			wireKind, err := endpointWireKind(endpoint.Type)
			if err != nil {
				return nil, fmt.Errorf("compile OSC output %q: %w", endpoint.Address, err)
			}
			outputs = append(outputs, outputBinding{
				Parameter: id,
				Address:   endpoint.Address,
				Operation: operation,
				WireKind:  wireKind,
				Range:     binding.Spec.Range,
				HasRange:  binding.Spec.HasRange,
			})
		}

		for _, binaryBinding := range binding.Binary {
			var quantizeMax uint32
			for _, bit := range binaryBinding.Bits {
				quantizeMax |= bit.Weight
			}
			if binaryBinding.Negative != nil {
				wireKind, err := endpointWireKind(binaryBinding.Negative.Type)
				if err != nil {
					return nil, fmt.Errorf("compile OSC output %q: %w", binaryBinding.Negative.Address, err)
				}
				outputs = append(outputs, outputBinding{
					Parameter:   id,
					Address:     binaryBinding.Negative.Address,
					Operation:   outputBinaryNegative,
					WireKind:    wireKind,
					QuantizeMax: quantizeMax,
					Range:       binding.Spec.Range,
					HasRange:    binding.Spec.HasRange,
				})
			}
			for _, bit := range binaryBinding.Bits {
				wireKind, err := endpointWireKind(bit.Endpoint.Type)
				if err != nil {
					return nil, fmt.Errorf("compile OSC output %q: %w", bit.Endpoint.Address, err)
				}
				outputs = append(outputs, outputBinding{
					Parameter:   id,
					Address:     bit.Endpoint.Address,
					Operation:   outputBinaryBit,
					WireKind:    wireKind,
					Weight:      bit.Weight,
					QuantizeMax: quantizeMax,
					Range:       binding.Spec.Range,
					HasRange:    binding.Spec.HasRange,
				})
			}
		}
	}

	sort.Slice(outputs, func(i, j int) bool {
		left, right := outputs[i], outputs[j]
		if left.Address != right.Address {
			return left.Address < right.Address
		}
		if left.Parameter != right.Parameter {
			return left.Parameter < right.Parameter
		}
		if left.Operation != right.Operation {
			return left.Operation < right.Operation
		}
		if left.WireKind != right.WireKind {
			return left.WireKind < right.WireKind
		}
		return left.Weight < right.Weight
	})

	unique := outputs[:0]
	for _, output := range outputs {
		if len(unique) > 0 && unique[len(unique)-1].Address == output.Address {
			if outputBindingsEqual(unique[len(unique)-1], output) {
				continue
			}
			return nil, fmt.Errorf("%w: %s", ErrConflictingOSCAddress, output.Address)
		}
		unique = append(unique, output)
	}
	if len(unique) > int(^uint16(0))+1 {
		return nil, ErrTooManyOSCOutputs
	}
	for index := range unique {
		unique[index].CacheIndex = uint16(index)
	}
	return unique, nil
}

func endpointWireKind(typ string) (scalarKind, error) {
	switch typ {
	case "f":
		return scalarFloat32, nil
	case "i":
		return scalarInt32, nil
	case "T", "F":
		return scalarFalse, nil
	default:
		return 0, fmt.Errorf("%w: endpoint type %q", ErrUnsupportedType, typ)
	}
}

func outputBindingsEqual(left, right outputBinding) bool {
	left.CacheIndex = 0
	right.CacheIndex = 0
	return left == right
}

type binaryGroup struct {
	negative *Endpoint
	bits     []BinaryBit
}

func isWritable(node *QueryNode) bool {
	if node.Access == nil {
		// OSCQuery says missing ACCESS should be treated as writable.
		return true
	}
	return *node.Access == AccessWriteOnly || *node.Access == AccessReadWrite
}

func supportedParameterType(typ string) bool {
	return typ == "f" || typ == "i" || typ == "T" || typ == "F"
}

func sortEndpoints(endpoints []Endpoint) {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Address == endpoints[j].Address {
			return endpoints[i].Type < endpoints[j].Type
		}
		return endpoints[i].Address < endpoints[j].Address
	})
}

func hashCatalog(catalog *Catalog) uint64 {
	h := fnv.New64a()
	var number [8]byte
	for _, output := range catalog.Outputs {
		_, _ = h.Write([]byte(output.Address))
		_, _ = h.Write([]byte{0, byte(output.Operation), byte(output.WireKind)})
		binary.BigEndian.PutUint64(number[:], uint64(output.Parameter))
		_, _ = h.Write(number[:])
		binary.BigEndian.PutUint32(number[:4], output.Weight)
		_, _ = h.Write(number[:4])
		binary.BigEndian.PutUint32(number[:4], output.QuantizeMax)
		_, _ = h.Write(number[:4])
		binary.BigEndian.PutUint32(number[:4], math.Float32bits(output.Range.Min))
		_, _ = h.Write(number[:4])
		binary.BigEndian.PutUint32(number[:4], math.Float32bits(output.Range.Max))
		_, _ = h.Write(number[:4])
		if output.HasRange {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}
	return h.Sum64()
}

func sortedCatalogIDs(catalog *Catalog) []parameters.ParameterID {
	ids := make([]parameters.ParameterID, 0, len(catalog.Bindings))
	for id := range catalog.Bindings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
