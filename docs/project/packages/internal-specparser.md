---
id: internal-specparser
kind: go-package
path: internal/specparser
milestone: M1
checks:
  - id: package-tests
    description: Parameter spec parser tests pass
    type: command
    command: go-test
    args: [./internal/specparser]
    weight: 3
    required: true
  - id: strict-validation
    description: Document validation is implemented
    type: symbol
    path: internal/specparser/spec.go
    pattern: 'func \(d \*Document\) Validate'
    weight: 1
    required: true
---
# Package: internal/specparser

## Purpose
Parse and validate the authoritative VRCFT parameter YAML schema.
## Responsibilities
Decode strict fields, classify parameters, parse semantics, and validate counts/ranges.
## Non-responsibilities
Code generation and runtime lookup belong to downstream packages.
## Current implementation
Document types, validation, classification, semantics parsing, and tests exist.
## Public/internal interfaces
Document parsing and validation APIs used by the generator.
## Owned data
In-memory source document representation.
## Dependencies
Uses YAML v3.
## Concurrency and lifecycle
Parsed documents are call-local and may be read immutably.
## Error handling
Schema errors include parameter and field context.
## Performance constraints
Parsing is a build-time path; clarity and strictness take priority.
## Security boundaries
Unknown fields, invalid ranges, and malformed semantics are rejected.
## Required tests
Valid document, unknown fields, count mismatch, ranges, and semantic forms.
## Known gaps
No product-runtime responsibilities are expected.
## Completion definition
Every accepted document is safe and deterministic for generation.
