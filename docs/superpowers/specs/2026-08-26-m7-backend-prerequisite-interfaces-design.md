# M7 Backend Prerequisite Interfaces Design

**Date:** 2026-08-26
**Status:** Approved
**Milestone:** M7 — Frontend, operations, and release

## Summary

M6 completed the backend data path, but the Wails root still does not own an
`internal/application.Application`, load persisted product settings, or expose
an intentional frontend-facing operations surface. This design supplies those
non-frontend prerequisites for M7.

The work is deliberately split into a Wails-independent settings package,
small operational additions to `internal/application`, an explicit OSC target
policy, and three narrowly bound root APIs. The frontend will consume these
interfaces later. This design does not implement pages, install frontend
dependencies, or generate Wails JavaScript/TypeScript bindings.

M7 remains incomplete until the separate frontend checks pass.

## Goals

1. Load a versioned user configuration and turn it into the explicit
   `application.Config` required by M6.
2. Give the Wails root sole ownership of one Application lifecycle.
3. Keep the shell available in a diagnostic mode when settings or backend
   startup fail.
4. Expose separate runtime, plugin, and settings APIs with module-local
   snapshots and events.
5. Apply plugin enabled preferences and plugin-owned JSON configuration
   immediately and durably without restarting the Application.
6. Persist construction-time product settings for the next process start.
7. Support automatic OSCQuery target selection and an explicit manual OSC
   target without allowing one mode to overwrite the other.
8. Preserve bounded queues, owned snapshots, sanitized diagnostics, and
   deterministic shutdown.

## Non-goals

- Svelte pages, components, styling, routing, or state management.
- Installing or changing frontend dependencies.
- Updating generated files under `frontend/wailsjs`.
- Hot-reloading or replacing a running Application after product settings are
  saved.
- Editing internal lifecycle, retry, queue, network-listener, or size-limit
  policies from the frontend.
- Plugin installation, updates, signing, or marketplace behavior.
- A host-defined form schema for plugin-owned configuration.
- Credential storage. Plugins must not place credentials in the JSON document
  exposed by this version of the plugin configuration API.
- Non-Windows runtime support. Other platforms remain buildable where the
  repository already supports them, but backend startup is rejected with a
  structured `unsupported_platform` diagnostic.

## Architectural Decision

Three approaches were considered:

1. Put persistence, lifecycle, controls, and Wails events directly in the root
   `App`. This minimizes files but makes `package main` own product semantics
   and makes tests unnecessarily dependent on Wails.
2. Add a settings layer, extend Application only with the operations it owns,
   and keep Wails adaptation in the root. This preserves package ownership and
   permits deterministic tests without Wails.
3. Add a generic command bus, event bus, and operations service. This is more
   general than the three known modules require and adds avoidable state
   synchronization.

The approved choice is approach 2:

```text
Wails
  +-- RuntimeAPI  ----+
  +-- PluginsAPI  ----+-- root App (lifecycle, DTOs, event adapters)
  +-- SettingsAPI ----+
             |
             +-- internal/userconfig
             +-- internal/application
                         |
                         +-- internal/osc
```

## Package Ownership

### `internal/userconfig`

This new package owns:

- the versioned, user-facing settings schema;
- Windows settings and product-data path resolution;
- first-run defaults;
- strict bounded JSON decoding and semantic validation;
- optimistic revision checks and external-file conflict detection;
- secure temporary-file writes, sync, and atomic replacement;
- preservation of the latest invalid file during an explicit repair; and
- conversion from user intent to a complete `application.Config`.

It does not import Wails, start goroutines, own an Application, or mutate a
running backend. `internal/application` does not import `internal/userconfig`.

### `internal/application`

Application remains the owner of the composed backend and its plugin Manager.
It gains the minimum public operations needed by the root:

```go
func (*Application) Plugins() []plugins.RuntimeSnapshot
func (*Application) PluginConfig(string) (pluginapi.Config, bool)
func (*Application) SetPluginEnabled(context.Context, string, bool) error
func (*Application) UpdatePluginConfig(
	context.Context,
	string,
	pluginapi.Config,
) error
func (*Application) SubscribePlugins(context.Context) <-chan []plugins.RuntimeSnapshot
```

These are the required Application method names. The underlying Manager gains
`PluginConfig(string) (pluginapi.Config, bool)` as the owned read method for one
persisted plugin configuration. Reads return clones.

Application rejects plugin mutations unless it is running. Enable/disable is
an idempotent desired-state operation. Configuration updates retain the
existing Manager contract: validate and durably save first, then send the
accepted complete configuration to a running plugin. Failure never makes a
non-durable value authoritative.

The plugin snapshot subscription is capacity one and latest-only. It publishes
an initial list and then a newly owned complete list after relevant Manager
events. Raw plugin logs, tracking frames, session credentials, process handles,
and mutable Manager objects are not exposed.

### Root package

The root `App` owns:

- platform and path resolution;
- the settings store;
- zero or one Application instance;
- startup and shutdown state;
- three API objects;
- three module-local snapshot stores; and
- cancellable Wails event forwarders.

The root translates internal values to stable DTOs and sanitizes all errors.
It does not duplicate configuration validation, plugin behavior, avatar
planning, or OSC behavior.

Only `RuntimeAPI`, `PluginsAPI`, and `SettingsAPI` are included in Wails
`Bind`. The root `App`, its lifecycle methods, the Application, and the store
are not bound.

### `internal/osc`

OSC continues to own listeners, OSCQuery discovery, target installation,
avatar-change reception, and sending. It gains an explicit construction-time
target policy with automatic and manual modes. Application still forces
external catalog ownership as established by M6.

## Windows Paths and Defaults

M7 v1 runtime support is explicitly Windows-only.

The path resolver derives:

```text
settings directory:  %AppData%/vrcft-go
settings file:       %AppData%/vrcft-go/config.json
plugin preferences:  %AppData%/vrcft-go/plugins.json
built-in plugins:    <executable-directory>/plugins
VRChat OSC root:     %UserProfile%/AppData/LocalLow/VRChat/VRChat/OSC
```

Environment lookups, executable lookup, and platform detection are injected in
tests. Required blank or malformed environment values produce a structured
startup diagnostic rather than a guessed relative path.

Operational values are supplied by safe backend defaults:

- `plugins.DefaultOptions()`;
- `processing.DefaultConfig()` for a first-run settings document;
- package-owned manifest, store, and command size limits;
- package-owned frame and plugin-control intervals; and
- loopback listener defaults already owned by OSC.

The frontend cannot override these operational policies.

## Persisted Settings Schema

The v1 wire document represents user intent rather than serializing internal Go
configuration:

```text
SettingsV1 {
  schemaVersion
  revision

  avatar {
    oscRoot
    fallbackPath
  }

  plugins {
    devRoots[]
  }

  processing {
    defaultChannel
    overrides[]
    activeStaleAfterMs
    mutualExclusion[]
  }

  osc {
    targetMode          // "auto" or "manual"
    preferredService   // optional in auto mode
    manualHost         // required in manual mode
    manualPort         // required in manual mode
  }
}
```

`processing` uses explicit wire DTOs. Durations are positive integer
milliseconds, channel IDs are stable strings, and overrides are a sorted list
rather than a JSON object keyed by implementation enums. Conversion rejects
unknown and duplicate channel IDs, invalid numeric ranges, and repeated or
invalid mutual-exclusion membership before calling the processing package's
own validation.

### Revision rules

- `schemaVersion` is `1`.
- `revision` begins at 1.
- Every successful semantic change increments revision once.
- Revision saturating or exhaustion is an error; it never wraps.
- A save supplies `expectedRevision` from the most recent Settings snapshot.
- A byte-for-byte different but semantically identical normalized candidate is
  a no-op: it does not write, increment revision, publish an event, or require a
  restart.
- The store serializes reads and writes and checks the current file fingerprint
  immediately before replacement. An out-of-process edit causes `conflict`
  even when the caller's in-memory revision still matches.

When an invalid document has no trustworthy file revision, the Settings module
still exposes its own current snapshot revision. An explicit repair must match
that revision, and the store must also match the invalid file fingerprint seen
by that snapshot.

### Validation and normalization

- The complete file is limited to 256 KiB.
- JSON decoding rejects unknown fields, duplicate fields, trailing values,
  invalid UTF-8, and `null` for required objects, arrays, strings, and numeric
  values.
- `oscRoot` is required. Non-empty paths are made absolute and cleaned before
  being stored. NUL and malformed Windows volume paths are rejected.
- A fallback path and development roots may point to entries that do not yet
  exist. This preserves the M5 rule that VRChat may create a configuration
  later and supports temporarily disconnected development directories.
- Duplicate normalized development roots are rejected. Stable output sorts
  roots by normalized case-insensitive Windows path, with the original
  normalized path as a tie breaker.
- `auto` requires manual fields to be empty. `manual` requires
  `preferredService` to be empty and a valid address and port.
- Manual addresses are explicit unicast IPv4 or IPv6 literals. V1 performs no
  DNS lookup and rejects unspecified, multicast, IPv4 broadcast, and port 0.
- Conversion performs package-owned validation again by constructing or
  validating the resulting lower-level configs. The persistence layer cannot
  bypass backend validation.

### First run and repair

If `config.json` is absent, startup creates the directory and writes a valid
revision-1 default document before constructing Application. Defaults are:

- the resolved VRChat OSC root;
- empty fallback path;
- no development plugin roots;
- `processing.DefaultConfig()`; and
- automatic OSC targeting with no preferred service.

An existing invalid or unsupported document is never replaced during startup.
The root enters diagnostic mode. A later explicit, revision-checked Settings
save may repair it. Before replacing the configuration, the old bytes are
durably written and atomically installed as the single latest invalid backup
in the settings directory. The backup uses the same user-only permissions. If
the later configuration replacement fails, the invalid original remains
authoritative and the refreshed backup is harmless.

### Durable file operations

Settings writes use a temporary file in the destination directory, user-only
permissions, complete writes, file sync, close, an atomic platform replacement,
and best-effort directory sync where supported. Any failure before successful
replacement leaves the old authoritative file intact and removes the temporary
file. No Settings event is emitted before replacement succeeds.

Plugin enabled preferences and plugin-private JSON remain in the existing
plugin preference store. They are not copied into `SettingsV1`.

## Application Configuration Mapping

`internal/userconfig` combines normalized settings and resolved product paths
into a fresh, fully owned `application.Config`:

- `Avatar.OSCRoot` and `Avatar.FallbackPath` come from settings;
- `PluginCatalog.BuiltinRoot` is derived from the executable;
- `PluginCatalog.DevRoots` comes from settings;
- `PluginStorePath` is the derived plugin-preference path;
- plugin limits/options are backend defaults;
- `Processing` is the validated converted processing document;
- `OSC` contains the package-owned service/listener defaults plus the approved
  target policy; and
- frame/control timing remains Application-owned defaults.

Every reference-backed field is copied. Neither the Settings response nor the
saved candidate aliases the Application config.

## OSC Target Policy

The construction config distinguishes:

```text
TargetModeAuto
TargetModeManual
```

### Automatic mode

Automatic mode retains OSCQuery discovery. With no preferred service, the
existing deterministic target selection applies. With a non-empty preferred
service, only the exact approved service identity may become the send target.
If it is absent, OSC remains running without a target and reports a bounded
diagnostic; it does not silently select a different instance.

### Manual mode

Manual mode installs the validated UDP address as the send target at startup.
OSC still starts its receive and OSCQuery facilities so `/avatar/change` and
VRChat discovery diagnostics remain available, but discovery cannot install,
clear, or replace the manual send target. Disconnect and reconnect events do
not clear it.

Internal and frontend-facing status distinguish the concepts:

- `HasTarget` means a send target is installed;
- `Connected` continues to mean OSCQuery has identified VRChat; and
- the DTO includes the configured target mode so a manual target is not
  misrepresented as an OSCQuery connection.

The external M6 avatar catalog remains independent of target mode.

## Root API Contracts

The root defines Wails-safe DTOs rather than binding internal structs.
Interface and event names are versioned.

### Common problem value

```text
Problem {
  code
  message
  field?             // stable settings field path
  currentRevision?   // supplied for revision conflicts
}
```

Allowed v1 codes are:

```text
validation
conflict
not_found
unavailable
unsupported_platform
timeout
internal
```

Expected failures live in a response's optional `problem`. Frontend code never
parses a Go error string to determine behavior. Unexpected errors are logged
internally and mapped to a sanitized `internal` problem.

All module responses include that module's `revision` and `updatedAt`. Module
revisions are positive, monotonic, and non-wrapping for the process lifetime.
They are independent of Application Status revision, plugin configuration
revision, and Settings file revision.

### `RuntimeAPI`

```text
GetStatus() -> RuntimeResponse
event: vrcft:v1:runtime-status
```

The Runtime snapshot contains:

- root phase: `created`, `starting`, `running`, `diagnostic`, `closing`, or
  `closed`;
- platform support;
- optional sanitized startup/shutdown Problem;
- Application lifecycle;
- current avatar ID, plan generation, plan status/source, selected avatar
  configuration path and ID, and generation-exhaustion flag;
- OSC running/discovery/target state and bounded diagnostic;
- bounded plugin-control failures; and
- bounded plan/runtime diagnostics.

Numeric internal enums are translated to stable strings. When no backend was
constructed, the Application portion is absent rather than populated with
misleading zero values.

### `PluginsAPI`

```text
List() -> PluginListResponse
GetConfig(pluginID) -> PluginConfigResponse
SetEnabled(pluginID, enabled) -> PluginMutationResponse
UpdateConfig(pluginID, expectedConfigRevision, jsonData)
  -> PluginMutationResponse
event: vrcft:v1:plugins-changed
```

Plugin list entries contain only:

- ID, name, description, version, and advertised capabilities;
- enabled, active, and stable state string;
- configuration revision;
- frame rate, consecutive failure count, and restart count;
- bounded timestamps useful to operations; and
- a sanitized last error.

They exclude PID, session ID, executable/root paths, raw logs, subscription
details, tracking frames, credentials, and private configuration data.

`GetConfig` is the sole method that returns the plugin-owned JSON document. It
returns an owned copy and its configuration revision. The document is limited
to 64 KiB at this boundary even if a lower-level store permits more.

`UpdateConfig` accepts JSON data, not a caller-selected next revision. The root
requires the expected revision to match the current plugin configuration and
constructs revision `expected + 1`. Invalid JSON, size excess, revision
exhaustion, a stale revision, and a concurrent conflicting update are rejected.
The Manager remains the final authority for revision validation.

`SetEnabled` sets a desired persistent state and is idempotent. It does not use
volatile runtime state as an optimistic concurrency token.

Commands for one plugin ID are serialized. Commands for different IDs may run
concurrently. Every command uses a backend-owned bounded context; the frontend
cannot supply or extend lifecycle deadlines.

### `SettingsAPI`

```text
Get() -> SettingsResponse
Validate(candidate) -> SettingsValidationResponse
Save(expectedRevision, candidate) -> SettingsSaveResponse
event: vrcft:v1:settings-changed
```

`Get` returns the normalized user settings or, for an invalid file, a
diagnostic plus a valid default candidate suitable for explicit repair.
`Validate` performs the complete decode-independent semantic normalization and
lower-level config validation but does not write. `Save` repeats validation,
performs the revision and fingerprint checks, and writes durably.

A successful semantic change returns `restartRequired: true`. A no-op returns
the existing revision and `restartRequired: false`. Saving settings never
mutates or reconstructs the current Application.

### Events and snapshot stores

Each API owns an independent immutable snapshot store and one capacity-one
latest-value publication path:

```text
vrcft:v1:runtime-status
vrcft:v1:plugins-changed
vrcft:v1:settings-changed
```

An event contains the same complete DTO returned by that module's query. Query
and event paths call the same conversion function. There are no cross-module
mega-snapshots and no incremental patches.

Frontend startup queries the three modules independently. If an event is lost,
the frontend re-queries only that module. Event forwarding never blocks a
backend publisher or accumulates an unbounded Wails queue; a new internal
snapshot replaces the pending one.

## Root Lifecycle

### Construction

Root `NewApp` constructs only passive values: platform resolver, settings
store, module stores, API objects, and injectable dependencies. It starts no
goroutine and constructs no Application.

### Startup

Wails `OnStartup` executes this order:

1. store the Wails process context and publish root phase `starting`;
2. reject unsupported platforms into diagnostic mode;
3. load settings, creating first-run defaults only when the file is absent;
4. convert settings to an explicit `application.Config`;
5. call `application.NewApp(config)` exactly once;
6. retain the returned Application before starting it so any partial startup
   can be closed;
7. synchronously call `Application.Start(ctx)`; and
8. on success, publish `running` and start Runtime and Plugins event adapters.

Wails calls `OnStartup` before frontend readiness. Because the callback cannot
return an error, every failure is published as root phase `diagnostic`; the
shell remains available for Runtime and Settings queries. A constructed
Application's sanitized status remains visible after a failed `Start`.

Plugin operations return `unavailable` unless Application is running. Settings
queries, validation, and explicit repair remain available in diagnostic mode.

### Runtime

Runtime forwarding consumes `Application.SubscribeStatus`. Plugin forwarding
consumes the Application's latest-only plugin snapshots. Settings publishes
only after durable local writes.

The root synchronizes its backend pointer and phase. A startup/shutdown race
cannot create a second Application, start forwarders after shutdown, or lose a
constructed Application that requires closing.

### Shutdown

Wails `OnShutdown` is idempotent and executes this order:

1. stop accepting new Runtime/Plugin commands;
2. cancel and join event forwarders;
3. publish root phase `closing`;
4. derive a fixed bounded context from `context.Background()`, not from a
   possibly canceled Wails startup context;
5. call `Application.Close` at most once when an Application exists; and
6. publish `closed` with any sanitized close problem.

Repeated shutdown returns the recorded outcome. No service is abandoned merely
because the Wails context was already canceled.

`main.go` registers both real callbacks:

```go
OnStartup:  app.startup,
OnShutdown: app.shutdown,
```

and binds only the three API values.

## Errors, Security, and Bounds

- User-facing messages are valid UTF-8 and no longer than 512 bytes after
  rune-safe truncation.
- Structured fields, lists, and plugin failures have explicit count and string
  bounds before crossing Wails.
- DTO conversion deep-copies all slices, maps, byte strings, addresses, and
  JSON documents.
- Application configs, tracking payloads, transport buffers, process handles,
  session/authentication tokens, plugin environments, raw internal errors, and
  credentials never cross the Wails binding.
- Settings may echo paths the user configured. Runtime may expose the selected
  avatar configuration path for diagnosis. Plugin executable and root paths
  remain private.
- Full wrapped errors are retained only in local logs and are sanitized before
  logging when they may contain tokens, environment data, or plugin JSON.
- Unknown plugin IDs map to `not_found`; invalid input to `validation`; stale
  revisions to `conflict`; lifecycle rejection to `unavailable`; deadline
  expiry to `timeout`; unsupported OS to `unsupported_platform`; and all other
  unexpected failures to `internal`.

The root package specification's prohibition on exposing "raw configuration"
is refined by this design: internal `application.Config`, runtime payloads, and
credentials remain prohibited. The one explicit `PluginsAPI.GetConfig` method
may return the user's plugin-owned, non-credential JSON document.

## Testing Strategy

### `internal/userconfig`

Table and fault-injection tests cover:

- Windows path derivation and unsupported-platform behavior;
- required environment and executable failures;
- first-run creation and exact defaults;
- strict JSON types, unknown/duplicate fields, nulls, trailing data, invalid
  UTF-8, and size limits;
- path normalization and duplicate Windows paths;
- processing wire conversion and all lower-level validation failures;
- auto/manual OSC invariants and address validation;
- settings-to-Application config ownership;
- revision/no-op/exhaustion behavior;
- in-process and external-file conflicts;
- partial write, sync, close, replace, and directory-sync behavior; and
- invalid-file repair and backup ordering.

### `internal/application` and plugins

Tests cover:

- owned plugin lists and configuration reads;
- running-only mutation admission;
- idempotent enable/disable;
- generated configuration revision and conflict propagation;
- persistence before runtime command;
- one-plugin serialization and independent-plugin concurrency;
- latest-only initial and updated plugin snapshots;
- shutdown races and subscription cancellation; and
- normal and race suites for the affected packages.

### `internal/osc`

Tests prove:

- existing automatic selection remains unchanged without a preference;
- an exact preferred service never falls back silently;
- valid manual targets are installed;
- invalid, unspecified, multicast, and broadcast targets fail closed;
- OSCQuery cannot replace or clear a manual target;
- disconnect/reconnect retains a manual target;
- manual mode still receives avatar changes; and
- target mode, discovery state, and target state remain distinct in status.

### Root

Root tests use injected settings, Application, clock, event emitter, and
platform dependencies. They cover:

- passive construction;
- missing, valid, invalid, and unsupported settings startup;
- Application construction and Start failures;
- exact lifecycle ownership and at-most-once construction/close;
- startup/shutdown races and bounded reverse shutdown;
- diagnostic-mode Settings availability and Plugin unavailability;
- the exact Wails Bind allowlist;
- DTO ownership and sanitization;
- module-local revision and capacity-one event coalescing;
- no events after forwarder shutdown; and
- equality of query and event conversion.

## Project Evidence

Implementation adds a package specification for `internal-userconfig` and
updates the specifications for `internal-application`, `internal-osc`, root,
and the end-to-end subsystem. Root evidence must prove real Application
construction, Start/Close ownership, and actual Wails lifecycle registration.

Verification uses the repository-local absolute `.go-gocache` and includes:

- focused package tests;
- repeated focused tests;
- race tests for affected packages;
- focused and full-repository vet;
- full-repository normal and race tests;
- gofmt and diff checks; and
- a fresh `go run ./cmd/projectstatus` report.

The generated report may show the non-frontend root and backend prerequisites
complete, but must continue to report M7 blocked until the independent frontend
requirements pass.

## Completion Criteria

This design's implementation is complete when:

1. Windows first run creates a valid versioned configuration and starts exactly
   one explicitly configured Application;
2. existing invalid settings remain intact and produce a usable diagnostic and
   explicit repair path;
3. construction settings validate and save atomically for the next restart;
4. plugin selection and plugin-private JSON updates apply immediately with the
   existing durable Manager ordering and revision protection;
5. auto and manual OSC modes are deterministic and cannot overwrite each
   other's target ownership;
6. Runtime, Plugins, and Settings expose separate owned snapshots and bounded
   latest-only events through an explicit Wails allowlist;
7. startup failure and shutdown retain fail-closed, bounded lifecycle behavior;
8. focused, repeated, race, vet, full-repository, formatting, and status checks
   provide current evidence; and
9. documentation states that the non-frontend prerequisites are complete
   without claiming frontend functionality or full M7 completion.
