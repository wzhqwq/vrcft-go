# Project Status

- Generated: 2026-08-15T17:50:36Z
- Commit: `8ce07a9e17ac2d6c77f2a09c20c39ad9f7f7252d`
- Source fingerprint: `a9b92338dbf6706fd2f62fc34be8f1a6f99f6639ad5614ef4cf91e6fdc178e32`
- Dirty: `false`
- State: `blocked`
- Progress: 88.1% (133/151 weight)

## Milestones

| Milestone | State | Progress |
|---|---|---:|
| M0 | complete | 100.0% (10/10) |
| M1 | complete | 100.0% (47/47) |
| M2 | complete | 100.0% (42/42) |
| M3 | complete | 100.0% (7/7) |
| M4 | complete | 100.0% (10/10) |
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
| M4 | internal-evaluator | internal/evaluator | complete | 100.0% (5/5) |
| M4 | internal-processing | internal/processing | complete | 100.0% (5/5) |
| M6 | end-to-end | docs/project | blocked | 0.0% (0/8) |
| M6 | internal-application | internal/application | in_progress | 40.0% (2/5) |
| M6 | internal-osc | internal/osc | complete | 100.0% (7/7) |
| M7 | build-release | build | complete | 100.0% (5/5) |
| M7 | frontend | frontend | not_started | 0.0% (0/7) |
| M7 | root | . | complete | 100.0% (3/3) |

## Failed Required Checks

- `end-to-end/integration-test` (failed): open C:\Users\wzhqwq\Documents\vrcft-go\internal\application\app_test.go: The system cannot find the file specified.
- `end-to-end/pipeline-components` (blocked): aggregate member incomplete: internal-application:tracking-wired
- `internal-application/tracking-wired` (failed): required symbol not found
- `frontend/production-build` (failed): node:internal/modules/cjs/loader:1228 throw err; ^ Error: Cannot find module 'C:\Users\wzhqwq\Documents\vrcft-go\frontend\node_modules\vite\bin\vite.js' at Function._resolveFilename (node:internal/modules/cjs/loader:1225:15) at Function._lo…
- `frontend/project-status-view` (failed): required symbol not found
- `frontend/type-check` (failed): node:internal/modules/cjs/loader:1228 throw err; ^ Error: Cannot find module 'C:\Users\wzhqwq\Documents\vrcft-go\frontend\node_modules\svelte-check\bin\svelte-check' at Function._resolveFilename (node:internal/modules/cjs/loader:1225:15) at…

## Next Action

Address `end-to-end/integration-test`: open C:\Users\wzhqwq\Documents\vrcft-go\internal\application\app_test.go: The system cannot find the file specified.
