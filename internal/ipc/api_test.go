package ipc

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePipeName(t *testing.T) {
	valid := []string{
		"a",
		"plugin_01",
		strings.Repeat("a", 128),
	}
	for _, name := range valid {
		t.Run("valid_"+name[:1], func(t *testing.T) {
			if err := validatePipeName(name); err != nil {
				t.Fatalf("validatePipeName(%q) error = %v", name, err)
			}
		})
	}

	invalid := []string{
		"",
		" ",
		strings.Repeat("a", 129),
		".",
		"a.b",
		"a/b",
		`a\b`,
		"a:b",
		"插件",
		`\\server\pipe\x`,
	}
	for index, name := range invalid {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			err := validatePipeName(name)
			if !errors.Is(err, ErrInvalidPipeName) {
				t.Fatalf("validatePipeName(%q) error = %v, want ErrInvalidPipeName", name, err)
			}
			if strings.TrimSpace(name) != "" && strings.Contains(err.Error(), name) {
				t.Fatalf("validatePipeName(%q) leaked input in error %q", name, err)
			}
		})
	}
}

func TestPipePath(t *testing.T) {
	if got := pipePath("plugin_01"); got != `\\.\pipe\vrcft-plugin_01` {
		t.Fatalf("pipePath() = %q", got)
	}
}
