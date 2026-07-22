---
id: root
kind: go-package
path: .
milestone: M7
depends_on: [internal-application]
checks:
  - id: package-builds
    description: Root application builds
    type: command
    command: go-test-build
    args: [.]
    weight: 2
    required: true
  - id: wails-config
    description: Wails configuration exists
    type: file
    path: wails.json
    weight: 1
    required: true
blockers:
  - check: package-builds
    blocks: [M7]
---
# Package: root

## Purpose
Provide the Wails executable entry point.
## Responsibilities
Construct the application and bind it to Wails.
## Non-responsibilities
Business logic and device protocols belong to internal packages.
## Current implementation
The Wails template starts `application.Application`.
## Public/internal interfaces
`main` and the root Wails application bootstrap.
## Owned data
Process-wide Wails options only.
## Dependencies
Depends on `internal/application` and generated frontend assets.
## Concurrency and lifecycle
Wails owns the process lifecycle and delegates service shutdown.
## Error handling
Startup failures must be surfaced before the UI is considered ready.
## Performance constraints
The bootstrap must not perform frame processing.
## Security boundaries
Only explicitly bound application methods are exposed to the frontend.
## Required tests
Root build and Wails configuration checks.
## Known gaps
The composed application does not yet contain the complete product pipeline.
## Completion definition
A release build starts and stops every product service safely.
