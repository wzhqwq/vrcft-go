# OSC Send Path Optimization Design

## Goal

Reduce CPU time, garbage collection pressure, and copying in the high-frequency
VRCFT OSC parameter send path without changing the OSC wire format or weakening
the general-purpose receive path.

The optimized path must avoid constructing `Value`, `Message`, `[]Message`, and
per-message byte slices for normal parameter output. General OSC messages and
decoded string values remain supported by the existing packet API.

## Scope

This change covers `internal/osc` parameter catalog compilation and parameter
sending. It includes:

- precompiling catalog bindings into a flat output plan;
- replacing address-keyed change detection with indexed scalar caches;
- introducing a compact scalar representation for outbound values;
- encoding scalar messages directly into reusable message and bundle buffers;
- removing per-send address sorting;
- removing the obsolete `SendPolicy` declaration;
- adding correctness and allocation benchmarks for the optimized path.

It does not redesign general OSC decoding, change controller behavior, alter
parameter definitions, or add support for new OSC types.

## Architecture

### General OSC model

`Value`, `Message`, `Bundle`, `MarshalMessage`, `MarshalBundle`, and
`UnmarshalPacket` remain available for receiving, tests, diagnostics, and other
low-frequency OSC use cases. Strings therefore stay out of the compact outbound
representation without sacrificing protocol support.

### Compiled output plan

`BuildCatalog` continues to build the existing diagnostic binding information
and additionally emits an immutable, flat `[]OutputBinding`. This keeps the
controller-facing catalog snapshot compatible while moving all repeatable work
out of the per-frame send loop.

Each output records:

- the source `parameters.ParameterID`;
- its final OSC address;
- a precompiled evaluation operation;
- the target scalar wire type;
- binary weight and quantization maximum where applicable;
- the range data needed for clamping or magnitude conversion;
- a contiguous cache index.

Evaluation operation and target wire type are separate. This preserves current
behavior when a boolean result is exposed through an OSC `i` or `f` endpoint,
instead of assuming every binary endpoint uses `T`/`F` tags.

Outputs are sorted once by address with deterministic tie breakers. Duplicate
addresses are collapsed during compilation. Conflicting duplicate definitions
for the same address are treated as a catalog build error rather than choosing
one implicitly.

The catalog hash is derived from the effective compiled outputs, so changes in
send behavior invalidate change-detection state.

### Compact scalar values

The outbound path uses an internal `ScalarValue` containing a 32-bit payload and
a small kind tag. Its supported forms are float32, int32, true, and false. Boolean
values are encoded in the kind and carry no payload.

`ScalarValue` is expected to occupy eight bytes on amd64. A size test documents
that expectation without changing the representation of decoded values.

Equality is kind-aware. Float values use `SenderConfig.FloatEpsilon`; integer and
boolean values use exact equality.

### Indexed change detection

`ParameterSender.last` becomes `[]cachedScalar`, indexed by the cache index in
each compiled output. A validity flag distinguishes an unsent value from the
zero value.

Catalog installation and sending use the same mutex boundary:

1. `SetCatalog` acquires the sender lock.
2. It publishes the catalog and replaces the cache with the exact required size.
3. `Send` acquires the lock before loading and executing the catalog plan.

This prevents a sender from evaluating one plan against another plan's cache.
`ResetChangeDetection` clears validity while retaining allocated capacity when
the plan size is unchanged.

## Encoding and Data Flow

For each call to `Send`:

1. Load the current catalog under the sender mutex.
2. Iterate its compiled outputs in address order.
3. Read the required source value and evaluate the precompiled operation.
4. Reject unavailable and non-finite float inputs, then apply existing clamping,
   conversion, sign, and binary quantization semantics.
5. Compare the scalar with its indexed cache entry.
6. Encode changed values directly into a reusable buffer.
7. Send completed datagrams through `UDPTransport`.

No `Value`, `Message`, address-keyed map entry, per-frame sort, or per-message
packet allocation is required in this flow.

### Bundle mode

`BundleBuilder` owns a reusable byte slice capped initially for
`SenderConfig.MaxDatagram`. `Reset` writes the `#bundle` marker and immediate
timetag. `AppendScalar` reserves the element length, appends the encoded message,
and backfills the length.

When the next element would exceed `MaxDatagram`, the current non-empty bundle is
sent and the builder is reset. If one scalar message cannot fit in a bundle, it
is encoded and sent as a standalone OSC message, preserving current behavior.
The builder must not retain an oversized allocation after this fallback.

### Non-bundle mode

`MessageBuilder` reuses one byte slice and encodes each scalar message directly.
`UDPTransport.Send` consumes the slice synchronously, so the buffer may be reused
after the call returns.

Shared append helpers encode padded strings, type tags, and big-endian payloads.
The optimized builders validate addresses consistently with `MarshalMessage`.

## Error Handling

- A nil value source remains an error.
- A nil catalog remains a no-op.
- Invalid compiled addresses fail catalog construction.
- Unsupported internal operation or scalar kinds return explicit errors; they
  are not silently skipped.
- Source values that are missing, NaN, or infinite remain skipped.
- UDP send errors stop the send call and are returned unchanged through the
  existing transport error context.
- The change cache is updated only after the corresponding datagram is sent
  successfully. If a send fails, affected values remain eligible for retry.

The last rule tightens current behavior, which records values before transport
success and can suppress retries after a transient UDP error.

## Compatibility

The generated OSC address order and scalar wire representation remain stable.
Float-to-int conversion continues to use rounding, boolean-to-number conversion
continues to emit zero or one, and binary magnitude calculation retains the
current range semantics.

The existing `Catalog.Bindings` and `RawMethods` remain readable for controller
snapshots and diagnostics. The compiled output plan is immutable after catalog
construction. Existing packet marshaling and unmarshaling APIs are unchanged.

## Testing and Measurement

Unit tests will cover:

- scalar construction, equality, type tags, and amd64 size;
- byte-for-byte parity between scalar encoding and `MarshalMessage`;
- bundle framing, reset, capacity boundaries, and standalone oversized fallback;
- direct float, int, and bool output conversions;
- signed and unsigned binary encoding;
- address ordering, duplicate handling, and cache-index continuity;
- change detection with float epsilon;
- cache reset on catalog changes;
- retry behavior after transport failure where practical;
- bundle and non-bundle sender modes.

Benchmarks will compare the existing general marshaler with scalar builders and
report allocations for representative VRCFT sends. Benchmarks document the
improvement but do not enforce machine-dependent timing thresholds.

## Rollout

The work can be implemented in four independently testable layers:

1. compact scalar values and direct message/bundle builders;
2. compiled catalog outputs;
3. indexed sender execution and transactional cache updates;
4. compatibility tests, benchmarks, and removal of obsolete code.

Each layer preserves a compiling test suite before the next layer is introduced.
