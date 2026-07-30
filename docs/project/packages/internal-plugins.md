---
id: internal-plugins
kind: go-package
path: internal/plugins
milestone: M2
depends_on: [internal-ipc, pkg-protocol, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Plugin manager package tests pass
    type: command
    command: go-test
    args: [./internal/plugins]
    weight: 3
    required: true
  - id: package-race-tests
    description: Plugin manager package race tests pass
    type: command
    command: go-test-race
    args: [./internal/plugins]
    weight: 3
    required: true
  - id: manifest-tests
    description: Manifest validation rejection test exists
    type: symbol
    path: internal/plugins/manifest_test.go
    pattern: '(?m)^func TestManifestValidateRejectsInvalidFields\('
    weight: 1
    required: true
  - id: catalog-tests
    description: Builtin and development catalog test exists
    type: symbol
    path: internal/plugins/catalog_test.go
    pattern: '(?m)^func TestDirectoryCatalogSortsAndLabelsSources\('
    weight: 1
    required: true
  - id: handshake-tests
    description: Host Hello Initialize Ready handshake test exists
    type: symbol
    path: internal/plugins/handshake_test.go
    pattern: '(?m)^func TestHostHandshakeHelloInitializeReady\('
    weight: 2
    required: true
  - id: direct-frame-sink-tests
    description: Direct plugin ID and generation FrameSink test exists
    type: symbol
    path: internal/plugins/session_test.go
    pattern: '(?m)^func TestPluginSessionRoutesRuntimeMessagesAndRejectsWrongDirection\('
    weight: 2
    required: true
  - id: no-frame-events
    description: Bounded manager events do not reintroduce EventPluginFrame
    type: not_placeholder
    path: internal/plugins/events.go
    patterns: ['EventPluginFrame']
    weight: 1
    required: true
  - id: supervisor-restart-tests
    description: Finite supervisor restart test exists
    type: symbol
    path: internal/plugins/supervisor_test.go
    pattern: '(?m)^func TestPluginSupervisorFiniteRestartAndStableReset\('
    weight: 2
    required: true
  - id: windows-integration-tests
    description: Windows real-process plugin integration tests exist
    type: file
    path: internal/plugins/integration_test.go
    weight: 2
    required: true
  - id: no-installer-placeholder
    description: Deferred distribution installer API has not been reintroduced
    type: not_placeholder
    path: internal/plugins/api.go
    patterns: ['(?m)^type Installer\b', '(?m)^func NewInstaller\b', '(?m)^func .*Install\b']
    weight: 1
    required: true
blockers:
  - check: package-tests
    blocks: [M2, M6]
---
# Package: internal/plugins

## Purpose
Discover configured builtin and development plugins, retain their user preferences, and supervise each plugin process as an authenticated Host session.

## Responsibilities
Validate manifests and catalog roots; maintain persistent enable/configuration preferences; construct fresh IPC credentials and process environments; perform the Host handshake; serialize session writes and controls; publish direct tracking frames; emit bounded lifecycle, status, and log events; and apply finite restart policy.

## Non-responsibilities
Vendor device implementation runs in plugin processes. Frame ordering, generation rejection, routing, and merging belong to `internal/tracking`. Package installation, updates, signing, marketplace/catalog distribution, and third-party SDK delivery are deliberately deferred.

## Current implementation
Directory discovery scans the required builtin root and optional development roots, labels their source, rejects duplicate IDs, links, unsafe entrypoints, and malformed or oversized manifests, and returns deterministic results. The manager loads preferences, creates independent supervisors, and preserves preferences for temporarily unavailable plugins. Each launch owns a fresh named-pipe endpoint and token, completes the Host Hello/Initialize/Ready handshake, and manages process lifecycle through graceful shutdown and kill fallback.

## Public/internal interfaces
`Manager`, `FrameSink`, `Catalog`, `Store`, `Manifest`, `InstalledPlugin`, runtime snapshots, events, process abstractions, and restart policy are internal Host contracts. `FrameSink.Submit(pluginID, generation, frame)` is the generation-bearing handoff to tracking.

## Owned data
The JSON store persists only `PluginPreference.Enabled` and configuration, with defensive copies and atomic replacement. Supervisor and session state—PID, lifecycle, credentials, heartbeats, active state, subscription, counters, and timestamps—is ephemeral; active and subscription are Host/avatar-session inputs and are never persisted.

## Dependencies
Uses `internal/ipc` for protected one-shot local transport and `pkg/protocol`, `pkg/pluginapi`, and `pkg/trackingmodel` for the versioned session and frame contracts.

## Concurrency and lifecycle
Each plugin has a serialized supervisor and an isolated session, so one failure does not interrupt peers. The session admits bounded pending controls during handshake, routes all outbound protocol messages through one writer, and uses a bounded shutdown sequence. Frames bypass the manager event hub and are delivered directly to the configured `FrameSink` with plugin identity and generation. Subscriber events are bounded: state/status can coalesce and logs report drops rather than blocking producers.

## Error handling
Manifest, catalog, authentication, protocol, descriptor, heartbeat, and process failures are classified into explicit states. Retryable failures use capped backoff and a finite consecutive-failure budget; incompatible failures do not restart, stable sessions reset the budget, and manual restart clears it. Shutdown joins relevant errors while bounding graceful and kill waits.

## Performance constraints
Catalog scanning is deterministic. Control and event queues are bounded, a slow event subscriber cannot block sessions, and frames avoid both event retention and an extra manager queue.

## Security boundaries
Catalog entries and executable paths must remain within validated roots. Manifests and settings use strict, size-bounded decoding; settings are written with secure temporary permissions and atomic replacement. Each launch injects a fresh local endpoint and session token, validates authentication and descriptor/protocol compatibility before readiness, and keeps token and configuration contents out of public error text.

## Required tests
Executable package and race tests cover manifest and builtin/dev catalog validation, Host handshake phases and secret redaction, direct FrameSink delivery with identity and generation, absence of frame events, bounded event behavior, finite supervisor restart, and process/session shutdown. `integration_test.go` is Windows-only real-process evidence for named pipes, handshake, controls, telemetry, cleanup, crashes, and bounded restart.

## Known gaps
No `internal/plugins` runtime implementation gap remains. The distribution ecosystem—installer APIs, package acquisition, updates, signing, and marketplace policy—is deferred until product requirements define it.

## Completion definition
Builtin and development plugins can be discovered and independently supervised as authenticated, bounded Host sessions; persistent preferences survive restart, session state does not, direct generation-bearing frames reach tracking, and crashes stop after the configured finite restart budget.
