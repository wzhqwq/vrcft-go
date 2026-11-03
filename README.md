# VRCFaceTracking-Go

[简体中文](README.zh-CN.md) | English

VRCFaceTracking-Go is a Windows desktop app that sends face and eye tracking data from local plugins to the active VRChat avatar over OSC. It provides a control panel for plugin input, avatar parameters, tracking adjustments, and diagnostics.

## What it does

- **Tracks the active avatar.** Reads VRChat avatar changes, loads the avatar's local OSC configuration, and sends only the parameters supported by that avatar. A fallback avatar configuration can be selected in Settings.
- **Manages local plugins.** Discover and enable plugins, and inspect their capabilities, activity, frame rate, and errors. The included Steam Link plugin receives eye and expression tracking over local OSC.
- **Tunes tracking output.** Set input calibration, response, smoothing, and signal-loss behavior globally or per channel. Mutual exclusion groups prevent conflicting channels from driving an avatar at the same time.
- **Connects to VRChat OSC.** Discover the OSC output target automatically or set an IP address and port manually.
- **Shows what is happening.** Overview displays avatar drive status, supported and currently driven parameters, plugin input, and OSC output. Diagnostics shows startup failures, module state, and recent redacted logs that can be copied when reporting a problem.

## Project origins and credits

VRCFaceTracking-Go is an independent Go redevelopment of [VRCFaceTracking](https://github.com/benaclejames/VRCFaceTracking). It is not an official release of, or endorsed by, the VRCFaceTracking project.

Some bundled plugin work draws on community projects. In particular, the Steam Link plugin's OSC field mapping was informed by Valve's documented settings and [LinkFT](https://github.com/ykeara/LinkFT). The adapter implements its own mapping and does not reuse LinkFT source code. See the [Steam Link source notes](plugins/steamlink/README.md#input-evidence-and-limits) for the specific references and limitations.

## Get started on Windows

1. Build the desktop app with `./build/build-desktop.ps1` in PowerShell. The output is `build/bin/vrcft-go2.exe`, with the included Steam Link plugin under `build/bin/plugins/steamlink/`. You can also use `./build/build-desktop.ps1 -NSIS` to create an installer when NSIS is available. See [build instructions](build/README.md) for the artifact layout.
2. Run the app. In **Settings**, check the Avatar OSC directory and, if needed, choose a fallback avatar configuration. Saved Settings changes take effect after restarting the app.
3. If using Steam Link, enable its eye and face tracking sharing and OSC output in Steam Link / SteamVR. Match its output port to the plugin's `listenPort` (default `9015`), then enable **Steam Link** on the **Plugins** page. See the [Steam Link setup guide](plugins/steamlink/README.md) for details.
4. Open VRChat and check **Overview** for the active avatar, plugin activity, and OSC target. If tracking does not reach the avatar, open **Diagnostics** to inspect the startup state and recent logs.

The Steam Link plugin implements a documented OSC profile. Production testing with Steam Link connected to a Pico 4 Pro confirmed correct eye tracking data; expression and other tracking parameters were not verified in that test. Automated tests use synthetic packets. See the [plugin's documented limits](plugins/steamlink/README.md#input-evidence-and-limits).

## Diagnostics

On Windows, the app stores redacted JSON-line logs in `%APPDATA%\vrcft-go\logs\`. The **Diagnostics** page shows recent records and retains a startup failure even when the app cannot start its tracking backend. Use **Copy diagnostics** when reporting an issue.

## Project documentation

For development, architecture, package checks, and current evidence, see the [project specifications](docs/project/README.md), [milestones](docs/project/milestones.md), and [generated project status](docs/project/status.md). The status page is generated evidence and may lag behind source changes.
