# Kelivo Web Search Runtime

## Snapshot

- Repository: `https://github.com/Chevey339/kelivo`
- Inspected commit: `545f7d67de250283232c9487aa5f4f42e85a7643`
- Commit date: `2026-07-20T06:45:33-07:00`

## External search flow

Kelivo treats an enabled external search provider as an optional model tool,
not as a command to execute a search before every answer.

1. The current assistant persists `searchEnabled`.
2. When search is enabled, built-in search is inactive, and the selected model
   supports tools, Kelivo adds one `search_web(query: string)` function tool.
3. The request includes the retained conversation context and uses
   `tool_choice: auto`.
4. The main chat model decides whether a search is needed and, when needed,
   produces the search query as the tool argument. There is no separate
   keyword router or standalone query-rewrite request in this path.
5. Kelivo executes only the currently selected search service, attaches an
   index and short ID to every result, emits tool-call/tool-result progress,
   appends the tool result to the conversation transcript, and makes a
   continuation model request for the final cited answer.
6. When the model emits no tool call, no search provider request is made.

### Loop termination and execution limits

Kelivo's provider implementations generally continue with `while (true)` until
the model stops emitting tool calls:

- OpenAI Chat Completions, Anthropic/Claude, Gemini, and Vertex paths have no
  shared maximum tool-round count.
- They do not impose a per-tool invocation count.
- Multiple client tool calls returned in one model round are executed with
  ordinary `for` loops, so execution is sequential rather than concurrent.
- The OpenAI Responses path is the exception: it fingerprints the exact set of
  tool names and arguments and breaks after the same signature is returned for
  three consecutive rounds. This is a repeated-call guard, not a total round
  limit; distinct calls can still continue indefinitely.

Therefore Kelivo's protocol loop is a useful reference, but its termination
policy should not be copied unchanged into a server that owns shared provider
quota. `mm-chat` needs explicit round/call/time budgets in addition to a
duplicate-call guard.

Relevant source:

- `lib/core/services/search/search_tool_service.dart`
- `lib/features/home/services/tool_handler_service.dart`
- `lib/features/home/services/message_generation_service.dart`
- `lib/core/services/api/providers/openai_common.dart`

The injected tool instruction recommends search for fresh information, fact
verification, technical documentation, and API information. Search results are
cited with turn-scoped `(index:id)` markers.

## Built-in search flow

Kelivo also supports provider-native search for a model/provider allowlist.
Provider-native search and external `search_web` mode are mutually exclusive in
the UI. The provider-native request is also configured for automatic tool use;
the upstream model/provider manages the actual search.

## Difference from mm-chat

Current `mm-chat` interprets `useSearch=true` as a routing input. When Knowledge
is not sufficient, `planSourceFusion` sets `SearchRequested=true`, so the
backend rewrites a query and calls Tavily before the main answer on nearly every
turn. The switch is therefore effectively forced search rather than Auto.

Kelivo's important semantic distinction is:

> enabled means the search capability is available to the model; it does not
> mean a search must be executed on every turn.

## Recommended mm-chat adaptation

Adopt the same user-facing semantics:

- globe off: do not plan or execute Web search;
- globe on: offer a single `search_web` tool with automatic selection;
- no tool call: answer normally without Tavily;
- one valid tool call: execute the configured provider, inject results, and
  generate the cited answer;
- planning/tool failure: fail closed to a normal model answer;
- sufficient Knowledge evidence: keep Web unavailable unless the question
  explicitly requires current public evidence.

`mm-chat` already has `ToolPlanner` implementations for OpenAI-compatible,
Gemini, and Anthropic providers. The smallest safe migration is to reuse that
native `tool_choice: auto` planner for one `search_web` definition and supply
bounded active-branch conversation context. This replaces both the current
always-search branch and the separate query rewriter with one decision/query
step. A full in-stream tool loop matching Kelivo exactly can be considered
later, but is a wider provider-interface change.

Required behavior tests:

- search disabled: zero planner and provider calls;
- enabled plus ordinary chat/writing: zero provider search calls;
- current facts or explicit search request: one provider search call;
- contextual follow-up: standalone query reflects recent conversation;
- malformed/failed planning: no provider search, normal answer continues;
- Knowledge sufficient: Web remains skipped for non-current questions;
- tool/query data is bounded and not persisted in message metadata.

## Locked product decisions

- The globe switch is the hard network boundary: off means no search planning
  and no Web provider request under any circumstances.
- With the globe enabled, an explicit user request to search the Web, check the
  latest information, or verify against an official site must force a search.
  Other requests remain eligible for automatic model/tool selection.
- With the globe enabled, current/public intent takes precedence over a
  sufficient Knowledge hit. The answer must fuse private Knowledge context
  with current Web evidence and keep their `[K#]` and `[W#]` citations
  distinct.
- Search planning, provider, timeout, and empty-result failures degrade to a
  normal model answer instead of failing the whole turn. The UI must briefly
  disclose that Web search was unavailable, no `[W#]` citation may be minted,
  and answers that depend on live facts must state that the latest value could
  not be confirmed. Backend diagnostics remain redacted.
- Implement a provider-normalized, server-side Tool Loop rather than a
  search-only preflight planner. With the globe enabled, the initial chat
  request exposes `search_web` using automatic tool choice; a tool call pauses
  answer generation, runs the selected search provider, appends the bounded
  tool result, and continues generation. The same loop is intended to support
  later first-party/plugin tools.
- The product must expose useful live progress for reasoning, Knowledge/RAG,
  Web search, tool calls, and answer generation. The exact boundary between
  provider-supplied reasoning and non-public model chain-of-thought is now
  fixed: render only reasoning content or summaries actually returned by the
  provider, never synthesize hidden reasoning. Separately render a factual
  execution trace for Knowledge retrieval, search, tools, citations, failures,
  and answer generation. System prompts, secrets, and internal safety rules
  are never exposed.
- Persist the rendered provider reasoning and a sanitized structured execution
  trace so they survive refresh and conversation switches. The retained trace
  may include phase/status/timing, displayed search query, redacted tool
  arguments, hit/source counts, failure category, and citation mapping. It must
  exclude credentials, authorization headers, raw provider payloads, full RAG
  chunks, system prompts, internal stack traces, and database details.
- After the Web tool loop is stable, migrate the selected Knowledge/RAG path to
  a first-class read-only `search_knowledge` tool. Register it only when the
  conversation has selected Knowledge collections; reuse the existing hybrid
  retrieval, reranker, evidence gate, and backend citation minting. Migrate and
  verify this as a separate committed group after `search_web`.
- The process panel is automatically expanded and live-updated while a turn is
  running. On completion it collapses to a one-line summary with duration and
  major stages. A user-expanded panel must not be forcibly collapsed or steal
  scroll focus, and ordinary answers with no reasoning/RAG/search/tool activity
  must not render an empty process panel.
- Tool execution is risk-gated. Read-only tools such as `search_web` and
  `search_knowledge` run automatically. Writes and external side effects require
  explicit user approval, while destructive/high-risk actions require stronger
  confirmation or remain disabled. Approval cards must support allow once,
  allow for the current conversation, and reject; approval is never required
  for the initial read-only retrieval tools.
- The owner explicitly chose not to impose product-level maximum tool rounds,
  per-tool counts, or total tool-call counts for this single-user deployment.
  The loop continues until the model stops calling tools, the user cancels the
  run, a provider/tool returns a terminal error, or the underlying request
  timeout/cancellation fires. Risk approval rules for side-effecting tools
  remain in force; they are independent from quota/count limits.
- When the current model/provider cannot perform native function calling, keep
  Web search available through a compatibility planner that uses the current
  selected chat model to return `shouldSearch` and a standalone query. Explicit
  search intent bypasses the decision and must search. The fallback must be
  identified in the process trace, must not silently switch models, and must
  degrade normally if planning fails.
- The globe control owns a strict three-state search mode: `off`,
  `model_builtin`, or `external`. Built-in and external search are mutually
  exclusive for a turn. External mode uses the configured selected external
  provider through `search_web`; built-in mode uses only the current model's
  native search capability. An unsupported built-in selection must be shown as
  unsupported and must not silently fall back to an external provider.
- Persist the three-state mode per conversation and restore it across page
  changes, refreshes, and restarts. A newly created conversation inherits the
  user's most recently selected mode. Changes are effective and saved
  immediately without a separate save action.
- Determine built-in search support automatically for known official OpenAI,
  Gemini, and Anthropic runtimes. A custom OpenAI-compatible model requires an
  administrator capability opt-in plus a bounded real connection test before
  built-in search becomes selectable. The globe menu keeps unavailable modes
  visible but disabled with a concrete reason; model-list naming alone is not
  treated as proof of capability.
- Keep provider reasoning controls independent from Search and Knowledge.
  Enabling either retrieval mode must not enable or change reasoning effort.
  When reasoning is disabled, the factual RAG/Search/Tool process remains
  visible but no reasoning text is fabricated; when enabled, only actual
  provider-returned reasoning is added to that process view.
- Keep retrieved sources distinct from cited evidence. Only current-turn,
  backend-minted markers actually present in the reconciled final answer render
  as inline Citation cards. Unused retrieval results remain inspectable only in
  the process panel; copied, fabricated, historical, and out-of-range markers
  are removed. A completed retrieval with no cited source is reported as such
  in the trace and must not append a synthetic source list to the answer.
