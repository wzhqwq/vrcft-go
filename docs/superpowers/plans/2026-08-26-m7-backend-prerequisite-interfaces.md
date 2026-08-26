# M7 Backend Prerequisite Interfaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Windows-only persisted configuration, backend lifecycle, plugin operations, modular Wails bindings, and OSC target policy required before the M7 frontend is built.

**Architecture:** `internal/userconfig` owns the versioned user document, Windows paths, validation, atomic storage, and conversion to `application.Config`. `internal/application` exposes owned plugin operations, `internal/osc` owns auto/manual target semantics, and the root adapts those packages into separate Runtime, Plugins, and Settings snapshots and Wails events while retaining sole lifecycle ownership.

**Tech Stack:** Go 1.25.6, standard-library JSON/filesystem/context/synchronization APIs, `golang.org/x/sys/windows`, Wails v2.13.0 runtime events, existing plugin/Application/OSC packages, table-driven tests, race detector, and the repository project-status generator.

**Spec:** `docs/superpowers/specs/2026-08-26-m7-backend-prerequisite-interfaces-design.md`

## Global Constraints

- Read the approved spec before every task and do not implement frontend pages, frontend dependencies, or generated files under `frontend/wailsjs`.
- M7 v1 runtime support is Windows-only; non-Windows builds return `unsupported_platform` without inventing paths or starting Application.
- Existing invalid settings or backend startup failure enter diagnostic mode while Runtime and Settings remain queryable.
- Construction settings save for the next process start. Do not hot-reload or replace a running Application.
- Plugin enabled state and plugin-owned JSON apply immediately through the existing Manager durability ordering.
- Bind only `RuntimeAPI`, `PluginsAPI`, and `SettingsAPI`; never bind the root `App`, Application, stores, tracking payloads, credentials, PIDs, paths to plugin executables, or raw internal errors.
- Settings JSON is at most 256 KiB, plugin JSON accepted through Wails is at most 64 KiB, and user messages are valid UTF-8 bounded to 512 bytes.
- Every queue introduced here is bounded and latest-only where the spec requires snapshots.
- Set `GOCACHE` to the absolute repository-local `.go-gocache` before every `go test`, `go vet`, `go run`, build, or generate command. In PowerShell:

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim()
$env:GOCACHE = Join-Path $repoRoot '.go-gocache'
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
```

- Use TDD for every behavior: observe the focused RED failure, implement the minimum complete contract, observe GREEN, then commit.
- Run `gofmt -w` only on Go files changed by the current task. Preserve unrelated worktree content and CRLF-only files outside the task.
- Project status must continue to report M7 blocked only by the untouched frontend requirements; do not claim full M7 completion.

---

## File Map

Create these focused files:

- `internal/userconfig/settings.go`: v1 persisted and candidate DTOs, constants, clone helpers.
- `internal/userconfig/processing.go`: stable channel-name conversion and processing wire conversion.
- `internal/userconfig/paths.go`: injectable Windows resolver and derived product paths.
- `internal/userconfig/config.go`: semantic normalization and `application.Config` construction.
- `internal/userconfig/store.go`: strict bounded decoding, cached document token, first-run creation, save/repair orchestration.
- `internal/userconfig/replace_windows.go`: write-through Windows atomic replacement.
- `internal/userconfig/replace_other.go`: portable build fallback used only by tests/builds.
- `internal/userconfig/*_test.go`: boundary-owned unit and fault-injection tests.
- `api_types.go`: Wails-safe DTOs and stable Problem codes.
- `snapshot_store.go`: owned monotonic capacity-one module snapshots.
- `sanitize.go`: error classification and bounded UTF-8 conversion.
- `settings_api.go`, `plugins_api.go`, `runtime_api.go`: one binding object per module.
- `events.go`: injected Wails emitter and module event forwarding.
- root `*_test.go`: binding, lifecycle, DTO, event, and race tests.

Modify these existing files:

- `internal/osc/controller.go`, `controller_test.go`, `service.go`, `service_test.go`: target mode and manual target behavior.
- `internal/plugins/api.go`, `manager.go`, `manager_test.go`: owned persisted plugin-config reads.
- `internal/application/app.go`, `app_test.go`: running-only plugin operations and latest snapshots.
- `app.go`, `main.go`: root ownership and real Wails callback/bind registration.
- project package/subsystem specifications and the project-status catalog count.

---

### Task 1: OSC Automatic and Manual Target Ownership

**Files:**
- Modify: `internal/osc/controller.go`
- Modify: `internal/osc/controller_test.go`
- Modify: `internal/osc/service.go`
- Modify: `internal/osc/service_test.go`

**Interfaces:**
- Consumes: existing `ControllerConfig`, `OSCTarget`, UDP listener target, OSCQuery discovery, and avatar mailbox.
- Produces: `TargetMode`, `TargetModeAuto`, `TargetModeManual`, `ControllerConfig.TargetMode`, `ControllerConfig.ManualTarget`, and `OSCStatus.TargetMode`.

- [ ] **Step 1: Write target validation RED tests**

Add table tests proving zero mode normalizes to auto, auto rejects a manual target, manual requires a literal unicast address and port `1..65535`, and unspecified/multicast/IPv4 broadcast values fail:

```go
func TestNewControllerValidatesTargetMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    TargetMode
		target  OSCTarget
		wantErr bool
	}{
		{"zero is auto", "", OSCTarget{}, false},
		{"auto rejects manual fields", TargetModeAuto, OSCTarget{Host: "127.0.0.1", Port: 9000}, true},
		{"manual IPv4", TargetModeManual, OSCTarget{Host: "127.0.0.1", Port: 9000}, false},
		{"manual IPv6", TargetModeManual, OSCTarget{Host: "::1", Port: 9000}, false},
		{"manual DNS rejected", TargetModeManual, OSCTarget{Host: "localhost", Port: 9000}, true},
		{"manual unspecified", TargetModeManual, OSCTarget{Host: "0.0.0.0", Port: 9000}, true},
		{"manual multicast", TargetModeManual, OSCTarget{Host: "239.1.1.1", Port: 9000}, true},
		{"manual broadcast", TargetModeManual, OSCTarget{Host: "255.255.255.255", Port: 9000}, true},
		{"manual zero port", TargetModeManual, OSCTarget{Host: "127.0.0.1"}, true},
	}
	// Construct with the existing valid source/spec fixture and compare err != nil.
}
```

- [ ] **Step 2: Run the validation test to verify RED**

Run: `go test ./internal/osc -run TestNewControllerValidatesTargetMode -count=1`

Expected: FAIL because `TargetMode` and the new config fields do not exist.

- [ ] **Step 3: Add the target types and strict normalization**

Add to `controller.go` and `service.go`:

```go
type TargetMode string

const (
	TargetModeAuto   TargetMode = "auto"
	TargetModeManual TargetMode = "manual"
)

type ControllerConfig struct {
	TargetMode   TargetMode
	ManualTarget OSCTarget
}

type OSCStatus struct {
	Running    bool
	Connected  bool
	HasTarget  bool
	TargetMode TargetMode
	Target     OSCTarget
	LastError  string
}
```

Add these two fields to the existing `ControllerConfig` without removing or
renaming any current field.

Normalize `""` to `TargetModeAuto`. Validate manual hosts with `net/netip.ParseAddr`, `IsUnspecified`, `IsMulticast`, and explicit rejection of `255.255.255.255`; do not call DNS. Auto requires an empty `ManualTarget`; manual requires an empty `PreferredVRChatService`.

- [ ] **Step 4: Run validation tests to verify GREEN**

Run: `go test ./internal/osc -run 'TestNewControllerValidatesTargetMode|TestNewOSCService' -count=1`

Expected: PASS.

- [ ] **Step 5: Write manual ownership and avatar reception RED tests**

Use the existing controller dependency seams to prove:

```go
func TestControllerManualTargetSurvivesDiscoveryTransitions(t *testing.T) {
	// Start with TargetModeManual and 127.0.0.1:9000.
	// Simulate a usable VRChat service, then clear/reconnect it.
	// Assert Status().Connected follows discovery while Target and HasTarget
	// remain the configured manual address throughout.
}

func TestControllerManualTargetStillPublishesAvatarChanges(t *testing.T) {
	// Send /avatar/change to the real loopback receiver used by existing tests.
	// Assert AvatarChanges receives the ID while Status().Target remains manual.
}
```

Also extend the preferred-service test so a configured missing preference does not select another discovered VRChat instance.

- [ ] **Step 6: Run ownership tests to verify RED**

Run: `go test ./internal/osc -run 'TestControllerManualTarget|TestControllerPreferred' -count=1`

Expected: FAIL because discovery currently installs and clears the UDP target.

- [ ] **Step 7: Implement mode-aware target transitions**

After UDP listen succeeds in `Start`, install the parsed manual address. In `probeService`, call `SetTarget` only in auto mode. In `clearActive`, clear the UDP target only in auto mode. Always retain OSCQuery `active` state so `Connected` remains discovery-specific, and include normalized mode in `Status`:

```go
if c.config.TargetMode == TargetModeAuto {
	c.udp.SetTarget(target)
}

if c.config.TargetMode == TargetModeAuto && c.udp != nil {
	c.udp.SetTarget(nil)
}
```

Keep the M6 external catalog and avatar mailbox logic unchanged.

- [ ] **Step 8: Verify the OSC package and commit**

Run:

```powershell
gofmt -w internal/osc/controller.go internal/osc/controller_test.go internal/osc/service.go internal/osc/service_test.go
go test ./internal/osc -count=1
go test -race ./internal/osc -count=1
go vet ./internal/osc
git diff --check
```

Expected: all commands PASS.

Commit:

```powershell
git add internal/osc/controller.go internal/osc/controller_test.go internal/osc/service.go internal/osc/service_test.go
git commit -m "feat(osc): add explicit target modes"
```

---

### Task 2: Versioned Settings, Windows Paths, and Config Mapping

**Files:**
- Create: `internal/userconfig/settings.go`
- Create: `internal/userconfig/settings_test.go`
- Create: `internal/userconfig/processing.go`
- Create: `internal/userconfig/processing_test.go`
- Create: `internal/userconfig/paths.go`
- Create: `internal/userconfig/paths_test.go`
- Create: `internal/userconfig/config.go`
- Create: `internal/userconfig/config_test.go`

**Interfaces:**
- Consumes: Task 1 `osc.TargetMode`, `osc.OSCTarget`, `processing.DefaultConfig`, `plugins.DefaultOptions`, and `application.Config`.
- Produces: `Settings`, `Candidate`, `Paths`, `ResolvePaths(Environment)`, `DefaultCandidate(Paths)`, `Normalize(Candidate)`, and `ApplicationConfig(Settings, Paths)`.

```go
func ResolvePaths(Environment) (Paths, error)
func DefaultCandidate(Paths) Candidate
func Normalize(Candidate) (Candidate, error)
func ApplicationConfig(Settings, Paths) (application.Config, error)
```

- [ ] **Step 1: Write DTO clone/default RED tests**

Define tests that mutate returned roots, overrides, mutual-exclusion groups, and candidate copies and prove no aliasing. Assert the first-run candidate has empty fallback/dev roots, default processing, auto OSC, and the resolved LocalLow root.

`Settings` is the Go representation of the persisted `SettingsV1` document;
`Candidate` is the same user-editable content without schema/file revision.

Use these exact top-level shapes:

```go
type Settings struct {
	SchemaVersion int        `json:"schemaVersion"`
	Revision      uint64     `json:"revision"`
	Avatar        Avatar     `json:"avatar"`
	Plugins       Plugins    `json:"plugins"`
	Processing    Processing `json:"processing"`
	OSC           OSC        `json:"osc"`
}

type Candidate struct {
	Avatar     Avatar     `json:"avatar"`
	Plugins    Plugins    `json:"plugins"`
	Processing Processing `json:"processing"`
	OSC        OSC        `json:"osc"`
}
```

- [ ] **Step 2: Run DTO tests to verify RED**

Run: `go test ./internal/userconfig -run 'Test(DefaultCandidate|SettingsClone)' -count=1`

Expected: FAIL because the package and types do not exist.

- [ ] **Step 3: Implement settings DTOs and clone helpers**

Set `SchemaVersion = 1`, `MaxSettingsBytes = 256 << 10`, and `MaxPluginConfigBytes = 64 << 10`. Represent every processing duration as `int64` milliseconds. Use explicit wire structs mirroring calibration, tuning, filter, and dropout fields; never JSON-marshal `time.Duration` or internal enum keys directly.

```go
const (
	SchemaVersion        = 1
	MaxSettingsBytes     = 256 << 10
	MaxPluginConfigBytes = 64 << 10
)

func (settings Settings) Clone() Settings {
	clone := settings
	clone.Plugins.DevRoots = append([]string(nil), settings.Plugins.DevRoots...)
	clone.Processing = settings.Processing.Clone()
	return clone
}
```

Implement complete deep clones for all slices and nested groups, then make the DTO tests pass.

- [ ] **Step 4: Write Windows path RED tests**

Test this exact resolver contract:

```go
type Environment struct {
	GOOS          string
	RoamingDir    string
	UserProfile   string
	Executable    string
}

type Paths struct {
	SettingsDir      string
	SettingsFile     string
	PluginStoreFile  string
	BuiltinPluginDir string
	DefaultOSCRoot   string
}
```

Assert Windows derives `%AppData%/vrcft-go/config.json`, `plugins.json`, `<exe>/plugins`, and `%UserProfile%/AppData/LocalLow/VRChat/VRChat/OSC`. Assert non-Windows, blank required values, relative executable, and NUL values return sentinel `ErrUnsupportedPlatform` or `ErrInvalidEnvironment`.

- [ ] **Step 5: Implement and verify Windows paths**

Run the RED test, implement `ResolvePaths`, then run:

`go test ./internal/userconfig -run 'TestResolvePaths' -count=1`

Expected: PASS. Do not read global environment variables inside validation; production environment collection belongs in a small constructor used by root.

The core derivation is explicit:

```go
paths.SettingsDir = filepath.Join(env.RoamingDir, "vrcft-go")
paths.SettingsFile = filepath.Join(paths.SettingsDir, "config.json")
paths.PluginStoreFile = filepath.Join(paths.SettingsDir, "plugins.json")
paths.BuiltinPluginDir = filepath.Join(filepath.Dir(env.Executable), "plugins")
paths.DefaultOSCRoot = filepath.Join(env.UserProfile, "AppData", "LocalLow", "VRChat", "VRChat", "OSC")
```

- [ ] **Step 6: Write processing conversion RED tests**

Use stable names:

```text
eye.left_gaze_x
eye.left_gaze_y
eye.right_gaze_x
eye.right_gaze_y
eye.left_openness
eye.right_openness
eye.left_pupil_diameter
eye.right_pupil_diameter
eye.left_pupil_dilation
eye.right_pupil_dilation
expression:<trackingmodel.ExpressionNames value>
```

Test full default round trip, every `processing.AllChannels()` value, unknown and duplicate override names, invalid milliseconds, duplicate mutual-exclusion membership, non-finite floats, and stable sorted output.

- [ ] **Step 7: Implement processing conversion and verify GREEN**

Build both lookup directions once from the ten eye names plus
`trackingmodel.ExpressionNames()`. Convert milliseconds with overflow checks,
build fresh maps/groups, and validate by calling `processing.NewPipeline` on
the result.

```go
for id, name := range trackingmodel.ExpressionNames() {
	channel, ok := processing.ExpressionChannel(trackingmodel.ExpressionID(id))
	if !ok {
		return nil, errors.New("userconfig: expression channel table is inconsistent")
	}
	register("expression:"+name, channel)
}
```

Run: `go test ./internal/userconfig -run 'TestProcessing' -count=1`

Expected: PASS.

- [ ] **Step 8: Write normalization/config mapping RED tests**

Cover absolute cleaned paths, case-insensitive duplicate dev roots, missing-but-allowed fallback/dev paths, required OSC root, auto/manual exclusivity, manual unicast validation, and ownership of every reference-backed `application.Config` field. Assert mapping sets:

```go
config.PluginCatalog.BuiltinRoot = paths.BuiltinPluginDir
config.PluginStorePath = paths.PluginStoreFile
config.PluginOptions = plugins.DefaultOptions()
config.OSC.TargetMode = settings.OSC.TargetMode
config.FrameInterval = 0
config.PluginControlTimeout = 0
```

Also assert normalized development roots sort by case-insensitive cleaned
Windows path, then by the cleaned original path as the deterministic tie
breaker.

- [ ] **Step 9: Implement normalization and Application mapping**

`Normalize` returns a normalized owned `Candidate` plus field-addressable validation errors. `ApplicationConfig` accepts only a normalized revisioned `Settings`, constructs a fresh `application.Config`, and calls the lower-level constructors/validators indirectly through `application.NewApp`'s existing normalization seam in tests without starting work.

Map errors to an exported value:

```go
type ValidationError struct {
	Field string
	Err   error
}
```

Implement `Error` and `Unwrap` so root can classify without parsing strings.

- [ ] **Step 10: Verify the new package slice and commit**

Run:

```powershell
gofmt -w internal/userconfig/settings.go internal/userconfig/settings_test.go internal/userconfig/processing.go internal/userconfig/processing_test.go internal/userconfig/paths.go internal/userconfig/paths_test.go internal/userconfig/config.go internal/userconfig/config_test.go
go test ./internal/userconfig -count=1
go test -race ./internal/userconfig -count=1
go vet ./internal/userconfig
git diff --check
```

Expected: PASS.

Commit:

```powershell
git add internal/userconfig
git commit -m "feat(config): define M7 user settings"
```

---

### Task 3: Strict Atomic Settings Store and Repair

**Files:**
- Create: `internal/userconfig/store.go`
- Create: `internal/userconfig/store_test.go`
- Create: `internal/userconfig/replace_windows.go`
- Create: `internal/userconfig/replace_other.go`

**Interfaces:**
- Consumes: Task 2 `Settings`, `Candidate`, `Paths`, `Normalize`, and defaults.
- Produces: `Store`, `Loaded`, `DocumentToken`, `SaveResult`, `NewStore(Paths)`, `LoadOrCreate(context.Context)`, `Validate(Candidate)`, and `Save(context.Context, Loaded, Candidate)`.

```go
func NewStore(Paths) (*Store, error)
func (*Store) LoadOrCreate(context.Context) (Loaded, error)
func (*Store) Validate(Candidate) (Candidate, error)
func (*Store) Save(context.Context, Loaded, Candidate) (SaveResult, error)
```

- [ ] **Step 1: Write strict-load RED tests**

Test absent-file classification separately from malformed existing files. For existing files cover unknown and duplicate fields, required `null`, trailing JSON, invalid UTF-8, zero/unknown schema, zero revision, and 256 KiB plus one. Assert invalid bytes remain unchanged and `Loaded` includes defaults plus a diagnostic state.

Use this public shape while keeping the fingerprint opaque:

```go
type Loaded struct {
	Settings *Settings
	Defaults Candidate
	Invalid  bool
	Diagnostic error
	Token    DocumentToken
}

type SaveResult struct {
	Loaded  Loaded
	Changed bool
}
```

- [ ] **Step 2: Run strict-load tests to verify RED**

Run: `go test ./internal/userconfig -run 'TestStoreLoad' -count=1`

Expected: FAIL because Store is undefined.

- [ ] **Step 3: Implement bounded strict decoding and first-run creation**

Use `io.LimitReader(file, MaxSettingsBytes+1)`, validate UTF-8, and pre-scan every JSON object with `json.Decoder.Token` to detect duplicate keys before decoding typed DTOs with `DisallowUnknownFields`. Require EOF after one value. Keep SHA-256 and file identity inside `DocumentToken` unexported fields.

```go
data, err := io.ReadAll(io.LimitReader(file, MaxSettingsBytes+1))
if err != nil || len(data) > MaxSettingsBytes || !utf8.Valid(data) {
	return invalidLoaded(data, err)
}
if err := rejectDuplicateObjectKeys(data); err != nil {
	return invalidLoaded(data, err)
}
decoder := json.NewDecoder(bytes.NewReader(data))
decoder.DisallowUnknownFields()
```

On absence, normalize defaults, assign schema/revision 1, encode deterministically, and durably create the file. On any existing-file failure, return `Loaded{Invalid: true, Defaults: ...}` without writing.

- [ ] **Step 4: Write atomic-save/fault RED tests**

Inject an operations table for open/read/stat/temp/write/sync/close/chmod/replace. Assert each failure preserves the old authoritative file, cleans its temporary file, and emits no successful result. Test context cancellation before I/O and while waiting for the Store mutex.

Also test:

```go
func TestStoreSaveNoOpDoesNotWriteOrIncrement(t *testing.T)
func TestStoreSaveRejectsStaleTokenAfterExternalEdit(t *testing.T)
func TestStoreSaveRejectsRevisionExhaustion(t *testing.T)
func TestStoreRepairInstallsBackupBeforeReplacement(t *testing.T)
```

- [ ] **Step 5: Implement Windows/portable replacement and Store save**

Use the existing plugin-store pattern on Windows:

```go
return windows.MoveFileEx(
	oldUTF16,
	newUTF16,
	windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
)
```

`Save` locks the Store, re-reads and hashes the authoritative path, compares the opaque token, normalizes again, detects semantic no-op, and increments the persisted revision exactly once. For repair, durably install `<settings-file>.invalid.bak` before replacing the invalid original. Every returned `Loaded` owns its values and a new token.

Create the settings directory with mode `0700`, chmod temporary settings and
backup files to `0600` before writing, sync each file before replacement, and
best-effort sync the containing directory after replacement. Windows inherits
the current-user directory ACL and uses `MOVEFILE_WRITE_THROUGH` for the
authoritative replace.

- [ ] **Step 6: Verify Store behavior and commit**

Run:

```powershell
gofmt -w internal/userconfig/store.go internal/userconfig/store_test.go internal/userconfig/replace_windows.go internal/userconfig/replace_other.go
go test ./internal/userconfig -count=20
go test -race ./internal/userconfig -count=5
go vet ./internal/userconfig
git diff --check
```

Expected: PASS with no temporary fixture leakage.

Commit:

```powershell
git add internal/userconfig
git commit -m "feat(config): persist settings atomically"
```

---

### Task 4: Owned Plugin Configuration Reads

**Files:**
- Modify: `internal/plugins/api.go`
- Modify: `internal/plugins/manager.go`
- Modify: `internal/plugins/manager_test.go`

**Interfaces:**
- Consumes: Manager's locked persistent `PluginSettings` and `pluginapi.Config.Clone`.
- Produces: `Manager.PluginConfig(string) (pluginapi.Config, bool)`.

- [ ] **Step 1: Write Manager read RED tests**

Add a test after Manager has loaded known settings:

```go
got, ok := manager.PluginConfig("vendor.alpha")
if !ok || got.Revision != 2 || string(got.Data) != `{"gain":2}` {
	t.Fatalf("PluginConfig = %#v, %v", got, ok)
}
got.Data[0] = 'x'
again, _ := manager.PluginConfig("vendor.alpha")
if string(again.Data) != `{"gain":2}` {
	t.Fatal("PluginConfig aliases manager settings")
}
if _, ok := manager.PluginConfig("missing"); ok {
	t.Fatal("unknown plugin reported present")
}
```

- [ ] **Step 2: Run the Manager test to verify RED**

Run: `go test ./internal/plugins -run TestManagerPluginConfigReturnsOwnedPreference -count=1`

Expected: FAIL because the method is missing.

- [ ] **Step 3: Implement the read under the Manager lock**

Extend `Manager` and implement:

```go
func (m *pluginManager) PluginConfig(id string) (pluginapi.Config, bool) {
	m.mu.RLock()
	preference, ok := m.settings.Plugins[id]
	_, installed := m.supervisors[id]
	m.mu.RUnlock()
	if !ok || !installed {
		return pluginapi.Config{}, false
	}
	return preference.Config.Clone(), true
}
```

Do not return preferences for IDs no longer installed.

- [ ] **Step 4: Verify plugins and commit**

Run:

```powershell
gofmt -w internal/plugins/api.go internal/plugins/manager.go internal/plugins/manager_test.go
go test ./internal/plugins -run 'TestManagerPluginConfig|TestManager(UpdateConfig|Enable|Disable)' -count=10
go test -race ./internal/plugins -run 'TestManagerPluginConfig|TestManagerUpdateConfig' -count=5
go vet ./internal/plugins
git diff --check
```

Expected: PASS.

Commit:

```powershell
git add internal/plugins/api.go internal/plugins/manager.go internal/plugins/manager_test.go
git commit -m "feat(plugins): expose owned config reads"
```

---

### Task 5: Application Plugin Operations and Latest Snapshots

**Files:**
- Modify: `internal/application/app.go`
- Modify: `internal/application/app_test.go`

**Interfaces:**
- Consumes: Task 4 Manager method plus existing `Enable`, `Disable`, `UpdateConfig`, `List`, and `Subscribe`.
- Produces: the five Application methods specified in the design: `Plugins`, `PluginConfig`, `SetPluginEnabled`, `UpdatePluginConfig`, and `SubscribePlugins`.

```go
func (*Application) Plugins() []plugins.RuntimeSnapshot
func (*Application) PluginConfig(string) (pluginapi.Config, bool)
func (*Application) SetPluginEnabled(context.Context, string, bool) error
func (*Application) UpdatePluginConfig(context.Context, string, pluginapi.Config) error
func (*Application) SubscribePlugins(context.Context) <-chan []plugins.RuntimeSnapshot
```

- [ ] **Step 1: Write lifecycle and delegation RED tests**

Extend the fake Manager with call recording. Test that created, starting, closing, failed, and closed states reject mutations with `ErrInvalidLifecycle`; running delegates enable/disable/update exactly once; nil contexts fail; and `Plugins`/`PluginConfig` return owned copies.

```go
err := app.SetPluginEnabled(context.Background(), "vendor.alpha", true)
if !errors.Is(err, ErrInvalidLifecycle) {
	t.Fatalf("SetPluginEnabled error = %v", err)
}
```

- [ ] **Step 2: Run operation tests to verify RED**

Run: `go test ./internal/application -run 'TestApplicationPlugin' -count=1`

Expected: FAIL because the methods and fake interface methods are missing.

- [ ] **Step 3: Implement running-only operations**

Add Manager capabilities to `applicationPluginManager`. Add a helper that checks `applicationRunning` under `a.mu` and releases the lock before calling Manager. `SetPluginEnabled` selects `Enable` or `Disable`; `UpdatePluginConfig` forwards a clone. Wrap errors with operation and plugin ID while preserving `errors.Is`.

```go
func (a *Application) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	if err := a.requireRunning(ctx); err != nil {
		return err
	}
	if enabled {
		return a.plugins.Enable(ctx, id)
	}
	return a.plugins.Disable(ctx, id)
}
```

- [ ] **Step 4: Write latest snapshot subscription RED tests**

Test initial list delivery, coalescing after a burst larger than capacity, cancellation closure, deep ownership, and subscription during Close. The output type is exactly `<-chan []plugins.RuntimeSnapshot` and its capacity is one.

- [ ] **Step 5: Implement snapshot subscription**

Subscribe to Manager events, immediately offer `a.plugins.List()`, and on every non-log event replace the pending list:

```go
func offerPluginList(out chan []plugins.RuntimeSnapshot, values []plugins.RuntimeSnapshot) {
	owned := append([]plugins.RuntimeSnapshot(nil), values...)
	select { case <-out: default: }
	select { case out <- owned: default: }
}
```

The goroutine exits and closes `out` on context cancellation or Manager event closure. Ignore raw log events because list DTOs do not contain logs.

- [ ] **Step 6: Verify Application and commit**

Run:

```powershell
gofmt -w internal/application/app.go internal/application/app_test.go
go test ./internal/application -count=10
go test -race ./internal/application -count=5
go vet ./internal/application
git diff --check
```

Expected: PASS.

Commit:

```powershell
git add internal/application/app.go internal/application/app_test.go
git commit -m "feat(application): expose plugin operations"
```

---

### Task 6: Root Problem DTOs and Module Snapshot Store

**Files:**
- Create: `api_types.go`
- Create: `api_types_test.go`
- Create: `sanitize.go`
- Create: `sanitize_test.go`
- Create: `snapshot_store.go`
- Create: `snapshot_store_test.go`

**Interfaces:**
- Consumes: internal Application/OSC/plugin/userconfig value types.
- Produces: stable response DTOs, `Problem`, `sanitizeProblem`, `moduleEnvelope[T]`, and `moduleStore[T]`.

- [ ] **Step 1: Write Problem and sanitization RED tests**

Define exact stable codes:

```go
const (
	ProblemValidation          = "validation"
	ProblemConflict            = "conflict"
	ProblemNotFound            = "not_found"
	ProblemUnavailable         = "unavailable"
	ProblemUnsupportedPlatform = "unsupported_platform"
	ProblemTimeout             = "timeout"
	ProblemInternal            = "internal"
)

type Problem struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	Field           string `json:"field,omitempty"`
	CurrentRevision uint64 `json:"currentRevision,omitempty"`
}
```

Test `errors.Is` classification for userconfig validation/platform errors, plugin unknown/revision errors, lifecycle errors, context deadline, and an opaque internal error. Feed invalid UTF-8 and a 600-byte multibyte message and assert valid UTF-8 at no more than 512 bytes. Assert token/config payload substrings are replaced by the generic internal message.

- [ ] **Step 2: Run Problem tests to verify RED**

Run: `go test . -run 'Test(Problem|Sanitize)' -count=1`

Expected: FAIL because the Problem types and sanitizers are undefined.

- [ ] **Step 3: Implement classification and bounded strings**

Use `errors.Is`/`errors.As`, never string matching, for codes. Implement rune-safe bounded UTF-8 conversion. Validation errors carry their stable field. Conflict responses accept a caller-supplied current revision; raw wrapped errors are not used as public messages for `internal`.

```go
var validation *userconfig.ValidationError
switch {
case errors.As(err, &validation):
	return Problem{Code: ProblemValidation, Message: boundedMessage(validation.Error()), Field: validation.Field}
case errors.Is(err, context.DeadlineExceeded):
	return Problem{Code: ProblemTimeout, Message: "operation timed out"}
default:
	return Problem{Code: ProblemInternal, Message: "internal operation failed"}
}
```

- [ ] **Step 4: Write module store RED tests**

Test initial revision 1, monotonic timestamp clamping, saturating revision behavior, clone-on-read, capacity-one subscribers, latest replacement, and cancellation. Use an injected clone function and clock:

```go
type moduleEnvelope[T any] struct {
	Revision  uint64    `json:"revision"`
	UpdatedAt time.Time `json:"updatedAt"`
	Value     T         `json:"value"`
	Problem   *Problem  `json:"problem,omitempty"`
}
```

- [ ] **Step 5: Run module store tests to verify RED**

Run: `go test . -run TestModuleStore -count=1`

Expected: FAIL because `moduleStore` is undefined.

- [ ] **Step 6: Implement the generic private module store**

Keep the generic type unbound and package-private. Every public response DTO embeds or copies revision/time/value fields into a concrete non-generic struct for Wails. Store subscribers in a mutex-protected set; each channel has capacity one and receives owned values.

```go
func (store *moduleStore[T]) snapshot() moduleEnvelope[T] {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.cloneEnvelope(store.current)
}
```

- [ ] **Step 7: Define concrete DTOs and conversion tests**

Create concrete `RuntimeResponse`, `PluginListResponse`, `PluginConfigResponse`, `PluginMutationResponse`, `SettingsResponse`, `SettingsValidationResponse`, and `SettingsSaveResponse`. Plugin DTOs exclude PID/session/executable/log/subscription fields. Runtime enums are strings. Settings DTOs use the userconfig candidate wire shape and keep persisted `FileRevision` distinct from the module `Revision`.

The plugin list item is exact and intentionally contains no private config:

```go
type PluginDTO struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	Version             string     `json:"version"`
	Capabilities        []string   `json:"capabilities"`
	Enabled             bool       `json:"enabled"`
	Active              bool       `json:"active"`
	State               string     `json:"state"`
	ConfigRevision      uint64     `json:"configRevision"`
	FrameRate           float64    `json:"frameRate"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	RestartCount        int        `json:"restartCount"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
	LastHeartbeatAt     *time.Time `json:"lastHeartbeatAt,omitempty"`
	LastFrameAt         *time.Time `json:"lastFrameAt,omitempty"`
	NextRestartAt       *time.Time `json:"nextRestartAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
}
```

```go
type SettingsResponse struct {
	Revision     uint64               `json:"revision"`
	UpdatedAt    time.Time            `json:"updatedAt"`
	FileRevision uint64               `json:"fileRevision"`
	Settings     userconfig.Candidate `json:"settings"`
	Problem      *Problem             `json:"problem,omitempty"`
}
```

- [ ] **Step 8: Verify root primitives and commit**

Run:

```powershell
gofmt -w api_types.go api_types_test.go sanitize.go sanitize_test.go snapshot_store.go snapshot_store_test.go
go test . -run 'Test(Problem|Sanitize|ModuleStore|DTO)' -count=20
go test -race . -run 'TestModuleStore' -count=10
go vet .
git diff --check
```

Expected: PASS.

Commit:

```powershell
git add api_types.go api_types_test.go sanitize.go sanitize_test.go snapshot_store.go snapshot_store_test.go
git commit -m "feat(root): define bounded API snapshots"
```

---

### Task 7: Settings API

**Files:**
- Create: `settings_api.go`
- Create: `settings_api_test.go`

**Interfaces:**
- Consumes: Task 3 Store and Task 6 DTO/module store.
- Produces: exported `SettingsAPI` with `Get`, `Validate`, and `Save`; unexported `loadForStartup` and settings update subscription used by root.

- [ ] **Step 1: Write query/validate/save RED tests**

Use a fake implementing:

```go
type settingsBackend interface {
	LoadOrCreate(context.Context) (userconfig.Loaded, error)
	Validate(userconfig.Candidate) (userconfig.Candidate, error)
	Save(context.Context, userconfig.Loaded, userconfig.Candidate) (userconfig.SaveResult, error)
}
```

Test valid Get, invalid-file Get with default repair candidate, validation without writes, stale module revision conflict, successful changed save with `RestartRequired: true`, no-op save with false, Store failure without snapshot publication, and save after API closure returning unavailable.

- [ ] **Step 2: Run Settings API tests to verify RED**

Run: `go test . -run 'TestSettingsAPI' -count=1`

Expected: FAIL because `SettingsAPI` is undefined.

- [ ] **Step 3: Implement serialized Settings operations**

`SettingsAPI` holds the last `userconfig.Loaded` token privately. Serialize Validate/Save admission with a mutex. Compare `expectedRevision` to the Settings module revision before Store I/O. On changed save, update the module once and return its new concrete response. On no-op, retain module revision. Do not construct or mutate Application.

Expose only these Wails methods:

```go
func (api *SettingsAPI) Get() SettingsResponse
func (api *SettingsAPI) Validate(candidate userconfig.Candidate) SettingsValidationResponse
func (api *SettingsAPI) Save(expectedRevision uint64, candidate userconfig.Candidate) SettingsSaveResponse
```

- [ ] **Step 4: Verify Settings API and commit**

Run:

```powershell
gofmt -w settings_api.go settings_api_test.go
go test . -run 'TestSettingsAPI' -count=20
go test -race . -run 'TestSettingsAPI' -count=10
go vet .
git diff --check
```

Expected: PASS.

Commit:

```powershell
git add settings_api.go settings_api_test.go
git commit -m "feat(root): add settings API"
```

---

### Task 8: Plugins API

**Files:**
- Create: `plugins_api.go`
- Create: `plugins_api_test.go`

**Interfaces:**
- Consumes: Task 5 Application operations and Task 6 DTO/module store.
- Produces: exported `PluginsAPI` with `List`, `GetConfig`, `SetEnabled`, and `UpdateConfig`; unexported backend attach/detach and snapshot consumer methods.

- [ ] **Step 1: Write allowlisted DTO and command RED tests**

Use a fake plugin backend matching the five Application methods. Assert List conversion omits PID/session/path/log/config fields. Test unknown ID, unavailable backend, timeout, idempotent SetEnabled, and sanitized internal failure.

For UpdateConfig test empty JSON, malformed JSON, 64 KiB plus one, expected revision mismatch, `math.MaxUint64`, generated next revision, and cloned bytes:

```go
response := api.UpdateConfig("vendor.alpha", 4, `{"gain":2}`)
if fake.updated.Revision != 5 || string(fake.updated.Data) != `{"gain":2}` {
	t.Fatalf("UpdatePluginConfig = %#v", fake.updated)
}
```

- [ ] **Step 2: Run Plugins API tests to verify RED**

Run: `go test . -run 'TestPluginsAPI' -count=1`

Expected: FAIL because `PluginsAPI` is undefined.

- [ ] **Step 3: Implement bounded plugin commands**

Use one keyed mutex/admission entry per plugin ID so the same ID serializes while different IDs can proceed. Derive every command context from the root process context with a fixed two-second timeout. Re-read current config inside the keyed admission before comparing revision. Let Manager remain the final conflict authority.

Expose exactly:

```go
func (api *PluginsAPI) List() PluginListResponse
func (api *PluginsAPI) GetConfig(pluginID string) PluginConfigResponse
func (api *PluginsAPI) SetEnabled(pluginID string, enabled bool) PluginMutationResponse
func (api *PluginsAPI) UpdateConfig(pluginID string, expectedConfigRevision uint64, data string) PluginMutationResponse
```

- [ ] **Step 4: Write snapshot consumer RED tests**

Feed initial and burst plugin lists into the API's unexported consumer. Assert module revision changes only for semantically changed DTO lists, event data owns its slices, and cancellation stops updates.

- [ ] **Step 5: Implement snapshot conversion/consumer and verify**

Sort entries by plugin ID. Clamp non-finite frame rates to zero and convert every error/timestamp through Task 6 helpers. Never call `GetConfig` while building list snapshots.
Convert known capability bits in fixed `eye`, `expression`, `lip` order and
omit unknown bits from the DTO.

```go
func pluginDTO(snapshot plugins.RuntimeSnapshot) PluginDTO {
	return PluginDTO{
		ID: snapshot.ID, Name: snapshot.Name, Description: snapshot.Description,
		Version: snapshot.Version, Enabled: snapshot.Enabled, Active: snapshot.Active,
		Capabilities: capabilityNames(snapshot.Capabilities),
		State: string(snapshot.State), ConfigRevision: snapshot.ConfigRevision,
		FrameRate: finiteFrameRate(snapshot.FrameRate),
		ConsecutiveFailures: snapshot.ConsecutiveFailures,
		RestartCount: snapshot.RestartCount, StartedAt: optionalTime(snapshot.StartedAt),
		LastHeartbeatAt: optionalTime(snapshot.LastHeartbeatAt),
		LastFrameAt: optionalTime(snapshot.LastFrameAt), NextRestartAt: optionalTime(snapshot.NextRestartAt),
		LastError: boundedMessage(snapshot.LastError),
	}
}
```

Run:

```powershell
gofmt -w plugins_api.go plugins_api_test.go
go test . -run 'TestPluginsAPI' -count=20
go test -race . -run 'TestPluginsAPI' -count=10
go vet .
git diff --check
```

Expected: PASS.

- [ ] **Step 6: Commit Plugins API**

```powershell
git add plugins_api.go plugins_api_test.go
git commit -m "feat(root): add plugins API"
```

---

### Task 9: Runtime API and Versioned Wails Events

**Files:**
- Create: `runtime_api.go`
- Create: `runtime_api_test.go`
- Create: `events.go`
- Create: `events_test.go`

**Interfaces:**
- Consumes: Task 6 stores/DTOs, Application Status subscriptions, and Task 7/8 module stores.
- Produces: exported `RuntimeAPI.GetStatus`; root phase transitions; `eventEmitter`; and the three exact v1 event forwarders.

- [ ] **Step 1: Write Runtime conversion RED tests**

Test phases `created`, `starting`, `running`, `diagnostic`, `closing`, and `closed`; absent backend status; avatar plan/source string conversion; OSC target mode/discovery/target separation; bounded plugin failures; owned values; and monotonic module revision.

Expose exactly:

```go
func (api *RuntimeAPI) GetStatus() RuntimeResponse
```

- [ ] **Step 2: Run Runtime tests to verify RED**

Run: `go test . -run TestRuntimeAPI -count=1`

Expected: FAIL because `RuntimeAPI` is undefined.

- [ ] **Step 3: Implement Runtime store and Application consumer**

Add unexported methods `setPhase`, `setProblem`, and `consumeStatus(context.Context, <-chan application.Status)`. A later Application snapshot updates only the Application portion and cannot overwrite the root phase/problem. Semantically identical values do not publish a new module revision.

```go
func (api *RuntimeAPI) consumeStatus(ctx context.Context, updates <-chan application.Status) {
	for {
		select {
		case <-ctx.Done():
			return
		case status, ok := <-updates:
			if !ok {
				return
			}
			api.setApplicationStatus(status)
		}
	}
}
```

- [ ] **Step 4: Write event bridge RED tests**

Inject:

```go
type eventEmitter interface {
	Emit(context.Context, string, ...any)
}
```

Assert the names are exactly:

```go
const (
	eventRuntimeStatus  = "vrcft:v1:runtime-status"
	eventPluginsChanged = "vrcft:v1:plugins-changed"
	eventSettingsChanged = "vrcft:v1:settings-changed"
)
```

Block the fake emitter, publish a burst, release it, and assert the bridge sends the first in-flight value followed only by the newest pending value. Assert cancellation joins all forwarders and no emission occurs afterward.

- [ ] **Step 5: Run event tests to verify RED**

Run: `go test . -run TestEventBridge -count=1`

Expected: FAIL because the bridge and event constants are undefined.

- [ ] **Step 6: Implement Wails adapter and latest-only forwarders**

Wrap `runtime.EventsEmit` behind a production emitter. Each forwarder reads a capacity-one module subscription and calls `Emit` serially. Start forwarders only from root startup and retain a cancel/join handle for shutdown.

```go
type wailsEmitter struct{}

func (wailsEmitter) Emit(ctx context.Context, name string, values ...any) {
	runtime.EventsEmit(ctx, name, values...)
}
```

- [ ] **Step 7: Verify Runtime/events and commit**

Run:

```powershell
gofmt -w runtime_api.go runtime_api_test.go events.go events_test.go
go test . -run 'Test(RuntimeAPI|EventBridge)' -count=20
go test -race . -run 'Test(RuntimeAPI|EventBridge)' -count=10
go vet .
git diff --check
```

Expected: PASS.

Commit:

```powershell
git add runtime_api.go runtime_api_test.go events.go events_test.go
git commit -m "feat(root): add runtime API events"
```

---

### Task 10: Root Application Ownership and Wails Wiring

**Files:**
- Modify: `app.go`
- Modify: `main.go`
- Create: `app_test.go`
- Create: `main_test.go`

**Interfaces:**
- Consumes: Tasks 2-9 path/config/store/Application/APIs/event bridge.
- Produces: passive `NewApp`, lifecycle-owned `App.startup`/`shutdown`, and exact Wails Bind allowlist.

- [ ] **Step 1: Write passive construction and startup RED tests**

Define injected dependencies for platform environment, Store, Application constructor, clock, emitter, and shutdown timeout. Test NewApp starts no goroutine and constructs no backend. Cover:

```text
unsupported platform -> diagnostic, no Application
missing settings -> defaults persisted, one Application constructed and started
invalid settings -> diagnostic, original bytes retained, no Application
config conversion failure -> diagnostic, no Application
NewApp failure -> diagnostic
Application.Start failure -> retained backend, diagnostic, later Close
successful Start -> running, Runtime/Plugins consumers started
```

Use this test seam so production still owns the concrete Application pointer:

```go
type backendOperations interface {
	Start(context.Context) error
	Close(context.Context) error
	Status() application.Status
	SubscribeStatus(context.Context) <-chan application.Status
	Plugins() []plugins.RuntimeSnapshot
	PluginConfig(string) (pluginapi.Config, bool)
	SetPluginEnabled(context.Context, string, bool) error
	UpdatePluginConfig(context.Context, string, pluginapi.Config) error
	SubscribePlugins(context.Context) <-chan []plugins.RuntimeSnapshot
}

type ownedBackendFactory func(application.Config) (*application.Application, backendOperations, error)

func productionOwnedBackend(config application.Config) (*application.Application, backendOperations, error) {
	backend, err := application.NewApp(config)
	return backend, backend, err
}
```

- [ ] **Step 2: Run startup tests to verify RED**

Run: `go test . -run 'TestApp(New|Startup)' -count=1`

Expected: FAIL against the Wails template App.

- [ ] **Step 3: Replace the template App with lifecycle ownership**

The production struct must visibly own the backend:

```go
type App struct {
	mu       sync.Mutex
	backend  *application.Application
	backendOps backendOperations
	runtime  *RuntimeAPI
	plugins  *PluginsAPI
	settings *SettingsAPI
	// lifecycle state, context, dependencies, and forwarder handle
}
```

`startup(ctx)` publishes starting, resolves/loads/builds config, constructs a local backend, assigns `a.backend` before `Start`, then attaches running APIs. It never constructs a second backend after any startup attempt.

```go
backend, operations, err := a.dependencies.newBackend(config)
if err != nil {
	a.enterDiagnostic(err)
	return
}
a.backend = backend
a.backendOps = operations
if err := a.backendOps.Start(ctx); err != nil {
	a.enterDiagnostic(err)
	return
}
```

Start all three module-to-Wails event forwarders before configuration loading,
so Runtime diagnostics and Settings repair events remain available even when no
backend starts. Attach Runtime and Plugins Application consumers only after a
successful backend Start.

- [ ] **Step 4: Write shutdown and race RED tests**

Test shutdown before startup, after successful startup, after failed Start, repeated shutdown sharing one result, canceled Wails context, Close timeout, and concurrent startup/shutdown. Assert forwarders stop before Close, Close receives a fresh background-derived fixed timeout, and final phase is closed.

- [ ] **Step 5: Implement bounded idempotent shutdown**

Use a once/result channel or explicit lifecycle operation token. Reject new plugin commands before canceling and joining forwarders. Derive:

```go
closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
```

Call `a.backend.Close(closeCtx)` at most once and retain its sanitized outcome.

- [ ] **Step 6: Write real main wiring RED tests**

Add AST/source tests that prove `options.App` registers both callbacks and `Bind` contains only the three API pointers. Remove the template `Greet` contract.

- [ ] **Step 7: Wire `main.go`**

Use:

```go
app := NewApp()
err := wails.Run(&options.App{
	// existing title/assets/window options
	OnStartup:  app.startup,
	OnShutdown: app.shutdown,
	Bind: []interface{}{
		app.runtime,
		app.plugins,
		app.settings,
	},
})
```

Keep the existing embedded `frontend/dist`; do not touch frontend source or generated bindings.

- [ ] **Step 8: Verify root lifecycle and commit**

Run:

```powershell
gofmt -w app.go app_test.go main.go main_test.go
go test . -count=20
go test -race . -count=10
go vet .
git diff --check
```

Expected: PASS, including race tests.

Commit:

```powershell
git add app.go app_test.go main.go main_test.go
git commit -m "feat(root): own backend lifecycle"
```

---

### Task 11: Cross-Package Integration Fixture

**Files:**
- Create: `m7_backend_integration_test.go`

**Interfaces:**
- Consumes: public root API methods, real userconfig Store, Application constructor seam, and real module event bridge seam.
- Produces: deterministic proof that the approved non-frontend workflow composes without Wails UI or real VRChat/plugin processes.

- [ ] **Step 1: Write the end-to-end integration fixture**

In a temporary Windows-style environment fixture:

1. start from a missing settings file and assert default creation;
2. start a fake composed backend through root and observe Runtime running;
3. publish plugin snapshots and observe only `vrcft:v1:plugins-changed`;
4. enable a plugin and update revision-checked JSON immediately;
5. save a construction setting and assert `restartRequired` without a second backend;
6. simulate a runtime status update and observe only the Runtime event;
7. shut down and assert one Close plus no post-close event.

Name the test:

```go
func TestM7BackendPrerequisiteInterfacesEndToEnd(t *testing.T)
```

- [ ] **Step 2: Run the fixture as a fresh integration verification**

Run: `go test . -run TestM7BackendPrerequisiteInterfacesEndToEnd -count=1`

Expected: PASS because Tasks 1-10 already implement every asserted behavior. If it fails, use `superpowers:systematic-debugging` to identify the owning task contract before editing; do not add a new product behavior to make the fixture pass.

- [ ] **Step 3: Verify fixture repeated/race and commit**

Run:

```powershell
gofmt -w m7_backend_integration_test.go
go test . -run TestM7BackendPrerequisiteInterfacesEndToEnd -count=20
go test -race . -run TestM7BackendPrerequisiteInterfacesEndToEnd -count=10
git diff --check
```

Expected: PASS.

Commit the fixture:

```powershell
git add m7_backend_integration_test.go
git commit -m "test(m7): prove backend prerequisite workflow"
```

---

### Task 12: Project Specifications, Generated Evidence, and Final Gates

**Files:**
- Create: `docs/project/packages/internal-userconfig.md`
- Modify: `docs/project/packages/internal-application.md`
- Modify: `docs/project/packages/internal-osc.md`
- Modify: `docs/project/packages/root.md`
- Modify: `docs/project/subsystems/end-to-end.md`
- Modify: `internal/projectstatus/catalog_integration_test.go`
- Modify through generator only: `docs/project/status.md`

**Interfaces:**
- Consumes: all implemented contracts and executable test names from Tasks 1-11.
- Produces: authoritative package ownership, corrected structural checks, current project evidence, and a clean reviewed source commit.

- [ ] **Step 1: Write package specs with executable checks**

Add `internal-userconfig` as M7 depending on `internal-application`, with required normal/race checks and structural evidence for `TestStoreRepairInstallsBackupBeforeReplacement` and Windows path resolution. Update Application/OSC/root/end-to-end prose to match implemented interfaces and security boundaries.

Replace root's currently impossible `a.backend, err := application.NewApp(...)` regex with a valid structural check that accepts local construction followed by field ownership, for example:

```text
backend, err := application.NewApp(...)
...
a.backend = backend
a.backendOps = backend
...
a.backendOps.Start(ctx)
...
a.backendOps.Close(closeCtx)
```

Add a root structural check proving `Bind` contains exactly the three API fields and no `app` binding. Keep frontend subsystem files unchanged.

- [ ] **Step 2: Update catalog count and verify project specs**

Change `internal/projectstatus/catalog_integration_test.go` expected specs from 24 to 25. Run:

```powershell
go test ./internal/projectstatus -count=1
go run ./cmd/projectstatus
```

Expected: catalog validation PASS; projectstatus exits nonzero only for the existing frontend `type-check`, `production-build`, and `project-status-view` blockers. Root and internal-userconfig required checks pass.

- [ ] **Step 3: Run focused affected-package gates**

Run:

```powershell
go test ./internal/userconfig ./internal/osc ./internal/plugins ./internal/application . -count=1
go test ./internal/userconfig ./internal/osc ./internal/plugins ./internal/application . -count=20
go test -race ./internal/userconfig ./internal/osc ./internal/plugins ./internal/application . -count=5
go vet ./internal/userconfig ./internal/osc ./internal/plugins ./internal/application .
```

Expected: PASS.

- [ ] **Step 4: Run full repository gates**

Run:

```powershell
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
git diff --check
```

Expected: all Go commands PASS. If restricted Windows process/pipe tests fail with `Access is denied`, rerun the exact failed command with approved elevated execution and record both results; do not weaken or skip the test.

- [ ] **Step 5: Check formatting only for changed Go files**

List changed Go files relative to the design commit `a4c22ec`, inspect the paths, then run `gofmt -d` on that explicit list. Expected: no output. Do not bulk-rewrite unrelated CRLF files.

```powershell
$m7GoFiles = @(
  'internal/osc/controller.go', 'internal/osc/controller_test.go', 'internal/osc/service.go', 'internal/osc/service_test.go',
  'internal/userconfig/settings.go', 'internal/userconfig/settings_test.go', 'internal/userconfig/processing.go', 'internal/userconfig/processing_test.go',
  'internal/userconfig/paths.go', 'internal/userconfig/paths_test.go', 'internal/userconfig/config.go', 'internal/userconfig/config_test.go',
  'internal/userconfig/store.go', 'internal/userconfig/store_test.go', 'internal/userconfig/replace_windows.go', 'internal/userconfig/replace_other.go',
  'internal/plugins/api.go', 'internal/plugins/manager.go', 'internal/plugins/manager_test.go',
  'internal/application/app.go', 'internal/application/app_test.go',
  'api_types.go', 'api_types_test.go', 'sanitize.go', 'sanitize_test.go', 'snapshot_store.go', 'snapshot_store_test.go',
  'settings_api.go', 'settings_api_test.go', 'plugins_api.go', 'plugins_api_test.go', 'runtime_api.go', 'runtime_api_test.go',
  'events.go', 'events_test.go', 'app.go', 'app_test.go', 'main.go', 'main_test.go', 'm7_backend_integration_test.go',
  'internal/projectstatus/catalog_integration_test.go'
)
gofmt -d $m7GoFiles
```

- [ ] **Step 6: Commit source documentation before generating status**

```powershell
git add docs/project/packages/internal-userconfig.md docs/project/packages/internal-application.md docs/project/packages/internal-osc.md docs/project/packages/root.md docs/project/subsystems/end-to-end.md internal/projectstatus/catalog_integration_test.go
git commit -m "docs: specify M7 backend interfaces"
git status --short
```

Expected: clean worktree before status generation.

- [ ] **Step 7: Generate status from the clean reviewed source commit**

Run:

```powershell
go run ./cmd/projectstatus -write
git diff -- docs/project/status.md
```

Expected: status names the clean source commit, reports M0-M6 complete, reports the new non-frontend M7 package/root checks complete, and leaves M7 blocked only by untouched frontend checks. It must not claim full M7 completion.

- [ ] **Step 8: Commit evidence and re-run freshness checks**

```powershell
git add docs/project/status.md
git commit -m "docs: refresh M7 backend evidence"
go run ./cmd/projectstatus -check
git status --short
```

Expected: `-check` reports no stale generated status; its process may remain nonzero solely because M7 frontend requirements are blocked. Worktree is clean.

- [ ] **Step 9: Request final code review and address only verified findings**

Use `superpowers:requesting-code-review` against base `a4c22ec`. Require reviewers to classify Critical/Important/Minor, verify the approved design and this plan, inspect security/redaction and lifecycle races, and run focused normal/race/vet checks. Process findings with `superpowers:receiving-code-review`; reproduce each issue before changing code and repeat the affected gates after fixes.

- [ ] **Step 10: Run the final completion gate after review fixes**

Freshly run:

```powershell
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go run ./cmd/projectstatus -check
git diff --check
git status --short
```

Expected: all Go tests/vet/diff checks pass, status is not stale, the only projectstatus failure is the intentionally untouched frontend, and the worktree is clean. Report exact command outcomes and do not describe M7 as complete.
