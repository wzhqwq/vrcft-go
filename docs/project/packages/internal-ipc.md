---
id: internal-ipc
kind: go-package
path: internal/ipc
milestone: M2
depends_on: [pkg-protocol]
checks:
  - id: package-builds
    description: IPC package builds
    type: command
    command: go-test-build
    args: [./internal/ipc]
    weight: 1
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
---
# Package: internal/ipc

## Purpose
Provide authenticated framed transport between host and plugin processes.
## Responsibilities
Listen, connect, authenticate, frame messages, enforce limits, and close connections.
## Non-responsibilities
Plugin policy and tracking merge belong elsewhere.
## Current implementation
Client and server files contain only package declarations; framing helpers exist separately.
## Public/internal interfaces
Host listener and plugin connector implementations of `protocol.Conn`.
## Owned data
Connection sessions, authentication tokens, and framing buffers.
## Dependencies
Depends on `pkg/protocol`.
## Concurrency and lifecycle
Each connection has bounded reader/writer lifetimes tied to context cancellation.
## Error handling
Malformed, oversized, unauthenticated, and closed connections are classified.
## Performance constraints
Latest-frame traffic must avoid unbounded queues.
## Security boundaries
Tokens, payload limits, peer identity, and local endpoint permissions are enforced.
## Required tests
Framing round trips, authentication, limits, cancellation, and backpressure.
## Known gaps
No client or server transport implementation exists.
## Completion definition
Host and plugin can exchange the versioned protocol safely under load.
