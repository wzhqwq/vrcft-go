---
id: parameter-spec
kind: parameter-spec
path: spec
milestone: M1
depends_on: [internal-specparser, internal-paramgen, internal-parameters]
checks:
  - id: source-exists
    description: Authoritative VRCFT parameter YAML exists
    type: file
    path: spec/vrcft_osc_parameters.yaml
    weight: 2
    required: true
  - id: parser-generator-tests
    description: Parameter parser and generator tests pass
    type: command
    command: go-test
    args: [./internal/specparser, ./internal/paramgen, ./internal/parameters]
    weight: 4
    required: true
---
# Subsystem: parameter-spec

## Purpose
Own the authoritative VRCFT OSC parameter definitions and generation workflow.
## Responsibilities
Define names, groups, types, ranges, encodings, units, semantics, counts, and stable ordering.
## Non-responsibilities
Runtime evaluation and avatar-specific bindings are not source-spec concerns.
## Current implementation
The YAML document, strict parser, generator, and generated model exist.
## Public/internal interfaces
The YAML schema and committed generated Go definitions.
## Owned data
`spec/vrcft_osc_parameters.yaml` is authoritative; generated files are derived.
## Dependencies
Depends on parser, generator, and parameter model packages.
## Concurrency and lifecycle
Generation runs during development/build workflows, not in the frame path.
## Error handling
Invalid schema, counts, ranges, and encodings fail before generation.
## Performance constraints
Stable output and reviewable diffs take priority over speed.
## Security boundaries
Generated identifiers and string literals are escaped safely.
## Required tests
Schema validation, generation determinism, ID stability, and committed parity.
## Known gaps
The status system does not yet run generation in an isolated full-module fixture.
## Completion definition
Source YAML and generated parameter model cannot drift undetected.
