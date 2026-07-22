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
Define ParameterID, value types, ranges, encodings, lookup, and address helpers.
## Non-responsibilities
Parsing source YAML and runtime evaluation belong elsewhere.
## Current implementation
Generated definitions and typed lookup helpers are implemented.
## Public/internal interfaces
Definitions, lookup, address resolution, binary resolution, and clamp helpers.
## Owned data
The generated immutable parameter catalog.
## Dependencies
Generated from the authoritative parameter spec.
## Concurrency and lifecycle
Data is immutable and safe for concurrent reads.
## Error handling
Unknown IDs and names return explicit false/error results.
## Performance constraints
ID-indexed access is constant-time and allocation-free.
## Security boundaries
Generated addresses are normalized and bounded by the source schema.
## Required tests
Definition counts, ID stability, lookup, ranges, and generation parity.
## Known gaps
Generation parity is enforced at subsystem level rather than package runtime.
## Completion definition
All generated definitions match the accepted spec deterministically.
