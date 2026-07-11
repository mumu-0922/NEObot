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
