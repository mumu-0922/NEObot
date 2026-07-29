# Planned chat Tool Loop contracts

Status: G19.2 process trace, G19.3 OpenAI-compatible/Gemini external Web, G19.4
Anthropic Tool Loop, G19.5 three-state/built-in Search administration, and
G19.6 server-authoritative Knowledge Tool migration were promoted on
2026-07-22. G19.9 continuation recovery was promoted on 2026-07-23. All
selected-Knowledge paths now execute after `message.started`; the Handler no
longer invokes the old pre-answer Auto RAG authority. G19.10 query-aware
Knowledge routing, unified compatibility planning, shared Tool-capability
state, and query-free route visibility were promoted on 2026-07-23. The
`search_memory` adapter described below is implemented for schema-v6
Development capture only; it is not a promoted production Tool or reader.

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

Selected Knowledge is authority scope, not retrieval intent. The current model
chooses one route per turn:

```text
route = direct | knowledge | web | both
```

Native Tool-capable models express the route through zero or more Auto Tool
Calls. Compatibility models return one bounded same-model plan:

```json
{
  "route": "direct|knowledge|web|both",
  "knowledgeQuery": "standalone private query",
  "webQuery": "standalone public query"
}
```

The routing catalog signature is:

```text
actor + current query + up to 8 selected collection IDs
  -> ACL/governance-filtered collection metadata + active filenames
  -> at most 4 KiB of untrusted routing JSON
```

Each collection contributes at most five lexically relevant active filenames
plus three representative active filenames. Collection name is capped at 128
bytes, description at 512 bytes, and each filename at 256 bytes. Catalog reads
must not read chunks/body text, generate embeddings, hydrate evidence, or
rerank.

Administrator Tool-capability DTOs are:

```json
{
  "toolCapability": {
    "default": "auto|enabled|disabled",
    "modelOverrides": {"model-id": "enabled|disabled"}
  }
}
```

```json
{
  "toolCapabilityDefault": "auto|enabled|disabled",
  "toolCapabilityModelOverrides": {"model-id": "enabled|disabled"}
}
```

Automatic results use the shared database key and states:

```text
model_tool_capability_cache(provider_config_hash, model_id)
status = supported | unsupported | unknown
TTL = 7 days | 24 hours | 5 minutes
```

G19.5 built-in protocols:

```text
OpenAI official    -> openai_responses
Gemini official    -> gemini_google_search
Anthropic official -> anthropic_web_search
Custom compatible  -> openai_responses + exact tested model only
```

The active OpenAI Responses built-in request contract is:

```http
POST {normalizedBaseURL}/responses
Content-Type: application/json

{
  "model": "exact-selected-model",
  "stream": true,
  "tools": [{ "type": "web_search" }],
  "tool_choice": "required",
  "include": [
    "web_search_call.results",
    "web_search_call.action.sources"
  ]
}
```

Conversation input-part mapping is role-sensitive:

```text
user text      -> input_text
assistant text -> output_text
```

Configured OpenAI-compatible model identity resolves before capability lookup:

```text
configured provider ID + accepted canonical alias -> configured provider ID
unbound provider + accepted alias                -> openai_compatible
unaccepted alias                                 -> UNSUPPORTED_PROVIDER
```

Resolver and administrator signatures:

```go
ResolveExternal(context.Context) (websearch.ActiveExecution, error)
ResolveModelBuiltIn(context.Context, websearch.ModelBuiltInResolutionRequest) (websearch.ActiveExecution, error)

type ModelBuiltInResolutionRequest struct {
    ProviderID string
    ModelID    string
    Protocol   ModelBuiltInProviderID
}
```

```http
POST /v1/admin/providers/{providerId}/built-in-search-test
Content-Type: application/json

{"protocol":"openai_responses","model":"exact-persisted-model"}
```

Success returns the normalized administrator provider DTO plus
`sourceCount > 0`. The provider DTO contains:

```text
modelBuiltInSearch.protocol?
modelBuiltInSearch.model?
modelBuiltInSearch.source = official | custom | none
modelBuiltInSearch.connectionTestValid
modelBuiltInSearch.connectionTestedAt?
```

Target provider round events:

```text
content.delta | reasoning.delta | tool.call.delta |
tool.call.completed | usage.updated | round.completed | round.error
```

Active G19.3/G19.4 provider seam:

```go
type ToolRoundProvider interface {
    Provider
    StreamToolRound(context.Context, ProviderRoundRequest) (<-chan ProviderEvent, error)
}

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

The normalized one-round planning seam reused by Development Memory routing is:

```go
type ToolPlanner interface {
    PlanTools(context.Context, ToolPlanRequest) ([]ToolCall, error)
}

type ToolPlanRequest struct {
    Prompt          string
    ModelRef        ModelRef
    Tools           []ToolDefinition
    DisableThinking bool
    MaxOutputTokens int
    Temperature     *float64
}
```

Its fixed Memory Tool contract is:

```text
name             = search_memory
contract version = memory-search-tool-v1
contract SHA-256 = f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6
arguments        = explicit {}
tool choice      = auto
temperature      = 0
maximum output   = 128
thinking         = disabled
```

OpenAI-compatible/Gemini continuation is an assistant message containing the
completed `tool_calls`, followed by one `role=tool` message per matching
`tool_call_id`. Fragmented names/arguments are accumulated before execution;
arguments are capped at 64 KiB.

Anthropic continuation uses the same normalized exchange but carries an
in-memory, provider-private `ProviderState` from `round.completed`. It preserves
ordered `thinking`/signature, `redacted_thinking`, `text`, and `tool_use`
blocks, followed by one user message containing matching `tool_result` blocks.
Failed results set `is_error=true`. Provider state never reaches SSE,
diagnostics, or persistence.

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
Singleton steps retain `<messageId>:<kind>:1`; G19.3 allocates each Tool/Web
execution as the next stable `<messageId>:tool|web:<n>` pair.

## 3. Contracts

- `off` means zero Search planning, resolver, built-in, and external I/O.
- Built-in and external Search are mutually exclusive and never fall back to
  one another.
- External and model-built-in resolution use separate methods. Do not scan
  model providers from the external path or external providers from the
  built-in path.
- Custom compatible attestation binds provider ID/type, normalized Base URL,
  encrypted secret reference, protocol, and exact model. Any bound change must
  invalidate it; commit a positive real test with a Postgres compare-and-set.
- When a runtime OpenAI-compatible provider has an authoritative configured ID,
  its accepted `openai_compatible`, `openai`, and `openai-compatible` aliases
  must resolve back to that ID before built-in capability lookup. Preserve the
  canonical `openai_compatible` ID only for an unbound provider.
- Explicit `model_builtin` is a hard request to execute the sole attested Web
  tool. OpenAI Responses therefore uses `web_search` with
  `tool_choice=required`; `web_search_preview` and the default Auto choice are
  forbidden because the former may be rejected by current gateways and the
  latter permits a false successful turn with zero Search I/O.
- The OpenAI Responses request copy appends a bounded public-page and accessible
  URL-citation requirement only to the latest user item. It must not mutate the
  persisted message or earlier history. This prevents provider-native Weather
  and similar vertical records containing only `{type,name}` from becoming the
  only evidence and producing a false zero-source result.
- Retry Responses startup once with the exact payload after 200 ms only for a
  transport failure, HTTP `408`, `429`, or `5xx`. Do not retry another `4xx`,
  cancellation, or an in-stream failure, and never switch provider/model.
- Parse sources from incremental annotations/output items and from final
  `response.completed.response.output`; use one accumulator so repeated final
  projections cannot duplicate citations.
- Encode Responses history by role. User text is `input_text`; assistant text
  is `output_text`. A first-turn smoke cannot validate this boundary, so every
  real browser-path proof must include at least one prior assistant message.
- New conversations with no explicit Search fields inherit the most recently
  updated conversation mode. The frontend must use the returned server session
  for the first message rather than a stale pre-create composer snapshot.
- Native Tools are sent on the initial current-model request with automatic
  selection. Explicit current/Search intent must use the selected Search mode.
- Tool Calls are accumulated and validated before execution, then returned in
  the provider's native continuation format to the same model.
- Do not add product-level Tool Round or Tool Call count limits for the current
  single-user deployment. Cancellation, request context, provider timeout,
  terminal errors, and approval rejection remain exit conditions.
- A Tool-unsupported or not-yet-known current model uses the same model for one
  bounded `direct|knowledge|web|both` compatibility plan when Knowledge is in
  scope. Never use a hidden model and never add this planner round to a known
  Tool-capable native path.
- Persist only rendered provider reasoning and sanitized steps. Credentials,
  raw payloads, system prompts, full source bodies, and internal errors remain
  forbidden.
- Read-only Web/Knowledge tools run automatically. Side effects require an
  approval policy before registration.
- Selected Knowledge only grants an allowed private-source scope. A native
  first round keeps `tool_choice=auto`: clear catalog/private overlap uses
  Knowledge, current public facts use Web, independently necessary private and
  public evidence may use Both, and visible-context/general requests stay
  Direct. Mere uncertainty and representative fallback filenames never force
  retrieval.
- Build the query-aware catalog only after actor ACL and per-target-model query
  plus collection answer-consent authorization. Catalog/ACL/governance failure
  omits the catalog and fails open to ordinary chat; it must not turn into
  unconditional Knowledge retrieval.
- Catalog values are untrusted metadata, not evidence. Escape prompt
  delimiters, never follow filename/description instructions, never cite the
  catalog, and never infer non-existence from omitted/truncated filenames.
- A compatibility plan executes exactly its declared narrow route. Planner
  invalid JSON, oversize output, timeout, or provider failure uses only the
  deterministic fallback: strong lexical/private signal -> Knowledge;
  explicitly forced available Search -> Web; otherwise Direct. It never
  defaults to Both.
- Knowledge miss is successful empty evidence. A public/current request may
  continue to Web; a private-document existence request reports no matching
  selected Knowledge evidence; ordinary answers must not fabricate `[K#]`.
- Tool capability resolves in this order: model override, provider default,
  unexpired probe cache, then `unknown` compatibility planning. `enabled` and
  `disabled` are explicit operator assertions; `auto` is the normal default.
- An Auto probe sends only a fixed fictional Tool definition and fixed prompt.
  It contains no user query, conversation, catalog, source body, provider raw
  payload, or credential. Only a valid completed Tool Call with matching name,
  non-empty ID, and JSON-object arguments records `supported`. Explicit
  tools/function-call incompatibility records `unsupported`; cancellation,
  timeout, rate limit, 5xx, transport failure, ordinary 400, and inconclusive
  prose remain `unknown`.
- Provider save/activation schedules detached background warmup for the first
  configured model and matching task models. An unknown first-use model uses
  Planner immediately and starts one singleflight probe; neither save nor chat
  waits for the probe. Probe/cache writes use bounded detached contexts.
- A real native first round downgrades capability only for explicit Tool
  incompatibility, writes the downgrade asynchronously, and continues through
  same-turn Planner. Transient provider failures remain provider failures.
- Capability cache identity includes provider config hash and exact model ID.
  The hash binds user/provider identity, type, normalized Base URL, model list,
  encrypted secret reference hash, connection hash, default, and model
  overrides. Config changes therefore miss old rows without exposing secrets.
- External Web execution may retry the exact same resolved provider once after
  a short context-aware delay only for `REQUEST_FAILED`, HTTP `408`, `429`, or
  `5xx`. It must not re-resolve, switch providers, retry authentication/other
  `4xx` or response/schema failures, or continue after context cancellation.
- Each successful Web Tool Result contains only sources newly minted by that
  execution. Prior Tool Results remain in native continuation history, so
  serializing the cumulative Web corpus again is forbidden. New sources keep
  their cumulative marker, for example the second execution may return `[W2]`
  without repeating `[W1]`.
- If a later native Tool continuation fails after Web or Knowledge evidence is
  ready but before any answer content was emitted, perform one no-Tools answer
  stream through the same provider and model with the bounded cumulative
  evidence prompt. Preserve cumulative usage and existing citation authority.
  Do not recover after partial answer content, without evidence, after
  cancellation, or by switching provider/model.
- Buffer a recovery answer until its provider stream closes successfully.
  Never expose a partial recovery draft. A failed first recovery attempt may be
  retried once with the exact same provider/model and concise evidence prompt;
  discard all first-attempt content/reasoning/usage. Both failures emit the
  final error with zero recovery content. Buffer at most 1 MiB and 8,192 events.
- Only current-turn backend-issued markers used by the final reconciled answer
  become Citations; unused results stay in the process trace.
- G19.2 persists sanitized `reasoning` and `processTrace` in terminal assistant
  metadata. A successful Generation-only answer persists its Generation step
  so reload can display `Direct`; failed and cancelled Generation remain
  durable as before.
- Persist the complete diagnostic Tool trace. The frontend display projection
  hides a generic `search_web`/`search_knowledge` Tool row only when the same
  `toolName` and Round has its specialized Web/Knowledge row. Unmatched or
  custom Tool rows remain visible, and summary counts use the projected rows.
  Do not mutate or discard the durable trace to remove a UI duplicate.
- Do not repeat lifecycle-only `outcome` details (`running`, `streaming`,
  `completed`, or `cancelled`) beneath the localized step Status. Meaningful
  outcomes such as `degraded` remain visible. Provider reasoning stays the
  sanitized provider-returned text and is not rewritten or translated.
- Process detail uses an allowlist and bounded values. Exact `query`,
  `redactedArgs`, catalog metadata, unknown fields, raw payloads, source bodies,
  headers, prompts, SQL, and internal errors are dropped before SSE and
  persistence. The frontend derives `Direct|Knowledge|Web|Both`, source counts,
  and localized reasons only from sanitized step kinds/counts and an explicit
  reason-category allowlist.
- Live reasoning redaction must retain a bounded un-emitted suffix across
  provider chunks. Sanitizing each chunk independently is forbidden because a
  split `apiKey=`/value or `Bearer` token can bypass the regex boundary.
- G19.3 external Web work and G19.6B Tool-capable selected-Knowledge work
  execute after SSE starts and are represented by live Tool/Web and
  Tool/Knowledge steps. `search_knowledge` accepts only a bounded Query;
  authenticated conversation selection remains collection authority. The
  non-Tool/model-built-in compatibility executor is also live after
  `message.started`; pre-SSE Knowledge retrieval is forbidden.
- Context cancellation is a terminal control state, not Tool degradation. A
  compatibility planner, native/compatibility Web execution, or Knowledge
  execution that observes `errors.Is(err, context.Canceled)` or a cancelled
  operation context emits `ProcessStepStatusCancelled`, carries no
  `FailureCategory`, stops fallback/continuation, and preserves
  `detail.outcome=cancelled`. Its terminal process event must remain deliverable
  after the operation context is cancelled; the Handler treats receipt of that
  event as message-cancellation authority. A provider timeout that is only
  `context.DeadlineExceeded` remains a failure unless the run was separately
  cancelled.
- Durable Web source blocks and metadata are projected from markers actually
  used by the reconciled answer. Filtering must preserve the originally minted
  marker: retaining only `[W2]` must never rename it to `[W1]`.
- A forced first native round that ignores required Tool choice is buffered and
  discarded before same-model compatibility planning, preventing duplicate
  user-visible answer text.
- Anthropic explicit Search without Thinking uses named `tool_choice`;
  extended Thinking uses `auto`, then the same buffered compatibility rule if
  no Tool Call is returned.
- Usage is accumulated across native provider rounds without double-counting
  repeated updates inside the current round.
- `internal/memoryroute` is the single `search_memory` definition authority for
  schema-v6 Development capture and future product integration. The Tool has
  no query argument: the server already owns the current request text, so the
  model decides only whether Memory is needed.
- The Development adapter binds one exact configured Provider ID/model and
  submits only the already-secret-redacted current query. Candidate bodies,
  Memory IDs, scopes, revisions, retrieval scores, and database authority are
  forbidden from this route request.
- Zero calls is a valid `no_memory` decision. Use Memory requires exactly one
  Provider choice and one call with a non-empty ID, exact `search_memory` name,
  and a non-nil empty decoded argument object. Missing, `null`, malformed, or
  non-empty arguments; unknown or duplicate calls; Provider failure; deadline;
  and model/contract drift fail closed.
- `OpenAICompatibleProvider.PlanTools` sends `enable_thinking=false` when the
  fixed route requests it. Official `OpenAIProvider.PlanTools` deliberately
  omits that non-standard extension while preserving the exact model,
  `temperature=0`, and `max_tokens=128` fields.
- This one-round planner currently returns only route evidence. It does not
  execute Memory retrieval or same-model continuation. The isolated
  `usermemory` Development policy controls BGE release, and Server composition
  installs neither the route policy nor the adapter.

## 4. Validation & Error Matrix

| Condition                        | Required behavior                                |
| -------------------------------- | ------------------------------------------------ |
| Search off                       | zero Search I/O                                  |
| Native Web Tool unsupported      | same-model compatibility plan                    |
| Native Knowledge Tool unsupported | live compatibility executor; no pre-SSE retrieval |
| Tool capability override enabled/disabled | bypass probe cache with explicit operator assertion |
| Auto capability cache miss/expired | current turn Planner; one background singleflight probe |
| Probe valid matching Tool Call   | shared `supported` row, seven-day TTL            |
| Probe explicit Tool incompatibility | shared `unsupported` row, 24-hour TTL          |
| Probe timeout/429/5xx/ordinary 400 | shared `unknown` retry backoff, five-minute TTL |
| Native first-round explicit incompatibility | async downgrade; same-turn unified Planner |
| Catalog ACL/consent/read failure | omit catalog; ordinary Auto/Planner behavior continues |
| Planner invalid/timeout/provider failure | strong Knowledge, forced Web, else Direct; never Both |
| Planner requests unavailable authority | reject plan and apply deterministic fallback |
| Tool arguments malformed/unknown | do not execute; redacted failed step             |
| External Search failure          | truthful degradation; ordinary answer; no `[W]`  |
| First transient external failure | one same-provider retry; no intermediate failure |
| Second transient failure         | return final redacted error; normal degradation  |
| Native continuation fails after evidence, before content | one same-model evidence answer stream |
| Native continuation fails after partial content | preserve the error; no duplicate answer fallback |
| First recovery answer stream fails after partial content | discard draft; one same-model retry |
| Both recovery answer streams fail | final failure; expose zero recovery content |
| Repeated Web source              | empty incremental Tool Result; stable prior marker |
| Built-in unsupported             | disabled/degraded; no external fallback          |
| Invalid persisted Search mode    | `INVALID_SEARCH_MODE`; no write                   |
| Custom model not exactly tested  | disabled / `MODEL_BUILT_IN_SEARCH_UNSUPPORTED`    |
| Configured provider receives a canonical alias | restore authoritative provider ID before lookup |
| Unbound compatible receives an accepted alias | canonical `openai_compatible` identity |
| OpenAI Responses built-in request | required `web_search`; at least one source or truthful `no_results` |
| Weather/vertical result has no URL | provider-only latest-user directive requests public URL evidence |
| Responses startup transport/408/429/5xx | one exact retry after 200 ms |
| Responses startup other 4xx or cancellation | no retry; redacted failure |
| Assistant history encoded as `input_text` | invalid request; must use `output_text` |
| Real test returns zero sources   | `MODEL_BUILT_IN_SEARCH_TEST_FAILED`; no attest    |
| Config changes during real test  | `MODEL_BUILT_IN_SEARCH_CONFIG_CHANGED`; no attest |
| Knowledge miss                   | successful empty result; continue                |
| Approval rejected                | do not execute; continue or terminate truthfully |
| Cancel during Provider/Tool      | cancel both; one terminal cancelled event        |
| Cancel during compatibility plan | Tool/Web/Generation cancelled; no `planner_failed` |
| Provider exposes no reasoning    | process only; no fabricated reasoning            |
| Successful Generation only       | persist Generation; reload summary is `Direct`    |
| Unknown process detail key       | drop before SSE/persistence                       |
| Exact query or redacted Tool args in process detail | drop before SSE/persistence        |
| Anthropic Thinking continuation  | retain block order/signature in memory only       |
| Anthropic failed Tool Result     | matching `tool_use_id` plus `is_error=true`       |
| Memory route returns zero calls  | valid `no_memory`; no hybrid final is released     |
| Memory route returns one exact `search_memory({})` call | route evidence only; `usermemory` still reauthorizes retrieval |
| Memory route omits arguments or returns `null` | reject; nil map is not an explicit empty object |
| Memory route returns unknown/duplicate calls or multiple choices | reject the entire route response |
| Memory route Provider/model/contract drifts or times out | fail closed; unchanged v1 chat authority |

## 5. Good / Base / Bad Cases

- Good: globe external, ordinary writing request, no Tool Call and no Search
  provider request.
- Good: contextual explicit Search generates one standalone Query, shows Tool
  progress, continues the same model, and keeps only used `[W]` markers.
- Good: selected Knowledge can run before Web and preserve distinct `[K]`/`[W]`
  authority.
- Good: `有小作文模板嘛` sees a matching bounded filename and uses Knowledge,
  while an unrelated birthday greeting in the same conversation remains
  Direct.
- Good: an unknown model answers through same-model Planner immediately while
  one user-data-free probe warms later turns.
- Good: the frontend sends `openai_compatible`, the configured runtime restores
  `SERVER_DEFAULT`, the exact attestation resolves, and one required Responses
  Web call persists normalized sources.
- Base: an unbound OpenAI-compatible provider still canonicalizes its accepted
  aliases to `openai_compatible` and has no configured-ID capability lookup.
- Good: Tavily transport fails once, the same resolved execution succeeds on
  its only retry, and one truthful Web result enters the Tool continuation.
- Good: a later native continuation stream fails before answer text; the same
  model answers once from the already-authorized `[K]`/`[W]` evidence without
  Tools and cumulative usage remains monotonic.
- Good Development-only Memory route: an exact configured GPT/DeepSeek model
  sees a redacted relevant query and the shared Tool, returns one explicit
  `search_memory({})` call, and no candidate body crosses that route boundary.
- Base Development-only Memory route: an unrelated request yields zero calls;
  speculative BGE work is discarded by `usermemory`.
- Base: a Tool-unsupported model uses the visible unified compatibility path
  and still answers Direct when planning fails without a strong signal.
- Base: a catalog is unavailable or governance-denied; the turn proceeds
  without leaking private metadata or forcing retrieval.
- Bad: retrying a bad Key/schema response, re-resolving into another provider,
  retrying after cancellation, pre-searching every enabled turn, running
  built-in and external Search together, repeating the cumulative Web corpus
  in every Tool Result, recovering after partial answer text, fabricating
  reasoning, rendering all retrieved sources as Citations, treating selection
  as mandatory RAG, defaulting to Both for “more context”, blocking chat on a
  capability probe/cache write, or persisting query/catalog/provider payloads.

## 6. Tests Required

1. Fragmented Tool arguments, 64-KiB rejection, forced/Auto choice, and native
   continuation fixtures for each promoted provider family.
2. Search off/Auto skip/explicit Search I/O assertions.
3. Ordered reasoning/process SSE, terminal persistence, reload, redaction, and
   cancellation.
   Cancellation fixtures must cover compatibility planning, native Web,
   Knowledge, Handler persistence, no failure category, zero Citation, and a
   repeated run that detects event-delivery races.
4. Capability mismatch and compatibility-planner tests with no hidden model.
5. Knowledge hit/miss/deletion plus mixed Knowledge/Web marker truth.
6. Real selected provider/Search smoke must prove ordinary zero-Search,
   explicit contextual Search, live Tool/Web steps, reload, and temporary-state
   deletion.
7. G19.2 reload mapping, manual expand/collapse authority, and no-empty-panel
   fixtures across backend and frontend.
8. G19.5 official provider/model allow and non-chat deny fixtures; custom exact-
   model attestation, bound-field invalidation, Postgres stale compare-and-set,
   route DTO, mode reload/inheritance, first-message inheritance, and separate
   resolver-call assertions.
9. External retry fixtures must prove one recovery after network/`408`/`429`/
   `5xx`, no retry for other `4xx` or response/schema errors, immediate
   cancellation, and the second stable error after two transient failures.
10. Continuation-recovery fixtures must cover synchronous start failure,
    in-stream failure, same provider/model, Web-only and mixed Knowledge/Web
    evidence, no fallback after content, cumulative usage, incremental Web Tool
    Results, stable markers, buffered partial-draft discard, one retry, both-
    attempt failure, empty-answer retry, output bounds, and cancellation.
11. Catalog units must cover CJK bigrams, English terms, five relevant plus
    three representative titles, eight-collection/4-KiB/UTF-8 field bounds,
    delimiter escaping, zero-document collections, and fallback titles not
    creating a strong match.
12. Catalog PostgreSQL integration must assert actor ACL, selected-only scope,
    deleted/non-active/unavailable exclusion, active filename ranking, and no
    body/chunk projection. Governance denial must prove the source is not read.
13. Unified Planner tests must cover Direct, Knowledge, Web, Both, Knowledge
    miss, and invalid JSON/timeout/provider-failure fallbacks to strong
    Knowledge, forced Web, and no-signal Direct respectively. No failure case
    may default to Both.
14. Capability tests must assert override precedence, valid structured-call
    classification, transient `unknown`, probe payload isolation,
    singleflight, non-blocking warmup, runtime downgrade, status TTLs, config
    hash invalidation, and optional multi-instance PostgreSQL visibility.
15. Frontend/provider tests must assert DTO round-trip, invalid/unselected
    override filtering, Inherit deletion when a model is deselected, query and
    argument redaction, four route summaries, source counts, and reason
    allowlisting.
16. OpenAI Responses request tests must assert `web_search`,
    `tool_choice=required`, source includes, and unchanged streaming. Resolver
    tests must cover both configured-ID restoration and unbound canonicalization;
    a real isolated chat replay must assert the authoritative persisted model
    reference, resolved/completed Web stages, at least one Search source, and
    temporary-conversation deletion.
17. Responses regressions must prove only the request copy's latest user item
    receives the URL-source requirement, final-only citations are normalized,
    one `503` startup response is retried, non-transient `4xx` is not retried,
    and upstream bodies/credentials remain redacted.
18. Responses history tests must assert user `input_text` plus assistant
    `output_text`. The real Search replay must contain a completed assistant
    turn before the exact browser query and still persist URL Citations.
19. Memory Tool-route tests must assert the exact definition/hash, one-choice
    decoding, zero-call abstention, explicit empty-object acceptance, missing/
    null/malformed/non-empty argument rejection, unknown/duplicate call
    rejection, model/contract provenance, candidate-body absence, official
    OpenAI `enable_thinking` omission, and OpenAI-compatible
    `enable_thinking=false` encoding. These tests are offline and confer no
    production activation authority.

## 7. Wrong vs Correct

Wrong:

```text
search enabled -> rewrite -> always Search -> answer
```

```text
Knowledge selected -> always retrieve Knowledge -> maybe search Web -> answer
```

```go
// Wrong: blocks the user turn and derives a probe from user content.
status := probeToolCapability(requestContext, provider, userPrompt)
```

```go
// User cancellation becomes a false degraded Search failure.
status := ProcessStepStatusFailed
failureCategory := "planner_failed"
```

```text
create server conversation -> send first turn with pre-create composer mode
```

```json
// Wrong: stale tool name plus optional execution.
{ "tools": [{ "type": "web_search_preview" }] }
```

```text
// Wrong: the route model sees candidate bodies before deciding whether to use
// Memory, or missing arguments are treated as an empty object.
query + candidate Memories -> answer model -> inspect self-reported usage
```

Correct after the owning G19 promotion:

```text
search mode + selected Knowledge + capabilities
  -> bounded governed catalog + capability resolution
  -> known native: expose allowed tools with Auto choice
  -> unknown/unsupported: same-model Direct|Knowledge|Web|Both Planner
  -> no Tool Call: answer
  -> Tool Call: validate/execute/trace -> native continuation
  -> reconcile only current-turn used citations -> persist
```

```go
// Current turn never waits for synthetic capability discovery.
status := resolveFromOverridesOrCache(providerConfigHash, modelID)
if status == ToolCapabilityUnknown {
    startSingleflightBackgroundProbe(providerConfigHash, modelID)
    return compatibilityPlanner
}
```

```go
// Retry only the same already-resolved read-only execution once.
result, err := service.Execute(ctx, execution, request)
```

```text
create server conversation -> read returned persisted mode -> send first turn
```

```json
// Correct: explicit built-in Search must execute the sole attested Web tool.
{
  "tools": [{ "type": "web_search" }],
  "tool_choice": "required"
}
```

```go
if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
    status = ProcessStepStatusCancelled
    failureCategory = ""
}
```

```go
// The continuation already contains earlier Tool Results. Return only sources
// added by this execution, then recover a pre-content continuation failure
// through the same model with bounded evidence and no Tools.
result := webSearchSuccessToolResult(previous, cumulative)
```

```text
// Correct Development-only Memory route.
secret-redacted query + exact search_memory Tool
  -> zero calls: no_memory
  -> one exact call with ID and explicit {}: provenance-bound use_memory
  -> usermemory independently reauthorizes and releases bounded BGE results
```

Operational rollback for catalog quality regressions is to omit
`WithKnowledgeRoutingCatalog` from Handler wiring: native models retain generic
Auto Tool behavior and compatibility routing remains bounded. Rolling back
migration `042_model_tool_capability_cache` drops only derived capability cache
state; no chat, Knowledge, credential, or provider configuration data is lost.

Full target contract: `mm-chat/docs/contracts/chat-tool-loop.md`.
