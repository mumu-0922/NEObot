# Chat Agent Runtime Operations

## Current deployment

Agent mode has two explicit execution routes. A Conversation grouped under a
bound Host Workspace uses the ordinary-user WSL Agent Host and reports
`mode=host_workspace`. An ungrouped legacy Conversation retains Docker
`local_direct`. A Host-bound Conversation never falls back to Docker.

Create the two bind sources as the normal runtime user before first start:

```bash
cd mm-chat
mkdir -p data/agent-skills data/agent-workspace
chmod 700 data/agent-skills data/agent-workspace
docker compose --env-file .env.single-server --profile app up -d --build
```

Compose uses `create_host_path: false`; a missing bind fails visibly instead of
creating a root-owned host directory. Backend runs with
`MM_CHAT_RUNTIME_UID:GID`, uses `init: true`, and mounts only:

```text
./data/agent-skills    -> /var/lib/mm-chat/agent-skills
./data/agent-workspace -> /workspace
```

See [`local-skill-runtime.md`](./local-skill-runtime.md) for every supported
limit and the user smoke flow.

## Security truth

Docker `local_direct` is not a permission Sandbox. Bound Host Workspaces use a
probed Bubblewrap write boundary: Read Only exposes the Host tree read-only,
Workspace Write adds one exact read-write Workspace bind, and Full access uses
the ordinary Host process user's authority with approval disabled. These modes
do not promise read confidentiality or network isolation and never elevate via
`sudo`. Do not bind `$HOME`, `.env.single-server`, `secrets/`, `backup/`, the
container socket, or unrelated projects into Docker. Installed Skill content
is untrusted; keep Docker `AGENT_LOCAL_APPROVAL_MODE` at `smart`.

## Skill Store and connectors

The standalone Skill Store uses `/v1/skills/*`; Assistant library, Knowledge,
Memory, Files, and MCP Tools remain separate product surfaces. MCP may provide
Browser or external connectors, but users do not configure an Agent Runner.

Conversational discovery/install is controlled independently:

```bash
RESOURCE_ORCHESTRATION_ENABLED=true
```

Set it to `false` and recreate only Backend to remove the Agent resource Tools
and reject `/v1/resources/install`. Read-only catalogs and the existing Skill
Store/MCP management pages remain available for diagnosis and rollback.

## Upgrade to migration 098

Before applying the retirement migration:

1. create a matched PostgreSQL and MinIO backup set;
2. stop any obsolete binaries from an older release;
3. verify no legacy fact table contains rows;
4. preserve `data/`, `secrets/`, `backup/`, and the live env file;
5. run the disposable PostgreSQL 17 drill.

Two legacy tables always contain one migration-created singleton state row and
are accepted only in that exact one-row shape. Any other old row aborts the
whole migration before an object is dropped.

```bash
bash scripts/verify-legacy-agent-cleanup-postgres17.sh
docker compose --env-file .env.single-server --profile ops run --rm migrate
```

After the current migration set, head must be
`103_chat_agent_permission_modes`,
`chat_agent_turns/events/goals` and Skill tables must exist, and no legacy
Agent control-plane relation/function/role may remain. The ledger checksum for
`096_chat_agent_event_log` must remain the production-applied
`f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042`;
`099` carries the idempotent gateway repair instead of rewriting that history;
`101` forward-widens the same authority for bounded Context/Reasoning blocks;
`102` extends the existing Workspace registry and adds immutable Conversation
execution snapshots; `103` adds durable checked per-Conversation permission.
The Host socket supports status, browse, native Windows selection, canonical
resolution, durable binding, bounded Tool execution, and advertises all three
permission modes only after exact WSL/DrvFS enforcement probes pass.

## Verification

```bash
bash scripts/verify-agent-local-runtime.sh
bash scripts/verify-chat-agent-event-log-postgres17.sh
bash scripts/verify-chat-agent-goals-postgres17.sh
bash scripts/verify-chat-artifacts-postgres17.sh
bash scripts/verify-standalone.sh --full
```

A Host Workspace smoke should execute `pwd`, `git status --short`, File read,
write/CAS, one background Job, and refresh replay in the selected Host project.
It must also prove Read Only denies all writes, Workspace Write denies an
outside write while allowing an inside write, and acknowledged Full access can
write outside under the ordinary Host user's authority.
Stopping the Host must make the next bound Tool fail without creating anything
under Docker `/workspace`; restarting the same pinned Runner restores it.

## Rollback

Disable new local execution without deleting data:

```bash
# Edit only the operator-owned live env file.
AGENT_LOCAL_RUNTIME_ENABLED=false

docker compose --env-file .env.single-server --profile app \
  up -d --force-recreate backend
```

This keeps installed packages and workspace files. Migration 098 does not
recreate the retired control plane on `down`; database rollback requires the
matched pre-upgrade PostgreSQL/MinIO restore and the previous application image.
Never attempt to reconstruct obsolete tables manually and never delete runtime
paths as part of source rollback.
