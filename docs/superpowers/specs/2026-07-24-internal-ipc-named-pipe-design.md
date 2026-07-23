# Internal IPC Named Pipe Design

## Status

Approved for implementation planning on 2026-07-24.

## Goal

Implement `internal/ipc` as an authenticated, framed, bounded Windows named
pipe transport for the typed messages in `pkg/protocol`. Preserve buildability
on non-Windows platforms with explicit unsupported-platform stubs.

## Scope

This design implements:

- a one-shot Host-side named pipe listener;
- a Plugin-side named pipe connector;
- a `pkg/protocol.Conn` implementation over a byte stream;
- bounded length-prefixed JSON framing;
- cancellation, closure, concurrency, and error semantics;
- Windows endpoint access control;
- the default `pkg/pluginruntime.Main` connection factory;
- unit, Windows integration, race, and cross-platform build tests.

This design does not implement:

- plugin discovery, process spawning, restart policy, or executable validation;
- Host-side protocol handshake policy beyond providing the connection;
- transport-level token exchange or a second authentication handshake;
- automatic reconnection;
- frame buffering or frame scheduling;
- remote named pipe access.

Those responsibilities remain with `internal/plugins`, the Host runtime,
`pkg/pluginruntime`, and `pkg/protocol` as appropriate.

## Constraints

- The concrete transport is Windows named pipes.
- Windows uses `github.com/Microsoft/go-winio` at the stable tagged version
  `v0.6.2`.
- Non-Windows builds expose the same API and return
  `ErrUnsupportedPlatform`.
- One listener serves exactly one plugin connection.
- The pipe endpoint is local-only and accessible only to the current Windows
  user and LocalSystem.
- Session authentication remains the responsibility of
  `protocol.Hello.Token`.
- A framed payload is at most 1 MiB, matching `pkg/protocol`.
- No IPC queue may accumulate tracking frames; latest-frame behavior remains
  in `pkg/pluginruntime`.

## Architecture

```text
Host
  ipc.Listen(ServerConfig)
      |
      v
  Listener.Accept(ctx) -- one successful connection only
      |
      v
  protocol.Conn

Plugin
  pluginruntime.Main(driver)
      |
      v
  default connection factory
      |
      v
  ipc.Connect(ctx, ClientConfig)
      |
      v
  protocol.Conn
```

`internal/ipc` has four internal layers:

1. `framing.go` encodes and decodes bounded length-prefixed protocol messages.
2. `conn.go` implements `protocol.Conn` over a stream connection.
3. Windows server and client files adapt `go-winio` to the package API.
4. Non-Windows files provide compile-safe unsupported-platform behavior.

The transport does not interpret initialization, configuration, subscription,
tracking, or shutdown policy. It validates the typed message through
`pkg/protocol` and transports it.

## Public Internal API

```go
package ipc

type ServerConfig struct {
    PipeName string
}

type ClientConfig struct {
    PipeName string
}

type Listener interface {
    Accept(context.Context) (protocol.Conn, error)
    Close() error
}

func Listen(ServerConfig) (Listener, error)
func Connect(context.Context, ClientConfig) (protocol.Conn, error)
```

The API is internal to the module but is treated as a stable boundary between
plugin process management and the protocol runtime.

`PipeName` is a logical name, not a path. A valid name:

- is between 1 and 128 ASCII characters;
- contains only ASCII letters, digits, `-`, and `_`;
- has no whitespace, slash, backslash, colon, dot, or UNC prefix.

The Windows implementation maps it to:

```text
\\.\pipe\vrcft-<PipeName>
```

The caller should generate a cryptographically unpredictable name for each
plugin launch. Validation prevents callers from selecting an arbitrary UNC or
remote endpoint through this API.

## Framing

Each stream frame is:

```text
4-byte unsigned big-endian payload length
JSON payload of protocol.Message
```

Rules:

- zero-length frames are invalid;
- lengths greater than 1 MiB are rejected before allocation;
- `io.ReadFull` reads both the header and payload;
- writes loop until the entire header and payload have been written;
- the payload is encoded and decoded by `pkg/protocol`, preserving strict
  message validation;
- malformed, oversized, or invalid messages make the connection unusable and
  close it, because stream synchronization is no longer trusted;
- framing never adds its own token or message type.

The length limit is checked both by `pkg/protocol` and by framing. The duplicate
boundary is intentional: protocol validation protects typed callers, while
framing protects memory allocation before decoding untrusted bytes.

## Connection Concurrency and Ownership

The connection wrapper owns the underlying stream.

- One receive and one send may execute concurrently.
- Sends are serialized with a write mutex so bytes from two messages cannot
  interleave.
- Receives are serialized with a read mutex so one frame has one consumer.
- `Close` is idempotent.
- Closing the connection releases a blocked send or receive.
- A context deadline is applied to the underlying read or write for the
  duration of the operation.
- A context without a deadline is still cancelable: cancellation forces the
  active operation to wake without leaking a permanent helper goroutine.
- After an operation, temporary context-derived deadlines are cleared so one
  call cannot poison a later call.
- The connection does not retry, reconnect, or buffer messages.

If context cancellation and transport failure race, a completed operation
wins; otherwise the returned error must match `context.Canceled` or
`context.DeadlineExceeded` when the context caused interruption.

## Listener Lifecycle

`Listen` validates the logical name, creates the Windows named pipe, and returns
a one-shot listener.

- The first successful `Accept(ctx)` consumes the listener.
- The underlying `net.Listener` closes immediately after the successful
  connection is obtained.
- A later `Accept` returns `ErrListenerConsumed`.
- Canceling `Accept` returns the context error and leaves the listener usable
  unless it has been closed.
- `Close` is idempotent and wakes a blocked `Accept`.
- Closing before a successful accept causes `Accept` to return an error matching
  `net.ErrClosed`.
- There is no accept loop and no multi-client mode.

The Host decides whether a disconnected plugin should be restarted by creating
a new name, token, listener, and process.

## Windows Security

The pipe uses `go-winio.ListenPipe` with:

- byte stream mode;
- input and output buffers large enough for normal control traffic without
  weakening the 1 MiB frame limit;
- remote clients rejected;
- an SDDL descriptor granting access only to LocalSystem and the current
  interactive user SID.

The implementation resolves the current process user SID at listener creation
and constructs the descriptor from the SID. It does not grant access to
Everyone, Authenticated Users, Administrators as a group, or network clients.

Endpoint ACLs establish local process access control. They do not replace the
per-launch session token. The Host validates `protocol.Hello.Token` before
trusting the peer.

## Plugin Runtime Integration

The default connection factory used by `pkg/pluginruntime.Main` reads:

```text
VRCFT_PIPE_NAME
VRCFT_SESSION_TOKEN
```

Both values must be present and nonblank. The factory:

1. reads and validates the environment values;
2. calls `ipc.Connect` with the logical pipe name;
3. returns the connection and token to the existing runtime handshake;
4. never logs either value.

Tests retain the existing ability to replace the connection factory. This
change closes the documented concrete connection-factory gap in
`pkg/pluginruntime`.

The future `internal/plugins` implementation will:

1. generate the logical pipe name and session token cryptographically;
2. call `ipc.Listen`;
3. place the two environment variables only in the child plugin environment;
4. start the child process;
5. accept the connection and perform the Host-side protocol handshake.

Those process-management steps are outside this implementation.

## Errors

The package exposes sentinel errors suitable for `errors.Is`:

```go
var (
    ErrUnsupportedPlatform = errors.New("ipc: named pipes are unsupported on this platform")
    ErrInvalidPipeName     = errors.New("ipc: invalid pipe name")
    ErrListenerConsumed    = errors.New("ipc: listener already accepted a connection")
    ErrFrameTooLarge       = errors.New("ipc: frame exceeds maximum size")
    ErrMalformedFrame      = errors.New("ipc: malformed frame")
)
```

Additional rules:

- invalid configuration wraps `ErrInvalidPipeName`;
- a zero length, truncated body, invalid JSON payload, or invalid typed message
  wraps `ErrMalformedFrame`;
- an excessive declared or encoded size wraps `ErrFrameTooLarge`;
- context-caused interruption preserves the context sentinel;
- local closure preserves `net.ErrClosed` where the underlying API provides it;
- a clean peer closure before another frame begins returns `io.EOF`;
- a partial header or partial payload returns `ErrMalformedFrame`, not clean EOF;
- transport-specific Windows errors may be wrapped but must not expose the
  session token or payload content.

## Testing

### Platform-independent framing and connection tests

Use deterministic in-memory stream implementations to cover:

- round-trip of every supported message category through framing;
- split reads and short writes;
- zero length and greater-than-1-MiB declarations;
- invalid JSON, unknown fields, and invalid typed messages;
- clean EOF versus partial header and partial body;
- simultaneous send and receive;
- serialization of two concurrent sends;
- serialization of two concurrent receives;
- context cancellation and deadlines;
- close unblocking send and receive;
- idempotent close;
- protocol validation before writing;
- no retained reference to caller-owned message data.

### Windows integration tests

Use a cryptographically random logical name for every test and cover:

- `Listen`, `Accept`, and `Connect`;
- bidirectional typed message exchange;
- one-shot consumption and `ErrListenerConsumed`;
- cancellation of `Accept` followed by a successful accept;
- cancellation of `Connect`;
- listener close waking accept;
- connection close waking receive;
- invalid logical names and path-injection attempts;
- pipe endpoint construction and the configured security descriptor.

Tests must close every listener and connection through `t.Cleanup`.

### Non-Windows coverage

Build-tagged tests verify that `Listen` and `Connect` return
`ErrUnsupportedPlatform`. CI or a local verification command cross-compiles the
package for at least `linux/amd64`.

### Runtime integration tests

Cover:

- missing pipe-name environment variable;
- missing session-token environment variable;
- invalid pipe name;
- connection failure;
- successful factory connection;
- exact token delivery to the existing Hello message;
- absence of credentials in returned error strings.

### Verification

The implementation is complete when:

- `go test ./...` passes on Windows;
- `go test -race ./internal/ipc ./pkg/protocol ./pkg/pluginruntime` passes;
- `go vet ./internal/ipc ./pkg/protocol ./pkg/pluginruntime` passes;
- `GOOS=linux GOARCH=amd64 go test ./internal/ipc` builds and passes;
- `go run ./cmd/projectstatus -check` reports no stale status;
- `docs/project/packages/internal-ipc.md` and
  `docs/project/packages/pkg-pluginruntime.md` describe the implemented
  behavior and evidence;
- `internal/ipc` contains no placeholder package-only source files.

## Completion Criteria

The Host can create a protected one-shot Windows named pipe endpoint and the
plugin can connect to it. Both sides exchange the versioned typed protocol
through `protocol.Conn` with strict framing, bounded allocation, cancellation,
concurrency safety, and deterministic closure. The plugin runtime uses this
transport by default without embedding transport policy into `pkg/protocol`.
