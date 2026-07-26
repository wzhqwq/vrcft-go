package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: 1,
		ID:            "vendor.device",
		Name:          "Vendor Device",
		Version:       "1.2.3",
		Description:   "Test device",
		ProtocolMin:   protocol.Version,
		ProtocolMax:   protocol.Version,
		Entrypoint:    "plugin.exe",
		Capabilities:  trackingmodel.CapabilityEye,
	}
}

func TestManifestValidateAcceptsValidManifest(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestManifestValidateRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"unsupported schema", func(m *Manifest) { m.SchemaVersion = 2 }},
		{"invalid ID", func(m *Manifest) { m.ID = "Vendor Device" }},
		{"invalid semantic version", func(m *Manifest) { m.Version = "1.2" }},
		{"blank name", func(m *Manifest) { m.Name = " \t" }},
		{"name exceeds bound", func(m *Manifest) { m.Name = strings.Repeat("n", 257) }},
		{"blank description", func(m *Manifest) { m.Description = "\n" }},
		{"description exceeds bound", func(m *Manifest) { m.Description = strings.Repeat("d", 4097) }},
		{"zero capabilities", func(m *Manifest) { m.Capabilities = 0 }},
		{"unknown capabilities", func(m *Manifest) { m.Capabilities = trackingmodel.Capability(1 << 16) }},
		{"protocol range below host", func(m *Manifest) { m.ProtocolMin, m.ProtocolMax = 0, 0 }},
		{"protocol range above host", func(m *Manifest) { m.ProtocolMin, m.ProtocolMax = protocol.Version+1, protocol.Version+1 }},
		{"empty entrypoint", func(m *Manifest) { m.Entrypoint = "" }},
		{"absolute entrypoint", func(m *Manifest) { m.Entrypoint = filepath.Join(t.TempDir(), "plugin.exe") }},
		{"UNC entrypoint", func(m *Manifest) { m.Entrypoint = `\\server\share\plugin.exe` }},
		{"volume entrypoint", func(m *Manifest) { m.Entrypoint = `C:plugin.exe` }},
		{"traversal entrypoint", func(m *Manifest) { m.Entrypoint = `bin\..\plugin.exe` }},
		{"NUL entrypoint", func(m *Manifest) { m.Entrypoint = "plugin\x00.exe" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			test.mutate(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want rejection")
			}
		})
	}
}

func TestResolveEntrypoint(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plugin.exe"), []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.exe")
	if err := os.WriteFile(outside, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		entrypoint string
		wantOK     bool
	}{
		{"regular file", "plugin.exe", true},
		{"missing file", "missing.exe", false},
		{"directory", "directory", false},
		{"parent escape", `..\outside.exe`, false},
		{"absolute path", outside, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executable, err := resolveEntrypoint(root, test.entrypoint)
			if test.wantOK {
				if err != nil {
					t.Fatalf("resolveEntrypoint() error = %v", err)
				}
				if !filepath.IsAbs(executable) {
					t.Fatalf("executable %q is not absolute", executable)
				}
				canonicalRoot, err := filepath.EvalSymlinks(root)
				if err != nil {
					t.Fatal(err)
				}
				relative, err := filepath.Rel(canonicalRoot, executable)
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					t.Fatalf("executable %q is outside root %q", executable, canonicalRoot)
				}
				return
			}
			if err == nil {
				t.Fatal("resolveEntrypoint() error = nil, want rejection")
			}
		})
	}

	link := filepath.Join(root, "escape.exe")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := resolveEntrypoint(root, "escape.exe"); err == nil {
		t.Fatal("resolveEntrypoint() accepted symlink escaping root")
	}
}
