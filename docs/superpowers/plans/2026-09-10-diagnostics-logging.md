# Diagnostics and Logging Implementation Plan

> Use superpowers:subagent-driven-development for the independent frontend task and integrated review.

**Goal:** Make startup failures actionable and retain bounded local logs in a compact localized diagnostics UI.
**Architecture:** Root owns slog and an independent diagnostic snapshot; Runtime status keeps existing compatibility. The frontend reads diagnostics separately and handles malformed status without losing logs.
**Tech Stack:** Go standard log/slog and JSON, Wails v2, Svelte 5, TypeScript, Vitest.
**Spec:** `docs/superpowers/specs/2026-09-10-diagnostics-logging-design.md`

## Constraints

- Fixed absolute repository `.go-gocache` for every Go command.
- Five log files total, 5 MiB each, 200 recent memory entries, bounded redacted text.
- Preserve existing user modifications to generated models and package checksum.
- Use existing workspace to keep the user's current generated bindings available; no commits or branch changes required for this delivery.

## Tasks

- [x] Backend logging: add `diagnostics.go` and `diagnostics_test.go`. Test startup errors with `errors.New("listen udp :9001: address already in use")`; assert stage `backend_start`, a nonempty correlation ID, JSON persistence and unchanged generic Problem contract. Test rollover with a small injected byte limit and a file used as the log directory to force memory fallback. Implement synchronized slog output, bounded snapshots, redaction and rotation.
- [x] Lifecycle: integrate initialization, boundaries, failure and shutdown in `app.go`; add `RuntimeAPI.GetDiagnostics()` and `ReportFrontendError(stage, message)` using the diagnostic store. Record changed runtime/OSC/plan/plugin failures without frame logs. Preserve `pluginFailures: []` through snapshot cloning; verify JSON and frontend null compatibility.
- [x] Frontend: modify runtime ports/types/module and diagnostics page; add local-time presentation helper; distinguish request/parse/event failures and query logs independently. Add page tests for visible/copied details, level filtering and timestamps. Tighten shared layout spacing. Root generates bindings after adding methods; do not edit generated bindings manually.
- [x] Integration: update package/subsystem documentation for the deliberate diagnostic surface expansion. Run root tests/race/vet and frontend tests/check/build; review changes and run responsive browser checks where available.

## Execution ledger

- Design approved in conversation; compact layout and local timestamps included without another approval gate.
- Suspected snapshot failure: nil Go plugin failure slice serializes as null and frontend calls `.map()` directly. Add regression coverage before changing behavior.

- Validation: Go full suite passed outside the IPC-restricted sandbox; root and Application race tests and vet passed. Frontend full suite passed 175 tests; final redaction fixes passed 39 targeted tests; Svelte check and production build passed. Final Playwright suite: 17 passed. Windows Wails production build succeeded at build/bin/vrcft-go2.exe.
- Review: backend and frontend credential/path redaction findings fixed with failing-first regressions; scoped re-review approved. Generated model whitespace predates this work and remains generator-owned.
