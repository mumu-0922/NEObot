# Planned chat Tool Loop contracts

Status: G19.2 process trace, G19.3 OpenAI-compatible/Gemini external Web, G19.4
Anthropic Tool Loop, and G19.5 three-state/built-in Search administration
promoted on 2026-07-22. G19.6A-D adds the server-authoritative Knowledge Tool
executor, connects it to Tool-round-capable providers, and moves non-Tool and
model-built-in compatibility retrieval after `message.started`. The Handler no
longer invokes the old pre-answer Auto RAG authority; the real-document
promotion gate remains in progress.

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

G19.5 built-in protocols:

```text
OpenAI official    -> openai_responses
Gemini official    -> gemini_google_search
Anthropic official -> anthropic_web_search
Custom compatible  -> openai_responses + exact tested model only
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
- G19.3 external Web work and G19.6B Tool-capable selected-Knowledge work
  execute after SSE starts and are represented by live Tool/Web and
  Tool/Knowledge steps. `search_knowledge` accepts only a bounded Query;
  authenticated conversation selection remains collection authority. The
  non-Tool/model-built-in compatibility executor is also live after
  `message.started`; pre-SSE Knowledge retrieval is forbidden.
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

## 4. Validation & Error Matrix

| Condition                        | Required behavior                                |
| -------------------------------- | ------------------------------------------------ |
| Search off                       | zero Search I/O                                  |
| Native Web Tool unsupported      | same-model compatibility plan                    |
| Native Knowledge Tool unsupported | live compatibility executor; no pre-SSE retrieval |
| Tool arguments malformed/unknown | do not execute; redacted failed step             |
| External Search failure          | truthful degradation; ordinary answer; no `[W]`  |
| Built-in unsupported             | disabled/degraded; no external fallback          |
| Invalid persisted Search mode    | `INVALID_SEARCH_MODE`; no write                   |
| Custom model not exactly tested  | disabled / `MODEL_BUILT_IN_SEARCH_UNSUPPORTED`    |
| Real test returns zero sources   | `MODEL_BUILT_IN_SEARCH_TEST_FAILED`; no attest    |
| Config changes during real test  | `MODEL_BUILT_IN_SEARCH_CONFIG_CHANGED`; no attest |
| Knowledge miss                   | successful empty result; continue                |
| Approval rejected                | do not execute; continue or terminate truthfully |
| Cancel during Provider/Tool      | cancel both; one terminal cancelled event        |
| Provider exposes no reasoning    | process only; no fabricated reasoning            |
| Successful Generation only       | omit empty durable process panel                  |
| Unknown process detail key       | drop before SSE/persistence                       |
| Anthropic Thinking continuation  | retain block order/signature in memory only       |
| Anthropic failed Tool Result     | matching `tool_use_id` plus `is_error=true`       |

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

1. Fragmented Tool arguments, 64-KiB rejection, forced/Auto choice, and native
   continuation fixtures for each promoted provider family.
2. Search off/Auto skip/explicit Search I/O assertions.
3. Ordered reasoning/process SSE, terminal persistence, reload, redaction, and
   cancellation.
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

## 7. Wrong vs Correct

Wrong:

```text
search enabled -> rewrite -> always Search -> answer
```

```text
create server conversation -> send first turn with pre-create composer mode
```

Correct after the owning G19 promotion:

```text
search mode + selected Knowledge + capabilities
  -> expose allowed tools to current model
  -> no Tool Call: answer
  -> Tool Call: validate/execute/trace -> native continuation
  -> reconcile only current-turn used citations -> persist
```

```text
create server conversation -> read returned persisted mode -> send first turn
```

Full target contract: `mm-chat/docs/contracts/chat-tool-loop.md`.
