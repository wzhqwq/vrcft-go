# Steam Link Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first managed Steam Link plugin with independently subscribed eye/expression output and complete offline acceptance before Pico 4 Pro hardware validation.

**Architecture:** An independent executable runs a Driver through the existing plugin runtime. The adapter owns a loopback UDP receiver, uses `pkg/osc.UnmarshalPacket`, and maps fresh observations into the existing tracking model. Routing, processing, IPC supervision, and final OSC output remain host responsibilities.

**Tech Stack:** Go 1.25.6 or newer as required by `go.mod`, standard-library UDP/JSON/time, public OSC/plugin/tracking packages, Windows named pipes through the existing runtime, PowerShell packaging.

**Spec:** [Steam Link Adapter Design](../specs/2026-09-13-steamlink-adapter-design.md), committed as `00634b1` on `feat/steamlink-adapter`.

## Global Constraints

- Documentation, identifiers, comments, and diagnostics use English; Chinese UI copy is the exception.
- Production `internal/steamlink` imports only the standard library, `pkg/osc`, `pkg/pluginapi`, and `pkg/trackingmodel`. The command also uses `pkg/pluginruntime`; its indirect `internal/ipc` dependency remains encapsulated.
- Descriptor: `id=steamlink`, display name `Steam Link`, initial version `0.1.0`, API 1, and capabilities `Eye | Expression`, excluding Lip.
- The default binding is `127.0.0.1:9015`. Configuration exposes only `listenPort`, an integer from 1 through 65535, defaulting to 9015.
- Limits: 65507 payload bytes, 512 messages per packet, 256 address bytes, 64 queued datagrams, 32 retained unknown address names. Adopt the shared codec's nesting bound and `i/f/s/T/F` subset; unsupported types discard the entire datagram.
- Publish at most 100 Hz; fields expire at age >= 250 ms; disconnection follows 2 seconds without recognized valid tracking input; initial bind retries every 2 seconds; diagnostic summaries are limited to one per category per 5 seconds.
- Reuse `pkg/osc`. No private wire decoder, blob skipping, partial recovery from codec errors, OSC scheduler, or public API change.
- Preserve the design's exact mapping table and formulas. Missing observations are not zero measurements. No fixed pupils or LinkFT power-function correction.
- No changes to IPC versions, expression IDs, metadata-only Lip, host source selection, or persistent user settings.
- All Go commands reuse absolute `GOCACHE=F:\dev\vrcft-go\.go-gocache`. Never clear it during this work.
- Hardware acceptance comes last. Synthetic fixtures prove implementation behavior, not Pico compatibility.

## Environment and Baseline

Work in `F:\dev\vrcft-go\.worktrees\steamlink-adapter`; do not create another worktree. Read the repository entry-point documents, design, and relevant package specifications. Inspect `git status --short` and preserve unrelated edits.

Initialize every PowerShell tool session before any Go command, including commands launched by scripts:

```powershell
$env:GOCACHE = 'F:\dev\vrcft-go\.go-gocache'
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
```

All commands below require this initialization in the same session. Check every exit code before continuing; a later successful Git command must not hide a failed test.

- [x] Record `git rev-parse HEAD` and working-tree status.
- [x] Run `go test ./pkg/osc ./pkg/pluginapi ./pkg/pluginruntime ./pkg/trackingmodel` and `go test ./internal/projectstatus -run '^TestParseSpecRejectsInvalidMetadata$' -count=1`. The September 13 failure is historical, not an assumed current blocker.
- [x] If root checks require frontend assets or Wails bindings, use the existing `wails build` workflow with the fixed cache. Inspect generated tracked-file changes; do not add fake frontend placeholders.
- [x] Record the exact LinkFT commit and source paths inspected during implementation in `plugins/steamlink/README.md`. Preserve licenses if reusing code.

The proposed private interfaces and tests below are implementation contracts, not existing code. No plugin implementation has been performed while preparing this plan.

## File Map

| Files | Responsibility |
| --- | --- |
| `internal/steamlink/config.go`, `config_test.go` | Strict configuration and defaults |
| `internal/steamlink/fields.go`, `input.go`, `input_test.go`, `input_fuzz_test.go` | Fixed raw registry, public codec boundary, device input checks |
| `internal/steamlink/mapping.go`, `mapping_test.go` | Mapping rules and subscription dependencies |
| `internal/steamlink/state.go`, `state_test.go` | Freshness, snapshots, publication metadata |
| `internal/steamlink/receiver.go`, `receiver_test.go` | Socket, bounded queue, timestamps and epochs |
| `internal/steamlink/driver.go`, `driver_test.go`, `diagnostics.go`, `diagnostics_test.go` | Controls, configuration recovery, status and summaries |
| `internal/steamlink/test_helpers_test.go` | Fake Host and clock, test fixtures |
| `cmd/steamlink-plugin/main.go`, `main_test.go` | Runtime entry, manifest consistency and dependency check |
| `plugins/steamlink/manifest.json`, `README.md` | Distribution template, setup, provenance and hardware checklist |
| `internal/plugins/steamlink_integration_test.go` | Windows manager/runtime/IPC process acceptance |
| `build/build-steamlink.ps1`, `build/build-desktop.ps1` | Explicit development and distribution builds |
| `build/windows/installer/project.nsi`, `build/README.md`, `README.md` | Existing installer inclusion and build documentation |
| `docs/project/packages/internal-steamlink.md`, `cmd-steamlink-plugin.md` | Responsibilities and executable acceptance |
| `internal/projectstatus/catalog_integration_test.go`, `docs/project/subsystems/build-release.md` | Package registration count and distribution checks |

No new module or third-party dependency is required. Keep production responsibilities in focused files.

## Task 1: Strict Configuration and Public OSC Input

**Files:** Create adapter `config.go`, `config_test.go`, `fields.go`, `input.go`, `input_test.go`, `input_fuzz_test.go` and its package spec. Update `internal/projectstatus/catalog_integration_test.go` from 26 to 27 specs when the package is added.

**Interfaces produced:**

```go
type config struct { ListenPort int }
func parseConfig(data json.RawMessage) (config, error)
type rawID uint16
type observation struct { ID rawID; Values [3]float32 }
type inputReport struct {
    InvalidMessages int
    UnknownMessages int
    UnknownAddresses []string // At most 32 names per report.
}
func decodeDatagram(packet []byte) ([]observation, inputReport, error)
```

Declare one `rawID` constant per distinct source in the design and a final `rawCount`. Required names used below are `rawGazePoint`, `rawEyesClosedL`, `rawUpperLidRaiserL`, `rawJawDrop`, `rawJawSidewaysLeft`, and `rawJawSidewaysRight`; use the same naming pattern for all remaining sources.

- [x] Write configuration tests for nil, `{}`, explicit port 9017, and endpoints 1/65535. Reject zero, 65536, negatives, fractions, strings, null, arrays, duplicate keys, unknown keys, and trailing JSON values:

```go
func TestConfigRejectsDuplicatePort(t *testing.T) {
    _, err := parseConfig(json.RawMessage(`{"listenPort":9015,"listenPort":9017}`))
    if err == nil { t.Fatal("duplicate listenPort accepted") }
}
```

- [x] Run `go test ./internal/steamlink -run '^TestConfig'` and confirm failure before implementation.
- [x] Implement `parseConfig` using `json.Decoder.Token` for one object, a seen-key flag, integer decoding, the closing delimiter, and EOF validation. Empty bytes use defaults; JSON null is rejected. Errors identify the problem without logging raw configuration.
- [x] Define a fixed address-to-ID/argument-count registry for gaze and every source in the mapping table, including both closed/wide eye pairs. Never extend it from received addresses.
- [x] Add and run this whole-datagram rejection test:

```go
func TestInputRejectsUnsupportedSibling(t *testing.T) {
    good, err := osc.MarshalMessage(osc.Message{
        Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Float32(0.5)},
    })
    if err != nil { t.Fatal(err) }
    blob := []byte{'/', 'x', 0, 0, ',', 'b', 0, 0, 0, 0, 0, 0}
    packet := make([]byte, 16)
    copy(packet, "#bundle\x00")
    binary.BigEndian.PutUint64(packet[8:], 1)
    for _, element := range [][]byte{good, blob} {
        var size [4]byte
        binary.BigEndian.PutUint32(size[:], uint32(len(element)))
        packet = append(packet, size[:]...)
        packet = append(packet, element...)
    }
    observations, _, err := decodeDatagram(packet)
    if !errors.Is(err, osc.ErrUnsupportedType) || len(observations) != 0 {
        t.Fatalf("got %v, %v; want whole-packet rejection", observations, err)
    }
}
```

- [x] Implement the boundary: reject payloads over 65507 bytes before decoding; propagate codec errors without partial observations; validate all message/address limits before processing fields. Define a private `errPacketLimit` sentinel.

```go
messages, err := osc.UnmarshalPacket(packet)
if err != nil { return nil, inputReport{}, err }
if len(messages) > 512 { return nil, inputReport{}, errPacketLimit }
for _, message := range messages {
    if len(message.Address) > 256 { return nil, inputReport{}, errPacketLimit }
}
```

Then exact-match addresses, require float32 and exact arity, reject nonfinite values, reject weights outside `[0,1]`, and require gaze z < 0. Preserve observation order for last-valid-duplicate semantics. Device-invalid messages increment counters without discarding valid siblings. Unknown supported-type messages affect bounded diagnostics only.

- [x] Test padding/trailing-byte errors, empty bundles, supported unknown addresses, message/address limits, invalid siblings, duplicates, gaze arity, NaN/Inf and weight bounds. Shared codec tests retain responsibility for generic nesting behavior.
- [x] Add `FuzzDecodeDatagram`: seed valid and malformed bytes, enforce the size limit, assert no observations on error and finite values/valid IDs on success. Run `go test ./pkg/osc ./internal/steamlink`.
- [x] Register `internal-steamlink` at M2 with `depends_on: [pkg-osc, pkg-pluginapi, pkg-trackingmodel]`, package/race checks, and a `FuzzDecodeDatagram` symbol check. Follow existing package-spec headings and accurately describe the implemented subset.
- [x] Commit as `feat(steamlink): validate configuration and OSC tracking input`.

## Task 2: Mapping and Subscription Dependencies

**Files:** Create `mapping.go`, `mapping_test.go`; complete the fixed registry in `fields.go` if necessary.

**Consumes:** Task 1 observations/IDs, public subscription and tracking types.

**Produces:**

```go
type rawSample struct { Values [3]float32; ReceivedAt time.Time; Seen bool }
type rawSnapshot [rawCount]rawSample
type rawSelection [rawCount]bool
func requiredFields(sub pluginapi.Subscription) rawSelection
func mapSnapshot(raw rawSnapshot, sub pluginapi.Subscription, now time.Time) trackingmodel.TrackingFrame
```

Define `fieldTTL = 250*time.Millisecond`. Fresh means Seen and `0 <= now.Sub(ReceivedAt) < fieldTTL`. Mapping does not assign sequence or timestamps.

- [x] Write the failing test and run `go test ./internal/steamlink -run '^TestMapping'`:

```go
func TestMappingJawDifferenceRequiresBothInputs(t *testing.T) {
    now := time.Unix(100, 0)
    sub := pluginapi.Subscription{Generation: 1,
        Capabilities: trackingmodel.CapabilityExpression,
        Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawX)}
    var raw rawSnapshot
    raw[rawJawSidewaysRight] = rawSample{Values: [3]float32{0.8}, ReceivedAt: now, Seen: true}
    first := mapSnapshot(raw, sub, now)
    if first.Expressions.Valid.Has(trackingmodel.ExpressionJawX) { t.Fatal("missing input treated as zero") }
    raw[rawJawSidewaysLeft] = rawSample{Values: [3]float32{0.3}, ReceivedAt: now, Seen: true}
    second := mapSnapshot(raw, sub, now)
    value, valid := second.Expressions.Get(trackingmodel.ExpressionJawX)
    if !valid || math.Abs(float64(value-0.5)) > 1e-6 { t.Fatalf("got %v, %v", value, valid) }
}
```

- [x] Transcribe every design mapping row into a fixed rule table: target expression ID, required source IDs, and copy/subtract operation. Expand both sides and all lip quadrants. Fan-out is separate target rules sharing sources. Use this table for both required input selection and evaluation.
- [x] Implement the gaze and eyelid conversions:

```go
func normalizedGaze(x, y, z float32) trackingmodel.Vec2 {
    clamp := func(v float64) float32 { return float32(math.Max(-1, math.Min(1, v))) }
    return trackingmodel.Vec2{
        X: clamp(math.Atan2(float64(x), -float64(z)) / (math.Pi / 4)),
        Y: clamp(math.Atan2(float64(y), -float64(z)) / (math.Pi / 4)),
    }
}
func openness(closed, wide float32) float32 { return (1 - closed) * (0.75 + 0.25*wide) }
```

Only fresh gaze produces gaze validity. Closed is required for an eyelid; absent/expired wide uses zero as the specified optional correction. Never synthesize pupil validity. Capabilities are the intersection of subscription and Eye/Expression; return `sub.TrimFrame(frame)`.

- [x] Add independent expectations for every target row, signed differences, fan-out, missing/expired inputs, valid zeros, forward and +/-45-degree gaze, saturation, shared gaze, independent lids, optional-wide expiry, and unsupported targets. Do not generate expected values from the implementation table.
- [x] Test required raw selections for eye-only lids, expression-only data, JawX's two inputs, full-group zero masks, and Lip-only subscriptions producing no numeric output.
- [x] Run `go test ./internal/steamlink`, then commit as `feat(steamlink): map subscribed eye and expression fields`.

## Task 3: Freshness and Publication State

**Files:** Create `state.go`, `state_test.go`.

**Consumes:** Task 2 raw snapshots, required selections and mapping.

**Produces:** A private `streamState` owning fixed raw state, normalized subscription, active flag, dirty flag, previous mapped output, process start time, sequence and last timestamp:

```go
func newStreamState(startedAt time.Time) *streamState
func (s *streamState) reset(active bool, sub pluginapi.Subscription)
func (s *streamState) observe(values []observation, receivedAt time.Time)
func (s *streamState) next(now time.Time) (trackingmodel.TrackingFrame, bool)
```

- [x] Add this failing test and run `go test ./internal/steamlink -run '^TestState'`:

```go
func TestStateExpiryPublishesOneInvalidation(t *testing.T) {
    start := time.Unix(100, 0)
    s := newStreamState(start)
    s.reset(true, pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression})
    s.observe([]observation{{ID: rawJawDrop, Values: [3]float32{0.5}}}, start)
    first, ok := s.next(start.Add(time.Millisecond))
    if !ok || !first.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) { t.Fatal("missing fresh frame") }
    if _, ok := s.next(start.Add(249*time.Millisecond)); ok { t.Fatal("cache republished") }
    expired, ok := s.next(start.Add(250*time.Millisecond))
    if !ok || !expired.Expressions.Valid.IsZero() { t.Fatal("missing expiration frame") }
    if _, ok := s.next(start.Add(260*time.Millisecond)); ok { t.Fatal("invalidation repeated") }
}
```

- [x] Implement reset without resetting sequence/start/last timestamp. Observe only selected inputs while active with a positive generation. Repeated identical observations still mark state dirty. No cache is retained while inactive.
- [x] Implement next by mapping current inputs and comparing payload/validity with the previous mapped snapshot, excluding metadata. Emit on dirty observations or changed output. This comparison must detect optional-wide expiry changing openness without clearing its valid bit. Suppress empty startup snapshots; emit one final empty-validity frame when previously valid data expires.
- [x] On emission increment Sequence and set `TimestampNS=max(1, now.Sub(startedAt).Nanoseconds(), previousTimestamp+1)` and `SourceClockNS=0`. Task 5's 10 ms ticker enforces publication frequency. Do not increment metadata for suppressed frames.
- [x] Test partial stream loss despite other fresh input, difference-input expiry, all fields missing, repeated values, optional-wide expiry with closed still fresh, no subscription, pause/resume, generation resets requiring new observations, and monotonic metadata across resets.
- [x] Run the package suite and commit as `feat(steamlink): expire observations and publish bounded snapshots`.

## Task 4: UDP Receiver and Buffer Ownership

**Files:** Create `receiver.go`, `receiver_test.go`.

**Produces:** A private receiver containing the UDP socket and atomic drop count:

```go
type datagram struct { Bytes []byte; ReceivedAt time.Time; Epoch uint64 }
func listenReceiver(address string) (*receiver, error)
func (r *receiver) localAddr() *net.UDPAddr
func (r *receiver) run(ctx context.Context, epoch *atomic.Uint64, now func() time.Time, out chan<- datagram) error
func (r *receiver) close() error
func (r *receiver) takeDropped() uint64
```

- [x] Add failing `TestReceiverOwnsQueuedBytes`, `TestReceiverDropsOnFullQueue`, and `TestReceiverCloseUnblocksRead`. Use real loopback sockets, context deadlines, and completion signals, not startup sleeps. Run `go test ./internal/steamlink -run '^TestReceiver'`.
- [x] Bind with `net.ListenUDP("udp4", addr)` and use one 65535-byte buffer. Production uses configured nonzero ports; tests may bind port zero directly. Reject oversized payloads before queueing. Copy bytes into each queued datagram:

```go
packet := datagram{Bytes: append([]byte(nil), buffer[:n]...), ReceivedAt: now(), Epoch: epoch.Load()}
select {
case out <- packet:
default:
    r.dropped.Add(1)
}
```

Stamp time immediately after reading, never while draining the queue. Define `dropped atomic.Uint64` and implement `takeDropped` via `Swap(0)`. Context cancellation closes the socket to wake reads; repeated close must be safe.

- [x] Assert owned bytes survive buffer reuse, full queues never block shutdown, coordinated close returns cleanly, substantive read errors remain errors, and changing UDP source ports does not create identities. Driver joins the receive worker before replacing it.
- [x] Run `go test -race ./internal/steamlink -run '^TestReceiver'`, then commit as `feat(steamlink): receive bounded loopback datagrams`.

## Task 5: Driver, Controls, Recovery, and Diagnostics

**Files:** Create `driver.go`, `driver_test.go`, `diagnostics.go`, `diagnostics_test.go`, `test_helpers_test.go`; update the adapter package spec with actual lifecycle behavior.

**Consumes:** Tasks 1-4 and `pluginapi.Host`.

**Produces:**

```go
type Driver struct { clock clock; listen func(string) (*receiver, error) }
func New() *Driver
func (d *Driver) Descriptor() pluginapi.Descriptor
func (d *Driver) Run(ctx context.Context, host pluginapi.Host) error
type clock interface { Now() time.Time; NewTicker(time.Duration) ticker }
type ticker interface { C() <-chan time.Time; Stop() }
```

New uses real time and `listenReceiver`; tests inject a fake clock/listener. Run-local state owns mutable lifecycle fields. Do not expose a general-purpose dependency injection framework.

- [x] Create a thread-safe fake Host implementing Startup/Events/PublishFrame/PublishStatus/Log exactly as in `pkg/pluginapi`. Return a cloned startup config, record frames by value, and support a false publication result. Fake tickers advance explicitly and report Stop.
- [x] Add failing `TestDriverPublishesSubscribedUDPInput`: start Run, wait for listener creation, send encoded JawDrop, advance the 10 ms clock, assert only selected JawOpen is valid, cancel, and require worker completion. Test descriptor validation. Run `go test ./internal/steamlink -run '^TestDriver'`.
- [x] Wire the loop: read Startup once; normalize current state; handle typed controls; prioritize cancellation/queued controls; otherwise process one datagram or tick. Use `decodeDatagram`, `streamState.observe`, and `streamState.next` instead of duplicating their logic.

The publication branch must not retry a rejected frame:

```go
if frame, publish := state.next(now); publish {
    host.PublishFrame(frame)
}
```

- [x] On activation, deactivation, generation change, or successful rebind: increment atomic receive epoch, record a monotonic transition fence, and reset stream state. Discard datagrams with a different epoch or ReceivedAt before the fence. Preserve sequence/timestamps. Network-buffered stale packets cannot be identified without device timestamps; do not promise otherwise.
- [x] Implement config handling: invalid initial JSON publishes DeviceError while controls remain alive; invalid updates preserve previous working configuration and latch an unapplied-revision error. Bind a candidate port before closing/joining the old receiver; only then change epoch/start the new worker. Failed candidates preserve old data. A valid same-port correction clears the error without rebinding. Initial occupied ports retry every 2 seconds; unrecoverable read errors return to the host supervisor.
- [x] Add fake-clock tests for initial bind retry, malformed config correction, failed/successful rebind, queued old packets, same-port recovery, closed Events, ShutdownRequested, and cancellation during sustained UDP traffic.
- [x] Track recognized valid input for health even when inactive/unsubscribed, without caching numerical state. Bound statuses to Initializing, Disconnected before first data or after 2 seconds, Ready with valid input, and latched configuration/bind Error taking priority. Unknown-only or invalid input cannot sustain readiness.
- [x] Implement counters for malformed/unsupported packets, invalid messages, queue drops, and unknown addresses. Keep at most 32 distinct names, count overflow, and emit each category at most once per 5 seconds. Tests must prove bounded retention and no raw values/configuration in logs.
- [x] Cover 100 Hz maximum publication, repeated values, independent groups, no-subscription reception, active transitions clearing caches, false PublishFrame without retry, worker/ticker cleanup, and no busy loop when Events closes.
- [x] Run `go test -race ./internal/steamlink` and `go vet ./internal/steamlink`; commit as `feat(steamlink): manage driver lifecycle and diagnostics`.

## Task 6: Executable, Manifest, and Package Registration

**Files:** Create `cmd/steamlink-plugin/main.go`, `main_test.go`, `plugins/steamlink/manifest.json`, and its command package spec. Update catalog count from 27 to 28.

**Consumes:** `steamlink.New() *Driver`, `pluginruntime.Main(pluginapi.Driver) error`.

- [x] Add a failing test loading the source manifest with `internal/plugins.Manifest`, calling Validate, and comparing it with `steamlink.New().Descriptor()`. Require entrypoint `steamlink-plugin.exe`. Test-only host-package imports do not alter production adapter dependencies. Run `go test ./cmd/steamlink-plugin`.
- [x] Implement the command:

```go
package main

import (
    "fmt"
    "os"
    "github.com/wzhqwq/vrcft-go/internal/steamlink"
    "github.com/wzhqwq/vrcft-go/pkg/pluginruntime"
)

func main() {
    if err := pluginruntime.Main(steamlink.New()); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

- [x] Add the exact manifest and use its description in Descriptor:

```json
{
  "schemaVersion": 1,
  "id": "steamlink",
  "name": "Steam Link",
  "version": "0.1.0",
  "description": "Eye and expression tracking from Steam Link OSC.",
  "protocolMin": 1,
  "protocolMax": 1,
  "entrypoint": "steamlink-plugin.exe",
  "capabilities": 3
}
```

- [x] Register `cmd-steamlink-plugin` at M2 with `[internal-steamlink, pkg-pluginruntime]`, required package tests and a main-symbol check. Add an architecture test using `go list -f '{{join .Imports "\n"}}' ./internal/steamlink` to reject project imports outside the three allowed public packages. Go subprocesses inherit the initialized absolute GOCACHE.
- [x] Run `go test ./cmd/steamlink-plugin ./internal/projectstatus` after normal asset prerequisites. Update cardinality only for the two added packages; retain coverage assertions. Verify starting without runtime environment variables fails clearly through the existing runtime validation.
- [x] Commit as `feat(steamlink): add managed plugin executable and manifest`.

## Task 7: Real Process and Named-Pipe Acceptance

**Files:** Create Windows-only `internal/plugins/steamlink_integration_test.go`; update executable acceptance evidence in the adapter spec.

**Consumes:** The actual built command, DirectoryCatalog, JSONStore, ProcessLauncher, Manager, and FrameSink. Follow `internal/plugins/integration_test.go` lifecycle patterns without substituting its generic helper for the Steam Link command.

- [x] Create a test-local sink whose bounded channel retains plugin ID, generation, and frame. Build the command into a temporary plugin directory with `exec.CommandContext(ctx, "go", "build", "-o", executable, "./cmd/steamlink-plugin")`, repository workdir, and inherited absolute GOCACHE. Fail if the cache is missing rather than silently falling back globally.
- [x] Copy the manifest into a real temporary catalog root, use a temporary JSONStore, and launch with `NewProcessLauncher`. Configure a dynamically selected nonzero loopback port. Use bounded retries for ephemeral-port allocation races; never bind production port 9015 in tests.
- [x] Add `TestSteamLinkProcessPipeline`: start Manager, apply port configuration revision 1, enable, subscribe with generation 7, activate, wait for running state, and send repeated synthetic packets until expected data or a context deadline. Repetition handles the asynchronous control boundary without a startup sleep. Validate the delivered data:

```go
if got.pluginID != "steamlink" || got.generation != 7 ||
    !got.frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) ||
    got.frame.Expressions.Values[trackingmodel.ExpressionJawOpen] != 0.5 {
    t.Fatalf("unexpected Steam Link frame: %+v", got)
}
```

- [x] Add cases for eye-only versus JawOpen-only, new subscription generations, pause/resume, port changes, codec rejection, disconnect, and disable. Check no unselected fields, no stale queued data after transitions, bounded shutdown, and release of UDP/pipe/process resources. Drain old sink observations when asserting a new generation.
- [x] Verify delivered valid and invalidated frames through `tracking.NewService`, `Service.SetGeneration`, `Service.Submit`, `Service.LatestMerged`, `processing.NewPipeline`, and `Pipeline.ProcessAt`. Keep host imports in these host-owned tests. After the first observed valid frame, use the following setup:

```go
service := tracking.NewService()
if err := service.SetGeneration(got.generation); err != nil { t.Fatal(err) }
if err := service.Submit(got.pluginID, got.generation, got.frame); err != nil { t.Fatal(err) }
merged, ok := service.LatestMerged()
if !ok { t.Fatal("missing merged frame") }
pipeline, err := processing.NewPipeline(processing.DefaultConfig())
if err != nil { t.Fatal(err) }
canonical, err := pipeline.ProcessAt(merged, merged.UpdatedAtNS)
if err != nil { t.Fatal(err) }
if !canonical.ExpressionActive { t.Fatal("fresh expression group is inactive") }
```

Submit the later invalidation through the same service and pipeline. Advance `ProcessAt` using host times past the default stale/hold/decay total (900 ms; use a 2-second advance) without fabricating newer input. Assert JawOpen becomes neutral and ExpressionActive becomes false. Preserve increasing input sequences; do not reset the pipeline between fresh data and dropout.
- [x] Run `go test ./internal/plugins -run '^TestSteamLink' -count=1` and `go test -race ./internal/plugins ./pkg/pluginruntime ./internal/steamlink`. Separate sandbox permission failures from assertion failures and use authorized normal permissions when needed.
- [x] Commit as `test(steamlink): exercise managed process and UDP tracking flow`.

## Task 8: Development and Desktop Distribution

**Files:** Create both build scripts and `plugins/steamlink/README.md`; update repository/build README files, existing NSIS script, and build-release spec.

- [x] Implement `build/build-steamlink.ps1` with optional `-PluginRoot`, defaulting to repository `build/bin/plugins`. Resolve paths with `Join-Path`/`GetFullPath`, create its `steamlink` directory, initialize GOCACHE, build, and copy the manifest. Use argument arrays/direct invocation, not composed shell strings. Core operations after resolving `$repoPath` and `$pluginDir`:

```powershell
$env:GOCACHE = 'F:\dev\vrcft-go\.go-gocache'
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
New-Item -ItemType Directory -Force -Path $pluginDir | Out-Null
Push-Location $repoPath
try {
    go build -o (Join-Path $pluginDir 'steamlink-plugin.exe') ./cmd/steamlink-plugin
    if ($LASTEXITCODE -ne 0) { throw 'Steam Link plugin build failed' }
    Copy-Item -LiteralPath (Join-Path $repoPath 'plugins/steamlink/manifest.json') -Destination (Join-Path $pluginDir 'manifest.json')
} finally {
    Pop-Location
}
```

- [x] Implement `build/build-desktop.ps1` to run `wails build`, fail on errors, then call the plugin build script. Output must contain `build/bin/vrcft-go2.exe` and `build/bin/plugins/steamlink/{manifest.json,steamlink-plugin.exe}`, matching `internal/userconfig/paths.go`. Add optional `-NSIS`: stage both executables first, then invoke existing Wails installer packaging. Check local `wails build -help` for flags before implementation; do not add another installer system.
- [x] Extend the existing NSIS install section after `wails.files`:

```nsis
SetOutPath "$INSTDIR\plugins\steamlink"
File "..\..\bin\plugins\steamlink\manifest.json"
File "..\..\bin\plugins\steamlink\steamlink-plugin.exe"
SetOutPath "$INSTDIR"
```

Require files rather than using `/nonfatal`. Existing uninstall removes the install directory; do not add removal of user settings. Verify the packaging invocation preserves staged plugin files, rebuilding/staging before NSIS compilation if the existing command cleans them.

- [x] Document developer builds, builtin/dev-root selection, duplicate-ID avoidance, explicit enablement, port conflicts, Steam Link sharing settings, source provenance, synthetic fixtures and unsupported fields. Do not write user configuration automatically.
- [x] Smoke-test the build script with an output root containing spaces. Validate the staged directory through DirectoryCatalog with a real executable and matching manifest. Compile the existing installer when NSIS tools are available; otherwise report this precise verification gap while completing the folder build.
- [x] Extend build-release acceptance with plugin build/layout checks and the adapter spec with process-test evidence. Do not mark Pico hardware compatibility complete.
- [x] Commit as `build: package the Steam Link tracking plugin`.

## Task 9: Offline Acceptance and Handoff

**Files:** Finalize package specs and plugin README with measured results; check completed steps in this plan. Refresh generated status only if intentionally requested, from a clean reviewed source commit.

- [x] Run final verification with the fixed cache and stop on failures. Both
  30-second fuzz commands completed with `PASS` and exit code 0:

```powershell
go test ./pkg/osc ./pkg/pluginapi ./pkg/pluginruntime ./pkg/trackingmodel ./internal/steamlink ./cmd/steamlink-plugin ./internal/plugins ./internal/tracking ./internal/processing
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test -race ./pkg/osc ./pkg/pluginapi ./pkg/pluginruntime ./internal/steamlink ./internal/plugins ./internal/tracking ./internal/processing
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go vet ./internal/steamlink ./cmd/steamlink-plugin
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./pkg/osc -run '^$' -fuzz '^FuzzUnmarshalPacket$' -fuzztime 30s
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./internal/steamlink -run '^$' -fuzz '^FuzzDecodeDatagram$' -fuzztime 30s
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

- [x] Run the documented desktop build and validate staged artifacts. Inspect generator changes. Repeat tests only when subsequent modifications invalidate their evidence.
- [x] Run `go run ./cmd/projectstatus` for terminal evidence. Report current failures honestly; do not run `-write` on a dirty implementation tree or weaken checks.
- [x] Review production imports, bounded state, all mapping rows, cancellation, config transitions, and coverage below. Apply the code-review workflow before declaring offline implementation complete.
- [x] Commit final docs and report actual checks, remaining blockers, artifact paths, and: `Pico 4 Pro hardware compatibility has not yet been validated.`

## Deferred Phase B: One Hardware Session

Hardware is not a prerequisite for the offline tasks. Do not request headset setup while those tasks are in progress.

- [ ] Record SteamVR, Steam Link and Pico versions/permissions; confirm one sender and matching port 9015.
- [ ] Exercise centered/directional gaze, independent blinking, widening, jaw/cheek movements, group selection, reconnect, and head motion without gaze changes.
- [ ] With user agreement, collect minimal replay samples and inspect actual addresses/types, grouping, confidence, update rate, missing fields and tracking-loss behavior.
- [ ] Add approved recordings with provenance. Adjust mappings and rerun affected tests if actual Pico output differs. Missing required signals remain unsupported, not valid zeros.
- [ ] Record tested capabilities and limitations; only then claim measured Pico 4 Pro compatibility.

## Coverage and Self-Review

| Requirement | Tasks |
| --- | --- |
| Public OSC reuse, strict subset, full-packet rejection and limits | 1, 4 |
| Mapping, signed differences, optional widening, absent pupils | 2 |
| Dependencies and independent subscriptions | 2, 3, 5, 7 |
| Freshness, invalidation, no repeated cache frames, metadata | 3 |
| Socket/queue ownership, timestamps and epochs | 4, 5 |
| Controls, retry, health, logging and shutdown | 5, 7 |
| Descriptor, manifest, runtime and import boundaries | 6 |
| Real process supervision and host dropout | 7 |
| Discovery, development artifacts and existing installer | 8 |
| Package registration and full offline evidence | 1, 6, 8, 9 |
| Upstream provenance and deferred Pico acceptance | Baseline, 8, phase B |

Self-review before execution: keep private names/signatures consistent; register both packages; match the rebased codec behavior; retain zero-versus-missing semantics; isolate test-only host dependencies. No implementation task requires the headset.
