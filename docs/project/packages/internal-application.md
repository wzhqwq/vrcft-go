---
id: internal-application
kind: go-package
path: internal/application
milestone: M6
depends_on: [internal-plugins, internal-tracking, internal-processing, internal-evaluator, internal-osc, internal-avatar]
checks:
  - id: package-tests
    description: Application composition tests pass
    type: command
    command: go-test
    args: [./internal/application]
    weight: 3
    required: true
  - id: package-race-tests
    description: Application coordinator is race-free
    type: command
    command: go-test-race
    args: [./internal/application]
    weight: 2
    required: true
  - id: tracking-wired
    description: Application runs the tracking pipeline
    type: symbol
    path: internal/application/coordinator.go
    pattern: '(?m)^func \(c \*coordinator\) run\('
    weight: 3
    required: true
---
# Package: internal/application

## Purpose
Compose the backend product services into one cancellable, avatar-aware lifecycle.
## Responsibilities
Validate explicit caller-supplied configuration; construct the plugin Manager with the tracking frame sink, tracking Service, processing Pipeline, avatar Planner, and external-catalog OSC runtime; serialize avatar-plan installation and frame evaluation; fail closed on every allocated plan; schedule processing for merged frames and dropout ticks; remove lost plugin sources; start, roll back, and stop components in deterministic order; and publish bounded payload-free status snapshots.
## Non-responsibilities
It does not reimplement plugin, tracking, processing, evaluator, avatar, or OSC algorithms; own root Wails construction or frontend bindings; select or persist user configuration; or define numeric Lip payloads or Expression-to-Lip mapping.
## Current implementation
`NewApp` accepts explicit component configuration and constructs the backend composition without inventing filesystem paths or persistent defaults. A single coordinator owns the current immutable avatar plan, caller-serialized processing state, evaluator calls, and recovery state. It installs every non-nil plan—including failed and ready-but-empty plans—by clearing the OSC runtime, advancing tracking generation, safely controlling plugin subscriptions, and then installing only a ready non-empty catalog. Current-generation merged frames flow through processing and selective evaluation to the generation-fenced OSC runtime; repeated ticks advance dropout even without a new frame. Status is immutable and uses capacity-one latest-value subscriptions.
## Public/internal interfaces
`Config`, `Application`, `NewApp`, `Start`, `Close`, `Status`, and `SubscribeStatus`; the coordinator, installation, and status helpers remain package-internal.
## Owned data
Process-lifetime component references, cancellation/join state, the coordinator's current plan and processing state, and the latest immutable bounded status. Application retains no frame history, OSC catalog ownership, or Wails context.
## Dependencies
Consumes `internal/plugins`, `internal/tracking`, `internal/processing`, `internal/evaluator`, `internal/osc`, and `internal/avatar` as their composition owner. Dependencies remain one-way: component packages do not depend on Application.
## Concurrency and lifecycle
The coordinator serializes control and frame-path state. Avatar, merged-frame, and status subscriptions are bounded latest-value streams. Start begins plugins, then the ready coordinator, then OSC; a startup failure cancels and joins already-started work. Close stops OSC, cancels and joins the coordinator after clearing its runtime, then closes plugins; repeated close calls share the stable result.
## Error handling
Construction rejects invalid configuration without starting work. Plan-control failures deactivate affected plugins and are recorded without restoring an old plan. Processing or generation failures clear OSC output before status reports degradation. Startup and shutdown join independent cleanup errors while preserving operation context.
## Performance constraints
The coordinator keeps only latest values and performs no unbounded queueing or frame history; component hot-path algorithms remain component-owned.
## Security boundaries
Configuration and component errors are sanitized at their owning boundaries. Status excludes tracking payloads, plugin configuration, credentials, and transport buffers; M7 will decide any Wails/frontend exposure.
## Required tests
Composition tests cover explicit construction, plan installation, plugin controls, generation fencing, lifecycle rollback, bounded status, frame scheduling, and `TestApplicationAvatarAwareOSCEndToEnd`; the package race suite exercises coordinator ownership and subscriptions.
## Known gaps
M6 backend composition is complete. M7 still owns root Wails construction and lifecycle hooks, persisted user configuration and path selection, frontend diagnostics/configuration UX, release integration, and any future numeric Lip or Expression-to-Lip product contract.
## Completion definition
One cancellable backend lifecycle deterministically carries selected current-generation plugin data through tracking, processing, evaluation, and current-avatar OSC bindings while failed transitions remain fail-closed and all queues/status remain bounded.
