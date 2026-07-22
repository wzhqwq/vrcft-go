# Project Status

- Generated: 2026-07-22T04:16:05Z
- Commit: `8b12266a760fd1f946067c02e9fd8b9c3450b40e`
- Source fingerprint: `4442b00b76b86ee695806fc73daa5998c62bfb831f31ad713d82775a99c311af`
- Dirty: `false`
- State: `blocked`
- Progress: 52.6% (60/114 weight)

## Milestones

| Milestone | State | Progress |
|---|---|---:|
| M0 | complete | 100.0% (10/10) |
| M1 | in_progress | 81.6% (31/38) |
| M2 | blocked | 10.5% (2/19) |
| M3 | in_progress | 14.3% (1/7) |
| M4 | in_progress | 20.0% (1/5) |
| M5 | not_started | 0.0% (0/0) |
| M6 | blocked | 35.0% (7/20) |
| M7 | in_progress | 53.3% (8/15) |

## Packages and Subsystems

| Milestone | Spec | Path | State | Progress |
|---|---|---|---|---:|
| M0 | cmd-projectstatus | cmd/projectstatus | complete | 100.0% (4/4) |
| M0 | internal-projectstatus | internal/projectstatus | complete | 100.0% (6/6) |
| M1 | cmd-paramgen | cmd/paramgen | complete | 100.0% (3/3) |
| M1 | internal-parameters | internal/parameters | complete | 100.0% (5/5) |
| M1 | internal-paramgen | internal/paramgen | complete | 100.0% (4/4) |
| M1 | internal-specparser | internal/specparser | complete | 100.0% (4/4) |
| M1 | parameter-spec | spec | complete | 100.0% (6/6) |
| M1 | pkg-pluginapi | pkg/pluginapi | complete | 100.0% (4/4) |
| M1 | pkg-protocol | pkg/protocol | in_progress | 16.7% (1/6) |
| M1 | pkg-trackingmodel | pkg/trackingmodel | in_progress | 66.7% (4/6) |
| M2 | internal-ipc | internal/ipc | in_progress | 14.3% (1/7) |
| M2 | internal-plugins | internal/plugins | blocked | 0.0% (0/5) |
| M2 | pkg-pluginruntime | pkg/pluginruntime | in_progress | 14.3% (1/7) |
| M3 | internal-tracking | internal/tracking | in_progress | 14.3% (1/7) |
| M4 | internal-processing | internal/processing | in_progress | 20.0% (1/5) |
| M6 | end-to-end | docs/project | blocked | 0.0% (0/8) |
| M6 | internal-application | internal/application | blocked | 0.0% (0/5) |
| M6 | internal-osc | internal/osc | complete | 100.0% (7/7) |
| M7 | build-release | build | complete | 100.0% (5/5) |
| M7 | frontend | frontend | not_started | 0.0% (0/7) |
| M7 | root | . | complete | 100.0% (3/3) |

## Failed Required Checks

- `pkg-protocol/connection-implemented` (failed): matched (?s)type Conn interface\s*\{\s*// TODO\s*\}
- `pkg-protocol/subscription-message` (failed): required symbol not found
- `pkg-trackingmodel/compatibility-tests` (failed): GetFileAttributesEx F:\dev\vrcft-go\pkg\trackingmodel\frame_test.go: The system cannot find the file specified.
- `internal-ipc/client-implemented` (failed): matched (?s)^package ipc\s*$
- `internal-ipc/server-implemented` (failed): matched (?s)^package ipc\s*$
- `internal-plugins/package-builds` (failed): # github.com/wzhqwq/vrcft-go/internal/plugins internal\plugins\manager.go:49:12: undefined: LogEntry
- `internal-plugins/runtime-loop` (failed): required symbol not found
- `pkg-pluginruntime/frame-delivery` (failed): required symbol not found
- `pkg-pluginruntime/main-implemented` (failed): matched (?s)func Main\([^)]*\)\s*\{\s*\}
- `internal-tracking/merged-frame-implemented` (failed): matched (?s)type MergedFrame struct\s*\{\s*\}
- `internal-tracking/service-implemented` (failed): required symbol not found
- `internal-processing/pipeline-implemented` (failed): required symbol not found
- `end-to-end/integration-test` (failed): open F:\dev\vrcft-go\internal\application\app_test.go: The system cannot find the file specified.
- `end-to-end/pipeline-components` (blocked): aggregate member incomplete: internal-application:tracking-wired
- `internal-application/package-builds` (failed): # github.com/wzhqwq/vrcft-go/internal/plugins internal\plugins\manager.go:49:12: undefined: LogEntry
- `internal-application/tracking-wired` (failed): required symbol not found
- `frontend/production-build` (failed): node:fs:2787 const stats = binding.lstat(base, true, undefined, true /* throwIfNoEntry */); ^ Error: EPERM: operation not permitted, lstat 'C:\Users\wzhqwq' at Object.realpathSync (node:fs:2787:29) at toRealPath (node:internal/modules/helpe…
- `frontend/project-status-view` (failed): required symbol not found
- `frontend/type-check` (failed): node:fs:2787 const stats = binding.lstat(base, true, undefined, true /* throwIfNoEntry */); ^ Error: EPERM: operation not permitted, lstat 'C:\Users\wzhqwq' at Object.realpathSync (node:fs:2787:29) at toRealPath (node:internal/modules/helpe…

## Next Action

Address `pkg-protocol/connection-implemented`: matched (?s)type Conn interface\s*\{\s*// TODO\s*\}
