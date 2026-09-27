---
id: build-release
kind: build-release
path: build
milestone: M7
depends_on: [root, frontend]
checks:
  - id: wails-config
    description: Wails build configuration exists
    type: file
    path: wails.json
    weight: 1
    required: true
  - id: platform-assets
    description: Platform build assets exist
    type: file
    path: build/windows
    weight: 1
    required: true
  - id: application-builds
    description: Root application builds
    type: command
    command: go-test-build
    args: [.]
    weight: 3
    required: true
  - id: desktop-build-script
    description: Windows desktop build script stages the release layout
    type: file
    path: build/build-desktop.ps1
    weight: 1
    required: true
  - id: steamlink-plugin-build-script
    description: Steam Link development plugin build script exists
    type: file
    path: build/build-steamlink.ps1
    weight: 1
    required: true
  - id: steamlink-plugin-layout
    description: Steam Link build script stages the manifest and executable
    type: symbol
    path: build/build-steamlink.ps1
    pattern: 'steamlink-plugin\\.exe'
    weight: 1
    required: true
---
# Subsystem: build-release

## Purpose
Produce tested installable desktop artifacts.
## Responsibilities
Own Wails configuration, platform assets, version metadata, packaging, and release verification.
## Non-responsibilities
Feature implementation and runtime update policy belong to product packages.
## Current implementation
Wails configuration and Windows/macOS asset directories exist. The Windows build
scripts stage the desktop executable and the builtin Steam Link manifest/executable
pair in the same `plugins` directory derived by `internal/userconfig/paths.go`.
## Public/internal interfaces
Documented developer build commands and release artifacts.
## Owned data
Build metadata, icons, manifests, and packaging configuration.
## Dependencies
Depends on the root application and frontend production build.
## Concurrency and lifecycle
Builds run in isolated CI/developer processes.
## Error handling
Missing assets, failed compilation, and packaging errors stop release creation.
## Performance constraints
Build reproducibility and artifact correctness take priority.
## Security boundaries
Signing credentials are external secrets and never committed or printed.
## Required tests
Application build, frontend build, platform asset validation, and release smoke
test. The release smoke test builds the Steam Link plugin into a root containing
spaces and scans its real manifest/executable pair through `DirectoryCatalog`.
## Known gaps
No complete CI/release workflow is specified in the current repository.
## Completion definition
A clean checkout produces verified versioned release artifacts, including
`plugins/steamlink/{manifest.json,steamlink-plugin.exe}` beside the desktop
executable. NSIS compilation is additionally verified when NSIS tooling is
available.
