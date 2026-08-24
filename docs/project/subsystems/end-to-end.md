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
      - internal-plugins:package-tests
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
    path: internal/application/integration_test.go
    pattern: '(?m)^func TestApplicationAvatarAwareOSCEndToEnd\('
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
M6 backend composition is implemented: Application constructs the component path, atomically installs each avatar-plan transition, routes selected current-generation plugin data through merge, processing, and evaluation, and publishes only through current-avatar OSC bindings. `TestApplicationAvatarAwareOSCEndToEnd` provides the deterministic cross-package fixture.
## Public/internal interfaces
The observable product behavior, diagnostics, and integration fixtures.
## Owned data
End-to-end test scenarios and expected cross-package behavior.
## Dependencies
The M6 path composes application, plugins, tracking, processing, evaluator, OSC, and avatar-planning components while preserving their existing ownership and dependency direction.
## Concurrency and lifecycle
The coordinator serializes plan activation and frame evaluation; bounded latest-value streams prevent control or diagnostics backlog. Integration coverage includes startup, avatar generation switching, stale-generation fencing, and reverse-order shutdown.
## Error handling
Failures retain package/check provenance and never silently send stale avatar data.
## Performance constraints
Tests assert bounded queues and selective IPC; benchmarks remain component-owned.
## Security boundaries
Fixtures use isolated files and loopback transports without real user data.
## Required tests
Component-owned plugin handshake and process evidence, combined with deterministic in-memory avatar-aware OSC output evidence, plus avatar switching, stale generation rejection, and failure recovery.
## Known gaps
M6 backend composition is complete. Real root Wails construction and lifecycle integration, persisted configuration/path selection, frontend diagnostics and configuration UX, and release work remain M7 concerns. Numeric Lip payload and Expression-to-Lip mapping are not claimed by M6.
## Completion definition
A deterministic fixture proves selected plugin data reaches only current-avatar OSC bindings.
