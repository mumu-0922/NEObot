# Interleave Agent Narration And Tool Timeline

## Goal

Render an Agent turn as one durable chronological transcript so concise
user-visible progress narration appears beside the Tool work it explains,
instead of collecting all Tools above one detached text block. Streaming,
reconnect, terminal replacement, and fresh reload must converge to the same
order.

## What I Already Know

- The user explicitly confirmed the desired order is `narration -> Tool call ->
  Tool result -> stage conclusion -> next narration -> next Tool`, followed by
  only a concise final answer.
- Raw hidden chain-of-thought must not be exposed. Existing Provider reasoning
  remains a distinct collapsible `Think` block.
- Backend `chat_agent_events.sequence` is already the durable ordering
  authority and the frontend already projects Context, Reasoning, and Tool rows
  from it.
- The current projector ignores ordinary assistant text. `MessageOutputRenderer`
  renders the entire `message.content` after `AgentTranscript`, which causes the
  detached layout.
- The Tool loop already distinguishes rounds that contain Tool Calls from a
  terminal no-Tool answer, but currently drops some buffered intermediate
  narration and streams other intermediate text through the final-message
  channel.
- Pi preserves Text, Thinking, and ToolCall as ordered assistant content blocks
  and renders them in array order. See
  [`research/pi-ordered-content-blocks.md`](research/pi-ordered-content-blocks.md).

## Requirements

- A Provider text response from a round that also produces Tool Calls is
  process narration, not final-answer content.
- Process narration must be represented by a bounded, sanitized durable Agent
  event block and rendered as an always-visible Markdown row at its first
  sequence position.
- The terminal Provider round with no Tool Calls remains authoritative
  `message.content` and renders once after the process transcript.
- Reasoning, narration, Tool call, Tool result, Context injection, and final
  answer must preserve their real event order. A later Tool result updates the
  Tool row fixed by the first Tool call rather than moving it.
- Streaming, reconnect replay, terminal message replacement, and fresh message
  reload must use the same event projector and converge byte-for-byte in node
  order.
- Agent-mode prompting should request brief user-facing progress narration
  before meaningful Tool work and a brief stage conclusion when useful, while
  prohibiting raw Tool-output repetition and fabricated results.
- If a Provider emits no narration, the UI must not invent factual prose; the
  typed Tool row remains the fallback evidence.
- Existing pre-v2 and v2 histories without narration events must retain their
  current renderer and final answer.
- Cancellation, interruption, malformed payloads, oversized chunks, duplicate
  reconnect events, and orphan deltas must fail closed without hiding the final
  answer.

## Acceptance Criteria

- [x] A fixture sequence `Think #1 -> narration A -> Tool A call -> Tool A
      result -> Think #2 -> narration B -> Tool B call -> Tool B result -> final`
      projects and renders in that exact order.
- [x] Narration is visible inline rather than behind the `Think` disclosure.
- [x] Final answer text renders exactly once after the last process node and
      excludes narration from Tool-bearing rounds.
- [x] Tool-result updates retain the Tool call's original position.
- [x] Live SSE, reconnect replay, terminal replacement, and reload produce the
      same transcript order.
- [x] Goal/completion-driven rounds no longer discard intermediate narration.
- [x] A no-Tool ordinary answer remains unchanged.
- [x] Malformed/unknown narration blocks and orphan deltas are dropped, bounded
      chunks are enforced, and legacy histories remain readable.
- [x] Focused Go and Vitest coverage, frontend format/lint/typecheck/build, and
      backend vet/tests pass.

## Definition Of Done

- Backend Provider-round classification, Agent-event recording, normalization,
  replay, and final-message persistence are covered by tests.
- Frontend types, normalizer, transcript projector, renderer, DTO/store flow,
  and refresh parity are covered by tests.
- Agent transcript and Tool-loop specs document the new narration authority.
- Existing deployment rollback boundary remains the current Agent timeline
  feature path; no data migration is required.

## Technical Approach

1. Buffer user-visible text for each Tool-capable Provider round until the
   round outcome is known.
2. If the round contains Tool Calls, emit the buffered text as a typed
   `ProviderEventNarrationDelta`; otherwise replay it through the existing
   final `ProviderEventDelta` path.
3. Persist narration with ordered `assistant.chunk` block events using a new
   supported `blockType`, close it before the matching Tool event, and emit the
   same durable events over SSE.
4. Extend the frontend normalized transcript union with a narration node and
   render it inline in the same ordered list. Keep final `message.content`
   outside and after the transcript.
5. Add an idempotent Agent progress instruction that asks models for concise
   user-visible updates without exposing hidden reasoning.

## Decision (ADR-lite)

**Context:** Frontend-only reordering cannot recover which text belongs to
which Tool round, and heuristic sentence splitting would diverge after reload.

**Decision:** Extend the existing durable event-block protocol with explicit
narration blocks and classify text at the Backend Tool-round boundary, following
Pi's ordered-content-block principle while retaining Neo Chat's immutable event
log.

**Consequences:** Tool-bearing round text may wait until that Provider round is
complete before display, but it is then correctly classified, persisted, and
replayed. No schema migration is needed because event payloads are JSONB and
event sequencing already exists.

## Expansion Sweep

- Future evolution: the ordered block protocol can later admit images or
  approval prompts without introducing another parallel renderer.
- Related scenarios: Search, Knowledge, MCP, local Skill, Goal, and Workspace
  Tools must share the same narration semantics.
- Failure edges: reconnect duplicates, interrupted open blocks, Providers that
  return text plus invalid Tool Calls, and terminal failures must preserve all
  admitted narration without treating it as a success claim.

## Out Of Scope

- Exposing private chain-of-thought or generating hidden reasoning summaries.
- Synthesizing factual narration when the Provider supplied none.
- Rewriting historical messages to fabricate narration events.
- Redesigning Tool cards, Context rows, or the overall message visual language.
- Adding a database migration or a second transcript store.

## Technical Notes

- Backend owners:
  `backend/internal/chat/web_tool_loop.go`, `provider.go`,
  `chat_agent_event_recorder.go`, `chat_agent_events.go`, and `handler.go`.
- Frontend owners:
  `frontend/src/lib/chat/agentTranscript.ts`,
  `frontend/src/components/content/AgentTranscript.tsx`, stream/store mappings,
  and transcript tests.
- Relevant specs:
  `.trellis/spec/backend/chat-tool-loop.md` and
  `.trellis/spec/frontend/agent-transcript.md`.
