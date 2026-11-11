# Pico Connect Adapter Design

Date: 2026-11-11. Status: approved for implementation.

Scope update, 2026-11-11: The user deferred protocol 0. The current plugin accepts only transfer protocol 2, defaults to 2, and checks the PICO Connect process and `lab.faceTrackingTransferProtocol` at startup and periodically. It warns and retries until both are ready. The original dual-protocol design below remains as historical context; protocol 0 acceptance is outside the current scope.

## Intent and evidence

Add a bundled Go plugin that reads eye and face tracking from PICO Connect and publishes the existing `pluginapi` Eye and Expression fields. Support both the documented legacy transfer setting (`faceTrackingTransferProtocol=2`) and the newer default setting (`0`), without silently labeling one as the other. The first hardware target is PICO 4 Pro / Enterprise. Keep the plugin process, Host subscription, generation, and output contracts used by Steam Link.

The [VRCFT setup guide](https://docs.vrcft.io/docs/hardware/vr/pico/pico4pe) says its Pico4SAFTExtTrackingModule requires protocol 2 and calls this a temporary workaround for a newer protocol. The [UnifiedPicoModule protocol reference](https://github.com/WolalaQAQ/UnifiedPicoModule/blob/main/docs/protocol.md) describes a 29765/UDP legacy packet with a 16-byte header and 892-byte body and reports hardware verification with PICO Connect 10.6.6 in protocol 2 mode. PICO's [developer face-tracking documentation](https://developer.picoxr.com/document/unity/face-tracking/) describes expression capabilities but does not specify the PICO Connect PC transfer format. No authoritative protocol 0 wire contract was found in the initial search. This machine has PICO Connect 10.6.6 installed but no `setting.json` at the current user's roaming AppData path.

These are separate evidence levels: official device capability, third-party observed protocol 2 packet, and currently unverified protocol 0 transport. Synthetic fixtures can prove parser behavior but not hardware compatibility.

## Goals and boundaries

* Deliver one discoverable `pico-connect` plugin, a development build command, release packaging, setup instructions, and package specifications. This goal remains open until both protocol paths work; protocol 2 is an intermediate milestone.
* Decode and map protocol 2 after explicit configuration, with bounded UDP input, exact validation, freshness, status, subscription trimming, and lifecycle behavior.
* Investigate protocol 0 from a real PICO Connect session or other primary evidence, document its transport and field semantics, then implement and test that receiver. If it is an OpenXR or shared-memory interface rather than UDP, use a separate receiver behind the same plugin API.
* Preserve distinct left/right eye and expression validity when the source provides it. Never invent pupil measurements or claim independent gaze when the source only carries expression-derived gaze.
* Provide actionable diagnostics that identify selected protocol, bind or connection failures, malformed input, and absence of data without logging biometric values.

Do not edit PICO Connect settings automatically, add a generic packet mapper, add a second Host tracking contract, or claim protocol 0 works based solely on protocol 2 fixtures. Hardware acceptance is required before describing either path as verified on a device.

## Approaches considered

| Approach | Benefit | Cost |
| --- | --- | --- |
| Protocol 2 only | Known packet layout; fastest usable plugin | Requires a manual legacy setting; misses the requested newer protocol |
| Protocol 0 only | Targets the default setting | Unknown interface makes completion dependent on further evidence; loses a documented fallback |
| One plugin with separate protocol receivers (selected) | Covers both requested modes while sharing Host mapping and lifecycle | More implementation and validation work; protocol 0 discovery is an explicit dependency |

The user selected support for both. The plugin must not guess the active format by interpreting arbitrary bytes as another protocol. Use an explicit `transferProtocol` configuration with values `0` and `2`; default to `0` when that receiver is ready, and report a clear unsupported status until then. A safe automatic mode can be added only with unambiguous protocol detection and source isolation.

## Architecture and behavior

Use `cmd/pico-connect-plugin`, `internal/picoconnect`, and `plugins/pico-connect/manifest.json`, parallel to Steam Link's package boundaries. Keep transport parsing, source fields, canonical mapping, freshness state, and `pluginapi.Driver` separate. The driver owns mutable state and receives control events. It publishes only subscribed fields, clears cached input on activation, subscription generation, or receiver switch, and emits a final invalid snapshot on expiry before reporting disconnected. A receiver cannot carry data across a generation boundary.

Protocol 2 binds IPv4 loopback UDP port 29765 by default. Parse little-endian values explicitly; require at least 908 bytes, a 16-byte header with `trackingType=2`, and a complete 892-byte body. Ignore trailing bytes, matching the 912-byte observed packets. Do not reject header fields or source timestamp order without evidence for their semantics. Validate bounds and finite shape values. Do not treat the 10 validity floats as authoritative without verification: the referenced PICO Connect 10.6.6 recording reports they stay unset while shapes change. Start with the first 52 PICO expression slots. Eye direction and openness derive from `EyeLook*` and `EyeBlink*` shapes; source slots without a trustworthy mapping remain invalid. Establish exact formulas and coordinate signs in mapping tests and an evidence table before publishing them. The transport timestamp and body face timestamp are distinct; use local monotonic time for Host freshness unless source clock semantics are verified.

Protocol 0 discovery first records the PICO Connect version, setting, endpoint or API, bounded packet or call trace, and field ordering with consent for any biometric capture. Inspect installed program resources and public APIs read-only first. If the installed copy cannot provide a running session, the receiver implementation waits for a real trace or public specification; its status must report unsupported or unverified rather than impersonating a successful connection. Protocol 2 development proceeds independently.

Follow Steam Link's bounded receive queue, cancellation, status, and subscription patterns where applicable. Do not copy its OSC decoder, because PICO Connect protocol 2 is binary UDP rather than OSC. Separate protocol parsers so a malformed datagram cannot mutate state or refresh validity. Cap datagram size and queue depth. Never log raw biometric packets.

## Acceptance

* Offline: deterministic parser tests cover short, oversized, wrong-type, invalid float, and trailing-byte packets; mapping tests cover eye and expression values, missing source fields, and per-field expiry; driver tests cover control transitions, rebinding, disconnect, and subscription trimming; plugin process handshake and build checks pass. Source timestamp ordering checks require prior evidence of clock semantics.
* Protocol 2 hardware: PICO Connect connected to a PICO 4 Pro / Enterprise with `faceTrackingTransferProtocol=2`; verify eye, face, neutral/dropout, reconnect, and settings instructions against actual output. Record version and whether `faceTrackingMode=1` is needed.
* Protocol 0 hardware: same acceptance with the default transfer setting and no manual downgrade; capture transport/version evidence and verify all claimed fields. Until this passes, release notes and UI mark protocol 0 experimental or unavailable.
* Run the relevant package specification checks and desktop build with the repository-local absolute `GOCACHE`. Do not refresh generated `docs/project/status.md` from a dirty source tree.

## Open implementation evidence

The protocol 0 transport and field layout remain unknown. The design does not authorize inventing a decoder or claiming complete support in their absence. The implementation plan must start with a short protocol 0 research task and identify the evidence needed to proceed; it can then implement protocol 2 while awaiting a real session. The overall request is unfinished if protocol 0 cannot be implemented or verified. Hardware validation may require access to a PICO 4 Pro / Enterprise and permission to inspect a local, temporary trace. Keep any captured biometric data outside the repository.
