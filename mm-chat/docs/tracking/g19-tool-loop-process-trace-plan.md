# G19 Provider-Native Tool Loop and Process Trace Plan

Status: in progress. G19 replaces the current pre-answer forced Web path with a
provider-normalized Tool Loop, adds an inspectable durable process trace, and
then promotes selected Knowledge retrieval into the same read-only tool
runtime. The work remains server-owned and is delivered one verified commit at
a time.

## Execution rule

- Deliver one bounded group at a time; test, record, and commit it before
  starting the next group.
- Keep the current Search/RAG path as rollback authority until the replacement
  path passes its real-provider and reload proof.
- Treat conversation history, Web results, Knowledge chunks, and tool output as
  untrusted data.
- Never persist credentials, authorization headers, raw provider payloads,
  system prompts, stack traces, or complete private Knowledge chunks.
- Real-provider smoke conversations, files, and temporary artifacts must be
  deleted after every group.
- Do not impose a product-level maximum Tool Round count, total Tool Call count,
  or per-tool invocation count for this single-user deployment. The loop ends
  when the model stops calling tools, the user cancels, a terminal error occurs,
  or the underlying request context ends.

## Locked behavior

### Search authority

- The composer globe owns exactly one persisted mode:
  `off`, `model_builtin`, or `external`.
- `off` is a hard network boundary: no Search planner, resolver, built-in
  Search, or external Search provider request may occur.
- `model_builtin` and `external` are strictly mutually exclusive.
- With either Search mode enabled, Search is Auto by default. Explicit user
  instructions to search, verify online, check an official site, or obtain
  current information must search.
- A current/public question may combine Knowledge and Web evidence even when
  Knowledge already has sufficient historical/internal evidence.
- Search failures degrade to an ordinary answer with a concise truthful notice;
  they never mint `[W#]` markers or silently change provider.
- The mode is saved immediately per conversation. New conversations inherit
  the user's most recently selected mode.

### Provider execution

- Tool-capable models receive provider-native Tool definitions in the initial
  chat request with automatic selection. A response without a Tool Call is the
  final answer; a Tool Call is executed and returned to the same model before
  generation continues.
- OpenAI-compatible and Gemini share the first implementation group; native
  Anthropic `tool_use`/`tool_result` follows in a separate group.
- A model that cannot perform native function calling uses the current selected
  model for a compatibility `shouldSearch + standaloneQuery` plan. It never
  switches to a hidden model.
- Known official OpenAI, Gemini, and Anthropic built-in Search capabilities are
  recognized by explicit runtime type. Custom OpenAI-compatible built-in Search
  requires an administrator opt-in and a real connection test.
- Unsupported or unconfigured modes remain visible but disabled with a concrete
  reason.

### Process and reasoning visibility

- Provider-returned reasoning and factual execution progress are distinct.
- Render only reasoning text or summaries actually returned by the provider;
  never synthesize or claim access to an unavailable private Chain-of-Thought.
- The process trace covers Knowledge retrieval, Web Search, Tool Calls, Tool
  Results, citations, failures, and answer generation with status and duration.
- While generation runs, the process panel is expanded and live. On completion
  it collapses to a one-line summary. Manual expansion must not be overridden
  or steal chat scroll focus.
- Persist the rendered reasoning plus a sanitized structured trace so both
  survive reload and conversation switching. Persist displayed queries,
  redacted arguments, counts, timings, failure categories, and citation maps;
  exclude secrets and raw internal payloads.
- Reasoning effort, Search mode, and Knowledge selection are independent. Tool
  execution never enables or changes reasoning effort.

### Tools, approvals, and citations

- G19 initially admits read-only `search_web`, followed by read-only
  `search_knowledge`.
- Read-only tools execute automatically. Future write/external-side-effect tools
  require explicit user approval; destructive tools require a stronger gate or
  remain disabled.
- Only current-turn, backend-minted `[K#]` and `[W#]` markers actually present in
  the reconciled final answer render Citation cards.
- Retrieved but unused sources remain inspectable in the process panel and are
  never appended as synthetic citations.

## G19.1 Contract, research, and sliced plan

Status: complete (2026-07-22).

- Audit Kelivo's external Tool Loop, provider continuation formats, process
  events, and termination behavior at an exact commit.
- Grill and freeze product decisions for Search authority, process visibility,
  persistence, approvals, citations, compatibility fallback, and rollout.
- Add the executable Tool Loop contract, this sliced plan, the process log, and
  the total progress entry.
- Make no production code, schema, runtime, provider, or UI behavior change.

## G19.2 Durable process-trace foundation

Status: complete (2026-07-22).

- Define provider-neutral reasoning and process-step event types with stable
  sequence ordering and terminal semantics.
- Persist a sanitized trace and rendered provider reasoning on the assistant
  message without persisting raw provider/tool payloads.
- Add the live expanded/completed collapsed process panel with stable scroll
  behavior, reload hydration, and no empty panel for ordinary answers.
- Preserve the existing Search and Knowledge execution paths in this group.

Promotion gate: fixture streams prove ordered live steps, reasoning separation,
terminal persistence/reload, cancellation, redaction, and manual expand/scroll
behavior through backend, frontend, and production builds.

Passed boundary: G19.2 emits live Provider reasoning and generation transitions,
hydrates the terminal trace after reload, and projects the unchanged legacy
Knowledge/external-Web stages as completed steps after `message.started`. It
does not claim that those pre-answer legacy stages are live. G19.3 moves
external Search into the streamed Tool Loop; G19.6 does the same for Knowledge.

## G19.3 OpenAI-compatible/Gemini external Tool Loop

Status: complete (2026-07-22).

- Extend the provider-neutral chat round with Tool definitions and normalized
  Tool Call events.
- Parse fragmented streaming arguments, execute `search_web` against the single
  active external provider, append the provider-specific Tool Result transcript,
  and continue with the same model until it stops calling tools.
- Expose `off` and `external` through the globe control while retaining the
  final three-state persisted data contract.
- Force explicit Search intent while retaining Auto selection for other turns.
- Add the current-model compatibility planner for models without function
  calling.

Promotion gate: focused and full Go/frontend gates plus a real DeepSeek/OpenAI-
compatible + Tavily contextual follow-up prove ordinary chat does not Search,
explicit/current questions do Search, Tool progress is visible, the final answer
streams, citations reload truthfully, and all smoke state is deleted.

Passed boundary: OpenAI-compatible and the Gemini Google OpenAI surface now
stream fragmented Tool Calls, execute live external Search, and continue with
native assistant `tool_calls` plus matching `role=tool` messages. Unsupported
Tool requests use a bounded same-model compatibility plan. The composer exposes
persisted `off` and `external`; `model_builtin` remains a valid data value for
G19.5. A real `gpt-5.6-sol` + Tavily two-turn replay made zero Search requests
for the ordinary turn, made two model-requested Search calls for the explicit
contextual turn, reloaded the used Citation and two Tool/Web trace pairs, and
returned `404` after deleting the temporary conversation. A final post-build
replay again proved ordinary zero-Search, one explicit live Search, `[W1]` in
both reloaded Tool/Web `citationMarkers`, one Search block, and `204 -> 404`
cleanup.

## G19.4 Anthropic Tool Loop

Status: complete (2026-07-22).

- Normalize Anthropic `tool_use` and `tool_result` without converting away
  provider-required block structure.
- Preserve provider-returned Thinking Blocks/signatures across continuation
  rounds while keeping the rendered reasoning boundary truthful.
- Prove cancellation, failures, multiple sequential rounds, usage aggregation,
  and reload.

Promotion gate: fixture coverage plus an owner-authorized real Anthropic proof
when an active tested credential exists. No other provider is used as fallback.

Passed boundary: Anthropic now streams fragmented `tool_use`, carries exact
ordered Thinking/signature and redacted-Thinking blocks through private
`round.completed` state, sends matching user `tool_result` blocks with
`is_error` on failure, and aggregates usage across unlimited native rounds.
Focused handler/provider fixtures prove live reasoning, Tool/Web progress,
Search, same-model continuation, reloadable `[W1]` trace markers, 64-KiB input
rejection, failures, cancellation, and multiple rounds. The live administrator
store contained zero active tested Anthropic providers, so the conditional real
Claude proof was correctly not attempted and no alternate provider was called
as Anthropic evidence. A post-build real `gpt-5.6-sol + Tavily` regression
proved the shared loop still performs ordinary zero-Search, explicit Search,
cumulative usage, Citation reload, and `204 -> 404` cleanup.

## G19.5 Three-state Search and built-in capability administration

Status: complete (2026-07-22).

- Complete the globe popover with `off`, `model_builtin`, and `external`.
- Persist the exact mode per conversation and the last-used inheritance source
  for new conversations.
- Connect official built-in Search capabilities and add administrator opt-in,
  protocol choice, and bounded real testing for custom OpenAI-compatible models.
- Keep unavailable options disabled with a reason and prevent built-in/external
  double execution.

Promotion gate: mode-transition, reload, model-switch, unsupported-capability,
and no-silent-fallback tests pass; provider-native positive fixtures pass; each
configured custom provider either passes its real source-bearing test or stays
disabled after a real negative test.

Passed boundary: the globe now exposes one persisted radio choice across all
three modes. New conversations inherit the latest server-owned mode, including
the first message created from the empty composer. Official OpenAI, Gemini, and
Anthropic built-in streams have provider fixtures; custom compatible providers
must pass an exact provider/model real test whose attestation is invalidated by
provider, type, Base URL, secret, protocol, or model changes. The deployed
compatible relay returned no Search sources for all four configured chat
models, so each real test failed closed and no attestation was retained. The
original provider state was restored. External Tavily remained independently
healthy and returned three sources; temporary mode/reload conversations were
deleted.

## G19.6 Knowledge Tool migration

Status: complete (2026-07-22).

- Register `search_knowledge` only when the conversation has selected
  collections.
- Reuse the active BM25 + pgvector, query-expansion, RRF, Jina reranker,
  authority hydration, no-evidence gate, and backend citation minting.
- Allow a model to perform Knowledge -> Web or Web -> Knowledge rounds without
  confusing `[K#]` and `[W#]` authority.
- Retire the old pre-answer Auto RAG Router only after parity, negative, and
  rollback proofs pass.

Execution slices:

- [x] G19.6A server-authoritative Knowledge executor and generic retrieval
  loop foundation. The model supplies only a bounded standalone Query; selected
  collection IDs remain server-owned. Native rounds can execute Knowledge ->
  Web or Web -> Knowledge, normal misses return a successful empty Tool Result,
  repeated Knowledge calls preserve stable cumulative `[K#]` markers, and live
  Tool/Knowledge trace steps carry counts rather than source bodies.
- [x] G19.6B connect the generic retrieval loop to chat streaming for selected
  Knowledge while retaining the legacy pre-answer path as a rollback seam.
- [x] G19.6C reconcile terminal Knowledge/Web authority, metadata, citations,
  cancellation, and reload across handler fixtures.
- [x] G19.6D remove the legacy pre-answer Auto RAG authority after fixture
  parity and pass the full static/test/build gates.
- [x] G19.6E pass the real uploaded-document hit/miss/follow-up/mixed/deletion/
  restart gate, clean all smoke state, and record rollback evidence.

Promotion gate: real uploaded documents prove hit, miss, contextual follow-up,
mixed current/public evidence, deletion invisibility, no false citation,
process reload, restart, and complete fixture cleanup.

## G19.7 Full live matrix and closure

Status: complete (2026-07-22).

- [x] Verify Search off, ordinary Auto no-search, explicit/current Search,
  contextual Query generation, Knowledge hit/miss, mixed Knowledge/Web,
  independent reasoning effort, compatibility planning, truthful citations,
  process reload, cancellation, provider failure degradation, and strict mode
  mutual exclusion.
- [x] Run full backend vet/test/race, frontend format/lint/typecheck/test/build,
  Compose source rebuild, restart, clean-copy, and runtime health gates.
- [x] Delete all temporary conversations/files/documents and record exact rollback
  anchors before marking G19 complete.

Promotion evidence: external Search `off` made zero Web calls; ordinary Auto
made zero Web calls; four contextual Queries resolved to `DeepSeek V4 Flash`;
reasoning `high` persisted independently from Search mode; Knowledge hit/miss
and mixed `[K1] + [W3]` passed; DeepSeek entered the same-model compatibility
path; unsupported model-built-in Search failed without external fallback;
live cancel persisted Message/Generation/Tool/Web as `cancelled` with no
`planner_failed` or Citation; restart preserved trace/markers; document
deletion was immediately invisible. Backend full vet/test/race/build,
Frontend 190-file/911-test/build gates, Compose health, and the full clean-copy
gate with Python 1730 passed/7 skipped all passed. Every temporary G19.7
conversation, File, Collection, and local artifact was removed.

## G19.8 Post-closure transient external Search recovery

Status: complete (2026-07-23).

- [x] Retry only the already-resolved external provider once after a bounded
  context-aware delay for transport `REQUEST_FAILED`, HTTP `408`, `429`, and
  `5xx`.
- [x] Do not retry authentication/other `4xx`, response/schema errors, or
  cancellation; do not re-resolve, switch provider, or change Citation truth.
- [x] Pass repeated unit, full Go, source-built Compose, real Tavily direct, and
  DeepSeek compatibility Tool Loop proofs; delete all temporary conversations.

Promotion evidence: the owner-visible failure ended near the existing
15-second response-header boundary while immediate Tavily repeats succeeded.
Focused tests passed 50 times; full Go vet/test/race/build passed; direct
Tavily returned five sources in three calls; the rebuilt Backend completed a
real `deepseek-v4-flash` compatibility Tool Loop with `[W1]`; cleanup returned
`204 -> 404`.

## Rollback order

1. G19.6 can restore the current Auto RAG preparation path while retaining the
   generic Tool Loop.
2. G19.5 can map persisted legacy `useSearch` back to off/external while
   retaining external Tool execution.
3. G19.4 can remove Anthropic Tool execution without affecting OpenAI-
   compatible/Gemini.
4. G19.3 can restore the current pre-answer external Search + standalone Query
   rewrite while retaining the inert process-trace foundation.
5. G19.2 can stop emitting/rendering the new trace while preserving existing
   assistant content and citation metadata.
6. G19.1 is documentation-only and has no runtime rollback.
