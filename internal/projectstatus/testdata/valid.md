---
id: internal-osc
kind: go-package
path: internal/osc
milestone: M6
depends_on: [internal-parameters]
checks:
  - id: package-tests
    description: OSC tests pass
    type: command
    command: go-test
    args: [./internal/osc]
    weight: 3
    required: true
blockers:
  - check: package-tests
    blocks: [M6]
---
# Package: internal/osc

## Purpose
OSC integration.

## Responsibilities
Encode and send OSC.

## Non-responsibilities
Tracking evaluation.

## Current implementation
OSC and OSCQuery are implemented.

## Public/internal interfaces
Controller and sender.

## Owned data
Binding catalog.

## Dependencies
Parameter definitions.

## Concurrency and lifecycle
Controller owns workers.

## Error handling
Errors are returned or emitted.

## Performance constraints
Sender stays allocation-free.

## Security boundaries
Network input is validated.

## Required tests
Unit and race tests.

## Known gaps
No evaluator input.

## Completion definition
End-to-end integration passes.
