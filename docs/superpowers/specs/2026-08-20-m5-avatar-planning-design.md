# M5 Avatar Configuration and Requirement Planning Design

## Context

M0 through M4 provide project evidence, stable tracking and plugin contracts,
generation-aware merge, stateful processing, dependency closure, and selective
parameter evaluation. The repository also has an OSC catalog compiler and
sender, but its compiler currently accepts only an OSCQuery tree. No package
resolves VRChat's local avatar configuration, converts its input endpoints into
VRCFT bindings, computes tracking requirements, or produces the generation-
tagged control-plane plan that M6 can apply atomically.

VRChat sends the active avatar ID to `/avatar/change`. Its documented local OSC
configuration lives at
`VRChat/VRChat/OSC/{userId}/Avatars/{avatarId}.json`; each parameter may define
an `input` address and type that external applications use to drive VRChat.
The format and path are documented at
<https://docs.vrchat.com/docs/osc-avatar-parameters>.

M5 turns that event payload and a configured optional fallback file into an
immutable avatar plan. M5 does not run the product lifecycle or atomically
install the plan across runtime components; that composition remains M6.

## Goals

M5 will provide:

1. deterministic discovery of an active avatar's local JSON across VRChat
   user-ID directories;
2. an explicit fallback-file policy for local test avatars that have no
   generated configuration;
3. bounded, forward-compatible decoding and strict validation of known avatar
   configuration fields;
4. compilation of `parameters[].input` endpoints into the existing OSC send
   catalog without duplicating binding semantics;
5. stable requested-parameter extraction, selective evaluator compilation,
   dependency closure, and tracking requirement computation;
6. generation-tagged base and per-plugin subscriptions; and
7. a fail-closed result for every accepted avatar-change attempt so M6 cannot
   accidentally retain stale bindings or requirements.

## Non-goals

M5 does not implement:

- Application event-loop construction, startup, rollback, or shutdown;
- atomic installation into tracking, plugins, evaluator, or OSC;
- frame scheduling, processing, evaluation, or network sending;
- persistence or frontend selection of the fallback path;
- filesystem watching or automatic reload outside an avatar-change attempt;
- VRChat configuration generation or modification;
- OSCQuery discovery as a substitute for the local JSON;
- numeric Lip payloads or Expression-to-Lip mapping; or
- plugin installation, update, signing, or marketplace behavior.

M6 consumes the plan. M7 owns persistence and UI for the configured fallback
path.

## Architecture

```text
/avatar/change avatar ID + configured fallback path
                      |
                      v
            internal/avatar Planner
       discovery -> decode -> endpoint validation
                      |
                      v
      internal/osc endpoint catalog compilation
                      |
                      v
  requested ParameterIDs -> evaluator.Compile
                      |
                      +-> parameterdeps.RequiredInputs
                      |
                      v
       immutable, generation-tagged avatar.Plan
                      |
                      v
      M6 Application atomic installation and wiring
```

`internal/avatar` is a synchronous planner. It starts no goroutine and performs
no runtime mutation outside its own generation counter. A mutex serializes
activation attempts so two callers cannot receive the same generation.

The existing `internal/osc` catalog compiler remains the owner of OSC output
binding semantics. It gains an endpoint-based compilation entry point. Its
existing OSCQuery entry point flattens a query tree to endpoints and delegates
to the same compiler. The avatar planner maps local JSON inputs to endpoints
and calls that entry point directly; it does not create a synthetic OSCQuery
tree.

The evaluator continues to have no dependency on OSC. The avatar package may
depend on both packages because it is the control-plane composition boundary
that deliberately joins a send catalog and an evaluator plan.

## Package and File Responsibilities

The proposed `internal/avatar` package is divided by responsibility:

- `config.go` defines the bounded external JSON shape and decoding rules;
- `discovery.go` validates avatar IDs and resolves regular files under the
  configured OSC root or the explicit fallback path;
- `requirements.go` converts requested parameter IDs and dependency leaves to
  a base plugin subscription and per-plugin projections;
- `plan.go` defines immutable plan, result, status, source, and diagnostic
  values;
- `planner.go` serializes generation allocation and performs the complete
  resolve/compile operation; and
- focused `_test.go` files cover each responsibility and cross-package
  compatibility.

`internal/osc/catalog.go` gains a public endpoint compiler while retaining
output operations, sorting, hashing, conflict checks, and sender-private output
details in the OSC package.

## Public Shape

The package exposes this contract:

```go
package avatar

type PlannerConfig struct {
	OSCRoot      string
	FallbackPath string
}

type Status uint8

const (
	StatusReady Status = iota + 1
	StatusFailed
)

type Source uint8

const (
	SourceAvatarConfig Source = iota + 1
	SourceFallback
	SourceNone
)

type Result struct {
	Plan *Plan
	Err  error
}

func NewPlanner(PlannerConfig) (*Planner, error)
func (*Planner) Activate(avatarID string) Result

func (*Plan) Generation() uint64
func (*Plan) Status() Status
func (*Plan) AvatarID() string
func (*Plan) ConfigID() string
func (*Plan) ConfigPath() string
func (*Plan) Source() Source
func (*Plan) ParameterIDs() []parameters.ParameterID
func (*Plan) Catalog() *osc.Catalog
func (*Plan) Evaluator() *evaluator.Plan
func (*Plan) RequiredInputs() parameterdeps.Inputs
func (*Plan) SubscriptionFor(trackingmodel.Capability) (pluginapi.Subscription, bool)
```

All slices and maps returned to callers are copies or are reached through
already immutable APIs. A caller cannot mutate a plan by retaining constructor
inputs or modifying an accessor result. `Evaluator` is immutable by its M4
contract. `Catalog` must be returned through an owned clone with the same
deep-copy guarantees as `osc.Controller.Catalog`, or the catalog type must gain
an equivalent clone operation used by both callers.

`Result.Plan` is non-nil for every activation attempt that successfully
allocates a generation, including discovery, read, decode, and compile
failures. `Result.Err` is nil only for a ready plan. This shape makes the safe
operation explicit: M6 installs `Result.Plan` first, then reports `Result.Err`.
It must not return early on a non-nil configuration error.

Generation exhaustion is the sole activation failure that cannot produce a
new plan. It returns a nil plan and an `ErrGenerationExhausted`-wrapping error;
the counter is never wrapped or reused.

## Generation and Plan States

The planner begins before generation one. Every call to `Activate`, including
a repeated avatar ID and every fail-closed attempt, reserves the next positive
generation before filesystem work. Repeated IDs are not coalesced: a repeated
avatar-change can intentionally pick up a modified configuration and must
reset downstream history when M6 applies the plan.

A ready plan may contain zero recognized VRCFT bindings. This is not an error:
the avatar configuration may legitimately contain no VRCFT parameters. Such a
plan has an empty catalog/evaluator request and no per-plugin subscription, but
retains ready status and source diagnostics.

A failed plan contains:

- the new generation;
- the incoming avatar ID;
- the selected path and source when selection progressed that far;
- failed status; and
- empty bindings, parameter IDs, evaluator outputs, required inputs, and
  plugin subscriptions.

M6 applies either kind. Applying a failed or ready-but-empty plan advances the
tracking generation, deactivates every plugin for avatar collection, and clears
the OSC catalog. Old plan contents are never copied into a new result.

## Avatar ID and Discovery

An avatar ID is non-empty, has a bounded length, and contains no slash,
backslash, drive separator, NUL, `.` or `..` path segment, or other component
that could escape an exact filename lookup. The planner treats the ID as an
opaque filename stem after this safety validation; it does not require the
`avtr_` prefix so local test identifiers remain possible.

For a validated ID, discovery examines exactly:

```text
{OSCRoot}/*/Avatars/{avatarId}.json
```

It does not recursively scan other directories or inspect JSON contents to
identify users. Each candidate is resolved to an absolute clean path, checked
for containment beneath `OSCRoot`, and required to be a regular file. Links,
directories, devices, and other special files are rejected.

If more than one user directory contains the avatar file, discovery selects
the file with the newest modification time. Equal timestamps are resolved by
ascending normalized absolute path. This produces a deterministic result and
lets the most recently maintained per-user configuration win. The chosen path
is retained for diagnostics.

An I/O or metadata error for an exact candidate is a candidate failure, not
absence. It produces a failed plan and does not activate fallback.

## Fallback Policy

`FallbackPath` is an explicit user-selected JSON file path. M5 accepts it as
configuration but does not persist it. An empty fallback path means fallback
is disabled.

Fallback is considered only when discovery finds no candidate for the incoming
avatar ID. It is not considered when a candidate exists but cannot be read,
decoded, validated, or compiled. This distinction prevents a malformed
published-avatar configuration from being silently hidden by an unrelated
fallback.

The fallback must resolve to a regular file. It may live outside `OSCRoot`
because the user explicitly selected it. The JSON `id` may differ from the
incoming avatar ID, because the purpose is to reuse a known binding set for a
local test avatar. Both IDs and the fallback source flag remain observable in
the plan.

If neither a regular avatar candidate nor a usable fallback exists, activation
returns a failed, empty plan for the newly allocated generation.

## Bounded JSON Decoding

The local file is limited to 4 MiB. Reading byte 4 MiB plus one proves an
oversized file without unbounded allocation. The `parameters` array is limited
to 4096 entries. OSC addresses are limited to 1024 bytes.

The consumed shape is:

```json
{
  "id": "avtr_example",
  "name": "Example",
  "parameters": [
    {
      "name": "Face/v2/JawOpen",
      "input": {
        "address": "/avatar/parameters/Face/v2/JawOpen",
        "type": "Float"
      }
    }
  ]
}
```

The decoder accepts unknown object fields for forward compatibility with an
externally owned VRChat format. Known fields must have the documented JSON
types. The root `id` and `parameters` fields are required. `name`, `output`,
and entries with no `input` are not used for planning. A normal avatar
candidate requires root `id` to equal the avatar-change ID. A fallback does
not.

Every present input requires a non-empty absolute OSC address and an exact
type of `Int`, `Bool`, or `Float`. These map to OSC scalar type tags `i`, `T`,
and `f`. The Boolean tag is a compilation category; the sender still chooses
the concrete true/false wire value per evaluated snapshot.

Malformed known fields, extra JSON values after the root object, excessive
entries, invalid addresses, or duplicate/conflicting recognized bindings fail
the entire selected configuration. Unknown, well-formed non-VRCFT parameter
addresses are ignored after endpoint validation.

Only `input` is compiled. In VRChat's terminology it is the endpoint on which
VRChat receives an external value, so it is the endpoint to which this product
sends the evaluator result. Avatar JSON `output` endpoints describe values
sent by VRChat and are outside the face-tracking send path.

## OSC Endpoint Compilation

`internal/osc` adds an entry point equivalent to:

```go
func BuildCatalogFromEndpoints(
	endpoints []Endpoint,
	specs *ParameterCatalog,
	generation uint64,
) (*Catalog, error)
```

The method owns a copy of its endpoint input. It validates supported wire
types, compiles direct and binary bindings, sorts deterministically, rejects
one address assigned to conflicting output operations, assigns cache indexes,
and computes the same stable hash as the existing OSCQuery path.

`BuildCatalog` retains its public behavior by flattening writable supported
OSCQuery methods and delegating to `BuildCatalogFromEndpoints`. Existing OSC
tests must prove byte-for-byte-equivalent public catalog fields and output
behavior for the same endpoints.

The avatar planner builds endpoints from every valid JSON input. The OSC
compiler resolves VRCFT identity from the input address, including documented
prefixes and binary forms. It does not infer VRCFT identity from the JSON
parameter's display/name field.

## Requested Parameters and Evaluator Plan

The requested ParameterIDs are the keys of the compiled catalog bindings. The
planner sorts them by stable numeric ID and stores one copy. Duplicated
equivalent endpoints do not duplicate requested IDs.

The sorted IDs are passed to `evaluator.Compile`. Any unknown dependency,
cycle, inconsistent operation, or other evaluator compilation error fails the
candidate plan. A zero-length requested list compiles successfully to M4's
empty evaluator plan and remains a ready plan.

The same sorted IDs are passed to `parameterdeps.RequiredInputs`. The result is
stored by value and drives subscription construction. This guarantees the
evaluator and acquisition subscription are derived from the same exact root
set.

## Base Tracking Requirements

The base required capabilities are derived as follows:

- non-zero required Eye fields or `ActiveStateEyeTracking` adds
  `CapabilityEye`;
- a non-zero Expression mask or `ActiveStateExpressionTracking` adds
  `CapabilityExpression`; and
- `ActiveStateLipTracking` adds `CapabilityLip`.

Eye numeric requirements use `Inputs.RequiredEyeValid()`. Expression numeric
requirements retain the exact normalized expression mask. Lip is metadata-only
and has no detail mask.

If a plan needs only Eye or Expression active state, its detail mask remains
zero. Existing `pluginapi.Subscription` semantics interpret zero detail under
an enabled capability as the complete group. That broader subscription is
intentional because tracking activity is derived from continuing group frames;
there is no active-only frame shape in protocol v1.

A plan with no required capabilities does not fabricate a positive-generation
empty `pluginapi.Subscription`, because that value is invalid under the stable
plugin API contract.

## Per-Plugin Subscription Projection

`Plan.SubscriptionFor` intersects the base capability set with a plugin's
advertised capabilities. Unknown capability bits in the argument are ignored
for intersection and do not appear in the result.

When the intersection is empty, it returns the zero subscription and `false`.
M6 uses `false` to set that plugin inactive for the avatar session. It does not
need to publish an invalid positive-generation empty subscription; the new
tracking generation already rejects any in-flight frame carrying the plugin's
older subscription generation.

When the intersection is non-empty, it returns `true` and a normalized,
validated subscription containing:

- the plan generation;
- only intersecting capabilities;
- the Eye detail mask only when Eye is present; and
- the Expression detail mask only when Expression is present.

M6 updates the subscription before activating a matching plugin. A plugin
without matching capability is deactivated. Plugin enable preferences remain
owned by `internal/plugins`; avatar activity must not persist as a user
preference.

## Error Model

Stable `errors.Is`-compatible sentinels distinguish at least:

- invalid planner configuration;
- invalid avatar ID;
- avatar configuration not found;
- invalid or unsafe configuration path;
- configuration too large;
- invalid JSON or known-field type;
- avatar configuration ID mismatch;
- excessive parameter count;
- invalid input endpoint;
- binding compilation failure;
- requirement/evaluator compilation failure; and
- generation exhaustion.

Detailed errors wrap these sentinels with the selected path, parameter index,
or field name. Errors never include configuration contents. A failed plan keeps
a stable diagnostic classification and selected source metadata without
retaining an open file, decoder buffer, or caller-owned error string buffer.

## Concurrency and Ownership

`Planner.Activate` is safe for concurrent calls. A mutex defines activation
order and protects generation allocation. The implementation may hold the lock
through synchronous compilation so result order is identical to generation
order; activation is a low-frequency control path, not a frame hot path.

Plans are immutable and safe for concurrent reads. Each plan owns its path and
ID strings, sorted parameter slice, required-input value, and cloned catalog.
The plan contains no context, open file, timer, goroutine, or callback. It does
not cache frames or evaluator snapshots.

## M6 Application Contract

M6 listens to `osc.EventAvatarChanged`, calls `Planner.Activate`, and installs
the returned plan as one ordered control transition. The detailed M6 design
will define rollback and inter-component sequencing, but it must preserve these
M5 guarantees:

1. apply the returned plan even when `Result.Err` is non-nil;
2. advance tracking to the plan generation before accepting new-generation
   frames;
3. prevent the old OSC catalog from sending after the new avatar event;
4. update and activate only plugins for which `SubscriptionFor` returns true;
5. deactivate plugins with no current requirement; and
6. report the M5 diagnostic without restoring the old plan.

M5 tests use fakes and direct calls; they do not construct the Application or
claim M6 completion.

## Testing Strategy

### Discovery and fallback

Tests use temporary directory fixtures and no real VRChat user data. They
cover:

- one and multiple user-ID directories;
- newest-modification selection and normalized-path tie breaking;
- invalid IDs and traversal attempts;
- exact one-level discovery without recursive leakage;
- regular files versus directories and links;
- missing current config with present, absent, and invalid fallback;
- fallback ID mismatch acceptance; and
- proof that read, decode, ID, and binding errors on an existing current
  candidate do not invoke fallback.

### JSON and endpoints

Tests cover:

- the documented root and parameter shape;
- Int, Bool, and Float input conversion;
- ignored output and absent input;
- accepted unknown fields with strict known-field types;
- trailing JSON rejection;
- 4 MiB, 4096-entry, and 1024-byte boundaries;
- invalid or relative OSC addresses; and
- unknown non-VRCFT inputs versus conflicting recognized bindings.

### OSC compiler reuse

`internal/osc` tests cover endpoint-input ownership, direct, prefixed, and
binary bindings, deterministic sorting and hashing, duplicate equivalence,
conflict rejection, and equivalence between OSCQuery and endpoint entry points.
Existing sender, race, and benchmark behavior must remain unchanged.

### Requirements and subscriptions

Tests cover:

- stable deduplicated requested IDs;
- evaluator compilation from the exact binding roots;
- dependency-leaf union;
- exact Eye and Expression detail masks;
- independent Eye, Expression, and Lip active requirements;
- active-only whole-group subscription semantics;
- empty ready plans with no subscription;
- full, partial, and empty plugin capability intersections; and
- positive plan generation on every returned subscription.

### State and ownership

Tests cover repeated avatar IDs, successive failures, ready-to-failed and
failed-to-ready transitions, generation exhaustion, concurrent unique
generation allocation under the race detector, and mutation attempts through
all slice/map-bearing inputs and accessors.

### Cross-package proof

A bounded test compiles a local avatar JSON fixture, obtains its requested
ParameterIDs and evaluator plan, evaluates a matching canonical frame, and
reads the resulting snapshot through the structural `osc.ValueSource`
interface. It does not start OSC networking or Application workers.

## Project Evidence and Documentation

M5 adds `docs/project/packages/internal-avatar.md` with executable package and
race checks plus structural checks for discovery, fallback, planning, and
subscription evidence. It updates the `internal-osc` package specification to
describe the shared endpoint compiler, and updates end-to-end/current-gap text
where M5 is no longer deferred.

Implementation verification uses the repository-local absolute `GOCACHE` and
includes:

```text
go test ./internal/avatar ./internal/osc
go test -race ./internal/avatar ./internal/osc
go vet ./internal/avatar ./internal/osc
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/projectstatus
go run ./cmd/projectstatus -check
```

The narrowest relevant checks run during each task. Full-repository checks run
before completion. Generated `docs/project/status.md` is refreshed only from a
clean, reviewed implementation commit using `go run ./cmd/projectstatus
-write`, followed by a status-check command and review of the generated diff.

## Completion Definition

M5 is complete when:

1. every accepted avatar-change attempt produces a unique generation-tagged
   ready or fail-closed plan;
2. discovery and fallback follow the exact multi-user and not-found-only
   policies;
3. bounded local JSON inputs compile through the shared OSC binding logic;
4. evaluator roots, dependency leaves, and per-plugin subscriptions derive
   from the same stable binding IDs;
5. failures cannot preserve bindings, requirements, or subscriptions from the
   prior avatar;
6. package, race, vet, full-repository, and project-status checks pass with the
   fixed repository-local Go cache;
7. package and subsystem specifications describe the implemented boundaries;
   and
8. generated project evidence reports M5 complete without claiming M6
   Application wiring or M7 fallback persistence/UI complete.
