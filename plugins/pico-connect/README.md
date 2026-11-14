# PICO Connect plugin

This bundled plugin receives PICO 4 Pro / Enterprise eye and face expressions from PICO Connect using transfer protocol **2**. Protocol 0 support is deferred.

## Protocol 2 setup

1. Enable eye and lip tracking on the headset and connect it through PICO Connect.
2. In `%APPDATA%\PICO Connect\settings.json`, set `lab.faceTrackingTransferProtocol` to `2`. PICO Connect 10.6.6 was observed using this setting and sending 912-byte packets to `127.0.0.1:29765`. Restart PICO Connect after changing it.
3. Enable the PICO Connect plugin in VRCFT-Go. Protocol 2 and port `29765` are the defaults, so plugin configuration can be empty. `listenPort` may be changed only when PICO Connect is configured to send to the same port.
4. Check Overview for plugin activity. Only one process can bind port 29765; disable any other PICO tracking module that occupies it.

The plugin binds loopback only and does not change PICO Connect settings. Values are mapped from the first 52 expression slots. Gaze is derived from EyeLook shapes, eyelid openness from EyeBlink, and no pupil value is fabricated. Mapping and device behavior still need a hands-on expression check; the live session so far confirmed packet size, type, and rate, not correctness of every tracking parameter.

At startup and every two seconds, the plugin checks that `PICO Connect.exe` is running and `lab.faceTrackingTransferProtocol` in its settings file is `2`. If either check fails, it closes the receiver, reports the reason in device status, logs a warning once for each change, and keeps polling. Tracking resumes automatically when both checks pass. Restart PICO Connect after changing its transfer setting.

When tracking is unavailable, the Overview and Plugins pages offer a help button with English and Chinese troubleshooting steps supplied by this plugin's manifest.

To build a standalone development plugin, run `powershell -NoProfile -ExecutionPolicy Bypass -File .\build\build-pico-connect.ps1`. The manifest and executable are staged under `build/bin/plugins/pico-connect/`.

## Protocol 0 status

The VRCFT [PICO guide](https://docs.vrcft.io/docs/hardware/vr/pico/pico4pe) calls setting 2 a legacy workaround. PICO has not published a PC transfer format for setting 0 in the documents found so far. See [the evidence audit](../../docs/research/pico-connect-protocol-0.md). The plugin accepts only protocol 2 while protocol 0 work is deferred.

## Sources and limits

The 16-byte header, 892-byte body, and PICO shape order were informed by [UnifiedPicoModule's protocol documentation](https://github.com/WolalaQAQ/UnifiedPicoModule/blob/main/docs/protocol.md) and [its source slot enum](https://github.com/WolalaQAQ/UnifiedPicoModule/blob/main/Core/Contracts.cs). The adapter implements its own parser and mapping; it does not copy source code. The first 5-second live packet count on PICO Connect 10.6.6 with protocol 2 was 362 datagrams, each 912 bytes with tracking type 2. This observation does not establish support for setting 0.
