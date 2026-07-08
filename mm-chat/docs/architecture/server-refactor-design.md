# mm-chat Server Refactor Design

## 1. Purpose

Neo Chat is currently a local-first Next.js/React app. Chat data, settings, and metadata are persisted in browser storage through IndexedDB/localforage, while file bytes use OPFS. The refactor goal is to create a server-backed edition without breaking the existing frontend in one risky rewrite.

The target is a single-server deployment first, with clean seams for later multi-server or K8s migration.

## 2. Non-Negotiable Constraints

- Existing app files are not edited during planning and initial scaffold work.
- New refactor work starts under `mm-chat/`.
- Frontend remains Next.js/React.
- Migration is incremental and reversible.
- Server-side secrets never return to the browser.
- Browser-local data is migrated only after explicit user action.
- Single-server deployment must work before any K8s design.

## 3. Current State Summary

Observed repository traits:

```text
src/app                  Next.js app routes and API routes
src/components           React UI components
src/services             frontend service wrappers
src/lib                  chat, providers, plugins, knowledge, security, utils
src/store                Zustand stores and local persistence
public                   static assets
```

Storage evidence:

```text
IndexedDB/localforage    settings, chat metadata, messages, plugins, memories
OPFS                     uploaded chat files, workspace files, knowledge files
S3/MinIO                 not currently present
```

This means current file bodies live in the user's browser, not on the server.

## 4. Target Architecture

```text
Browser
  ↓
Next.js / React Frontend
  ↓ HTTP/SSE
Go API Backend
  ├─ Postgres: users, sessions, conversations, messages, file metadata, audit logs
  ├─ Redis: session cache, rate limits, short-lived jobs, cancellation flags
  ├─ MinIO: uploaded files, images, PDFs, audio, knowledge source files
  └─ Python FastAPI RAG Sidecar: optional later phase for parsing, embedding, retrieval
```

### Why This Shape

- Go handles API, auth, streaming, provider proxying, and deployment simplicity.
- Postgres is the source of truth for structured data.
- Redis is fast but non-authoritative; use it for temporary state only.
- MinIO gives S3-compatible file storage on one server.
- Python stays optional and focused on AI/RAG workloads.

## 5. Migration Strategy: Strangler Pattern

Do not replace everything at once. Introduce a new backend path and switch capabilities one by one.

```text
Current local-first path
  └─ remains available as fallback

New server path
  └─ grows behind feature flags until stable
```

Recommended flags:

```env
NEXT_PUBLIC_API_MODE=local|server
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

Rollback for each migrated module is switching the flag back to `local` until the old path is intentionally removed.

## 6. Phase Plan

### Phase 0 — Refactor Workspace and Documentation

Objective: create a controlled workspace and living docs.

Actions:

- Create `mm-chat/`.
- Add design, progress, and process documents.
- Record completed steps immediately.

Outputs:

- `README.md`
- `docs/architecture/server-refactor-design.md`
- `docs/tracking/progress.md`
- `docs/tracking/process.md`

Verification:

- Files exist.
- No existing application source file changed.

Rollback:

- Delete `mm-chat/` if the planning workspace is rejected.

### Phase 1 — Inventory Existing App Boundaries

Objective: map what the current app does before extracting services.

Actions:

- Inventory `src/app/api` routes.
- Inventory `src/services`, `src/lib/chat`, `src/lib/providers`, `src/lib/plugin`, `src/lib/knowledge`.
- Document every place that reads/writes IndexedDB, localforage, OPFS, localStorage.
- Identify current chat streaming flow and provider key handling.

Outputs:

```text
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/storage.md
mm-chat/docs/inventory/chat-flow.md
mm-chat/docs/inventory/provider-flow.md
```

Verification:

- Every current API route has owner, input, output, storage touched, and replacement target.

Rollback:

- Inventory docs only; no runtime impact.

### Phase 2 — Frontend API Boundary

Objective: make the frontend call one internal client layer before switching to Go.

Actions:

- Define `chatApi`, `fileApi`, `settingsApi`, `pluginApi`, `authApi` contracts.
- Replace scattered direct `fetch`, provider calls, and storage-coupled flows later in small PRs.
- Keep local implementation and server implementation behind the same interface.

Target shape:

```ts
chatApi.createConversation(input)
chatApi.listConversations()
chatApi.sendMessage(input)
chatApi.streamMessage(input)
fileApi.upload(file)
fileApi.getContent(fileId)
```

Outputs:

```text
mm-chat/docs/contracts/frontend-api-client.md
```

Verification:

- Components depend on API client contracts, not direct backend details.

Rollback:

- Keep existing local implementations as the default mode.

### Phase 3 — Go Backend Skeleton

Objective: create the new server without owning critical traffic yet.

Recommended layout:

```text
mm-chat/backend/
  cmd/api/main.go
  internal/config/
  internal/http/
    middleware/
    routes/
  internal/auth/
  internal/chat/
  internal/provider/
  internal/files/
  internal/storage/postgres/
  internal/cache/redis/
  internal/objectstore/
    local.go
    s3.go
  migrations/
```

Initial endpoints:

```text
GET  /health
GET  /ready
GET  /v1/version
```

Verification:

```bash
go test ./...
curl http://localhost:8080/health
```

Rollback:

- Stop the Go container; frontend remains on local mode.

### Phase 4 — Postgres Source of Truth

Objective: persist core chat data server-side.

Initial tables:

```text
users
sessions
conversations
messages
message_attachments
files
provider_configs
audit_logs
```

Core rules:

- Messages and conversations live in Postgres.
- Files table stores metadata only.
- Provider secrets are encrypted or stored server-side only.
- Every write path is idempotent where retry is possible.

Example file metadata:

```text
id
user_id
filename
mime_type
size
sha256
storage_backend
storage_key
created_at
deleted_at
```

Verification:

- Server restart does not lose conversations.
- Database migration can apply and roll back in development.

Rollback:

- Feature flag returns frontend to local storage mode.
- DB data remains for later retry.

### Phase 5 — Chat Streaming Spine

Objective: make server-side chat generation work end-to-end.

Flow:

```text
Frontend
  ↓ POST /v1/chat/conversations/:id/messages
Go Backend
  ↓ provider adapter
OpenAI / Gemini / other provider
  ↓ SSE events
Frontend renderer
```

Provider interface:

```go
type Provider interface {
    StreamChat(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error)
}
```

Required behavior:

- SSE streaming.
- User cancellation.
- Timeout and retry boundaries.
- Provider errors normalized into stable API errors.
- Token usage captured when available.
- Assistant response persisted after completion.

Verification:

- Send a message and receive streaming output.
- Cancel mid-stream and verify no orphaned job continues.
- Refresh page and see persisted conversation.

Rollback:

- Switch chat API mode back to local.

### Phase 6 — Files Through MinIO

Objective: move server-backed file bodies into object storage.

Single-server topology:

```text
Go backend -> MinIO private endpoint -> ./data/minio
```

MinIO must not be exposed directly to the public internet. The Go backend enforces auth and permissions.

Upload flow:

```text
POST /v1/files
  ↓ auth check
  ↓ size/MIME validation
  ↓ sha256 calculation
  ↓ objectstore.Put(storage_key)
  ↓ insert files row
  ↓ return file_id
```

Download flow:

```text
GET /v1/files/:id/content
  ↓ lookup files row
  ↓ permission check
  ↓ objectstore.Get(storage_key)
  ↓ stream bytes to browser
```

Storage abstraction:

```go
type ObjectStore interface {
    Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
    Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
    Delete(ctx context.Context, key string) error
}
```

Verification:

- Upload file.
- Download same file.
- Verify SHA-256 match.
- Verify another user cannot access it.

Rollback:

- Disable server file mode; existing browser OPFS path remains until migrated.

### Phase 7 — Redis for Temporary State

Objective: add speed and coordination without making Redis the source of truth.

Use Redis for:

```text
session cache
rate limit counters
SSE cancellation flags
temporary upload/job state
short-lived provider response cache, if needed
```

Do not store canonical conversations or files in Redis.

Verification:

- Rate limits trigger predictably.
- App still works after Redis cache flush, except active temporary jobs.

Rollback:

- Disable Redis-dependent middleware or fall back to in-memory dev mode.

### Phase 8 — Browser Data Import

Objective: migrate old local-first users only when they opt in.

Flow:

```text
Frontend export local data
  ↓ normalized manifest + ZIP blobs under files/sha256/* generated in browser
User confirms upload
  ↓
Go preview endpoint validates schema
  ↓ user confirms
Go import endpoint commits
  ↓
Postgres rows + MinIO file objects created
```

Rules:

- No silent upload.
- Preview what will be imported.
- Preserve original timestamps and message roles.
- Reject malformed imports with detailed errors.
- Keep `mm-chat/docs/contracts/browser-data-import.md` as the endpoint and
  validation contract before implementing runtime code.

Verification:

- Export/import a sample conversation with attachments.
- Imported data survives refresh and server restart.

Rollback:

- Imports are append-only until confirmed safe; allow deleting imported conversation.

### Phase 9 — Python RAG Sidecar

Objective: keep RAG complexity out of the Go core.

Topology:

```text
Go backend -> Python FastAPI RAG service -> vector index / provider embeddings
```

Python owns:

```text
document parsing
chunking
embedding
retrieval
rerank
```

Go owns:

```text
auth
task status
file ownership
API response shape
```

Verification:

- Index one uploaded document.
- Ask a question and receive grounded citations.
- RAG failure does not break normal chat.

Rollback:

- Disable RAG endpoints; core chat remains available.

### Phase 10 — Deployment, Backup, and Operations

Objective: make single-server production survivable.

Compose services:

```text
frontend
backend
postgres
redis
minio
rag-service optional
```

Persistent directories:

```text
./data/postgres
./data/redis
./data/minio
./backup
```

Minimum backup plan:

```text
pg_dump daily
MinIO data sync/archive daily
.env encrypted backup
restore drill monthly
```

Security baseline:

- MinIO bound to private/internal network.
- Backend is the only public file gateway.
- Upload size limits.
- MIME sniffing and extension checks.
- UUID storage keys, never raw filenames as object paths.
- Audit log for file access and provider calls.

Verification:

- Fresh server restore from backup.
- Health and readiness checks pass.
- Public ports limited to frontend/reverse proxy/API.

Rollback:

- Keep previous release image and database backup before each deployment.

## 7. Initial API Contract Draft

```text
GET    /health
GET    /ready
GET    /v1/version

POST   /v1/auth/login
POST   /v1/auth/logout
GET    /v1/me

POST   /v1/chat/conversations
GET    /v1/chat/conversations
GET    /v1/chat/conversations/:id
PATCH  /v1/chat/conversations/:id
DELETE /v1/chat/conversations/:id

GET    /v1/chat/conversations/:id/messages
POST   /v1/chat/conversations/:id/messages
POST   /v1/chat/conversations/:id/stream
POST   /v1/chat/runs/:runId/cancel

POST   /v1/files
GET    /v1/files/:id
GET    /v1/files/:id/content
DELETE /v1/files/:id
```

SSE event types:

```text
message.started
message.delta
message.completed
message.error
message.cancelled
usage.updated
```

## 8. Data Boundary Rules

| Boundary | Contract Rule |
|---|---|
| Frontend -> API | JSON request/response, stable error envelope |
| API -> DB | validated domain models, explicit nullable fields |
| API -> MinIO | storage key is UUID/path generated server-side |
| API -> Provider | provider adapter hides vendor-specific formats |
| Go -> Python RAG | internal-only endpoints, file ownership verified by Go |

Error envelope:

```json
{
  "error": {
    "code": "FILE_TOO_LARGE",
    "message": "File exceeds upload limit.",
    "requestId": "req_..."
  }
}
```

## 9. Rollout Order

1. Health-only Go backend.
2. Postgres migrations.
3. Conversation/message CRUD.
4. SSE chat streaming.
5. Frontend feature flag switch for chat.
6. File metadata table.
7. MinIO upload/download.
8. Redis session cache/rate limit.
9. Import tool for local data.
10. Optional Python RAG.

## 10. Acceptance Gates

A phase is complete only when:

- The checklist item in `docs/tracking/progress.md` is marked `[x]`.
- `docs/tracking/process.md` has a dated entry with evidence.
- Verification commands or manual checks are recorded.
- Rollback path is known.
