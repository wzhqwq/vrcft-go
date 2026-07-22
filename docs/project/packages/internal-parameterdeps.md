---
id: internal-parameterdeps
kind: go-package
path: internal/parameterdeps
milestone: M1
depends_on: [internal-parameters, internal-specparser, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Parameter dependency package tests pass
    type: command
    command: go-test
    args: [./internal/parameterdeps]
    weight: 3
    required: true
  - id: package-race-tests
    description: Parameter dependency package race tests pass
    type: command
    command: go-test-race
    args: [./internal/parameterdeps]
    weight: 2
    required: true
---
# Package: internal/parameterdeps

## Purpose
Map every OSC float output to primitive tracking inputs.

## Responsibilities
Build direct and derived dependency plans, close leaf dependencies, union required primitive inputs, and cover cycle, missing, and orphan dependency cases.

## Non-responsibilities
Numeric parameter evaluation and avatar planning are outside this package.

## Current implementation
Dependency planning, acyclic leaf closure, required-input union, and cycle, missing, and orphan validation are implemented.

## Public/internal interfaces
Dependency-plan and closure APIs consumed by parameter catalog validation and downstream planning.

## Owned data
In-memory direct and derived dependency plans and primitive-input sets.

## Dependencies
Consumes generated `internal/parameters` catalog entries, parsed `internal/specparser` semantics, and `pkg/trackingmodel` primitive IDs.

## Concurrency and lifecycle
Plans are constructed per input document and can be read immutably after validation.

## Error handling
Cycles, missing dependencies, and orphan primitives are reported with dependency context.

## Performance constraints
Closure and required-input union are bounded by the fixed parameter and primitive catalogs.

## Security boundaries
Only catalog-defined parameter IDs and primitive tracking inputs participate in dependency plans.

## Required tests
Executable package and race tests cover direct and derived plans, leaf closure, required input union, cycle detection, missing dependencies, and orphan coverage.

## Known gaps
No package implementation gap is known; numeric evaluation and avatar planning are deliberate non-responsibilities.

## Completion definition
Every YAML detailed or simplified float resolves acyclically with no missing or orphan primitive input.
