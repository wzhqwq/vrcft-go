# Public OSC Codec and UDP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish the reusable OSC wire subset and a synchronous UDP server in `pkg/osc`, then make the existing VRChat OSC transport consume that public implementation without changing host behavior.

**Architecture:** `pkg/osc` owns the standard-library-only value model, strict packet codec, and one-handler UDP Server. `internal/osc` keeps OSCQuery, VRChat discovery, VRCFT parameter compilation, target selection, and the optimized sender; a thin compatibility layer and UDP target adapter delegate generic wire/socket behavior to the public package.

**Tech Stack:** Go 1.25.6 standard library, Go fuzzing, loopback UDP tests, and the repository project-status generator.

**Spec:** `docs/superpowers/specs/2026-09-17-public-osc-codec-server-design.md`

## Global Constraints

- Keep all new code, comments, test names, errors, and documentation in English.
- `pkg/osc` must depend only on the Go standard library.
- Support only OSC `i`, `f`, `s`, `T`, and `F`; do not claim complete OSC 1.0 or 1.1 support.
- Do not move or change OSCQuery, mDNS, VRChat discovery, plugin IPC, or application lifecycle behavior.
- Keep routing caller-owned through one synchronous handler; do not add a mux or per-message goroutines.
- Silently drop malformed and unsupported UDP datagrams, while direct decoding retains `errors.Is` classifications.
- Keep the optimized scalar/bundle builder internal and preserve its benchmark evidence.
- Use the absolute repository-local `.go-gocache` for every Go command.
- Change `docs/project/status.md` only through `go run ./cmd/projectstatus -write` after a clean reviewed source commit.
- Preserve unrelated user changes and inspect `git status --short` before every commit.

## File Structure

- Create `pkg/osc/doc.go`: public scope and supported-subset documentation.
- Create `pkg/osc/packet.go`: values, errors, strict packet codec, address validation, and NTP conversion.
- Create `pkg/osc/packet_test.go` and `pkg/osc/fuzz_test.go`: public wire, validation, ownership, and fuzz evidence.
- Create `pkg/osc/server.go`: UDP Server lifecycle, synchronous handler, and explicit `SendTo`.
- Create `pkg/osc/server_test.go` and `pkg/osc/example_test.go`: loopback and plugin-facing evidence.
- Modify `internal/osc/packet.go`: public aliases and forwarding wrappers only.
- Modify `internal/osc/packet_test.go`: retain optimized-builder/public-codec parity tests.
- Modify `internal/osc/udp.go` and create `internal/osc/udp_test.go`: host target adapter over `pkg/osc.Server`.
- Create `docs/project/packages/pkg-osc.md`; update `internal-osc.md` and `milestones.md`.
- Modify `docs/project/status.md` through its generator only.

---

### Task 1: Publish and Harden the OSC Wire Codec

**Files:**
- Create: `pkg/osc/doc.go`
- Create: `pkg/osc/packet.go`
- Create: `pkg/osc/packet_test.go`
- Create: `pkg/osc/fuzz_test.go`

**Interfaces:**
- Consumes: only Go standard-library binary, error, string, and time types.
- Produces: `ValueKind`, `Value`, constructors, `Message`, `Bundle`, `MarshalMessage`, `MarshalBundle`, `UnmarshalPacket`, `NTPTime`, `ValidAddress`, and the six public sentinel errors specified below.

- [ ] **Step 1: Create the package shell and failing external-package tests**

Create `pkg/osc/doc.go`:

```go
// Package osc implements the OSC value and packet subset used by VRCFT-Go and
// its plugins. It supports int32, float32, string, boolean, messages, and
// bundles. Decoding preserves message order but flattens nested bundles and
// does not schedule their timetags.
package osc
```

Create `pkg/osc/packet_test.go` as `package osc_test`. Start with this public
round trip, then add exact-wire assertions for `i`, `f`, `s`, `T`, and `F`:

```go
func TestMessageRoundTrip(t *testing.T) {
    original := osc.Message{Address: "/plugin/status", Args: []osc.Value{
        osc.Int32(-4), osc.Float32(0.25), osc.String("ready"),
        osc.Bool(true), osc.Bool(false),
    }}
    packet, err := osc.MarshalMessage(original)
    if err != nil {
        t.Fatal(err)
    }
    messages, err := osc.UnmarshalPacket(packet)
    if err != nil {
        t.Fatal(err)
    }
    if !reflect.DeepEqual(messages, []osc.Message{original}) {
        t.Fatalf("messages = %#v, want %#v", messages, []osc.Message{original})
    }
}
```

Add `TestBundleRoundTripAndNestedOrder`, building raw nested bundles and
requiring addresses `[/one /two /three]`. Add zero-timetag normalization and
NTP epoch tests. Add `TestUnmarshalPacketRejectsMalformedInput` with short
input, relative address, nonzero padding, unsupported `b` tag, trailing bytes,
zero bundle element size, truncated element, and 33 nested bundle levels.
Check `ErrMalformedPacket` or `ErrUnsupportedType` using `errors.Is`.

Add marshal tests for invalid/valid addresses, address NUL, string NUL,
`ValueKind(255)`, empty/oversized/invalid bundle elements, and unchanged caller
element bytes. Add an ownership test that mutates the input packet after
decoding and confirms Address, string arguments, and Args remain unchanged.

- [ ] **Step 2: Run the tests and verify RED**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim()
$env:GOCACHE = Join-Path $repoRoot '.go-gocache'
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./pkg/osc -count=1
```

Expected: FAIL because the public codec API does not exist.

- [ ] **Step 3: Implement the exported model and strict decoder**

Create `pkg/osc/packet.go` from the existing internal codec and export these
sentinels:

```go
var (
    ErrMalformedPacket = errors.New("malformed OSC packet")
    ErrUnsupportedType = errors.New("unsupported OSC type")
    ErrInvalidAddress  = errors.New("invalid OSC address")
    ErrInvalidArgument = errors.New("invalid OSC argument")
    ErrServerRunning   = errors.New("OSC server is already serving")
    ErrServerClosed    = errors.New("OSC server is closed")
)

const maxBundleDepth = 32
```

Keep the approved `Value`, `Message`, and `Bundle` shapes. Make invalid
addresses wrap `ErrInvalidAddress` and string NUL wrap `ErrInvalidArgument`.
Validate every MarshalBundle element by decoding it before writing it.

Use a depth-aware private decoder and exact message consumption:

```go
func UnmarshalPacket(packet []byte) ([]Message, error) {
    return unmarshalPacket(packet, 0)
}

func unmarshalPacket(packet []byte, depth int) ([]Message, error) {
    if depth > maxBundleDepth || len(packet) < 4 {
        return nil, ErrMalformedPacket
    }
    if bytes.HasPrefix(packet, []byte("#bundle\x00")) {
        return unmarshalBundle(packet, depth)
    }
    message, consumed, err := unmarshalMessage(packet)
    if err != nil {
        return nil, err
    }
    if consumed != len(packet) {
        return nil, ErrMalformedPacket
    }
    return []Message{message}, nil
}
```

Pass `depth+1` for bundle children. Require all padding bytes after OSC strings
to be zero. Preserve zero-to-one timetag normalization and ensure returned
messages do not retain packet-buffer storage.

- [ ] **Step 4: Add the fuzz target**

Create `pkg/osc/fuzz_test.go` as `package osc_test`:

```go
func FuzzUnmarshalPacket(f *testing.F) {
    valid, err := osc.MarshalMessage(osc.Message{Address: "/seed", Args: []osc.Value{
        osc.Int32(1), osc.Float32(0.5), osc.String("ok"), osc.Bool(true),
    }})
    if err != nil {
        f.Fatal(err)
    }
    f.Add(valid)
    f.Add([]byte("#bundle\x00\x00\x00\x00\x00\x00\x00\x00\x01"))
    f.Add([]byte{0, 1, 2, 3})
    f.Fuzz(func(t *testing.T, packet []byte) {
        _, _ = osc.UnmarshalPacket(packet)
    })
}
```

- [ ] **Step 5: Verify codec tests, fuzzing, and race behavior**

```powershell
go test ./pkg/osc -count=1
go test ./pkg/osc -run '^$' -fuzz '^FuzzUnmarshalPacket$' -fuzztime 5s
go test -race ./pkg/osc -count=1
```

Expected: PASS with no fuzz panic.

- [ ] **Step 6: Commit the codec**

```powershell
git status --short
git add pkg/osc/doc.go pkg/osc/packet.go pkg/osc/packet_test.go pkg/osc/fuzz_test.go
git commit -m "feat(osc): publish strict packet codec"
```

---

### Task 2: Add the Unified-Handler UDP Server

**Files:**
- Create: `pkg/osc/server.go`
- Create: `pkg/osc/server_test.go`
- Create: `pkg/osc/example_test.go`

**Interfaces:**
- Consumes: Task 1's decoder and sentinel errors.
- Produces: `HandlerFunc`, `Server`, `ListenUDP`, `LocalAddr`, `Serve`, `SendTo`, and `Close` with the approved signatures.

- [ ] **Step 1: Write failing loopback and lifecycle tests**

Create external-package tests using loopback addresses and one-second
deadlines. The central test sends malformed bytes followed by a three-message
bundle and requires ordered delivery:

```go
func TestServerServesMessagesInOrderAndDropsMalformedDatagrams(t *testing.T) {
    server, err := osc.ListenUDP("127.0.0.1:0")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = server.Close() })
    ctx, cancel := context.WithCancel(context.Background())
    received := make(chan string, 3)
    serveErr := make(chan error, 1)
    go func() {
        serveErr <- server.Serve(ctx, func(message osc.Message, _ *net.UDPAddr) {
            received <- message.Address
        })
    }()
    client, err := net.DialUDP("udp", nil, server.LocalAddr())
    if err != nil {
        t.Fatal(err)
    }
    defer client.Close()
    _, _ = client.Write([]byte{1, 2, 3})
    writeBundle(t, client, "/one", "/two", "/three")
    for _, want := range []string{"/one", "/two", "/three"} {
        select {
        case got := <-received:
            if got != want { t.Fatalf("address = %q, want %q", got, want) }
        case <-time.After(time.Second):
            t.Fatalf("timed out waiting for %s", want)
        }
    }
    cancel()
    if err := <-serveErr; err != nil { t.Fatalf("Serve = %v", err) }
}
```

Add tests for nil context/handler, concurrent Serve rejection, cancellation
then sequential reuse, Close and handler-triggered Close unblocking Serve,
idempotent Close, copied LocalAddr available after Close, post-close
Serve/SendTo, nil target, empty packet, same-socket SendTo source port,
retained message/remote ownership, and concurrent lifecycle operations.

- [ ] **Step 2: Run the server tests and verify RED**

```powershell
go test ./pkg/osc -run '^TestServer' -count=1
```

Expected: FAIL because the Server API is undefined.

- [ ] **Step 3: Implement Server state and construction**

Create `pkg/osc/server.go` with:

```go
type HandlerFunc func(Message, *net.UDPAddr)

type Server struct {
    conn  *net.UDPConn
    local *net.UDPAddr
    mu      sync.Mutex
    serving bool
    closed  bool
    closeOnce sync.Once
    closeErr  error
}
```

`ListenUDP` resolves, listens, and caches a deep copy of the bound address.
`LocalAddr` always returns another deep copy. A private `cloneUDPAddr` copies
both the struct and IP bytes.

- [ ] **Step 4: Implement lifecycle, receive, and explicit sending**

Admit only one Serve under `mu`; never hold it during reads or callbacks. Use
`context.AfterFunc` to set an immediate read deadline on cancellation, then
stop that callback and clear the deadline before releasing Serve ownership.
Read into one reused 65,535-byte buffer. Continue on every decode error. Call
the handler synchronously in decoded order with a fresh remote-address copy.

Return nil on context cancellation or synchronized closed state. Wrap other
read failures with `read OSC UDP packet`. `SendTo` rejects empty packets and
nil targets, checks closed state, and normalizes a write racing with Close to
`ErrServerClosed`. `Close` marks closed before closing, uses `sync.Once`, and
normalizes `net.ErrClosed` to nil.

- [ ] **Step 5: Add plugin-facing examples**

In `pkg/osc/example_test.go`, add `ExampleServer` that listens on
`127.0.0.1:0`, cancels a context, and demonstrates the unified callback. Add
`ExampleMarshalMessage` for `/plugin/status` with one float argument. Use
external-package imports so examples prove the supported plugin surface.

- [ ] **Step 6: Verify repeated and race tests**

```powershell
go test ./pkg/osc -count=20
go test -race ./pkg/osc -count=10
```

Expected: PASS with no timeouts, races, or leaked Serve calls.

- [ ] **Step 7: Commit the server**

```powershell
git status --short
git add pkg/osc/server.go pkg/osc/server_test.go pkg/osc/example_test.go
git commit -m "feat(osc): add reusable UDP server"
```

---

### Task 3: Reuse the Public Package Inside VRChat OSC

**Files:**
- Modify: `internal/osc/packet.go`
- Modify: `internal/osc/packet_test.go`
- Modify: `internal/osc/udp.go`
- Create: `internal/osc/udp_test.go`

**Interfaces:**
- Consumes: public codec and `*pkgosc.Server` from Tasks 1-2.
- Produces: unchanged internal packet names and `UDPTransport` target-policy methods expected by Controller and ParameterSender.

- [ ] **Step 1: Write failing target-adapter tests**

Create `internal/osc/udp_test.go`. Verify SetTarget and Target deep-copy both
the UDPAddr and IP bytes. Add a loopback test that binds the transport, sends a
public-codec message to a separate receiver, decodes it, and requires the
source port to equal `transport.LocalAddr().Port`. Add delegated Serve,
target-nil, unstarted-transport, and idempotent-Close cases.

Because this is a behavior-preserving refactor, add an in-package structural
assertion to make the test RED before the adapter exists:

```go
func TestUDPTransportUsesPublicServer(t *testing.T) {
    transport, err := ListenUDP("127.0.0.1:0")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = transport.Close() })
    if transport.server == nil {
        t.Fatal("UDPTransport has no public OSC server")
    }
}
```

```go
func TestUDPTransportTargetOwnership(t *testing.T) {
    transport := &UDPTransport{}
    input := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000}
    transport.SetTarget(input)
    input.IP[0], input.Port = 8, 1
    first := transport.Target()
    if first.String() != "127.0.0.1:9000" { t.Fatalf("Target = %v", first) }
    first.IP[0] = 9
    if got := transport.Target().String(); got != "127.0.0.1:9000" {
        t.Fatalf("retained target changed to %s", got)
    }
}
```

- [ ] **Step 2: Run adapter tests and verify RED against the required shape**

```powershell
go test ./internal/osc -run '^TestUDPTransport' -count=1
```

Expected: FAIL to compile because `UDPTransport.server` does not exist. Do not
weaken the structural assertion to accept the old direct `net.UDPConn` field.

- [ ] **Step 3: Replace internal wire code with aliases**

Rewrite `internal/osc/packet.go` to import `pkgosc` and contain no binary codec:

```go
type ValueKind = pkgosc.ValueKind
const (
    ValueInt32 = pkgosc.ValueInt32
    ValueFloat32 = pkgosc.ValueFloat32
    ValueString = pkgosc.ValueString
    ValueBool = pkgosc.ValueBool
)
type Value = pkgosc.Value
type Message = pkgosc.Message
type Bundle = pkgosc.Bundle
var (
    ErrMalformedPacket = pkgosc.ErrMalformedPacket
    ErrUnsupportedType = pkgosc.ErrUnsupportedType
    Int32 = pkgosc.Int32
    Float32 = pkgosc.Float32
    String = pkgosc.String
    Bool = pkgosc.Bool
    MarshalMessage = pkgosc.MarshalMessage
    MarshalBundle = pkgosc.MarshalBundle
    NTPTime = pkgosc.NTPTime
    UnmarshalPacket = pkgosc.UnmarshalPacket
)
func validAddress(address string) bool { return pkgosc.ValidAddress(address) }
```

Remove generic round-trip tests now owned by `pkg/osc`; retain internal
optimized-builder versus public-codec wire parity tests.

- [ ] **Step 4: Convert UDPTransport into a target adapter**

Replace `conn *net.UDPConn` with `server *pkgosc.Server`. Delegate ListenUDP,
LocalAddr, Serve, and Close. Preserve empty `&UDPTransport{}` safety used by
Controller tests. Keep empty Send as a no-op, the current missing-target error,
and target copy semantics. After target lookup, reject an unstarted transport
and otherwise call `server.SendTo(packet, target)`.

- [ ] **Step 5: Verify OSC compatibility and hot-path performance**

```powershell
go test ./internal/osc -run 'Test(UDPTransport|MessageBuilder|BundleBuilder|ScalarWireEncoding|ImmediateBundleWireEncoding|ControllerManualTargetStillPublishesAvatarChanges)' -count=20
go test -race ./internal/osc -count=10
go test ./internal/osc -run '^$' -bench 'Benchmark(Marshal|MessageBuilder|BundleBuilder|ParameterSender)' -benchmem
go test ./pkg/osc ./internal/osc ./internal/avatar ./internal/application -count=1
go vet ./pkg/osc ./internal/osc ./internal/avatar ./internal/application
```

Expected: PASS; optimized unchanged-frame paths retain zero allocations and
OSCQuery/Controller/Application behavior is unchanged.

- [ ] **Step 6: Commit host integration**

```powershell
git status --short
git add internal/osc/packet.go internal/osc/packet_test.go internal/osc/udp.go internal/osc/udp_test.go
git commit -m "refactor(osc): reuse public codec and server"
```

---

### Task 4: Register the Public Package and Ownership Boundary

**Files:**
- Create: `docs/project/packages/pkg-osc.md`
- Modify: `docs/project/packages/internal-osc.md`
- Modify: `docs/project/milestones.md`

**Interfaces:**
- Consumes: completed public package and host adapter.
- Produces: authoritative M1 registration and executable project-status checks.

- [ ] **Step 1: Demonstrate missing package registration**

```powershell
go run ./cmd/projectstatus
```

Expected: degraded/failing output identifies `pkg/osc` as unregistered. Do not
use `-write` yet.

- [ ] **Step 2: Add `pkg-osc.md` with executable evidence**

Use this front matter:

```yaml
---
id: pkg-osc
kind: go-package
path: pkg/osc
milestone: M1
depends_on: []
checks:
  - id: package-tests
    description: Public OSC codec and server tests pass
    type: command
    command: go-test
    args: [./pkg/osc]
    weight: 4
    required: true
  - id: race-tests
    description: Public OSC server lifecycle is race-free
    type: command
    command: go-test-race
    args: [./pkg/osc]
    weight: 2
    required: true
  - id: fuzz-regression
    description: OSC decoder fuzz regression target exists
    type: symbol
    path: pkg/osc/fuzz_test.go
    pattern: '(?m)^func FuzzUnmarshalPacket\('
    weight: 1
    required: true
  - id: server-integration
    description: UDP malformed-drop and ordered delivery are tested
    type: symbol
    path: pkg/osc/server_test.go
    pattern: '(?m)^func TestServerServesMessagesInOrderAndDropsMalformedDatagrams\('
    weight: 2
    required: true
---
```

Write every standard package-spec section. Explicitly document the supported
types, flat/no-scheduling bundle behavior, synchronous handler, malformed-drop
policy, standard-library-only dependency, and no OSCQuery responsibility.

- [ ] **Step 3: Update internal ownership and M1 wording**

Change `internal-osc.md` to `depends_on: [internal-parameters, pkg-osc]`.
State that it consumes the public codec/server while retaining OSCQuery,
VRChat discovery, target policy, parameter compilation, optimized sending,
generation runtime, and Controller lifecycle. Preserve every current target-
policy acceptance statement. Extend M1 in `milestones.md` to name the public
OSC wire/server contract without changing milestone ordering.

- [ ] **Step 4: Verify registration without generating status**

```powershell
go run ./cmd/projectstatus
go run ./cmd/projectstatus -check
```

Expected: live report lists `pkg-osc` complete. `-check` may report only stale
generated status; no catalog or required-check failure is allowed.

- [ ] **Step 5: Commit package documentation**

```powershell
git status --short
git add docs/project/packages/pkg-osc.md docs/project/packages/internal-osc.md docs/project/milestones.md
git commit -m "docs: register public OSC package"
```

---

### Task 5: Review, Verify, and Generate Final Evidence

**Files:**
- Modify through generator only: `docs/project/status.md`

**Interfaces:**
- Consumes: Tasks 1-4 and design commit `cf15061`.
- Produces: reviewed full-repository evidence and a generated clean-source status snapshot.

- [ ] **Step 1: Format and review the complete range**

```powershell
gofmt -w pkg/osc/doc.go pkg/osc/packet.go pkg/osc/packet_test.go pkg/osc/fuzz_test.go pkg/osc/server.go pkg/osc/server_test.go pkg/osc/example_test.go internal/osc/packet.go internal/osc/packet_test.go internal/osc/udp.go internal/osc/udp_test.go
git diff --check cf15061..HEAD
git diff --stat cf15061..HEAD
git diff cf15061..HEAD -- pkg/osc internal/osc docs/project/packages docs/project/milestones.md
```

Expected: no uncommitted formatting change. Confirm no OSCQuery move, external
dependency, handler goroutine, public default target, packet-content error, or
unrelated diff.

- [ ] **Step 2: Run focused repeated/race/fuzz/benchmark verification**

```powershell
go test ./pkg/osc ./internal/osc -count=20
go test -race ./pkg/osc ./internal/osc -count=10
go test ./pkg/osc -run '^$' -fuzz '^FuzzUnmarshalPacket$' -fuzztime 10s
go test ./internal/osc -run '^$' -bench 'Benchmark(Marshal|MessageBuilder|BundleBuilder|ParameterSender)' -benchmem
```

Expected: PASS; benchmark evidence remains executable and unchanged-frame hot
paths remain zero allocation.

- [ ] **Step 3: Run full repository verification**

```powershell
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

Expected: PASS. If named-pipe sandbox restrictions block a required IPC test,
rerun the identical command with required permission rather than narrowing it.

- [ ] **Step 4: Commit review corrections and require clean source**

If review found changes, rerun their owning tests and commit them:

```powershell
git status --short
git add pkg/osc internal/osc docs/project/packages docs/project/milestones.md
git commit -m "fix(osc): address public package review"
```

Do not create an empty commit. Require empty `git status --short` output before
the next step.

- [ ] **Step 5: Generate status from the clean reviewed source commit**

```powershell
go run ./cmd/projectstatus -write
git diff -- docs/project/status.md
```

Expected: `Dirty: false`, the clean source commit, `pkg-osc` complete, no
failed required checks, and only `docs/project/status.md` changed.

- [ ] **Step 6: Commit generated evidence and check freshness**

```powershell
git add docs/project/status.md
git commit -m "docs: refresh project status for public OSC"
go run ./cmd/projectstatus -check
git status --short
```

Expected: freshness check PASS and clean worktree.

- [ ] **Step 7: Report the final handoff**

Report commit IDs, import path `github.com/wzhqwq/vrcft-go/pkg/osc`, supported
types, synchronous/malformed-drop contract, focused/full/race/vet/fuzz output,
benchmark allocation evidence, and generated status state. Explicitly state
that OSCQuery remains internal and unchanged.
