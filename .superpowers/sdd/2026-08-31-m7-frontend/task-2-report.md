# Task 2 Report: Theme, Copy, and Problem Mapping

## Scope

- Replaced the frontend stylesheet with Tailwind v4's approved `@import "tailwindcss"` entrypoint, dark semantic tokens, shared semantic classes, reduced-motion handling, and `100dvh` sizing/overflow rules.
- Added a centralized Simplified Chinese copy module for navigation, actions, settings success messaging, and stable problem presentation strings.
- Added a typed `presentProblem()` mapper that exhaustively handles the known backend problem codes and preserves a stable fallback for unknown codes.
- Added focused Vitest coverage for all supported problem-code presentations.

## RED/GREEN Evidence

### Cycle 1: problem presentation mapping

RED:

```text
$ pnpm.cmd --dir frontend test -- src/lib/presentation/problem.test.ts
FAIL  src/lib/presentation/problem.test.ts [ src/lib/presentation/problem.test.ts ]
Error: Failed to resolve import "./problem" from "src/lib/presentation/problem.test.ts". Does the file exist?
Test Files  1 failed | 2 passed (3)
```

GREEN:

```text
$ pnpm.cmd --dir frontend test -- src/lib/presentation/problem.test.ts
RUN  v4.1.11 C:/Users/wzhqwq/Documents/vrcft-go/frontend
Test Files  3 passed (3)
Tests  10 passed (10)
```

## Final Verification

```text
$ pnpm.cmd --dir frontend test -- src/lib/presentation/problem.test.ts
RUN  v4.1.11 C:/Users/wzhqwq/Documents/vrcft-go/frontend
Test Files  3 passed (3)
Tests  10 passed (10)
```

```text
$ pnpm.cmd --dir frontend check
svelte-check found 0 errors and 0 warnings
```

```text
$ pnpm.cmd --dir frontend build
vite v8.1.5 building client environment for production...
✓ built in 287ms
```

```text
$ git diff --check
exit 0
notes: git reported only a CRLF normalization warning for frontend/src/style.css; no diff-check whitespace errors
```

## Self-Review

- `presentProblem()` is frontend-only and typed against `ProblemWire`, so Task 1 Wails ports and bindings remain unchanged.
- Conflict details prefer `currentRevision` when present and otherwise fall back to bounded backend text, which matches the stable UI-treatment requirement without inventing extra backend contracts.
- The accepted Vite 8 experimental support warning still appears during Vitest, `svelte-check`, and `vite build`; it remains baseline noise from the approved toolchain.
