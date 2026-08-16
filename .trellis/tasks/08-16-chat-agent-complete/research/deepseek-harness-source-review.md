# DeepSeek Harness source review and Neo Chat gap map

## Source baseline

- Upstream: `https://github.com/deepseek-ai/deepseek-harness.git`
- Full clone inspected at `/tmp/deepseek-harness-review.bbTFhb/source`
- Default branch: `master`
- Reviewed revision: `47f943859bef60e4160492346772ded9b24f765a`
- Local revision matches `origin/master` after `git fetch --prune origin`.
- The clone contains 7,412 tracked files and 284,496 packed Git objects; it is
  not a shallow README-only checkout.

## What DeepSeek Harness actually does

### Turn/Step loop

`packages/core/agent-loop/src/agent.ts` owns the real agent loop. A Turn opens,
then executes one or more Steps. Each Step streams an assistant message. No Tool
Call completes the Turn; Tool Calls are executed and their result contexts enter
the next Step. Relevant source ranges: `245-330` and `332-400`.

`packages/core/agent-loop/src/tool-calls.ts` schedules calls, records
`tool/call`, executes calls, commits results in model order, records
`tool/result`, and hands result messages to the next Step. Safe calls may run in
parallel, while exclusive calls remain barriers.

### Tool Registry

`packages/core/tools/src/index.ts` provides a registry and one normalized
execution pipeline. A Tool owns its name, description, parameter schema,
canonical output schema/rendering, execution body, optional content finalizer,
timeout metadata, concurrency classification, and replayable UI presentation.
Execution passes through pre-policy, approval, monotonic guards, around-execute,
post-policy, final content materialization, and final result notification.

### Skill progressive disclosure

`packages/skill/tool-skill/src/index.ts` registers one `skill({name})` Tool.
The session receives a bounded Skill catalog. The prompt explicitly requires the
model to call `skill` before acting when the user names a Skill or the task
clearly matches its description. `/skill-name` is a deterministic user
invocation. Catalog changes publish a complete replacement, including an empty
replacement when every Skill disappears.

`packages/skill/skill-filesystem/src/index.ts` is one provider implementation.
It watches project/user roots and invalidates the catalog after filesystem
changes. Neo Chat should preserve its server-owned Store rather than copying
this host-directory precedence model.

### Durable session, Goal, and long context

`packages/core/session/src/index.ts` models a Session as an append-only event
log with contiguous sequence numbers and a Surface projection used to derive
the exact model history. Tool calls/results and assistant messages are replayable
events rather than final-message-only metadata.

`packages/goal/goal/src/*`, `packages/goal/goal-round-driver/src/*`, and
`packages/goal/tool-goal/src/*` implement a persisted same-session Goal with
revision CAS, phases, round caps, automatic continuation, completion evidence,
and process-local activation. Restored active Goals are deliberately disarmed
until a human asks to resume.

Compaction first prunes oversized Tool Results using head/tail replacements,
then summarizes a balanced Surface region without splitting Tool Call/Result
pairs. A provider context-overflow can retry only after durable Surface
reduction.

### Important limitations not to misrepresent

- `packages/jobs/jobs-local/src/index.ts` explicitly stores Jobs in a
  process-local in-memory Map. Jobs can outlive a chat Step/Turn but not a
  service restart.
- The persistent terminal registry is also process-local.
- The shipped standard preset sets `tool-web.config.fetch: false`.
- The repository has no production GUI Browser Tool. Playwright usage is for
  the Harness Web UI tests.
- Code Mode (`run_code` plus a generated TypeScript/Python Tool SDK) is a
  round-trip optimization, not the foundation of agent correctness.

## Neo Chat current runtime truth

### What already works

`mm-chat/backend/internal/chat/web_tool_loop.go` already has a same-model native
Tool loop. It iterates rounds, accumulates assistant calls and Tool results into
`ProviderToolExchange`, and continues until the model returns no call or a
configured budget/terminal condition ends the loop.

The `local_direct` implementation already executes commands as the ordinary
Backend user in the configured workspace. It needs no sudo, Podman, OCI Runner,
or per-Skill isolation and must remain that way for this product direction.

### Why it does not feel like a complete Agent

1. `local_skill_tool_loop.go` exposes three hard-coded functions:
   `skills_list`, `skill_view`, and `terminal`. The catalog is already injected,
   yet the prompt says only “when needed” and asks the model to list again.
2. `web_tool_loop.go` dispatches MCP, Local Skill, Memory, Knowledge, and Web
   through separate batches and a large per-name branch rather than one Tool
   Registry.
3. Chat Tool execution emits live SSE and later serializes a sanitized
   `processTrace` into the terminal assistant message metadata. It does not
   persist each Turn, Step, Tool Call, and Tool Result as an independently
   replayable event.
4. The G20/G21 `agentorchestrator` event log is a separate control-plane model.
   Its event vocabulary records Run/Step/Attempt/lease transitions, not model
   messages and Tool results, and its README explicitly says importing the
   package does not enable Agent execution.
5. `agentcontrol.EnqueueRootRun` admits only a fixed Product Canary request.
   Agent Center is therefore not the conversational execution path.
6. There is no same-session Goal continuation, mutation verification gate,
   File Tool suite, background Job completion inbox, or context compaction in
   ordinary Chat.

## Recommended adoption

Adopt the behavior and contracts, not the Cordis framework:

1. One `skill({name})` loader and mandatory catalog-matching rule.
2. A native Go Tool Registry shared by every current Chat Tool.
3. An explicit Turn/Step driver over the existing provider adapters.
4. A Chat-owned append-only event stream for model/tool replay.
5. A same-session Goal domain with human re-arm after restart.
6. Read-before-write File Tools, background Jobs, and compaction.
7. Browser capability through a registered Tool/MCP provider when implemented.
8. Optional Code Mode only after the native path is stable.

Do not copy Cordis, default Subagents/Workflow/Ralph, host-local Skill
precedence, OCI isolation, or Developer Preview packages. Do not expose a
Subagent Tool in the initial or default catalog.
