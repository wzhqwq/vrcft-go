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
  - id: config-validation-is-passive
    description: Application configuration validation constructs no runtime dependencies
    type: symbol
    path: internal/application/app_test.go
    pattern: '(?m)^func TestValidateConfigChecksWithoutConstructingApplication\('
    weight: 1
    required: true
  - id: plugin-operations-tested
    description: Running-only plugin operations have lifecycle evidence
    type: symbol
    path: internal/application/app_test.go
    pattern: '(?m)^func TestApplicationPluginMutationsRequireRunningLifecycle\('
    weight: 1
    required: true
  - id: plugin-snapshots-tested
    description: Owned latest-only plugin snapshots have executable evidence
    type: symbol
    path: internal/application/app_test.go
    pattern: '(?m)^func TestApplicationPluginSubscriptionPublishesInitialLatestOwnedSnapshots\('
    weight: 1
    required: true
---
# Package: internal/application

## Purpose
Compose the backend product services into one cancellable, avatar-aware lifecycle.
## Responsibilities
Validate explicit caller-supplied configuration without construction when requested; construct the plugin Manager with the tracking frame sink, tracking Service, processing Pipeline, avatar Planner, and external-catalog OSC runtime; serialize avatar-plan installation and frame evaluation; fail closed on every allocated plan; schedule processing for merged frames and dropout ticks; remove lost plugin sources; start, roll back, and stop components in deterministic order; publish bounded payload-free status snapshots; and expose owned plugin reads, running-only mutations, and latest-only complete plugin snapshots to the root.
## Non-responsibilities
It does not reimplement plugin, tracking, processing, evaluator, avatar, or OSC algorithms; own root Wails construction or frontend bindings; select or persist user configuration; or define numeric Lip payloads or Expression-to-Lip mapping.
## Current implementation
`ValidateConfig` applies the same normalization and lower-level validation as `NewApp` without constructing tracking, plugin, processing, planning, OSC, installation, or coordinator dependencies. `NewApp` accepts explicit component configuration and constructs the backend composition without inventing filesystem paths, persistent defaults, or background work. A single coordinator owns the current immutable avatar plan, caller-serialized processing state, evaluator calls, and recovery state. It installs every non-nil plan—including failed and ready-but-empty plans—by clearing the OSC runtime, advancing tracking generation, safely controlling plugin subscriptions, and then installing only a ready non-empty catalog. If failed activation cannot be positively compensated with inactivity, the catalog remains absent and frame recovery stays blocked until the next avatar-plan transition. Current-generation merged frames flow through processing and selective evaluation to the generation-fenced OSC runtime; repeated ticks advance dropout even without a new frame. Status and plugin lists are immutable capacity-one latest-value subscriptions; plugin log events do not trigger list publication.
## Public/internal interfaces
`Config`, `Application`, `ValidateConfig`, `NewApp`, `Start`, `Close`, `Status`, `SubscribeStatus`, `Plugins`, `PluginConfig`, `SetPluginEnabled`, `UpdatePluginConfig`, and `SubscribePlugins`; the coordinator, installation, status, and cloning helpers remain package-internal.
## Owned data
Process-lifetime component references, cancellation/join state, the coordinator's current plan and processing state, and the latest immutable bounded status. Plugin reads and publications return fresh list/configuration copies. Application retains no frame history, OSC catalog ownership, persisted user settings, or Wails context.
## Dependencies
Consumes `internal/plugins`, `internal/tracking`, `internal/processing`, `internal/evaluator`, `internal/osc`, and `internal/avatar` as their composition owner. Dependencies remain one-way: component packages do not depend on Application.
## Concurrency and lifecycle
The coordinator serializes control and frame-path state. Monotonic plugin session identities make a coalesced running-to-running restart remove the prior tracking source. Avatar, merged-frame, status, and plugin snapshot subscriptions are bounded latest-value streams. Plugin mutations reject nil/canceled contexts and every lifecycle except running before delegating outside the lifecycle lock. Start begins plugins, then the ready coordinator, then OSC; a startup failure cancels and joins already-started work. Close linearizes with an admitted Start, stops OSC, cancels and joins the coordinator after clearing its runtime, then closes plugins; repeated close calls share the stable result. Lifecycle transitions snapshot OSC status after component calls without holding the Application lock.
## Error handling
Construction rejects invalid configuration without starting work. Plan-control failures deactivate affected plugins and are recorded without restoring an old plan; unconfirmed compensating inactivity blocks output for that plan generation. Processing or generation failures clear OSC output before status reports degradation. Startup and shutdown join independent cleanup errors while preserving operation context.
## Performance constraints
The coordinator keeps only latest values and performs no unbounded queueing or frame history; component hot-path algorithms remain component-owned.
## Security boundaries
Configuration and component errors are sanitized at their owning boundaries. Status and plugin snapshots exclude tracking payloads, plugin-private configuration, credentials, process handles, and transport buffers. `PluginConfig` is the one explicit owned internal read of the user's non-credential plugin JSON; the root applies the Wails allowlist, size limit, cloning, and sanitized Problem mapping.
## Required tests
Composition tests cover passive validation, explicit construction, plan installation, plugin controls, generation fencing, lifecycle rollback, bounded status, frame scheduling, owned plugin reads, running-only mutations, and latest-only subscription cancellation. `TestValidateConfigChecksWithoutConstructingApplication`, `TestApplicationPluginMutationsRequireRunningLifecycle`, `TestApplicationPluginSubscriptionPublishesInitialLatestOwnedSnapshots`, and `TestApplicationAvatarAwareOSCEndToEnd` are explicit evidence; the package race suite exercises lifecycle, coordinator, and subscription ownership.
## Known gaps
M6 backend composition and the non-frontend M7 operational methods are complete. Persisted settings and Wails adaptation are owned outside this package. Frontend diagnostics/configuration UX, generated bindings, and any future numeric Lip or Expression-to-Lip product contract remain incomplete.
## Completion definition
One cancellable backend lifecycle deterministically carries selected current-generation plugin data through tracking, processing, evaluation, and current-avatar OSC bindings while failed transitions remain fail-closed and all queues/status remain bounded.
