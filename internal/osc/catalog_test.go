package osc

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

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
