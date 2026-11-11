---
id: internal-picoconnect
kind: go-package
path: internal/picoconnect
milestone: M2
depends_on: [pkg-pluginapi, pkg-trackingmodel]
checks:
  - id: package-tests
    description: PICO Connect parser, mapping, state, and driver tests pass
    type: command
    command: go-test
    args: [./internal/picoconnect]
    weight: 3
    required: true
  - id: race-tests
    description: PICO Connect driver and receiver are race-free
    type: command
    command: go-test-race
    args: [./internal/picoconnect]
    weight: 2
    required: true
  - id: protocol-zero-receiver
    description: Default PICO Connect transfer protocol receiver exists after live interface verification
    type: file
    path: internal/picoconnect/protocol0.go
    weight: 3
    required: true
---
# Package: internal/picoconnect

## Purpose

Adapt PICO Connect face tracking to subscribed Eye and Expression frames.

## Responsibilities

Parse legacy 29765/UDP face packets, map finite PICO shapes, expire per-slot values, receive host controls, and report device status. Protocol 0 discovery remains an open implementation dependency.

## Non-responsibilities

Host source selection, final OSC output, Pico Connect settings changes, and unverified protocol decoding.

## Current implementation

The protocol 2 path listens on IPv4 loopback, requires a 16-byte header with tracking type 2 and at least 892 body bytes, and maps the first 52 finite shapes. The default protocol 0 selection reports an explicit unsupported status pending interface evidence. The driver uses a bounded packet queue and per-field 250 ms freshness. Initial bind conflicts retry every two seconds; an unexpected receiver failure returns to the supervisor.

## Public/internal interfaces

`Driver` implements `pluginapi.Driver`; parser, mapping, state, and configuration helpers are private.

## Owned data

At most one UDP socket, a 64-packet queue, one current shape cache, and aggregate connection state. Raw packets are not logged or persisted.

## Dependencies

Go standard library, `pkg/pluginapi`, and `pkg/trackingmodel` only.

## Concurrency and lifecycle

The receiver owns the socket read loop. The driver owns mapping and cache state, fences old queued packets by epoch, and closes the socket on shutdown or rebind.

## Error handling

Invalid packets are discarded without changing state and summarized at most once every five seconds without payloads. Invalid configuration or bind failures are surfaced through status; protocol 0 is explicitly unsupported until its interface is verified.

## Performance constraints

UDP input is bounded to 2048 bytes after a full-size read; queue length is 64; publication is capped by the 10 ms ticker.

## Security boundaries

Bind only `127.0.0.1`; never log biometric values or packet bytes.

## Required tests

Parser boundaries, mapping, freshness, configuration, live loopback reception, oversized packet recovery, and race tests.

## Known gaps

Protocol 0 transport and field layout are not yet known. Protocol 2 field semantics and expression correctness need hardware validation; only packet transport structure was observed on a real PICO Connect 10.6.6 session.

## Completion definition

Both protocol 0 and 2 receive and publish verified tracking from a real PICO 4 Pro / Enterprise session, and package checks pass.
