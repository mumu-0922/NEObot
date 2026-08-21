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
- Migration `100` owns durable Chat Agent approvals; migration `101` is the
  current forward-only Transcript event head. Runtime roles have
  no Approval/Grant table DML and use only the three hardened gateways. Startup
  invokes recovery before serving traffic so no pending pre-restart command can
  resume. Roll back the UI/runtime path by disabling Agent local execution; do
  not down a database that contains approval/grant rows.
- `AGENT_TIMELINE_ENABLED` defaults false and
  `AGENT_TIMELINE_CANARY_USER_IDS` defaults empty. Both are API-only; exact
  authenticated UUID admission is required. Clearing either is the immediate
  UI/control rollback and must preserve durable Agent events and legacy
  ProcessStep projection.
- Widening the typed transport beyond exact canaries or deleting the legacy
  control/rollback fallback requires one content-free, verifier-eligible
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
| cleanup, repair, approval/transcript migrations succeed | head 101; immutable 096 checksum and Chat/Skill/MCP/File/Memory data retained |
| timeline flag false, canary invalid/empty/non-matching | invalid config stops preflight/startup; otherwise legacy projection only |
| focused-canary evidence is missing or verifier-ineligible | keep the exact-user scope and legacy control/rollback fallback; do not widen or delete it |

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
- Run `verify-chat-agent-event-log-postgres17.sh` for the migration-101 event
  whitelist, hardened append gateway, immutable replay, and clean head 101.
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

## Scenario: operate the dark WSL Agent Host foundation

### Scope / trigger

Apply when changing `scripts/agent-host.sh`, `scripts/test-agent-host.sh`, Host
runtime paths, identity/token lifecycle, or the future Backend socket mount.

### Signatures

```bash
cd mm-chat
./scripts/agent-host.sh build
./scripts/agent-host.sh start
./scripts/agent-host.sh prepare
./scripts/agent-host.sh status
./scripts/agent-host.sh stop
./scripts/agent-host.sh restart
```

Default runtime state:

```text
.runtime/agent-host/                 mode 0700, disposable process state
secrets/agent-host-token             mode 0600, persistent, create-if-absent
secrets/agent-host-runner-id         mode 0600, persistent, create-if-absent
```

### Contracts

- Run as the ordinary WSL user. Both the wrapper and binary refuse effective
  UID `0`; do not use `sudo` as an ownership repair.
- Do not require a user systemd manager. The wrapper owns build, `nohup` plus
  `setsid` detachment,
  startup, exact PID recording, bounded authenticated health, stop, and stale
  PID/socket handling.
- Keep `.runtime/` gitignored. Keep the stable token and Runner id in the
  existing protected `secrets/` tree because the current root-owned `data/`
  directory is not writable by the ordinary Host user. Never chmod/chown or
  delete `data/` to make the Runner start.
- Generate token and Runner id only when absent via an exclusive same-directory
  install. Never overwrite, rotate, print, stage, or delete them implicitly.
- Require absolute, canonical non-symlink runtime/identity paths, owner match,
  private modes, and non-group/other-writable identity parents.
- A PID is signalable only when `/proc/<pid>/cmdline` names the exact configured
  binary and it is not a zombie. A malformed or unrelated live PID fails
  closed; the wrapper does not clean it up.
- Build through a temporary binary and persist a bounded fingerprint of the
  Host Go sources/module files. `prepare` reuses a healthy process only when
  protocol, Runner id, and source fingerprint all match; changed code restarts
  the exact managed process.
- The dark foundation is not mounted into the Docker Backend and changes no
  live Tool routing. A future Compose slice must mount only the exact socket
  directory plus independent token, add a default-off feature flag and health
  projection, and preserve fail-closed Host-bound routing.
- Rollback stops the process and reverts source. Preserve persistent identity
  files unless the owner explicitly authorizes credential destruction.

### Validation and error matrix

| Condition | Required result |
| --- | --- |
| wrapper or binary runs as root | refuse before listening |
| state directory is symlinked, wrong owner, or not `0700` | refuse without repair |
| token/Runner id is unsafe or malformed | refuse without overwrite |
| healthy protocol/id/version already active | reuse the process |
| managed old-version process | bounded stop, then start the new build |
| PID points to unrelated live process | refuse to signal or remove PID file |
| owned stale socket | same-file recheck, replace, bind mode `0600` |
| active socket | refuse a second listener |
| Host stopped during dark rollout | no effect on current Backend/Frontend/DB |

### Good / base / bad cases

- **Good**: `prepare`, `status`, repeated `prepare`, and `stop` complete as the
  normal user while token/Runner id remain byte-stable.
- **Base**: no systemd user manager exists; wrapper lifecycle remains complete.
- **Base**: `mm-chat/data/` is root-owned and non-writable; the wrapper uses
  `.runtime/` without changing protected runtime ownership.
- **Bad**: `sudo mkdir/chown mm-chat/data` or delete a PID/socket/token merely
  because startup failed.
- **Bad**: mount the Host user's Home directory, Docker socket, unrelated
  secrets, or arbitrary parent directory into Backend.

### Tests required

```bash
cd mm-chat
bash -n scripts/agent-host.sh scripts/test-agent-host.sh
bash scripts/test-agent-host.sh
cd backend
go test -race ./internal/agenthost ./cmd/agent-host
go vet ./internal/agenthost ./cmd/agent-host
```

The lifecycle smoke uses only a temporary tree and must prove build, start,
authenticated status, idempotent same-version prepare, explicit restart,
identity/token preservation, and stop. A source-deploy smoke may then run the
default `prepare` and `status`; it must not recreate Compose services.

### Wrong vs correct

```text
Wrong: sudo ./scripts/agent-host.sh prepare
Correct: ./scripts/agent-host.sh prepare as the project-owning WSL user

Wrong: kill $(cat pid) without verifying process identity
Correct: parse private PID -> verify exact /proc cmdline -> signal -> recheck

Wrong: Runner rollout -> rebuild/recreate all Docker services
Correct: focused Go/lifecycle gates -> start dark Host only -> leave live Compose unchanged
```

## Scenario: deploy the durable Host Workspace schema dark

### Scope / trigger

Apply when releasing migration `102`, the `/v1/workspaces*` Backend API, or the
later Host socket mount that changes binding availability.

### Signatures

```bash
cd mm-chat
docker compose --env-file .env.single-server --profile ops run --rm migrate
docker compose --env-file .env.single-server --profile app up -d backend
./scripts/agent-host.sh status
```

```text
Expected database head: 102_host_workspaces
Dark bind result:         503 HOST_WORKSPACE_UNAVAILABLE
```

### Contracts

- Apply `102` with the migrator credential before recreating Backend; API
  startup never runs migrations.
- This slice does not mount the Host socket/token into Backend. The Host process
  may remain healthy and dark while Workspace settings become durable.
- Build and recreate Backend only. Do not rebuild Frontend, RAG, Postgres,
  Redis, MinIO, or MCP Runner for this Backend/schema-only slice.
- Preserve `data/`, `secrets/`, `backup/`, `.env.single-server`, the stable Host
  token/Runner id, and existing project directories.
- `102.down` is clean only before any imported settings, Host binding, or
  execution snapshot. After durable state exists, use the compatible previous
  application path or a matched backup; never purge records to force down.
- A later socket rollout must mount only the exact private runtime directory
  and independent token, pin the persisted Runner id, add health projection,
  and keep existing unbound Conversations on their current behavior.

### Validation and error matrix

| Condition | Required result |
| --- | --- |
| schema remains at 101 with new Backend | readiness/API smoke fails; do not serve Workspace writes |
| migration 102 empty down/re-up | succeeds and restores exact grants/constraints |
| migration 102 down after imported/bound state | `HOST_WORKSPACE_ROLLBACK_BLOCKED`; head/data unchanged |
| Host healthy but socket not mounted | list/import work; bind is generic `503` |
| Host stopped during dark schema rollout | no Tool routing change |
| protected runtime paths differ after release | release fails review |

### Good / base / bad cases

- **Good**: disposable PG17 replay and runtime-role tests pass, migration runs,
  Backend-only image is recreated, head is 102, API health/list pass, and bind
  fails closed without a path leak.
- **Base**: the Host process is stopped; the durable API remains usable except
  bind, while existing `local_direct` execution is unchanged.
- **Bad**: mount `$HOME`, rewrite the Host token, rebuild every service, or
  down/purge durable Workspaces merely to return to schema 101.

### Tests required

```bash
cd mm-chat/backend
go test -race ./internal/hostworkspace ./internal/agenthost \
  ./internal/migration ./internal/httpserver ./cmd/api
go vet ./internal/hostworkspace ./internal/agenthost \
  ./internal/migration ./internal/httpserver ./cmd/api
cd ..
./scripts/agent-host.sh status
docker compose --env-file .env.single-server ps backend postgres
```

Also query `schema_migrations` numerically for exact head `102`, exercise one
authenticated Workspace list/import, and prove the bind response is `503`
without the submitted path.

### Wrong vs correct

```text
Wrong: new Backend image -> auto-migrate on startup -> mount arbitrary Host root
Correct: explicit migrate -> Backend-only recreate -> dark 503 until exact socket wiring

Wrong: rollback requires 101 -> delete Workspace/project state -> migrate down
Correct: preserve state -> use compatible image or restore an operator backup
```
