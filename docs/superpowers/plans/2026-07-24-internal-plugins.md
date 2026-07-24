# Internal Plugins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `internal/plugins` as the Host-side catalog, process supervisor, authenticated protocol-session manager, bounded event source, and direct tracking-frame ingress for builtin and development plugins.

**Architecture:** One package contains small components with explicit ownership: catalog and store provide launch inputs, Manager owns public coordination and subscriptions, one supervisor goroutine owns each plugin's state, and one session owns each launched process and IPC connection. Frames bypass Manager events and go directly to `FrameSink`; low-frequency state, status, and log events use bounded fanout.

**Tech Stack:** Go 1.25.6, Windows `os/exec`, `crypto/rand`, `internal/ipc`, `pkg/pluginapi`, `pkg/pluginruntime`, `pkg/protocol`, `pkg/trackingmodel`, standard `encoding/json`, contexts, timers, and table-driven/race/integration tests.

## Global Constraints

- Current scope supports explicitly configured builtin and development roots only.
- Do not implement marketplace, download, install, update, removal, signing, or package distribution.
- Delete the empty installer placeholder; do not replace it with speculative APIs.
- One registered plugin owns one supervisor; one launch owns one process and one IPC session.
- Every launch uses a fresh logical pipe name and a cryptographically random token with at least 256 bits of entropy.
- Manifest and runtime Descriptor must match ID, Version, and Capabilities.
- `Enable/Disable` owns process lifetime; `SetActive` changes output state without stopping the process.
- Config revision and Subscription generation are monotonic, idempotent for equal content, and conflicting for unequal content at the same number.
- Frames go directly to `FrameSink.Submit(pluginID, generation, frame)` and never enter Manager events.
- Event subscriptions are independent, bounded, cancelable, and cannot block session or supervisor workers.
- No shell launches plugins; resolved entrypoints remain beneath their registered RootDir.
- Tokens, raw environment, raw Config, and frames never appear in logs, snapshots, or errors.
- Automatic restart is finite: 1s, 2s, 4s, 8s, capped at 30s, maximum 5 consecutive failures, reset after 60s stable runtime.
- All new behavior follows RED → GREEN → refactor TDD.
- Dependency/cache access, commits, and permission-sensitive operations are performed by the primary agent.

---

## File Structure

- Delete `internal/plugins/installer.go`: distribution is out of scope.
- Replace `internal/plugins/registry.go` with:
  - `manifest.go`: manifest validation and safe executable resolution.
  - `catalog.go`: builtin/dev scanning, strict JSON, duplicates, sorting.
- Replace `internal/plugins/store.go`: persistent Enabled + Config schema and atomic JSON store.
- Replace `internal/plugins/manager.go` with:
  - `api.go`: public Manager, FrameSink, Options, errors.
  - `snapshot.go`: State and RuntimeSnapshot.
  - `events.go`: bounded low-frequency subscriptions.
  - `manager.go`: catalog/store composition and supervisor dispatch.
- Replace `internal/plugins/runtime.go` with:
  - `handshake.go`: Host Hello/Initialize/Ready.
  - `control.go`: monotonic control state and single writer.
  - `session.go`: one process/connection reader and bounded shutdown.
- Replace `internal/plugins/supervisor.go` with:
  - `process.go`: Process, ProcessLauncher, ProcessSpec, real `exec.Cmd` adapter.
  - `supervisor.go`: serialized plugin state and finite restart.
- Add focused tests beside every component.
- Add `internal/plugins/integration_test.go`: Windows test-process + real named-pipe lifecycle.
- Modify `docs/project/packages/internal-plugins.md`: behavioral package contract.
- Modify `docs/project/status.md`: regenerated evidence.

---

### Task 1: Manifest Validation and Safe Catalog Discovery

**Files:**
- Delete: `internal/plugins/installer.go`
- Delete: `internal/plugins/registry.go`
- Create: `internal/plugins/manifest.go`
- Create: `internal/plugins/manifest_test.go`
- Create: `internal/plugins/catalog.go`
- Create: `internal/plugins/catalog_test.go`

**Interfaces:**
- Produces:

```go
type Source string
const (
    SourceBuiltin Source = "builtin"
    SourceDev Source = "dev"
)

type Manifest struct {
    SchemaVersion int                          `json:"schemaVersion"`
    ID            string                       `json:"id"`
    Name          string                       `json:"name"`
    Version       string                       `json:"version"`
    Description   string                       `json:"description"`
    ProtocolMin   uint16                       `json:"protocolMin"`
    ProtocolMax   uint16                       `json:"protocolMax"`
    Entrypoint    string                       `json:"entrypoint"`
    Capabilities  trackingmodel.Capability     `json:"capabilities"`
}

type InstalledPlugin struct {
    Manifest   Manifest
    RootDir    string
    Executable string
    Source     Source
}

type Catalog interface {
    Scan(context.Context) ([]InstalledPlugin, error)
}

type DirectoryCatalogConfig struct {
    BuiltinRoot string
    DevRoots    []string
    MaxManifestBytes int64
}

func NewDirectoryCatalog(DirectoryCatalogConfig) (Catalog, error)
```

- [ ] **Step 1: Write manifest validation RED tests**

Use a valid manifest:

```go
Manifest{
    SchemaVersion: 1,
    ID: "vendor.device",
    Name: "Vendor Device",
    Version: "1.2.3",
    Description: "Test device",
    ProtocolMin: protocol.Version,
    ProtocolMax: protocol.Version,
    Entrypoint: "plugin.exe",
    Capabilities: trackingmodel.CapabilityEye,
}
```

Table-test invalid schema, ID, SemVer, blank/bounded text, zero/unknown
capabilities, protocol range, empty/absolute/UNC/volume/traversal/NUL
entrypoint. Construct a `pluginapi.Descriptor` inside validation so ID, Name,
Version, and Capabilities use the same public validation semantics.

- [ ] **Step 2: Write executable containment RED tests**

In `t.TempDir`, cover regular entrypoint, missing file, directory, `..` escape,
absolute path, and a symlink/reparse-point escape when Windows permits its
creation. The returned executable must be absolute and remain under the
canonical RootDir.

- [ ] **Step 3: Write catalog RED tests**

Create builtin/dev roots with strict `manifest.json` files. Test:

- deterministic ID sorting;
- correct Source;
- unknown JSON field;
- oversized manifest;
- duplicate ID within one root and across roots;
- missing root behavior defined as empty for optional dev roots and explicit
  error for configured builtin root;
- context cancellation;
- returned InstalledPlugin values do not alias mutable scan buffers.

- [ ] **Step 4: Verify RED**

Run:

```powershell
go test ./internal/plugins -run "TestManifest|TestResolveEntrypoint|TestDirectoryCatalog" -count=1
```

Expected: compilation fails because the new validation/catalog APIs do not
exist.

- [ ] **Step 5: Implement strict manifest and catalog behavior**

Decode with:

```go
decoder := json.NewDecoder(io.LimitReader(file, maxBytes+1))
decoder.DisallowUnknownFields()
```

Reject trailing JSON. Use `filepath.Abs`, `filepath.EvalSymlinks`,
`filepath.Rel`, and reject `rel == ".."` or a path beginning with
`".."+separator`. Re-stat the canonical executable and require a regular file.
Sort by Manifest.ID before returning.

- [ ] **Step 6: Verify GREEN**

Run:

```powershell
go test ./internal/plugins -run "TestManifest|TestResolveEntrypoint|TestDirectoryCatalog" -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/plugins
git commit -m "feat(plugins): validate builtin and dev catalog"
```

---

### Task 2: Persistent Plugin Preferences

**Files:**
- Replace: `internal/plugins/store.go`
- Create: `internal/plugins/store_test.go`

**Interfaces:**
- Consumes: `pluginapi.Config`
- Produces:

```go
type PluginPreference struct {
    Enabled bool             `json:"enabled"`
    Config  pluginapi.Config `json:"config"`
}

type PluginSettings struct {
    Plugins map[string]PluginPreference `json:"plugins"`
}

type Store interface {
    Load(context.Context) (PluginSettings, error)
    Save(context.Context, PluginSettings) error
}

func NewJSONStore(path string, maxBytes int64) (Store, error)
```

- [ ] **Step 1: Write Store RED tests**

Cover:

- missing file returns empty settings;
- Enabled and Config revision/data round trip;
- unknown IDs remain present;
- strict unknown-field rejection;
- invalid Config;
- file and per-config size limits;
- Load and Save defensive ownership;
- deterministic output ordered by plugin ID;
- atomic replacement leaves the old file intact when an injected write/rename
  seam fails;
- context cancellation before I/O;
- JSON contains no Active, Subscription, pipe, token, PID, or runtime fields.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./internal/plugins -run TestJSONStore -count=1
```

Expected: compilation or behavioral failure because JSONStore does not exist.

- [ ] **Step 3: Implement the minimal atomic store**

Clone config data on every boundary. Render a deterministic wire form using a
sorted slice:

```go
type wirePreference struct {
    ID      string
    Enabled bool
    Config  pluginapi.Config
}
```

Write a temporary file in the destination directory, `Sync`, close, rename,
and preserve the destination on failure. Do not persist session state.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./internal/plugins -run TestJSONStore -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/plugins/store.go internal/plugins/store_test.go
git commit -m "feat(plugins): persist bounded plugin preferences"
```

---

### Task 3: Public API, Snapshots, and Bounded Events

**Files:**
- Replace: `internal/plugins/manager.go`
- Create: `internal/plugins/api.go`
- Create: `internal/plugins/snapshot.go`
- Create: `internal/plugins/events.go`
- Create: `internal/plugins/events_test.go`
- Create: `internal/plugins/snapshot_test.go`

**Interfaces:**
- Produces the approved `Manager`, `FrameSink`, `Event`, `EventType`,
  `RuntimeSnapshot`, `State`, and sentinel error contracts.

```go
type FrameSink interface {
    Submit(string, uint64, trackingmodel.TrackingFrame)
}

type Manager interface {
    Start(context.Context) error
    Close(context.Context) error
    List() []RuntimeSnapshot
    Get(string) (RuntimeSnapshot, bool)
    Enable(context.Context, string) error
    Disable(context.Context, string) error
    Restart(context.Context, string) error
    UpdateConfig(context.Context, string, pluginapi.Config) error
    SetActive(context.Context, string, bool) error
    UpdateSubscription(context.Context, string, pluginapi.Subscription) error
    Subscribe(context.Context) <-chan Event
}
```

- [ ] **Step 1: Write snapshot RED tests**

Pin every approved state string and every RuntimeSnapshot field. Verify copying
a snapshot/event cannot mutate Manager-owned Config, status, log, or manifest
data. Assert there is no `EventPluginFrame` and Event contains no
`*trackingmodel.TrackingFrame`.

- [ ] **Step 2: Write bounded event hub RED tests**

Create multiple subscribers with independent contexts. Verify:

- monotonically increasing global sequence;
- one slow subscriber does not block Publish;
- state/status for the same plugin coalesce to the latest snapshot;
- logs preserve order up to capacity and report dropped count;
- canceling one subscriber closes only its channel;
- hub Close closes all channels and is idempotent;
- published reference-backed values are copied.

- [ ] **Step 3: Verify RED**

Run:

```powershell
go test ./internal/plugins -run "TestRuntimeSnapshot|TestEventHub" -count=1
```

Expected: compilation fails because the approved API and hub do not exist.

- [ ] **Step 4: Implement API and event ownership**

Use one hub goroutine and per-subscriber bounded state. Publishing is a
nonblocking command into a bounded hub queue; state/status replacement uses
`pluginID + event type` as the coalescing key. Logs use a fixed capacity and a
dropped counter rather than unbounded growth.

- [ ] **Step 5: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/plugins -run "TestRuntimeSnapshot|TestEventHub" -count=1
go test -race ./internal/plugins -run TestEventHub -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/plugins/manager.go internal/plugins/api.go internal/plugins/snapshot.go internal/plugins/events.go internal/plugins/*_test.go
git commit -m "feat(plugins): define bounded manager observations"
```

---

### Task 4: Process Launch and Credential Environment

**Files:**
- Delete: `internal/plugins/supervisor.go`
- Create: `internal/plugins/process.go`
- Create: `internal/plugins/process_test.go`
- Create: `internal/plugins/process_windows.go`
- Create: `internal/plugins/process_other.go`

**Interfaces:**
- Produces:

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

func NewProcessLauncher() ProcessLauncher
func launchEnvironment(base []string, pipeName, token string) ([]string, error)
func newSessionCredentials() (pipeName, token string, err error)
```

- [ ] **Step 1: Write credential and environment RED tests**

Verify 1,000 generated pipe names satisfy `internal/ipc` logical-name rules,
tokens decode to exactly 32 random bytes, and no generated credential repeats.
Test Windows environment keys case-insensitively:

```text
VRCFT_PIPE_NAME
vrcft_pipe_name
VRCFT_SESSION_TOKEN
```

The result must contain exactly one effective key for each credential, retain
unrelated values, avoid logging secrets, and reject blank credentials and NUL
entries.

- [ ] **Step 2: Write real launcher RED tests**

Start the current test executable in a helper-process mode. Verify absolute
Executable, fixed WorkingDir, exact args/environment, PID, one Wait result,
context cancellation before Start, Kill, and sanitized launch errors.

- [ ] **Step 3: Verify RED**

Run:

```powershell
go test ./internal/plugins -run "TestSessionCredentials|TestLaunchEnvironment|TestProcessLauncher" -count=1
```

Expected: compilation fails because the launcher and credential helpers do not
exist.

- [ ] **Step 4: Implement direct `exec.Cmd` ownership**

The concrete process wraps only `*exec.Cmd` and guards Wait with `sync.Once`.
Never expose both `*exec.Cmd` and Process to a supervisor. Check `ctx.Err()`
immediately before launch, then use `exec.Command` without a shell so
cancellation after successful Start cannot bypass protocol Shutdown and the
supervisor's kill escalation. Build-tag Windows containment setup separately;
the initial implementation must at minimum prevent shell parsing and support
reliable Kill.

- [ ] **Step 5: Verify GREEN**

Run:

```powershell
go test ./internal/plugins -run "TestSessionCredentials|TestLaunchEnvironment|TestProcessLauncher" -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/plugins
git commit -m "feat(plugins): launch isolated plugin processes"
```

---

### Task 5: Host Handshake

**Files:**
- Create: `internal/plugins/handshake.go`
- Create: `internal/plugins/handshake_test.go`

**Interfaces:**
- Consumes: `protocol.Conn`, Manifest, `pluginapi.Startup`
- Produces:

```go
type handshakeResult struct {
    Descriptor pluginapi.Descriptor
}

func hostHandshake(
    context.Context,
    protocol.Conn,
    Manifest,
    string,
    pluginapi.Startup,
) (handshakeResult, error)
```

- [ ] **Step 1: Write handshake RED tests**

Use a real in-memory `protocol.Conn` pair rather than asserting calls on a mock.
Cover:

- Hello → Initialize → Ready success;
- Initialize contains one defensively owned Startup snapshot;
- wrong, blank, and length-mismatched token;
- token does not appear in errors;
- protocol range;
- invalid Descriptor;
- manifest ID, Version, and Capabilities mismatch;
- runtime Name/Description adoption;
- Ready before Hello, duplicate Hello, Status/Frame before Ready, and other
  wrong-phase messages;
- context timeout waiting for Hello and Ready;
- connection failure joined without raw Config or token.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./internal/plugins -run TestHostHandshake -count=1
```

Expected: compilation fails because `hostHandshake` does not exist.

- [ ] **Step 3: Implement exact-phase handshake**

Use `subtle.ConstantTimeCompare` over fixed-size token bytes. Receive Hello,
validate manifest/runtime identity, send `protocol.Initialize`, receive Ready,
and reject all other message types. Clone Startup before building the message.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./internal/plugins -run TestHostHandshake -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/plugins/handshake.go internal/plugins/handshake_test.go
git commit -m "feat(plugins): authenticate plugin host handshake"
```

---

### Task 6: Monotonic Controls and Single Session Writer

**Files:**
- Create: `internal/plugins/control.go`
- Create: `internal/plugins/control_test.go`

**Interfaces:**
- Produces:

```go
type controlState struct {
    Active       bool
    Config       pluginapi.Config
    Subscription pluginapi.Subscription
}

type controlKind uint8
type controlRequest struct {
    kind controlKind
    state controlState
    reply chan error
}

func (s *controlState) applyConfig(pluginapi.Config) (changed bool, err error)
func (s *controlState) applySubscription(pluginapi.Subscription) (changed bool, err error)
func (s *controlState) applyActive(bool) (changed bool)
```

- [ ] **Step 1: Write control-state RED tests**

For Config and Subscription test lower, same/equal, same/conflicting, and
higher revision/generation. Test invalid values, ownership of Config.Data,
Active idempotence, and construction of exact typed protocol control messages.

- [ ] **Step 2: Write writer RED tests**

Use an instrumented real Conn and concurrent requests. Verify:

- exactly one concurrent Send;
- accepted controls preserve order;
- queue capacity returns `ErrControlBackpressure`;
- context cancellation before acceptance;
- no changed message for idempotent updates;
- Shutdown follows prior accepted controls and blocks later controls;
- send failure completes the owning request and terminates the writer.

- [ ] **Step 3: Verify RED**

Run:

```powershell
go test ./internal/plugins -run "TestControlState|TestSessionWriter" -count=1
```

Expected: compilation fails because control-state and writer types are absent.

- [ ] **Step 4: Implement monotonic state and the writer**

Use `bytes.Equal` for Config.Data and exact value comparison for Subscription.
The writer is the only runtime caller of `Conn.Send`. Queue capacity is supplied
through Options and is never unbounded.

- [ ] **Step 5: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/plugins -run "TestControlState|TestSessionWriter" -count=1
go test -race ./internal/plugins -run TestSessionWriter -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/plugins/control.go internal/plugins/control_test.go
git commit -m "feat(plugins): serialize monotonic runtime controls"
```

---

### Task 7: One Process/IPC Session

**Files:**
- Delete: `internal/plugins/runtime.go`
- Create: `internal/plugins/session.go`
- Create: `internal/plugins/session_test.go`

**Interfaces:**
- Consumes: Catalog entry, ProcessLauncher, `ipc.Listen`, hostHandshake,
  session writer, FrameSink, event publisher.
- Produces:

```go
type sessionConfig struct {
    Plugin InstalledPlugin
    Startup pluginapi.Startup
    HandshakeTimeout time.Duration
    HeartbeatTimeout time.Duration
    GracefulTimeout time.Duration
    KillTimeout time.Duration
    ControlCapacity int
}

type sessionResult struct {
    StartedAt time.Time
    StableFor time.Duration
    Err error
    Retryable bool
}

type pluginSession interface {
    Control(context.Context, controlRequest) error
    Stop(context.Context) error
    Done() <-chan sessionResult
}
```

- [ ] **Step 1: Write startup and frame-path RED tests**

Inject listener and launcher factories. Verify:

- listener created before process launch;
- fresh pipe/token in the exact child environment;
- RootDir working directory and validated executable;
- accept and handshake deadlines;
- listener/process/connection cleanup on every startup failure;
- successful Hello/Initialize/Ready;
- TrackingFrame calls only
  `FrameSink.Submit(pluginID, generation, frame)`;
- no Manager frame event;
- Heartbeat, Status, and Log update the correct low-frequency paths;
- a known message in the wrong runtime direction terminates the session.

- [ ] **Step 2: Write terminal and shutdown RED tests**

Cover:

- clean and unexpected process exit;
- IPC EOF and reader error;
- heartbeat timeout enters unresponsive diagnosis;
- concurrent reader/process/writer errors are retained;
- Stop sends Shutdown and waits for ShutdownAck plus process exit;
- GracefulTimeout calls Kill once;
- KillTimeout is returned;
- context cancellation does not replace the primary error;
- token, Config, and frame values are absent from every error.

- [ ] **Step 3: Write handshake/control race RED test**

Block between receiving Hello and sending Initialize. Apply Config,
Subscription, and Active changes. Assert each is observed either in Initialize
or as exactly one ordered post-Ready control, with no missed or duplicated
state.

- [ ] **Step 4: Verify RED**

Run:

```powershell
go test ./internal/plugins -run TestPluginSession -count=1
```

Expected: compilation fails because the session implementation is absent.

- [ ] **Step 5: Implement bounded session orchestration**

Use one cancellation scope and explicit workers:

- accept/handshake startup;
- reader;
- single writer;
- process waiter;
- heartbeat watchdog;
- terminal collector.

Tag callbacks with a session instance ID. Close the listener after accept and
the connection exactly once. Do not retain frames.

- [ ] **Step 6: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/plugins -run TestPluginSession -count=1
go test -race ./internal/plugins -run TestPluginSession -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/plugins
git commit -m "feat(plugins): run bounded plugin sessions"
```

---

### Task 8: Serialized Supervisor and Finite Restart

**Files:**
- Create: `internal/plugins/supervisor.go`
- Create: `internal/plugins/supervisor_test.go`

**Interfaces:**
- Consumes: `pluginSession` factory, event publisher, persisted preference.
- Produces:

```go
type RestartPolicy struct {
    InitialBackoff time.Duration
    Multiplier uint
    MaxBackoff time.Duration
    MaxFailures int
    StableWindow time.Duration
}

type supervisorCommand struct {
    kind supervisorCommandKind
    config pluginapi.Config
    subscription pluginapi.Subscription
    active bool
    reply chan error
}

type pluginSupervisor interface {
    Command(context.Context, supervisorCommand) error
    Snapshot() RuntimeSnapshot
    Close(context.Context) error
}
```

- [ ] **Step 1: Write state-machine RED tests**

Use a fake clock/timer and scripted session factory. Cover every state:
disabled, stopped, starting, handshaking, running, stopping, backoff, crashed,
unresponsive, incompatible.

Verify Enable, Disable, SetActive, Config, Subscription, Restart, and Close
transitions. Confirm controls while stopped become the next Startup, controls
while stopping reject, and only the supervisor goroutine mutates state.

- [ ] **Step 2: Write restart RED tests**

Pin retry delays 1s, 2s, 4s, 8s, then 30s cap, maximum 5 failures, stable
60-second reset, lifetime RestartCount, incompatible/no-auth restart
suppression, Disable/Close backoff cancellation, manual Restart reset, and
stale prior-session results ignored.

- [ ] **Step 3: Verify RED**

Run:

```powershell
go test ./internal/plugins -run TestPluginSupervisor -count=1
```

Expected: compilation fails because the supervisor does not exist.

- [ ] **Step 4: Implement the serialized loop**

One goroutine selects over command, current-session result, and optional timer.
It publishes immutable snapshots after transitions. Session creation returns a
classification that distinguishes retryable and incompatible failures.

- [ ] **Step 5: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/plugins -run TestPluginSupervisor -count=1
go test -race ./internal/plugins -run TestPluginSupervisor -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/plugins/supervisor.go internal/plugins/supervisor_test.go
git commit -m "feat(plugins): supervise finite plugin restarts"
```

---

### Task 9: Manager Composition and Multi-Plugin Isolation

**Files:**
- Create or replace: `internal/plugins/manager.go`
- Create: `internal/plugins/manager_test.go`

**Interfaces:**
- Consumes: Catalog, Store, ProcessLauncher, FrameSink, supervisor/session
  factories, event hub.
- Produces:

```go
type Options struct {
    HandshakeTimeout time.Duration
    HeartbeatTimeout time.Duration
    GracefulTimeout time.Duration
    KillTimeout time.Duration
    ControlCapacity int
    EventCapacity int
    Restart RestartPolicy
}

func DefaultOptions() Options
func NewManager(
    Catalog,
    Store,
    ProcessLauncher,
    FrameSink,
    Options,
) (Manager, error)
```

- [ ] **Step 1: Write Manager startup RED tests**

Cover catalog scan, store load, deterministic snapshots, missing preference,
enabled supervisor start, disabled state, unavailable-plugin preference
preservation, startup rollback, double Start, and invalid Options.

- [ ] **Step 2: Write Manager command RED tests**

Cover unknown ID; not-started/closed; Enable/Disable persistence ordering;
Restart; Config/Subscription/Active routing; context cancellation;
backpressure; List/Get defensive copies; and save failure rollback semantics.

- [ ] **Step 3: Write multi-plugin and close RED tests**

Run two scripted supervisors. One crashes/restarts while the other continues
frames and controls. Verify Manager Close:

- rejects new controls first;
- prevents restart without persisting Enabled=false;
- stops supervisors concurrently;
- joins errors;
- closes event subscribers;
- is idempotent;
- obeys caller context.

- [ ] **Step 4: Verify RED**

Run:

```powershell
go test ./internal/plugins -run TestManager -count=1
```

Expected: compilation or behavioral failure because the concrete Manager is
absent.

- [ ] **Step 5: Implement composition**

Manager owns a map fixed from the current catalog scan, a sorted ID slice, the
event hub, and supervisor handles. It never reads or writes supervisor internals
directly. Persist preference changes atomically before reporting success; on
runtime command failure, retain the persisted user intent and expose the
runtime failure in snapshot state.

- [ ] **Step 6: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/plugins -run TestManager -count=1
go test -race ./internal/plugins -run TestManager -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/plugins/manager.go internal/plugins/manager_test.go
git commit -m "feat(plugins): compose multi-plugin manager"
```

---

### Task 10: Real Windows Process and Named Pipe Integration

**Files:**
- Create: `internal/plugins/integration_test.go`
- Modify only if integration exposes a tested defect in earlier components.

**Interfaces:**
- Exercises `NewManager` through real ProcessLauncher, `internal/ipc`, and a
  helper plugin using `pkg/pluginruntime.Main`.

- [ ] **Step 1: Add the helper plugin process**

Use the current test binary:

```go
func TestPluginHelperProcess(t *testing.T) {
    if os.Getenv("VRCFT_TEST_PLUGIN_HELPER") != "1" {
        return
    }
    os.Exit(runHelperPlugin())
}
```

The helper Driver publishes Heartbeat through runtime behavior, status, log,
one subscribed frame, accepts Config/Subscription/Active, and returns after
ShutdownRequested. Select behavior through non-secret test environment flags
for normal, crash, hang, descriptor mismatch, and bad token scenarios.

- [ ] **Step 2: Write happy-path integration RED test**

Register the test executable as a dev plugin, start Manager, Enable, wait for
running, assert real FrameSink delivery with ID/generation, send all three
controls, Disable, and observe ShutdownAck/process exit.

- [ ] **Step 3: Write failure integration RED tests**

Cover descriptor mismatch → incompatible/no restart, crash → finite restart,
heartbeat hang → unresponsive/kill/restart, and Manager Close leaving no child
process or named-pipe listener.

- [ ] **Step 4: Verify RED**

Run:

```powershell
go test ./internal/plugins -run "TestWindowsPluginIntegration" -count=1 -v
```

Expected: tests fail until the real component seams and lifecycle are correct.

- [ ] **Step 5: Apply only integration-proven fixes through focused RED/GREEN**

For each failure, add or narrow a focused unit regression first, then change the
owning component. Do not add sleep-based synchronization; wait on observable
state/event conditions with bounded contexts.

- [ ] **Step 6: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/plugins -run "TestWindowsPluginIntegration" -count=1 -v
go test -race ./internal/plugins -run "TestWindowsPluginIntegration" -count=1
```

Expected: PASS and no leaked helper process.

- [ ] **Step 7: Commit**

```powershell
git add internal/plugins
git commit -m "test(plugins): verify real process IPC lifecycle"
```

---

### Task 11: Package Specification and Project Evidence

**Files:**
- Modify: `docs/project/packages/internal-plugins.md`
- Modify: `docs/project/packages/internal-tracking.md` only to document the
  generation-bearing FrameSink boundary if its text is contradictory.
- Modify: `docs/project/status.md`

**Interfaces:**
- Documents actual completed responsibilities and behavioral evidence.

- [ ] **Step 1: Replace weak status checks**

Require:

- `go-test ./internal/plugins`;
- `go-test-race ./internal/plugins`;
- manifest/catalog tests;
- handshake tests;
- direct FrameSink tests and absence of EventPluginFrame;
- supervisor restart tests;
- Windows integration test file;
- no installer placeholder.

Do not mark completion from a symbol-only `run|loop` regex.

- [ ] **Step 2: Rewrite the package narrative**

Document builtin/dev scope, persistent/session state split, Host handshake,
single writer, direct frames, bounded events, finite restart, security,
platform integration evidence, and the deferred distribution ecosystem.

- [ ] **Step 3: Run project evidence**

Run:

```powershell
go test ./cmd/projectstatus ./internal/projectstatus -count=1
go run ./cmd/projectstatus -write
```

Expected: status writes successfully. Exit 1 remains acceptable while unrelated
milestones are incomplete.

- [ ] **Step 4: Commit specs, then regenerated status with a clean fingerprint**

Commit package specs first:

```powershell
git add docs/project/packages/internal-plugins.md docs/project/packages/internal-tracking.md
git commit -m "docs: specify plugin supervision behavior"
```

Regenerate status from that clean commit, then:

```powershell
git add docs/project/status.md
git commit -m "docs: record internal plugins completion"
```

---

### Task 12: Full Verification and Review

**Files:**
- Modify only for defects first reproduced by focused failing tests.

**Interfaces:**
- Verifies the complete responsibilities design.

- [ ] **Step 1: Format and check whitespace**

Run:

```powershell
$pluginFiles = Get-ChildItem internal/plugins -Filter *.go | ForEach-Object FullName
gofmt -w $pluginFiles
git diff --check
```

Expected: no whitespace errors and no remaining gofmt changes.

- [ ] **Step 2: Run complete tests**

Run:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run scoped race tests**

Run:

```powershell
go test -race ./internal/ipc ./internal/plugins ./pkg/protocol ./pkg/pluginruntime
```

Expected: PASS with no race report.

- [ ] **Step 4: Run vet**

Run:

```powershell
go vet ./internal/ipc ./internal/plugins ./pkg/protocol ./pkg/pluginruntime
```

Expected: PASS.

- [ ] **Step 5: Verify Linux build compatibility**

Build, but do not execute, the non-Windows test binary:

```powershell
$env:GOOS='linux'
$env:GOARCH='amd64'
$crossBinary = 'F:\dev\vrcft-go\.cross-plugins-linux.test'
go test -c ./internal/plugins -o $crossBinary
Remove-Item Env:GOOS
Remove-Item Env:GOARCH
```

After verifying the resolved target is under the repository, remove the exact
cross binary.

- [ ] **Step 6: Review the full implementation diff**

Check the complete range against every design section:

- package has no distribution/install behavior;
- no frame queue or frame event exists;
- every process/listener/connection/goroutine/timer/channel has an owner and
  bound;
- single-writer and handshake/control ordering are preserved;
- stale session results cannot mutate the new session;
- retry classifications and limits are exact;
- manifest/path/environment/token boundaries are enforced;
- tokens, Config, environment, and frames cannot enter errors;
- snapshots/events/configs are defensively owned;
- docs describe actual behavior.

- [ ] **Step 7: Fix every Critical/Important review finding through TDD**

For each finding: write a focused failing test, observe RED, apply one owning
fix, observe GREEN, and rerun Steps 1–5.

- [ ] **Step 8: Verify status freshness and repository state**

Run:

```powershell
go run ./cmd/projectstatus -check
git diff --check
git status --short
git log -12 --oneline
```

Expected: no stale status message, clean worktree, and intentional commits only.
