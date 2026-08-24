---
id: internal-processing
kind: go-package
path: internal/processing
milestone: M4
depends_on: [internal-tracking, pkg-trackingmodel]
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
    path: internal/processing/pipeline.go
    pattern: 'type Pipeline struct'
    weight: 4
    required: true
---
# Package: internal/processing

## Purpose
Transform Host-merged tracking snapshots into bounded, stable canonical Eye and Expression channels plus independent Eye/Expression/Lip activity metadata.
## Responsibilities
Validate an entire merged input before mutation; compile and own configuration; apply calibration, tuning, filtering, per-channel freshness/dropout, and post-dropout mutual exclusion in deterministic order; preserve source metadata; derive group activity from independent Host freshness; and reset history at defined generation/source boundaries.
## Non-responsibilities
Source selection belongs to `internal/tracking`, and final VRCFT parameter evaluation belongs to `internal/evaluator`. Numeric Lip payload/Expression-to-Lip mapping, avatar planning/binding, OSC networking, persistence/UI, and Application wiring remain outside this package.
## Current implementation
`NewPipeline` validates and deep-copies defaults, overrides, and mutual-exclusion groups into compiled configuration. Caller-serialized `Pipeline.ProcessAt` validates and classifies a `tracking.MergedFrame`, ingests only fresh Eye/Expression groups, runs calibration then tuning then the configured none/EMA/One-Euro filter, applies stale/hold/linear-decay/final-zero dropout, projects mutual exclusion over the resulting candidates, and returns a fixed-layout `CanonicalFrame`. Lip contributes source and activity metadata only and has no numeric history.
## Public/internal interfaces
`Config`, `ChannelConfig`, transform/filter/dropout configuration types, stable `ChannelID` helpers, `DefaultConfig`, `NewPipeline`, caller-serialized `Pipeline.ProcessAt`, and value-type `CanonicalFrame`.
## Owned data
A compiled configuration that does not alias caller maps or nested slices; one fixed state entry per Eye scalar and expression channel; the last accepted merged value; and bounded filter, last-fresh, and dropout history per channel. No frame list or unbounded history is retained.
## Dependencies
Consumes `internal/tracking.MergedFrame` and fixed Eye, Expression, validity, and capability types from `pkg/trackingmodel`.
## Concurrency and lifecycle
One pipeline instance owns sequential history for one stream and must be serialized by its caller. It starts no goroutine and has no Start/Close lifecycle. `ProcessAt` works on a complete receiver copy and commits it only after success; a repeated identical frame may advance caller time and dropout without re-ingesting filter inputs.
## Error handling
Construction rejects invalid/non-finite configuration and overlapping or malformed mutual-exclusion groups. Processing rejects inconsistent capability/source/freshness/data, non-finite valid inputs, generation/revision conflicts, and time regressions through stable sentinel errors. Failed calls leave all history unchanged. Generation changes reset every channel; selected non-empty Eye or Expression source changes reset only that group's channels, while source loss starts dropout at the Host update boundary.
## Performance constraints
Per-frame work is bounded by the fixed channel catalog. State is fixed-size except for immutable compiled mutual-exclusion groups, and no backlog, frame history, or background queue is created.
## Security boundaries
All caller-supplied configuration and merged metadata are range/shape/finite validated before activation or state mutation. The package owns copies of reference-backed configuration collections.
## Required tests
Executable package tests cover channel mapping, configuration ownership, calibration/tuning/filter formulas, validate-before-mutate behavior, generation and per-group source reset boundaries, independent freshness/activity, repeated-frame dropout, hold/decay/final neutral behavior, transform order, deterministic post-dropout mutual exclusion, and Lip/Expression independence.
## Known gaps
M5 avatar planning/binding is implemented by `internal/avatar`, and M6 Application installation plus evaluator-to-OSC composition are complete outside this package. M7 persistence/UI and release integration remain outside this package. Numeric Lip payload and Expression-to-Lip mapping are not implemented here.
## Completion definition
The caller-serialized deterministic chain validates before mutation, owns bounded history, applies transforms and dropout in the documented order, resets at exact boundaries, projects mutual exclusion without corrupting history, and emits tested canonical value snapshots.
