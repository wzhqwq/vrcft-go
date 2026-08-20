---
id: internal-osc
kind: go-package
path: internal/osc
milestone: M6
depends_on: [internal-parameters]
checks:
  - id: package-tests
    description: OSC tests pass
    type: command
    command: go-test
    args: [./internal/osc]
    weight: 4
    required: true
  - id: race-tests
    description: OSC concurrency is race-free
    type: command
    command: go-test-race
    args: [./internal/osc]
    weight: 2
    required: true
  - id: hot-path-benchmark
    description: Sender allocation benchmark exists
    type: symbol
    path: internal/osc/sender_benchmark_test.go
    pattern: 'BenchmarkParameterSenderChangedFrame'
    weight: 1
    required: true
---
# Package: internal/osc

## Purpose
Integrate VRChat OSC and OSCQuery with compiled VRCFT output bindings.
## Responsibilities
Discover VRChat, receive `/avatar/change`, query paths, compile bindings from OSCQuery or validated endpoints, suppress changes, encode bundles, and send UDP.
## Non-responsibilities
Avatar JSON discovery/decoding and avatar-plan construction belong to `internal/avatar`; tracking merge and parameter evaluation belong upstream.
## Current implementation
Discovery, OSCQuery, packet parsing, compiled scalar sending, retries, and benchmarks are implemented. `BuildCatalog` flattens writable supported OSCQuery methods and delegates to `BuildCatalogFromEndpoints`, so OSCQuery and avatar JSON inputs use one deterministic binding compiler. Catalogs deep-clone bindings, raw endpoints, and output plans for ownership-safe consumers.
## Public/internal interfaces
`OSCService`, `Controller`, `ParameterSender`, `ValueSource`, and packet APIs.
## Owned data
VRChat connection state, query catalog, send plan, change cache, packet buffers, and compiled output-binding semantics.
## Dependencies
Depends on generated parameter definitions.
## Concurrency and lifecycle
Controller workers share cancellable context; send plan/cache transitions are synchronized.
## Error handling
Network failures emit controller events; malformed UDP input is dropped safely.
## Performance constraints
Steady-state sender paths remain zero-allocation and datagrams respect configured limits.
## Security boundaries
OSC addresses, packet sizes, discovered targets, and query responses are validated.
## Required tests
Wire parity, catalog compilation, race tests, retries, boundaries, and benchmarks.
## Known gaps
The service is not fed by the final evaluator. Avatar discovery is intentionally outside this package; M6 must install an `internal/avatar` plan's catalog atomically with the rest of the control transition.
## Completion definition
It participates in an atomic avatar-aware end-to-end pipeline.
