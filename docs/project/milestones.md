# Project Milestones

## M0 — Specification and status infrastructure

Every package and subsystem is registered, checks run safely, and deterministic Markdown/JSON status can be regenerated.

## M1 — Shared data contracts

Tracking frames, plugin APIs, IPC messages, parameter identities, YAML parsing, and generated artifacts have stable compatibility tests.

## M2 — Plugin runtime and IPC

Plugins handshake, receive configuration and subscriptions, publish selected fields, report health, and shut down or restart safely.

## M3 — Host tracking merge

Host ingest validates frames, rejects stale generations, selects sources, and emits a stable merged frame.

## M4 — Processing and parameter evaluation

Calibration, tuning, filtering, mutual exclusion, dropout, dependency closure, and selective parameter evaluation are executable.

## M5 — Avatar configuration and requirement planning

`/avatar/change` resolves local avatar JSON, compiles input bindings, computes required tracking fields, and publishes generation-tagged subscriptions.

## M6 — OSC end-to-end loop

Selected plugin data reaches VRChat through merge, processing, evaluation, compiled bindings, and atomic avatar-plan switching.

## M7 — Frontend, operations, and release

Users can inspect and configure the system, diagnose failures, and build tested release artifacts.
