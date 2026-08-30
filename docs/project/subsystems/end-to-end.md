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
  - id: m7-backend-prerequisite-integration
    description: Persisted settings, root APIs, events, and lifecycle integrate without frontend code
    type: symbol
    path: m7_backend_integration_test.go
    pattern: '(?m)^func TestM7BackendPrerequisiteInterfacesEndToEnd\('
    weight: 2
    required: true
---
# Subsystem: end-to-end

## Purpose
Define the product-level path from vendor tracking data to the active VRChat avatar.
## Responsibilities
Verify avatar configuration, selective IPC, ingest, merge, processing, evaluation, binding, OSC, and backend lifecycle together; separately evidence the non-frontend M7 path from persisted settings through root module APIs/events and deterministic shutdown.
## Non-responsibilities
Component algorithms remain owned by their packages.
## Current implementation
M6 backend composition is implemented: Application constructs the component path, atomically installs each avatar-plan transition, routes selected current-generation plugin data through merge, processing, and evaluation, and publishes only through current-avatar OSC bindings. `TestApplicationAvatarAwareOSCEndToEnd` provides that deterministic cross-package fixture. The distinct `TestM7BackendPrerequisiteInterfacesEndToEnd` fixture resolves injected Windows paths, creates revision-1 settings, constructs/starts one backend through root, observes independent Runtime/Plugins/Settings events, applies plugin enabled/config changes immediately, durably saves restart-required construction settings without rebuilding, and joins one backend Close with no post-close events.
## Public/internal interfaces
The observable backend product behavior, bounded root diagnostics/operations contracts, versioned module events, and integration fixtures. Frontend pages are not part of this subsystem evidence.
## Owned data
End-to-end test scenarios and expected cross-package behavior.
## Dependencies
The M6 path composes application, plugins, tracking, processing, evaluator, OSC, and avatar-planning components while preserving their ownership and dependency direction. The M7 prerequisite fixture additionally composes root with `internal/userconfig` and Application operation seams without granting root ownership of backend algorithms.
## Concurrency and lifecycle
The coordinator serializes plan activation and frame evaluation; bounded latest-value streams prevent control or diagnostics backlog. M6 integration covers startup, avatar generation switching, stale-generation fencing, and reverse-order shutdown. The M7 fixture covers module-local latest events, immediate/durable operation ordering, at-most-once backend construction/Close, consumer cancellation, and suppression after close.
## Error handling
Failures retain package/check provenance and never silently send stale avatar data. Root-facing failures are bounded sanitized Problems; invalid settings remain diagnostic/repairable, failed startup retains constructed ownership for Close, and no settings or plugin document becomes authoritative before its owning store succeeds.
## Performance constraints
Tests assert bounded queues and selective IPC; benchmarks remain component-owned.
## Security boundaries
Fixtures use isolated temporary files, injected platform values, in-memory backends/events, and loopback transports without real user data. Wails exposure is restricted to owned DTOs; credentials, process handles, internal configs, tracking frames, raw errors/logs, and plugin paths remain below the tested boundary.
## Required tests
Component-owned plugin handshake and process evidence, deterministic in-memory avatar-aware OSC output evidence, avatar switching, stale-generation rejection, and failure recovery remain required for M6. `TestM7BackendPrerequisiteInterfacesEndToEnd` additionally provides non-frontend M7 evidence for first-run settings, three independent module events, plugin mutations, restart-required settings persistence, and idempotent shutdown.
## Known gaps
M6 backend composition and the non-frontend M7 root/settings/operations fixture are implemented. Frontend diagnostics/configuration UX, generated bindings, dependency installation, and frontend type-check/production-build/status-view evidence remain incomplete, so this does not claim M7 completion. Numeric Lip payload and Expression-to-Lip mapping are not claimed.
## Completion definition
A deterministic fixture proves selected plugin data reaches only current-avatar OSC bindings.
