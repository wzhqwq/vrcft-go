# Internal Tracking Merge Design

Date: 2026-08-02  
Status: approved design, pending implementation plan

## Goal

Complete milestone M3 by implementing `internal/tracking` as the Host-owned
tracking ingest, validation, source-selection, and merge boundary.

The package accepts generation-tagged frames from authenticated plugin
sessions, rejects frames that do not belong to the Host's current generation,
selects stable sources for each supported tracking group, and publishes a
deterministic merged value snapshot for downstream processing.

## Scope

M3 implements:

- a concrete concurrent tracking Service;
- Host-controlled global generation changes;
- generation-bearing plugin-frame ingestion;
- frame validation and canonicalization;
- per-plugin sequence and timestamp ordering;
- Eye and Expression routing;
- deterministic sticky automatic source selection;
- manual source selection without implicit fallback;
- stable merged-frame snapshots;
- bounded latest-value merged and diagnostic subscriptions;
- a transport-neutral adapter compatible with `plugins.FrameSink`;
- tests for correctness, ownership, concurrency, and backpressure.

M3 does not implement:

- Head routing or data, because `pkg/trackingmodel` has no Head capability or
  sample model;
- calibration, gain, filtering, mutual exclusion, dropout hold/decay, or
  parameter evaluation;
- avatar-file discovery or generation planning;
- Application lifecycle wiring;
- automatic translation of plugin lifecycle events into source removal;
- shared-memory slot layout, synchronization, or notification transport.

`RoutingConfig.Head` is removed. No current production code depends on the
obsolete field.

## Package Boundary

`internal/tracking` depends only on `pkg/trackingmodel` in production. It must
not import `internal/plugins`.

The package owns:

- current tracking generation;
- latest accepted frame and ordering baselines for each plugin;
- routing configuration and sticky automatic selections;
- latest immutable merged snapshot;
- service-lifetime ingest diagnostics;
- merged-frame and summary subscriber registries.

It does not own plugin identity authentication. The upstream plugin Manager
adds the authenticated plugin ID before invoking the frame-sink boundary.

## Architecture

The implementation uses one mutex-protected state machine. Public mutations
are synchronous and linearizable. Validation, state mutation, source
selection, merge recomputation, and nonblocking publication occur as one
logical operation.

The implementation is divided by responsibility:

- `service.go` owns the mutex, public control methods, and mutation ordering;
- `source.go` owns per-plugin current-generation state and ordering checks;
- `routing.go` validates routing and performs sticky deterministic selection;
- `frame.go` defines and constructs `MergedFrame`;
- `subscription.go` owns bounded latest-value publication;
- focused test files mirror these boundaries.

The core service has no background ingest loop and requires no Start or Close
lifecycle. A subscriber's context owns that subscription's lifetime.

## Public Types and Service Shape

The following identifiers and method signatures are normative:

```go
type SourceSelection struct {
	Auto     bool
	PluginID string
}

type RoutingConfig struct {
	Eye        SourceSelection
	Expression SourceSelection
}

type Service interface {
	Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) error
	SetGeneration(generation uint64) error
	Generation() uint64

	SetRouting(config RoutingConfig) error
	Routing() RoutingConfig
	RemoveSource(pluginID string)

	LatestMerged() (MergedFrame, bool)
	SubscribeMerged(ctx context.Context) <-chan MergedFrame
	SubscribeSummary(ctx context.Context) <-chan Summary
}
```

`NewService` returns a ready-to-use Service with both groups in Auto mode and
with generation unset.

`PluginFrameSink` is a thin value-owning wrapper whose no-result `Submit`
method calls the Service method and intentionally leaves diagnostics in the
Service Summary. Its method set structurally satisfies `plugins.FrameSink`.
An external test package may import both packages to assert compatibility,
without adding a production dependency.

## Generation Semantics

Generation is a positive, Host-controlled, globally current subscription or
avatar-plan generation.

`SetGeneration` behaves as follows:

- zero is invalid;
- a lower value is a generation regression error;
- the current value is idempotent and does not clear state or publish a new
  snapshot;
- a higher value atomically clears all source frames, ordering baselines, and
  sticky source choices;
- a higher value creates and publishes an empty but valid merged snapshot for
  the new generation.

Before the first successful `SetGeneration`, `LatestMerged` returns false and
frame submissions are rejected. Afterwards it returns true, including while
the current generation has no available source data.

`Submit` accepts only a generation exactly equal to the current generation.
This deliberately prevents old avatar-plan data from surviving between a Host
generation change and the first new-generation frame.

## Frame Ownership and Validation

`Submit` receives a complete frame value. The Service retains only owned value
copies and never retains a caller pointer, slice, transport buffer, or shared
memory view.

Validation occurs before any source or merged state changes:

1. plugin ID must be non-empty;
2. current generation must be set and match the submitted generation;
3. `TrackingFrame.Canonicalize` must succeed;
4. Sequence must be strictly greater than the last accepted Sequence for this
   plugin in the current generation;
5. `TimestampNS` and `SourceClockNS` must be non-negative;
6. each non-zero timestamp must not regress relative to the last accepted
   non-zero value for this plugin and generation.

Zero timestamps mean "not provided" and do not establish or update a timestamp
baseline. Sequence zero is permitted as the first accepted sequence, but a
second zero is rejected because the sequence must strictly increase.
Negative timestamps return an error matching `ErrInvalidFrame`; timestamp
regression sentinels apply only to otherwise valid non-negative values.

Ordering is never compared across plugins. A generation advance resets all
per-plugin ordering baselines.

Invalid submissions update only rejection diagnostics. They do not partially
mutate source state, selections, or the merged snapshot.

## Routing Validation

Both Eye and Expression selections follow the same validation rules:

- Auto requires an empty PluginID;
- manual selection requires a non-empty PluginID;
- an identical RoutingConfig update is idempotent;
- a manual selection for a currently unknown or unavailable plugin is valid
  configuration and produces an unavailable group;
- manual routing never falls back to Auto.

A routing change immediately recomputes from cached current-generation frames.
It does not wait for another frame.

## Automatic Source Selection

Automatic selection is stable and deterministic:

1. retain the current automatic source while its latest frame still advertises
   the group's capability;
2. if it is removed or no longer advertises that capability, scan all current
   candidates and select the lexicographically smallest plugin ID;
3. retain the replacement until it also becomes unavailable.

The scan is O(P) and does not need sorting.

Capability availability is distinct from field validity. A source whose frame
still advertises Eye or Expression capability remains selected even if its
current validity mask is empty. An empty validity mask is dropout data for M4,
not a reason for M3 to switch sources.

`RemoveSource` is idempotent. Removing a selected source triggers immediate
automatic reselection or makes a manual route unavailable. Removing an unknown
source, including an empty ID, changes nothing.

## Merged Frame

`MergedFrame` contains Host-owned merge metadata and group values:

```go
type MergedFrame struct {
	Generation  uint64
	Sequence    uint64
	UpdatedAtNS int64

	Capabilities trackingmodel.Capability
	Eye          trackingmodel.EyeSample
	Expressions  trackingmodel.ExpressionSet

	EyeSourceID        string
	ExpressionSourceID string
}
```

Eye and Expression may come from different plugins. A group's capability and
sample are copied only from its selected, available source. An unavailable
group has a zero source ID, no capability bit, and zero sample data.

The merged Sequence is a Host-owned service-lifetime revision. It advances
when the externally observable merged result changes because of:

- a generation advance;
- a routing change;
- selected-source removal or automatic reselection;
- an accepted frame from a selected source.

An accepted frame from a non-selected source does not advance the merged
Sequence unless it causes selection to change. Equal control updates and
idempotent removals do not publish.

`UpdatedAtNS` uses Host time and is clamped to be non-decreasing. Tests use an
internal injectable clock rather than sleeps.

## Diagnostics

Failures use these stable sentinel errors discoverable with `errors.Is`:

```go
var (
	ErrGenerationUnset       error
	ErrGenerationZero        error
	ErrGenerationRegression  error
	ErrStaleGeneration       error
	ErrFutureGeneration      error
	ErrInvalidPluginID       error
	ErrInvalidFrame          error
	ErrSequenceNotIncreasing error
	ErrTimestampRegression   error
	ErrSourceClockRegression error
	ErrInvalidRouting        error
)
```

`ErrGenerationRegression` applies to `SetGeneration`. A submitted generation
below the current generation returns `ErrStaleGeneration`; a submitted
generation above it returns `ErrFutureGeneration`.

Errors never include a full frame or expression values.

Diagnostics use fixed value types rather than maps:

```go
type RejectionReason uint8

const (
	RejectionNone RejectionReason = iota
	RejectionGenerationUnset
	RejectionGenerationZero
	RejectionStaleGeneration
	RejectionFutureGeneration
	RejectionInvalidPluginID
	RejectionInvalidFrame
	RejectionSequenceNotIncreasing
	RejectionTimestampRegression
	RejectionSourceClockRegression
)

type RejectionCounts struct {
	GenerationUnset       uint64
	GenerationZero        uint64
	StaleGeneration       uint64
	FutureGeneration      uint64
	InvalidPluginID       uint64
	InvalidFrame          uint64
	SequenceNotIncreasing uint64
	TimestampRegression   uint64
	SourceClockRegression uint64
}

type Rejection struct {
	PluginID   string
	Generation uint64
	Reason     RejectionReason
}

type Summary struct {
	Generation uint64
	Routing    RoutingConfig
	SourceCount int

	EyeSourceID        string
	EyeAvailable       bool
	ExpressionSourceID string
	ExpressionAvailable bool

	AcceptedFrames uint64
	RejectedFrames uint64
	Rejected       RejectionCounts
	LastRejection  Rejection
}
```

`SourceCount` counts plugins with one accepted frame in the current
generation. Availability means that the selected source's latest frame still
advertises the corresponding capability; it does not require a non-empty
validity mask.

Counters cover the Service lifetime and use saturating uint64 addition.
Generation changes reset data state but not diagnostics. Control-method errors
are returned to their caller and are not counted as rejected frames.

Summary contains no TrackingFrame or expression payload.

## Subscription Semantics

Merged and Summary subscriptions use capacity-one channels with latest-value
replacement:

- producers never block on a subscriber;
- if a subscriber has not consumed its old value, a newer value replaces it;
- Summary subscribers immediately receive the current Summary;
- Merged subscribers immediately receive the latest snapshot only when a
  generation has been set;
- cancellation removes and closes the channel while holding the same mutex
  used by publication, preventing send/close races.

The replacement operation uses only nonblocking channel operations; it must
not assume that a channel observed full remains full while a subscriber is
receiving.

## Data Flow

The normal frame path is:

```text
authenticated plugin session
    -> generation-bearing plugins.FrameSink
    -> tracking.PluginFrameSink
    -> Service.Submit(pluginID, generation, frame value)
    -> validate and canonicalize
    -> update per-plugin latest state
    -> preserve or reselect Eye / Expression sources
    -> recompute immutable MergedFrame when observable output changes
    -> publish latest merged snapshot and Summary without blocking
```

Application construction and Manager lifecycle event wiring remain M6 work.
M3 supplies the tested adapter and `RemoveSource` control required for that
later wiring.

## Future Shared-Memory Compatibility

Shared-memory double or triple buffering may replace the payload transport
without changing this package. The future Host transport adapter must copy a
stable committed slot into a complete TrackingFrame value before calling
`Submit`.

The shared-memory layer is responsible for:

- writing an inactive slot before publishing its index or commit sequence with
  release semantics;
- reading publication state with acquire semantics;
- validating the commit sequence before and after copying to detect torn
  reads;
- committing generation and frame Sequence as one logical record;
- treating a notification as "new data may exist," not as a strict one-event
  per-frame contract.

Sequence gaps are valid, so coalesced notifications and reading only the newest
committed slot are compatible with M3. Duplicate or older committed sequences
remain rejected. Authenticated plugin identity continues to come from the IPC
session rather than from self-reported shared-memory data.

Double buffering can be overwritten while the Host copies if the writer laps
the reader. Triple buffering or a commit-sequence/seqlock protocol is therefore
the safer future implementation, but this choice belongs to IPC transport.

## Concurrency and Complexity

All public methods are safe for concurrent use. The mutex covers only bounded
validation, fixed-size value copies, map mutation, O(P) source reselection, and
nonblocking channel operations.

- ordinary accepted Submit is O(1);
- reselection is O(P);
- each plugin consumes one fixed-size latest-frame record;
- no frame history or unbounded ingest queue exists;
- subscriber channels have capacity one;
- subscriber count is controlled by caller-owned contexts.

## Test Strategy

Implementation follows strict test-driven development. Required tests include:

- generation unset, zero, regression, idempotence, advance, atomic clearing,
  and stale-frame rejection;
- canonicalization and rejection of malformed capability, validity, and
  non-finite values;
- Sequence duplicate/regression and timestamp regression;
- ordering reset after generation advance;
- split Eye/Expression merging from different plugins;
- deterministic initial Auto selection and sticky retention;
- selected-source removal and deterministic reselection;
- dropout validity masks not triggering source changes;
- manual unavailable routing without fallback;
- routing recomputation from cached frames;
- non-selected submissions not advancing merged Sequence;
- Summary reason counters, last rejection, and saturation;
- immediate subscription snapshots, latest-value replacement, cancellation
  closure, and nonblocking producers;
- value ownership and absence of retained transport buffers;
- concurrent Submit, SetGeneration, SetRouting, RemoveSource, reads, and
  subscription cancellation under the race detector;
- compile-time `PluginFrameSink` compatibility with `plugins.FrameSink` and
  exact plugin ID/generation/frame forwarding.

## Completion Criteria

M3 is complete when:

- `internal/tracking` package tests and race tests pass;
- stale generations, invalid frames, and non-increasing source data cannot
  alter merged state;
- source selection and merged output are deterministic under concurrent input;
- slow or canceled subscribers cannot block ingest or race with publication;
- the plugin FrameSink adapter preserves authenticated plugin ID, generation,
  and owned frame value;
- package responsibilities and generated project status are updated;
- independent review finds no open Critical or Important issues.
