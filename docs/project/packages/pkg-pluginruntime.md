---
id: pkg-pluginruntime
kind: go-package
path: pkg/pluginruntime
milestone: M2
depends_on: [internal-ipc, pkg-pluginapi, pkg-protocol, pkg-trackingmodel]
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
Perform handshake, initialize the driver, deliver ordered controls, selectively deliver latest frames, serialize writes through one writer, preserve metadata-only Lip capability negotiation/publication, apply heartbeat, status, and log policies, and perform bounded shutdown.

## Non-responsibilities
Device mapping belongs to the driver; process supervision and credential
generation belong to the Host; endpoint construction belongs to internal IPC.
The runtime does not define a numeric Lip payload or map Expression values to Lip.

## Current implementation
Immutable initialization and mutable current state, handshake, ordered
controls, validated latest-frame selective delivery, single-writer operation,
heartbeat/status/log handling, deadline-bound terminal arbitration, and
`Main(driver) error` are implemented. Lip-only descriptors, subscriptions, and
frames using capability value 4 pass through the existing metadata contract
without gaining numeric payload. `Main` connects through `internal/ipc`
using `VRCFT_PIPE_NAME` and `VRCFT_SESSION_TOKEN`.

## Public/internal interfaces
`Main(driver) error` and the runtime-managed implementation of `pluginapi.Host`.

## Owned data
Plugin connection, immutable initial and mutable current host snapshots, ordered control channel, latest-frame slot, writer state, and cancellation.

## Dependencies
Depends on internal IPC, plugin API, protocol, and tracking model.

## Concurrency and lifecycle
Driver, control-reader, heartbeat/frame writer workers share one cancellation scope. One terminal collector retains all worker failures, enforces the shutdown deadline, and closes a cancellation-insensitive connection exactly once when needed.

## Error handling
Protocol failure and malformed publication cancel or reject at their documented boundaries; stable known capability validation includes metadata-only Lip. Substantive worker failures, shutdown acknowledgement failures, timeout, and close errors are preserved within the bounded lifecycle.

## Performance constraints
Only the latest pending subscribed frame is serialized; stale frames are overwritten before the single writer sends them.

## Security boundaries
The runtime requires nonblank pipe-name and session-token environment values,
does not include the token in connection errors, and validates the token and
protocol/configuration sizes during the handshake before activating the driver.

## Required tests
Executable package and race tests cover handshake, environment-based real
named-pipe connection, ordered controls, frame coalescing and subscription,
heartbeat, status/log policy, and bounded shutdown; the named integration test
file remains additional integration evidence.

## Known gaps
Host-side Application composition remains deferred to M6. Numeric Lip payload,
Expression-to-Lip mapping, avatar planning, persistence/UI, and OSC networking
remain outside this runtime.

## Completion definition
A driver started with a valid logical pipe name and session token connects
through the versioned Host protocol, preserves Eye/Expression/Lip capability
compatibility, and runs with bounded lifecycle behavior.
