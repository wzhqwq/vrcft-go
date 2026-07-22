---
id: internal-osc
kind: go-package
path: internal/osc
milestone: M6
checks:
  - id: package-tests
    description: OSC tests pass
    type: command
    command: go-test
    args: [./internal/osc]
    weight: 1
    required: true
---
# Package: internal/osc

## Purpose
OSC integration.
