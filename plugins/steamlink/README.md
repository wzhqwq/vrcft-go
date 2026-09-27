# Steam Link tracking plugin

This builtin plugin receives Steam Link OSC tracking on IPv4 loopback and publishes
eye and expression data to vrcft-go. It is an offline implementation of the
`steamlink-osc-v1` profile; it does not establish Pico 4 Pro compatibility.

## Build and discover

From the repository root, build a development plugin root with:

```powershell
.\build\build-steamlink.ps1
```

The default output is `build/bin/plugins/steamlink/` and contains both
`manifest.json` and `steamlink-plugin.exe`. To use another root, pass its parent
directory (the script creates its `steamlink` child):

```powershell
.\build\build-steamlink.ps1 -PluginRoot 'C:\dev\VRCFT plugins'
```

Add that parent directory to the application's plugin development roots in
Settings, then enable **Steam Link** in the Plugins page. The desktop build stages
the same layout under the executable's builtin `plugins` directory:

```powershell
.\build\build-desktop.ps1
.\build\build-desktop.ps1 -NSIS
```

The scripts do not write user settings or enable the plugin. A plugin ID must be
unique across the builtin root and every development root. Remove a duplicate
Steam Link development root before using the builtin copy, or catalog discovery
will reject the duplicate `steamlink` ID.

## Steam Link setup

In Steam Link / SteamVR, enable the eye and face tracking sharing options and OSC
output. Configure the OSC output port to match the plugin's `listenPort`; its
default is `9015`. The plugin listens only on `127.0.0.1`, so one local sender is
supported. If another program already owns the port, select an unused port in the
plugin configuration and change Steam Link to the same port. The plugin does not
change SteamVR or Steam Link settings for you.

## Input evidence and limits

The field mapping is based on Valve's documented sharing/OSC settings and LinkFT
commit `b043175c8eee91a94909b0a137ebb39330a90018`: its
`SteamLinkVRCFTModule/SteamLinkVRCFTModule/OSCHandler.cs` and
`SteamLinkVRCFTModule/SteamLinkVRCFTModule/SteamLinkVRCFTModule.cs` files were
inspected as implementation references. This plugin independently implements its
own mapping; it does not reuse LinkFT source code. Those sources describe
provenance, not a guarantee that a Pico headset sends the same data. Test packets
in this repository are synthetic fixtures, never Pico recordings. The plugin
intentionally leaves pupil diameter, dilation, Lip data, and unverified
face-confidence fields unsupported. Actual Pico 4 Pro hardware compatibility has
not yet been validated.

## Offline acceptance evidence

Offline acceptance on 2026-09-27 passed the targeted package and integration
suite, its race-enabled counterpart, and `go vet` for the adapter and command.
Both OSC fuzz targets ran for 30 seconds. The desktop build staged
`build/bin/vrcft-go2.exe` and
`build/bin/plugins/steamlink/{manifest.json,steamlink-plugin.exe}`; the staged
manifest declares `steamlink`, `Steam Link`, version `0.1.0`, and Eye plus
Expression capabilities.

The repository-wide `go test ./...` still reports the independent,
pre-existing `internal/projectstatus` `TestParseSpecRejectsInvalidMetadata`
`duplicate_check` failure. It is not an adapter test failure.

Pico 4 Pro hardware compatibility has not yet been validated.
