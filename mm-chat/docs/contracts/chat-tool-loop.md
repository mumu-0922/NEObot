# Chat Tool Loop and Process Trace Contract

## 1. Scope

G19 replaces pre-answer forced Search with a server-owned, provider-normalized
Tool Loop. It applies to chat generation, provider continuation, Search mode,
selected Knowledge retrieval, live process visibility, persisted reasoning and
trace data, tool approval, cancellation, and source reconciliation.

The first admitted tools are read-only:

```text
search_web(query)
search_knowledge(query)
```

The generic runtime may admit more tools later only after assigning an explicit
risk class and approval policy.

## 2. Search mode

The authoritative conversation configuration is:

```ts
type SearchMode = "off" | "model_builtin" | "external";
```

Compatibility during migration:

```text
legacy useSearch=false -> off
legacy useSearch=true  -> external
```

Rules:

- `off` performs no Search planning, Search resolver lookup, model-built-in
  Search request, or external Search request.
- `model_builtin` and `external` are strictly mutually exclusive in state,
  outbound provider payloads, process events, and persisted source artifacts.
- Enabled Search is Auto unless the user explicitly requests Search/current
  verification, which forces Search within the selected mode.
- The mode is saved immediately per conversation. New conversations inherit
  the user's latest selected mode.
- An unavailable mode remains visible but disabled with a redacted reason.
- A configured mode never silently falls back to another Search provider or
  mode.

## 3. Provider-normalized Tool Loop

Conceptual round input:

```go
type ProviderRoundRequest struct {
    ProviderRequest
    Tools        []ToolDefinition
    ToolChoice   string
    Continuation []ProviderToolExchange
}

type ProviderToolExchange struct {
    AssistantContent   string
    AssistantReasoning string
    Calls              []ProviderToolCall
    Results            []ProviderToolResult
    ProviderState      any
}
```

Conceptual normalized provider events:

```text
content.delta
reasoning.delta
tool.call.delta
tool.call.completed
usage.updated
provider.search.sources
round.completed
round.error
```

Loop behavior:

```text
build active conversation context
  -> expose tools allowed by mode, selection, capability, and risk
  -> stream one provider round
     -> no Tool Call: finalize answer
     -> Tool Call: validate -> approve if required -> execute
        -> append provider-native assistant Tool Call + Tool Result
        -> continue the same model in another round
```

Provider adapters must preserve their native continuation form:

- OpenAI Chat Completions: assistant `tool_calls` followed by `role=tool` with
  the matching `tool_call_id`.
- OpenAI Responses: `function_call` followed by `function_call_output` with the
  matching call ID.
- Anthropic: assistant `tool_use` followed by user `tool_result`, retaining any
  required Thinking Block/signature.
- Gemini external function tools currently use Google's documented OpenAI-
  compatible endpoint and the same `tool_calls`/`role=tool` continuation.
  Gemini model-built-in Search uses native `streamGenerateContent` with
  `google_search`; a future native generic Tool adapter must preserve
  `functionCall`/`functionResponse`, thought signatures, and part ordering.

Streaming Tool arguments may arrive in fragments. Go must accumulate them by
provider call identity, reject malformed/oversized/unknown arguments, and never
execute a Tool before the provider has completed the call.

`round.completed` may carry an in-memory provider-private continuation state.
Anthropic uses it to preserve the exact ordered assistant `thinking`,
`redacted_thinking`, `text`, and `tool_use` blocks plus required signatures.
That state is used only to build the next provider request; it is never sent in
SSE, placed in process details, or persisted in message metadata. Anthropic
failure results set `tool_result.is_error=true`. Tool input remains capped at
64 KiB.

Anthropic extended Thinking does not use a forced named `tool_choice`. An
explicit Search turn is buffered with `auto`; if Claude returns no Tool Call,
the existing same-model compatibility path enforces the explicit Search
contract without exposing the discarded answer. Without Thinking, a forced
Search names `search_web` directly.

Usage events are cumulative across native rounds. Each round reports its own
provider usage to the loop; the SSE-visible update adds all completed prior
rounds exactly once.

There is no product-level maximum Tool Round count, total Tool Call count, or
per-tool count for this single-user deployment. The loop terminates only when:

- the model returns no Tool Call;
- the user cancels the run;
- the request context or configured provider timeout ends;
- a Tool/Provider returns a terminal non-degradable error; or
- an approval is rejected.

The existing run cancellation must cancel the active provider request and any
in-flight Tool request. A cancelled loop emits exactly one terminal
`message.cancelled` and cannot later finalize as completed.

## 4. Tool capability and compatibility fallback

- A tool-capable current model receives native Tool definitions on the initial
  chat request with automatic tool selection.
- Explicit Search intent must force `search_web` or native Search within the
  selected mode even when automatic selection would skip it.
- When the current model/provider cannot perform function calling, external
  mode may call a compatibility planner through the same selected model:

```json
{ "shouldSearch": true, "query": "one standalone query" }
```

- The planner consumes bounded active-branch context, treats it as untrusted,
  returns no answer, and never switches model/provider.
- Planner failure degrades to an ordinary answer with a truthful unavailable
  notice; it does not search a raw ambiguous phrase or change provider.
- Official built-in Search is admitted only through an explicit provider/model
  capability. A custom OpenAI-compatible model requires administrator opt-in
  and a successful bounded real capability test.

Built-in Search authority is provider/model exact:

- official OpenAI uses Responses Web Search, Gemini uses native Google Search,
  and Anthropic uses `web_search_20250305`;
- image, audio, realtime, embedding, transcription, and TTS model families are
  never admitted as chat Search capability;
- custom OpenAI-compatible providers may opt into `openai_responses` only and
  must name one model from the provider's persisted model list;
- `POST /v1/admin/providers/{providerId}/built-in-search-test` performs a real,
  bounded request and attests only when the provider returns at least one
  Search source; and
- the custom attestation fingerprint binds provider ID/type, normalized Base
  URL, encrypted secret reference, protocol, and exact model. Changing any
  bound field invalidates the attestation before runtime use.

The external resolver and model-built-in resolver are separate authority
paths. Neither scans or returns the other mode, and a capability failure
degrades without a cross-mode fallback.

## 5. Tool registry and approvals

Every registered Tool has server-owned metadata:

```text
name + version + JSON schema + risk class + executor + redaction policy
```

Risk classes:

```text
read_only       -> automatic
write_local     -> approval required
external_effect -> approval required
destructive     -> stronger approval or disabled
```

Approval UI must show a human-readable Tool name, target, redacted action
summary, and `allow once | allow for this conversation | reject`. Credentials,
raw payloads, and hidden Tool parameters are never rendered. G19's initial Web
and Knowledge tools are read-only and require no approval.

## 6. Process trace and reasoning

Public process event shape:

```ts
type ProcessStepKind =
  "reasoning" | "knowledge" | "web" | "tool" | "generation";

type ProcessStepStatus =
  | "pending"
  | "running"
  | "awaiting_approval"
  | "completed"
  | "failed"
  | "skipped"
  | "cancelled";

interface ProcessStep {
  id: string;
  kind: ProcessStepKind;
  status: ProcessStepStatus;
  labelKey: string;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  detail?: Record<string, unknown>;
}
```

The exact SSE wrapper reuses the existing `runId`, `conversationId`,
`messageId`, monotonically increasing `sequence`, and `createdAt` fields.

G19.2 activates these two SSE events:

```text
event: reasoning.delta
data: { type, runId, conversationId, messageId, sequence, createdAt, delta }

event: process.step.updated
data: { type, runId, conversationId, messageId, sequence, createdAt, step }
```

`message.started` remains the first stream event. Every reasoning, process,
content, usage, Search, and terminal event increments the same stream-local
`sequence`. Singleton steps use stable `<messageId>:<kind>:1` IDs. G19.3 keeps
those IDs and assigns every external Tool/Web execution the next stable
`<messageId>:tool|web:<n>` pair.

Terminal assistant metadata is:

```json
{
  "reasoning": "sanitized provider-returned text or summary",
  "processTrace": ["sanitized terminal ProcessStep objects"]
}
```

Both fields are omitted for an ordinary successful answer that has only a
Generation step and no provider reasoning. Failed and cancelled Generation
steps remain durable. Detail fields are allowlisted and bounded; unknown keys
are dropped before SSE/persistence. Provider reasoning is bounded to 1 MiB for
persistence and receives credential-pattern redaction.
Live reasoning keeps a bounded suffix before emission so a credential pattern
split across adjacent provider chunks is redacted before any complete secret
can reach the browser.

Rules:

- Provider-returned reasoning is streamed separately from factual process
  steps. When a provider exposes no reasoning, the UI may say "Analyzing" as a
  process status but must not fabricate reasoning text.
- Running generation auto-expands the process panel. Completion collapses it to
  a one-line duration/stage summary. Manual expansion is authoritative and must
  not force chat scroll-to-bottom.
- Ordinary answers with no reasoning, retrieval, or Tool activity render no
  empty completed panel.
- Persist rendered provider reasoning and sanitized Process Steps so reload and
  conversation switching reproduce the completed view.
- Allowed persisted details include displayed Search Query, redacted Tool
  arguments, hit/source counts, duration, provider/mode identifiers, failure
  category, and Citation mapping.
- Forbidden persisted/rendered details include credentials, authorization
  headers, ciphertext, raw provider events, complete Web/Knowledge bodies,
  system prompts, internal safety instructions, stack traces, SQL, and database
  topology.
- Reasoning effort, Search mode, and selected Knowledge are independent inputs.

G19.3 moves external Web retrieval into the live provider loop after the
assistant SSE starts. Every accepted `search_web` call produces its own running
and terminal Tool/Web steps. Provider-returned reasoning and Generation remain
live, and model-built-in Search is live once its provider stream is established.
G19.6B registers selected Knowledge in that live loop for Tool-round-capable
providers when Search is `off` or `external`. Each accepted call produces live
Tool/Knowledge steps and returns a bounded Tool Result. G19.6D removes the old
Handler pre-answer authority. Non-Tool providers and model-built-in Search now
run the same server-authoritative Knowledge executor after `message.started`,
then continue through the existing same-model external planner, built-in Search
stream, or ordinary answer path. This compatibility path is visibly traced and
never restores pre-SSE retrieval.

## 7. Web and Knowledge tools

### `search_web`

- Registered only in `external` mode with one active tested external provider.
- Receives a standalone Query created by the current model from bounded active
  conversation context.
- The external provider receives only the Query, never raw conversation
  history.
- Results are normalized, deduplicated, bounded, treated as untrusted, and
  assigned current-turn `[W#]` capabilities by Go.
- Tool-unsupported providers use the same selected model for one bounded JSON
  decision/query pass, then answer with the existing evidence prompt only when
  that pass requested Search.

### `search_knowledge`

- Registered only when the conversation has selected Knowledge collections.
- The model argument schema contains only `query`. Collection IDs are
  server-authoritative and copied from the authenticated conversation
  selection; a model cannot expand the selected set through Tool arguments.
- Reuses the active BM25/pgvector, Query Expansion, RRF, Jina reranker,
  authority hydration, deletion visibility, and no-evidence policy.
- Results receive current-turn `[K#]` capabilities only after the existing
  evidence gate.
- A normal miss is a successful empty Tool Result, not a user-visible error.

Knowledge and Web may run in either order. Current/public claims prefer Web;
internal/private claims prefer Knowledge; conflicts disclose scope/time and may
cite both.

## 8. Citation truth

- Retrieval is not citation use.
- Only current-turn markers issued by Go and present in the reconciled final
  answer render Citation cards.
- Historical, copied, invented, wrong-kind, out-of-range, and unused markers
  are removed before terminal SSE and persistence.
- Retrieved but unused sources remain available only in the process panel.
- A completed Search with no used marker records "final answer did not cite"
  in the trace and does not append a synthetic source list.
- Filtering preserves the marker minted against the original result list. If
  the answer uses only `[W2]`, its card and metadata remain `[W2]`; projection
  never renumbers it to `[W1]`.

## 9. Failure behavior

| Condition                             | Required result                                    |
| ------------------------------------- | -------------------------------------------------- |
| Search mode off                       | zero Search planner/resolver/provider I/O          |
| Auto decides no Search                | direct answer; no empty Search process/source card |
| Explicit Search intent                | Search in the selected mode                        |
| External Search unavailable/fails     | truthful notice; ordinary answer; no `[W#]`        |
| Built-in capability unavailable       | mode disabled or degraded; no external fallback    |
| Native Tool unsupported               | same-model compatibility planner                   |
| Compatibility planner fails           | ordinary answer with truthful unavailable notice   |
| Knowledge miss                        | empty successful result; continue Model/Web        |
| Tool arguments malformed/unknown      | reject execution; redacted failed step             |
| Write/external Tool awaiting approval | pause loop until allow/reject/cancel               |
| User cancels during Provider/Tool     | cancel both; one terminal cancelled event          |
| Provider reasoning unavailable        | process only; no fabricated reasoning              |
| Final answer uses no issued marker    | no Citation card                                   |

## 10. Required verification

1. Provider fixtures for fragmented Tool Calls and multi-round continuation on
   OpenAI-compatible/Gemini and Anthropic formats.
2. Zero Search I/O with mode off and ordinary Auto no-search.
3. Explicit/current Search plus contextual follow-up Query correctness.
4. Compatibility planning only for Tool-unsupported current models.
5. Strict built-in/external mutual exclusion and no provider fallback.
6. Ordered reasoning/process SSE, cancellation, redaction, terminal persistence,
   reload, collapsed summary, and manual-scroll behavior.
7. Knowledge hit/miss/deletion and Knowledge/Web chained evidence.
8. Current-turn Citation reconciliation for used, unused, copied, and invented
   markers.
9. Full Go vet/test/race, frontend format/lint/typecheck/test/build, Compose
   rebuild/restart/health, clean-copy, real-provider smoke, and complete smoke
   cleanup.
