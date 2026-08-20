package osc

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

func TestBuildCatalogFromEndpointsMatchesQueryTree(t *testing.T) {
	parameterCatalog, err := NewParameterCatalog(parameters.Definitions[:])
	if err != nil {
		t.Fatal(err)
	}

	endpoints := []Endpoint{
		{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"},
		{Address: "/avatar/parameters/Face/v2/JawXNegative", Type: "T"},
		{Address: "/avatar/parameters/Face/v2/JawX1", Type: "T"},
		{Address: "/avatar/parameters/Face/v2/JawX2", Type: "T"},
	}
	fromEndpoints, err := BuildCatalogFromEndpoints(endpoints, parameterCatalog, 9)
	if err != nil {
		t.Fatal(err)
	}

	root := NewQueryRoot()
	for _, endpoint := range endpoints {
		if err := root.Add(NewMethod(endpoint.Address, endpoint.Type, AccessWriteOnly)); err != nil {
			t.Fatal(err)
		}
	}
	fromQuery, err := BuildCatalog(root, parameterCatalog, 9)
	if err != nil {
		t.Fatal(err)
	}
	if fromEndpoints.UpdatedAt.IsZero() || fromQuery.UpdatedAt.IsZero() {
		t.Fatalf("catalog timestamps = %v, %v; want non-zero", fromEndpoints.UpdatedAt, fromQuery.UpdatedAt)
	}
	comparableEndpoints := *fromEndpoints
	comparableQuery := *fromQuery
	comparableEndpoints.UpdatedAt = time.Time{}
	comparableQuery.UpdatedAt = time.Time{}
	if !reflect.DeepEqual(&comparableEndpoints, &comparableQuery) {
		t.Fatalf("endpoint catalog = %#v, query catalog = %#v", fromEndpoints, fromQuery)
	}

	endpoints[0] = Endpoint{Address: "/avatar/parameters/Face/v2/JawX", Type: "i"}
	if got, want := fromEndpoints.RawMethods[0], (Endpoint{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"}); got != want {
		t.Fatalf("catalog retained caller endpoint: got %#v, want %#v", got, want)
	}

	if _, err := BuildCatalogFromEndpoints([]Endpoint{{Address: "avatar/parameters/Face/v2/JawOpen", Type: "f"}}, parameterCatalog, 9); err == nil {
		t.Fatal("relative endpoint was accepted")
	}
	if _, err := BuildCatalogFromEndpoints([]Endpoint{{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "s"}}, parameterCatalog, 9); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("unsupported endpoint type error = %v, want ErrUnsupportedType", err)
	}

	deduplicated, err := BuildCatalogFromEndpoints([]Endpoint{
		{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"},
		{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"},
	}, parameterCatalog, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(deduplicated.Outputs); got != 1 {
		t.Fatalf("deduplicated outputs = %d, want 1", got)
	}

	_, err = BuildCatalogFromEndpoints([]Endpoint{
		{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"},
		{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "i"},
	}, parameterCatalog, 9)
	if !errors.Is(err, ErrConflictingOSCAddress) {
		t.Fatalf("conflicting output error = %v, want ErrConflictingOSCAddress", err)
	}
}

func TestCatalogCloneOwnsNestedBindings(t *testing.T) {
	parameterCatalog, err := NewParameterCatalog(parameters.Definitions[:])
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := BuildCatalogFromEndpoints([]Endpoint{
		{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"},
		{Address: "/avatar/parameters/Face/v2/JawXNegative", Type: "T"},
		{Address: "/avatar/parameters/Face/v2/JawX1", Type: "T"},
		{Address: "/avatar/parameters/Face/v2/JawX2", Type: "T"},
	}, parameterCatalog, 9)
	if err != nil {
		t.Fatal(err)
	}
	before := catalog.Clone()
	clone := catalog.Clone()
	clone.RawMethods[0].Address = "/mutated/raw"

	direct := clone.Bindings[parameters.ParameterJawOpen]
	direct.Direct[0].Address = "/mutated/direct"
	clone.Bindings[parameters.ParameterJawOpen] = direct

	binary := clone.Bindings[parameters.ParameterJawX]
	binary.Binary[0].Bits[0].Endpoint.Address = "/mutated/bit"
	binary.Binary[0].Negative.Address = "/mutated/negative"
	clone.Bindings[parameters.ParameterJawX] = binary
	clone.Outputs[0].Address = "/mutated/output"

	if !reflect.DeepEqual(catalog, before) {
		t.Fatalf("catalog changed after clone mutation: got %#v, want %#v", catalog, before)
	}
	var nilCatalog *Catalog
	if nilCatalog.Clone() != nil {
		t.Fatal("nil catalog clone is not nil")
	}
}

func TestBuildCatalogWithPrefixAndBinary(t *testing.T) {
	parameterCatalog, err := NewParameterCatalog(parameters.Definitions[:])
	if err != nil {
		t.Fatal(err)
	}

	root := NewQueryRoot()
	if err := root.Add(NewMethod("/avatar/parameters/Face/v2/JawX", "f", AccessWriteOnly)); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"Negative", "1", "2", "4", "8"} {
		if err := root.Add(NewMethod("/avatar/parameters/Face/v2/JawX"+suffix, "T", AccessWriteOnly)); err != nil {
			t.Fatal(err)
		}
	}

	catalog, err := BuildCatalog(root, parameterCatalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := catalog.Bindings[parameters.ParameterJawX]
	if !ok {
		t.Fatal("JawX binding not found")
	}
	if len(binding.Direct) != 1 || len(binding.Binary) != 1 || len(binding.Binary[0].Bits) != 4 || binding.Binary[0].Negative == nil {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestBuildCatalogHonorsDefinitionEncodings(t *testing.T) {
	definitions := []parameters.ParameterDefinition{
		{
			ID: 0, OSCName: "v2/DirectOnly", ValueType: parameters.ValueFloat,
			Encodings: parameters.EncodingFloat,
		},
	}
	parameterCatalog, err := NewParameterCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}

	root := NewQueryRoot()
	_ = root.Add(NewMethod("/avatar/parameters/v2/DirectOnly", "f", AccessWriteOnly))
	_ = root.Add(NewMethod("/avatar/parameters/v2/DirectOnly1", "T", AccessWriteOnly))

	catalog, err := BuildCatalog(root, parameterCatalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	binding := catalog.Bindings[0]
	if len(binding.Direct) != 1 {
		t.Fatalf("direct bindings = %d", len(binding.Direct))
	}
	if len(binding.Binary) != 0 {
		t.Fatalf("binary bindings = %d, want 0", len(binding.Binary))
	}
}

func TestBuildCatalogCompilesSortedOutputPlan(t *testing.T) {
	definitions := []parameters.ParameterDefinition{
		{
			ID: 0, OSCName: "Float", ValueType: parameters.ValueFloat,
			Encodings: parameters.EncodingFloat | parameters.EncodingBinary,
			Range:     parameters.ValueRange{Min: -1, Max: 1}, HasRange: true,
		},
		{
			ID: 1, OSCName: "Active", ValueType: parameters.ValueBool,
			Encodings: parameters.EncodingBool,
		},
	}
	specs, err := NewParameterCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}

	root := NewQueryRoot()
	methods := []*QueryNode{
		NewMethod("/z/Float", "f", AccessWriteOnly),
		NewMethod("/a/Float", "i", AccessWriteOnly),
		NewMethod("/b/FloatNegative", "T", AccessWriteOnly),
		NewMethod("/b/Float1", "T", AccessWriteOnly),
		NewMethod("/b/Float2", "T", AccessWriteOnly),
		NewMethod("/c/Active", "f", AccessWriteOnly),
	}
	for _, method := range methods {
		if err := root.Add(method); err != nil {
			t.Fatal(err)
		}
	}

	catalog, err := BuildCatalog(root, specs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(catalog.Outputs), len(methods); got != want {
		t.Fatalf("outputs = %d, want %d", got, want)
	}
	for index, output := range catalog.Outputs {
		if output.CacheIndex != uint16(index) {
			t.Fatalf("cache index %d = %d", index, output.CacheIndex)
		}
		if index > 0 && catalog.Outputs[index-1].Address > output.Address {
			t.Fatalf("outputs are not address sorted: %#v", catalog.Outputs)
		}
	}

	byAddress := make(map[string]outputBinding, len(catalog.Outputs))
	for _, output := range catalog.Outputs {
		byAddress[output.Address] = output
	}
	if output := byAddress["/a/Float"]; output.Operation != outputDirectFloat || output.WireKind != scalarInt32 {
		t.Fatalf("integer direct output = %#v", output)
	}
	if output := byAddress["/c/Active"]; output.Operation != outputDirectBool || output.WireKind != scalarFloat32 {
		t.Fatalf("float bool output = %#v", output)
	}
	if output := byAddress["/b/FloatNegative"]; output.Operation != outputBinaryNegative || output.WireKind != scalarFalse {
		t.Fatalf("negative output = %#v", output)
	}
	for _, address := range []string{"/b/Float1", "/b/Float2"} {
		output := byAddress[address]
		if output.Operation != outputBinaryBit || output.QuantizeMax != 3 {
			t.Fatalf("binary output %s = %#v", address, output)
		}
	}
}
