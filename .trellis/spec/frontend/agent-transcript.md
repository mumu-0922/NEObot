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

type AgentTranscriptNode = ContextNode | ReasoningNode | NarrationNode | ToolNode;

type NarrationChunk = {
  type: "assistant.chunk";
  payload: {
    chunkType: "block-start" | "narration-delta";
    blockType: "narration";
    blockIndex: number;
    content?: string;
  };
};
```

`assistant.chunk` accepts `block-start`, `reasoning-delta`, or
`narration-delta`. Its `blockType` is exactly `reasoning|narration`, its delta
kind must match that block type, and it carries a positive `blockIndex`, an
optional positive Provider round through `stepSequence`, and bounded content.

New migration-101 Turns carry
`turn.started.payload.transcriptVersion = 2`. The marker admits Tool-only v2
transcripts; the three v2-only Context/assistant event types also admit a
transcript when a partial replay lacks the start event.

## 3. Contracts

- Normalize every untrusted event before projection. Deduplicate by `eventId`
  and order by durable inner `sequence`; the outer SSE sequence is only the
  reconnect cursor.
- Project one flat sequence: Context and each reasoning block are independent
  expandable rows; narration is an always-visible Markdown row; the first Tool
  event fixes a Tool row's position and later call/result events update it in
  place. The terminal `message.content` remains outside and after this process
  transcript, and must never duplicate narration.
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
- Preserve `agentEvents` through every intermediate Message conversion,
  including `mapChatMessageDtoToMessage` and `toStoreMessageFromServer`.
  `message.completed` replaces the streaming draft with the terminal snapshot;
  omitting the field at either hop makes details disappear at completion and
  again after reload even though durable storage and SSE are correct.
- Context/Reasoning/Narration limits are 64 KiB per event and 1 MiB per
  Reasoning or Narration stream per Turn. Unknown sources, orphan deltas,
  mismatched delta kinds, unsupported block types, and malformed values fail
  closed.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| duplicate/replayed `eventId` | ignore duplicate; preserve first durable fact |
| reasoning delta without block start | drop the orphan delta |
| narration delta without block start or with a mismatched block type | drop the orphan/malformed delta |
| Tool called then Tool result | retain first position; replace status/presentation in place |
| Turn ends with open Think or Narration block | render it as interrupted, not completed |
| Tool-only Turn with numeric v2 start marker | render the flat typed Tool row |
| Tool-only Turn without v2 start marker | return no v2 nodes and retain the legacy renderer |
| no v2 node exists | use legacy `ProcessTracePanel` and legacy Reasoning behavior |
| unknown context source/block type or oversized text | drop or bound it; never expose raw payload |

## 5. Good / Base / Bad Cases

- **Good:** `Context -> Think #1 -> narration -> Terminal -> stage conclusion ->
  Think #2 -> final answer` renders in the same order during streaming and
  after reload.
- **Base:** a Provider returns no reasoning; typed Tool rows and the final answer
  render without a fake Think row.
- **Bad:** show one `已调用 N 次工具` accordion, append all reasoning at its
  bottom, or infer reasoning from Tool arguments/results.

## 6. Tests Required

- Pure projection: context, two reasoning blocks, interleaved narration, Tool
  call/result replacement, durable ordering, interrupted blocks, marker-gated
  Tool-only compatibility, and malformed/orphan fail-closed behavior.
- Component SSR/DOM: independent expandable rows, typed Terminal details,
  localized labels, no aggregate wrapper, and no hostile raw fields.
- DTO/store/SSE: retain `agentEvents` through the terminal Message replacement
  and a fresh list/reload, deduplicate reconnect events, and converge both paths
  with the same order.
- Run frontend format, lint, typecheck, focused Vitest, and production build.

## 7. Wrong vs Correct

```text
Wrong: grouped Tools first + detached process prose + final answer
Correct: normalized durable events -> interleaved Context/Think/Narration/Tool rows -> final answer
```

## 8. Safe answer continuation

- Show `Continue answer` only for a server Assistant with exact
  `PROVIDER_STREAM_INTERRUPTED`, non-empty partial content, and an available
  continuation callback. Keep Regenerate visible as the separate full-rerun
  action.
- Continue sends the source Assistant ID as `continuationOfMessageId` through
  the existing typed stream client and generation state machine. Do not create
  a visible synthetic User message or mutate the source branch locally.
- The Backend-created sibling becomes the active version. Its first streamed
  delta is the preserved prefix; terminal replacement and reload must retain
  the combined content and `continuationOfMessageId` metadata.
- Disable duplicate submission through the existing active-generation guard.
  A second interruption displays Continue on the new longer partial sibling.
- Localized copy must state that Continue finishes prose without rerunning
  completed Tools; Regenerate may rerun all execution steps.

Required focused coverage: request serialization, store sibling projection,
exact eligibility/copy in every locale, coexistence with Regenerate, and
terminal replacement parity.
