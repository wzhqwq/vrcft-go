# OSC Send Path Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the VRCFT parameter sender's allocation-heavy generic OSC path with a compiled, indexed, reusable-buffer scalar path while preserving its wire behavior.

**Architecture:** `BuildCatalog` compiles existing diagnostic bindings into an immutable address-sorted output plan. `ParameterSender` evaluates that plan into compact scalar values, compares indexed cache entries, and writes changed values directly into reusable message or bundle buffers; general OSC marshaling and decoding remain unchanged.

**Tech Stack:** Go standard library, `testing`, UDP loopback integration tests, Go benchmarks.

## Global Constraints

- Preserve existing OSC wire format and controller behavior.
- Keep `Value`, `Message`, `Bundle`, `MarshalMessage`, `MarshalBundle`, and `UnmarshalPacket` available.
- Keep `Catalog.Bindings` and `Catalog.RawMethods` readable for snapshots and diagnostics.
- Do not add dependencies or new OSC types.
- Cache values only after their datagram is sent successfully.
- Use `SenderConfig.MaxDatagram` as the bundle limit and preserve standalone fallback for one oversized message.

---

## File Structure

- Create `internal/osc/scalar.go`: compact outbound scalar representation and equality.
- Create `internal/osc/scalar_test.go`: scalar layout and equality tests.
- Create `internal/osc/builder.go`: reusable standalone-message and bundle encoders.
- Create `internal/osc/builder_test.go`: wire parity and bundle boundary tests.
- Modify `internal/osc/catalog.go`: compiled output types, plan construction, validation, sorting, deduplication, and hashing.
- Modify `internal/osc/catalog_test.go`: compiled-plan behavior tests.
- Modify `internal/osc/controller.go`: deep-copy the compiled output slice in catalog snapshots.
- Modify `internal/osc/sender.go`: indexed plan execution and reusable builders.
- Create `internal/osc/sender_test.go`: conversions, change detection, modes, plan switching, and retry tests.
- Create `internal/osc/sender_benchmark_test.go`: representative allocation benchmarks.
- Modify `internal/osc/parameter_catalog.go`: remove obsolete `SendPolicy`.

### Task 1: Compact outbound scalar values

**Files:**
- Create: `internal/osc/scalar.go`
- Create: `internal/osc/scalar_test.go`

**Interfaces:**
- Produces: `scalarValue`, `scalarKind`, `floatScalar(float32) scalarValue`, `intScalar(int32) scalarValue`, `boolScalar(bool) scalarValue`, `scalarValue.typeTag() byte`, and `scalarEqual(scalarValue, scalarValue, float32) bool`.
- Consumes: only `math` from the standard library.

- [ ] **Step 1: Write the failing scalar tests**

```go
package osc

import (
    "testing"
    "unsafe"
)

func TestScalarValueLayoutAndTags(t *testing.T) {
    if got := unsafe.Sizeof(scalarValue{}); got != 8 {
        t.Fatalf("sizeof(scalarValue) = %d, want 8", got)
    }
    tests := []struct {
        value scalarValue
        tag   byte
    }{
        {floatScalar(0.25), 'f'},
        {intScalar(-4), 'i'},
        {boolScalar(false), 'F'},
        {boolScalar(true), 'T'},
    }
    for _, test := range tests {
        if got := test.value.typeTag(); got != test.tag {
            t.Fatalf("typeTag() = %q, want %q", got, test.tag)
        }
    }
}

func TestScalarEqual(t *testing.T) {
    if !scalarEqual(floatScalar(1), floatScalar(1.0005), 0.001) {
        t.Fatal("floats within epsilon differ")
    }
    if scalarEqual(floatScalar(1), floatScalar(1.002), 0.001) {
        t.Fatal("floats outside epsilon compare equal")
    }
    if !scalarEqual(intScalar(-4), intScalar(-4), 0.001) {
        t.Fatal("equal ints differ")
    }
    if scalarEqual(boolScalar(false), boolScalar(true), 0.001) {
        t.Fatal("different bools compare equal")
    }
    if scalarEqual(intScalar(1), floatScalar(1), 0.001) {
        t.Fatal("different scalar kinds compare equal")
    }
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/osc -run 'TestScalar(ValueLayoutAndTags|Equal)$'`

Expected: build failure because `scalarValue` and its constructors do not exist.

- [ ] **Step 3: Implement the compact scalar**

```go
package osc

import "math"

type scalarKind uint8

const (
    scalarFloat32 scalarKind = iota + 1
    scalarInt32
    scalarFalse
    scalarTrue
)

type scalarValue struct {
    bits uint32
    kind scalarKind
}

func floatScalar(value float32) scalarValue {
    return scalarValue{bits: math.Float32bits(value), kind: scalarFloat32}
}

func intScalar(value int32) scalarValue {
    return scalarValue{bits: uint32(value), kind: scalarInt32}
}

func boolScalar(value bool) scalarValue {
    if value {
        return scalarValue{kind: scalarTrue}
    }
    return scalarValue{kind: scalarFalse}
}

func (value scalarValue) typeTag() byte {
    switch value.kind {
    case scalarFloat32:
        return 'f'
    case scalarInt32:
        return 'i'
    case scalarFalse:
        return 'F'
    case scalarTrue:
        return 'T'
    default:
        return 0
    }
}

func scalarEqual(left, right scalarValue, epsilon float32) bool {
    if left.kind != right.kind {
        return false
    }
    if left.kind == scalarFloat32 {
        delta := math.Float32frombits(left.bits) - math.Float32frombits(right.bits)
        return float32(math.Abs(float64(delta))) <= epsilon
    }
    return left.bits == right.bits
}
```

- [ ] **Step 4: Run focused and package tests**

Run: `go test ./internal/osc`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/osc/scalar.go internal/osc/scalar_test.go
git commit -m "feat(osc): add compact outbound scalar"
```

### Task 2: Reusable scalar message and bundle builders

**Files:**
- Create: `internal/osc/builder.go`
- Create: `internal/osc/builder_test.go`

**Interfaces:**
- Consumes: `scalarValue` and `scalarKind` from Task 1.
- Produces: `messageBuilder.encodeScalar(address string, value scalarValue) ([]byte, error)`, `newBundleBuilder(maxDatagram int) bundleBuilder`, `(*bundleBuilder).reset()`, `(*bundleBuilder).appendScalar(string, scalarValue) (bool, error)`, `(*bundleBuilder).bytes() []byte`, and `(*bundleBuilder).empty() bool`.

- [ ] **Step 1: Write failing byte-parity and boundary tests**

```go
package osc

import (
    "bytes"
    "strings"
    "testing"
)

func TestMessageBuilderMatchesMarshalMessage(t *testing.T) {
    tests := []struct {
        address string
        scalar  scalarValue
        value   Value
    }{
        {"/float", floatScalar(0.25), Float32(0.25)},
        {"/int", intScalar(-4), Int32(-4)},
        {"/false", boolScalar(false), Bool(false)},
        {"/true", boolScalar(true), Bool(true)},
    }
    var builder messageBuilder
    for _, test := range tests {
        got, err := builder.encodeScalar(test.address, test.scalar)
        if err != nil { t.Fatal(err) }
        want, err := MarshalMessage(Message{Address: test.address, Args: []Value{test.value}})
        if err != nil { t.Fatal(err) }
        if !bytes.Equal(got, want) {
            t.Fatalf("packet for %s = %x, want %x", test.address, got, want)
        }
    }
}

func TestBundleBuilderFramesAndLimitsElements(t *testing.T) {
    first, _ := MarshalMessage(Message{Address: "/one", Args: []Value{Float32(1)}})
    second, _ := MarshalMessage(Message{Address: "/two", Args: []Value{Bool(false)}})
    max := 16 + 4 + len(first) + 4 + len(second)
    builder := newBundleBuilder(max)
    if ok, err := builder.appendScalar("/one", floatScalar(1)); err != nil || !ok {
        t.Fatalf("append first = %v, %v", ok, err)
    }
    if ok, err := builder.appendScalar("/two", boolScalar(false)); err != nil || !ok {
        t.Fatalf("append second = %v, %v", ok, err)
    }
    want, _ := MarshalBundle(Bundle{Timetag: 1, Elements: [][]byte{first, second}})
    if !bytes.Equal(builder.bytes(), want) {
        t.Fatalf("bundle = %x, want %x", builder.bytes(), want)
    }
    if ok, err := builder.appendScalar("/three", intScalar(3)); err != nil || ok {
        t.Fatalf("over-limit append = %v, %v", ok, err)
    }
}

func TestBuildersRejectInvalidInput(t *testing.T) {
    var message messageBuilder
    if _, err := message.encodeScalar("invalid", floatScalar(1)); err == nil {
        t.Fatal("invalid address accepted")
    }
    invalid := scalarValue{kind: 255}
    if _, err := message.encodeScalar("/valid", invalid); err == nil {
        t.Fatal("invalid scalar accepted")
    }
    bundle := newBundleBuilder(1200)
    if _, err := bundle.appendScalar("/"+strings.Repeat("x", 1201), invalid); err == nil {
        t.Fatal("invalid scalar accepted by bundle")
    }
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/osc -run 'Test(MessageBuilder|BundleBuilder|Builders)'`

Expected: build failure because builder types do not exist.

- [ ] **Step 3: Implement direct append encoding**

Create builders using `encoding/binary.AppendUint32`, a shared
`appendPaddedString([]byte, string) []byte`, and a shared
`appendScalarMessage([]byte, string, scalarValue) ([]byte, error)`. Validate the
address with `validAddress`, reject `typeTag() == 0` with `ErrUnsupportedType`,
and append payload bytes only for `scalarFloat32` and `scalarInt32`.

```go
type messageBuilder struct { buffer []byte }

func (builder *messageBuilder) encodeScalar(address string, value scalarValue) ([]byte, error) {
    packet, err := appendScalarMessage(builder.buffer[:0], address, value)
    if err != nil { return nil, err }
    builder.buffer = packet
    return packet, nil
}

type bundleBuilder struct {
    buffer []byte
    maxDatagram int
}

func newBundleBuilder(maxDatagram int) bundleBuilder {
    builder := bundleBuilder{buffer: make([]byte, 0, maxDatagram), maxDatagram: maxDatagram}
    builder.reset()
    return builder
}
```

`appendScalar` must first compute `scalarMessageSize(address, value)`, return
`false, nil` without mutating the buffer when the element cannot fit, reserve
four bytes, append the message, and backfill the element length with
`binary.BigEndian.PutUint32`.

- [ ] **Step 4: Run builder and package tests**

Run: `go test ./internal/osc`

Expected: PASS, including byte-for-byte parity with the general marshaler.

- [ ] **Step 5: Commit**

```bash
git add internal/osc/builder.go internal/osc/builder_test.go
git commit -m "feat(osc): add reusable scalar packet builders"
```

### Task 3: Compile an immutable output plan in the catalog

**Files:**
- Modify: `internal/osc/catalog.go`
- Modify: `internal/osc/catalog_test.go`
- Modify: `internal/osc/controller.go`
- Modify: `internal/osc/parameter_catalog.go`

**Interfaces:**
- Produces: `outputOperation`, `outputBinding`, and `Catalog.Outputs []outputBinding`.
- Consumes: existing `ParameterBinding`, `ParameterSpec`, `Endpoint`, and binary grouping logic.

- [ ] **Step 1: Add failing compiled-plan tests**

Extend `catalog_test.go` with a query tree containing direct float (`f`), direct
integer (`i`), boolean exposed as float (`f`), signed negative (`T`), and binary
bits. Assert:

```go
if got := len(catalog.Outputs); got != expectedCount {
    t.Fatalf("outputs = %d, want %d", got, expectedCount)
}
for index, output := range catalog.Outputs {
    if output.CacheIndex != uint16(index) {
        t.Fatalf("cache index %d = %d", index, output.CacheIndex)
    }
    if index > 0 && catalog.Outputs[index-1].Address > output.Address {
        t.Fatalf("outputs are not address sorted: %#v", catalog.Outputs)
    }
}
```

Also assert that binary bit outputs store the group's OR-ed maximum, signed
negative outputs use the sign operation, and a boolean source targeting `f`
retains a float wire kind.

- [ ] **Step 2: Run the catalog tests and verify they fail**

Run: `go test ./internal/osc -run 'TestBuildCatalog'`

Expected: build failure because `Catalog.Outputs` does not exist.

- [ ] **Step 3: Define compiled output types**

Add these private types:

```go
type outputOperation uint8

const (
    outputDirectFloat outputOperation = iota + 1
    outputDirectBool
    outputBinaryNegative
    outputBinaryBit
)

type outputBinding struct {
    Parameter   parameters.ParameterID
    Address     string
    Operation   outputOperation
    WireKind    scalarKind
    Weight      uint32
    QuantizeMax uint32
    Range       parameters.ValueRange
    HasRange    bool
    CacheIndex  uint16
}
```

Map endpoint types with a helper: `f` to `scalarFloat32`, `i` to `scalarInt32`,
and `T`/`F` to a boolean placeholder kind whose true/false form is chosen during
evaluation.

- [ ] **Step 4: Compile, validate, sort, and index outputs**

After existing binding grouping, flatten direct endpoints and binary groups.
Compute each binary group's `QuantizeMax` once by OR-ing all weights. Sort by
address, then parameter, operation, wire kind, and weight. Collapse identical
entries. Return a catalog error when the same address has non-identical compiled
semantics. Reject more than `math.MaxUint16 + 1` outputs before assigning
`CacheIndex`.

Update `hashCatalog` to hash every effective output field in sorted order rather
than walking the binding map. Add a named `ErrConflictingOSCAddress` error for
conflicts.

- [ ] **Step 5: Preserve snapshot immutability and remove SendPolicy**

In `Controller.Catalog`, add:

```go
copyCatalog.Outputs = append([]outputBinding(nil), catalog.Outputs...)
```

Delete `type SendPolicy uint8` from `parameter_catalog.go`.

- [ ] **Step 6: Run catalog and full repository tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/osc/catalog.go internal/osc/catalog_test.go internal/osc/controller.go internal/osc/parameter_catalog.go
git commit -m "feat(osc): compile catalog output plan"
```

### Task 4: Execute the compiled plan with indexed caching

**Files:**
- Modify: `internal/osc/sender.go`
- Create: `internal/osc/sender_test.go`

**Interfaces:**
- Consumes: `Catalog.Outputs`, scalar helpers, `messageBuilder`, and `bundleBuilder`.
- Produces: the unchanged public constructor, catalog methods, reset method, and `Send(ValueSource) error` behavior.

- [ ] **Step 1: Add a transport seam and failing sender tests**

Define the narrow internal interface:

```go
type packetSender interface { Send([]byte) error }
```

Keep `NewParameterSender(transport *UDPTransport, config SenderConfig)` unchanged
and delegate to an unexported constructor accepting `packetSender` for tests.
Use this fake:

```go
type recordingSender struct {
    packets [][]byte
    failAt  int
}

func (sender *recordingSender) Send(packet []byte) error {
    if sender.failAt > 0 && len(sender.packets)+1 == sender.failAt {
        return errors.New("injected send failure")
    }
    sender.packets = append(sender.packets, append([]byte(nil), packet...))
    return nil
}
```

Write table tests that build small catalogs and verify:

- float sources produce `f` and rounded `i` messages;
- bool sources produce `T`/`F`, `0`/`1`, or `0.0`/`1.0` by target type;
- signed and unsigned binary values match the old quantization results;
- a second equal send emits no packet and an epsilon-exceeding value does;
- `ResetChangeDetection` resends all current outputs;
- installing a different catalog resizes and invalidates the indexed cache;
- bundle packets never exceed `MaxDatagram`;
- non-bundle mode emits one standalone message per changed output;
- a failed send does not commit cache entries and the next call retries them.

- [ ] **Step 2: Run sender tests and verify they fail**

Run: `go test ./internal/osc -run 'TestParameterSender'`

Expected: failures because sender still walks `Bindings`, sorts messages, and
updates its map cache before sending.

- [ ] **Step 3: Replace sender state with indexed caches and builders**

Use:

```go
type cachedScalar struct {
    value scalarValue
    valid bool
}

type ParameterSender struct {
    transport packetSender
    config SenderConfig
    catalog atomic.Pointer[Catalog]
    mu sync.Mutex
    last []cachedScalar
    messageBuilder messageBuilder
    bundleBuilder bundleBuilder
}
```

Initialize `bundleBuilder` after applying config defaults. Make `SetCatalog`
lock before publishing the pointer and replace `last` when the hash changes.
Make `Send` lock before loading the pointer.

- [ ] **Step 4: Implement output evaluation**

Add:

```go
func evaluateOutput(output outputBinding, source ValueSource) (scalarValue, bool, error)
```

Direct floats read `source.Float`, reject non-finite values, clamp to the compiled
range, and convert according to `WireKind`. Direct bools read `source.Bool` and
convert to boolean, integer zero/one, or float zero/one. Binary negative and bit
operations read finite floats, reuse `binaryMagnitude`, and return the requested
wire representation. Unknown operation or wire kinds return
`ErrUnsupportedType` with operation/address context.

- [ ] **Step 5: Implement transactional send batching**

Track pending cache updates as output indexes for the current datagram. In bundle
mode, append changed scalars until the builder is full, send the bundle, and only
then commit those indexes. In non-bundle mode, send one encoded scalar and commit
its index after success. For a scalar too large for a bundle, encode it with the
message builder, send it standalone, and commit only after success.

Do not mutate `last` during change comparison. Remove `changedMessage`,
`binaryMessages`, `sendMessages`, `floatToEndpoint`, `boolToEndpoint`,
`valuesEqual`, the address map, per-frame slices, and `sort.Slice`.

- [ ] **Step 6: Run sender, race, and repository tests**

Run: `go test -race ./internal/osc`

Expected: PASS with no data races.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/osc/sender.go internal/osc/sender_test.go
git commit -m "perf(osc): execute compiled scalar send plan"
```

### Task 5: Allocation benchmarks and final verification

**Files:**
- Create: `internal/osc/sender_benchmark_test.go`
- Modify: `internal/osc/packet_test.go`

**Interfaces:**
- Consumes: completed scalar builders and sender.
- Produces: repeatable benchmark output and wire-compatibility regression coverage.

- [ ] **Step 1: Add wire regression cases**

Extend `packet_test.go` with exact hexadecimal or byte-slice expectations for one
float, one int, true, false, and a two-element immediate bundle. Exercise both
the generic marshaler and optimized builders against the same expected bytes.

- [ ] **Step 2: Add allocation benchmarks**

Create these benchmarks using `b.ReportAllocs()`. Each benchmark constructs its
fixtures before `b.ResetTimer()`, loops from zero to `b.N`, calls the named path,
and stores the resulting packet length in a package-level integer sink:

```go
func BenchmarkMarshalScalarMessage(b *testing.B)
func BenchmarkMessageBuilderScalar(b *testing.B)
func BenchmarkMarshalScalarBundle(b *testing.B)
func BenchmarkBundleBuilderScalars(b *testing.B)
func BenchmarkParameterSenderUnchangedFrame(b *testing.B)
func BenchmarkParameterSenderChangedFrame(b *testing.B)
```

The message pair encodes `/avatar/parameters/v2/JawX` with float `0.25`. The
bundle pair encodes that float plus a true boolean at
`/avatar/parameters/v2/TrackingActive`. Sender benchmarks use the same two
outputs; the unchanged case primes the cache before timing, while the changed
case alternates the float between `0.25` and `0.5` each iteration.

Use a discard `packetSender` whose `Send` method only records packet length so
the compiler cannot eliminate the call. Build a representative catalog once
outside each timed loop.

- [ ] **Step 3: Run formatting and static checks**

Run: `gofmt -w internal/osc/scalar.go internal/osc/scalar_test.go internal/osc/builder.go internal/osc/builder_test.go internal/osc/catalog.go internal/osc/catalog_test.go internal/osc/controller.go internal/osc/parameter_catalog.go internal/osc/sender.go internal/osc/sender_test.go internal/osc/packet_test.go internal/osc/sender_benchmark_test.go`

Run: `go vet ./...`

Expected: PASS with no diagnostics.

- [ ] **Step 4: Run final tests and benchmarks**

Run: `go test -race ./...`

Expected: PASS with no data races.

Run: `go test ./internal/osc -run '^$' -bench 'Benchmark(Marshal|MessageBuilder|BundleBuilder|ParameterSender)' -benchmem`

Expected: all benchmarks complete; builder benchmarks report fewer allocations
than their generic marshaler counterparts, and the unchanged-frame sender path
reports zero steady-state allocations.

- [ ] **Step 5: Check the final diff**

Run: `git diff --check`

Expected: no output.

Run: `git status --short`

Expected: only the intended OSC implementation and test files are listed.

- [ ] **Step 6: Commit**

```bash
git add internal/osc/packet_test.go internal/osc/sender_benchmark_test.go
git commit -m "test(osc): benchmark optimized send path"
```
