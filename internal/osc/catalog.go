package osc

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
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

type Catalog struct {
	Generation uint64
	UpdatedAt  time.Time
	Hash       uint64
	Bindings   map[parameters.ParameterID]ParameterBinding
	RawMethods []Endpoint
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

	catalog.Hash = hashCatalog(catalog)
	return catalog, nil
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
	ids := sortedCatalogIDs(catalog)
	for _, id := range ids {
		binding := catalog.Bindings[id]
		_, _ = h.Write([]byte(strconv.FormatUint(uint64(id), 10)))
		for _, endpoint := range binding.Direct {
			_, _ = h.Write([]byte(endpoint.Address))
			_, _ = h.Write([]byte(endpoint.Type))
		}
		for _, binary := range binding.Binary {
			if binary.Negative != nil {
				_, _ = h.Write([]byte(binary.Negative.Address))
				_, _ = h.Write([]byte(binary.Negative.Type))
			}
			for _, bit := range binary.Bits {
				_, _ = h.Write([]byte(bit.Endpoint.Address))
				_, _ = h.Write([]byte(bit.Endpoint.Type))
				_, _ = h.Write([]byte(strconv.FormatUint(uint64(bit.Weight), 10)))
			}
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
