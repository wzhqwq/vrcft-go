---
id: internal-steamlink
kind: go-package
path: internal/steamlink
milestone: M2
depends_on: [pkg-osc, pkg-pluginapi, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Steam Link configuration and OSC input tests pass
    type: command
    command: go-test
    args: [./internal/steamlink]
    weight: 3
    required: true
  - id: race-tests
    description: Steam Link input handling is race-free
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
---
# Package: internal/steamlink

## Purpose
Validate the managed Steam Link adapter's local configuration and accepted OSC tracking input.

## Responsibilities
Decode the one supported configuration object; maintain the fixed Steam Link raw-field registry; decode complete OSC datagrams through `pkg/osc`; and classify recognized invalid and unknown input without retaining packet data.

## Non-responsibilities
This package does not bind UDP sockets, manage plugin lifecycle, map raw fields to tracking-model outputs, publish frames, select sources, or configure SteamVR.

## Current implementation
The package accepts only an optional `listenPort` integer from 1 through 65535, defaulting to 9015. Its input boundary accepts the fixed Steam Link gaze and face-weight address profile, preserves valid wire order, and rejects malformed, unsupported, oversized, over-message-limit, and over-address-limit datagrams as a whole. Recognized malformed field values are counted without suppressing valid siblings; unknown supported messages produce bounded diagnostics.

## Public/internal interfaces
`parseConfig`, `rawID`, `observation`, `inputReport`, and `decodeDatagram` are private adapter contracts. The package consumes the public `pkg/osc` message representation and does not define a second OSC wire decoder.

## Owned data
The package owns decoded configuration values, fixed field IDs, per-call observations, and bounded input diagnostics. No returned observation or report retains the received datagram.

## Dependencies
Production code may use the Go standard library and `pkg/osc`, `pkg/pluginapi`, and `pkg/trackingmodel`. This initial input boundary currently consumes `pkg/osc` only.

## Concurrency and lifecycle
Configuration parsing and datagram decoding are pure per-call operations. They own no sockets, goroutines, or retained input state.

## Error handling
Malformed or unsupported OSC input and packet-limit violations reject the complete datagram without observations. Invalid known messages increment the report and permit valid siblings. Unknown supported addresses increment bounded diagnostics only. Configuration errors identify the invalid document condition without including raw configuration text.

## Performance constraints
Datagrams are limited to 65507 bytes, 512 flattened messages, and 256-byte addresses. Unknown-address retention is limited to 32 names per report.

## Security boundaries
The adapter accepts only loopback-facing configuration in later lifecycle code. At this input boundary, strict JSON parsing and fixed address matching prevent configuration ambiguity and untrusted addresses from extending the supported tracking profile.

## Required tests
Package tests cover strict configuration, whole-datagram codec rejection, packet limits, known field validation, sibling handling, duplicate observations, unknown diagnostics, and malformed packet behavior. `FuzzDecodeDatagram` checks that arbitrary input cannot return observations with errors or non-finite values.

## Known gaps
Raw-to-model mapping, freshness state, UDP reception, Driver lifecycle, plugin command wiring, and hardware validation are implemented by later Steam Link adapter tasks.

## Completion definition
The package is complete when its configuration and input boundary accepts only the fixed profile and records valid observations without partial recovery from malformed datagrams.
