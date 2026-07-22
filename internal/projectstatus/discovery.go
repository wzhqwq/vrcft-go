package projectstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrInvalidCatalog = errors.New("invalid project spec catalog")

type Catalog struct {
	Specs []Spec
	ByID  map[string]int
}

func FindRepositoryRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(current); statErr == nil && !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("repository root not found from %s", start)
		}
		current = parent
	}
}

func LoadCatalog(root string) (*Catalog, error) {
	patterns := []string{
		filepath.Join(root, "docs", "project", "packages", "*.md"),
		filepath.Join(root, "docs", "project", "subsystems", "*.md"),
	}
	var paths []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	catalog := &Catalog{ByID: make(map[string]int)}
	for _, specPath := range paths {
		content, err := os.ReadFile(specPath)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(root, specPath)
		if err != nil {
			return nil, err
		}
		spec, err := ParseSpec(filepath.ToSlash(relative), content)
		if err != nil {
			return nil, err
		}
		if _, exists := catalog.ByID[spec.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate id %q", ErrInvalidCatalog, spec.ID)
		}
		catalog.ByID[spec.ID] = len(catalog.Specs)
		catalog.Specs = append(catalog.Specs, spec)
	}
	return catalog, nil
}

func DiscoverGoPackages(ctx context.Context, root string, runner CommandRunner) ([]string, error) {
	moduleName, err := readModuleName(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	output := runner.Run(ctx, CommandRequest{
		ID: "go-list", Args: []string{"./..."}, Timeout: 120 * time.Second, Dir: root,
	})
	if output.ExitCode != 0 || output.TimedOut {
		return nil, fmt.Errorf("go package discovery failed: %s", strings.TrimSpace(output.Stderr))
	}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output.Stdout, "\n") {
		importPath := strings.TrimSpace(line)
		if importPath == "" {
			continue
		}
		var packagePath string
		switch {
		case importPath == moduleName:
			packagePath = "."
		case strings.HasPrefix(importPath, moduleName+"/"):
			packagePath = strings.TrimPrefix(importPath, moduleName+"/")
		default:
			return nil, fmt.Errorf("discovered package %q is outside module %q", importPath, moduleName)
		}
		seen[filepath.ToSlash(packagePath)] = struct{}{}
	}
	packages := make([]string, 0, len(seen))
	for packagePath := range seen {
		packages = append(packages, packagePath)
	}
	sort.Strings(packages)
	return packages, nil
}

func ValidateCatalog(catalog *Catalog, discovered []string) error {
	if catalog == nil {
		return fmt.Errorf("%w: nil catalog", ErrInvalidCatalog)
	}
	catalog.ByID = make(map[string]int, len(catalog.Specs))
	byPath := make(map[string]string, len(catalog.Specs))
	validMilestones := map[string]bool{"M0": true, "M1": true, "M2": true, "M3": true, "M4": true, "M5": true, "M6": true, "M7": true}
	for index, spec := range catalog.Specs {
		if _, exists := catalog.ByID[spec.ID]; exists {
			return fmt.Errorf("%w: duplicate id %q", ErrInvalidCatalog, spec.ID)
		}
		catalog.ByID[spec.ID] = index
		if owner, exists := byPath[spec.Path]; exists {
			return fmt.Errorf("%w: duplicate path %q used by %s and %s", ErrInvalidCatalog, spec.Path, owner, spec.ID)
		}
		byPath[spec.Path] = spec.ID
		if !validMilestones[spec.Milestone] {
			return fmt.Errorf("%w: %s uses unknown milestone %q", ErrInvalidCatalog, spec.ID, spec.Milestone)
		}
	}
	for _, spec := range catalog.Specs {
		for _, dependency := range spec.DependsOn {
			if _, exists := catalog.ByID[dependency]; !exists {
				return fmt.Errorf("%w: %s depends on unknown spec %s", ErrInvalidCatalog, spec.ID, dependency)
			}
		}
	}
	if cycle := dependencyCycle(catalog); len(cycle) > 0 {
		return fmt.Errorf("%w: dependency cycle %s", ErrInvalidCatalog, strings.Join(cycle, " -> "))
	}

	discoveredSet := make(map[string]struct{}, len(discovered))
	for _, packagePath := range discovered {
		discoveredSet[filepath.ToSlash(packagePath)] = struct{}{}
	}
	for packagePath := range discoveredSet {
		id, exists := byPath[packagePath]
		if !exists || catalog.Specs[catalog.ByID[id]].Kind != KindGoPackage {
			return fmt.Errorf("%w: discovered package %s is not registered", ErrInvalidCatalog, packagePath)
		}
		if catalog.Specs[catalog.ByID[id]].Planned {
			return fmt.Errorf("%w: discovered package %s is still marked planned", ErrInvalidCatalog, packagePath)
		}
	}
	for _, spec := range catalog.Specs {
		if spec.Kind != KindGoPackage || spec.Planned {
			continue
		}
		if _, exists := discoveredSet[spec.Path]; !exists {
			return fmt.Errorf("%w: registered package %s was not discovered", ErrInvalidCatalog, spec.Path)
		}
	}
	return nil
}

func dependencyCycle(catalog *Catalog) []string {
	const (
		unvisited = iota
		visiting
		visited
	)
	states := make(map[string]int, len(catalog.Specs))
	stack := make([]string, 0, len(catalog.Specs))
	var visit func(string) []string
	visit = func(id string) []string {
		states[id] = visiting
		stack = append(stack, id)
		spec := catalog.Specs[catalog.ByID[id]]
		for _, dependency := range spec.DependsOn {
			switch states[dependency] {
			case unvisited:
				if cycle := visit(dependency); len(cycle) > 0 {
					return cycle
				}
			case visiting:
				start := 0
				for stack[start] != dependency {
					start++
				}
				cycle := append([]string(nil), stack[start:]...)
				return append(cycle, dependency)
			}
		}
		stack = stack[:len(stack)-1]
		states[id] = visited
		return nil
	}
	for _, spec := range catalog.Specs {
		if states[spec.ID] == unvisited {
			if cycle := visit(spec.ID); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

func readModuleName(goModPath string) (string, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("module directive not found in %s", goModPath)
}
