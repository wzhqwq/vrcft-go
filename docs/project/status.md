# Project Status

- Generated: 2026-07-23T18:14:11Z
- Commit: `1a7c15735c91cb44bf8c560831a55c4edba4ba88`
- Source fingerprint: `ce5a251114f1661ae71beaae0560cfdb820357a6ccc8aecdaa0f2a7302e8f1c8`
- Dirty: `false`
- State: `blocked`
- Progress: 78.2% (104/133 weight)

## Milestones

| Milestone | State | Progress |
|---|---|---:|
| M0 | complete | 100.0% (10/10) |
| M1 | complete | 100.0% (47/47) |
| M2 | in_progress | 89.7% (26/29) |
| M3 | in_progress | 14.3% (1/7) |
| M4 | in_progress | 20.0% (1/5) |
| M5 | not_started | 0.0% (0/0) |
| M6 | blocked | 45.0% (9/20) |
| M7 | in_progress | 66.7% (10/15) |

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
| M2 | internal-plugins | internal/plugins | in_progress | 40.0% (2/5) |
| M2 | pkg-pluginruntime | pkg/pluginruntime | complete | 100.0% (8/8) |
| M3 | internal-tracking | internal/tracking | in_progress | 14.3% (1/7) |
| M4 | internal-processing | internal/processing | in_progress | 20.0% (1/5) |
| M6 | end-to-end | docs/project | blocked | 0.0% (0/8) |
| M6 | internal-application | internal/application | in_progress | 40.0% (2/5) |
| M6 | internal-osc | internal/osc | complete | 100.0% (7/7) |
| M7 | build-release | build | complete | 100.0% (5/5) |
| M7 | frontend | frontend | in_progress | 28.6% (2/7) |
| M7 | root | . | complete | 100.0% (3/3) |

## Failed Required Checks

- `internal-plugins/runtime-loop` (failed): required symbol not found
- `internal-tracking/merged-frame-implemented` (failed): matched (?s)type MergedFrame struct\s*\{\s*\}
- `internal-tracking/service-implemented` (failed): required symbol not found
- `internal-processing/pipeline-implemented` (failed): required symbol not found
- `end-to-end/integration-test` (failed): open F:\dev\vrcft-go\internal\application\app_test.go: The system cannot find the file specified.
- `end-to-end/pipeline-components` (blocked): aggregate member incomplete: internal-application:tracking-wired
- `internal-application/tracking-wired` (failed): required symbol not found
- `frontend/project-status-view` (failed): required symbol not found
- `frontend/type-check` (failed): 02:14:08 [vite-plugin-svelte] !!! Support for vite 8 beta in vite-plugin-svelte is experimental (rolldown: 1.1.5, vite: 8.1.5) !!! See https://github.com/sveltejs/vite-plugin-svelte/issues/1143 for a list of known issues and to report feedb…

## Next Action

Address `internal-plugins/runtime-loop`: required symbol not found
