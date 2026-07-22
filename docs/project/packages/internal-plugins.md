---
id: internal-plugins
kind: go-package
path: internal/plugins
milestone: M2
depends_on: [internal-ipc, pkg-protocol, pkg-trackingmodel]
checks:
  - id: package-builds
    description: Plugin manager package builds
    type: command
    command: go-test-build
    args: [./internal/plugins]
    weight: 2
    required: true
  - id: runtime-loop
    description: Runtime command loop exists
    type: symbol
    path: internal/plugins/runtime.go
    pattern: 'func .*run|func .*loop'
    weight: 3
    required: true
blockers:
  - check: package-builds
    blocks: [M2, M6]
---
# Package: internal/plugins

## Purpose
Manage plugin discovery, configuration, processes, health, logs, and frames.
## Responsibilities
Install/register plugins, launch and supervise processes, handshake IPC, route commands, and publish frames.
## Non-responsibilities
Vendor device code runs in plugin processes; frame merge belongs to tracking.
## Current implementation
Manager interfaces, runtime snapshots, stores, installer, registry, and supervisor models exist.
## Public/internal interfaces
`Manager`, events, runtime snapshots, launcher, and process abstractions.
## Owned data
Plugin registry, preferences, runtime state, restart counters, and health timestamps.
## Dependencies
Depends on IPC, protocol, and tracking model contracts.
## Concurrency and lifecycle
Each runtime has serialized commands and bounded shutdown/restart policy.
## Error handling
Crashes, handshake failures, heartbeat timeouts, and incompatibility are explicit states.
## Performance constraints
Frame events must not create unbounded manager queues.
## Security boundaries
Executable paths, manifests, tokens, permissions, and process environment are validated.
## Required tests
Lifecycle transitions, restart limits, handshake, logs, frames, and shutdown escalation.
## Known gaps
`LogEntry` is unresolved and the runtime command/IPC loop is incomplete.
## Completion definition
Multiple plugins run safely and deliver subscribed frames to host tracking.
