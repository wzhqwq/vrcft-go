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
Describe every OSC float and tracking-active Boolean as a typed operation over primitive tracking inputs, active-state leaves, parameter dependencies, or a combination of those operands.

## Responsibilities
Build direct and derived dependency plans, support generalized operands from both `Inputs` and `DependsOn`, distinguish concrete eye fields, convert required eye fields to subscription validity, represent independent tracking-active sources, union all leaf kinds, and cover cycle, missing, and orphan dependency cases.

## Non-responsibilities
Numeric parameter evaluation belongs to `internal/evaluator`; avatar planning/binding, OSC networking, persistence/UI, numeric Lip payloads, and Expression-to-Lip mapping are outside this package.

## Current implementation
A fixed package-global dependency table keyed by generated `ParameterID` provides typed `EyeField`, expression-mask, `ActiveState`, and parameter-reference operands; operation metadata for Direct, Average, Max, SignedPair, and SumClamp; acyclic leaf closure; required-input unions; and eye-validity conversion. `ParameterPupilDilation` is metadata-defined as `OperationAverage` over exactly the left and right pupil-dilation primitive eye fields.

## Public/internal interfaces
`Plan` returns an owned `DependencyPlan` with generalized `Inputs`, `DependsOn`, and `Operation` metadata. `ResolveLeaves` and `RequiredInputs` expose concrete eye/expression/active leaf closure and required `trackingmodel.EyeValid` conversion.

## Owned data
The fixed package-global dependency table and operation/operand metadata keyed by generated `ParameterID`; callers receive defensive copies of dependency slices.

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
Executable package and race tests cover every YAML float and tracking-active Boolean, concrete eye-field mapping, independent active-state mapping, generalized primitive/dependency operands, PupilDilation average metadata, leaf closure, required input union, defensive plan copies, cycle detection, missing dependencies, and orphan coverage.

## Known gaps
No M4 dependency-metadata implementation gap is known. M5 avatar planning/binding is implemented by `internal/avatar`; M6 Application installation and evaluator-to-OSC composition plus M7 persistence/UI remain incomplete and outside this package. Numeric Lip payload and Expression-to-Lip mapping decisions remain deferred.

## Completion definition
Every YAML detailed or simplified float and all three independent tracking-active Booleans have typed operation/operand metadata and resolve acyclically with no missing or orphan primitive input.
