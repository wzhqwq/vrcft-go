# Task 7 report: Steam Link real-process acceptance

## RED / GREEN evidence

- RED: the first focused test compilation failed because `EyeValid` is a bitmask without a `Has` method. After correcting the test to use the public bitmask operation, the first real-process run exposed two contract mismatches: processing represents completed dropout as a neutral valid channel, and a datagram accepted before a rebind can be delivered while the transition is still being consumed.
- GREEN: the acceptance test now treats neutral value and inactive group as the dropout contract, confirms the new receiver before probing the retired port, and drains to a bounded quiet window before testing codec rejection. It uses repeated UDP sends and condition waits; it contains no startup sleep.
- Review follow-up: generation-8 observation drains late generation-7 sink entries, as required by the manager's asynchronous control boundary, then asserts that the first new-generation frame contains only the new selected fields. Codec rejection now proves that the same process remains Running and accepts a later valid packet.

## Commands and results

- `GOCACHE=F:\dev\vrcft-go\.go-gocache go test ./internal/plugins -run '^TestSteamLink' -count=1` — passed (9.736s) with normal Windows permissions.
- `GOCACHE=F:\dev\vrcft-go\.go-gocache go test -race ./internal/plugins ./pkg/pluginruntime ./internal/steamlink -count=1 -v` — passed (all three packages).
- `GOCACHE=F:\dev\vrcft-go\.go-gocache go test ./...` — failed only in pre-existing `internal/projectstatus`: `TestParseSpecRejectsInvalidMetadata/duplicate_check` expected `ErrInvalidSpec`, received nil. The changed plugin, runtime, and Steam Link packages passed in that run.

The sandbox itself cannot resolve Windows temporary catalog roots: the existing `TestDirectoryCatalogSortsAndLabelsSources` also fails there with `plugin catalog root cannot be resolved`. The focused and race checks were rerun with normal authorized permissions and passed.

## Files

- `internal/plugins/steamlink_integration_test.go`: Windows-only built-command, catalog, JSON store, manager, named-pipe, UDP, subscription, lifecycle, dropout, and resource-release acceptance tests.
- `docs/project/packages/internal-steamlink.md`: records the executable acceptance evidence and leaves only hardware validation as a known gap.

## Commit

`test(steamlink): exercise managed process and UDP tracking flow`.

## Self-review

- Uses the real Steam Link executable, `DirectoryCatalog`, `JSONStore`, `NewProcessLauncher`, `Manager`, and `FrameSink`; no generic helper process is substituted.
- Dynamically reserves nonzero loopback ports with bounded retries and never binds port 9015.
- Verifies plugin ID/generation, exact JawOpen value, field trimming, new generation, pause/resume, rebind, malformed codec input, disconnect, disable, UDP/named-pipe/process cleanup, and tracking-to-processing dropout without pipeline reset.
- Review findings were addressed: stale old-generation sink entries are drained during the asynchronous generation transition, while the accepted new-generation frame proves the driver reset its cache and trims to new fields. Malformed input must leave the same process running and recover to a valid frame.
- The command build disables VCS stamping because `go build` cannot obtain VCS status in this disposable linked worktree. It otherwise runs the requested real command build and requires the inherited absolute `GOCACHE` to exist.

## Concerns

- Full `go test ./...` remains red because of the unrelated project-status parser test noted above.
- Hardware validation is intentionally outside this offline acceptance scope.
