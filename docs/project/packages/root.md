---
id: root
kind: go-package
path: .
milestone: M7
depends_on: [internal-application]
checks:
  - id: package-builds
    description: Root application builds
    type: command
    command: go-test-build
    args: [.]
    weight: 2
    required: true
  - id: wails-config
    description: Wails configuration artifact exists
    type: file
    path: wails.json
    weight: 1
    required: true
  - id: backend-lifecycle-wired
    description: Root constructs and owns the Application lifecycle through Wails callbacks
    type: symbol
    path: app.go
    pattern: '(?s)type App struct\s*\{.*backend\s+\*application\.Application.*func \(a \*App\) startup\(ctx context\.Context\) \{.*a\.backend, err := application\.NewApp\(.*\).*a\.backend\.Start\(ctx\).*func \(a \*App\) shutdown\(ctx context\.Context\) \{.*a\.backend\.Close\(ctx\)'
    weight: 3
    required: true
blockers:
  - check: package-builds
    blocks: [M7]
  - check: backend-lifecycle-wired
    blocks: [M7]
---
# Package: root

## Purpose
Provide the M7 Wails executable entry point and eventually bridge its process lifecycle to the composed backend.
## Responsibilities
Host Wails options and generated frontend assets. M7 must construct `internal/application.Application` from explicit persisted configuration, start it before frontend readiness, close it through Wails shutdown, and bind only intended diagnostics/configuration APIs.
## Non-responsibilities
Backend algorithms, plugin/device protocols, avatar planning, OSC output, and configuration semantics belong to internal packages; root must not duplicate their ownership or invent backend defaults.
## Current implementation
The Wails template constructs only its local `App`, records the Wails startup context, and binds `Greet`. It does not import or construct `internal/application.Application`, provide its explicit configuration, start the backend, register a Wails shutdown callback, or close backend services.
## Public/internal interfaces
`main`, local `App`, and the Wails bootstrap. Backend lifecycle and frontend-facing operations remain to be designed and implemented for M7.
## Owned data
Process-wide Wails options, generated frontend assets, and the Wails context. It does not yet own a composed backend instance or persisted configuration.
## Dependencies
The planned M7 bridge depends on `internal/application` and generated frontend assets. The current template uses the assets but does not yet use the Application dependency.
## Concurrency and lifecycle
Wails currently owns only template lifecycle callbacks. M7 must make its startup and shutdown callbacks own one Application instance, invoke `Start` before UI readiness, and invoke bounded `Close` during shutdown.
## Error handling
The template surfaces only `wails.Run` failure. M7 must surface Application construction and startup failures before the UI is considered ready and report bounded shutdown failures without silently abandoning services.
## Performance constraints
The bootstrap must not perform frame processing.
## Security boundaries
Only explicitly bound M7 diagnostics/configuration methods may be exposed to the frontend; the backend must not expose tracking payloads, credentials, or raw configuration through the template binding.
## Required tests
The root build proves that the current executable compiles, and the file check proves that the Wails configuration artifact exists. `backend-lifecycle-wired` is required source evidence for the future root bridge: `app.go` must construct `internal/application.Application`, start it in the Wails startup callback, and close it in a Wails shutdown callback.
## Known gaps
M6 backend composition is complete below root. M7 still needs real root Wails construction and lifecycle wiring, persisted configuration/path selection, frontend diagnostics/configuration UX, and release integration.
## Completion definition
The root constructs one explicitly configured Application, starts it before presenting the UI, closes it reliably through Wails lifecycle callbacks, and exposes only the approved M7 frontend surface.
