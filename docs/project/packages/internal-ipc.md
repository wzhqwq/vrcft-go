---
id: internal-ipc
kind: go-package
path: internal/ipc
milestone: M2
depends_on: [pkg-protocol]
checks:
  - id: package-builds
    description: IPC package tests pass
    type: command
    command: go-test
    args: [./internal/ipc]
    weight: 3
    required: true
  - id: package-race-tests
    description: IPC package race tests pass
    type: command
    command: go-test-race
    args: [./internal/ipc]
    weight: 3
    required: true
  - id: client-implemented
    description: IPC client is not an empty package
    type: not_placeholder
    path: internal/ipc/client.go
    patterns: ['(?s)^package ipc\s*$']
    weight: 3
    required: true
  - id: server-implemented
    description: IPC server is not an empty package
    type: not_placeholder
    path: internal/ipc/server.go
    patterns: ['(?s)^package ipc\s*$']
    weight: 3
    required: true
  - id: windows-transport
    description: Windows named pipe adapter exists
    type: file
    path: internal/ipc/platform_windows.go
    weight: 2
    required: true
  - id: platform-tests
    description: Platform adapters have tests
    type: file
    path: internal/ipc/platform_windows_test.go
    weight: 2
    required: true
---
# Package: internal/ipc

## Purpose
Provide the protected framed transport used by the authenticated protocol
between Host and plugin processes.

## Responsibilities
Create and connect one-shot local Windows named pipes, validate logical endpoint
names, apply endpoint ACLs, frame typed messages, enforce allocation limits,
honor cancellation, serialize same-direction I/O, and close connections.

## Non-responsibilities
Session-token generation and validation, plugin policy, process supervision,
automatic reconnection, frame scheduling, and tracking merge belong elsewhere.

## Current implementation
`Listen` creates a one-shot Windows named pipe listener and `Connect` opens its
client. Microsoft go-winio provides overlapped Windows I/O. A four-byte
big-endian prefix frames strict `protocol.Message` JSON. Non-Windows builds
return `ErrUnsupportedPlatform`.

## Public/internal interfaces
`ServerConfig`, `ClientConfig`, `Listener`, `Listen`, and `Connect`. Accepted
and connected streams implement `protocol.Conn`.

## Owned data
Logical pipe endpoints, the single accept result, framing buffers, directional
deadlines, and connection closure state. Session tokens are not owned or
inspected by this package.

## Dependencies
Depends on `pkg/protocol`, Microsoft go-winio v0.6.2, and
`golang.org/x/sys/windows`.

## Concurrency and lifecycle
One receive and one send may proceed concurrently; calls in the same direction
are serialized. Context cancellation interrupts active I/O and derived
deadlines are cleared afterward. Listener and connection close operations are
idempotent. A canceled caller may resume waiting for the listener's sole
underlying accept; exactly one caller can consume the connection.

## Error handling
Invalid names, consumed listeners, unsupported platforms, malformed frames,
oversized frames, clean EOF, context cancellation, and closure are
distinguishable with sentinel errors. Inbound malformed frames and partial
writes close the stream; outbound validation before writing leaves it usable.

## Performance constraints
The four-byte length is checked against `protocol.MaxMessageSize` before
allocation. IPC has no message queue; latest-frame coalescing remains in
`pkg/pluginruntime`.

## Security boundaries
Logical names cannot inject UNC paths. go-winio rejects remote clients. The
pipe SDDL grants access only to LocalSystem and the current process user's SID.
`protocol.Hello.Token` remains the session authentication boundary and is
never stored, parsed, or logged by IPC.

## Required tests
`api_test.go`, `framing_test.go`, `conn_test.go`, and `server_test.go` cover
portable validation, hostile framing, cancellation, concurrency, and
lifecycle. `platform_windows_test.go` performs real named-pipe exchange and
security configuration checks. `platform_other_test.go` covers unsupported
platform behavior when run on non-Windows.

## Known gaps
Host plugin spawning is not implemented yet. `internal/plugins` must generate
the per-launch logical name and token, pass the child environment, accept the
connection, and apply Host-side Hello authentication.

## Completion definition
Host and plugin can exchange the versioned protocol over a protected,
bounded, cancelable one-shot Windows named pipe without an unbounded IPC queue.
