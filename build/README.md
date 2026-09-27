# Build Directory

The build directory is used to house all the build files and assets for your application. 

## Windows desktop and Steam Link plugin

Run `./build/build-desktop.ps1` from PowerShell to produce
`build/bin/vrcft-go2.exe` and the builtin plugin layout
`build/bin/plugins/steamlink/{manifest.json,steamlink-plugin.exe}`. It uses the
repository-local Go build cache at `F:\dev\vrcft-go\.go-gocache`.

Use `./build/build-desktop.ps1 -NSIS` to stage the same files before Wails invokes
the existing NSIS installer project. The installer copies the plugin alongside the
desktop executable. Use `./build/build-steamlink.ps1 -PluginRoot <root>` when only
a development-root plugin build is needed; its output is `<root>/steamlink/`.

Plugin roots are selected by the user through application settings. The scripts do
not edit settings or enable plugins. See
[plugins/steamlink/README.md](../plugins/steamlink/README.md) for discovery,
duplicate-ID, port, source-provenance, fixture, and hardware-support details.

The structure is:

* bin - Output directory
* darwin - macOS specific files
* windows - Windows specific files

## Mac

The `darwin` directory holds files specific to Mac builds.
These may be customised and used as part of the build. To return these files to the default state, simply delete them
and
build with `wails build`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds. It is used when building using `wails build`.
- `Info.dev.plist` - same as the main plist file but used when building using `wails dev`.

## Windows

The `windows` directory contains the manifest and rc files used when building with `wails build`.
These may be customised for your application. To return these files to the default state, simply delete them and
build with `wails build`.

- `icon.ico` - The icon used for the application. This is used when building using `wails build`. If you wish to
  use a different icon, simply replace this file with your own. If it is missing, a new `icon.ico` file
  will be created using the `appicon.png` file in the build directory.
- `installer/*` - The files used to create the Windows installer. These are used when building using `wails build`.
- `info.json` - Application details used for Windows builds. The data here will be used by the Windows installer,
  as well as the application itself (right click the exe -> properties -> details)
- `wails.exe.manifest` - The main application manifest file.
