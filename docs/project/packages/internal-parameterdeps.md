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
Map every OSC float and tracking-active Boolean output to primitive tracking inputs or active-state leaves.

## Responsibilities
Build direct and derived dependency plans, distinguish concrete eye fields, convert required eye fields to subscription validity, represent tracking-active sources, union all leaf kinds, and cover cycle, missing, and orphan dependency cases.

## Non-responsibilities
Numeric parameter evaluation and avatar planning are outside this package.

## Current implementation
A fixed package-global dependency table keyed by generated `ParameterID` provides typed `EyeField`, expression-mask, and `ActiveState` leaves, acyclic closure, required-input unions, and eye-validity conversion.

## Public/internal interfaces
Lookup and leaf-union APIs expose the package-global table, concrete eye/active leaf sets, and required `trackingmodel.EyeValid` conversion to callers.

## Owned data
The fixed package-global dependency table, its derived leaf closures, and primitive-input unions keyed by generated `ParameterID`.

## Dependencies
Uses generated `internal/parameters` IDs and `pkg/trackingmodel` primitive IDs at runtime. Tests use `internal/specparser` to load the YAML specification and prove complete dependency coverage.

## Concurrency and lifecycle
The fixed table is initialized as package data and read immutably for the process lifetime.

## Error handling
Cycles, missing dependencies, and orphan primitives are reported with dependency context.

## Performance constraints
Closure and required-input union are bounded by the fixed parameter and primitive catalogs.

## Security boundaries
Only catalog-defined parameter IDs and primitive tracking inputs participate in dependency plans.

## Required tests
Executable package and race tests cover every YAML float and tracking-active Boolean, concrete eye-field mapping, active-state mapping, leaf closure, required input union, cycle detection, missing dependencies, and orphan coverage.

## Known gaps
No package implementation gap is known; numeric evaluation and avatar planning are deliberate non-responsibilities.

## Completion definition
Every YAML detailed or simplified float and all three tracking-active Booleans resolve acyclically with no missing or orphan primitive input.
