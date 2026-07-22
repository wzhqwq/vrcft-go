---
id: end-to-end
kind: end-to-end
path: docs/project
milestone: M6
depends_on: [internal-application, internal-plugins, internal-tracking, internal-processing, internal-osc]
checks:
  - id: pipeline-components
    description: Required product pipeline components are complete
    type: aggregate
    members:
      - internal-application:tracking-wired
      - internal-plugins:runtime-loop
      - internal-tracking:service-implemented
      - internal-processing:pipeline-implemented
      - internal-osc:package-tests
    weight: 5
    required: true
  - id: integration-test
    description: Avatar-aware end-to-end integration test exists
    type: symbol
    path: internal/application/app_test.go
    pattern: 'Test.*Avatar.*OSC|Test.*EndToEnd'
    weight: 3
    required: true
---
# Subsystem: end-to-end

## Purpose
Define the product-level path from vendor tracking data to the active VRChat avatar.
## Responsibilities
Verify avatar configuration, selective IPC, ingest, merge, processing, evaluation, binding, OSC, and lifecycle together.
## Non-responsibilities
Component algorithms remain owned by their packages.
## Current implementation
OSC transport is advanced, while IPC, merge, processing, evaluation, avatar loading, and wiring remain incomplete.
## Public/internal interfaces
The observable product behavior, diagnostics, and integration fixtures.
## Owned data
End-to-end test scenarios and expected cross-package behavior.
## Dependencies
Depends on application, plugins, tracking, processing, and OSC components.
## Concurrency and lifecycle
Tests cover startup, avatar generation changes, frame races, and reverse-order shutdown.
## Error handling
Failures retain package/check provenance and never silently send stale avatar data.
## Performance constraints
Tests assert bounded queues and selective IPC; benchmarks remain component-owned.
## Security boundaries
Fixtures use isolated files and loopback transports without real user data.
## Required tests
Plugin handshake through OSC output, avatar switching, stale generation rejection, and failure recovery.
## Known gaps
No end-to-end application integration test or complete upstream data path exists.
## Completion definition
A deterministic fixture proves selected plugin data reaches only current-avatar OSC bindings.
