# Diagnostics and local logging

Approved in conversation on 2026-09-10, including compact frontend layout and localized timestamps.

The existing Runtime, Plugins and Settings bindings remain the only bound objects. Runtime gains an independent `GetDiagnostics()` snapshot and bounded `ReportFrontendError(stage, message)` command. Diagnostics must remain readable when Runtime status conversion fails. Normalize empty plugin failure lists to JSON arrays and accept legacy null lists in the frontend.

Root owns a passive slog-backed diagnostic store, initialized on startup before environment/settings/backend work. It records startup boundaries, failures, backend status changes, plugin failures and shutdown, with timestamp, level, component, stage and correlation ID. Production writes JSON lines to `%APPDATA%/vrcft-go/logs`, rotating at 5 MiB with five files total. A bounded 200-entry memory buffer remains available if disk initialization or writes fail. Failures include a bounded redacted error chain; no configuration documents or tracking frames are logged. Credentials and user profile paths are redacted before storage or display.

The diagnostics response is `{entries: DiagnosticEntry[], failure?: DiagnosticEntry, logPath: string, diskError: string}`. Each entry is `{id, time, level, component, stage, message}` (strings). Entries are chronological. The startup failure is retained separately even after the ring rolls over. Frontend errors use allowlisted stages, bounded messages and rate limiting. Log reads remain independent of runtime events and polling occurs only while the diagnostic page is mounted.

The page uses compact module rows, visible error detail and a bounded scrollable recent-log list with level filtering, refresh and copy. Copy includes explicit diagnostic fields and errors, not complete raw snapshots. Time uses `Intl.DateTimeFormat('zh-CN')` in the operating system timezone, with safe missing/invalid fallbacks. Shared spacing and detail rows become denser while controls remain usable at 640px window width.

Validation covers null-list wire compatibility, startup error correlation and redaction, disk persistence/rotation/unwritable fallback, lifecycle concurrency, frontend request versus parsing errors, independent log retrieval, log filtering/copy and local dates. Run root Go tests/race/vet, changed internal-package tests, frontend test/check/build and available responsive browser checks. Keep unrelated generated frontend changes intact.
