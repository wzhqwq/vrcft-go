# Public OSC Codec and UDP Server Design

## Summary

Extract the reusable OSC wire codec from `internal/osc` into a new public
`pkg/osc` package and add a small UDP server suitable for third-party plugins.
The public package owns only common OSC values, packet encoding and decoding,
and socket-level receive/send behavior. It does not expose OSCQuery, mDNS,
VRChat discovery, VRCFT parameter bindings, target selection, or application
lifecycle policy.

The UDP server preserves the current VRC OSC receive model: one synchronous
callback receives every decoded message, and the caller decides how to route
by OSC address. Malformed or unsupported datagrams are silently dropped
without stopping the server.

## Goals

- Let external plugin modules import a supported OSC package without copying
  code from the repository's Go `internal` tree.
- Make the OSC wire codec have one public source of truth shared by plugins and
  the host.
- Provide a receive-oriented UDP server with a unified callback and bounded,
  deterministic lifecycle.
- Let the host continue receiving and sending through the same bound UDP
  socket while retaining its existing mutable-target policy internally.
- Preserve the optimized VRCFT parameter send path and its wire format.
- Harden the public decoder against malformed, trailing, and excessively
  nested input before treating it as a plugin-facing API.

## Non-Goals

- Complete OSC 1.0 or OSC 1.1 type coverage.
- OSCQuery, HTTP, WebSocket, mDNS, or service discovery APIs.
- Address-pattern matching, handler registration, or a routing multiplexer.
- Timetag scheduling or preservation of a decoded bundle tree.
- A default remote target, reconnect policy, retransmission, ordering across
  UDP datagrams, or delivery guarantees.
- Public VRCFT parameter catalogs, parameter senders, generation fences, or
  avatar-change semantics.
- Changes to the plugin IPC protocol, `pluginapi.Driver`, or Host behavior.

## Package Boundary

`pkg/osc` depends only on the Go standard library. It owns:

- the supported OSC value kinds and constructors;
- `Message` and raw-element `Bundle` values;
- message and bundle encoding;
- packet decoding into an ordered flat message list;
- NTP timetag conversion;
- OSC address validation used by the supported encoder; and
- a UDP `Server` that receives, decodes, dispatches, and sends raw packets to
  an explicit address.

`internal/osc` continues to own:

- VRChat and OSCQuery service discovery and target selection;
- OSCQuery client, server, tree, and change-notification behavior;
- VRCFT parameter definitions, endpoint matching, catalogs, and binary
  parameter encoding;
- change suppression and the zero-allocation compiled send path;
- generation-fenced send runtime and avatar-change mailbox; and
- Controller, Application-facing service, status, and lifecycle policy.

The dependency direction is:

```text
external plugin ------> pkg/osc ------> standard library
                           ^
                           |
internal/osc --------------+
    |
    +-- internal/parameters and host-only policies
```

Neither `pkg/pluginapi` nor `pkg/pluginruntime` imports `pkg/osc`. Plugins use
the package as an optional source-level utility, not as an IPC capability.

## Supported Wire Contract

The first public version supports the value types already exercised by the
host and VRChat integration:

- OSC `i` as `int32`;
- OSC `f` as `float32`;
- OSC `s` as string;
- OSC `T` and `F` as bool;
- OSC messages; and
- OSC bundles, including nested bundles during decoding.

The package documentation calls this the project's commonly used OSC subset;
it does not claim complete OSC 1.0 conformance. Additional value kinds such as
blob, int64, or double may be added later without changing existing names.

The exported wire API is:

```go
type ValueKind uint8

const (
    ValueInt32 ValueKind = iota + 1
    ValueFloat32
    ValueString
    ValueBool
)

type Value struct {
    Kind ValueKind
    I32  int32
    F32  float32
    Str  string
    Bool bool
}

func Int32(int32) Value
func Float32(float32) Value
func String(string) Value
func Bool(bool) Value

type Message struct {
    Address string
    Args    []Value
}

type Bundle struct {
    Timetag  uint64
    Elements [][]byte
}

func MarshalMessage(Message) ([]byte, error)
func MarshalBundle(Bundle) ([]byte, error)
func UnmarshalPacket([]byte) ([]Message, error)
func NTPTime(time.Time) uint64
func ValidAddress(string) bool
```

`MarshalMessage` rejects unsupported value kinds, invalid addresses, and
strings containing NUL. `MarshalBundle` normalizes timetag zero to the OSC
immediate value one and rejects empty, oversized, or invalid packet elements.

`UnmarshalPacket` returns messages in wire order. It recursively decodes
nested bundles but returns a flat message slice and deliberately discards
bundle timetags; scheduling remains the caller's responsibility. Decoding
requires every message and bundle element to be consumed exactly, validates
zero padding, rejects unsupported tags, and enforces a fixed nesting limit.
These rules apply identically when decoding directly or through `Server`.

Decoded messages and argument slices own their retained data and do not refer
to a reusable UDP receive buffer. Encoded byte slices are newly owned by the
caller. `MarshalBundle` does not mutate caller element slices.

The public sentinel errors are `ErrMalformedPacket`, `ErrUnsupportedType`,
`ErrInvalidAddress`, `ErrInvalidArgument`, `ErrServerRunning`, and
`ErrServerClosed`. Operation-specific errors wrap those classifications so
callers can use `errors.Is` without parsing text. An empty SendTo packet is
malformed; a nil target, context, or handler is an invalid argument; and Serve
or SendTo after Close reports a closed server.

## UDP Server API

The public server API is:

```go
type HandlerFunc func(Message, *net.UDPAddr)

type Server struct {
    // unexported
}

func ListenUDP(address string) (*Server, error)
func (s *Server) LocalAddr() *net.UDPAddr
func (s *Server) Serve(context.Context, HandlerFunc) error
func (s *Server) SendTo([]byte, *net.UDPAddr) error
func (s *Server) Close() error
```

`ListenUDP` resolves and binds a UDP address. Port zero is supported. A
successfully constructed Server owns its socket until `Close`.

`Serve` blocks and permits only one active call per Server. A second
concurrent call returns a stable server-running error. After a Serve call ends
through context cancellation, the same Server may serve again if it has not
been closed. A nil context or handler is rejected.

Each valid datagram is decoded and its messages are passed synchronously to
the one handler in wire order. This preserves bounded resource use and the
current VRC OSC behavior. A slow handler delays later reads. The package does
not recover handler panics. Address routing, if wanted, is an ordinary switch
or a caller-owned helper above the Server.

Malformed packets and packets containing unsupported value tags are dropped
without calling the handler and without ending Serve. This is intentional
compatibility with the current receiver and prevents untrusted UDP input from
turning a parse error into a service failure.

Context cancellation wakes a blocked read promptly and makes Serve return
nil. Calling `Close`, including from the handler, also wakes Serve and is a
normal nil-returning shutdown. Other socket read failures return an error with
operation context.

`SendTo` sends an already encoded packet through the same bound socket. It is
safe to call concurrently with Serve and other SendTo calls. It rejects a nil
target and an empty packet but does not decode the packet again. It retains no
default target and performs no target selection. The packet need only remain
unchanged until SendTo returns.

`LocalAddr` returns an independently owned address value cached at
construction and remains available after Close. The remote address passed to
a handler is also independently owned and may be retained after the callback.
`Close` is idempotent, concurrency-safe, and permanently closes the Server.

## Host Integration and Compatibility

The current generic definitions and functions move from
`internal/osc/packet.go` to `pkg/osc`. A temporary internal compatibility file
aliases the public value types and forwards the codec functions so the
existing Controller and focused tests do not need a simultaneous naming
rewrite:

```go
type Message = pkgosc.Message
type Value = pkgosc.Value

var MarshalMessage = pkgosc.MarshalMessage
var UnmarshalPacket = pkgosc.UnmarshalPacket
```

The compatibility layer contains no wire implementation. Internal address
validation delegates to the public validator.

The receive and raw-send socket behavior moves under `pkg/osc.Server`.
`internal/osc.UDPTransport` becomes a host-policy adapter containing:

- one `pkg/osc.Server`;
- the synchronized default target used by the Controller;
- `SetTarget` and `Target` copy semantics; and
- `Send`, which loads the current target and delegates to `Server.SendTo`.

Its existing `Serve`, `LocalAddr`, and `Close` methods delegate to the public
Server. The Controller therefore continues using one local port for receiving
VRChat messages and sending VRCFT output.

The private `scalar.go` and `builder.go` remain in `internal/osc`. They are an
optimized implementation of the compiled parameter hot path, not public
protocol concepts. Wire-parity tests continue comparing their output with the
public codec so optimizations cannot silently diverge.

OSCQuery files, APIs, dependencies, and behavior do not move or change.

## Concurrency and Lifecycle

Server state synchronizes active Serve ownership and permanent closure without
holding a mutex while invoking user code or performing a blocking read. The
underlying UDP connection supports concurrent reads and writes; the package
adds only the state needed to reject concurrent Serve calls and make Close
idempotent.

Serve installs cancellation wake-up before blocking and removes it before
releasing active-Serve ownership. Read deadlines are cleared before a later
Serve call so a canceled run cannot poison a restart. Close and cancellation
are distinguished from unexpected network errors through synchronized server
state and context state, not error-string matching.

No goroutine is created per packet or message. The Server may use only bounded
control machinery needed to wake a blocked read. Its receive buffer is sized
for one maximum UDP datagram and is reused between reads; decoded retained
values do not alias that buffer.

## Error and Security Model

- Invalid construction and direct method inputs return explicit errors.
- Untrusted malformed datagrams are silently discarded.
- Unsupported wire type tags are treated like malformed input by Serve but
  remain distinguishable when callers invoke `UnmarshalPacket` directly.
- Exact packet consumption, zero padding checks, positive bundle element
  lengths, datagram bounds, and a nesting limit prevent ambiguous parsing and
  recursive resource exhaustion.
- The server does not trust or filter remote addresses, authorize senders, or
  implement application-level authentication. Plugins that need such policy
  inspect the supplied remote address in their handler.
- Error strings contain operation and address context but never packet
  contents.

## Testing Strategy

### `pkg/osc` codec

Tests use external-package style where practical to prove the plugin-facing
surface. They cover:

- exact wire encoding and round trips for every supported value kind;
- ordinary, immediate, and nested bundles with stable message order;
- zero timetag normalization and NTP conversion;
- invalid addresses and embedded NUL strings;
- truncation, unsupported tags, non-zero padding, trailing bytes, invalid
  bundle lengths and elements, and excessive nesting;
- caller/callee ownership of messages, arguments, elements, and output bytes;
- errors.Is classifications; and
- fuzz decoding with a no-panic invariant.

### `pkg/osc` server

Loopback tests cover:

- message and bundle delivery through one handler;
- synchronous callback ordering;
- malformed and unsupported datagrams followed by a valid datagram;
- context cancellation and Close unblocking Serve;
- sequential Serve reuse after cancellation;
- concurrent Serve rejection;
- SendTo using the bound socket;
- concurrent Serve, SendTo, LocalAddr, and Close;
- idempotent Close and post-close method behavior; and
- ownership of delivered messages and remote addresses.

### Host compatibility

`internal/osc` tests continue covering:

- public codec versus optimized-builder wire parity;
- UDPTransport target copying, replacement, clearing, and send behavior;
- Controller `/avatar/change` receive behavior;
- sender retries, bundles, change suppression, and benchmarks;
- OSCQuery behavior remaining internal and unchanged; and
- Controller/runtime race safety.

Required verification uses the repository-local absolute `GOCACHE` and
includes focused normal and race tests, relevant benchmarks where wire-path
allocations may change, full repository tests, full race tests, and `go vet`.

## Project Evidence

Add `docs/project/packages/pkg-osc.md` as a public shared-contract package and
register its normal, race, fuzz-regression, and server integration evidence.
Update `docs/project/packages/internal-osc.md` to depend on `pkg-osc` and to
state that it retains only host-specific OSC/OSCQuery integration and the
optimized VRCFT send path.

The generated project status is refreshed only after implementation, review,
and required verification from an otherwise clean reviewed source commit, in
accordance with repository policy.

## Completion Definition

The work is complete when:

1. an external-style consumer can encode, decode, listen for, and explicitly
   send supported OSC packets using only `pkg/osc` and the standard library;
2. malformed UDP input is dropped while later valid input is delivered;
3. Server cancellation, Close, sequential reuse, and concurrent-operation
   contracts pass normal and race tests;
4. the host still receives `/avatar/change` and sends VRCFT output through one
   bound socket without changing target-selection behavior;
5. the private optimized sender remains wire-compatible and retains its
   required benchmark evidence;
6. OSCQuery remains internal and behaviorally unchanged; and
7. package specifications and generated project evidence accurately describe
   the new boundary.
