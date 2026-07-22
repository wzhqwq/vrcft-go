# Project Specifications

This directory is the authoritative map of package ownership, subsystem boundaries, milestones, and executable acceptance criteria for VRCFaceTracking-Go.

The product has a control path and a frame path. The control path turns VRChat avatar changes and local avatar configuration into compiled OSC bindings, required parameters, and plugin subscriptions. The frame path carries only subscribed tracking data through ingest, merge, processing, evaluation, and OSC output.

- [Milestones](milestones.md) defines dependency order and product completion.
- [Generated status](status.md) reports current evidence and blockers.
- `packages/` specifies every Go package returned by `go list ./...`.
- `subsystems/` specifies frontend, parameter definitions, release, and end-to-end behavior.

Run `go run ./cmd/projectstatus` for a fresh terminal report, `-write` to update the committed Markdown report, `-format json` for machine-readable output, and `-check` to detect stale status.

States are `not_started`, `in_progress`, `complete`, `degraded`, and `blocked`. Percentages are weighted acceptance results, not estimates based on code volume.
