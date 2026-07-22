---
id: internal-parameters
kind: go-package
path: internal/parameters
milestone: M1
checks:
  - id: package-tests
    description: Parameter model tests pass
    type: command
    command: go-test
    args: [./internal/parameters]
    weight: 3
    required: true
  - id: generated-model
    description: Generated parameter definitions exist
    type: file
    path: internal/parameters/definitions_gen.go
    weight: 2
    required: true
---
# Package: internal/parameters

## Purpose
Provide stable identities and semantics for VRCFT output parameters.

## Responsibilities
Define ParameterID, value types, ranges, encodings, lookup, address helpers, and the generated parameter catalog.

## Non-responsibilities
Parsing source YAML, numeric parameter evaluation, and executable dependency closure belong elsewhere.

## Current implementation
Generated definitions and typed lookup helpers are implemented.

## Public/internal interfaces
Definitions, lookup, address resolution, binary resolution, and clamp helpers.

## Owned data
The generated immutable parameter catalog.

## Dependencies
Generated from the authoritative parameter spec. `internal/parameterdeps` imports this catalog to compute executable dependency closure; this package does not depend on it, avoiding a reverse specification cycle.

## Concurrency and lifecycle
Data is immutable and safe for concurrent reads.

## Error handling
Unknown IDs and names return explicit false or error results.

## Performance constraints
ID-indexed access is constant-time and allocation-free.

## Security boundaries
Generated addresses are normalized and bounded by the source schema.

## Required tests
Definition counts, ID stability, lookup, ranges, and generation parity.

## Known gaps
No package-runtime gap is implied by catalog generation; executable dependency closure is owned by `internal/parameterdeps`.

## Completion definition
All generated definitions match the accepted spec deterministically.
