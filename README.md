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
