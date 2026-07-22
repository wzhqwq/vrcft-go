---
id: cmd-projectstatus
kind: go-package
path: cmd/projectstatus
milestone: M0
depends_on: [internal-projectstatus]
checks:
  - id: package-tests
    description: Project status CLI tests pass
    type: command
    command: go-test
    args: [./cmd/projectstatus]
    weight: 3
    required: true
  - id: cli-entry
    description: CLI exposes main
    type: symbol
    path: cmd/projectstatus/main.go
    pattern: 'func main\('
    weight: 1
    required: true
---
# Package: cmd/projectstatus

## Purpose
Expose project specification evaluation and report generation.
## Responsibilities
Parse modes, orchestrate evaluation, render output, and write status safely.
## Non-responsibilities
Check semantics and progress calculation belong to `internal/projectstatus`.
## Current implementation
Markdown, JSON, write, and freshness-check modes are implemented.
## Public/internal interfaces
The documented command-line flags and exit codes 0, 1, and 2.
## Owned data
Only the generated `docs/project/status.md` file.
## Dependencies
Depends on `internal/projectstatus`.
## Concurrency and lifecycle
One synchronous evaluation per invocation.
## Error handling
Tool errors are distinct from valid incomplete-project reports.
## Performance constraints
Repeated checks are cached within an invocation.
## Security boundaries
The command never evaluates arbitrary shell text from specs.
## Required tests
All CLI modes, exit codes, stale detection, and writes.
## Known gaps
No continuous daemon mode is required.
## Completion definition
Every documented mode is deterministic and tested.
