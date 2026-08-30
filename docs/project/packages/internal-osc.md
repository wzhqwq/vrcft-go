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
  - id: target-policy-tested
    description: Automatic and manual OSC target policy validation is tested
    type: symbol
    path: internal/osc/controller_test.go
    pattern: '(?m)^func TestNewControllerValidatesTargetMode\('
    weight: 2
    required: true
  - id: manual-target-owned
    description: Manual targets survive OSCQuery transitions
    type: symbol
    path: internal/osc/controller_test.go
    pattern: '(?m)^func TestControllerManualTargetSurvivesDiscoveryTransitions\('
    weight: 2
    required: true
  - id: preferred-target-fails-closed
    description: A missing preferred service never falls back silently
    type: symbol
    path: internal/osc/controller_test.go
    pattern: '(?m)^func TestControllerPreferredMissingServiceDoesNotSelectAnotherVRChat\('
    weight: 1
    required: true
---
# Package: internal/osc

## Purpose
Integrate VRChat OSC and OSCQuery with compiled VRCFT output bindings.
## Responsibilities
Discover VRChat, receive `/avatar/change`, query paths, compile bindings from OSCQuery or validated endpoints, suppress changes, encode bundles, and send UDP. Own an explicit construction-time automatic/manual target policy. In Application-owned external-catalog mode, retain discovery and target selection while accepting only the explicitly installed avatar-plan catalog and same-generation evaluator source.
## Non-responsibilities
Avatar JSON discovery/decoding and avatar-plan construction belong to `internal/avatar`; tracking merge and parameter evaluation belong upstream.
## Current implementation
Discovery, OSCQuery, packet parsing, compiled scalar sending, retries, and benchmarks are implemented. `BuildCatalog` flattens writable supported OSCQuery methods and delegates to `BuildCatalogFromEndpoints`, so OSCQuery and avatar JSON inputs use one deterministic binding compiler. Catalogs deep-clone bindings, raw endpoints, and output plans for ownership-safe consumers. Automatic target mode retains deterministic discovery; an exact preferred service stays targetless with a bounded diagnostic when absent instead of selecting another instance. Manual mode validates and installs one explicit unicast IP/port at startup; OSCQuery connect, disconnect, and reconnect events cannot replace or clear it, while receive and avatar-change facilities remain active. `OSCStatus` reports target mode, discovery connection, and installed-target state independently. The default OSCQuery catalog mode remains available for standalone Controller behavior. External-catalog mode does not compile, refresh, or overwrite the catalog from OSCQuery; its generation-fenced runtime couples an installed cloned catalog with a same-generation source, and its capacity-one avatar mailbox publishes the newest control notification independently of diagnostic events.
## Public/internal interfaces
`TargetModeAuto`, `TargetModeManual`, `OSCTarget`, target fields in `ControllerConfig`/`OSCStatus`, `OSCService`, `Controller`, `ParameterSender`, `ValueSource`, and packet APIs.
## Owned data
VRChat discovery connection state, normalized target policy, installed send target, query catalog, send plan, change cache, packet buffers, and compiled output-binding semantics.
## Dependencies
Depends on generated parameter definitions.
## Concurrency and lifecycle
Controller workers share cancellable context; send plan/cache transitions are synchronized. In auto mode discovery owns target transitions; in manual mode the validated configured address owns them for the controller lifetime. External-runtime clear/install/publish operations fence generations so clearing waits for an old send and a stale evaluator source cannot start a new one.
## Error handling
Invalid target-mode combinations and non-unicast, unspecified, multicast, broadcast, DNS, or zero-port manual targets fail construction. A missing preferred auto service leaves no target and reports a bounded diagnostic. Network failures emit controller events; malformed UDP input is dropped safely.
## Performance constraints
Steady-state sender paths remain zero-allocation and datagrams respect configured limits.
## Security boundaries
OSC addresses, packet sizes, discovered targets, and query responses are validated. Manual targets accept only explicit unicast IP literals and valid ports, avoiding DNS and discovery substitution; a manual address cannot be overwritten by untrusted OSCQuery changes. Diagnostics are valid UTF-8 and bounded, and packet/query payloads do not cross the root API.
## Required tests
Wire parity, catalog compilation, default and external catalog modes, generation-fenced runtime/mailbox behavior, race tests, retries, boundaries, and benchmarks. `TestNewControllerValidatesTargetMode`, `TestControllerManualTargetSurvivesDiscoveryTransitions`, `TestControllerManualTargetStillPublishesAvatarChanges`, and `TestControllerPreferredMissingServiceDoesNotSelectAnotherVRChat` cover target policy; `TestControllerExternalCatalogModeDoesNotRefreshCatalog` remains the structural external-mode evidence.
## Known gaps
Avatar discovery and avatar-plan construction remain outside this package. M6 Application composition installs an `internal/avatar` plan's catalog atomically with its control transition and feeds current-generation evaluator snapshots. M7 target-policy configuration and status fields are implemented; persisted selection and Wails adaptation are owned outside this package. Frontend diagnostics/configuration work and numeric Lip/Expression-to-Lip behavior remain deferred.
## Completion definition
It participates in an atomic avatar-aware end-to-end pipeline.
