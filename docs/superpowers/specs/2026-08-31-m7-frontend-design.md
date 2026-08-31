# M7 Frontend Design

**Date:** 2026-08-31
**Status:** Draft for written-spec review; implementation plan pending
**Milestone:** M7 — Frontend, operations, and release

## Context

The repository currently contains the minimal Wails Svelte TypeScript template.
The M7 backend prerequisites expose three separately bound, versioned modules:

- `RuntimeAPI`, with `vrcft:v1:runtime-status`;
- `PluginsAPI`, with `vrcft:v1:plugins-changed`; and
- `SettingsAPI`, with `vrcft:v1:settings-changed`.

The frontend must turn these interfaces into a desktop control center for
runtime status, plugin operations, construction settings, and diagnostics.
Business rules, filesystem access, process access, plugin-private JSON, and
hardware/network operations remain below the Wails boundary.

This design replaces the template UI. It also authorizes one narrow backend
extension: preserve the optional name from the current VRChat avatar
configuration and expose it in the bounded Runtime DTO. No other backend
surface expansion is part of this design.

## Goals

1. Build an accessible Svelte 5 desktop UI using Tailwind CSS v4 and Bits UI.
2. Develop and verify reusable components before composing product pages.
3. Make every component elastically sized and every layout responsive down to
   the supported `640x480` minimum window.
4. Keep Runtime, Plugins, and Settings state independent so one module update
   does not cause a global application refresh.
5. Give users immediate visibility into the current avatar, OSC discovery and
   output target, plugin health, runtime problems, and saved settings.
6. Keep construction settings explicit, typed, and hard-coded as product
   forms, with clear restart-required behavior.
7. Provide deterministic component, interaction, and responsive acceptance
   tests.

## Non-goals

- A light theme or runtime theme switch in M7 v1.
- A generic schema-driven form renderer.
- A raw JSON editor for plugin-private configuration.
- Plugin-private configuration forms before concrete plugin-specific fields
  are defined.
- URL routing, browser history, or a routing library.
- A separate Avatar page, raw log page, or advanced performance page.
- Arbitrary filesystem or process access from frontend code.
- Rendering tracking frames at frame rate.

## Product and Visual Direction

The approved visual direction is an adaptive dark workbench: technical enough
for diagnosis, but calmer and less dense than an engineering console. The
first visual layer shows facts users need during normal operation. Additional
diagnostic detail expands progressively rather than dominating every page.

M7 v1 has one complete dark theme. All colors, spacing, typography, radii,
shadows, transitions, and status roles use semantic theme tokens so a later
light theme can replace token values without changing component markup. No
theme preference or persistence logic is implemented in this milestone.

The interface language is Simplified Chinese. Page and business strings live
in a typed copy module instead of being scattered across components. Generic
components receive labels and descriptions through props or snippets and do
not contain product-specific text. This boundary permits a future i18n system
without introducing an i18n runtime in M7.

## Supported Window and Responsive Model

The default Wails window remains `1024x768`. The implementation adds a Wails
minimum size of `640x480`, which is also the formal responsive acceptance
floor.

- At viewport widths of `720px` and above, `AppShell` uses a left navigation
  rail and a flexible main content region.
- Below `720px`, the left rail becomes a horizontal Tab Bar beneath the
  application header and above the page title/content.
- The narrow Tab Bar is never placed at the bottom of the window. It distributes
  items evenly when possible and becomes horizontally scrollable before text
  is compressed or replaced by icon-only navigation.
- Reusable components respond primarily to their own container width. The
  Shell owns the viewport breakpoint; cards, form rows, summaries, and action
  groups use container queries and wrapping behavior.
- Every flex/grid child that may contain user or backend text handles
  `min-width: 0`. Long IDs and paths use intentional wrap, ellipsis plus a
  detail/copy action, or a scrollable code surface.
- Form controls fill their available container width. Action groups wrap.
- Pages may scroll vertically. Page-level horizontal overflow is a failure.

Responsive acceptance covers `640x480`, `720x600`, `1024x768`, and one wider
desktop viewport.

## Technology and Integration

The frontend retains Svelte 5, TypeScript, Vite, and pnpm. It adds:

- `tailwindcss` v4;
- `@tailwindcss/vite`;
- `bits-ui`;
- `lucide-svelte`;
- Vitest and Svelte Testing Library dependencies; and
- Playwright.

Tailwind follows the official Vite integration:

1. register `@tailwindcss/vite` alongside the Svelte Vite plugin; and
2. import Tailwind from the application stylesheet with
   `@import "tailwindcss"`.

No legacy `tailwind.config.js` is introduced unless a later requirement cannot
be expressed through Tailwind v4's CSS-first configuration.

Bits UI is a headless Svelte 5 primitive library. Product pages do not import
Bits UI directly. Project-owned UI components wrap its primitives, styling
hooks, state data attributes, and CSS variables. This keeps accessibility and
interaction behavior centralized while allowing the product visual system to
remain independent of Bits UI markup details.

Lucide is the only general icon source. Pages do not copy arbitrary inline SVG.
Decorative icons are hidden from assistive technology. Icon-only actions use a
project `IconButton` with a Chinese accessible name and Tooltip.

Authoritative integration references:

- Bits UI LLM index: <https://bits-ui.com/llms.txt>
- Bits UI getting started: <https://bits-ui.com/docs/getting-started/llms.txt>
- Bits UI styling: <https://bits-ui.com/docs/styling/llms.txt>
- Tailwind v4 Vite installation:
  <https://tailwindcss.com/docs/installation/using-vite>
- Tailwind directives, including `@apply` and `@reference`:
  <https://tailwindcss.com/docs/functions-and-directives>

## Directory and Dependency Architecture

The intended source structure is:

```text
frontend/src/
  App.svelte
  style.css
  copy/
    zh-CN.ts
  lib/
    components/
      ui/
      layout/
    patterns/
    modules/
      runtime/
      plugins/
      settings/
    wails/
    test/
  pages/
    OverviewPage.svelte
    PluginsPage.svelte
    SettingsPage.svelte
    DiagnosticsPage.svelte
  dev/
    ComponentWorkbench.svelte
```

Dependencies flow in one direction:

```text
AppShell and pages
        |
        v
patterns and project UI/layout components
        |
        v
Runtime / Plugins / Settings typed frontend modules
        |
        v
generated Wails bindings and Wails runtime events
```

Rules:

- `App.svelte` creates the three modules, starts/disposes them, owns the active
  page ID, and composes `AppShell`. It does not contain product page markup.
- Only `lib/wails` and module adapters import generated `frontend/wailsjs`
  bindings or know event names.
- Pages consume typed module view state and commands. They do not call Wails
  methods directly.
- UI/layout components contain interaction and presentation only. They do not
  import module state.
- Patterns combine project UI components into reusable product-neutral
  arrangements. They may understand stable view model shapes but not Wails
  DTOs.
- No mutable mega-store combines all three modules.
- The four page IDs are a TypeScript union. A router dependency is unnecessary
  for this desktop application.

## Frontend Module Contract

Each module owns:

- a typed immutable snapshot/view model;
- loading, ready, stale, and problem state;
- the last accepted module revision and update time;
- an initial query;
- exactly one Wails event subscription;
- command state scoped to the resource being changed; and
- an idempotent disposal operation.

Application startup initializes Runtime, Plugins, and Settings concurrently.
An event is only an invalidation hint: the receiving module re-queries its own
API, and ignores a response older than the currently accepted revision. An
event never causes another module to refresh.

If a refresh fails after a valid snapshot exists, the module retains the last
valid snapshot, marks it stale, and exposes the bounded Problem and last update
time. Initial failure produces a no-data problem state.

Runtime has no mutations in M7. Plugins tracks pending/error state by plugin ID
so one command never blocks unrelated cards. Settings owns the server snapshot,
an independently cloned draft, dirty state, field problems, validation state,
and one save operation.

Module state is implemented with Svelte 5-compatible reactive TypeScript. The
module's public contract remains framework-small and testable without mounting
a complete page.

## Component System

### UI components

`components/ui` owns the reusable interactive vocabulary:

- `Button` and `IconButton`;
- `TextField` and `NumberField`;
- `SelectField` and `SwitchField`;
- `Tabs`;
- `Dialog`;
- `Tooltip`;
- `Badge`;
- `Spinner`; and
- `Separator`.

Bits UI primitives are wrapped here where they add accessible interaction or
state management. Native elements remain appropriate where a native control is
more direct. Every form component has a stable label, description, error,
disabled, and required contract. Form components generate or accept stable IDs
and associate messages through ARIA attributes.

### Layout components

`components/layout` owns reusable spatial rules:

- `AppShell`;
- `PageHeader`;
- `ResponsiveGrid`;
- `Stack`;
- `Inline`;
- `ScrollablePanel`.

Every layout component accepts an external class, avoids hard-coded page
widths, and defines explicit shrink/wrap/overflow behavior.

### Patterns

`patterns` owns arrangements shared by multiple pages:

- `StatusCard`;
- `AvatarSummary`;
- `OSCSummary`;
- `PluginCard`;
- `FormSection` and `FormRow`;
- `ProblemBanner`;
- `EmptyState`;
- `DetailList`;
- `UnsavedChangesBar`.

When a page needs a reusable control or arrangement that is missing, work stops
at the page boundary. The component or pattern is added, tested, and exercised
in the workbench before page composition resumes.

### Component workbench

`dev/ComponentWorkbench.svelte` is reachable only in development mode. It
shows normal, hover/focus guidance, disabled, loading, empty, error, long-text,
and narrow-container examples. It is not shipped as a production navigation
destination. Storybook is not introduced.

## Styling Rules

`style.css` owns:

- the Tailwind import;
- `@theme` semantic tokens;
- font faces and base typography;
- application background and base element behavior;
- focus-ring and reduced-motion rules;
- shared animations; and
- public semantic component classes.

Long, public, semantically meaningful utility combinations may be expressed in
the main stylesheet with `@apply`. Expected examples include:

- `.surface-card`;
- `.form-control`;
- `.status-pill`;
- `.focus-ring`;
- `.page-grid`.

An `@apply` class must have more than one consumer or represent a deliberate
cross-component contract. One-off page layouts and short utility sets remain in
Svelte markup. Global classes are not created merely to shorten a single file.

If a Svelte scoped style needs Tailwind tokens or `@apply`, it must use
`@reference` to the main stylesheet. Reusable public styling stays global when
possible. Bits UI states use documented `data-state`, `data-disabled`,
mount-transition attributes, and public CSS variables. Selectors must not
depend on undocumented internal descendants.

Tailwind class names are complete static strings. Runtime concatenation of
partial class names is prohibited because it defeats source detection.

## Accessibility and Motion

- Keyboard focus is always visible.
- Tabs, Dialogs, Selects, Switches, and Tooltips retain Bits UI keyboard and
  focus behavior through project wrappers.
- Opening a Dialog moves focus appropriately; closing restores focus to the
  invoking control.
- Status is never encoded by color alone. Text and, where useful, an icon
  accompany color.
- Body text, controls, focus indicators, and error messages target WCAG AA
  contrast.
- The interface respects `prefers-reduced-motion`. Animation communicates a
  transition but never delays completion or blocks input.
- Controls preserve a practical pointer target without assuming a touch-first
  bottom navigation model.

## Page Design

### Overview

The Overview is a runtime dashboard, not a welcome page.

First visual layer:

- current Avatar name;
- full Avatar ID through detail/copy behavior;
- application phase and primary Problem;
- OSC discovery state; and
- current OSC output target `host:port`.

The UI labels OSC states distinctly: not running, discovering, discovered,
manual target, and discovery error. The port shown is the current output target
port. Local randomly assigned listener and OSCQuery WebSocket ports are not
displayed.

Second layer:

- plan state, source, generation, and configuration readiness;
- active/total/problem plugin summary;
- plugin cards for important current states; and
- bounded runtime/plugin failure summaries.

At wide sizes Avatar and OSC summaries sit side by side. At narrow sizes they
stack beneath the top Tab Bar. Long identifiers never widen the page.

### Plugins

The Plugins page provides search, state filtering, status inspection, and
immediate enable/disable commands.

- Problems and recovery states sort ahead of healthy plugins, then by display
  name and ID.
- A card shows display name, ID, allowlisted capabilities, enabled/active
  state, lifecycle state, frame rate, restart count, and bounded last error.
- A mutation disables only the target plugin control.
- Failure retains the current snapshot and displays a card-level Problem.
- The frontend paginates the bounded list instead of mounting all 1024 entries
  simultaneously.
- Plugin-private configuration editing is absent in M7. It will be added only
  when a concrete plugin has a known, dedicated, componentized form.

### Settings

The Settings page is a hard-coded, explicit product form. It uses horizontal
internal Tabs for `常规`, `处理`, and `OSC`; these remain below the page header
and scroll horizontally when narrow.

`常规` contains:

- Avatar OSC root path;
- fallback Avatar configuration path; and
- the plugin development-root list.

`处理` contains:

- default channel calibration, tuning, filter, and dropout controls;
- explicit channel overrides; and
- mutual-exclusion groups.

Complex repeated sections use componentized Accordion/Collapsible patterns,
but the fields themselves are explicitly implemented. No schema renderer is
introduced.

`OSC` contains:

- automatic/manual target mode;
- preferred discovered service in automatic mode; and
- manual host and port in manual mode.

Fields irrelevant to the selected mode are disabled and explained. Draft state
survives internal Tab changes. A sticky `UnsavedChangesBar` stays visible
within the page content region.

Editing updates only the local draft. Blur-level validation may call
`SettingsAPI.Validate`; Save always validates again. Save uses the current
module revision. Success replaces the server snapshot and displays the
persistent message `已保存，将在重启后生效`. A revision conflict preserves the
draft and offers an explicit reload action; it never overwrites silently.

Changing the top-level page or closing the window with a dirty draft prompts
the user. Plugin enable/disable remains immediate and is not part of this
restart-required settings flow.

### Diagnostics

Diagnostics contains a `Project Status` view built from the three frontend
module states. It shows each module's readiness/staleness, revision, update
time, and Problem, followed by:

- Application lifecycle and platform support;
- Avatar plan status, source, configuration, and error;
- OSC status, target mode, target, and last error;
- plugin control failures; and
- actionable empty/unsupported/startup-failure states.

Users may copy bounded safe diagnostics. Credentials, plugin-private JSON,
process/session IDs, executable paths, mutable backend objects, and raw internal
errors remain unavailable.

## Minimal Avatar Name Backend Extension

The current Avatar configuration decoder validates an optional `name` but
discards it. The Overview requirement needs a stable current display name.

Implementation extends the existing owned status path:

```text
avatar config optional name
  -> decoded config / plan
  -> application.Status.AvatarName
  -> RuntimeApplicationDTO.avatarName
  -> runtime frontend view model
```

The name receives the same bounded, valid-UTF-8 public treatment as other
Runtime text. Absence is valid. The UI falls back to Avatar ID and does not
derive a name from a path or identifier. This field is diagnostic only and
does not affect planning, identity comparison, persistence, or OSC behavior.

OSC discovery state is derived from existing bounded Runtime fields. The UI
does not require service instance names or local listener ports, so no OSC DTO
extension is authorized.

## Forms and Validation

Form controls expose values in frontend-friendly typed units. Conversion to
the exact Wails Candidate shape occurs in the Settings module. Field paths from
`Problem.field` map through one central table to controls and section-level
summaries.

Validation behavior:

- frontend checks provide immediate required/range/format guidance;
- the backend remains authoritative;
- invalid backend responses are shown as module Problems, not trusted as form
  values;
- Save is unavailable while an operation is active or known client-side errors
  exist;
- errors do not clear dirty state; and
- reloading after conflict is an explicit destructive action against the local
  draft and requires confirmation.

## Error and Feedback Model

Stable Problem codes map to consistent UI treatment:

- validation: field message plus form summary;
- conflict: persistent conflict panel with reload guidance;
- unavailable: persistent module/page unavailable state;
- unsupported platform: diagnostic-mode explanation with Settings access where
  available;
- internal: sanitized persistent error with safe copy action.

The last valid snapshot remains visible when a refresh fails and is labeled
stale. Initial loading uses skeletons; confirmed absence uses `EmptyState`.
These states are not conflated.

Toast is reserved for brief success confirmation. Errors, stale data,
unsupported-platform state, conflicts, dirty settings, and restart-required
state remain visible in the page until resolved or dismissed where safe.

## Testing Strategy

### TypeScript and module tests

Pure tests cover:

- Wails DTO to view-model mapping;
- Runtime/Plugin/Settings status classification;
- plugin sorting, filtering, and pagination;
- Problem-to-presentation and Problem-field mapping;
- stale response rejection by revision;
- Settings draft cloning, dirty comparison, and save reconciliation; and
- module start/dispose idempotence.

### Component tests

Vitest and Svelte Testing Library cover every UI component and pattern:

- normal, disabled, loading, empty, and error behavior;
- labels, descriptions, ARIA relationships, and accessible names;
- keyboard operation and focus restoration for Bits UI wrappers;
- form draft and validation display;
- per-plugin pending isolation;
- long text and narrow-container DOM contracts; and
- event subscription cleanup.

The Component Workbench supplies visual development fixtures for all important
states. A fixture does not replace an automated behavior test.

### Responsive browser tests

Playwright runs the Vite frontend against deterministic Wails mocks. It covers
the four page workflows at `640x480`, `720x600`, `1024x768`, and a wider desktop
viewport. Assertions include:

- left navigation versus top Tab Bar placement;
- absence of page-level horizontal overflow;
- visibility and operability of important actions;
- responsive card/form reflow;
- Dialog and internal Tabs keyboard use;
- dirty-settings navigation protection;
- plugin mutation isolation; and
- diagnostic-mode rendering.

M7 does not begin with pixel screenshot baselines. Structural and interaction
assertions avoid cross-platform font rendering brittleness. Targeted visual
snapshots may be added later if stable components benefit from them.

### Completion gates

- Svelte/TypeScript check;
- Vitest unit and component suite;
- Playwright responsive suite;
- frontend production build;
- relevant Go and Wails boundary tests;
- project status generation/check; and
- clean formatting and worktree checks.

## Required Development Order

1. Add Tailwind v4, Bits UI, Lucide, Vitest/Svelte Testing Library, Playwright,
   and deterministic Wails test mocks.
2. Establish semantic tokens, global CSS, font, focus, motion, and responsive
   container rules.
3. Implement and test `components/ui`.
4. Implement and test `components/layout`.
5. Implement and test cross-page `patterns`.
6. Complete Component Workbench fixtures for all component states and narrow
   containers.
7. Implement and test the three typed frontend modules.
8. Implement and test the minimal Avatar-name backend extension and regenerate
   Wails bindings through the repository generator.
9. Implement pages in order: Overview, Plugins, Settings, Diagnostics.
10. Compose AppShell, module lifecycle, events, and dirty-navigation handling.
11. Run Playwright responsive acceptance, production gates, and refresh project
    status from a clean reviewed source commit.

Pages cannot introduce temporary private versions of missing shared controls or
patterns. Component work resumes first, then page composition continues.

## Acceptance Criteria

1. Tailwind v4 uses the official Vite plugin and CSS import, and Bits UI is
   consumed only through project-owned wrappers.
2. All shared controls and patterns are implemented and verified before their
   first product page consumer.
3. Every page is usable without page-level horizontal overflow at `640x480`.
4. Wide windows use left navigation; narrow windows use a top Tab Bar.
5. Overview shows current Avatar name/ID, OSC discovery status, and the current
   output target host/port.
6. Plugins can be searched, filtered, inspected, enabled, and disabled with
   per-plugin command isolation.
7. Global Settings are explicit componentized forms, preserve drafts, report
   field errors, use optimistic revision, and state that successful changes
   require restart.
8. Plugin-private configuration editing is not exposed without a dedicated
   known form.
9. Diagnostics exposes meaningful `Project Status` and runtime problems while
   preserving backend security boundaries.
10. Module events refresh only their owning module, subscriptions are disposed,
    and stale responses cannot overwrite newer revisions.
11. Keyboard, focus, reduced-motion, status redundancy, and contrast contracts
    are covered by component or browser tests.
12. Svelte check, Vitest, Playwright, production build, Wails contracts, and
    project status gates pass before M7 is marked complete.
