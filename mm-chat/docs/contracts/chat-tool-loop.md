# Chat Tool Loop and Process Trace Contract

## 1. Scope

G19 replaces pre-answer forced Search with a server-owned, provider-normalized
Tool Loop. It applies to chat generation, provider continuation, Search mode,
selected Knowledge/Memory retrieval, live process visibility, persisted
reasoning and trace data, tool approval, cancellation, and source
reconciliation.

The active native Tool catalog includes read-only retrieval/Skill discovery and
the explicitly executing local `terminal` Tool:

```text
search_web(query)
search_knowledge(query)
search_memory()  # default-off; first round only
skill(name)      # when local_direct is enabled and the user installed Skills
file_read(path, offset?, limit?)
file_write(path, content, expectedVersion)
file_edit(path, oldText, newText, replaceAll, expectedVersion)
file_search(path?, query, glob?, maxResults?)
publish_file(path, displayName, contentType)
terminal(command, skill?, workingDir?, timeoutSeconds?, runInBackground)
job_list()
job_output(jobId, wait, timeoutSeconds?)
job_kill(jobId)
get_goal()
create_goal(objective, maxGoalRounds?)
update_goal(goalId, revision, action, objective?, maxGoalRounds?, blockedReason?)
verify_completion(evidenceToolCallId, summary)
```

The four Goal Tools are present only when the selected model supports native
Tools and the Repository implements migration-`097` Goal persistence. This
keeps legacy/fake repositories on their prior behavior.

Every active Tool now enters one server-owned Registry with its exact Provider
definition, Backend executor, `read|write|execute|external` risk class, timeout,
output budget, parallel permission, optional approval rule, model Result
projector, and replayable `search|tool` presentation. Name collisions fail
closed, and the default Registry contains no Subagent or delegation Tool.

The local Tool family implements workspace execution plus progressive Skill
disclosure. Every Turn receives a bounded complete catalog replacement with a
content-derived revision; an empty catalog is an explicit tombstone. `skill`
loads the selected package's `SKILL.md`, while File/Job/`terminal` Tools operate
in the configured local workspace. An empty catalog omits only `skill`; the
workspace, Job, and `terminal` Tools remain available whenever `local_direct`
is enabled.
The retired `skills_list` and `skill_view` names remain execution-compatible
for a bounded migration period but are never advertised to the model.
`local_direct` is not an isolated Sandbox. Its contract is
[`local-skill-runtime.md`](./local-skill-runtime.md).

Selected MCP Tools join this same Registry under their frozen provider-safe
aliases. The reviewed `Browser (Playwright)` manifest artifact is a real
headless browser path for navigation, accessibility snapshots, clicks, form
input, tabs, and waits; it is not an HTTP-fetch compatibility shortcut. Its
page process is scoped to one Chat Run, and the manifest excludes arbitrary
Playwright code/evaluation, file upload, request-body inspection, and storage-
state injection.

`search_memory` is absent unless `MEMORY_TOOL_LOOP_ENABLED=true`. The schema-v7
answer-model routing evidence remains failed and immutable, but the owner later
promoted the separately passing fixed schema-v14 BGE/Luna selection semantics
for this product Tool. The configuration default remains `false`; the flag is
the immediate rollback boundary and no Tool failure falls back to v1.

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
    FollowupPrompt     string
    Checkpoint         string
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

The implementation treats one user request as a Turn and every Provider call
as a Step. A required Skill load is a `skill_prelude` Step and does not consume
the ordinary task-Step sequence; this preserves first-task-Step Memory/Search
authority. The compatibility `round` field currently carries the physical Step
sequence for provider-loop ordering. Durable events project the existing
process updates without adding a provider sideband that could change Tool
scheduling or cancellation order.

The Registry is rebuilt for every Step from current Backend authority. Calls
execute in the model's original order. A contiguous group of at most four
explicitly safe reads may execute concurrently across reviewed MCP and local
backends; the completed Results are still committed to the continuation in the
original call order. Local parallel reads are limited to `file_read`,
`file_search`, `job_list`, `job_output`, and legacy `skill_view`. Every
write/execute/unknown/retrieval/Goal call is an ordered barrier. `skill` is also
a barrier because it changes the Turn-local loaded catalog state, and
`mcp_tool_search` is a barrier because it changes the visible MCP catalog for
the next Step. A concluding Goal call rejects only later calls in that batch as
`goal_concluded`; earlier calls are never reordered behind it.

Tool output cannot register arbitrary names. Parallel execution shares the
parent cancellation context, cancels sibling reads after a terminal failure,
and never widens MCP classification authority: only a current reviewed
`read` policy may set `AllowParallel`.

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

Before the next Provider Step, a Tool Result larger than 64 KiB is replaced by
a UTF-8-safe 24-KiB head and tail with an omission marker. If the complete Turn
continuation still exceeds 256 KiB, complete old `ProviderToolExchange` units
are replaced by one bounded synthetic `Checkpoint`; the newest four exchanges,
including their Provider-private state, remain exact. A checkpoint contains
only Tool name, call ID, success/failure, and result byte count, never command,
arguments, file content, or raw Tool output.

A typed `PROVIDER_CONTEXT_OVERFLOW` is recognized only from HTTP `413` or an
allowlisted stable upstream JSON `code|type|status|reason`; free-form upstream
messages do not classify it. The loop may aggressively compact to 6-KiB
Tool-Result heads/tails and two exact recent exchanges, then retry the same
Provider/model/Step once only if the continuation became smaller. Both a
synchronous response and the first SSE error event use this path. A second
overflow is terminal and never loops. Every shrink is persisted as a
content-free `context.replaced` event.

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

The Turn has hard caps of 32 Provider Steps and 128 Tool Calls in addition to
the lower MCP and `local_direct` per-Run call, round, wall-clock, output, and
concurrency budgets. A call beyond the Turn cap receives a structured
`turn_call_budget_exhausted` Result without execution. Any exhausted budget
then causes one same-model continuation without Tools, except that an
outstanding completion-verification requirement fails with
`AGENT_VERIFICATION_REQUIRED` rather than permitting an unverified success.
The loop otherwise
terminates when:

- the model returns no Tool Call;
- the user cancels the run;
- the request context or configured provider timeout ends;
- a Tool/Provider returns a terminal non-degradable error; or
- an approval is rejected.

Unknown names, bad arguments, ordinary execution errors, and a per-Tool
deadline are Tool Results so the same model can repair or report them. In
particular, an MCP call deadline is `tool_timeout` while the parent Run remains
healthy. Cancellation, a parent Run deadline, and a write whose outcome is
unknown remain terminal and are never converted into retryable Results.

### Code Mode decision

The default catalog does not expose a general `run_code` Tool. The native Tool
path already provides the correctness boundary, fallback, replay, risk policy,
and completion verification. The upstream Playwright
`browser_run_code_unsafe` Tool is RCE-equivalent in the MCP process and is
explicitly outside the reviewed Browser allowlist. A future Code Mode may be
added only as an optional round-trip optimization over the same Registry; it
must not replace or weaken native Tool fallback.

### Same-session Goal and completion verification

Migration `097` stores at most one current Goal for each Conversation. Goal
phase is `active|paused|blocked|complete`; every mutation carries the exact Goal
ID and revision. `create_goal`, `edit`, `pause`, `resume`, and `cancel` require
the direct human request. A restored active Goal starts disarmed; the model may
re-arm it with `resume` only when the human explicitly asks to continue.
Activation itself is process-local and is never reconstructed as armed after a
Backend restart.

An armed active Goal automatically starts another Goal Round when a model Step
would otherwise end without a Tool Call. The Backend appends
`goal.round.started`, injects a synthetic user `FollowupPrompt`, and continues
the same Provider/model in the same SSE request. OpenAI-compatible framing is
`assistant -> user`; Anthropic preserves the exact assistant Thinking blocks
before the synthetic user prompt. Goal budgets default to 8 and are constrained
to 3-32 rounds. Automatic `blocked` is unavailable before round 3; the prompt
also requires the same blocking condition to persist rather than treating
difficulty or incomplete work as a blocker.

Successful `write` or `execute` calls activate a process-local completion gate.
The Agent must observe a successful Tool result at or after the latest mutation
and call `verify_completion` with that exact Tool Call ID and a bounded truthful
summary. Goal Tools themselves never count as mutation or evidence. An active
gate prevents normal completion and prevents `update_goal(..., complete)`.
Failure to satisfy it before Step/Tool/runtime exhaustion is
`AGENT_VERIFICATION_REQUIRED`.

`file_write` and `file_edit` cannot verify their own mutation; a later
`file_read`, `file_search`, or suitable command must observe the result. A
background `terminal` start, `job_list`, `job_kill`, or a non-completed
`job_output` is also not evidence. Only `job_output` with `status=completed`
may verify a successful background command.

`complete`, `blocked`, and `cancel` enter a Tool-free wrap-up. This state is
latched for the rest of the Turn: even if a Provider hallucinates a Tool Call
while `Tools=nil`, the Backend returns only `goal_concluded`, performs no side
effect, keeps Tools disabled, and asks the same model for the closing answer.
Intermediate Goal/verification narration is buffered and never merged into the
final visible answer. Unexpected Goal persistence failure is terminal
`AGENT_GOAL_PERSISTENCE_FAILED`.

Local Skill process events contain only Tool name, round, `local_direct`, risk
classification, optional timeout, duration, and failure category. Command text,
working-directory text, stdout, stderr, Skill file content, and storage paths
must never enter SSE process metadata or persisted process trace.

Provider-facing local Skill functions keep `strict=true`. Every declared
property is therefore present in `required`; semantically optional values use
nullable JSON Schema types and explicit `null` maps to the runtime default.
This preserves OpenAI-compatible strict-schema admission without weakening the
Backend's unknown-field, path, command, timeout, or package validation.

The current claimed user message may select Skills before the ordinary task
round:

- an exact installed Skill name, or one unique strong lexical match against a
  catalog description, queues that Skill as a required prelude;
- the prelude exposes only `skill`, constrains `name` to the exact match,
  disables incompatible thinking modes, and cannot execute MCP, retrieval, or
  `terminal` first;
- after all required Skills load, the ordinary first task round still owns the
  existing explicit Memory/Search priority;
- a whitespace-bounded `/skill-name` token deterministically reads the current
  installed `SKILL.md` into a user-authorized instruction block before the
  first Provider round. Unknown names and path-like tokens remain ordinary
  prose;
- a second `skill` call for the same name and catalog revision returns a
  bounded `alreadyLoaded` acknowledgement instead of repeating the content.

The shared `tool.call.updated` Chat event accepts `mode=mcp` with
`read|write|unknown`, or `mode=local_direct` with `read|write|execute`. The frontend
must validate the pair rather than rejecting a valid local event as an invalid
MCP update; `execute` never widens MCP Server classification authority.

Workspace File Tools accept only workspace-relative paths and use anchored
`os.Root` operations. Reads return a complete-file `sha256:<hex>` version;
writes and edits require that exact version (`absent` only for creation), write
through a same-directory synced temporary file, recheck for external change,
and atomically rename. Version drift returns `version_conflict`, not overwrite.
All file sizes, UTF-8 windows, search files/bytes/results, and previews have
hard bounds.

Background Jobs are process-local, share foreground terminal concurrency and
Run timeout limits, and are authorized by exact user plus Conversation.
`job_output(wait=true)` blocks at most ten seconds on completion rather than
busy-polling. Completion notices enter the next Agent Step/request without
output. `Executor.Close` kills and reaps Jobs; the UI receives
`durability=process_local` and warns that Backend restart cannot recover them.

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
  initial chat request with automatic tool selection. A bounded bilingual gate
  over only the current user message makes one exception: explicit saved-
  Memory read/use/search commands and direct personal recall questions order
  `search_memory` first and require that exact Tool.
- Explicit Search intent must force `search_web` or native Search within the
  selected mode even when automatic selection would skip it.
- When selected Knowledge is in scope and capability is confirmed unsupported,
  or when persisted Chat sees unknown capability, the same selected model
  performs one bounded unified plan:

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
- An unknown persisted Chat turn uses Planner immediately and starts one
  background singleflight synthetic probe. An unknown persisted Agent turn
  waits for that shared bounded probe; `supported` admits the same request,
  confirmed `unsupported` downgrades to Chat, and transient/inconclusive
  `unknown` preserves the adapter-native Tool round. Provider save/activation
  also prewarms the first model and matching task models. No request waits for
  the best-effort cache write.
- The probe contains a fixed fictional Tool and fixed prompt only, with
  thinking disabled, temperature zero, and maximum output `128`. It never
  includes user text, conversation, catalog, source bodies, raw provider
  payloads, or credentials. A valid matching completed Tool Call records
  `supported`; explicit Tool incompatibility records `unsupported`; timeout,
  cancellation, 429, 5xx, transport/ordinary 400, and inconclusive output stay
  `unknown`.
- Official `api.deepseek.com` native Tool rounds and Tool continuations send
  `thinking.type=disabled` and omit `reasoning_effort`; plain no-Tool DeepSeek
  chat retains the selected reasoning settings. Generic compatible gateways
  retain their own wire shape and never receive the DeepSeek-only field.
- Official DeepSeek may synthesize fields such as `query` despite a
  server-declared zero-argument Tool schema. For only those zero-argument
  functions, the adapter canonicalizes a bounded valid JSON object to `{}`
  before validation and continuation and grants none of its members query
  authority. Malformed/non-object/oversized arguments remain invalid; generic
  compatible Providers and argument-bearing Tools remain byte-preserved. The
  canonical Memory Tool definition and hash do not change.
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
not recover. They retain their existing terminal behavior. A typed Provider
stream-read/incomplete failure after visible content uses
`PROVIDER_STREAM_INTERRUPTED`, preserves that content in a failed assistant,
and is never represented as an MCP Tool failure. Provider reasoning alone does
not count as answer content and may precede a recovery answer.

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
read     -> server-authorized observation
write    -> ordered local/state mutation
execute  -> ordered command/process control
external -> server-authorized external I/O/effect policy
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
  | "cancelled"
  | "outcome_unknown"
  | "interrupted";

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

Migration `096` makes a separate Chat-owned append-only event log the durable
process authority for new assistant Messages:

```text
chat_agent_turns(message_id, run_id, status, next_sequence, started_at, ended_at)
chat_agent_events(turn_id, sequence, event_id, event_type, step_sequence,
                  payload, occurred_at)

turn.started / turn.ended
step.started / step.ended
assistant.message
tool.called / tool.result
goal.changed / goal.round.started
context.replaced
```

`chat_agent_append_event` locks the Turn and allocates `next_sequence`; it never
derives the next value with `MAX(sequence)`. Event IDs are replay-safe and event
rows are immutable. The API runtime has SELECT plus the two exact append
functions and no direct event-table DML. `agent_run_events` remains the
independent optional G20/G21 control-plane stream and never receives full Chat
Tool Results.

The backend persists each process or Tool projection before emitting the same
sanitized projection over SSE. Historical message reads attach `agentEvents`;
the frontend sorts and deduplicates valid events and prefers their process
projection over legacy `metadata.processTrace`. Old Messages without valid
events retain the legacy fallback. Tool event payloads retain only bounded
display facts such as name, mode, risk, status, duration, Round and public
Server label; command, arguments, query, raw Result, credentials, private
Server refs and paths are forbidden.

Job-related process rows may additionally retain only
`durability=process_local`. `context.replaced` payloads retain reason,
before/after byte counts, pruned-result count, and replaced-exchange count;
they never retain removed context.

At startup, a `running` Turn older than the recovery cutoff receives one
terminal event. An already terminal assistant Message keeps its committed
`completed|failed|cancelled` status. Only a `pending|streaming` or invalid
Message state becomes `interrupted`, which atomically marks the Message failed
with `AGENT_RUN_INTERRUPTED`. The frontend closes any still-active projected
steps as `interrupted`. Message finalization and the terminal event are not one
database transaction; this reconciliation is the explicit torn-write boundary.

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
- Prefer the durable Chat Agent event projection on reload and use
  `metadata.processTrace` only for pre-`096` or invalid-event compatibility.
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
- Explicit saved-Memory read intent uses named `required` with `search_memory`
  first and disables optional reasoning only for that first decision round.
  General questions about memory and ordinary tasks remain `auto`. A normal
  same-model continuation restores the selected answer settings; official
  DeepSeek Tool continuations remain thinking-disabled for protocol
  compatibility.
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
- Candidate-empty retrieval is successful only when user-scoped Memory health
  is `ready`. Indexing, unavailable/failed workers or projections, disabled
  Memory, and unreadable health become distinct bounded failed Tool steps.
  Failed, stale, or fully redacted retrieval likewise returns a bounded Tool
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
| Provider stream read/incomplete failure after visible answer text | failed partial answer with `PROVIDER_STREAM_INTERRUPTED`; no replay |
| First recovery attempt emits partial text then fails | discard all events; retry once |
| Both recovery attempts fail             | final failure with zero recovery answer content |
| Later Search adds no source            | empty incremental Tool Result; keep prior markers  |
| Built-in capability unavailable       | mode disabled or degraded; no external fallback    |
| Native Tool confirmed unsupported     | same-model unified compatibility planner           |
| Auto capability cache miss/expired in Chat | current turn Planner; background singleflight probe |
| Auto capability cache miss/expired in Agent | wait for shared bounded probe; supported admits the same request |
| Agent probe/cache remains unknown     | preserve native Agent Tool round; do not synthesize Chat metadata |
| Valid/explicitly incompatible probe   | shared supported/unsupported TTL row               |
| Transient/inconclusive probe          | shared five-minute unknown retry backoff            |
| Runtime explicit incompatibility      | async downgrade plus same-turn Planner              |
| Catalog ACL/consent/read failure      | omit metadata; chat continues without forced RAG    |
| Compatibility planner fails           | strong Knowledge, forced Web, else Direct; never Both |
| Knowledge miss                        | empty successful result; continue Model/Web        |
| Tool arguments malformed/unknown      | reject execution; redacted failed step             |
| Workspace path/symlink escape         | bounded `path_invalid`; no read/write               |
| Workspace version changed             | bounded `version_conflict`; no overwrite            |
| Background Job cross-scope lookup     | `job_not_found`; no existence disclosure            |
| Background Job still running          | no completion evidence; wait/read later             |
| Backend restarts with a Job           | Job is killed/reaped; no recovery claim              |
| First typed context overflow after a real shrink | retry same Provider/model/Step once        |
| Second overflow or no smaller continuation | terminal Provider failure; no retry loop       |
| Memory Tool flag absent/false         | do not expose `search_memory`; continue without Memory and never invoke the old reader |
| Explicit saved-Memory read on a supported model | order `search_memory` first; named `required`; no forced Web/Knowledge |
| General memory discussion/ordinary task | preserve `tool_choice=auto`; no forced Memory retrieval |
| Explicit read while Chat capability is unknown | no Memory this turn; start the fixed background probe |
| Explicit read after Agent probe remains unknown | preserve native Agent admission and required-Memory Tool policy |
| No first-round Memory call            | zero hybrid retrieval; release buffered answer     |
| Exact first-round `search_memory({})`  | current-authorized hybrid retrieval and same-model continuation |
| Buffered first round fails after a call is assembled | discard call/draft; compatibility path; zero Memory retrieval |
| Memory call name is whitespace/case variant | reject; only the exact raw name authorizes retrieval |
| Memory retrieval empty with ready health | successful empty Tool Result; continue without Memory |
| Memory indexing/unavailable/disabled/unknown health | bounded failed Tool Result with a safe visible reason; continue without Memory |
| Memory retrieval failed/stale         | bounded failed Tool Result; continue without Memory |
| Memory continuation fails before text | recover from original request with no Memory body  |
| Memory continuation fails after text  | preserve error; no duplicate recovery              |
| Official DeepSeek Tool round/continuation | `thinking.type=disabled`, no `reasoning_effort`; plain no-Tool chat unchanged |
| Official DeepSeek adds fields to a server-declared zero-argument Tool | discard all members from the bounded JSON object and validate/continue with canonical `{}`; never adopt a model-generated query |
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
4. Native Auto Tool for known-supported models and persisted Agent unknowns;
   unified four-route Planner for confirmed-unsupported or persisted-Chat
   unknown selected-Knowledge models, with Direct, Knowledge, Web, and Both
   fixtures.
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
    thinking-disabled/temperature-zero/output-128 bounds, valid-call
    classification, transient unknown, TTL/config-hash isolation, background
    warmup, Agent waiters sharing one probe, same-request workspace Tool
    admission, confirmed-only downgrade, Chat non-blocking behavior,
    non-blocking cache writes, and optional multi-instance PostgreSQL visibility.
15. Frontend provider round-trip and Inherit cleanup plus durable query-free
    Direct/Knowledge/Web/Both summaries, dual source counts, and reason
    allowlisting.
16. Memory Tool exact definition/hash, default-off/direct-action/model-built-in
    exclusions, first-round buffering, no-call release, exact `{}` acceptance,
    malformed/null/non-empty/unknown/duplicate/later-round denial, multi-tool
    coexistence, migration-065 final hydration drift, secret re-redaction,
    same-model continuation, body-free recovery, and no v1 fallback/Usage
    mutation. Include bilingual explicit-read positive/general-memory negative
    intent cases, Memory-first named-required ordering, first-decision
    reasoning suppression, unchanged ordinary Auto behavior, and official
    DeepSeek Tool/continuation versus plain-chat thinking wire shapes. Pin
    zero-argument JSON-object canonicalization plus malformed, generic
    compatible, and argument-bearing preservation negatives.
17. Workspace traversal/symlink/UTF-8/size/search bounds, read-version-write,
    external-change conflict, atomic replacement, and mandatory post-write
    evidence.
18. Background Job scope authorization, foreground concurrency sharing,
    wait-without-polling, completion notice, timeout/kill/process-group reap,
    shutdown, restart warning, and completion-evidence gating.
19. UTF-8 Tool-result pruning, whole-exchange checkpointing, latest Provider
    state preservation, stable overflow classification, synchronous and first-
    SSE shrink/retry, and proof that a second overflow never retries.
20. Safe-read scheduler overlap across MCP/local backends, a four-call bound,
    write/execute/catalog/retrieval barriers, original Result order, and Goal
    conclusion affecting only later calls.
21. Playwright MCP initialize/list/navigate/snapshot smoke against a local
    fixture, exact manifest allowlist filtering, per-Run instance binding, and
    proof that `browser_run_code_unsafe` cannot reach the Runner call route.

## 11. Rollback

For a catalog-quality regression, omit `WithKnowledgeRoutingCatalog` from the
Handler wiring. Native providers retain generic Auto Tool behavior and
compatibility routing remains bounded; no Knowledge or chat data changes.
Migration `042_model_tool_capability_cache` contains derived capability state
only. Its down migration drops the cache table without removing provider
configuration, credentials, conversations, Knowledge, or Citations.

For a Memory Tool regression, set `MEMORY_TOOL_LOOP_ENABLED=false` and restart
the API. This removes Tool exposure and continues without Memory; it never
restores the retired v1 prompt/Usage path and does not delete Memory,
projections, observations, health state, or migration `065`.

For a local workspace/Job regression, set
`AGENT_LOCAL_RUNTIME_ENABLED=false` and restart the API. This removes File,
Job, `terminal`, and `skill` definitions without deleting installed Skills or
workspace files. Process-local Jobs are killed during shutdown; no OCI fallback
is activated.

For a Browser regression, remove `manifest:playwright-browser-0.0.79` from the
Conversation selection or set `MCP_STDIO_ENABLED=false` and restart the API.
This removes the Browser Tool surface without changing `local_direct` Skills,
other remote MCP Servers, Conversations, or stored user data.
