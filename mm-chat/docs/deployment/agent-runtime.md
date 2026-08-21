# Chat Agent Runtime Operations

## Current deployment

Agent mode runs inside the ordinary single-server Backend through
`local_direct`. No second machine, `sudo`, Podman/OCI daemon, Runner certificate,
Canary worker, or per-Skill container is required.

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

`local_direct` is not isolated from the Backend container. Commands can change
workspace files and reach networks available to Backend. Do not bind `$HOME`,
`.env.single-server`, `secrets/`, `backup/`, the container socket, or unrelated
projects. Installed Skill content is untrusted; keep `AGENT_LOCAL_APPROVAL_MODE`
at `smart` unless a test explicitly requires otherwise.

## Skill Store and connectors

The standalone Skill Store uses `/v1/skills/*`; Assistant library, Knowledge,
Memory, Files, and MCP Tools remain separate product surfaces. MCP may provide
Browser or external connectors, but users do not configure an Agent Runner.

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

After the current migration set, head must be `102_host_workspaces`,
`chat_agent_turns/events/goals` and Skill tables must exist, and no legacy
Agent control-plane relation/function/role may remain. The ledger checksum for
`096_chat_agent_event_log` must remain the production-applied
`f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042`;
`099` carries the idempotent gateway repair instead of rewriting that history;
`101` forward-widens the same authority for bounded Context/Reasoning blocks;
`102` extends the existing Workspace registry and adds immutable Conversation
execution snapshots. The interactive Host socket now supports workspace
status, browsing, native Windows selection, canonical resolution, and durable
binding, but it does not route Tools until execution and permission
capabilities are implemented and advertised.

## Verification

```bash
bash scripts/verify-agent-local-runtime.sh
bash scripts/verify-chat-agent-event-log-postgres17.sh
bash scripts/verify-chat-agent-goals-postgres17.sh
bash scripts/verify-chat-artifacts-postgres17.sh
bash scripts/verify-standalone.sh --full
```

A chat smoke should cover Skill installation, one `skill -> terminal -> verify
-> publish_file -> answer` flow, refresh replay, authorized download, and a
cross-user download denial.

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
