# Project Status

- Generated: 2026-08-02T16:30:01Z
- Commit: `0b7e3fe22870a1012cc47954f6ce3da1e6f29e3b`
- Source fingerprint: `d384a6bc9f946824e81d9a078540d1112e50ed2fe6824dab32f65886d87c5b7c`
- Dirty: `false`
- State: `blocked`
- Progress: 84.9% (124/146 weight)

## Milestones

| Milestone | State | Progress |
|---|---|---:|
| M0 | complete | 100.0% (10/10) |
| M1 | complete | 100.0% (47/47) |
| M2 | complete | 100.0% (42/42) |
| M3 | complete | 100.0% (7/7) |
| M4 | in_progress | 20.0% (1/5) |
| M5 | not_started | 0.0% (0/0) |
| M6 | blocked | 45.0% (9/20) |
| M7 | in_progress | 53.3% (8/15) |

## Packages and Subsystems

| Milestone | Spec | Path | State | Progress |
|---|---|---|---|---:|
| M0 | cmd-projectstatus | cmd/projectstatus | complete | 100.0% (4/4) |
| M0 | internal-projectstatus | internal/projectstatus | complete | 100.0% (6/6) |
| M1 | cmd-paramgen | cmd/paramgen | complete | 100.0% (3/3) |
| M1 | internal-parameterdeps | internal/parameterdeps | complete | 100.0% (5/5) |
| M1 | internal-parameters | internal/parameters | complete | 100.0% (5/5) |
| M1 | internal-paramgen | internal/paramgen | complete | 100.0% (4/4) |
| M1 | internal-specparser | internal/specparser | complete | 100.0% (4/4) |
| M1 | parameter-spec | spec | complete | 100.0% (6/6) |
| M1 | pkg-pluginapi | pkg/pluginapi | complete | 100.0% (7/7) |
| M1 | pkg-protocol | pkg/protocol | complete | 100.0% (6/6) |
| M1 | pkg-trackingmodel | pkg/trackingmodel | complete | 100.0% (7/7) |
| M2 | internal-ipc | internal/ipc | complete | 100.0% (16/16) |
| M2 | internal-plugins | internal/plugins | complete | 100.0% (18/18) |
| M2 | pkg-pluginruntime | pkg/pluginruntime | complete | 100.0% (8/8) |
| M3 | internal-tracking | internal/tracking | complete | 100.0% (7/7) |
| M4 | internal-processing | internal/processing | in_progress | 20.0% (1/5) |
| M6 | end-to-end | docs/project | blocked | 0.0% (0/8) |
| M6 | internal-application | internal/application | in_progress | 40.0% (2/5) |
| M6 | internal-osc | internal/osc | complete | 100.0% (7/7) |
| M7 | build-release | build | complete | 100.0% (5/5) |
| M7 | frontend | frontend | not_started | 0.0% (0/7) |
| M7 | root | . | complete | 100.0% (3/3) |

## Failed Required Checks

- `internal-processing/pipeline-implemented` (failed): required symbol not found
- `end-to-end/integration-test` (failed): open F:\dev\vrcft-go\internal\application\app_test.go: The system cannot find the file specified.
- `end-to-end/pipeline-components` (blocked): aggregate member incomplete: internal-application:tracking-wired
- `internal-application/tracking-wired` (failed): required symbol not found
- `frontend/production-build` (failed): node:fs:2787 const stats = binding.lstat(base, true, undefined, true /* throwIfNoEntry */); ^ Error: EPERM: operation not permitted, lstat 'C:\Users\wzhqwq' at Object.realpathSync (node:fs:2787:29) at toRealPath (node:internal/modules/helpe…
- `frontend/project-status-view` (failed): required symbol not found
- `frontend/type-check` (failed): node:fs:2787 const stats = binding.lstat(base, true, undefined, true /* throwIfNoEntry */); ^ Error: EPERM: operation not permitted, lstat 'C:\Users\wzhqwq' at Object.realpathSync (node:fs:2787:29) at toRealPath (node:internal/modules/helpe…

## Next Action

Address `internal-processing/pipeline-implemented`: required symbol not found
