---
id: internal-avatar
kind: go-package
path: internal/avatar
milestone: M5
depends_on: [internal-osc, internal-evaluator, internal-parameterdeps, internal-parameters, pkg-pluginapi, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Avatar planning package tests pass
    type: command
    command: go-test
    args: [./internal/avatar]
    weight: 3
    required: true
  - id: package-race-tests
    description: Avatar planning is race-free
    type: command
    command: go-test-race
    args: [./internal/avatar]
    weight: 2
    required: true
  - id: planner-implemented
    description: Generation-tagged avatar planner exists
    type: symbol
    path: internal/avatar/planner.go
    pattern: '(?m)^func \(p \*Planner\) Activate\('
    weight: 2
    required: true
  - id: fallback-policy-tested
    description: Fallback is limited to missing avatar configs
    type: symbol
    path: internal/avatar/discovery_test.go
    pattern: '(?m)^func TestResolveConfigUsesFallbackOnlyWhenAvatarMissing\('
    weight: 1
    required: true
  - id: subscriptions-tested
    description: Per-plugin capability projection is tested
    type: symbol
    path: internal/avatar/requirements_test.go
    pattern: '(?m)^func TestPlanSubscriptionForIntersectsCapabilities\('
    weight: 2
    required: true
---
# Package: internal/avatar

## Purpose

Compile one `/avatar/change` avatar ID plus an optional configured fallback JSON file into an immutable, generation-tagged control-plane plan. The plan joins local VRChat avatar input endpoints to the existing OSC catalog, selective evaluator roots, dependency-derived tracking requirements, and per-plugin subscriptions.

## Responsibilities

Validate and resolve avatar configuration files deterministically across one level of `{OSCRoot}/*/Avatars/{avatarID}.json`; choose the newest regular non-link candidate and break timestamp ties by normalized absolute path. Use the explicit fallback only when no avatar candidate exists. Decode the bounded, forward-compatible JSON shape; validate known `id`, `parameters`, and `input` fields; compile only `parameters[].input` endpoints; and keep source/path/ID diagnostics.

`Planner.Activate` reserves a new positive generation for each normal attempt, including repeated IDs and fail-closed failures. It compiles endpoint bindings through `osc.BuildCatalogFromEndpoints`, derives stable sorted `parameters.ParameterID` roots, compiles an `evaluator.Plan`, derives `parameterdeps.Inputs`, and projects those requirements with `Plan.SubscriptionFor` onto each plugin's advertised Eye, Expression, and metadata-only Lip capabilities.

## Non-responsibilities

This package does not receive OSC events, run an application lifecycle, atomically install a plan into tracking/plugins/evaluator/OSC, schedule frames, evaluate or send network data, watch files, write VRChat configuration, persist or select the fallback path in a UI, or implement numeric Lip payloads or Expression-to-Lip mapping. It does not replace local JSON with OSCQuery discovery.

## Current implementation

`NewPlanner(PlannerConfig)` normalizes a required OSC root and optional fallback path, then constructs the VRCFT parameter catalog. `(*Planner).Activate(string)` serially resolves, reads, validates, and compiles the selected file. Successful results are `StatusReady`; ordinary failures return `StatusFailed` with a non-nil empty operational plan. Only exhausted generations return a nil plan.

Normal avatar configurations must have a matching root `id`; fallback configurations may use another ID. The decoder accepts unknown JSON fields but limits a file to 4 MiB, parameters to 4096 entries, IDs to 256 bytes, and input addresses to 1024 bytes. It accepts only absolute `Int`, `Bool`, and `Float` input endpoints. Invalid/unsafe IDs, links, directories, special files, malformed known fields, trailing JSON, oversized input, invalid endpoints, conflicting recognized bindings, and evaluator/dependency failures fail closed without reusing the prior plan.

## Public/internal interfaces

`PlannerConfig`, `NewPlanner`, `Planner.Activate`, `Status`, `Source`, `Result`, and `Plan` are the package contract. `Result` contains `Plan *Plan` and `Err error`. `Plan` exposes `Generation`, `Status`, `AvatarID`, `ConfigID`, `ConfigPath`, `Source`, `ParameterIDs`, `Catalog`, `Evaluator`, `RequiredInputs`, and `SubscriptionFor(trackingmodel.Capability)`.

`SubscriptionFor` returns a normalized, positive-generation `pluginapi.Subscription` only when the ready plan and advertised capabilities intersect. It retains exact Eye/Expression detail masks, treats active-only Eye/Expression requirements as a whole group (zero detail mask), and returns zero/false instead of an invalid empty subscription. Unknown advertised capability bits are ignored.

## Owned data

A plan owns its sorted parameter-ID slice, cloned OSC catalog, required-input value, diagnostics, and compiled immutable evaluator plan. `ParameterIDs` returns a fresh slice and `Catalog` returns a deep clone; caller mutation cannot alter plan state. The planner owns its normalized paths, parameter catalog, mutex, and monotonic generation counter. It retains no open file, decoder buffer, frame, evaluator snapshot, callback, timer, or goroutine.

## Dependencies

The planner deliberately composes `internal/osc`, `internal/evaluator`, `internal/parameterdeps`, and `internal/parameters` at the control-plane boundary. It produces subscriptions using `pkg/pluginapi` and `pkg/trackingmodel`. Evaluator remains independent of OSC; OSC retains endpoint/binding and sender ownership.

## Concurrency and lifecycle

`Activate` holds the planner mutex through generation allocation and synchronous resolution/compilation, defining a unique, ordered sequence of plans for concurrent callers. Returned plans are immutable for concurrent reads. The package starts no goroutine and has no Start/Close lifecycle.

## Error handling

Errors preserve `errors.Is` categories for invalid planner configuration or avatar ID, missing/unsafe paths, oversized files, invalid JSON, ID mismatch, excessive parameters, invalid endpoints, binding compilation, requirement compilation, and generation exhaustion. Context includes paths or field/index metadata where useful, never configuration contents. An ordinary failed plan has its new generation and available source/path/ID diagnostics but empty catalog, evaluator, IDs, inputs, and subscriptions, so a caller cannot retain stale operational data.

## Performance constraints

Activation is a low-frequency synchronous control path. File and decoded-field limits bound work and allocation; discovery examines exactly one user-directory level rather than recursively scanning. Requirement and subscription operations are bounded by the fixed tracking and generated parameter catalogs. No frame hot-path work or background polling is introduced.

## Security boundaries

Avatar IDs reject traversal and Windows-reserved path characters. Candidate paths are absolute/clean, contained beneath the OSC root, and every component is inspected to reject links/reparse points, irregular files, and non-regular targets. Fallback is an explicit regular non-link file and may be outside the root. Endpoint validation requires bounded absolute addresses and supported scalar types; unrecognized but valid non-VRCFT endpoints do not become bindings.

## Required tests

Package and race checks cover bounded JSON decoding, input-only endpoint handling, deterministic discovery, traversal/link rejection, missing-only fallback, endpoint/OSCQuery compiler parity, catalog deep-clone ownership, ready and fail-closed generation transitions, generation exhaustion, concurrent activation, requirement masks, capability projection, immutable accessors, and an external evaluator-to-`osc.ValueSource` compatibility fixture. The named fallback and subscription tests are catalog evidence for this package.

## Known gaps

M6 must listen for `osc.EventAvatarChanged`, call `Activate`, and atomically install every returned plan—even when `Result.Err` is non-nil—by advancing tracking generation, clearing/replacing OSC bindings, updating only matching plugin subscriptions, and deactivating nonmatching plugins. Application lifecycle, rollback, frame processing/evaluation-to-OSC wiring, and end-to-end integration remain M6 work. M7 owns fallback-path persistence and frontend selection/diagnostics. Numeric Lip payloads and Expression-to-Lip mapping remain deferred.

## Completion definition

M5 planning is complete when each accepted avatar-change attempt has a unique generation-tagged ready or fail-closed plan; discovery, fallback, bounded decode, shared binding compilation, requirement derivation, immutable ownership, and subscription projection obey this contract; and the package/race evidence passes. This does not assert M6 atomic Application installation or M7 fallback persistence/UI completion.
