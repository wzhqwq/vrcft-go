# README

## Project specifications and status

- [Project architecture and package specifications](docs/project/README.md)
- [Milestones](docs/project/milestones.md)
- [Generated project status](docs/project/status.md)

Refresh the evidence-backed status with:

```powershell
go run ./cmd/projectstatus
go run ./cmd/projectstatus -write
go run ./cmd/projectstatus -format json
go run ./cmd/projectstatus -check
```

## About

This is the official Wails Svelte-TS template.

## Live Development

To run in live development mode, run `wails dev` in the project directory. This will run a Vite development
server that will provide very fast hot reload of your frontend changes. If you want to develop in a browser
and have access to your Go methods, there is also a dev server that runs on http://localhost:34115. Connect
to this in your browser, and you can call your Go code from devtools.

## Building

To build a redistributable, production mode package, use `wails build`.

## Diagnostics and logs

The 诊断 page shows module errors, the failed startup stage and correlation ID,
and recent logs with level filtering. Update times use the system timezone and
Chinese date/time formatting. Copy diagnostic information when reporting a
startup problem; status decoding failures do not prevent querying logs.

Windows writes redacted JSON-line logs to
`%APPDATA%\vrcft-go\logs\application.jsonl`. Rotation retains this file and four
backups, up to 5 MiB each. The page retains the latest 200 records for the current
process and the startup failure. If disk logging fails, recent memory logs remain
available and the page reports the disk error. High-volume logging uses a bounded
disk queue and reports omitted records rather than blocking frame processing.
