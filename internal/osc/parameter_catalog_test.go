package osc

import (
	"errors"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

func TestNewParameterCatalogUsesGeneratedDefinitions(t *testing.T) {
	catalog, err := NewParameterCatalog(parameters.Definitions[:])
	if err != nil {
		t.Fatalf("NewParameterCatalog: %v", err)
	}
	if got, want := catalog.Len(), int(parameters.ParameterCount); got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	spec, ok := catalog.Spec(parameters.ParameterJawX)
	if !ok {
		t.Fatal("JawX spec not found")
	}
	if spec.OSCName != "v2/JawX" {
		t.Fatalf("JawX OSCName = %q", spec.OSCName)
	}
	if !spec.Encodings.Has(parameters.EncodingFloat) || !spec.Encodings.Has(parameters.EncodingBinary) {
		t.Fatalf("JawX encodings = %v", spec.Encodings)
	}
	if !spec.HasRange || spec.Range.Min != -1 || spec.Range.Max != 1 {
		t.Fatalf("JawX range = %#v, hasRange=%v", spec.Range, spec.HasRange)
	}
}

func TestNewParameterCatalogRejectsDuplicateOSCName(t *testing.T) {
	definitions := []parameters.ParameterDefinition{
		{
			ID: 0, OSCName: "v2/Test", ValueType: parameters.ValueFloat,
			Encodings: parameters.EncodingFloat,
		},
		{
			ID: 1, OSCName: "v2/Test", ValueType: parameters.ValueFloat,
			Encodings: parameters.EncodingFloat,
		},
	}

	_, err := NewParameterCatalog(definitions)
	if !errors.Is(err, ErrDuplicateOSCName) {
		t.Fatalf("error = %v, want ErrDuplicateOSCName", err)
	}
}

func TestParameterCatalogResolvesPrefixedAndBinaryAddresses(t *testing.T) {
	catalog, err := NewParameterCatalog(parameters.Definitions[:])
	if err != nil {
		t.Fatal(err)
	}

	direct, ok := catalog.ResolveAddress("/avatar/parameters/Face/v2/JawX")
	if !ok || direct.ID != parameters.ParameterJawX || direct.Prefix != "Face" {
		t.Fatalf("direct match = %#v, ok=%v", direct, ok)
	}

	negative, ok := catalog.ResolveBinaryAddress("/avatar/parameters/Face/v2/JawXNegative")
	if !ok || negative.ID != parameters.ParameterJawX || negative.Prefix != "Face" || !negative.Negative {
		t.Fatalf("negative match = %#v, ok=%v", negative, ok)
	}

	bit, ok := catalog.ResolveBinaryAddress("/avatar/parameters/Face/v2/JawX8")
	if !ok || bit.ID != parameters.ParameterJawX || bit.Prefix != "Face" || bit.Weight != 8 {
		t.Fatalf("bit match = %#v, ok=%v", bit, ok)
	}

	if _, ok := catalog.ResolveBinaryAddress("/avatar/parameters/Face/v2/JawX3"); ok {
		t.Fatal("non-power-of-two binary weight was accepted")
	}
}
