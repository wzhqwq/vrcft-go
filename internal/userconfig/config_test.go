package userconfig

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
)

// This catches any future change that lets a Wails-sized candidate allocate or
// normalize before its public field, nested-collection, and aggregate limits
// have been established.
func TestValidateCandidateBoundsEnforcesExactAndNestedLimits(t *testing.T) {
	base := DefaultCandidate(Paths{DefaultOSCRoot: `C:\VRChat\OSC`})
	if err := ValidateCandidateBounds(base); err != nil {
		t.Fatalf("ValidateCandidateBounds(default) error = %v", err)
	}

	tests := []struct {
		name, field string
		mutate      func(*Candidate)
	}{
		{"oversized path", "avatar.fallbackPath", func(c *Candidate) { c.Avatar.FallbackPath = strings.Repeat("a", MaxSettingsPathBytes+1) }},
		{"invalid utf8 path", "avatar.oscRoot", func(c *Candidate) { c.Avatar.OSCRoot = string([]byte{'a', 0xff}) }},
		{"too many development roots", "plugins.devRoots", func(c *Candidate) { c.Plugins.DevRoots = make([]string, MaxSettingsDevRoots+1) }},
		{"too many overrides", "processing.overrides", func(c *Candidate) { c.Processing.Overrides = make([]ProcessingOverride, MaxSettingsOverrides+1) }},
		{"too many mutual exclusion groups", "processing.mutualExclusion", func(c *Candidate) {
			c.Processing.MutualExclusion = make([][]string, MaxSettingsMutualExclusionGroups+1)
		}},
		{"too many mutual exclusion members", "processing.mutualExclusion", func(c *Candidate) {
			c.Processing.MutualExclusion = [][]string{make([]string, MaxSettingsMutualExclusionMembers+1)}
		}},
		{"oversized nested name", "processing.overrides", func(c *Candidate) {
			c.Processing.Overrides = []ProcessingOverride{{Name: strings.Repeat("a", MaxSettingsChannelNameBytes+1)}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base.Clone()
			test.mutate(&candidate)
			err := ValidateCandidateBounds(candidate)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Field != test.field {
				t.Fatalf("ValidateCandidateBounds() error = %v, want field %q", err, test.field)
			}
		})
	}
}

func TestValidateCandidateBoundsRejectsValuesThatCannotBeEncodedAsJSON(t *testing.T) {
	candidate := DefaultCandidate(Paths{DefaultOSCRoot: `C:\VRChat\OSC`})
	candidate.Processing.DefaultChannel.Calibration.Neutral = float32(math.NaN())
	err := ValidateCandidateBounds(candidate)
	var validationError *ValidationError
	if !errors.As(err, &validationError) || validationError.Field != "settings" {
		t.Fatalf("ValidateCandidateBounds(NaN) error = %v, want settings validation", err)
	}
}

func TestValidateCandidateBoundsEnforcesEncodedAggregateLimit(t *testing.T) {
	// Seven maximum-length Windows paths leave less than one path's worth of
	// aggregate JSON space. The last root can therefore exercise the precise
	// 256 KiB wire boundary without exceeding an individual field limit.
	candidate := Candidate{
		Avatar: Avatar{
			OSCRoot:      strings.Repeat("a", MaxSettingsPathBytes),
			FallbackPath: strings.Repeat("b", MaxSettingsPathBytes),
		},
		Plugins: Plugins{DevRoots: []string{
			strings.Repeat("c", MaxSettingsPathBytes),
			strings.Repeat("d", MaxSettingsPathBytes),
			strings.Repeat("e", MaxSettingsPathBytes),
			strings.Repeat("f", MaxSettingsPathBytes),
			strings.Repeat("g", MaxSettingsPathBytes),
			"",
		}},
	}
	encodedSize := func(value Candidate) int {
		t.Helper()
		data, err := json.Marshal(Settings{SchemaVersion: SchemaVersion, Revision: 1, Avatar: value.Avatar, Plugins: value.Plugins, Processing: value.Processing, OSC: value.OSC})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		return len(data)
	}
	remaining := MaxSettingsBytes - encodedSize(candidate)
	if remaining <= 0 || remaining > MaxSettingsPathBytes {
		t.Fatalf("aggregate fixture remaining path bytes = %d, want 1..%d", remaining, MaxSettingsPathBytes)
	}
	candidate.Plugins.DevRoots[len(candidate.Plugins.DevRoots)-1] = strings.Repeat("h", remaining)
	if got := encodedSize(candidate); got != MaxSettingsBytes {
		t.Fatalf("encoded candidate size = %d, want %d", got, MaxSettingsBytes)
	}
	if err := ValidateCandidateBounds(candidate); err != nil {
		t.Fatalf("ValidateCandidateBounds(exact aggregate) error = %v", err)
	}
	candidate.Plugins.DevRoots[len(candidate.Plugins.DevRoots)-1] += "i"
	if err := ValidateCandidateBounds(candidate); err == nil {
		t.Fatal("ValidateCandidateBounds(maximum plus one) succeeded")
	} else {
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Field != "settings" {
			t.Fatalf("ValidateCandidateBounds(maximum plus one) error = %v, want settings validation", err)
		}
	}
}

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

func TestNormalizeManualTargetAcceptsOnlyUnicastLiterals(t *testing.T) {
	base := DefaultCandidate(Paths{DefaultOSCRoot: `C:\VRChat\OSC`})
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"IPv4 loopback", "127.0.0.1", false},
		{"IPv6 loopback", "::1", false},
		{"IPv4 mapped unicast", "::ffff:127.0.0.1", false},
		{"IPv4 unspecified", "0.0.0.0", true},
		{"IPv6 unspecified", "::", true},
		{"IPv4 multicast", "224.0.0.1", true},
		{"IPv6 multicast", "ff02::1", true},
		{"IPv4 broadcast", "255.255.255.255", true},
		{"mapped IPv4 unspecified", "::ffff:0.0.0.0", true},
		{"mapped IPv4 multicast", "::ffff:224.0.0.1", true},
		{"mapped IPv4 broadcast", "::ffff:255.255.255.255", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := base.Clone()
			candidate.OSC.TargetMode = osc.TargetModeManual
			candidate.OSC.ManualHost = tt.host
			candidate.OSC.ManualPort = 9000
			_, err := Normalize(candidate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Normalize(%q) error = %v, want error %t", tt.host, err, tt.wantErr)
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
	if err := application.ValidateConfig(config); err != nil {
		t.Fatalf("application config validation failed: %v", err)
	}
}
