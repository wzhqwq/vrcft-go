---
id: end-to-end
kind: end-to-end
path: docs/project
milestone: M6
depends_on: [internal-application, internal-plugins, internal-tracking, internal-processing, internal-evaluator, internal-osc, internal-avatar]
checks:
  - id: pipeline-components
    description: Required product pipeline components are complete
    type: aggregate
    members:
      - internal-application:tracking-wired
      - internal-plugins:runtime-loop
      - internal-tracking:service-implemented
      - internal-processing:pipeline-implemented
      - internal-evaluator:package-tests
      - internal-osc:package-tests
      - internal-avatar:package-tests
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
Plugin IPC/session management, generation-aware merge, processing, evaluator, OSC transport, and M5 avatar planning are implemented as components. Application composition, atomic installation of an avatar plan, evaluator-to-OSC feeding, and a complete integration fixture remain absent.
## Public/internal interfaces
The observable product behavior, diagnostics, and integration fixtures.
## Owned data
End-to-end test scenarios and expected cross-package behavior.
## Dependencies
The planned M6 path depends on application, plugins, tracking, processing, evaluator, OSC, and avatar-planning components. The dependency and aggregate entries record required components only; they do not claim production Application wiring or plan installation.
## Concurrency and lifecycle
Required integration coverage includes startup, avatar generation changes, frame races, and reverse-order shutdown; that fixture is not implemented yet.
## Error handling
Failures retain package/check provenance and never silently send stale avatar data.
## Performance constraints
Tests assert bounded queues and selective IPC; benchmarks remain component-owned.
## Security boundaries
Fixtures use isolated files and loopback transports without real user data.
## Required tests
Plugin handshake through OSC output, avatar switching, stale generation rejection, and failure recovery.
## Known gaps
No end-to-end Application integration test or complete avatar-aware data path exists. M5 avatar planning is implemented, but M6 atomic plan installation/binding and lifecycle wiring remain deferred, along with M7 fallback persistence/UI, frontend/release work, numeric Lip payload, and Expression-to-Lip mapping.
## Completion definition
A deterministic fixture proves selected plugin data reaches only current-avatar OSC bindings.
