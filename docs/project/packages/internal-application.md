---
id: internal-application
kind: go-package
path: internal/application
milestone: M6
depends_on: [internal-plugins, internal-tracking, internal-processing, internal-osc]
checks:
  - id: package-builds
    description: Application package builds
    type: command
    command: go-test-build
    args: [./internal/application]
    weight: 2
    required: true
  - id: tracking-wired
    description: Application runs the tracking pipeline
    type: symbol
    path: internal/application/app.go
    pattern: 'runTracking|startTrackingPipeline'
    weight: 3
    required: true
blockers:
  - check: package-builds
    blocks: [M6]
---
# Package: internal/application

## Purpose
Compose product services into one lifecycle.
## Responsibilities
Construct, start, connect, and stop plugin, tracking, processing, evaluation, and OSC services.
## Non-responsibilities
It does not implement subsystem algorithms.
## Current implementation
Only the OSC service is constructed and started.
## Public/internal interfaces
`Application`, `NewApp`, `Start`, and `Close`.
## Owned data
References to process-wide services and Wails context.
## Dependencies
Depends on plugin, tracking, processing, and OSC packages.
## Concurrency and lifecycle
Starts in dependency order and closes in reverse order.
## Error handling
Startup must unwind already-started services on failure.
## Performance constraints
Orchestration must stay outside the frame hot path.
## Security boundaries
Only application APIs intended for Wails may cross into the frontend.
## Required tests
Lifecycle ordering, partial-start rollback, and end-to-end wiring.
## Known gaps
Tracking and plugin services are not instantiated or connected.
## Completion definition
The full data and control paths run under one cancellable lifecycle.
