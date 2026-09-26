---
id: internal-steamlink
kind: go-package
path: internal/steamlink
milestone: M2
depends_on: [pkg-osc, pkg-pluginapi, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Steam Link driver, configuration, OSC input, and diagnostics tests pass
    type: command
    command: go-test
    args: [./internal/steamlink]
    weight: 3
    required: true
  - id: race-tests
    description: Steam Link driver and input handling are race-free
    type: command
    command: go-test-race
    args: [./internal/steamlink]
    weight: 2
    required: true
  - id: input-fuzz-regression
    description: Steam Link datagram decoder fuzz target exists
    type: symbol
    path: internal/steamlink/input_fuzz_test.go
    pattern: '(?m)^func FuzzDecodeDatagram\('
    weight: 1
    required: true
  - id: public-import-boundary
    description: Steam Link production imports use only public tracking packages
    type: symbol
    path: internal/steamlink/architecture_test.go
    pattern: '(?m)^func TestProductionImportsOnlyPublicPackages\('
    weight: 1
    required: true
---
# Package: internal/steamlink

## Purpose
Adapt managed Steam Link loopback OSC telemetry into subscribed tracking frames while preserving host lifecycle and diagnostic contracts.

## Responsibilities
Decode the one supported configuration object; receive bounded loopback UDP datagrams; maintain the fixed raw-field registry; decode complete OSC datagrams through `pkg/osc`; map fresh subscribed fields; and drive plugin activation, configuration recovery, health, and bounded diagnostics through `pluginapi.Host`.

## Non-responsibilities
This package does not select sources, apply host-side filtering or calibration, perform final OSC output, configure SteamVR, or promise hardware compatibility beyond the documented synthetic input profile.

## Current implementation
The package accepts only an optional `listenPort` integer from 1 through 65535, defaulting to 9015, and listens only on IPv4 loopback. Its input boundary accepts the fixed Steam Link gaze and face-weight address profile, preserves valid wire order, and rejects malformed, unsupported, oversized, over-message-limit, and over-address-limit datagrams as a whole. A Driver receives startup once, then applies typed host controls. It publishes only subscribed, fresh mapped fields at no more than 100 Hz; it retains no numeric cache while inactive or unsubscribed. Activation, subscription changes, and successful port rebinds reset the stream and fence queued receiver data by epoch and local receive time. Invalid configuration and failed binds leave controls alive; initial bind failures retry every two seconds, while a failed live port change preserves the working receiver. Health is Ready only after recognized valid input, Disconnected before input or after two seconds, and Error while a configuration or bind failure is unresolved.

## Public/internal interfaces
`Driver` is the plugin entry point and exposes `Descriptor` and `Run` required by `pluginapi.Driver`. `parseConfig`, `rawID`, `observation`, `inputReport`, `decodeDatagram`, `streamState`, and receiver/diagnostic helpers are private adapter contracts. The package consumes the public `pkg/osc` message representation and does not define a second OSC wire decoder.

## Owned data
The package owns decoded configuration values, fixed field IDs, per-call observations, a 64-datagram receive queue, one receiver worker per binding, per-Run stream state, and aggregate diagnostics. No returned observation or report retains received packet bytes; diagnostics retain at most 32 unknown address names and no configuration or tracking values.

## Dependencies
Production code may use the Go standard library and `pkg/osc`, `pkg/pluginapi`, and `pkg/trackingmodel`.

## Concurrency and lifecycle
Configuration parsing, decoding, mapping, and stream state are independently testable. `Driver.Run` exclusively owns lifecycle state and field caches, starts at most one receive worker for the active binding, closes it and waits for it at shutdown/rebind, and stops its publication ticker. The worker tags packets with an atomic epoch; the driver discards packets from older epochs or received before the current transition fence. Buffered network packets cannot be identified as stale without device timestamps.

## Error handling
Malformed or unsupported OSC input and packet-limit violations reject the complete datagram without observations. Invalid known messages increment the report and permit valid siblings. Unknown supported addresses, malformed/unsupported packets, invalid messages, and receiver queue drops are summarized at most once per five seconds. Configuration errors identify only the revision/state, without raw configuration text. Unrecoverable receiver read errors return from `Run` for host-supervised restart.

## Performance constraints
Datagrams are limited to 65507 bytes, 512 flattened messages, and 256-byte addresses. The receive queue holds 64 copied datagrams. Unknown-address retention is limited to 32 names, and publication is limited to 100 Hz.

## Security boundaries
The receiver binds only `127.0.0.1`. Strict JSON parsing and fixed address matching prevent configuration ambiguity and untrusted addresses from extending the supported tracking profile. Diagnostics never log packet bytes, raw tracking values, or configuration data.

## Required tests
Package tests cover strict configuration, whole-datagram codec rejection, packet limits, known field validation, mapping/freshness state, UDP queueing, lifecycle controls and rebinds, status transitions, bounded diagnostics, cleanup, and rejected frame handling. `FuzzDecodeDatagram` checks that arbitrary input cannot return observations with errors or non-finite values. Windows-only `internal/plugins/steamlink_integration_test.go` builds the real command, discovers it through `DirectoryCatalog`, persists configuration through `JSONStore`, and runs it through `Manager`, named-pipe IPC, `ProcessLauncher`, and a bounded `FrameSink`. It verifies dynamic loopback ports, subscription generations and field trimming, pause/resume, rebind fencing, malformed-datagram rejection, disable cleanup, and fresh-to-dropout delivery through the host tracking and processing pipeline without resetting it.

## Known gaps
Hardware validation is still required before making claims about actual Pico 4 Pro Steam Link output.

## Completion definition
The package is complete when the Driver safely manages loopback reception, controls, recovery, health, and bounded diagnostics while publishing only fresh subscribed data from the fixed profile.
