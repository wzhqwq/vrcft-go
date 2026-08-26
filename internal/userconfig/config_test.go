package userconfig

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
)

func TestNormalizeCleansAndSortsOwnedPaths(t *testing.T) {
	candidate := DefaultCandidate(Paths{DefaultOSCRoot: `C:\VRChat\OSC`})
	candidate.Avatar.OSCRoot = `C:\VRChat\x\..\OSC`
	candidate.Avatar.FallbackPath = `C:\missing\x\..\avatar.json`
	candidate.Plugins.DevRoots = []string{`C:\Zed\..\Zed`, `c:\alpha`, `C:\Beta`}
	normalized, err := Normalize(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized.Plugins.DevRoots, []string{`c:\alpha`, `C:\Beta`, `C:\Zed`}) {
		t.Fatalf("roots = %#v", normalized.Plugins.DevRoots)
	}
	normalized.Plugins.DevRoots[0] = `C:\changed`
	if candidate.Plugins.DevRoots[1] != `c:\alpha` {
		t.Fatal("normalized candidate aliases input")
	}
}

func TestNormalizeRejectsSemanticErrors(t *testing.T) {
	base := DefaultCandidate(Paths{DefaultOSCRoot: `C:\VRChat\OSC`})
	tests := []struct {
		name, field string
		mutate      func(*Candidate)
	}{
		{"missing OSC root", "avatar.oscRoot", func(c *Candidate) { c.Avatar.OSCRoot = "" }},
		{"duplicate dev root", "plugins.devRoots", func(c *Candidate) { c.Plugins.DevRoots = []string{`C:\dev`, `c:\DEV`} }},
		{"auto manual fields", "osc.manualHost", func(c *Candidate) { c.OSC.ManualHost = "127.0.0.1"; c.OSC.ManualPort = 9000 }},
		{"manual preferred service", "osc.preferredService", func(c *Candidate) {
			c.OSC.TargetMode = osc.TargetModeManual
			c.OSC.ManualHost = "127.0.0.1"
			c.OSC.ManualPort = 9000
			c.OSC.PreferredService = "x"
		}},
		{"manual non-unicast", "osc.manualHost", func(c *Candidate) {
			c.OSC.TargetMode = osc.TargetModeManual
			c.OSC.ManualHost = "localhost"
			c.OSC.ManualPort = 9000
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := base.Clone()
			tt.mutate(&candidate)
			_, err := Normalize(candidate)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Field != tt.field {
				t.Fatalf("Normalize() error = %v, want field %q", err, tt.field)
			}
		})
	}
}

func TestApplicationConfigMapsFreshOwnedBackendConfig(t *testing.T) {
	paths := Paths{BuiltinPluginDir: `C:\bin\plugins`, PluginStoreFile: `C:\AppData\vrcft-go\plugins.json`, DefaultOSCRoot: `C:\VRChat\OSC`}
	candidate := DefaultCandidate(paths)
	candidate.Plugins.DevRoots = []string{`C:\dev`}
	normalized, err := Normalize(candidate)
	if err != nil {
		t.Fatal(err)
	}
	settings := Settings{SchemaVersion: SchemaVersion, Revision: 1, Avatar: normalized.Avatar, Plugins: normalized.Plugins, Processing: normalized.Processing, OSC: normalized.OSC}
	config, err := ApplicationConfig(settings, paths)
	if err != nil {
		t.Fatal(err)
	}
	if config.PluginCatalog.BuiltinRoot != paths.BuiltinPluginDir || config.PluginStorePath != paths.PluginStoreFile || !reflect.DeepEqual(config.PluginOptions, plugins.DefaultOptions()) || config.OSC.TargetMode != settings.OSC.TargetMode || config.FrameInterval != 0 || config.PluginControlTimeout != 0 {
		t.Fatalf("config = %#v", config)
	}
	settings.Plugins.DevRoots[0] = `C:\changed`
	settings.Processing.Overrides = append(settings.Processing.Overrides, ProcessingOverride{Name: "eye.left_gaze_x"})
	if config.PluginCatalog.DevRoots[0] != `C:\dev` || len(config.Processing.Overrides) != 0 {
		t.Fatal("application config aliases settings")
	}
	if _, err := application.NewApp(config); err != nil {
		t.Fatalf("application config not accepted: %v", err)
	}
}
