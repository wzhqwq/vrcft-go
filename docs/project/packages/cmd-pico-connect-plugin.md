---
id: cmd-pico-connect-plugin
kind: go-package
path: cmd/pico-connect-plugin
milestone: M2
depends_on: [internal-picoconnect, pkg-pluginruntime]
checks:
  - id: package-tests
    description: PICO Connect plugin command and manifest tests pass
    type: command
    command: go-test
    args: [./cmd/pico-connect-plugin]
    weight: 2
    required: true
  - id: command-entry
    description: PICO Connect plugin executable exposes main
    type: symbol
    path: cmd/pico-connect-plugin/main.go
    pattern: 'func main\('
    weight: 1
    required: true
---
# Package: cmd/pico-connect-plugin

## Purpose

Start the managed PICO Connect tracking plugin process.

## Responsibilities

Create the driver and run it through `pluginruntime.Main`.

## Non-responsibilities

Transport, mapping, and host IPC internals belong to the adapter and runtime packages.

## Current implementation

The command passes `picoconnect.New()` to the runtime. Its manifest matches the descriptor.

## Public/internal interfaces

Executable entry point and plugin runtime environment contract.

## Owned data

No persistent state.

## Dependencies

`internal/picoconnect` and `pkg/pluginruntime`.

## Concurrency and lifecycle

The runtime owns process lifecycle.

## Error handling

Runtime startup errors print to standard error and cause nonzero exit.

## Performance constraints

No extra workers beyond the driver.

## Security boundaries

Session credentials remain in `pkg/pluginruntime`.

## Required tests

Source manifest validation and descriptor alignment.

## Known gaps

Protocol 0 and hardware acceptance remain open in the adapter package.

## Completion definition

The executable runs the adapter under the managed runtime with a matching source manifest.
