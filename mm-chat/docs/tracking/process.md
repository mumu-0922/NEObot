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
