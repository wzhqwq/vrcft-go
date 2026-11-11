# Pico Connect Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a bundled Pico Connect plugin that supports transfer protocols 2 and 0 with evidence-backed input mapping.

**Architecture:** One `pluginapi.Driver` owns lifecycle and freshness. Separate receivers feed a canonical PICO shape sample to a pure mapper. Protocol 2 uses loopback UDP; protocol 0 transport follows research evidence before coding.

**Tech Stack:** Go, standard library UDP, `pkg/pluginapi`, `pkg/trackingmodel`, PowerShell build scripts.

**Spec:** `docs/superpowers/specs/2026-11-11-pico-connect-adapter-design.md`

**Scope update, 2026-11-11:** The user deferred Task 6 and requested protocol 2 only. The plugin now defaults to protocol 2, rejects protocol 0, and polls the PICO Connect process and `lab.faceTrackingTransferProtocol` until both are ready. The original Task 6 below is retained as historical planning context and is outside current completion criteria.

## Global Constraints

- The plugin ID is `pico-connect`; target PICO 4 Pro / Enterprise.
- Protocol 2 uses IPv4 loopback UDP port 29765, a 16-byte header, at least 892 body bytes, and `trackingType=2`.
- Protocol 0 requires primary transport evidence; no guessed decoder or claim of device support.
- Raw biometric data and packet captures stay out of the repository and logs.
- Set absolute repository-local `GOCACHE` before each Go command.

## Review Focus

- Another process owns port 29765: report a bind error and keep the plugin responsive.
- A valid packet has four trailing bytes: accept the first 892 body bytes.
- Input stops during an active subscription: publish one invalid snapshot and report disconnected.
- Subscription generation changes with queued packets: drop stale observations.
- Protocol 0 is selected without a verified receiver: expose an explicit unsupported status, never fake readiness.

---

### Task 1: Protocol 0 evidence report

**Files:** Create `docs/research/pico-connect-protocol-0.md`.

**Interfaces:** Produces a documented transport hypothesis, observed version and endpoint evidence, and a list of still-missing facts; later receiver work uses this as its input contract.

- [ ] Inspect PICO Connect 10.6.6 installed resources, public documentation, and running endpoints read-only; record only non-biometric facts.
- [ ] Identify the protocol 0 transport, packet or API boundary, and field layout from a real session or primary source. If unavailable, state exactly what evidence is missing and keep Task 6 open.
- [ ] Review the report against the design's evidence boundaries and commit it.

### Task 2: Protocol 2 packet parser

**Files:** Create `internal/picoconnect/packet.go`, `internal/picoconnect/packet_test.go`.

**Interfaces:** `parseLegacyPacket([]byte) (shapePacket, error)` returns the first 52 little-endian float32 PICO slots, a local source timestamp value for diagnostics, and a finite-value presence mask. It rejects payloads shorter than 908 bytes and `trackingType != 2`; it ignores trailing bytes and non-finite individual slots.

- [ ] Write tests with synthetic 908-byte and 912-byte packets, short input, wrong type, and non-finite slot values; note which production branch each test protects.
- [ ] Run `go test ./internal/picoconnect -run TestParseLegacyPacket -count=1` with fixed `GOCACHE`; observe expected failures.
- [ ] Implement `parseLegacyPacket` with explicit little-endian reads and no unsafe struct layout.
- [ ] Re-run parser tests, inspect diff, commit.

### Task 3: Shape mapping and freshness

**Files:** Create `internal/picoconnect/shapes.go`, `mapping.go`, `mapping_test.go`, `state.go`, `state_test.go`.

**Interfaces:** `mapShapes(shapePacket, pluginapi.Subscription) trackingmodel.TrackingFrame` maps known 52-slot PICO shapes; `streamState.observe(shapePacket,time.Time)` and `streamState.next(time.Time)` manage 250 ms validity and monotonic Host timestamps.

- [ ] Pin the 52 slot order to PICO developer API or versioned primary source in `shapes.go` comments and tests; add eye direction, blink, representative mouth and brow mapping tests.
- [ ] Run focused tests red; implement exact map and validity rules; run green.
- [ ] Add expiry, generation reset, and subscription trimming tests; run red, implement state, run green; commit.

### Task 4: Driver and plugin process

**Files:** Create `internal/picoconnect/driver.go`, `receiver.go`, `config.go`, their tests, `cmd/pico-connect-plugin/main.go`, and `plugins/pico-connect/manifest.json`.

**Interfaces:** `New() *Driver`, `(*Driver).Descriptor() pluginapi.Descriptor`, `(*Driver).Run(context.Context, pluginapi.Host) error`; configuration uses explicit `transferProtocol` 0 or 2. Protocol 2 receiver supplies owned datagrams to the driver.

- [ ] Write driver tests for startup, subscribe, stop, bind failure, control transition, and unsupported protocol 0 status; run red.
- [ ] Implement the driver and bounded UDP reception using Steam Link's lifecycle pattern; run green and race tests.
- [ ] Add process/manifest smoke tests and commit.

### Task 5: Build and user documentation

**Files:** Create `build/build-pico-connect.ps1`, `plugins/pico-connect/README.md`, package specs; modify `build/build-desktop.ps1`, `build/README.md`, root `README.md`, and release acceptance checks.

**Interfaces:** `build/build-pico-connect.ps1 -PluginRoot <root>` stages the executable and manifest; desktop build stages both bundled plugins.

- [ ] Add or update build acceptance test/check for the Pico manifest and executable; run red.
- [ ] Implement build script and staging; run focused build, then plan-required package checks; document setup and current evidence level; commit.

### Task 6: Protocol 0 receiver and hardware acceptance

**Files:** Create a separate `internal/picoconnect` protocol 0 receiver and tests after Task 1 identifies its interface; update config, docs, and acceptance evidence.

**Interfaces:** It emits the same `shapePacket` or a richer canonical sample when source fields differ. Its contract must be written from observed bytes or an official API, before implementation.

- [ ] Obtain a real protocol 0 session or primary specification and record version, endpoint/API, field order, timing, and sample validity without committing biometric data.
- [ ] Write failing decoder and receiver tests from that contract; run red.
- [ ] Implement the receiver, map only verified fields, run focused and race tests.
- [ ] Validate protocol 0 and protocol 2 on real PICO 4 Pro / Enterprise sessions; record results and update claims. Complete only after both modes pass.

## Final verification

- [ ] Run relevant package-spec commands with fixed absolute `GOCACHE`.
- [ ] Build desktop staging and inspect both manifests/executables.
- [ ] Run `git diff --check`, review the entire branch, and report hardware limits precisely.
