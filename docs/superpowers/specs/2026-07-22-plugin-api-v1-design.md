# Plugin API v1 Design

## Goal

Replace the current declaration-only plugin API with a validated, testable v1
contract and carry selective tracking subscriptions through `pkg/pluginapi`,
`pkg/protocol`, and `pkg/pluginruntime`.

The repository has not been released and no third-party plugins exist, so this
design intentionally permits breaking changes to the current provisional Go
API. A plugin process represents exactly one device and one logical tracking
source.

## Scope

This change covers:

- the public contract used by vendor driver authors;
- coarse capability and optional field-level subscriptions;
- the shared expression-selection bitset in `pkg/trackingmodel`;
- protocol v1 messages and the connection abstraction;
- the plugin-side runtime that maps protocol messages to the public API;
- validation, compatibility, concurrency, lifecycle, and example-driver tests.

This change does not implement host process supervision, host IPC transports,
plugin discovery, avatar file loading, subscription planning, tracking merge,
or OSC output. Those components consume the contracts established here.

## Package Responsibilities

### `pkg/trackingmodel`

`pkg/trackingmodel` owns the expression bit layout already used by
`ExpressionSet`. It exposes `ExpressionMask`, backed by the same word count, and
operations to test, set, normalize, intersect, and detect an empty mask. This
prevents `pluginapi` and `protocol` from duplicating private layout constants.

### `pkg/pluginapi`

`pkg/pluginapi` is the complete API visible to plugin authors. It owns:

- the single-device `Driver` lifecycle;
- immutable startup state and the host environment;
- typed control events;
- configuration, subscription, descriptor, device-status, and logging values;
- validation and normalization helpers;
- subscription matching and frame trimming.

It does not expose IPC framing, protocol messages, host policy, persistence, or
log storage. The existing `LogStore` is removed because storage is a host
responsibility.

### `pkg/protocol`

`pkg/protocol` owns versioned host/plugin messages and the abstract connection
used by each side. It maps public API values onto protocol v1 without deciding
which fields an avatar requires or how a device acquires them.

### `pkg/pluginruntime`

`pkg/pluginruntime` runs one driver in one plugin process. It performs the
handshake, exposes a runtime-managed `pluginapi.Host`, serializes control
events, coalesces frames, applies final subscription trimming before IPC,
publishes heartbeat/status/log messages, and coordinates shutdown.

## Public Plugin API

### Driver and host

The v1 lifecycle is:

```go
type Driver interface {
    Descriptor() Descriptor
    Run(context.Context, Host) error
}

type Host interface {
    Startup() Startup
    Events() <-chan ControlEvent
    PublishFrame(trackingmodel.TrackingFrame) bool
    PublishStatus(DeviceStatus)
    Log(LogLevel, string)
}
```

`Descriptor` is immutable for the lifetime of a process. Validation requires a
non-empty stable ID, display name, semantic plugin version, API version 1, and
at least one known tracking capability. Unknown capability bits are rejected.

`Startup` is one consistent initial snapshot:

```go
type Startup struct {
    Active       bool
    Config       Config
    Subscription Subscription
}
```

The runtime deep-copies `Config.Data` at ownership boundaries so plugin and
control workers never share mutable JSON backing storage.

### Typed control events

The provisional multi-purpose `Command` is replaced with a closed event
interface:

```go
type ControlEvent interface {
    controlEvent()
}

type ActiveChanged struct { Active bool }
type ConfigChanged struct { Config Config }
type SubscriptionChanged struct { Subscription Subscription }
type ShutdownRequested struct{}
```

Only this package can implement `ControlEvent`. Events are delivered in wire
receive order. Configuration revision and subscription generation are strictly
monotonic. Duplicate updates are ignored idempotently; decreasing values are
protocol errors.

### Subscription model

Subscriptions use two levels:

```go
type Subscription struct {
    Generation   uint64
    Capabilities trackingmodel.Capability
    Eye          trackingmodel.EyeValid
    Expressions  trackingmodel.ExpressionMask
}
```

The capability mask enables or disables whole data groups. A zero field mask
inside an enabled group means the entire group is requested, allowing devices
that cannot reduce acquisition granularity to use the same contract. A nonzero
field mask requests only those fields. Field masks belonging to disabled
capabilities are cleared during normalization.

Generation zero means no active subscription and is allowed only for the
inactive/empty startup state. Every effective host subscription update has a
positive, strictly increasing generation.

The API provides validation, normalization, membership tests, and a frame trim
operation. Trimming clears unsubscribed capabilities and validity bits and
zeroes corresponding eye and expression values, preventing unwanted fields
from crossing IPC even when a driver publishes a full device frame.

### Publication semantics

`PublishFrame` is safe for device callback workers and does not block on IPC.
Its Boolean result means only that the frame entered the runtime's latest-frame
slot; it does not promise delivery to the host. It returns false when the
runtime is stopped, inactive, or has no effective subscription.

After driver shutdown, frame publication returns false and status/log calls are
safe no-ops. `DeviceStatus`, `LogLevel`, configuration revisions, and descriptor
fields have explicit validation rather than relying on string conventions.

## Protocol v1

`pkg/protocol` defines protocol version 1 and a typed message envelope. The
connection contract is:

```go
type Conn interface {
    Send(context.Context, Message) error
    Receive(context.Context) (Message, error)
    Close() error
}
```

Each `MessageType` maps to exactly one payload type. Constructors and decoding
reject unknown types, mismatched payloads, payloads larger than 1 MiB, invalid
versions, malformed public API values, and invalid revisions or generations.

The protocol includes hello, initialize, ready, heartbeat, tracking frame,
status, log, config change, subscription change, active change, shutdown,
shutdown acknowledgement, and error messages. Initialization carries the
atomic `Startup` state. A tracking-frame envelope carries both the frame and the
subscription generation under which it was accepted. This lets the host reject
in-flight data from an older avatar plan.

The abstract `Conn` and message contract are implemented here. Concrete named
pipe, socket, or other transport remains the responsibility of `internal/ipc`.

## Runtime Data Flow

### Startup

1. Validate the driver descriptor.
2. Connect and send hello with protocol range fixed to version 1.
3. Validate initialize and negotiate version 1.
4. Construct the host environment from the received startup snapshot.
5. Start the driver, protocol reader, frame writer, and heartbeat workers under
   one cancellation scope.

Any critical worker failure cancels the shared scope and begins bounded
shutdown.

### Control flow

The protocol reader is the sole producer of the bounded control-event queue.
It applies revision/generation checks before publishing events. Control events
are never silently discarded: if the driver persistently fails to consume the
bounded queue, the runtime terminates with an explicit backpressure error.

A shutdown request is emitted once, then the driver context is canceled. A
driver return of `context.Canceled` during coordinated shutdown is success; any
other driver error is sent as a protocol error where possible and results in a
nonzero process exit.

### Frame flow

`PublishFrame` stores only the newest accepted frame and records the current
subscription generation with it. The writer trims the frame using that same
subscription snapshot immediately before sending it. When a newer subscription
arrives, pending frames from older generations are discarded. Inactive or empty
subscriptions suppress frame publication entirely.

The writer clears unsubscribed capability flags, validity bits, and associated
values. This runtime enforcement is mandatory even if a driver already uses the
subscription to reduce device acquisition.

### Status, logs, and heartbeat

Status publication retains the latest status and may overwrite an unsent older
status. Logs use a bounded queue. When full, lower-severity messages are
dropped; dropped-message counts are attached to the next successfully sent log
record. An error log that cannot be queued is counted rather than blocking a
device callback.

Heartbeat is generated by the runtime, not the driver. A connection write or
read failure terminates the runtime so the host supervisor can apply restart
policy.

## Concurrency and Ownership

The driver, reader, writer, and heartbeat workers share one context. The event
channel has one producer and is closed exactly once. Mutable startup config,
subscription values, and frames are copied when ownership crosses goroutine
boundaries. The latest-frame slot is bounded to one frame, and all other queues
have fixed capacities defined as runtime constants and exercised at capacity in
tests.

No runtime method holds a mutex while invoking driver code or performing a
connection operation. `Close` is idempotent, and shutdown waits are bounded.

## Errors and Compatibility

The project has no published API or third-party plugins, so the provisional API
is replaced rather than deprecated. API version 1 and protocol version 1 begin
the supported compatibility line.

Validation errors identify the field and rejected value. Protocol violations,
control queue exhaustion, and connection errors terminate the runtime. Invalid
frames are rejected before entering the latest-frame slot. Expected
cancellation during shutdown is not reported as a driver failure.

## Testing

`pkg/trackingmodel` tests cover mask set/test/intersection, out-of-range IDs,
empty masks, and clearing unused tail bits.

`pkg/pluginapi` tests cover every exported validation rule, subscription
normalization and membership, full-group and field-level trimming, value
zeroing, configuration copying, post-shutdown publication, and compilation of a
minimal example driver.

`pkg/protocol` tests cover every message round trip, message/payload mismatch,
size limits, version negotiation, public-value validation, and revision and
generation rules.

`pkg/pluginruntime` tests use an in-memory `protocol.Conn` to cover handshake,
initialization, ordered events, idempotent duplicates, regressive updates,
latest-frame overwrite, inactive and empty-subscription suppression, generation
switches, precise trimming, heartbeat, status and log backpressure, coordinated
shutdown, driver failure, and connection failure.

An integration test runs a sample driver against an in-memory host, applies a
field-level subscription, and proves that the wire frame contains only selected
fields with the current generation.

The acceptance commands are:

```powershell
go test ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime
go test -race ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime
go vet ./pkg/trackingmodel ./pkg/pluginapi ./pkg/protocol ./pkg/pluginruntime
```

## Delivery Order

1. Add shared tracking masks and frame trimming primitives.
2. Replace the provisional plugin API with the validated v1 contract.
3. Define protocol v1 messages and the connection abstraction.
4. Implement the plugin runtime lifecycle and control path.
5. Implement selective latest-frame delivery, status/log/heartbeat behavior,
   and end-to-end in-memory integration tests.
6. Update package specifications so completion requires behavioral and race
   tests rather than symbol presence.
