# Planned chat Tool Loop contracts

Status: G19.2 process-trace foundation promoted on 2026-07-22; Tool execution,
three-state Search, and Knowledge Tool sections remain planned. Apply each
section only after its owning G19 group is promoted. Until G19.3/G19.6, the
existing `chat-source-fusion.md` execution contract remains authoritative.

## 1. Scope / Trigger

Use this spec when changing provider streaming to expose function tools,
executing multi-round Tool Calls, adding Web/Knowledge tools, persisting process
steps, or changing conversation Search authority.

## 2. Signatures

Target conversation Search state:

```text
searchMode = off | model_builtin | external
legacy useSearch=false -> off
legacy useSearch=true  -> external
```

Target provider round events:

```text
content.delta | reasoning.delta | tool.call.delta |
tool.call.completed | usage.updated | round.completed | round.error
```

Target process steps reuse the chat run/message sequence and contain a stable
step ID, `reasoning|knowledge|web|tool|generation` kind,
`pending|running|awaiting_approval|completed|failed|skipped|cancelled` status,
timings, label key, and sanitized details.

Active G19.2 SSE signatures:

```text
reasoning.delta = wrapper + delta:string
process.step.updated = wrapper + step:ProcessStep
wrapper = runId + conversationId + messageId + sequence + createdAt
```

The same monotonically increasing `sequence` covers all chat SSE event types.
G19.2 singleton IDs are `<messageId>:<kind>:1`; later Tool work must add stable
per-call identity rather than reusing one Tool step for multiple calls.

## 3. Contracts

- `off` means zero Search planning, resolver, built-in, and external I/O.
- Built-in and external Search are mutually exclusive and never fall back to
  one another.
- Native Tools are sent on the initial current-model request with automatic
  selection. Explicit current/Search intent must use the selected Search mode.
- Tool Calls are accumulated and validated before execution, then returned in
  the provider's native continuation format to the same model.
- Do not add product-level Tool Round or Tool Call count limits for the current
  single-user deployment. Cancellation, request context, provider timeout,
  terminal errors, and approval rejection remain exit conditions.
- A Tool-unsupported current model may use the same model for a bounded
  compatibility `shouldSearch + query` plan; never use a hidden model.
- Persist only rendered provider reasoning and sanitized steps. Credentials,
  raw payloads, system prompts, full source bodies, and internal errors remain
  forbidden.
- Read-only Web/Knowledge tools run automatically. Side effects require an
  approval policy before registration.
- Only current-turn backend-issued markers used by the final reconciled answer
  become Citations; unused results stay in the process trace.
- G19.2 persists sanitized `reasoning` and `processTrace` in terminal assistant
  metadata. A successful Generation-only answer omits both fields; failed or
  cancelled Generation remains durable.
- Process detail uses an allowlist and bounded values. Unknown fields, raw
  payloads, source bodies, headers, prompts, SQL, and internal errors are
  dropped before SSE and persistence.
- Live reasoning redaction must retain a bounded un-emitted suffix across
  provider chunks. Sanitizing each chunk independently is forbidden because a
  split `apiKey=`/value or `Bearer` token can bypass the regex boundary.
- Until G19.3/G19.6, legacy external Web and Knowledge work finishes before SSE
  creation and is projected as a terminal step. Do not describe it as live Tool
  execution. Provider reasoning/Generation transitions are streamed after the
  provider channel is established.

## 4. Validation & Error Matrix

| Condition                        | Required behavior                                |
| -------------------------------- | ------------------------------------------------ |
| Search off                       | zero Search I/O                                  |
| Native Tool unsupported          | same-model compatibility plan                    |
| Tool arguments malformed/unknown | do not execute; redacted failed step             |
| External Search failure          | truthful degradation; ordinary answer; no `[W]`  |
| Built-in unsupported             | disabled/degraded; no external fallback          |
| Knowledge miss                   | successful empty result; continue                |
| Approval rejected                | do not execute; continue or terminate truthfully |
| Cancel during Provider/Tool      | cancel both; one terminal cancelled event        |
| Provider exposes no reasoning    | process only; no fabricated reasoning            |
| Successful Generation only       | omit empty durable process panel                  |
| Unknown process detail key       | drop before SSE/persistence                       |

## 5. Good / Base / Bad Cases

- Good: globe external, ordinary writing request, no Tool Call and no Search
  provider request.
- Good: contextual explicit Search generates one standalone Query, shows Tool
  progress, continues the same model, and keeps only used `[W]` markers.
- Good: selected Knowledge can run before Web and preserve distinct `[K]`/`[W]`
  authority.
- Base: a Tool-unsupported model uses the visible compatibility path and still
  answers if planning fails.
- Bad: pre-searching every enabled turn, running built-in and external Search
  together, hiding a provider switch, fabricating reasoning, or rendering all
  retrieved sources as Citations.

## 6. Tests Required

1. Fragmented Tool arguments and native continuation fixtures for each
   provider family.
2. Search off/Auto skip/explicit Search I/O assertions.
3. Ordered reasoning/process SSE, terminal persistence, reload, redaction, and
   cancellation.
4. Capability mismatch and compatibility-planner tests with no hidden model.
5. Knowledge hit/miss/deletion plus mixed Knowledge/Web marker truth.
6. Real selected provider/Search smoke with temporary state deleted.
7. G19.2 reload mapping, manual expand/collapse authority, and no-empty-panel
   fixtures across backend and frontend.

## 7. Wrong vs Correct

Wrong:

```text
search enabled -> rewrite -> always Search -> answer
```

Correct after the owning G19 promotion:

```text
search mode + selected Knowledge + capabilities
  -> expose allowed tools to current model
  -> no Tool Call: answer
  -> Tool Call: validate/execute/trace -> native continuation
  -> reconcile only current-turn used citations -> persist
```

Full target contract: `mm-chat/docs/contracts/chat-tool-loop.md`.
