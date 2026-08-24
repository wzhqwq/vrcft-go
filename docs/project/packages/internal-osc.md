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
  - id: external-catalog-mode-test
    description: External catalog mode never refreshes an OSCQuery catalog
    type: symbol
    path: internal/osc/controller_test.go
    pattern: '(?m)^func TestControllerExternalCatalogModeDoesNotRefreshCatalog\('
    weight: 2
    required: true
---
# Package: internal/osc

## Purpose
Integrate VRChat OSC and OSCQuery with compiled VRCFT output bindings.
## Responsibilities
Discover VRChat, receive `/avatar/change`, query paths, compile bindings from OSCQuery or validated endpoints, suppress changes, encode bundles, and send UDP. In Application-owned external-catalog mode, retain discovery and target selection while accepting only the explicitly installed avatar-plan catalog and same-generation evaluator source.
## Non-responsibilities
Avatar JSON discovery/decoding and avatar-plan construction belong to `internal/avatar`; tracking merge and parameter evaluation belong upstream.
## Current implementation
Discovery, OSCQuery, packet parsing, compiled scalar sending, retries, and benchmarks are implemented. `BuildCatalog` flattens writable supported OSCQuery methods and delegates to `BuildCatalogFromEndpoints`, so OSCQuery and avatar JSON inputs use one deterministic binding compiler. Catalogs deep-clone bindings, raw endpoints, and output plans for ownership-safe consumers. The default OSCQuery catalog mode remains available for standalone Controller behavior. External-catalog mode does not compile, refresh, or overwrite the catalog from OSCQuery; its generation-fenced runtime couples an installed cloned catalog with a same-generation source, and its capacity-one avatar mailbox publishes the newest control notification independently of diagnostic events.
## Public/internal interfaces
`OSCService`, `Controller`, `ParameterSender`, `ValueSource`, and packet APIs.
## Owned data
VRChat connection state, query catalog, send plan, change cache, packet buffers, and compiled output-binding semantics.
## Dependencies
Depends on generated parameter definitions.
## Concurrency and lifecycle
Controller workers share cancellable context; send plan/cache transitions are synchronized. External-runtime clear/install/publish operations fence generations so clearing waits for an old send and a stale evaluator source cannot start a new one.
## Error handling
Network failures emit controller events; malformed UDP input is dropped safely.
## Performance constraints
Steady-state sender paths remain zero-allocation and datagrams respect configured limits.
## Security boundaries
OSC addresses, packet sizes, discovered targets, and query responses are validated.
## Required tests
Wire parity, catalog compilation, default and external catalog modes, generation-fenced runtime/mailbox behavior, race tests, retries, boundaries, and benchmarks. `TestControllerExternalCatalogModeDoesNotRefreshCatalog` is the structural external-mode evidence.
## Known gaps
Avatar discovery and avatar-plan construction remain outside this package. M6 Application composition now installs an `internal/avatar` plan's catalog atomically with its control transition and feeds current-generation evaluator snapshots. M7 retains root Wails construction, persisted configuration, and frontend diagnostics/configuration work; numeric Lip payload and Expression-to-Lip mapping remain deferred.
## Completion definition
It participates in an atomic avatar-aware end-to-end pipeline.
