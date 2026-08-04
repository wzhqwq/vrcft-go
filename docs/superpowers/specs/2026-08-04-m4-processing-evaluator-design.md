# M4 Processing and Selective Parameter Evaluation Design

## Context

M3 ends at a generation-aware, deterministically routed `tracking.MergedFrame`.
M4 must turn that value snapshot into stable canonical channels and evaluate
only the VRCFT parameters required by the active avatar plan.

The repository already contains:

- the M3 ingest, routing, and merge state machine in `internal/tracking`;
- canonical Eye and Expression value types in `pkg/trackingmodel`;
- generated parameter identities and ranges in `internal/parameters`;
- complete primitive dependency closure and five fixed operations in
  `internal/parameterdeps`;
- configuration-shaped placeholders in `internal/processing`; and
- the structural `osc.ValueSource` read contract.

It does not contain an executable processing pipeline or numeric evaluator.
The current capability model also cannot represent lip-tracking activity
independently from expression tracking.

## Goals

M4 will provide:

1. an independent, metadata-only Lip capability carried through the existing
   plugin, IPC, and M3 boundaries;
2. a deterministic, caller-driven processing pipeline for calibration, tuning,
   filtering, dropout, and configurable mutual exclusion;
3. a compiled evaluator that computes only requested parameter outputs and
   their dependency closure; and
4. package specifications, tests, race evidence, and generated project status
   that accurately establish M4 completion.

The design deliberately favors fixed catalogs and fixed operations over a
generic processing graph.

## Non-goals

M4 does not implement:

- Application construction, subscription loops, tick scheduling, or shutdown;
- avatar JSON loading, avatar-plan compilation, or atomic avatar switching;
- OSC transport wiring or network output;
- configuration persistence, migration, or frontend editing;
- a dynamic expression language, user-defined evaluator operations, or
  plugin-defined processing nodes;
- a numeric Lip payload or a mapping from any `ExpressionID` to Lip; or
- shared-memory layout, synchronization, or notification protocols.

Those responsibilities remain in M5, M6, frontend work, or a future transport
cycle.

## Architecture

```text
Plugin/IPC TrackingFrame
          |
          v
internal/tracking
MergedFrame + Eye/Expression/Lip source and freshness metadata
          |
          | ProcessAt(frame, hostTime)
          v
internal/processing
CanonicalFrame
          |
          | Plan.Evaluate(frame)
          v
internal/evaluator
immutable parameter Snapshot
          |
          | Float(id) / Bool(id)
          v
internal/osc ValueSource contract (wired in M6)
```

The dependency direction is strictly downstream:

```text
pkg/trackingmodel
      ^
      |
internal/tracking <- internal/processing <- internal/evaluator
                                            |       |
                                            v       v
                               internal/parameterdeps
                               internal/parameters
```

`internal/evaluator` does not import `internal/osc`. Its snapshot satisfies the
OSC source contract structurally.

## Lip Capability Compatibility Extension

### Stable bit assignment

`trackingmodel.Capability` gains a third stable flag:

```text
CapabilityEye        = 1
CapabilityExpression = 2
CapabilityLip        = 4
```

The existing bit values do not change. The project has not been released, so
all in-repository endpoints can move atomically without a legacy branch or wire
version increase.

### Tracking model semantics

A `TrackingFrame` may declare Lip alone or alongside other capabilities. Lip is
metadata-only: it has no value structure, validity mask, or relationship to
`ExpressionSet`. Validation and canonicalization accept and retain the Lip bit
without making any Eye or Expression value valid.

### Plugin and protocol propagation

The known-capability masks used by plugin descriptors and subscriptions include
Lip. A positive-generation Lip-only subscription is valid. Subscription frame
trimming preserves Lip only when both the subscription and submitted frame
contain it; trimming never copies or infers Expression values because of Lip.

The existing numeric capability field in protocol messages carries the new
bit. No message or payload shape changes. Descriptor, manifest, handshake,
subscription, runtime publication, and protocol round-trip checks cover Lip-only
and mixed capability cases.

### M3 routing and freshness extension

`tracking.RoutingConfig` gains an independent `Lip SourceSelection`. It follows
the existing rules exactly:

- Auto is sticky while the current source remains Lip-capable.
- Auto reselection chooses the lexicographically smallest capable plugin.
- Manual selection has no fallback.
- removal or capability loss makes the source unavailable and reselects only
  according to the configured Lip rule.

`MergedFrame` and `Summary` gain Lip source and availability metadata. No Lip
numeric payload is added. Lip source changes do not clear or switch the selected
Expression source.

M3 also exposes Host receive freshness for every independently selected group:

- `EyeUpdatedAtNS`;
- `ExpressionUpdatedAtNS`; and
- `LipUpdatedAtNS`.

These values come from the selected source state's accepted Host receive time.
They are zero when the group is unavailable. Every accepted update from a
selected source produces an observable merged snapshot even if its numeric
value is unchanged, because downstream stale detection must distinguish a
fresh repeated measurement from a stopped source. Lip-only selected updates
have the same rule.

## Processing Package

### Public shape

`internal/processing` provides, conceptually:

```go
func DefaultConfig() Config
func NewPipeline(Config) (*Pipeline, error)
func (*Pipeline) ProcessAt(tracking.MergedFrame, int64) (CanonicalFrame, error)
```

The `int64` argument is Host nanoseconds in the same clock domain as M3 receive
timestamps. The Pipeline is a sequential state machine. It starts no goroutine,
retains no caller-owned reference, and is not safe for concurrent calls.

Configuration is validated and compiled at construction. It is immutable after
construction. A configuration change creates a new Pipeline and intentionally
resets runtime history.

### Canonical output

`CanonicalFrame` is a value snapshot containing:

- generation and M3 merged revision;
- processing time;
- Eye, Expression, and Lip source IDs;
- independent Eye, Expression, and Lip active flags;
- processed `trackingmodel.EyeSample`; and
- processed `trackingmodel.ExpressionSet`.

It contains no map, slice, pointer, transport view, or history. Eye and
Expression validity describe numeric output availability. Active flags describe
upstream freshness and are independent from held or decaying numeric validity.

### Channel catalog and configuration

A stable `ChannelID` catalog names every scalar Eye field and every
`trackingmodel.ExpressionID`. Gaze X and Y are separately configurable even
though the tracking model groups their validity bit. Lip has no numeric channel.

`Config` contains:

- one fully resolved default `ChannelConfig`;
- sparse `ChannelID -> ChannelConfig` replacements; and
- one positive `ActiveStaleAfter` duration shared by Eye, Expression, and Lip
  activity detection; and
- zero or more mutual-exclusion groups.

An override replaces the complete channel configuration. Callers may copy the
default channel configuration and alter selected fields. This avoids a second
layer of optional-field or pointer semantics. Construction copies all maps and
slices before retaining the compiled configuration.

`ChannelConfig` contains calibration, tuning, filter, and dropout policy.

`DefaultConfig()` is intentionally conservative:

- calibration disabled, so the raw finite value passes through;
- deadzone `0`, tuning gain `1`, exponent `1`, and clamp disabled;
- filter mode `FilterNone`;
- no mutual-exclusion groups; and
- active stale after `500ms`; and
- stale after `500ms`, hold for `100ms`, then decay for `300ms`.

The misspelled placeholder file `tunning.go` is renamed to `tuning.go`. No
compatibility file is retained because there is no consumer and no released API.

### Calibration

When disabled, calibration is the identity operation. When enabled, the raw
value is first clamped to `[Min, Max]`, then mapped piecewise:

```text
[Min, Neutral] -> [-1, 0]
[Neutral, Max] -> [0, 1]
```

`Min == Neutral` or `Neutral == Max` is allowed for a one-sided channel. All
three values may not be equal. The normalized value is then inverted when
configured and multiplied by the positive calibration gain.

### Tuning

Tuning runs after calibration:

1. A deadzone in `[0,1)` maps magnitudes at or below the threshold to zero and
   continuously rescales the remaining magnitude by `(magnitude-d)/(1-d)`.
2. A positive tuning gain is applied.
3. A positive exponent is applied to the magnitude while preserving sign.
4. An optional clamp applies only when explicitly enabled and requires
   `ClampMin < ClampMax`.

Every intermediate result must remain finite.

### Filters

The fixed filter modes are `none`, `ema`, and `one_euro`.

- EMA requires `Alpha` in `(0,1]`.
- One Euro requires `MinCutoff > 0`, `Beta >= 0`, and
  `DerivativeCutoff > 0`.
- One Euro uses the standard time-dependent low-pass coefficient, filters the
  derivative, then uses `MinCutoff + Beta*abs(filteredDerivative)` for the value
  cutoff.

A filter initializes from the first value after construction, generation reset,
or affected-group source change. It updates only for a new merged input and a
positive time delta. Repeating the same snapshot never double-ingests a sample.

### Input ordering and reset rules

`ProcessAt` validates the complete call before mutating runtime state:

- generation and merged revision must be positive and non-regressing;
- Host times and group freshness times must be non-negative and not later than
  the supplied processing time;
- a repeated unsaturated merged revision must represent the same input value;
- effective time must not regress; and
- every value marked valid must be finite and consistent with capability.

M3's saturating revision is handled explicitly: at the maximum revision, a
new value snapshot with non-regressing receive metadata is still accepted as a
new input. An identical value snapshot is a repeated sample.

A higher generation atomically resets all filter and dropout history before its
first sample. Within one generation, changing a group source ID resets only that
group. The new source's first value initializes its filters; values from two
devices are never blended. Lip source changes affect only Lip active state.

### Freshness, dropout, and active state

Freshness is tracked per numeric channel from the selected group's Host receive
time. A valid channel sample updates its last-valid value and time. Repeating a
merged snapshot only advances the caller-provided time.

Eye, Expression, or Lip active becomes false immediately when its capability or
source disappears. A still-selected source becomes inactive when its group
receive age exceeds the Config's `ActiveStaleAfter`. `LipActive` is never
inferred from Expression validity. This group-level duration is separate from
numeric channel dropout policy because Lip intentionally has no numeric
ChannelConfig.

For a previously valid numeric channel:

1. it remains at the last filtered value during the hold interval;
2. it decays linearly to processed neutral `0` during the decay interval; and
3. it remains valid at neutral until a fresh value returns.

A channel that has never received a valid sample remains invalid. Keeping the
final neutral value valid lets OSC send one deterministic reset; subsequent
change suppression prevents ongoing traffic. Capability loss begins dropout
immediately. A source that merely stops updating begins dropout at its
last-valid time plus `StaleAfter`, so a late caller tick advances directly to the
correct timeline point.

### Mutual exclusion

Mutual exclusion is opt-in. Every group contains two or more unique numeric
ChannelIDs. A channel may not occur twice or in more than one group. Unknown
channels and duplicate membership are configuration errors.

After calibration, tuning, filtering, and dropout have produced the frame's
candidate values, each group retains the valid channel with the largest absolute
value and writes zero to the other valid members. An absolute-value tie is won
by the smallest stable ChannelID. Mutual exclusion is an output projection and
does not overwrite each channel's independent filter or dropout history.

Applying the projection after dropout guarantees exclusion during hold and
decay. Applying dropout after exclusion could reintroduce a held old winner next
to a fresh new winner.

### Processing errors and atomicity

Stable `errors.Is`-compatible sentinels distinguish invalid configuration,
unknown channel, invalid input, generation/revision regression, revision
conflict, and time regression. Detailed errors wrap the sentinel with field or
channel context.

Construction has no partial output. A failed `ProcessAt` call leaves generation,
source identities, filter state, dropout timers, and the last output unchanged.

## Selective Evaluator Package

### Public shape

`internal/evaluator` provides, conceptually:

```go
func Compile([]parameters.ParameterID) (*Plan, error)
func (*Plan) Evaluate(processing.CanonicalFrame) Snapshot

func (Snapshot) Float(parameters.ParameterID) (float32, bool)
func (Snapshot) Bool(parameters.ParameterID) (bool, bool)
```

`Plan` is immutable after compilation and safe for concurrent evaluation.
`Snapshot` is a self-owned value. Neither retains the caller's requested-ID
slice or any frame reference.

### Compilation

Compilation validates every requested ID, deduplicates repeated requests, and
uses `internal/parameterdeps` to resolve the complete dependency closure. It
builds one deterministic topological order. Unknown IDs, missing plans, cycles,
or an unsupported/inconsistent operation fail compilation with stable wrapped
errors.

The evaluator supports exactly the five existing operations:

- `Direct`: read one Eye, Expression, or Active leaf;
- `Average`: arithmetic mean of every dependency;
- `Max`: maximum of every dependency;
- `SignedPair`: take the maximum of every dependency except the last and
  subtract the last negative dependency; with two dependencies this is simply
  positive minus negative; and
- `SumClamp`: sum all dependencies and clamp to the target parameter range.

The dependency order used by `SignedPair` is part of the fixed
`internal/parameterdeps` operation contract and is covered for every such plan.
No runtime expression parsing or dynamic operation registration is introduced.

### Evaluation and validity

Evaluation uses fixed-size temporary storage indexed by `ParameterID`; it does
not build a per-frame map. Direct Eye leaves read canonical Eye fields, direct
Expression leaves use the fixed name/ID mapping, and Active leaves read the
three explicit canonical active flags.

Every dependency must be valid. If any direct or derived dependency is invalid,
the dependent result is invalid; values are never fabricated from one side or
by substituting zero. Non-finite arithmetic also makes that result invalid.

Final floats are clamped to their generated `parameters.Definition` range.
Only explicitly requested parameters are exposed as valid in the returned
snapshot. Values computed solely as internal dependencies remain inaccessible.

`LipTrackingActive` reads only `CanonicalFrame.LipActive`. It is not equalized
with Expression active and is not inferred from mouth, jaw, or tongue values.

### Snapshot representation

Snapshot storage is dense and fixed-size:

- `[parameters.ParameterCount]float32`;
- `[parameters.ParameterCount]bool`; and
- fixed validity bitsets for float and Boolean results.

`Float` and `Bool` validate both ID and generated parameter type. This shape
structurally satisfies `osc.ValueSource` without an evaluator-to-OSC import.

## Complexity and Performance

The design keeps bounds explicit:

- M3 ordinary retained-source submission remains O(1); capability reselection
  is O(P) over current plugins.
- Processing is O(C + G), where C is the fixed numeric channel count and G is
  total configured mutual-exclusion membership.
- Evaluator compilation is O(R + D) for requested parameters and their fixed
  dependency edges.
- Evaluation is O(D) for the compiled closure, not O(all parameters), and uses
  fixed-size value storage.
- No stage retains frame history, creates an unbounded queue, or starts a
  background worker.

## Testing Strategy

### Lip compatibility

Tests cover:

- tracking-model validation and canonicalization for Lip-only and mixed frames;
- descriptor, subscription, and trimming behavior;
- protocol round-trip and exact numeric capability preservation;
- runtime publication, manifest, and handshake compatibility;
- independent M3 Lip Auto/Manual/sticky selection, removal, capability loss,
  and generation reset;
- group-specific freshness metadata; and
- normal and race regression for the existing Eye/Expression behavior.

No test associates Lip with an Expression value.

### Processing

Table and sequence tests cover:

- every calibration boundary and one-sided calibration;
- continuous deadzone, gain, signed exponent, and optional clamp;
- hand-calculated EMA and One Euro timelines;
- invalid configuration and mutation-free failed calls;
- generation reset, source-specific reset, repeated revision, saturated
  revision, and time discontinuity;
- exact stale, hold, decay, recovery, and final-neutral boundaries;
- immediate Active changes independent from numeric dropout;
- mutual-exclusion winner, stable tie, invalid members, and exclusion during
  dropout; and
- complete, one-to-one ChannelID coverage of Eye scalars and ExpressionIDs.

Tests use injected integer nanoseconds and no sleeps.

### Evaluator

Tests cover:

- hand-calculated results for all five operations;
- strict dependency validity;
- generated range clamping and non-finite containment;
- requested-ID deduplication and invalid compile inputs;
- hidden internal dependencies;
- independent Eye, Expression, and Lip active values;
- compilation coverage for all 127 YAML/generated parameters;
- structural compatibility with OSC `ValueSource`; and
- concurrent use of one immutable Plan under the race detector.

### Cross-package proof

A bounded integration fixture passes a generation-tagged merged snapshot through
Pipeline and a compiled evaluator plan, then reads the result through the OSC
source interface. It does not construct Application, start a ticker, or send a
network packet.

## Documentation and Completion Evidence

M4 adds an authoritative `internal/evaluator` package specification and updates
`internal-processing` to depend on `internal-tracking`. Specifications for the
capability-bearing packages and M3 are amended only where the Lip/freshness
extension changes their current contract.

M4 is complete only when:

1. capability extension, processing, and evaluator tests pass normally and
   under the race detector where concurrency is supported;
2. full repository tests, relevant `go vet`, formatting, and repository checks
   pass from a clean source commit;
3. an independent whole-range review has no open Critical or Important finding;
4. package documents describe the implemented boundaries accurately; and
5. `projectstatus` generated from a clean commit reports `internal-processing`,
   `internal-evaluator`, and M4 complete while retaining unrelated M5, M6,
   frontend, or release blockers.

Passing a placeholder-symbol check alone is not sufficient evidence.
