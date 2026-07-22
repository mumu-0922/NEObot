# G19 Provider-Native Tool Loop and Process Trace Process

## 2026-07-22 — G19.1 contract and owner lock

The owner reported that enabling the current globe causes external Search on
nearly every Knowledge-unavailable turn. Runtime tracing confirmed the cause:
`useSearch=true` reaches `planSourceFusion`, which sets `SearchRequested=true`
for non-Knowledge questions before the final model request. The recent
conversation-aware Query repair corrected what was searched but did not decide
whether Search was needed.

Kelivo was audited at commit
`545f7d67de250283232c9487aa5f4f42e85a7643`. Its external mode exposes one
`search_web(query)` function beside the retained conversation, sends
`tool_choice:auto`, accumulates streamed Tool Call arguments, executes the
selected provider, appends the provider-native Tool Result, and continues the
same model. When the model emits no Tool Call, no Search request occurs.

The audit also found that Kelivo generally uses `while (true)` until a provider
stops returning Tool Calls. OpenAI Chat Completions, Anthropic, Gemini, and
Vertex paths have no shared total-round or per-tool count. Only its OpenAI
Responses path blocks an identical Tool signature repeated for three
consecutive rounds. Multiple client Tool Calls are executed sequentially.

The owner completed a seventeen-question behavior lock:

- the globe is `off | model_builtin | external`, persisted per conversation,
  inherited by new conversations, and strictly mutually exclusive;
- off means no network planning or Search I/O; enabled modes are Auto, while
  explicit/current Search intent must Search;
- current/public questions may fuse Knowledge with Web evidence;
- Search failure degrades truthfully without fabricated `[W#]` citations;
- a provider-native generic Tool Loop is required, with current-model
  compatibility planning only when function calling is unsupported;
- the single-user deployment deliberately has no product-level Tool Round or
  Tool Call count limit;
- real provider reasoning and factual process steps are displayed separately,
  persisted in sanitized form, live-expanded, and collapsed on completion;
- Reasoning, Search, and Knowledge controls remain independent;
- read-only tools run automatically, while side effects require approval;
- model built-in and external Search never execute together;
- official built-in capabilities are explicit, while custom compatible models
  require administrator opt-in plus a real test;
- unused retrieval sources remain in the process view, while only final-answer,
  current-turn backend-minted markers render Citation cards;
- `search_web` is implemented before the existing RAG chain is promoted to
  `search_knowledge`.

G19.1 adds only research, contract, plan, process, and tracking documentation.
It does not change source code, database schema, Docker state, provider
configuration, credentials, network behavior, or live conversations.

Verification:

```text
Kelivo audit commit recorded                  passed
current Go forced-Search branch traced        passed
17 owner decisions captured                   passed
G19.1-G19.7 scope/rollback boundaries         passed
production code/schema/runtime changes        none
```

Next: implement G19.2 durable provider-reasoning/process-step events and the
reloadable process panel while leaving current Search and RAG execution
unchanged. Test, record, and commit G19.2 before beginning the Tool Loop.

## 2026-07-22 — G19.2 durable process-trace foundation

Go now normalizes provider-returned reasoning into
`ProviderEventReasoningDelta`. OpenAI Responses consumes
`response.reasoning_summary_text.delta`; OpenAI-compatible, including the
Gemini OpenAI surface, accepts string `reasoning_content`/`reasoning`; and
Anthropic accepts `thinking` block starts plus `thinking_delta`. Unsupported or
object-shaped compatible fields are ignored rather than rendered as invented
reasoning.

The chat stream now emits `reasoning.delta` and `process.step.updated` through
the existing run/message SSE wrapper. Both use the same monotonically
increasing sequence as content, usage, Search, and terminal events. The process
model has stable IDs, allowlisted kinds/statuses/details, bounded strings,
credential-pattern redaction, timings, and terminal transitions for completed,
failed, and cancelled runs. Terminal assistant metadata stores only sanitized
`reasoning` and `processTrace`; an ordinary successful Generation-only answer
does not retain an empty panel.

Quality review found that independent per-chunk redaction could miss a secret
whose label and value arrive in different provider events. The final stream
redactor therefore retains a bounded suffix, recomputes the sanitized prefix,
flushes it before answer content/terminal events, and fails closed if a later
chunk would rewrite already emitted text. A split API-Key plus split Bearer
fixture proves both live SSE and persisted reasoning contain only redaction
markers.

The unchanged legacy Knowledge and Search execution paths are projected into
the trace without changing their decision authority. Knowledge and external
Web finish before the assistant SSE is established, so G19.2 sends their first
snapshot as a completed/failed step immediately after `message.started`.
Provider reasoning, Generation, and provider-streamed built-in Search then
update live. This boundary is intentional: G19.3 owns live external
`search_web`, and G19.6 owns live `search_knowledge`.

The frontend validates both new event shapes, incrementally upserts steps in
the active assistant draft, and hydrates terminal reasoning/trace data from
server message metadata after reload or conversation switching. The new
process panel auto-expands while a step is active, auto-collapses when all
steps are terminal, preserves a manual expand/collapse choice, uses its own
bounded scroll area, and makes no chat-scroll or focus call. Existing
`ReasoningBlock` remains the fallback for messages without a process trace.

Verification:

```text
go vet ./...                                      passed
go test ./...                                     passed
go test -race ./...                               passed
go build ./cmd/api                                passed
pnpm format:check                                 passed
pnpm lint                                         passed
pnpm typecheck                                    passed
pnpm test                                         188 files / 899 tests passed
pnpm build                                        passed
focused backend process/provider/handler tests    passed
focused frontend process/API/store tests          4 files / 105 tests passed
git diff --cached --check (complete G19.2 scope)   passed
real provider, database, Compose, and quota use    none
```

Production build warnings remain the existing proxy-environment notice and
Next middleware-convention deprecation. The temporary Go build artifact was
deleted. Rollback is limited to removing the new provider event parsing, SSE
dispatch, message metadata projection, and process panel; existing assistant
content, Search/RAG decisions, citations, and database schema are unchanged.

Next: implement and independently commit G19.3 OpenAI-compatible/Gemini
external `search_web` Tool Loop with Auto no-search, explicit-search force,
same-model compatibility planning, provider-native continuation, and live Tool
steps.

## 2026-07-22 — G19.3 OpenAI-compatible/Gemini external Tool Loop

The external globe mode no longer performs pre-answer forced Search. Go now
offers `search_web(query)` to the selected OpenAI-compatible provider during
the initial model round, accumulates fragmented Tool Call names/arguments,
executes the single active external Search provider only after a valid call,
and continues the same model with assistant `tool_calls` plus matching
`role=tool` results. The configured Gemini integration uses Google's
OpenAI-compatible surface and therefore shares this continuation format;
native Gemini `functionCall`/`functionResponse` remains outside this group.

The provider seam now separates an ordinary content stream from a Tool-aware
round without changing existing providers. Tool arguments are capped at 64
KiB, unknown/malformed calls fail as sanitized process steps, every accepted
execution receives a stable Tool/Web step pair, and usage/content/reasoning
continue through the existing ordered SSE sequence. There is deliberately no
product-level Tool Round or Tool Call count limit; request cancellation,
provider deadlines, terminal errors, and the model stopping Tool Calls remain
the exit conditions.

Search authority is now the three-state `off | model_builtin | external`
contract. This group exposes `off` and `external` in the composer while
retaining `model_builtin` as a persisted value for G19.5. `off` performs zero
resolver, planner, built-in, or external Search I/O. External mode is Auto for
ordinary prompts, but explicit online/current intent forces a Search attempt.
If the selected provider rejects native function calling, the same current
model performs one bounded JSON `shouldSearch + query` compatibility plan; no
hidden model or alternate Search provider is used.

Citation projection was tightened at the terminal boundary. Go persists only
sources whose current-turn markers survive in the reconciled final answer,
preserves the originally minted marker instead of renumbering a retained
`[W2]`, and reconciles each completed Tool/Web step to either its actually used
`citationMarkers` or `completed_unreferenced`. A forced native first round that
ignores required Tool choice is buffered and discarded before compatibility
planning so its incorrect answer cannot be emitted twice.

Changed surfaces:

```text
backend/internal/chat/handler.go
backend/internal/chat/process_trace.go
backend/internal/chat/process_trace_runtime.go
backend/internal/chat/provider.go
backend/internal/chat/provider_openai_compatible.go
backend/internal/chat/provider_tool_round.go
backend/internal/chat/search_mode.go
backend/internal/chat/source_fusion.go
backend/internal/chat/web_search_context.go
backend/internal/chat/web_tool_loop.go
frontend/src/components/app/ChatApp.tsx
frontend/src/components/chat/MessageInput.tsx
frontend/src/config/defaults.ts
frontend/src/i18n/locales/{en,ja,zh}/MessageInput.json
frontend/src/lib/api/schemas.ts
frontend/src/lib/chat/entities.ts
frontend/src/lib/chat/searchMode.ts
frontend/src/lib/chat/types.ts
frontend/src/lib/settings/appConfig.ts
frontend/src/services/api/chatCrudService.ts
frontend/src/services/api/chatService.ts
frontend/src/store/core/chatStore.ts
frontend/src/types.ts
frontend/src/__tests__/{appConfig,chatAppServerModeComposition}.test.ts
frontend/src/__tests__/{chatEntities,chatStore,chatStoreServerRead}.test.ts
frontend/src/__tests__/{effectiveChatContext,messageInputComposition}.test.ts
frontend/src/__tests__/searchMode.test.ts
docs/contracts/chat-tool-loop.md
docs/tracking/g19-tool-loop-process-trace-plan.md
docs/tracking/progress.md
.trellis/spec/backend/chat-source-fusion.md
.trellis/spec/backend/chat-tool-loop.md
```

Verification:

```text
focused Tool Loop/Search-mode/provider/handler tests       passed
go vet ./...                                                passed
go test ./...                                               passed
go test -race ./...                                         passed
go build ./cmd/api                                          passed
pnpm format:check                                           passed
pnpm lint                                                   passed
pnpm typecheck                                              passed
pnpm test                                                   189 files / 901 tests passed
pnpm build                                                  passed
Backend/Frontend source build and health                    passed / healthy
```

The first real `gpt-5.6-sol` plus active Tavily replay made no Search request
for the ordinary turn and two model-requested Search calls for the explicit
contextual turn. After the final Citation-trace reconciliation build, a fresh
two-turn replay used an unambiguously static translation as the ordinary
control and an explicit official-document request as the positive control:

```text
ordinary terminal / Search events / Tool-Web steps          completed / 0 / 0
explicit terminal / Search events                           completed / 1
explicit live Tool-Web transitions                          running -> completed
final answer / persisted Search block                       [W1] / 1
reloaded Tool citationMarkers / Web citationMarkers         [W1] / [W1]
temporary conversation delete / subsequent message read     204 / 404
```

Two preflight conversations that intentionally exercised incomplete live
runtime descriptors failed closed with sanitized `PROVIDER_REQUIRED` responses
and were also deleted (`204`, then `404`). No provider/search configuration,
credential, existing conversation, schema, or Knowledge behavior was changed.

Rollback removes the G19.3 Tool-aware provider seam and restores the bounded
conversation-aware pre-answer external Search path while retaining the G19.2
reasoning/process foundation. Next: implement and independently commit G19.4
native Anthropic `tool_use`/`tool_result` continuation.
