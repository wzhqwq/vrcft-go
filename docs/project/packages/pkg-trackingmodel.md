---
id: pkg-trackingmodel
kind: go-package
path: pkg/trackingmodel
milestone: M1
checks:
  - id: package-tests
    description: Tracking model package tests pass
    type: command
    command: go-test
    args: [./pkg/trackingmodel]
    weight: 3
    required: true
  - id: package-race-tests
    description: Tracking model package race tests pass
    type: command
    command: go-test-race
    args: [./pkg/trackingmodel]
    weight: 2
    required: true
  - id: expression-tests
    description: Expression contract tests exist
    type: file
    path: pkg/trackingmodel/expression_test.go
    weight: 2
    required: true
---
# Package: pkg/trackingmodel

## Purpose
Provide the shared canonical primitive tracking data contract across process boundaries.

## Responsibilities
Define the complete 76 primitive expression IDs, fixed-size validity masks, safe expression accessors, eye samples, stable Eye/Expression/Lip capability metadata, timestamps, sequences, received-frame metadata, and shared tracking-frame validation/canonicalization.

## Non-responsibilities
Vendor conversion, routing, filtering, numeric parameter evaluation, numeric Lip payloads, and Expression-to-Lip mapping are outside the model.

## Current implementation
`TrackingFrame`, eye and expression data, fixed masks, safe accessors, received metadata, and malformed-frame rejection/canonicalization are implemented and tested. `CapabilityLip` is the stable metadata-only capability value 4; it adds no numeric field and never implies or changes Expression validity or values.

## Public/internal interfaces
All exported value types used by plugin API and protocol are public contracts.

## Owned data
Schema definitions only; instances are owned by producers and consumers.

## Dependencies
No project package dependencies; `internal/parameterdeps` consumes these primitives to prove dependency coverage.

## Concurrency and lifecycle
Frames are values transferred as immutable snapshots.

## Error handling
Validity masks express absent fields, safe accessors reject out-of-range IDs, and frame validation accepts only the stable Eye, Expression, and Lip capability bits while rejecting unknown or capability-inconsistent validity bits. Canonicalization clears disabled Eye/Expression payloads without synthesizing data for Lip.

## Performance constraints
Fixed masks and bounded layouts support frequent serialization without dynamic primitive growth.

## Security boundaries
Fixed counts and indices prevent unbounded payloads.

## Required tests
Executable package and race tests cover every stable expression ID/name pair, fixed masks, safe accessors, frame validation/canonicalization, layout, limits, and serialization behavior; the named expression test file is integration evidence only.

## Known gaps
No M4 tracking-model implementation gap is known. A numeric Lip schema and any Expression-to-Lip mapping remain deferred product decisions.

## Completion definition
All 76 primitive IDs, fixed masks, safe accessors, and the metadata-only Lip value 4 are stable, bounded, and covered by executable tests plus `internal/parameterdeps` dependency closure.
