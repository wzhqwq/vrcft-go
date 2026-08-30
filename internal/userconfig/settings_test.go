package userconfig

import (
	"reflect"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/osc"
)

func TestDefaultCandidateUsesIndependentBackendDefaults(t *testing.T) {
	paths := Paths{DefaultOSCRoot: `C:\\Users\\Tester\\AppData\\LocalLow\\VRChat\\VRChat\\OSC`}
	first := DefaultCandidate(paths)
	second := DefaultCandidate(paths)
	if first.Avatar.OSCRoot != paths.DefaultOSCRoot || first.Avatar.FallbackPath != "" {
		t.Fatalf("avatar defaults = %#v", first.Avatar)
	}
	if len(first.Plugins.DevRoots) != 0 || first.OSC.TargetMode != osc.TargetModeAuto {
		t.Fatalf("candidate defaults = %#v", first)
	}
	first.Plugins.DevRoots = append(first.Plugins.DevRoots, `C:\\dev`)
	first.Processing.Overrides = append(first.Processing.Overrides, ProcessingOverride{Name: "eye.left_gaze_x"})
	first.Processing.MutualExclusion = append(first.Processing.MutualExclusion, []string{"eye.left_gaze_x", "eye.right_gaze_x"})
	if len(second.Plugins.DevRoots) != 0 || len(second.Processing.Overrides) != 0 || len(second.Processing.MutualExclusion) != 0 {
		t.Fatal("default candidates alias mutable state")
	}
}

func TestSettingsCloneDeepCopiesMutableFields(t *testing.T) {
	settings := Settings{
		SchemaVersion: SchemaVersion,
		Revision:      3,
		Plugins:       Plugins{DevRoots: []string{`C:\\dev`}},
		Processing: Processing{
			Overrides:       []ProcessingOverride{{Name: "eye.left_gaze_x"}},
			MutualExclusion: [][]string{{"eye.left_gaze_x", "eye.right_gaze_x"}},
		},
	}
	clone := settings.Clone()
	clone.Plugins.DevRoots[0] = `C:\\changed`
	clone.Processing.Overrides[0].Name = "eye.right_gaze_x"
	clone.Processing.MutualExclusion[0][0] = "eye.left_gaze_y"
	if reflect.DeepEqual(settings, clone) {
		t.Fatal("clone mutation did not change clone")
	}
	if settings.Plugins.DevRoots[0] != `C:\\dev` || settings.Processing.Overrides[0].Name != "eye.left_gaze_x" || settings.Processing.MutualExclusion[0][0] != "eye.left_gaze_x" {
		t.Fatalf("settings was mutated through clone: %#v", settings)
	}
}

func TestSettingsClonePreservesNilMutualExclusion(t *testing.T) {
	settings := Settings{Processing: Processing{}}
	clone := settings.Clone()
	if clone.Processing.MutualExclusion != nil {
		t.Fatalf("clone mutual exclusion = %#v, want nil", clone.Processing.MutualExclusion)
	}
}

func TestSettingsAndCandidateClonePreserveSliceShapeAndOwnership(t *testing.T) {
	cloners := []struct {
		name  string
		clone func(Candidate) Candidate
	}{
		{
			name: "Settings.Clone",
			clone: func(value Candidate) Candidate {
				settings := Settings{Plugins: value.Plugins, Processing: value.Processing}
				cloned := settings.Clone()
				return Candidate{Plugins: cloned.Plugins, Processing: cloned.Processing}
			},
		},
		{name: "Candidate.Clone", clone: func(value Candidate) Candidate { return value.Clone() }},
	}
	cases := []struct {
		name  string
		value Candidate
		shape cloneSliceShape
	}{
		{name: "nil", value: Candidate{}, shape: cloneSlicesNil},
		{
			name: "non-nil empty",
			value: Candidate{
				Plugins: Plugins{DevRoots: []string{}},
				Processing: Processing{
					Overrides:       []ProcessingOverride{},
					MutualExclusion: [][]string{},
				},
			},
			shape: cloneSlicesEmpty,
		},
		{
			name: "non-empty",
			value: Candidate{
				Plugins: Plugins{DevRoots: []string{`C:\\dev`}},
				Processing: Processing{
					Overrides: []ProcessingOverride{{Name: "eye.left_gaze_x"}},
					MutualExclusion: [][]string{
						{},
						{"eye.left_gaze_x", "eye.right_gaze_x"},
					},
				},
			},
			shape: cloneSlicesNonEmpty,
		},
	}

	for _, cloner := range cloners {
		t.Run(cloner.name, func(t *testing.T) {
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					cloned := cloner.clone(test.value)
					assertCandidateCloneSlices(t, test.value, cloned, test.shape)
				})
			}
		})
	}
}

func TestProcessingClonePreservesSliceShapeAndOwnership(t *testing.T) {
	tests := []struct {
		name  string
		value Processing
		shape cloneSliceShape
	}{
		{name: "nil", value: Processing{}, shape: cloneSlicesNil},
		{
			name:  "non-nil empty",
			value: Processing{Overrides: []ProcessingOverride{}, MutualExclusion: [][]string{}},
			shape: cloneSlicesEmpty,
		},
		{
			name: "non-empty",
			value: Processing{
				Overrides: []ProcessingOverride{{Name: "eye.left_gaze_x"}},
				MutualExclusion: [][]string{
					{},
					{"eye.left_gaze_x", "eye.right_gaze_x"},
				},
			},
			shape: cloneSlicesNonEmpty,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cloned := test.value.Clone()
			assertProcessingCloneSlices(t, test.value, cloned, test.shape)
		})
	}
}

type cloneSliceShape uint8

const (
	cloneSlicesNil cloneSliceShape = iota
	cloneSlicesEmpty
	cloneSlicesNonEmpty
)

func assertCandidateCloneSlices(t *testing.T, source, cloned Candidate, shape cloneSliceShape) {
	t.Helper()
	switch shape {
	case cloneSlicesNil:
		if cloned.Plugins.DevRoots != nil || cloned.Processing.Overrides != nil || cloned.Processing.MutualExclusion != nil {
			t.Fatalf("nil clone slices = roots %#v overrides %#v mutual %#v", cloned.Plugins.DevRoots, cloned.Processing.Overrides, cloned.Processing.MutualExclusion)
		}
	case cloneSlicesEmpty:
		if cloned.Plugins.DevRoots == nil || cloned.Processing.Overrides == nil || cloned.Processing.MutualExclusion == nil || len(cloned.Plugins.DevRoots) != 0 || len(cloned.Processing.Overrides) != 0 || len(cloned.Processing.MutualExclusion) != 0 {
			t.Fatalf("empty clone slices = roots %#v overrides %#v mutual %#v, want non-nil empty", cloned.Plugins.DevRoots, cloned.Processing.Overrides, cloned.Processing.MutualExclusion)
		}
	case cloneSlicesNonEmpty:
		assertProcessingCloneSlices(t, source.Processing, cloned.Processing, shape)
		cloned.Plugins.DevRoots[0] = `C:\\changed`
		if source.Plugins.DevRoots[0] != `C:\\dev` {
			t.Fatalf("DevRoots aliases source: %#v", source.Plugins.DevRoots)
		}
	}
}

func assertProcessingCloneSlices(t *testing.T, source, cloned Processing, shape cloneSliceShape) {
	t.Helper()
	switch shape {
	case cloneSlicesNil:
		if cloned.Overrides != nil || cloned.MutualExclusion != nil {
			t.Fatalf("nil processing clone slices = overrides %#v mutual %#v", cloned.Overrides, cloned.MutualExclusion)
		}
	case cloneSlicesEmpty:
		if cloned.Overrides == nil || cloned.MutualExclusion == nil || len(cloned.Overrides) != 0 || len(cloned.MutualExclusion) != 0 {
			t.Fatalf("empty processing clone slices = overrides %#v mutual %#v, want non-nil empty", cloned.Overrides, cloned.MutualExclusion)
		}
	case cloneSlicesNonEmpty:
		if cloned.MutualExclusion[0] == nil || len(cloned.MutualExclusion[0]) != 0 {
			t.Fatalf("empty mutual-exclusion group = %#v, want non-nil empty", cloned.MutualExclusion[0])
		}
		cloned.Overrides[0].Name = "eye.right_gaze_x"
		cloned.MutualExclusion[1][0] = "eye.left_gaze_y"
		if source.Overrides[0].Name != "eye.left_gaze_x" || source.MutualExclusion[1][0] != "eye.left_gaze_x" {
			t.Fatalf("processing clone aliases source: source %#v cloned %#v", source, cloned)
		}
	}
}
