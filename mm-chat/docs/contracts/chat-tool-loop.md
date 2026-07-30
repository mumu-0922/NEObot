# Chat Tool Loop and Process Trace Contract

## 1. Scope

G19 replaces pre-answer forced Search with a server-owned, provider-normalized
Tool Loop. It applies to chat generation, provider continuation, Search mode,
selected Knowledge/Memory retrieval, live process visibility, persisted
reasoning and trace data, tool approval, cancellation, and source
reconciliation.

The first admitted tools are read-only:

```text
search_web(query)
search_knowledge(query)
search_memory()  # default-off; first round only
```

The generic runtime may admit more tools later only after assigning an explicit
risk class and approval policy.

`search_memory` is implemented but not promoted: it is absent unless
`MEMORY_TOOL_LOOP_ENABLED=true`. The deployed default remains `false`: the
schema-v7 GPT and DeepSeek Flash Development profiles both failed unchanged
gates, and no Validation evidence exists.

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
rounds exactly once. A continuation-recovery answer stream inherits that same
completed-usage base, so its terminal update cannot move the visible count
backward.

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

Cancellation is not a degraded Tool result. Compatibility planning,
native/compatibility Web execution, and Knowledge execution must translate
`context.Canceled` or a cancelled operation context to `cancelled` Tool and
source steps, omit `failureCategory`, stop fallback/continuation, and retain
`detail.outcome=cancelled`. The terminal process event must still reach the
stream consumer after the operation context is cancelled; receiving it is
also Handler authority to finalize the assistant as cancelled. A provider
timeout that reports only `context.DeadlineExceeded` remains a provider
failure unless the run was separately cancelled.

## 4. Tool capability and compatibility fallback

- Capability resolution order is model override, provider default, unexpired
  shared probe cache, then `unknown`. Overrides use
  `auto|enabled|disabled`; ordinary users keep `auto`.
- Administrator provider responses expose
  `toolCapability.default` plus `toolCapability.modelOverrides`; updates send
  `toolCapabilityDefault` plus `toolCapabilityModelOverrides`. Per-model
  `Inherit` removes the map entry, and deselecting a model removes its override.
- A known tool-capable current model receives native Tool definitions on the
  initial chat request with automatic tool selection.
- Explicit Search intent must force `search_web` or native Search within the
  selected mode even when automatic selection would skip it.
- When selected Knowledge is in scope and capability is unsupported or unknown,
  the same selected model performs one bounded unified plan:

```json
{
  "route": "direct|knowledge|web|both",
  "knowledgeQuery": "standalone private query",
  "webQuery": "standalone public query"
}
```

- The planner consumes bounded active-branch context, treats it as untrusted,
  returns no answer, and never switches model/provider.
- Planner invalid JSON, oversize output, timeout, or provider failure falls back
  only on deterministic authority: strong catalog/private signal uses
  Knowledge, explicitly forced available Search uses Web, and all other turns
  answer Direct. Failure never defaults to Both.
- An unknown current turn uses Planner immediately and starts one background
  singleflight synthetic probe. Provider save/activation also prewarms the first
  model and matching task models. Neither chat nor provider-save waits for a
  probe/cache write.
- The probe contains a fixed fictional Tool and fixed prompt only. It never
  includes user text, conversation, catalog, source bodies, raw provider
  payloads, or credentials. A valid matching completed Tool Call records
  `supported`; explicit Tool incompatibility records `unsupported`; timeout,
  cancellation, 429, 5xx, transport/ordinary 400, and inconclusive output stay
  `unknown`.
- Probe state is shared in
  `model_tool_capability_cache(provider_config_hash, model_id)` with seven-day
  supported, 24-hour unsupported, and five-minute unknown TTLs. The config hash
  binds provider identity/configuration and secret reference hash without
  storing a credential.
- Explicit first-round Tool incompatibility writes an asynchronous downgrade
  and enters Planner in the same turn. Transient provider failures must not
  masquerade as capability failure.
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

### Native continuation recovery

Every successful Web execution returns only sources newly added during that
execution. Earlier Tool Results already remain in the provider-native
continuation, so repeating all cumulative Web source bodies in every later
result is forbidden. Markers remain cumulative and stable: a later result may
contain `[W2]` without repeating or renumbering `[W1]`. A repeat-only Search
returns `sources: []` and instructs the model to reuse prior Tool Results.

If a later native continuation fails synchronously or in-stream after bounded
Web or authorized Knowledge evidence exists, Go may perform one recovery answer
stream with these exact constraints:

- use the same provider instance, `modelRef`, conversation context, and
  cumulative usage base;
- disable Tools for the recovery stream and inject only the bounded cumulative
  Knowledge/Web evidence through the existing answer-context builders;
- preserve backend-issued markers and the normal final-answer Citation
  reconciliation; and
- run only before any answer content was emitted.

Cancellation, no-evidence failures, and failures after partial answer text do
not recover. They retain their existing terminal behavior. Provider reasoning
alone does not count as answer content and may precede a recovery answer.

The recovery answer is server-buffered until its provider stream closes
successfully. A failed first attempt is discarded in full and retried once
through the exact same provider/model/request. Neither partial content,
reasoning, nor usage from the failed attempt reaches SSE or persistence. The
successful attempt is then emitted with the prior native-round usage base. If
both attempts fail, emit the final error with zero recovery answer content.
Cancellation never retries. Recovery is constrained to 1 MiB/8,192 events and
uses a concise complete-answer instruction capped at 300 Chinese characters or
180 English words without raw HTML.

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

A selected Knowledge collection is only an allowed private-source scope. Native
rounds retain Auto Tool choice: clear catalog/private overlap uses Knowledge,
current public facts use Web, independently necessary private and public
evidence may use Both, and visible-context/general questions remain Direct.
Mere uncertainty and “more context” do not force retrieval. An empty result is
a successful miss without `[K#]`.

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

A successful answer with only a Generation step persists that step so reload
can display the `Direct` route. Failed and cancelled Generation steps remain
durable. Detail fields are allowlisted and bounded; unknown keys and exact
`query`/`redactedArgs` are dropped before SSE/persistence. Provider reasoning is
bounded to 1 MiB for persistence and receives credential-pattern redaction.
Live reasoning keeps a bounded suffix before emission so a credential pattern
split across adjacent provider chunks is redacted before any complete secret
can reach the browser.

Rules:

- Provider-returned reasoning is streamed separately from factual process
  steps. When a provider exposes no reasoning, the UI may say "Analyzing" as a
  process status but must not fabricate reasoning text.
- Running generation auto-expands the process panel. Completion collapses it to
  a one-line `Direct|Knowledge|Web|Both` summary with source counts. Manual
  expansion is authoritative and must
  not force chat scroll-to-bottom.
- Ordinary answers render a durable `Direct` summary rather than an empty panel.
- Persist rendered provider reasoning and sanitized Process Steps so reload and
  conversation switching reproduce the completed view.
- Keep the durable diagnostic trace complete, but project specialized read-only
  tools once in the ordinary UI: a generic `search_web`/`search_knowledge`
  Tool row is hidden only when the same `toolName` and Round has a matching
  Web/Knowledge row. Unmatched failures and custom Tools remain visible. Panel
  counts, active state, and summaries use the projected rows without mutating
  persisted metadata.
- Do not render lifecycle-only `outcome` details (`running`, `streaming`,
  `completed`, or `cancelled`) below a Status that already expresses them.
  Keep meaningful outcomes such as `degraded`. Sanitized provider reasoning is
  shown as returned; its language is not rewritten or translated.
- Allowed persisted details include hit/source counts, duration,
  provider/mode identifiers, allowlisted failure category, and Citation
  mapping.
- Forbidden persisted/rendered details include credentials, authorization
  headers, ciphertext, exact queries, redacted/raw Tool arguments, catalog
  metadata, raw provider events, complete Web/Knowledge bodies, system prompts,
  internal safety instructions, stack traces, SQL, and database topology.
- Reasoning effort, Search mode, and selected Knowledge are independent inputs.

G19.3 moves external Web retrieval into the live provider loop after the
assistant SSE starts. Every accepted `search_web` call produces its own running
and terminal Tool/Web steps. Provider-returned reasoning and Generation remain
live, and model-built-in Search is live once its provider stream is established.
G19.6B registers selected Knowledge in that live loop for Tool-round-capable
providers when Search is `off` or `external`. Each accepted call produces live
Tool/Knowledge steps and returns a bounded Tool Result. G19.6D removes the old
Handler pre-answer authority. Non-Tool/unknown providers and model-built-in
Search now run the unified same-model route Planner after `message.started`,
execute only the selected Knowledge/Web authority, and then answer. This
compatibility path is visibly traced and never restores pre-SSE retrieval.

## 7. Web and Knowledge tools

### `search_web`

- Registered only in `external` mode with one active tested external provider.
- Receives a standalone Query created by the current model from bounded active
  conversation context.
- The external provider receives only the Query, never raw conversation
  history.
- Results are normalized, deduplicated, bounded, treated as untrusted, and
  assigned current-turn `[W#]` capabilities by Go.
- The same resolved external provider is retried once after a 250 ms
  context-aware delay only for transport `REQUEST_FAILED`, HTTP `408`, `429`,
  or `5xx`. Authentication/other `4xx`, schema/response failures, and cancelled
  contexts do not retry; no retry may re-resolve or switch providers.
- Web-only Tool-unsupported providers use the same selected model for one
  bounded decision/query pass. When Knowledge is selected, the unified
  four-route Planner owns both authorities instead.

### `search_knowledge`

- Registered only when the conversation has selected Knowledge collections.
- Before routing, an ACL- and consent-authorized metadata query ranks active
  collection names/descriptions and filenames against the current question.
  It sends at most eight collections and 4 KiB total: at most five relevant
  plus three representative titles per collection, with UTF-8-safe 128-byte
  name, 512-byte description, and 256-byte title bounds.
- The catalog is untrusted routing metadata, never answer evidence. Catalog
  access reads no chunks/body text, embeddings, hydration, or reranker state.
  Delimiters are escaped, omitted titles do not prove absence, and catalog or
  governance failure omits the hint without blocking chat or forcing Knowledge.
- The model argument schema contains only `query`. Collection IDs are
  server-authoritative and copied from the authenticated conversation
  selection; a model cannot expand the selected set through Tool arguments.
- Reuses the active BM25/pgvector, Query Expansion, RRF, Jina reranker,
  authority hydration, deletion visibility, and no-evidence policy.
- Results receive current-turn `[K#]` capabilities only after the existing
  evidence gate.
- A normal miss is a successful empty Tool Result, not a user-visible error.

### `search_memory`

- `internal/chat` owns the canonical no-argument definition, JSON SHA-256, and
  call validator. The contract is `memory-search-tool-v1` with SHA-256
  `f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6`.
- The Tool is exposed only on the first native Tool round when the default-off
  flag is true, Conversation Memory use is allowed, the selected Provider is
  Tool-round capable, Search is not model-built-in, and the turn is not a
  direct `remember|correct|forget` action.
- Before a call, the Provider receives the normal chat request and Tool
  definition but no Memory candidate body, ID, revision, scope, score, or
  database authority. First-round content/reasoning is buffered; no Memory call
  releases the ordinary answer without hybrid retrieval.
- If that buffered first round fails before it closes, even after a call was
  assembled, the call and draft are discarded before execution and the
  original compatibility path continues with zero Memory retrieval.
- Memory use requires exactly one first-round call with a non-empty ID, exact
  raw name, and explicitly decoded `{}` arguments. Whitespace/case variants,
  missing, `null`, malformed, non-empty, duplicate, unknown, or later-round
  Memory calls fail closed for this lane. `search_memory` is removed from all
  later rounds.
- A valid call runs the fixed BGE embedding, exact/BM25/vector RRF, admission,
  rerank, and Top-5/600/900 selector without invoking v1 or `MarkUsed`. After
  Record, migration `065` hydrates the exact final lane through current source,
  settings, epoch, projection, revision/hash, scope/lifecycle, time, and
  Sensitive authority. Go rechecks identity and redacts each body again.
- Empty, failed, stale, or fully redacted retrieval returns a bounded Tool
  Result and ordinary same-model continuation without Memory. A pre-content
  continuation failure recovers through the same Provider/model from the
  original request without any Memory body. Partial content is never replayed.
- Historical schema-v6 `PlanTools` results are immutable failed preflight
  evidence. Schema-v7 uses the real first-`ToolRoundProvider` shape. Its first
  GPT and DeepSeek Flash Development results completed only `28/300` and
  `33/300` routes and failed unchanged quality, slice, cutoff, and latency
  gates; they grant no rollout authority.

Knowledge and Web may run in either order only when each authority is relevant.
Current/public claims use Web; internal/private claims use Knowledge; a genuine
mixed request may cite both without treating extra material as automatically
more accurate.

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
| First transient external failure      | one same-provider retry; no intermediate notice    |
| Second transient external failure     | final redacted failure; ordinary answer; no `[W#]` |
| Continuation fails after evidence, before answer text | same-model no-Tools evidence answer |
| Continuation fails after partial answer text | terminal failure; no duplicate answer recovery |
| First recovery attempt emits partial text then fails | discard all events; retry once |
| Both recovery attempts fail             | final failure with zero recovery answer content |
| Later Search adds no source            | empty incremental Tool Result; keep prior markers  |
| Built-in capability unavailable       | mode disabled or degraded; no external fallback    |
| Native Tool unsupported/unknown       | same-model unified compatibility planner           |
| Auto capability cache miss/expired    | current turn Planner; background singleflight probe |
| Valid/explicitly incompatible probe   | shared supported/unsupported TTL row               |
| Transient/inconclusive probe          | shared five-minute unknown retry backoff            |
| Runtime explicit incompatibility      | async downgrade plus same-turn Planner              |
| Catalog ACL/consent/read failure      | omit metadata; chat continues without forced RAG    |
| Compatibility planner fails           | strong Knowledge, forced Web, else Direct; never Both |
| Knowledge miss                        | empty successful result; continue Model/Web        |
| Tool arguments malformed/unknown      | reject execution; redacted failed step             |
| Memory Tool flag absent/false         | do not expose `search_memory`; preserve v1 default |
| No first-round Memory call            | zero hybrid retrieval; release buffered answer     |
| Exact first-round `search_memory({})`  | current-authorized hybrid retrieval and same-model continuation |
| Buffered first round fails after a call is assembled | discard call/draft; compatibility path; zero Memory retrieval |
| Memory call name is whitespace/case variant | reject; only the exact raw name authorizes retrieval |
| Memory retrieval empty/failed/stale   | bounded Tool Result; continue without Memory       |
| Memory continuation fails before text | recover from original request with no Memory body  |
| Memory continuation fails after text  | preserve error; no duplicate recovery              |
| Write/external Tool awaiting approval | pause loop until allow/reject/cancel               |
| User cancels during Provider/Tool     | cancel both; one terminal cancelled event          |
| User cancels compatibility planner    | Tool/Web/Generation cancelled; no `planner_failed` |
| Provider reasoning unavailable        | process only; no fabricated reasoning              |
| Final answer uses no issued marker    | no Citation card                                   |
| Exact query/Tool args in process detail | dropped before SSE and persistence               |

## 10. Required verification

1. Provider fixtures for fragmented Tool Calls and multi-round continuation on
   OpenAI-compatible/Gemini and Anthropic formats.
2. Zero Search I/O with mode off and ordinary Auto no-search.
3. Explicit/current Search plus contextual follow-up Query correctness.
4. Native Auto Tool only for known-supported models; unified four-route Planner
   for unsupported/unknown selected-Knowledge models, with Direct, Knowledge,
   Web, and Both fixtures.
5. Strict built-in/external mutual exclusion and no provider fallback.
6. Ordered reasoning/process SSE, cancellation, redaction, terminal persistence,
   reload, collapsed summary, and manual-scroll behavior.
   Cancellation must cover compatibility planning, native Web, Knowledge,
   Handler persistence, no failure category, zero Citation, and repeated runs
   that detect cancellation-event delivery races.
7. Knowledge hit/miss/deletion and Knowledge/Web chained evidence.
8. Current-turn Citation reconciliation for used, unused, copied, and invented
   markers.
9. Full Go vet/test/race, frontend format/lint/typecheck/test/build, Compose
   rebuild/restart/health, clean-copy, real-provider smoke, and complete smoke
   cleanup.
10. External Search retry recovery for network/`408`/`429`/`5xx`, no retry for
    other `4xx` or response/schema failures, cancellation during delay, and
    final-error preservation after two transient failures.
11. Native continuation recovery for synchronous and in-stream failure,
    Web-only and mixed Knowledge/Web evidence, same provider/model, no recovery
    after answer content, cumulative usage, incremental Web Tool Results, and
    stable markers. Recovery tests must also prove buffering, first-attempt
    partial-draft discard, one retry, both-attempt failure with zero content,
    empty-answer retry, bounded output, and no retry after cancellation.
12. Query-aware catalog CJK/English ranking, five-plus-three title selection,
    4-KiB/field/collection bounds, ACL/deletion/active-file filtering,
    governance denial, delimiter escaping, and no body/chunk reads.
13. Unified Planner invalid JSON, timeout, and provider failure must prove
    strong Knowledge, forced Web, and no-signal Direct fallbacks respectively;
    failure never defaults to Both.
14. Capability override precedence, fixed user-data-free probe payload,
    valid-call classification, transient unknown, TTL/config-hash isolation,
    background warmup, singleflight, runtime downgrade, non-blocking cache
    writes, and optional multi-instance PostgreSQL visibility.
15. Frontend provider round-trip and Inherit cleanup plus durable query-free
    Direct/Knowledge/Web/Both summaries, dual source counts, and reason
    allowlisting.
16. Memory Tool exact definition/hash, default-off/direct-action/model-built-in
    exclusions, first-round buffering, no-call release, exact `{}` acceptance,
    malformed/null/non-empty/unknown/duplicate/later-round denial, multi-tool
    coexistence, migration-065 final hydration drift, secret re-redaction,
    same-model continuation, body-free recovery, and no v1 fallback/Usage
    mutation.

## 11. Rollback

For a catalog-quality regression, omit `WithKnowledgeRoutingCatalog` from the
Handler wiring. Native providers retain generic Auto Tool behavior and
compatibility routing remains bounded; no Knowledge or chat data changes.
Migration `042_model_tool_capability_cache` contains derived capability state
only. Its down migration drops the cache table without removing provider
configuration, credentials, conversations, Knowledge, or Citations.

For a Memory Tool regression, set `MEMORY_TOOL_LOOP_ENABLED=false` and restart
the API. This removes Tool exposure and restores the deployed v1 prompt/Usage
path without deleting Memory, projections, observations, or migration `065`.
