# Completion-driven Chat Agent runtime

## Goal

Replace Neo Chat's arbitrary five-minute whole-Agent failure boundary with a
completion-driven Tool loop: healthy runs continue while they make progress,
finish only when the model stops using Tools and required completion evidence
is satisfied, and stop safely on user cancellation, a real system failure, or
a detected no-progress condition.

## What I already know

- The user explicitly selected Pi-style completion-driven execution.
- Long tasks currently fail with `LOCAL_SKILL_BUDGET_EXHAUSTED` when the parent
  Tool-loop Context reaches the default five-minute local Run timeout.
- A recent XLSX task created and verified its file just before the deadline but
  was cancelled before `publish_file`, demonstrating that elapsed time is not a
  valid completion signal.
- Neo Chat already persists Agent turns/events and already has completion
  verification and Goal blocked/completed concepts.
- Pi has no generic whole-Agent timeout or generic round cap; its HTTP and idle
  timeouts are scoped to connections and inactive sessions.

## Requirements

- A local/MCP Tool runtime timeout must not be installed as the parent deadline
  of the entire Provider Agent loop.
- A healthy Agent run must continue across arbitrary elapsed wall time while it
  is making observable progress.
- The normal completion boundary is: the assistant returns no Tool Calls and
  the existing completion policy reports no outstanding mutation or background
  Job verification.
- Foreground Tool calls retain their configured per-call timeout. Background
  Jobs retain their own lifecycle, polling, cancellation, and process-local
  durability disclosure.
- Provider HTTP idle/error behavior, explicit user cancellation, approval
  denial/expiry, and Backend shutdown remain terminal boundaries.
- Repeated identical Tool outcomes or repeated all-error rounds must be treated
  as no progress. After the bounded repetition threshold, the model receives a
  final no-Tools instruction to explain the concrete blocker without claiming
  success.
- Absolute round/call limits must no longer convert a healthy progressing run
  into `LOCAL_SKILL_BUDGET_EXHAUSTED`. Existing limits may remain only as scoped
  anti-stall windows or compatibility limits outside completion-driven Agent
  execution.
- State-changing work continues to use the existing evidence-gated completion
  contract. A requested downloadable artifact is not successful until it is
  published through `publish_file` and represented in the final response.
- Tool failures remain Tool Results whenever their outcome is known, allowing
  the model to repair and retry. Only fatal/unknown outcomes end the run.
- Agent events, Process Trace, total duration, Tool results, and final outcome
  remain persisted and reconnectable after browser refresh or conversation
  switching.
- The existing Host Runner permission and approval model must not be widened to
  Pi's direct unrestricted host Shell behavior.

## Acceptance Criteria

- [x] A focused integration test runs past a deliberately short legacy
      `RunTimeout`, continues making progress, and completes successfully.
- [x] A run that generates, verifies, and publishes an artifact after that
      legacy boundary completes with a downloadable attachment.
- [x] Three repeated identical Tool outcome rounds enter the blocked wrap-up
      path instead of looping forever or reporting a wall-clock timeout.
- [x] Repeated Tool failures with a later successful alternative are returned
      to the model and the run can still complete.
- [x] User cancellation still cancels the active Provider request and running
      process group and finalizes the message/turn as cancelled.
- [x] A genuinely timed-out individual foreground command remains a failed
      Tool Result and does not masquerade as verified work.
- [x] Outstanding mutations/background Jobs still cannot be reported complete
      without valid completion evidence.
- [x] Refreshing or switching conversations does not cancel the active run and
      persisted duration/event projection remains correct.
- [x] Focused Go tests, `go vet ./...`, frontend tests if UI metadata changes,
      documentation checks, and the relevant standalone verification gate pass.

## Definition of Done

- Backend loop, timeout ownership, no-progress guard, outcome projection, and
  regression tests are implemented.
- Frontend is changed only if a distinct blocked presentation is required by
  the final Backend contract; otherwise the final blocked explanation remains
  an ordinary completed assistant response with explicit Agent outcome
  metadata.
- Runtime/deployment/contracts documentation reflects completion-driven
  behavior and scoped timeouts.
- Configuration compatibility and rollback are documented.
- A focused conventional commit is created after verification; nothing is
  pushed.

## Technical Approach

- Remove the Handler-level minimum Tool runtime deadline from the Provider loop;
  parent it only to cancellation/shutdown.
- Keep timeout enforcement inside the owning Tool backend.
- Replace absolute Tool-loop termination with a progress tracker that observes
  canonical Tool call/result batches. It blocks after three identical outcomes
  or a bounded sequence of all-error rounds, while successful novel outcomes
  reset the stall window.
- Reuse `chatCompletionPolicy` and its verification-only Tool registry to gate
  mutation completion and artifact publication.
- Emit/persist a typed Agent outcome for no-progress blocking without mapping it
  to a Provider/system failure.
- Keep a defensive internal ceiling only if it cannot terminate a healthy run;
  otherwise remove the 32-Step/128-call absolute admission boundary from this
  execution path.

## Decision (ADR-lite)

**Context**: Whole-run wall time is not correlated with task completion. Pi
demonstrates a simpler completion-driven loop, while Neo Chat has stronger
durability, security, and evidence contracts worth preserving.

**Decision**: Adopt completion-driven loop semantics with scoped Tool/network
timeouts, explicit cancellation, evidence-gated completion, and no-progress
blocking. Do not copy Pi's direct Shell authority or process-only persistence.

**Consequences**: Healthy tasks can run longer and consume more Tokens; the
runtime therefore needs observable duration/cost metrics and a deterministic
stall guard. Backend restart still interrupts the in-flight provider stream,
but persisted conversation and Agent events remain available for a subsequent
continuation.

## Out of Scope

- Durable replay of an in-flight LLM stream after Backend process restart.
- A distributed Agent worker/lease queue.
- Removing per-command, HTTP idle, approval, or user-cancellation boundaries.
- Expanding filesystem permissions or bypassing Host Runner policy.
- Multi-Agent orchestration changes unrelated to one conversation run.

## Research References

- [`research/pi-runtime-comparison.md`](research/pi-runtime-comparison.md) — Pi
  uses a completion-driven loop; Neo Chat should copy the semantics while
  retaining database durability and permission boundaries.

## Technical Notes

- Primary files: `backend/internal/chat/handler.go`,
  `backend/internal/chat/web_tool_loop.go`,
  `backend/internal/chat/chat_agent_turn.go`,
  `backend/internal/chat/chat_completion_policy.go`, and focused tests.
- Configuration defaults currently define local Tool `RunTimeout=5m`,
  `MaxCalls=32`, and `MaxRounds=8`; preflight allows at most 30m, 128 calls,
  and 32 rounds.
- The current Handler chooses the minimum MCP/local Run timeout and applies it
  to the complete stream Context. This ownership is the primary defect.
