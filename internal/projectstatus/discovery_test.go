package projectstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type staticRunner struct {
	output  CommandOutput
	request CommandRequest
}

func (runner *staticRunner) Run(_ context.Context, request CommandRequest) CommandOutput {
	runner.request = request
	return runner.output
}

func TestFindRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRepositoryRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}

func TestDiscoverGoPackages(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &staticRunner{output: CommandOutput{
		Stdout: "example.test/project/internal/osc\nexample.test/project\nexample.test/project/pkg/protocol\n",
	}}
	got, err := DiscoverGoPackages(context.Background(), root, runner)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".", "internal/osc", "pkg/protocol"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
	if runner.request.ID != "go-list" || !reflect.DeepEqual(runner.request.Args, []string{"./..."}) {
		t.Fatalf("request = %#v", runner.request)
	}
}

func TestValidateCatalogRejectsUnregisteredPackage(t *testing.T) {
	catalog := catalogWith(specForCatalog("root", "."))
	err := ValidateCatalog(catalog, []string{".", "internal/osc"})
	if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), "internal/osc") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCatalogRejectsDuplicatePath(t *testing.T) {
	catalog := catalogWith(specForCatalog("one", "internal/shared"), specForCatalog("two", "internal/shared"))
	err := ValidateCatalog(catalog, []string{"internal/shared"})
	if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), "duplicate path") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCatalogRejectsUnknownDependency(t *testing.T) {
	spec := specForCatalog("one", "internal/one")
	spec.DependsOn = []string{"absent"}
	err := ValidateCatalog(catalogWith(spec), []string{"internal/one"})
	if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCatalogRejectsDependencyCycle(t *testing.T) {
	a := specForCatalog("a", "internal/a")
	b := specForCatalog("b", "internal/b")
	c := specForCatalog("c", "internal/c")
	a.DependsOn, b.DependsOn, c.DependsOn = []string{"b"}, []string{"c"}, []string{"a"}
	err := ValidateCatalog(catalogWith(a, b, c), []string{"internal/a", "internal/b", "internal/c"})
	if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), "a -> b -> c -> a") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCatalogAcceptsMissingPlannedPackage(t *testing.T) {
	root := specForCatalog("root", ".")
	planned := specForCatalog("future", "internal/future")
	planned.Planned = true
	if err := ValidateCatalog(catalogWith(root, planned), []string{"."}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCatalogRejectsDiscoveredPackageStillMarkedPlanned(t *testing.T) {
	planned := specForCatalog("future", "internal/future")
	planned.Planned = true
	err := ValidateCatalog(catalogWith(planned), []string{"internal/future"})
	if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), "planned") {
		t.Fatalf("error = %v", err)
	}
}

func specForCatalog(id, packagePath string) Spec {
	return Spec{
		ID: id, Kind: KindGoPackage, Path: packagePath, Milestone: "M1",
		Checks: []CheckSpec{{
			ID: "exists", Description: "exists", Type: CheckFile,
			Path: packagePath, Weight: 1, Required: true,
		}},
	}
}

func catalogWith(specs ...Spec) *Catalog {
	return &Catalog{Specs: specs}
}
