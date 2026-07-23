# Internal IPC Named Pipe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a protected, one-shot Windows named pipe transport that satisfies `protocol.Conn`, integrates with `pluginruntime.Main`, and remains buildable on non-Windows systems.

**Architecture:** A common framing and stream-connection layer owns strict bounded JSON messages and context-aware I/O. Build-tagged platform adapters use Microsoft go-winio on Windows and explicit unsupported stubs elsewhere. The plugin runtime obtains its logical pipe name and session token from its child-process environment.

**Tech Stack:** Go 1.25.6, `pkg/protocol`, `github.com/Microsoft/go-winio v0.6.2`, `golang.org/x/sys/windows`, Windows named pipes, Go build tags, standard `testing`.

## Global Constraints

- Windows uses `github.com/Microsoft/go-winio v0.6.2`.
- Non-Windows builds expose the same API and return `ErrUnsupportedPlatform`.
- A listener accepts exactly one plugin connection.
- Logical pipe names contain 1–128 ASCII letters, digits, `-`, or `_`.
- Windows endpoints are `\\.\pipe\vrcft-<logical-name>`.
- The endpoint allows only LocalSystem and the current Windows user SID.
- `protocol.Hello.Token` remains the sole session-authentication handshake.
- `protocol.MaxPayloadSize` remains 1 MiB.
- Complete framed JSON is bounded by `protocol.MaxMessageSize`, defined as `MaxPayloadSize + 256`.
- IPC never buffers or schedules tracking frames and never reconnects.
- Tests are written and observed failing before production implementation.
- Dependency downloads, Go cache access, commits, and other permission-sensitive commands are performed by the primary agent.

---

## File Structure

- Modify `pkg/protocol/message.go`: export the complete-message size limit.
- Modify `pkg/protocol/message_test.go`: pin payload and envelope limit behavior.
- Create `internal/ipc/api.go`: common configs, listener interface, errors, and name validation.
- Create `internal/ipc/api_test.go`: platform-independent configuration tests.
- Replace `internal/ipc/framing.go`: bounded frame encoding and decoding.
- Create `internal/ipc/framing_test.go`: hostile and partial stream tests.
- Create `internal/ipc/conn.go`: context-aware, concurrency-safe `protocol.Conn`.
- Create `internal/ipc/conn_test.go`: cancellation, closure, ownership, and concurrency tests.
- Replace `internal/ipc/server.go`: common one-shot listener state machine over an injected `net.Listener`.
- Create `internal/ipc/server_test.go`: listener lifecycle tests independent of Windows.
- Replace `internal/ipc/client.go`: common connector wrapping and test seam.
- Create `internal/ipc/client_test.go`: connector validation and wrapping tests.
- Create `internal/ipc/platform_windows.go`: go-winio listener, dialer, endpoint, SID, and SDDL.
- Create `internal/ipc/platform_windows_test.go`: real named-pipe integration and Windows security tests.
- Create `internal/ipc/platform_other.go`: unsupported-platform implementations.
- Create `internal/ipc/platform_other_test.go`: non-Windows sentinel tests.
- Modify `pkg/pluginruntime/main.go`: default environment-based IPC connector.
- Modify `pkg/pluginruntime/runtime_test.go`: default factory environment and real transport tests.
- Modify `go.mod` and `go.sum`: pin go-winio.
- Modify `docs/project/packages/internal-ipc.md`: document completed transport evidence.
- Modify `docs/project/packages/pkg-pluginruntime.md`: remove the connection-factory gap.
- Regenerate `docs/project/status.md`.

---

### Task 1: Unify the Protocol Message Limit

**Files:**
- Modify: `pkg/protocol/message.go`
- Modify: `pkg/protocol/message_test.go`

**Interfaces:**
- Produces: `const MaxMessageSize = MaxPayloadSize + 256`
- Consumed by: `internal/ipc/framing.go`

- [ ] **Step 1: Write the failing constant-boundary test**

Add a test that unmarshals a byte slice of exactly `MaxMessageSize+1` and
expects a total-size error. Also assert:

```go
if MaxMessageSize != MaxPayloadSize+256 {
    t.Fatalf("MaxMessageSize = %d, want %d", MaxMessageSize, MaxPayloadSize+256)
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
go test ./pkg/protocol -run TestMessageSizeConstants -count=1
```

Expected: compilation fails because `MaxMessageSize` does not exist.

- [ ] **Step 3: Export the existing envelope allowance**

Replace the private allowance with:

```go
const MaxPayloadSize = 1024 * 1024
const MaxMessageSize = MaxPayloadSize + 256
```

Use `MaxMessageSize` in `Message.UnmarshalJSON` instead of
`MaxPayloadSize+messageEnvelopeAllowance`. Keep the payload checks unchanged.

- [ ] **Step 4: Verify GREEN and protocol regressions**

Run:

```powershell
go test ./pkg/protocol -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add pkg/protocol/message.go pkg/protocol/message_test.go
git commit -m "feat(protocol): export complete message limit"
```

---

### Task 2: Define IPC API, Errors, and Logical Names

**Files:**
- Create: `internal/ipc/api.go`
- Create: `internal/ipc/api_test.go`

**Interfaces:**
- Produces:

```go
type ServerConfig struct { PipeName string }
type ClientConfig struct { PipeName string }
type Listener interface {
    Accept(context.Context) (protocol.Conn, error)
    Close() error
}
func validatePipeName(string) error
func pipePath(string) string
```

- [ ] **Step 1: Write table-driven name validation tests**

Test valid names `a`, `plugin_01`, and a 128-character ASCII name. Test invalid
empty, whitespace, 129-character, `.`, `a.b`, slash, backslash, colon, Unicode,
and `\\server\pipe\x` values. Each invalid result must match
`ErrInvalidPipeName` and must not echo a UNC path.

Also pin:

```go
if got := pipePath("plugin_01"); got != `\\.\pipe\vrcft-plugin_01` {
    t.Fatalf("pipePath() = %q", got)
}
```

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./internal/ipc -run "TestValidatePipeName|TestPipePath" -count=1
```

Expected: compilation fails because the API and helpers do not exist.

- [ ] **Step 3: Implement the minimal common API**

Define the five sentinel errors from the design, the configs, and Listener.
Validate bytes without a regexp:

```go
func validPipeNameByte(b byte) bool {
    return b >= 'a' && b <= 'z' ||
        b >= 'A' && b <= 'Z' ||
        b >= '0' && b <= '9' ||
        b == '-' || b == '_'
}
```

Wrap `ErrInvalidPipeName` with a reason but never include the raw invalid name.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./internal/ipc -run "TestValidatePipeName|TestPipePath" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/ipc/api.go internal/ipc/api_test.go
git commit -m "feat(ipc): define named pipe API"
```

---

### Task 3: Implement Bounded Framing

**Files:**
- Replace: `internal/ipc/framing.go`
- Create: `internal/ipc/framing_test.go`

**Interfaces:**
- Consumes: `protocol.MaxMessageSize`, `protocol.Message`
- Produces:

```go
func writeFrame(io.Writer, protocol.Message) error
func readFrame(io.Reader) (protocol.Message, error)
```

- [ ] **Step 1: Write framing RED tests**

Use real `protocol.NewMessage(protocol.Heartbeat{UptimeMS: 7})` values. Cover:

- four-byte big-endian header and JSON round trip;
- a reader returning one byte per read;
- a writer accepting one byte per write;
- zero declared length → `ErrMalformedFrame`;
- declared `protocol.MaxMessageSize+1` → `ErrFrameTooLarge` before body read;
- clean EOF before any header → `io.EOF`;
- partial header and partial body → `ErrMalformedFrame`;
- invalid JSON and a JSON message with an unknown field →
  `ErrMalformedFrame`;
- semantically invalid typed JSON → `ErrMalformedFrame`;
- an outbound invalid message is rejected before any bytes are written;
- an encoded frame greater than `protocol.MaxMessageSize` →
  `ErrFrameTooLarge`.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./internal/ipc -run "TestReadFrame|TestWriteFrame" -count=1
```

Expected: compilation failure or behavioral failures because framing is empty.

- [ ] **Step 3: Implement minimal framing**

`writeFrame` must call `message.Validate()`, then `json.Marshal`, check the
complete JSON size, encode a `uint32` big-endian header, and use a `writeFull`
loop that rejects zero-progress writes.

`readFrame` must use `io.ReadFull`. Translate:

```go
if errors.Is(err, io.EOF) && headerBytesRead == 0 {
    return protocol.Message{}, io.EOF
}
```

Every partial frame or JSON/protocol decode error wraps
`ErrMalformedFrame`. Allocate the payload only after checking the declared
length.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./internal/ipc -run "TestReadFrame|TestWriteFrame" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/ipc/framing.go internal/ipc/framing_test.go
git commit -m "feat(ipc): add bounded protocol framing"
```

---

### Task 4: Implement Context-Aware Stream Connections

**Files:**
- Create: `internal/ipc/conn.go`
- Create: `internal/ipc/conn_test.go`

**Interfaces:**
- Produces:

```go
func newConn(net.Conn) protocol.Conn
type streamConn struct { /* owned stream and directional locks */ }
func (*streamConn) Send(context.Context, protocol.Message) error
func (*streamConn) Receive(context.Context) (protocol.Message, error)
func (*streamConn) Close() error
```

- [ ] **Step 1: Write connection RED tests**

Use `net.Pipe` and small instrumented `net.Conn` wrappers. Cover:

- bidirectional message exchange;
- one blocked receive canceled by context;
- one blocked send canceled by context;
- deadline exceeded remains discoverable with `errors.Is`;
- cancellation does not poison the next operation;
- `Close` unblocks send and receive;
- two `Close` calls are safe;
- one send and one receive can progress concurrently;
- two sends never interleave frames;
- two receives consume two complete frames;
- inbound framing failure closes the stream;
- outbound validation failure before the first byte leaves the stream usable;
- a failure after a partial write closes the stream;
- the caller may mutate source payload storage after `Send` returns without
  affecting bytes already delivered.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./internal/ipc -run "TestStreamConn" -count=1
```

Expected: compilation fails because `newConn` is undefined.

- [ ] **Step 3: Implement directional locking and cancelable deadlines**

The connection owns separate `readMu` and `writeMu`, plus `sync.Once` for
close. For each operation:

1. reject an already-canceled context;
2. set its deadline when the context has one;
3. install `context.AfterFunc` to set an immediate directional deadline;
4. perform `readFrame` or `writeFrame`;
5. stop the callback and wait for it if it already started;
6. clear the directional deadline;
7. prefer `ctx.Err()` only when cancellation caused an incomplete operation.

On framing errors, close the stream. `Close` returns the first underlying close
error and otherwise remains idempotent.

- [ ] **Step 4: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/ipc -run "TestStreamConn" -count=1
go test -race ./internal/ipc -run "TestStreamConn" -count=1
```

Expected: PASS with no race report.

- [ ] **Step 5: Commit**

```powershell
git add internal/ipc/conn.go internal/ipc/conn_test.go
git commit -m "feat(ipc): implement protocol stream connection"
```

---

### Task 5: Implement the One-Shot Listener State Machine

**Files:**
- Replace: `internal/ipc/server.go`
- Create: `internal/ipc/server_test.go`

**Interfaces:**
- Produces:

```go
type oneShotListener struct { /* listener, result, state, close */ }
func newOneShotListener(net.Listener) Listener
```

- [ ] **Step 1: Write listener lifecycle RED tests**

Use a controllable fake `net.Listener`. Cover:

- first successful Accept returns a wrapped `protocol.Conn`;
- a second Accept returns `ErrListenerConsumed`;
- canceling one Accept returns `context.Canceled`;
- after that cancellation, a later Accept receives the same pending underlying
  accept result rather than starting a second accept;
- Close wakes a blocked Accept with `net.ErrClosed`;
- Close is idempotent;
- success closes the underlying listener but not the accepted connection;
- concurrent Accept callers yield exactly one connection and all others receive
  `ErrListenerConsumed`.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./internal/ipc -run "TestOneShotListener" -count=1
```

Expected: compilation fails because `newOneShotListener` is undefined.

- [ ] **Step 3: Implement one persistent accept worker**

Start exactly one underlying `Accept` worker. Publish its result once. A caller
whose context is canceled stops waiting but does not abandon or duplicate the
underlying accept. Protect consumption with a mutex so only one caller can
claim a successful result. Close the underlying listener immediately after
success and wrap the connection with `newConn`.

- [ ] **Step 4: Verify GREEN and race safety**

Run:

```powershell
go test ./internal/ipc -run "TestOneShotListener" -count=1
go test -race ./internal/ipc -run "TestOneShotListener" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/ipc/server.go internal/ipc/server_test.go
git commit -m "feat(ipc): add one-shot listener lifecycle"
```

---

### Task 6: Add Platform Adapters and Real Windows Named Pipes

**Files:**
- Replace: `internal/ipc/client.go`
- Create: `internal/ipc/client_test.go`
- Create: `internal/ipc/platform_windows.go`
- Create: `internal/ipc/platform_windows_test.go`
- Create: `internal/ipc/platform_other.go`
- Create: `internal/ipc/platform_other_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `newConn`, `newOneShotListener`, `pipePath`
- Produces:

```go
func Listen(ServerConfig) (Listener, error)
func Connect(context.Context, ClientConfig) (protocol.Conn, error)
```

- [ ] **Step 1: Add the dependency through the primary agent**

Run:

```powershell
go get github.com/Microsoft/go-winio@v0.6.2
```

Expected: `go.mod` and `go.sum` pin v0.6.2. This is a dependency operation and
must not be delegated to a worker lacking permission.

- [ ] **Step 2: Write Windows integration RED tests**

Under `//go:build windows`, generate names from 16 random bytes encoded as hex.
Cover:

- Listen, concurrent Accept, Connect, and bidirectional Heartbeat/Ready;
- a second Accept matching `ErrListenerConsumed`;
- canceled Accept followed by a successful connection;
- canceled Connect to a nonexistent random endpoint;
- Listener.Close waking Accept;
- connection Close waking Receive;
- invalid names rejected before a Windows API call;
- `currentUserSID()` returns a valid SID string;
- `pipeSecurityDescriptor(sid)` equals
  `D:P(A;;GA;;;SY)(A;;GA;;;<sid>)`;
- `windowsPipeConfig` has byte mode and nonzero bounded buffers.

- [ ] **Step 3: Write non-Windows API tests**

Under `//go:build !windows`, assert valid configs passed to `Listen` and
`Connect` both match `ErrUnsupportedPlatform`, while invalid names still match
`ErrInvalidPipeName`.

- [ ] **Step 4: Verify RED**

Run:

```powershell
go test ./internal/ipc -run "TestNamedPipe|TestWindows|TestPlatform" -count=1
```

Expected: compilation fails because platform functions do not exist.

- [ ] **Step 5: Implement the Windows adapter**

In `platform_windows.go`:

- validate the logical name before constructing the path;
- obtain the current token with `windows.OpenCurrentProcessToken()`;
- call `GetTokenUser`, then `User.Sid.String()`;
- build `D:P(A;;GA;;;SY)(A;;GA;;;<sid>)`;
- call `winio.ListenPipe` with `MessageMode: false`, 64 KiB input/output
  buffers, and the descriptor;
- wrap the listener with `newOneShotListener`;
- call `winio.DialPipeContext` for Connect and wrap with `newConn`.

The selected go-winio version always creates named pipes with
`FILE_PIPE_REJECT_REMOTE_CLIENTS`.

- [ ] **Step 6: Implement the non-Windows adapter**

Use `//go:build !windows`. Validate the name first, then return
`ErrUnsupportedPlatform` from both functions. Do not import go-winio.

- [ ] **Step 7: Verify Windows GREEN and non-Windows build**

Run:

```powershell
go test ./internal/ipc -count=1
$env:GOOS='linux'
$env:GOARCH='amd64'
$crossBinary = Join-Path $env:TEMP 'vrcft-ipc-linux.test'
go test -c ./internal/ipc -o $crossBinary
Remove-Item -LiteralPath $crossBinary
Remove-Item Env:GOOS
Remove-Item Env:GOARCH
```

Expected: Windows tests PASS and the Linux test binary compiles successfully.
The build-tagged unsupported-platform tests run on non-Windows CI.

- [ ] **Step 8: Verify race behavior**

Run:

```powershell
go test -race ./internal/ipc -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add go.mod go.sum internal/ipc
git commit -m "feat(ipc): implement Windows named pipe transport"
```

---

### Task 7: Integrate the Default Plugin Runtime Connector

**Files:**
- Modify: `pkg/pluginruntime/main.go`
- Modify: `pkg/pluginruntime/runtime_test.go`

**Interfaces:**
- Consumes: `ipc.Connect(context.Context, ipc.ClientConfig)`
- Produces:

```go
const PipeNameEnv = "VRCFT_PIPE_NAME"
const SessionTokenEnv = "VRCFT_SESSION_TOKEN"
func connectFromEnvironment(context.Context) (protocol.Conn, string, error)
```

- [ ] **Step 1: Write environment factory RED tests**

Test one environment variable at a time with `t.Setenv`:

- missing/blank pipe name returns an error identifying `VRCFT_PIPE_NAME`;
- missing/blank token returns an error identifying `VRCFT_SESSION_TOKEN`;
- invalid logical pipe name matches `ipc.ErrInvalidPipeName`;
- error text never contains a nonblank token value;
- on Windows, a real `ipc.Listen` plus environment values allows
  `connectFromEnvironment` to connect and returns the exact token;
- existing `Main` handshake sends that token in `protocol.Hello`.

Do not run tests that mutate process environment in parallel.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./pkg/pluginruntime -run "TestConnectFromEnvironment|TestMainFactoryPaths" -count=1
```

Expected: compilation fails because the environment connector is undefined.

- [ ] **Step 3: Implement the default factory**

Import `internal/ipc`, `os`, and `strings`. Set:

```go
var connect = connectFromEnvironment
```

Read and trim only for blank checking; pass the original nonblank token through
unchanged so authentication is byte-exact. Never format the token into an
error. Call:

```go
conn, err := ipc.Connect(ctx, ipc.ClientConfig{PipeName: pipeName})
return conn, token, err
```

Keep `ErrConnectionUnavailable` only if it remains part of the documented
public error set; otherwise remove it and adjust its existing membership test.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./pkg/pluginruntime -run "TestConnectFromEnvironment|TestMainFactoryPaths" -count=1
go test ./pkg/pluginruntime -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add pkg/pluginruntime/main.go pkg/pluginruntime/runtime_test.go
git commit -m "feat(pluginruntime): connect through named pipe IPC"
```

---

### Task 8: Complete Package Specifications and Project Evidence

**Files:**
- Modify: `docs/project/packages/internal-ipc.md`
- Modify: `docs/project/packages/pkg-pluginruntime.md`
- Modify: `docs/project/status.md`

**Interfaces:**
- Documents: implemented APIs, security boundary, framing limit, lifecycle,
  platform behavior, runtime environment contract, and verification evidence.

- [ ] **Step 1: Update the IPC package spec**

Replace placeholder/current-gap text with:

- Windows named pipe and non-Windows behavior;
- one-shot Listener and Client APIs;
- logical-name grammar and endpoint mapping;
- LocalSystem/current-user ACL and local-only endpoint;
- protocol token ownership;
- four-byte big-endian framing and `protocol.MaxMessageSize`;
- context, closure, concurrency, and error guarantees;
- actual test file names and verification commands.

Strengthen status checks so they require the concrete exported functions,
connection methods, Windows adapter, tests, and go-winio dependency rather than
merely non-empty files.

- [ ] **Step 2: Update the plugin runtime spec**

Remove the concrete connection-factory known gap. Document
`VRCFT_PIPE_NAME` and `VRCFT_SESSION_TOKEN`, their ownership, and the fact that
process spawning remains in `internal/plugins`.

- [ ] **Step 3: Run focused documentation/status checks**

Run:

```powershell
go test ./cmd/projectstatus ./internal/projectstatus -count=1
go run ./cmd/projectstatus -write
```

Expected: status is regenerated. Exit code 1 is acceptable while unrelated
project milestones remain incomplete; writing the file must succeed.

- [ ] **Step 4: Commit**

```powershell
git add docs/project/packages/internal-ipc.md docs/project/packages/pkg-pluginruntime.md docs/project/status.md
git commit -m "docs: record named pipe IPC completion"
```

---

### Task 9: Full Verification and Cross-Package Review

**Files:**
- Modify only if a failing test exposes a defect; every defect first receives a
  focused regression test.

**Interfaces:**
- Verifies the complete design contract.

- [ ] **Step 1: Run formatting and whitespace checks**

Run:

```powershell
gofmt -w (Get-ChildItem internal/ipc -Filter *.go | ForEach-Object FullName) pkg/protocol/message.go pkg/protocol/message_test.go pkg/pluginruntime/main.go pkg/pluginruntime/runtime_test.go
git diff --check
```

Expected: no formatting diff after gofmt and no whitespace errors.

- [ ] **Step 2: Run the complete test suite**

Run:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run the scoped race suite**

Run:

```powershell
go test -race ./internal/ipc ./pkg/protocol ./pkg/pluginruntime
```

Expected: PASS with no race report.

- [ ] **Step 4: Run vet**

Run:

```powershell
go vet ./internal/ipc ./pkg/protocol ./pkg/pluginruntime
```

Expected: PASS.

- [ ] **Step 5: Verify the non-Windows build**

Run in one PowerShell process and restore the environment:

```powershell
$env:GOOS='linux'
$env:GOARCH='amd64'
$crossBinary = Join-Path $env:TEMP 'vrcft-ipc-linux.test'
go test -c ./internal/ipc -o $crossBinary
Remove-Item -LiteralPath $crossBinary
Remove-Item Env:GOOS
Remove-Item Env:GOARCH
```

Expected: the Linux test binary compiles successfully. Non-Windows CI runs the
unsupported-platform tests.

- [ ] **Step 6: Check generated status freshness**

Run:

```powershell
go run ./cmd/projectstatus -check
```

Expected: no `project status is stale` message. Exit code 1 remains acceptable
only because unrelated project milestones are incomplete.

- [ ] **Step 7: Perform a full diff review**

Review the complete range from the design commit through the implementation.
Check:

- no token or payload appears in errors or logs;
- every allocation is bounded before use;
- cancellation does not leak an unowned goroutine;
- listener and connection closure are deterministic;
- all Windows-only imports have correct build tags;
- there is no import cycle;
- no tracking-frame queue was added;
- specs describe actual rather than aspirational behavior.

- [ ] **Step 8: Fix review findings through TDD and rerun Steps 1–6**

For every behavioral issue, add a failing regression test, observe RED, apply
the minimal fix, and observe GREEN before rerunning the complete verification.

- [ ] **Step 9: Confirm repository state**

Run:

```powershell
git status --short
git log -5 --oneline
```

Expected: clean worktree and intentional task commits only.
