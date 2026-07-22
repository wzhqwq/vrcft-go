---
id: internal-processing
kind: go-package
path: internal/processing
milestone: M4
depends_on: [pkg-trackingmodel]
checks:
  - id: package-builds
    description: Processing package builds
    type: command
    command: go-test-build
    args: [./internal/processing]
    weight: 1
    required: true
  - id: pipeline-implemented
    description: Executable processing pipeline exists
    type: symbol
    path: internal/processing/canonical.go
    pattern: 'type Pipeline|func Process'
    weight: 4
    required: true
---
# Package: internal/processing

## Purpose
Transform merged tracking samples into stable canonical channels.
## Responsibilities
Apply validation, calibration, tuning, filters, mutual exclusion, and dropout policy.
## Non-responsibilities
Source selection and final VRCFT parameter evaluation belong to adjacent stages.
## Current implementation
Canonical data and configuration types exist, but no executable pipeline exists.
## Public/internal interfaces
Future immutable pipeline configuration and `Process` operation.
## Owned data
Filter state, calibration, tuning, and dropout history per channel.
## Dependencies
Consumes the shared tracking model and merged host data.
## Concurrency and lifecycle
A pipeline instance owns sequential filter history for one processing stream.
## Error handling
Invalid/non-finite samples are rejected or invalidated without poisoning filter state.
## Performance constraints
Per-frame processing must be bounded and avoid unnecessary allocations.
## Security boundaries
Persisted tuning values are range-validated before activation.
## Required tests
Each transform, reset, time discontinuity, dropout, and composed pipeline order.
## Known gaps
Only policy/configuration structs and canonical frame shape are present.
## Completion definition
The complete deterministic processing chain emits tested canonical frames.
