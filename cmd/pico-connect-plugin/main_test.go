package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/picoconnect"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
)

func TestSourceManifestMatchesDriverDescriptor(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "plugins", "pico-connect", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest plugins.Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	descriptor := picoconnect.New().Descriptor()
	if descriptor.ID != manifest.ID || descriptor.Name != manifest.Name || descriptor.Version != manifest.Version || descriptor.Description != manifest.Description || descriptor.Capabilities != manifest.Capabilities {
		t.Fatalf("descriptor=%+v manifest=%+v", descriptor, manifest)
	}
}
