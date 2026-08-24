# Pi runtime comparison

## Scope

Compare the checked-out Pi Web runtime with Neo Chat's current Chat Agent Tool
loop to choose a completion-driven execution model without copying Pi's weaker
durability and authority boundaries.

## Pi Web / Pi Agent findings

- The inspected checkout is `@agegr/pi-web` 0.8.9 with
  `@earendil-works/pi-agent-core` and `pi-coding-agent` 0.84.2.
- Pi Agent Core loops while the assistant continues producing Tool Calls. The
  generic loop has no whole-run wall-clock timeout and no fixed round cap.
- A single `AbortController` cancels the current model request and propagates to
  Tool execution. User abort and process shutdown remain terminal boundaries.
- Pi's Bash Tool has no default timeout. A timeout is applied only when the
  model supplies the optional `timeout` argument.
- Pi's five-minute HTTP value is an Undici headers/body *idle* timeout, not a
  whole Agent-run deadline.
- Pi Web keeps one in-process `AgentSessionWrapper` per session. Its ten-minute
  timer destroys only idle sessions; an active session reschedules the timer.
- The POST request acknowledges a Prompt after preflight. The Agent continues
  in the server process and the browser observes it through SSE.
- Conversation history is appended to JSONL, but active execution lives in the
  Node process and is aborted by server restart.
- Tool exceptions become error Tool Results so the model can recover in the
  next turn instead of treating every command failure as a run failure.

## Neo Chat findings

- Neo Chat already owns the stronger persistence model: PostgreSQL-backed Chat
  Agent turns/events, durable message finalization, cancellation tracking, and
  SSE reattachment.
- `handler.go` currently takes the minimum MCP/local `RunTimeout` and wraps the
  entire Provider Tool loop in `context.WithTimeout`. The local default is five
  minutes, so a healthy multi-step run is cancelled even when it is still
  making progress.
- Local Tools already distinguish foreground command timeout, background jobs,
  Tool-result failures, approval waiting, user cancellation, and fatal runtime
  errors.
- The Tool loop already has completion evidence gating: successful mutations
  cannot be claimed complete until a later verification Tool result is
  presented through `verify_completion`.
- Goal mode already persists active/paused/blocked/completed state and blocks
  premature completion when evidence is outstanding.
- Absolute limits still exist at three layers: local/MCP configured rounds and
  calls, the 32-Step/128-call turn driver, and the parent wall-clock deadline.

## Feasible approaches

### A. Raise the existing limits

- Increase the whole-run timeout, calls, and rounds.
- Low implementation cost, but a long valid task still fails at an arbitrary
  boundary and generated artifacts can still miss final publication.

### B. Completion-driven loop with scoped safety fuses (selected)

- Do not apply Tool `RunTimeout` to the whole Provider loop.
- Continue while Tool outcomes show progress and stop naturally when the model
  returns no Tool Calls and completion evidence is satisfied.
- Preserve per-Tool deadlines, provider HTTP idle timeout, user cancellation,
  approval boundaries, and process shutdown.
- Detect repeated identical outcomes and repeated all-error rounds, then force
  a no-Tools blocked wrap-up rather than reporting a timeout failure.
- Persist the outcome in the existing Agent event/message metadata path.
- Reuse the completion policy so requested artifacts must be verified and
  published before success can be claimed.

### C. Durable external Agent worker

- Move each run into a leased background worker with resumable checkpoints.
- Best restart durability, but requires a new queue/lease/replay protocol and
  is substantially broader than the Pi-style behavior requested here.

## Decision

Use Approach B. Keep Neo Chat's database authority and Host Runner isolation;
copy Pi's completion-driven loop semantics, not its direct host Shell or
process-only session storage. Automatic replay of an in-flight provider stream
after Backend restart remains future work.

