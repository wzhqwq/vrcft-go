# Overview Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the overview's collection of summary cards with a compact avatar drive panel and a plugin/OSC control console, backed by truthful current parameter-drive status.

**Architecture:** The Application coordinator derives a bounded parameter status list from the active plan and recent valid plugin input; the root Wails API exposes only names and driven flags. Plugin enable/install timestamps are persisted in the plugin preference store and passed through the existing plugin DTO. Svelte presents a fixed two-area dashboard and a persistent top navigation without changing module ownership or the existing navigation guard.

**Tech Stack:** Go, Wails v2, Svelte 5, TypeScript, Bits UI, Lucide, Tailwind CSS v4.

**Spec:** `docs/superpowers/specs/2026-09-30-overview-redesign-design.md`

## Global Constraints

- Supported minimum window: `640×480`; no page-level horizontal overflow.
- Display at most three overview plugins; status priority is fault, recovery, working, other. Within one group sort by last manual enable time, otherwise first discovery time, then name and ID.
- Parameter list contains only recognized VRCFT bindings from the active ready plan; `current driven` requires recent valid input for every leaf and a valid evaluated result, independent of OSC connection.
- Use existing active-stale settings and generation fencing; do not send tracking values or per-frame events to Wails.
- Keep plugin-private JSON out of the runtime parameter summary. Store times as UTC; no old plugin-preference migration or compatibility tests.
- Do not add regression tests. Add only focused checks that establish new behavior. Do not update generated `docs/project/status.md` as part of this plan.
- Every Go build/test/vet command first sets absolute repository-local `GOCACHE` to `.go-gocache` in its PowerShell session; retain the cache. For example: `$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null`.
- Change generated Wails files only through the Wails generator/build command and inspect their diff.

## Review Focus

The focused checks in Tasks 1–6 must cover these likely mistakes:

1. A plugin can be enabled without ever providing frames: it remains available in the console but drives zero parameters (Task 2).
2. Held or decaying processing values after a plugin stalls must not keep the current-driven count positive (Task 2).
3. An old Avatar generation must not leak parameter flags into the new Avatar's list (Task 2).
4. OSC can disconnect while plugin input remains valid: the drive count remains unchanged and the OSC panel alone changes state (Tasks 2 and 6).
5. Equal or missing plugin times must produce a deterministic, name-and-ID-stable three-item selection (Task 3).

---

### Task 1: Persist Plugin Discovery and Manual Enable Times

**Files:**
- Modify: `internal/plugins/store.go`, `internal/plugins/manager.go`, `internal/plugins/snapshot.go`
- Test: `internal/plugins/store_test.go`, `internal/plugins/manager_test.go`

**Interfaces:**
- Produce: `PluginPreference.InstalledAt time.Time`, `PluginPreference.LastEnabledAt time.Time`; matching `RuntimeSnapshot` fields. Zero `LastEnabledAt` means never manually enabled.
- Keep `Store.Load/Save` and `Manager` public methods unchanged.

- [ ] **Step 1: Add focused tests** named `TestPluginPreferenceTimesPersist` and `TestManagerRecordsDiscoveryAndManualEnableTimes`. Assert one persisted first-discovery UTC time; `Enable` after `Disable` advances `LastEnabledAt`; repeated `Enable`, restart, and automatic start do not. Inject a clock through `managerDependencies.now func() time.Time` for deterministic assertions.
- [ ] **Step 2: Run the focused tests and verify they fail for missing timestamps.** Initialize the fixed `GOCACHE`, then run `go test ./internal/plugins -run 'Test(PluginPreferenceTimesPersist|ManagerRecordsDiscoveryAndManualEnableTimes)$'`.
- [ ] **Step 3: Implement the new store schema and manager updates.** Give `wireSettings` a new schema version; an older development-format file may be reset and overwritten on discovery. Populate `InstalledAt` once for every discovered plugin and save before publishing snapshots. Set `LastEnabledAt` only on a successful false-to-true preference transition. Copy times through preference cloning and snapshot overlay; do not put metadata in plugin-private `Config.Data`.
- [ ] **Step 4: Re-run the focused tests, then `go test ./internal/plugins`.** Confirm both exit successfully with the fixed cache.
- [ ] **Step 5: Commit only this task's files** with `feat(plugins): persist overview ordering times`.

### Task 2: Compute Current Parameter Drive Status in Application

**Files:**
- Create: `internal/application/parameter_drive.go`, `internal/application/parameter_drive_test.go`
- Modify: `internal/application/status.go`, `internal/application/coordinator.go`
- Test: `internal/application/coordinator_test.go`

**Interfaces:**
- Produce: `application.ParameterDriveStatus { Name string; Driven bool }` and `Status.PlanParameters []ParameterDriveStatus`; the existing `PlanGeneration` identifies the owning plan.
- Internal: `newParameterDriveProbe(ids []parameters.ParameterID) (parameterDriveProbe, error)` precomputes each ID's `parameterdeps.ResolveLeaves`; `parameterDriveProbe.Evaluate(frame tracking.MergedFrame, canonical processing.CanonicalFrame, values evaluator.Snapshot) []ParameterDriveStatus` returns an owned, stable-ID-ordered slice.

- [ ] **Step 1: Add `TestParameterDriveProbeRequiresFreshValidLeaves`.** Cover a valid Eye field, a missing Expression leaf in a combined parameter, a boolean active parameter with no recent source, a valid evaluator result, and a held/dropout value with stale source. Assert the output names use `parameters.Definition(id).OSCName` and contain no unrelated Avatar JSON parameters.
- [ ] **Step 2: Run this test to verify the absent probe fails.** Initialize `GOCACHE`, then run `go test ./internal/application -run '^TestParameterDriveProbeRequiresFreshValidLeaves$'`.
- [ ] **Step 3: Implement the probe.** Require all needed capability groups to be active in `canonical`, all required Eye/Expression fields present in the merged frame's validity masks, and `Snapshot.Float/Bool` validity for the requested ID. The coordinator checks the frame generation before calling the probe. Do not count dropout/hold values after group freshness expires.
- [ ] **Step 4: Add `TestCoordinatorParameterDriveStatusFollowsPlanAndInput`** to assert initial ready-plan flags are false, valid input turns the relevant flag true, a stale tick turns it false, a new generation clears the old list, and OSC disconnect does not change plugin-drive flags.
- [ ] **Step 5: Run the coordinator test to verify it fails, then connect the probe to install/process/tick transitions.** Deep-copy `Status.PlanParameters` in `cloneStatus`. Publish plan changes immediately and parameter telemetry at most once per second; skip status updates when the derived content is unchanged. Never publish on each frame solely because a frame arrived.
- [ ] **Step 6: Run the focused tests and `go test ./internal/application` with fixed `GOCACHE`; commit** as `feat(application): expose current parameter drive status`.

### Task 3: Expose Bounded DTOs and Select Overview Plugins

**Files:**
- Modify: `api_types.go`, `runtime_api.go`, `plugins_api.go`
- Modify: `frontend/src/lib/wails/types.ts`, `frontend/src/lib/modules/runtime/types.ts`, `frontend/src/lib/modules/runtime/map.ts`, `frontend/src/lib/modules/plugins/types.ts`, `frontend/src/lib/modules/plugins/selectors.ts`
- Test: `runtime_api_test.go`, `plugins_api_test.go`, `frontend/src/lib/modules/runtime/runtime.test.ts`, `frontend/src/lib/modules/plugins/plugins.test.ts`
- Regenerate: `frontend/wailsjs/go/models.ts` if Wails emits a changed model.

**Interfaces:**
- Produce: `RuntimeApplicationDTO.PlanParameters []ParameterDriveDTO` with `{name, driven}`; `PluginDTO.InstalledAt time.Time` and optional `LastEnabledAt *time.Time`.
- Frontend: `RuntimePlanView.parameters: readonly {name: string; driven: boolean}[]` and `PluginView.installedAt: string`, `lastEnabledAt?: string | null`.
- Replace the overview's `selectImportantPlugins` call with `selectOverviewPlugins(plugins: readonly PluginView[]): readonly PluginView[]`; keep `selectPlugins` and plugin-page ordering intact.

- [ ] **Step 1: Add focused checks** named `TestRuntimeParameterDriveDTOIsBoundedAndOwned`, `TestPluginDTOIncludesPreferenceTimes`, `maps current plan parameters`, and `selects three overview plugins by status and effective time`. Assert no more than `parameters.ParameterCount` parameter items cross Wails, slices are copied, empty lists serialize as `[]`, plugin UTC times survive cloning, and the selector returns fault → recovery → working → other with descending effective time and name/ID ties.
- [ ] **Step 2: Run only the new named tests to verify expected failures.** Use fixed `GOCACHE` for Go checks; run frontend Vitest through `node node_modules/vitest/vitest.mjs run` from `frontend` because PowerShell blocks `pnpm.ps1` in this workspace.
- [ ] **Step 3: Add the allowlisted fields and frontend mapping.** Reject malformed public timestamps using existing DTO validation, copy slices and time pointers at all module boundaries, and ignore a parameter summary whose plan generation differs from the current plan. Update typed frontend fixtures for the new required `installedAt` field. The selector must not mutate the source array.
- [ ] **Step 4: Regenerate Wails bindings with the repository's Wails build/generation flow**, using the fixed `GOCACHE`; inspect `frontend/wailsjs` for unrelated changes. Do not edit generated models by hand.
- [ ] **Step 5: Run `go test .`, the focused frontend module tests, and `node node_modules/svelte-check/bin/svelte-check --tsconfig ./tsconfig.json`; commit** as `feat(api): publish overview status fields`.

### Task 4: Make the Top Navigation Persistent

**Files:**
- Modify: `frontend/src/lib/components/layout/AppShell.svelte`, `frontend/src/lib/components/layout/NavigationItems.svelte`, `frontend/src/App.svelte`
- Test: `frontend/src/lib/components/layout/layout.test.ts`, `frontend/src/App.test.ts`

**Interfaces:**
- Keep `NavigationItem`, `PageId`, and `AppShell.onNavigate(page: PageId)` intact. `App.navigate` remains the only page-change entry point, including dirty-settings confirmation.

- [ ] **Step 1: Add a focused test** named `keeps one top navigation at every width`. Assert a single top `nav[aria-label="主导航"]`, left-aligned icon+text buttons, horizontal overflow containment, and `aria-current`. Check actual viewport visibility in Task 6's browser scenario; retain the existing dirty-settings guard without adding a regression test for it.
- [ ] **Step 2: Run the layout/App tests and verify the desktop-rail assertion fails.**
- [ ] **Step 3: Replace the rail/header switch with one top navigation.** Render Lucide icons with `aria-hidden="true"`; preserve text labels, focus indicators, readable tap targets and horizontal overflow containment.
- [ ] **Step 4: Run the focused tests and Svelte check; commit** as `feat(frontend): keep navigation across the top`.

### Task 5: Build the State-Aware Overview

**Files:**
- Create: `frontend/src/lib/patterns/AvatarDrivePanel.svelte`, `frontend/src/lib/patterns/ParameterListDialog.svelte`, `frontend/src/lib/patterns/PluginOverviewPanel.svelte`
- Modify: `frontend/src/pages/OverviewPage.svelte`, `frontend/src/App.svelte`, `frontend/src/copy/zh-CN.ts`, `frontend/src/lib/patterns/index.ts`
- Test: `frontend/src/pages/OverviewPage.test.ts`

**Interfaces:**
- `OverviewPage` adds `onNavigate: (page: PageId) => void`; `App` passes its existing `navigate` function.
- `AvatarDrivePanel` owns Avatar/plan presentation and the parameter-list trigger; `PluginOverviewPanel` owns up to three compact plugin rows and their existing `plugins.setEnabled` commands. `OverviewPage` composes these with a compact OSC panel in the fixed B layout.

- [ ] **Step 1: Add focused page tests** named `shows ready drive counts and parameter flags`, `replaces avatar content for empty pending and failed plans`, `shows three capable plugins with quick switches`, and `keeps drive count during OSC failure`. Assert ready counts and per-item modal flags; ready-empty, no Avatar, pending, missing config, and stale states; capability labels (`eye`, `expression`, `lip`), ordering and quick switch; independent OSC fault; and button navigation. Include keyboard opening/closing and focus return for the dialog.
- [ ] **Step 2: Run `OverviewPage.test.ts` and verify the new assertions fail for the old page.**
- [ ] **Step 3: Build the compact components and copy.** Use existing `Dialog` and `Button` primitives. Map `eye` to glasses, `expression` to face, and `lip` to a mouth-related Lucide icon; give each a text tooltip and accessible name. Show FPS only for active plugins. Keep the Avatar panel at natural height and use warning/danger tones at fixed positions.
- [ ] **Step 4: Connect `OverviewPage` and `App.navigate`.** Remove the normal-phase card, duplicate Avatar/plan cards, full plugin cards and repetitive summary rows from overview; leave the dedicated Plugins, Settings and Diagnostics pages intact.
- [ ] **Step 5: Run the focused page and App tests plus Svelte check; commit** as `feat(frontend): redesign overview dashboard`.

### Task 6: Verify the Integrated Experience and Update Documentation

**Files:**
- Modify: `docs/project/subsystems/frontend.md`, `docs/project/packages/internal-application.md`, `docs/project/packages/internal-plugins.md`, `docs/project/packages/root.md`
- Modify: `frontend/e2e/fixtures.ts`, `frontend/e2e/responsive.spec.ts`, `frontend/e2e/app.spec.ts` only as needed for new-feature acceptance, not old-layout regression.

**Interfaces:** No new product interfaces.

- [ ] **Step 1: Update project specifications** for the new overview composition, status summary, plugin preference fields, public DTO allowlist and constant top navigation.
- [ ] **Step 2: Exercise focused integrated scenarios** using the mocked Wails fixture: valid parameter summary and modal, independent OSC fault, three-plugin selection, a quick toggle, top navigation at `640×480` and desktop width, and navigation to Settings through a visible button. Update fixture fields to the new DTO contract.
- [ ] **Step 3: Run required build/type gates.** Initialize fixed `GOCACHE`; run `go test ./internal/plugins ./internal/application .`, `go vet ./internal/plugins ./internal/application .`, `node node_modules/svelte-check/bin/svelte-check --tsconfig ./tsconfig.json`, and `node node_modules/vite/bin/vite.js build`. Run the focused Playwright scenarios if the browser runner is available. Inspect `git diff --check` and generated-file changes.
- [ ] **Step 4: Commit docs and acceptance fixture changes** as `docs: describe redesigned overview contracts`. Keep `docs/project/status.md` untouched until a separately requested clean-source refresh.
