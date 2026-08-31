---
id: root
kind: go-package
path: .
milestone: M7
depends_on: [internal-application, internal-userconfig]
checks:
  - id: package-builds
    description: Root application builds
    type: command
    command: go-test-build
    args: [.]
    weight: 2
    required: true
  - id: package-tests
    description: Root lifecycle and API tests pass
    type: command
    command: go-test
    args: [.]
    weight: 3
    required: true
  - id: wails-config
    description: Wails configuration artifact exists
    type: file
    path: wails.json
    weight: 1
    required: true
  - id: backend-lifecycle-owned
    description: Root owns and invokes the composed Application lifecycle
    type: symbol
    path: app.go
    pattern: '(?s)func productionOwnedBackend\(config application\.Config\).*backend, err := application\.NewApp\(config\).*return backend, backend, err.*type App struct\s*\{.*backend\s+\*application\.Application\s*backendOps\s+backendOperations.*func \(a \*App\) startup\(parent context\.Context\) \{.*backend, operations, err := a\.deps\.newBackend\(config\).*a\.backend = backend\s*a\.backendOps = operations.*operations\.Start\(op\.ctx\).*func \(a \*App\) shutdown\(context\.Context\) \{.*operations := a\.backendOps.*operations\.Close\(closeCtx\)'
    weight: 2
    required: true
  - id: wails-lifecycle-callbacks
    description: Wails registers the owned backend startup and shutdown callbacks
    type: symbol
    path: main.go
    pattern: '(?s)wails\.Run\(&options\.App\{.*OnStartup:\s*app\.startup,.*OnShutdown:\s*app\.shutdown,'
    weight: 1
    required: true
  - id: wails-bind-allowlist
    description: Wails binds exactly Runtime, Plugins, and Settings APIs without the root App
    type: symbol
    path: main.go
    pattern: '(?s)Bind:\s*\[\]interface\{\}\s*\{\s*app\.runtime,\s*app\.plugins,\s*app\.settings,\s*\},'
    weight: 2
    required: true
  - id: backend-prerequisite-integration
    description: M7 backend prerequisite interfaces have a cross-package fixture
    type: symbol
    path: m7_backend_integration_test.go
    pattern: '(?m)^func TestM7BackendPrerequisiteInterfacesEndToEnd\('
    weight: 2
    required: true
blockers:
  - check: package-builds
    blocks: [M7]
  - check: backend-lifecycle-owned
    blocks: [M7]
  - check: wails-lifecycle-callbacks
    blocks: [M7]
  - check: wails-bind-allowlist
    blocks: [M7]
  - check: backend-prerequisite-integration
    blocks: [M7]
---
# Package: root

## Purpose
Provide the M7 Wails executable entry point, own the composed backend lifecycle, and expose three bounded diagnostic/configuration modules to Wails.
## Responsibilities
Host Wails options and generated frontend assets; resolve Windows product paths; attach the persisted settings store; construct at most one concrete `internal/application.Application` from explicit normalized configuration; start it before publishing running state; retain any constructed backend for shutdown; close it through Wails shutdown; and bind only Runtime, Plugins, and Settings APIs with module-local latest-only events.
## Non-responsibilities
Backend algorithms, plugin/device protocols, avatar planning, OSC output, and configuration semantics belong to internal packages; root must not duplicate their ownership or invent backend defaults.
## Current implementation
`NewApp` constructs only passive dependencies, API objects, and snapshot stores. Wails `OnStartup` admits one operation, starts event forwarders before settings load so diagnostics remain observable, rejects unsupported platforms into diagnostic mode, loads or creates settings, maps them to an explicit Application config, calls the production `application.NewApp` path once, atomically retains the concrete backend plus its operations seam, and calls `Start` only while startup remains active. Runtime and plugin consumers attach only after successful Start. Every startup failure remains queryable as a sanitized diagnostic, and a constructed failed backend is retained for Close.

`OnShutdown` atomically closes admission and cancels the process context, joins any admitted startup without holding the root lock, stops Plugins/Settings admission, joins consumers and event forwarders, and closes the retained backend at most once using a fixed timeout derived from `context.Background()`. Concurrent shutdown callers share the recorded result, and `closed` is not published until admitted startup ownership has been delivered or exited. `main.go` registers both callbacks and binds exactly `app.runtime`, `app.plugins`, and `app.settings`; the root App itself and template `Greet` are not bound.
## Public/internal interfaces
The Wails-visible surface is only `RuntimeAPI.GetStatus`, `PluginsAPI.List/GetConfig/SetEnabled/UpdateConfig`, and `SettingsAPI.Get/Validate/Save`, with versioned `vrcft:v1:runtime-status`, `vrcft:v1:plugins-changed`, and `vrcft:v1:settings-changed` events. `main`, `App`, lifecycle callbacks, API attach/detach/consumer methods, stores, backend seams, and converters remain unbound implementation details.
## Owned data
Process-wide Wails options, generated frontend assets, injected platform/store/backend/clock/emitter dependencies, the admitted startup operation and process cancellation, zero or one concrete Application, its operations seam, three immutable module stores, event forwarders, and stable shutdown result.
## Dependencies
Depends on `internal/userconfig` for paths and durable settings, `internal/application` for the composed backend and status/operations, existing plugin/API DTO contracts for conversion, Wails runtime events, and generated frontend assets. Product semantics remain owned below root.
## Concurrency and lifecycle
Startup and shutdown are one linearized lifecycle. Shutdown can mark closing and cancel a context-aware admitted boundary while a synchronous backend factory runs outside the root mutex, then joins that operation before reading delivered ownership and closing. A factory has no context by contract and must be synchronously bounded; it is never called under the root lock. API command admission and Settings saves are context-aware, close rejects new work promptly, and close joins admitted work and subscriptions before final publication. All event and backend subscriptions are bounded/latest-only and are canceled and joined before backend Close.
## Error handling
Unsupported platform, invalid/missing environment, settings load/validation/conflict, Application construction/Start, command deadline, unknown plugin, revision conflict, and Close failures map to stable bounded Problems. Plugin caller IDs are valid UTF-8 ASCII `a-z0-9` segments separated by one `.`, `_`, or `-`, and are at most 256 bytes; invalid IDs are rejected before keyed command admission, backend calls, or response echo. Expected failures enter diagnostic mode rather than terminating Wails. Full internal errors remain below the binding; repeated shutdown observes one sanitized recorded outcome.
## Performance constraints
The bootstrap must not perform frame processing.
## Security boundaries
The closed Bind literal contains exactly the three API fields and no root App. DTO conversion allowlists fields, deep-copies slices/JSON, zeros non-finite rates, and uses valid UTF-8 byte limits: plugin ID 256, name 256, description 4096, version 256, last error 512, JSON configuration 64 KiB, and sorted plugin lists 1024 entries. The list cap bounds Wails snapshot/event memory while comfortably exceeding ordinary local plugin inventories; overflow is deterministically omitted after sorting and produces a module validation Problem. Malformed descriptor snapshots are likewise omitted rather than truncated, so IDs are never changed. Capabilities are a fixed allowlisted set. Tracking frames, internal Application configuration, credentials/tokens, process/session identifiers, plugin executable/root paths, raw logs/errors, mutable backend objects, and stores never cross Wails. The sole permitted configuration document is the bounded owned non-credential JSON returned by `PluginsAPI.GetConfig`.
## Required tests
Root normal/race tests cover passive construction, startup diagnostics, one-time construction, ownership delivery, settings and factory boundaries, Start failure, consumer attachment, shutdown-before/during/after startup, cancellation, background timeout, at-most-once Close, module admission, snapshot ownership, bounded sanitization, semantic event coalescing, public plugin exact-limit/+1/invalid-UTF-8 cases, overflow aggregation/revision stability, and post-close suppression. `backend-lifecycle-owned` proves the production NewApp path, concrete field ownership, and Start/Close calls; `wails-lifecycle-callbacks` proves real callback registration; `wails-bind-allowlist` proves the exact three bindings with no App; and `TestM7BackendPrerequisiteInterfacesEndToEnd` exercises first-run persistence, all three module events, immediate plugin mutations, restart-required settings save, and idempotent shutdown together.
## Known gaps
The non-frontend M7 root prerequisites are implemented and evidenced. The untouched frontend still lacks diagnostics/configuration UX, dependency installation, generated bindings, type-check/build evidence, and the project-status view; therefore M7 remains blocked and is not complete.
## Completion definition
The root constructs one explicitly configured Application, starts it before presenting the UI, closes it reliably through Wails lifecycle callbacks, and exposes only the approved M7 frontend surface.
