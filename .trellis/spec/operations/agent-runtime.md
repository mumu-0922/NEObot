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
bash mm-chat/scripts/verify-chat-agent-approvals-postgres17.sh
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
- A nonempty `AGENT_LOCAL_WORKSPACE_HOST_ROOT` may identify only the exact
  canonical Host directory already bound to `/workspace`. It enables pasted
  Linux/WSL path aliases but adds no mount or second authority. Never bind its
  Home/parent directory merely to expose several unrelated projects.
- Roll back local execution by setting `AGENT_LOCAL_RUNTIME_ENABLED=false` and
  recreating Backend only. Never delete packages, workspace, `data/`,
  `secrets/`, `backup/`, or the live env.
- Compose must not contain legacy Agent control, Canary, Cron/Learning, product,
  or relay services/networks. Production override must not reference them.
- Before migration `098`, take a matched PostgreSQL/MinIO backup and run its
  disposable PostgreSQL 17 drill. Nonempty legacy facts stop the upgrade.
- `098.down` does not recreate execution authority; restore the matched backup
  plus previous image for database rollback.
- Migration `100` is the durable Chat Agent approval head. Runtime roles have
  no Approval/Grant table DML and use only the three hardened gateways. Startup
  invokes recovery before serving traffic so no pending pre-restart command can
  resume. Roll back the UI/runtime path by disabling Agent local execution; do
  not down a database that contains approval/grant rows.
- `AGENT_TIMELINE_ENABLED` defaults false and
  `AGENT_TIMELINE_CANARY_USER_IDS` defaults empty. Both are API-only; exact
  authenticated UUID admission is required. Clearing either is the immediate
  UI/control rollback and must preserve durable Agent events and legacy
  ProcessStep projection.
- Legacy ProcessStep removal requires one content-free, verifier-eligible
  focused canary session on unchanged immutable Backend/Frontend digests and
  Git commit. Its UTC end must be later than its start; no arbitrary soak time
  is required. Use exactly one canary plus a disjoint control; five synthetic
  Agent turns and Tool calls; Terminal/Search/File/MCP plus the full event
  chain; live/reload/reconnect/approval/cancel/retry gates; the 300 ms
  visible-update and 20% durable-reload limits; zero leak probes; and a
  flag-only rollback rehearsal that preserves events and unrelated services.
  Evidence files remain outside Git at mode `0600`; only the content-free
  report may be retained operationally.

### Validation matrix

| Condition | Required result |
| --- | --- |
| bind source missing/not writable | visible startup/preflight failure; no privileged repair |
| Host alias differs from or escapes the mounted project | reject the Tool input; do not add another bind |
| local runtime false | local Tools absent; packages/workspace preserved |
| zero installed Skills | workspace Tools remain available |
| API restart | prior process-local Jobs unavailable and labeled non-durable |
| legacy Compose/env/binary path returns | local runtime gate fails |
| legacy fact exists | migration 098 fails atomically |
| cleanup, repair, approval migration succeed | head 100; immutable 096 checksum and Chat/Skill/MCP/File/Memory data retained |
| timeline flag false, canary invalid/empty/non-matching | invalid config stops preflight/startup; otherwise legacy projection only |
| focused-canary evidence is missing or verifier-ineligible | keep legacy projection and exact-user scope; do not widen or retire transport |

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
- Run `verify-chat-agent-approvals-postgres17.sh` for fresh/replay head, exact
  grants, CAS, expiry, restart denial, guarded down, and clean down/up.
- Run Backend vet/tests, frontend gates, RAG gates, and standalone full.
- Prove protected runtime paths remain untouched by source cleanup.
- Run `bash mm-chat/scripts/test-agent-timeline-canary-evidence.sh`; a real
  removal additionally requires an external eligible report from the pinned
  focused acceptance session.

### Wrong vs correct

```text
Wrong: sudo mkdir/chown + privileged Runner + second Agent service
Correct: normal-user bind roots + Backend UID/GID + explicit local limits

Wrong: run 098.down and expect the old database authority to return
Correct: restore the matched pre-098 PostgreSQL/MinIO set and previous image
```
