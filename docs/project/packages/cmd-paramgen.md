---
id: cmd-paramgen
kind: go-package
path: cmd/paramgen
milestone: M1
depends_on: [internal-paramgen]
checks:
  - id: package-tests
    description: Parameter generator command builds
    type: command
    command: go-test
    args: [./cmd/paramgen]
    weight: 2
    required: true
  - id: command-entry
    description: Generator has a main entry point
    type: symbol
    path: cmd/paramgen/main.go
    pattern: 'func main\('
    weight: 1
    required: true
---
# Package: cmd/paramgen

## Purpose
Expose deterministic parameter generation as a command.
## Responsibilities
Parse command arguments, invoke the generator, and report failures.
## Non-responsibilities
Parameter semantics and rendering rules belong to parser/generator packages.
## Current implementation
The command invokes the existing VRCFT parameter generator.
## Public/internal interfaces
Command-line flags and process exit status.
## Owned data
No persistent state.
## Dependencies
Depends on `internal/paramgen`.
## Concurrency and lifecycle
Runs synchronously and exits after generation.
## Error handling
Invalid input and filesystem errors produce a nonzero exit.
## Performance constraints
Generation must remain deterministic; throughput is not a hot path.
## Security boundaries
Output paths must remain explicitly selected by the command.
## Required tests
Package build and deterministic generator tests.
## Known gaps
Generation cleanliness is verified at subsystem level.
## Completion definition
The command reproducibly generates the committed parameter model.
