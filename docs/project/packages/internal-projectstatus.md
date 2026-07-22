---
id: internal-projectstatus
kind: go-package
path: internal/projectstatus
milestone: M0
checks:
  - id: package-tests
    description: Project status engine tests pass
    type: command
    command: go-test
    args: [./internal/projectstatus]
    weight: 4
    required: true
  - id: race-tests
    description: Project status engine is race-free
    type: command
    command: go-test-race
    args: [./internal/projectstatus]
    weight: 2
    required: true
---
# Package: internal/projectstatus

## Purpose
Turn package specifications and executable evidence into project status.
## Responsibilities
Parse specs, discover packages, execute safe checks, calculate progress, and render reports.
## Non-responsibilities
It does not implement or reinterpret product behavior.
## Current implementation
Schema, discovery, checks, progress, fingerprinting, and reports are implemented.
## Public/internal interfaces
The exported parser, catalog, evaluator, status, and renderer functions used by the CLI.
## Owned data
In-memory catalog, check results, and report models.
## Dependencies
Uses YAML v3 and standard-library process/filesystem APIs.
## Concurrency and lifecycle
Commands use contexts and bounded lifetimes; a run owns its caches.
## Error handling
Invalid specs and tool errors are returned with field or check context.
## Performance constraints
Identical commands run once and output capture is bounded.
## Security boundaries
Commands are registered and paths cannot escape the repository.
## Required tests
Parser, graph, runner, progress, renderer, fingerprint, and real-catalog coverage.
## Known gaps
Windows child-process-tree termination is limited to standard `CommandContext` behavior.
## Completion definition
M0 checks pass and the real catalog covers every project package.
