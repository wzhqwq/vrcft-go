---
id: internal-evaluator
kind: go-package
path: internal/evaluator
milestone: M4
depends_on: [internal-processing, internal-parameterdeps, internal-parameters, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Evaluator package tests pass
    type: command
    command: go-test
    args: [./internal/evaluator]
    weight: 3
    required: true
  - id: package-race-tests
    description: Evaluator package race tests pass
    type: command
    command: go-test-race
    args: [./internal/evaluator]
    weight: 2
    required: true
---
# Package: internal/evaluator

## Purpose
Compile selected generated VRCFT parameter IDs into immutable evaluation plans and evaluate canonical tracking values into owned, typed snapshots.

## Responsibilities
Build deterministic dependency-first plans for only the requested outputs and their hidden dependencies; validate parameter existence, dependency closure, operation arity, and operand/output types; evaluate primitive Eye, Expression, active-state, and parameter operands; require every operand to be valid and finite; apply Direct, Average, Max, SignedPair, and SumClamp; clamp every valid float to its generated range; and expose only requested typed results.

## Non-responsibilities
Tracking ingest/routing and stateful transforms belong upstream. Avatar planning/binding, OSC networking and lifecycle, persistence/UI, Application wiring, numeric Lip payloads, and Expression-to-Lip mapping remain outside this package.

## Current implementation
`Compile` validates every requested generated ID, deduplicates and orders roots by stable generated ID, performs dependency-first DFS, copies generalized primitive and parameter operands into private instructions, and rejects missing plans, cycles, invalid operations, arity errors, and type mismatches. `Plan.Evaluate` executes that fixed plan against a `processing.CanonicalFrame`, contains invalid or non-finite operands by omitting the affected result, clamps valid floats through generated metadata, and returns a value-type `Snapshot` containing only requested outputs. Eye, Expression, and Lip active Booleans are independent; false is a valid Boolean value.

## Public/internal interfaces
`Compile([]parameters.ParameterID) (*Plan, error)`, immutable `Plan.Evaluate(processing.CanonicalFrame) Snapshot`, and typed `Snapshot.Float`/`Snapshot.Bool` accessors. Unknown IDs, wrong requested types, hidden dependencies, and invalid results are not exposed as values.

## Owned data
A plan owns a fixed requested-ID bitset plus private instruction and operand slices that do not retain the caller request or dependency slices. Each returned `Snapshot` owns dense fixed float/Boolean arrays and fixed validity bitsets; it retains no frame, map, slice, or pointer-backed result data.

## Dependencies
Consumes canonical frames from `internal/processing`, generalized dependency/operation metadata from `internal/parameterdeps`, generated IDs, types, and clamp ranges from `internal/parameters`, and primitive Eye/Expression value types from `pkg/trackingmodel`. It has no dependency on `internal/osc` or Application wiring.

## Concurrency and lifecycle
Compilation is synchronous. A compiled plan is immutable and can be shared by concurrent callers; evaluation allocates all mutable work per call and returns an owned value snapshot. The package starts no goroutine and has no Start/Close lifecycle.

## Error handling
Compilation returns stable wrapped sentinels for unknown parameters, missing plans, dependency cycles, and invalid operations. Evaluation has no partial-error return: a float result is valid only when every leaf/dependency is valid and finite and the generated clamp succeeds; otherwise that output remains absent. Boolean active operands remain valid when false.

## Performance constraints
Plan size is bounded by the fixed generated parameter catalog. Evaluation uses fixed arrays/bitsets, retains no history, and is tested to perform zero allocations after compilation.

## Security boundaries
Only generated parameter definitions and catalog-owned dependency metadata can become instructions. Strict compile-time type and operation validation prevents malformed metadata from reaching evaluation.

## Required tests
Executable package and race tests cover empty/selective/deduplicated compilation, stable dependency topology, caller-slice ownership, missing/cyclic/invalid plans, all five operation formulas, primitive PupilDilation averaging, strict validity and non-finite containment, generated clamping, independent active flags, hidden dependency suppression, typed snapshot ownership/access, zero-allocation evaluation, and concurrent use of one plan. External integration proves merged tracking can flow through processing and a snapshot can satisfy `osc.ValueSource` without reversing the production dependency direction.

## Known gaps
M5 avatar planning/binding is implemented by `internal/avatar`. M6 Application installation and evaluator-to-OSC composition remain unimplemented, and M7 persistence/UI remains incomplete. OSC networking/lifecycle stays outside this package; numeric Lip payload and Expression-to-Lip mapping remain deferred.

## Completion definition
All generated parameters compile through deterministic typed dependency plans; requested valid outputs evaluate with the documented formulas and clamps; invalid data cannot leak as valid output; hidden dependencies stay hidden; and immutable plans plus owned snapshots remain race-free and bounded.
