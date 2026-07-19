package osc

import "testing"

func TestBuildCatalogWithPrefixAndBinary(t *testing.T) {
	root := NewQueryRoot()
	if err := root.Add(NewMethod("/avatar/parameters/Face/v2/JawX", "f", AccessWriteOnly)); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"Negative", "1", "2", "4", "8"} {
		if err := root.Add(NewMethod("/avatar/parameters/Face/v2/JawX"+suffix, "T", AccessWriteOnly)); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := BuildCatalog(root, []ParameterSpec{{
		Key: "v2/JawX", Suffix: "v2/JawX", Class: ParameterFloat, Signed: true,
	}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	binding := catalog.Bindings["v2/JawX"]
	if len(binding.Direct) != 1 || len(binding.Binary) != 1 || len(binding.Binary[0].Bits) != 4 || binding.Binary[0].Negative == nil {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestVRCFTSpecCount(t *testing.T) {
	specs := VRCFTParameterSpecs()
	floats := 0
	bools := 0
	seen := make(map[string]struct{})
	for _, spec := range specs {
		if _, exists := seen[spec.Key]; exists {
			t.Fatalf("duplicate spec %s", spec.Key)
		}
		seen[spec.Key] = struct{}{}
		switch spec.Class {
		case ParameterFloat:
			floats++
		case ParameterBool:
			bools++
		}
	}
	if floats != 124 || bools != 3 {
		t.Fatalf("got %d float and %d bool specs", floats, bools)
	}
}
