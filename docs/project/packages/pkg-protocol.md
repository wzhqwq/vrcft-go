---
id: pkg-protocol
kind: go-package
path: pkg/protocol
milestone: M1
depends_on: [pkg-pluginapi, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Protocol package tests pass
    type: command
    command: go-test
    args: [./pkg/protocol]
    weight: 4
    required: true
  - id: message-tests
    description: Protocol message tests exist
    type: file
    path: pkg/protocol/message_test.go
    weight: 2
    required: true
---
# Package: pkg/protocol

## Purpose
Define the versioned JSON wire contract between plugin runtime and host.

## Responsibilities
Define typed JSON protocol v1 messages, strict message-type and payload correspondence, payload limits, handshake values, controls, frames, heartbeat, status, logs, errors, and the context-aware abstract Conn contract.

## Non-responsibilities
Concrete connection framing, socket implementation, and process policy belong to internal IPC and host packages.

## Current implementation
Typed JSON protocol v1 messages, strict decoding with exact tracking-array widths, typed-construction payload limits, and context-aware Conn methods are implemented.

## Public/internal interfaces
Conn, message types, and serializable payloads are shared across processes.

## Owned data
Wire version constants and bounded JSON message schemas.

## Dependencies
References public plugin API and tracking-model payloads.

## Concurrency and lifecycle
The abstract Conn contract accepts contexts; concrete framing and write serialization are internal IPC responsibilities.

## Error handling
Unknown versions or types, payload-type mismatches, oversized concrete or decoded payloads, malformed JSON, incorrect fixed-array widths, and invalid fields return explicit protocol errors.

## Performance constraints
Payload size is bounded by MaxPayloadSize and encoding avoids unnecessary copies where practical.

## Security boundaries
Strict JSON correspondence, version checks, and payload limits protect the protocol boundary; binary headers and magic values are not part of this protocol.

## Required tests
Executable package tests cover typed JSON messages, strict payload correspondence, validation, limits, and context-aware connection behavior; the named message test file is integration evidence only.

## Known gaps
No incomplete subscription or binary-header protocol gap remains; concrete framing is intentionally outside this package.

## Completion definition
Host and plugin runtime share a stable, bounded, fully tested typed JSON protocol v1 contract.
