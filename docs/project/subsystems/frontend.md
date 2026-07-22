---
id: frontend
kind: frontend
path: frontend
milestone: M7
depends_on: [internal-application]
checks:
  - id: type-check
    description: Svelte and TypeScript checks pass
    type: command
    command: frontend-test
    weight: 3
    required: true
  - id: production-build
    description: Frontend production build passes
    type: command
    command: frontend-build
    weight: 2
    required: true
  - id: project-status-view
    description: UI exposes project and runtime status
    type: symbol
    path: frontend/src/App.svelte
    pattern: 'Project Status|projectStatus'
    weight: 2
    required: true
---
# Subsystem: frontend

## Purpose
Provide the desktop user interface for setup, routing, diagnostics, and status.
## Responsibilities
Display plugins, sources, avatar, subscriptions, OSC health, errors, performance, and project/runtime status.
## Non-responsibilities
Business rules and hardware/network access remain in Go services.
## Current implementation
A Wails Svelte template and generated bindings exist.
## Public/internal interfaces
Wails-bound application methods and frontend view models.
## Owned data
Ephemeral UI state and user input before backend validation.
## Dependencies
Depends on `internal/application` APIs.
## Concurrency and lifecycle
Reactive UI subscriptions are disposed with components and application shutdown.
## Error handling
Backend failures are rendered with actionable context and retry state.
## Performance constraints
High-frequency data is summarized or throttled before rendering.
## Security boundaries
No arbitrary filesystem/process access is implemented in frontend code.
## Required tests
Svelte checks, production build, component behavior, and Wails contract tests.
## Known gaps
Product views and status presentation are not implemented.
## Completion definition
Users can configure and diagnose every supported product workflow.
