package projectstatus

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRepositoryCatalogCoversEveryPackage(t *testing.T) {
	root, err := FindRepositoryRoot(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := DiscoverGoPackages(context.Background(), root, NewRunner(root))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalog(catalog, packages); err != nil {
		t.Fatal(err)
	}
	if got, want := len(catalog.Specs), 23; got != want {
		t.Fatalf("specs = %d, want %d", got, want)
	}
}
