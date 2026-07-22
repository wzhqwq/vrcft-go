---
id: pkg-pluginapi
kind: go-package
path: pkg/pluginapi
milestone: M1
depends_on: [pkg-trackingmodel]
checks:
  - id: package-tests
    description: Plugin API package builds and tests
    type: command
    command: go-test
    args: [./pkg/pluginapi]
    weight: 2
    required: true
  - id: driver-contract
    description: Driver and environment contracts exist
    type: symbol
    path: pkg/pluginapi/plugin.go
    pattern: 'type Driver interface'
    weight: 2
    required: true
---
# Package: pkg/pluginapi

## Purpose
Define the stable API implemented by vendor tracking plugins.
## Responsibilities
Describe plugins, expose lifecycle environment, publish frames/status/logs, and receive commands.
## Non-responsibilities
IPC framing and host policy are hidden from plugin authors.
## Current implementation
Driver, environment, configuration, commands, status, descriptor, and logging types exist.
## Public/internal interfaces
All exported types are part of the plugin author contract.
## Owned data
Serializable configuration and status values only.
## Dependencies
Depends on the public tracking model.
## Concurrency and lifecycle
Drivers run under context cancellation; environment publishing is safe for driver workers.
## Error handling
Driver errors terminate the run and status/log APIs report device conditions.
## Performance constraints
Frame publication should support latest-frame/drop behavior without blocking device callbacks.
## Security boundaries
Plugins receive only their scoped configuration and commands.
## Required tests
API compatibility, JSON contracts, and example driver compilation.
## Known gaps
Selective subscription is not yet represented in the public command contract.
## Completion definition
Third-party drivers compile against a versioned, documented, tested API.
