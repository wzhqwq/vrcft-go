# Steam Link Adapter Design

Date: 2026-09-13. Status: approved for implementation planning on 2026-09-19.

Updated: 2026-09-19, after rebasing onto `eb25123`. The adapter reuses the public `pkg/osc` codec, including the nesting-limit fix, instead of implementing a private OSC decoder.

## Goal and Scope

The first tracking plugin provides eye and expression data from Steam Link's local OSC output through an independent Go process connected to the existing Host. Each data group is published independently according to host subscriptions. The first hardware target is Pico 4 Pro. Because hardware setup is inconvenient, offline tests and integration come first, followed by one concentrated hardware validation phase.

Offline completion means the implementation satisfies the input contract in this document. It does not establish compatibility with actual Pico 4 Pro output. Compatibility claims require hardware acceptance. LinkFT's use of FB addresses does not establish that Pico supplies the same fields.

Scope includes the plugin executable, manifest, device adaptation, offline tests, development build entry point, and operating instructions. It excludes a plugin marketplace, installer, automatic updates, generic mapping editor, and OpenXR sessions. The existing IPC version, 76 expression IDs, and metadata-only Lip contract remain unchanged. Source selection, calibration, filtering, and final OSC output remain host responsibilities.

## Sources and Evidence Boundaries

1. [Valve SteamVR release notes](https://store.steampowered.com/news/posts/?appgroupname=SteamVR&appids=250820&enddate=1731543882&feed=steam_community_announcements) confirm that shared eye tracking can be used by OpenXR and enabled OSC output. A complete Valve specification for OSC addresses, coordinates, per-field validity, or timing has not been found.
2. Installed Valve SteamVR resources, `drivers/vrlink/resources/settings/default.vrsettings`, inspected on 2026-09-13, contain `useOSC=false`, `useOSCFace=false`, `shareEyeTrackingData=false`, `OSCOutPort=9000`, and `OSCInPort=9016`. `localization/localization_en_US.json` confirms the corresponding setting labels. These are defaults for the installed version, not current user settings or protocol guarantees across versions.
3. [OSC 1.0](https://opensoundcontrol.stanford.edu/spec-1_0.html) supplies the wire-format rules for byte order, string alignment, message types, and bundle boundaries. This design implements a real-time telemetry receiver profile, not a complete OSC scheduling server.
4. [Khronos XR_FB_face_tracking2](https://registry.khronos.org/OpenXR/specs/1.1/man/html/XR_FB_face_tracking2.html) provides a reference for checking FB field semantics. It does not define Steam Link's OSC encoding or establish that Pico uses the FB format.
5. [LinkFT OSCHandler](https://github.com/ykeara/LinkFT/blob/main/SteamLinkVRCFTModule/SteamLinkVRCFTModule/OSCHandler.cs) and its [module conversion code](https://github.com/ykeara/LinkFT/blob/main/SteamLinkVRCFTModule/SteamLinkVRCFTModule/SteamLinkVRCFTModule.cs) provide implementation references for `/sl/eyeTrackedGazePoint` and `/sl/xrfb/facew/*`. Their fixed pupil values, expression corrections, and coordinate conversions do not define the device protocol.

During implementation, record the exact commit and file paths for adopted upstream references to avoid drift on `main`. Preserve the applicable MIT copyright and license if code is reused. Label constructed fixtures as synthetic, never as Pico recordings.

## Alternatives and Decision

| Approach | Benefits | Costs / limitations |
| --- | --- | --- |
| Independent Go OSC plugin (selected) | Uses the existing runtime, isolates failures, and supports easy offline input injection | Requires explicit device fields and conversion rules |
| C# bridge to LinkFT | Can reuse its existing mapping implementation | Adds a runtime environment, VRCFT shared state, and model conversion |
| Direct OpenXR access | Uses public API types | Requires runtime sessions, extensions, and device support; eye extensions do not imply expression support |

The user has approved separate responsibilities for OSC reception, device field mapping, and plugin lifecycle. Source code initially lives in this repository as the first builtin plugin. Moving it to a separate repository is not a prerequisite.

## Components and Dependencies

| Path | Responsibility | Dependencies |
| --- | --- | --- |
| `cmd/steamlink-plugin` | Create the Driver, call `pluginruntime.Main`, and exit nonzero on failure | Plugin package, pluginruntime |
| `internal/steamlink` | Driver, UDP reception, device input validation, mapping, freshness, and configuration | Standard library, pkg/osc, pluginapi, trackingmodel |
| `plugins/steamlink/manifest.json` | Version and discoverable entry-point template | Existing manifest schema |
| `docs/project/packages/internal-steamlink.md` | Plugin responsibilities and executable acceptance checks | Project specification infrastructure |
| `docs/project/packages/cmd-steamlink-plugin.md` | Command build acceptance | Project specification infrastructure |

Within `internal/steamlink`, separate receiver, input validation, mapping, state, driver, and config files. Only the Driver uses Host. Input validation consumes `osc.Message` values; pure mapping and state objects driven by explicit time are independently testable. The implementation package depends on the public codec, not `internal/osc`, `internal/plugins`, application, or the host OSC service. Its package specification must include `pkg-osc` in `depends_on`.

The executable also uses `pkg/pluginruntime`, which internally depends on `internal/ipc`. This indirect runtime dependency does not belong to device adaptation and does not require the adapter package to import another internal package.

Use `pkg/osc.UnmarshalPacket` for wire decoding and its `Message`, `Value`, and `ValueKind` types for device input validation. The public codec validates padding, element lengths, exact message consumption, and nesting depth. It returns independently owned messages in wire order, flattens nested bundles, and discards timetags. Reuse this behavior without copying the parser or adding a second structural decoder. Generic codec fixes belong in `pkg/osc` with shared regression tests.

Retain a standard-library UDP socket and one receive worker. The current `osc.Server.Serve` API dispatches individual messages and silently drops malformed or unsupported datagrams; it exposes neither a datagram callback nor parse-error reporting. Calling `UnmarshalPacket` on each received datagram preserves the adapter's whole-packet state updates, receive timestamp, packet limits, and diagnostic counters. No public server API extension is required. Tests use `osc.MarshalMessage` and `osc.MarshalBundle` for normal fixtures, plus manually authored byte fixtures for independent boundary and unsupported-type checks.

## Reception and Data Flow

`Steam Link -> IPv4 loopback UDP -> osc.UnmarshalPacket -> device input validation -> raw field state -> canonical model mapping -> subscription trimming -> Host.PublishFrame -> existing runtime/IPC`

The default binding is `127.0.0.1:9015`. Port 9015 is the adapter's choice; users must set Steam Link's OSC Output Port to match. It is not Valve's default. The first version does not listen on the LAN, modify SteamVR configuration, or bind SteamVR's OSC input port.

One process represents one logical Steam Link source. UDP source ports do not identify devices because a sender restart may change its port. Multiple headsets sending to the same port are unsupported; operating instructions must state this limitation.

### Input Boundaries

- Accept individual messages and nested bundles, with a maximum UDP payload of 65507 bytes. Use the codec's existing nesting bound (`maxBundleDepth = 32` at the rebased revision) rather than a separate depth-8 parser. The constant is private; rely on the package's boundary tests rather than importing or duplicating it.
- Reject payloads exceeding the byte limit before decoding. After successful decoding, reject the entire datagram if it contains more than 512 messages or any address exceeds 256 bytes, before changing field state. These two application limits are post-decode checks, not codec allocation limits; decoding work remains bounded by the payload size and codec nesting bound.
- Delegate wire-length, element-boundary, alignment, zero-padding, type-tag, and exact-consumption checks to `osc.UnmarshalPacket`. Any returned error discards the entire datagram without changing the cache. Do not retain partial results or reparse individual bundle elements.
- Recognized addresses require exact matching. Tracking weights contain one `f`; gaze points contain three `f` arguments. Do not coerce integers, strings, or booleans into tracking values.
- Adopt the public codec's `i/f/s/T/F` subset. Blob (`b`) and other unsupported types return `osc.ErrUnsupportedType` and cause the entire datagram to be discarded, even if the unsupported message has an unknown address. Do not implement blob skipping or partial-bundle recovery in the adapter. Unknown addresses using supported types can be ignored after successful decoding. Any future need for additional wire types must be addressed in the shared package with its own tests.
- NaN/Inf, incorrect argument counts, or tracking weights outside `[0,1]` invalidate the message without refreshing field timestamps. They do not fail the connection. Unknown addresses increment counters without creating dynamic fields.
- When an address appears multiple times in a packet, the last valid value wins. After decoding and packet-limit checks succeed, validate device arguments and commit the valid recognized messages as one state update. A device-level invalid message does not prevent valid siblings from being applied; a codec error rejects the whole packet.
- Process packets in local receive order and messages in the order returned by the codec. Bundle timetags are discarded by `UnmarshalPacket` and are unavailable to the adapter; do not infer scheduling or device timestamps. This is an explicit real-time telemetry compatibility policy; future timetags are not queued either. Revise the shared codec contract and adapter policy separately if subsequent evidence establishes that the device relies on timetags.

Raw fields come from a fixed table. One receive worker passes complete, independently owned datagrams and local receive times to the driver through a queue capped at 64 packets. Queued bytes must not alias the worker's reusable read buffer. A full queue drops new packets and increments a counter. The driver calls `osc.UnmarshalPacket`, validates device input, and exclusively owns field state so mapping never reads concurrently mutated data. Each main-loop iteration checks cancellation and queued controls first; continuous UDP traffic must not starve control processing.

### Freshness and Publication

Initial policy: publish at most 100 Hz, keep fields valid for 250 ms, and report disconnection after 2 seconds without recognized valid tracking input. These are plugin policies, not device protocol requirements.

Each field records the monotonic time of its last valid reception. An unchanged value received again is a new observation. Unknown data and malformed messages cannot sustain ready status. A field is fresh when age < 250 ms and expires at the boundary. Every required dependency of a derived field must be fresh.

Produce at most one frame every 10 ms. New observations or validity changes trigger a snapshot of fresh fields within the current subscription. Do not repeatedly publish the cache when neither input nor validity changes. Clear expired validity bits in the next frame. When all fields expire, publish one frame with empty validity masks, then stop numeric publication. That frame retains the intersection of subscribed and supported capabilities so the host can process dropout.

A cached snapshot is not a simultaneous sample: field reception times within it can differ by up to 250 ms. The public model has no per-field timestamps, so this window also bounds how long newer fields can carry older fields into subsequent frames. Host dropout may continue smoothing afterward. Tests must prove that continuing expression updates cannot indefinitely extend the lifetime of eye data or other fields.

`Sequence` increases throughout the process lifetime and does not reset on port rebinding or subscription changes. `TimestampNS` uses monotonic elapsed process time and must be positive and increasing. `SourceClockNS=0` indicates that no trustworthy device clock is available. Do not represent repeated publication times as device sampling times or claim to detect all UDP reordering.

## Mapping Profile: `steamlink-osc-v1`

This fixed internal profile is a compatibility contract based on observed implementations. It is not named as a Pico protocol and does not automatically identify headsets. Unlisted source fields produce no output; unavailable target fields retain cleared validity bits.

### Eye Tracking

Initially interpret `/sl/eyeTrackedGazePoint` as a head-relative 3D gaze point with right = +X, up = +Y, and forward = -Z. This coordinate frame is an assumption requiring hardware confirmation. Accept only finite points with z < 0.

Convert to an approximate direction shared by both eyes: `X = clamp(atan2(x, -z)/(pi/4), -1, 1)` and `Y = clamp(atan2(y, -z)/(pi/4), -1, 1)`. Mapping 45 degrees to the normalized extreme is the adapter's initial scale, not an official definition. Host tuning can adjust gain. The output represents one shared gaze direction and does not claim to recover independent binocular convergence.

The host evaluator maps gaze components directly to output. Repository YAML requires `[-1,1]` values with right/up positive, so the radians that LinkFT writes into its own host cannot be copied directly.

For each eye, use `EyesClosedL/R` as the required observation and `UpperLidRaiserL/R` as an optional observation. The `/sl/xrfb/facew/` prefix is omitted here and in the table below. Openness is `(1-closed) * (0.75 + 0.25*wide)`. If wide is missing or expired, retain only the ordinary openness mapping based on closed. This follows the repository's EyeLid convention: ordinary open = 0.75 and widened = 1.0. A wide observation alone cannot produce valid openness. LidTightener maps independently to squint without applying LinkFT's eye-closure correction.

Eyelid dependencies are raw inputs for Eye and must still be read when Expression is not subscribed. Left and right validity bits are independent. No verified inputs exist for pupil diameter or dilation; leave them invalid instead of filling fixed values.

### Expressions

Target names below omit the Go constant prefix `Expression`. L/R correspond to Left/Right, LT/RT to UpperLeft/UpperRight, and LB/RB to LowerLeft/LowerRight. The table defines all expression outputs for the first version. Target fields without a row remain invalid.

| Raw suffix | Target / formula |
| --- | --- |
| LidTightener L/R | EyeSquint Left/Right |
| InnerBrowRaiser L/R | BrowInnerUp Left/Right |
| OuterBrowRaiser L/R | BrowOuterUp Left/Right |
| BrowLowerer L/R | Same value to BrowLowerer and BrowPinch Left/Right |
| NoseWrinkler L/R | NoseSneer Left/Right |
| CheekRaiser L/R | CheekSquint Left/Right |
| CheekPuff L/R, CheekSuck L/R | CheekPuffSuck Left/Right = Puff - Suck |
| JawDrop | JawOpen |
| LipsToward | MouthClosed |
| JawSidewaysRight, JawSidewaysLeft | JawX = Right - Left |
| JawThrust | JawZ (only forward motion is observable) |
| MouthRight, MouthLeft | MouthUpperX and MouthLowerX = Right - Left |
| ChinRaiserT/B | MouthRaiserUpper/Lower |
| Dimpler L/R | MouthDimple Left/Right |
| LipCornerPuller L/R | Same value to MouthCornerPull and MouthCornerSlant Left/Right |
| LipCornerDepressor L/R | MouthFrown Left/Right |
| LowerLipDepressor L/R | MouthLowerDown Left/Right |
| UpperLipRaiser L/R | MouthUpperUp Left/Right |
| LipTightener L/R | MouthTightener Left/Right |
| LipPressor L/R | MouthPress Left/Right |
| LipStretcher L/R | MouthStretch Left/Right |
| LipPucker L/R | LipPuckerUpper and LipPuckerLower on the same side |
| LipFunneler LT/RT/LB/RB | Corresponding four LipFunnel quadrants |
| LipSuck LT/RT/LB/RB | Corresponding four LipSuck quadrants |
| TongueOut | TongueOut |

Difference mappings require both inputs to be fresh. An input that has never arrived must not be treated as zero; positive zero is a valid value, not absence. Every direct mapping depends on one field. Outputs sharing an input are still trimmed by subscription. Assigning the same value to mouth-corner slant and pull is an initial compatibility approximation, not an independent measurement.

Do not copy LinkFT's power-function correction for upper-lip suck or synthesize additional tongue, nasal, or throat fields. The transport and threshold semantics of `/sl/xrfb/facec/*` are insufficiently verified, so the first version does not use them to determine validity. Hardware recordings must check their presence and behavior; the implementation must not claim to use device confidence.

## Lifecycle, Subscriptions, and Configuration

Descriptor: `id=steamlink`, display name `Steam Link`, initial version `0.1.0`, API 1, and capabilities `Eye | Expression`, excluding Lip. Manifest and Descriptor must match.

`Run` reads Startup once and subsequently updates current state only through Events. Handle `ActiveChanged`, `SubscriptionChanged`, and `ConfigChanged`, and respond to ShutdownRequested and context cancellation. The host decides source selection and preemption. The plugin does not adopt LinkFT's initialization-order ownership of eye/face tracking.

Compile subscriptions into required raw fields before mapping. A zero field mask selects the whole group under the existing API. The runtime retains responsibility for final trimming and generation tagging; the driver does not operate IPC directly.

While inactive or without an effective subscription, continue reception to observe device health, but retain no numeric cache and publish no frames. On reactivation, subscription-generation changes, or port rebinding, clear the cache and change the receive epoch. Discard queued packets from old epochs or received before the transition boundary. New subscriptions require new observations. The public Host API has no publication method with a driver-specified generation, so this design makes no stronger atomic-barrier claim between runtime control reception and driver control consumption. Use the existing trimming and generation protection, and test that interleaved controls cannot publish unsubscribed fields.

The first configuration JSON exposes only `listenPort`, an integer from 1 through 65535, defaulting to 9015. Empty configuration and `{}` use the default. Reject unknown fields, duplicate keys, incorrect types, and extra JSON values. Freshness and publication frequency initially remain named internal constants instead of unverified user tuning options.

On invalid initial configuration, publish DeviceError and wait for correction while keeping the process able to receive controls. An invalid live update preserves the previous working configuration; the status message identifies the new revision as unapplied. Restore status after the next valid correction. For a port change, bind the new socket before closing the old one and switching epochs. Binding failure preserves the old port and reports an error without logging the full configuration. If the initial port is occupied, retain the control loop and retry every 2 seconds instead of repeatedly restarting the process.

Status meanings: Initializing indicates startup; Disconnected means listening without any recognized valid tracking messages yet, or none for 2 seconds; Ready means recognized valid data is arriving; Error indicates configuration or binding failure. Ready does not imply that every subscribed field is present and does not establish Pico compatibility. Ordinary data arrival must not override an unresolved port/configuration error. Inactive does not mean disconnected.

Discard malformed input. Rate-limit logs per category to one summary every 5 seconds, with counters rather than complete packets. Retain at most 32 distinct unknown address names for diagnostics, counting further entries. Do not log per-frame gaze or expression values. Health detection observes recognized valid tracking input; unrecognized formats should indicate that the plugin is waiting for recognizable data.

On shutdown, close the socket to unblock reads, stop tickers, and wait for the single receive worker to exit. Do not use a fixed startup sleep. `PublishFrame=false` means only that the runtime did not accept the frame; do not retry it in a loop. Return unrecoverable socket failures as Driver errors so the host can apply its bounded restart policy.

## Build and Discovery

Use the existing catalog's builtin/dev root layout: one plugin directory contains `manifest.json` and `steamlink-plugin.exe`. The source manifest uses that relative executable name as its entrypoint and the existing v1 schema/protocol versions.

Provide an explicit development build command or PowerShell script that places the manifest and executable under `steamlink/` within a specified plugin root. Default development output belongs in an ignored build directory, with instructions for selecting it as a development root. Do not change persistent user settings or enable the plugin without the user's choice. Desktop release packaging places the same directory in the product's builtin root. The first version adds no installation mechanism.

The release smoke test must build into a plugin-root path containing spaces, then
scan the real staged manifest and executable with `DirectoryCatalog`. Windows NSIS
packaging must copy that staged directory after the Wails application files without
removing user settings. This is process/layout evidence only; it does not validate
Steam Link or Pico hardware output.

All Go commands reuse the absolute `GOCACHE=F:\dev\vrcft-go\.go-gocache` without clearing it. Add package specifications and command documentation. Generated status is not a source of requirements, and status.md must not be refreshed from unreviewed dirty source.

## Verification and Completion Criteria

### A: Offline Implementation Completion

- Shared codec: run the existing `pkg/osc` packet, nesting-boundary, ownership, and fuzz regression tests. Wire-format implementation and generic parser regressions remain in that package; do not create a private decoder to retest the same implementation.
- Adapter input: use public encoder fixtures and independent raw bytes to test whole-datagram rejection on codec errors (including an unsupported blob beside otherwise valid tracking messages), no partial cache mutation, supported-type unknown addresses, message/address limits, argument counts, NaN/Inf, out-of-range weights, duplicate addresses, and datagram buffer ownership. Fuzz the bounded adapter input path to check for panics, out-of-bounds access, and invalid state updates in addition to the shared decoder target.
- Mapping: independent expectations cover every table row, positive and negative differences, missing required inputs, zero values, left/right naming, shared gaze direction, 45-degree normalization, closed/ordinary-open/widened eyes, and optional wide expiration.
- State: virtual-clock tests cover the 249/250 ms boundary, 2-second disconnection, repeated values, partial stream loss, full expiration, one-time invalidation notifications, subscription dependencies, absent subscriptions, pause/resume, and configured port changes.
- Integration: real loopback UDP and a simulated Host cover aggregation across bundles, queue overflow, recovery, port closure, and worker cleanup. Runtime/host subprocess integration covers handshake, manifest consistency, subscription trimming, configuration, and shutdown.
- Run new plugin package and command tests; public `pkg/osc` tests; relevant plugin API/runtime/manager and tracking/processing regressions; corresponding race and vet checks; then the full Go suite and desktop build. Run the existing `pkg/osc/FuzzUnmarshalPacket` target and the adapter input fuzz target for at least 30 seconds each during development acceptance, retaining fixed seeds for regular regression coverage.
- Establish that tests verify policy without treating synthetic inputs as hardware evidence. Verify that the host remains operational and applies existing dropout when plugin data is absent.

Baseline recorded on 2026-09-13: worktree `feat/steamlink-adapter`, based on `b000b11`. The Wails build passed. The full Go suite had one known failure, `internal/projectstatus/TestParseSpecRejectsInvalidMetadata/duplicate_check`; all other packages passed. Track this failure separately from the device design without weakening tests to bypass it. Final verification must still report whether it remains.

Rebase verification on 2026-09-19: the design branch now includes `eb25123` and the public OSC nesting fix `6f2f149`. `go test ./pkg/osc` passed. The earlier full-suite/build results are historical evidence, not verification of the rebased tree; this documentation update does not establish whether the earlier project-status failure remains.

### B: Final Pico 4 Pro Hardware Acceptance

Record SteamVR, Steam Link, and Pico system versions and enabled permissions. The user enables eye/face sharing and OSC, sets the output port to 9015, and runs only one sending device.

One concentrated session covers looking forward/up/down/left/right, closing each eye, widening the eyes, opening the mouth, moving the jaw sideways, puffing/sucking cheeks, disconnect/reconnect, and eye-only/expression-only operation. Record actual addresses, tags, update rates, packet grouping, confidence fields, and behavior when tracking is unavailable. Verify that head movement is not incorrectly converted into eye movement.

With user agreement, capture minimal samples, remove unrelated information, and retain them as replayable regression fixtures. If Pico uses a different protocol, adjust the fixed mapping profile and its tests. Evaluate a second profile only when multiple formats are demonstrated; do not first build a generic mapping platform.

Claim tested Pico 4 Pro compatibility only after phase B. List missing device fields individually as unsupported instead of masking them with zeros. Hardware setup is not required at every development step.

## Review Focus

The approved scope includes the independent Go plugin, the three component responsibilities, separate eye/expression subscriptions, and deferred hardware validation. The implementation plan uses the fixed OSC mapping profile, 45-degree gaze normalization, the 0.75 ordinary eye-openness region, a 250 ms field window, a 100 Hz publication limit, a 2-second disconnection threshold, and a configuration surface containing only listenPort. Hardware assumptions remain subject to final Pico acceptance.

The 2026-09-19 revision replaces the private decoder with `pkg/osc`, adopts its nesting bound and whole-datagram rejection of unsupported types, and assigns generic wire-format tests to the shared package. Device mapping, publication timing, and deferred Pico hardware acceptance remain unchanged.
