---
id: pkg-pluginruntime
kind: go-package
path: pkg/pluginruntime
milestone: M2
depends_on: [pkg-pluginapi, pkg-protocol, pkg-trackingmodel]
checks:
  - id: package-builds
    description: Plugin runtime package builds
    type: command
    command: go-test-build
    args: [./pkg/pluginruntime]
    weight: 1
    required: true
  - id: main-implemented
    description: Plugin runtime Main is implemented
    type: not_placeholder
    path: pkg/pluginruntime/main.go
    patterns: ['(?s)func Main\([^)]*\)\s*\{\s*\}']
    weight: 4
    required: true
  - id: frame-delivery
    description: Latest frame slot has a consumer loop
    type: symbol
    path: pkg/pluginruntime/frame.go
    pattern: 'func \(s \*LatestFrameSlot\) (Load|Run|Next)'
    weight: 2
    required: true
---
# Package: pkg/pluginruntime

## Purpose
Run a vendor driver inside a managed plugin process.
## Responsibilities
Handshake, initialize the driver, deliver commands/subscriptions, publish latest frames, heartbeat, and shut down.
## Non-responsibilities
Device mapping belongs to the driver; process supervision belongs to the host.
## Current implementation
Runtime state and latest-frame storage exist; `Main` and frame delivery are incomplete.
## Public/internal interfaces
`Main(driver)` and runtime-managed implementation of `pluginapi.Environment`.
## Owned data
Plugin connection, initial config, command channel, frame slot, and cancellation.
## Dependencies
Depends on plugin API, protocol, and tracking model.
## Concurrency and lifecycle
Driver, command, heartbeat, and frame writer workers share one cancellation scope.
## Error handling
Protocol failure cancels the driver and returns a meaningful process exit.
## Performance constraints
Only the latest pending frame is serialized; stale frames are overwritten.
## Security boundaries
The runtime authenticates the host endpoint and validates protocol/config sizes.
## Required tests
Handshake, command delivery, frame coalescing, subscription, heartbeat, and shutdown.
## Known gaps
Main is empty and no frame consumer sends IPC messages.
## Completion definition
A sample driver operates end-to-end through the versioned host protocol.
