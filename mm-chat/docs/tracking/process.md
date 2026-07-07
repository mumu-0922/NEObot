# mm-chat Refactor Process Log

Record each completed action here. Keep entries factual: date, action, evidence, decision, next step.

## 2026-07-07 — Initial Refactor Workspace

### Action

Created the isolated `mm-chat/` workspace and generated the first design documents.

### Evidence

Files created:

```text
mm-chat/README.md
mm-chat/docs/architecture/server-refactor-design.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Repository findings used for the plan:

```text
Current app: Next.js/React/TypeScript
Current durable browser metadata: IndexedDB/localforage
Current browser file storage: OPFS
Existing S3/MinIO integration: not found
Target single-server stack: Go + Postgres + Redis + MinIO, optional Python FastAPI RAG
```

### Decision

Use a strangler migration instead of direct rewrite:

```text
Keep frontend stable
Add API boundary
Introduce Go backend
Move conversations/messages to Postgres
Move file bodies to MinIO
Add Redis only for temporary state
Add Python RAG only after core chat is stable
```

All future refactor work should stay under `mm-chat/` until a later task explicitly migrates a specific piece into the existing application.

### Verification

- Confirmed `mm-chat/` did not exist before creation.
- Created planning, progress, and process documents only under `mm-chat/`.
- No existing application source file was intentionally modified for this documentation step.

### Next Step

Review and lock MVP scope, then begin Phase 1 inventory:

```text
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/storage.md
mm-chat/docs/inventory/chat-flow.md
mm-chat/docs/inventory/provider-flow.md
```

## 2026-07-07 — Initial Documentation Verification

### Action

Ran a lightweight Markdown structure and checklist verification for the new `mm-chat/` documents.

### Evidence

```text
ok: mm-chat markdown structure and completed checklist verified
```

Confirmed tracked scope for this step:

```text
mm-chat/
.trellis/tasks/07-07-mm-chat-server-refactor-design/  # workflow metadata
```

### Decision

Full `pnpm` checks were not run because this step changed documentation only and `node_modules/` is not installed in the workspace. Application source code was not modified by this step.

### Next Step

Start Phase 1 inventory and create:

```text
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/storage.md
mm-chat/docs/inventory/chat-flow.md
mm-chat/docs/inventory/provider-flow.md
```

## 2026-07-07 — Phase 1 Static Inventory

### Action

Completed the first static inventory pass for existing API routes, service wrappers, local storage, OPFS usage, chat streaming, and provider flow.

### Evidence

Inventory documents created:

```text
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/storage.md
mm-chat/docs/inventory/chat-flow.md
mm-chat/docs/inventory/provider-flow.md
```

Key findings:

```text
src/app/api/**/route.ts contains 25 current API route files.
src/services/api/chatService.ts owns the browser-side streaming workflow.
src/lib/api/chat-handler.ts owns current provider stream dispatch.
src/lib/providers/base.ts owns OpenAI/Gemini client construction and API-key validation.
src/store/storage/storageConfig.ts defines localStorage and IndexedDB storage keys.
src/utils/opfs.ts owns opfs:// file storage helpers.
```

### Decision

Treat chat streaming as the first backend migration spine. Defer plugins, code execution, document parsing, voice, and full RAG until the server chat path is stable.

### Verification

Static inspection covered:

```text
src/app/api
src/services
src/lib/api/chat-handler.ts
src/lib/providers
src/store/storage
src/utils/opfs.ts
src/store/README.md
src/services/README.md
```

Updated `mm-chat/docs/tracking/progress.md` Phase 1 checklist to mark completed inventory outputs.

### Next Step

Begin Phase 2 by defining `mm-chat/docs/contracts/frontend-api-client.md`, including local/server mode boundaries and feature flags.


## 2026-07-07 — Documentation Directory Reorganization

### Action

Moved `mm-chat` documentation into a categorized `docs/` tree and added category indexes for future work.

### Evidence

New documentation layout:

```text
mm-chat/docs/README.md
mm-chat/docs/architecture/server-refactor-design.md
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/storage.md
mm-chat/docs/inventory/chat-flow.md
mm-chat/docs/inventory/provider-flow.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
mm-chat/docs/contracts/README.md
mm-chat/docs/deployment/README.md
```

### Decision

Keep only the workspace entrypoint at `mm-chat/README.md`. All detailed planning, inventory, contracts, deployment, and tracking docs now live under `mm-chat/docs/`.

### Verification

Updated root README links and progress references to point at the new docs paths.

### Next Step

Start Phase 2 contract work in:

```text
mm-chat/docs/contracts/frontend-api-client.md
```


## 2026-07-07 — Phase 2 Frontend API Client Contract Draft

### Action

Created the first Phase 2 contract for the frontend API client boundary.

### Evidence

New/updated documents:

```text
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

The contract defines:

```text
ApiMode: local | server
chatApi / fileApi / authApi / settingsApi / providerApi
server endpoint mapping
SSE event envelope and event types
error envelope and error matrix
migration sequence and test requirements
```

### Decision

Keep `local` mode as the default rollback path. Server mode remains opt-in behind `NEXT_PUBLIC_API_MODE=server` and `NEXT_PUBLIC_API_BASE_URL` until Go backend and persistence phases are implemented.

### Verification

Read-only reviewer subagent requested by owner; findings recorded in the next process entry.

### Next Step

Apply accepted reviewer findings before commit.


## 2026-07-07 — Phase 2 Reviewer Findings Applied

### Action

Applied the read-only reviewer findings for the frontend API client contract.

### Evidence

Reviewer found seven issues: provider/model identity ambiguity, incomplete endpoint mapping, undefined DTO/config types, loose attachment boundaries, missing SSE wire examples, weak runtime rollback semantics, and missing `pluginApi` placeholder.

Updated contract areas:

```text
ModelRef providerId/modelId identity
ApiClientConfig definition
MessageOutputBlockDto and MessageVersionDto definitions
message tree/version compatibility fields
runtime config bootstrap via /api/config or /v1/config
strict AttachmentRef source union and source matrix
canonical SSE event/data frames
settings/provider/plugin endpoint mapping
pluginApi placeholder with plugins capability disabled for MVP
```

### Decision

Treat `local` mode as default and require runtime config for safe rollback where possible. Treat plugin execution as deferred, but keep a minimal `pluginApi` boundary so future plugin work does not leak route calls into components.

### Verification

Local validation passed after edits:

```text
ok: Phase 2 contract fixes verified
git diff --check: clean
```

Validated Markdown links, code fence balance, required contract sections, absence of stale `model: string` / `/v1/auth/verify` residues, and Phase 2 progress checkboxes.

### Next Step

Commit and push Phase 2 contract docs.


## 2026-07-07 — Phase 2 Frontend Call-Site Inventory

### Action

Completed the Phase 2 inventory of frontend-facing direct API, storage, and OPFS call sites.

### Evidence

New/updated documents:

```text
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/inventory/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Inventory sources:

```text
rg "fetch(" src --glob '!src/__tests__/**'
rg "localStorage|localforage|indexedDB|getAppDbStorage|getBrowserLocalStorage|saveToOPFS|resolveOPFSUrl|deleteFromOPFS|writeToOPFS|listOPFSDirectory|opfs://" src --glob '!src/__tests__/**'
rg service imports across src/components src/features src/lib src/store
```

Key findings:

```text
Direct component route calls exist in AccessPasswordPage, ChatApp, ProviderSettings, and DeploymentHealth.
Service-layer fetches are concentrated in src/services/api/* and can become local adapters.
OPFS display and upload paths are spread across chat, media, markdown, workspace, and knowledge UI.
Zustand stores remain the local adapter source of truth for chat/settings/knowledge/memory until server mode is implemented.
```

### Decision

Treat `chatService` wrapping, runtime config/model fetches, and OPFS file adapter extraction as the first code-migration targets. Keep plugin/RAG/doc-parse/voice/code-execution behind disabled or deferred capabilities.

### Verification

Local validation passed:

```text
ok: frontend call-site inventory verified
git diff --check: clean
```

Validated Markdown links, code fence balance, required inventory sections, and Phase 2 progress checkboxes.

### Next Step

Commit and push the Phase 2 call-site inventory, then proceed to Phase 3 Go backend skeleton planning.

## 2026-07-07 — Phase 3 Go Backend Skeleton

### Action

Created the first Go backend skeleton under the isolated `mm-chat/backend/` workspace and added the Phase 3 single-server deployment draft.

### Evidence

Backend files created:

```text
mm-chat/backend/go.mod
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/internal/health/handler.go
mm-chat/backend/internal/health/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/middleware.go
mm-chat/backend/internal/httpserver/server_test.go
```

Deployment docs updated:

```text
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/single-server-compose.md
```

Implemented runtime surface:

```text
MM_CHAT_ADDR default: :8080
MM_CHAT_VERSION default: dev
GET /health      -> {"status":"healthy"}
GET /ready       -> {"status":"ready"}
GET /v1/version  -> {"version":"..."}
```

### Decision

Keep Phase 3 dependency-free by using the Go standard library only. The first backend pass proves process startup, env config, routing, health/readiness/version endpoints, JSON error envelopes, security headers, and panic recovery before adding Postgres, Redis, MinIO, or provider streaming.

The single-server deployment document remains a runbook and topology contract only; no Compose implementation file is created in Phase 3.

### Verification

Validated with Docker Go 1.22 because host `go` is not installed:

```bash
docker run --rm -v "$PWD/mm-chat/backend":/app -w /app golang:1.22-alpine \
  sh -lc '/usr/local/go/bin/gofmt -w $(find . -name "*.go" -print) && /usr/local/go/bin/go test ./...'
```

Result:

```text
?   	neo-chat/mm-chat/backend/cmd/api	[no test files]
ok  	neo-chat/mm-chat/backend/internal/config
ok  	neo-chat/mm-chat/backend/internal/health
ok  	neo-chat/mm-chat/backend/internal/httpserver
```

Docker runtime smoke also passed:

```text
/health      {"status":"healthy"}
/ready       {"status":"ready"}
/v1/version  {"version":"smoke-test"}
X-Content-Type-Options: nosniff
```

`git diff --check -- mm-chat/backend mm-chat/docs/deployment` passed.

### Next Step

Run a read-only reviewer pass across backend, deployment docs, and tracking docs. Then commit and push the Phase 3 skeleton if no blocking findings remain.

## 2026-07-07 — Phase 4 Postgres Migration and Container Plan

### Action

Created the Phase 4 Postgres persistence skeleton: reversible SQL migrations, schema documentation, and single-server Postgres deployment plan.

### Evidence

Migration files created:

```text
mm-chat/backend/migrations/README.md
mm-chat/backend/migrations/001_initial_schema.up.sql
mm-chat/backend/migrations/001_initial_schema.down.sql
```

Documentation created or updated:

```text
mm-chat/docs/persistence/README.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/deployment/postgres-single-server.md
mm-chat/docs/deployment/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

The initial schema creates these tables:

```text
users
sessions
provider_configs
conversations
messages
files
message_attachments
audit_logs
```

### Decision

Use plain reversible SQL for the initial Postgres skeleton and avoid a migration runner dependency until the backend DB wiring phase. UUID primary keys are generated by the Go application. The migration avoids `CREATE EXTENSION`, database-side UUID generators, enum types, triggers, and custom functions.

Postgres owns structured records only. File bytes remain outside Postgres and will move to MinIO/S3 in a later phase. Redis remains future non-authoritative temporary state.

### Verification

Validated against Docker Postgres 16:

```bash
docker run --rm -d --name mm-chat-pg-phase4-<pid> \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=mm_chat \
  postgres:16-alpine

cat mm-chat/backend/migrations/001_initial_schema.up.sql | \
  docker exec -i mm-chat-pg-phase4-<pid> \
    psql -U postgres -d mm_chat -v ON_ERROR_STOP=1

cat mm-chat/backend/migrations/001_initial_schema.down.sql | \
  docker exec -i mm-chat-pg-phase4-<pid> \
    psql -U postgres -d mm_chat -v ON_ERROR_STOP=1
```

Observed result:

```text
up tables: audit_logs, conversations, files, message_attachments, messages, provider_configs, sessions, users
constraint checks: invalid message role rejected; negative file byte_size rejected
down tables_after_down=0
```

Additional checks passed:

```bash
docker run --rm -v "$PWD/mm-chat/backend":/app -w /app golang:1.22-alpine \
  sh -lc '/usr/local/go/bin/gofmt -w $(find . -name "*.go" -print) && /usr/local/go/bin/go test ./...'

git diff --check -- mm-chat/backend/migrations mm-chat/docs/persistence mm-chat/docs/deployment

grep -R "gen_random_uuid\|uuid_generate\|CREATE EXTENSION" -n \
  mm-chat/backend/migrations mm-chat/docs/persistence mm-chat/docs/deployment
```

The grep produced no matches. Deployment docs were also checked to avoid
unconditional references to a migration version table before a runner exists.

### Boundary

This completes the Phase 4 schema, migration, and Postgres container-plan checklist. It does not yet implement a Go database connector, migration runner, repositories, DB-aware readiness, or runtime CRUD endpoints.

### Next Step

Run the required reviewer agent across Phase 4 migrations, docs, and tracking updates. If clean, commit and push. Next implementation phase should add the Go database connector and migration runner before chat repositories.

## 2026-07-07 — Phase 4.5 Postgres Runtime Wiring

### Action

Connected the Go backend skeleton to Postgres runtime wiring without adding chat repositories or CRUD endpoints.

### Evidence

Backend files created or updated:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/cmd/migrate/main.go
mm-chat/backend/go.mod
mm-chat/backend/go.sum
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/internal/database/database.go
mm-chat/backend/internal/database/database_test.go
mm-chat/backend/internal/health/handler.go
mm-chat/backend/internal/health/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/migration/runner.go
mm-chat/backend/internal/migration/runner_test.go
mm-chat/backend/migrations/001_initial_schema.up.sql
mm-chat/backend/migrations/001_initial_schema.down.sql
mm-chat/backend/migrations/README.md
mm-chat/backend/migrations/embed.go
```

Docs created or updated:

```text
mm-chat/docs/persistence/runtime-wiring.md
mm-chat/docs/persistence/README.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/postgres-single-server.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Runtime behavior now defined:

```text
DATABASE_URL empty    -> DB disabled, /ready returns 200
DATABASE_URL nonempty -> startup opens Postgres with pgx and PingContext
DB later unavailable  -> /ready returns 503 DATABASE_NOT_READY
API startup           -> does not run migrations automatically
Migration CLI         -> go run ./cmd/migrate up | down --all
Runner metadata       -> schema_migrations(version, name, applied_at)
```

### Decision

Use `github.com/jackc/pgx/v5 v5.6.0` through the `database/sql` stdlib adapter. The latest pgx release observed by Worker A required a newer Go toolchain, so this phase pins a Go 1.22-compatible pgx version.

The migration runner owns transaction boundaries and updates `schema_migrations` in the same transaction as each migration. SQL migration files therefore do not contain `BEGIN`, `COMMIT`, or `ROLLBACK`.

### Verification

Unit tests passed with Docker Go 1.22:

```bash
docker run --rm -v "$PWD/mm-chat/backend":/app -w /app golang:1.22-alpine \
  sh -lc '/usr/local/go/bin/gofmt -w $(find . -name "*.go" -print) && /usr/local/go/bin/go test ./...'
```

Result:

```text
?    neo-chat/mm-chat/backend/cmd/api       [no test files]
?    neo-chat/mm-chat/backend/cmd/migrate   [no test files]
ok   neo-chat/mm-chat/backend/internal/config
ok   neo-chat/mm-chat/backend/internal/database
ok   neo-chat/mm-chat/backend/internal/health
ok   neo-chat/mm-chat/backend/internal/httpserver
ok   neo-chat/mm-chat/backend/internal/migration
?    neo-chat/mm-chat/backend/migrations    [no test files]
```

Docker Postgres 16 integration passed:

```text
go run ./cmd/migrate up      -> up 001_initial_schema
public tables after up       -> audit_logs, conversations, files, message_attachments, messages, provider_configs, schema_migrations, sessions, users
schema_migrations            -> 1:initial_schema
API with DATABASE_URL set     -> /health 200, /ready 200, /v1/version integration-test
go run ./cmd/migrate down --all -> down 001_initial_schema
domain tables after down     -> 0
schema_migrations rows       -> 0
```

Additional checks passed:

```bash
git diff --check -- mm-chat
grep -R "BEGIN;\|COMMIT;\|ROLLBACK;" -n mm-chat/backend/migrations/*.sql
```

The grep produced no matches.

### Boundary

This phase adds DB connectivity, readiness, and migration execution only. It still does not implement conversation/message repositories, provider streaming persistence, DB-backed auth flows, file APIs, Redis, MinIO, or RAG.

### Next Step

Run the required reviewer agent across backend code, runtime docs, deployment docs, and tracking. If clean, commit and push. The next implementation phase should begin the chat repository and API spine.
