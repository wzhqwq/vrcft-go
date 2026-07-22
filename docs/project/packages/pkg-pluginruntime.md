---
id: pkg-pluginruntime
kind: go-package
path: pkg/pluginruntime
milestone: M2
depends_on: [pkg-pluginapi, pkg-protocol, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Plugin runtime package tests pass
    type: command
    command: go-test
    args: [./pkg/pluginruntime]
    weight: 3
    required: true
  - id: package-race-tests
    description: Plugin runtime package race tests pass
    type: command
    command: go-test-race
    args: [./pkg/pluginruntime]
    weight: 3
    required: true
  - id: integration-tests
    description: Plugin runtime integration tests exist
    type: file
    path: pkg/pluginruntime/integration_test.go
    weight: 2
    required: true
---
# Package: pkg/pluginruntime

## Purpose
Run a vendor driver inside a managed plugin process.

## Responsibilities
Perform handshake, initialize the driver, deliver ordered controls, selectively deliver latest frames, serialize writes through one writer, apply heartbeat, status, and log policies, and perform bounded shutdown.

## Non-responsibilities
Device mapping belongs to the driver; process supervision belongs to the host; concrete endpoint construction belongs to internal IPC.

## Current implementation
Runtime state, handshake, ordered controls, latest-frame selective delivery, single-writer operation, heartbeat/status/log handling, bounded shutdown, and `Main(driver) error` are implemented.

## Public/internal interfaces
`Main(driver) error` and the runtime-managed implementation of `pluginapi.Host`.

## Owned data
Plugin connection, initial configuration, ordered control channel, latest-frame slot, writer state, and cancellation.

## Dependencies
Depends on plugin API, protocol, and tracking model.

## Concurrency and lifecycle
Driver, command, heartbeat, frame, and writer workers share one cancellation scope and terminate within bounded shutdown policy.

## Error handling
Protocol failure cancels the driver and returns a meaningful process exit; shutdown acknowledgement and close errors are handled within the bounded lifecycle.

## Performance constraints
Only the latest pending subscribed frame is serialized; stale frames are overwritten before the single writer sends them.

## Security boundaries
The runtime validates the handshake token and protocol/configuration sizes before activating the driver.

## Required tests
Executable package and race tests cover handshake, ordered controls, frame coalescing and subscription, heartbeat, status/log policy, and bounded shutdown; the named integration test file is integration evidence only.

## Known gaps
The only known gap is a concrete connection factory: it remains intentionally unavailable until `internal/ipc` integrates it. `Main(driver) error` and the runtime are not empty.

## Completion definition
A driver runs through the versioned host protocol with bounded lifecycle behavior once internal IPC supplies the concrete connection factory.
