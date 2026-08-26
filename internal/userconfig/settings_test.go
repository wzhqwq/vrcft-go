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
