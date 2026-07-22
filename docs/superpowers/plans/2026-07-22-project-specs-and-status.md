# Project Specs and Live Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an authoritative spec for every package and subsystem, plus a safe Go tool that derives current package, milestone, and project progress from executable acceptance checks.

**Architecture:** Markdown specs carry YAML front matter containing dependencies and typed checks. `internal/projectstatus` parses and validates those specs, discovers repository packages, executes only registered commands, calculates weighted states, and renders deterministic Markdown or JSON; `cmd/projectstatus` exposes print, write, and freshness-check modes.

**Tech Stack:** Go 1.25.6, `gopkg.in/yaml.v3`, Go standard library (`context`, `os/exec`, `regexp`, `encoding/json`, `text/template`, `testing`), Markdown documentation.

## Global Constraints

- Cover all packages returned by `go list ./...` and the frontend, parameter-spec, build-release, and end-to-end subsystems.
- Progress is derived from acceptance checks, never code lines, commit counts, or manually edited percentages.
- Specs may invoke only registered command IDs with argument arrays; never execute shell strings.
- Repository-relative paths must resolve inside the repository root.
- Command checks default to a 120-second timeout and identical commands execute once per run.
- Independent checks continue after one check fails.
- `status.md` is generated atomically and must not be used as an input for current progress except to identify degradation.
- Exit code 0 means all required checks pass, 1 means a valid report contains failed or blocked required checks, and 2 means the tool or specification is invalid.
- Generated Markdown is deterministic after excluding generation time; JSON schema version starts at `1`.
- This implementation documents missing product features but does not implement those features.

---

## File Structure

- Create `internal/projectstatus/model.go`: schema and result types.
- Create `internal/projectstatus/parser.go`: front matter and required-section parsing.
- Create `internal/projectstatus/discovery.go`: repository root, package discovery, spec coverage, and dependency graph validation.
- Create `internal/projectstatus/checks.go`: typed checks, registered commands, caching, timeouts, bounded output, and safe paths.
- Create `internal/projectstatus/progress.go`: check, spec, milestone, and project state calculation.
- Create `internal/projectstatus/report.go`: deterministic Markdown and JSON report models.
- Create matching focused `*_test.go` files and `testdata/` fixtures.
- Create `cmd/projectstatus/main.go`: CLI argument parsing and exit behavior.
- Create `docs/project/README.md`, `milestones.md`, 15 package specs, and 4 subsystem specs.
- Generate `docs/project/status.md` only after the real catalog validates.
- Modify root `README.md` to link the project documentation and status command.

### Task 1: Specification Model and Parser

**Files:**
- Create: `internal/projectstatus/model.go`
- Create: `internal/projectstatus/parser.go`
- Create: `internal/projectstatus/parser_test.go`
- Create: `internal/projectstatus/testdata/valid.md`
- Create: `internal/projectstatus/testdata/missing-section.md`

**Interfaces:**
- Produces: `Spec`, `CheckSpec`, `BlockerSpec`, `ParseSpec(path string, content []byte) (Spec, error)`, and `ValidateSpec(Spec) error`.
- Consumes: `gopkg.in/yaml.v3` already present in `go.mod`.

- [ ] **Step 1: Write parser tests before production code**

Create a valid fixture containing YAML front matter and all 14 required body
headings. Test exact field decoding, front matter delimiters, and body capture:

```go
func TestParseSpec(t *testing.T) {
    content, err := os.ReadFile("testdata/valid.md")
    if err != nil { t.Fatal(err) }
    spec, err := ParseSpec("testdata/valid.md", content)
    if err != nil { t.Fatal(err) }
    if spec.ID != "internal-osc" || spec.Kind != KindGoPackage || spec.Path != "internal/osc" {
        t.Fatalf("unexpected spec: %#v", spec)
    }
    if len(spec.Checks) != 1 || spec.Checks[0].Command != "go-test" {
        t.Fatalf("unexpected checks: %#v", spec.Checks)
    }
}

func TestParseSpecRejectsMissingRequiredSection(t *testing.T) {
    content, err := os.ReadFile("testdata/missing-section.md")
    if err != nil { t.Fatal(err) }
    _, err = ParseSpec("testdata/missing-section.md", content)
    if !errors.Is(err, ErrMissingSection) {
        t.Fatalf("error = %v, want ErrMissingSection", err)
    }
}
```

Add table cases for missing closing delimiter, duplicate check IDs, zero weight,
no required checks, unknown kind, absolute paths, `..` paths, unknown check
type, and a blocker referencing an unknown check.

- [ ] **Step 2: Run the parser tests and verify RED**

Run: `go test ./internal/projectstatus -run 'TestParseSpec'`

Expected: build failure because `ParseSpec`, schema types, and validation errors
do not exist.

- [ ] **Step 3: Define exact schema types**

```go
type SpecKind string
const (
    KindGoPackage SpecKind = "go-package"
    KindFrontend SpecKind = "frontend"
    KindParameterSpec SpecKind = "parameter-spec"
    KindBuildRelease SpecKind = "build-release"
    KindEndToEnd SpecKind = "end-to-end"
)

type CheckType string
const (
    CheckCommand CheckType = "command"
    CheckFile CheckType = "file"
    CheckSymbol CheckType = "symbol"
    CheckNotPlaceholder CheckType = "not_placeholder"
    CheckGeneratedClean CheckType = "generated_clean"
    CheckDependsComplete CheckType = "depends_complete"
    CheckAggregate CheckType = "aggregate"
)

type CheckSpec struct {
    ID string `yaml:"id"`
    Description string `yaml:"description"`
    Type CheckType `yaml:"type"`
    Command string `yaml:"command,omitempty"`
    Args []string `yaml:"args,omitempty"`
    Path string `yaml:"path,omitempty"`
    Pattern string `yaml:"pattern,omitempty"`
    Members []string `yaml:"members,omitempty"`
    Weight int `yaml:"weight"`
    Required bool `yaml:"required"`
    TimeoutSeconds int `yaml:"timeout_seconds,omitempty"`
}

type BlockerSpec struct {
    Check string `yaml:"check"`
    Blocks []string `yaml:"blocks"`
}

type Spec struct {
    SourcePath string `yaml:"-"`
    ID string `yaml:"id"`
    Kind SpecKind `yaml:"kind"`
    Path string `yaml:"path"`
    Milestone string `yaml:"milestone"`
    Planned bool `yaml:"planned,omitempty"`
    DependsOn []string `yaml:"depends_on,omitempty"`
    Checks []CheckSpec `yaml:"checks"`
    Blockers []BlockerSpec `yaml:"blockers,omitempty"`
    Body string `yaml:"-"`
}
```

Define sentinel errors `ErrMalformedFrontMatter`, `ErrInvalidSpec`, and
`ErrMissingSection` and wrap them with source path and field context.

- [ ] **Step 4: Implement front matter and body validation**

Split only on delimiter lines equal to `---`, decode YAML with
`yaml.Decoder.KnownFields(true)`, normalize dependency/member slices to stable
order, and verify the headings listed in the design in their required order.
Use `path.Clean` for spec paths and reject absolute paths, `.` for non-root
specs, and any cleaned path beginning with `../`.

- [ ] **Step 5: Run parser and package tests**

Run: `go test ./internal/projectstatus -run 'TestParseSpec|TestValidateSpec'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/projectstatus/model.go internal/projectstatus/parser.go internal/projectstatus/parser_test.go internal/projectstatus/testdata
git commit -m "feat(projectstatus): parse package specifications"
```

### Task 2: Repository Discovery and Dependency Validation

**Files:**
- Create: `internal/projectstatus/discovery.go`
- Create: `internal/projectstatus/discovery_test.go`

**Interfaces:**
- Consumes: validated `[]Spec` from Task 1.
- Produces: `FindRepositoryRoot(start string) (string, error)`, `DiscoverGoPackages(ctx context.Context, root string, runner CommandRunner) ([]string, error)`, `LoadCatalog(root string) (*Catalog, error)`, and `ValidateCatalog(*Catalog, []string) error`.

- [ ] **Step 1: Write failing catalog tests**

Use temporary directories and a fake package discoverer. Cover:

```go
func TestValidateCatalogRejectsUnregisteredPackage(t *testing.T)
func TestValidateCatalogRejectsDuplicatePath(t *testing.T)
func TestValidateCatalogRejectsUnknownDependency(t *testing.T)
func TestValidateCatalogRejectsDependencyCycle(t *testing.T)
func TestValidateCatalogAcceptsMissingPlannedPackage(t *testing.T)
func TestValidateCatalogRejectsDiscoveredPackageStillMarkedPlanned(t *testing.T)
```

For the cycle case use `a -> b -> c -> a` and assert the error includes the full
cycle. For discovery normalization, feed module paths such as
`github.com/wzhqwq/vrcft-go/internal/osc` and expect `internal/osc`.

- [ ] **Step 2: Run discovery tests and verify RED**

Run: `go test ./internal/projectstatus -run 'Test(ValidateCatalog|DiscoverGoPackages|FindRepositoryRoot)'`

Expected: build failure because catalog discovery functions do not exist.

- [ ] **Step 3: Implement catalog loading and root discovery**

Walk from `start` toward the filesystem root until `go.mod` is found. Load
`docs/project/packages/*.md` and `docs/project/subsystems/*.md` in sorted order,
parse each with `ParseSpec`, reject duplicate IDs and paths, then validate the
dependency graph with depth-first visitation states and a stack that produces a
readable cycle.

- [ ] **Step 4: Implement Go package discovery**

Introduce the testable interface:

```go
type CommandRequest struct {
    ID string
    Args []string
    Timeout time.Duration
    Dir string
}

type CommandOutput struct {
    ExitCode int
    Stdout string
    Stderr string
    Duration time.Duration
    TimedOut bool
}

type CommandRunner interface {
    Run(context.Context, CommandRequest) CommandOutput
}

type Catalog struct {
    Specs []Spec
    ByID map[string]int
}
```

Call registered command `go-list` with `./...`, parse one import path per line,
map the module root package to `.`, and reject packages outside the module.

- [ ] **Step 5: Run discovery tests**

Run: `go test ./internal/projectstatus -run 'Test(ValidateCatalog|DiscoverGoPackages|FindRepositoryRoot)'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/projectstatus/discovery.go internal/projectstatus/discovery_test.go
git commit -m "feat(projectstatus): discover and validate project catalog"
```

### Task 3: Safe Acceptance Check Runner

**Files:**
- Create: `internal/projectstatus/checks.go`
- Create: `internal/projectstatus/checks_test.go`
- Create: `internal/projectstatus/testdata/helper_process_test.go`

**Interfaces:**
- Consumes: `Spec`, `CheckSpec`, and command request/output types.
- Produces: `NewRunner(root string) *Runner`, `(*Runner).Run(context.Context, CommandRequest) CommandOutput`, `Evaluator`, `CheckResult`, and `(*Evaluator).Evaluate(context.Context, []Spec) []SpecResult`.

- [ ] **Step 1: Write failing runner safety tests**

Test that command IDs expand exactly as follows:

```text
go-list        -> go list <args>
go-test        -> go test <args>
go-test-build  -> go test -run ^$ <args>
go-test-race   -> go test -race <args>
go-vet         -> go vet <args>
go-generate-check -> go generate <args>
frontend-test  -> npm run check --prefix frontend
frontend-build -> npm run build --prefix frontend
git-head       -> git rev-parse HEAD
git-status     -> git status --porcelain
```

Test unknown IDs, shell tokens in args, path escape, context timeout, nonzero
exit, 64 KiB output truncation per stream, and cache sharing. The cache test
evaluates two identical requests and asserts the helper process runs once.

- [ ] **Step 2: Run check tests and verify RED**

Run: `go test ./internal/projectstatus -run 'Test(Runner|Evaluator|SafePath)'`

Expected: build failure because runner and evaluator do not exist.

- [ ] **Step 3: Implement registered commands and bounded execution**

Use a private registry:

```go
type commandDefinition struct {
    executable string
    prefix []string
    validate func([]string) error
}
```

Run `exec.CommandContext` directly with `cmd.Dir = root`. Reject arguments that
contain NUL, equal shell operators (`|`, `||`, `&&`, `;`, `>`, `>>`, `<`), or
begin with an unsupported environment-assignment form. Capture stdout and
stderr through a mutex-protected writer capped at 64 KiB while recording
truncation. On Windows, rely on `CommandContext` process termination for the
first version and record timeout explicitly.

- [ ] **Step 4: Implement typed checks**

Define:

```go
type CheckState string
const (
    CheckPassed CheckState = "passed"
    CheckFailed CheckState = "failed"
    CheckBlocked CheckState = "blocked"
)

type CheckResult struct {
    SpecID string
    CheckID string
    State CheckState
    Weight int
    Required bool
    Evidence string
    Duration time.Duration
}

type SpecResult struct {
    Spec Spec
    Checks []CheckResult
}
```

Implement file-kind checks with `os.Stat`, symbol checks with compiled regular
expressions, placeholder checks using only patterns listed in that spec,
dependency and aggregate checks from already calculated results, and command
checks through cached `CommandRequest` keys. Resolve every path with
`filepath.Abs` plus `filepath.Rel` and reject paths whose relative form is `..`
or starts with `..` plus a separator.

For `generated_clean`, copy only declared `Members` into a temporary directory,
run registered `go-generate-check` there, and compare SHA-256 hashes before and
after. Never run a generator in the user's working tree.

- [ ] **Step 5: Run runner tests**

Run: `go test ./internal/projectstatus -run 'Test(Runner|Evaluator|SafePath|GeneratedClean)'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/projectstatus/checks.go internal/projectstatus/checks_test.go internal/projectstatus/testdata/helper_process_test.go
git commit -m "feat(projectstatus): execute safe acceptance checks"
```

### Task 4: Weighted Progress and Deterministic Reports

**Files:**
- Create: `internal/projectstatus/progress.go`
- Create: `internal/projectstatus/progress_test.go`
- Create: `internal/projectstatus/report.go`
- Create: `internal/projectstatus/report_test.go`

**Interfaces:**
- Consumes: `[]SpecResult`, milestone definitions, repository metadata, and optional previous report state.
- Produces: `BuildStatus(StatusInput) Status`, `RenderMarkdown(Status) ([]byte, error)`, and `RenderJSON(Status) ([]byte, error)`.

- [ ] **Step 1: Write failing state and weight tests**

Table-test all states and precedence:

```go
func TestCalculateSpecStatus(t *testing.T) {
    tests := []struct {
        name string
        previous State
        checks []CheckResult
        want State
        passed, total int
    }{
        {"not started", "", checks(false, false), StateNotStarted, 0, 4},
        {"in progress", "", checks(true, false), StateInProgress, 1, 4},
        {"complete", "", checks(true, true), StateComplete, 4, 4},
        {"degraded", StateComplete, checks(true, false), StateDegraded, 1, 4},
        {"blocked wins", StateComplete, blockedChecks(), StateBlocked, 1, 4},
    }
}
```

Add milestone and project aggregation cases showing exact passed/total integer
weights and one-decimal percentages. Verify optional failures reduce percentage
without preventing `complete`.

- [ ] **Step 2: Write deterministic report tests**

Construct deliberately unsorted input. Assert Markdown ordering by milestone,
spec ID, and check ID; forward-slash paths; blocker and downstream sections;
next action selection; dirty marker; and absence of command durations. Render
twice with different generation time, commit, and dirty values but the same
source fingerprint and assert the normalized committed body is identical.
Unmarshal JSON and assert `schemaVersion == 1`, exact integer
weights, durations, and dependency arrays.

- [ ] **Step 3: Run progress/report tests and verify RED**

Run: `go test ./internal/projectstatus -run 'Test(Calculate|BuildStatus|Render)'`

Expected: build failure because progress and render functions do not exist.

- [ ] **Step 4: Implement status calculation**

Define states and report types with integer `PassedWeight` and `TotalWeight`.
Evaluate package states first, then milestones in dependency order, then project
state. A dependency-incomplete required check becomes blocked. A previous
complete state becomes degraded only when no current explicit blocker has
higher precedence.

- [ ] **Step 5: Implement Markdown and JSON rendering**

Build a stable view model sorted before rendering. Markdown contains header,
metadata, project summary, milestone table, package table, blockers, failed
required checks, downstream impact, and next action. JSON includes schema
version, generated time, exact weights, durations, stdout/stderr evidence
summaries, dependencies, and dirty state. Use `json.MarshalIndent` with a final
newline.

- [ ] **Step 6: Run report tests**

Run: `go test ./internal/projectstatus -run 'Test(Calculate|BuildStatus|Render)'`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/projectstatus/progress.go internal/projectstatus/progress_test.go internal/projectstatus/report.go internal/projectstatus/report_test.go
git commit -m "feat(projectstatus): calculate and render project progress"
```

### Task 5: Project Status CLI

**Files:**
- Create: `cmd/projectstatus/main.go`
- Create: `cmd/projectstatus/main_test.go`

**Interfaces:**
- Consumes: all `internal/projectstatus` public functions.
- Produces: default terminal output, `-write`, `-format json`, and `-check` CLI modes with exit codes 0/1/2.

- [ ] **Step 1: Write failing CLI tests**

Extract `run(ctx context.Context, args []string, stdout, stderr io.Writer) int`.
Use a temporary fixture repository and injected evaluation function to test:

- default mode writes Markdown only to stdout;
- JSON mode writes valid JSON only to stdout;
- `-write` atomically creates `docs/project/status.md`;
- `-check` returns 0 for matching normalized content and 1 for stale content;
- `-write` and `-check` together return 2;
- unknown format and flags return 2;
- valid incomplete status returns 1 after printing the report;
- schema/catalog failure returns 2 and does not overwrite the report.

- [ ] **Step 2: Run CLI tests and verify RED**

Run: `go test ./cmd/projectstatus`

Expected: build failure because the command does not exist.

- [ ] **Step 3: Implement orchestration and atomic writes**

Parse flags with a private `flag.FlagSet`. Locate the root, load and validate the
catalog, discover packages, evaluate checks, read prior generated state if
present, collect `git rev-parse HEAD` and `git status --porcelain` through
registered commands, calculate the source-content fingerprint, build status,
and render the requested format. The fingerprint hashes stable relative paths
and contents while excluding `docs/project/status.md`, `.git`, frontend
dependency/build directories, and transient Go build outputs.

For writes, create a temporary file in `docs/project`, write and sync it, close
it, then use `os.Rename` to replace `status.md`. Remove the temporary file on
every error. `-check` compares normalized Markdown with the existing file and
prints a concise regeneration command when stale.

- [ ] **Step 4: Run CLI and projectstatus tests**

Run: `go test ./cmd/projectstatus ./internal/projectstatus`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/projectstatus internal/projectstatus
git commit -m "feat(projectstatus): add project status command"
```

### Task 6: Author Every Package and Subsystem Specification

**Files:**
- Create: `docs/project/README.md`
- Create: `docs/project/milestones.md`
- Create: all 17 files under `docs/project/packages/`
- Create: all 4 files under `docs/project/subsystems/`
- Test: `internal/projectstatus/catalog_integration_test.go`

**Interfaces:**
- Consumes: spec schema and check types from Tasks 1–5.
- Produces: the complete authoritative project catalog.

- [ ] **Step 1: Write the real-catalog integration test**

```go
func TestRepositoryCatalogCoversEveryPackage(t *testing.T) {
    root := repositoryRootForTest(t)
    catalog, err := LoadCatalog(root)
    if err != nil { t.Fatal(err) }
    packages, err := DiscoverGoPackages(context.Background(), root, NewRunner(root))
    if err != nil { t.Fatal(err) }
    if err := ValidateCatalog(catalog, packages); err != nil { t.Fatal(err) }
    if got, want := len(catalog.Specs), 21; got != want {
        t.Fatalf("specs = %d, want %d", got, want)
    }
}
```

- [ ] **Step 2: Run the integration test and verify RED**

Run: `go test ./internal/projectstatus -run TestRepositoryCatalogCoversEveryPackage`

Expected: FAIL because `docs/project/packages` and `subsystems` do not exist.

- [ ] **Step 3: Write stable project and milestone indexes**

`docs/project/README.md` must explain the control/data paths, documentation
authority, generated status, commands, state meanings, and links to M0–M7.
`milestones.md` must reproduce the exact owners, dependencies, and completion
definitions from the approved design.

- [ ] **Step 4: Write package specs with this exact responsibility map**

Each file includes all 14 required sections and check metadata tailored to the
package:

| Spec | Milestone | Core responsibility | Current gap evidence |
|---|---|---|---|
| `root` | M7 | Wails entry point and process composition | product services not composed |
| `cmd-paramgen` | M1 | parameter generator CLI | verify deterministic generation |
| `internal-application` | M6 | lifecycle and service wiring | starts only OSC |
| `internal-ipc` | M2 | authenticated framed host transport | client/server files empty |
| `internal-osc` | M6 | OSC/OSCQuery discovery, binding, encoding, transport | no evaluated tracking source |
| `internal-parameters` | M1 | generated parameter identity and semantics | require generation parity |
| `internal-paramgen` | M1 | deterministic Go model generation | require golden/id stability tests |
| `internal-plugins` | M2 | plugin discovery, process lifecycle, health | unresolved `LogEntry`, incomplete runtime |
| `internal-processing` | M4 | canonical calibration/filter/dropout pipeline | configuration types only |
| `internal-specparser` | M1 | strict YAML parameter schema parser | maintain validation coverage |
| `internal-tracking` | M3 | ingest validation, routing, merge | empty merged frame/service implementation |
| `pkg-pluginapi` | M1 | stable vendor-plugin author API | runtime integration incomplete |
| `pkg-pluginruntime` | M2 | plugin-side commands and selective frame publishing | `Main` and delivery incomplete |
| `pkg-protocol` | M1 | versioned IPC wire contract | `Conn` empty, message coverage incomplete |
| `pkg-trackingmodel` | M1 | shared canonical tracking frame contract | compatibility/serialization tests needed |

Use command checks for existing build/test behavior, file/symbol checks for
required interfaces, placeholder checks for known skeletons, generated-clean
checks for parameter generation, and dependency-complete checks only where a
package cannot meet its product role independently.

- [ ] **Step 5: Write subsystem specs with this exact responsibility map**

| Spec | Milestone | Responsibility | Required checks |
|---|---|---|---|
| `frontend` | M7 | Svelte UI and Wails bindings | `npm run check`, `npm run build`, required status views |
| `parameter-spec` | M1 | authoritative VRCFT YAML definitions | parser validation and generated-clean |
| `build-release` | M7 | platform assets and reproducible artifacts | Wails build configuration and platform assets |
| `end-to-end` | M6 | avatar change through selective IPC to OSC | aggregate M2–M6 checks and integration test symbol |

- [ ] **Step 6: Run schema, coverage, and catalog tests**

Run: `go test ./internal/projectstatus -run 'TestRepositoryCatalog|TestParseSpec'`

Expected: PASS with exactly 21 initial specs and no unknown dependencies.

- [ ] **Step 7: Commit**

```bash
git add docs/project internal/projectstatus/catalog_integration_test.go
git commit -m "docs: specify every project package and subsystem"
```

### Task 7: Generate the Baseline and Integrate the Workflow

**Files:**
- Modify: `README.md`
- Create: `docs/project/status.md`
- Modify: `internal/projectstatus/report_test.go`

**Interfaces:**
- Consumes: complete real catalog and CLI.
- Produces: committed baseline status and discoverable update workflow.

- [ ] **Step 1: Add a failing stale-report integration test**

Run the CLI's `-check` mode against a copied repository fixture containing an
intentionally stale status file and assert exit 1 plus this exact guidance:

```text
project status is stale; run: go run ./cmd/projectstatus -write
```

- [ ] **Step 2: Run the stale-report test and verify RED**

Run: `go test ./cmd/projectstatus -run TestCheckReportsStaleBaseline`

Expected: FAIL until the committed-report normalization handles source commit,
generation time, and dirty state consistently.

- [ ] **Step 3: Make committed comparison deterministic**

Compare the stored source-content fingerprint and deterministic results. Ignore
generation timestamp, source commit, command duration, and dirty marker for
freshness. Ensure source/test/spec changes alter the fingerprint, changing only
`docs/project/status.md` does not, and `-write` records forward-slash paths with
a final newline.

- [ ] **Step 4: Add README entry points**

Add a concise “Project specifications and status” section linking
`docs/project/README.md`, `docs/project/milestones.md`, and
`docs/project/status.md`, plus the four CLI commands. Do not duplicate package
responsibilities in the root README.

- [ ] **Step 5: Generate the first real status report**

Run: `go run ./cmd/projectstatus -write`

Expected: exit 1 because the report validly records incomplete/blocked product
checks, while still writing `docs/project/status.md`. Confirm the report includes
the existing `internal/plugins` build failure and unfinished IPC/tracking/
processing/application checks.

- [ ] **Step 6: Run final focused verification**

Run: `go test -race ./internal/projectstatus ./cmd/projectstatus`

Expected: PASS.

Run: `go vet ./internal/projectstatus ./cmd/projectstatus`

Expected: PASS.

Run: `go run ./cmd/projectstatus -format json`

Expected: valid JSON with `schemaVersion: 1`; process exit is 1 while required
project checks remain incomplete.

Run: `git diff --check`

Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add README.md docs/project/status.md internal/projectstatus/report_test.go cmd/projectstatus/main_test.go
git commit -m "docs: publish generated project status baseline"
```
