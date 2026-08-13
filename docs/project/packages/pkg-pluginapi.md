---
id: pkg-pluginapi
kind: go-package
path: pkg/pluginapi
milestone: M1
depends_on: [pkg-trackingmodel]
checks:
  - id: package-tests
    description: Plugin API package tests pass
    type: command
    command: go-test
    args: [./pkg/pluginapi]
    weight: 3
    required: true
  - id: package-race-tests
    description: Plugin API package race tests pass
    type: command
    command: go-test-race
    args: [./pkg/pluginapi]
    weight: 2
    required: true
  - id: example-driver
    description: Example driver test exists
    type: file
    path: pkg/pluginapi/example_test.go
    weight: 2
    required: true
---
# Package: pkg/pluginapi

## Purpose
Define the stable API implemented by vendor tracking plugins.

## Responsibilities
Describe plugins, expose Host and Startup, publish frames, typed status and logs, validate typed controls, and deliver mixed configuration, subscription, and active-state controls, including metadata-only Lip capability negotiation.

## Non-responsibilities
IPC framing, host process policy, and log persistence are hidden from plugin authors. Numeric Lip payloads and Expression-to-Lip mapping are not defined by this API.

## Current implementation
Driver, Host, Startup, typed controls, configuration, commands, status, descriptor, subscriptions, and logging types are implemented. Descriptors and positive-generation subscriptions accept Lip-only capability value 4, and `Subscription.TrimFrame` preserves that metadata bit without adding Eye, Expression, or other numeric Lip data.

## Public/internal interfaces
All exported types are part of the plugin author contract.

## Owned data
Serializable configuration and status values only. `Config.Clone` provides independent ownership of configuration bytes, and this package owns no LogStore.

## Dependencies
Depends on the public tracking model.

## Concurrency and lifecycle
Drivers run under context cancellation; Host publishing is safe for driver workers. `Host.Startup` remains the immutable initialization snapshot while typed control events communicate later state. `Subscription.TrimFrame` does not mutate either input and returns a filtered copy containing only known subscribed capabilities and validity.

## Error handling
Descriptor SemVer 2.0, control, configuration, subscription, and status validation return explicit errors; known capability validation includes Eye, Expression, and Lip while rejecting all other bits. Zero-length configuration data is canonicalized to nil and Host methods communicate bounded publication outcomes.

## Performance constraints
Frame publication supports latest-frame/drop behavior without blocking device callbacks.

## Security boundaries
Plugins receive only scoped configuration and typed controls.

## Required tests
Executable package and race tests cover API contracts, validation, mixed subscriptions, immutable trimming, JSON behavior, and driver use; the named example driver file is integration evidence only.

## Known gaps
No M4 public API implementation gap remains. A future numeric Lip contract and any Expression-to-Lip mapping remain deferred and must not be inferred from the metadata-only capability.

## Completion definition
Third-party drivers compile against a versioned, documented, fully tested API with Host/Startup, typed controls, and backward-compatible Eye/Expression/Lip capability metadata.
