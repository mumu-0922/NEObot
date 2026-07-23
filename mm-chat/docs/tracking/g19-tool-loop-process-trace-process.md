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

## 2026-07-22 — G19.4 native Anthropic Tool Loop

`AnthropicProvider` now implements the same `ToolRoundProvider` seam without
converting Claude's continuation into an OpenAI transcript. It maps function
definitions to Anthropic `tools`, accumulates fragmented `input_json_delta`
payloads by content-block index, emits normalized Tool Call completion only at
block stop, and caps accumulated input at 64 KiB before execution.

Each completed Anthropic round carries a server-private continuation snapshot
containing only allowlisted ordered assistant blocks. The next request replays
provider-returned `thinking` plus signature, `redacted_thinking`, `text`, and
`tool_use` in their original block order, followed by one user message with
matching `tool_result` blocks. Failed calls set `is_error=true`. Signatures and
redacted-Thinking data never enter public SSE details or persisted message
metadata; only rendered reasoning continues through the existing redactor.

Explicit external Search without Thinking uses a named `search_web`
`tool_choice`. Anthropic extended Thinking uses `auto` for protocol
compatibility; because the forced first round remains buffered, a no-Tool
response still moves to the same-model compatibility path without leaking a
duplicate answer. The generic loop now also reports cumulative usage across
native rounds while treating repeated updates inside one round as a snapshot,
not an additive delta.

Changed surfaces:

```text
backend/internal/chat/provider.go
backend/internal/chat/provider_tool_round.go
backend/internal/chat/provider_anthropic.go
backend/internal/chat/provider_anthropic_test.go
backend/internal/chat/web_tool_loop.go
backend/internal/chat/web_tool_loop_test.go
backend/internal/chat/handler_test.go
docs/contracts/chat-tool-loop.md
docs/tracking/g19-tool-loop-process-trace-plan.md
docs/tracking/g19-tool-loop-process-trace-process.md
docs/tracking/process.md
docs/tracking/progress.md
.trellis/spec/backend/chat-tool-loop.md
```

Verification:

```text
Anthropic fragmented Tool/Thinking/signature continuation fixture   passed
Anthropic named/Thinking Auto Tool choice fixtures                  passed
Anthropic failure is_error / 64-KiB / cancellation fixtures         passed
two native Search rounds plus cumulative usage fixture              passed
handler live SSE/Search/persistence/reload fixture                  passed
go vet ./...                                                        passed
go test ./...                                                       passed
go test -race ./...                                                 passed
go build ./cmd/api                                                  passed
Backend source image rebuild/restart                                healthy
```

The administrator provider list contained two configured providers and zero
enabled, connection-tested Anthropic providers. The promotion gate makes a real
Claude call conditional on such a credential, so no Anthropic quota was called
and no other model was misrepresented as Claude evidence. No credential or
provider configuration was changed.

After deploying the source-built Backend, a real shared-loop regression used
the existing `gpt-5.6-sol` plus active Tavily configuration:

```text
ordinary terminal / Search / Tool-Web IDs        completed / 0 / 0
explicit terminal / Search / Tool-Web IDs        completed / 1 / 2
explicit final cumulative usage                  9,390 tokens
reloaded answer / Search block                    [W] present / 1
temporary conversation cleanup                   204 -> 404
```

Rollback removes Anthropic `StreamToolRound`, the private round-state replay,
and cross-round usage aggregation while retaining G19.3 OpenAI-compatible
execution. Next: implement and independently commit G19.5 three-state Search
and built-in capability administration.

## 2026-07-22 — G19.5 three-state Search and built-in capability administration

The composer globe now renders one strict radio selection for `off`,
`model_builtin`, or `external`. Official OpenAI, Gemini, and Anthropic chat
models are recognized with Responses Web Search, native Gemini
`google_search`, and Anthropic `web_search_20250305` respectively. Unsupported
models and unavailable configurations remain visible but disabled with a short
reason. External and built-in resolver methods are separate, so selecting one
mode cannot scan, execute, or fall back to the other.

Custom OpenAI-compatible providers expose a server-managed administrator
opt-in. The administrator selects one persisted model and runs a real bounded
`openai_responses` Search test. A positive result must contain at least one
provider Search source before Go commits the attestation. Its fingerprint binds
provider ID/type, normalized Base URL, encrypted secret reference, protocol,
and exact model; the Postgres commit uses a compare-and-set update so concurrent
configuration changes reject the result. No key or attestation is browser
authority.

Conversation writes normalize the exact three-state mode and the legacy
`useSearch` mirror. A create request without either field inherits the first
valid mode from the server's latest-conversation ordering. The frontend also
re-reads the newly created server session before sending its first message, so
that first generation cannot accidentally use the pre-create `off` render
snapshot.

Changed surfaces:

```text
backend/internal/websearch/{types.go,service.go}
backend/internal/runtimeconfig/{types.go,service.go,handler.go,repository_postgres.go,model_built_in_search.go}
backend/internal/chat/{handler.go,service.go,search_mode.go,provider_gemini.go,provider_gemini_search.go,provider_anthropic.go}
backend/internal/httpserver/server.go
frontend/src/components/{app/ChatApp.tsx,chat/MessageInput.tsx,settings/ProviderSettings.tsx}
frontend/src/lib/{chat/searchCapabilities.ts,providers/config.ts,providers/types.ts}
frontend/src/services/api/client/{types.ts,server/providerApi.ts,local/providerApi.ts}
frontend/src/i18n/locales/{zh,en,ja}/{MessageInput.json,Providers.json}
```

Verification:

```text
focused authority/provider/route/frontend tests                     passed
disposable Postgres attestation commit/stale-config proof           passed; database dropped
go vet ./...                                                        passed
go test ./...                                                       passed
go test -race ./...                                                 passed
go build ./cmd/api                                                  passed
pnpm format:check / lint / typecheck                                passed
pnpm test                                                           190 files / 911 tests passed
pnpm build                                                          passed
Backend/Frontend source image rebuild                               passed
Backend/Frontend/Postgres health                                    healthy
mode persist -> inherit -> external update -> inherit -> reload     passed
temporary mode-proof conversations                                  3 deleted; 0 remain
real external Tavily regression                                     3 sources / 3 images
real custom compatible built-in tests                               4 models rejected; 0 attested
provider built-in configuration after smoke                         restored
```

The live compatible relay did not return provider Search sources for any of
its four configured chat models. This is a valid negative capability result,
not a positive official-provider proof: the new route returned the stable
failure boundary and retained no attestation. No official OpenAI, Gemini, or
Anthropic credential is currently configured, so their positive live calls
remain conditional; native HTTP/provider fixtures cover their promoted code
paths without mislabeling the relay as official evidence.

Rollback removes the three-state menu addition, native built-in provider
adapters, and custom attestation fields while preserving G19.3/G19.4 external
Tool execution. Persisted JSON needs no schema rollback; legacy `useSearch`
continues to map to `off|external`. Next: implement and independently commit
G19.6 Knowledge Tool migration.

## 2026-07-22 — G19.6A Knowledge executor and retrieval-loop foundation

The first Knowledge migration slice is intentionally not promoted through the
chat Handler yet. It adds the server-owned `search_knowledge` definition and
executor beside the existing external Web Tool runtime while leaving the
legacy pre-answer Auto RAG path as current rollback authority.

The Tool schema exposes only one bounded standalone `query`; collection IDs
are copied from the authenticated conversation selection and never accepted
from model arguments. Execution reuses the active `RAGAnswerAssembler` and
answer-governance gate, so candidate retrieval, reranking, final hydration
reauthorization, deletion visibility, and citation minting retain their
existing authority boundaries. A normal no-evidence result is returned to the
model as `ok=true` with an empty source list. Dependency or governance failure
returns no private evidence and is marked as a failed Tool execution.

The provider-native loop can now register Web and Knowledge together, execute
them in either order over unlimited rounds, and retain separate `[W#]` and
`[K#]` namespaces. Repeated Knowledge calls deduplicate by backend citation ID,
preserve already-issued markers, and cap cumulative Knowledge authority at the
existing eight-citation boundary. Provider continuation receives only bounded
snippets/locators; process persistence receives Query, status, counts,
reranker status, and used marker IDs, never complete Knowledge bodies.

Verification passed:

```text
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/api
git diff --check (G19.6A scope)
```

Fixtures cover server-only collection authority, hit, successful miss,
governance fail-closed, stable repeated markers, Knowledge -> Web, Web ->
Knowledge, independent process steps, and unused-marker reconciliation. Next:
wire the generic retrieval loop into authenticated chat streaming and keep the
old pre-answer route as an explicit rollback seam until handler parity passes.

## 2026-07-22 — G19.6B Tool-capable Handler cutover

Authenticated chat streaming now selects live Knowledge execution when the
current provider implements the normalized Tool-round contract and Search is
`off` or `external`. With Search off the model receives only
`search_knowledge`; with external Search it receives Web first plus Knowledge,
so explicit Search still names Web while Auto turns may choose either order.
No Knowledge Tool is registered without a selected collection.

The Handler builds Knowledge runtime authority from the authenticated user and
session, current conversation, selected server collection IDs, and the
resolved answer-governance processor/model. The model supplies only a Query.
The current user text and the model-resolved standalone Query feed the existing
original/rewrite lanes, preserving contextual Query Expansion before active
BM25/pgvector fusion, Jina reranking, and final hydration reauthorization.

Live terminal execution updates the current-turn Knowledge decision used by
source-marker reconciliation, message metadata, Knowledge/Web fusion
authority, and process persistence. A hit can become Knowledge-only or mixed;
a miss remains a successful `no_evidence` step; unused or failed-answer
Knowledge markers are removed from both Citation metadata and Tool/Knowledge
trace marker maps. Provider failure after retrieval therefore cannot persist a
false Citation. Cancellation continues through the same request context into
candidate retrieval.

Focused Handler/loop fixtures now prove:

```text
Search off + selected Knowledge -> live Tool/Knowledge hit + reload
Knowledge miss -> successful empty result + no false [K1]
contextual follow-up -> original/rewrite retrieval lanes
runtime provider processor -> answer-governance authority
Knowledge -> Web and Web -> Knowledge -> isolated [K1]/[W1]
provider failure -> completed_unreferenced + zero Citation
in-flight Knowledge cancellation -> retrieval context cancelled
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/api
```

The old pre-answer Auto RAG path is deliberately retained only for non-Tool
providers and model-built-in Search while the remaining parity and rollback
proofs run. Next: close terminal fixture parity, decide the built-in mixed
adapter boundary, then remove this compatibility authority before live
promotion.

## 2026-07-22 — G19.6C/D terminal parity and pre-answer retirement

The Handler no longer calls `decideAutoRAG`, builds Knowledge evidence, or
projects a completed Knowledge step before `message.started`. The dead
`decideAutoRAG`, answer-authorization wrapper, and legacy Knowledge trace
projection were removed. All selected-Knowledge execution now begins inside an
SSE event source.

Tool-round-capable `off|external` turns retain native model-selected
`search_knowledge`. Non-Tool providers and `model_builtin` use a bounded live
compatibility executor: it emits running Tool/Knowledge steps, applies the same
server-selected collection and governance authority, and then continues into
the existing same-model external planner, built-in Search provider, or ordinary
answer stream. Contextual questions retain the original/rewrite retrieval
lanes. A built-in startup failure emits a failed Web step and continues with
Knowledge plus a truthful Web-unavailable instruction; it does not restore
pre-SSE retrieval or switch providers.

Terminal reconciliation now runs for success, provider failure, and
cancellation. Knowledge metadata is completed against the terminal content,
fusion authority is recomputed from actually used current-turn `[K#]/[W#]`
markers, and unused trace marker maps become `completed_unreferenced` before
persistence. A provider failure after a successful retrieval therefore stores
no false Citation and no false Knowledge fusion authority.

Fixture coverage includes:

```text
message.started -> live compatibility Tool/Knowledge running/completed
non-Tool Knowledge -> same-model external planner -> mixed [K1]/[W1]
model-built-in Knowledge -> built-in Search -> reload of both authorities
model-built-in startup failure -> Knowledge-only ordinary fallback
normal miss / reranker failure / governance failure -> no evidence leak
provider failure and cancellation -> terminal Citation reconciliation
```

Full source gates passed from `mm-chat/backend`:

```text
gofmt -d (G19.6C/D Go scope)
go vet ./...
go test ./...
go test -race ./...
go build -o /tmp/mm-chat-api-g19.6cd ./cmd/api
git diff --check (G19.6C/D scope)
```

Rollback is now code-level: revert this group to restore the retained G19.6B
pre-answer compatibility seam. Runtime contains no configuration switch that
can silently reactivate the retired Handler authority. Next: Compose source
rebuild and the real uploaded-document promotion matrix.

## 2026-07-22 — G19.6E real-document promotion

The current source-built Backend image was rebuilt with the explicit
`.env.single-server` and `compose.single-server.yml` boundary, recreated, and
returned healthy. The smoke then created one isolated personal collection,
uploaded one disposable native-text document, and waited for the real Jina
1024-dimensional indexing pipeline to publish an active version. Existing
collections and conversations were not mutated.

The live Server Default `gpt-5.6-sol` and active Tavily provider exercised the
promoted runtime. Knowledge-only retrieval returned the private sentinel with
one `[K1]` Citation. A contextual follow-up resolved “it” against conversation
history and returned a second document fact with one `[K1]`. An unrelated
question completed with zero `[K#]` markers and zero Citations. The mixed turn
executed Web, Knowledge, then Web again and retained one `[K1]` plus the
actually used `[W3]`; the surviving Web marker was not renumbered.

The first direct-API attempt omitted the non-secret Server Default provider
descriptor that the normal frontend sends and correctly failed before SSE with
`PROVIDER_REQUIRED`. Supplying `source=server-default` exercised the same
server-stored credential path as the UI; no Key or provider configuration was
copied into the harness, and no code change was needed.

Reload before and after a Backend restart preserved completed Tool/Knowledge/
Web steps and the same terminal marker/Citation authority. Deleting the
document returned `204`, its immediate read returned `404`, and a fresh
selected-collection turn could no longer recover the sentinel or mint a
Knowledge Citation. Cleanup deleted all four temporary conversations, the
File, and the Collection; resource reads returned `404`, prefixed collection
and conversation listings returned zero, and all local smoke artifacts were
removed.

Promotion evidence:

```text
Backend source build / recreate / readiness                 passed / healthy
native text upload -> real Jina 1024 index                  active
Knowledge hit / contextual follow-up                        [K1] / [K1]
unrelated miss                                              0 [K], 0 Citation
Knowledge + Tavily mixed Tool Loop                          [K1] + [W3]
mixed execution order                                       Web -> Knowledge -> Web
reload / Backend restart / post-restart reload              passed / healthy / passed
document delete / immediate read / post-delete retrieval    204 / 404 / no evidence
temporary conversations / File / Collection                deleted / 404
prefixed server state / local artifacts                     zero / zero
```

G19.6 is promoted. Rollback remains the reverse application of G19.6D through
G19.6A; no schema, provider setting, secret, or retained user resource changed
during this promotion. G19.7 owns the complete cross-provider/live/clean-copy
closure matrix.

## 2026-07-22 — G19.7 full live matrix and closure

The final matrix exercised the deployed source-built runtime rather than only
fixtures. Search `off` made zero Web calls. External Auto skipped an ordinary
request, while four contextual Search requests resolved their subject to
`DeepSeek V4 Flash`. Reasoning effort `high` and Search mode remained
independently persisted. Knowledge hit/miss, mixed Knowledge/Web, used-marker
reconciliation, process reload, and immediate document deletion visibility all
retained the G19.6 authority.

The provider boundary covered both native and compatibility behavior. Real
DeepSeek entered `mode=compatibility`. An unattested `model_builtin` request
recorded a failed Web step, completed an ordinary answer, emitted no `[W]`, and
did not fall back to the active external provider. Backend restart preserved
the same terminal trace and citation markers. Every temporary conversation,
File, Collection, and local smoke artifact was deleted; prefixed server
resource listings returned zero.

The first cancellation replay exposed one final truthfulness defect: the
assistant Message and Generation were `cancelled`, but compatibility Tool/Web
steps were persisted as `failed / planner_failed`. The compatibility planner
was treating every error as degradation, `toolProcessDetail` had no cancelled
outcome branch, and Handler Web diagnostics had no cancelled case. The fix in
`94b50f0` now:

- recognizes `context.Canceled` and cancelled operation contexts across the
  compatibility planner, native/compatibility Web, and Knowledge execution;
- emits terminal cancelled Tool/source events without a failure category and
  keeps that event deliverable after operation-context cancellation;
- persists `detail.outcome=cancelled` and treats a cancelled Tool event as
  message-cancellation authority; and
- prevents cancellation from writing degraded Search diagnostics or starting
  ordinary fallback/continuation.

Focused cancellation tests ran repeatedly to cover event-delivery races. The
source-built Backend was then rebuilt and recreated. A real
`deepseek-v4-flash` compatibility-planner cancellation returned one
`message.cancelled` and reloaded exactly Generation, Tool, and Web steps, all
`cancelled / outcome=cancelled`, with zero `failureCategory`, zero output
blocks, zero citation markers, and no `planner_failed`. Both cancellation
conversations and their local capture state were deleted; no `G19.7`-prefixed
conversation remained.

Final evidence:

```text
Search off / external Auto ordinary                        zero Web / zero Web
contextual Search Queries                                  4, all DeepSeek V4 Flash
reasoning high / Search mode                               independently persisted
Knowledge hit / miss / mixed                               [K1] / zero / [K1] + [W3]
DeepSeek execution                                         compatibility
unattested model_builtin                                   failed closed; no fallback
live compatibility cancel                                 Message/Generation/Tool/Web cancelled
cancel failure category / Citation                        none / zero
restart trace and marker reload                            passed
document delete / immediate retrieval                     204 / 404
Backend gofmt/vet/test/race/build                          passed
Frontend format/lint/typecheck/test/build                  190 files / 911 tests / passed
Compose source rebuild / Backend health                    passed / healthy
clean-copy Go/Frontend/Python                              passed / passed / 1730 passed, 7 skipped
temporary G19.7 server/local state                         zero
```

G19 is complete. Rollback anchors remain the ordered G19.6 -> G19.2 reversions
in the plan; the cancellation-only repair can be reverted independently with
`94b50f0`. No schema, secret, administrator provider setting, retained user
conversation, or retained Knowledge object changed during closure.

## 2026-07-23 — G19.8 transient external Search recovery

The owner-visible `西安天气预报` turn reached the correct DeepSeek compatibility
planner and correct Tavily Query, then failed after about 16 seconds with
`provider_failed`. The active provider remained enabled, Key-backed, and
attested. A fresh administrator test returned `200`, and three immediate direct
Tavily calls returned five relevant sources in roughly 1.7 seconds each. The
failure duration matched the secure client's 15-second response-header timeout,
so the defect was transient-recovery policy rather than routing, model, Query,
or provider configuration.

External Search now makes at most two attempts against the exact same resolved
execution. Only `REQUEST_FAILED`, HTTP `408`, `429`, and `5xx` trigger the one
250 ms context-aware retry. Authentication/other `4xx`, content type, encoding,
size, decode/schema failures, and cancellation return immediately. A second
failure returns its final redacted error to the existing truthful degradation
path; no provider fallback or Citation behavior changed.

Verification evidence:

```text
focused websearch tests, count=50                         passed
Go vet / test / race / build                             passed
source Backend rebuild / recreate / health               passed / healthy
real Tavily direct Search, three calls                    200 / 5 sources each
real DeepSeek compatibility Tool/Web                     completed / [W1]
temporary conversation delete / post-delete read         204 / 404
```

No administrator configuration, secret, retained conversation, or provider
selection changed. Rollback is the isolated external retry code/doc commit;
the pre-existing degradation path remains intact.

## 2026-07-23 — G19.9 native Tool continuation recovery

The owner-visible `洛阳天气预报` turn was inspected by persisted message and
process state rather than by its generic frontend error. Both Tavily executions
completed and returned five sources, but the third `gpt-5.6-sol` provider round
failed with `provider stream failed`. The assistant persisted `failed` with
zero answer bytes and 36 reasoning characters. The Search provider, Query
routing, and external retry path were therefore not the failing boundary.

One disposable replay reproduced the same two-Web-then-third-round failure and
was deleted. A temporary redacted error-classification probe was then deployed;
four consecutive real replays succeeded, so it did not capture a more precise
upstream transport category. The probe was removed before the final source
build. An intermittent upstream continuation interruption remains the narrow
runtime cause, while code review found a deterministic amplification defect:
each later Web Tool Result reserialized the complete cumulative source corpus,
even though all earlier Tool Results were already present in native
continuation history.

G19.9 now returns only sources newly minted by each Web execution and preserves
their cumulative markers. If a later native continuation fails synchronously
or in-stream after authorized Web/Knowledge evidence exists and before any
answer content was emitted, the same provider/model performs one no-Tools
answer stream from the bounded cumulative evidence. Existing backend marker
reconciliation remains citation authority, cumulative usage includes prior
native rounds, and cancellation/no-evidence/partial-answer failures do not
recover.

Regression coverage includes:

```text
in-stream continuation failure + Web evidence                 recovered
synchronous continuation start failure + Web evidence         recovered
Knowledge then Web continuation failure                       [K1] + [W1]
failure after partial answer content                           no recovery
repeated Web source                                            sources: []
new source after prior result                                  stable [W2]
fallback usage                                                 cumulative
focused retrieval fixtures, count=20                           passed
complete internal/chat package, count=50                       passed
Go vet / test / race / build                                   passed
standalone Frontend / Go                                       190 files, 911 tests / passed
standalone Python                                              1,730 passed, 7 skipped
```

The source Backend was rebuilt and recreated and returned healthy readiness.
A real Server Default `gpt-5.6-sol + Tavily` stress replay then completed with
20 distinct Search Queries, 21 durable Tool steps, 21 durable Web steps, 278
answer deltas, six cumulative usage updates, and one terminal
`message.completed`. The reconciled answer retained only three actually used
sources with `[W4]`, `[W5]`, and `[W6]`; its temporary conversation was deleted
and the subsequent read returned `404`. No owner conversation, provider/search
configuration, or secret was changed.

The stress replay lasted 295,914 ms and emitted 1,586,274 SSE bytes. This is
direct evidence that the owner-selected no-product-limit policy can let the
model consume most of `PROVIDER_TIMEOUT=5m`; the model made far more calls than
the two required for regression proof. G19.9 intentionally does not override
that product decision. The remaining operational risk is timeout/cost growth
from model-directed Tool loops, not silent provider switching or fabricated
Citation authority.

Root-cause classification is **D: test coverage gap** plus **E: implicit
assumption**. Multi-round success fixtures existed, but no fixture interrupted
a continuation after successful evidence, and the implementation assumed the
upstream continuation would finish. Prevention is now structural incremental
Tool Results, explicit same-model pre-content recovery, negative partial-answer
tests, mixed-evidence tests, cumulative-usage tests, and this executable
contract. Rollback is the isolated G19.9 code/doc commit; G19.8 remains intact.
The code rollback anchor is `e57675e`.
