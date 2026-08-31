---
id: internal-userconfig
kind: go-package
path: internal/userconfig
milestone: M7
depends_on: [internal-application]
checks:
  - id: package-tests
    description: Versioned user configuration tests pass
    type: command
    command: go-test
    args: [./internal/userconfig]
    weight: 3
    required: true
  - id: package-race-tests
    description: User configuration operations are race-free
    type: command
    command: go-test-race
    args: [./internal/userconfig]
    weight: 2
    required: true
  - id: repair-backup-order-tested
    description: Invalid-file repair installs its durable backup before replacement
    type: symbol
    path: internal/userconfig/store_test.go
    pattern: '(?m)^func TestStoreRepairInstallsBackupBeforeReplacement\('
    weight: 2
    required: true
  - id: windows-path-resolution-tested
    description: Windows product path derivation has executable evidence
    type: symbol
    path: internal/userconfig/paths_test.go
    pattern: '(?m)^func TestResolvePathsWindowsDerivesProductPaths\('
    weight: 2
    required: true
blockers:
  - check: package-tests
    blocks: [M7]
  - check: package-race-tests
    blocks: [M7]
  - check: repair-backup-order-tested
    blocks: [M7]
  - check: windows-path-resolution-tested
    blocks: [M7]
---
# Package: internal/userconfig

## Purpose

Own the Windows-only, versioned user settings document and convert normalized user intent into a complete, independently owned `application.Config` for process startup.

## Responsibilities

Resolve settings, plugin-preference, builtin-plugin, and VRChat OSC paths from injected platform values; create exact revision-1 defaults only when the settings file is absent; strictly decode and semantically normalize bounded JSON; convert stable processing and OSC target DTOs; enforce revision and external-file fingerprint conflicts; durably save with secure temporary files and atomic replacement; preserve the latest invalid bytes before explicit repair; and clone every reference-backed value crossing the package boundary.

## Non-responsibilities

It does not import Wails, bind frontend methods, start goroutines, construct or own a running Application, mutate a running backend, supervise plugins, discover OSCQuery services, or choose operational timeouts and queue limits that belong to lower-level packages.

## Current implementation

`ResolvePaths` rejects non-Windows platforms, blank or NUL-containing environment values, and non-absolute executable paths before deriving `%AppData%/vrcft-go/config.json`, `plugins.json`, the executable-relative builtin plugin directory, and the VRChat OSC root. `DefaultCandidate`, `Normalize`, and processing conversion preserve the v1 wire schema while sorting stable lists, rejecting duplicate or unknown channels and Windows paths, validating finite/ranged values, and enforcing disjoint automatic/manual OSC settings. `ApplicationConfig` maps a normalized `Settings` clone plus resolved paths into lower-level configs and calls `application.ValidateConfig`, which performs Application normalization without constructing runtime dependencies.

`Store.LoadOrCreate` uses strict, size-bounded JSON and preserves an existing invalid or unsupported document as diagnostic state. `Store.Save` serializes access, rereads the authoritative file inside its gate, validates caller revision and an unforgeable file token, treats semantic equality as a no-op, rejects revision exhaustion, and publishes only a fully owned `SaveResult` after durable replacement. The full-file SHA-256 and invalid-file backup streams poll their context before and after every 32 KiB read/write chunk; cancellation closes the source and removes partial temporary files without changing the authoritative document or installing a partial backup. Repair first atomically installs the complete invalid bytes as `.invalid.bak`; temporary write, sync, close, replacement, and cleanup failures leave the prior authoritative settings file intact.

## Public/internal interfaces

`Environment`, `Paths`, `ResolvePaths`, `Settings`, `Candidate`, `DefaultCandidate`, `Normalize`, `ApplicationConfig`, `ValidationError`, `Store`, `NewStore`, `Loaded`, `DocumentToken`, `SaveResult`, and the stable conflict/validation/platform error categories form the package contract. File-operation seams, strict decoder helpers, normalized path keys, and replacement implementations remain package-internal.

## Owned data

Settings, candidates, processing overrides, mutual-exclusion groups, development roots, application configs, loaded snapshots, save results, and file bytes are cloned at ownership boundaries. A `Store` owns only its resolved paths, injected file operations, and a capacity-one serialization token; it retains no open file or caller context between calls.

## Dependencies

Depends one-way on `internal/application` for construction-config validation and DTO mapping, and uses Application's existing processing, plugin, avatar, and OSC configuration types transitively. `internal/application` does not import `internal/userconfig`.

## Concurrency and lifecycle

The package starts no goroutine and has no Start/Close lifecycle. Each Store serializes load/save operations with context-aware admission, performs the authoritative reread and compare-and-swap decision within that gate, and releases all file handles before returning. Independent Store instances rely on the file fingerprint/identity check to reject out-of-process replacement.

## Error handling

Strict decoding rejects oversize input, invalid UTF-8, unknown and duplicate fields, required nulls, trailing JSON, invalid schema versions, and malformed field types. `ValidateCandidateBounds` runs before cloning or normalization for Wails candidates: the encoded `SettingsV1` wire value is at most 256 KiB; paths are valid UTF-8 and at most 32 KiB; OSC endpoint strings are at most 255 bytes; target mode, filter mode, and channel names are limited to 16, 32, and 128 bytes; development roots, overrides, mutual-exclusion groups, and members per group are each at most 128. The 32 KiB path bound matches practical Windows extended-path limits; the 128 processing bounds cover the current fixed 85-channel catalog (10 eye plus generated expressions) while leaving controlled growth headroom. Semantic errors identify a stable settings field; missing first-run files are created, while existing invalid files are returned without replacement. Conflicts, revision exhaustion, unsupported platform, invalid environment, and invalid loaded state retain stable `errors.Is` categories, and filesystem failures retain operation context without embedding document contents.

## Performance constraints

Settings files and Wails candidates are bounded to 256 KiB after JSON encoding. Candidate strings and nested lists use the explicit limits above before clone, normalization, or backend dispatch; normalization is therefore bounded by fixed processing catalogs. Storage work is synchronous low-frequency control-path I/O with cancellation polling per 32 KiB stream chunk. No timer polling, unbounded queue, recursive filesystem scan, or frame-path work is introduced.

## Security boundaries

Path inputs are required, absolute where applicable, cleaned, and checked for NUL or malformed Windows forms. JSON decoding and collections are bounded. Writes use user-only permissions, complete writes, file sync, close, atomic platform replacement, best-effort directory sync, and temporary cleanup. Errors never include settings JSON, plugin-private configuration, credentials, or process environment values. The package may return paths the user configured but exposes nothing directly to Wails.

## Required tests

Normal and race package gates cover strict decoding, exact first-run defaults, clone shape and ownership, exact/max-plus-one candidate field, nested-list, and encoded-size bounds, Windows path derivation and unsupported environments, processing and target-mode conversion, lower-level validation without construction, no-op/revision/external-file conflicts, file identity, deterministic cancellation of blocked read/backup/write chunks, secure atomic writes, fault cleanup, invalid-file preservation, and durable backup-before-replacement repair. `TestStoreRepairInstallsBackupBeforeReplacement` and `TestResolvePathsWindowsDerivesProductPaths` are explicit catalog evidence.

## Known gaps

The non-frontend M7 settings prerequisite is implemented. Frontend settings forms, status presentation, generated Wails bindings, dependency installation, and release UX remain outside this package; settings saves intentionally require restart rather than hot-replacing the current Application.

## Completion definition

One Windows process can derive deterministic product paths, create or load a strict owned revisioned settings snapshot, repair invalid bytes without losing the original, save semantic changes atomically with conflict protection, and produce a validated owned Application config while all normal/race and structural evidence passes.
