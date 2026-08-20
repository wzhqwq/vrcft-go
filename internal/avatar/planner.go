package avatar

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/wzhqwq/vrcft-go/internal/evaluator"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/parameterdeps"
	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

type PlannerConfig struct {
	OSCRoot      string
	FallbackPath string
}

type Planner struct {
	mu           sync.Mutex
	generation   uint64
	oscRoot      string
	fallbackPath string
	specs        *osc.ParameterCatalog
}

func NewPlanner(config PlannerConfig) (*Planner, error) {
	if config.OSCRoot == "" {
		return nil, fmt.Errorf("%w: OSC root is empty", ErrInvalidPlannerConfig)
	}

	oscRoot, err := absoluteCleanPath(config.OSCRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: OSC root %q: %v", ErrInvalidPlannerConfig, config.OSCRoot, err)
	}

	var fallbackPath string
	if config.FallbackPath != "" {
		fallbackPath, err = absoluteCleanPath(config.FallbackPath)
		if err != nil {
			return nil, fmt.Errorf("%w: fallback path %q: %v", ErrInvalidPlannerConfig, config.FallbackPath, err)
		}
	}

	specs, err := osc.NewVRCFTParameterCatalog()
	if err != nil {
		return nil, fmt.Errorf("%w: compile OSC parameter catalog: %w", ErrInvalidPlannerConfig, err)
	}

	return &Planner{
		oscRoot:      oscRoot,
		fallbackPath: fallbackPath,
		specs:        specs,
	}, nil
}

func (p *Planner) Activate(avatarID string) Result {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.generation == math.MaxUint64 {
		return Result{Err: fmt.Errorf("%w: counter reached %d", ErrGenerationExhausted, uint64(math.MaxUint64))}
	}
	p.generation++
	generation := p.generation

	if err := validateAvatarID(avatarID); err != nil {
		return failedResult(generation, avatarID, SourceNone, "", "", err)
	}

	resolved, err := resolveConfig(p.oscRoot, p.fallbackPath, avatarID)
	if err != nil {
		return failedResult(generation, avatarID, SourceNone, "", "", err)
	}

	config, err := readConfig(resolved.path)
	if err != nil {
		return failedResult(
			generation,
			avatarID,
			resolved.source,
			resolved.path,
			"",
			fmt.Errorf("decode configuration %q: %w", resolved.path, err),
		)
	}
	if resolved.requireIDMatch && config.id != avatarID {
		return failedResult(
			generation,
			avatarID,
			resolved.source,
			resolved.path,
			config.id,
			fmt.Errorf("%w: configuration ID %q does not match avatar ID %q", ErrConfigIDMismatch, config.id, avatarID),
		)
	}

	catalog, err := osc.BuildCatalogFromEndpoints(config.endpoints, p.specs, generation)
	if err != nil {
		return failedResult(
			generation,
			avatarID,
			resolved.source,
			resolved.path,
			config.id,
			fmt.Errorf("%w: %w", ErrBindingCompilation, err),
		)
	}

	ids := sortedBindingIDs(catalog)
	evaluatorPlan, err := evaluator.Compile(ids)
	if err != nil {
		return failedResult(
			generation,
			avatarID,
			resolved.source,
			resolved.path,
			config.id,
			fmt.Errorf("%w: evaluator: %w", ErrRequirementCompilation, err),
		)
	}
	inputs, err := parameterdeps.RequiredInputs(ids)
	if err != nil {
		return failedResult(
			generation,
			avatarID,
			resolved.source,
			resolved.path,
			config.id,
			fmt.Errorf("%w: dependencies: %w", ErrRequirementCompilation, err),
		)
	}

	return Result{Plan: newReadyPlan(
		generation,
		avatarID,
		config.id,
		resolved.path,
		resolved.source,
		ids,
		catalog,
		evaluatorPlan,
		inputs,
	)}
}

func sortedBindingIDs(catalog *osc.Catalog) []parameters.ParameterID {
	ids := make([]parameters.ParameterID, 0, len(catalog.Bindings))
	for id := range catalog.Bindings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func failedResult(generation uint64, avatarID string, source Source, path, configID string, err error) Result {
	return Result{
		Plan: newFailedPlan(generation, avatarID, source, path, configID),
		Err:  err,
	}
}
