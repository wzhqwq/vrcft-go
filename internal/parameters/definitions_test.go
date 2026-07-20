package parameters

import "testing"

func TestGeneratedDefinitionsAreCompleteAndUnique(t *testing.T) {
	if got, want := len(Definitions), 127; got != want {
		t.Fatalf("len(Definitions) = %d, want %d", got, want)
	}
	if got, want := len(ParameterByOSCName), 127; got != want {
		t.Fatalf("len(ParameterByOSCName) = %d, want %d", got, want)
	}

	seen := make(map[string]struct{}, len(Definitions))
	for index, def := range Definitions {
		if def.ID != ParameterID(index) {
			t.Fatalf("definition %d has id %d", index, def.ID)
		}
		if _, exists := seen[def.OSCName]; exists {
			t.Fatalf("duplicate OSC name %q", def.OSCName)
		}
		seen[def.OSCName] = struct{}{}
	}
}

func TestResolveAddressSupportsNestedPrefix(t *testing.T) {
	id, prefix, ok := ResolveAddress("/avatar/parameters/Example/Nest/v2/JawOpen")
	if !ok {
		t.Fatal("ResolveAddress() did not match")
	}
	if id != ParameterJawOpen {
		t.Fatalf("id = %v, want ParameterJawOpen", id)
	}
	if prefix != "Example/Nest" {
		t.Fatalf("prefix = %q, want %q", prefix, "Example/Nest")
	}
}

func TestResolveBinaryAddress(t *testing.T) {
	part, ok := ResolveBinaryAddress("/avatar/parameters/Face/v2/JawX8")
	if !ok {
		t.Fatal("ResolveBinaryAddress() did not match")
	}
	if part.Parameter != ParameterJawX || part.Prefix != "Face" || part.Weight != 8 || part.Negative {
		t.Fatalf("unexpected binary part: %+v", part)
	}

	negative, ok := ResolveBinaryAddress("Face/v2/JawXNegative")
	if !ok || !negative.Negative || negative.Parameter != ParameterJawX {
		t.Fatalf("unexpected negative part: %+v, ok=%v", negative, ok)
	}
}

func TestClampUsesGeneratedRange(t *testing.T) {
	got, ok := Clamp(ParameterJawX, 2)
	if !ok || got != 1 {
		t.Fatalf("Clamp(ParameterJawX, 2) = %v, %v; want 1, true", got, ok)
	}
}
