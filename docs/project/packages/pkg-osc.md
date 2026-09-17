---
id: pkg-osc
kind: go-package
path: pkg/osc
milestone: M1
depends_on: []
checks:
  - id: package-tests
    description: Public OSC codec and server tests pass
    type: command
    command: go-test
    args: [./pkg/osc]
    weight: 4
    required: true
  - id: race-tests
    description: Public OSC server lifecycle is race-free
    type: command
    command: go-test-race
    args: [./pkg/osc]
    weight: 2
    required: true
  - id: fuzz-regression
    description: OSC decoder fuzz regression target exists
    type: symbol
    path: pkg/osc/fuzz_test.go
    pattern: '(?m)^func FuzzUnmarshalPacket\('
    weight: 1
    required: true
  - id: server-integration
    description: UDP malformed-drop and ordered delivery are tested
    type: symbol
    path: pkg/osc/server_test.go
    pattern: '(?m)^func TestServerServesMessagesInOrderAndDropsMalformedDatagrams\('
    weight: 2
    required: true
---
# Package: pkg/osc

## Purpose
Provide a standard-library-only public OSC wire codec and UDP server for host and third-party plugin use.

## Responsibilities
Expose the supported OSC value types and constructors; encode messages and raw-element bundles; decode packets into ordered flat messages; convert NTP timetags; validate supported OSC addresses; and receive, decode, synchronously dispatch, and explicitly send UDP packets.

## Non-responsibilities
This package does not provide OSCQuery, HTTP, WebSocket, mDNS, VRChat discovery, address routing, target selection, parameter bindings, timetag scheduling, delivery guarantees, or application lifecycle policy.

## Current implementation
The codec supports `int32`, `float32`, string, and bool values; messages; and bundles. Decoder support includes nested bundles, but `UnmarshalPacket` returns a flat message list in wire order and discards bundle timetags, so scheduling remains the caller's responsibility. `Server` invokes one handler synchronously for every decoded message; malformed or unsupported UDP datagrams are silently dropped and later valid datagrams continue to be served.

## Public/internal interfaces
The public contract comprises `ValueKind`, `Value`, `Int32`, `Float32`, `String`, `Bool`, `Message`, `Bundle`, `MarshalMessage`, `MarshalBundle`, `UnmarshalPacket`, `NTPTime`, `ValidAddress`, `HandlerFunc`, and `Server` with `ListenUDP`, `LocalAddr`, `Serve`, `SendTo`, and `Close`.

## Owned data
The package owns encoded packet results, decoded messages and argument data, raw bundle elements, and socket-level server state. Retained decoded data and returned local/remote addresses are independently owned and do not alias the UDP receive buffer.

## Dependencies
Depends only on the Go standard library.

## Concurrency and lifecycle
One `Serve` call may be active per Server. Valid datagrams are handled synchronously and in wire order; a slow handler delays later reads. Context cancellation or `Close` wakes a blocked serve, and a non-closed Server may serve again after cancellation. `SendTo` is safe concurrently with serving and uses an explicit destination; `Close` is idempotent and permanently closes the socket.

## Error handling
Encoding and direct API validation return classified errors for malformed packets, unsupported types, invalid addresses or arguments, an already-running server, and a closed server. The decoder requires exact consumption and validates padding, bundle elements, and nesting. The UDP receive loop drops malformed and unsupported packets without invoking the handler or terminating the server.

## Performance constraints
The server uses bounded control state, reuses one maximum-UDP receive buffer, creates no goroutine per packet or message, and keeps handler dispatch synchronous.

## Security boundaries
Untrusted UDP input is parsed with packet bounds, zero-padding, element-length, exact-consumption, and nesting validation. The package neither authenticates remote senders nor selects or trusts a default target; callers inspect remote addresses and provide explicit send destinations.

## Required tests
Executable package and race tests cover codec and server contracts. Fuzz regression evidence protects the decoder's no-panic behavior, and server integration evidence verifies malformed datagrams are dropped while valid messages are delivered in order.

## Known gaps
The contract deliberately covers only the project's commonly used OSC subset, not complete OSC 1.0 or 1.1 type coverage. OSCQuery and all VRChat-specific policy remain outside this package.

## Completion definition
An external consumer can use only `pkg/osc` and the standard library to encode and decode supported OSC packets, serve valid UDP messages in order, drop malformed input safely, and send an already encoded packet to an explicit address.
