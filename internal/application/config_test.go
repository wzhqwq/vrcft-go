package application

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/processing"
)

func TestConfigNormalizeDefaultsAndRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		check  func(t *testing.T, normalized normalizedConfig)
		want   error
		text   string
	}{
		{
			name: "zero frame interval defaults",
			check: func(t *testing.T, normalized normalizedConfig) {
				t.Helper()
				if normalized.frameInterval != 10*time.Millisecond {
					t.Fatalf("frameInterval = %v, want 10ms", normalized.frameInterval)
				}
			},
		},
		{
			name: "zero plugin control timeout defaults",
			check: func(t *testing.T, normalized normalizedConfig) {
				t.Helper()
				if normalized.pluginControlTimeout != 2*time.Second {
					t.Fatalf("pluginControlTimeout = %v, want 2s", normalized.pluginControlTimeout)
				}
			},
		},
		{
			name:   "negative frame interval",
			mutate: func(config *Config) { config.FrameInterval = -time.Nanosecond },
			want:   ErrInvalidConfig,
		},
		{
			name:   "negative plugin control timeout",
			mutate: func(config *Config) { config.PluginControlTimeout = -time.Nanosecond },
			want:   ErrInvalidConfig,
		},
		{
			name:   "empty avatar OSC root",
			mutate: func(config *Config) { config.Avatar.OSCRoot = "" },
			want:   avatar.ErrInvalidPlannerConfig,
		},
		{
			name:   "empty plugin builtin root",
			mutate: func(config *Config) { config.PluginCatalog.BuiltinRoot = "" },
			want:   ErrInvalidConfig,
			text:   "plugin catalog requires a builtin root",
		},
		{
			name:   "empty plugin store path",
			mutate: func(config *Config) { config.PluginStorePath = "" },
			want:   ErrInvalidConfig,
		},
		{
			name:   "zero plugin store max bytes",
			mutate: func(config *Config) { config.PluginStoreMaxBytes = 0 },
			want:   ErrInvalidConfig,
		},
		{
			name:   "invalid processing configuration",
			mutate: func(config *Config) { config.Processing.ActiveStaleAfter = 0 },
			want:   processing.ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validApplicationConfig(t)
			if tt.mutate != nil {
				tt.mutate(&config)
			}
			normalized, err := normalizeConfig(config)
			if tt.want != nil {
				if !errors.Is(err, ErrInvalidConfig) {
					t.Fatalf("normalizeConfig() error = %v, want errors.Is(_, ErrInvalidConfig)", err)
				}
				if !errors.Is(err, tt.want) {
					t.Fatalf("normalizeConfig() error = %v, want errors.Is(_, %v)", err, tt.want)
				}
				if tt.text != "" && !strings.Contains(err.Error(), tt.text) {
					t.Fatalf("normalizeConfig() error = %q, want %q", err, tt.text)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeConfig() error = %v", err)
			}
			tt.check(t, normalized)
		})
	}
}

func TestConfigNormalizeOwnsCallerCollectionsAndUsesExternalOSCMode(t *testing.T) {
	config := validApplicationConfig(t)
	config.PluginCatalog.DevRoots = []string{"first-dev-root"}
	config.Processing.Overrides = map[processing.ChannelID]processing.ChannelConfig{
		processing.ChannelEyeLeftGazeX: config.Processing.DefaultChannel,
	}
	config.Processing.MutualExclusion = [][]processing.ChannelID{{
		processing.ChannelEyeLeftGazeX,
		processing.ChannelEyeRightGazeX,
	}}
	config.OSC.CatalogMode = osc.CatalogOSCQuery
	config.OSC.Interfaces = []net.Interface{{Name: "original-interface", HardwareAddr: net.HardwareAddr{1, 2, 3}}}

	normalized, err := normalizeConfig(config)
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}

	config.PluginCatalog.DevRoots[0] = "changed-dev-root"
	config.Processing.Overrides[processing.ChannelEyeLeftGazeX] = processing.ChannelConfig{}
	config.Processing.MutualExclusion[0][0] = processing.ChannelEyeLeftGazeY
	config.OSC.Interfaces[0].Name = "changed-interface"
	config.OSC.Interfaces[0].HardwareAddr[0] = 9

	if got := normalized.pluginCatalog.DevRoots[0]; got != "first-dev-root" {
		t.Fatalf("normalized dev root = %q, want original value", got)
	}
	if got := normalized.processing.Overrides[processing.ChannelEyeLeftGazeX]; got != config.Processing.DefaultChannel {
		t.Fatalf("normalized processing override = %#v, want original channel config", got)
	}
	if got := normalized.processing.MutualExclusion[0][0]; got != processing.ChannelEyeLeftGazeX {
		t.Fatalf("normalized processing mutual exclusion first value = %d, want original value", got)
	}
	if got := normalized.osc.CatalogMode; got != osc.CatalogExternal {
		t.Fatalf("normalized OSC catalog mode = %d, want CatalogExternal", got)
	}
	if got := normalized.osc.Interfaces[0].Name; got != "original-interface" {
		t.Fatalf("normalized OSC interface name = %q, want original value", got)
	}
	if got := normalized.osc.Interfaces[0].HardwareAddr[0]; got != 1 {
		t.Fatalf("normalized OSC interface address byte = %d, want original value", got)
	}
}

func TestConfigNormalizeHasNoConstructionSideEffects(t *testing.T) {
	root := t.TempDir()
	config := validApplicationConfig(t)
	config.Avatar.OSCRoot = filepath.Join(root, "avatar")
	config.PluginCatalog.BuiltinRoot = filepath.Join(root, "builtin")
	config.PluginStorePath = filepath.Join(root, "plugin-settings.json")

	beforeEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	beforeGoroutines := runtime.NumGoroutine()
	if _, err := normalizeConfig(config); err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	afterEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEntries) != len(beforeEntries) {
		t.Fatalf("normalizeConfig() created filesystem entries: before %d, after %d", len(beforeEntries), len(afterEntries))
	}
	if got := runtime.NumGoroutine(); got != beforeGoroutines {
		t.Fatalf("normalizeConfig() goroutines = %d after %d before, want no goroutine started", got, beforeGoroutines)
	}
}

func validApplicationConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	return Config{
		Avatar: avatar.PlannerConfig{
			OSCRoot: filepath.Join(root, "avatar"),
		},
		PluginCatalog: plugins.DirectoryCatalogConfig{
			BuiltinRoot: filepath.Join(root, "builtin"),
		},
		PluginStorePath:     filepath.Join(root, "plugins.json"),
		PluginStoreMaxBytes: 1024,
		PluginOptions:       plugins.DefaultOptions(),
		Processing:          processing.DefaultConfig(),
		OSC:                 osc.ControllerConfig{},
	}
}
