# Chat Agent Runtime Operations Contract

## Scenario: operate the single-server `local_direct` backend

### Scope / trigger

Apply when changing the Backend runtime image, Agent environment, Compose
mounts, workspace authority, shutdown, backup/restore, migration `098`, or
verification scripts.

### Signatures

```bash
mkdir -p mm-chat/data/agent-skills mm-chat/data/agent-workspace
docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server.example \
  -f mm-chat/compose.single-server.yml config --quiet
bash mm-chat/scripts/verify-agent-local-runtime.sh
bash mm-chat/scripts/verify-legacy-agent-cleanup-postgres17.sh
```

### Contracts

- Compose defaults `AGENT_LOCAL_RUNTIME_ENABLED=true`; the bare binary defaults
  false. Supported configuration is only the `AGENT_LOCAL_*` set documented in
  `.env.single-server.example`.
- Run as `MM_CHAT_RUNTIME_UID:GID` with `init: true`. Bind the normal-user-created
  Skill cache and workspace with `create_host_path: false`.
- The Backend image includes Bash, Python/pip, Node/npm, Git, curl, jq, ripgrep,
  zip/unzip, and `file`. Never require `sudo`, OCI/Podman, a second daemon,
  certificates, a machine restart, or a container socket.
- Workspace access is deliberate Backend-user read/write access, not Sandbox
  isolation. Keep secrets, live env, backups, and unrelated files outside it.
- Roll back local execution by setting `AGENT_LOCAL_RUNTIME_ENABLED=false` and
  recreating Backend only. Never delete packages, workspace, `data/`,
  `secrets/`, `backup/`, or the live env.
- Compose must not contain legacy Agent control, Canary, Cron/Learning, product,
  or relay services/networks. Production override must not reference them.
- Before migration `098`, take a matched PostgreSQL/MinIO backup and run its
  disposable PostgreSQL 17 drill. Nonempty legacy facts stop the upgrade.
- `098.down` does not recreate execution authority; restore the matched backup
  plus previous image for database rollback.

### Validation matrix

| Condition | Required result |
| --- | --- |
| bind source missing/not writable | visible startup/preflight failure; no privileged repair |
| local runtime false | local Tools absent; packages/workspace preserved |
| zero installed Skills | workspace Tools remain available |
| API restart | prior process-local Jobs unavailable and labeled non-durable |
| legacy Compose/env/binary path returns | local runtime gate fails |
| legacy fact exists | migration 098 fails atomically |
| cleanup and repair succeed | head 099; immutable 096 checksum and Chat/Skill/MCP/File/Memory data retained |

### Good / base / bad cases

- **Good**: a normal user creates both bind sources, Preflight validates exact
  ownership and bounds, and Compose starts only the ordinary Backend runtime.
- **Base**: `AGENT_LOCAL_RUNTIME_ENABLED=false` removes local Tools without
  deleting installations, packages, or workspace files.
- **Bad**: use `sudo` to repair bind ownership, mount Docker/Podman sockets, or
  recover across `098` by manually recreating retired roles and tables.

### Required tests

- Render Compose with example and active env files.
- Run `verify-agent-local-runtime.sh` and the PostgreSQL 17 cleanup drill.
- Run Backend vet/tests, frontend gates, RAG gates, and standalone full.
- Prove protected runtime paths remain untouched by source cleanup.

### Wrong vs correct

```text
Wrong: sudo mkdir/chown + privileged Runner + second Agent service
Correct: normal-user bind roots + Backend UID/GID + explicit local limits

Wrong: run 098.down and expect the old database authority to return
Correct: restore the matched pre-098 PostgreSQL/MinIO set and previous image
```
