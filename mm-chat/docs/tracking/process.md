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
Migration CLI         -> go run ./cmd/migrate up | down --all | baseline
Runner metadata       -> schema_migrations(version, name, checksum, applied_at)
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

## 2026-07-07 — Phase 5.1 Chat Repository and CRUD API

### Action

Implemented the first Postgres-backed chat CRUD slice under the isolated
`mm-chat/backend` workspace.

### Evidence

Backend files created or updated:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/chat/errors.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/uuid.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
```

Docs created or updated:

```text
mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/README.md
mm-chat/docs/persistence/README.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Implemented API surface:

```text
POST /v1/chat/conversations
GET  /v1/chat/conversations
POST /v1/chat/conversations/{id}/messages
GET  /v1/chat/conversations/{id}/messages
```

Implemented runtime behavior:

```text
DATABASE_URL empty -> chat endpoints return 503 DATABASE_REQUIRED
fixed dev user     -> 00000000-0000-0000-0000-000000000001
conversation DTO   -> modelRef + config
message creation   -> role=user, status=completed, completedAt set
not found          -> 404 CONVERSATION_NOT_FOUND
forbidden message  -> 400 FORBIDDEN_MESSAGE_FIELD
idempotency reuse  -> 409 IDEMPOTENCY_CONFLICT
```

### Decision

Keep Phase 5.1 deliberately narrow: conversation/message CRUD only. Cursor
pagination is not implemented yet; list endpoints retain the `ApiPage` envelope
and return the full active set for the fixed development user. Idempotency keys
are stored as retry guards and mapped to `409` on duplicate key conflicts, but
response replay and payload-hash comparison are deferred.

### Verification

Unit tests passed with Docker Go 1.22 because host `go` is not installed:

```bash
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/mm-chat/backend":/app -w /app \
  -e GOCACHE=/tmp/go-cache -e GOMODCACHE=/tmp/go-mod-cache \
  golang:1.22-alpine \
  sh -lc '/usr/local/go/bin/gofmt -w $(find . -name "*.go" -print) && /usr/local/go/bin/go test ./...'
```

Result:

```text
ok neo-chat/mm-chat/backend/internal/chat
ok neo-chat/mm-chat/backend/internal/config
ok neo-chat/mm-chat/backend/internal/database
ok neo-chat/mm-chat/backend/internal/health
ok neo-chat/mm-chat/backend/internal/httpserver
ok neo-chat/mm-chat/backend/internal/migration
```

DB-disabled API smoke passed:

```text
/ready                                           -> 200 ready
GET  /v1/chat/conversations                     -> 503 DATABASE_REQUIRED
POST /v1/chat/conversations with malformed JSON -> 503 DATABASE_REQUIRED
POST /v1/chat/conversations/{id}/messages       -> 503 DATABASE_REQUIRED
```

Docker Postgres 16 integration passed after `go run ./cmd/migrate up`:

```text
POST /v1/chat/conversations                  -> 201 conversation
POST duplicate conversation idempotencyKey    -> 409 IDEMPOTENCY_CONFLICT
POST forbidden conversation userId            -> 400 VALIDATION_ERROR
GET  /v1/chat/conversations                   -> listed created conversation
POST /v1/chat/conversations/{id}/messages     -> 201 user/completed message
POST duplicate message idempotencyKey          -> 409 IDEMPOTENCY_CONFLICT
POST role=assistant                           -> 400 FORBIDDEN_MESSAGE_FIELD
POST status=streaming                         -> 400 FORBIDDEN_MESSAGE_FIELD
GET  unknown conversation messages            -> 404 CONVERSATION_NOT_FOUND
GET  /v1/chat/conversations/{id}/messages     -> listed one message
Postgres table counts                         -> users=1, conversations=1, messages=1, other app tables=0
```

### Boundary

This phase does not add provider interfaces, mock providers, real provider
adapters, SSE streaming, stream cancellation, assistant streaming persistence,
auth/sessions, Redis, MinIO/S3 file storage, RAG, browser import, or frontend
integration.

### Reviewer Notes

A read-only reviewer found initial contract drift around DTO shape, pagination,
forbidden fields, DB-disabled precedence, and idempotency conflict mapping. The
accepted fixes were applied by making `modelRef/config` the Phase 5.1 canonical
DTO, documenting pagination as not implemented, rejecting server-managed fields,
checking DB-required before POST body parsing, and scoping duplicate-key mapping
to the idempotency unique indexes. Final review also found that message append
did not reject `ownerId`/identity-hint fields; the handler now rejects
`ownerId`, session, token, authorization, and impersonation body fields for both
conversation and message creation, with regression tests.

### Next Step

Run final reviewer and diff checks, then commit and push Phase 5.1. The next
implementation phase should add the provider interface, mock provider, SSE
streaming endpoint, cancellation path, and assistant-message persistence.

## 2026-07-07 — Phase 5.2 Mock Provider and SSE Streaming Spine

### Action

Added the first provider-neutral streaming spine for `mm-chat/backend`: a
provider interface, deterministic mock provider for tests, SSE stream route, and
two-step assistant message persistence.

### Evidence

Backend files created or updated:

```text
mm-chat/backend/internal/chat/provider.go
mm-chat/backend/internal/chat/active_runs.go
mm-chat/backend/internal/chat/errors.go
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/migrations/002_messages_run_id_index.up.sql
mm-chat/backend/migrations/002_messages_run_id_index.down.sql
mm-chat/backend/internal/httpserver/server.go
```

Docs created or updated:

```text
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/contracts/README.md
mm-chat/docs/persistence/README.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Implemented stream surface:

```text
POST /v1/chat/conversations/{id}/stream
```

Request contract:

```text
userMessageId required
modelRef required
idempotencyKey required
content/attachments/user identity/server-managed message fields rejected
```

Persistence behavior:

```text
existing user message -> create assistant role=assistant/status=streaming
provider deltas       -> SSE message.delta frames
provider usage        -> SSE usage.updated frame when present
success               -> finalize assistant status=completed and emit message.completed
provider error        -> finalize assistant status=failed and emit message.error
request cancellation  -> finalize assistant status=cancelled and emit message.cancelled
```

### Decision

Do not append user messages inside `/stream`. The caller must first persist the
user message with `POST /v1/chat/conversations/{id}/messages`, then pass the
returned `userMessageId` into `/stream`. This keeps user-message idempotency and
assistant-run idempotency separate and avoids sequence-number ambiguity.

Do not enable a provider by default in `cmd/api`. If no provider is injected,
`/stream` returns `503 PROVIDER_REQUIRED`. The mock provider is available for
unit tests and future explicit local smoke configuration; real provider adapters
remain later work.

### Verification

Unit tests passed with Docker Go 1.22:

```bash
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/mm-chat/backend":/app -w /app \
  -e GOCACHE=/tmp/go-cache -e GOMODCACHE=/tmp/go-mod-cache \
  golang:1.22-alpine \
  sh -lc '/usr/local/go/bin/gofmt -w $(find . -name "*.go" -print) && /usr/local/go/bin/go test ./...'
```

Covered behavior:

```text
mock provider emits message.started -> message.delta -> usage.updated -> message.completed
assistant message is persisted with parent user message, modelRef, completed status, final content
DB-disabled /stream returns 503 DATABASE_REQUIRED before JSON parsing
provider-missing /stream returns 503 PROVIDER_REQUIRED
unsupported stream body fields are rejected before streaming starts
duplicate assistant stream idempotency key returns 409 IDEMPOTENCY_CONFLICT
temporary Docker Postgres smoke verified streaming assistant insert, duplicate idempotency conflict, finalize completed, and message ordering
```

Reviewer fixes applied after the first Phase 5.2 review:

```text
SSE write failures now finalize the assistant row as cancelled instead of leaving status=streaming.
Completed assistant messages may have empty content, matching the zero-delta SSE contract.
chat-stream-api.md now documents pre-SSE 502 PROVIDER_ERROR.
```

### Boundary

This phase does not add OpenAI/Gemini/OpenAI-compatible adapters, provider
secret management, explicit run cancellation endpoint, Redis cancellation state,
stream resume, durable run records, file attachments, tools/plugins, RAG, auth,
or frontend integration.

### Next Step

Run final reviewer and integration checks, then commit and push Phase 5.2. The
next implementation phase should add a first real provider adapter or the
explicit cancellation endpoint, depending on whether provider execution or run
control is more urgent.

## 2026-07-07 — Phase 5.3 OpenAI-Compatible Provider Adapter

### Action

Verified the owner-provided relay settings from local `mm-chat/backend/.env`
without printing secrets, normalized the file from CRLF to LF, and added the
first real provider adapter for OpenAI-compatible streaming Chat Completions.

### Evidence

Provider probe:

```text
PROVIDER_BASE_URL=[configured OpenAI-compatible relay /v1 URL]
PROVIDER_MODEL=gpt-5.5
HTTP 200
SSE sample returned delta content "pong" and usage.
```

Backend files created or updated:

```text
mm-chat/backend/.env.example
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/chat/provider_openai_compatible.go
mm-chat/backend/internal/chat/provider_openai_compatible_test.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
```

Docs updated:

```text
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/deployment/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Use Go standard-library `net/http` instead of a provider SDK for the first
adapter. This keeps the relay boundary explicit, avoids SDK version churn, and
matches OpenAI-compatible providers that expose `/v1/chat/completions`.

Provider secrets stay in process environment variables only. `cmd/api` enables
the provider only when `PROVIDER_TYPE=openai_compatible` and
`PROVIDER_BASE_URL`, `PROVIDER_MODEL`, and `PROVIDER_API_KEY` are all present.
Missing fields keep streaming disabled with `503 PROVIDER_REQUIRED`; unsupported
provider types fail startup.

### Verification

Unit tests passed with Docker Go 1.22:

```bash
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/mm-chat/backend":/app -w /app \
  -e GOCACHE=/tmp/go-cache -e GOMODCACHE=/tmp/go-mod-cache \
  golang:1.22-alpine \
  sh -lc '/usr/local/go/bin/gofmt -w $(find . -name "*.go" -print) && /usr/local/go/bin/go test ./...'
```

Live smoke passed against Docker Postgres + API + the configured relay before
and after reviewer fixes:

```text
ready_status=200
stream_http_status=200
events: message.started -> message.delta -> usage.updated -> message.completed
assistant persisted status=completed content="pong"
```

Covered behavior:

```text
OpenAI-compatible request path/header/body shape
delta extraction from choices[].delta.content
usage extraction from provider usage chunk
data: [DONE] stream termination
default model fallback
non-2xx provider startup errors without API key leakage
malformed stream frames become provider error events
EOF without data: [DONE] becomes provider error event
unsupported modelRef.providerId is rejected before persistence
provider env config trimming/defaults
handler regression: unsupported providerId does not create assistant row
handler regression: provider startup cancellation finalizes cancelled
```

Reviewer fixes applied:

```text
EOF without data: [DONE] now emits provider error instead of silent completion.
200 OK non-SSE bodies now emit provider error instead of empty completion.
Unsupported modelRef.providerId is rejected before assistant persistence.
Provider startup cancellation finalizes assistant status=cancelled instead of failed.
Deployment docs now state .env is not auto-loaded by go run.
Committed docs/templates no longer include the owner relay hostname.
Handler-level tests now lock unsupported-provider and startup-cancel behavior.
Final reviewer reported no blocking findings after the fixes.
```

### Boundary

This phase does not add Redis cancellation flags, explicit cancel endpoint,
Gemini/native OpenAI Responses API adapters, provider secret encryption at rest,
frontend integration, file attachments, tools/plugins, RAG, or auth.

### Next Step

Commit and push Phase 5.3. Then implement the explicit cancellation endpoint
before expanding provider features.

## 2026-07-07 — Phase 5.4 Durable Run Cancellation Endpoint

### Action

Added the first backend cancellation endpoint for streaming assistant runs:

```text
POST /v1/chat/runs/{runId}/cancel
```

The endpoint validates `runId`, finds the assistant message by
`messages.metadata.runId`, and marks a `streaming` assistant row as
`cancelled`. Already cancelled runs return success idempotently; completed or
failed runs return `409 RUN_NOT_CANCELLABLE`.

### Evidence

Backend files updated:

```text
mm-chat/backend/internal/chat/active_runs.go
mm-chat/backend/internal/chat/errors.go
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/migrations/002_messages_run_id_index.up.sql
mm-chat/backend/migrations/002_messages_run_id_index.down.sql
```

Docs updated:

```text
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Keep Phase 5.4 cancellation narrow: it updates canonical Postgres state and
interrupts in-flight provider streams inside the same API process via an active
run registry. Redis cancellation flags remain Phase 7 work for cross-process
and restart-safe cancellation. The repository prevents a later stream
finalization from overwriting a row that has already reached `cancelled`.

Cancel error semantics:

```text
400 INVALID_RUN_ID
404 RUN_NOT_FOUND
409 RUN_NOT_CANCELLABLE
503 DATABASE_REQUIRED
```

### Verification

Unit tests passed with Docker Go 1.22:

```bash
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/mm-chat/backend":/app -w /app \
  -e GOCACHE=/tmp/go-cache -e GOMODCACHE=/tmp/go-mod-cache \
  golang:1.22-alpine \
  sh -lc '/usr/local/go/bin/gofmt -w $(find . -name "*.go" -print) && /usr/local/go/bin/go test ./...'
```

Postgres/API cancellation smoke passed:

```text
ready_status=200
cancel_http_status=200
idempotent_http_status=200
terminal_http_status=409
missing_http_status=404
invalid_http_status=400
db_status=cancelled:api
run_id_index_exists=t
```

Covered behavior:

```text
streaming run -> 200 cancelled response and assistant row status=cancelled
cancelled run -> 200 idempotent response
completed run -> 409 RUN_NOT_CANCELLABLE
missing run -> 404 RUN_NOT_FOUND
invalid run id -> 400 INVALID_RUN_ID
wrong method -> 405 METHOD_NOT_ALLOWED
active stream cancel calls provider context cancel and emits message.cancelled
outer httpserver mux routes /v1/chat/runs/{runId}/cancel
002 migration creates idx_messages_assistant_run_id
```

### Boundary

This phase does not add Redis-backed cancellation flags, provider request abort
across processes, frontend wiring, run resume, durable run table, auth, or rate
limiting.

### Next Step

Run final reviewer, commit, and push. Then move to Redis temporary state or
frontend server-mode integration based on owner priority.

## 2026-07-07 — Phase 5.4 Review Fix: Cancellation Lock Order

### Action

Fixed the reviewer-blocking Postgres deadlock risk in run cancellation.
`CancelRun` now finds the run target, locks the parent conversation first, then
updates the assistant message. This matches `FinalizeAssistantMessage` and avoids
the previous message-before-conversation lock order.

Also made already-cancelled runs merge cancel metadata so an API cancel cannot
lose `cancelledBy=api` when the stream finalizer wins the race first.

### Evidence

Updated files:

```text
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/repository_postgres_test.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Verification

Added Postgres integration coverage for:

```text
CancelRun waits on the conversation lock before taking the message lock
already-cancelled CancelRun merges cancel metadata idempotently
```

Final Docker Go and Postgres smoke verification passed after the fix.

```text
go test ./...: passed
TestPostgresCancelRunLocksConversationBeforeMessage: passed
TestPostgresCancelRunMergesMetadataForAlreadyCancelledRun: passed
ready_status=200
cancel_http_status=200
idempotent_http_status=200
terminal_http_status=409
missing_http_status=404
invalid_http_status=400
db_status=cancelled:api
idempotent_metadata=cancelled:api
run_id_index_exists=t
```

### Next Step

Rerun unit tests, integration cancellation tests, final reviewer, then commit and
push Phase 5.4.

## 2026-07-07 — Phase 5.4 Final Review and Contract Sync

### Action

Ran final review after the cancellation lock-order fix. No blocking findings
remained. Tightened the frontend API client contract so server-mode streaming
requires a persisted `userMessageId` and does not accept direct `content` /
`attachments` on `/stream`.

### Verification

Final reviewer result:

```text
Blocking findings: none
Ship recommendation: ship
```

Local checks already passed after the lock-order fix:

```text
go test ./...: passed
Postgres CancelRun lock-order integration: passed
API cancellation smoke: passed
```

### Boundary

No `.trellis/spec` file was updated because the owner constraint for this
refactor is to keep implementation artifacts under `mm-chat/`. The executable
API/DB contract is recorded in `mm-chat/docs/contracts/` and
`mm-chat/docs/persistence/`.

### Next Step

Commit and push Phase 5.4, then continue with the next planned refactor slice.

## 2026-07-07 — Phase 6.1 Local Object Storage Boundary

### Action

Added the first file-byte storage boundary under `mm-chat/backend/internal/storage`:

```text
ObjectStore.Put(ctx, key, body, size, contentType)
ObjectStore.Get(ctx, key) -> reader + ObjectInfo
ObjectStore.Delete(ctx, key)
```

Implemented a local filesystem backend for the single-server MVP. It rejects
unsafe object keys, writes via temp file + rename, stores lightweight local
content-type metadata, and cleans up failed writes.

### Files

```text
mm-chat/backend/internal/storage/store.go
mm-chat/backend/internal/storage/local.go
mm-chat/backend/internal/storage/local_test.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/.env.example
mm-chat/docs/storage/README.md
mm-chat/docs/storage/object-storage.md
mm-chat/docs/contracts/file-api.md
mm-chat/docs/contracts/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Keep Phase 6.1 storage-only. The object store does not own auth, file metadata,
SHA-256, upload limits, or message attachments. Phase 6.2 will add the file
service/repository and HTTP endpoints. MinIO/S3 will later implement the same
interface without exposing object keys to the browser.

### Verification

Docker Go 1.22 verification passed:

```text
go test ./...: passed
internal/storage tests: passed
```

### Next Step

Run tests and reviewer, then commit/push. Next implementation slice is Phase
6.2: file metadata repository plus upload/download/delete HTTP endpoints.

## 2026-07-07 — Phase 6.1 Final Review Fixes

### Action

Ran final review for the local object-storage boundary. No blocking findings
remained. Applied low-cost hardening from review: reject drive-style colon keys
such as `C:/...`, document that rule, and close the test reader before delete
for cross-platform hygiene.

### Verification

```text
review blocking findings: none
go test ./...: passed after review fixes
```

### Boundary

Still storage-only. No file HTTP endpoint, file metadata repository, MinIO/S3
adapter, auth, or attachment wiring was added in this slice.

### Next Step

Commit and push Phase 6.1, then implement Phase 6.2 file metadata repository
and upload/download/delete endpoints.

## 2026-07-07 — Phase 6.2 File Metadata API and Local Storage Wiring

### Action

Added the first server file API implementation above the Phase 6.1 object-store
boundary:

```text
POST   /v1/files
GET    /v1/files/{fileId}
GET    /v1/files/{fileId}/content
DELETE /v1/files/{fileId}
```

The upload path streams bytes into `ObjectStore`, computes SHA-256, stores
metadata in Postgres `files`, and deletes the object if metadata insertion
fails. Metadata and content reads resolve the private object key from Postgres;
responses do not expose local paths, object keys, buckets, or MinIO URLs.

### Files

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/files/*
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/docs/contracts/file-api.md
mm-chat/docs/storage/object-storage.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Keep this slice local-object-store first. MinIO/S3 remains a later adapter.
Ownership checks are fixed-development-user scoped until auth lands. Message
attachment linking remains separate from raw file upload/download.

### Verification

Docker Go 1.22 unit verification passed:

```text
go test ./...: passed
internal/files handler tests: passed
httpserver /v1/files route test: passed
```

Postgres integration and API smoke verification passed:

```text
TestPostgresRepositoryCreatesGetsAndDeletesFileMetadata: passed
ready_status=200
upload_status=201
metadata_status=200
content_status=200
delete_status=204
after_delete_status=404
invalid_status=400
db_row=deleted:chat
```

### Next Step

Commit and push Phase 6.2, then continue with message attachment linking or MinIO/S3 adapter based on owner priority.

## 2026-07-07 — Phase 6.2 Final Review Fixes

### Action

Ran final review for file metadata/API wiring. No blocking findings remained.
Added an explicit service regression test for the rollback path: when metadata
insert fails after object write, the service deletes the just-written object.

### Verification

```text
review blocking findings: none
go test ./...: passed after rollback test
```

### Boundary

Object deletion after metadata soft-delete is still best-effort in this local
MVP. A future object cleanup/retry job should handle orphan cleanup when moving
to MinIO/S3 or multi-worker deployment.

## 2026-07-07 — Phase 6.3 Message Attachment Links

### Action

Added the first file-to-chat link path without changing the existing frontend
or original app source. `POST /v1/chat/conversations/{id}/messages` now accepts
server file references in `attachments`, validates UUIDs, source, purpose, and
duplicates, then writes `message_attachments` in the same Postgres transaction
as the user message. Message create/get/list responses include browser-safe
attachment metadata.

### Files

```text
mm-chat/backend/internal/chat/*
mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/file-api.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Attachment linking is metadata-only in this slice. The stream endpoint still
rejects `attachments` in its request body, and provider adapters do not yet
consume file bytes as multimodal input. Message DTOs expose `fileId`,
filename, MIME type, size, SHA-256, and purpose only; object keys, local paths,
buckets, and direct object-store URLs remain private.

### Verification

```text
go test ./...: passed with Docker Go 1.22
handler attachment create/list tests: passed
Postgres attachment integration tests against Docker Postgres: passed
API smoke with Docker Postgres + Go API: upload -> attach -> list passed
unsupported opfs attachment source smoke: 400 UNSUPPORTED_ATTACHMENT_SOURCE
```

### Next Step

Run review, commit, and push Phase 6.3.

## 2026-07-07 — Phase 6.3 Review Fixes

### Action

Reviewed the message attachment linking path across chat handler, service,
Postgres repository, contracts, and tracking docs. Tightened attachment read
queries to require both `message_attachments.user_id` and `files.user_id` to
match the fixed development user, then added regression coverage for missing
attachment mapping, attachment count limits, and transaction rollback when a
later attachment link fails.

### Files

```text
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/repository_postgres_test.go
mm-chat/docs/tracking/process.md
```

### Verification

```text
gofmt -l $(find . -name "*.go" -print): passed with Docker Go 1.22
go test ./...: passed with Docker Go 1.22
go vet ./...: passed with Docker Go 1.22
Postgres attachment integration tests: passed against Docker Postgres 16
API smoke after review fixes: upload -> attach -> list passed
git diff --check -- mm-chat: passed
```

### Next Step

Commit Phase 6.3 after final main-session review.

## 2026-07-07 — Phase 6.4 MinIO/S3 Object Store Adapter

### Action

Added a MinIO/S3-compatible implementation behind the existing `ObjectStore`
interface while keeping the file HTTP contract unchanged. The Go API now
supports `STORAGE_BACKEND=local`, `STORAGE_BACKEND=minio`, and
`STORAGE_BACKEND=s3`. The S3 adapter validates the same server-generated object
keys as the local store, maps missing objects to `storage.ErrObjectNotFound`,
and optionally creates the bucket only when `S3_BUCKET_AUTO_CREATE=true`.

### Files

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/cmd/api/main_test.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/internal/storage/s3.go
mm-chat/backend/internal/storage/s3_test.go
mm-chat/backend/go.mod
mm-chat/backend/go.sum
mm-chat/backend/.env.example
mm-chat/docs/storage/object-storage.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Use `github.com/minio/minio-go/v7` as the S3-compatible SDK, pinned to a Go
1.22-compatible version instead of latest because recent latest releases require
a newer Go toolchain. Use `S3_*` env names consistently:
`S3_ENDPOINT`, `S3_BUCKET`, `S3_REGION`, `S3_ACCESS_KEY_ID`,
`S3_SECRET_ACCESS_KEY`, `S3_USE_SSL`, `S3_FORCE_PATH_STYLE`, and
`S3_BUCKET_AUTO_CREATE`.

### Verification

```text
go test ./...: passed with Docker Go 1.22
MinIO storage integration test: passed against private Docker MinIO
API smoke with Docker Postgres + Docker MinIO: upload/download/delete passed
DB file metadata storage_backend=minio: verified
```

### Next Step

Run review, then commit and push Phase 6.4.

## 2026-07-07 — Phase 6.4 Review Fix

### Action

Reviewed Phase 6.4 docs and fixed the stale file API contract wording that
still described MinIO/S3 as a later adapter. The contract now states that
`STORAGE_BACKEND=minio|s3` uses the same `ObjectStore` and keeps HTTP response
shapes unchanged.

### Verification

```text
go test ./...: passed with Docker Go 1.22
go vet ./...: passed with Docker Go 1.22
git diff --check -- mm-chat: passed
```

### Next Step

Commit and push Phase 6.4.

## 2026-07-08 — Phase 7 Redis Temporary Cancellation Flags

### Action

Added Redis as a non-authoritative temporary-state dependency for stream
cancellation coordination only. The Go API now reads `REDIS_URL`,
`REDIS_KEY_PREFIX`, and `REDIS_RUN_CANCEL_TTL`; an empty `REDIS_URL` disables
Redis, while a configured but unreachable Redis fails startup. Cancel requests
still update Postgres first, then set a short-lived Redis flag so other API
processes can interrupt active provider streams.

### Files

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/config/*
mm-chat/backend/internal/redisstate/*
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/run_cancellation.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/.env.example
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/deployment/redis-temporary-state.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Redis must never become canonical storage. Postgres remains the source of truth
for conversations, messages, files, and run status. Redis flags are best-effort
coordination for active streams; runtime Redis errors degrade cross-process
interruption but must not corrupt durable state or expose credentials.

### Verification

```text
go test ./... with Docker Go 1.22: passed
config/default/override/blank/invalid Redis tests: passed
redisstate unit + Docker Redis integration: passed
handler cancellation-store stream test: passed
startup helper invalid REDIS_URL secret-leak test: passed
Postgres + Redis API smoke after Redis FLUSHDB: conversation/message read passed
```

### Next Step

Run final review agent, then commit and push Phase 7. Rate-limit middleware and
session cache remain unchecked Phase 7 follow-up items.

## 2026-07-08 — Phase 7 Review Fix: Durable-First Cancellation

### Action

Review found the cancel handler still cancelled same-process active streams before
`CancelRun` durably updated Postgres. Removed the pre-DB active cancellation so
all temporary interruption paths happen only after the durable cancel succeeds,
matching Redis non-authoritative semantics.

### Files

```text
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/docs/deployment/redis-temporary-state.md
```

### Verification

```text
docker run --rm -v "$PWD/mm-chat/backend":/src -w /src golang:1.22 go test ./internal/chat -run 'TestHandlerCancelRun' -count=1: passed
docker run --rm -v "$PWD/mm-chat/backend":/src -w /src golang:1.22 go test -race ./internal/chat -run 'TestHandler(CancelRun|StopsActiveStream)' -count=1: passed
docker run --rm -v "$PWD/mm-chat/backend":/src -w /src golang:1.22 go test ./...: passed
docker run --rm -v "$PWD/mm-chat/backend":/src -w /src golang:1.22 /bin/sh -c 'test -z "$(gofmt -l .)" && go vet ./...': passed
git diff --check -- mm-chat: passed
main-session Docker Go 1.22 go test ./... && go vet ./... after review fix: passed
main-session Docker Redis integration after review fix: passed
```

### Next Step

Commit Phase 7 after main-session final review.

## 2026-07-08 — Phase 7 Redis Rate Limit Middleware

### Action

Added opt-in Redis-backed fixed-window HTTP rate limiting. The backend now reads
`REDIS_RATE_LIMIT_ENABLED`, `REDIS_RATE_LIMIT_REQUESTS`, and
`REDIS_RATE_LIMIT_WINDOW`. When enabled and Redis is configured, non-health HTTP
routes are limited by hashed `RemoteAddr` client identity. Health, readiness, and
version endpoints remain exempt.

### Files

```text
mm-chat/backend/internal/ratelimit/store.go
mm-chat/backend/internal/redisstate/rate_limit.go
mm-chat/backend/internal/redisstate/rate_limit_test.go
mm-chat/backend/internal/httpserver/rate_limit.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/cmd/api/main.go
mm-chat/backend/cmd/api/main_test.go
mm-chat/backend/.env.example
mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/contracts/file-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/deployment/redis-temporary-state.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Rate limiting is non-authoritative temporary state. Startup still fails fast when
`REDIS_URL` is configured but unreachable, but runtime Redis counter errors fail
open so Redis outages do not block canonical Postgres-backed API reads/writes.
`X-Forwarded-For` is not trusted yet; reverse-proxy-aware identity requires a
future explicit trusted-proxy config. Enabling rate limits without `REDIS_URL`
fails startup so deployments do not accidentally believe rate limiting is active
when no Redis store exists. Redis counter increments use Lua to bind `INCR` and
TTL assignment atomically for new window keys.

### Verification

```text
Docker Go 1.22 go test ./...: passed
httpserver rate-limit middleware tests: passed
Docker Redis integration for cancellation + rate-limit stores: passed
API smoke with Redis rate limit enabled: 404, 404, then 429 RATE_LIMITED; /health exempt
Fail-fast smoke with REDIS_RATE_LIMIT_ENABLED=true and no REDIS_URL: passed
```

### Next Step

Run review agent, then commit and push this Phase 7 slice. Session cache
integration remains unchecked.

## 2026-07-08 — Phase 7 Review Fix: Rate Limit Contract Coverage

### Action

Review found two consistency gaps: the stream contract still listed Redis
rate-limit state as a non-goal, and tests did not cover every exempt health
route or the full `429` header contract. Updated the stream contract, expanded
HTTP middleware tests, and added Redis integration assertions that rate-limit
counter TTL is positive and is not extended by later hits in the same window.

### Files

```text
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/redisstate/rate_limit_test.go
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/tracking/process.md
```

### Verification

```text
Docker Go 1.22 gofmt check: passed
Docker Go 1.22 go vet ./...: passed
Docker Go 1.22 go test ./...: passed
Docker Redis integration for cancellation + rate-limit stores: passed
git diff --check -- mm-chat: passed
main-session Docker Go 1.22 go test ./... && go vet ./... after review fix: passed
main-session Redis integration/API rate-limit smoke/fail-fast after review fix: passed
```

### Next Step

Commit Phase 7 after main-session review approval. Session cache integration
remains unchecked.

## 2026-07-08 — Phase 7 Redis Session Cache Integration

### Action

Added the Redis-backed session-cache substrate without changing the current fixed-development-user HTTP behavior. The new auth resolver checks Redis first, falls back to Postgres on cache miss or Redis errors, refuses expired/revoked sessions, and caches only browser-safe session snapshots. The Redis store hashes token-hash cache keys again, stores short-lived revocation hints, and never stores raw bearer tokens, token hashes, provider secrets, IP addresses, or user agents.

### Files

```text
mm-chat/backend/.env.example
mm-chat/backend/cmd/api/main.go
mm-chat/backend/cmd/api/main_test.go
mm-chat/backend/internal/auth/session_repository_postgres.go
mm-chat/backend/internal/auth/session_repository_postgres_test.go
mm-chat/backend/internal/auth/session_resolver.go
mm-chat/backend/internal/auth/session_resolver_test.go
mm-chat/backend/internal/auth/types.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/internal/redisstate/session_cache.go
mm-chat/backend/internal/redisstate/session_cache_test.go
mm-chat/backend/internal/sessioncache/store.go
mm-chat/docs/architecture/server-refactor-design.md
mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/chat-stream-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/redis-temporary-state.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Session cache is reusable infrastructure for the later auth phase, not runtime auth enforcement yet. Postgres remains the canonical session and revocation source. Redis flushes become cache misses; Redis runtime errors fall back to Postgres; Postgres errors fail closed. Cache TTL is bounded by both `REDIS_SESSION_CACHE_TTL` and `sessions.expires_at`.

### Verification

```text
Docker Go 1.22 gofmt + go test ./... + go vet ./...: passed
Docker Redis integration for session cache store: passed
Docker Postgres integration for auth session repository: passed
Docker Redis+Postgres integration for resolver fallback after Redis FLUSHDB: passed
git diff --check -- mm-chat: passed
```

### Next Step

Run Redis integration, vet/diff checks, review agent, then commit and push the Phase 7 session-cache slice.

## 2026-07-08 — Phase 7 Review Fix: Session Cache Canonicality

### Action

Review found two P2 issues. Updated the resolver so a Redis revocation hint no longer rejects a session by itself; the resolver deletes the cached token snapshot, rechecks canonical Postgres state, and clears stale revocation hints after a successful active-session lookup. Also added `MM_CHAT_TEST_REDIS_ALLOW_FLUSH=true` as an explicit safety guard before any integration test calls Redis `FLUSHDB`.

### Files

```text
mm-chat/backend/internal/auth/session_resolver.go
mm-chat/backend/internal/auth/session_resolver_test.go
mm-chat/backend/internal/auth/session_repository_postgres_test.go
mm-chat/docs/deployment/redis-temporary-state.md
mm-chat/docs/tracking/process.md
```

### Verification

```text
Docker Go 1.22 gofmt + go test ./... + go vet ./...: passed
Docker Redis+Postgres integration with MM_CHAT_TEST_REDIS_ALLOW_FLUSH=true: passed
git diff --check -- mm-chat: passed
```

### Next Step

Run final Trellis quality check, commit, and push the Phase 7 session-cache slice.

## 2026-07-08 — Phase 8 Browser Data Import Contract

### Action

Started Phase 8 with a documentation-first import contract. Inventoried the current browser export surfaces, including full-app `AppExportPayload`, single-session export payloads, IndexedDB/localforage keys, per-session message storage, and OPFS reference risks. Added a backend import contract for explicit preview-before-commit imports using `neo-chat-browser-import-v2.zip`, a normalized manifest, and SHA-256 addressed file blobs. Added the frontend `importApi` boundary so later UI code has one import surface.

### Files

```text
mm-chat/docs/inventory/browser-data-export.md
mm-chat/docs/inventory/README.md
mm-chat/docs/contracts/browser-data-import.md
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/architecture/server-refactor-design.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

The Go backend should validate a normalized import manifest instead of parsing every historical Zustand/localforage shape. The browser-side exporter remains responsible for reading IndexedDB and OPFS, converting millisecond timestamps to UTC RFC3339, mapping local role `model` to server role `assistant`, and building SHA-256 addressed ZIP blobs for OPFS/inline files. Preview performs ZIP/schema/blob validation without writes; commit repeats the confirmed package and persists rows/objects. Runtime import code remains a later Phase 8 slice.

### Verification

```text
Source inspection: src/lib/data/appExport.ts, src/lib/chat/sessionExport.ts, src/store/storage/storageConfig.ts, src/store/core/chatStore.ts, src/utils/opfs.ts
Docs updated under mm-chat only; upgraded after Scout finding that current all-data JSON omits session_messages_* and OPFS bytes: pending final review
```

### Next Step

Run review agent, fix contract gaps, then commit and push the Phase 8 contract slice.

## 2026-07-08 — Phase 8 Review Fix: Import Package Atomicity

### Action

Addressed review findings in the browser import contract before runtime work.
Removed the remaining old "file part" wording from the Phase 8 flow, hardened
the uploaded ZIP whitelist, and aligned commit/batch statuses with an atomic
all-or-nothing import model.

### Files

```text
mm-chat/docs/architecture/server-refactor-design.md
mm-chat/docs/contracts/browser-data-import.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/browser-data-export.md
mm-chat/docs/tracking/process.md
```

### Decision

The uploaded server import ZIP may contain only `manifest.json` and
`files/sha256/*`. Diagnostic `stores/*` and `messages/*` exports are local-only
debug artifacts and must be rejected if they appear in an uploaded package.
Commit is atomic: validation, database, or object-storage failures abort the
batch and return an error instead of exposing a partial-success state. Review
also found an idempotency wording conflict; the contract now specifies same
package replay returns the prior completed result, while reusing the same
idempotency key with different package bytes returns `409 IDEMPOTENCY_CONFLICT`.

### Verification

```text
Review agent found one P2 idempotency wording conflict after the first fix pass.
Contract wording updated locally.
Review agent rerun: P0/P1/P2 no findings.
git diff --check -- mm-chat: passed.
Trellis spec update: no `.trellis/spec` change; the executable import contract
is task-scoped under `mm-chat/docs/contracts/browser-data-import.md`, preserving
the owner rule that refactor artifacts stay under `mm-chat/`.
```

### Next Step

Commit and push the Phase 8 browser import contract slice, then begin runtime
conversation/message import implementation.

## 2026-07-08 — Phase 8 Runtime: Browser Import Chat Rows

### Action

Implemented the first browser import runtime slice in the Go backend. Added a
dedicated `internal/browserimport` package for ZIP parsing, manifest validation,
HTTP endpoints, Postgres persistence, idempotency replay, and rollback. Wired
the handler into `/v1/import/browser/*` from the shared HTTP server and API
startup path. Added migration `003_import_batches` to track committed import
batches and preserve replay/rollback metadata.

### Files

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/browserimport/*
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/migrations/003_import_batches.up.sql
mm-chat/backend/migrations/003_import_batches.down.sql
mm-chat/backend/migrations/README.md
mm-chat/docs/contracts/browser-data-import.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Import uses a separate repository path instead of `chat.CreateMessage` because
the normal chat CRUD endpoint intentionally accepts only new user messages and
server-owned timestamps. Browser import must preserve historical
`role/status/sequenceNo/createdAt/completedAt/outputBlocks` and original client
ID mappings. This slice commits chat-only packages and rejects packages with
`files[]` or file attachments until the MinIO attachment import slice is built,
so no attachment data is silently dropped.

Rollback is batch-scoped. `DELETE /v1/import/browser/{batchId}` soft-deletes
imported messages and conversations and marks the batch `rolled_back`; if rows
were modified after commit, it returns `409 IMPORT_BATCH_MODIFIED`.

### Verification

```text
Docker Go 1.22 gofmt + go test ./...: passed
Docker Go 1.22 go vet ./...: passed
Disposable Docker Postgres integration for internal/browserimport: passed
git diff --check -- mm-chat: passed
Review agent first pass: P1 idempotency replay, top-level timestamp validation,
secret scanning; P2 ZIP symlink, orphan blob, HTTP/docs sync.
Fixes added: concurrent same-package replay, generatedAt/exportedAt validation,
outputBlocks/attachment secret checks, symlink/orphan blob rejection, route
matrix, rollback modified detection, 003 up/down integration, and GET/preview
contract docs.
Review agent second pass: P1 remote URL userinfo/fragment secret coverage and
P2 imported-message modified rollback coverage remained. Added URL userinfo and
fragment-token rejection plus message-row rollback modified integration test.
Final review agent rerun: P0/P1/P2 no findings.
```

### Next Step

Run review agent for the Phase 8 runtime slice, fix findings, then commit and
push. Next implementation slice: import `files[]` blobs into MinIO/S3 and link
message attachments.

## 2026-07-08 — Phase 8 Runtime: Browser Import File Attachments

### Action

Implemented the attachment slice for browser data import. ZIP blobs are now
retained by `PackageReader`, validated against manifest `files[]`, uploaded to
the configured object store during commit, inserted into `files`, and linked to
imported messages through `message_attachments`.

### Files

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/browserimport/errors.go
mm-chat/backend/internal/browserimport/handler.go
mm-chat/backend/internal/browserimport/handler_test.go
mm-chat/backend/internal/browserimport/package.go
mm-chat/backend/internal/browserimport/package_test.go
mm-chat/backend/internal/browserimport/repository_postgres.go
mm-chat/backend/internal/browserimport/repository_postgres_test.go
mm-chat/backend/internal/browserimport/types.go
mm-chat/docs/contracts/browser-data-import.md
mm-chat/docs/persistence/postgres-schema.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Import keeps preview DB/storage-independent, but commit now requires object
storage only when the package contains files. The repository writes object bytes
with server-generated keys (`users/{userId}/files/{fileId}`), stores file
metadata in the same import transaction, and compensates by deleting staged
objects when any DB write or transaction commit fails. Rollback deletes
`message_attachments`, soft-deletes imported `files/messages/conversations`,
marks the batch `rolled_back`, then hard-deletes object bytes.

Remote and `knowledge_ref` attachments remain metadata-only in message metadata;
the backend does not fetch URLs or expose object keys/bucket URLs.

### Verification

```text
Docker Go 1.22 gofmt ./cmd ./internal ./migrations: passed
Docker Go 1.22 go test ./...: passed
Disposable Docker Postgres integration for internal/browserimport PostgresRepository: passed
```

### Next Step

Run review agent on the attachment slice, fix findings, then commit and push.

## 2026-07-08 — Phase 8 Review Fix: Attachment Import Safety

### Action

Fixed review findings from the file-attachment import slice. Rollback now treats
attachment links as part of the rollback safety boundary, commit-error handling
avoids deleting objects when database commit state is unknown, and preview
validation rejects duplicate file attachments on the same message.

### Decision

Rollback blocks when an imported file or imported message has any external
`message_attachments` reference, preventing deletion of user-created links or
file bytes after import. Commit cleanup still deletes staged objects for known
pre-commit failures, but if `tx.Commit()` returns an error and the backend cannot
verify whether the batch committed, it leaves objects in place instead of
risking a committed DB row with missing bytes. If the committed batch can be
verified by idempotency key and hashes, the stored completed response is
returned.

The import contract now explicitly allows attachment `purpose = "output"` to
match the existing `message_attachments` schema. File `originalUrl` is limited to
`opfs://...` and secret-like file metadata is rejected before persistence.

### Verification

```text
Docker Go 1.22 gofmt ./cmd ./internal ./migrations: passed
Docker Go 1.22 go test ./...: passed
Docker Go 1.22 go vet ./...: passed
Disposable Docker Postgres integration for internal/browserimport PostgresRepository: passed
git diff --check -- mm-chat: passed
Review fixes covered by tests: duplicate file attachment validation, object Put
failure leaves no DB rows, response does not leak object keys, rollback rejects
external attachment refs, modified imported files still block rollback.
Final review agent rerun: P0/P1/P2 no findings.
```

### Next Step

Commit and push the Phase 8 attachment import slice.

## 2026-07-08 — Phase 10 Runtime: Single-Server Compose Deployment

### Action

Implemented the single-server Docker Compose runtime under `mm-chat/`. The stack
now defines Postgres, Redis, private MinIO, MinIO bucket/user initialization, a
Go backend image, an explicit migration service, and an ops-only MinIO client.
Added sanitized stack env, gitignored runtime data/backup paths, backup scripts,
restore drills, release/rollback docs, and updated deployment indexes.

### Files

```text
mm-chat/.env.single-server.example
mm-chat/.gitignore
mm-chat/README.md
mm-chat/backend/.dockerignore
mm-chat/backend/Dockerfile
mm-chat/compose.single-server.yml
mm-chat/docs/README.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/backup-restore.md
mm-chat/docs/deployment/release-rollback.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
mm-chat/scripts/backup-minio.sh
mm-chat/scripts/backup-postgres.sh
```

### Decision

The Compose stack keeps only the backend port bound to `127.0.0.1:8080`;
Postgres, Redis, MinIO API, and MinIO console stay on the private Compose
network. API startup still does not run migrations: operators run the `migrate`
service before starting or restarting the backend. MinIO initializes a private
bucket and least-privilege app user; the backend remains the public file gateway.

Backups are operator-triggered scripts. Postgres uses a custom-format
`pg_dump`, MinIO uses `mc mirror` through the `minio-client` service, and both
write `.sha256` sidecars. Restore documentation requires temporary DB/bucket
drills before any destructive production restore.

### Verification

```text
bash -n backup scripts: passed
docker compose config with app+ops profiles: passed
Docker backend image build: passed
Docker Go 1.22 go test ./...: passed
Disposable Compose smoke with temp bind mounts: passed
  - postgres/redis healthy
  - minio-init created bucket/user/policy
  - migrate applied 001/002/003
  - backend /health, /ready, /v1/version returned 200
Backup script smoke against disposable stack: passed
  - Postgres dump + sha256 created and verified
  - MinIO archive + sha256 created and verified
git diff --check -- mm-chat: passed
```

### Next Step

Run a review agent on the Phase 10 deployment slice, fix findings, then commit
and push only the `mm-chat/` changes.

## 2026-07-08 — Phase 10 Review Fix: Deployment Safety

### Action

Fixed review findings in the single-server deployment slice. MinIO init is now
fail-fast for policy attach and validates the app credentials by writing,
statting, and deleting a temporary object before the backend can start. Backup
checksum docs now match basename-based `.sha256` files, rollback docs use a
real Compose `migrate ... down` command, and deployment docs distinguish
Compose secrets from direct `go run` env files.

### Verification

```text
bash -n backup scripts: passed
docker compose config with app+ops profiles: passed
Disposable Compose smoke with temp bind mounts: passed
  - backend image build passed
  - minio-init bucket/user/policy/app-credential smoke passed
  - migrate up applied 001/002/003
  - backend /health, /ready, /v1/version returned 200
  - backup-postgres and backup-minio created sha256-verified artifacts
  - documented migrate down command rolled back 003, then migrate up re-applied 003
Cleanup removed disposable containers, network, and temp bind data.
```

### Review

Final review agent rerun: P0/P1/P2 no findings. Remaining P3 is commit hygiene:
only targeted `mm-chat/` paths may be staged because the root workspace contains
unrelated dirty files.

### Next Step

Commit and push only the `mm-chat/` slice.

## 2026-07-08 — Roadmap Rule and Phase 11+ Planning

### Action

Recorded the owner decision that new plans and scope changes must be written to
repository docs before implementation starts. Added the post-Phase-10 roadmap so
frontend integration, import UI, auth hardening, production hardening, optional
RAG, and future K8s/multi-server migration do not depend on chat memory.

### Files

```text
mm-chat/docs/architecture/phase-11-plus-roadmap.md
mm-chat/docs/architecture/server-refactor-design.md
mm-chat/docs/architecture/README.md
mm-chat/docs/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Phase 11 becomes the next recommended implementation phase: frontend
server-mode integration. Optional RAG is deferred until core chat, files, import,
and frontend server mode are stable. Future plans must define objective, scope,
outputs, verification, rollback, and tracking checklist before code changes.

### Verification

```text
Docs-only change under mm-chat/.
Roadmap linked from architecture and docs indexes.
Progress checklist now includes Planning Rule and Phase 11-16 items.
Original Phase 9 RAG placeholder is marked deferred behind Phase 11-14, with
Phase 15 as the active RAG gate.
```

### Next Step

Review and commit the roadmap docs, then start Phase 11 only after confirming
the frontend integration slice.

## 2026-07-08 — Phase 11 Kickoff: Documentation-First Slice Plan

### Action

Started Phase 11 with a documentation-only kickoff. Split frontend server-mode
integration into five implementation slices:

```text
11.1 adapter scaffold
11.2 conversation/message CRUD
11.3 SSE stream
11.4 file upload/download
11.5 browser smoke/local rollback
```

No application code was changed in this kickoff.

Scope note: that statement is scoped to the Phase 11 docs slice under
`mm-chat/`. The repository worktree also contains unrelated out-of-scope dirty
files outside `mm-chat/`; they are not part of this slice and must not be staged
or committed with the Phase 11 docs work.

### Evidence

Updated planning/tracking documents only:

```text
mm-chat/docs/architecture/phase-11-plus-roadmap.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

The Phase 11 progress checklist is now split by slice. Implementation checkboxes
remain unchecked until code, tests/smoke evidence, and a matching dated process
entry exist.

### Decision

Follow the roadmap planning rule before implementation starts: each Phase 11
slice must have objective, scope, outputs, verification, rollback, and a
tracking checklist.

The first implementation slice is intentionally narrow. Phase 11.1 may scaffold
the server-mode adapter and mode selection only; it must not touch browser
import/export UI, auth UI or enforcement, RAG/knowledge flows, provider-settings
redesign, or unrelated product UI.

Browser server-mode smoke needs an explicit network-edge decision before code
work: either route the frontend through a same-origin proxy/reverse proxy to Go,
or add and verify backend CORS allowlisting for the chosen frontend origin. The
current Go API does not emit CORS headers, so direct browser fetches from a
Next.js dev origin to `http://127.0.0.1:8080` are treated as a Phase 11.1 gap
until one of those paths is implemented.

### Verification

This was a docs-first kickoff, so application tests were not run. Verification
for this step is limited to the edited docs and diff hygiene. Functional checks
belong to the later implementation slices.

### Review

Multi-agent review found and the lead fixed these documentation risks before
implementation:

- scoped the Phase 11 docs slice away from unrelated dirty files outside
  `mm-chat/`;
- marked `/v1/config`, `/v1/settings`, `/v1/providers*`, `/v1/auth*`, and
  `/v1/plugins*` as unsupported in Phase 11 until Go routes exist;
- hardened CRUD gap wording so server mode uses server-data derivation or
  explicit unsupported responses, never implicit browser-local fallback;
- corrected known stream/cancel error code handling and kept the complete set
  tied to the Go handler contracts.

Final review result: no remaining findings.

### Next Step

Implement Phase 11.1 adapter scaffold next, then update `progress.md` and add a
dated `process.md` entry only after the slice is implemented and verified.

## 2026-07-08 — Phase 11.1 Start: Adapter Scaffold Constraints

### Action

Prepared the Phase 11.1 opening record only. No implementation checkbox is
completed by this entry, and no application code is changed by this record.

### Scope

Phase 11.1 targets only:

```text
adapter scaffold
local|server mode selection
browser network-edge decision
```

Phase 11.1 explicitly does not wire:

```text
conversation/message CRUD
SSE streaming
file upload/download
browser import/export
auth enforcement
RAG/knowledge flows
provider-settings redesign
unrelated product UI
```

### Constraints

Original owner constraint remains active: refactor work stays under
`mm-chat/`, and the original app must not be modified casually. If the Phase
11.1 implementation needs changes under `src/`, that must be recorded before
editing as either:

```text
owner approval required
pending decision: confirm original-app modification boundary
```

Multi-agent execution plus a review agent is a Phase 11.1 execution
requirement. The implementation pass should include an independent review
before any progress checkbox is marked complete.

### Decision

The next implementation pass will first verify whether the adapter scaffold can
live entirely under `mm-chat/`. If it can, proceed with the isolated scaffold
and keep the original app read-only. If it cannot, stop before editing `src/`
and request/confirm the permitted original-app modification boundary.

### Verification

Tracking-only preparation. Verification for this record is limited to checking
that only these files changed:

```text
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

No `pnpm` or backend tests are required for this documentation-only opening
record.

### Next Step

Start Phase 11.1 by inspecting the current frontend boundary read-only, then
prove whether the scaffold can be placed only in `mm-chat/`. If not, record the
needed `src/` boundary decision and ask for owner approval before editing.

## 2026-07-08 — Phase 11.1A: Isolated Adapter Scaffold

### Action

Created the first Phase 11.1 adapter scaffold under `mm-chat/frontend/` only.
The original Next.js app under `src/` remains read-only for this slice.

The scaffold includes:

```text
mm-chat/frontend/README.md
mm-chat/frontend/DESIGN.md
mm-chat/frontend/src/api-client/types.ts
mm-chat/frontend/src/api-client/mode.ts
mm-chat/frontend/src/api-client/errors.ts
mm-chat/frontend/src/api-client/index.ts
mm-chat/frontend/src/api-client/local/chat-api.ts
mm-chat/frontend/src/api-client/server/http-client.ts
mm-chat/frontend/src/api-client/server/chat-api.ts
mm-chat/frontend/src/api-client/server/sse.ts
mm-chat/frontend/__tests__/api-client.test.ts
```

### Decision

Use an isolated `mm-chat/frontend/` scaffold as the safe pre-integration path.
This satisfies the owner constraint that refactor work stays under `mm-chat/`
until original-app modification is explicitly approved.

The full app-boundary Phase 11.1 work is still pending because wiring the
scaffold into `src/services/api/*` would modify the existing Next.js app.
That next step requires an explicit owner decision before editing `src/`.

Read-only frontend boundary evidence from this pass:

```text
src/services/api/chatService.ts
src/config/api.ts
src/components/app/ChatApp.tsx
src/features/chat/hooks/useChatGenerationController.ts
src/store/core/chatStore.ts
src/__tests__/chatServiceToolConfirmation.test.ts
src/__tests__/clientApi.test.ts
next.config.ts
src/middleware.ts
```

The inspection confirmed that `src/services/api/chatService.ts` remains the
current chat API boundary, `NEXT_PUBLIC_API_MODE` is not implemented in the
original app, and there is no existing Next rewrite/proxy path for the Go API.

### Coverage

Implemented and tested scaffold behavior for:

- `NEXT_PUBLIC_API_MODE` normalization with missing/invalid mode falling back
  to `local`;
- `NEXT_PUBLIC_API_BASE_URL` normalization without network calls;
- browser network-edge classification as same-origin proxy or direct-CORS;
- safe fallback to `local` when `server` mode lacks a base URL;
- server HTTP URL building, JSON error envelope normalization, and network/CORS failure normalization;
- Go SSE named-event parsing and fail-closed event/type mismatch handling;
- compile-safe local/server chat adapter shells that return or throw explicit
  unsupported results instead of silently falling back to browser-local
  persistence.

### Verification

The root project has no installed local `pnpm` binaries in this environment, so
targeted verification used `corepack pnpm dlx` with pinned tool versions.

```text
corepack pnpm dlx vitest@4.1.9 run mm-chat/frontend/__tests__/api-client.test.ts
  passed: 1 file, 10 tests

corepack pnpm --package=typescript@5.9.3 dlx tsc --noEmit --target ES2020 --module ESNext --moduleResolution Bundler --lib DOM,ESNext --strict --skipLibCheck mm-chat/frontend/src/api-client/index.ts
  passed

corepack pnpm dlx prettier@3.9.4 --check 'mm-chat/frontend/**/*.ts' mm-chat/frontend/README.md mm-chat/frontend/DESIGN.md
  passed

module scanner script unavailable; fallback README/DESIGN/__tests__ check
  passed

security scanner script unavailable; fallback secret-pattern grep under mm-chat/frontend
  passed

git diff --check -- mm-chat
  passed
```

### Boundary

No `src/` file is part of this slice. The current app still has no active
`NEXT_PUBLIC_API_MODE` integration. The next implementation decision is whether
the owner approves adding the scaffold to `src/services/api/client/*` while
still avoiding `ChatApp`, stores, CRUD, SSE, files, auth, RAG, plugins, and
provider-settings changes.

### Review

Multi-agent review result: no code/security findings after fixes.

The only remaining review warning was commit hygiene: the root worktree still
contains many unrelated dirty files outside `mm-chat/`. This slice must be
staged with an explicit allowlist only:

```text
mm-chat/README.md
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
mm-chat/frontend/**
```

### Spec Update Judgment

No `.trellis/spec/` file was changed for this slice. The project-level spec
files are still generic placeholders, and the executable contract for this
work is task-local: `mm-chat/frontend/DESIGN.md`,
`mm-chat/docs/contracts/frontend-api-client.md`, and this process log. Keeping
the spec update inside `mm-chat/` also avoids mixing this scoped refactor commit
with unrelated untracked `.trellis/` workspace files.

## 2026-07-08 — Owner Decision: Preserve Frontend Stack and UI

### Decision

The original frontend technology stack and visible UI must stay unchanged while
server-mode functionality is connected.

```text
Keep:
- Next.js / React / TypeScript stack
- current component layout and visual UI
- current route structure and user-facing flows
- existing local mode rollback behavior

Change first:
- service/API-client boundary
- adapter mode selection
- DTO/error/SSE mapping
- targeted tests and docs
```

### Integration Rule

Original app changes under `src/` are now allowed only when they are narrow,
additive, and necessary to connect the adapter boundary. The preferred path is:

```text
src/services/api/client/*      -> add adapter boundary
src/services/api/chatService.ts -> later one narrow delegation point
ChatApp/components/store        -> unchanged unless a later phase authorizes it
```

This means functionality must be connected through the service layer, not by
rewriting UI components or replacing frontend technology.

### Next Step

Proceed to Phase 11.1B by adding the adapter boundary to the original app with
minimal files and tests. Do not wire CRUD, SSE, files, auth, RAG, plugins, or
provider-settings UI in 11.1B.

## 2026-07-08 — Phase 11.1B: Original App Adapter Boundary

### Action

Added the Phase 11.1B API-client scaffold to the original Next.js app service
layer without activating it from UI, stores, routes, or legacy
`chatService.ts`.

Created:

```text
src/services/api/client/types.ts
src/services/api/client/mode.ts
src/services/api/client/errors.ts
src/services/api/client/index.ts
src/services/api/client/local/chatApi.ts
src/services/api/client/server/httpClient.ts
src/services/api/client/server/chatApi.ts
src/services/api/client/server/sse.ts
src/__tests__/apiClientScaffold.test.ts
```

### Decision

Keep this slice as a compile-safe boundary only. `createNeoChatApiClient()`
resolves `local|server` mode, normalizes base URL/network-edge behavior, and
exposes explicit unsupported local/server chat shells. Conversation CRUD, SSE
streaming, files, auth, RAG, plugins, provider settings, and visible UI wiring
remain deferred to later Phase 11 slices.

Default behavior remains safe rollback:

```text
missing/invalid NEXT_PUBLIC_API_MODE -> local
NEXT_PUBLIC_API_MODE=server without NEXT_PUBLIC_API_BASE_URL -> local + warning
```

### Verification

Targeted verification passed:

```text
corepack pnpm dlx vitest@4.1.9 run src/__tests__/apiClientScaffold.test.ts
  passed: 1 file, 11 tests

corepack pnpm --package=typescript@5.9.3 dlx tsc --noEmit --target ES2020 --module ESNext --moduleResolution Bundler --lib DOM,ESNext --strict --skipLibCheck src/services/api/client/index.ts
  passed

corepack pnpm dlx prettier@3.9.4 --check 'src/services/api/client/**/*.ts' src/__tests__/apiClientScaffold.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed

git diff --check -- src/services/api/client src/__tests__/apiClientScaffold.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed

rg -n "services/api/client|createNeoChatApiClient" src/components src/features src/store src/services/api/chatService.ts
  no matches
```

ESLint could not be run through the incomplete local dependency install
(`corepack pnpm exec eslint` reported `Command "eslint" not found`). The
targeted TypeScript, Vitest, Prettier, whitespace, and no-UI-import checks were
used for this scaffold slice.

### Review

Multi-agent implementation plus independent review completed. Review result:
no code findings. Review warning remains commit hygiene only: the root worktree
contains many unrelated dirty files, so this slice must be staged with an
explicit allowlist.

### Boundary

This slice intentionally does not import the new client from
`src/components`, `src/features`, `src/store`, or
`src/services/api/chatService.ts`. It therefore cannot change visible UI or
runtime chat behavior until a later slice adds a narrow service-layer
delegation point.

### Next Step

Proceed to Phase 11.2: implement server-mode conversation/message CRUD inside
`src/services/api/client/server/chatApi.ts`, with targeted tests and no UI
rewrite.

## 2026-07-08 — Phase 11.2A: Server Chat CRUD Adapter Methods

### Action

Implemented the first Phase 11.2 server adapter slice for conversation and
message CRUD. This remains inside the API-client boundary and still does not
modify `ChatApp`, stores, routes, or legacy `chatService.ts`.

Changed:

```text
src/services/api/client/types.ts
src/services/api/client/index.ts
src/services/api/client/server/chatApi.ts
src/__tests__/apiClientScaffold.test.ts
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Evidence

Read-only backend contract inspection confirmed:

```text
POST /v1/chat/conversations                    -> 201 ConversationDTO
GET  /v1/chat/conversations                    -> 200 { items: ConversationDTO[] }
POST /v1/chat/conversations/{id}/messages      -> 201 ChatMessageDTO
GET  /v1/chat/conversations/{id}/messages      -> 200 { items: ChatMessageDTO[] }
```

Idempotency keys are JSON body fields (`idempotencyKey`), not headers. Backend
errors use `{ "error": { "code": string, "message": string } }`, which remains
handled by the shared HTTP client.

### Decision

Enable `capabilities.chatCrud` only for configured server mode while keeping
`chatStream`, `files`, `auth`, `imports`, `rag`, `plugins`, and
`providerSettings` disabled. This makes CRUD availability explicit without
turning on streaming or UI integration.

Server adapter rules for this slice:

```text
createConversation -> POST /v1/chat/conversations
listConversations  -> GET  /v1/chat/conversations, return page.items
appendUserMessage  -> POST /v1/chat/conversations/{id}/messages
listMessages       -> GET  /v1/chat/conversations/{id}/messages, return page.items
```

The adapter blocks blank user messages before the network call and only sends
server file references in attachments. Server-managed fields remain absent from
request bodies.

### Verification

Targeted verification passed:

```text
corepack pnpm dlx vitest@4.1.9 run src/__tests__/apiClientScaffold.test.ts
  passed: 1 file, 17 tests

corepack pnpm --package=typescript@5.9.3 dlx tsc --noEmit --target ES2020 --module ESNext --moduleResolution Bundler --lib DOM,ESNext --strict --skipLibCheck src/services/api/client/index.ts
  passed

corepack pnpm dlx prettier@3.9.4 --check 'src/services/api/client/**/*.ts' src/__tests__/apiClientScaffold.test.ts
  passed

git diff --check -- src/services/api/client src/__tests__/apiClientScaffold.test.ts
  passed

rg -n "services/api/client|createNeoChatApiClient" src/components src/features src/store src/services/api/chatService.ts
  no matches
```

Direct `tsc` against the Vitest test file was not used because the temporary
`dlx` TypeScript environment does not expose local `vitest` type declarations.
The test file is covered by Vitest execution.

### Review

Independent review found no blocking CRUD adapter issue. One non-blocking DTO
alignment risk was fixed by adding the backend-returned `outputBlocks`,
`metadata`, `attachments`, and `parentMessageId` fields to the frontend
`ChatMessageDTO` contract.

### Boundary

This slice does not prove browser refresh persistence or local-mode UI
regression, because the new adapter is still not imported by existing UI/service
callers. Those remain Phase 11.2B or later work.

### Next Step

Proceed to Phase 11.2B: add the narrow legacy service-layer delegation point
that can use the adapter in server mode while leaving local mode and visible UI
unchanged.

## 2026-07-08 — Phase 11.2B-1: CRUD Mapper and Service Gateway

### Action

Added a service-layer CRUD gateway above the Phase 11.2A API client adapter.
This slice prepares the bridge for later store integration but does not import
the gateway from UI, `ChatApp`, `chatStore`, or legacy `chatService.ts`.

Created:

```text
src/services/api/chatCrudService.ts
src/__tests__/chatCrudService.test.ts
```

Updated:

```text
mm-chat/docs/architecture/phase-11-plus-roadmap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Keep the gateway lightweight and dependency-narrow. It exposes
legacy-compatible session/message records without importing the full app store
or UI types. The service fails closed unless the API client is in configured
server mode with `capabilities.chatCrud = true`.

Mapping rules:

```text
ConversationDTO.updatedAt -> session.updatedAt (epoch ms)
ConversationDTO.modelRef  -> session.model ("provider:model")
ChatMessageDTO.role=user  -> message.role=user
ChatMessageDTO.role=assistant -> message.role=model
server file attachment -> /v1/files/{fileId}/content gateway URL
```

Unsupported backend roles such as `tool` or `system` are rejected with
`UNSUPPORTED_MESSAGE_ROLE` instead of being silently rendered incorrectly.

### Verification

Targeted verification passed:

```text
corepack pnpm dlx vitest@4.1.9 run src/__tests__/chatCrudService.test.ts src/__tests__/apiClientScaffold.test.ts
  passed: 2 files, 23 tests

corepack pnpm --package=typescript@5.9.3 dlx tsc --noEmit --target ES2020 --module ESNext --moduleResolution Bundler --lib DOM,ESNext --strict --skipLibCheck src/services/api/client/index.ts src/services/api/chatCrudService.ts
  passed

corepack pnpm dlx prettier@3.9.4 --check src/services/api/chatCrudService.ts src/__tests__/chatCrudService.test.ts
  passed
```

### Review

Independent review found two mapper hardening issues, both fixed before
commit:

- server attachment `downloadUrl` is no longer trusted or forwarded; the mapper
  always constructs the backend file-content gateway URL;
- conversation `config` is now whitelisted to legacy-compatible fields
  (`useSearch`, `useReasoning`, `activePlugins`, `activeSkills`) instead of
  casting arbitrary server metadata.

### Boundary

This slice still does not solve the async `createSession(): string` versus
server `createConversation(): Promise<ConversationDTO>` mismatch. Store
hydration/select/write integration remains deferred so visible UI behavior and
local rollback stay unchanged.

### Next Step

Proceed to Phase 11.2B-2: use the gateway for server-mode read path
experiments (`listConversations` + `listMessages`) while keeping the legacy
local path unchanged.

## 2026-07-08 — Phase 11.2B-2: Store Server Read Path Actions

### Action

Added explicit server-mode read actions to `chatStore` without calling them
from UI, bootstrap, `ChatApp`, or legacy `chatService.ts`.

Changed:

```text
src/store/core/chatStore.ts
src/__tests__/chatStoreServerRead.test.ts
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Keep the new read path opt-in until the async write-path mismatch is resolved.
The actions use `chatCrudService` only when server CRUD is enabled:

```text
refreshServerSessions() -> listConversations() -> serverReadState.sessions
selectServerSession(id) -> listMessages(id) -> serverReadState.activeMessageTree
```

Both actions return `false` without server calls or IndexedDB reads/writes when
server CRUD is disabled. Server-owned messages are not written to
`session_messages_*`; server-owned metadata and current selection are also kept
out of the persisted legacy `sessions/currentSessionId/activeMessages` path.
The backend remains source of truth for this path.

### Review Finding

An initial draft wrote server conversation metadata into the legacy
`sessions/currentSessionId` fields. Review flagged that those fields are
persisted by Zustand `partialize` into the main IndexedDB chat metadata key.
The implementation was corrected before commit by adding a non-persisted
`serverReadState` snapshot:

```text
serverReadState.sessions
serverReadState.currentSessionId
serverReadState.activeMessages
serverReadState.activeMessageTree
serverReadState.isLoading
serverReadState.error
```

`serverReadState` is initialized empty, reset during migration, and deliberately
omitted from `partialize`.

### Verification

Trellis check found one test-harness quality issue: the targeted store test
mock used a mutable `initialState` binding and invoked Zustand `partialize`
without narrowing the optional function/unknown return type. This failed
project lint/type-check. The test was tightened with an `initialStateRef`,
a runtime partialize assertion, and a narrow persisted-state cast.

Targeted verification passed after the fix:

```text
corepack pnpm vitest run src/__tests__/chatStoreServerRead.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/apiClientScaffold.test.ts
  passed: 3 files, 28 tests

corepack pnpm typecheck
  passed

corepack pnpm exec eslint src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts
  passed

corepack pnpm exec prettier --check src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed

git diff --check -- src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed
```

Full-suite caveats:

- `corepack pnpm lint` is blocked before linting by filesystem permissions while
  ESLint scans `mm-chat/data/postgres`.
- `corepack pnpm test` still has pre-existing/out-of-scope failures in
  `darkThemeTokens.test.ts`, `byokRoutes.test.ts`,
  `messageInputComposition.test.ts`, and `serverDefaults.test.ts`.

### Boundary

This slice still does not enable server mode in the visible UI. Existing local
`selectSession`, IndexedDB hydration, message writes, local streaming, and
rollback behavior remain unchanged until a later bootstrap/service integration
slice explicitly calls these server read actions.

### Next Step

Proceed to Phase 11.2B-3: decide the write-path strategy for async server
conversation creation versus the current synchronous `createSession(): string`
contract.

## 2026-07-08 — Phase 11.2B-3: Store Server Write Facade

### Action

Added opt-in async server write actions to `chatStore` while keeping the legacy
visible UI and local write path unchanged.

Changed:

```text
src/store/core/chatStore.ts
src/__tests__/chatStoreServerRead.test.ts
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Do not change `createSession(): string`. `ChatApp`, sidebar actions, shell
hooks, and existing tests depend on getting a local session id synchronously.
Instead, server writes use separate async facade actions:

```text
createServerSession(options) -> createConversation() -> serverReadState
appendServerUserMessage(options) -> appendUserMessage() -> serverReadState
```

The facade is still hidden/opt-in. It is not called by UI/bootstrap or
`chatService.ts`, and it does not claim a full server-backed send flow. Assistant
streaming remains Phase 11.3.

### Boundary

Server write actions deliberately avoid the local persistence chain:

```text
createSession()
addMessage()
syncActiveSession()
selectSession()
session_messages_*
sessions/currentSessionId/activeMessages
```

Results are stored only in non-persisted `serverReadState`. When server CRUD is
disabled, the actions return `null` and do not call server APIs or IndexedDB.
Missing `idempotencyKey` values are generated with `uuidv7()` before calling the
server CRUD gateway, and the selected model is converted with
`modelStringToModelRef()`.

### Trellis-check Finding

Review found two write-facade edge cases before this slice was closed:

- A successful stale server write returned `null`, even though the server had
  already created the conversation or persisted the user message. That would
  make the later SSE slice lose the server `conversationId`/`userMessageId`.
- Replaying an append with the same idempotency result could re-append the same
  message id to the active server tree and could reduce a known server
  `messageCount` when the active tree was not fully loaded.

Fix: stale writes now skip outdated `serverReadState` updates but still return
the persisted server id/message. Active server append updates now replace an
existing message id instead of duplicating it and use a monotonic count update.

### Verification

Targeted verification passed:

```text
corepack pnpm vitest run src/__tests__/chatStoreServerRead.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/apiClientScaffold.test.ts
  passed: 3 files, 34 tests

corepack pnpm exec eslint src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts
  passed

corepack pnpm typecheck
  passed

corepack pnpm exec prettier --check src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed

git diff --check -- src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed
```

### Review Inputs

Parallel read-only review confirmed:

- `createSession(): string` cannot be made async without breaking `ChatApp`,
  sidebar, hooks, and existing local tests.
- `chatService.ts` is not the persistence cut point; it streams provider output
  and relies on callbacks/store actions for persistence.
- The minimum safe B-3 scope is an async server write facade only, not visible
  ChatApp send-path integration.

### Next Step

Proceed to Phase 11.3: implement server SSE streaming against persisted server
messages, using the server-created `conversationId`, persisted `userMessageId`,
`modelRef`, and `idempotencyKey` without duplicating local placeholders.

## 2026-07-08 — Phase 11.3A: Server API Client SSE Adapter

### Action

Implemented the server-mode API client stream transport without wiring visible
UI, `ChatApp`, or store generation state to server streaming.

Changed:

```text
src/services/api/client/server/sse.ts
src/services/api/client/server/httpClient.ts
src/services/api/client/server/chatApi.ts
src/services/api/client/index.ts
src/__tests__/apiClientScaffold.test.ts
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Keep Phase 11.3A at the API-client boundary. The adapter now targets the Go
streaming contract:

```text
POST /v1/chat/conversations/{conversationId}/stream
Accept: text/event-stream
Content-Type: application/json
```

The request body is restricted to:

```text
userMessageId
modelRef
config?
systemInstruction?
systemPrompt?
metadata?
idempotencyKey
```

`conversationId` stays in the path only. The adapter does not send message
content, attachments, role/status, timestamps, identity hints, or other
server-managed fields.

### Implementation Notes

- Added an incremental SSE parser that preserves partial frames across chunks.
- Added `HttpClient.requestSse()` for POST + `ReadableStream` consumption.
- Implemented `streamAssistantMessage()` event dispatch for:
  - `message.started`
  - `message.delta`
  - `usage.updated`
  - `message.completed`
  - `message.error`
  - `message.cancelled`
- Implemented `cancelRun()` for `POST /v1/chat/runs/{runId}/cancel`.
- Enabled server-mode `chatStream` capability in the API client scaffold.

### Review Finding

Parallel contract review flagged two missing stream semantics before commit:

1. `sequence` values are monotonic per `runId`; duplicate sequence numbers must
   be ignored and gaps must fail closed with recoverable `STREAM_INTERRUPTED`.
2. If `AbortSignal` fires after `message.started`, the adapter must call the
   cancel endpoint with the captured server `runId`.

Both were implemented before commit. The adapter now ignores duplicate sequence
frames, fails on gaps, and calls `cancelRun` after a started stream is aborted.

Self-check follow-up fixed two adapter edge cases before handoff:

1. Incremental SSE parsing now preserves `\r\n` line endings split across
   chunks instead of treating a split CRLF as a blank-line frame delimiter.
2. If a caller aborts inside `onStarted` while the response already has buffered
   terminal frames, the adapter now stops consuming and posts the captured
   `runId` to the cancel endpoint.

### Verification

Targeted verification passed:

```text
corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/chatCrudService.test.ts
  passed: 2 files, 31 tests

corepack pnpm exec eslint src/services/api/client/server/sse.ts src/services/api/client/server/httpClient.ts src/services/api/client/server/chatApi.ts src/services/api/client/index.ts src/__tests__/apiClientScaffold.test.ts
  passed

corepack pnpm typecheck
  passed

corepack pnpm exec prettier --check src/services/api/client/server/sse.ts src/services/api/client/server/httpClient.ts src/services/api/client/server/chatApi.ts src/services/api/client/index.ts src/__tests__/apiClientScaffold.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed

git diff --check -- src/services/api/client/server/sse.ts src/services/api/client/server/httpClient.ts src/services/api/client/server/chatApi.ts src/services/api/client/index.ts src/__tests__/apiClientScaffold.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed
```

### Boundary

This is not yet full server-backed chat generation. The visible app still uses
the existing local provider streaming path. Server-mode UI/store integration,
terminal UI state mapping, and live Go backend verification remain later Phase
11.3 work.

### Next Step

Proceed to Phase 11.3B: add an opt-in server stream facade above the API client
that can combine `appendServerUserMessage()` with `streamAssistantMessage()` and
update `serverReadState` without touching the local chat write path.

## 2026-07-08 — Phase 11.3B: Store Server Stream Facade

### Action

Added a hidden store-level server stream facade without wiring visible UI,
`ChatApp`, or bootstrap to server streaming.

Changed:

```text
src/services/api/chatStreamService.ts
src/services/api/chatCrudService.ts
src/store/core/chatStore.ts
src/__tests__/chatStreamService.test.ts
src/__tests__/chatCrudService.test.ts
src/__tests__/chatStoreServerRead.test.ts
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

### Decision

Keep stream lifecycle semantics out of `chatCrudService`. CRUD remains focused
on conversation/message create/list and DTO mapping. `chatStreamService` owns
server stream/cancel delegation and terminal message mapping.

The store-level facade is still opt-in only:

```text
sendServerMessageAndStream(options)
  -> chatCrudService.appendUserMessage()
  -> chatStreamService.streamAssistantMessage()
  -> serverReadState only
```

It does not call `createSession`, `addMessage`, `updateMessage`,
`setMessages`, `syncActiveSession`, `selectSession`, local provider streaming,
or any IndexedDB `session_messages_*` path.

### Boundary

Server stream state is written only into non-persisted `serverReadState`.
Legacy local fields remain untouched:

```text
sessions
currentSessionId
activeMessages
activeMessageTree
isActiveSessionLoading
```

The facade creates/updates assistant placeholders from `message.started` and
`message.delta`, then replaces the placeholder with the terminal server message
when the stream result includes one. When CRUD or stream capability is disabled,
it returns `null` and makes no server or local-storage writes.

### Trellis-check Finding

Review found one hidden snapshot edge case before handoff: assistant
`message.started`/`message.delta` draft events for a non-current server session
would call the shared message apply helper while the active tree did not contain
that assistant id, inflating the non-current session `messageCount` once per
draft event.

Fix: assistant draft updates now no-op unless the streamed session is the
current `serverReadState.currentSessionId`. The persisted user message and the
terminal assistant message still update the target server session count once
each, while the current server snapshot and legacy local chat state remain
unchanged.

### Verification

Targeted verification passed:

```text
corepack pnpm vitest run src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/__tests__/chatStoreServerRead.test.ts src/__tests__/apiClientScaffold.test.ts
  passed: 4 files, 47 tests

corepack pnpm exec eslint src/services/api/chatCrudService.ts src/services/api/chatStreamService.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts
  passed

corepack pnpm typecheck
  passed

corepack pnpm exec prettier --check src/services/api/chatCrudService.ts src/services/api/chatStreamService.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts
  passed

git diff --check -- src/services/api/chatCrudService.ts src/services/api/chatStreamService.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts
  passed
```

Reviewer follow-up verification passed:

```text
corepack pnpm vitest run src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/__tests__/chatStoreServerRead.test.ts
  passed: 3 files, 22 tests

corepack pnpm exec eslint src/services/api/chatCrudService.ts src/services/api/chatStreamService.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts
  passed

corepack pnpm typecheck
  passed

corepack pnpm exec prettier --check src/services/api/chatCrudService.ts src/services/api/chatStreamService.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed

git diff --check -- src/services/api/chatCrudService.ts src/services/api/chatStreamService.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed
```

### Remaining Boundary

This slice still does not map server stream lifecycle into the visible UI
terminal state and has not been smoke-tested against the live Go backend. Those
remain Phase 11.3 follow-up work.

### Next Step

Proceed to Phase 11.3C: decide whether to add non-persisted server generation
state (`generation`, `activeServerRunId`) before any visible UI wiring, then
verify against the local Go backend.

## 2026-07-08 — Phase 11.3C: Terminal Server Generation State

### Action

Added hidden, non-persisted server stream lifecycle state under
`serverReadState` without wiring visible UI, `ChatApp`, or server cancel
controls.

Changed:

```text
src/store/core/chatStore.ts
src/__tests__/chatStoreServerRead.test.ts
src/__tests__/chatStreamService.test.ts
mm-chat/docs/architecture/phase-11-plus-roadmap.md
mm-chat/docs/tracking/progress.md
```

### Decision

Keep Phase 11.3C at the hidden store snapshot boundary. Server streams now track
an explicit lifecycle record in `serverReadState.generation`:

```text
status
sessionId
userMessageId
assistantMessageId
activeServerRunId
error
```

`message.started.runId` is captured as `activeServerRunId` while the hidden
server stream is active. Completed, failed, unsupported, and cancelled terminal
results clear the active run id and set terminal generation state. The state is
not written to the legacy local chat fields or persisted chat metadata.

### Boundary

This slice still does not connect the visible send path, visible stop/cancel UI,
or `ChatApp` to server streaming. Local provider streaming remains the active UI
path unless a later Phase 11 slice explicitly wires server mode into the UI.

### Verification

Targeted verification passed:

```text
corepack pnpm vitest run src/__tests__/chatStreamService.test.ts src/__tests__/chatStoreServerRead.test.ts src/__tests__/apiClientScaffold.test.ts src/__tests__/chatCrudService.test.ts
  passed: 4 files, 51 tests

corepack pnpm typecheck
  passed

corepack pnpm exec eslint src/store/core/chatStore.ts src/services/api/chatStreamService.ts src/__tests__/chatStoreServerRead.test.ts src/__tests__/chatStreamService.test.ts
  passed
```

The new tests cover successful streaming, provider failure, cancellation,
run-id propagation, stale terminal suppression after a newer server selection,
error-envelope preservation, and persisted-state exclusion through the existing
`serverReadState` partialize check.

### Review

A `trellis-check` review agent was dispatched for this slice. Findings, if any,
will be recorded in the follow-up entry before commit.

### Next Step

Address review findings, then proceed to live Go backend smoke for server CRUD +
SSE before any visible UI wiring.

## 2026-07-08 — Phase 11.3C Review Follow-up

### Action

Applied the review-agent findings for the terminal server generation state slice.

### Findings Fixed

```text
src/store/core/chatStore.ts
  Unsupported stream terminal results now use the same terminal error fallback as
  failed results, so both generation.error and serverReadState.error surface the
  failure consistently.

src/__tests__/chatStoreServerRead.test.ts
  Added unsupported terminal mapping coverage and strengthened the persist
  boundary test so active server run ids and request ids cannot leak into the
  persisted chat payload.

mm-chat/docs/architecture/phase-11-plus-roadmap.md
  Clarified the 11.3C output as progress entries instead of implying a separate
  process-entry output requirement inside the architecture doc.

mm-chat/docs/tracking/progress.md
  Updated 11.3C coverage notes to include unsupported fallback behavior.
```

### Verification

Review-agent verification passed:

```text
corepack pnpm exec eslint src/store/core/chatStore.ts src/__tests__/chatStoreServerRead.test.ts src/__tests__/chatStreamService.test.ts
  passed

corepack pnpm exec tsc --noEmit --pretty false
  passed

corepack pnpm exec vitest run src/__tests__/chatStoreServerRead.test.ts src/__tests__/chatStreamService.test.ts
  passed: 2 files, 21 tests
```

### Next Step

Run final main-session verification for the full Phase 11.3C touched scope, then
commit only the explicit slice files.

## 2026-07-08 — Phase 11.3D: Live Backend SSE Smoke

### Action

Verified the existing local single-server Go backend path end-to-end without
wiring the visible frontend UI.

### Runtime Boundary

Used the already-running Compose stack from `mm-chat/compose.single-server.yml`
with the local secret file passed by path only:

```bash
cd mm-chat
docker compose --env-file .env.single-server -f compose.single-server.yml ps
```

Services observed running:

```text
backend   healthy   127.0.0.1:8080->8080/tcp
postgres  healthy
redis     healthy
minio     running
```

The provider configuration was read by the backend process from
`.env.single-server`; secrets were not copied into docs or command output.

### Smoke Flow

The smoke script called the local API directly:

```text
GET  /health
GET  /ready
GET  /v1/version
POST /v1/chat/conversations
POST /v1/chat/conversations/{conversationId}/messages
POST /v1/chat/conversations/{conversationId}/stream  # Accept: text/event-stream
GET  /v1/chat/conversations/{conversationId}/messages
```

Smoke identifiers:

```text
run:              phase-11-3-smoke-1783500901
conversationId:   f47b6de9-dab7-4864-b8da-4e6e5a2a3934
userMessageId:    89929427-e328-4b59-bc4c-5a9304d98744
assistantRunId:   285338b6-3433-459f-9826-2547c3e270f8
assistantMessage: d26c5c29-e0c1-4f90-bc73-389652e5ca60
SSE artifact:     /tmp/mm-chat-smoke/phase-11-3-smoke-1783500901.sse
```

### Result

Health/readiness/version:

```text
/health      200 healthy
/ready       200 ready
/v1/version  200 single-server-dev
```

Conversation/message/stream:

```text
POST conversation  201 created
POST user message  201 completed user row
SSE events         7 frames
SSE terminal       message.completed
assistant status   completed
assistant content  16 bytes
GET messages       200, 2 rows persisted
```

Observed SSE event sequence:

```text
message.started
message.delta
message.delta
message.delta
message.delta
usage.updated
message.completed
```

Post-stream list confirmed two persisted rows:

```text
user       completed  contentLength=48
assistant  completed  contentLength=16
```

### Local-Mode Regression

Ran targeted legacy local-mode/frontend rollback checks:

```text
corepack pnpm vitest run src/__tests__/chatServiceToolConfirmation.test.ts src/__tests__/chatStore.test.ts src/__tests__/apiClientScaffold.test.ts
  passed: 3 files, 64 tests
```

### Cleanup / Reset Notes

The smoke intentionally left one test conversation and its two messages in the
local Postgres volume for auditability. Cleanup options for future destructive
reset drills:

```bash
cd mm-chat
docker compose --env-file .env.single-server -f compose.single-server.yml down
# add `-v` only when intentionally deleting local Postgres/Redis/MinIO data
```

Do not use `down -v` unless local smoke data loss is intended.

### Next Step

Proceed to Phase 11.4 file upload/download adapter planning and implementation,
or stop here if the goal is only to prove Phase 11.3 backend stream persistence.

## 2026-07-08 — Phase 11.3D Review

### Action

Ran a read-only review agent against the Phase 11.3D live backend smoke docs.

### Result

```text
no findings
```

Review confirmed:

```text
progress entries have dated process evidence
process log records command shape, artifact path, results, and cleanup notes
provider secrets are not copied into docs
roadmap records objective, scope, outputs, verification, and rollback
visible UI wiring is not claimed
```

### Next Step

Commit the Phase 11.3D smoke documentation slice only.

## 2026-07-08 — Phase 11.4A: Server File API Client Adapter

### Action

Added the hidden server-mode file API adapter under the existing frontend API
client boundary. This slice does not wire visible UI, `ChatApp`, OPFS
replacement, message input uploads, or browser attachment rendering.

Changed:

```text
src/services/api/client/types.ts
src/services/api/client/index.ts
src/services/api/client/server/httpClient.ts
src/services/api/client/server/fileApi.ts
src/services/api/client/local/fileApi.ts
src/__tests__/apiClientScaffold.test.ts
src/__tests__/chatCrudService.test.ts
src/__tests__/chatStreamService.test.ts
mm-chat/docs/architecture/phase-11-plus-roadmap.md
mm-chat/docs/tracking/progress.md
```

### Decision

Keep Phase 11.4A at the API-client transport boundary only:

```text
client.files.uploadFile()          -> POST /v1/files multipart/form-data
client.files.getFile()             -> GET /v1/files/{id}
client.files.downloadFileContent() -> GET /v1/files/{id}/content
client.files.deleteFile()          -> DELETE /v1/files/{id}
```

Configured server mode now advertises `capabilities.files = true`. Local mode
uses an explicit unsupported file shell so local OPFS behavior remains the
existing rollback path and no server fallback is hidden inside UI code.

### Boundary

The server file adapter whitelists the public file record fields returned to
frontend callers:

```text
id, fileName, mimeType, size, sha256, purpose, createdAt, downloadUrl
```

`downloadUrl` is accepted only when it exactly matches the backend gateway
shape `/v1/files/{id}/content` for the returned UUID `id`. The adapter rejects
absolute MinIO/S3 URLs, object-key style nested paths, path traversal, mismatched
file IDs, encoded path segments, and unsupported purpose values.

### Verification

Targeted verification passed:

```text
corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/__tests__/chatStoreServerRead.test.ts
  passed: 4 files, 57 tests

corepack pnpm typecheck
  passed

corepack pnpm exec eslint src/services/api/client/types.ts src/services/api/client/index.ts src/services/api/client/server/httpClient.ts src/services/api/client/server/fileApi.ts src/services/api/client/local/fileApi.ts src/__tests__/apiClientScaffold.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts
  passed
```

Tests cover server capability gating, local unsupported behavior, multipart
upload request shape without manual multipart `Content-Type`, metadata URL
encoding, binary download through the backend gateway, delete routing,
error-envelope normalization, and private object-store path rejection.

### Review Follow-up

A read-only review pass flagged that file metadata should bind `downloadUrl` to
its returned UUID `id`, not only to the generic `/v1/files/{segment}/content`
shape. The adapter now requires an exact `/v1/files/{id}/content` match and
rejects mismatched IDs plus encoded path-style IDs. The follow-up review
reported no findings.

### Next Step

Proceed to Phase 11.4B: add a small file-service gateway and/or live backend
file smoke for upload, download, message attachment, and refresh metadata.

## 2026-07-08 — Phase 11.4B Plan: File Service Gateway and Attachment Smoke

### Decision

Split Phase 11.4 into two smaller frontend-safe slices:

```text
11.4B1 -> service gateway, server attachment mapper, DTO metadata preservation,
          and reusable live API smoke
11.4B2 -> MessageInput/ChatApp wiring for browser-selected files
```

Reason: `ChatApp.tsx`, `MessageInput.tsx`, and several UI files currently carry
unrelated line-ending noise in the working tree. This slice avoids touching
visible UI files and prevents accidental UI churn while still proving the
server upload/download/attach contract.

### Scope

11.4B1 will change only service/test/docs/script boundaries:

```text
src/services/api/fileService.ts
src/lib/utils/serverAttachments.ts
src/services/api/chatCrudService.ts
src/__tests__/fileService.test.ts
src/__tests__/serverAttachments.test.ts
src/__tests__/chatCrudService.test.ts
mm-chat/scripts/smoke-phase-11-4b-file-attachments.sh
```

No `ChatApp`, `MessageInput`, OPFS utilities, visible component structure, or
local attachment path changes are in scope for this slice.

### Verification Plan

- Unit tests prove server-mode file upload maps `FileRecordDTO` to a legacy
  attachment with server metadata and fail-closed local mode.
- Unit tests prove only server-backed attachments can become Go message
  attachment references.
- CRUD mapper tests prove refreshed message attachments keep `source`, `fileId`,
  `size`, `sha256`, `purpose`, and a backend-gateway URL while ignoring any
  unsafe server `downloadUrl`.
- Live smoke script proves upload, metadata, byte download, message attach, and
  list-message refresh against `http://127.0.0.1:8080`.

### Risks

- Browser UI remains unwired until 11.4B2.
- Files uploaded during smoke are intentionally retained with their smoke
  conversation/message rows because deleting the file would remove attachment
  metadata from later list-message verification.
- Attachment-only messages still need a policy decision in the UI wiring slice
  because Go chat message creation requires non-empty `content`.

## 2026-07-08 — Phase 11.4B1: File Service Gateway and Live Smoke

### Action

Added the service/mapper seam for server-backed file attachments without wiring
visible UI:

```text
src/services/api/fileService.ts
src/lib/utils/serverAttachments.ts
src/services/api/chatCrudService.ts
src/__tests__/fileService.test.ts
src/__tests__/serverAttachments.test.ts
src/__tests__/chatCrudService.test.ts
mm-chat/scripts/smoke-phase-11-4b-file-attachments.sh
```

The file service uploads chat files with server file purpose `chat`, maps the
returned `FileRecordDTO` into a legacy attachment carrying `source: "server"`,
`fileId`, `size`, `sha256`, `purpose: "input"`, and a backend gateway URL.
The server attachment mapper converts only server-backed attachments into Go
message attachment refs and rejects local/base64/OPFS/remote attachments.

### Verification

Targeted checks passed:

```text
corepack pnpm vitest run src/__tests__/fileService.test.ts src/__tests__/serverAttachments.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/apiClientScaffold.test.ts
  passed: 4 files, 44 tests

corepack pnpm typecheck
  passed

corepack pnpm exec eslint src/lib/utils/serverAttachments.ts src/services/api/fileService.ts src/services/api/chatCrudService.ts src/__tests__/serverAttachments.test.ts src/__tests__/fileService.test.ts src/__tests__/chatCrudService.test.ts
  passed

corepack pnpm exec prettier --check <11.4B1 ts/md files>
  passed

bash -n mm-chat/scripts/smoke-phase-11-4b-file-attachments.sh
  passed
```

Local Compose services were healthy before smoke:

```text
backend: 127.0.0.1:8080 healthy
postgres: healthy
redis: up
minio: up
```

Live API smoke passed:

```text
command:        mm-chat/scripts/smoke-phase-11-4b-file-attachments.sh
run:            phase-11-4b-file-smoke-1783503755-27227
fileId:         948591cb-52b7-497b-b9c7-157e2fefd490
conversationId: feaec225-b164-4c9f-a189-b06977388e10
messageId:      95851edd-b8c7-4c71-8d0b-5fb8914241b1
artifacts:      /tmp/mm-chat-smoke/phase-11-4b-file-smoke-1783503755-27227
sha256:         dd2696e7eaaa64645250e5d0a9b6c1cfea4949856fe7c2cd7e0f728901cf3bc0
byte compare:   passed
```

Smoke verified upload metadata, `GET /v1/files/{id}`, byte download through
`/content`, message append with `{source:"server", fileId, purpose:"input"}`,
and list-message refresh preserving the same attachment metadata. Responses did
not expose object keys, bucket names, local paths, MinIO/S3 URLs, or presigned
URLs.

### Review Follow-up

A read-only review pass reported no target-code blockers. It warned that
unrelated line-ending churn exists in visible UI/OPFS files and must remain
excluded from this commit. It also suggested extending the smoke metadata check
to reject forbidden storage fields on `GET /v1/files/{id}`; the smoke script now
checks metadata responses for the same object-key, bucket, local-path,
storage-backend, and presigned-URL leaks as upload responses.

### Cleanup Notes

The smoke intentionally leaves one test file, conversation, and message in the
local Compose data so the refreshed attachment metadata remains auditable. Do
not run `docker compose down -v` unless local smoke data loss is intended.

### Next Step

Run a review pass for 11.4B1, then proceed to 11.4B2: wire `MessageInput` and
`ChatApp` to the service/mapper seam while preserving local OPFS behavior.

## 2026-07-08 — Phase 11.4C: Server-Mode Browser Send Wiring

### Decision

Use a smaller UI wiring path than originally sketched:

```text
MessageInput remains unchanged
existing Attachment.data/base64 -> upload at send time -> server fileId refs
ChatApp chooses local state or serverReadState based on API mode
```

This preserves visible UI and avoids changing the file picker/parser path.
Local mode still uses the existing `processMessageForSending`, OPFS, IndexedDB,
and `/api/chat` provider route.

### Action

Changed:

```text
eslint.config.mjs
src/components/app/ChatApp.tsx
src/components/chat/MessageInput.tsx
src/features/chat/hooks/useChatGenerationController.ts
src/features/chat/hooks/useChatShellState.ts
src/services/api/fileService.ts
src/__tests__/chatAppServerModeComposition.test.ts
src/__tests__/fileService.test.ts
src/__tests__/messageInputComposition.test.ts
```

Implementation notes:

- `useChatShellState()` now exposes `serverReadState`,
  `refreshServerSessions`, `selectServerSession`, `createServerSession`, and
  `sendServerMessageAndStream`.
- `ChatApp` computes a server-mode branch from `createNeoChatApiClient()`
  without a network call.
- In server mode, visible sidebar/messages read from `serverReadState`; in
  local mode they still read from `sessions/activeMessages`.
- Sending in server mode uploads inline/base64 attachments through
  `uploadMessageAttachmentsForServer()`, converts them with
  `toServerMessageAttachments()`, then calls `sendServerMessageAndStream()`.
- Workspace OPFS attachment hydration is skipped in server mode; local mode
  remains unchanged.
- Local-only actions without Go endpoints fail closed with a visible error
  instead of mutating local IndexedDB while viewing server messages.
- `MessageInput` keeps the same visible tool buttons but receives
  `localSessionToolsDisabled` in server mode. Plugin, skill, search, and
  reasoning controls fail closed before calling local Zustand write actions.
- `useChatGenerationController()` now exposes `abortActiveGeneration()`.
  Server-mode stop, new-chat, and sidebar session selection use abort-only
  handling instead of `stopActiveGeneration()`, so stopped server streams do not
  persist local `activeMessages` into IndexedDB.
- Server send uses the active server conversation config for `useSearch` and
  `useReasoning`, and passes empty local plugin lists to the effective context
  resolver until Go plugin/skill endpoints exist.

### Review Follow-up

Review found three blockers and they were fixed before handoff:

1. `MessageInput` still read plugin/skill state directly from local stores.
   Fixed with explicit server-mode fail-closed guards.
2. Stop/new-chat/session-switch could call the local stopped-generation sync
   path. Fixed by splitting abort-only generation control from persisted stop.
3. Server session config was not consistently used by composer/send context.
   Fixed by deriving `composerChatConfig` from server session config in server
   mode and sending only the server-safe config subset.
4. Full-project ESLint attempted to traverse local Docker runtime volumes under
   `mm-chat/data`. Fixed by adding `mm-chat/data/**` and `mm-chat/backup/**`
   to the global ESLint ignore list.

### Verification

Targeted checks passed:

```text
corepack pnpm vitest run src/__tests__/fileService.test.ts src/__tests__/chatStoreServerRead.test.ts src/__tests__/chatAppFirstScreenComposition.test.ts src/__tests__/chatAppServerModeComposition.test.ts src/__tests__/messageInputComposition.test.ts src/__tests__/sidebarComposition.test.ts src/__tests__/messageItemComposition.test.ts
  passed: 7 files, 32 tests

corepack pnpm typecheck
  passed

corepack pnpm exec eslint src/components/app/ChatApp.tsx src/components/chat/MessageInput.tsx src/features/chat/hooks/useChatGenerationController.ts src/features/chat/hooks/useChatShellState.ts src/services/api/fileService.ts src/__tests__/fileService.test.ts src/__tests__/messageInputComposition.test.ts src/__tests__/chatAppServerModeComposition.test.ts
  passed

corepack pnpm lint
  passed: 0 errors, 19 existing unused-argument warnings

corepack pnpm exec prettier --check src/components/app/ChatApp.tsx src/components/chat/MessageInput.tsx src/features/chat/hooks/useChatGenerationController.ts src/features/chat/hooks/useChatShellState.ts src/services/api/fileService.ts src/__tests__/fileService.test.ts src/__tests__/messageInputComposition.test.ts src/__tests__/chatAppServerModeComposition.test.ts mm-chat/docs/architecture/phase-11-plus-roadmap.md mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed

git diff --check -- src/components/app/ChatApp.tsx src/components/chat/MessageInput.tsx src/features/chat/hooks/useChatShellState.ts src/features/chat/hooks/useChatGenerationController.ts src/services/api/fileService.ts src/__tests__/fileService.test.ts src/__tests__/messageInputComposition.test.ts src/__tests__/chatAppServerModeComposition.test.ts mm-chat/docs/architecture/phase-11-plus-roadmap.md mm-chat/docs/tracking/progress.md mm-chat/docs/tracking/process.md
  passed
```

### Remaining Gap

Full browser validation was deferred to Phase 11.5 and is now recorded in the
2026-07-09 Phase 11.5 entry below. The accepted browser path uses
`NEXT_PUBLIC_API_BASE_URL=/mm-api` through a same-origin development proxy
instead of direct browser calls to `http://127.0.0.1:8080`, because the Go
backend does not yet emit CORS headers.

## 2026-07-09 — Phase 11.5: Browser Smoke and Local Rollback

### Decision

Browser smoke uses a same-origin development proxy instead of direct browser
calls to the Go backend. The backend is still verified at
`http://127.0.0.1:8080`, but the browser calls `/mm-api/v1/...` through a
temporary local proxy on port `3000` to avoid CORS drift while Go has no CORS
allowlist.

Two frontend blockers were found during smoke and fixed before accepting the
result:

- `readApiClientEnv()` used a dynamic `globalThis.process?.env` lookup, so the
  browser bundle did not inline `NEXT_PUBLIC_API_MODE=server` and silently stayed
  on the local `/api/chat` path.
- The frontend server-default model id `SERVER_DEFAULT:gpt-5.5` had to be
  normalized to Go's configured provider id `openai_compatible:gpt-5.5` before
  streaming, otherwise the backend would reject it as `UNSUPPORTED_PROVIDER`.

### Action

Changed:

```text
src/services/api/client/mode.ts
src/services/api/chatCrudService.ts
src/__tests__/apiClientScaffold.test.ts
src/__tests__/chatCrudService.test.ts
src/__tests__/chatStoreServerRead.test.ts
mm-chat/docs/architecture/phase-11-plus-roadmap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Server-mode run shape:

```bash
rm -rf .next
set -a; . mm-chat/.env.single-server; set +a

NEXT_PUBLIC_API_MODE=server \
NEXT_PUBLIC_API_BASE_URL=/mm-api \
NEXT_PUBLIC_SITE_URL=http://127.0.0.1:3000 \
DEFAULT_PROVIDER_TYPE="OpenAI Compatible" \
DEFAULT_PROVIDER_NAME="Smoke Relay" \
DEFAULT_PROVIDER_BASE_URL="$PROVIDER_BASE_URL" \
DEFAULT_PROVIDER_API_KEY=[redacted] \
DEFAULT_PROVIDER_MODELS="$PROVIDER_MODEL" \
corepack pnpm dev -H 127.0.0.1 -p 3001

node /tmp/mm-chat-phase-11-5-proxy.mjs
# /mm-api/* -> http://127.0.0.1:8080/*
# /*        -> http://127.0.0.1:3001/*
```

Important command correction: with this package script, use
`corepack pnpm dev -H 127.0.0.1 -p <port>`. The `pnpm dev -- -H ...` form makes
Next treat `-H` as a project directory and exits.

### Verification

Compose backend stayed healthy:

```text
docker compose -f mm-chat/compose.single-server.yml --env-file mm-chat/.env.single-server ps
backend: 127.0.0.1:8080 healthy
/ready:      {"status":"ready"}
/v1/version: {"version":"single-server-dev"}
```

Targeted tests passed after the fixes:

```text
corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStoreServerRead.test.ts src/__tests__/fileService.test.ts src/__tests__/chatAppServerModeComposition.test.ts
  passed: 5 files, 64 tests
```

Server-mode browser smoke passed with a real Chromium browser over the visible
UI:

```text
artifacts:       /tmp/mm-chat-smoke/phase-11-5-browser-smoke-20260709-133930
conversationId:  feaec225-b164-4c9f-a189-b06977388e10
fileId:          36af1e00-796f-4532-9f23-f81fa8f8d649
userMessageId:   95ed4c11-b36f-4dc7-a12a-97f428e24804
assistantId:     3ff06dea-a8a5-4bf8-a4df-18f67275e99e
token:           MM_CHAT_BROWSER_ATTACHMENT_OK_1783576072233
file sha256:     5d7c8bf6af63ecd0de3a732c4333ae4154c18a6067bfe9cc1668a915402a7cdd
```

Observed browser network path:

```text
GET  /mm-api/v1/chat/conversations                         200 application/json
GET  /mm-api/v1/chat/conversations/{id}/messages           200 application/json
POST /mm-api/v1/files                                      201 application/json
POST /mm-api/v1/chat/conversations/{id}/messages           201 application/json
POST /mm-api/v1/chat/conversations/{id}/stream             200 text/event-stream
```

The stream request body used:

```json
{ "modelRef": { "providerId": "openai_compatible", "modelId": "gpt-5.5" } }
```

Backend verification confirmed:

- the user message persisted with a server attachment reference;
- the assistant message completed with the expected token;
- `GET /v1/files/{fileId}` returned metadata only, with a relative
  `downloadUrl`;
- `GET /v1/files/{fileId}/content?disposition=attachment` returned bytes whose
  SHA-256 matched the message attachment metadata;
- a fresh browser reload reloaded the same token and attachment through only
  `GET /mm-api/v1/chat/conversations` and `GET /mm-api/v1/chat/conversations/{id}/messages`.

Local rollback passed after stopping the server-mode proxy and restarting Next
directly on port `3000`:

```bash
NEXT_PUBLIC_API_MODE=local \
NEXT_PUBLIC_API_BASE_URL= \
NEXT_PUBLIC_SITE_URL=http://127.0.0.1:3000 \
DEFAULT_PROVIDER_API_KEY=[redacted] \
corepack pnpm dev -H 127.0.0.1 -p 3000
```

Rollback browser artifact:

```text
artifacts: /tmp/mm-chat-smoke/phase-11-5-browser-smoke-20260709-133930/local-rollback-localhost
token:     MM_CHAT_BROWSER_ATTACHMENT_OK_1783576252663
```

Observed rollback network path:

```text
POST /api/chat                   200 text/event-stream
POST /api/chat                   200 text/event-stream
POST /api/chat/related-questions requested
POST /api/chat/generate-title    requested
```

No `/mm-api` or `/v1/chat` calls were made during the accepted local rollback
run.

Visible Windows Chrome smoke also passed after the automated run:

```text
browser URL:    http://localhost:3000
manual prompt:  请只回复：MM_CHAT_MANUAL_OK
manual result:  MM_CHAT_MANUAL_OK
file prompt:    读取附件内容，只回复：MM_CHAT_FILE_OK
file result:    MM_CHAT_FILE_OK
file path:      C:\Users\Administrator\Desktop\mm-chat-manual-file-test.txt
```

Proxy evidence confirmed the visible browser used the Go path:

```text
POST /mm-api/v1/chat/conversations/{id}/messages -> go /v1/chat/conversations/{id}/messages
POST /mm-api/v1/chat/conversations/{id}/stream   -> go /v1/chat/conversations/{id}/stream
POST /mm-api/v1/files                            -> go /v1/files
GET  /mm-api/v1/chat/conversations               -> go /v1/chat/conversations
GET  /mm-api/v1/chat/conversations/{id}/messages -> go /v1/chat/conversations/{id}/messages
```

### Known Gaps / Cleanup Notes

- The temporary `/tmp/mm-chat-phase-11-5-proxy.mjs` is smoke harness only; no
  repository proxy file was added in this slice.
- Direct browser calls to `http://127.0.0.1:8080` still lack CORS headers. Keep
  using same-origin proxy in development or add a documented CORS allowlist in a
  later slice.
- Local rollback should be opened as `http://localhost:3000` for browser smoke.
  A direct `127.0.0.1:3000` POST can trip the existing Next request-origin guard
  in dev.
- Manual visible-Chrome server-mode testing used `3000` for the same-origin
  proxy and `3001` for Next; both were stopped after the smoke. The Go Compose
  backend on `127.0.0.1:8080` was intentionally left running.
- The smoke intentionally leaves test conversation/file data in the local
  Compose volumes for auditability. Do not run `docker compose down -v` unless
  losing local smoke data is intended.

## 2026-07-09 — Phase 11.2 Top-Level Reconciliation

Action: reconciled the seven remaining Phase 11.2 parent checklist items
against completed sub-slices and review findings, then marked them complete in
`progress.md`.

Evidence mapping:

```text
DTO contract/mapping:
  process.md Phase 11.2A confirmed Go CRUD shapes.
  process.md Phase 11.2B-1 recorded ConversationDTO/ChatMessageDTO -> legacy
  session/message mapping and targeted tests.

Conversation create/list:
  Phase 11.2A implemented createConversation/listConversations.
  Phase 11.2B-2 added refreshServerSessions() through listConversations().
  Phase 11.2B-3 added createServerSession().
  Phase 11.4C wired ChatApp server mode to serverReadState.

Message create/list:
  Phase 11.2A implemented appendUserMessage/listMessages.
  Phase 11.2B-2 added selectServerSession() through listMessages().
  Phase 11.2B-3 added appendServerUserMessage().
  Phase 11.5 browser smoke hit Go message append/list through /mm-api.

Error mapping:
  Phase 11.1/11.2A recorded backend error-envelope normalization.
  The shared server HTTP client maps validation, not-found, conflict, and
  database-required envelopes into ApiClientError.

Browser refresh/local rollback:
  Phase 11.5 recorded refresh reloading server-owned state through
  GET /mm-api/v1/chat/conversations and GET /messages.
  Phase 11.5 recorded local rollback using /api/chat only, with no /mm-api or
  /v1/chat calls.
```

Review result: the first read-only evidence pass considered all seven parent
items complete. The independent review agent found one documentation gap: the
accepted Phase 11.5 browser smoke reused an existing conversation and did not
show a frontend server-mode `POST /v1/chat/conversations`.

Remediation smoke: ran the frontend server API client against the local Go
backend at `http://127.0.0.1:8080` and verified create/list conversation plus
append/list message with a fresh server-created conversation.

```text
command:
  RUN_ID="phase-11-reconcile-20260709T191936" \
  ARTIFACT="/tmp/mm-chat-smoke/phase-11-reconcile-api-client.json" \
  corepack pnpm dlx tsx@4.20.6 /tmp/mm-chat-phase-11-reconcile-smoke.mts

artifact:
  /tmp/mm-chat-smoke/phase-11-reconcile-api-client.json

observed requests:
  POST /v1/chat/conversations -> 201
  GET  /v1/chat/conversations -> 200 contains created conversation
  POST /v1/chat/conversations/{id}/messages -> 201
  GET  /v1/chat/conversations/{id}/messages -> 200 contains created message

conversationId: 0dff0b88-e3f3-4017-aa44-ea1b11c4af95
userMessageId:  e9e924f8-72c1-4567-ae77-e5286349ca36
token:          MM_CHAT_PHASE11_RECONCILE_phase-11-reconcile-20260709T191936
```

Boundary: this reconciliation did not change runtime code. It only updated
tracking docs after existing evidence plus the missing API-client smoke closed
the parent checklist gap.

## 2026-07-09 — Phase 12 local browser import UI plan

Action: started Phase 12 local-first browser migration work and created the
detailed implementation plan before editing code.

Evidence:

```text
plan:     mm-chat/docs/architecture/phase-12-browser-import-ui-plan.md
tracking: mm-chat/docs/tracking/progress.md Phase 12 plan checkbox marked
```

Decision: keep the first implementation local-only: Next.js/React UI unchanged in
shape, Go import endpoints reached through `/mm-api`, and browser-local data is
never cleared by import or rollback.

Next: implement the exporter package builder and server import API client, then
add preview/commit/rollback UI in System Settings.

## 2026-07-09 — Phase 12 local implementation slice

Action: implemented the local browser-to-server import flow in the existing
System Settings data-management section.

Changed files owned by this slice:

```text
src/lib/data/browserImportPackage.ts
src/utils/opfs.ts
src/services/api/client/server/importApi.ts
src/services/api/client/local/importApi.ts
src/services/api/client/index.ts
src/services/api/client/types.ts
src/services/api/importService.ts
src/components/settings/BrowserDataMigrationPanel.tsx
src/components/settings/SystemSettings.tsx
src/i18n/locales/{en,zh,ja}/System.json
src/__tests__/browserImportPackage.test.ts
src/__tests__/importApi.test.ts
src/__tests__/settingsDataExport.test.ts
```

Evidence:

```text
corepack pnpm exec tsc --noEmit --pretty false
  passed

corepack pnpm vitest run \
  src/__tests__/browserImportPackage.test.ts \
  src/__tests__/importApi.test.ts \
  src/__tests__/settingsDataExport.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/messagesParity.test.ts
  passed: 5 files, 44 tests

corepack pnpm exec eslint \
  src/lib/data/browserImportPackage.ts \
  src/utils/opfs.ts \
  src/services/api/client/types.ts \
  src/services/api/client/index.ts \
  src/services/api/client/server/importApi.ts \
  src/services/api/client/local/importApi.ts \
  src/services/api/importService.ts \
  src/components/settings/BrowserDataMigrationPanel.tsx \
  src/components/settings/SystemSettings.tsx \
  src/__tests__/browserImportPackage.test.ts \
  src/__tests__/importApi.test.ts \
  src/__tests__/settingsDataExport.test.ts
  passed
```

Notes:

- The exporter builds only the backend-accepted ZIP layout: `manifest.json` and
  `files/sha256/{sha256}`.
- The UI keeps the existing JSON backup button and adds an explicit server
  preview/confirm/rollback panel.
- Preview and commit reuse the same in-memory ZIP blob to avoid idempotency hash
  drift.
- Browser-local IndexedDB and OPFS data are not deleted by import or rollback.

Next: run local browser smoke through `/mm-api`: create/keep local data, preview
the ZIP, confirm import, refresh server sessions, verify rendering, and test
batch rollback if unmodified.

## 2026-07-09 — Phase 12 review fix

Action: reviewed the Phase 12 local browser import UI slice against the Phase 8
Go import contract and tightened package generation so remote attachment URLs
with secret-like query, fragment, userinfo, path, or non-HTTP(S) scheme data are
not written into `manifest.json`.

Evidence:

```text
corepack pnpm vitest run \
  src/__tests__/browserImportPackage.test.ts \
  src/__tests__/importApi.test.ts \
  src/__tests__/settingsDataExport.test.ts
  passed: 3 files, 7 tests

corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/messagesParity.test.ts
  passed: 2 files, 37 tests

corepack pnpm exec eslint \
  src/lib/data/browserImportPackage.ts \
  src/utils/opfs.ts \
  src/services/api/client/types.ts \
  src/services/api/client/index.ts \
  src/services/api/client/server/importApi.ts \
  src/services/api/client/local/importApi.ts \
  src/services/api/importService.ts \
  src/components/settings/BrowserDataMigrationPanel.tsx \
  src/components/settings/SystemSettings.tsx \
  src/__tests__/browserImportPackage.test.ts \
  src/__tests__/importApi.test.ts \
  src/__tests__/settingsDataExport.test.ts
  passed

corepack pnpm exec tsc --noEmit --pretty false
  passed
```

Next: Go toolchain is not installed in this environment, so backend package tests
must be run where `go` is available before the final Phase 12 smoke.

## 2026-07-09 — Phase 12 local browser smoke

Action: ran a Windows Chrome browser smoke against the local server-mode app via
the same-origin `/mm-api` proxy. The smoke seeded browser-local IndexedDB and
OPFS data, opened Settings → System → Data Management, previewed the generated
server import package, confirmed import, verified server persistence, and rolled
the imported batch back.

Environment:

```text
browser URL: http://localhost:3000
Next dev:    127.0.0.1:3001
proxy:       127.0.0.1:3000 /mm-api -> 127.0.0.1:8080
backend:     127.0.0.1:8080 /ready -> {"status":"ready"}
```

Smoke artifact values:

```text
token:          MM_CHAT_IMPORT_OPFS_OK_1783581485178
local session:  phase-12-import-smoke-1783581485178
opfs url:       opfs://chat/phase-12-import-smoke/source.txt
source bytes:   MM_CHAT_IMPORT_OPFS_FILE_MM_CHAT_IMPORT_OPFS_OK_1783581485178
batchId:        f509eca0-c199-4290-b4b4-c77685f98ddb
conversationId: 15d7d1b6-c46f-4e06-86f0-bae1f8105d06
fileId:         392c4023-52cd-4ef8-84f4-426b21316161
file sha256:    a84aa30e338d2b7675a22fcf597663bfaec421dcfe04fbbcdac338917f608105
```

Observed UI results:

```text
preview summary: 1 conversation, 2 messages, 1 file, 61 B
commit banner:   imported batch f509eca0-c199-4290-b4b4-c77685f98ddb
rollback banner: rolled back
```

Observed proxy path:

```text
POST   /mm-api/v1/import/browser/preview
POST   /mm-api/v1/import/browser
GET    /mm-api/v1/chat/conversations
GET    /mm-api/v1/chat/conversations/{id}/messages
DELETE /mm-api/v1/import/browser/f509eca0-c199-4290-b4b4-c77685f98ddb
GET    /mm-api/v1/chat/conversations
```

Backend verification before rollback:

```text
GET /v1/chat/conversations
  found imported title: Phase 12 import smoke MM_CHAT_IMPORT_OPFS_OK_1783581485178
  messageCount: 2

GET /v1/chat/conversations/15d7d1b6-c46f-4e06-86f0-bae1f8105d06/messages
  user attachment: source.txt text/plain size=61 fileId=392c4023-52cd-4ef8-84f4-426b21316161
  assistant content: MM_CHAT_IMPORT_OPFS_OK_1783581485178

GET /v1/files/392c4023-52cd-4ef8-84f4-426b21316161/content?disposition=attachment
  bytes matched: MM_CHAT_IMPORT_OPFS_FILE_MM_CHAT_IMPORT_OPFS_OK_1783581485178
```

Rollback verification:

```text
GET /v1/import/browser/f509eca0-c199-4290-b4b4-c77685f98ddb
  {"status":"rolled_back"}

GET /v1/chat/conversations
  imported smoke conversation present: false
```

Cleanup:

```text
stopped: Next dev on 127.0.0.1:3001
stopped: temporary proxy on 127.0.0.1:3000
left running: Go Compose backend on 127.0.0.1:8080 for auditability
```

Next: Phase 12 local browser path is complete. The next decision is whether to
commit this slice now or proceed to VPS/deployment hardening planning.

## 2026-07-09 — Local Go toolchain installed

Action: installed a project-compatible Go toolchain for local backend development
without requiring sudo. Ubuntu apt only offered Go 1.18, while
`mm-chat/backend/go.mod` requires Go 1.22, so the official Go 1.22.12 linux/amd64
archive was installed under the user profile.

Environment change:

```text
installed: /home/mumu/.local/go
symlink:   /home/mumu/.local/bin/go -> /home/mumu/.local/go/bin/go
symlink:   /home/mumu/.local/bin/gofmt -> /home/mumu/.local/go/bin/gofmt
shell rc:  ~/.bashrc exports /home/mumu/.local/go/bin for interactive shells
```

Verification:

```text
go version
  go version go1.22.12 linux/amd64

cd mm-chat/backend && go test ./... && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/tracking/process.md \
  mm-chat/docs/tracking/progress.md
  passed

git diff --check -- <Phase 13.4 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are test token strings only
```

Next: host-side Go backend tests can now be run directly with
`cd mm-chat/backend && go test ./...`; Docker Go is no longer required for the
normal local backend verification loop.

## 2026-07-09 — Phase 12 pre-commit verification

Action: re-ran targeted frontend, backend, and formatting checks after the local
Go toolchain was installed. This keeps the Phase 12 browser-import UI slice
verifiable without Docker-only Go.

Verification:

```text
corepack pnpm exec prettier --check <phase-12 touched files>
  passed

corepack pnpm exec eslint <phase-12 touched source/tests>
  passed

corepack pnpm exec tsc --noEmit --pretty false
  passed

corepack pnpm vitest run \
  src/__tests__/browserImportPackage.test.ts \
  src/__tests__/importApi.test.ts \
  src/__tests__/settingsDataExport.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/messagesParity.test.ts
  passed: 5 files, 44 tests

cd mm-chat/backend && go test ./... && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/tracking/process.md \
  mm-chat/docs/tracking/progress.md
  passed

git diff --check -- <Phase 13.4 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are test token strings only
```

Spec sync: executable browser-import contracts already live in
`mm-chat/docs/contracts/browser-data-import.md` and
`mm-chat/docs/contracts/frontend-api-client.md`; no additional tracked spec file
is required for this commit.

## 2026-07-09 — Phase 13.1 request identity plumbing

Action: started Phase 13 with a documented auth/multi-user plan, then replaced
backend fixed-user repository fields with request-scoped identity plumbing. The
new middleware is optional: when a Bearer token is present it hashes the raw
token with SHA-256, resolves the existing Postgres/Redis session substrate, and
attaches browser-safe user identity to request context. Missing Bearer tokens
still fall back to the development user until enforced auth mode is added.

Files:

```text
mm-chat/docs/architecture/phase-13-auth-multi-user-plan.md
mm-chat/docs/contracts/auth-session-api.md
mm-chat/docs/contracts/README.md
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/auth/context_test.go
mm-chat/backend/internal/auth/types.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/files/types.go
mm-chat/backend/internal/files/service.go
mm-chat/backend/internal/files/repository_postgres.go
mm-chat/backend/internal/files/handler_test.go
mm-chat/backend/internal/browserimport/types.go
mm-chat/backend/internal/browserimport/repository_postgres.go
mm-chat/docs/tracking/progress.md
```

Behavior:

```text
Authorization: Bearer <raw-token>
  -> sha256(raw-token) lowercase hex
  -> auth.SessionResolver.ResolveByTokenHash
  -> auth.WithUser(request context, resolved session user)
  -> chat/files/import repositories use context user ID

missing Authorization
  -> development-user fallback remains for local mode
```

Verification:

```text
cd mm-chat/backend && go test ./... && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/tracking/process.md \
  mm-chat/docs/tracking/progress.md
  passed

git diff --check -- <Phase 13.4 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are test token strings only
```

Next: Phase 13.2 should add `/v1/me`, `POST /v1/auth/login`, and
`POST /v1/auth/logout`, then introduce an explicit auth mode so hosted requests
can fail closed when credentials are missing.

## 2026-07-09 — Phase 13.2 bootstrap auth endpoints

Action: added the first Go auth endpoint slice using a configured bootstrap owner
token. The backend now exposes login/logout/me routes without introducing a
password table or third-party OAuth. Login creates a new Postgres `sessions` row,
returns the raw bearer token once, and stores only `sha256(raw-session-token)` in
Postgres. Logout revokes the session row and clears Redis session-cache entries
when Redis is configured.

Files:

```text
mm-chat/.env.single-server.example
mm-chat/compose.single-server.yml
mm-chat/backend/.env.example
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/auth/handler.go
mm-chat/backend/internal/auth/handler_test.go
mm-chat/backend/internal/auth/service.go
mm-chat/backend/internal/auth/service_test.go
mm-chat/backend/internal/auth/session_repository_postgres.go
mm-chat/backend/internal/auth/token.go
mm-chat/backend/internal/auth/types.go
mm-chat/backend/internal/auth/uuid.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/docs/contracts/auth-session-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/deployment/redis-temporary-state.md
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

Runtime contract:

```text
POST /v1/auth/login {"token":"<AUTH_BOOTSTRAP_TOKEN>"}
  -> 200 {user, token, expiresAt}
  -> DB sessions.token_hash = sha256(token)

GET /v1/me
  -> Bearer session user when Authorization is valid
  -> development user while development fallback remains active

POST /v1/auth/logout
  -> requires Authorization: Bearer <token>
  -> revokes Postgres session and clears Redis cache hints
```

Environment keys:

```text
AUTH_BOOTSTRAP_TOKEN
AUTH_BOOTSTRAP_USER_ID
AUTH_BOOTSTRAP_DISPLAY_NAME
AUTH_SESSION_TTL
```

Review fixes:

```text
- Removed the Compose runtime fallback for AUTH_BOOTSTRAP_TOKEN; startup now
  requires the variable to be set explicitly.
- Exempted POST /v1/auth/login from optional session middleware so a stale or
  expired bearer token cannot block re-login.
- Aligned the frontend API contract with the backend `UNAUTHENTICATED` 401
  error code.
- Follow-up review agent reported no blocker/major/minor findings after these
  fixes.
```

Verification:

```text
cd mm-chat/backend && go test ./...
  passed

cd mm-chat/backend && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/contracts/auth-session-api.md \
  mm-chat/docs/contracts/frontend-api-client.md \
  mm-chat/docs/deployment/redis-temporary-state.md \
  mm-chat/docs/tracking/progress.md \
  mm-chat/docs/tracking/process.md \
  mm-chat/compose.single-server.yml
  passed

git diff --check -- <Phase 13.2 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are env placeholders, docs examples, and test
  fixtures only
```

Next: Phase 13.3 should add explicit auth mode configuration so hosted/server
mode can reject missing credentials instead of falling back to the development
user.

## 2026-07-09 — Phase 13.3 enforced hosted auth mode

Action: added explicit backend auth mode configuration so local development can
keep the development-user fallback while hosted/single-server deployments fail
closed when credentials are missing. `AUTH_MODE=development` preserves current
local smoke behavior. `AUTH_MODE=required` rejects unauthenticated protected
routes before they reach chat, file, import, or `/v1/me` handlers. Health,
readiness, version, and login stay public.

Files:

```text
mm-chat/.env.single-server.example
mm-chat/compose.single-server.yml
mm-chat/backend/.env.example
mm-chat/backend/internal/auth/session_resolver.go
mm-chat/backend/internal/auth/session_resolver_test.go
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/config/config_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/docs/contracts/auth-session-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

Runtime contract:

```text
AUTH_MODE=development
  -> missing Authorization keeps development-user fallback
  -> when a session resolver is installed, malformed/invalid Bearer returns
     401 UNAUTHENTICATED

AUTH_MODE=required
  -> /health, /ready, /v1/version, POST /v1/auth/login remain public
  -> protected routes without Authorization return 401 UNAUTHENTICATED
  -> protected routes with Bearer but no session resolver return 503 DATABASE_REQUIRED
  -> unknown non-empty AUTH_MODE values normalize to required
```

Review fixes:

```text
- Added a direct test proving development mode with an installed resolver still
  falls back when Authorization is missing.
- Narrowed the process note so malformed/invalid Bearer returning 401 is tied to
  cases where session middleware is installed.
- Follow-up review agent reported no blocker/major/minor findings.
```

Verification:

```text
cd mm-chat/backend && go test -count=1 ./... && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/contracts/auth-session-api.md \
  mm-chat/docs/contracts/frontend-api-client.md \
  mm-chat/docs/tracking/progress.md \
  mm-chat/docs/tracking/process.md \
  mm-chat/compose.single-server.yml
  passed

docker compose --env-file .env.single-server.example \
  -f compose.single-server.yml --profile app config
  passed; rendered AUTH_MODE=required

git diff --check -- <Phase 13.3 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are env placeholders, docs examples, and test
  fixtures only
```

Next: Phase 13.4 should verify two-user isolation across chat, files, browser
imports, and run cancellation.

## 2026-07-09 — Phase 13.4 two-user isolation tests

Action: added targeted two-user isolation coverage across the request-scoped
backend data paths. This slice is test-first hardening: existing repository and
service code already scopes by `auth.UserOrDevelopment(ctx)`, and the new tests
pin that behavior so future changes do not accidentally reintroduce shared
fixed-user access.

Files:

```text
mm-chat/backend/internal/auth/session_repository_postgres_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go
mm-chat/backend/internal/files/repository_postgres_test.go
mm-chat/backend/internal/files/handler_test.go
mm-chat/backend/internal/browserimport/repository_postgres_test.go
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

Coverage:

```text
Auth/session
  -> two distinct users can create independent sessions and resolve to their own identity

Chat
  -> user B cannot list/get/create/finalize messages in user A conversation
  -> user B cannot attach user A files to user B messages
  -> user B cannot cancel user A runId
  -> same conversation idempotency key can exist for different users

Files
  -> user B cannot read or delete user A file metadata
  -> user A object keys include users/{userId}/files/{fileId}
  -> service does not call object-store Get/Delete when metadata lookup fails

Browser import
  -> user B cannot read or roll back user A import batch
  -> same import idempotency key can create different batches for different users
  -> imported object keys include users/{userId}/files/{fileId}
  -> user A rollback does not delete user B objects or batch state
```

Review fixes:

```text
- Changed new integration tests to generate unique user IDs, tokens, and
  idempotency keys so repeated runs do not pollute shared Postgres state.
- Converted older auth Postgres fixture rows from fixed user/session/token/email
  values to generated unique values.
- Added post-rejected cross-user rollback assertions that user A batch status,
  object, conversation, messages, file row, and attachment row remain active.
- Added post-owner-rollback assertions that user B object, batch status,
  conversation, messages, file row, and attachment row remain active.
- Asserted two users can persist the same chat conversation idempotency key
  without one user's row masking the other.
- Ran the Phase 13.4 Postgres tests against a disposable postgres:16-alpine
  container instead of relying on skip-only default `go test`.
- Used `go test -p 1` for the multi-package disposable-Postgres verification so
  package-level migration setup runs sequentially against the shared fresh DB.
```

Verification:

```text
MM_CHAT_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:<ephemeral>/mm_chat_test?sslmode=disable \
  go test -p 1 -count=1 ./internal/auth ./internal/chat ./internal/files ./internal/browserimport \
  -run 'TestPostgresSessionRepositoryLookupSessionByTokenHash|TestPostgresSessionRepositoryCreatesTwoUserSessions|TestPostgresRepositoryEnforcesTwoUserIsolation|TestPostgresRepositoryEnforcesTwoUserFileIsolation|TestServiceDoesNotTouchObjectStoreWhenMetadataIsNotOwned|TestPostgresRepositoryEnforcesTwoUserImportIsolation'
  passed

MM_CHAT_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:<ephemeral>/mm_chat_test?sslmode=disable \
  go test -count=1 ./internal/browserimport -run TestPostgresRepositoryEnforcesTwoUserImportIsolation
  passed

MM_CHAT_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:<ephemeral>/mm_chat_test?sslmode=disable \
  go test -count=1 ./internal/chat -run TestPostgresRepositoryEnforcesTwoUserIsolation
  passed

cd mm-chat/backend && go test ./... && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/tracking/process.md \
  mm-chat/docs/tracking/progress.md
  passed

git diff --check -- <Phase 13.4 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are test token strings only

follow-up review agent
  no findings
```

Next: Phase 14 should start production hardening and observability unless a
browser-level auth/session UI slice is pulled forward first.

## 2026-07-09 — Phase 14.1 request IDs, structured logs, and readiness detail

Action: added the first production observability slice for the Go backend. This
keeps the frontend unchanged and only hardens the server boundary.

Files:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/cmd/api/main_test.go
mm-chat/backend/internal/health/handler.go
mm-chat/backend/internal/health/handler_test.go
mm-chat/backend/internal/httpserver/middleware.go
mm-chat/backend/internal/httpserver/observability.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/redisstate/client.go
mm-chat/backend/internal/redisstate/run_cancellation_test.go
mm-chat/backend/internal/storage/local.go
mm-chat/backend/internal/storage/local_test.go
mm-chat/backend/internal/storage/s3.go
mm-chat/backend/internal/storage/s3_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/release-rollback.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/persistence/runtime-wiring.md
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

Runtime contract:

```text
Every request:
  -> accepts a clean incoming X-Request-Id or generates one
  -> returns X-Request-Id in the response
  -> stores request_id in context
  -> emits a structured JSON http_request log with method, path, status, bytes,
     duration_ms, remote_addr, and user_agent
  -> does not log URL query strings, request bodies, Authorization headers, or
     provider secrets
  -> redacts URL userinfo, assignment values whose key names contain
     password/secret/token/api_key/authorization, and Bearer tokens before
     writing startup/lifecycle error strings

Panic recovery:
  -> emits structured http_panic log with request_id and panic type only
  -> returns 500 INTERNAL_ERROR without leaking panic details to the client or
     raw panic payload to logs

/ready:
  -> no configured checks: 200 {"status":"ready"}
  -> configured checks ready: 200 with checks.<name>.status="ready"
  -> any configured check fails: 503 status=not_ready and DEPENDENCY_NOT_READY
  -> raw DB/Redis/S3 errors are not exposed in the HTTP body
  -> readiness checks must not run migrations or create S3/MinIO buckets
```

Readiness wiring:

```text
database -> database.DB.CheckReady -> PingContext, only when DATABASE_URL enabled
redis    -> redisstate.Client.CheckReady -> PING, only when REDIS_URL enabled
storage  -> LocalStore.CheckReady or S3Store.CheckReady, when the store supports it
```

Review/scout notes addressed:

```text
- Added JSON slog default in cmd/api so request logs and lifecycle logs are
  structured in production.
- Added startup/lifecycle error-string redaction for URL userinfo,
  assignment values whose key names contain password/secret/token/api_key or
  authorization, and Bearer tokens.
- Addressed review findings by covering S3_SECRET_ACCESS_KEY-style names,
  token-only URL userinfo, malformed URL userinfo, Authorization Bearer header
  shapes, and a full-chain panic test that emits both http_panic and
  http_request logs with the same request_id.
- Kept ObjectStore as Put/Get/Delete only; storage readiness uses an optional
  CheckReady type assertion so file storage semantics do not widen.
- Documented /ready checks as additive detail so old health consumers can keep
  reading only status.
```

Verification:

```text
cd mm-chat/backend && go test ./... && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/contracts/frontend-api-client.md \
  mm-chat/docs/deployment/README.md \
  mm-chat/docs/deployment/release-rollback.md \
  mm-chat/docs/deployment/single-server-compose.md \
  mm-chat/docs/persistence/runtime-wiring.md \
  mm-chat/docs/tracking/process.md \
  mm-chat/docs/tracking/progress.md
  passed

git diff --check -- <Phase 14.1 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are docs references to secret env names only
```

Next: Phase 14.2 should add metrics visibility or run the documented backup and
restore drill.

## 2026-07-09 — Phase 14.2 backup and restore drill

Action: ran the documented Postgres plus MinIO backup/restore drill against the
local single-server Docker Compose stack without restoring into production DB or
production bucket.

Files changed:

```text
mm-chat/scripts/backup-minio.sh
mm-chat/docs/deployment/backup-restore.md
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

Script fix:

```text
backup-minio.sh now runs the minio-client backup container as the invoking host
UID/GID and sets HOME=/tmp. This prevents root-owned files in the host staging
directory and lets the cleanup trap remove `.staging-*` reliably.
```

Backup artifacts used for the drill:

```text
/tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834/postgres/postgres-20260709T100246Z.dump
/tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834/postgres/postgres-20260709T100246Z.dump.sha256
/tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834/minio/minio-20260709T100235Z.tar.gz
/tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834/minio/minio-20260709T100235Z.tar.gz.sha256
```

Commands executed, with only placeholder Compose interpolation values shown:

```bash
AUTH_BOOTSTRAP_TOKEN=drill-placeholder \
BACKUP_DIR=/tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834 \
./mm-chat/scripts/backup-minio.sh

AUTH_BOOTSTRAP_TOKEN=drill-placeholder \
BACKUP_DIR=/tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834 \
./mm-chat/scripts/backup-postgres.sh

(cd /tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834/postgres && \
  sha256sum -c postgres-20260709T100246Z.dump.sha256)

(cd /tmp/mm-chat-phase14-drill-rerun-20260709T100235Z-85834/minio && \
  sha256sum -c minio-20260709T100235Z.tar.gz.sha256)
```

Postgres restore drill:

```text
restore target: neo_chat_restore_drill_phase14
checksum: postgres-20260709T100246Z.dump: OK
schema_migrations: 1 initial_schema, 2 messages_run_id_index, 3 import_batches
users: 1
conversations: 7
messages: 19
files: 7
available file object keys sampled for MinIO stat checks: 5
cleanup: temporary drill database dropped
```

MinIO restore drill:

```text
restore target: temporary bucket neo-chat-files-restore-drill-phase14-100721
checksum: minio-20260709T100235Z.tar.gz: OK
local payload files: 5
restored object count: 5
Postgres files.object_key values checked with mc stat: 5
cleanup: temporary drill bucket removed
```

Documentation updates:

```text
- Corrected the schema_migrations drill query to use version/name.
- Added Postgres cleanup command for the temporary drill DB.
- Added MinIO temporary-bucket cleanup and local staging cleanup.
- Documented that app S3 credentials may not create drill buckets; use MinIO
  root/admin credentials for the temporary-bucket drill.
- Documented that PROJECT_NAME does not isolate bind-mounted data directories.
```

Verification:

```text
Postgres backup: created and checksum verified
MinIO backup: created and checksum verified
Postgres restore: restored into temporary DB and counted core tables
MinIO restore: restored into temporary bucket and stat-checked DB object keys
Cleanup: temporary DB removed; temporary bucket removed; failed root-owned
staging from the first attempt removed
```

Risk notes:

```text
- The drill used the running local single-server stack and restored only to
  temporary resources; production DB and production bucket were not overwritten.
- Backup artifacts remain in /tmp for short-term inspection and must not be
  committed.
- Metrics visibility, reverse proxy/TLS notes, and secret rotation notes remain
  open Phase 14 work.
```

Review agent:

```text
no findings
- UID/GID + HOME=/tmp MinIO backup fix is reasonable.
- Postgres drill uses a temporary DB and schema_migrations version/name.
- MinIO drill uses a temporary bucket with root/admin credentials and cleanup.
- progress.md has a matching process.md record and no real secrets were found.
```

Final verification before commit:

```text
corepack pnpm exec prettier --check <Phase 14.2 docs>
  passed

bash -n mm-chat/scripts/backup-minio.sh mm-chat/scripts/backup-postgres.sh
  passed

git diff --check -- <Phase 14.2 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are env-name references and placeholder token text
```

Next: commit the Phase 14.2 script and docs changes, then continue with the
remaining Phase 14 metrics, reverse proxy/TLS, and secret rotation items.

## 2026-07-09 — Phase 14.3 API metrics visibility

Action: added the first metrics visibility slice for the Go backend. The
endpoint is intentionally lightweight and uses Prometheus text output without
adding a monitoring stack or new Go dependency.

Files:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/backend/internal/httpserver/rate_limit.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/docs/architecture/phase-14-production-hardening-plan.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/persistence/runtime-wiring.md
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

Runtime contract:

```text
GET /metrics
  -> returns Prometheus text exposition
  -> public alongside /health, /ready, and /v1/version so localhost or
     allowlisted Prometheus can scrape in AUTH_MODE=required
  -> exempt from Redis HTTP rate limiting
  -> includes X-Content-Type-Options: nosniff
  -> rejects non-GET with 405 METHOD_NOT_ALLOWED JSON

HTTP metrics:
  -> mm_chat_http_requests_total{method,path,status}
  -> mm_chat_http_response_bytes_total{method,path,status}
  -> mm_chat_http_request_duration_seconds histogram
  -> dynamic route labels are bounded, for example /v1/files/{id}/content and
     /v1/chat/runs/{id}/cancel
  -> raw UUIDs, run IDs, object keys, query strings, bearer tokens, and
     provider parameters must not appear in labels

Dependency metrics:
  -> mm_chat_dependency_ready{dependency="database|redis|storage"}
  -> mirrors configured readiness checks; disabled dependencies are omitted
  -> storage represents local storage or MinIO/S3 readiness, depending on
     STORAGE_BACKEND

Postgres pool metrics:
  -> exposed when DATABASE_URL enables the database/sql pool
  -> includes max/open/in-use/idle connections and wait counters
```

Implementation notes:

```text
- Reused the existing response-writer wrapper shape so metrics preserve
  http.Flusher for SSE streaming.
- Inserted request metrics before request logging/recovery so 401, 429, 404,
  503, and recovered 500 responses are counted.
- Kept MinIO visibility through the backend storage readiness gauge; direct
  MinIO admin metrics are deferred until a dedicated Prometheus/Grafana stack is
  planned.
```

Verification:

```text
cd mm-chat/backend && go test ./internal/httpserver -run 'Metrics|RateLimit|AuthRequired|RequestLogging|Panic'
  passed

cd mm-chat/backend && go test ./...
  passed

cd mm-chat/backend && go vet ./...
  passed

corepack pnpm exec prettier --check \
  mm-chat/docs/architecture/phase-14-production-hardening-plan.md \
  mm-chat/docs/deployment/single-server-compose.md \
  mm-chat/docs/deployment/README.md \
  mm-chat/docs/persistence/runtime-wiring.md
  passed

local source-run smoke:
  MM_CHAT_ADDR=127.0.0.1:18080 MM_CHAT_VERSION=metrics-smoke \
  DATABASE_URL= REDIS_URL= STORAGE_BACKEND=local AUTH_MODE=development \
  PROVIDER_TYPE=none go run ./cmd/api

  curl -fsS http://127.0.0.1:18080/health
  curl -fsS http://127.0.0.1:18080/metrics

  observed:
    mm_chat_build_info{version="metrics-smoke",storage_backend="local"} 1
    mm_chat_http_requests_total{method="GET",path="/health",status="200"} 1
```

Next: run final formatting/diff/secret checks, send a review agent over the
metrics slice, then commit if clean.

Review finding addressed:

```text
Finding: unknown request paths and unknown HTTP methods could create unbounded
metrics labels or preserve secret-like path segments.

Fix:
- Unknown paths collapse to /__unknown__.
- Unknown HTTP methods collapse to OTHER.
- Known dynamic routes use explicit route-pattern labels.
- /v1/import/browser/preview is labeled distinctly, and browser import delete
  remains /v1/import/browser/{id}.

Regression tests:
- TestMetricsEndpointBoundsUnknownPathAndMethodLabels verifies a request to
  /missing/sk_live_secret_token?api_key=hidden is recorded only as
  method="OTHER", path="/__unknown__".
- TestNormalizeMetricPathBoundsKnownDynamicRoutes covers import preview,
  import id, unknown UUID paths, and secret-like unknown paths.
```

Second review finding addressed:

```text
Finding: escaped or doubled slash unknown paths could bypass the unknown-path
collapse and leak secret-like segments as labels, for example
/%2Fmissing/sk_live_secret_token.

Fix:
- knownMetricPath miss now returns /__unknown__ directly.
- Removed fallback UUID-only segment rewriting for unknown paths.
- Added regression coverage for //missing/sk_live_secret_token and
  /%2Fmissing/sk_live_secret_token.
- Runtime smoke now probes curl --path-as-is against the escaped-slash path and
  verifies metrics contain /__unknown__ but not the secret-like segment.
```

Final review:

```text
third review agent: no findings
```

Final verification before commit:

```text
cd mm-chat/backend && go test ./internal/httpserver -run 'Metrics|Flusher|Panic|RateLimit|AuthRequired' -count=1
  passed

cd mm-chat/backend && go test ./... -count=1 && go vet ./...
  passed

corepack pnpm exec prettier --check <Phase 14.3 docs>
  passed

runtime metrics smoke with escaped slash unknown path
  passed; metrics contained /__unknown__ and did not contain the secret-like path segment

git diff --check -- <Phase 14.3 target files>
  passed

targeted secret-pattern scan
  no real secrets found; hits are documentation terms and fake regression-test strings only
```

## 2026-07-09 — Phase 14.4/14.5 reverse proxy TLS and secret rotation notes

Action: added the remaining Phase 14 production hardening runbooks after
pushing the backup/restore and metrics commits.

Files:

```text
mm-chat/docs/architecture/phase-14-production-hardening-plan.md
mm-chat/docs/deployment/README.md
mm-chat/docs/deployment/reverse-proxy-tls.md
mm-chat/docs/deployment/secret-rotation.md
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/tracking/process.md
mm-chat/docs/tracking/progress.md
```

Reverse proxy/TLS contract:

```text
- Keep Go backend bound to 127.0.0.1:8080.
- Expose only the frontend origin on 80/443.
- Use same-origin /mm-api/* and strip the prefix before proxying to the Go API.
- Disable API proxy buffering so SSE chat streams render incrementally.
- Keep /metrics localhost-only or allowlisted.
- Never expose MinIO API/console, Postgres, or Redis publicly.
- Set proxy upload limits at or above MAX_UPLOAD_BYTES.
```

Secret rotation contract:

```text
- Rotate one secret class at a time.
- Record only secret names and verification evidence, never secret values.
- AUTH_BOOTSTRAP_TOKEN rotation does not revoke existing sessions.
- Bulk session revocation requires Postgres session revocation plus Redis
  session-cache cleanup or TTL wait.
- Existing Postgres volumes require ALTER ROLE before changing DATABASE_URL.
- Redis password rotation requires REDIS_PASSWORD and REDIS_URL to stay aligned.
- MinIO app credential rotation should create a new app user, update backend
  env, verify upload/download, then disable the old app user.
```

Verification before review:

```text
corepack pnpm exec prettier --write <Phase 14.4/14.5 docs>
  applied formatting to secret-rotation.md; other checked docs unchanged

markdown path sanity check
  no /api residuals, TODO, or FIXME markers in the Phase 14.4/14.5 docs

scout review
  identified the /api residual in single-server-compose.md, confirmed the new
  runbooks should be the source of truth, and called out same-origin/CORS plus
  proxy-layer rate-limit caveats
```

Next: run final checks, review agent over the runbooks, then commit and push
the Phase 14.4/14.5 docs.

Review findings addressed:

```text
- Added NEXT_PUBLIC_API_MODE=server beside NEXT_PUBLIC_API_BASE_URL=/mm-api in
  the reverse proxy/TLS verification section, and documented rollback to
  NEXT_PUBLIC_API_MODE=local.
- Added an explicit /mm-api/metrics allow/deny block before the summary
  /mm-api/ Nginx location in single-server-compose.md so copied snippets do not
  expose public metrics.
```

Final review:

```text
review agent: no findings
- /mm-api/metrics is blocked before the summary /mm-api/ proxy location.
- reverse-proxy-tls.md documents NEXT_PUBLIC_API_MODE=server and rollback to local mode.
- secret-rotation.md has no obvious destructive or misleading command pattern.
```

Final verification before commit:

```text
corepack pnpm exec prettier --check <Phase 14.4/14.5 docs>
  passed

git diff --check -- <Phase 14.4/14.5 target files>
  passed

doc sanity check
  no stale /api references, TODO, or FIXME markers in the Phase 14.4/14.5 docs

targeted secret-pattern scan
  no real secrets found; hits are documentation terms and placeholder examples only
```

## 2026-07-10 — Phase 15 accuracy-first RAG architecture research

Action: replaced the placeholder-only Phase 15 direction with an evidence-based
accuracy-first proposal before implementation. No runtime code, Compose service,
database schema, or external index was changed.

Created and updated:

```text
mm-chat/docs/architecture/phase-15-accuracy-first-rag-design.md
mm-chat/docs/architecture/README.md
mm-chat/docs/architecture/phase-11-plus-roadmap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Parallel research covered four independent areas plus local source inspection:

```text
1. Parsing and chunking:
   format/page-aware routing, Docling, MinerU, LlamaParse, PyMuPDF, OCR,
   canonical blocks, parent/child/window chunks, table/formula/image handling.

2. Retrieval:
   dense + BM25, learned sparse, RRF, ColBERT, cross-encoder reranking,
   contextual retrieval, late chunking, query decomposition, RAPTOR, GraphRAG.

3. Search engines:
   pgvector, Qdrant, OpenSearch/Elasticsearch, and Vespa capabilities,
   filtering, multi-vector support, single-server operation, and recovery.

4. Evaluation and security:
   golden qrels, Recall/nDCG/MRR, citation correctness, faithfulness,
   abstention, prompt injection, ACL filtering, deletion fencing, and A/B gates.
```

Decision proposal:

```text
Go          public identity/ACL/files/chat/citations/degradation boundary
Python      private parsing/indexing/query/reranking services
Postgres    authoritative metadata, ACL, versions, jobs, outbox, citations
MinIO       original files plus parser-native and canonical rebuild artifacts
Redis       non-authoritative wake-up, lease, rate-limit, and query cache
Qdrant      leading rebuildable search candidate, pending controlled bake-off
```

Accuracy pipeline:

```text
format/page-aware parse -> canonical blocks
-> parent/child/window chunks with provenance
-> dense + exact lexical + evaluated learned-sparse recall
-> RRF candidate fusion
-> evaluated ColBERT stage when beneficial
-> cross-encoder reranking
-> dynamic parent/sibling evidence expansion
-> source-version/page/span citations and strict-grounded answer policy
```

The design explicitly does not enable every advanced method globally.
Contextual retrieval, late chunking, ColBERT, query decomposition, RAPTOR, and
GraphRAG are query/corpus-specific candidates and require measured gains. This
avoids query drift, generated-summary contamination, duplicate evidence, and
untraceable citations while retaining the target capabilities.

Important current-state findings recorded during research and used to motivate
the design:

- the old browser RAG path flattens parser output to Markdown and loses page,
  bbox, table-cell, image, and formula provenance;
- the current splitter approximates tokens with `Intl.Segmenter`, applies a
  generic overlap, and emits metadata without parent/page/version/ACL fields;
- the current server-mode Go stack has no Phase 15 RAG service or search index;
- the Compose Postgres image is plain `postgres:16-alpine`, so pgvector is not
  already available despite earlier architectural discussion;
- post-retrieval client filtering is not a multi-user security boundary.

Primary evidence included official Anthropic Contextual Retrieval, Qwen3
Embedding/Reranker, BGE-M3, Qdrant, Docling, MinerU, ColBERTv2, RAPTOR,
Microsoft GraphRAG, BEIR, Ragas/ALCE, and OWASP sources. Public benchmark or
vendor claims remain hypotheses until reproduced on Neo Chat's golden corpus.

Independent review found no P0 blocker and required the following design
hardening before owner lock:

```text
- Split strict-grounded fail-closed behavior from optional chat enrichment.
- Add acl_revision/visibility_epoch deny fences and Go-side evidence reauthorization.
- Make Postgres outbox rescan authoritative after Redis loss.
- Add frozen-holdout, relative-regression, parser, judge-calibration, and
  explicit injection gates.
- Treat parser routes and Qdrant as evaluated candidates, not foregone winners.
- Add service identity, scoped MinIO capabilities, parser sandboxing, and
  external-parser data-governance controls.
- Return evidence/source-span IDs from Python and mint citations only in Go.
- Version derived artifacts and bind restore to tombstone/outbox watermarks.
- Version lexical analyzers and separate BM25 from exact phrase/key/path search.
```

These findings were incorporated into the architecture, roadmap, and Phase 15
checklist. The design also added source-aligned visual retrieval candidates,
structured table execution, claim/evidence verification, and a two-phase
versioned-alias publish protocol without turning them into unconditional global
features.

A second independent review found no P0 blocker and surfaced five P1 plus five
P2 consistency gaps. All were incorporated:

```text
- Strict answers now buffer privately, verify, recheck fences, atomically
  persist message+citations, and only then emit answer SSE.
- Index generation is separate from mutable corpus projection revision;
  aliases bind physical collections/configs and cache keys include both axes.
- Search-only restore is separate from coordinated full DR with
  timeline/LSN/WAL/outbox/MinIO/search watermarks and rebuild-on-gap behavior.
- DuckDB runs in a no-secret/no-network/no-host-files sandbox behind an AST
  SELECT allowlist and explicit escape/resource tests.
- External parser/model/VLM egress, domain training data, and all heavy jobs
  share governance, deletion lineage, admission control, and capacity gates.
- Signed Go-to-Python requests bind method/path/body/profile/mTLS identity and
  add replay, clock-skew, and key-rotation controls.
- Evaluation now defines paired statistics, slice power, relevant-drop, and
  dedicated visual/table/adaptation security and accuracy gates.
- Roadmap and progress now mirror the new candidate lanes and required outputs.
- Full-context wording now covers only candidate-retrieval truncation and
  requires long-context/citation evaluation.
```

Final regression review:

```text
independent review agent: no findings
- all five P1 and five P2 findings are closed;
- no new P0/P1/P2 issue was introduced by the corrections.
```

Next: owner lock, then implement the canonical schema, ACL invariants, and
frozen evaluation corpus before selecting a model or adding a container.

## 2026-07-10 — Phase 15 design translated to Simplified Chinese

Action: translated the complete accuracy-first RAG design in place so the owner
can review and lock the architecture without relying on an abbreviated chat
summary. Technical identifiers, field names, thresholds, state machines, code
blocks, and primary-reference URLs remain unchanged where translation would
alter their contract.

Files:

```text
mm-chat/docs/architecture/phase-15-accuracy-first-rag-design.md
mm-chat/docs/tracking/process.md
```

Completeness verification against the English source copy:

```text
headings:           23 -> 23
fence markers:      16 -> 16
bullet items:       69 -> 69
numbered items:     17 -> 17
reference URLs:     19 -> 19, identical set
inline identifiers: 45 -> 45, identical ordered list
Prettier:            passed
git diff --check:    passed
```

Independent translation review found one P1 and two P2 wording issues. The
translation now says that the Postgres fence rejects access to deleted data,
not the delete operation; preserves the original bake-off attribution rule;
and describes the MinerU route as high-compute rather than claiming accuracy in
advance. Regression review returned `no findings`.

Next: owner reads the Chinese design and decides whether to lock Phase 15.

## 2026-07-10 — Phase 15 owner-review implementation profile

Action: converted the architecture options into a concrete but unlocked Chinese
recommendation for owner review. The new profile does not modify runtime code or
silently lock a vendor; it records the recommended defaults and the conditions
that require a different choice.

Files:

```text
mm-chat/docs/architecture/phase-15-recommended-implementation-profile.md
mm-chat/docs/architecture/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Recommended profile:

```text
deterministic native parsers + MinerU Precise for complex visual documents
Jina Embeddings v4 hosted 2048/1024 Development/Validation candidate
Qdrant-first engine bake-off with versioned BM25 + exact-key/phrase manifests
query-class-aware weighted RRF and mandatory cross-encoder reranking
Go-owned strict-grounded answer, citation, ACL, and SSE boundaries
Postgres/MinIO authority with Redis and the winning search engine as projections
```

Source inspection confirmed that the original app does not pin an embedding
model. It sends raw chunk text to an external service implementing
`upsert-data`, `query-data`, and `delete-data`; that upstream chooses the
embedding. Text MIME types bypass document parsing, non-text files default to
MinerU, and LlamaParse remains a selectable alternative.

The draft explicitly records that the Jina API cannot be used as the old
`DEFAULT_RAG_BASE_URL`: the Python sidecar must call the approved active
embedding profile and store/query vectors in the winning search projection;
Jina is one candidate. It also records the data-egress, model/dimension
generation, storage sizing, evaluation, and the two-axis publish/rollback
contract inherited from the master design.

Independent review found no P0 and required six P1 plus four P2 corrections.
The draft now:

```text
- tunes only on Development/Validation and reserves Frozen Holdout for one
  preregistered final promotion;
- treats Jina v4 Hosted as an accuracy candidate until license, SLA, limits,
  latency, error rate, version pinning, data policy, and fallback gates pass;
- defines parser, reranker, provenance, citation, visual/table, statistical,
  and hosted-production gates;
- defaults all external data egress to deny and adds a Processor x Data Type
  consent/region/retention/deletion/training-use matrix;
- versions the lexical compute location, tokenizer, BM25 parameters,
  exact-match tiers, RRF formula/order/k/router/failure behavior, and digest;
- preserves index_generation_id plus corpus_projection_revision with outbox
  catch-up, active pointer, dual-write, and rollback readiness;
- labels ten unresolved assumptions as explicit owner-lock blockers.
```

Regression review then found two remaining P1 and one P2 gap. The Qdrant BM25
candidate now pins `model`, `modifier`, `k`, `b`, generated `avg_len`,
`language`, `tokenizer`, and lowercase behavior on both ingest and query. Every
RRF Query Class now has a complete enabled-lane order and equal-length weight
array, including Title/Summary, Exact, and Visual behavior. The service and
process wording now treats Jina and Qdrant as candidates rather than hard-coded
active services.

Final independent regression review returned `no findings`: P0/P1/P2 are all
zero, and the recommendation, tracking checklist, document index, and process
record are consistent.

Next: owner answers the blocking assumption table before any profile is merged
into the Phase 15 master design.

## 2026-07-10 — MinerU API availability confirmed

Owner confirmed that a MinerU API credential is available. The recommendation
now records credential availability without recording or requesting the secret
value. This closes only the credential-availability question; external document
egress remains default-deny until the consent, classification, region,
retention, deletion, training-use, and audit decision is approved.

Recommended handling when implementation starts:

```text
store the token only in the Python worker/server secret environment
never expose it through NEXT_PUBLIC_* or browser configuration
never write the token to docs, logs, process records, or test fixtures
use MinerU Precise only through the authenticated private ingestion worker
```

Next: confirm whether non-confidential knowledge documents may be sent to
MinerU, then record the approved Processor x Data Type policy before any live
API call.

## 2026-07-10 — MinerU data-egress scope consent approved

Owner confirmed that the current knowledge corpus contains no confidential
documents and approved sending it to MinerU for document parsing. The approval
is recorded narrowly as:

```text
approved processor: MinerU
approved corpus: current non-confidential knowledge corpus
approved purpose: document parsing
approved data: original file plus required page/table/image assets
```

This does not authorize LlamaParse, Jina or another embedding API, a hosted
reranker, or RAG evidence submission to an LLM provider. All remain
default-deny. LlamaParse stays Disabled and does not block Owner Lock unless it
is promoted into the candidate profile; Jina or another embedding API, a
hosted reranker, and LLM RAG Evidence are current independent Owner Lock
blockers. Any newly confidential corpus, changed classification, or expanded
Processor scope requires a new decision before egress.

Independent review separated this scope consent from Processor Governance.
MinerU Region, Retention, Deletion, Training-use, and Audit behavior are not yet
recorded; therefore the consent decision is complete, but the first real API
call and Promotion remain blocked until those controls are verified. This
preserves the owner's approval without misrepresenting vendor due diligence as
complete.

The implementation profile now also pins the recommended deterministic parser
stack and requires every parser to emit versioned Canonical IR plus native
artifacts instead of treating Markdown as the source of truth. MinerU tokens
remain server-only secrets and are never written to Git, logs, docs, fixtures,
or `NEXT_PUBLIC_*` variables.

The same review required an executable XML hardening profile and an explicit
Canonical IR locator union. The profile now pins entity/DTD/network/XInclude/
XSLT restrictions and distinguishes text, line, page, slide, sheet, and OOXML
locators instead of inventing BBox or Offset values for every format.

Final independent regression review returned `no findings`: P0/P1/P2 are all
zero across the recommendation, progress checklist, and process record.

Next: verify MinerU Processor Governance before the first real parse call, then
obtain a separate owner decision for sending non-confidential chunks and
queries to the Jina Embeddings API before any live embedding call.

## 2026-07-10 — Public-corpus egress approved for all processors

Owner confirmed that the current corpus is public and contains no private data,
then approved all external-processing paths listed in the Phase 15 profile:

```text
MinerU and LlamaParse: original files plus required page/table/image assets
Jina or another embedding API: chunks, queries, code, and approved image crops
Hosted reranker: queries, candidate children, and breadcrumbs
LLM provider: queries, final evidence spans, and approved image crops
```

This closes the Data Owner Scope Consent blockers for the current public
corpus. It does not waive Provider Governance, model/version pinning, SLA,
rate-limit, retention/deletion/training-use documentation, audit, reliability,
or accuracy gates. LlamaParse remains a disabled fallback despite having scope
consent. Any future non-public or private corpus returns to Default Deny until
separately approved.

Owner also requested that every remaining question be asked in one batch. The
next revision therefore consolidates only genuine product, workload, budget,
SLO, recovery, and evaluation-staffing decisions; vendor research and Bake-off
choices stay with the technical implementation rather than being delegated to
the owner.

Next: complete the one-shot Owner questionnaire, then record all answers before
locking the Phase 15 implementation profile.

The one-shot questionnaire is now persisted in
`docs/architecture/phase-15-recommended-implementation-profile.md` §11.3. It
contains ten grouped questions covering product failure behavior, future data
scope, representative corpus, workload, server capacity, credential
availability, Golden Set staffing, budget, SLO, and recovery/operations. Each
question includes a recommended default and a compact reply template.

Technical due diligence and Bake-off choices remain assigned to implementation:
Provider Governance, exact Model/API versions, fallback profiles, dimensions,
Reranker, Search Engine, Parser routes, Chunk/RRF/Top-K tuning, and security
fences are not delegated back to the owner unless they cannot meet the declared
constraints.

Independent final review returned `no findings`: all-processor Scope Consent,
LlamaParse Disabled status, remaining governance gates, the one-shot Owner
questionnaire, and tracking records are consistent; P0/P1/P2 are all zero.

## 2026-07-10 — Small-team Personal/Team Knowledge model confirmed

Owner confirmed that Jina API credentials are available and that the product is
for a small team with this knowledge model:

```text
each user -> private Personal Knowledge
team -> Shared Team Knowledge
team administrator -> manages Team Knowledge
```

No secret value was requested or recorded. Jina credential availability closes
only the credential question; exact Model/API version, License, SLA, Region,
Retention, Deletion, Training-use, and accuracy gates remain technical blockers.

Source inspection found that the current Go backend has request-scoped
`user_id` isolation but no Team, Membership, Knowledge Collection, Processing
Consent, or usable RBAC schema. `workspaceId` and `knowledgeCollectionId` are
currently metadata rather than authoritative entities, and the hosted login
path still resolves a single Bootstrap User. The design therefore adopts these
safe defaults without claiming implementation:

```text
Admin invite; public registration disabled; independent user sessions
Personal Owner manages only their Personal Knowledge
Team Admin manages Team members/documents/consent but cannot read Personal data
Team Member queries Team Knowledge but cannot mutate it
at least one Active Team Admin must remain
```

The earlier all-processor approval is now scoped to a concrete Bootstrap Public
Collection. It cannot authorize future Personal/Team uploads or user queries.
Those require Collection Data Consent plus Request User Query Consent, in
addition to a passed Processor Governance profile. Upload alone is not consent.

The Phase 15 architecture, recommendation, Phase 13 forward-compatibility note,
progress checklist, and new Knowledge ACL contract record this superseding
decision. Existing Phase 13 auth/file behavior remains the implemented baseline
until the explicit Phase 15 schema and APIs are delivered.

Next: retain the unanswered entries in the already-issued one-shot Owner
questionnaire; no new Owner question is introduced by this ACL decision.

## 2026-07-10 — Knowledge ACL contract security closure

Independent review found five P1 and two P2 gaps in the first Personal/Team ACL
contract. The design was corrected before Owner Lock:

```text
invite token -> delivered only to invited mailbox; Admin receives metadata only
member auth -> Argon2id credential + email/password re-login + mailbox recovery
external calls -> operation-specific Governance/Collection/Query consent matrix
revisions -> Collection Processing, User Query, Governance Head/Profile split
documents -> stable logical Document + immutable Version rows/current pointer
files -> FK RESTRICT + shared FOR UPDATE serialization for bind/delete
search -> one canonical Payload and Mutation-to-Fence contract
```

The previous process wording that future uploads and queries always require all
three Governance + Collection + Query conditions is superseded. Parse/Passage
Embedding requires Governance + Collection Consent; Query Embedding requires
Governance + the requesting User's Query Consent; Rerank/Answer/Evidence
requires Governance + Query Consent + every selected Collection's Consent.

Consent changes advance their own Processing/Query/Governance Revisions and do
not fake ACL/Visibility changes or force a Search Point rewrite. Content access
tightening and deletion remain the operations that advance visibility fences
and write Search tombstones. Processor-derived cleanup, when contractually
required, is explicit Outbox work and disables the affected lane until rebuilt.

The adjacent Phase 6 File Contract was also corrected to reflect implemented
Phase 13 request-scoped ownership rather than the obsolete fixed development
user. Runtime Team/Knowledge/Consent implementation remains unchecked in the
progress plan.

Next: complete regression review of the corrected contract, then retain only
the unanswered items from the existing Owner questionnaire.

Final independent regression returned `no findings`: P0/P1/P2 are all zero.
Auth/recovery, Invite delivery, Personal/Team ACLs, operation-specific Consent,
Governance Heads, independent Revisions, logical Document/Version identity,
File bind/delete serialization, Search Payload, Outbox, tests, and tracking are
consistent. No runtime Team/RAG implementation item was marked complete.

Next: keep the remaining fields from the existing one-shot Owner questionnaire
open; this decision introduces no new Owner questions.

## 2026-07-10 — Phase 15.1A schema foundation verified

The first implementation slice from
`docs/architecture/phase-15-1-knowledge-control-plane-plan.md` added migration
`004_phase15_identity_knowledge_acl` and a static migration contract test. The
schema establishes account status and credentials, recovery tokens, Teams,
Memberships, Invites, Personal/Team Collections, logical Documents and
immutable Versions, query-consent revisions, immutable Governance Profiles and
Heads, Processing Consent, and the Knowledge Outbox.

Review tightened the opening draft before runtime validation:

```text
outbox watermark -> BIGSERIAL id + independent UUID event_id
email identity -> unique lower(email)
new document -> processing with no current version
serving document -> active with same-document Active-version composite FKs
failed processing -> explicit Document Version state
active governance head -> exact Approved Profile/Revision composite FK
consent -> collection/query subject and purpose matrix enforced in Postgres
file/governance binding -> composite or RESTRICT foreign keys
```

Validation used an automatically removed `postgres:16-alpine` container with a
random loopback port, separate from the running Compose database and its data
directory. The Go migration runner applied `001` through `004`; a transaction
then proved positive inserts and rejection of case-insensitive duplicate email,
invalid account/scope/lifecycle states, cross-document current versions,
bound-file deletion, mismatched Governance revisions, invalid Consent purposes,
and non-object Outbox payloads. Two Outbox inserts also proved advancing
sequence allocation IDs. Sequence allocation is not transaction commit order:
consumers may advance a durable high-watermark only across a contiguous applied
prefix, must rescan claimable rows below it, and deduplicate replay by
`event_id`. A one-step Down removed only `004`; catalog assertions found no
Phase 15 table, index, sequence, column, or constraint residue and confirmed
that migrations `001` through `003` remained.

`go test ./internal/migration` passed. The first sandboxed `go test ./...` run
was blocked only because `httptest` could not bind a loopback port; rerunning
outside that network restriction passed every Go package. No Provider secret or
real user data was used.

The independent xhigh schema/rollback/security review found and fixed two P1
and two P2 classes before commit:

```text
P1: Active Document could point to a non-Active Version
P1: Active Governance Head could point to a Candidate/Retired Profile
P2: composite-FK static assertions allowed extra columns and could false-pass
P2: identity token, lifecycle, Outbox, and allocation-order wording was thin
```

Generated status columns plus exact composite FKs now bind an Active Document
to an Active current Version and an Active Governance Head to an Approved
Profile/Revision. Static tests now require exact FK mappings, strip comments,
and cover additional lifecycle/token/Outbox constraints. The review ended with
`P0/P1/P2 = 0` and `no findings`.

Because the review changed executable DDL, the complete isolated PostgreSQL 16
Up/constraint/Down/zero-residue drill was rerun from a new empty container and
passed again. `gofmt`, `git diff --check`, `go test ./internal/migration`, and
the final `go test ./...` all passed after those fixes.

Next: commit Phase 15.1A with an explicit path allowlist, then start identity
services in 15.1B.

## 2026-07-10 — Phase 15.1B identity services verified

Phase 15.1B replaced the public Bootstrap Token exchange with independent
Email/Password identities while preserving the Bearer Session contract. The Go
backend now provides Login, Invite Acceptance, Recovery Request/Completion,
logout, `/me`, and revoke-all session flows. The existing Next.js/React UI was
not changed in this slice.

The implementation pins these authority and secret boundaries:

```text
password hash -> Argon2id PHC v=19, m=65536, t=3, p=2, bounded to 2 jobs
email -> lower(trim(email)), one mailbox, at most 254 bytes
password -> at least 15 UTF-8 runes, at most 256 bytes, no trim
session/invite/recovery token -> 32 random bytes as lowercase hex
persistence -> SHA-256 token hashes only
Bearer authorization -> Postgres rechecked on every request
Redis -> non-authoritative snapshot/revocation hints and rate limits only
```

Repository transactions now fence Login against `credential_revision`, consume
Invites and Recovery Tokens once, prevent Invite credential overwrite, revoke
all Sessions after Recovery, reject disabled/deleted accounts, and allow only a
single operator-side bootstrap identity. The `mm-chat-admin` command reads the
initial password from one stdin line; runtime Compose no longer receives
`AUTH_BOOTSTRAP_TOKEN`.

Public Identity routes use strict 8 KiB JSON bodies, reject unknown and
caller-supplied identity fields, and apply independent IP plus hashed
account/token limits. Redis failure falls back to a bounded process-local
limiter. Recovery delivery uses one bounded SMTP worker, STARTTLS with TLS 1.2
or newer, and never places a raw token in a URL, log, metric, response, command
argument, environment value, or Git artifact. A syntactically valid Recovery
Request keeps the same `202` response for known and unknown accounts and for
delivery failure/overload.

The security gate found `GO-2026-5004` in the previous
`github.com/jackc/pgx/v5 v5.6.0`; the backend explicitly used the affected
simple-protocol path. The dependency was upgraded to `pgx/v5 v5.9.2`, requiring
the backend and Docker builder baseline to move from Go 1.22 to Go 1.25.
`govulncheck` then reported zero called vulnerabilities.

Parallel xhigh Workers synchronized deployment, secret-rotation, Redis,
contract, architecture, and environment documentation. A separate xhigh Review
Agent found one P1: loopback proxy requests trusted the first
`X-Forwarded-For` address while Nginx preserved client-supplied prefixes. The
backend now selects the rightmost valid proxy address, Nginx replaces the header
with `$remote_addr`, and a spoof-prefix regression test covers the boundary.
Independent re-review ended with `P0/P1/P2 = 0`.

Verification completed after the review fix:

```text
gofmt + go vet ./...                                      passed
go test -race ./internal/auth ./internal/ratelimit        passed
go test ./...                                             passed
govulncheck ./...                                         0 called vulnerabilities
Docker Compose config + Go 1.25 backend image build       passed
backend image API/migrate/admin binary check              passed
PostgreSQL 16 migration 001 -> 004 and identity drill     passed
Prettier Markdown + scoped git diff --check               passed
diff-only quality/security/change gates                   passed
```

The PostgreSQL drill reproduced first-only bootstrap, Invite Acceptance,
Recovery rotation and concurrent one-time consumption, credential revision
fencing, disabled-account denial, required-mode Login/`/me`/revoke-all, and
secret-free API logs. Temporary containers were removed automatically.

The SMTP queue remains intentionally bounded and process-local: an accepted
request is not a durable delivery guarantee. A transactional Mail Outbox is a
future reliability enhancement, not part of the locked Phase 15.1B contract.

Next: commit Phase 15.1B with the explicit task allowlist, then begin Phase
15.1C Team services; do not start Python RAG processing before 15.1E passes.

## 2026-07-10 — Phase 15.1C Team services design locked

The Team/Membership/Invite slice now has an executable design in
`docs/architecture/phase-15-1c-team-services-plan.md`. It keeps the existing
Next.js/React UI unchanged and makes Postgres Membership rows, never the global
`CurrentUser.role`, authoritative for Team permissions.

The first independent xhigh review found three P1 and one P2 classes. The plan
was corrected before implementation:

```text
account disable -> User fence, then UUID-ordered Team locks and membership reread
invite acceptance -> existing/new identity branches plus credential revision fence
invite delivery -> Invite + AES-256-GCM Mail Outbox in one transaction; no RAM queue
cursor -> keyId + endpoint/user/team/filter/sort HMAC binding and rotation key ring
```

Membership-effective writes now share `User -> Team -> Membership/Invite ->
Revision/Outbox` ordering. Recovery uses the same User-before-Credential fence.
The durable Invite worker leases/reclaims rows, retries with capped backoff, and
orders SMTP delivery against revocation by locking only its Mail Outbox row.
Acceptance requires the corresponding delivery state to be `sent`.

The second independent xhigh review returned `P0=0`, `P1=0`, and `P2=0`.
Prettier and scoped `git diff --check` passed for the design set. No Team runtime
or migration checkbox was marked complete.

Next: synchronize the Auth, Knowledge ACL, frontend client, and RAG profile
contracts; then implement migration `005` and the Team vertical slice with
disjoint xhigh Workers plus an independent Review Agent.

## 2026-07-11 — Phase 15.1C Team services implemented and verified

Phase 15.1C now provides the authoritative Go/Postgres Team control plane while
leaving the existing Next.js/React UI unchanged. Migration `005` adds scoped
idempotency, pending-Invite uniqueness, the Membership User `RESTRICT` fence,
and an AES-256-GCM identity Mail Outbox. The new `internal/teams` vertical
slice implements Team CRUD, Membership roles and revisions, last-usable-Admin
protection, hash-only Invites, authenticated cursors, durable delivery, and
strict HTTP DTO/error mapping.

Invite delivery is closed until Postgres, SMTP, Mail keys, acceptance URL, and
the worker's first successful store probe are ready. Tokens persist only as a
SHA-256 hash plus authenticated ciphertext. Email links carry the raw Token in
`#token=...`, never in the HTTP path or query. The worker uses leased
`SKIP LOCKED` claims, bounded retry, stable Message-ID, row-lock ordering
against revoke/accept, and exits after three consecutive store failures so the
API can shut down instead of silently losing delivery.

Runtime wiring registers protected `/v1/teams*` routes, bounded log/metric
labels, required-mode HTTPS, loopback-only development HTTP, key rotation, and
operator-only `admin disable-account`. Cursor/Mail key material must be
distinct from each other and from database, Redis, SMTP, provider, and object
storage secrets. Published example keys are rejected in required mode.

The first independent xhigh code review found and fixed P1/P2 issues in Token
URL placement, key/config fail-closed behavior, worker readiness/lifecycle,
lock and disclosure ordering, canonical mailbox reuse, strict body/query
parsing, and bounded observability. A real PostgreSQL run then exposed one
stale query-token E2E helper; it was changed to the fragment contract and
independently re-reviewed. Final review result: `P0=0`, `P1=0`, `P2=0`.

Verification evidence:

```text
gofmt + go vet ./...                                      passed
go test ./...                                             passed
go test -race auth/teams/httpserver/api/admin             passed
PostgreSQL 16 migration 001 -> 005 and 005 replay         passed
PostgreSQL 16 auth + Team/Invite/Mail worker race tests   passed
Invite pending -> sent -> accept -> replay rejection      passed
isolated PostgreSQL test schema residue                   0
govulncheck ./...                                         0 called vulnerabilities
Docker Compose app/ops config + Go 1.25 image build       passed
independent xhigh review and post-PG re-review             P0/P1/P2 = 0
```

The generic security scanner's three High matches were inspected: all were
synthetic Token/API-key literals in tests, including two pre-existing provider
tests; no production credential was found. The quality scanner passed with
non-blocking file-length/line-length warnings. The full-repository change
analyzer remained noisy because hundreds of unrelated owner paths are dirty,
so promotion used scoped `mm-chat/` checks and an explicit commit allowlist.
The temporary PostgreSQL 16 container and local verification artifacts were
removed after the clean replay.

Next: commit this Phase 15.1C slice, then start Phase 15.1D
Collection/Document/Consent APIs. Do not start Python RAG processing before the
remaining Go/Postgres control-plane gates pass.

## 2026-07-11 — Phase 15.1D Knowledge service design locked

The Collection/Document/Consent implementation contract is now persisted in
`docs/architecture/phase-15-1d-collection-document-consent-plan.md`. This slice
keeps the existing Next.js/React UI unchanged and makes Go/Postgres authoritative
for Personal/Team ACLs, immutable source Versions, Consent, Governance, Jobs,
and Outbox revisions.

Source inspection found that the current File delete path performs an owner-
scoped `UPDATE` without `FOR UPDATE` or a Knowledge binding check. Migration
`004` has the core Knowledge tables but lacks frontend display metadata,
operation idempotency columns, and a durable Processing Job table. The plan
therefore requires reversible migration `006` and one shared File-row locking
protocol before any Document API can be promoted.

The design fixes authorization disclosure order, immutable Collection scope,
Version replacement semantics, Consent purpose matrices, wall-clock expiry,
operator-only Governance, transaction lock order, safe Outbox payloads, and
two-user/two-team concurrency gates. No runtime checkbox was marked complete,
and no Python/vector processing is allowed yet.

Next: synchronize `knowledge-acl-api.md` with the 15.1D DTO/idempotency/Job
contract, implement migration `006`, and run an independent review before
starting repositories.

## 2026-07-11 — Phase 15.1D-1 contract and migration implemented

The public Knowledge contract now defines Collection/Document/Consent DTOs,
authenticated paging, strict error/disclosure behavior, mutation idempotency,
and the minimal future `knowledgeApi` adapter boundary. The existing
Next.js/React Knowledge UI and local store were not changed.

Reversible migration `006_phase15_knowledge_services` adds bounded Collection
display fields, actor-scoped idempotency plus canonical request hashes,
independent Document visibility epochs, one-nonterminal-Version fencing, exact
Consent lookup support, and `knowledge_processing_jobs`. Jobs are split by
`parse|passage_embedding|purge` stage and pin the exact Collection, Document,
Version, File, Governance, Consent, and revision snapshot with composite foreign
keys. This prevents a Collection Consent from authorizing another Collection's
Document or a different source File.

Verification completed against an automatically removed PostgreSQL 16
container:

```text
001 -> 006 Up, 006 Down, 006 Up                         passed
legacy 004/005 Collection and Document compatibility    passed
Collection/Version idempotency and nonterminal conflict passed
Governance + Consent + Processing Job insert            passed
complete migration package PostgreSQL replay             passed
go vet ./...                                             passed
go test ./...                                            passed
go test -race ./internal/migration                       passed
```

No Processor credential, Provider secret, object key, or source content was
added to public DTOs or Outbox payload contracts. Runtime repositories/routes
remain unchecked.

Next: implement 15.1D-2 Personal/Team Collection service with the fixed
Session -> Team/Membership -> Collection authorization and disclosure order.

## 2026-07-11 — Phase 15.1D-2 Collection service implemented

The Go backend now registers protected Personal/Team Collection CRUD under
`/v1/knowledge/collections`. The new `internal/knowledge` vertical slice owns
strict DTO validation, canonical create hashes, HMAC cursor binding, Service
rules, Postgres authorization/locking, revision changes, and transactional
Knowledge Outbox events. The existing Next.js/React Knowledge UI remains
unchanged.

Personal Collections resolve only through the Session owner. Team Collections
require an active Membership for reads and an active Admin Membership for
writes. Unknown, cross-user, cross-Team, removed-Membership, and deleted targets
share `404 COLLECTION_NOT_FOUND`; only a visible active Member receives
`403 TEAM_ADMIN_REQUIRED`. List queries never expose totals and bind cursors to
the request User plus normalized scope/Team filters.

Create retries persist a canonical request hash under actor-scoped Postgres
uniqueness. Same-key/same-payload requests return one Collection; changed
payloads return `409 IDEMPOTENCY_CONFLICT`. Metadata no-ops emit no event and do
not change ACL fences. Delete locks Team/Collection and dependent
Document/Version/Job rows in deterministic order, cancels active Jobs,
tombstones dependents, increments Collection ACL/Visibility exactly once, and
writes `knowledge.collection.tombstoned` atomically. Repeated authorized delete
is `204` without a second event.

Verification evidence:

```text
go vet ./...                                             passed
go test ./...                                            passed
go test -race ./internal/knowledge ./internal/httpserver passed
PostgreSQL 16 Personal/Team/Admin/Member/outsider ACL    passed
PostgreSQL idempotency replay/conflict/concurrency       passed
PostgreSQL update no-op and delete revision fencing      passed
synthetic Outbox failure transaction rollback            passed
protected routing and bounded metric labels              passed
```

The real PostgreSQL test runs in a fresh schema inside an automatically removed
PostgreSQL 16 container. No Provider secret, source content, or object-store key
is written to Collection responses or Outbox payloads.

Next: implement 15.1D-3 logical Document/Version lifecycle and the shared
File-row `FOR UPDATE` binding/deletion protocol.

## 2026-07-11 — Phase 15.1D-3A File deletion fence implemented

Direct File deletion now starts a Postgres transaction, locks the caller-owned
available `files` row with `FOR UPDATE`, and checks all live Knowledge Version
states before changing metadata. A binding in
`uploaded|processing|failed|active|purging` returns `409 FILE_IN_USE`; unknown,
deleted, unavailable, and cross-user Files retain the same `404` disclosure.

Successful metadata deletion and `file.object.delete.requested` Outbox insertion
commit atomically. The existing synchronous ObjectStore deletion remains the
fast path; the durable File-ID-only event is the retry/reconciliation source and
does not expose bucket or object keys.

A real PostgreSQL 16 race test held the File lock in a simulated Document bind,
started concurrent direct deletion, inserted the Version, and committed. The
waiting delete then observed the binding and failed closed; after Document and
Version tombstoning, deletion succeeded and emitted exactly one cleanup event.

```text
go test ./internal/files                                 passed
PostgreSQL bind/delete row-lock serialization            passed
synthetic cleanup-Outbox failure rollback                passed
two-user File disclosure behavior                        passed
FILE_IN_USE HTTP mapping                                 passed
```

Next: complete 15.1D-3B Document/Version routes, Parse Job admission,
authorized content reads, reprocess, and tombstone transactions.

## 2026-07-11 — Phase 15.1D-3B first Document binding implemented

The internal Knowledge Service/Repository now accepts a caller-owned
`purpose=knowledge` File and creates the first logical Document, immutable
Source Version, Parse Processing Job, and
`knowledge.document.version.requested` Outbox event in one transaction.

The transaction first authorizes Personal owner or active Team Admin, locks the
Collection, then locks the same File row used by direct deletion. Admission
requires a current granted, unexpired Collection Consent whose purpose and MIME
data type include Parse, plus an Active Governance Head pinned to the exact
Approved Profile/Revision. Public callers cannot provide Processor, Endpoint,
Profile, Governance revision, or Job stage; the server selects the `mineru`
Processor alias.

Actor/Collection-scoped idempotency returns the original Document for a
same-key/same-File replay and creates only one Version, Job, and Outbox event.
The real PostgreSQL 16 test proves the complete authority and persistence chain.
HTTP routes and content streaming remain deliberately unregistered until D3C.

```text
go test ./internal/knowledge                             passed
PostgreSQL 16 File lock + Consent/Governance admission   passed
Document + Version + Parse Job + Outbox atomic insert    passed
same-key replay produces one Job and one Outbox event    passed
```

Next: expose strict Document list/get/create routes, then add authorized source
content, replacement, reprocess, and tombstone deletion.

## 2026-07-11 — Phase 15.1D-3C first-bind HTTP admission exposed

`POST /v1/knowledge/collections/{collectionId}/documents` now exposes the
verified first-bind transaction through the protected Knowledge handler. The
route accepts only strict `{ fileId, idempotencyKey }` JSON, rejects query and
identity/fence hints, maps hidden Files to `404 FILE_NOT_FOUND`, missing Parse
authority to `403 PROCESSING_CONSENT_REQUIRED`, and returns the Processing
Document plus pending immutable Version. Dynamic metrics use the bounded
`/v1/knowledge/collections/{collectionId}/documents` label.

Document list/get/content, replacement, reprocess, and delete remain closed.

## 2026-07-11 — Phase 15.1D-3D Document reads exposed

The protected Knowledge API now exposes cursor-paged Document metadata,
single-Document metadata, and source-content reads. Personal reads require the
current owner; Team reads require a current active Membership. Unknown,
deleted, cross-user, cross-Team, and removed-Membership Documents collapse to
`404` without exposing Collection, File, bucket, or object-key details.

Content serving is fail-closed: only an `active` logical Document's exact
`current_version_id` may resolve bytes, and that immutable Version must also be
`active`. Uploaded, processing, failed, stale, or newer pending Versions are
never served. Authorization and active-version resolution happen in Postgres
before ObjectStore access; Team access does not relax owner-only `/v1/files/*`.
The handler streams safe metadata with bounded route labels and independent
auth enforcement for `/v1/knowledge/documents/*`.

```text
go test ./...                                             passed
go vet ./...                                              passed
PostgreSQL 16 knowledge package under -race               passed
Owner/Member/outsider/removed-member ACL matrix           passed
Active pointer wins over newer failed Pending Version     passed
ObjectStore is not called before authorization            passed
```

Independent review raised two high-severity hypotheses. Same-origin active
content was hardened to `Content-Disposition: attachment` plus
`Cache-Control: private, no-store`, closing executable HTML/SVG preview risk.
The recommendation to hide Pending metadata was not applied: the locked public
DTO intentionally exposes processing/failed status to authorized Collection
readers, while “Active-only” applies strictly to source-byte serving. No
Pending object key or bytes are resolved by metadata routes.

Next: implement immutable replacement Version admission, then reprocess and
logical Document tombstone transactions.

## 2026-07-11 — Phase 15.1D-3E replacement Version admission implemented

`POST /v1/knowledge/documents/{documentId}/versions` now admits a new immutable
Source Version without moving the serving pointer. Go rechecks current
Personal-owner or Team-admin authority, locks Collection then active Document,
and locks the current/new File rows in sorted UUID order. The new File must be
caller-owned, available, non-deleted, and marked `purpose=knowledge`.

The transaction allocates `source_version = max + 1`, resolves the current
MinerU Parse Consent and approved Governance Head/Profile, then inserts the
uploaded Version, `operation=replace` Processing Job, and version-requested
Outbox event atomically. `current_version_id` remains unchanged, so readers
continue receiving the old Active bytes. A second nonterminal replacement gets
`409 DOCUMENT_PROCESSING`; same-key replay returns the original Version, while
changed payload returns `409 IDEMPOTENCY_CONFLICT`.

Processor admission locks the server-selected Active Governance Head and
Approved Profile before the exact current Consent. Missing or incompatible
processor authority returns `503 KNOWLEDGE_PROCESSOR_UNAVAILABLE`; missing,
revoked, expired, or MIME-incompatible Collection Consent remains
`403 PROCESSING_CONSENT_REQUIRED`. The Outbox payload carries immutable
Governance, Consent, Collection, and Document revision fences but never an
object key or credential.

```text
go test ./internal/knowledge ./internal/httpserver       passed
PostgreSQL 16 replacement test under -race               passed
two concurrent replacements: one winner, one 409         passed
same-key concurrent replay: one Version/Job/Event          passed
Active content pointer unchanged before publish          passed
replacement Job/Outbox/idempotency assertions            passed
```

Next: implement same-Version reprocess admission, followed by logical Document
tombstone deletion.

## 2026-07-11 — Phase 15.1D-3F same-Version reprocess implemented

`POST /v1/knowledge/documents/{documentId}/reprocess` now accepts only a strict
`{ idempotencyKey }` body. It rechecks current Personal-owner or Team-admin
authority, locks Collection then Document and source File, and resolves current
MinerU Governance plus Parse Consent before creating work.

Target selection is deterministic: the newest failed Version whose
`source_version` is newer than the Active current Version is reopened as
`uploaded`; otherwise the exact Active `current_version_id` is
reprocessed without changing its status. The transaction creates no Source
Version and never changes the serving pointer or Active artifacts. It inserts
one `operation=reprocess` Parse Job linked through `caused_by_job_id`, plus a
`knowledge.document.reprocess.requested` Outbox event carrying Governance,
Consent, ACL, visibility, and processing fences.

Same-key concurrent requests return the same Job-backed logical result and
write one Job/Event. A different request while any Version or Job is
nonterminal returns `409 DOCUMENT_PROCESSING`. Replacement admission now uses
the same Version-or-Job processing gate, preventing replace/reprocess overlap.
Initial bind, replacement, and reprocess Job idempotency scopes include the
authenticated actor ID, preventing two Team Admins who reuse the same client
key from replaying or conflicting with each other's operation.

```text
go test ./internal/knowledge ./internal/httpserver       passed
PostgreSQL 16 reprocess tests under -race                passed
same-key concurrent reprocess: one Job/Event             passed
Active and failed-Pending target selection               passed
no new Source Version / current pointer unchanged        passed
ACL, strict payload, caused-by, and Outbox assertions    passed
```

Next: implement logical Document tombstone deletion and cancellation/purge
Outbox work.

## 2026-07-11 — Phase 15.1D-3G Document tombstone deletion implemented

`DELETE /v1/knowledge/documents/{documentId}` now performs a metadata-only,
transactional deletion. Go rechecks current Personal-owner or Team-admin
authority, locks Collection then Document, locks all Version and cancelable Job
rows in deterministic ID order, and advances the Document visibility epoch
exactly once. Purge admission is additionally protected by a partial unique
index on Document, Version, and Document visibility epoch. The post-lock
mutation timestamp uses `clock_timestamp()` so lock waits behind
replace/reprocess cannot write a stale transaction-start timestamp.
The index lives in additive migration `007`; committed migration `006` remains
unchanged so databases that already recorded it still receive the new fence.

The transaction cancels pending/processing Jobs, tombstones every non-deleted
Version while advancing each Version visibility epoch, and tombstones the
logical Document. It creates one authority-free `stage=purge, operation=purge`
Job per immutable Version for derived artifact/index cleanup. Source File rows,
object keys, and object bytes are untouched; after commit no live Version
binding remains, so the File Owner may separately delete the Source File.

Each cancelled Job emits `knowledge.processing.cancelled`. Each tombstoned
Version emits `knowledge.document.tombstoned` with content-hash, File,
visibility, Collection-revision, and purge-Job references, but no object key,
filename, raw content, or credential. Concurrent/repeated authorized DELETEs
return `204` while emitting one set of Jobs/events. Any purge-ID or Outbox
failure rolls the entire tombstone transaction back.

```text
go test ./internal/knowledge ./internal/httpserver       passed
PostgreSQL 16 deletion tests under -race                 passed
concurrent/repeated delete idempotency                    passed
Version tombstone + Job cancellation + purge admission   passed
Source Files retained / live bindings removed            passed
synthetic Outbox failure rollback                        passed
synthetic purge-ID failure rollback                       passed
database-enforced per-Version/fence purge uniqueness      passed
```

Next: reconcile the completed 15.1D Document/File lifecycle, then begin
Governance and Collection/User Consent service APIs.

## 2026-07-11 — Phase 15.1D-4A operator Governance implemented

Added operator-only `governance-apply --manifest-stdin` and
`governance-disable --processor ... --endpoint-id ...` commands to
`mm-chat-admin`. The strict, bounded JSON manifest contains policy declarations
only; unknown fields and credential-like additions are rejected. Service-side
normalization sorts/deduplicates purposes and data types and computes the
versioned canonical SHA-256 manifest identity. Declaration values use bounded
lowercase identifiers, data types use MIME/wildcard grammar, and duplicate or
case-variant JSON keys are rejected.
Policy declarations are closed to the reviewed baseline values, and data types
permit exact MIME values or global `*` only so Governance and admission match.

Postgres serializes each Processor/Endpoint binding with a transaction advisory
lock, inserts immutable Approved Profiles, advances Active/Disabled Head
revisions, and writes `knowledge.governance.head.changed` in the same
transaction. Exact active-manifest reapply and repeated disable are semantic
no-ops. Profile/event ID or Outbox failure rolls back both Profile and Head.
Migration `008` enforces immutable Profile history by rejecting UPDATE/DELETE
in PostgreSQL rather than relying only on Repository convention.

```text
Go unit tests for strict manifest and canonical hash       passed
PostgreSQL 16 lifecycle tests with race detector           passed
concurrent first apply serialization                       passed
actual Outbox uniqueness failure rollback                  passed
policy/credential fields absent from Outbox                passed
database-enforced Profile UPDATE/DELETE rejection           passed
```

Next: implement Collection Consent reads, grant/revoke, ACL, expiry validation,
processing revision fences, and transactional Outbox.

## 2026-07-11 — Phase 15.1D-4B Collection Consent implemented

Added strict authenticated Collection Consent routes for list, PUT grant, and
DELETE revoke. Personal owners and Team Admins may mutate; active Team Members
may read redacted current decisions. Outsiders and inactive memberships follow
the disclosure-safe Collection `404` path. Public DTOs expose only Processor
alias, terms, decision, expiry, and decision time.

PUT resolves exactly one active Approved Governance Head/Profile, validates
purposes and exact MIME/global-wildcard data types, and pins the resulting
Profile and revisions. Canonically equivalent PUTs are no-ops. DELETE inserts
an immutable revoked decision; repeated revoke is a no-op. Each real transition
supersedes the old current row, advances `collection_processing_revision`, and
writes `knowledge.collection.consent.changed` atomically.

```text
Go unit/HTTP strict payload and redaction tests             passed
Personal owner / Team Admin / Team Member / outsider ACL    passed
future-expiry validation and expired grant rejection        passed
concurrent identical PUT: one revision and one event        passed
second-endpoint apply versus PUT phantom serialization      passed
actual Outbox uniqueness failure rollback                   passed
PostgreSQL 16 integration tests under race                  passed
```

Next: implement authenticated User Query Consent list/grant/revoke with its
independent query-consent revision fence and Outbox.

## 2026-07-11 — Phase 15.1D-4C User Query Consent implemented

Added protected `/v1/me/knowledge/query-consents` list, PUT, and DELETE routes.
The subject is always derived from the authenticated Session; no path or body
can nominate another User, and Team roles grant no authority to consent for a
member. Transactions lock the active User before Governance and Consent state,
serializing account disablement with new egress authorization.

Query Consent accepts only `query_embedding`, `rerank`, and `answer`, exact
MIME/global-wildcard data types, a bounded policy version, and an optional
future expiry. PUT pins the unique active Approved Governance binding;
equivalent PUT and repeated revoke are no-ops. Each real transition inserts an
immutable history row, advances `user_query_consent_state`, and emits
`knowledge.user.query-consent.changed` with the exact endpoint/Profile/Head
tuple in the same transaction.

```text
Go unit/HTTP auth, strict payload, and redaction tests       passed
two-user subject isolation                                  passed
Governance replacement requires a new Consent revision      passed
first transition query revision baseline 1 -> 2             passed
concurrent identical PUT: one transition                    passed
actual Outbox uniqueness failure rollback                   passed
nanosecond expiry canonicalized to PostgreSQL microseconds   passed
PUT/DELETE races and concurrent DELETE                      passed
account-disable versus queued PUT serialization              passed
DELETE Outbox failure restores Consent and state revision    passed
PostgreSQL 16 integration tests under race                  passed
```

Next: reconcile Phase 15.1D Governance/Consent expiry and wiring contracts,
then run the complete verification and promotion gates.

## 2026-07-11 — Phase 15.1D-4D/5 expiry and wiring reconciled

Migration `009` adds an indexed `expiry_materialized_at` time-fact marker.
Expiry never forges a User `revoked` decision: the immutable grant remains
auditable while `effectiveStatus=expired` is returned and emitted. The API
runtime starts a Postgres expiry worker that scans candidates without locking,
then reacquires User or Team/Collection locks in canonical order, rechecks the
current due row, advances the applicable revision, and writes Outbox in one
transaction. PUT/DELETE materialize an elapsed current grant first so a race
cannot swallow its expiry fence/event.

HTTP/wiring reconciliation confirmed every Phase 15.1D route is registered,
Bearer protected, safely decoded/redacted, and assigned a bounded metric/log
path. Contracts now register `KnowledgeApi` at the top-level frontend boundary,
mark search explicitly future/unregistered, align runtime errors, and document
the complete Knowledge deployment/rollback smoke. The executable frontend
remains unchanged until its later minimal adapter slice.

```text
two concurrent expiry workers: exactly-once markers/events  passed
expiry versus PUT/DELETE revision ordering                  passed
expiry Outbox failure full rollback                         passed
effectiveStatus redaction DTO                               passed
migration 009 up/down schema contract                       passed
18/18 Knowledge routes protected and metric-bounded         passed
full Go race suite and go vet                               passed
PostgreSQL 16 Knowledge/migration race suites                passed
independent xhigh review P0/P1/P2                            0/0/0
Knowledge security scan and diff check                       passed
```

Next: execute Phase 15.1D-6 full verification and replay/migration drills.

## 2026-07-11 — Phase 15.1D-6 migration replay gate hardened

The first full verification audit found a release-blocking historical replay
gap: the originally published migration `006` owned the purge-Job fence, while
a later source variant removed it and migration `007` recreated the same index
without `IF NOT EXISTS`. A database already migrated by commit `2010d73` would
therefore fail while upgrading to `007`.

Restored the original immutable `006` Up/Down pair. Migration `007` is now a
compatibility reconciliation using `CREATE UNIQUE INDEX IF NOT EXISTS`; its
Down preserves the index owned by `006`. The migration runner now records a
SHA-256 checksum covering migration identity plus both SQL directions, checks
the stored name/checksum before every Up/Down, and fails closed on legacy rows
until an operator explicitly accepts the reviewed source with `baseline`.
Up, Down, and baseline hold one PostgreSQL advisory lock across metadata setup,
validation, and all requested migration changes.

Added a tracked PostgreSQL integration drill covering fresh `001 -> 009`,
verified PostgreSQL major version 16, `009 -> 007` tail rollback, `007 -> 009`
reapply, no-op replay, and migration name/checksum drift rejection. A separate
historical-artifact drill built the migrator from commit `2010d73`, applied its
published `001 -> 006`, explicitly baselined those reviewed legacy rows, then
upgraded with current source through `009`.

```text
historical 2010d73 migration 006 -> current 009             passed
current 009 Down -> Up -> no-op replay                       passed
fresh PostgreSQL 16 001 -> 009 integration                  passed
migration name/checksum drift rejection                      passed
legacy checksum baseline                                    passed
two concurrent migrators serialized by advisory lock         passed
held advisory lock forced a distinct backend PID to wait      passed
Go migration unit and race suites                            passed
```

Phase 15.1D-6 remains open while the explicit two-User/two-Team ACL,
membership/mutation race, delete/reprocess race, and Outbox replay gates are
reconciled.

## 2026-07-11 — Phase 15.1D-6 ACL and Outbox source gates

Added a real PostgreSQL two-User/two-Team matrix covering Personal ownership,
cross-Team isolation, Team Admin inability to infer another User's Personal
Knowledge, Collection/Document/content/Consent reads, cross-scope mutation
denial, and disabled-actor read/write denial. Repository read predicates now
require an active, non-deleted actor; existing Collection/Document/Consent
mutations lock and recheck that User before Team, Membership, and Collection.

Source inspection confirmed that this Go control plane currently produces
`knowledge_outbox` rows but has no consumer, projection checkpoint, or search
generation. The 15.1D gate was therefore corrected rather than faked: Go owns
producer durability and source-recovery prerequisites, while real duplicate/
out-of-order application, contiguous checkpoints, restart recovery, and search
reconstruction remain mandatory Python RAG worker gates.

Added a PostgreSQL source test proving BIGSERIAL allocation order is not commit
order: a higher ID can commit and become visible while a lower allocated ID is
still open, and a later full pending-row rescan recovers both. The same test verifies unique
`event_id`, JSON-object payload, and status/timestamp constraints.

```text
two Users / two Teams Personal and Team ACL matrix           passed
cross-Team and cross-Personal disclosure-safe denial         passed
Team Admin plus representative Member/removed paths           passed
disabled actor public repository read/write denial            passed
Outbox allocation gap and post-commit full rescan             passed
duplicate event ID and invalid Outbox shape rejection        passed
PostgreSQL 16 targeted race suite                             passed
```

The integration evidence used `postgres:16-alpine` with an explicit disposable
database URL; both named tests reported `RUN` and `PASS`. They skip without the
URL, and a skip is not accepted as promotion evidence. With `PORT` set to the
container's dynamically published local port, the replayable command was:

```bash
MM_CHAT_TEST_DATABASE_URL="postgres://neo_chat:test-only-password@127.0.0.1:${PORT}/neo_chat?sslmode=disable" \
MM_CHAT_REQUIRE_POSTGRES_TESTS=true \
GOCACHE=/tmp/mm-chat-go-cache \
go test -count=1 -race ./internal/knowledge \
  -run 'TestPostgresKnowledgeACLTwoUsersTwoTeamsAndDisabledActor|TestPostgresKnowledgeOutboxSourceRecoveryInvariants'
```

Next: implement both lock schedules for Membership removal versus Team
Collection/Document/Consent mutations, then delete versus reprocess.

## 2026-07-11 — Phase 15.1D-6 Membership mutation races verified

Added deterministic PostgreSQL concurrency coverage for Membership removal
versus Team Collection update, Document deletion, and Collection Consent grant.
Every case runs both legal schedules. A gate transaction locks the Team;
`pg_blocking_pids` then proves the first operation is waiting on that exact
backend and the second operation is waiting on the first operation's User lock.
No timing-only scheduling is used.

Removal-first commits the Membership removal before the Knowledge writer can
recheck authorization, so the writer receives disclosure-safe Collection or
Document `404` semantics and leaves no mutation event. Mutation-first commits
the authorized Knowledge change before removal; removal then commits, and both
effects remain as the equivalent serial order. Both schedules assert one
Membership revision/event, final removed status, exact Knowledge revisions and
events, and no timeout or deadlock.

```text
Collection update: removal-first / mutation-first             passed / passed
Document delete: removal-first / mutation-first                passed / passed
Collection Consent: removal-first / mutation-first             passed / passed
exact backend blocking chain via pg_blocking_pids              passed
PostgreSQL 16 targeted -race                                   passed
```

Replay command; `MM_CHAT_REQUIRE_POSTGRES_TESTS=true` converts a missing test
URL from Skip into failure:

```bash
MM_CHAT_TEST_DATABASE_URL="postgres://neo_chat:test-only-password@127.0.0.1:${PORT}/neo_chat?sslmode=disable" \
MM_CHAT_REQUIRE_POSTGRES_TESTS=true \
GOCACHE=/tmp/mm-chat-go-cache \
go test -count=1 -race ./internal/knowledge \
  -run 'TestPostgresMembershipRemovalSerializesTeamKnowledgeMutations' -v
```

Next: run the final Document delete versus reprocess lock-order gate.

## 2026-07-12 — Phase 15.1D-6 Document delete/reprocess race verified

Added the final planned Document lock-order race with both legal schedules.
A pinned Collection-row gate plus the isolated test `application_name`, exact
single `pg_blocking_pids` relationship, and Collection/User query matching prove
which operation owns the User lock and which is waiting; the test does not use
sleep-based ordering.

Delete-first tombstones the Document/Version before reprocess authorization can
complete, so reprocess returns disclosure-safe `DOCUMENT_NOT_FOUND` and creates
no Job or request/cancellation event. Reprocess-first creates the fenced Job and
request event; deletion then cancels that Job, creates one purge Job, advances
Document/Version visibility epochs, and emits exact cancellation/tombstone
payloads. Both schedules end in the same authoritative tombstone state.

```text
delete-first lock schedule                                  passed
reprocess-first lock schedule                               passed
Document/Version visibility epochs 1 -> 2                  passed
reprocess request plus subsequent cancellation transition   passed
one purge Job and exact tombstone revision payload          passed
PostgreSQL 16 targeted -race                                passed
```

Replay command:

```bash
MM_CHAT_TEST_DATABASE_URL="postgres://neo_chat:test-only-password@127.0.0.1:${PORT}/neo_chat?sslmode=disable" \
MM_CHAT_REQUIRE_POSTGRES_TESTS=true \
GOCACHE=/tmp/mm-chat-go-cache \
go test -count=1 -race ./internal/knowledge \
  -run 'TestPostgresDocumentDeleteAndReprocessSerialize' -v
```

Next: reconcile the complete 15.1D-6 matrix, then mark it only if no remaining
Go/PostgreSQL gate is unverified.

## 2026-07-12 — Phase 15.1D-6 concurrent first-bind gate verified

The final matrix audit found one remaining explicit case: two concurrent first
Document binds using the same actor-scoped idempotency key. Added a PostgreSQL
race with different candidate Document, Version, and Job IDs. A pinned
Collection-row gate proves the first request is waiting on that Collection and
the second request is waiting on the first request's User lock before release.
Collection/File locking plus idempotency replay returns the same winning
Document and Pending Version to both callers and persists exactly one Document,
Version, initial parse Job, and version-request event with the exact authority
revision payload. The source File remains available and bound.

Added `scripts/verify-phase15d-postgres.sh` as the fail-closed promotion entry:
it refuses to run without `MM_CHAT_TEST_DATABASE_URL`, forces
`MM_CHAT_REQUIRE_POSTGRES_TESTS=true`, and executes the complete Knowledge plus
migration race suites. The target must be a disposable PostgreSQL 16 database.

```text
two concurrent first binds -> one replay-safe transition     passed
one Document / Version / initial Job / exact Outbox event     passed
source File remains available                                passed
delete/reprocess expanded Job and revision assertions         passed
fail-closed Phase 15.1D PostgreSQL verification script        added
```

Next: run the complete fail-closed script and independent final 15.1D-6 review;
only then check the verification item.

## 2026-07-12 — Phase 15.1D-6 verification closed

The fail-closed `verify-phase15d-postgres.sh` completed against a disposable
PostgreSQL 16 instance after the deterministic concurrent first-bind and
expanded delete/reprocess assertions were added. The script ran the complete
Knowledge and migration suites under the race detector; the normal full Go
race suite, `go vet`, diff check, shell syntax check, and Knowledge security
scan also passed.

The independent final review returned `P0/P1/P2 = 0/0/0` and confirmed that
the Go/Postgres ACL, Consent, migration replay, deletion, idempotency,
concurrency, and Outbox producer/source-recovery gates are closed. Real Outbox
consumer replay/checkpoint/projection reconstruction remains explicitly owned
by the later Python RAG worker phase and was not counted as a Go gate.

```text
fail-closed PostgreSQL 16 Knowledge + migration race script  passed
full Go race suite and go vet                                passed
shell syntax, diff check, Knowledge security scan            passed
independent final review                                     P0/P1/P2 = 0/0/0
```

Phase 15.1D-6 is complete. Next: execute Phase 15.1D-7 Promotion and commit the
final verification slice with an explicit `mm-chat/` allowlist.

## 2026-07-12 — Phase 15.1D-7 Promotion hardening and pre-review gates

Two independent xhigh audits blocked Promotion on fail-open production
placeholders, mutable-image rollback, non-executable historical migration
evidence, and drift across the Knowledge contract, persistence, restore, worker,
and tracking documents. The findings were fixed before checking the Promotion
item.

The new `scripts/preflight-single-server.sh` parses the operator env file as
data rather than sourcing it, requires mode `0600`, rejects missing or example
values without printing them, checks Postgres/Redis URL credential consistency,
requires HTTPS external endpoints, and requires a release-specific
`BACKEND_IMAGE` plus `MM_CHAT_VERSION`. Its shell regression test covers the
example file, insecure permissions, placeholders, URL mismatch, success, and
secret-free errors. Compose now gives backend/migrate/admin one shared image
reference. The runbook retains the previous image ID/digest and rolls back
without rebuilding; Go and Alpine base images are digest-pinned.

Migration verification now pins SHA-256 for every published `2010d73`
`001-006` Up/Down file, applies that exact checksum-less historical state,
requires explicit baseline, upgrades with current `007-009`, and verifies the
tail catalog. Contracts now match required Document idempotency, Consent time
validation, authoritative status sets, and registered routes. Persistence and
deployment docs now describe schema head `009`, Knowledge Jobs/Outbox,
Governance immutability, Consent expiry materialization, fail-closed worker
shutdown, and restore checks spanning Postgres metadata plus sampled MinIO
objects.

Pre-review verification:

```text
gofmt + go test -count=1 -race ./... + go vet            passed
govulncheck ./...                                         0 called vulnerabilities
published 2010d73 -> baseline -> current 009 replay       passed
fail-closed PostgreSQL 16 Knowledge + migration suite     passed
Compose app/ops config + digest-pinned backend build      passed
backend image API/migrate/admin binary presence           passed
preflight shell regression and syntax checks              passed
changed-document Prettier + scoped diff check              passed
quality scanner                                            passed; baseline warnings only
security scanner                                           3 known test-fixture literals only
```

The scanner matches are synthetic password/API-key strings in existing auth
and provider tests; no production credential or new secret was found. Next:
run the independent final xhigh review, close every P0/P1/P2 finding, then mark
15.1D-7 and commit only the explicit `mm-chat/` allowlist.

## 2026-07-12 — Phase 15.1D-7 Promotion review closed

Successive independent xhigh reviews found and closed production-only gaps that
ordinary Compose config/build checks did not expose: shell environment
precedence over `--env-file`, Compose dotenv interpolation/escape divergence,
mutable image tags, retained `build:` fallbacks, recovery metadata checksum
drift, conflicting rebuild-based rollback instructions, direct backup/restore
Compose paths, and temporary MinIO bucket collision cleanup.

The final boundary is fail-closed:

- production env syntax is direct and unambiguous; quoting, escaping, inline
  comments, interpolation, duplicates, reserved Compose/Docker names, insecure
  permissions, placeholders, and inconsistent URL credentials are rejected;
- production images require a full registry `@sha256:` digest;
- `compose-single-server-production.sh` starts Compose under `env -i`, disables
  implicit `.env`, fixes the base/production file pair, rejects file/env/build
  overrides, and removes backend/migrate/admin `build:` with `!reset`;
- production backup and restore use dedicated clean-environment wrappers;
  MinIO restore is limited to a uniquely created temporary bucket, verifies
  sampled Knowledge objects, and cleans only a bucket it created;
- restore acceptance compares exact migration `001-009` version/name/checksum
  tuples and rejects missing, extra, NULL, or drifted rows.

Final evidence:

```text
Go race/vet + govulncheck                                 passed / 0 called
PostgreSQL 16 Knowledge/migration + historical replay     passed
Compose dev build + production app/ops/restore config     passed
production preflight/wrapper regression suite             passed
restore manifest acceptance + drift rejection             passed
shell syntax + changed-doc Prettier + scoped diff          passed
quality scanner                                            passed
security scanner                                           3 known test fixtures only
independent xhigh final review                              P0/P1/P2 = 0/0/0
```

The executable infra/API contracts and lessons are captured under `mm-chat/`
per the refactor isolation rule; `.trellis/spec/` remains unchanged because it
is outside the authorized task write set. Phase 15.1D-7 is complete. The next
slice may begin Python RAG Outbox consumption/indexing design; Go/Postgres
control-plane Promotion is no longer the blocker.

Post-commit mode verification found that local Git uses `core.filemode=false`:
the new scripts were executable on disk but initially recorded as `100644`.
Commit `f62000a` corrected every `mm-chat/scripts/*.sh` index entry to `100755`
without amending the Promotion commit `9f5e907`. A clean checkout can therefore
execute every documented `./scripts/...` command directly. No push was
performed.

## 2026-07-12 — Phase 15.2 single-server RAG design locked

The Owner Grill closed the listed product-scope decisions before implementation:
one Compose server, no Qdrant/Kubernetes, all common document formats plus
scanned PDF, no standalone image knowledge in the first release, explicit
Collection processing consent, server-owned MinerU/Jina credentials, user BYOK
for chat, parent/child chunks with bounded overlap, strict grounded mode only
when Knowledge Attachments are selected, source citations, immediate logical
deletion, three query concurrency, one indexing concurrency, and encrypted R2
backup. Relevance tuning uses 100 human-confirmed questions split into 80
Development/Validation and 20 Frozen Holdout; ACL, Consent, deletion,
injection, citation, and parser formats use independent matrices/corpora.

Three independent xhigh research agents inspected the current migration `009`
runtime, Outbox/Job invariants, single-server resource envelope, and current
PostgreSQL BM25 candidates. The resulting design is recorded in
`docs/architecture/phase-15-2-single-server-python-rag-consumer-indexing-plan.md`.
It keeps Postgres authoritative, uses Redis only as a wake/cache layer, adds a
private Python API/worker, and selects PostgreSQL 16 + pgvector + ParadeDB
`pg_search` as the first Bake-off candidate. ParadeDB is not promoted: AGPL,
logical restore, crash recovery, Chinese tokenization, ACL-in-query, resource,
upgrade, and rollback gates remain blocking.

Important compatibility findings are explicit in the plan: Outbox BIGSERIAL is
not commit order; current Jobs lack a stale-worker lease token; no applied-event,
artifact, chunk, generation, or projection schema exists; Jina 2048 dimensions
cannot be assumed to fit a pgvector `vector` HNSW profile; and changing the
Postgres distribution requires a new volume plus logical restore rather than
reusing the Alpine PGDATA directory.

```text
Owner decisions                                            locked
current migration/outbox/runtime evidence                  reconciled
single-server Python consumer/indexing design              created
Qdrant-first implementation wording                        superseded
implementation                                             not started
```

Next: independent design review, then begin Phase 15.2A by freezing the
internal Evidence API and running the PostgreSQL BM25/pgvector Bake-off.

## 2026-07-12 — Phase 15.2 design review closed

The independent xhigh reviewer initially found `P0/P1/P2 = 0/9/4`. The design
was corrected instead of treating those findings as implementation details:

- BYOK answer evidence now requires exact answer-purpose Governance plus
  Collection/User Consent; Team members cannot exfiltrate Team evidence to an
  arbitrary Provider endpoint;
- Python DB/S3 access now uses separate least-privilege roles, Go-issued
  object capabilities, per-Generation temporary artifact access, and negative
  permission tests;
- Outbox claim/reclaim/ack, stale-worker lease tokens, non-null applied-event
  scope, atomic Version/Generation publish, and durable Collection purge
  fan-out are explicit;
- Source File retention remains aligned with the existing separate File-delete
  contract, while Projection/Artifact purge keeps the 15-minute target;
- backup uses a short local consistency barrier that never blocks emergency
  ACL/Consent revocation, then uploads to R2 asynchronously;
- the container starting envelope was reduced to 2464 MiB, maintenance jobs are
  mutually exclusive, HNSW low-selectivity ACL recall is a required Bake-off,
  and MinIO loss fails strict citation verification closed;
- the 100-question set is explicitly Relevance-only; security, deletion,
  consent, citation, injection, and parser formats use independent gates.

Second review reduced the result to `0/2/1`; the remaining dynamic S3 Prefix,
long backup barrier, and tracking-scope issues were corrected. Third-round
read-only review returned:

```text
P0/P1/P2 = 0/0/0
```

Phase 15.2 design review is closed. Implementation remains pending.

## 2026-07-12 — Phase 15.2A Evidence/schema contracts and Postgres bake-off

Three disjoint xhigh workers froze the internal Evidence API, the future
`010/011` persistence contract, and an isolated PostgreSQL search bake-off.
The main integration pass reconciled one corpus-wide Index Generation with
per-document Materializations, added the missing exact Projection identifiers
to the Evidence DTO, fixed Query-Embedding versus Rerank Consent shapes, and
froze Redis as the single fail-closed Replay Nonce Backend. Current runtime
gaps remain explicit: authoritative `sessionId` is not propagated to handlers,
Stream Chat has no Knowledge Selection DTO, user BYOK is not wired, Consent
uniqueness omits Endpoint, Governance has no independent Model ID, and no
Citation/Python RAG runtime exists.

The Bake-off harness is isolated under `ops/bakeoff/postgres/`; its runner uses
a unique Compose project, no Host Port, a disposable Volume and trap-scoped
cleanup. It pins:

```text
PostgreSQL             16.14
pg_search              0.24.2
pgvector               0.8.2
image digest           sha256:556edd8c...3dcf
cgroup                  1 GiB / 2 CPU
shared/work/maintenance 256MB / 4MB / 64MB
```

The first worker result only covered Jieba and used `shared_buffers=128MB`.
Integration did not accept that partial result: Lindera Chinese,
`chinese_compatible`, a separate Exact Keyword/Phrase Lane, filtered-HNSW
exact-recall comparison, and full `halfvec(2048)` exact/HNSW recall were added;
the locked 256MB Postgres setting was restored. The final clean run reported:

```text
Jieba/Lindera/chinese_compatible BM25 ACL hits       1 / 1 / 1
vector(1024) HNSW recall@20                          1.00
halfvec(2048) HNSW recall@20                         1.00
4%-selectivity authorized HNSW top-10 / leakage      10 / 0
filtered HNSW recall@10                              1.00
Exact Keyword/Phrase authorized match                1
template0 logical restore                            passed
graceful restart / SIGKILL recovery                  passed / passed
peak sampled memory under 1 GiB limit                196.2 MiB
container / volume / network cleanup                 passed
report                                                /tmp/mm-chat-phase15-pg-bakeoff.6NIInx
```

This is a Synthetic Operational Baseline, not Production Promotion. It proves
the pinned image can execute all required mechanics and recovery paths without
touching the running mm-chat stack. The Relevance Set must still choose the
Tokenizer/Dimension winner; AGPL, real Tail Latency, Extension upgrade and
rollback remain blocking before winner-specific `011` DDL.

Next: close the independent xhigh review, commit only the explicit `mm-chat/`
allowlist, then start Phase 15.2B Durable Consumer.

## 2026-07-12 — Phase 15.2A independent review closed

The independent xhigh reviewer first returned `P0/P1/P2 = 0/0/1`: all contract
findings were fixed, while one P2 remained because executable SQL assertions
added during review had not yet been rerun. The fixes established:

- exact `processor + endpoint + model` Governance/Consent namespaces with
  simultaneous approved Models per Endpoint;
- Stage-aware Job Model binding, preserving `model_id=NULL` for Purge;
- bounded, already-fenced Python Candidate/Expansion text for Jina without
  arbitrary table reads or text in the Python→Go response;
- a separate Go-only exact-ref Reauthorization/Hydration role and Function;
- strict separation of Document Materialization Publish from Corpus Generation
  Promotion;
- a Composite Outbox ID/Event ID Ledger FK;
- explicit Exact Lane GIN plans and Index-only-scan-disabled vector baselines.

After those changes, Prettier, Bash syntax, Compose config, scoped diff checks,
local Markdown links, cleanup checks, and the complete Bake-off passed. The
security scanner reported only the same three synthetic password/API-key
fixtures in existing auth/provider tests; no changed file contains a credential.
A final read-only review returned:

```text
P0/P1/P2 = 0/0/0
```

Phase 15.2A is complete. Production Search Profile Promotion remains open by
design; the next implementation slice is Phase 15.2B Durable Consumer.

## 2026-07-12 — Phase 15.2B plan and compatibility audit locked

Three parallel xhigh read-only audits traced migration `009`, every current
Knowledge Outbox/Job producer, Go worker lifecycle, Redis/Compose wiring and the
future Python boundary before implementation. They found that adding a worker
alone would be unsafe: the migration runner could not inject the explicit
Governance Model Mapping; Consent and routes were still processor-only; current
Go producers create Jobs before any Corpus Generation exists; and production
DB credentials are shared.

The executable response is recorded in
`docs/architecture/phase-15-2b-durable-consumer-plan.md`:

- Go remains the authority and migration owner; Postgres Functions own every
  atomic Claim/Ack/Heartbeat/Finish/Replay transition;
- Python runs in an independent `rag-worker` container and calls only those
  Functions; it never runs DDL or direct authoritative DML;
- migration `010` is complete and extension-independent, while current producer
  Jobs are explicitly marked legacy and never Claimable;
- Worker Dispatch defaults off and the real Stage Handler Registry is empty
  until `011`, the first Generation and Phase 15.2C are promoted;
- Redis Wake is optional acceleration around mandatory Postgres Poll/Rescan;
- DLQ remains durable Postgres state and Replay is audited, exact-ID,
  expected-error fenced and dry-run by default;
- API, migrator, worker and replay credentials are separate.

Four xhigh workers now own disjoint implementation slices: migration/functions,
Go model-aware compatibility, Python worker, and Compose/operations. A separate
review agent will run after integration; no Phase 15.2B runtime item is checked
until executable evidence passes.

## 2026-07-12 — Phase 15.2B implementation and independent review closed

The independent final review exercised the complete uncommitted `mm-chat/**`
slice and fixed the remaining fail-closed gaps before accepting the phase:

- Governance Mapping now rejects duplicate JSON object keys, recomputes every
  mapped `profileContractHash` from the locked schema-`009` row and mapped
  Model, and rejects authority/head Model mismatch atomically;
- capability roles must be restricted NOLOGIN roles with no inherited
  memberships; `go_api_runtime` retains schema-`009` API capability across
  `010.down` while remaining unable to claim Worker work, replay DLQ rows,
  mutate migration state, or create schema objects;
- Worker readiness now verifies the actual caller can execute the complete
  durable function set instead of treating function presence as implicit;
- legacy projection-unbound Jobs remain unclaimable and cannot create a stuck
  Replay successor; replay remains exact-error fenced and audited;
- Python Action/Stage allowlists now exactly match the `010` Functions, and a
  heartbeat exception or shutdown cancellation fences and awaits the active
  handler instead of allowing orphaned work;
- the Python image installs a non-editable wheel into the same absolute venv
  path used at runtime; the regression smoke starts both `rag-worker` and
  `rag-replay` instead of accepting a build-only result;
- durable Collection purge roots now have token-fenced claim and paginated,
  resumable enumeration before item processing/completion;
- published schema-`009` deployment docs now mount the reviewed Mapping as a
  read-only one-shot file instead of showing a fresh-only migration command;
- Compose keeps migrator, API/admin, Worker, and Replay database routes
  isolated, production resolves digest-only images with no `build:` path, and
  the RAG containers receive no MinIO or provider credential.
- restore acceptance now pins migration `001-010`, asserts the durable
  projection catalog/Functions, and the persistence/release runbooks use only
  `MIGRATION_DATABASE_URL` for migration operations.

Final executable evidence:

```text
Go vet + full race suite with PostgreSQL 16                    passed
Fresh/Published/Mapping/Down-Up/Role/Lease/Purge integration  passed
Python Ruff/format/Mypy/Pytest coverage                       passed / 87 tests / >=90%
Python live PostgreSQL + Redis integration                    included / 2 tests
pip-audit                                                     no vulnerabilities
RAG Docker build/start/read-only/non-root/replay smoke        passed
Compose dev/production render + no production build          passed
preflight regression + shell syntax + scoped diff check       passed
independent review rounds                                     0/1/2 -> 0/0/1 -> 0/0/1 -> 0/0/0
```

Phase 15.2B is complete only for durable dark-run mechanics. Real Parse,
Embedding, Purge handlers, Search migration `011`, first Generation promotion,
and user-visible RAG remain Phase 15.2C or later and stay disabled.

## 2026-07-12 — Phase 15.2C generation-bound indexing plan drafted

Four parallel xhigh read-only audits traced the current Go producer path,
Postgres projection functions, Python worker state machine, Provider/Search
contracts, Compose credentials, deletion behavior, and recovery boundary. The
audits agreed that merely filling the Handler Registry would be unsafe:

- current `dispatch` can Ledger/Ack without creating any generation-bound Job;
- all new Go Jobs remain `legacy_projection_unbound=true` and are intentionally
  excluded from Claim;
- no worker-safe Artifact/Block/Chunk staging surface exists;
- same-Generation Reprocess conflicts with Chunk uniqueness that omits the
  Materialization;
- MinerU submit, physical object deletion, and Stage-specific job completion
  have no durable crash protocol;
- current Publish does not close the complete Profile/Consent/Manifest/Version
  activation contract;
- the real Jina relevance winner, Cosine index shape, Chinese tokenizer,
  extension license and rollback gates are not yet frozen.

The draft response is recorded in
`docs/architecture/phase-15-2c-generation-bound-indexing-plan.md`. Published
migration `010` remains immutable. Winner-specific Search DDL is reserved for
`011`; Processing Request, Dispatcher V2, generation fan-out, Provider
Operation, gateways, staging/finalizer, physical deletion and rebuild enter
`012`. Evaluation-only 1024/2048 candidates stay outside production Generation
state until one Search Profile wins.

The activation order is fail closed: external wire contracts and offline
parser/fake-provider tests first, real relevance bake-off second, migrations
and private gateways third, Canary/Building Generation fourth, then Dispatcher
and each Job Stage one at a time. `RAG_WORKER_DISPATCH_ENABLED=false`, the empty
production Handler Registry, no user Query, and no Corpus Generation Promotion
remain mandatory until their explicit gates pass.

```text
Phase 15.2B durable dark-run                 complete
Phase 15.2C executable plan draft            created
MinerU/Jina redacted wire contract           pending
production 011/012 and real handlers         not implemented
user query / generation promotion            disabled
```

First independent review returned `P0/P1/P2 = 0/7/3`; implementation remains
blocked while Request/Replay identity, N-1 rollback, replacement retry, purge
fan-out, Outbox effects, Generation operator, Gateway crash recovery and C/E
boundaries are corrected. Next: repair every finding and rerun the independent
review before marking the plan locked or beginning C0 Provider work.

## 2026-07-12 — Phase 15.2C review rounds two and three

The second independent pass returned `P0/P1/P2 = 0/5/3`. It exposed missing
multi-Generation allocation preparation, shared-Outbox event ownership, legal
Profile creation, Canary Job termination, delayed Search grants, persistence
registration and Parser IPC. The plan now requires:

- DB-generated, nonce-fenced, ordered Dispatch Preparations and Python
  multi-Generation Plans;
- complete Event Subscription ownership, Global versus Request-bound Effects,
  and Generation Rebuild Root/Child events;
- an Approved Profile Bundle Function with exact non-placeholder Rerank
  metadata;
- a Canary Finalizer that ends the Stage Attempt without changing a Head;
- Search Functions owned and `PUBLIC`-revoked in `011`, with Execute granted
  only by a Phase 15.2D forward migration;
- a no-network Parser Sidecar using a bounded Unix-socket Wire Contract.

The third pass reduced the result to `0/2/3`. Its remaining corrections are now
applied: one Admission Request fans out to many Generation allocations in a
fixed `(generation_seq, generation_id)` order; `012.down` requires every
Request/Attempt/Preparation/Operation/Profile/Rebuild/Event table to be empty;
the Parser IPC uses a tmpfs-backed Docker named volume with fixed numeric IDs;
and the matrix covers Verified Candidate and no-Active initial-build races.

```text
review round 1    0/7/3
review round 2    0/5/3
review round 3    0/2/3
review round 4    0/1/0
review round 5    0/0/0
```

Round four found one final migration self-consistency issue: a clean `012.up`
seeds the migration-owned Approved Profile Registry, while the Down rule
incorrectly required that Registry to be empty. The plan now permits Down to
remove only an unreferenced Static Seed whose Canonical Bytes, signed Report
Hash and migration checksum exactly match the embedded values; runtime
Profiles/Work or any Registry drift still fail closed.

The fifth read-only review returned `P0/P1/P2 = 0/0/0`. Phase 15.2C design is
locked. Implementation may begin at C0, but real Provider calls and Activation
remain blocked until the redacted MinerU/Jina Wire Contract, immutable Model/API
Build, license, retention and SLA gates close.

## 2026-07-12 — Phase 15.2C C0 provider-contract intake implemented

Three parallel xhigh audits inspected the Python test boundary, Provider/
Governance contracts, and current official MinerU/Jina public documentation.
They found a fail-open operational path: the previous deployment guide piped a
syntactically valid `default/model-v1/v1` example directly into
`governance-apply`, which would create an Approved Profile without a frozen
Provider Contract.

The C0 intake slice now adds:

- a closed Draft 2020-12 JSON Schema and explicit
  `draft → verified → frozen → retired` lifecycle;
- a fixed-root loader that rejects arbitrary paths, duplicate keys, NaN/Inf,
  NUL, placeholders, secret-like values, credential-bearing URLs, unknown
  Evidence, invalid operation sets and Freeze/Hash drift;
- RFC 8785/JCS contract hashing with dev-only exact-pinned dependencies;
- blocked public MinerU, Jina Passage 1024/2048 and Jina Rerank Draft Fixtures;
- complete 1024/2048 Synthetic Vector widths rather than shortened examples;
- a Starlette/HTTPX in-memory Fake that opens no network and retains no Header
  Value or Body bytes;
- a non-executable `governance-mineru.blocked.json` and deployment runbook that
  forbids Governance Apply until a Frozen Contract derives the manifest.

Public documentation verifies candidate paths and shapes but does not close the
external gate. MinerU immutable API/build, account Endpoint, Query-by-key,
Cancel, BBox and terms remain unresolved. Jina immutable model build, region,
account Batch/Quota, normalization, terms and final Rerank selection remain
unresolved. No `.env`, API Key, Task ID or Signed URL was read or committed.

```text
C0 schema/loader/fake baseline             implemented
public MinerU/Jina fixtures                draft / blocked
production handler registries              empty
RAG_WORKER_DISPATCH_ENABLED                false
external Provider Contract Gate            open
```

Next: independent xhigh review of this C0 slice, then collect authorized
redacted captures and reviewed terms without exposing credentials.

## 2026-07-12 — Phase 15.2C C0 review round one repaired

The first independent xhigh review returned `P0/P1/P2 = 0/7/3`; the C0
baseline was therefore moved back to unchecked/in-review instead of being
treated as complete. The repair closes every reported class:

- strict JSON now rejects non-finite exponent results and integers outside the
  JCS safe range; path, URL, placeholder, secret-field and UTF-8 body-byte
  checks cover the complete wire tree;
- callers cannot override the checked-in Schema, validated Contracts are
  deeply immutable, and `require_frozen` reruns fixed Schema plus semantic
  validation;
- Evidence carries content/expiry metadata at Freeze, Terms only accept
  `reviewed_terms`, and the Freeze Gate hashes the actual report bytes;
- MinerU and Jina validators enforce operation/phase uniqueness, success and
  frozen error-class coverage, provider response shape, dimensions, indexes,
  finite values and non-negative usage;
- unverified Jina account limits and Rerank identity were removed; request and
  recording byte ceilings are explicitly local Fixture caps;
- Governance Apply is hard-blocked with
  `PROVIDER_WIRE_CONTRACT_NOT_FROZEN` before stdin/DB access, and the Service
  rejects placeholder Endpoint/Model/API values;
- downstream integrity is split into `wireContractHash`,
  `termsSnapshotHash`, `fixtureSetHash`, plus exact `freezeReportHash`; Fake
  calls retain only the static path template, never dynamic Provider IDs.

Repair evidence:

```text
Python Ruff/format/Mypy                         passed
Python unit suite                              116 passed / 2 deselected
Python coverage                                90.19%
pip-audit                                      no known vulnerabilities
Go full unit suite                             passed
Go full race suite                             passed
Go targeted vet                               passed
production handler registry                    empty
RAG_WORKER_DISPATCH_ENABLED                    false
```

Next: independent xhigh review round two. C0 stays unchecked and the external
Provider Contract Gate stays open until that review reaches `0/0/0`.

## 2026-07-12 — Phase 15.2C C0 review round two returned 0/3/3

The second independent xhigh review returned `P0/P1/P2 = 0/3/3`. It confirmed
the Go Governance hard gate, immutable/fixed-Schema loader, three canonical
Hash boundaries, invented-fact removal, full-width vectors, static Fake path,
empty production registries and disabled dispatch. It then reproduced three
remaining authority gaps:

- direct-dict unsafe integers/lone surrogates, whitespace/scheme-relative URLs
  and Policy-declared secret fields bypassed the complete-tree gate;
- Capability/Term/MIME values were only broadly typed, while Evidence
  `contentHash` was not recomputed from supplied Snapshot bytes;
- Jina request semantics, Rerank Identity binding, HTTP classification and
  non-synthetic frozen behavior coverage were not yet exact.

It also found missing Fake Content-Type value validation, inconsistent
`freezeReportHash`/`bakeoffReportHash` Bundle wording, and matching negative
tests. C0 remains unchecked. The repair now validates one JCS-safe Unicode JSON
tree for every entry point, merges Policy secret names, uses Closed Fact
Shapes, requires exact Evidence Snapshot bytes at Freeze, binds redacted Case
Evidence, excludes synthetic-only behavior coverage, fixes Jina/MinerU request
semantics and HTTP classifications, validates Fake media types, and aligns all
five Provider/Search Bundle hashes.

Next: rerun the complete Python/Go/format/security gates and submit the repaired
slice to independent xhigh review round three. External Provider calls remain
forbidden.

## 2026-07-12 — Phase 15.2C C0 review round three returned 0/3/1

The third independent xhigh review returned `P0/P1/P2 = 0/3/1`. Every round-two
PoC was rejected, but the reviewer then found three cross-contract gaps:

- URL tokens embedded inside Provider text could bypass start-anchored URL
  checks;
- Closed Capability objects were not yet bound to Provider Kind, frozen state
  requirements or the Jina Request Dimension;
- the Rerank public response omitted its OpenAPI-required Model, while MinerU
  Running/Done shapes omitted official `err_msg/start_time` fields and lacked a
  Failed variant.

The repair scans URL tokens at any string position, binds Capability State and
Shape per MinerU/Jina/Rerank, checks Capability/Request Dimension equality,
requires redacted-capture behavior for frozen coverage, restores the official
Rerank Response Model, and models MinerU Pending/Running/Done/Failed as separate
closed variants including `terminal_failure` over a 2xx provider envelope.
Matching negative and positive fixtures cover every reproduced path. C0 remains
unchecked pending round four; production egress remains disabled.

## 2026-07-12 — Phase 15.2C C0 review round four returned 0/1/1

The fourth independent xhigh review confirmed all prior Provider, Capability,
Evidence, Hash, Fake and Governance findings closed, then found one Unicode
edge: scheme-relative `//例子.测试/path` was not recognized because the
authority detector assumed an ASCII first host character. The detector is now
Unicode-aware (`//` followed by any non-whitespace, non-delimiter authority
character). Tests reject direct and embedded IDN scheme-relative forms while
retaining a positive credential-free `https://例子.测试/path` case. C0 remains
unchecked until the next independent review reaches `0/0/0`.

## 2026-07-12 — Phase 15.2C C0 independent review closed

The fifth independent xhigh review replayed the Unicode IDN authority cases and
the complete prior finding set, then returned `P0/P1/P2 = 0/0/0`.

```text
review round 1                         0/7/3
review round 2                         0/3/3
review round 3                         0/3/1
review round 4                         0/1/1
review round 5                         0/0/0
Provider targeted tests                59 passed
Python non-integration                 144 passed / 2 deselected
Python coverage                        90.19%
Ruff / format / Mypy                   passed
pip-audit                              no known vulnerabilities
Go full unit / race / vet              passed
security scanner                       no findings in changed test/Go scopes
production dispatch/handler registries empty
RAG_WORKER_DISPATCH_ENABLED            false
```

The C0 implementation baseline is complete and checked. This does **not** close
the external Provider Contract Gate: all four public Fixtures remain
`draft/blocked`; immutable builds, redacted captures, exact account limits and
reviewed terms still require authorized external intake before any real
Provider call or Governance Apply can be enabled.

## 2026-07-13 — Phase 15.2C C0 Provider Capture Harness checked

The follow-up C0 slice added the operator Capture Harness without performing a
real Provider call. The plan and threat model are frozen in
`docs/contracts/provider-capture-harness.md`; implementation remains outside the
production package and Docker copy boundary under `rag/tools/provider_capture*`.

Implemented boundaries:

- default CLI is canonical dry-run with zero network, zero Key requirement and
  zero evidence writes; only explicit `--execute` can enter the fixed plan;
- credentials come only from the process environment, never dotenv or CLI;
  exact HTTPS hostname/port/path allowlists, `trust_env=false`, redirects off,
  no retry, concurrency one, fixed timeouts and bounded streaming are enforced;
- Jina is capped at 1024 embedding, 2048 embedding and one two-document rerank;
  request semantics and response model/usage/count/dimension/index/finite-score
  shapes are checked;
- MinerU uses the researched v4 local-upload Submit endpoint once. Signed PUT
  and Result Poll budgets are deliberately zero, so successful evidence says
  `staged_after_submit`; response loss says `unknown_submission` and is never
  retried;
- only synthetic code-owned text/PDF enters requests. Closed v1 canonical
  evidence excludes text, vectors, Key material, response bytes, unknown Header
  values, Jina request IDs, MinerU IDs/URLs and Provider error detail;
- output is a newly created `0700` directory with one atomic `0600` file;
  parent components are walked as no-symlink directory FDs and all mutation is
  direct-child FD-relative. A hard-link publish prevents overwrite races and
  never deletes a foreign race target;
- HTTP requests force identity encoding, consume bounded raw streams and clear
  cookies between calls. Invalid argv/custom transport errors are reduced to
  allowlisted codes, and MinerU `unknown_submission` returns nonzero after its
  recovery evidence is written;
- public Fixtures were not changed, Governance was not generated/applied, and
  production Dispatch/Handler registries and disabled dispatch default remain
  unchanged.

Executable evidence:

```text
Ruff check .                                      passed
Ruff format --check .                             passed / 37 already formatted
Mypy src tests/support tools + capture tests      23 source files / no issues
Provider Capture targeted tests                   34 passed
Python non-integration suite                      178 passed / 2 deselected
Python coverage                                   90.19% (>= 90%)
pip-audit --skip-editable                         no known vulnerabilities
security scanner on rag/tools                     0 findings
quality checker on rag/tools                      0 errors / 0 warnings
real Provider requests                            not executed
.env / repository secrets                        not read
real Evidence Snapshot files                      not created (tmp tests only)
```

Independent Capture Harness review closed at `P0/P1/P2 = 0/0/0`. This result
applies to the Harness slice only; it does not freeze any Provider fixture or
close the External Gate.

The research boundary remains open. Jina OpenAPI baseline is
`2026.06.29.1712`, but public Tier values conflict and immutable build, region,
account limits and SLA are not frozen. MinerU immutable build, region, terms,
BBox, cancel and query-by-key remain unverified. All four Public Fixtures stay
`draft/blocked`; the next step is a separately authorized human-operated
capture and independent review, not production activation.

## 2026-07-13 — Capture Harness explicit private-proxy compatibility

The first Owner-authorized Jina attempt returned `PROVIDER_RESPONSE_LOST` before
an Evidence Snapshot was created. A credential-free connectivity probe then
proved the cause without reading the Key: `api.jina.ai` returned HTTP 200 through
the Owner-controlled WSL proxy, while direct port 443 timed out. The Owner
confirmed the private port `7890` belongs to their proxy software.

The Harness now supports one explicit process-environment input,
`PROVIDER_CAPTURE_PROXY_URL`. It accepts only uncredentialed literal RFC1918/
loopback IPv4 or unique-local/loopback IPv6 over `http` with an explicit nonzero
port. Hostnames, public/link-local/unspecified addresses, credentials, non-root
paths, query and fragment are rejected as `CAPTURE_PROXY_INVALID`. Generic
`HTTP_PROXY`, `HTTPS_PROXY` and `ALL_PROXY` remain ignored; `trust_env=false`,
Provider TLS verification, exact target allowlists, identity encoding, fixed
budget, zero retry and redacted Evidence remain unchanged. Proxy URL/Host/Port
never enter output, Evidence or logs.

Verification after the compatibility patch:

```text
Ruff / format                                    passed / 37 files
Mypy                                             23 source files / no issues
Provider Capture targeted tests                  61 passed
Python non-integration suite                     205 passed / 2 deselected
Python coverage                                  90.19% (>= 90%)
pip-audit --skip-editable                        no known vulnerabilities
security scanner on rag/tools                    0 findings
quality checker on rag/tools                     0 errors / 0 warnings
real Capture retry                               not executed yet
Public Fixtures / runtime registries             unchanged / disabled
```

This compatibility is operator-only and does not authorize a production RAG
proxy, freeze any Provider Contract, or close the External Gate. Next: commit
the reviewed patch, then repeat the Jina Capture once with the dedicated proxy
variable copied from the Owner's existing WSL proxy environment.

## 2026-07-13 — First authorized Jina Evidence captured and reviewed

After the explicit private-proxy patch was committed, the Owner repeated the
Jina-only fixed plan with a temporary process-environment Key and the dedicated
proxy variable. The command completed with `captureOutcome=fixed_plan_complete`
and wrote one canonical Evidence Snapshot. No Key, Proxy URL or Provider raw
response was retained.

Review evidence:

```text
Provider / state                         jina / captured
fixed operation budget                   3 / 3
operations                               embedding_1024, embedding_2048, rerank
captured dimensions                      1024, 2048
Evidence Schema                          valid closed v1
Evidence bytes                           canonical
Evidence SHA-256                         e0c1ccd82b1a3d09ac65ea37dd7e18c36e06d9cf3b57cf235f150b273436d1a8
directory / file mode                    0700 / 0600
forbidden dynamic ID/header/proxy keys   none
URL values in Evidence                   none
```

The Snapshot was moved out of the repository to the operator Evidence Store at
`~/.local/share/mm-chat/provider-evidence/jina-20260713T013005Z`; the repository
now ignores `provider-capture-*/` as a second defense against accidental
Evidence commits. This is an initial operator review, not the two-reviewer
Freeze approval. Public Jina Fixtures remain `draft/blocked` because immutable
build, inference region, account limits, current terms and SLA are unresolved.
Next: execute the separately authorized staged MinerU Submit Capture; do not
upload its Signed URL or poll.

## 2026-07-13 — MinerU staged Evidence captured; exposed Token revoked

The Owner executed the MinerU-only staged plan through the reviewed explicit
private proxy. The Harness made one local-upload Batch Submit and intentionally
made zero Signed Upload and zero Poll calls. The resulting Evidence passed the
closed v1 Schema, canonical-byte, budget, redaction and permission checks:

```text
Provider / state                         mineru / staged_after_submit
Submit / Signed Upload / Poll            1/1, 0/0, 0/0
Batch ID present / Signed URL count      true / 1 (presence/count only)
Evidence SHA-256                         a47a34559fbd262ba29a59181fe7b3ecedc8f1652305b2f4a22afdb342d23b46
directory / file mode                    0700 / 0600
forbidden dynamic ID/header/proxy keys   none
URL values in Evidence                   none
```

The Snapshot was moved to
`~/.local/share/mm-chat/provider-evidence/mineru-20260713T014528Z`; no Capture
directory remains in the repository.

During operator input, a shell helper built from repeated `read -s -n1` calls
attempted partial display of a long Token. Clipboard buffering and terminal echo
transitions exposed substantial Token fragments in scrollback/message output.
The Owner confirmed revocation immediately after detection; clipboard and
terminal cleanup was instructed but not independently verified. The revoked
credential was never written to Evidence, Git or Harness logs, so the redacted
Capture remains usable.

The partial-echo pattern is now forbidden. Future operator Key entry must use a
single complete no-echo prompt or a controlled Secret injection path; no Key
prefix/suffix confirmation is permitted. Both Provider Captures are now present,
but their Fixtures remain `draft/blocked`: this operator review does not replace
independent Terms, immutable-build, region, account-limit, recovery and Freeze
review.

## 2026-07-13 — Evidence-to-Fixture promotion readiness audited

Both Git-external Capture Snapshots were compared field-by-field with the four
checked-in Public Draft Fixtures before any Lifecycle or Source Label change.
The audit found one decisive mismatch: the MinerU Fixture models Remote URL
`/api/v4/extract/task`, while the real Capture proves Local Upload Batch
`/api/v4/file-urls/batch`. The Capture therefore cannot verify the current
MinerU `submit/poll/result` cases.

The production candidate is now Local Upload Batch because source files remain
private behind MinIO/Object Gateway and must not be exposed through public
Source URLs. Only Allocate is proven; Signed Upload Host, Batch Poll, Result
Download, cancel/query-by-key and crash recovery remain blocked pending a new
closed Operation Set and reviewed evidence.

Jina success Shapes map cleanly to the 1024, 2048 and Rerank drafts, but the
Harness intentionally retained Summary rather than raw Vector/Body bytes. The
existing synthetic response Cases were not relabeled as `redacted_capture`.
Summary-derived Capture semantics must first become explicit in the closed
Schema, or a separately approved safe Wire-body Capture must be performed.

Eight public authorities were snapshotted outside Git under
`~/.local/share/mm-chat/provider-evidence/public-sources-20260713T015502Z` and
bound by SHA-256 in
`docs/contracts/provider-capture-promotion-readiness.md`. Jina's current Legal
page says Elastic terms govern after the acquisition and warns old Jina terms
may be stale; model-page licenses do not establish Hosted API license. MinerU
API Docs prove technical flow only and expose no reviewed retention/deletion/
training/license/SLA authority. All Governance Terms, Region, immutable Build,
account limits and stable error coverage remain `unknown`.

```text
Provider contract + capture tests             120 passed
Python non-integration suite                   205 passed / 2 deselected
Python coverage                                90.19% (>= 90%)
Ruff / format / Mypy                           passed / 23 source files
pip-audit --skip-editable                      no known vulnerabilities
security scanner on rag/tools                  0 findings
Public Draft Fixture diff                      none
Governance / runtime activation                none
```

The readiness report is a fail-closed Candidate Mapping, not a Promotion. Next:
extend the MinerU Local Batch operation contract and define reviewed
Summary-derived Capture Case semantics before modifying any Fixture lifecycle.

## 2026-07-13 — MinerU Local Batch draft-contract plan locked

Before changing the shared Provider Contract Schema, the next slice was written
to `docs/contracts/mineru-local-batch-draft-contract-plan.md`. It creates a
distinct `mineru_local_batch` Provider Kind and exact six-operation surface so
the existing `mineru_async` Remote URL Draft cannot be silently repurposed.

Only `allocate_upload` may be `observed` in this slice. Its safe Wire example is
`public_schema_synthetic`; the existing Capture proves only a redacted Summary
and is not promoted into a fake full-body Capture. Upload, Batch Poll, Result
Download, Cancel, and Query-by-key stay `unknown` without invented Method, Path,
Request, Response, or Signed Host fields. Runtime adapters, Governance,
Dispatch, `011`, and `012` remain out of scope and disabled.

## 2026-07-13 — MinerU Local Batch public Draft implemented and reviewed

The shared closed Schema now recognizes `mineru_local_batch` separately from
the existing `mineru_async` Remote URL Flow. The new Fixture fixes the six
operation phases but exposes Wire fields only for `allocate_upload` at
`POST /api/v4/file-urls/batch`. Its one success body is explicitly
`public_schema_synthetic`; `CAPTURE_BODY_NOT_RETAINED` remains a lifecycle
blocker, and no Signed URL from real Evidence was copied into Git.

The semantic validator closes the Allocate request fields, one-file fixture,
model consistency, response envelope, Batch ID, and one credential-free
synthetic upload URL. Regressions reject Remote/Local cross-labeling, extra
request/response fields, unsafe filenames, dynamic query-bearing URLs,
secret-like fields, guessed Wire metadata on unknown operations, and fake
replay of an unknown stage. The old Remote Fixture remains byte-unchanged.

```text
Provider Contract targeted tests                 70 passed
Python non-integration suite                     216 passed / 2 deselected
Python coverage                                  90.19% (>= 90%)
Ruff check / format                              passed / 37 files
Mypy                                             24 source files / no issues
pip-audit --skip-editable                        no known vulnerabilities
security scan on changed support/fixtures        no findings
full tests scanner                               2 existing synthetic fixture strings
Runtime Registry / Governance / Dispatch         unchanged / disabled
```

The Fixture remains `draft/blocked`; this slice does not freeze Upload Host,
Batch Poll, Result Download, recovery behavior, stable errors, immutable build,
Region, Terms, or SLA. Next: design a separately authorized staged
Upload/Poll/Result Capture before any Adapter implementation or lifecycle
promotion.

## 2026-07-13 — MinerU Lifecycle Capture Harness plan locked

The next Provider intake slice was persisted before implementation at
`docs/contracts/mineru-lifecycle-capture-harness-plan.md`. It keeps the original
Submit-only Harness and Evidence v1 stable, and introduces a separate
no-network-by-default Lifecycle CLI plus Evidence v2 for one in-process
Allocate, Signed PUT, bounded Poll, and Result ZIP Download chain.

Dynamic URLs remain untrusted Provider data. Upload and Download require exact
documented HTTPS hosts/path prefixes, default port, no userinfo/fragment/control
characters, redirects, Auth replay, Cookie replay, or caller-supplied Host.
Signed Query values, Batch/Trace IDs, Provider errors, ZIP entry names/content,
and response bytes never enter Evidence. Poll and archive budgets are fixed in
code, every failure state is redacted, and no stage automatically retries or
resubmits. This plan does not authorize a real call or any Runtime activation.

## 2026-07-13 — MinerU Lifecycle Capture Harness implemented and reviewed

The isolated `tools.provider_capture_mineru_lifecycle` CLI now defaults to a
zero-network plan and requires explicit `--execute` plus a process-environment
`MINERU_API_KEY`. It performs one in-process Allocate, Signed PUT, bounded Poll,
and Result ZIP Download chain; no caller can supply a Stage, URL, Host, Batch
ID, file, retry, timeout, or call budget. The original Submit-only CLI and all
historical Evidence v1 validation remain compatible.

Security-sensitive logic is split into closed modules for orchestration, HTTP,
response shapes, dynamic targets, ZIP validation, and Evidence v2. Upload and
Result targets require exact documented HTTPS authorities/path shapes and no
redirect; Batch IDs are constrained to one safe path segment. Upload/Download
drop Auth, Cookie, and inherited Content-Type. Poll is capped at 60 calls with
fixed 5-second spacing and no network retry.

Result ZIPs are consumed only in memory and capped at 32 MiB compressed, 256
entries, 128 MiB aggregate uncompressed, 64 MiB per entry, and a 200:1 ratio.
Encrypted, symlink, duplicate, absolute/traversal, CRC-invalid, or incomplete
archives fail closed. Evidence v2 stores only fixed stage state/count/Hash/
presence summaries; Signed URL/Query, Batch/Trace ID, Provider Error, file name,
Entry Name/Content, response bytes, Key, and Proxy remain absent.

```text
Lifecycle Capture targeted tests                43 passed
Python non-integration suite                     259 passed / 2 deselected
Python coverage                                  90.19% (>= 90%)
Ruff check / format                              passed / 46 files
Mypy                                             33 source files / no issues
pip-audit --skip-editable                        no known vulnerabilities
security scan on tools/new support/tests         no new findings
quality scan on new modules/tests                0 errors / 0 warnings
real Provider requests                           0
Runtime Registry / Governance / Dispatch         unchanged / disabled
```

This implementation does not promote the Local Batch Fixture: Upload/Poll/
Download still lack real reviewed Evidence, and immutable build, Region, stable
errors, Terms, Retention, License, and SLA remain unknown. Next: commit the
reviewed no-network implementation, then obtain a newly issued MinerU Token and
separately authorize exactly one Lifecycle Capture.

## 2026-07-13 — First MinerU Lifecycle Capture stopped at Download

The Owner injected a newly issued MinerU Token through a complete no-echo prompt
and authorized one fixed Lifecycle Capture. Allocate and Signed PUT returned
success; four Poll calls observed one each of `waiting-file`, `pending`,
`running`, and `done`. The one allowed Result Download ended in
`unknown_download`, so the CLI wrote Evidence and returned exit code `3` without
retry or resubmit.

The Evidence hash is
`06edec92a8cbc3dbf96dd261ccfa88cea34b08de703eaefd8ffb088c1aabc4b1`; directory
and file modes are `0700/0600`, and the immutable bytes were moved to the
Git-external Evidence Store. Offline review confirmed budgets `1/1/4/1`, no
dynamic target/identifier/error/archive/key fields, and no ZIP success
metadata. Because the old producer intentionally collapsed all non-contract
exceptions, the exact transport cause is unrecoverable and must remain unknown.

The follow-up patch preserves this legacy v2 Snapshot and adds an optional,
closed `transportFailureClass` only for future HTTPX transport-loss states.
Classification uses hard-coded `isinstance()` branches and never serializes an
exception message, class name, Request, URL, proxy, or identifier. Non-HTTPX
programming failures now escape to the CLI's fixed `CAPTURE_FAILED` path instead
of producing misleading Provider Evidence. No real retry was performed and all
Fixtures, Governance, Dispatch, and Runtime handlers remain blocked.

The diagnostic follow-up then passed the complete local gate:

```text
Lifecycle targeted tests                    58 passed
Python non-integration suite                274 passed / 2 deselected
Python coverage                             90.19% (>= 90%)
Ruff check / format                         passed / 46 files
Mypy                                        passed / 29 source files
pip-audit --skip-editable                   no known vulnerabilities
legacy v2 Evidence validation               passed; bytes/hash unchanged
security scan on rag/tools                  0 findings
full unit-test security scan                2 existing synthetic fixtures only
independent final review                    P0/P1/P2 = 0/0/0
real Provider retries after incident        0
Runtime Registry / Governance / Dispatch    unchanged / disabled
```

## 2026-07-13 — Second MinerU Lifecycle Capture isolated Download connect error

Before the authorized run, one multiline shell paste was split by the terminal.
It executed only the CLI dry-run, made zero Provider requests, created no
Evidence directory, and was replaced by a private temporary no-echo helper. The
helper was removed immediately after use; the Token existed only in that child
process environment.

The separately authorized Capture then used the fixed plan once. Allocate and
Signed PUT succeeded; two Poll calls observed `waiting-file` and `done`; the
Result URL passed the fixed CDN Target Gate. The sole Download ended as
`unknown_download` with the new closed `transportFailureClass=connect_error`.
The CLI wrote Evidence and returned terminal exit code `3`; no retry, resume,
third Submit, Fixture mutation, Governance apply, or Runtime activation occurred.

```text
Actual Allocate / Upload / Poll / Download       1 / 1 / 2 / 1
Evidence SHA-256                                 7041a1c09e2f741875ffccb11d97ea806fc63e90e059f390124a1f953f047b55
Evidence directory / file mode                   0700 / 0600
Evidence schema validation                       passed
Transport failure class                          connect_error
Dynamic URL / ID / error / Token retained        none
Evidence storage                                 Git-external
```

`connect_error` proves only that HTTPX failed in the connection path. It cannot
distinguish the local Private Proxy, its upstream tunnel, TCP, DNS, TLS, CDN
reachability, or a transient remote condition. A third full Capture would only
repeat Allocate/Upload and is therefore forbidden until an independently
authorized, credential-free CDN/proxy connectivity probe resolves this branch.

## 2026-07-13 — Direct MinerU Capture crossed transport and hit Download gate

Under the Owner's explicit authorization, a credential-free comparison probe
ran before any further Submit. WSL reached the Private Proxy TCP listener and
the Proxy completed TLS to `mineru.net`, but Proxy-to-CDN TLS ended with an
unexpected EOF. Direct TLS succeeded for `mineru.net`, the fixed OSS Upload
Host, and the fixed CDN Result Host. No Token, API request, upload, or dynamic
Result URL was used by this probe.

The Owner then supplied a one-time Token and explicitly requested direct
execution. It was injected through a no-echo PTY stdin path, never placed in a
command, file, Evidence, or stdout, and was removed with the child process. The
Harness ignored generic proxy variables and ran one all-direct fixed Capture.
Allocate, Signed PUT, and two Poll calls succeeded; Download crossed the prior
connect boundary but ended as `download_failed`. The Token was not reused and
must be revoked by the Owner.

```text
Actual Allocate / Upload / Poll / Download       1 / 1 / 2 / 1
Evidence SHA-256                                 ec5ad91cf1c062d713aa70a62381f2d36b86810ec59c6ba92f93419f3d62dc96
Evidence directory / file mode                   0700 / 0600
Evidence schema validation                       passed
Outcome / Download state                         download_failed / failed
Dynamic URL / ID / error / Token retained        none
Evidence storage                                 Git-external
```

The historical failed Snapshot cannot identify whether the rejected gate was
HTTP status, encoding, content type/length, archive size, or ZIP shape. The
public MinerU docs snapshot still hashes to
`6b72fd975b37f5d64996bdd97d97f755b7de82602f7e6c1f37cc27b9f51e24fa` and
documents the expected `full.md`, `*_content_list.json`, `*_middle.json`, and
`*_model.json` outputs, but it cannot reconstruct the lost response.

The follow-up implementation therefore adds only a closed, non-authoritative
`downloadFailureClass` derived from stable internal `CaptureError` codes. It
keeps legacy v2 failed Evidence valid, rejects unknown enums and wrong-state
placement, records no response detail, and does not authorize another Capture
or Fixture promotion.

```text
Lifecycle targeted tests                    65 passed
Python non-integration suite                281 passed / 2 deselected
Python coverage                             90.19% (>= 90%)
Ruff check / format                         passed / 46 files
Mypy                                        passed / 29 source files
pip-audit --skip-editable                   no known vulnerabilities
all three Lifecycle Evidence snapshots      valid under current v2 schema
security scan on rag/tools                  0 findings
full unit-test security scan                2 existing synthetic fixtures only
independent final review                    P0/P1/P2 = 0/0/0
additional Provider calls after direct run  0
Runtime Registry / Governance / Dispatch    unchanged / disabled
```

## 2026-07-13 — Archive failure classification added after Token revocation

The Owner supplied a fresh Token only through the local no-echo direct Capture
helper. One fixed `1/1/2/1` lifecycle again passed Allocate, Signed PUT, and Poll
`waiting-file/done`; Download recorded `downloadFailureClass=archive_invalid`.
Evidence SHA-256 is
`6d227220d52b944a0824a779d00bc595fd3b6f086cdc1753f8e1719c363a4dd6`, stored
Git-external with `0700/0600`. The helper was deleted and the Owner confirmed
Token revocation before implementation continued.

This historical Evidence proves the response crossed status, encoding,
content-type, content-length, compressed-size, and transport gates before ZIP
validation failed. It cannot identify invalid ZIP bytes, CRC, unsafe Entry,
expanded-size/ratio, or a missing required Artifact, and it was not rewritten.

The offline follow-up introduces `ArchiveValidationError` carrying one hard-coded
`archiveFailureClass`, while its public error remains `MINERU_ARCHIVE_INVALID`.
Evidence accepts the new field only under `download_failed + archive_invalid`;
legacy v2 Snapshots without it remain valid. Entry names/content and raw archive
metadata remain absent. The expanded tests also exposed and fixed a pre-existing
flaky Mock ZIP SHA caused by current-time ZIP timestamps; fixtures now use one
fixed timestamp.

Independent review found and closed two additional P1 boundaries before merge:
directory entries can no longer impersonate required files, and unsupported ZIP
compression is normalized to the closed `unsupported_compression` class instead
of escaping as `NotImplementedError`.

```text
Lifecycle targeted tests                    75 passed
Python non-integration suite                291 passed / 2 deselected
Python coverage                             90.19% (>= 90%)
Ruff check / format                         passed / 46 files
Mypy                                        passed / 29 source files
pip-audit --skip-editable                   no known vulnerabilities
all four Lifecycle Evidence snapshots       valid under current v2 schema
security scan on rag/tools                  0 findings
full unit-test security scan                2 existing synthetic fixtures only
independent final review                    P0/P1/P2 = 0/0/0
additional Provider calls after revocation  0
Runtime Registry / Governance / Dispatch    unchanged / disabled
```

## 2026-07-13 — Cloud v4 layout role fixed; Lifecycle Capture completed

The first Capture after archive subclassing returned
`archiveFailureClass=missing_middle_json`. Three parallel read-only agents
compared live Evidence, project contracts, official Cloud v4 documentation,
Open-source output naming, and downstream reconstruction risk. The decisive
mismatch was semantic naming: Cloud v4 packages the Middle role as fixed
`layout.json`, while local output uses `middle.json` or `*_middle.json`. The
Harness had recognized only the local names and rejected a valid Cloud archive.

The fix adds `layout.json` to the Middle role matcher without weakening ZIP
safety, size, CRC, four-role Presence, Evidence v2, Promotion, Governance, or
Runtime boundaries. Mock Cloud layout and CLI exit-zero regressions pass; Local
middle compatibility remains covered.

Using the Owner-authorized one-hour Token through no-echo PTY stdin, one final
all-direct Capture completed successfully. Offline inspection confirms that the
target Evidence and the semantic `mm-chat` diff contain no Token, dynamic URL,
or dynamic identifier; this statement does not claim global terminal or
external-system provenance.

```text
Actual Allocate / Upload / Poll / Download       1 / 1 / 2 / 1
Capture outcome / CLI exit                       lifecycle_complete / 0
Download                                         200 application/zip
Archive bytes / entries                          2,344 / 6
Four semantic Role Presence                      true / true / true / true
Evidence SHA-256                                 5b4c3c8289c6c9ce8eec5f6bdc8af8fda60dea325376d55b7be62d72aaaa50e3
Archive SHA-256                                  484549392910218a94bc52598563734d23ffdd0c0dee4e5a2624329a469bdaa8
Evidence directory / file mode                   0700 / 0600
Evidence storage                                 Git-external
Runtime Registry / Governance / Dispatch         unchanged / disabled
```

The successful Summary proves the fixed synthetic Acquisition chain only. ZIP
Entry names/content, JSON Schema, Canonical IR equivalence, citation locators,
immutable build, Region, Terms, Retention, SLA, and Recovery remain blocked and
must not be inferred from Presence booleans.

```text
Lifecycle targeted tests                    78 passed
Python non-integration suite                294 passed / 2 deselected
Python coverage                             90.19% (>= 90%)
Ruff check / format                         passed / 46 files
Mypy                                        passed / 29 source files
pip-audit --skip-editable                   no known vulnerabilities
all six Lifecycle Evidence snapshots        valid under current v2 schema
security scan on rag/tools                  0 findings
full unit-test security scan                2 existing synthetic fixtures only
parallel diagnosis agents                   3 completed
independent final review                    P0/P1/P2 = 0/0/0
Runtime Registry / Governance / Dispatch    unchanged / disabled
```

## 2026-07-13 — MinerU Lifecycle Summary mapped to the blocked Draft

Three parallel xhigh read-only audits mapped the final Git-external Evidence,
Provider Contract Schema, and Governance/Recovery blockers. The Local Batch
Fixture now references only stable `redacted_capture_summary` metadata: upstream
HTTPS entrypoint, observed time, Evidence v2 version, and the canonical Evidence
SHA-256. The private Snapshot, Token, dynamic targets/IDs, raw bodies, and ZIP
Entry names/content remain outside Git.

Upload, Poll, and Download stay `support.state=unknown`; their stale
`*_NOT_CAPTURED` reasons were replaced by exact Wire/Body/Archive blockers.
Result Entry Schema/Content, Canonical IR, Citation Locator, Recovery,
Idempotency, immutable identity, Region, and Terms remain unresolved. The
Allocate response remains `public_schema_synthetic`, and the Fixture remains
`public_documentation + draft/blocked` with Runtime/Governance disabled.

Independent review first found that editable provenance labels could be used to
misrepresent Summary bytes. The fix requires Summary Version/Hash, validates the
exact Git-external bytes against the v2 Producer Schema, observed time, content
hash, and canonical JSON at Freeze, rejects Summary relabeling, and forbids every
non-Allocate Local Batch phase from entering Observed until a provider-specific
Fixture-grade Schema exists. It also closes dangling Support refs and Response
Cases on Unknown/Unsupported operations. Omission, relabel, arbitrary Wire,
invalid Producer Schema, observed-time drift, and non-canonical bytes now have
negative regressions.

```text
Provider Contract focused tests                    72 passed
Python non-integration suite                       296 passed / 2 skipped
Python coverage                                    90.19% (>= 90%)
Ruff check / format                                passed / 46 files
Mypy                                               passed / 18 source files
pip-audit --skip-editable                          no known vulnerabilities
security / quality scanners on changed code        0 findings
git diff --check                                   passed
independent final review                           P0/P1/P2 = 0/0/0
additional Provider calls / Token use              0 / 0
Runtime Registry / Governance / Dispatch           unchanged / disabled
```

Next: define the Offline Parser, Canonical IR, deterministic artifact manifest,
parent/child chunking, and Citation Locator implementation slice without
reopening Provider Capture or the production External Gate.

## 2026-07-13 — Offline Parser and Canonical IR C1 plan locked

Four parallel xhigh read-only audits were consolidated into the executable C1
plan. The locked slice is strictly offline: no Postgres, Redis, MinIO, Provider,
Runtime Handler, Dispatch, or migration `011/012` activation. Native parsing,
the test-only MinerU artifact normalizer, Canonical IR/Source Locator v2,
Normalization Map, deterministic manifests, quality gates, and hierarchical
chunking now have one ordered implementation contract.

Review remediation closed the unsafe or ambiguous edges before lock: source-
derived payload classification, safe test-output cleanup, Anchor/Fragment/View
locators, raw-byte and Unicode-scalar normalization mapping, exact chunk span
reconstruction, integer geometry, JCS/hash ID DAGs, archive/OOXML ambiguity,
per-child sandbox cleanup, Online Purge versus Retained-copy semantics, sealed
Deletion Authority/restore replay, and the future `012`/Evidence/Citation v2
boundaries. The parent Phase 15.2C plan and persistence contract were updated to
use the same `52,428,800`-byte limit and deletion terminology.

An initial xhigh review chain drove the design through multiple remediation
rounds. A fresh independent xhigh reviewer then re-audited the complete result
and closed at `P0/P1/P2 = 0/0/0`. No source code, Registry, migration, Provider
fixture, credential, or production configuration changed.

```text
changed scope                                  mm-chat/docs only
Markdown Prettier                              passed
relative-link and document consistency checks  passed
git diff --check -- mm-chat                    passed
independent final review                       P0/P1/P2 = 0/0/0
additional Provider calls / Token use          0 / 0
Runtime Registry / Dispatch                    unchanged / disabled
migration head                                 010 (011/012 absent)
```

Next: implement **C1.1 Contract and Corpus** only—Closed Schemas, Hash/ID
envelopes, Stable Error Enum, Golden/Adversarial/Recipe Corpus, and cross-runtime
RFC 8785 vectors—before parallel Native Parser implementation begins.

## 2026-07-14 — C1.1 Contract and Corpus implemented

C1.1 now provides an installable, runtime-inert parser contract package with 18
Draft 2020-12 Closed Schemas. Strict test tooling rejects duplicate keys,
floats, unsafe integers, BOM, NUL, surrogates, invalid UTF-8, non-canonical JCS,
unknown fields, invalid unions, and mismatched hashes before later Parser code
may depend on the contract. All 22 Stable Error branches and all 24 Logical Hash
Envelope kinds are frozen and checked against Python, Go 1.22, and Node 22.

The checked-in corpus contains 49 source fixtures plus 27 deterministic binary
recipes across text, Markdown, HTML, CSV, OOXML, native/adversarial PDF, archive,
encoding, limit, and synthetic MinerU cases. `layout` and `middle` are distinct
single-role synthetic artifacts; their pair validator proves source/profile/
page/geometry agreement without claiming live Provider compatibility or
Canonical IR output. Corpus coverage, license basis, raw bytes, SHA-256,
recipes, expectations, and aggregate hash are closed and reproducible.

Two review/remediation rounds converted descriptive rules into executable
test-only contracts: normalization exact-cover and legal split boundaries,
Map-to-Locator projection, Child-to-Parent Fragment/View subset, references and
DAGs, table grids, chunk reconstruction cardinality, full Canonical Manifest
artifact binding, and real A–F Logical ID/Hash recomputation. The production
registries remain empty; no DB, Redis, MinIO, Provider, migration, Docker, or
runtime dependency changed.

```text
packaged schemas / logical envelopes                 18 / 24
source fixtures / deterministic binary recipes       49 / 27
cross-runtime JCS + logical-ID cases                  89 / 3 runtimes
focused C1.1 tests                                    380 passed
full Python suite                                     676 passed / 2 skipped
Python coverage                                       90.33% (>= 90%)
Ruff check / format / strict Mypy                     passed / passed / passed
pip-audit --skip-editable                             no known vulnerabilities
offline wheel build / verifier                        passed / 18 schemas
security scanner                                     0 findings
independent initial review                            P0/P1/P2 = 0/4/0
independent final review                              P0/P1/P2 = 0/0/0
additional Provider calls / Token use                 0 / 0
Runtime Registry / Dispatch / migration               unchanged / disabled / 010
```

Next: implement **C1.2 Router and Sandbox Protocol** only. Keep Native Parser,
Provider Adapter, migration `011/012`, and production Handler activation closed
until their ordered gates are complete.

## 2026-07-14 — C1.2 Router and Sandbox Protocol implemented

C1.2 now provides a runtime-inert `offline_parser` package. Magic/Container
precedes structured preflight and MIME/Extension reconciliation; all 49 frozen
Corpus route/error expectations match without Binary-to-TXT, OOXML-to-ZIP-text,
PDF-to-Provider, Unicode replacement, or best-effort fallback. Exact MMCP v1
frames bind Invocation, Config, Source length/hash, deadline, caller result
limit, and a domain-separated request hash; Controller-local Cancel and
Sandbox-Unavailable outcomes remain forbidden on the wire.

The Sidecar runs only under the isolated `parser-c1` Compose Profile as PID 1,
UID/GID `10002:10001`, with no network, credentials, database, Redis, MinIO,
Provider, host mount, or Runtime Registry. A clean interpreter is created in a
Supervisor-prebuilt Process Group; Parent verifies PID/PGID and opens pidfd,
Child re-confirms Group, installs RLIMIT and a hashed classic-BPF Seccomp filter,
then reports the compiled Filter Hash before receiving Source bytes. `clone3`
returns `ENOSYS`; Namespace clone flags, new sessions/groups, ptrace, namespace
operations, and sockets are denied. OOM, timeout, cancel, double-fork, and a
bounded fork bomb prove kill/reap and Residual-process Restart behavior.

The test-output root accepts no cleanup path or ambient temp variable. It binds
Run ID, Boot ID, device/inode, PID/start time, mode, exclusive flock, heartbeat,
and quota ledger to retained dir FDs. Files are reserved before
`O_EXCL|O_NOFOLLOW` creation; cleanup removes only registered children.
Unexpected symlinks, marker drift, active locks, ambiguous PID identity, or
unverified objects are retained rather than followed or deleted.

```text
C1.2 focused tests                              190 passed
full Python suite                               866 passed / 2 skipped
Python coverage                                 91.16% (>= 90%)
Ruff / format / strict Mypy                     passed / passed / passed
pip-audit --skip-editable                       no known vulnerabilities
security scanner                               0 findings
quality scanner                                passed (0 errors; advisory warnings only)
Python/Go/Node JCS                              89 cases / 3 runtimes passed
offline wheel / contract verifier              passed / 18 schemas
Docker parser-c1 image + Compose smoke          passed
parser config hash                              5d24254518a9f6333812e0906e3c111070a93e2312b62c2e2ef025f2804a971f
child compiled Seccomp SHA-256                  9bcecc9c30f208fbd2c21192d9f39b352b36110854fc5d6da6c99007cfa8e58e
Runtime Registry / Dispatch / migration         unchanged / disabled / 010
Provider calls / credentials                    0 / 0
```

Next: implement **C1.3 Native Parsers** behind the existing C1.2 route,
protocol, process, resource, and cleanup gates. Keep Provider, Registry,
Dispatch, Postgres/Redis/MinIO, migrations `011/012`, and production Handler
activation closed.

## 2026-07-14 — C1.3A TXT / Markdown / HTML Native Parsers implemented

C1.3A adds deterministic TXT, Markdown, and HTML parsing only inside the
existing exec-isolated Child, after RLIMIT, `no-new-privileges`, and the hashed
Seccomp filter are active. Decoding is frozen as BOM -> UTF-8 -> GB18030 with
compact Raw-byte/Scalar/Line indexes. Markdown uses the pinned
`markdown-it-py==4.2.0` CommonMark + Table profile; HTML uses a hardened
`HTMLParser(convert_charrefs=False)` policy that rejects active content and
external-fetch constructs. The new direct dependency
`markdown-it-py==4.2.0` and its exact-locked `mdurl==0.1.2` dependency are
MIT-licensed.

The Child emits a closed, canonical `parser-native-artifact.v1` internal frame.
The Supervisor validates its Closed Shape, JCS bytes, lengths, hashes, limits,
format, and exact Source binding without decoding or reparsing Source bytes.
This is deliberately not `canonical-ir.v2`: MMCP success remains frozen to that
future contract, so the Sidecar still returns zero-body `FORMAT_UNSUPPORTED`
and `stageable == false`. Runtime Registry/Dispatch, Provider access, database,
Redis, MinIO, migrations `011/012`, and production handlers remain closed.

Independent implementation streams owned Markdown and HTML separately; a
security reviewer then found and drove fixes for inline Raw HTML Locator drift,
forged Controller-only Child errors, missing Parent-side Artifact/Source
validation, Raw HTML policy inconsistency, unmatched-backtick quadratic work,
Locator-index memory amplification, duplicate Router decoding, and the 10 MiB
ASCII Sandbox memory limit. A final independent review closed at
`P0/P1/P2 = 0/0/0`. The reusable allocation rule was recorded in
`.trellis/spec/guides/agent-orchestration.md`: parallelize only bounded,
disjoint work with net quality benefit, and allocate an independent reviewer
only for a stable, risk-bearing diff.

```text
full Python suite                               1069 passed / 2 skipped
Python coverage                                 91.19% (>= 90%)
Ruff / format / strict Mypy                     passed / passed / passed
pip-audit --skip-editable                       no known vulnerabilities
security scanner                               0 findings
module / quality scanners                       passed (advisory warnings only)
Python/Go/Node JCS                              89 cases / 3 runtimes passed
offline wheel / contract verifier              passed / 21 artifacts / 18 schemas
Docker parser-c1 image + Compose smoke          passed; isolated resources removed
parser config hash                              8a72668218932f6af95d3b6276646304451d7f9ea59ff658ca7887d925e83ea7
independent final review                        P0/P1/P2 = 0/0/0
Runtime Registry / Dispatch / migration         unchanged / disabled / 010
Provider calls / credentials                    0 / 0
```

No complete reproduction transcript is required; the decisive boundaries,
review remediations, and verification evidence above are retained for the next
slice. Next: implement **C1.3B DOCX / PPTX / XLSX / CSV Native Parsers** behind
the same Child and internal-artifact gate. Do not advance to C1.3D MinerU,
Canonical IR, or runtime activation before the intervening slices pass.

## 2026-07-14 — C1.3B DOCX / PPTX / XLSX / CSV Native Parsers implemented

C1.3B upgrades the closed Child-internal DTO to `parser-native-artifact.v2`.
Text Formats retain one decoded Raw File Source Unit; OOXML uses binary Raw Unit
`0` plus canonical URI-ordered Part Units. Node and Fragment Locators bind the
exact Source Unit, and Dispatch recomputes every used Part-local Byte/Scalar/
Line position before the Artifact crosses the Child/Supervisor frame.

CSV now uses a fixed comma/double-quote FSM with no Sniffer or Header inference.
DOCX preserves paragraph/list/table/footnote/endnote structure; unsupported
Header/Footer references fail closed rather than dropping text. PPTX preserves
slide order, shapes, tables, notes, and exact-rational geometry. XLSX preserves
sheet/row/cell/shared-string/formula/cached/merge/hidden structure without
formula execution.

Router and the three OOXML Parsers consume one `ValidatedOpcPackage`; there is
no second ZIP/XML admission. The capability reconciles EOCD, Local/Central
Header, optional Descriptor, CRC/Size, disk start, compression and expanded
budgets; rejects ZIP64, recursive Archive, path aliases, Macro/OLE, invalid
Content Types and open Relationship graphs; and parses XML through strict
source-aware Expat without DTD/custom Entity/PI/XInclude or external fetch.

Independent review initially found `P1=3/P2=3`: unbounded integer conversion,
Presentation-root active content, Header/Footer text loss, Central disk-start,
Relationship exact-type, and Profile self-binding gaps. All were fixed and two
read-only remediation reviews closed at `P0/P1/P2 = 0/0/0`. The same integer
guard was applied to canonical JSON, and `canonical.py`, `errors.py`,
`config.py`, and `profile.py` were added to component source inventories.

```text
full Python suite                               1408 passed / 2 skipped
Python coverage                                 92.32% (>= 90%)
Ruff / format / strict Mypy                     passed / passed / passed
pip-audit --skip-editable                       no known vulnerabilities
security / quality / module scanners            passed; 0 security findings
Python/Go/Node JCS                              89 cases / 3 runtimes passed
offline wheel / contract verifier              passed / 21 artifacts / 18 schemas
wheel Native runtime modules                    6 required modules present
Docker parser-c1 image + Compose smoke          passed; isolated containers removed
parser config hash                              6251a7a71ec35d7d55e030b8ca1ef49da8995257734a76e8cd6864c25d88d8c3
independent final review                        P0/P1/P2 = 0/0/0
Runtime Registry / Dispatch / migration         unchanged / disabled / 010
Provider calls / credentials                    0 / 0
```

No complete reproduction transcript is required; the decisive flow, review
remediations, and final gates above are sufficient process evidence. Next:
implement **C1.3C Native PDF safe subset and `MINERU_REQUIRED` classifier**.
Keep C1.3D MinerU, C1.4 Canonical IR, Provider/network access, production
Registry/Dispatch, Postgres/Redis/MinIO, migrations `011/012`, and staging
closed.

## 2026-07-14 — Standalone frontend live cutover smoke completed

Docker Desktop registry access was restored through its explicit HTTP/HTTPS
proxy. The standalone frontend image then built from `mm-chat/frontend/` with
the server-only build contract (`/mm-api` and the private `backend:8080`
upstream). Both Node build stages are pinned to the exact base-image digest
observed during the successful build.

The first host publications on ports `3000` and `3100` failed before container
startup. Windows reported `WSAEACCES`; `netsh` proved that the machine excludes
the full TCP range `2893-3692`. The ignored local environment was moved to
`FRONTEND_PORT=18080` with a matching `NEXT_PUBLIC_SITE_URL`. This is a host
port reservation issue, not WSL memory pressure or an application failure.

Live Windows-host evidence:

```text
GET  http://127.0.0.1:18080/               200 OK
GET  http://127.0.0.1:18080/mm-api/ready  ready
frontend container health                 healthy / failing streak 0
backend database / redis / storage        ready / ready / ready
```

The first `verify-standalone.sh --full` run then exposed a gate defect: it used
the host's generic Python 3.10 and treated the PEP 735 `dev` dependency group as
an optional extra. The verifier and direct-development instructions now require
Python 3.13 and install with `pip install -e . --group dev`.

The repaired verifier passed from a fresh isolated copy with no original-root
access:

```text
frontend format / lint / typecheck           passed (6 legacy warnings)
frontend tests / production build            795 passed / passed
Go format / vet / full tests                 passed / passed / passed
RAG Ruff / format / strict Mypy              passed / passed / passed
RAG full tests                               1408 passed / 2 skipped
standalone verification                      passed (full)
```

The live smoke and isolated-copy gate close install, test, build, and local
runtime independence. Backup/restore, browser visual-regression, and final
rollback verification remain required before any original-root deletion plan.

## 2026-07-15 — Server model, skill, and plugin send path restored

The server-mode composer showed no selectable model and rejected both skill and
plugin menus. The model failure came from a split configuration boundary: Go
owned `PROVIDER_*`, while `/api/config` had no browser-safe provider/model
metadata. Compose now passes only provider type/name/model metadata into the
frontend. `PROVIDER_API_KEY` remains exclusive to the Go backend.

Selected text skills now resolve deterministically before the server send and
join the effective system instruction. Plugins use a split trust path: Go calls
the configured model for a bounded non-streaming tool plan, the browser rejects
any call not present in the active installed set, the existing hardened Next
route executes the plugin with encrypted browser auth, and a bounded 64 KiB
untrusted-data context feeds the normal Go final stream. Search and reasoning
remain explicitly gated because their server contracts are not part of this
slice.

Security and failure boundaries:

- planning accepts at most 32 tools/calls, a 16 KiB prompt, 128-byte names,
  2,048-byte descriptions, and 32 KiB parameter/argument JSON;
- provider errors never include response bodies or provider credentials;
- duplicate or unoffered function names fail closed before execution;
- plugin auth/config is absent from the Go planning payload;
- plugin execution errors remain `status: error`; final answers are instructed
  not to claim success;
- plugin results are explicitly untrusted and truncated without breaking UTF-8
  or exceeding 64 KiB.

Verification evidence:

```text
Go targeted tests / vet                         passed / passed
frontend format / ESLint / typecheck            passed / passed / passed
frontend full tests                             805 passed (174 files)
frontend local + Docker production builds       passed / passed
standalone structure / full clean-copy gates    passed / passed
RAG clean-copy tests                            1408 passed / 2 skipped
live /api/config model list                     SERVER_DEFAULT:gpt-5.5
live Go tool plan                               getCurrentWeather(Shanghai)
live Next plugin execution                      Weather response returned
live final Go SSE                               completed, 187 tokens
persisted final answer reload                   passed
isolated Plugin smoke conversation cleanup      DELETE 1
frontend/backend container health               healthy / healthy
```

Chromium was not installed in the available Playwright runtime, so this slice
does not claim a new visual-regression screenshot. Component composition tests,
production rendering build, live HTTP routes, persistence reload, and container
health passed. The owner should hard-refresh `http://127.0.0.1:18080` and use
the existing model, skill, and plugin buttons; no Docker restart is required.

### Browser plugin CSRF false positive and remediation

The first owner browser retry proved that model selection, message send, and Go
tool planning worked, but `/api/plugins/execute` returned
`CSRF_ORIGIN_BLOCKED`. The guard compared the browser `Origin` on external port
`18080` with Next's container-internal `nextUrl.origin` on port `3000`, so a
legitimate relative same-origin fetch looked cross-origin only after Docker
port publication.

The guard now uses the HTTP `Host` as the default external origin authority and
uses forwarded host/proto only when `TRUST_PROXY_HEADERS=true`. The original
cross-site rejection tests remain green, and a new regression models
`Origin/Host=http://127.0.0.1:18080` with an internal
`http://frontend:3000` request URL.

```text
browser-shaped CSRF probe (Origin + Sec-Fetch-Site)  200 OK
browser-shaped Weather plugin execution              real result returned
cross-origin rejection regression                    passed
frontend format / lint / typecheck                    passed
frontend tests                                        805 passed (174 files)
Docker production build                               passed
frontend / backend health                             healthy / healthy
```

## 2026-07-15 — Server reasoning path restored

The reasoning button remained behind the broad server-session tool gate, and
the server composer reused stale conversation config instead of the current
browser toggle. Even if `useReasoning` reached Go, the handler discarded it
before the provider boundary. The server-mode button now updates the current
chat config, the stream request forwards the boolean, and the OpenAI-compatible
provider emits `reasoning_effort: high` only when enabled. GPT-5 model names
also use the existing capability fallback when explicit model metadata is not
published.

Verification evidence:

```text
frontend format / lint / typecheck                    passed
frontend tests                                        805 passed (174 files)
Go tests / vet                                        passed / passed
Docker frontend / backend build                       passed / passed
frontend / backend container health                   healthy / healthy
live gpt-5.5 reasoning stream                         REASONING_OK
live persisted completion                             completed, 43 tokens
```

This toggle changes provider reasoning effort; it does not claim that hidden
chain-of-thought text is exposed by the provider.

## 2026-07-15 — Standalone parity sliced cutover plan created

Owner changed the remaining-migration operating mode: collect every unfinished
standalone/parity item into one new active plan, migrate one bounded group at a
time, and test each group immediately instead of repeatedly treating all
remaining work as a single giant full-suite migration.

New active authority:

```text
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

The new plan groups remaining work into G0-G10: plan guardrails,
conversation/message operations, helper/catalog routes, Auth/config/provider
settings/BYOK, plugins, Search, Voice/Image/Code jobs, Knowledge/RAG/citations,
Teams/Knowledge UI wiring, data-authority/route removal, and final
ops/visual/clean-copy/delete gates.

Future cutover implementation evidence should be recorded primarily in the new
process log, with this legacy process file reserved for major cross-reference
milestones.

## 2026-07-17 — G11.3c Server Default admin provider persistence cross-reference

Provider configuration authority was corrected per owner direction: the
production Server Default remains backend-owned, seeded by env/secret, editable
from Provider Settings, and persisted through the Go backend/Postgres path.

Detailed changed surfaces, verification commands, and residual risks are logged
in the active sliced process entry:

```text
mm-chat/docs/tracking/standalone-parity-sliced-process.md
# 2026-07-17 — G11.3c Server Default admin provider persistence
```

## 2026-07-17 — G11.3d Multi-provider backend authority cross-reference

Owner browser proof found two regressions after G11.3c: server mode hid the Add
button, and a successful seven-model response was overwritten by the one-model
selected list returned from the subsequent config save. The active sliced log
records the multi-provider backend lifecycle, model-list fix, stable local BYOK
configuration, source-build deployment, and live proxy/API proof:

```text
mm-chat/docs/tracking/standalone-parity-sliced-process.md
# 2026-07-17 — G11.3d Multi-provider backend authority and model-list repair
```

## 2026-07-17 — G11.3e Provider editor autosave cross-reference

Owner browser evidence showed successful model discovery but no provider PUT
after checkbox changes. The active sliced log records the serialized autosave
fix and the stale-response overwrite guard.

## 2026-07-17 — G11.4 Image-model chat dispatch cross-reference

The owner selected `gpt-image-2` in normal chat and received a failed empty
assistant card. Runtime evidence showed the model was incorrectly sent to the
chat-completions stream. The active sliced process log records the image
executor dispatch, assistant-attachment persistence, five-minute rewrite proxy
timeout, real provider proof, and temporary artifact cleanup.

## 2026-07-17 — G11.7 Native document indexing repair cross-reference

The original Knowledge UI advertised DOCX and other native formats, but the Go
bind contract forced every MIME through PDF-only MinerU governance. The active
sliced process log records MIME-aware processor authority, sandboxed Native
worker composition, automatic server governance/consent reconciliation,
unbound-upload cleanup, source-build deployment, and the live DOCX → Native →
Jina 1024 → Postgres active-document proof. Reprocess is closed to failed
Versions only; an already published active projection cannot be rebuilt inside
the same index generation.

## 2026-07-17 — G11.8 multilingual Knowledge recall cross-reference

The first owner strict-RAG question exposed that active Chinese document text
could not pass the `simple` lexical candidate function. Migration `025` adds
bounded phrase/bigram recall without weakening selected-collection, active
projection, reference-only, or Go hydration authorization fences. Live SQL
proved Chinese evidence queries changed from 0 to 1 candidate while unrelated
weather and unsupported Lindo-procedure questions remained at 0.

## 2026-07-17 — G11.9A Auto Knowledge closure cross-reference

Development mode now supplies the database-valid internal Session required by
Go evidence hydration. The strict refusal branch and frontend strict flags were
removed; relevant selected Knowledge augments the normal provider stream with
`[K]` citations, while a normal miss silently falls back to the model. The
active DOCX answered `研究方向是什么？` with “推荐系统” and `[K1]`, and an unrelated
weather query completed without a refusal/status card. Full runtime evidence
and the answer-consent projection-revision correction are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9C.1 Contextual retrieval cross-reference

Context-dependent follow-ups now receive a bounded standalone rewrite while the
original query remains an independent recall lane. Deterministic global RRF
fuses both keyword/CJK lists before the unchanged Go authorization/hydration
boundary. Tests, privacy constraints, open Dense/rerank slices, and rollback are
recorded in `docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9B Persistent Knowledge binding cross-reference

Selected Knowledge is now persisted once in Postgres conversation metadata and
reused by every later message. The dedicated server composer control, removable
chips, eight-collection validation, explicit unbind behavior, one-time legacy
migration, tests, source builds, and rollback boundary are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9C.1 Model-governance regression cross-reference

The selected `gpt-5.6-sol` model initially fell back with
`answer_governance_required` because startup had provisioned only the `.env`
`gpt-5.5` identity. Startup now merges all enabled backend-persisted models,
backfills their exact Answer governance/consent, and auto-provisions the same
set for future collections. Source-built live proof returned the expected
Knowledge answer and `[K1]`; detailed evidence and rollback are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9B.1 Knowledge visual-state cross-reference

The composer Knowledge icon now shares the neutral tool palette when no
collection is bound and becomes purple only after selection. Citation headers
no longer show the redundant `AUTO` badge. Verification and rollback are
recorded in `docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9B.2 Citation helper-copy cross-reference

Expanded Knowledge citations no longer repeat that verified evidence was used;
the heading, count, and source content already convey it. Dead locale keys were
removed, and the standalone frontend README now forbids UI copy that merely
restates visible state. Detailed checks are in
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9C.2 Dense retrieval cross-reference

The private Python RAG boundary now produces real Jina
`retrieval.query`/1024 vectors for Go. Migration `027` fuses the existing
keyword/CJK lane with selected-collection Dense references through RRF while
retaining publication, generation, visibility, deletion, and hydration
authorization fences. Live calibration added a conservative pre-rerank query
signal gate, and stopping Jina proved the original keyword answer still
completes with `[K1]`. Full evidence, cleanup, rollback, and the still-open C.3
rerank boundary are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9C.3 Rerank promotion cross-reference

The private Python boundary now calls `jina-reranker-v3` only after Go checks
exact query/collection consent and reauthorizes/hydrates the global candidate
set. Evaluated `>= 0.0` scores produce global Top5; rerank-only failure keeps
hybrid/RRF order. Live promotion passed real `applied`, isolated
`degraded/[K1]`, and two-collection `[K1]/[K2]` paths.

The live replay also caught that rerank consent incorrectly invalidated current
search materializations. Rerank and Answer consent are now explicitly
query-time and do not advance `collection_processing_revision`; indexed parse
and passage-embedding authority still does. Full evidence, guarded local repair,
tests, cleanup, and rollback are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9D.1 Chunk-planning cross-reference

The first structure-aware reindex slice freezes a pure Parent/Child plan without
mutating the live generation. It preserves heading sections and protected
structural units, emits UTF-8-safe source references, and bounds Parent, Child,
overlap, unit, and document sizes. Detailed contracts, tests, rollback, and the
D.2/D.3 handoff are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9D.2.3c embedding closure cross-reference

The shared structure candidate completed three real Jina
`retrieval.passage`/1024 embedding jobs through the existing fenced handler.
Exact three-document coverage, published materialization hashes, ready Child
vectors, and generation-scoped document heads passed on a disposable clone;
the candidate remained building and the formal active generation did not move.
The run changed no production code or database state. Full provider evidence,
ACL-preserving clone lesson, cleanup, D.2.3 closure, and D.3 handoff are recorded
in `docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9D.2.3b candidate parse cross-reference

Lease-fenced generation-profile resolution now preserves baseline parsing while
routing only the shared candidate profile to Native/MinerU structure mappers.
Disposable-clone proof staged one real MinerU PDF and two Native DOCX documents,
left all three passage-embedding jobs pending, and kept the formal active
generation unchanged. The live run also admitted the observed MinerU
`pdf_info[]` shape, fixed replay timestamp ordering, and prevented signed result
URL queries from entering logs. Full evidence, cleanup, remaining Jina/cutover
gates, and rollback are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9D.3a verifier cross-reference

The new database-only verifier derives exact candidate coverage, successful
job pairs, artifacts, Block/Parent/Child/vector/locator lineage, deterministic
manifest, and frozen counts under the expected corpus-head lock. Real-clone
proof transitioned only the candidate to `verified/ready`, reproduced the same
manifest on replay, and rejected a transactionally missing vector. The formal
active generation did not move. Full defects, tests, cleanup, rollback, and the
D.3b/D.3c handoff are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9D.3b cutover-fence cross-reference

Promotion now shares the corpus-head lock with document deletion and reruns the
full generation verifier before any active state can move. Real-clone proof
made two verified candidates stale by deletion, rejected both promotions,
recorded idempotent `failed/failed` rollbacks, and allocated replacements while
the formal active generation remained unchanged. The concurrent case waited
1,908 ms behind the delete lock before failing on coverage. No successful
promotion permission or execution entered this slice. Full evidence, cleanup,
rollback, and the D.3c handoff are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9D.3c atomic-cutover cross-reference

The fenced structure candidate was promoted on a disposable production-shape
clone after three real Parse and three real Jina jobs plus deterministic
generation verification. Active-head retrieval and a real model stream emitted
`[K1]` bound to the new generation's Parent and Child. The new rollback function
rejected a transactionally missing old vector, then restored the exact source
generation; direct retrieval and a second real `[K1]` stream switched back to
that old head, while stale replay failed closed. Migration down/up, cleanup,
quality gates, and formal-database non-mutation passed. G11.9D is complete; full
evidence is recorded in `docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9E.1 Go search-provider cross-reference

Tavily, Firecrawl, Exa, and Bocha now have one closed Go adapter boundary with
fixture-tested legacy shapes, hardened outbound HTTPS/DNS/IP/redirect/response
limits, redacted errors, and bounded normalized results. No route, Key,
SearXNG, fallback, or provider spend entered this slice. Runtime/model-built-in
wiring and final frontend deletion remain E.2/E.3. Full contracts and evidence
are recorded in `docs/contracts/go-web-search-providers.md` and
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-18 — G11.9E.2 Go Search execution cross-reference

Go now resolves exactly one server-owned active Search execution and exposes
authenticated `POST /v1/search` without accepting browser provider settings or
secrets. Explicit `OpenAI` runtime providers add Responses
`web_search_preview`; `OpenAI Compatible` remains Chat Completions-only and
fails the built-in capability check without fallback. Normalized grounded
sources stream as transient `search.results`; `[W]` persistence and frontend
cutover remain E.3. Full tests, vet, source build, module/security/quality, and
redaction gates passed without network calls or Key reads. Detailed evidence is
in `docs/contracts/go-web-search-providers.md` and
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-18 — G11.9D.2.2a profile convergence cross-reference

Native and MinerU Chunk Manifests now share one structure profile hash, closing
the mixed-format generation admission conflict discovered before live staging.
No runtime generation was created or switched.

## 2026-07-18 — G11.9D.2.3a rebuild allocator cross-reference

Migration 028 adds the fail-closed allocation boundary for one non-active
structure rebuild generation. Disposable-clone proof covered exact
active-document membership, staging materialization/parse-job counts,
concurrent-candidate rejection, and an unchanged active generation. Detailed
defects, verification, cleanup, rollback, and the remaining real MinerU/Jina
handoff are recorded in `docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9D.2.2 MinerU projection cross-reference

Digest-bound MinerU page elements now preserve heading/text/table/formula
structure and page-BBox locators through Canonical IR, Chunk Manifest, and
Postgres projection DTOs. The mapper remains offline and fails closed on
unknown text-bearing shapes. Detailed proof, limitations, rollback, and D.2.3
handoff are recorded in `docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9D.2.1 Native projection cross-reference

Validated Native headings, paragraphs, lists, table rows, code, logical heading
paths, source locators, planner spans, and exact overlap now project into
Canonical IR v2 / Chunk Manifest v2 and the existing Postgres DTO builder. This
is intentionally offline: the production gateway, Jina, persistence, and active
generation are unchanged. Detailed implementation, verification, rollback, and
D.2.2/D.2.3 handoff are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/rag-structure-chunking.md`.

## 2026-07-18 — G11.9E.3 Search cutover cross-reference

Go chat now resolves Search once, executes the selected external or built-in
capability without fallback, emits cumulative `search.results`, and persists
bounded `[W]` citation/output artifacts through Postgres reload. The frontend
consumes the Go SSE and server-owned availability; legacy Next Search, browser
provider/Key/Base URL authority, client preflight, built-in Next flags, and the
retired self-hosted path are deleted. Backend tests/race/vet, 846 frontend tests,
lint/typecheck/build, live Postgres round-trip, Compose build/health, and
owner-authorized real negative provider probes passed. Positive credentialed
activation remains G11.9F. Detailed evidence and the persisted-volume principal
drift repair are in `docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-18 — G11.9F.1 provider-vault foundation cross-reference

The unused Go `internal/providersecrets` package now provides a strict bounded
Docker-Secret keyring loader, context-bound AES-256-GCM envelope, retained-key
decryption, and active-key rotation primitive. No database, Compose, route,
runtime provider, `.env`, real Key, or network state changed. Full module,
tests/race/vet, quality, security, rollback, and F.2 handoff evidence is in
`docs/contracts/provider-secret-vault.md` and
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-18 — G11.9F.2.1 model-provider vault cutover cross-reference

Administrator model-provider BYOK ingress is now re-encrypted into
context-bound `A256GCM` vault envelopes before Postgres writes. Legacy BYOK
rows and the Server Default env secret lazily import on metadata save; custom
providers cannot inherit the default. A disposable `mm_chat_*_test` database
proved ciphertext-only storage and fresh-Vault reload, then was deleted while
the formal schema remained at version 27.

Live Compose restart found and fixed the mode-`600` file-Secret ownership
boundary: backend/admin now run as the matching configured non-root host
UID/GID, and preflight rejects drift. Full Go tests/race/vet, preflight,
Compose, module, quality, security review, image build, mount, and restart
health passed without provider calls or quota use. Bulk rotation/backup remains
F2.2; connection-test activation and model `.env` removal remain F2.3. Full
evidence and asymmetric rollback requirements are in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/provider-secret-vault.md`.

## 2026-07-18 — G11.9F.2.2 transactional rewrite cross-reference

The operator-only model-provider rewrite now defaults to a redacted dry-run,
binds all source state/actions and the active key into an exact plan SHA, and
requires that plan plus a verified backup SHA before one locked Serializable
transaction can backfill legacy or rotate retained-key envelopes. Deleted rows
are included; unknown/copy/stale/blocked state fails before writes. Keyring
prepare/prune candidates are closed, owner-only, and no-overwrite.

Disposable Postgres rewrite plus dump/restore proved active-key-only
ciphertext, rollback on wrong/blocked plans, and absence of plaintext/keyring
material. Formal cutover used an owner-only full dump and restore drill, then
rewrote one legacy custom row and one historical Server Default via the still-
configured env fallback. Final audit is 2 current / 0 changed / 0 blocked;
backend/frontend are healthy, schema remains version 27, and the pre-rewrite
backup is retained. No provider request occurred. Full evidence and rollback
are in `docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/deployment/secret-rotation.md`.

## 2026-07-19 — G11.9F.3 Search administrator implementation cross-reference

The four fixed external Search providers now have Postgres/vault-backed
administrator CRUD, BYOK save-and-test, atomic single activation, dynamic Go
runtime resolution, kind-aware rotation, and a server-loaded settings page.
Backend/full frontend gates, isolated one-active/fresh-vault Postgres proof,
source-build deployment, hydrated Chrome UI inspection, and reversible live
no-Key CRUD cleanup passed. Owner-entered Tavily then passed positive
`/v1/search`, real model chat `[W]` persistence/reload, backend restart, and a
forced active-provider 502 without fallback; exact provider state was restored,
the smoke conversation was deleted, and an owner-only rollback dump was
verified. Full evidence and rollback details are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/search-provider-admin.md`.

## 2026-07-20 — G11.10 in-thread generation progress cross-reference

The blank pre-stream wait now renders a compact Knowledge, Web, or model status
inside the conversation, and an empty assistant draft renders generation status
instead of an unlabelled bubble animation. Focused and full frontend gates,
production build, Compose source rebuild, and a real Search/provider browser
flow passed; the temporary smoke conversation was deleted. Exact root cause,
implementation, evidence, rollback, and the coarse-stage SSE boundary are in
`docs/tracking/g11-10-chat-generation-progress-process.md`.

## 2026-07-20 — G11.11 browser SSE streaming repair cross-reference

Go and the provider were already emitting and flushing incremental deltas, but
the same-origin Next rewrite gzip-compressed browser requests and buffered the
small SSE writes until completion. Successful SSE responses now declare
`Cache-Control: no-cache, no-transform`; browser-like compressed probes no
longer receive `Content-Encoding`, and live browser content advances over many
renders. Full backend tests/race/vet, Compose rebuild/health, and smoke cleanup
passed. Exact timing evidence and rollback are in
`docs/tracking/g11-11-sse-streaming-process.md`.

## 2026-07-20 — G11.12 stream scroll follow cross-reference

Repeated smooth bottom-scroll calls no longer fight the reader during live
output. Wheel/pointer intent now pauses following, returning to the bottom
resumes it, programmatic layout movement is ignored, and measured composer
clearance replaces the undersized fixed bottom reserve. Focused tests,
lint/typecheck, Docker build, live pause/resume/clearance proof, Compose health,
and smoke cleanup passed. Exact behavior and rollback are recorded in
`docs/tracking/g11-12-stream-scroll-follow-process.md`.

## 2026-07-20 — G12.4 server image repair cross-reference

Legacy `openai_compatible` image refs now resolve through the exact configured
Server Default model allowlist, chat preserves the proven `1024x1024` and
base64 response request shape, and failed live/reloaded image assistants stop
instead of restarting an infinite progress timer. Full backend/frontend gates,
production builds, a real 2,407,661-byte `gpt-image-2` chat artifact, an 8 ms
negative failure, and file/conversation cleanup passed. Exact evidence and
rollback are recorded in `docs/tracking/g12-provider-protocols-process.md` and
`docs/contracts/media-job-executor-seams.md`.

## 2026-07-20 — G12.4.1 image policy failure cross-reference

The failed named-character request was an upstream HTTP 400
`content_policy_violation`, not a routing, Key, storage, or frontend-progress
failure. Go now exposes a sanitized terminal policy code, retries only bounded
transient failures once, distinguishes shared-deadline timeout, and the frontend
renders localized rewrite/retry guidance. Full backend/frontend gates,
production builds, two real policy rejections, one post-build timeout, one
unrelated 3,285,229-byte positive image, one final-build 2,283,680-byte
same-prompt image, health checks, and complete smoke cleanup passed.
Exact evidence and rollback are recorded in
`docs/tracking/g12-provider-protocols-process.md` and
`docs/contracts/media-job-executor-seams.md`.

## 2026-07-22 — Dedicated API Knowledge lifecycle repair

The first post-PG17 manual Knowledge upload exposed direct Go access to
projection-owner tables that the former database-owner runtime had masked.
Migrations `039` and `040` now keep upload/reprocess/delete projection work and
the token-fenced source metadata lookup behind narrow SECURITY DEFINER calls;
`neo_chat_api` retains zero direct privileges on the seven internal projection
relations checked. Real TXT upload/indexing, Jina query/rerank, exact BM25,
model answer with `[K1]`, deletion invisibility, and complete synthetic fixture
cleanup passed. Full evidence is in
`docs/tracking/g18-bm25-pgvector-retrieval-process.md`.

The follow-up function audit found 21 older SECURITY DEFINER functions still
using `"$user", public`. Migration `041` hardened all 53 current-schema
SECURITY DEFINER functions while preserving owners and grants. Live checks
returned zero unsafe paths, zero PUBLIC execute grants, and zero direct API
grants across nine guarded projection relations. A fresh TXT upload/bind reached
`active`, then deletion returned `204 / 204 / 404` for Document, File, and the
deleted Document lookup.

## 2026-07-20 — G12.4.3 GPT Image SSE closure

SSH evidence showed the active OpenResty route already uses 600-second proxy
timeouts: its `499` recorded the caller disconnecting, while sub2api completed
`200` one second later and hit a broken pipe. The previous OpenResty-timeout
attribution is superseded. The official `gpt-image-2` Image API SSE path was
proven against the configured relay, then implemented for `gpt-image-*` with
one non-persisted partial image and final-image fallback. Full Go tests/vet,
focused race tests, Backend rebuild/health, a 67,327 ms real complex-poster
artifact, and HTTP 204 file/conversation cleanup passed. Exact evidence and
rollback are recorded in `docs/tracking/g12-provider-protocols-process.md` and
`docs/contracts/media-job-executor-seams.md`.

## 2026-07-20 — G12.5 reasoning effort cross-reference

The reasoning lightbulb now opens a model-aware Off/Auto/Low/Medium/High menu,
adds XHigh for known compatible families and Max for GPT-5.6, and persists the
choice through the Go-owned conversation config. Go validates and normalizes
the level before mapping it to Chat Completions, Responses, or Anthropic
thinking budgets. Full backend/frontend gates, production builds, real
`gpt-5.6-sol` Low/Max requests with HTTP 204 cleanup, and headless selection/
reload verification passed. Exact research, contract, and rollback evidence is
in `docs/tracking/g12-provider-protocols-process.md`,
`docs/contracts/chat-stream-api.md`, and
`.trellis/tasks/07-07-mm-chat-server-refactor-design/research/reasoning-effort-kelivo.md`.

## 2026-07-21 — G12.4.4 image continuation and HTML containment

The failed `继续画` request was stored as 6,895 characters of `gpt-5.6-sol`
HTML with no image attachment despite a recent successful `gpt-image-2`
message. Bounded active-branch context now restores the prior image model for
explicit continuation language. Complete flex/grid HTML survives CommonMark
blank lines, while positioned poster HTML uses a nonce-constrained inline
sandbox instead of partial conversation-DOM execution. Fixed pixel canvases
are uniformly fitted to the iframe, so inline view no longer requires reading
mode to expose the right and bottom edges. Exact evidence and rollback are recorded
in `docs/tracking/g12-provider-protocols-process.md` and the executable contract
is in `docs/contracts/frontend-api-client.md`.

## 2026-07-20 — G12.4.2 image connection failure cross-reference

The owner-visible 17:50 image request exhausted the bounded provider transport
retry without receiving an HTTP response (`IMAGE_PROVIDER_REQUEST_FAILED`,
106,583 ms). Go now persists and streams the distinct recoverable
`IMAGE_PROVIDER_CONNECTION_ERROR`, while the frontend renders localized retry
guidance and still excludes raw network/provider details. Exact correlation and
contract changes are recorded in
`docs/tracking/g12-provider-protocols-process.md` and
`docs/contracts/media-job-executor-seams.md`.

## 2026-07-22 — Turn-scoped Knowledge Citation repair

The apparent no-evidence `[K1]` was not a retrieval false positive. Persisted
metadata proved `no_evidence / citationCount=0 / evidenceUsed=false`; the model
had copied a reserved marker from the previous assistant turn. Backend history
assembly now strips prior `[K#]/[W#]`, completion accepts only current-turn
issued markers, and persistence plus terminal SSE use the reconciled content.
Frontend server-message mapping also removes unissued Knowledge markers from
legacy persisted messages. Full Go and Frontend gates, healthy rebuilt Compose
services, and a real two-turn `gpt-5.6-sol` hit-then-miss replay passed; the
temporary conversation was deleted. Exact evidence and rollback are recorded
in `docs/tracking/g18-bm25-pgvector-retrieval-process.md`.

## 2026-07-22 — Conversation-aware external Search repair

The latest owner conversation proved ordinary external Search had regressed to
literal current-message queries during the Go migration: vague follow-ups
returned generic AI networking pages, and “你知道你是谁吗？” matched a song.
The former Next path had used six recent messages to plan a standalone query.
Go now performs the same bounded, active-branch and runtime-model-aware rewrite
before Tavily/Firecrawl/Exa/Bocha, fails open to the current message, and stores
only redacted outcome/derived flags. Full Go gates, a rebuilt healthy Backend,
and a real three-turn DeepSeek V4 Flash + Tavily replay passed twice for
contextual follow-ups; both result sets were DeepSeek-specific and all smoke
state was deleted. Exact evidence and rollback are recorded in
`docs/tracking/g11-knowledge-auto-rag-process.md` and
`docs/contracts/chat-source-fusion.md`.

## 2026-07-22 — G19.1 Tool Loop and process-trace contract

The current globe remained forced Search after the conversation-aware Query
repair because `useSearch=true` still selected Search before the answer model
ran. Kelivo commit `545f7d67de250283232c9487aa5f4f42e85a7643`
was audited as a provider-native `tool_choice:auto` reference. Seventeen owner
decisions now freeze a generic no-count-limit Tool Loop, strict
`off | model_builtin | external` Search state, same-model compatibility
planning, truthful provider Reasoning, durable sanitized process steps,
read-only automatic tools, side-effect approval, and actual-use-only
Citations. G19 is split into seven independently tested and committed groups;
G19.1 changes documentation only. The exact scope, contract, rollback, and next
gate are recorded in `docs/contracts/chat-tool-loop.md`,
`docs/tracking/g19-tool-loop-process-trace-plan.md`, and
`docs/tracking/g19-tool-loop-process-trace-process.md`.

## 2026-07-22 — G19.2 durable process-trace foundation

Go now emits ordered `reasoning.delta` and `process.step.updated` SSE, parses
provider-returned reasoning summaries/thinking across OpenAI Responses,
OpenAI-compatible/Gemini, and Anthropic, and persists only sanitized terminal
reasoning plus process steps. Cross-chunk redaction holds back a bounded suffix
so split API-Key/Bearer values cannot bypass live SSE sanitization. The frontend
live-upserts the trace, reloads it from server metadata, auto-expands active
work, auto-collapses completion, and preserves manual panel state without
touching chat scroll. Ordinary successful Generation-only answers retain no
empty panel; failures and cancellations stay durable. Full Go
vet/test/race/build and frontend format/lint/typecheck/test/build gates passed.
Legacy pre-answer Knowledge/external Web remains unchanged and is projected as
terminal steps; G19.3 owns the first live external Search Tool Loop. Exact
contracts, evidence, and rollback are in
`docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-22 — G19.3 external Web Tool Loop

OpenAI-compatible and the configured Gemini OpenAI surface now expose
`search_web` to the current model, stream fragmented Tool Calls, execute the
single active external provider, and continue with native assistant
`tool_calls` plus matching Tool results. The persisted Search contract is now
`off | model_builtin | external`; the composer exposes off/external in this
group, off performs zero Search work, ordinary external-mode prompts remain
Auto, and explicit current/online intent is forced. Unsupported native Tools
use a bounded same-model planner without a hidden model. Final source blocks,
metadata, and Tool/Web trace markers now contain only citations used by the
reconciled answer and preserve original marker numbers. Full Backend/Frontend
gates, source builds, healthy Compose services, and two real
`gpt-5.6-sol + Tavily` replays passed. The final replay proved ordinary
zero-Search, live explicit Tool/Web transitions, `[W1]` reload in both trace
steps, and `204 -> 404` temporary-state cleanup. Exact implementation,
evidence, and rollback are recorded in
`docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-22 — G19.4 native Anthropic Tool Loop

Anthropic now joins the provider-native loop with fragmented `tool_use`
parsing, matching user `tool_result` continuation, failed-result `is_error`,
and private preservation of ordered Thinking/signature, redacted-Thinking,
text, and Tool blocks. The private round state never reaches SSE or storage.
Extended Thinking uses Auto Tool choice with the existing buffered explicit-
Search fallback, while non-Thinking explicit Search names `search_web`.
Native-round usage is now cumulative. Provider/handler fixtures proved
multiple rounds, live reasoning and Tool/Web steps, persistence/reload,
64-KiB rejection, failure, and cancellation; all Go gates and the source-built
Backend passed. No active tested Anthropic credential exists in the live
administrator store, so the conditional real Claude proof was not attempted.
A real `gpt-5.6-sol + Tavily` shared-loop regression passed and its temporary
conversation was deleted. Exact evidence and rollback are in
`docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-22 — G19.5 three-state Search and built-in capability administration

The globe now persists exactly one `off | model_builtin | external` mode, new
conversations inherit the latest server mode, and the first message uses that
inherited state. Official OpenAI/Gemini/Anthropic built-in Search streams and a
server-owned custom compatible attestation route are wired without cross-mode
fallback. Custom proof binds the exact provider, Base URL, encrypted secret,
protocol, and model and is invalidated by configuration drift. Full Go and
Frontend gates, disposable Postgres CAS proof, source image rebuild, health,
mode reload, and Tavily regression passed. The configured compatible relay
returned no Search sources for all four chat models, correctly retained no
attestation, and was restored to its original state; no official credential is
currently available for a positive live vendor call. Exact evidence and
rollback are in `docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-23 — G19.8 transient external Search recovery

An owner-visible Tavily request failed at the 15-second response-header
boundary while immediate repeats succeeded. Go now retries only the same
resolved external provider once for transport, `408`, `429`, or `5xx`
failures, stops on cancellation/permanent response errors, and never falls back
to another provider. Focused tests passed 50 times, full Go gates passed, the
source Backend rebuilt healthy, three real Tavily calls returned five sources
each, and a real DeepSeek compatibility Tool Loop completed with `[W1]` before
its temporary conversation was deleted (`204 -> 404`). Exact policy, evidence,
and rollback are in `docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-23 — G19.9 native Tool continuation recovery

An owner-visible `gpt-5.6-sol` turn completed two Tavily searches but failed on
the third provider continuation with no answer content. Web Tool Results now
carry only sources added by their own execution instead of repeating the
cumulative corpus. A synchronous or in-stream continuation failure after
authorized evidence and before answer text receives one same-provider/model,
no-Tools evidence answer stream; cancellation, no-evidence, and partial-answer
failures remain terminal. Cumulative usage and backend marker reconciliation
remain authoritative. Repeated Web-only and mixed Knowledge/Web fixtures, full
Go vet/test/race/build, source Backend rebuild/health, and a real 20-Query
`gpt-5.6-sol + Tavily` stress replay passed. The standalone gate also passed
Frontend 190 files/911 tests, all Go packages, and Python 1,730 passed/7
skipped. The answer retained three used sources and the disposable conversation
was deleted (`204 -> 404`). The live run consumed 295,914 ms of the configured
five-minute timeout, so unlimited model-directed Tool loops remain an
explicitly recorded operational risk. Exact evidence and rollback are in
`docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-23 — G19.10 buffered evidence-recovery answer retry

An owner-visible recovery answer produced 1,064 bytes and then interrupted at
`出行建议：随身`, so the Backend truthfully persisted a partial answer with a
failed terminal state. Recovery answers are now buffered server-side and are
only released after a successful provider-stream close. A failed or empty first
attempt is discarded and retried once with the exact same provider/model and
Evidence; cancellation never retries, and two failures expose zero recovery
content. The buffer is capped at 1 MiB/8,192 events. Deterministic Handler
integration proved partial-draft isolation, retry completion, Citations, and
zero-content double failure; repeated focused tests and full Go
vet/test/race/build passed. The standalone gate passed Frontend 190 files/911
tests, all Go packages, and Python 1,730 passed/7 skipped. The source Backend
rebuilt healthy, and a final real
`gpt-5.6-sol + Tavily` regression completed with `[W2]`/`[W3]`, two source
cards, and `204 -> 404` cleanup. The intermittent break did not recur live, so
the retry branch is attributed only to deterministic integration evidence.
Exact evidence and rollback are in
`docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-23 — G19.11 process-trace display projection

A successful `东京天气` turn rendered the same `search_web` execution as both
generic Tool and specialized Web rows with identical Query and 16-second
duration. The Backend trace was correct; the ordinary UI projection was
duplicative. The frontend now hides a generic Search/Knowledge Tool row only
when its same-`toolName`, same-Round specialized row exists, while retaining the
complete durable trace, unmatched failures, and custom Tools. Projected rows
drive active state and summaries, and redundant lifecycle outcome captions are
omitted without translating or rewriting provider reasoning. Focused fixtures
passed 7/7; Frontend format/lint/typecheck, 190 files/913 tests, and production
build passed. The source Frontend image rebuilt and returned healthy. Exact
evidence and rollback are in
`docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-23 — G19.12 selected-Knowledge uncertainty guard

An owner turn had a selected Knowledge collection but answered “you have not
told me” without issuing `search_knowledge`; the next explicit Knowledge prompt
hit the same collection with `[K1]`. This was native Auto Tool routing omission,
not retrieval failure. Selected-Knowledge native rounds now instruct the current
model to search once before claiming potentially user/project/organization/
document-specific information is unknown, while general or visible-context
questions still skip retrieval and empty results remain normal misses. Focused
fixtures passed 50 times, full Go gates passed, and the Backend rebuilt healthy.
A clean real `gpt-5.6-sol` replay of `我是哪个学校的？` immediately produced a
native Knowledge hit and `[K1]`, then cleaned up with `204 -> 404`. The full
standalone gate passed Frontend 190 files/913 tests, all Go packages, and Python
1,730 passed/7 skipped. Exact evidence and rollback are in
`docs/tracking/g19-tool-loop-process-trace-process.md` and
`docs/contracts/chat-tool-loop.md`.

## 2026-07-23 — Migration plan-ledger reconciliation

The documentation ledger was reconciled against runtime evidence and the later
bounded migration groups instead of treating every old unchecked Phase 9/15
design item as a second active backlog. G7 is now recorded complete for live
MinerU/Jina/Postgres indexing and Citation delivery, G18 complete for the
Golden-gated PostgreSQL 17 BM25/pgvector cutover, and G19 complete through the
selected-Knowledge uncertainty guard. The former paused G5 Search entry is
closed by G11.9/G19, and G8 records both its original delivery and the later
owner-approved G11 single-user Team retirement.

Earlier experimental requirements that were not selected for the production
profile are recorded as retired rather than falsely marked implemented. These
include immutable provider SLA/wire capture fixtures, promotion of the broad
offline-parser program, restic/R2 as a mandatory local gate, and advanced
ColBERT/RAPTOR/GraphRAG/visual/table/tenant-specific retrieval. The active
profile remains MinerU, Jina 1024, PostgreSQL 17 BM25/pgvector, RRF, Jina
reranking, Go source reauthorization, and server-owned Tool/Citation authority.

The Trellis acceptance ledger was updated to the proven current state:
standalone frontend, no required former-root dependency, complete capability
mapping, visual regression, and clean-copy deployment are all closed. The
destructive guard itself is complete, but no former-root deletion was performed.

After reconciliation, unresolved checkboxes in the authoritative progress
ledger belong only to three explicit classes:

```text
G6       hosted Voice/TTS provider selection and authorized live smoke
G10.4b   one-shot owner-authorized former-root deletion and post-delete proof
Phase 16 optional multi-server/Kubernetes migration
```

No application code, runtime configuration, provider secret, user chat,
Knowledge document, database row, or object-store artifact changed during this
documentation-only reconciliation.
