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
- A model without native Tool-round support executes effective Chat while the
  stored user choice remains unchanged.
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
- The runtime enforces call timeout, Run timeout, output bytes, Tool calls,
  Tool rounds, and concurrency. `smart` approval denies destructive patterns;
  the catastrophic blocklist remains active even with approval mode `off`.
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
authority is `chat_agent_turns`, `chat_agent_events`, and `chat_agent_goals`.
Message metadata is compatibility projection, not a second event authority.

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

## Required verification

```bash
bash scripts/verify-agent-local-runtime.sh
bash scripts/verify-legacy-agent-cleanup-postgres17.sh
bash scripts/verify-chat-artifacts-postgres17.sh

cd backend
GOCACHE=/tmp/neo-chat-go-cache go vet ./...
GOCACHE=/tmp/neo-chat-go-cache go test ./...
```
