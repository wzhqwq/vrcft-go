---
id: pkg-trackingmodel
kind: go-package
path: pkg/trackingmodel
milestone: M1
checks:
  - id: package-tests
    description: Tracking model package builds and tests
    type: command
    command: go-test
    args: [./pkg/trackingmodel]
    weight: 2
    required: true
  - id: frame-contract
    description: TrackingFrame contract exists
    type: symbol
    path: pkg/trackingmodel/frame.go
    pattern: 'type TrackingFrame struct'
    weight: 2
    required: true
  - id: compatibility-tests
    description: Tracking frame compatibility tests exist
    type: file
    path: pkg/trackingmodel/frame_test.go
    weight: 2
    required: true
---
# Package: pkg/trackingmodel

## Purpose
Provide the shared canonical tracking data contract across process boundaries.
## Responsibilities
Define capabilities, validity masks, eye samples, expression IDs/sets, timestamps, sequences, and received-frame metadata.
## Non-responsibilities
Vendor conversion, routing, filtering, and parameter evaluation are outside the model.
## Current implementation
TrackingFrame, eye/expression data, capabilities, and received metadata exist.
## Public/internal interfaces
All exported value types used by plugin API and protocol.
## Owned data
Schema definitions only; instances are owned by producers/consumers.
## Dependencies
No project package dependencies.
## Concurrency and lifecycle
Frames are values transferred as immutable snapshots.
## Error handling
Validity masks express absent fields; consumers reject invalid timestamps/sequences.
## Performance constraints
Layout remains bounded and suitable for frequent serialization.
## Security boundaries
Counts and indices are fixed to prevent unbounded payloads.
## Required tests
Layout/limits, validity helpers, compatibility, and serialization round trips.
## Known gaps
No compatibility test file currently protects the public contract.
## Completion definition
The cross-process tracking schema is versioned, bounded, and regression-tested.
