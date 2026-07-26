package plugins

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeCatalogPlugin(t *testing.T, root, directory string, manifest Manifest) string {
	t.Helper()
	pluginDir := filepath.Join(root, directory)
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, manifest.Entrypoint), []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	return pluginDir
}

func newCatalog(t *testing.T, builtin string, devRoots ...string) Catalog {
	t.Helper()
	catalog, err := NewDirectoryCatalog(DirectoryCatalogConfig{
		BuiltinRoot:      builtin,
		DevRoots:         devRoots,
		MaxManifestBytes: 4096,
	})
	if err != nil {
		t.Fatalf("NewDirectoryCatalog() error = %v", err)
	}
	return catalog
}

func TestDirectoryCatalogSortsAndLabelsSources(t *testing.T) {
	builtinRoot := t.TempDir()
	devRoot := t.TempDir()
	first := validManifest()
	first.ID = "zeta.device"
	writeCatalogPlugin(t, builtinRoot, "zeta", first)
	second := validManifest()
	second.ID = "alpha.device"
	writeCatalogPlugin(t, devRoot, "alpha", second)

	plugins, err := newCatalog(t, builtinRoot, devRoot).Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("len(plugins) = %d, want 2", len(plugins))
	}
	if plugins[0].Manifest.ID != "alpha.device" || plugins[0].Source != SourceDev {
		t.Fatalf("plugins[0] = %+v, want alpha development plugin", plugins[0])
	}
	if plugins[1].Manifest.ID != "zeta.device" || plugins[1].Source != SourceBuiltin {
		t.Fatalf("plugins[1] = %+v, want zeta builtin plugin", plugins[1])
	}
}

func TestDirectoryCatalogRejectsUnknownManifestField(t *testing.T) {
	root := t.TempDir()
	pluginDir := writeCatalogPlugin(t, root, "plugin", validManifest())
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(`{"schemaVersion":1,"id":"vendor.device","name":"Vendor Device","version":"1.2.3","description":"Test device","protocolMin":1,"protocolMax":1,"entrypoint":"plugin.exe","capabilities":1,"unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newCatalog(t, root).Scan(context.Background()); err == nil {
		t.Fatal("Scan() error = nil, want rejection of unknown manifest field")
	}
}

func TestDirectoryCatalogRejectsOversizedManifest(t *testing.T) {
	root := t.TempDir()
	writeCatalogPlugin(t, root, "plugin", validManifest())
	catalog, err := NewDirectoryCatalog(DirectoryCatalogConfig{BuiltinRoot: root, MaxManifestBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Scan(context.Background()); err == nil {
		t.Fatal("Scan() error = nil, want oversized manifest rejection")
	}
}

func TestDirectoryCatalogRejectsDuplicateIDs(t *testing.T) {
	t.Run("within root", func(t *testing.T) {
		root := t.TempDir()
		writeCatalogPlugin(t, root, "one", validManifest())
		writeCatalogPlugin(t, root, "two", validManifest())
		if _, err := newCatalog(t, root).Scan(context.Background()); err == nil {
			t.Fatal("Scan() error = nil, want duplicate rejection")
		}
	})
	t.Run("across roots", func(t *testing.T) {
		builtinRoot := t.TempDir()
		devRoot := t.TempDir()
		writeCatalogPlugin(t, builtinRoot, "builtin", validManifest())
		writeCatalogPlugin(t, devRoot, "dev", validManifest())
		if _, err := newCatalog(t, builtinRoot, devRoot).Scan(context.Background()); err == nil {
			t.Fatal("Scan() error = nil, want duplicate rejection")
		}
	})
}

func TestDirectoryCatalogDoesNotAcceptPluginDirectorySymlinkEscape(t *testing.T) {
	catalogRoot := t.TempDir()
	externalRoot := t.TempDir()
	manifest := validManifest()
	manifest.ID = "outside.device"
	externalPluginRoot := writeCatalogPlugin(t, externalRoot, "outside", manifest)
	linkedPluginRoot := filepath.Join(catalogRoot, "linked-plugin")
	if err := os.Symlink(externalPluginRoot, linkedPluginRoot); err != nil {
		t.Skipf("directory symlink creation unavailable: %v", err)
	}

	if _, err := newCatalog(t, catalogRoot).Scan(context.Background()); err == nil {
		t.Fatal("Scan() error = nil, want rejection of plugin directory symlink")
	}
}

func TestDirectoryCatalogDoesNotAcceptPluginDirectoryJunctionEscape(t *testing.T) {
	catalogRoot := t.TempDir()
	externalRoot := t.TempDir()
	manifest := validManifest()
	manifest.ID = "outside.device"
	externalPluginRoot := writeCatalogPlugin(t, externalRoot, "outside", manifest)
	junction := filepath.Join(catalogRoot, "junction-plugin")
	if err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, externalPluginRoot).Run(); err != nil {
		t.Skipf("directory junction creation unavailable: %v", err)
	}
	if _, err := newCatalog(t, catalogRoot).Scan(context.Background()); err == nil {
		t.Fatal("Scan() error = nil, want rejection of plugin directory junction")
	}
}

func TestDirectoryCatalogHandlesMissingRoots(t *testing.T) {
	t.Run("missing development root is empty", func(t *testing.T) {
		builtinRoot := t.TempDir()
		writeCatalogPlugin(t, builtinRoot, "builtin", validManifest())
		missingDevRoot := filepath.Join(t.TempDir(), "missing")
		plugins, err := newCatalog(t, builtinRoot, missingDevRoot).Scan(context.Background())
		if err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if len(plugins) != 1 || plugins[0].Source != SourceBuiltin {
			t.Fatalf("plugins = %+v, want only builtin plugin", plugins)
		}
	})
	t.Run("missing builtin root is an error", func(t *testing.T) {
		missingBuiltinRoot := filepath.Join(t.TempDir(), "missing")
		if _, err := newCatalog(t, missingBuiltinRoot).Scan(context.Background()); err == nil {
			t.Fatal("Scan() error = nil, want missing builtin root rejection")
		}
	})
}

func TestDirectoryCatalogHonorsCanceledContext(t *testing.T) {
	root := t.TempDir()
	writeCatalogPlugin(t, root, "plugin", validManifest())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newCatalog(t, root).Scan(ctx); err == nil {
		t.Fatal("Scan() error = nil, want context cancellation")
	}
}

func TestDirectoryCatalogReturnsIndependentScanResults(t *testing.T) {
	root := t.TempDir()
	writeCatalogPlugin(t, root, "plugin", validManifest())
	catalog := newCatalog(t, root)
	first, err := catalog.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first[0].Manifest.ID = "mutated"
	second, err := catalog.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Manifest.ID != "vendor.device" {
		t.Fatalf("second scan ID = %q, want original manifest ID", second[0].Manifest.ID)
	}
}
