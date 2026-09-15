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
    path: frontend/src/pages/DiagnosticsPage.svelte
    pattern: 'Project Status|projectStatus'
    weight: 2
    required: true
---
# Subsystem: frontend

## Purpose
Provide the accessible desktop control center for runtime status, plugin operations, restart-required settings, and safe diagnostics.
## Responsibilities
Own the Overview, Plugins, Settings, and Diagnostics pages; present bounded Runtime, Plugins, and Settings module state; collect typed settings drafts; and render safe runtime, avatar-plan, OSC, plugin, and project-status information.
## Non-responsibilities
Business rules, hardware and network access, filesystem and process access, avatar planning, plugin-private configuration editing, and persistence remain in the bound Go services.
## Current implementation
`App.svelte` creates and independently starts/disposes Runtime, Plugins, and Settings modules, owns the active page, and protects a dirty Settings draft before top-level navigation. `OverviewPage.svelte` presents runtime, avatar, OSC, and plugin summaries and directs users to configure a fallback file when the current avatar has no configuration; `PluginsPage.svelte` filters, paginates, and controls plugins; `SettingsPage.svelte` owns the explicit settings form and restart-required save flow; and `DiagnosticsPage.svelte` owns `Project Status`, module readiness/revision/problem summaries, runtime diagnostics, and bounded safe-copy output. Processing settings use one selector for the default profile and channel-specific profiles, clone the default profile when adding an override, and present calibration, response, smoothing, and signal-loss controls as vertical tabs.
## Public/internal interfaces
Pages consume typed module view state and commands. `lib/wails/production.ts` is the direct adapter boundary for generated Wails RuntimeAPI, PluginsAPI, SettingsAPI, and runtime events; the adapters expose typed ports to the modules rather than allowing pages or reusable components to call generated bindings.
## Owned data
Each Runtime, Plugins, and Settings module owns its own immutable snapshot/view state, revision, update time, loading/stale/problem state, event subscription, and disposal lifecycle. Settings normalizes nullable Go collections into an owned non-null form candidate and owns its independently cloned draft, dirty state, validation state, save availability, and save operation; Plugins scopes command state to each plugin. Overview selects at most four important plugin states from the Plugins snapshot independently of the plugin list's filter and Runtime control failures.
## Dependencies
Depends on the three versioned Wails APIs backed by `internal/application` and the generated Wails bindings. Dependency flow is pages and `AppShell`, then project UI/layout components and patterns, then typed modules, then `lib/wails` adapters and generated bindings.
## Concurrency and lifecycle
The three modules start independently and each owns one invalidation subscription. An event refreshes only its owning module; modules reject stale revisions, retain the last valid snapshot after refresh failure, and dispose their subscriptions when the application unmounts.
## Error handling
Runtime distinguishes request, parsing, revision and subscription failures. Null legacy plugin-failure arrays normalize to empty arrays. Diagnostics fetches recent logs independently of Runtime status decoding and polls only while the page is mounted. The compact module rows show problem details; retained startup failure and bounded filtered logs can be copied. Displayed and copied error text is redacted, and timestamps use Chinese formatting in the system timezone.

Initial module failures render no-data problem states, while later failures retain and label the last valid state as stale. An OSC discovery connection failure with no active connection or output target is presented as stopped rather than as an active output error; errors after an output connection exists remain visible. Settings surfaces field, validation, conflict, and save outcomes without discarding a dirty draft; plugin failures remain isolated to the affected card; diagnostics exposes bounded sanitized problems and empty or unsupported states.
## Performance constraints
The UI renders bounded status snapshots rather than tracking frames, paginates plugin lists, and keeps module refreshes independent so one event does not trigger a global refresh.
## Security boundaries
Only the typed Wails port adapters call generated bindings. Pages and reusable UI/pattern components do not import generated Wails code, and diagnostics excludes credentials, plugin-private JSON, process/session identifiers, executable paths, mutable backend objects, and raw internal errors.
## Required tests
Vitest covers Wails ports and DTO mapping, independent module lifecycle/revision behavior, Settings draft and validation behavior, reusable UI/layout/pattern accessibility and interaction contracts, and all four pages. Playwright covers deterministic mocked Wails workflows and responsive behavior at the supported floor and desktop viewports. `pnpm` test, browser, Svelte check, and production-build gates provide frontend evidence; Go tests, vet, and the Wails build verify the integration boundary.
## Known gaps
Final source review and the generated-evidence refresh remain separate completion gates. Generated project status stays read-only until the reviewed source is intentionally refreshed through its generator.
## Completion definition
Users can navigate the four pages, inspect runtime and plugin state, make immediate plugin changes, edit and save restart-required settings with dirty-draft protection, and inspect safe project/runtime diagnostics through independently refreshed modules. The UI remains usable from the `640x480` floor: a left navigation rail is used at desktop widths, while narrower windows use a horizontal top tab bar above page content without page-level horizontal overflow.
