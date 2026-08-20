# Chat Agent Runtime Executable Contract

## Scope

This contract covers the user-facing Agent mode implemented by the ordinary Go
Backend. It combines persisted Chat/Agent policy, installed Skills, bounded
local workspace Tools, Browser/MCP connectors, durable Tool events and Goals,
and protected output attachments.

It excludes Subagents, delegation, OCI/Podman Runner control, per-Skill
containers, Canary runs, schedules, autonomous learning, and Agent Center.

## Mode authority

- The authenticated persisted Conversation is the mode authority.
- `config.toolMode` is `"chat"` or `"agent"`; missing/invalid historical values
  mean Agent.
- A model with confirmed missing native Tool-round support executes effective
  Chat while the stored user choice remains unchanged. Auto-capability cache
  misses wait for one shared bounded probe in Agent; transient/inconclusive
  unknown results retain the adapter-native Agent round.
- Chat mode physically omits Skill, File, Terminal, Job, Goal, Browser, and MCP
  Tool definitions. Knowledge, Memory, Search, and non-Tool image generation
  remain available.
- Agent mode uses the same model for Tool continuation and never delegates to a
  child agent.

## Skill and Tool authority

- `/v1/skills/*` is the only Skill Store/library API.
- Package discovery, admission, installation, revision checks, and uninstall
  are server-authoritative.
- `SKILL.md` and package files are untrusted input. Installation does not imply
  execution; the Backend registry and current user installation remain gates.
- `local_direct` runs as the Backend UID/GID only under the configured Skill
  cache and workspace roots.
- An optional clean `AGENT_LOCAL_WORKSPACE_HOST_ROOT` is an input alias for the
  exact directory already mounted as `/workspace`; it does not authorize a
  second root. Linux absolute and WSL UNC inputs must map below it before the
  existing relative-path and `os.Root` checks run.
- The runtime enforces call timeout, Run timeout, output bytes, Tool calls,
  Tool rounds, and concurrency. In `smart` mode, a destructive Terminal call
  creates a durable five-minute approval and waits for `Allow once`, an
  authorized conversation grant, or `Deny`. The catastrophic blocklist remains
  active and cannot be bypassed even with approval mode `off`.
- Jobs are process-local and must be presented as non-durable. Shutdown cancels
  and reaps them; restart does not resume them.
- No Docker/Podman socket, host-wide home bind, privileged user, or automatic OS
  package installation is allowed.

## Tool loop and persistence

Each Agent Turn follows:

```text
user input -> persisted policy/context -> model -> Tool call -> durable event
-> bounded execution -> durable result -> same model -> assistant response
```

The Tool registry, arguments, results, approvals, errors, cancellation, and
Goal state follow the ordinary `internal/chat` contracts. Current durable
authority is `chat_agent_turns`, `chat_agent_events`, `chat_agent_goals`,
`chat_agent_approvals`, and exact conversation Tool grants. Approval rows are
mutable decision authority only; `chat_agent_events` remains the immutable
timeline/presentation authority. Message metadata is compatibility projection,
not a second event authority.

Pending approvals survive browser refresh because their sanitized presentation
is already in the durable event stream. The decision API uses the presented
revision as CAS input; the first terminal decision wins and later duplicate or
conflicting tabs receive that current row without overwriting history. Approval
waits expire after five minutes. Backend startup denies every still-pending row
with `restart_denied`, because the original process and waiter cannot be proved
recoverable. Conversation grants are scoped to the exact Conversation, Tool
name, and risk class. Approval storage never contains Tool arguments or results.

Browser transport loss does not stop the Run. Each active Run retains a bounded
process-local SSE cursor ring and exposes an authenticated `after` resume
endpoint. Retained events replay in original sequence; evicted data produces an
explicit gap and the UI converges on the final persisted Message snapshot. This
delivery buffer is not durable Tool authority and never causes per-chunk event
rows.

Background Terminal execution is displayed as one Job lifecycle. Durable Agent
events retain each immutable start/output/kill Tool fact plus only safe Job
metadata; the UI joins those facts by exact `jobId`. Because Jobs are
process-local, startup recovery projects any unresolved `running` or `stopping`
Job as `interrupted` rather than claiming that it survived the Backend.

Typed timeline rollout is server-authoritative. It requires
`AGENT_TIMELINE_ENABLED=true` and an exact authenticated UUID in
`AGENT_TIMELINE_CANARY_USER_IDS`. When either gate is absent, Backend continues
durable Agent event writes but returns only the legacy ProcessStep projection
and does not enter a hidden approval wait. Clearing either gate is the
non-destructive rollback.

## Artifact publication

- `publish_file` exists only in effective Agent mode and accepts a
  workspace-relative path.
- The executor resolves the path beneath the configured workspace, rejects
  traversal/symlinks/non-regular files, and enforces size and count budgets.
- The File service rebinds ownership to the authenticated user and writes bytes
  through the configured object store.
- Only successfully published files become `purpose=output` assistant
  attachments. Refresh replays them from persisted message/file authority.
- Cross-user download, deleted files, arbitrary host paths, and unlinked
  temporary artifacts fail closed.

## Runtime configuration

The supported Agent environment is exactly:

```text
AGENT_LOCAL_RUNTIME_ENABLED
AGENT_LOCAL_RUNTIME_SOURCE
AGENT_LOCAL_RUNTIME_ROOT
AGENT_LOCAL_WORKSPACE_SOURCE
AGENT_LOCAL_WORKSPACE_ROOT
AGENT_LOCAL_WORKSPACE_HOST_ROOT
AGENT_LOCAL_SHELL
AGENT_LOCAL_APPROVAL_MODE
AGENT_LOCAL_CALL_TIMEOUT
AGENT_LOCAL_RUN_TIMEOUT
AGENT_LOCAL_MAX_OUTPUT_BYTES
AGENT_LOCAL_MAX_CALLS_PER_RUN
AGENT_LOCAL_MAX_ROUNDS_PER_RUN
AGENT_LOCAL_MAX_CONCURRENT
```

The single-server Compose default is enabled; the bare Backend binary default
is disabled. Invalid enabled roots, shell, approval mode, durations, or numeric
bounds stop Backend startup.

## Legacy retirement

Migrations `084`–`095` are published history and are never rewritten.
Migration `098_retire_legacy_agent_control_plane`:

1. acquires exclusive locks on an explicit 48-table whitelist;
2. requires all 46 fact tables to contain zero rows;
3. allows only the one bootstrap row in each singleton state table;
4. aborts atomically on partial schema, object drift, or retained facts;
5. removes explicitly named triggers, constraints, functions, views, tables,
   grants, and 19 NOLOGIN roles without `CASCADE` or wildcard deletion;
6. is intentionally irreversible; source plus matched database/object backups
   are the rollback path.

A previously completed cleanup re-applies as a no-op. `chat_agent_*`, Skills,
MCP, Chat, Knowledge, Memory, and Files are outside every cleanup whitelist.

The production-applied bytes of `096_chat_agent_event_log` are immutable.
Migration `099_chat_agent_event_log_function_repair` forward-repairs the two
Chat event gateways, reasserts their hardened `search_path` and exact runtime
grants, and keeps those corrected bodies on down. Never repair an applied
migration by changing its source bytes or `schema_migrations.checksum`.

Migration `100_chat_agent_approvals` adds the approval/CAS and exact
conversation-grant authorities. The API runtime has no table DML and may call
only `chat_agent_create_approval`, `chat_agent_decide_approval`, and
`chat_agent_recover_approvals`. Down is refused while either table contains
data.

## Required verification

```bash
bash scripts/verify-agent-local-runtime.sh
bash scripts/verify-legacy-agent-cleanup-postgres17.sh
bash scripts/verify-chat-artifacts-postgres17.sh
bash scripts/verify-chat-agent-approvals-postgres17.sh

cd backend
GOCACHE=/tmp/neo-chat-go-cache go vet ./...
GOCACHE=/tmp/neo-chat-go-cache go test ./...
```
