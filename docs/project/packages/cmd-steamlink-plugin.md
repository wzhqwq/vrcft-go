---
id: cmd-steamlink-plugin
kind: go-package
path: cmd/steamlink-plugin
milestone: M2
depends_on: [internal-steamlink, pkg-pluginruntime]
checks:
  - id: package-tests
    description: Steam Link plugin command and manifest contract tests pass
    type: command
    command: go-test
    args: [./cmd/steamlink-plugin]
    weight: 2
    required: true
  - id: command-entry
    description: Steam Link plugin command exposes main
    type: symbol
    path: cmd/steamlink-plugin/main.go
    pattern: 'func main\('
    weight: 1
    required: true
---
# Package: cmd/steamlink-plugin

## Purpose
Start the managed Steam Link tracking plugin process.

## Responsibilities
Create the Steam Link driver, run it through `pluginruntime.Main`, write runtime startup failures to standard error, and exit nonzero on failure.

## Non-responsibilities
OSC reception, mapping, lifecycle behavior, manifest discovery, and host IPC protocol handling belong to the adapter, catalog, and runtime packages.

## Current implementation
The command creates `steamlink.New()` and passes it to `pluginruntime.Main`. Its source manifest declares the matching Steam Link descriptor and `steamlink-plugin.exe` entrypoint.

## Public/internal interfaces
The executable uses the environment variables required by `pkg/pluginruntime`: `VRCFT_PIPE_NAME` and `VRCFT_SESSION_TOKEN`.

## Owned data
The command owns no persistent data.

## Dependencies
Depends on `internal/steamlink` and `pkg/pluginruntime`.

## Concurrency and lifecycle
The plugin runtime owns the process lifecycle after command startup.

## Error handling
Missing or invalid runtime environment values are reported on standard error and cause a nonzero exit.

## Performance constraints
Startup delegates directly to the runtime without additional workers or buffering.

## Security boundaries
Session credentials remain owned and validated by `pkg/pluginruntime`; the command does not log them.

## Required tests
Command tests validate the source manifest and its descriptor alignment. The adapter architecture test rejects non-public production imports.

## Known gaps
Real process and catalog integration is covered by later Steam Link adapter tasks.

## Completion definition
The executable starts the Steam Link driver under the managed plugin runtime and the source manifest remains aligned with the driver descriptor.
