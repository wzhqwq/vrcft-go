---
id: internal-paramgen
kind: go-package
path: internal/paramgen
milestone: M1
depends_on: [internal-specparser]
checks:
  - id: package-tests
    description: Generator tests pass
    type: command
    command: go-test
    args: [./internal/paramgen]
    weight: 3
    required: true
  - id: generator-entry
    description: Generator implementation exists
    type: symbol
    path: internal/paramgen/generator.go
    pattern: 'func Generate|func Write'
    weight: 1
    required: true
---
# Package: internal/paramgen

## Purpose
Render validated parameter specs into deterministic Go definitions.
## Responsibilities
Assign stable IDs, emit types/maps, format output, and preserve deterministic order.
## Non-responsibilities
YAML validation and runtime parameter evaluation are outside this package.
## Current implementation
Generator and focused tests exist.
## Public/internal interfaces
Generator entry functions consumed by `cmd/paramgen`.
## Owned data
Temporary generation buffers only.
## Dependencies
Consumes classified specs from `internal/specparser`.
## Concurrency and lifecycle
Each generation call owns its state and is synchronous.
## Error handling
Invalid documents and write failures are returned.
## Performance constraints
Determinism is more important than generation throughput.
## Security boundaries
Generated identifiers and literals are escaped through Go-aware formatting.
## Required tests
Golden output, deterministic ordering, and ID stability.
## Known gaps
Repository cleanliness after generation is measured by the parameter subsystem.
## Completion definition
Repeated generation produces byte-identical accepted output.
