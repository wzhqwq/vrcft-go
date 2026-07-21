# Project Specs and Live Status Design

## Goal

Create an auditable specification for every Go package and non-Go subsystem in
the repository, then derive package, milestone, and project progress from
executable acceptance checks instead of manually edited percentages.

The system must answer four questions at any commit:

1. What does each package own, and what must it not own?
2. Which interfaces and data cross package boundaries?
3. Which acceptance requirements currently pass?
4. Which failure blocks the next useful milestone?

## Scope

The initial catalog covers all packages returned by `go list ./...`:

- repository root;
- `cmd/paramgen`;
- `internal/application`;
- `internal/ipc`;
- `internal/osc`;
- `internal/parameters`;
- `internal/paramgen`;
- `internal/plugins`;
- `internal/processing`;
- `internal/specparser`;
- `internal/tracking`;
- `pkg/pluginapi`;
- `pkg/pluginruntime`;
- `pkg/protocol`;
- `pkg/trackingmodel`.

It also covers these non-Go subsystems:

- `frontend`;
- parameter definitions under `spec`;
- build and release assets under `build`;
- the end-to-end plugin-to-VRChat product flow.

Planned packages such as `internal/avatar`, `internal/evaluator`, and
`internal/requirements` receive specs when their implementation cycle begins.
The registry permits planned entries but distinguishes them from packages that
must already exist, so package-discovery checks remain meaningful.

This work documents and measures the existing project. It does not implement
the missing plugin, IPC, tracking, processing, evaluator, avatar, frontend, or
release features.

## Documentation Architecture

The repository will contain:

```text
docs/project/
├── README.md
├── milestones.md
├── status.md
├── packages/
│   ├── root.md
│   ├── cmd-paramgen.md
│   ├── internal-application.md
│   ├── internal-ipc.md
│   ├── internal-osc.md
│   ├── internal-parameters.md
│   ├── internal-paramgen.md
│   ├── internal-plugins.md
│   ├── internal-processing.md
│   ├── internal-specparser.md
│   ├── internal-tracking.md
│   ├── pkg-pluginapi.md
│   ├── pkg-pluginruntime.md
│   ├── pkg-protocol.md
│   └── pkg-trackingmodel.md
└── subsystems/
    ├── frontend.md
    ├── parameter-spec.md
    ├── build-release.md
    └── end-to-end.md

cmd/projectstatus/
└── main.go

internal/projectstatus/
├── model.go
├── parser.go
├── discovery.go
├── checks.go
├── progress.go
├── report.go
└── *_test.go
```

`README.md` is the stable entry point. `milestones.md` explains product order
and dependencies. Package and subsystem files are authoritative specifications.
`status.md` is generated output and must never be edited to change progress.

## Package Specification Format

Each specification is Markdown with YAML front matter. The front matter is the
machine-readable acceptance contract; the body explains design and ownership.

Example:

```yaml
---
id: internal-osc
kind: go-package
path: internal/osc
milestone: M6
depends_on:
  - internal-parameters
checks:
  - id: package-builds
    description: OSC package can compile
    type: command
    command: go-test-build
    args: [./internal/osc]
    weight: 1
    required: true
  - id: package-tests
    description: OSC unit tests pass
    type: command
    command: go-test
    args: [./internal/osc]
    weight: 3
    required: true
  - id: race-tests
    description: OSC concurrency is race-free
    type: command
    command: go-test-race
    args: [./internal/osc]
    weight: 2
    required: true
  - id: sender-zero-alloc
    description: The hot sender path has an allocation benchmark
    type: symbol
    path: internal/osc/sender_benchmark_test.go
    pattern: BenchmarkParameterSenderChangedFrame
    weight: 1
    required: true
blockers:
  - check: package-builds
    blocks: [M6]
---
```

Commands use registered command IDs and argument arrays. Specifications cannot
contain arbitrary shell strings.

Every body uses these sections:

1. Purpose
2. Responsibilities
3. Non-responsibilities
4. Current implementation
5. Public or internal interfaces
6. Owned data
7. Dependencies
8. Concurrency and lifecycle
9. Error handling
10. Performance constraints
11. Security boundaries
12. Required tests
13. Known gaps
14. Completion definition

Sections may state that a concern is not applicable, but they may not be
omitted. Current implementation and known gaps describe observed repository
state rather than intended future behavior.

## Milestones

Milestones follow product dependencies rather than directory order.

### M0: Specification and Status Infrastructure

Deliverables:

- all initial package and subsystem specifications;
- project architecture and milestone index;
- deterministic status command and generated report;
- schema, discovery, progress, and report tests.

Completion means one command can detect unregistered packages, invalid specs,
failed checks, stale generated status, and dependency cycles.

### M1: Shared Data Contracts

Owned by:

- `pkg/trackingmodel`;
- `pkg/pluginapi`;
- `pkg/protocol`;
- `internal/parameters`;
- `internal/specparser`;
- `internal/paramgen`;
- `cmd/paramgen`;
- parameter definitions under `spec`.

Completion means tracking frames, parameter IDs, plugin messages, versioning,
and generated artifacts have executable compatibility and round-trip tests.

### M2: Plugin Runtime and IPC

Owned by:

- `pkg/pluginruntime`;
- `internal/ipc`;
- `internal/plugins`.

Completion means a plugin can handshake, receive configuration and tracking
subscriptions, publish selected frame fields, report health, and shut down or
restart safely.

### M3: Host Tracking Merge

Owned by `internal/tracking`.

Completion means frames are validated, stale generations are rejected, sources
are selected by capability and routing configuration, and a stable merged frame
is produced.

### M4: Processing and Parameter Evaluation

Owned by `internal/processing` and planned `internal/evaluator`.

Completion means calibration, tuning, filtering, mutual exclusion, dropout,
parameter dependency closure, and selective evaluation are executable and
tested.

### M5: Avatar Configuration and Requirement Planning

Owned by planned `internal/avatar` and `internal/requirements`.

Completion means `/avatar/change` leads to local VRChat avatar JSON discovery,
input-address parsing, required-parameter compilation, tracking dependency
closure, and generation-tagged plugin subscriptions. OSCQuery remains a dynamic
discovery and validation source rather than a replacement for file config.

### M6: OSC End-to-End Loop

Owned by `internal/osc` and `internal/application`.

Completion means selected plugin data traverses ingestion, merge, processing,
evaluation, compiled avatar bindings, and OSC transport. Avatar changes switch
plans atomically and old-generation data cannot reach VRChat.

The optimized OSC encoder may pass its local acceptance checks before M6 is
complete; milestone completion also depends on M2 through M5.

### M7: Frontend, Operations, and Release

Owned by `frontend`, `build-release`, and application diagnostics.

Completion means users can inspect devices, routing, current avatar, tracking
subscriptions, OSC status, failures, and performance, and the repository can
produce a tested release artifact.

## Check Model

The first version supports only deterministic checks with clear boundaries.

### Command

Runs a registered executable with validated arguments. Initial command IDs are:

- `go-list`;
- `go-test`;
- `go-test-build`;
- `go-test-race`;
- `go-vet`;
- `go-generate-check`;
- `frontend-test`;
- `frontend-build`.

Each ID maps to a fixed executable and fixed leading arguments in Go code.
Specs may supply paths and supported flags but no shell operators, redirection,
environment assignments, or arbitrary executable names.

### File

Checks that a repository-relative file or directory exists with the expected
kind. Paths must resolve inside the repository root.

### Symbol

Checks a repository-relative source file for a required literal or regular
expression. It provides evidence that a named interface, implementation, or
test exists; it does not prove behavior without a paired command check.

### Not Placeholder

Checks critical implementation files for forbidden skeleton forms selected by
the spec, including empty package bodies, `panic("unimplemented")`, empty
required methods, and explicit implementation TODO markers. Generic TODOs are
not globally forbidden because explanatory debt may be legitimate.

### Generated Clean

Copies the repository inputs and generated targets to a temporary workspace,
runs a registered generator there, and compares expected outputs. It must not
mutate the user's working tree.

### Dependency Complete

Passes only when every named dependency has status `complete`. Dependency
cycles are schema errors and prevent report generation.

### Aggregate

Combines named checks or package results for subsystem and end-to-end specs.
Aggregate checks cannot execute commands themselves.

## Progress and State

Each check has a positive integer weight. Package progress is:

```text
sum(weights of passed checks) / sum(weights of all checks)
```

Milestone progress uses the same formula over every check owned by the
milestone. Project progress aggregates all milestone checks. Percentages are
displayed to one decimal place but computed from integer weights.

State precedence is:

```text
blocked > degraded > complete > in_progress > not_started
```

Definitions:

- `not_started`: no check passes and no blocking condition is active;
- `in_progress`: at least one check passes but a required check does not;
- `complete`: every required check passes;
- `blocked`: a required dependency, build prerequisite, or explicit blocker
  prevents useful downstream verification;
- `degraded`: the previous committed status records `complete`, but a current
  required check now fails.

Optional checks affect the displayed percentage but do not prevent `complete`.
A package with no required checks is a schema error.

The generated report records the prior committed state solely to identify
degradation. It never treats a prior percentage as current evidence.

## Status Command

The command interface is:

```text
go run ./cmd/projectstatus
go run ./cmd/projectstatus -write
go run ./cmd/projectstatus -format json
go run ./cmd/projectstatus -check
```

Default mode prints the latest report without changing files. `-write` writes
`docs/project/status.md`. JSON mode emits the same model for CI and future UI
consumers. `-check` fails when the committed Markdown report differs from a
fresh report after excluding volatile generation time.

Each command check has a 120-second default timeout. Identical command IDs and
arguments execute once per run and share results. Failure of one check does not
stop independent checks. The tool distinguishes:

- exit 0: report generated and all required project checks pass;
- exit 1: report generated with failed or blocked required checks;
- exit 2: invalid specification, discovery mismatch, unsafe path, dependency
  cycle, or report-generation failure.

This separation lets the current incomplete repository produce an accurate
report with exit 1 without presenting incompleteness as a tool error.

## Discovery and Coverage

The generator runs `go list ./...` through the registered command layer and
normalizes import paths to repository-relative package paths. Every discovered
package must have exactly one spec with `kind: go-package` and `planned: false`.

Every non-Go subsystem listed in `docs/project/README.md` must have exactly one
subsystem spec. Specs marked `planned: true` are excluded from the discovered
package equality check until their package exists; once discovered, keeping
`planned: true` is a schema error.

Unknown dependencies, duplicate IDs, duplicate paths, unknown milestones, and
dependency cycles are fatal schema errors.

## Report Format

`docs/project/status.md` contains:

- source Git commit;
- generation time and dirty-worktree marker;
- overall state and progress;
- milestone table with dependency state;
- package and subsystem table;
- current blockers;
- failed required checks with concise evidence;
- downstream milestones affected by each blocker;
- the next actionable incomplete check selected by dependency order.

Rows and diagnostics are sorted by milestone, spec ID, and check ID. Paths use
forward slashes in generated output on every platform. Volatile durations are
shown in terminal and JSON output but omitted from committed Markdown to keep
diffs stable.

The JSON representation includes schema version `1`, exact integer weight
totals, check duration, stdout/stderr summaries, and dependency relationships.

## Initial Repository Assessment

The first generated report must derive its result from checks, but specifications
will reflect these observed boundaries:

- `internal/osc` has discovery, OSCQuery, packet parsing, compiled bindings,
  change suppression, bundling, race tests, and zero-allocation benchmarks; it
  is not connected to the tracking/evaluation pipeline;
- `internal/parameters`, `internal/specparser`, `internal/paramgen`, and
  `cmd/paramgen` contain the generated parameter model and generator flow;
- `internal/plugins` contains lifecycle models but currently fails repository
  compilation because `LogEntry` is unresolved;
- `internal/ipc` has no transport implementation;
- `pkg/protocol.Conn`, `pkg/pluginruntime.Main`, and runtime frame delivery are
  incomplete;
- `internal/tracking` exposes routing and service shapes but has no merge
  implementation;
- `internal/processing` defines configuration types but no processing pipeline;
- `internal/application` currently starts only OSC;
- avatar JSON loading, dependency planning, and selective IPC subscriptions do
  not yet have packages;
- frontend and release readiness are measured independently from Go package
  compilation.

Specs must cite concrete files, symbols, and checks for these statements.

## Error Handling and Safety

- Spec paths are cleaned, made absolute against the repository root, and
  rejected if they escape it.
- The runner uses `exec.CommandContext` directly and never invokes a shell.
- Output capture is bounded per stream; truncation is recorded.
- Timeouts terminate the process tree where the platform permits and mark the
  check failed with timeout evidence.
- Generated reports are written atomically through a temporary file in the
  destination directory.
- `-check` is read-only.
- Dirty worktrees are reported but do not automatically fail checks, because
  developers need accurate status during active work.
- Malformed specs, unsafe commands, and internal tool failures never produce a
  partial committed report.

## Testing

`internal/projectstatus` tests cover:

- valid front matter and all required body sections;
- missing fields, duplicate IDs, unknown checks, and invalid weights;
- discovered-package equality and planned-package transitions;
- unknown dependencies and dependency cycles;
- registered command expansion without shell interpretation;
- repository-root path containment;
- timeout, nonzero exit, bounded output, and shared-command caching;
- required versus optional checks;
- every state transition and precedence rule;
- weighted package, milestone, and project progress;
- degraded-state comparison with committed status;
- stable Markdown ordering and path normalization;
- JSON schema version and weight totals;
- atomic writes and stale-report detection;
- Windows command and path behavior.

Integration tests use temporary fixture repositories and small helper processes;
they do not run the full repository test suite repeatedly. One end-to-end test
loads the real specs with command execution replaced by recorded results.

## Delivery Sequence

The documentation system is delivered in five reviewable increments:

1. schema, parser, discovery, and dependency validation;
2. safe check execution and progress calculation;
3. deterministic Markdown and JSON reporting;
4. package and subsystem specifications with initial checks;
5. generated baseline status, README integration, and stale-report command.

Each increment includes focused tests. The final baseline is expected to report
an incomplete or blocked project accurately; M0 completion does not require the
product milestones M1 through M7 to be complete.
