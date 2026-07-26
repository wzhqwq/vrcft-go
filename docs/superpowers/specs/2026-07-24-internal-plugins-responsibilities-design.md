# Internal Plugins Responsibilities Design

## Status

Approved for specification on 2026-07-24. This document defines package
responsibilities and required behavior. It does not authorize implementation.

## Goal

Turn `internal/plugins` into the Host-side owner of registered plugin process
lifecycles and protocol sessions, with bounded supervision and direct tracking
ingest. Defer third-party installation and distribution until the product has a
released plugin ecosystem.

## Current Problem

The current package mixes several incomplete concerns:

- `manager.go` exposes process controls, snapshots, low-frequency events, and
  high-frequency frame events;
- `runtime.go` combines process state, IPC state, restart commands, and health
  measurements without a running state machine;
- `registry.go` defines installation metadata but not strict discovery or
  validation;
- `store.go` does not distinguish persistent preferences from session state;
- `supervisor.go` contains process abstractions and timeouts but no supervisor;
- `installer.go` is an empty placeholder for a distribution system that is not
  currently needed.

The package builds, but it has no complete Host-side launch, handshake, control,
ingest, shutdown, or restart path. The existing specification also says that
the package routes frames through Manager events, which conflicts with the
latest-frame tracking architecture.

## Scope

This design includes:

- builtin and development plugin catalog discovery;
- strict manifest validation;
- persistent Enabled and Config preferences;
- one Host-side supervisor per registered plugin;
- per-launch pipe-name and session-token generation;
- child-process launch and environment construction;
- Host-side IPC accept and protocol handshake;
- runtime config, subscription, and active controls;
- heartbeat, status, log, and frame reception;
- direct frame submission to tracking ingest;
- runtime snapshots and bounded low-frequency event subscriptions;
- graceful shutdown, kill escalation, bounded automatic restart, and manual
  restart;
- deterministic tests, including real Windows child-process and named-pipe
  integration.

This design excludes:

- plugin marketplace, download, installation, update, removal, signing, or
  package distribution;
- arbitrary third-party plugin directory search;
- Vendor SDK access and device-to-unified mapping;
- plugin-side Driver lifecycle, which belongs to `pkg/pluginruntime`;
- byte framing and named-pipe security, which belong to `internal/ipc`;
- tracking sequence validation, stale-generation rejection, source selection,
  merge, calibration, filtering, evaluation, avatar binding, and OSC output;
- Wails APIs, UI state, and application-wide composition.

## Package Boundary

`internal/plugins` owns registered plugin identity and each plugin process from
launch request through final process exit. It converts a registered manifest
and current Host state into a validated `pkg/protocol` session.

```text
builtin/dev registrations
        |
        v
Catalog + manifest validation
        |
        v
Manager -- enable/disable/restart/config/active/subscription
        |
        v
Per-plugin supervisor
        |
        v
Per-launch session
  |-- random pipe name + token
  |-- ipc.Listen
  |-- launch child process
  |-- Hello validation
  |-- Initialize / Ready
  |-- runtime reader and writer
  `-- bounded shutdown
        |
        |-- TrackingFrame --------> tracking.FrameSink
        `-- state/status/log -----> bounded Manager events
```

The package must not add another tracking-frame queue. A frame accepted from
the protocol reader is submitted directly to the tracking sink with plugin
identity and subscription generation.

## Proposed File Structure

The package remains one Go package while its files are split by responsibility:

- `manifest.go`: Manifest, InstalledPlugin, Source, strict validation, and
  executable resolution.
- `catalog.go`: builtin/dev roots, scanning, duplicate detection, and stable
  ordering.
- `store.go`: persistent Enabled and Config preferences.
- `manager.go`: public API, lifecycle composition, snapshots, and event
  subscriptions.
- `supervisor.go`: one serialized lifecycle loop per plugin and restart policy.
- `session.go`: one process plus IPC connection lifetime.
- `handshake.go`: Hello validation and Initialize/Ready exchange.
- `control.go`: revision/generation checks and the single session writer.
- `events.go`: low-frequency event fanout and coalescing.
- `process.go`: Process, ProcessLauncher, ProcessSpec, and real launcher.
- `integration_test.go`: real child-process and named-pipe lifecycle tests.

Current file migration:

- delete the empty `installer.go`; installation is outside the current scope;
- replace `registry.go` with `manifest.go` and `catalog.go`;
- keep and narrow `store.go`;
- split the current `runtime.go` into session and handshake behavior;
- keep the useful process abstractions from `supervisor.go`, moving them to
  `process.go` if that produces clearer ownership;
- reduce `manager.go` to public coordination and observable state.

The split is by ownership rather than by abstract technical layer. All files
remain in `internal/plugins` to avoid premature exported subpackage contracts.

## Public Internal API

```go
type Manager interface {
    Start(context.Context) error
    Close(context.Context) error

    List() []RuntimeSnapshot
    Get(id string) (RuntimeSnapshot, bool)

    Enable(context.Context, string) error
    Disable(context.Context, string) error
    Restart(context.Context, string) error

    UpdateConfig(
        context.Context,
        string,
        pluginapi.Config,
    ) error

    SetActive(
        context.Context,
        string,
        bool,
    ) error

    UpdateSubscription(
        context.Context,
        string,
        pluginapi.Subscription,
    ) error

    Subscribe(context.Context) <-chan Event
}
```

Rules:

- Manager methods validate the plugin ID and lifecycle state before queuing a
  command to the plugin's supervisor.
- Commands for the same plugin are serialized by that supervisor.
- Different plugins can progress independently.
- `Enable` and `Disable` update persistent preference and start or stop the
  process.
- `SetActive(false)` leaves the process running and changes only tracking
  output state.
- `UpdateConfig` consumes a complete validated `pluginapi.Config`, including
  its revision.
- `UpdateSubscription` consumes a complete validated
  `pluginapi.Subscription`, including its generation.
- return values and stored inputs are defensively copied where they contain
  reference-backed data.
- `Close` is idempotent.

The Manager must return explicit errors for unknown plugin ID, not started,
already closed, invalid state, control backpressure, revision conflict, and
generation conflict.

## Frame Sink

```go
type FrameSink interface {
    Submit(
        pluginID string,
        generation uint64,
        frame trackingmodel.TrackingFrame,
    )
}
```

The protocol layer has already validated the typed frame. `internal/plugins`
adds the authenticated plugin identity and passes through the generation in
`protocol.TrackingFrame`.

`internal/plugins` does not:

- reorder frames;
- reject an old generation relative to current avatar state;
- select a source;
- merge fields;
- filter values;
- publish a frame event to UI subscribers.

Those behaviors belong to `internal/tracking`.

## Low-Frequency Events

```go
type EventType string

const (
    EventPluginDiscovered  EventType = "plugin_discovered"
    EventPluginRemoved     EventType = "plugin_removed"
    EventPluginStateChanged EventType = "plugin_state_changed"
    EventPluginStatus      EventType = "plugin_status"
    EventPluginLog         EventType = "plugin_log"
)
```

`EventPluginFrame` is removed. The ambiguous `EventPluginChanged` is replaced
by explicit state and status events.

Each `Subscribe(ctx)` call owns a bounded channel:

- cancellation closes only that subscription;
- Manager close closes all remaining subscriptions;
- a slow subscriber never blocks a supervisor, session reader, or FrameSink;
- state/status changes may coalesce into the latest snapshot when the subscriber
  is behind;
- logs may be rate-limited and carry a dropped count;
- events have a Manager-wide monotonically increasing sequence;
- event payloads are immutable copies.

## Persistent and Session State

Only user preferences persist:

```go
type PluginPreference struct {
    Enabled bool             `json:"enabled"`
    Config  pluginapi.Config `json:"config"`
}
```

Active and Subscription are not persisted. They reflect the current Host and
Avatar session and are supplied by the application after startup.

The store:

- strictly decodes its schema;
- returns defensive copies;
- writes atomically;
- rejects unknown plugin IDs only when applying settings to a catalog, not when
  decoding, so temporarily unavailable plugins retain their preferences;
- bounds total file and per-plugin config sizes;
- never stores pipe names, tokens, PIDs, errors, or runtime state.

## Manifest and Catalog

Manifest is Host-readable launch metadata:

```go
type Manifest struct {
    SchemaVersion int
    ID            string
    Name          string
    Version       string
    Description   string
    ProtocolMin   uint16
    ProtocolMax   uint16
    Entrypoint    string
    Capabilities  trackingmodel.Capability
}
```

Validation includes:

- exact supported schema version;
- the same ID grammar and SemVer behavior used by `pluginapi.Descriptor`;
- nonblank bounded Name and Description;
- nonempty known capabilities;
- protocol range containing `protocol.Version`;
- a relative Entrypoint with no volume, UNC, absolute, traversal, or NUL
  component;
- resolved executable path remains within the registered RootDir, including
  protection from symlink/reparse-point escape;
- executable exists, is a regular file, and satisfies Windows launch
  requirements.

Catalog roots are configured explicitly:

- builtin root;
- zero or more development roots.

Catalog does not scan arbitrary user or system directories. It strictly decodes
manifest JSON, rejects unknown fields, bounds file size, rejects duplicate IDs,
and returns plugins sorted by ID. Duplicate IDs are errors rather than
last-writer-wins behavior.

Builtin and development plugins pass the same identity, path, and protocol
validation. Development source is not a security bypass.

## Manifest and Runtime Descriptor

Manifest is the pre-launch declaration. `protocol.Hello.Descriptor` is the
runtime declaration.

During handshake, these fields must match exactly:

- ID;
- Version;
- Capabilities.

Runtime Descriptor Name and Description become the running display values after
successful handshake. Manifest Name and Description remain available while the
plugin is stopped or failed.

The Hello protocol range must:

- contain `protocol.Version`;
- be compatible with the manifest-declared range;
- satisfy the Host-supported range.

A critical mismatch means the installed executable does not correspond to its
manifest. The supervisor enters `incompatible`, records a sanitized reason, and
does not automatically restart.

## Runtime Snapshot

```go
type RuntimeSnapshot struct {
    ID           string
    Name         string
    Version      string
    Capabilities trackingmodel.Capability

    Enabled bool
    Active  bool
    State   State
    PID     int

    ConfigRevision         uint64
    SubscriptionGeneration uint64

    StartedAt       time.Time
    LastHeartbeatAt time.Time
    LastFrameAt     time.Time
    NextRestartAt   time.Time

    FrameRate float64

    ConsecutiveFailures int
    RestartCount       int
    LastError          string
}
```

Snapshot state:

```go
const (
    StateDisabled     State = "disabled"
    StateStopped      State = "stopped"
    StateStarting     State = "starting"
    StateHandshaking  State = "handshaking"
    StateRunning      State = "running"
    StateStopping     State = "stopping"
    StateBackoff      State = "backoff"
    StateCrashed      State = "crashed"
    StateUnresponsive State = "unresponsive"
    StateIncompatible State = "incompatible"
)
```

Meaning:

- disabled: user preference prevents launch;
- stopped: enabled but Manager has not launched it, or a normal stopped
  transition is still being reconciled;
- starting: listener/process setup is in progress;
- handshaking: process exists and Host waits for protocol readiness;
- running: Ready succeeded and runtime workers are active;
- stopping: graceful or forced shutdown is in progress;
- backoff: a retryable failure is waiting for its next attempt;
- crashed: the consecutive-failure limit was reached;
- unresponsive: heartbeat watchdog diagnosed a timeout and shutdown is being
  initiated;
- incompatible: non-retryable identity, API, manifest, or protocol failure.

Runtime-only timestamps are zero when not applicable. Snapshots never expose
tokens, pipe paths, process environment, raw configuration, or unsanitized
errors.

## Per-Launch Session

Each launch performs these steps:

1. confirm the supervisor is enabled and the manifest/executable remains valid;
2. generate a cryptographically random logical pipe name;
3. generate a cryptographically random 256-bit session token;
4. call `ipc.Listen`;
5. build the child environment with exactly one effective value for
   `VRCFT_PIPE_NAME` and `VRCFT_SESSION_TOKEN`;
6. start the process with RootDir as its working directory;
7. accept one connection within HandshakeTimeout;
8. receive Hello;
9. validate token, protocol range, Descriptor, and manifest consistency;
10. send Initialize with one atomic Startup snapshot;
11. receive Ready before any runtime message is accepted;
12. run reader, writer, process waiter, and heartbeat watchdog;
13. collect all terminal errors and perform bounded shutdown.

The token comparison uses constant-time comparison over decoded random bytes or
fixed-length canonical text. Tokens, full environment entries, raw
configuration, and frame payloads never appear in errors or logs.

Startup contains the supervisor's current:

- Active value;
- Config;
- Subscription.

If a control changes during handshake, the serialized supervisor/session
boundary must ensure the plugin observes either:

- the new value in Initialize; or
- the old Initialize followed by exactly one ordered control message.

It must never silently miss the update.

## Runtime Message Direction

Plugin to Host:

- Heartbeat;
- TrackingFrame;
- Status;
- Log;
- ShutdownAck;
- Error where allowed by the terminal protocol.

Host to Plugin:

- Initialize;
- ConfigChanged;
- SubscriptionChanged;
- ActiveChanged;
- Shutdown;
- Error where allowed by the terminal protocol.

Hello and Ready are legal only at their handshake positions. A known message in
the wrong direction or phase is a protocol violation and terminates the
session.

Message handling:

- Heartbeat updates health state;
- TrackingFrame submits directly to FrameSink;
- Status updates snapshot and emits EventPluginStatus;
- Log converts to `pluginapi.LogEntry` and enters the bounded log path;
- ShutdownAck advances graceful shutdown;
- peer Error is retained as a sanitized terminal cause.

## Control Ordering

Each session has exactly one protocol writer. Initialize, runtime controls, and
Shutdown are serialized through that owner.

Config rules:

- lower revision: reject;
- same revision and equal content: idempotent success;
- same revision and different content: conflict;
- higher revision: validate, own a copy, update current state, and send once.

Subscription rules:

- lower generation: reject;
- same generation and equal content: idempotent success;
- same generation and different content: conflict;
- higher generation: validate, own a copy, update current state, and send once.

Active rules:

- equal value: idempotent success;
- changed value: update current state and send once.

Controls accepted while stopped update the current supervisor state and become
part of the next Initialize. Controls accepted during starting/handshaking obey
the no-missed-update rule. Controls accepted while stopping are rejected.

Control queues are bounded. Backpressure is returned to the Manager caller
rather than silently dropping a control.

## Process Abstractions

```go
type Process interface {
    PID() int
    Wait() error
    Kill() error
}

type ProcessLauncher interface {
    Start(context.Context, ProcessSpec) (Process, error)
}

type ProcessSpec struct {
    Executable string
    Args       []string
    WorkingDir string
    Env        []string
}
```

The real launcher owns `exec.Cmd` internally; `runtimeInstance` must not store
both `*exec.Cmd` and `Process` as competing process owners.

The launcher:

- does not invoke a shell;
- uses an absolute validated executable;
- fixes WorkingDir to RootDir;
- constructs environment keys case-insensitively on Windows;
- replaces inherited credential keys instead of appending duplicates;
- starts the plugin in a process containment mechanism appropriate for Windows
  so Host termination cannot orphan it where practical;
- returns sanitized errors.

## Supervisor

Each registered plugin owns one supervisor goroutine. It serializes:

- Manager controls;
- session startup result;
- process exit;
- reader/writer/watchdog result;
- backoff timer;
- Manager shutdown.

No other goroutine mutates supervisor state or publishes snapshots.

Default restart policy:

```go
type RestartPolicy struct {
    InitialBackoff time.Duration // 1 second
    Multiplier     uint          // 2
    MaxBackoff     time.Duration // 30 seconds
    MaxFailures    int           // 5
    StableWindow   time.Duration // 60 seconds
}
```

Retryable failures:

- unexpected process exit;
- IPC EOF or transport failure;
- heartbeat timeout;
- temporary listener or launch failure.

Non-retryable failures:

- invalid manifest;
- entrypoint escape or invalid executable;
- token authentication failure;
- Descriptor mismatch;
- unsupported API or protocol;
- structurally invalid Host configuration.

The process launcher's wrapped `os.ErrNotExist` and `os.ErrPermission`
sentinels are non-retryable startup contract failures. Generic listener and IPC
transport failures remain retryable and must not be classified from error text.

The exponential delay function yields 1s, 2s, 4s, 8s, 16s, then 30s and
remains capped at 30s. `MaxFailures` counts the failure that ends automatic
restart: with the default `MaxFailures=5`, failures one through four schedule
the 1s, 2s, 4s, and 8s retries, while the fifth failure enters crashed without
creating another timer. Policies with a higher failure limit reach the 16s and
30s delay steps. Stable running for StableWindow clears consecutive failures
but not the lifetime RestartCount.

After a session reaches running with prior consecutive failures, its supervisor
starts an instance-tagged StableWindow timer. If that same session remains
running until expiry, the supervisor clears and publishes the consecutive
failure count immediately. Leaving running, ending or replacing the session,
Disable, Restart, and Close cancel that timer. `sessionResult.StableFor`
provides the same reset at the termination boundary when timer delivery races
with session exit.

Disable and Manager Close cancel the backoff and prohibit restart. Manual
Restart clears consecutive failures, cancels backoff, stops any current
session, and starts immediately when enabled.

## Health and Frame Metrics

Heartbeat timeout transitions running to unresponsive before initiating
shutdown. It is a diagnosis, not a stable terminal state.

LastHeartbeatAt and LastFrameAt update only from the authenticated running
session. Events from an older session instance are tagged internally and
ignored after restart.

FrameRate is a diagnostic rolling measurement. It must not require retaining
frames and must not influence delivery. No frame payload is copied into
RuntimeSnapshot or Manager events.

## Shutdown

Disable, Restart, Manager Close, and terminal failures enter the same bounded
shutdown machinery with different restart decisions.

```text
stop accepting new controls
        |
        v
send Shutdown when connection is usable
        |
        v
wait for ShutdownAck and process exit
        |
        v  GracefulTimeout
Process.Kill
        |
        v
wait for process exit up to KillTimeout
```

The session collector retains substantive reader, writer, process, handshake,
shutdown-ack, timeout, and kill errors. Expected context cancellation must not
overwrite the primary cause.

Ready-handshake control replay uses this same shutdown path; Stop does not
bypass the serialized writer by immediately closing and killing the process.
If an in-flight replay prevents Shutdown from being sent, GracefulTimeout
expires before the connection is closed and the process is killed.

Protocol v1 has no shutdown nonce, and `protocol.Conn.Send` does not expose the
first physical byte write or prove that the peer received a frame. The
enforceable Host boundary is therefore the serialized writer's start of its
Shutdown Send attempt: an Ack observed before that attempt is a protocol
violation; an Ack observed while the attempt is in progress is held pending
and becomes effective only if Send returns success; a failed Send discards the
pending Ack. This boundary prevents an earlier Ack from satisfying a later
Shutdown, but it does not claim cryptographic or remote-receipt causality.

Manager Close:

1. atomically stops accepting new public controls;
2. disables all automatic restart without changing persisted Enabled values;
3. asks every supervisor to stop concurrently;
4. waits within the caller context;
5. closes all event subscriptions;
6. returns joined substantive errors;
7. remains idempotent.

## Security

- Pipe names and tokens use `crypto/rand`.
- Session tokens contain at least 256 bits of entropy.
- Entrypoint validation occurs both at catalog load and immediately before
  launch.
- Resolved paths remain beneath the exact registered RootDir.
- No shell parses ProcessSpec.
- The environment contains one effective value per key.
- Pipe name and token are supplied only to the child process environment.
- Manifest and settings files have explicit size bounds.
- Config size also obeys `pkg/protocol` limits.
- Tokens, environment dumps, raw Config, and frames are never logged.
- LastError and EventPluginLog are length-bounded and sanitized.
- Development source receives no relaxed identity, path, or protocol rules.

## Error Model

Sentinel categories should cover:

- unknown plugin;
- manager not started or closed;
- invalid manifest;
- duplicate plugin ID;
- invalid entrypoint;
- control backpressure;
- config revision regression/conflict;
- subscription generation regression/conflict;
- handshake timeout;
- authentication failure;
- descriptor mismatch;
- protocol incompatibility/violation;
- heartbeat timeout;
- graceful shutdown timeout;
- forced kill timeout;
- restart limit reached.

Errors preserve causes with wrapping or joining, but redact token, environment,
raw configuration, and frame data. RuntimeSnapshot.LastError is a bounded
user-facing summary rather than an arbitrary full error chain.

## Testing

### Manifest and Catalog

- strict JSON and unknown fields;
- schema version, ID, SemVer, name, description, capabilities, and protocol
  range;
- empty, absolute, UNC, volume, traversal, NUL, symlink, and reparse-point
  entrypoint escape;
- missing or non-regular executable;
- duplicate ID across and within roots;
- deterministic ID ordering;
- builtin/dev source assignment and identical security validation;
- manifest size bounds.

### Store

- missing file defaults;
- strict schema and size bounds;
- atomic save;
- Enabled and Config round trip;
- preservation of preferences for temporarily unavailable plugin IDs;
- defensive Config ownership;
- no persistence of Active, Subscription, credentials, or runtime state.

### Handshake

- correct Hello/Initialize/Ready sequence;
- blank, incorrect, and length-mismatched token;
- timeout before accept, Hello, and Ready;
- protocol range;
- Descriptor validation;
- manifest ID, Version, and Capabilities mismatch;
- runtime display metadata adoption;
- wrong-direction and wrong-phase messages;
- atomic Startup and a control racing handshake.

### Session

- Heartbeat, Status, Log, and TrackingFrame handling;
- direct FrameSink delivery with plugin ID and generation;
- absence of frame events;
- one writer under concurrent controls and shutdown;
- bounded control backpressure;
- config revision and subscription generation regression, idempotence, and
  conflict;
- active idempotence;
- EOF, process exit, and simultaneous terminal errors;
- stale worker result from an older session is ignored;
- shutdown acknowledgement, graceful timeout, kill, and kill timeout;
- token and Config redaction.

### Supervisor

- every legal state transition;
- enable, disable, manual restart, and Manager close;
- retryable versus incompatible failures;
- exact backoff sequence and cap;
- stable-window reset;
- maximum consecutive failures;
- disable/close cancel pending backoff;
- manual restart clears failure count;
- one plugin failure does not affect another.

### Manager and Events

- catalog/store startup and rollback;
- List/Get stable ordering and defensive copies;
- current controls while stopped, handshaking, running, and stopping;
- multiple independent subscriptions;
- subscriber cancellation and Manager close;
- snapshot/status coalescing;
- log drop accounting;
- slow subscribers do not block supervisor, session reader, or FrameSink;
- Manager Close idempotence and joined errors.

### Windows Integration

Build a test plugin executable from repository test code. Exercise:

1. real ProcessLauncher;
2. real `internal/ipc` named pipe;
3. Hello with injected token;
4. Initialize and Ready;
5. Heartbeat, Status, Log, and TrackingFrame;
6. runtime Config/Subscription/Active controls;
7. Shutdown and ShutdownAck;
8. process exit;
9. crash and bounded restart.

The integration test does not require a Vendor SDK or third-party plugin.

## Project Evidence

The current `internal-plugins` project status checks are too weak. Completion
evidence must require:

- package tests;
- package race tests;
- manifest/catalog behavior;
- Host handshake behavior;
- frame sink behavior;
- supervisor restart behavior;
- Windows real-process integration;
- the absence of `EventPluginFrame`;
- the absence of an installer placeholder.

## Completion Criteria

`internal/plugins` is complete when:

- builtin/dev manifests produce deterministic registered plugins;
- at least two plugin supervisors can run independently;
- every launch uses a fresh protected IPC endpoint and token;
- manifest and runtime Descriptor identity are reconciled;
- Config, Subscription, and Active controls have deterministic ordering;
- frames go directly to FrameSink with plugin ID and generation;
- Manager events remain bounded and contain no frames;
- graceful shutdown, forced kill, heartbeat failure, and bounded restart behave
  deterministically;
- no goroutine, process, listener, connection, control, event, log, or frame
  resource can grow or remain orphaned without a bound;
- `go test ./internal/plugins` passes;
- `go test -race ./internal/plugins` passes;
- Windows real-process integration passes;
- package specification and project status report behavioral evidence at 100%.
