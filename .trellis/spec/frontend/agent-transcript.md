# Agent Transcript Frontend Contract

## 1. Scope / Trigger

Apply when changing `ChatAgentEvent`, server SSE normalization, live Message
projection, `AgentTranscript`, `ProcessTracePanel`, Context/Think rows, or typed
Tool presenters.

## 2. Signatures

```ts
type ChatAgentEventType =
  | "context.injected"
  | "assistant.chunk"
  | "assistant.block.completed"
  | /* existing Turn/Step/Tool events */ string;

projectAgentTranscript(events: unknown): AgentTranscriptNode[];

type AgentTranscriptNode = ContextNode | ReasoningNode | ToolNode;
```

`assistant.chunk` accepts `block-start` or `reasoning-delta`,
`blockType="reasoning"`, a positive `blockIndex`, optional positive Provider
round through `stepSequence`, and bounded delta content.

New migration-101 Turns carry
`turn.started.payload.transcriptVersion = 2`. The marker admits Tool-only v2
transcripts; the three v2-only Context/assistant event types also admit a
transcript when a partial replay lacks the start event.

## 3. Contracts

- Normalize every untrusted event before projection. Deduplicate by `eventId`
  and order by durable inner `sequence`; the outer SSE sequence is only the
  reconnect cursor.
- Project one flat sequence: Context and each reasoning block are independent
  expandable rows; the first Tool event fixes a Tool row's position and later
  call/result events update it in place.
- `Context injection` shows a bounded source label and backend-sanitized text.
  `Think` shows only Provider-returned content and keeps separate blocks across
  Tool boundaries. Never merge `message.reasoning` into Transcript v2.
- Reuse typed Tool presenters from `ProcessTracePanel`; do not introduce raw
  argument/result rendering. `ProcessTracePanel` remains the renderer only when
  authoritative Transcript v2 nodes are absent.
- Do not admit a Tool-only event history without the exact numeric
  `transcriptVersion: 2` start marker. Pre-101 Tool events belong to the legacy
  renderer and must not hide `message.reasoning` or fabricate a v2 history.
- Keep `agentEvents` on the live Message draft and on DTO reload mapping so
  streaming, reconnect, terminal replacement, and reload run the same projector.
- Context/Reasoning limits are 64 KiB per event and 1 MiB reasoning per Turn.
  Unknown sources, orphan deltas, unsupported block types, and malformed values
  fail closed.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| duplicate/replayed `eventId` | ignore duplicate; preserve first durable fact |
| reasoning delta without block start | drop the orphan delta |
| Tool called then Tool result | retain first position; replace status/presentation in place |
| Turn ends with open Think block | render it as interrupted, not completed |
| Tool-only Turn with numeric v2 start marker | render the flat typed Tool row |
| Tool-only Turn without v2 start marker | return no v2 nodes and retain the legacy renderer |
| no v2 node exists | use legacy `ProcessTracePanel` and legacy Reasoning behavior |
| unknown context source/block type or oversized text | drop or bound it; never expose raw payload |

## 5. Good / Base / Bad Cases

- **Good:** `Context -> Think #1 -> Terminal -> Think #2 -> final answer` renders
  in the same order during streaming and after reload.
- **Base:** a Provider returns no reasoning; typed Tool rows and the final answer
  render without a fake Think row.
- **Bad:** show one `已调用 N 次工具` accordion, append all reasoning at its
  bottom, or infer reasoning from Tool arguments/results.

## 6. Tests Required

- Pure projection: context, two reasoning blocks, Tool call/result replacement,
  durable ordering, interrupted block, marker-gated Tool-only compatibility,
  malformed/orphan fail-closed behavior.
- Component SSR/DOM: independent expandable rows, typed Terminal details,
  localized labels, no aggregate wrapper, and no hostile raw fields.
- DTO/store/SSE: retain `agentEvents`, deduplicate reconnect events, and converge
  terminal Message/reload with the same order.
- Run frontend format, lint, typecheck, focused Vitest, and production build.

## 7. Wrong vs Correct

```text
Wrong: message.reasoning + grouped ProcessTracePanel + Tool count summary
Correct: normalized durable events -> flat AgentTranscript nodes -> typed rows
```
