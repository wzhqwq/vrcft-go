package osc

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ParameterClass uint8

const (
	ParameterFloat ParameterClass = iota + 1
	ParameterBool
)

type ParameterSpec struct {
	// Key is the stable application-side semantic identifier.
	Key string
	// Suffix is matched at an OSC path segment boundary, for example v2/JawOpen.
	Suffix    string
	Class     ParameterClass
	Signed    bool
	Unbounded bool
}

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
	Bindings   map[string]ParameterBinding
	RawMethods []Endpoint
}

func BuildCatalog(root *QueryNode, specs []ParameterSpec, generation uint64) (*Catalog, error) {
	if root == nil {
		return nil, fmt.Errorf("nil OSCQuery root")
	}
	methods := root.FlattenMethods()
	catalog := &Catalog{
		Generation: generation,
		UpdatedAt:  time.Now(),
		Bindings:   make(map[string]ParameterBinding, len(specs)),
	}

	for _, method := range methods {
		if !isWritable(method) || !supportedParameterType(method.Type) {
			continue
		}
		catalog.RawMethods = append(catalog.RawMethods, Endpoint{
			Address: method.FullPath,
			Type:    method.Type,
		})
	}
	sortEndpoints(catalog.RawMethods)

	for _, spec := range specs {
		if spec.Key == "" || spec.Suffix == "" {
			return nil, fmt.Errorf("invalid parameter spec: key and suffix are required")
		}
		binding := ParameterBinding{Spec: spec}
		groups := make(map[string]*binaryGroup)

		for _, endpoint := range catalog.RawMethods {
			if pathHasSuffix(endpoint.Address, spec.Suffix) {
				binding.Direct = append(binding.Direct, endpoint)
				continue
			}
			if spec.Class != ParameterFloat {
				continue
			}

			if pathHasSuffix(endpoint.Address, spec.Suffix+"Negative") {
				prefix := trimPathSuffix(endpoint.Address, spec.Suffix+"Negative")
				group := groups[prefix]
				if group == nil {
					group = &binaryGroup{}
					groups[prefix] = group
				}
				copyEndpoint := endpoint
				group.negative = &copyEndpoint
				continue
			}

			weight, prefix, ok := binaryWeight(endpoint.Address, spec.Suffix)
			if !ok {
				continue
			}
			group := groups[prefix]
			if group == nil {
				group = &binaryGroup{}
				groups[prefix] = group
			}
			group.bits = append(group.bits, BinaryBit{Endpoint: endpoint, Weight: weight})
		}

		sortEndpoints(binding.Direct)
		prefixes := make([]string, 0, len(groups))
		for prefix := range groups {
			prefixes = append(prefixes, prefix)
		}
		sort.Strings(prefixes)
		for _, prefix := range prefixes {
			group := groups[prefix]
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
		catalog.Bindings[spec.Key] = binding
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

func pathHasSuffix(fullPath, suffix string) bool {
	fullPath = cleanOSCPath(fullPath)
	suffix = strings.TrimPrefix(suffix, "/")
	return strings.HasSuffix(fullPath, "/"+suffix)
}

func trimPathSuffix(fullPath, suffix string) string {
	return strings.TrimSuffix(fullPath, strings.TrimPrefix(suffix, "/"))
}

func binaryWeight(fullPath, suffix string) (uint32, string, bool) {
	fullPath = cleanOSCPath(fullPath)
	suffix = strings.TrimPrefix(suffix, "/")
	marker := "/" + suffix
	index := strings.LastIndex(fullPath, marker)
	if index < 0 {
		return 0, "", false
	}
	trailing := fullPath[index+len(marker):]
	if trailing == "" || strings.Contains(trailing, "/") {
		return 0, "", false
	}
	parsed, err := strconv.ParseUint(trailing, 10, 32)
	if err != nil || parsed == 0 {
		return 0, "", false
	}
	weight := uint32(parsed)
	if weight&(weight-1) != 0 {
		return 0, "", false
	}
	return weight, fullPath[:index+1], true
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
	keys := make([]string, 0, len(catalog.Bindings))
	for key := range catalog.Bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		binding := catalog.Bindings[key]
		_, _ = h.Write([]byte(key))
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
