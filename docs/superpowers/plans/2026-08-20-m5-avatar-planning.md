# M5 Avatar Configuration and Requirement Planning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build deterministic, fail-closed avatar JSON planning that compiles OSC input bindings, evaluator roots, tracking requirements, and generation-tagged per-plugin subscriptions.

**Architecture:** `internal/avatar` synchronously discovers and decodes the selected VRChat avatar config, then joins the existing OSC catalog, evaluator, and parameter-dependency contracts into one immutable plan. `internal/osc` gains a shared endpoint compiler and deep catalog clone; M6 remains responsible for listening to `/avatar/change` and atomically installing each returned plan.

**Tech Stack:** Go 1.25.6, standard-library `encoding/json`/`io`/`os`/`filepath`/`sync`, existing `internal/osc`, `internal/evaluator`, `internal/parameterdeps`, `pkg/pluginapi`, and `pkg/trackingmodel`.

**Spec:** `docs/superpowers/specs/2026-08-20-m5-avatar-planning-design.md`

## Global Constraints

- M5 starts no goroutine and performs no Application, frame-loop, persistence, frontend, or network wiring.
- Every fresh PowerShell terminal must initialize the fixed absolute repository cache before a Go command:

  ```powershell
  $repoRoot = (git rev-parse --show-toplevel).Trim()
  $env:GOCACHE = Join-Path $repoRoot '.go-gocache'
  New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
  ```

- Do not delete `.go-gocache/` or use the global Go cache.
- Config files are limited to 4 MiB, parameter arrays to 4096 entries, OSC addresses to 1024 bytes, and avatar IDs to 256 bytes.
- Unknown JSON object fields are accepted; known fields retain strict JSON types and trailing JSON values are rejected.
- Fallback is an explicit file path and is attempted only when no current-avatar candidate exists.
- A found current-avatar candidate never falls back after metadata, read, decode, ID, endpoint, binding, evaluator, or requirement failure.
- Normal config IDs must equal the avatar-change ID; fallback config IDs may differ.
- Every accepted `Activate` call reserves a new positive generation before configuration work; only generation exhaustion returns no plan.
- Failed and ready-but-empty plans contain no bindings, requirements, or plugin subscription and never reuse prior plan state.
- Returned plans own mutable data; evaluator plans remain immutable and OSC catalogs are deep-cloned at public boundaries.
- Preserve `internal/evaluator`'s no-OSC dependency and leave M6/M7 responsibilities deferred.
- Use TDD for every production change: focused RED, minimal GREEN, focused regression, then commit.

## File Map

- `internal/osc/catalog.go`: shared endpoint compiler and deep `Catalog.Clone`.
- `internal/osc/catalog_test.go`: endpoint/query equivalence, validation, ownership, and clone regressions.
- `internal/osc/controller.go`: use the shared catalog clone in `Controller.Catalog`.
- `internal/avatar/errors.go`: stable M5 sentinel errors.
- `internal/avatar/config.go`: bounded JSON decode and input endpoint conversion.
- `internal/avatar/config_test.go`: schema, boundary, and input conversion evidence.
- `internal/avatar/discovery.go`: avatar ID validation, multi-user resolution, regular-file checks, and fallback selection.
- `internal/avatar/discovery_test.go`: deterministic selection and not-found-only fallback evidence.
- `internal/avatar/plan.go`: immutable plan/result/status/source types and accessors.
- `internal/avatar/requirements.go`: dependency-to-capability conversion and per-plugin projection.
- `internal/avatar/requirements_test.go`: exact mask and capability-intersection evidence.
- `internal/avatar/planner.go`: serialized generation allocation and complete fail-closed compilation.
- `internal/avatar/planner_test.go`: state transitions, failure provenance, exhaustion, and concurrency.
- `internal/avatar/integration_external_test.go`: public cross-package JSON-to-evaluator/OSC compatibility proof.
- `docs/project/packages/internal-avatar.md`: authoritative M5 package specification and executable checks.
- `docs/project/packages/internal-osc.md`: shared endpoint compiler ownership.
- `docs/project/packages/internal-application.md`: planned M6 consumption of avatar plans.
- `docs/project/subsystems/end-to-end.md`: M5 component completion without claiming M6 wiring.
- `docs/project/status.md`: generator-owned final evidence only.

---

### Task 1: Share OSC Endpoint Compilation and Deep Catalog Ownership

**Files:**
- Modify: `internal/osc/catalog.go`
- Modify: `internal/osc/catalog_test.go`
- Modify: `internal/osc/controller.go`

**Interfaces:**
- Consumes: existing `Endpoint`, `ParameterCatalog`, `QueryNode`, `ParameterBinding`, and sender-private `outputBinding`.
- Produces: `BuildCatalogFromEndpoints([]Endpoint, *ParameterCatalog, uint64) (*Catalog, error)` and `(*Catalog).Clone() *Catalog`.

- [ ] **Step 1: Write endpoint compiler RED tests**

Add `TestBuildCatalogFromEndpointsMatchesQueryTree` with the same direct and binary endpoints sent through both entry points:

```go
endpoints := []Endpoint{
	{Address: "/avatar/parameters/Face/v2/JawOpen", Type: "f"},
	{Address: "/avatar/parameters/Face/v2/JawXNegative", Type: "T"},
	{Address: "/avatar/parameters/Face/v2/JawX1", Type: "T"},
	{Address: "/avatar/parameters/Face/v2/JawX2", Type: "T"},
}
fromEndpoints, err := BuildCatalogFromEndpoints(endpoints, parameterCatalog, 9)
if err != nil {
	t.Fatal(err)
}
fromQuery, err := BuildCatalog(root, parameterCatalog, 9)
if err != nil {
	t.Fatal(err)
}
if !reflect.DeepEqual(fromEndpoints, fromQuery) {
	t.Fatalf("endpoint catalog = %#v, query catalog = %#v", fromEndpoints, fromQuery)
}
```

Also assert that endpoint compilation rejects a relative address and type `s`, preserves the caller endpoint slice after the caller overwrites it, deduplicates identical outputs, and retains `ErrConflictingOSCAddress` for one address mapped to conflicting parameters.

- [ ] **Step 2: Write deep-clone RED tests**

Add `TestCatalogCloneOwnsNestedBindings`. Build a catalog containing a direct endpoint and binary group, clone it, mutate the clone's `RawMethods`, direct slice, binary bit slice, negative endpoint, and output slice through package-private access, then assert the original remains byte-for-byte equal to a pre-mutation copy. Assert `(*Catalog)(nil).Clone()` returns nil.

- [ ] **Step 3: Run the focused tests to verify RED**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/osc -run 'Test(BuildCatalogFromEndpoints|CatalogClone)' -count=1
```

Expected: build failure because both new exported methods are missing.

- [ ] **Step 4: Implement the endpoint compiler and clone**

Refactor `BuildCatalog` so it only filters writable supported query methods into owned endpoint values, then delegates:

```go
func BuildCatalog(root *QueryNode, specs *ParameterCatalog, generation uint64) (*Catalog, error) {
	if root == nil {
		return nil, fmt.Errorf("nil OSCQuery root")
	}
	endpoints := make([]Endpoint, 0)
	for _, method := range root.FlattenMethods() {
		if isWritable(method) && supportedParameterType(method.Type) {
			endpoints = append(endpoints, Endpoint{Address: method.FullPath, Type: method.Type})
		}
	}
	return BuildCatalogFromEndpoints(endpoints, specs, generation)
}
```

Move the existing binding/group/output/hash body to `BuildCatalogFromEndpoints`. Reject any endpoint for which `validAddress` is false or `supportedParameterType` is false before resolution. Copy `endpoints` before sorting or retaining values.

Implement `Catalog.Clone` with fresh `Bindings`, `Direct`, `Binary`, `Bits`, negative endpoint pointers, `RawMethods`, and private `Outputs`. Do not share a mutable slice or map with the source catalog.

- [ ] **Step 5: Route Controller through the clone and run GREEN**

Replace the hand-written shallow copy in `Controller.Catalog` with:

```go
func (c *Controller) Catalog() *Catalog {
	return c.catalog.Load().Clone()
}
```

Verify focused and complete OSC tests:

```powershell
go test ./internal/osc -run 'Test(BuildCatalog|CatalogClone|Controller)' -count=50
go test -race ./internal/osc -count=10
```

Expected: PASS; existing sender tests and benchmark contracts remain unchanged.

- [ ] **Step 6: Format, inspect, and commit**

```powershell
gofmt -w internal/osc/catalog.go internal/osc/catalog_test.go internal/osc/controller.go
git diff --check
git add internal/osc/catalog.go internal/osc/catalog_test.go internal/osc/controller.go
git commit -m "refactor(osc): compile catalogs from endpoints"
```

---

### Task 2: Decode Bounded Avatar JSON Inputs

**Files:**
- Create: `internal/avatar/errors.go`
- Create: `internal/avatar/config.go`
- Create: `internal/avatar/config_test.go`

**Interfaces:**
- Consumes: Task 1 `osc.Endpoint` and endpoint compiler validation rules.
- Produces: `readConfig(path string) (decodedConfig, error)` where `decodedConfig` owns `id string` and `endpoints []osc.Endpoint`.

- [ ] **Step 1: Define error expectations in RED tests**

Create package-internal tests with a helper that writes one fixture beneath `t.TempDir()`. The primary success test must use the documented shape and exact mappings:

```go
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "avatar.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

got, err := readConfig(writeConfig(t, `{
  "id":"avtr_demo",
  "name":"Demo",
  "future":{"accepted":true},
  "parameters":[
    {"name":"JawOpen","input":{"address":"/avatar/parameters/v2/JawOpen","type":"Float"}},
    {"name":"JawX","input":{"address":"/avatar/parameters/v2/JawX","type":"Int"}},
    {"name":"EyeTrackingActive","input":{"address":"/avatar/parameters/v2/EyeTrackingActive","type":"Bool"}},
    {"name":"IgnoredOutput","output":{"address":"/outside","type":"Float"}},
    {"name":"NoInput"}
  ]
}`))
if err != nil {
	t.Fatal(err)
}
want := []osc.Endpoint{
	{Address: "/avatar/parameters/v2/JawOpen", Type: "f"},
	{Address: "/avatar/parameters/v2/JawX", Type: "i"},
	{Address: "/avatar/parameters/v2/EyeTrackingActive", Type: "T"},
}
if got.id != "avtr_demo" || !reflect.DeepEqual(got.endpoints, want) {
	t.Fatalf("decoded = %#v", got)
}
```

Table-test missing/null/wrong-type `id` and `parameters`, wrong known `name`, malformed input objects, unknown input type, empty/relative/NUL address, trailing JSON, 4097 parameters, a 1025-byte address, and a file larger than 4 MiB. Assert the exact `errors.Is` category for every row.

- [ ] **Step 2: Run config tests to verify RED**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/avatar -run 'TestReadConfig' -count=1
```

Expected: build failure because the package and decoder do not exist.

- [ ] **Step 3: Add stable sentinels and bounded schema**

Define these exact sentinels in `errors.go`:

```go
var (
	ErrInvalidPlannerConfig   = errors.New("avatar: invalid planner configuration")
	ErrInvalidAvatarID        = errors.New("avatar: invalid avatar ID")
	ErrConfigNotFound         = errors.New("avatar: configuration not found")
	ErrInvalidConfigPath      = errors.New("avatar: invalid configuration path")
	ErrConfigTooLarge         = errors.New("avatar: configuration too large")
	ErrInvalidJSON            = errors.New("avatar: invalid configuration JSON")
	ErrConfigIDMismatch       = errors.New("avatar: configuration ID mismatch")
	ErrTooManyParameters      = errors.New("avatar: too many parameters")
	ErrInvalidInputEndpoint   = errors.New("avatar: invalid input endpoint")
	ErrBindingCompilation     = errors.New("avatar: binding compilation failed")
	ErrRequirementCompilation = errors.New("avatar: requirement compilation failed")
	ErrGenerationExhausted    = errors.New("avatar: generation exhausted")
)
```

Use constants `maxConfigBytes = 4 << 20`, `maxParameters = 4096`, `maxOSCAddressBytes = 1024`, and `maxAvatarIDBytes = 256`.

- [ ] **Step 4: Implement exact decode and endpoint conversion**

Use typed structs so unknown fields are naturally ignored while known fields reject wrong JSON types:

```go
type configEndpoint struct {
	Address string `json:"address"`
	Type    string `json:"type"`
}

type configParameter struct {
	Name   string          `json:"name"`
	Input  *configEndpoint `json:"input"`
	Output *configEndpoint `json:"output"`
}

type configDocument struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Parameters []configParameter `json:"parameters"`
}
```

Read through `io.LimitReader(file, maxConfigBytes+1)` and reject `len(data) > maxConfigBytes`. Decode once into `json.RawMessage`, then require a second decode into `struct{}` to return `io.EOF`. Unmarshal that root message into a small `map[string]json.RawMessage` to prove `id` and `parameters` are present and non-null, then unmarshal the same root into `configDocument`. Validate the array count before allocating endpoints.

For each non-nil input, require byte length `1..1024`, leading `/`, and no NUL. Map only `Int -> i`, `Bool -> T`, and `Float -> f`; wrap failures with `ErrInvalidInputEndpoint` and the parameter index. Return owned endpoint values and never retain JSON bytes.

- [ ] **Step 5: Run boundary and regression GREEN**

```powershell
gofmt -w internal/avatar/errors.go internal/avatar/config.go internal/avatar/config_test.go
go test ./internal/avatar -run 'TestReadConfig' -count=100
go test ./internal/osc ./internal/avatar -count=10
```

Expected: PASS, including exact-size and exact-count boundary cases.

- [ ] **Step 6: Commit the decoder**

```powershell
git diff --check
git add internal/avatar/errors.go internal/avatar/config.go internal/avatar/config_test.go
git commit -m "feat(avatar): decode bounded avatar configs"
```

---

### Task 3: Resolve Multi-User Configs and Not-Found-Only Fallback

**Files:**
- Create: `internal/avatar/discovery.go`
- Create: `internal/avatar/discovery_test.go`

**Interfaces:**
- Consumes: Task 2 sentinels and limits.
- Produces: `validateAvatarID(string) error` and `resolveConfig(oscRoot, fallbackPath, avatarID string) (resolvedConfig, error)` with `path`, `source`, and `requireIDMatch` fields.

- [ ] **Step 1: Write deterministic discovery RED tests**

Build `OSC/usr_a/Avatars/avtr_demo.json` and `OSC/usr_b/Avatars/avtr_demo.json` in a temporary root. Use `os.Chtimes` with literal UTC times and assert the newer file wins. Set equal times and assert the lexically smaller normalized absolute path wins. Add a deeper `OSC/usr_a/nested/Avatars` file and prove it is ignored.

Table-test `""`, `.`, `..`, `/`, `\`, `:`, NUL, and a 257-byte ID against `ErrInvalidAvatarID`. Verify local IDs such as `local_test_avatar` remain valid.

- [ ] **Step 2: Write fallback and unsafe-file RED tests**

Add `TestResolveConfigUsesFallbackOnlyWhenAvatarMissing` with these exact cases:

```text
current absent + regular fallback       -> SourceFallback, requireIDMatch false
current absent + empty fallback         -> ErrConfigNotFound
current absent + missing fallback       -> ErrConfigNotFound wrapping path context
current present regular + fallback      -> SourceAvatarConfig, requireIDMatch true
current present directory + fallback    -> ErrInvalidConfigPath, no fallback
current present symlink + fallback      -> ErrInvalidConfigPath, no fallback
```

Skip only the symlink row when `os.Symlink` itself reports an OS privilege error; all other rows must run on every platform.

- [ ] **Step 3: Run discovery tests to verify RED**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/avatar -run 'Test(ValidateAvatarID|ResolveConfig)' -count=1
```

Expected: build failure because discovery symbols are missing.

- [ ] **Step 4: Implement one-level candidate selection**

Define:

```go
type resolvedConfig struct {
	path           string
	source         Source
	requireIDMatch bool
}
```

Normalize `OSCRoot` to an absolute clean path. Glob exactly `root/*/Avatars/<id>.json`; do not walk recursively. For every glob result, call `os.Lstat`, require `Mode().IsRegular()`, reject `ModeSymlink`, and verify `filepath.Rel(root, candidate)` neither equals `..` nor begins with `..` plus a separator. Sort candidates by modification time descending and normalized absolute path ascending.

If the glob has one or more exact results, any candidate metadata/type error fails resolution and fallback is not inspected. If it has zero results, validate the configured fallback as a regular non-link file; return `ErrConfigNotFound` when fallback is empty or absent, and `ErrInvalidConfigPath` for an existing non-regular fallback.

- [ ] **Step 5: Stress deterministic selection and commit**

```powershell
gofmt -w internal/avatar/discovery.go internal/avatar/discovery_test.go
go test ./internal/avatar -run 'Test(ValidateAvatarID|ResolveConfig)' -count=200
go test -race ./internal/avatar -run 'Test(ValidateAvatarID|ResolveConfig)' -count=20
git diff --check
git add internal/avatar/discovery.go internal/avatar/discovery_test.go
git commit -m "feat(avatar): resolve avatar and fallback configs"
```

---

### Task 4: Build Immutable Requirements and Per-Plugin Subscriptions

**Files:**
- Create: `internal/avatar/plan.go`
- Create: `internal/avatar/requirements.go`
- Create: `internal/avatar/requirements_test.go`

**Interfaces:**
- Consumes: Task 1 `Catalog.Clone`, M4 `evaluator.Plan`, `parameterdeps.Inputs`, and stable plugin capability values.
- Produces: the complete spec-approved `Plan` accessor contract and `SubscriptionFor(trackingmodel.Capability) (pluginapi.Subscription, bool)`.

- [ ] **Step 1: Write requirement conversion RED tests**

Create `TestPlanSubscriptionForIntersectsCapabilities` and construct the package-internal plan directly from literal `parameterdeps.Inputs`. Assert:

```go
inputs := parameterdeps.Inputs{
	Eye: parameterdeps.EyeFieldsOf(parameterdeps.EyeFieldLeftGazeX),
	Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen),
	Active: parameterdeps.ActiveStatesOf(parameterdeps.ActiveStateLipTracking),
}
plan := &Plan{
	generation: 17,
	status:     StatusReady,
	inputs:     inputs,
	required:   requirementsFromInputs(inputs),
}
subscription, ok := plan.SubscriptionFor(
	trackingmodel.CapabilityEye | trackingmodel.CapabilityLip,
)
if !ok {
	t.Fatal("expected intersecting subscription")
}
if subscription.Generation != 17 ||
	subscription.Capabilities != trackingmodel.CapabilityEye|trackingmodel.CapabilityLip ||
	subscription.Eye != trackingmodel.EyeValidLeftGaze ||
	!subscription.Expressions.IsZero() {
	t.Fatalf("subscription = %#v", subscription)
}
if err := subscription.Validate(true); err != nil {
	t.Fatalf("subscription invalid: %v", err)
}
```

Cover Eye-active-only and Expression-active-only producing zero detail masks with enabled capability, Lip-only, full match, partial match, unknown advertised bits ignored, and no intersection returning the zero subscription plus false.

- [ ] **Step 2: Write ownership and ready/failed plan RED tests**

Assert `ParameterIDs()` returns a fresh slice each call and `Catalog()` returns deep clones. A failed plan must expose generation/avatar/source diagnostics but nil catalog/evaluator, empty IDs/inputs, and no subscription. A ready plan with no IDs must remain `StatusReady` and also have no subscription.

- [ ] **Step 3: Run plan tests to verify RED**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/avatar -run 'Test(Requirements|Plan|SubscriptionFor)' -count=1
```

Expected: build failure for missing plan and requirement symbols.

- [ ] **Step 4: Implement the immutable plan shape**

Use unexported fields and exact accessors from the design. Store a base capability/detail value rather than a fabricated empty plugin subscription:

```go
type Status uint8

const (
	StatusReady Status = iota + 1
	StatusFailed
)

type Source uint8

const (
	SourceAvatarConfig Source = iota + 1
	SourceFallback
	SourceNone
)

type Result struct {
	Plan *Plan
	Err  error
}

type trackingRequirements struct {
	capabilities trackingmodel.Capability
	eye          trackingmodel.EyeValid
	expressions  trackingmodel.ExpressionMask
}

type Plan struct {
	generation uint64
	status     Status
	avatarID   string
	configID   string
	configPath string
	source     Source
	ids        []parameters.ParameterID
	catalog    *osc.Catalog
	evaluator  *evaluator.Plan
	inputs     parameterdeps.Inputs
	required   trackingRequirements
}

func (p *Plan) Generation() uint64
func (p *Plan) Status() Status
func (p *Plan) AvatarID() string
func (p *Plan) ConfigID() string
func (p *Plan) ConfigPath() string
func (p *Plan) Source() Source
func (p *Plan) ParameterIDs() []parameters.ParameterID
func (p *Plan) Catalog() *osc.Catalog
func (p *Plan) Evaluator() *evaluator.Plan
func (p *Plan) RequiredInputs() parameterdeps.Inputs
func (p *Plan) SubscriptionFor(trackingmodel.Capability) (pluginapi.Subscription, bool)

func requirementsFromInputs(parameterdeps.Inputs) trackingRequirements
func newReadyPlan(generation uint64, avatarID, configID, configPath string, source Source, ids []parameters.ParameterID, catalog *osc.Catalog, evaluatorPlan *evaluator.Plan, inputs parameterdeps.Inputs) *Plan
func newFailedPlan(generation uint64, avatarID string, source Source, configPath, configID string) *Plan
```

Derive capability bits from non-zero numeric leaves and the three independent `ActiveState` bits. `SubscriptionFor` intersects only `CapabilityEye|CapabilityExpression|CapabilityLip`, clears detail masks for removed groups, normalizes the result, and returns false when the intersection is empty. Tests must call `Validate(true)` on every true result so an invalid constructed subscription cannot pass.

- [ ] **Step 5: Verify immutable behavior and commit**

```powershell
gofmt -w internal/avatar/plan.go internal/avatar/requirements.go internal/avatar/requirements_test.go
go test ./internal/avatar -run 'Test(Requirements|Plan|SubscriptionFor)' -count=100
go test -race ./internal/avatar -run 'Test(Requirements|Plan|SubscriptionFor)' -count=20
git diff --check
git add internal/avatar/plan.go internal/avatar/requirements.go internal/avatar/requirements_test.go
git commit -m "feat(avatar): compile tracking requirements"
```

---

### Task 5: Orchestrate Generation-Tagged Fail-Closed Planning

**Files:**
- Create: `internal/avatar/planner.go`
- Create: `internal/avatar/planner_test.go`

**Interfaces:**
- Consumes: Tasks 1–4 discovery, decoder, OSC compiler, immutable plan, evaluator, and dependency closure.
- Produces: `NewPlanner(PlannerConfig) (*Planner, error)` and serialized `(*Planner).Activate(string) Result`.

- [ ] **Step 1: Write successful planner RED tests**

Create a real temp fixture containing direct JawOpen, derived EyeX, and LipTrackingActive inputs. Assert the first result is ready generation 1, uses `SourceAvatarConfig`, returns IDs in numeric order, has catalog generation 1, compiles a non-nil evaluator, resolves exact Eye/Expression/Lip requirements, and produces valid projected subscriptions.

Call `Activate` again with the same ID after replacing the JSON with a different valid binding. Assert generation 2 and prove no catalog, ID, or requirement from generation 1 remains.

- [ ] **Step 2: Write fail-closed transition RED tests**

Use one planner for this sequence:

```text
generation 1: valid current config                  -> ready
generation 2: current config malformed              -> failed, empty, ErrInvalidJSON
generation 3: current absent + valid fallback       -> ready, SourceFallback
generation 4: current ID mismatch + valid fallback  -> failed, empty, ErrConfigIDMismatch
generation 5: current binding conflict              -> failed, empty, ErrBindingCompilation
```

For every failed result, assert non-nil plan, exact new generation, nil catalog/evaluator, zero IDs/inputs, and `SubscriptionFor` false. The generation-4 row proves a found bad current config never uses fallback.

- [ ] **Step 3: Write generation and concurrency RED tests**

Set the package-private counter to `math.MaxUint64-1`, require one plan at `MaxUint64`, then require `ErrGenerationExhausted` with nil plan and no wrap. Launch 32 goroutines against one valid fixture; collect `Result` values through a buffered channel, never call `testing.T` inside workers, and assert 32 distinct consecutive positive generations under `-race`.

- [ ] **Step 4: Run planner tests to verify RED**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/avatar -run 'TestPlanner' -count=1
```

Expected: build failure because Planner is missing.

- [ ] **Step 5: Implement constructor and serialized activation**

Define:

```go
type PlannerConfig struct {
	OSCRoot      string
	FallbackPath string
}

type Planner struct {
	mu           sync.Mutex
	generation   uint64
	oscRoot      string
	fallbackPath string
	specs        *osc.ParameterCatalog
}
```

`NewPlanner` requires non-empty `OSCRoot`, converts root and non-empty fallback to absolute clean paths, and compiles `osc.NewVRCFTParameterCatalog()` once. It need not require the root or fallback to exist at construction because VRChat may create them later.

`Activate` holds the mutex through the complete synchronous operation. Reserve generation first; validate ID; resolve; decode; enforce normal config ID equality; call `osc.BuildCatalogFromEndpoints`; sort catalog binding IDs; call `evaluator.Compile`; call `parameterdeps.RequiredInputs`; then build the ready plan.

Use one helper for every ordinary failure:

```go
func failedResult(generation uint64, avatarID string, source Source, path, configID string, err error) Result {
	return Result{
		Plan: newFailedPlan(generation, avatarID, source, path, configID),
		Err:  err,
	}
}
```

Wrap decoder, binding, and evaluator/dependency errors with their exact M5 sentinel while preserving the owning package error for `errors.Is`. Never retain any previous plan in `Planner`.

- [ ] **Step 6: Run state, stress, and race GREEN**

```powershell
gofmt -w internal/avatar/planner.go internal/avatar/planner_test.go
go test ./internal/avatar -run 'TestPlanner' -count=100
go test -race ./internal/avatar -run 'TestPlanner' -count=20
go test ./internal/avatar ./internal/osc ./internal/evaluator ./internal/parameterdeps -count=10
```

Expected: PASS with unique generations and exact failure categories.

- [ ] **Step 7: Commit the complete planner**

```powershell
git diff --check
git add internal/avatar/planner.go internal/avatar/planner_test.go
git commit -m "feat(avatar): plan generation-tagged avatar requirements"
```

---

### Task 6: Prove Public Cross-Package Compatibility and Ownership

**Files:**
- Create: `internal/avatar/integration_external_test.go`
- Modify: `internal/avatar/planner_test.go` only when an ownership failure requires a package-internal regression

**Interfaces:**
- Consumes: Task 5 public planner/plan APIs and existing processing/evaluator/OSC contracts.
- Produces: external-package evidence that local JSON bindings drive only requested evaluator outputs and satisfy `osc.ValueSource`.

- [ ] **Step 1: Write the external integration fixture**

Use `package avatar_test`. Create a temp `OSC/usr_test/Avatars/avtr_demo.json` containing JawOpen and ExpressionTrackingActive inputs. Construct the planner, activate the avatar, and assert ready generation 1.

Evaluate this exact canonical input:

```go
var frame processing.CanonicalFrame
frame.Generation = 1
frame.ExpressionActive = true
frame.Expressions.Set(trackingmodel.ExpressionJawOpen, 0.75)
snapshot := result.Plan.Evaluator().Evaluate(frame)
var source osc.ValueSource = snapshot
if value, ok := source.Float(parameters.ParameterJawOpen); !ok || value != 0.75 {
	t.Fatalf("JawOpen = %v,%t", value, ok)
}
if value, ok := source.Bool(parameters.ParameterExpressionTrackingActive); !ok || !value {
	t.Fatalf("ExpressionTrackingActive = %v,%t", value, ok)
}
if _, ok := source.Float(parameters.ParameterJawX); ok {
	t.Fatal("unbound JawX became externally visible")
}
```

Assert the catalog contains exactly those two ParameterIDs and matching input addresses. Do not start a UDP listener, Controller, Application, or goroutine.

- [ ] **Step 2: Add public ownership proof**

Fetch `ParameterIDs` and `Catalog`, mutate all exported slice/map layers, fetch them again, and assert the plan remains unchanged. Activate a second time after modifying caller-returned values and prove the next plan compiles from disk rather than mutated prior data.

- [ ] **Step 3: Run integration RED/GREEN and full M5 race checks**

The new fixture should pass without production changes. If it fails, add a focused package-internal RED test for the owning defect before changing code.

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/avatar -run 'TestAvatarPlanDrivesRequestedEvaluatorOutputs' -count=100
go test -race ./internal/avatar ./internal/osc -count=20
go vet ./internal/avatar ./internal/osc
```

- [ ] **Step 4: Review dependency direction and commit**

Confirm with `go list -deps ./internal/evaluator` that no line equals `github.com/wzhqwq/vrcft-go/internal/osc`, and confirm `internal/avatar` contains no `go` statement or network constructor.

```powershell
go list -deps ./internal/evaluator | Select-String 'github.com/wzhqwq/vrcft-go/internal/osc'
rg -n '^\s*go\s+|NewController|ListenUDP|net\.' internal/avatar
```

Expected: both inspection commands produce no matches.

```powershell
git add internal/avatar/integration_external_test.go internal/avatar/planner_test.go
git commit -m "test(avatar): prove M5 planning compatibility"
```

---

### Task 7: Register M5 Package Ownership and Executable Evidence

**Files:**
- Create: `docs/project/packages/internal-avatar.md`
- Modify: `docs/project/packages/internal-osc.md`
- Modify: `docs/project/packages/internal-application.md`
- Modify: `docs/project/subsystems/end-to-end.md`

**Interfaces:**
- Consumes: reviewed Tasks 1–6 and their exact symbols/tests.
- Produces: authoritative project ownership and M5 acceptance checks without claiming M6/M7 completion.

- [ ] **Step 1: Create the internal-avatar package spec**

Use this exact front matter:

```yaml
---
id: internal-avatar
kind: go-package
path: internal/avatar
milestone: M5
depends_on: [internal-osc, internal-evaluator, internal-parameterdeps, internal-parameters, pkg-pluginapi, pkg-trackingmodel]
checks:
  - id: package-tests
    description: Avatar planning package tests pass
    type: command
    command: go-test
    args: [./internal/avatar]
    weight: 3
    required: true
  - id: package-race-tests
    description: Avatar planning is race-free
    type: command
    command: go-test-race
    args: [./internal/avatar]
    weight: 2
    required: true
  - id: planner-implemented
    description: Generation-tagged avatar planner exists
    type: symbol
    path: internal/avatar/planner.go
    pattern: '(?m)^func \(p \*Planner\) Activate\('
    weight: 2
    required: true
  - id: fallback-policy-tested
    description: Fallback is limited to missing avatar configs
    type: symbol
    path: internal/avatar/discovery_test.go
    pattern: '(?m)^func TestResolveConfigUsesFallbackOnlyWhenAvatarMissing\('
    weight: 1
    required: true
  - id: subscriptions-tested
    description: Per-plugin capability projection is tested
    type: symbol
    path: internal/avatar/requirements_test.go
    pattern: '(?m)^func TestPlanSubscriptionForIntersectsCapabilities\('
    weight: 2
    required: true
---
```

Document purpose, responsibilities, non-responsibilities, current implementation, exact public interfaces, owned data, dependencies, synchronization, errors, bounds, security, tests, remaining M6/M7 gaps, and completion definition from the approved design.

- [ ] **Step 2: Update adjacent specs with implemented facts**

In `internal-osc.md`, state that endpoint and OSCQuery sources share one binding compiler and catalogs have deep-clone ownership; keep avatar discovery outside OSC. In `internal-application.md`, add `internal-avatar` to `depends_on` and describe plan installation as remaining M6 work. In `end-to-end.md`, add `internal-avatar` to `depends_on` and add `internal-avatar:package-tests` to `pipeline-components`; change prose from “M5 deferred” to “avatar planning implemented as a component, Application atomic installation absent.”

- [ ] **Step 3: Verify catalog discovery before committing docs**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
go test ./internal/projectstatus -count=1
go run ./cmd/projectstatus
```

Expected: catalog validation succeeds, `internal-avatar` appears under M5, and its checks pass. Global status may remain blocked by M6/frontend checks.

- [ ] **Step 4: Inspect symbols and commit package specs**

```powershell
rg -n 'func \(p \*Planner\) Activate|func TestResolveConfigUsesFallbackOnlyWhenAvatarMissing|func TestPlanSubscriptionForIntersectsCapabilities' internal/avatar
git diff --check
git add docs/project/packages/internal-avatar.md docs/project/packages/internal-osc.md docs/project/packages/internal-application.md docs/project/subsystems/end-to-end.md
git commit -m "docs: specify M5 avatar planning"
```

---

### Task 8: Complete Verification, Review, and Generated M5 Status

**Files:**
- Modify: production/tests/specs only for a focused review finding proven by RED
- Modify: `docs/project/status.md` only through `go run ./cmd/projectstatus -write`

**Interfaces:**
- Consumes: clean reviewed Tasks 1–7.
- Produces: final normal/race/vet evidence, review closure, generated M5 completion status, and a clean tracked worktree.

- [ ] **Step 1: Run formatting and focused M5 verification**

```powershell
$repoRoot = (git rev-parse --show-toplevel).Trim(); $env:GOCACHE = Join-Path $repoRoot '.go-gocache'; New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
$files = Get-ChildItem internal/avatar,internal/osc -Filter '*.go' -File
gofmt -d ($files.FullName)
go test ./internal/avatar ./internal/osc -count=20
go test -race ./internal/avatar ./internal/osc -count=10
go vet ./internal/avatar ./internal/osc
git diff --check
```

Expected: no gofmt output; all commands exit 0.

- [ ] **Step 2: Run complete repository verification**

```powershell
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/projectstatus
git status --short
```

Expected: Go tests/race/vet exit 0; projectstatus reports M5 complete while unrelated M6/M7 checks may keep global state blocked; tracked worktree is clean before review.

- [ ] **Step 3: Perform whole-range code review**

Invoke `superpowers:requesting-code-review` over design commit `56a8b2d` through implementation/spec HEAD. The review must explicitly check:

- newest-mtime plus lexical tie discovery and root containment;
- fallback only on true absence, never after a found-candidate failure;
- bounded forward-compatible JSON decoding and input-only semantics;
- endpoint/query compiler parity and deep catalog ownership;
- stable ID ordering and exact evaluator/dependency root agreement;
- Eye/Expression/Lip active-only subscription behavior and per-plugin intersection;
- fail-closed non-nil plans, repeated-ID generation advance, no generation wrap;
- no retained mutable slices/maps and race-free activation;
- evaluator-to-OSC dependency direction; and
- accurate M5 completion without M6/M7 overclaiming.

For each Critical or Important finding, write a focused failing test, run it to prove RED, make the smallest owning fix, run focused GREEN plus affected race tests, and commit the fix before requesting re-review. Finish with no open Critical or Important finding.

- [ ] **Step 4: Regenerate status from the reviewed clean source commit**

Confirm `git status --short` is empty, then run:

```powershell
go run ./cmd/projectstatus -write
git diff -- docs/project/status.md
```

The generated report must name the reviewed source commit, say `Dirty: false`, show `internal-avatar` complete and M5 100%, retain M6 Application/end-to-end blockers and M7 frontend gaps, and contain no stale M5 wording.

- [ ] **Step 5: Commit generated evidence and recheck freshness**

```powershell
git add docs/project/status.md
git commit -m "docs: refresh M5 completion evidence"
go run ./cmd/projectstatus -check
git status --short
git log -12 --oneline --decorate
```

`projectstatus -check` may exit non-zero only because the overall project remains blocked; its output must not say the generated status is stale. Final `git status --short` must be empty.

---

## Plan Completion Gate

Do not mark M5 complete from symbol checks alone. Completion requires all eight tasks, task-level RED/GREEN evidence, fixed-cache normal/race/vet verification, whole-range review with no open Critical or Important finding, accurate package/subsystem specs, clean-source generated status showing M5 100%, and a clean tracked worktree. Application installation, fallback persistence/UI, and the end-to-end OSC loop remain M6/M7 work.
