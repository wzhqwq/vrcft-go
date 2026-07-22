package projectstatus

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestParseSpec(t *testing.T) {
	content, err := os.ReadFile("testdata/valid.md")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := ParseSpec("testdata/valid.md", content)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ID != "internal-osc" || spec.Kind != KindGoPackage || spec.Path != "internal/osc" {
		t.Fatalf("unexpected spec: %#v", spec)
	}
	if len(spec.Checks) != 1 || spec.Checks[0].Command != "go-test" || spec.Checks[0].Weight != 3 {
		t.Fatalf("unexpected checks: %#v", spec.Checks)
	}
	if spec.SourcePath != "testdata/valid.md" || !strings.Contains(spec.Body, "## Completion definition") {
		t.Fatalf("source/body not retained: %#v", spec)
	}
}

func TestParseSpecRejectsMissingRequiredSection(t *testing.T) {
	content, err := os.ReadFile("testdata/missing-section.md")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseSpec("testdata/missing-section.md", content)
	if !errors.Is(err, ErrMissingSection) {
		t.Fatalf("error = %v, want ErrMissingSection", err)
	}
}

func TestParseSpecRejectsInvalidMetadata(t *testing.T) {
	valid, err := os.ReadFile("testdata/valid.md")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "duplicate check", old: "blockers:\n", new: "  - id: package-tests\n    description: duplicate\n    type: file\n    path: x\n    weight: 1\n    required: true\nblockers:\n"},
		{name: "zero weight", old: "weight: 3", new: "weight: 0"},
		{name: "unknown kind", old: "kind: go-package", new: "kind: unknown"},
		{name: "absolute path", old: "path: internal/osc", new: "path: C:/outside"},
		{name: "parent path", old: "path: internal/osc", new: "path: ../outside"},
		{name: "unknown check type", old: "type: command", new: "type: mystery"},
		{name: "unknown blocker check", old: "check: package-tests", new: "check: absent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := strings.Replace(string(valid), test.old, test.new, 1)
			_, err := ParseSpec("invalid.md", []byte(content))
			if !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("error = %v, want ErrInvalidSpec", err)
			}
		})
	}
}

func TestParseSpecRejectsMalformedFrontMatter(t *testing.T) {
	_, err := ParseSpec("bad.md", []byte("---\nid: bad\n"))
	if !errors.Is(err, ErrMalformedFrontMatter) {
		t.Fatalf("error = %v, want ErrMalformedFrontMatter", err)
	}
}
