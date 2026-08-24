---
id: internal-tracking
kind: go-package
path: internal/tracking
milestone: M3
depends_on: [pkg-trackingmodel]
checks:
  - id: package-builds
    description: Tracking package builds
    type: command
    command: go-test-build
    args: [./internal/tracking]
    weight: 1
    required: true
  - id: merged-frame-implemented
    description: Merged frame has canonical fields
    type: not_placeholder
    path: internal/tracking/frame.go
    patterns: ['(?s)type MergedFrame struct\s*\{\s*\}']
    weight: 3
    required: true
  - id: service-implemented
    description: Tracking service implementation exists
    type: symbol
    path: internal/tracking/service.go
    pattern: 'type service struct|func NewService'
    weight: 3
    required: true
---
# Package: internal/tracking

## Purpose

Provide the Host-owned, generation-aware boundary for ingesting plugin tracking frames, selecting stable Eye, Expression, and metadata-only Lip sources, and publishing deterministic merged tracking snapshots with per-group Host freshness.

## Responsibilities

- Require every submission to include a non-empty plugin ID and the current positive Host generation.
- Canonicalize each `trackingmodel.TrackingFrame`, then validate its capability/validity/value contract, strict per-plugin Sequence ordering, non-negative timestamps, and non-regressing non-zero time fields before committing it.
- Retain one latest valid canonical frame per plugin for the current generation and reject unset, zero, stale, or future generations.
- Select Eye, Expression, and Lip independently. Automatic selection is sticky and deterministically reselects the lexicographically smallest capable plugin only when the current source becomes unavailable; manual selection has no implicit fallback.
- Merge the selected groups into one immutable value snapshot with Host-owned generation, revision, overall update time, capability/source metadata, and independent `EyeUpdatedAtNS`, `ExpressionUpdatedAtNS`, and `LipUpdatedAtNS` Host receipt freshness. An accepted non-selected source update does not publish an unchanged merged snapshot.
- Publish the latest merged frame and payload-free `Summary` diagnostics through bounded latest-value subscriptions.
- Maintain service-lifetime saturating accepted/rejected counters, fixed rejection-reason counts, and the last rejection.

## Non-responsibilities

- Plugin discovery, authentication, process supervision, session lifecycle, and restart policy.
- Calibration, deadzone/gain, filtering, mutual exclusion, dropout hold/decay, or parameter evaluation.
- Avatar discovery, avatar-plan generation, parameter binding, or OSC/OSCQuery output.
- Application construction and lifecycle wiring.
- Numeric Lip payloads or Expression-to-Lip mapping; Lip is capability/source/freshness metadata only.
- Head tracking data or routing.
- Shared-memory slot layout, synchronization, consistency checks, or notification/transport protocol.

## Current implementation

`NewService` returns a ready-to-use concurrent `Service` with generation unset and Eye, Expression, and Lip routing in automatic mode. `SetGeneration` establishes or advances the positive Host generation. An advance atomically clears current sources and all sticky selections and publishes a valid empty snapshot for the new generation; an equal generation is idempotent, while zero and regression are rejected.

`Submit` validates and canonicalizes the complete frame before changing source or merged state. Ordering baselines are independent per plugin and reset on generation advance. Zero time fields mean absent and do not change their baselines. Eye, Expression, and Lip may each be sourced from different plugins, and advertised capability—not a non-empty validity mask—controls source availability. The service records its own receipt time on every accepted source frame and projects that time independently into each selected group's freshness field; plugin timestamps do not define group freshness.

`PluginFrameSink` is the implemented transport-neutral adapter for the no-result plugin sink boundary. It forwards the authenticated plugin ID, generation, and frame value to a `FrameSubmitter`; submission failures remain observable through the Service diagnostics. Production code depends only on the standard library and `pkg/trackingmodel`, not `internal/plugins`.

## Public/internal interfaces

- `Service` owns submission, generation and routing controls, source removal, latest merged reads, and merged/Summary subscriptions.
- `FrameSubmitter`, `PluginFrameSink`, and `NewPluginFrameSink` provide the generation-bearing plugin-ingest adapter boundary.
- `RoutingConfig` and `SourceSelection` configure independent Eye, Expression, and Lip routing. There is no Head field.
- `MergedFrame` is the Host-owned value snapshot containing generation, revision, Host update time, capabilities, Eye/Expression values, selected source IDs, and per-group Host freshness timestamps. Lip has source/activity metadata but no numeric field.
- `Summary`, `Rejection`, `RejectionCounts`, and `RejectionReason` expose payload-free current state and lifetime ingest diagnostics.
- Stable error sentinels are `ErrGenerationUnset`, `ErrGenerationZero`, `ErrGenerationRegression`, `ErrStaleGeneration`, `ErrFutureGeneration`, `ErrInvalidPluginID`, `ErrInvalidFrame`, `ErrSequenceNotIncreasing`, `ErrTimestampRegression`, `ErrSourceClockRegression`, and `ErrInvalidRouting`.

## Owned data

- One current positive generation, or the initial unset state.
- One latest valid canonical frame and ordering baseline per plugin in that generation.
- Routing configuration and sticky automatic Eye/Expression/Lip selections.
- The latest immutable merged value snapshot and Host-owned service-lifetime revision/time state.
- Service-lifetime accepted/rejected counters, rejection-reason counts, and last rejection.
- Merged-frame and Summary subscriber registries.

## Dependencies

Production code consumes `pkg/trackingmodel` frame, capability, Eye, and Expression value types. It intentionally has no production dependency on `internal/plugins`.

## Concurrency and lifecycle

All public operations participate in one mutex-protected, linearizable state machine. Validation, state mutation, deterministic reselection, merge recomputation, and nonblocking publication form one synchronous operation; there is no background ingest loop and no Service `Start`/`Close` lifecycle.

Generation advance atomically discards previous-generation sources and selections before exposing the new empty snapshot. Merged and Summary subscribers use capacity-one channels with nonblocking latest-value replacement, so slow consumers cannot block producers or create a backlog. A subscriber context owns its lifetime; removal and channel close are serialized with sends by the same mutex to prevent send/close races. Summary subscribers immediately receive the current Summary, while merged subscribers receive an immediate value only after a generation has been set.

## Error handling

Callers can classify stable sentinel failures with `errors.Is`. Every rejected `Submit` updates exactly one rejection category, the rejected total, the last rejection, and Summary publication, but never mutates source or merged state. Accepted submissions increment the accepted total only after the canonical source value is committed.

Control-method errors are returned without being counted as rejected frames. Invalid controls, equal generation/routing updates, and removal of an unknown or empty source are non-publishing. Manual selection of an unavailable plugin is valid routing configuration and exposes that group as unavailable rather than falling back automatically.

## Performance constraints

An ordinary submission from a retained selected source performs O(1) map work and fixed-size value copies. Automatic reselection scans current plugins in O(P) without sorting. The package retains no frame history, ingest backlog, or unbounded queue; it keeps one latest frame per plugin and capacity-one buffers per subscriber.

## Security boundaries

The upstream authenticated session supplies the plugin ID; this package validates that the ID is non-empty but does not authenticate it. Plugin identity and generation accompany every submitted frame and are checked before state mutation. The Service stores canonical value copies only and retains no caller pointer, slice, transport buffer, or shared-memory view.

## Required tests

The package tests prove generation unset/advance/regression/idempotence and atomic clearing; frame canonicalization, ownership, per-plugin Sequence and time ordering; stale/future rejection; split Eye/Expression/Lip merging and independent Host freshness; sticky deterministic automatic routing; manual no-fallback behavior; metadata-only Lip behavior; capability-based dropout behavior; source removal and revision rules; saturating diagnostics; immediate capacity-one latest-value subscriptions; nonblocking producers; cancellation closure; exact sink forwarding and structural plugin-sink compatibility. Concurrent control, submission, read, and subscription activity is exercised under the race detector.

## Known gaps

- M6 Application construction now wires the Service and `PluginFrameSink` and removes sources when plugin lifecycle state is lost; tracking retains ingest, generation, routing, and merge ownership.
- A future optional shared-memory transport must define its own slot, commit-sequence, synchronization, torn-read protection, and notification protocol before copying a stable frame value into this boundary.
- M5 avatar planning/binding is implemented by `internal/avatar`, and M6 Application installation plus evaluator-to-OSC composition are complete outside this package. M7 persistence/UI and OSC networking remain outside this package; numeric Lip payload and Expression-to-Lip mapping remain deferred to later product decisions.

## Completion definition

This package is complete when its normal and race tests demonstrate that concurrent generation controls and plugin submissions produce deterministic, canonical, generation-correct merged value snapshots; rejected frames cannot alter source or merged state; routing and source removal preserve the documented sticky/manual semantics; diagnostics remain exact and saturating; bounded subscribers cannot block ingest or race with cancellation; and the adapter preserves plugin ID, generation, and owned frame values without reversing the production dependency direction.
