---
id: internal-tracking
kind: go-package
path: internal/tracking
milestone: M3
depends_on: [pkg-trackingmodel]
checks:
  - id: package-builds
    description: Tracking package builds
    type: command
    command: go-test-build
    args: [./internal/tracking]
    weight: 1
    required: true
  - id: merged-frame-implemented
    description: Merged frame has canonical fields
    type: not_placeholder
    path: internal/tracking/frame.go
    patterns: ['(?s)type MergedFrame struct\s*\{\s*\}']
    weight: 3
    required: true
  - id: service-implemented
    description: Tracking service implementation exists
    type: symbol
    path: internal/tracking/service.go
    pattern: 'type service struct|func NewService'
    weight: 3
    required: true
---
# Package: internal/tracking

## Purpose
Ingest plugin frames and produce one routed merged tracking frame.
## Responsibilities
Validate sequence/time/capability, reject stale generations, select sources, merge fields, and publish summaries.
## Non-responsibilities
Plugin process supervision and signal filtering belong elsewhere.
## Current implementation
Routing configuration and service interfaces exist; merged frame and service implementation are empty.
## Public/internal interfaces
`Service`, `RoutingConfig`, `SourceSelection`, and `MergedFrame`.
## Owned data
Latest valid frames per plugin, routing state, and merged snapshot.
## Dependencies
Consumes `pkg/trackingmodel` frames.
## Concurrency and lifecycle
Submissions from multiple plugins are serialized into snapshot publication.
## Error handling
Invalid, out-of-order, stale, and unavailable sources are reflected in validity/summary state.
## Performance constraints
Latest-frame semantics prevent backlog and bound merge work.
## Security boundaries
Plugin identity and generation must be associated with every accepted frame.
## Required tests
Routing, capability fallback, merge, stale generation, order, and subscriptions.
## Known gaps
No concrete service or merged frame model exists.
## Completion definition
Concurrent plugin input produces a deterministic validated merged snapshot.
