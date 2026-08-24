# Fix Agent Terminal Contract and Status

## Goal

Make Agent Terminal calls advertise only executable foreground timeouts, report
non-zero shell exits as failures without hiding their output, and guide models
to use the deployed Host Workspace runtime correctly so recoverable command
mistakes do not consume the entire Tool-round budget.

## What I Already Know

- The affected production Turn first received usable Web sources, then exhausted
  later searches and attempted direct Host Workspace commands.
- `python` exited `127` because the Host Workspace provides `python3` but no
  `python` alias.
- The UI rendered that non-zero exit as successful because the Tool transport
  completed and Backend process status did not distinguish command failure.
- Three later `curl` calls requested `timeoutSeconds=40`; the public Terminal
  schema allowed up to the five-minute Run timeout while foreground execution
  rejected anything above the 30-second Call timeout.
- The Turn reached the default eight-round ceiling before it could create and
  publish the requested workbook.
- The user approved the recommended general Terminal repair with “修”.

## Requirements

- The Terminal Tool schema must use the foreground Call timeout as the maximum
  accepted explicit `timeoutSeconds` value, so model-produced arguments cannot
  pass schema admission and fail immediately at execution for contract drift.
- Background jobs must retain their existing bounded Run timeout when the
  caller omits an explicit timeout; this repair must not weaken cancellation,
  approval, workspace, output, or concurrency limits.
- A foreground command that exits non-zero must produce a failed/degraded Tool
  process step with the existing stable `nonzero_exit` category while preserving the
  bounded `exitCode`, stdout, stderr, timeout, truncation, duration, and Terminal
  presentation needed by the model and UI.
- Exit code zero remains successful. Timeout remains visibly failed and must
  not be mislabeled as transport success.
- Durable Agent events and live events must agree so the UI shows the same
  Terminal status before and after reload.
- Agent runtime instructions must say that the Host Workspace does not
  guarantee a `python` alias, direct Python commands should use `python3` (or
  discover an interpreter), and commands needing longer than the foreground
  limit must use background-job flow.
- Focused Backend and Frontend regression coverage must pin the contract and
  user-visible status projection.

## Acceptance Criteria

- [x] The advertised Terminal `timeoutSeconds.maximum` equals the configured
      foreground Call timeout.
- [x] An explicit timeout just above that limit is rejected by schema/contract
      before provider execution; values at the limit remain accepted.
- [x] A background command with omitted timeout still uses the configured Run
      timeout path.
- [x] `exit 0` produces a completed Tool step.
- [x] `exit 127` produces a failed Tool step with `nonzero_exit`, retains the
      exit code and stderr, and gives the model the bounded result payload.
- [x] A timed-out process is visibly failed with the existing `timeout` category.
- [x] Live and persisted transcript tests render non-zero Terminal exits as
      errors rather than “success”.
- [x] The runtime prompt mentions `python3` and the foreground/background
      timeout rule.
- [x] Focused Go and Vitest suites, Go vet, frontend format/lint/typecheck, and
      relevant builds pass.

## Definition of Done

- Tests cover bounds, zero/non-zero exits, timeout, and live/durable status.
- Lint, typecheck, focused tests, and proportional build checks are green.
- Executable Agent runtime/spec documentation records the shared timeout and
  exit-status contract.
- The deployed Backend/Frontend are rebuilt and healthy if verification is
  green.
- Changes are committed without pushing.

## Technical Approach

Use the existing `localskills.Config.CallTimeout` as the single foreground
timeout authority in both the Tool definition and executor. Keep
`RunTimeout` as the background-job default. Return the normal bounded Terminal
result to the provider continuation, but attach a typed failure category when
the shell exit is non-zero or timed out so process events become failed without
discarding diagnostic output. Extend the existing generated runtime
instruction rather than adding a parallel prompt path. Consume the resulting
Backend process status in the existing frontend transcript projection.

## Decision (ADR-lite)

**Context:** One Tool schema serves foreground and background commands, but an
explicit timeout was validated against the Run limit while foreground execution
used the lower Call limit.

**Decision:** Advertise the lower foreground Call limit for explicit Terminal
timeouts. Long work continues through `runInBackground=true`, where an omitted
timeout receives the existing Run limit. Treat process exit failure separately
from Tool transport completion and persist the process failure.

**Consequences:** Explicit custom background timeouts above the foreground limit
remain unavailable through this shared schema; background jobs still get the
full configured default. A future split Terminal/background schema can add that
control without reintroducing drift.

## Out of Scope

- Adding or purchasing a dedicated finance/history-data provider.
- Fabricating missing weekend or intraday commodity prices.
- Increasing the global eight-round Agent budget.
- Installing a global `python -> python3` alias on the user's Host.
- Rewriting historical Agent events that already mislabeled old command exits.

## Technical Notes

- Backend Tool definition: `mm-chat/backend/internal/chat/local_skill_runtime.go`
- Backend execution and timeout validation:
  `mm-chat/backend/internal/localskills/runtime.go`
- Backend Tool result/event mapping:
  `mm-chat/backend/internal/chat/local_skill_tool_loop.go`
- Frontend durable/live projection is governed by
  `.trellis/spec/frontend/agent-transcript.md`.
- Cross-layer producer/validator and live/durable replay rules are documented in
  `.trellis/spec/guides/cross-layer-thinking-guide.md`.
