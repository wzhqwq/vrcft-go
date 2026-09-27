package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/steamlink"
)

func TestSourceManifestMatchesDriverDescriptor(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "plugins", "steamlink", "manifest.json")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read source manifest: %v", err)
	}

	var manifest plugins.Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatalf("decode source manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate source manifest: %v", err)
	}
	wantManifest := plugins.Manifest{
		SchemaVersion: 1,
		ID:            "steamlink",
		Name:          "Steam Link",
		Version:       "0.1.0",
		Description:   "Eye and expression tracking from Steam Link OSC.",
		ProtocolMin:   1,
		ProtocolMax:   1,
		Entrypoint:    "steamlink-plugin.exe",
		Capabilities:  3,
	}
	if manifest != wantManifest {
		t.Fatalf("manifest = %#v, want %#v", manifest, wantManifest)
	}

	descriptor := steamlink.New().Descriptor()
	if descriptor.ID != manifest.ID ||
		descriptor.Name != manifest.Name ||
		descriptor.Version != manifest.Version ||
		descriptor.Description != manifest.Description ||
		descriptor.Capabilities != manifest.Capabilities {
		t.Fatalf("descriptor = %#v, want manifest identity %#v", descriptor, manifest)
	}
}
