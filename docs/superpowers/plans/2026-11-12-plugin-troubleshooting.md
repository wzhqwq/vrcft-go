# Plugin Troubleshooting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show plugin-authored localized troubleshooting in Overview and Plugins when device tracking is unavailable.

**Architecture:** The plugin manifest carries bounded English and Chinese steps. The manager snapshot and Wails DTO copy them to a shared frontend dialog used by both warning views.

**Tech Stack:** Go, JSON manifest validation, Svelte 5, existing Bits UI Dialog.

**Spec:** `docs/superpowers/specs/2026-11-12-plugin-troubleshooting-design.md`

## Global Constraints

- Keep manifest schema version 1 and support manifests without troubleshooting.
- Preserve the repository-local absolute `GOCACHE` for every Go command.
- Render plugin strings as text; never interpret markup or allow unbounded strings.

## Review Focus

- A third-party plugin has no guide: no help control and no catalog error.
- Only one locale is supplied: reject the manifest rather than expose a mixed-language guide.
- A plugin changes language while a dialog is open: the visible steps update.
- A caller mutates a returned plugin snapshot or DTO: later reads keep their original guide.
- A device recovers: the warning and its help button disappear.

---

### Task 1: Manifest guidance and bundled copy

**Files:** `internal/plugins/manifest.go`, tests, both bundled `manifest.json` files.

- [x] Write rejection and acceptance tests for optional paired localized arrays, bounds, and cloning.
- [x] Add validated troubleshooting metadata and English/Chinese steps to Pico Connect and Steam Link manifests.
- [x] Run focused Go tests.

### Task 2: Host snapshot and Wails DTO

**Files:** `internal/plugins/snapshot.go`, `supervisor.go`, root `api_types.go`, `plugins_api.go`, tests.

- [x] Write tests showing guide ownership through supervisor snapshot and the public allowlist.
- [x] Copy validated guide into snapshots and bounded DTOs, including deep clone paths.
- [x] Run package race tests and root tests.

### Task 3: Localized help UI

**Files:** frontend wire/view types, shared dialog component, Overview and Plugins patterns, tests and copy.

- [x] Write tests for both help buttons, dialog content, focus return, recovery, and language change.
- [x] Add one shared guide dialog and render it next to device warnings only when guidance exists.
- [x] Run frontend tests, check, and production build.

### Final verification

- [x] Build desktop staging and inspect the two bundled manifests.
- [x] Review the diff, run `git diff --check`, and report hardware limitations separately.
