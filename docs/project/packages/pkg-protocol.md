---
id: pkg-protocol
kind: go-package
path: pkg/protocol
milestone: M1
depends_on: [pkg-pluginapi, pkg-trackingmodel]
checks:
  - id: package-builds
    description: Protocol package builds
    type: command
    command: go-test-build
    args: [./pkg/protocol]
    weight: 1
    required: true
  - id: connection-implemented
    description: Protocol connection contract is implemented
    type: not_placeholder
    path: pkg/protocol/connection.go
    patterns: ['(?s)type Conn interface\s*\{\s*// TODO\s*\}']
    weight: 3
    required: true
  - id: subscription-message
    description: Tracking subscription message is defined
    type: symbol
    path: pkg/protocol/message.go
    pattern: 'Message.*Subscription'
    weight: 2
    required: true
---
# Package: pkg/protocol

## Purpose
Define the versioned wire contract between plugin runtime and host.
## Responsibilities
Message framing, version negotiation, payload limits, handshake, commands, frames, heartbeat, status, logs, and errors.
## Non-responsibilities
Socket implementation and process policy belong to internal packages.
## Current implementation
Headers and several message types exist; `Conn` and subscription messages are incomplete.
## Public/internal interfaces
`Conn`, headers, message types, and serializable payloads shared across processes.
## Owned data
Wire constants and message schemas.
## Dependencies
References public plugin API and tracking model payloads.
## Concurrency and lifecycle
Connection contract must define serialized writes and cancellation-safe reads.
## Error handling
Unknown versions/types, invalid lengths, and malformed payloads are explicit protocol errors.
## Performance constraints
Frame encoding is bounded by `MaxPayloadSize` and avoids unnecessary copies.
## Security boundaries
Magic, version, payload length, sequence, and authentication handshake are validated.
## Required tests
Binary header golden tests, payload round trips, version negotiation, limits, and subscriptions.
## Known gaps
Connection behavior and selective tracking subscription are not defined.
## Completion definition
Host and plugin runtime share a stable fully tested protocol contract.
