# Chat Tool Loop contracts

Status: G19.2 process trace, G19.3 OpenAI-compatible/Gemini external Web, G19.4
Anthropic Tool Loop, G19.5 three-state/built-in Search administration, and
G19.6 server-authoritative Knowledge Tool migration were promoted on
2026-07-22. G19.9 continuation recovery was promoted on 2026-07-23. All
selected-Knowledge paths now execute after `message.started`; the Handler no
longer invokes the old pre-answer Auto RAG authority. G19.10 query-aware
Knowledge routing, unified compatibility planning, shared Tool-capability
state, and query-free route visibility were promoted on 2026-07-23. The
historical schema-v6 `search_memory` `PlanTools` preflight failed Development
and remains immutable failed evidence. The successor now joins the existing
first `ToolRoundProvider` round, executes bounded current-authorized hybrid
Memory only after a valid call, and continues on the same Provider/model.
Historical schema-v7 GPT and DeepSeek Flash routing results failed unchanged
gates. The owner later and separately promoted the passing schema-v14 fixed
BGE-then-Luna candidate-selection semantics for this product Tool only.
`MEMORY_TOOL_LOOP_ENABLED` remains default-off and is the global rollback
boundary. `MEMORY_TOOL_LOOP_CANARY_USER_IDS` is an independent API-only exact
UUID admission set and defaults empty; there is no v1 fallback. The frozen
production-v2 guard/buffered candidate failed its sole schema-v18 Validation,
so no live canary is authorized.
Schema v8 introduced bounded typed Provider/Tool failure categories without
changing the product Tool request or runtime flag, but two live attempts
published no artifact; the second stopped at bounded `admission_state`. The
schema-v9 route-only successor retains route categories independently from
fail-closed retrieval-completeness aggregates. It is Development-only, retains
no upstream body/error text, and has no policy-selection authority. Its first
live report completed only `12/300` routes, classified the `288` failures into
`31` context deadlines, `83` invalid Tool Calls, and `174` unclassified router
failures, and kept Validation/Promotion blocked.

## Scenario: Conversation-scoped Run admission and projection

### 1. Scope / Trigger

Apply when changing chat/image streaming, active Run cancellation/retention,
Conversation list DTOs, detached execution, or multi-Conversation scheduling.

### 2. Signatures

```go
reserve(runID, userID, conversationID, messageID string) (release func(), ok bool)
attach(runID string, cancel context.CancelFunc, stream *activeRunStream) bool
activeForUser(userID string) []activeConversationRun
```

```json
{
  "activeGeneration": {
    "runId": "uuid",
    "messageId": "uuid",
    "status": "pending|streaming"
  }
}
```

### 3. Contracts

- Run ownership key is `(userId, conversationId)`, not Workspace and not one
  process-global UI flag. One Conversation admits one active Run; separate
  Conversations may execute concurrently.
- Reserve before creating/starting the Assistant runtime, attach the exact
  cancel function when streaming begins, and release on every terminal/error
  path. Image and text generation obey the same admission rule.
- HTTP/SSE disconnect does not cancel accepted work. Only the exact Run cancel
  endpoint or a runtime deadline invokes the registered cancel function.
- `GET /v1/chat/conversations` projects only the requesting user's active Runs.
  `pending` means reserved but not attached; `streaming` means cancellation is
  attached. Finished retained SSE streams are never projected as active.
- The projection is process-local by design; restart recovery finalizes durable
  non-terminal Messages through the existing startup recovery contract.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| active `(user, Conversation)` already reserved | `409 CONVERSATION_RUN_ACTIVE` |
| reservation cannot be attached | `409 CONVERSATION_RUN_UNAVAILABLE` and release/finalize |
| same user, different Conversation | admit concurrently |
| different user, same opaque Conversation ID | separate user scope; repository ACL still applies |
| finished retained stream | replayable by Run endpoint but absent from active projection |
| explicit exact Run cancel | cancel only that Run and emit one terminal cancellation |

### 5. Good / Base / Bad Cases

- Good: two Conversations in one Workspace run concurrently and both remain
  independently cancellable/projected.
- Base: one Conversation has one pending/streaming Run and finishes normally.
- Bad: `map[runId]` tracks cancellation but admits duplicate Conversation Runs,
  or list projection leaks another user's Run.

### 6. Tests Required

- Registry unit coverage for duplicate rejection, sibling admission,
  pending-to-streaming transition, exact release, and user-filtered projection.
- Handler list coverage for `activeGeneration` fields and terminal omission.
- Text and image Handler regressions for detached completion, explicit cancel,
  one terminal result, and `409` admission conflicts.
- Full Backend `go vet ./...` and `go test ./...`; cross-layer changes also run
  frontend DTO/store/concurrency tests.

### 7. Wrong vs Correct

```go
// Wrong: Run IDs are unique, but one Conversation can still start twice.
activeRuns.register(runID, cancel)

// Correct: Conversation admission precedes runtime attachment.
release, ok := activeRuns.reserve(runID, actor.ID, conversationID, messageID)
if !ok { return conflict }
defer release()
activeRuns.attach(runID, cancel, stream)
```

## Scenario: Continue a Chat Agent Goal through natural Tool completion

### 1. Scope / Trigger

Apply when changing migration `097`, Goal Tool definitions, automatic
continuation, Provider follow-up framing, no-progress handling, or terminal
wrap-up behavior.

### 2. Signatures

```text
get_goal({})
create_goal({objective,maxGoalRounds})
update_goal({goalId,revision,action,objective,maxGoalRounds,blockedReason})

chat_agent_create_goal(...)
chat_agent_change_goal(...)
chat_agent_cancel_goal(...)
chat_agent_start_goal_round(...)

ProviderToolExchange.FollowupPrompt
```

`verify_completion` is historical replay data only. It is not a definition,
dispatch name, Result field, or visible process row for a new Turn.

### 3. Contracts

- Register Goal Tools only for a native-Tool model when the Repository supports
  `ChatAgentGoalRepository`. Store at most one current Goal per Conversation;
  every mutation uses exact Goal ID/revision CAS.
- Activation is process-local. Each HTTP request starts disarmed; only a direct
  human request may create/edit/pause/resume/cancel. Restored active state does
  not self-start. Automatic blocked requires the same blocker through round 3.
- Goal round budgets are 3-32, default 8. When armed work would otherwise stop,
  start the exact next round and append `assistant -> synthetic user` through
  `FollowupPrompt`; preserve provider-native Thinking state.
- The ordinary Agent loop is natural: any Tool Call continues on the same model
  and a model response with no Tool Call ends the Turn. Tool Results are the
  execution facts. Do not force a self-verification Tool or expose Provider
  call IDs as evidence fields.
- Goal complete/blocked/cancel latches a Tool-free wrap-up. A hallucinated Tool
  call receives `goal_concluded`, executes nothing, and cannot restore Tools.
- Effective Agent mode has no whole-Turn deadline or fixed Step/Tool cutoff
  while outcomes make progress. Retain per-Tool timeout, cancellation,
  approval, output/concurrency bounds, Provider failure, and the three-repeat /
  five-all-error no-progress guard.
- Goal process events contain only Tool name/round, `mode=goal`, classification,
  duration/status/failure category. Frontend normalization drops historical
  `verify_completion` rows without invalidating the rest of the transcript.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| absent Goal repository | no Goal Tools |
| bad strict arguments / unknown Goal | bounded Tool error; no mutation |
| stale revision / wrong exact round | `stale_revision` / `round_invalid`; no event gap |
| non-human create/edit/pause/resume/cancel | `human_authority_required` |
| automatic blocked before round 3 | `blocked_round_threshold` |
| model emits no Tool Call after a successful write/execute | natural final answer; no injected follow-up |
| completion-driven Agent progresses beyond legacy budgets | continue on the same Provider/model |
| three identical outcomes or five all-error rounds | Tool-free blocked wrap-up; no success claim |
| Goal persistence failure | terminal `AGENT_GOAL_PERSISTENCE_FAILED` |
| wrap-up Provider emits Tool Call | `goal_concluded`; no dispatch; Tools remain disabled |

### 5. Good / Base / Bad Cases

- **Good:** `create_goal -> read/write/bash -> update_goal(complete) ->` Tool-free
  final answer, with only real results and meaningful remaining issues stated.
- **Base:** a routine one-Turn task uses no Goal and naturally ends after its
  last Tool Result.
- **Bad:** force `verify_completion`, inject a verification-only grace loop,
  narrate ritual evidence, arm a restored Goal on startup, or mark blocked in
  the first automatic round.

### 6. Tests Required

- Strict Goal definitions exclude `verify_completion`; default has no Subagent.
- Foreground `bash` and structured writes continue once, then accept a Tool-free
  final answer without synthetic verification prompts or evidence fields.
- Automatic continuation, CAS/human authority, blocker floor, round cap,
  wrap-up Tool disabling, no-progress blocking, and PostgreSQL Goal replay stay
  covered.
- Historical `verify_completion` ProcessSteps load but are omitted from the
  user-visible frontend timeline.

### 7. Wrong vs Correct

```text
Wrong: write -> check -> copy call ID -> verify_completion -> final narration
Correct: write/check Tool Results -> same-model continuation -> no Tool Call -> final
```

## Scenario: Drive one Chat Turn through the unified Tool Registry

### 1. Scope / Trigger

Apply when adding a model-visible Chat Tool, changing Tool order or risk,
dispatching a provider Tool batch, changing Turn/Step completion or no-progress
rules, or changing the model-facing Result and process presentation. This
Registry belongs to ordinary Chat and is independent from the optional G20/G21
control-plane registry.

### 2. Signatures

```text
newChatAgentTurnDriver(completionDriven)
turn.beginStep(skillPrelude) -> {sequence, taskSequence, purpose}
turn.admitToolCalls(count) -> admittedCount

newChatAgentProgressTracker()
progress.observe(calls, results) -> {reason, blocked}

newChatToolRegistry(input)
registry.definitions(taskStep)
registry.executeToolBatch(ctx, events, input, calls, step, retrievalState)
registry.executeParallelReadGroup(ctx, events, input, calls, step)
registry.executeRetrievalCall(ctx, call, callIndex, state)

maxParallelChatReadTools = 4
```

Every `chatToolRegistration` binds one exact name to a Provider definition,
Backend executor, `read|write|execute|external` risk, timeout, output budget,
parallel permission, optional approval rule, model Result projector, and
replayable `search|tool` presentation.

### 3. Contracts

- Build one Registry replacement for every provider Step from current server
  authority. Memory ordering, current MCP visibility, Conversation-selected or
  run-only-auto Skills, and
  Search/Knowledge availability must be recomputed; Tool output cannot register
  another Tool.
- Execute calls in the model's exact order. A contiguous group of at most four
  registrations with `AllowParallel=true` may overlap across MCP and local
  backends; always merge Results into exact Provider call order before same-
  model continuation.
- Local parallel reads are exactly `read`, `grep`, `job_list`, `job_output`,
  and hidden legacy `file_read`, `file_search`, and `skill_view`. `skill`
  mutates loaded state and is a barrier. MCP requires current reviewed `read` classification;
  `mcp_tool_search` mutates the next visible catalog and is a barrier.
- Every `write|execute|unknown`, retrieval, Goal, unregistered, and
  non-parallel Tool is an ordered barrier between read groups. A Goal conclude
  result rejects only later calls as `goal_concluded`; it never suppresses or
  reorders earlier calls.
- A Skill prelude uses a separate Registry containing only the exact required
  `skill` definition. It increments the physical Step sequence but not the
  ordinary task-Step sequence.
- `search_memory` exists only on task Step 1. `skills_list` and `skill_view`
  are hidden compatibility registrations; every new Provider definition omits
  them.
- Exact name collisions remove that name from both model visibility and
  execution. The default Registry never registers a Subagent/delegation Tool.
- The default Registry exposes no general Code Mode or `run_code`. Native Tools
  remain the correctness/fallback path. Playwright's
  `browser_run_code_unsafe` is RCE-equivalent and is not a Browser Tool; any
  future Code Mode must remain optional above the same Registry policy.
- Effective Agent mode has no absolute Provider-Step, Tool-call, whole-Turn,
  local-round, or MCP-round cutoff while progress continues. The legacy
  32-Step/128-call and configured per-Run budgets remain scoped to non-Agent
  compatibility paths.
- Unknown names, invalid arguments, ordinary Tool errors, and per-Tool timeouts
  are structured Results. A per-MCP-call deadline is `tool_timeout`; explicit
  cancellation, `outcome_unknown`, and unrecoverable execution errors remain
  terminal. A non-Agent compatibility path may additionally retain its scoped
  parent Run deadline.
- Existing `ProviderToolExecutionEvent.Round` remains the provider-loop
  ordering input and is mirrored into durable event `step_sequence` when
  positive. Do not add a Provider sideband merely to manufacture Agent Step
  events: it changes Tool scheduling/cancellation semantics. Registry Result
  projection and presentation must not add credentials, private paths, or
  unbounded Tool output to process data. The sole command exception is the
  bounded/redacted typed `local_direct terminal` presentation defined below;
  generic Tool detail and MCP remain command-free.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| duplicate registered name | remove the name from definitions and lookup |
| unknown Provider Tool name | structured `unknown_tool`; no Backend call |
| registered Tool with bad arguments | Backend-specific structured invalid-argument Result |
| effective Agent reaches physical Step 33 or Tool Call 129 with new outcomes | continue; preserve exact ordering/evidence |
| non-Agent compatibility path reaches physical Step 33 | no Tool execution; one Tool-free final continuation |
| non-Agent compatibility path reaches Tool Call 129 | `turn_call_budget_exhausted`; no side effect |
| MCP per-Tool deadline while parent Run is healthy | structured `tool_timeout`; same model may recover |
| effective Agent passes MCP/local `RunTimeout` while progressing | continue; no parent deadline |
| three identical sanitized outcomes | emit `agent.outcome(blocked,repeated_tool_outcome)` then Tool-free blocked wrap-up |
| five distinct consecutive all-error rounds | emit `agent.outcome(blocked,consecutive_tool_errors)` then Tool-free blocked wrap-up |
| write outcome becomes unknown | terminal `MCP_OUTCOME_UNKNOWN`; never retry |
| Skill prelude receives another Tool | `skill_required_before_action`; no dispatch |
| contiguous safe reads exceed four | split into ordered groups of at most four |
| write/execute/catalog/retrieval occurs between reads | finish prior reads, execute barrier alone, then start later reads |
| Goal concludes at call N | preserve results `< N`; calls `> N` receive `goal_concluded` without execution |

### 5. Good / Base / Bad Cases

- **Good:** `skill` prelude -> one task Step calls Web plus `bash` -> Results
  are returned in original call order -> same model answers.
- **Good:** local `job_output` and reviewed MCP reads overlap, finish out of
  order, and enter the continuation in original model order.
- **Good:** a file task runs beyond the legacy `RunTimeout`, registers its
  workspace-relative deliverable, then ends naturally with no Tool Call.
- **Base:** no Tool is available, so Chat streams the ordinary compatibility
  answer without constructing a fake execution Step.
- **Bad:** append Tool definitions in one function but dispatch names in an
  unrelated switch, let MCP output add an alias, execute a colliding name,
  terminate the Turn for elapsed wall time, or loop forever on identical Tool
  outcomes.

### 6. Tests Required

- Registry definition order, complete policy metadata, first-task-Step Memory
  removal, collision fail-closed behavior, and default absence of Subagents.
- Cross-Backend `skill -> (Web + bash) -> answer` continuation with exact
  Provider Result order.
- Existing MCP dynamic-search, read concurrency, write serialization,
  Knowledge/Memory/Web, local Skill, cancellation, and recovery suites.
- Cross-backend overlap, local safe-read overlap, maximum-four grouping,
  write barriers, reverse-completion Result order, `mcp_tool_search` barrier,
  and Goal-conclusion ordering.
- Non-Agent Step/Call cap boundaries; completion-driven progress beyond those
  boundaries; three identical outcomes; five all-error rounds with reset after
  a successful alternative; recoverable per-Tool timeout; explicit
  cancellation; and `outcome_unknown`.
- Handler integration where a task crosses a deliberately short legacy
  `RunTimeout`, then persists a `workspace_file` output block and replays it.
  Persist `agentOutcome`/`agentOutcomeReason` for a blocked run.

### 7. Wrong vs Correct

#### Wrong

```text
batch all Goals -> all MCP -> all local -> retrieval
```

#### Correct

```text
Turn -> Step -> current Registry -> ordered safe-read groups + serial barriers
     -> ordered Result projection -> same-model next Step
```

#### Wrong

```go
streamCtx, cancel := context.WithTimeout(runCtx, localRunTimeout)
```

#### Correct

```text
effective Agent Turn -> cancellation-scoped Context
each Tool/backend Job -> its own timeout
repetition without progress -> typed blocked wrap-up
```

## Scenario: Persist and replay ordinary Chat Agent events

### 1. Scope / Trigger

Apply when changing Chat Turn/Step persistence, Message history DTOs, process
SSE ordering, restart recovery, event payloads, or migration `096`. This event
stream belongs to ordinary Chat and must remain separate from optional G20/G21
`agent_run_events`.

### 2. Signatures

```text
chat_agent_start_turn(turn, event, user, conversation, message, run, at)
chat_agent_append_event(turn, event, type, step_sequence, payload, at)
RecoverIncompleteChatAgentTurns(cutoff)
ChatMessageDTO.agentEvents[]

ProviderEventNarrationDelta = "narration.delta"

assistant.chunk {
  chunkType: "block-start" | "narration-delta",
  blockType: "narration",
  blockIndex: positive integer,
  content?: bounded sanitized UTF-8
}
```

### 3. Contracts

- One Turn binds one current-user Conversation, assistant Message and Run.
  Event sequence is contiguous per Turn, allocated by locking the Turn row and
  incrementing `next_sequence`; never use `MAX(sequence)+1`.
- Event IDs are idempotent. Event rows are immutable. The fixed vocabulary is
  Turn/Step/assistant/Tool/Goal/context only. `go_api_runtime` receives SELECT
  plus the exact start/append Functions and no direct table DML.
- `context.injected.payload.source` is a closed contract:
  `system-prompt|skill-catalog|skill-instruction|runtime-context`. Callers must
  reuse a declared source or change the event validator, contract, and real
  Handler/PostgreSQL tests together; an ad hoc label is not a new source.
- In `RETURNS TABLE` PL/pgSQL Functions, every output column is also a local
  variable. SQL inside `chat_agent_start_turn` / `chat_agent_append_event`
  must therefore use named constraints for `ON CONFLICT` and table aliases for
  columns such as `event_id`, `conversation_id`, `user_id`, and `status`;
  unqualified names are rejected as ambiguous at runtime even when migration
  creation succeeds.
- Persist each sanitized process or Tool projection before emitting the same
  projection over SSE. Never persist arguments, query, raw Result,
  credentials, private Server refs, paths, or unbounded output. A bounded,
  redacted command may exist only inside an authorized `local_direct terminal`
  presentation; it must replay byte-identically through the same ProcessStep.
- Buffer ordinary Provider text until the current Tool round's outcome is
  known. Text from a Tool-bearing or automatically continued round is bounded,
  sanitized user-visible Narration, persisted as
  `assistant.chunk(block-start|narration-delta)` plus
  `assistant.block.completed`, and emitted before that round's first Tool fact.
  Text from the terminal no-Tool round remains the sole final
  `message.content`. Never copy Narration into final content or fabricate it
  from Tool arguments/results when the Provider emitted none.
- Message reads attach ordered events. Frontend normalization sorts and
  deduplicates valid events, prefers their process projection, and uses
  `metadata.processTrace` only when no valid durable projection exists.
- On startup, reconcile an old `running` Turn to the already committed Message
  terminal status. Only `pending`, `streaming`, missing, or unknown Message
  states become `interrupted`; this marks the Message failed with
  `AGENT_RUN_INTERRUPTED` and closes active UI steps as interrupted.
- Message finalization and terminal-event append are not one transaction. The
  startup reconciliation is the explicit torn-write repair. It must not change
  a completed Message into interrupted.
- After an Assistant Message exists, every pre-SSE context persistence failure
  must finalize that Message as `failed` with
  `AGENT_EVENT_PERSISTENCE_FAILED` and finish the bound Turn as `failed` before
  returning the HTTP error. A failed Turn paired with a `streaming` Message is
  an invalid state because the frontend will continue polling it as active.
- Down is guarded while either Chat Agent table contains data. All older
  migration tail drills peel empty `096` before `095` and return to head `096`.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| concurrent appends | unique contiguous sequence under Turn row lock |
| repeated event ID with identical input | exact stored event replay |
| repeated event ID with changed input | replay conflict |
| append after terminal Turn | `CHAT_AGENT_TURN_TERMINAL` |
| process/Tool persistence fails | fail the Chat Run; do not emit an unpersisted projection |
| Tool-bearing round emits ordinary text | persist/stream it as Narration before the Tool row; exclude it from final content |
| terminal no-Tool round emits ordinary text | persist it only as final Message content |
| pre-SSE context event persistence fails | HTTP error plus failed Message and failed Turn; no orphan `streaming` Message |
| restart finds streaming Message | interrupted Turn and failed Message |
| restart finds completed/failed/cancelled Message | preserve that terminal status |
| historical Message has no valid events | legacy `metadata.processTrace` fallback |
| dirty migration Down | `CHAT_AGENT_EVENT_LOG_DOWN_DATA_EXISTS` |

### 5. Good / Base / Bad Cases

- **Good:** Narration commits before its Tool/process event, both reach SSE,
  refresh rebuilds the same order, the final answer appears once afterward,
  and startup repairs a torn terminal append.
- **Base:** a pre-`096` Message has no events and renders the legacy trace.
- **Bad:** emit before commit, store raw Tool output, reuse `agent_run_events`,
  derive sequence with an aggregate, or mark an already completed Message
  interrupted after restart.

### 6. Tests Required

- Concurrent sequence allocation, identical replay, immutable event rows,
  runtime DML denial, dirty Down refusal and clean down/up replay on PostgreSQL
  17 through `scripts/verify-chat-agent-event-log-postgres17.sh`.
- The PostgreSQL drill must execute both start and append Functions, including
  interrupted Message repair. Its dirty-Down proof creates an explicit event
  fixture after peeling any tail migration reapplied by integration-test setup;
  it must not rely on test residue. The drill derives the current embedded
  migration head and peels every post-`096` migration rather than hard-coding
  an obsolete head.
- Handler ordering and redaction tests, full existing cancellation suites, and
  startup recovery for both unfinished and already completed Messages. A real
  Handler regression must assert Resource orchestration records
  `runtime-context`; failure injection on `context.injected` must assert both
  Message and Turn terminal states.
- Frontend invalid-event fallback, ordering/deduplication, interrupted active
  steps, status copy, typecheck, Vitest and build.

### 7. Wrong vs Correct

```text
Wrong: SSE -> final metadata only -> restart guesses success
Correct: append event -> project same event to SSE -> refresh/restart replay

Wrong: context append fails -> Turn failed -> Message remains streaming
Correct: context append fails -> finalize Message failed -> finish Turn failed
```

## Scenario: Continue Chat through local Agent Skill Tools

### 1. Scope / Trigger

Apply when changing the `local_direct` Skill index, workspace File Tools,
background Jobs, native Tool definitions, Tool ordering, same-model
continuation, workspace-file delivery, budgets, cancellation, errors or
process-trace redaction. Local Tools join the existing provider-native loop;
they do not create another model protocol or enable Child Agents.

### 2. Signatures

```text
skill({name})
read({path, offset, limit})
write({path, content, expectedVersion})
edit({path, oldText, newText, replaceAll, expectedVersion})
grep({path, query, glob, maxResults})
publish_file({path, displayName, contentType})
bash({command, skill, workingDir, timeoutSeconds, runInBackground, outputFiles})
job_list({})
job_output({jobId, wait, timeoutSeconds})
job_kill({jobId})
```

- Catalog: `LocalSkillCatalog.PrepareRuntimeSkills(ctx, userID, runtimeRoot)`.
- Runtime: `newLocalSkillToolRuntime(executor, installedSkills)`.
- Selection: `runtime.prepareUserPrompt(providerPrompt, currentUserText)`.
- Batch: `executeLocalSkillBatch(ctx, events, runtime, calls, round)`.
- Required prelude:
  `executeRequiredLocalSkillBatch(ctx, events, runtime, calls, round)`.
- Failures: `SKILL_MODEL_UNSUPPORTED`, `SKILL_RUNTIME_UNAVAILABLE`, and
  `LOCAL_SKILL_BUDGET_EXHAUSTED`; a Provider that omits a required load returns
  `LOCAL_SKILL_REQUIRED_CALL_MISSING`.

### 3. Contracts

- Prepare only current-user selected Skills plus at most two bounded, relevant
  current-user installed/admitted run-only auto activations before the first
  model request. Automatic activation is snapshot-only and must never write a
  Conversation selection. Disabled runtime exposes no catalog or local Tool.
  An enabled runtime with no active packages publishes an empty catalog tombstone, omits
  only `skill`, and still exposes `read`, `write`, `edit`, `grep`, Job, and
  `bash` Tools.
- Add only a compact bounded name/version/description catalog replacement to
  the system prompt. Bind it to a deterministic SHA-256 revision and mark it
  untrusted routing metadata. Full instructions load only through `skill` or a
  current user's deterministic `/skill-name` gesture and cannot override
  system/developer instructions.
- New model requests expose `skill` only when installations exist, plus
  canonical Pi-style File/Job/`bash` names whenever the runtime is enabled.
  Keep `file_read`, `file_write`, `file_edit`, `file_search`, `terminal`,
  `skills_list`, and `skill_view` executable only for bounded in-Turn/history
  compatibility; never advertise them in new Provider definitions or guidance.
- Exact installed-name mentions queue every named Skill. Otherwise, only one
  unique strong lexical name/description match may queue automatically. A
  queued Skill runs in a prelude that exposes only `skill`, constrains `name`
  with an exact enum, disables incompatible thinking, and rejects every
  non-`skill` call before MCP, retrieval, or `bash` dispatch.
- Scan only the claimed current user text for whitespace-bounded
  `/skill-name`. Unknown names, punctuation-attached tokens, paths, attachments,
  Tool results, and historical messages cannot forge deterministic loading.
  JSON-frame the injected content so Skill text cannot close its wrapper.
- Track successful loads by `(name, catalog revision)` within the Turn. A
  duplicate returns `alreadyLoaded=true` without repeating `SKILL.md`.
- Tool schemas are strict and reject additional/invalid arguments.
  Provider-facing strict schemas list every property in `required`; values that
  are semantically optional use a nullable type and the runtime treats `null`
  as the documented default. Do not combine `strict=true` with an omitted
  property, because OpenAI-compatible providers reject that definition before
  the first Tool Call.
  `skill` reads only validated UTF-8 `SKILL.md`. `bash.skill` resolves only
  the prepared catalog and exposes the package through
  `NEO_CHAT_ACTIVE_SKILL_ROOT`; never reveal or accept server paths.
- Outside a required prelude, execute each provider Tool batch in this order:
  MCP, local Skill Tools, then ordinary Knowledge/Memory/Web Tools. Merge
  results by original call index and continue on the exact same Provider/model
  with native Tool result framing. Required Skill preludes do not consume the
  ordinary first task-round semantics; explicit Memory/Search keeps its
  existing priority after loading.
- In non-Agent compatibility paths, count local calls across rounds and cap
  local Tool rounds independently. Effective Agent mode instead continues
  while Tool outcomes show progress and uses the no-progress guard to block
  repetition; no executor Run deadline wraps the whole Agent loop.
- The strict `bash.timeoutSeconds` schema uses
  `max=floor(AGENT_LOCAL_CALL_TIMEOUT / 1s)` for both foreground and background
  calls because one Tool definition serves both paths. `null` means the
  foreground Call timeout or, with `runInBackground=true`, the background Run
  timeout. Never advertise `AGENT_LOCAL_RUN_TIMEOUT` as an explicit maximum;
  long work uses background mode with a null timeout and later
  `job_output(wait=true)`.
- Foreground `bash` exit zero returns a successful Provider Tool Result. A
  nonzero exit returns `IsError=true`, `error=nonzero_exit`; a timed-out result
  returns `IsError=true`, `error=timeout`. Both error results keep bounded
  `exitCode`, stdout, stderr, `timedOut`, `truncated`, and `durationMillis` for
  same-model recovery and the typed Terminal presentation. No successful or
  failed Result carries a completion-evidence ID. The final ProcessStep and `tool.result` status must be
  failed in live SSE and durable replay.
- Runtime guidance must not assume a `python` alias. Direct Python commands use
  `python3` after an availability check when needed; interpreter discovery or
  missing binaries remain ordinary observable command failures.
- File paths are workspace-relative and anchored with `os.Root`. A read returns
  a complete-file `sha256:<hex>` version; write/edit requires that exact
  version, rechecks before atomic rename, and returns `version_conflict` rather
  than overwriting an external change. Reads/writes/searches are UTF-8 and
  byte/file/result bounded. Successful `write`/`edit` records a typed
  `workspace_file` reference when a Host Workspace is bound; its path and
  version come from the Tool result rather than assistant prose.
- `bash.outputFiles` is a bounded array of exact workspace-relative paths.
  Foreground success captures those files immediately. Background `bash`
  binds the same paths to its exact Job ID and captures them only after a
  successful `job_output(status=completed)`; missing/unsafe paths are typed
  Tool errors and are never guessed from stdout or narration.
- `publish_file` is a compatibility fallback only when the actor-owned File
  service is wired and no Host Workspace is bound.
  It snapshots binary or text bytes after the same path/symlink checks, rejects
  empty files, applies the server upload limit to one file and Turn total, and
  admits at most eight unique `(path, version)` artifacts. An unchanged replay
  reuses the exact File ID. Successful Files use `purpose=export` and become
  assistant-only `purpose=output` Message attachments on every completed,
  failed, or cancelled terminal path. A bound Host Workspace instead keeps the
  project file as the sole authority and emits no automatic attachment copy.
- If assistant finalization fails, reread Message authority before cleanup:
  delete only artifacts proven unlinked, preserve an attachment already linked
  by an ambiguous commit, and retain private Files when the authority read is
  unavailable. Never expose a workspace/object path as a URL.
- Background Jobs are in-memory, scoped to exact user plus Conversation, share
  foreground terminal concurrency/Run timeout, and never survive restart.
  `job_output(wait=true)` waits at most ten seconds without polling. Declared
  output files become visible only on successful completion. Start, list,
  kill, running, failed, and canceled states do not claim file delivery.
  Shutdown kills/reaps every Job.
- Model capability is checked before creating the assistant response when an
  installed Skill would require native Tools. Catalog preparation fails closed
  without exposing object keys, fingerprints, package bytes or local paths.
- Cancellation propagates through Provider and local executor context. The
  executor kills the complete command process group; cancellation remains the
  terminal Chat Run outcome rather than an ordinary Tool failure.
- Process Tool events retain only Tool name, round, `local_direct`,
  classification, optional timeout, duration, failure category and allowlisted
  `durability=process_local` in diagnostic `detail`. A Terminal ProcessStep may
  additionally carry the separate tagged presentation
  `{card:"terminal", command, cwd?, exitCode?, timedOut?, truncated?, background?}`.
  Accept it for canonical `toolName=bash` or historical `terminal` plus
  `mode=local_direct|host_workspace`; bound
  command/cwd on UTF-8 boundaries, apply common-secret redaction, and replace
  Workspace/Host Workspace/Skill cache roots with stable aliases. Never put
  command/cwd in `detail` or persist/stream raw stdout, stderr, other arguments,
  Skill content, or materialized paths. The same sanitized ProcessStep is the
  durable event, live `process.step.updated`, and Conversation replay source.
- A destructive Terminal result classified `approval_required` is not finalized
  as an ordinary Tool failure when durable approval authority is available.
  Create a five-minute approval tied to the exact Turn/execution/Tool/risk,
  persist and emit its sanitized `awaiting_approval` ProcessStep, then wait on
  the same Tool call. First valid revision/CAS decision wins. Allow resumes
  exactly once; deny/expiry/cancel/restart denial never execute. Exact
  Conversation + Tool + risk grants may auto-allow later calls. Approval rows
  retain no raw arguments/results and the hard blocklist remains non-bypassable.
- The best-effort primary SSE writer must publish every sequenced event to the
  active Run ring before attempting socket delivery. Reconnect uses the same
  monotonic sequence/SSE ID; never manufacture a second event order. Snapshot
  and subscriber registration are atomic so events cannot fall between replay
  and live follow. Bound frames, bytes, subscribers, subscriber queues, terminal
  grace, and finished Run count. A queue overflow closes only that subscriber.
  An evicted cursor emits safe gap metadata, never missing payload content.
- Background Terminal is a Job lifecycle start, not a completed command card.
  Persist safe process-local Job metadata in the existing ProcessStep
  presentation. Keep later Job Tool events immutable, merge only the display
  projection by exact `jobId`, and reconcile `running`/`stopping` to
  `interrupted` when startup recovery proves the owning Turn was interrupted.
- Run `bash mm-chat/scripts/verify-chat-artifacts-postgres17.sh` for artifact
  publication changes; it must prove output-link reload, two-user isolation,
  deleted-file rejection, and ephemeral PostgreSQL 17 teardown.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| active Skills plus non-Tool-capable model | fail before assistant creation with `SKILL_MODEL_UNSUPPORTED` |
| catalog/package preparation fails | `SKILL_RUNTIME_UNAVAILABLE`; no internal detail |
| strict arguments fail | Tool result `arguments_invalid`; no file read/process |
| strict schema omits an optional property from `required` | Provider rejects the Run before Tool execution; repair the schema with required + nullable, not by weakening runtime validation |
| explicit `bash.timeoutSeconds` exceeds Call timeout | rejected by the advertised strict schema; no foreground process starts |
| background Terminal uses `timeoutSeconds=null` | start the Job with the configured Run timeout; return its process-local Job ID |
| foreground `bash` exits nonzero | failed Tool Result with `nonzero_exit` and bounded process output/flags |
| foreground `bash` times out | failed Tool Result with `timeout`, `timedOut=true`, and bounded output/flags |
| `skill.name` unknown | bounded `skill_not_found` |
| duplicate `skill.name` under the same catalog revision | success with `alreadyLoaded=true`; omit content |
| deterministic `/skill-name` package read fails | `SKILL_RUNTIME_UNAVAILABLE`; no Provider request |
| required prelude returns no Tool Call | `LOCAL_SKILL_REQUIRED_CALL_MISSING`; discard buffered prose |
| required prelude returns MCP/retrieval/`bash` or a retired Tool | bounded `skill_required_before_action`; no side effect |
| materialized fingerprint drifts | bounded `package_drift`; no content/process |
| catastrophic command blocked | typed Tool failure before process creation; approval cannot bypass |
| destructive command, no durable approval authority | typed `approval_required` Tool failure |
| destructive command with durable approval authority | durable wait; same Tool resumes only after allow |
| primary SSE disconnects | generation continues; reconnect from last accepted sequence |
| requested cursor predates ring | `stream.gap(cursor_evicted)` then retained suffix |
| Backend restarts with process-local Job still running/stopping | durable lifecycle projects `interrupted`; never claim recovery |
| workspace traversal/symlink escape | bounded `path_invalid`; no file access |
| file version changed | bounded `version_conflict`; preserve current bytes |
| Job lookup across user/Conversation | `job_not_found`; no existence disclosure |
| Job is running or failed | no workspace file reference; no success claim |
| Backend shutdown/restart | kill/reap active Jobs; never claim recovery |
| non-Agent compatibility local call/round budget exhausted | bounded failure then Tool-free final continuation |
| foreground call/background Job deadline expires | bounded timeout result; descendants killed |
| client cancellation | terminal canceled Run; descendants killed |

### 5. Good / Base / Bad Cases

- **Good:** one native continuation performs
  `skill -> read -> edit -> bash/job_output -> final answer`, with exact
  Results only in model context, declared deliverables as workspace file cards, and a bounded,
  redacted Terminal card in persistence/SSE.
- **Good:** a command that needs longer than the foreground Call timeout starts
  with `runInBackground=true, timeoutSeconds=null`, then observes its exact Job
  through `job_output(wait=true)`.
- **Base:** local execution is enabled but the user has no installed Skills;
  an empty replacement tombstone is injected, File/Job/`bash` remain, and
  ordinary MCP/Knowledge/Memory/Web planning is unchanged.
- **Bad:** paste every `SKILL.md` into the first prompt, trust a Tool-supplied
  object/path, execute an unadvertised action during a required prelude, run
  local Tools after losing call ordering, put raw stdout/stderr or command data
  in generic Tool detail, accept an MCP-forged Terminal card, or spawn a Child
  Agent to execute the Skill.
- **Bad:** advertise the longer Run timeout for a shared Terminal schema while
  foreground validation enforces the shorter Call timeout, or treat a returned
  `exit 127` result as successful merely because the shell process was started.

### 6. Tests Required

- Complete `skill -> bash -> answer` success with the same Provider/model,
  exact Tool order, and only `skill` visible during a required prelude.
- Exact-name, unique CJK/Latin lexical match, deterministic slash invocation,
  duplicate-load suppression, revision replacement, and empty tombstone.
- A forged `bash` call in a required prelude returns a Tool error and creates
  no workspace marker. Explicit Memory remains the first ordinary task round
  after the Skill prelude.
- Provider-schema assertions prove every strict `properties` key is present in
  `required`, nullable local defaults accept explicit `null`, and runtime
  unknown-field/path/command checks remain active.
- Empty/disabled catalog, Tool-incapable model, preparation failure, invalid
  arguments, missing/drifted file, blocked/destructive command and nonzero exit.
- Workspace traversal/symlink/UTF-8/size/search bounds, read-version-write,
  external conflict, atomic replacement and read-back evidence.
- Background Job same-scope authorization, cross-scope denial, wait, completion
  notices, shared concurrency, timeout/kill/process-group reap, shutdown, UI
  restart warning, and evidence gating.
- Call, round, output, call-timeout and Run-timeout boundaries plus cancellation
  process-group termination.
- Assert the Terminal schema maximum equals Call timeout, accepts the exact
  bound, and cannot advertise a value above it. Prove a background call with a
  null timeout retains the Run-timeout path.
- Assert exit zero succeeds while exit 127 and timeout return model-visible Tool
  errors, preserve bounded diagnostics, omit completion evidence, and project
  failed status identically before and after reload.
- Process event/persistence/SSE assertions prove the typed Terminal card keeps
  only bounded/redacted command, stable cwd alias and result flags; raw
  stdout/stderr, file content, materialized paths and credentials remain absent.
- Running/completed, exit 0/nonzero, timeout, truncation and background cases
  must replay identically. Unknown/malformed presentation is dropped without
  invalidating the enclosing ProcessStep; non-Terminal/MCP behavior is unchanged.
- Existing MCP, Knowledge, Memory, Web, detached Run and Citation tests remain
  green; no source-fusion fallback is introduced.

### 7. Wrong vs Correct

#### Wrong

```text
system prompt + every installed file + guessed server path + child agent exec
```

#### Correct

```text
bounded revisioned catalog -> optional required/deterministic Skill load
-> versioned workspace/Job Tool rounds -> same-model natural completion
-> redacted process facts -> final answer
```

```text
Wrong: timeoutSeconds=RunTimeout -> foreground arguments_invalid -> burn a round
Correct: timeoutSeconds<=CallTimeout OR background=true + timeoutSeconds=null
```

## Scenario: Compact a Chat Agent continuation and recover from context overflow

### 1. Scope / Trigger

Apply when changing Provider Tool continuation storage/serialization, Tool
Result bounds, synthetic checkpoints, Provider context-overflow classification,
retry behavior, or durable `context.replaced` events. Compaction must preserve
native call/result framing and must never use removed content as diagnostics.

### 2. Signatures

```go
compactChatAgentContinuation(exchanges []ProviderToolExchange, aggressive bool)
    ([]ProviderToolExchange, *ProviderContextReplacementEvent)

type ProviderContextReplacementEvent struct {
    Reason string
    BeforeBytes, AfterBytes int
    ResultsPruned, ExchangesReplaced int
}
```

- Normal bounds: Tool Result `64 KiB`, head/tail `24 KiB`, Turn `256 KiB`,
  newest `4` complete exchanges exact.
- Overflow bounds: Tool Result `16 KiB`, head/tail `6 KiB`, newest `2`
  complete exchanges exact.
- Typed failure: `PROVIDER_CONTEXT_OVERFLOW`.

### 3. Contracts

- Prune each oversized Tool Result with a UTF-8-safe head, omission marker and
  tail. Preserve call ID, Tool name, `IsError`, call arguments, and the owning
  complete `ProviderToolExchange`; never mutate the input slice's nested
  results.
- If the continuation remains oversized, replace only a prefix of complete
  exchanges with one bounded synthetic checkpoint. Never split a Tool Call
  from its Result. Checkpoints contain Tool name/call ID/status/result byte
  count only; merge older checkpoints without nesting their wrapper.
- Preserve the newest exact Provider state. Anthropic thinking/redacted
  thinking/signatures in retained exchanges remain byte-authoritative.
- Classify overflow only from HTTP `413` or an allowlisted stable JSON
  `code|type|status|reason`; never inspect free-form upstream `message` text.
- `ProviderFailureCategories()` is a shared hash-bound input to the Memory
  Judge failure taxonomy. Adding this category requires a new taxonomy version,
  recomputed sorted-JSON SHA-256, and synchronized capture docs/scripts/tests;
  a focused Chat test alone is not a complete change.
- Retry the exact Provider, model and Step once after aggressive compaction only
  when `AfterBytes < BeforeBytes`. Handle synchronous failure and an error in
  the first SSE event. A retry failure does not compact/retry again.
- Emit `ProviderEventContextReplaced` before retry/continuation. The Handler
  appends durable `context.replaced` with counts/reason only; no removed content
  enters PostgreSQL, SSE process detail, logs, or terminal message metadata.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Tool Result is at or below 64 KiB | retain exact Result |
| UTF-8 code point crosses a cut boundary | move boundary; emit valid UTF-8 |
| continuation exceeds 256 KiB with more than four exchanges | checkpoint complete old prefix only |
| aggressive compaction does not shrink | no retry; return original overflow |
| synchronous first overflow after shrink | retry same Provider/model/Step once |
| first SSE event is overflow after shrink | discard that event and retry once |
| retry also overflows | terminal failure; total attempts remain two |
| JSON message merely says “context too long” | ordinary request rejection, not overflow |
| event persistence fails | terminal `AGENT_EVENT_PERSISTENCE_FAILED` |

### 5. Good / Base / Bad Cases

- **Good:** a large terminal Result is pruned, an oversized old exchange prefix
  becomes one content-free checkpoint, the same Step retries once and succeeds.
- **Base:** continuation stays below thresholds and no replacement event or
  retry occurs.
- **Bad:** truncate raw serialized messages, detach Tool Calls from Results,
  discard recent Anthropic signatures, classify human-readable error text, or
  loop retries until the Provider accepts the request.

### 6. Tests Required

- UTF-8 pruning preserves head/tail and the exact call/result identity without
  mutating input.
- Whole-exchange compaction keeps the newest four/two exchanges and recent
  Provider state; serializer fixtures render one synthetic user checkpoint.
- Stable-code/HTTP-413 positives and message-only/unknown-code negatives.
- Synchronous and first-SSE overflow both prove strict shrink before the single
  retry; a second overflow proves no third Provider call.
- Event recorder tests prove positive counts, continuous sequence and
  content-free payload.

### 7. Wrong vs Correct

```text
Wrong: provider says “too long” -> truncate arbitrary messages -> retry loop
Correct: typed overflow -> shrink complete exchanges -> record bounded counts
        -> same Provider/model/Step retry once
```

## 1. Scope / Trigger

Use this spec when changing provider streaming to expose function tools,
executing multi-round Tool Calls, adding Web/Knowledge tools, persisting process
steps, changing conversation Search authority, or changing the ownership of a
Run across browser navigation/SSE disconnect.

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

type ProviderFailureCategory string

func ProviderFailureCategoryOf(error) (ProviderFailureCategory, bool)
```

`ProviderFailureCategoryOf` exposes only the fixed request/transport/HTTP/SSE/
context category. It never returns an upstream body or raw error string; the
Development Memory adapter maps it into the hash-bound v8 taxonomy.

The normalized one-round planning seam used by compatibility routing is:

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

The canonical product Memory Tool contract is owned by `internal/chat`:

```text
name             = search_memory
contract version = memory-search-tool-v1
contract SHA-256 = f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6
arguments        = explicit {}
tool choice      = auto; required search_memory for explicit saved-Memory reads
first round only = true
adapter version  = chat-first-tool-round-memory-decision-v1
global gate      = MEMORY_TOOL_LOOP_ENABLED=true
user gate        = authenticated UUID exactly in MEMORY_TOOL_LOOP_CANARY_USER_IDS
```

Only the current user message may force the product Tool. A bounded bilingual
lexical gate recognizes explicit saved-Memory read/use/search commands and
direct personal recall questions, including bounded first-person preference
forms such as `我喜欢喝什么？` and `what do I like to drink?`. It orders
`search_memory` first, selects the existing normalized `required` choice, and
disables optional reasoning for that first decision round. The personal form
must describe the current user and occupy the request itself: second-/third-
person questions, advice such as `我应该喝什么？`, and quoted writing tasks
remain Auto. Direct remember/correct/forget actions keep their higher-priority
write path; general questions about memory and ordinary turns remain `auto`.
The same-model continuation restores the selected answer settings, except that
the official DeepSeek Tool protocol stays thinking-disabled through its
continuation.

The old schema-v6 preflight additionally forced `temperature=0`, maximum output
`128`, and disabled thinking. Those fields remain historical schema-v6 evidence
and are not part of the schema-v7 first-round contract. Its disabled-thinking
wire shape was Provider-specific:

```json
// Official DeepSeek host: api.deepseek.com
{"thinking":{"type":"disabled"}}

// Other OpenAI-compatible gateways
{"enable_thinking":false}

// Official OpenAI
// Omit both non-standard fields.
```

OpenAI-compatible/Gemini continuation is an assistant message containing the
completed `tool_calls`, followed by one `role=tool` message per matching
`tool_call_id`. Fragmented names/arguments are accumulated before execution;
arguments are capped at 64 KiB.

Raw model-generated Tool protocol in a Provider `content` delta is never a
Tool Call. In particular, an ASCII or fullwidth-bar DSML marker is an invalid
OpenAI-compatible response: the adapter retains a bounded UTF-8-safe suffix
across chunks, emits neither the marker nor its trailing payload, and returns
the fixed `PROVIDER_RESPONSE_INVALID` category. It must not parse, persist, or
execute the embedded name/arguments. Only the provider's structured
`tool_calls` field may enter Registry validation, mode admission, and Tool
execution.

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

Detached Server-mode generation uses two distinct contexts and one best-effort
delivery wrapper:

```go
generationCtx := context.WithoutCancel(r.Context()) // keep auth/request values
streamCtx, cancelRun := context.WithCancel(generationCtx)
delivery := newBestEffortStreamWriter(w)             // write/flush failure detaches
```

`activeRuns.register(runID, cancelRun)` plus the durable cancellation store are
the only Run-cancellation seams after acceptance. The HTTP request context and
SSE socket own delivery only.

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
  An explicit saved-Memory read is the sole product exception: order
  `search_memory` first and require that exact Tool without forcing Web or
  Knowledge.
- Tool Calls are accumulated and validated before execution, then returned in
  the provider's native continuation format to the same model.
- Do not add product-level Tool Round or Tool Call count limits to the existing
  Web/Knowledge/Memory loop. MCP is an intentional bounded extension: whenever
  an MCP Tool participates, apply the frozen MCP run limits (default 8 rounds,
  32 calls, 30 seconds/call, and 120 seconds/run) from `mcp-tools.md`.
- A Tool-unsupported or not-yet-known current model uses the same model for one
  bounded `direct|knowledge|web|both` compatibility plan when Knowledge is in
  scope. Never use a hidden model and never add this planner round to a known
  Tool-capable native path.
- Persist only rendered provider reasoning and sanitized steps. Credentials,
  raw payloads, system prompts, full source bodies, and internal errors remain
  forbidden.
- Read-only Web/Knowledge tools run automatically. Existing non-MCP side
  effects require an approval policy before registration. MCP is the explicit
  exception: current conversation server/Tool selection is prior
  authorization, every call is rechecked against the frozen snapshot, and no
  per-call approval dialog is added.
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
  unexpired probe cache, then `unknown`. `enabled` and `disabled` are explicit
  operator assertions; `auto` is the normal default. Persisted Chat keeps the
  non-blocking compatibility path. Persisted Agent waits for the shared bounded
  probe on a cache miss; a supported result admits the same request, an explicit
  unsupported result downgrades it, and transient/inconclusive `unknown`
  preserves the adapter-native Agent Tool round.
- An Auto probe sends only a fixed fictional Tool definition and fixed prompt
  with thinking disabled, temperature zero, and maximum output `128`.
  It contains no user query, conversation, catalog, source body, provider raw
  payload, or credential. Only a valid completed Tool Call with matching name,
  non-empty ID, and JSON-object arguments records `supported`. Explicit
  tools/function-call incompatibility records `unsupported`; cancellation,
  timeout, rate limit, 5xx, transport failure, ordinary 400, and inconclusive
  prose remain `unknown`.
- Official `api.deepseek.com` native Tool rounds and Tool continuations send
  `thinking.type=disabled` and omit `reasoning_effort`, even when the ordinary
  chat request enabled reasoning. Plain no-Tool DeepSeek chat retains the
  selected reasoning settings. Generic compatible gateways must not receive
  the DeepSeek-only field.
- Official DeepSeek may return a model-generated `query` member for a function
  whose server-owned schema permits no arguments. For only a server-declared
  zero-argument function, the adapter may canonicalize a bounded valid JSON
  object to `{}` before validation and native continuation. It must not expose
  any returned member as query authority. Malformed, non-object, and oversized
  arguments remain invalid; generic compatible Providers and argument-bearing
  Tools remain unchanged. This wire normalization does not change the
  canonical Memory Tool definition or SHA-256.
- Provider save/activation schedules detached background warmup for the first
  configured model and matching task models. An unknown first-use Chat request
  uses Planner immediately and starts one singleflight probe. An unknown
  first-use Agent request waits for that same bounded probe, but no request waits
  for the subsequent cache write. Probe/cache writes use bounded detached
  contexts.
- A real native first round downgrades capability only for explicit Tool
  incompatibility, writes the downgrade asynchronously, and continues through
  same-turn Planner. Transient provider failures remain provider failures.
- Before any Tool executes, the first native Tool-round startup may retry the
  exact same resolved Provider/model/request once for a typed timeout, rate,
  upstream-5xx, transport, stream-read, or incomplete-stream failure. Use an
  explicit `Retry-After` capped at five seconds or the fixed 200 ms fallback;
  cancellation ends the wait immediately. Never re-resolve, switch Provider,
  retry deterministic failures, or replay a round after any visible event or
  Tool mutation.
- If that bounded retry still fails, preserve the fixed typed
  `ProviderFailureCategory` through Assistant failure and public stream error.
  The mere presence of Local Skill, MCP, Resource, Retrieval, or Workspace
  runtimes must not rewrite a Provider failure as a runtime failure. Only a
  real explicit Tools/function-call incompatibility becomes
  `SKILL_MODEL_UNSUPPORTED` or `MCP_MODEL_UNSUPPORTED`; public error messages
  remain fixed and contain no upstream body, Base URL, credential, or secret.
- Explicit human install intent plus exactly one supported Skill/MCP discovery
  link is handled by the Resource Orchestrator before the Provider round.
  AIHero Skill links use the owner-private pinned-source adapter with zero Store
  search/review; other links retain exact Store/Marketplace resolution.
  Unsupported or multiple links remain ordinary Agent input.
- Capability cache identity includes provider config hash and exact model ID.
  The hash binds user/provider identity, type, normalized Base URL, model list,
  encrypted secret reference hash, connection hash, default, and model
  overrides. Config changes therefore miss old rows without exposing secrets.
- External Web execution may retry the exact same resolved provider once after
  a short context-aware delay only for `REQUEST_FAILED`, HTTP `408`, `429`, or
  `5xx`. It must not re-resolve, switch providers, retry authentication/other
  `4xx` or response/schema failures, or continue after context cancellation.
- A native external-Web round exposes `read_web_url` beside `search_web`.
  `read_web_url` accepts exactly one public HTTP(S) URL. The exact selected
  external Provider's bounded Extract capability is preferred when available;
  the server-owned safe reader is the no-provider fallback. It never receives
  browser cookies, environment proxies, or model-selected headers. Provider
  credentials stay inside their existing adapter. Public Discourse topic/post
  URLs use the same-origin
  topic JSON representation and select the requested `post_number`; ordinary
  pages use bounded HTML/plain-text extraction. Every initial/final redirect
  target is DNS/IP revalidated and loopback/private/link-local/reserved targets,
  IP literals, userinfo, fragments, unsupported MIME/encoding, oversized bodies,
  and empty content fail closed.
- Direct page content is untrusted evidence. It enters only the existing Web
  `Result`/`[W#]` path with an explicit model instruction never to follow source
  instructions. The raw model URL argument does not enter process detail; the
  validated canonical source URL may exist only in normal Citation authority.
- Each successful Web Tool Result contains only sources newly minted by that
  execution. Prior Tool Results remain in native continuation history, so
  serializing the cumulative Web corpus again is forbidden. New sources keep
  their cumulative marker, for example the second execution may return `[W2]`
  without repeating `[W1]`.
- Turn-level Web diagnostics aggregate all Search and URL-read calls. Any
  source-bearing success plus any failed call is `partial`; it preserves Web
  authority and successful Citations. `degraded` means no usable Web source was
  obtained. A later call must not overwrite these aggregate semantics.
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
- If an ordinary Provider answer has already emitted content and its SSE read
  fails or closes without the required terminal event, persist the partial
  content with `status=failed` and emit stable code
  `PROVIDER_STREAM_INTERRUPTED`. Do not replay, auto-continue, or mark the
  message completed. The public message is fixed and must not include upstream
  response bodies or transport error text.
- Process detail uses an allowlist and bounded values. Exact `query`,
  `redactedArgs`, catalog metadata, unknown fields, raw payloads, source bodies,
  headers, prompts, SQL, and internal errors are dropped before SSE and
  persistence. The frontend derives `Direct|Knowledge|Web|Both`, source counts,
  and localized reasons only from sanitized step kinds/counts and an explicit
  reason-category allowlist.
- Live reasoning redaction must retain a bounded un-emitted suffix across
  provider chunks. Sanitizing each chunk independently is forbidden because a
  split `apiKey=`/value or `Bearer` token can bypass the regex boundary.
- Server-mode text and image generation outlive browser navigation, tab close,
  and SSE write/flush failure. `context.WithoutCancel` must preserve
  authentication and request-scoped values without inheriting HTTP disconnect;
  the Run receives a separate cancellable child. After the first delivery
  failure, later SSE writes are successful no-ops while Provider consumption,
  Tool/Memory/Search continuation, finalization, Usage, and capture continue.
  Never call the Run-cancel/finalize-cancelled path merely because delivery
  failed.
- G19.3 external Web work and G19.6B Tool-capable selected-Knowledge work
  execute after SSE starts and are represented by live Tool/Web and
  Tool/Knowledge steps. `search_knowledge` accepts only a bounded Query;
  authenticated conversation selection remains collection authority. The
  non-Tool/model-built-in compatibility executor is also live after
  `message.started`; pre-SSE Knowledge retrieval is forbidden.
- Run-context cancellation from explicit cancel authority is a terminal
  control state, not Tool degradation. A
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
- `internal/chat` is the single Tool definition/hash/validation authority.
  `internal/memoryroute` delegates to it only for Development capture. The Tool
  has no query argument: the server already owns the current request text, so
  the model decides only whether Memory is needed.
- The product exposes `search_memory` only when the default-off global flag is
  true, the authenticated user UUID exactly matches the API-only canary set,
  the selected Provider implements `ToolRoundProvider`, current Conversation
  Memory use is allowed, Search is not model-built-in, and the turn is not a
  direct `remember|correct|forget` action. Empty/invalid/duplicate allowlists,
  unauthenticated requests, and non-canary users fail closed before retrieval
  or Judge work. The Memory Worker never receives the canary variable.
- The first round carries the normal chat request and may expose
  `search_web`, `read_web_url`, `search_knowledge`, and `search_memory` together.
  Before any
  Memory call, the Provider receives no Memory candidate body, ID, scope,
  revision, retrieval score, or database authority.
- First-round content and reasoning are buffered. With no Tool Call they are
  released unchanged; with any Tool Call the partial draft is discarded before
  execution and same-model continuation. A synchronous or in-stream failure
  before that buffered round closes returns to the existing compatibility path
  without executing an already assembled call; no partial draft or Memory body
  has crossed the user boundary at that point.
- Product Memory use requires exactly one first-round call with a non-empty ID,
  exact raw `search_memory` name, and a non-nil empty decoded argument object.
  Validation compares the raw name, not a trimmed/normalized substitute.
  Missing, `null`, malformed, non-empty, duplicate, unknown, or later-round
  Memory calls fail closed for the Memory lane. Valid Web/Knowledge calls in the
  same batch retain their independent authority.
- After a valid call, `usermemory.SearchRelevantAfterMemoryToolCall` runs fixed
  BGE embedding, exact/BM25/vector RRF, the frozen bilingual negative-policy
  guard, admission, fixed BGE rerank, buffered fixed Luna
  candidate judging, ordinal intersection, and Top-5/600/900 selection
  without calling the v1 reader or `MarkUsed`. Product composition accepts only
  the production policy and re-resolves the stored `SERVER_DEFAULT` / OpenAI
  Compatible / attested Base-URL hash / `gpt-5.6-luna` tuple for every Judge
  attempt. Dependency, secret, endpoint, model, prompt, decoder, or policy drift
  fails closed. Only typed transient Judge Provider failures retry, at most
  twice, using valid `Retry-After` or fixed five/ten-second waits. Migration
  `065` rehydrates the recorded final set through current user/source/settings/
  epoch/projection/revision/hash/scope/Sensitive authority, then Go rechecks
  final identity and redacts content again.
- Empty retrieval is successful only when migration-070 user health is
  `ready`. Indexing, unavailable, disabled, or unreadable health produces one
  bounded failed Tool step with a frontend-allowlisted reason. Other retrieval
  failures also return bounded Tool Results and allow ordinary same-model
  continuation. `search_memory` is removed from every later
  round. A pre-content continuation failure recovers through the same Provider/
  model from the original request without any Memory body, including the empty-
  result path; partial answer content preserves the error and never duplicates
  an answer.
- A successful non-empty Memory Tool execution retains only the exact
  context-budgeted rows placed in its Tool Result. If the same-model
  continuation completes, assistant finalization atomically persists immutable
  L1 Usage links for those rows. No-call, empty, invalid, failed, cancelled, or
  original-request recovery paths persist no Tool Memory Usage.
- The 2026-08-04 production replay completed one native `search_memory` step,
  returned the saved school, retained the unrelated name fixture only through
  RRF/rerank, and persisted exactly the school Memory Usage link. The Memory
  Worker stayed stopped and the corpus did not change during the proof.
- Schema-v6/profile-v6/cost-basis-v4 remain immutable failed `PlanTools`
  evidence. Schema-v7/profile-v7/cost-basis-v5 use Development adapter
  `chat-first-tool-round-memory-decision-v1` and artifact
  `memory-first-tool-round-development.json`. Offline gates pass. The first
  live GPT and DeepSeek Flash schema-v7 runs completed only `28/300` and
  `33/300` routes respectively and failed quality, slice, cutoff, and latency
  gates; no policy is frozen.
- The v7 aggregate cannot split `MEMORY_TOOL_ROUTE_FAILED`. OpenAI-compatible
  request and SSE failures now carry fixed typed categories through the
  Development adapter. Schema v8 publishes category counts only, binds the
  taxonomy version/SHA in profile configuration, and always reports
  `policySelected=false`; normal chat still exposes only its existing bounded
  Provider failure behavior.

## 4. Validation & Error Matrix

| Condition                        | Required behavior                                |
| -------------------------------- | ------------------------------------------------ |
| Search off                       | zero Search I/O                                  |
| Native Web Tool unsupported      | same-model compatibility plan                    |
| Native Knowledge Tool unsupported | live compatibility executor; no pre-SSE retrieval |
| Tool capability override enabled/disabled | bypass probe cache with explicit operator assertion |
| Auto capability cache miss/expired in Chat | current turn Planner; one background singleflight probe |
| Auto capability cache miss/expired in Agent | wait for the shared bounded probe; supported admits the same request |
| Agent probe/cache remains unknown | preserve the native Tool round; only real explicit incompatibility may downgrade |
| Probe valid matching Tool Call   | shared `supported` row, seven-day TTL            |
| Probe explicit Tool incompatibility | shared `unsupported` row, 24-hour TTL          |
| Probe timeout/429/5xx/ordinary 400 | shared `unknown` retry backoff, five-minute TTL |
| Native first-round explicit incompatibility | async downgrade; same-turn unified Planner |
| Native first startup typed transient failure | exact same request retries once; no Tool has executed |
| Second typed transient startup failure | preserve fixed Provider category; no Local Skill/MCP wrapper |
| Startup retry wait is cancelled | stop immediately; no second request or Tool execution |
| Explicit AIHero Skill install link | Backend owner-private pinned-source install before Provider; zero Store search/review |
| Explicit non-direct Resource link | Backend bounded search before Provider; exact-authority install or completed no-install answer |
| Catalog ACL/consent/read failure | omit catalog; ordinary Auto/Planner behavior continues |
| Planner invalid/timeout/provider failure | strong Knowledge, forced Web, else Direct; never Both |
| Planner requests unavailable authority | reject plan and apply deterministic fallback |
| Tool arguments malformed/unknown | do not execute; redacted failed step             |
| External Search failure          | truthful degradation; ordinary answer; no `[W]`  |
| Exact public URL                 | safe direct read; normalized source and `[W]`     |
| Exact Discourse post URL         | same-origin topic JSON; exact post selection      |
| URL targets private/unsafe/oversized content | bounded Tool failure; zero outbound authority |
| One Web call succeeds and another fails | `partial`; retain successful Web evidence |
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
| Browser navigation/tab close cancels HTTP or breaks SSE | detach delivery; continue the Run to one durable terminal state |
| SSE write or flush fails after `message.started` | suppress later delivery; keep Provider/Tool/finalize work alive |
| Cancel during Provider/Tool      | cancel both; one terminal cancelled event        |
| Cancel during compatibility plan | Tool/Web/Generation cancelled; no `planner_failed` |
| Provider exposes no reasoning    | process only; no fabricated reasoning            |
| Successful Generation only       | persist Generation; reload summary is `Direct`    |
| Unknown process detail key       | drop before SSE/persistence                       |
| Exact query or redacted Tool args in process detail | drop before SSE/persistence        |
| Anthropic Thinking continuation  | retain block order/signature in memory only       |
| Anthropic failed Tool Result     | matching `tool_use_id` plus `is_error=true`       |
| Current user explicitly requests saved Memory | order `search_memory` first, use named `required`, and disable optional reasoning only for the first decision round |
| Current user discusses memory generally or submits an ordinary task | keep `tool_choice=auto`; do not force retrieval |
| Capability is `unknown` on an explicit read in Chat | start the bounded background probe and release no Tool Memory on that turn |
| Capability probe remains `unknown` for persisted Agent | preserve native Agent admission; normal required-Memory Tool policy applies |
| First product round returns no Memory call | flush buffered answer, perform zero hybrid retrieval, and keep ordinary chat |
| First product round returns one exact `search_memory({})` call | run bounded hybrid retrieval, rehydrate through migration `065`, and continue on the same Provider/model |
| Product Memory policy is absent/non-production or fixed Judge tuple drifts | fail closed to an empty/failed Memory Tool result; do not call v1 or switch Judge Provider/model |
| Global Memory Tool flag is true but canary allowlist is empty or user does not exactly match | do not expose `search_memory`; perform zero retrieval/Judge work |
| Canary UUID configuration is malformed or duplicated | fail server configuration validation; never normalize it into broader admission |
| Fixed Judge returns a typed transient Provider failure | retry at most twice with `Retry-After` precedence or five/ten-second waits; deterministic/protocol/provenance failures do not retry |
| Memory Tool continuation completes with projected rows | record immutable Usage for exactly those ordered rows in assistant finalization |
| Memory Tool is empty/failed or continuation recovers from the original request | record zero Tool Memory Usage links |
| SSE shows `search_memory` completed but Usage is zero | Treat this as a valid empty Tool result only when user health was `ready`, not as successful recall. Inspect current projection readiness and Worker heartbeat before changing routing or prompts. |
| Buffered first round fails after assembling a call but before closing | discard the draft/call, execute zero Memory retrieval, and use the original compatibility path |
| Memory call name differs by whitespace or case | reject; normalized display names are not contract authority |
| Memory call omits arguments or returns `null` | reject; nil map is not an explicit empty object |
| Memory call is unknown/duplicate/later-round | fail closed for Memory; never retrieve or accept a second Memory call |
| Memory retrieval returns empty with ready health | bounded successful empty Tool Result; ordinary continuation without Memory |
| Memory retrieval returns empty while indexing/unavailable/disabled/health-unknown | one bounded failed Tool step using `memory_indexing`, `memory_service_unavailable`, `memory_disabled`, or `memory_status_unavailable`; ordinary continuation without Memory |
| Memory retrieval otherwise fails | bounded failed Tool Result; ordinary continuation without Memory |
| Continuation fails before content after a Memory call | recover from the original request with no Memory body |
| Continuation fails after partial content | preserve the error; do not replay or duplicate the answer |
| Provider stream read fails or ends incomplete after visible content | preserve partial output as failed; emit `PROVIDER_STREAM_INTERRUPTED`; no replay or completion |
| Runtime Tool contract hash drifts | fail closed before Memory retrieval |
| Official DeepSeek receives `enable_thinking=false` | protocol mismatch; the run is not model-quality evidence |
| Official DeepSeek Tool round or continuation requests reasoning | adapter sends `thinking.type=disabled`, omits `reasoning_effort`, and leaves plain no-Tool chat unchanged |
| Official DeepSeek synthesizes fields for a server-declared zero-argument Tool | discard all members from a bounded valid JSON object and continue with canonical `{}`; malformed/non-object/oversized input remains denied |
| Generic compatible receives `thinking.type=disabled` | forbidden Provider-specific leakage; retain `enable_thinking=false` |
| Product Memory route adds a separate `PlanTools` preflight | reject the architecture; use the existing first Tool round |
| Diagnostic Provider error carries an upstream body/raw message into a retained report | reject the artifact; only a fixed category may cross the capture boundary |
| Unknown diagnostic cause reaches the capture recorder | map once to `ROUTER_FAILURE_UNCLASSIFIED`; never create a dynamic category |
| Diagnostic retrieval is incomplete | retain only a normalized aggregate code and require empty Final/Injected/token surfaces; do not erase the route result |

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
- Good: an unknown model in Chat answers through same-model Planner immediately
  while one user-data-free probe warms later turns; the same cache miss in Agent
  waits for that shared probe and enters the Tool Registry in the same request
  when supported.
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
- Good default-off product route: ordinary Server chat exposes no Memory Tool,
  makes no hybrid Provider call, and continues without Memory; it never invokes
  the retired reader.
- Good enabled product route: the selected Tool-capable model sees the normal
  first-round request and canonical Tool but no Memory body, calls
  `search_memory({})`, fixed BGE reranks current candidates, the current exact
  stored Luna tuple selects useful ordinals, and only their intersection enters
  the bounded Tool Result and immutable completed-answer Usage.
- Good explicit-read route: `你知道我的信息嘛` and `我喜欢喝什么？` force only
  the canonical `search_memory` first-round call; fixed BGE/Luna selection may
  still return an empty result, and the answer continuation uses no rejected
  candidate. Live acceptance of a known saved fact additionally requires the
  expected non-empty answer and exact Usage; a completed Tool step alone is not
  sufficient evidence.
- Base personal-question route: `你喜欢喝什么？`, `人们喜欢喝什么？`,
  `我应该喝什么？`, and `帮我写“我喜欢喝什么”的文案` remain Auto.
- Good detached route: the browser switches Conversations or closes after an
  accepted turn, SSE delivery fails, and the Provider/Tool loop still persists
  one complete assistant plus its exact Usage/capture state.
- Base enabled product route: an unrelated request yields no Memory call and
  its buffered first-round answer is released without hybrid retrieval.
- Base intent route: “what is long-term memory?” remains Auto and performs no
  forced personal retrieval.
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
  capability probe/cache write, persisting query/catalog/provider payloads, or
  treating an HTTP/SSE disconnect as explicit Run cancellation.

## 6. Tests Required

1. Fragmented Tool arguments, 64-KiB rejection, forced/Auto choice, and native
   continuation fixtures for each promoted provider family.
2. Search off/Auto skip/explicit Search I/O assertions.
3. Ordered reasoning/process SSE, terminal persistence, reload, redaction, and
   cancellation.
   Cancellation fixtures must cover compatibility planning, native Web,
   Knowledge, Handler persistence, no failure category, zero Citation, and a
   repeated run that detects event-delivery races. Separate disconnect fixtures
   must cancel the HTTP context and fail delivery before the first event and
   during a delta, assert the Provider context remains live, and verify one
   completed full assistant. Image generation and explicit Run cancellation
   require independent regressions.
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
19. Memory Tool tests must assert the exact definition/hash, default-off/direct-
    action/model-built-in exclusions, zero-call buffered answer release, exact
    first-round empty-object acceptance, missing/null/malformed/non-empty/
    non-exact-name/unknown/duplicate/later-round rejection, multi-tool
    coexistence, removal from later rounds, failure after an assembled first-
    round call but before execution, retrieval failure/empty continuation,
    final hydration drift/redaction, original-request recovery without Memory
    bodies, partial-content failure, same Provider/model continuation, and
    Development adapter delegation to the canonical contract. Historical
    schema-v6 thinking-control tests remain immutable protocol coverage but do
    not define schema-v7 decoding authority. Product tests additionally pin the
    separate production policy, exact Provider/type/Base-URL hash/model/secret
    authority, tuple re-resolution, typed Judge two-retry schedule, deterministic
    no-retry behavior, zero v1 fallback, bilingual explicit-read and direct
    first-person preference positives, second-/third-person/advice/quoted-task
    negatives, Memory-first named-required ordering, first-decision reasoning
    suppression, unchanged ordinary Auto choice,
    bounded no-thinking capability probes, and official DeepSeek Tool/
    continuation versus plain-chat wire shapes. Provider-adapter tests must
    additionally pin zero-argument JSON-object canonicalization, generic
    compatible byte preservation, argument-bearing Tool preservation, and
    malformed argument denial.
    Product admission tests must additionally cover exact UUID match,
    case-normalized canonical UUIDs, unauthenticated/non-canary/empty fail-
    closed behavior, and zero retrieval/Judge calls outside the canary.
20. Diagnostic tests must cover HTTP/transport/SSE/context classification,
    malformed/remote stream events, bounded adapter mapping, unknown-cause
    fallback, absence of Provider response/error text, schema-v9 route and
    retrieval aggregate reconciliation, incomplete-retrieval empty-final
    enforcement, and explicit empty v9 maps while v7 bytes omit every
    diagnostic field.
21. Public stream-error tests must map typed read/incomplete failures to
   `PROVIDER_STREAM_INTERRUPTED`, preserve any already-emitted answer as a
   failed partial message, and prove upstream error text is absent.
22. Native Tool startup tests must prove one exact retry for synchronous and
    first-event typed transient failures, zero retry for deterministic
    failures, cancellation during the wait, and preservation of the second
    fixed Provider category even when Local Skill/MCP/Resource runtimes exist.
    Explicit Resource-link tests must prove zero Provider calls. AIHero must
    perform one owner-private mutation with zero Store calls; other links retain
    bounded search, completed zero-candidate behavior, and one mutation only
    for a unique exact admitted candidate.

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

```text
// Wrong: rely on every answer model to interpret an explicit Memory command
// under Auto, or force all ordinary turns through Memory.
explicit read -> tool_choice=auto
every turn    -> tool_choice=required
```

```go
// User cancellation becomes a false degraded Search failure.
status := ProcessStepStatusFailed
failureCategory := "planner_failed"
```

```go
// Wrong: browser delivery owns Provider execution.
streamCtx, cancel := context.WithCancel(r.Context())
if writeSSEEvent(w, event, payload) != nil {
    cancelAssistant()
}
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

```go
// Wrong: normalization silently broadens the hash-bound Tool contract.
if normalizedToolName(call.Name) == "search_memory" {
    executeMemory()
}
```

Correct after the owning G19 promotion:

```text
search mode + selected Knowledge + capabilities
  -> bounded governed catalog + capability resolution
  -> known native: expose allowed tools with Auto choice
  -> Chat unknown or confirmed unsupported: same-model Direct|Knowledge|Web|Both Planner
  -> Agent cache miss: await shared probe; supported/unknown native, unsupported Chat
  -> no Tool Call: answer
  -> Tool Call: validate/execute/trace -> native continuation
  -> reconcile only current-turn used citations -> persist
```

```go
// Chat never waits; Agent shares and awaits only the bounded probe.
requested := requestedChatToolMode(conversation.Metadata)
status, probe := resolveFromOverridesCacheOrProbe(providerConfigHash, modelID)
if requested == chatToolModeChat && status == ToolCapabilityUnknown {
    return compatibilityPlanner
}
if requested == chatToolModeAgent && status == ToolCapabilityUnknown {
    status = awaitSharedProbe(probe)
    if status != ToolCapabilityUnsupported {
        return nativeAgentToolRound
    }
}
```

```text
// Correct: current-user intent controls only Tool selection; the fixed Judge
// still controls which current-authorized Memory bodies may be released.
explicit saved-Memory read       -> search_memory first + named required
ordinary/general turn            -> auto
Chat unknown capability          -> no Memory this turn + background fixed probe
Agent unknown after bounded probe -> native required/auto Tool policy
```

```go
// Correct: HTTP disconnect detaches delivery; explicit Run cancel owns work.
generationCtx := context.WithoutCancel(r.Context())
streamCtx, cancelRun := context.WithCancel(generationCtx)
activeRuns.register(runID, cancelRun)
delivery := newBestEffortStreamWriter(w)
```

```go
// Retry only the same already-resolved read-only execution once.
result, err := service.Execute(ctx, execution, request)
```

```text
// Wrong: runtime presence overwrites the cause.
Provider 502 + Local Skill enabled -> LOCAL_SKILL_PROVIDER_FAILED

// Correct: retry once before any Tool, then preserve typed authority.
Provider 502 -> same Provider/model/request once -> PROVIDER_UPSTREAM_FAILED
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
// Correct product Memory route behind the default-off flag.
first StreamToolRound(normal request + allowed read-only tools + search_memory)
  -> no Memory call: release buffered first-round answer
  -> one exact first-round call with ID and explicit {}
  -> production policy + current fixed Luna tuple reauthorization
  -> fixed BGE rerank -> fixed Luna ordinal intersection
  -> migration-065 current-authority final hydration
  -> same-provider/same-model continuation without search_memory
  -> completed answer: persist exact projected Tool-result Usage
  -> pre-content continuation failure: original-request recovery without Memory
```

```text
// Wrong: the global switch admits every authenticated account.
MEMORY_TOOL_LOOP_ENABLED=true -> expose search_memory to all users

// Correct: infrastructure plus exact current-user admission.
MEMORY_TOOL_LOOP_ENABLED=true
  + authenticated UUID in MEMORY_TOOL_LOOP_CANARY_USER_IDS
  -> expose search_memory only for that request
```

```text
// Wrong product integration after the failed schema-v6 Development preflight.
PlanTools(search_memory) -> Provider request #1
  -> StreamToolRound(answer) -> Provider request #2

// Correct implemented product architecture.
first StreamToolRound(search_web + search_knowledge + search_memory)
  -> execute the exact called read-only tools
  -> same-model continuation with bounded authorized results
```

```go
// Correct: only the exact raw contract name can authorize Memory retrieval.
if call.Name != usermemory.HybridMemoryToolName {
    return "unknown_tool"
}
```

Operational rollback for catalog quality regressions is to omit
`WithKnowledgeRoutingCatalog` from Handler wiring: native models retain generic
Auto Tool behavior and compatibility routing remains bounded. Rolling back
migration `042_model_tool_capability_cache` drops only derived capability cache
state; no chat, Knowledge, credential, or provider configuration data is lost.

Full target contract: `mm-chat/docs/contracts/chat-tool-loop.md`.

## Scenario: Continue an interrupted final answer without replaying Tools

### Scope / trigger

Apply when changing `PROVIDER_STREAM_INTERRUPTED`, Assistant regeneration,
stream request fields, durable Agent event recovery, or post-Tool answer
continuation.

### Contract

- `continuationOfMessageId` is Backend-authorized from one owned durable source
  Assistant. Require `failed`, exact `PROVIDER_STREAM_INTERRUPTED`, non-empty
  partial content, and the same submitted User parent.
- Create a new sibling Assistant. Never mutate the failed source Turn, append to
  its ended event stream, copy its Tool events, or persist a synthetic User row.
- Fail closed when an Agent source has no durable events or any latest Tool
  state is pending, running, awaiting approval, interrupted, or
  `outcome_unknown`.
- Use only bounded Backend-sanitized presentation evidence and frame it as
  untrusted data. Raw Tool arguments/results remain unavailable by design.
- Physically bypass every Tool/Search/RAG/Memory/direct-action/context-summary
  preparation and call only plain `Provider.StreamChat`. Browser config cannot
  re-enable a runtime.
- Emit the exact preserved prefix before suffix deltas and persist their
  combination. Another exact interruption may continue from the new longer
  partial sibling.
- Keep Regenerate distinct: it is the explicit full rerun and may execute Tools
  again.

### Required proof

- Focused Handler tests prove plain-stream-only dispatch with a provider that
  also implements `ToolRoundProvider` and would fail if called through a Tool
  round.
- Reject wrong role/status/error/parent, empty content, missing Agent events,
  and every unresolved Tool state.
- Prove source content/events remain unchanged and the continuation sibling has
  no Tool events or Memory capture.
