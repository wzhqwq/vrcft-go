# Repository Working Guide

## Start Here

- Read `README.md` for the repository entry points and common commands.
- Read `docs/project/README.md` for the authoritative package and subsystem map.
- Read `docs/project/milestones.md` for dependency order and completion criteria.
- Read `docs/project/status.md` for generated evidence and current blockers. Treat it as generated output, not as the source of requirements.
- Package ownership, responsibilities, dependencies, and executable checks live in `docs/project/packages/`.
- Cross-package subsystem contracts live in `docs/project/subsystems/`.
- Approved designs live in `docs/superpowers/specs/`; implementation plans live in `docs/superpowers/plans/`.

Use `rg --files` to locate files and `rg '<symbol-or-term>'` to trace code or requirements. Before editing, inspect `git status --short` and preserve unrelated user changes.

## Fixed Go Build Cache

All Go build, test, vet, generate, and run commands must reuse the repository-local cache at `.go-gocache/`. `GOCACHE` must be an absolute path.

Initialize it once in each PowerShell session from the repository root:

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim()
$env:GOCACHE = Join-Path $repoRoot '.go-gocache'
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
```

For a POSIX shell:

```sh
repo_root=$(git rev-parse --show-toplevel)
export GOCACHE="$repo_root/.go-gocache"
mkdir -p "$GOCACHE"
```

Keep this cache between commands, tasks, and sessions. Do not run `go clean -cache` and do not delete `.go-gocache/` during ordinary builds, test failures, branch changes, or task cleanup.

Clear only `.go-gocache/` when a reproducible Go command reports cache corruption or the same cache-related compilation error persists after one retry. Record the triggering error before clearing it. Never clear the global Go module cache as a substitute; `GOMODCACHE` is outside this project cache policy.

## Working With Project Evidence

- Run the narrowest relevant package checks while developing, then the plan- or package-spec-required checks before completion.
- Set the fixed `GOCACHE` above before every `go test`, `go vet`, `go generate`, `go run`, or build command.
- Use `go run ./cmd/projectstatus` for a fresh terminal report.
- Use `go run ./cmd/projectstatus -write` only when intentionally refreshing `docs/project/status.md` from a clean reviewed source commit.
- Use `go run ./cmd/projectstatus -check` to detect stale generated status.
- Generated Go files must be changed through their repository generator; inspect the resulting diff for unrelated changes.

When requirements disagree, direct user instructions govern, followed by the approved design, implementation plan, package/subsystem specifications, and generated status in that order.
