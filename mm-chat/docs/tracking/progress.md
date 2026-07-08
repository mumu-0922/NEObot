# mm-chat Refactor Progress

Update this file whenever a phase or task is completed. Every `[x]` entry must have a matching dated note in [`process.md`](./process.md).

## Phase 0 — Workspace and Planning

- [x] Create isolated `mm-chat/` workspace.
- [x] Add `README.md` with workspace rules and document map.
- [x] Add `docs/architecture/server-refactor-design.md` with detailed refactor architecture and phases.
- [x] Add `docs/tracking/progress.md` as living checklist.
- [x] Add `docs/tracking/process.md` to record decisions and evidence.
- [x] Reorganize documentation under `docs/` by category.
- [ ] Review plan with owner and lock MVP scope.

## Phase 1 — Existing App Inventory

- [x] Inventory current Next.js API routes.
- [x] Inventory frontend service wrappers.
- [x] Inventory local storage usage: IndexedDB/localforage/localStorage.
- [x] Inventory OPFS file flows.
- [x] Inventory provider and streaming flow.
- [x] Produce `mm-chat/docs/inventory/api-routes.md`.
- [x] Produce `mm-chat/docs/inventory/storage.md`.
- [x] Produce `mm-chat/docs/inventory/chat-flow.md`.
- [x] Produce `mm-chat/docs/inventory/provider-flow.md`.

## Phase 2 — Frontend API Boundary

- [x] Define API client contracts.
- [x] Define local-mode implementation contract.
- [x] Define server-mode implementation contract.
- [x] Add feature flag design for `local|server` mode.
- [x] Produce `mm-chat/docs/contracts/frontend-api-client.md`.
- [x] Address reviewer findings for Phase 2 contract.
- [x] Define plugin API placeholder contract for deferred plugin migration.
- [x] Identify components that directly call storage or fetch.
- [x] Produce `mm-chat/docs/inventory/frontend-call-sites.md`.

## Phase 3 — Go Backend Skeleton

- [x] Create backend directory under `mm-chat/backend/`
- [x] Initialize Go module.
- [x] Add config loader.
- [x] Add router and middleware skeleton.
- [x] Add `/health`, `/ready`, `/v1/version`.
- [x] Add basic tests.

## Phase 4 — Postgres Persistence

- [x] Add Postgres container plan.
- [x] Add migrations directory.
- [x] Create users/sessions schema.
- [x] Create conversations/messages schema.
- [x] Create files metadata schema.
- [x] Create audit logs schema.
- [x] Verify migration up/down locally.

## Phase 4.5 — Postgres Runtime Wiring

- [x] Add `DATABASE_URL` and DB pool config.
- [x] Add pgx-backed database connector.
- [x] Add DB-aware `/ready` behavior.
- [x] Add embedded migration files.
- [x] Add migration runner with `schema_migrations`.
- [x] Add `cmd/migrate` CLI for `up` and `down --all`.
- [x] Verify migration CLI against Docker Postgres.
- [x] Verify API readiness with DB enabled.
- [x] Document runtime wiring and deployment flow.

## Phase 5 — Chat Streaming Spine

- [x] Add Phase 5.1 chat CRUD API contract.
- [x] Add Postgres chat repository for development-user conversation/message CRUD.
- [x] Add chat CRUD HTTP routes and DB-disabled `503 DATABASE_REQUIRED` behavior.
- [x] Add idempotency-conflict and ownership-not-found error mapping.
- [x] Add provider interface.
- [x] Add mock provider for tests.
- [x] Add first real provider adapter.
- [x] Add conversation/message CRUD endpoints.
- [x] Add SSE streaming endpoint.
- [x] Add cancellation endpoint.
- [x] Persist assistant response after stream completion.
- [x] Verify cancellation uses conversation-before-message lock order.
- [x] Verify idempotent cancellation preserves cancel metadata.

## Phase 6 — File Storage with MinIO

- [x] Add object storage interface.
- [x] Add local filesystem implementation for dev fallback.
- [x] Add S3/MinIO implementation.
- [x] Add file upload endpoint.
- [x] Add file content download endpoint.
- [x] Add fixed-development-user file ownership checks.
- [x] Add SHA-256 hashing for uploaded file records.
- [x] Link uploaded files to chat messages through `message_attachments`.
- [x] Return server attachment metadata from message create/list endpoints.

## Phase 7 — Redis Temporary State

- [x] Add Redis container plan.
- [x] Add rate limit middleware.
- [ ] Add session cache integration.
- [x] Add stream cancellation flag storage.
- [x] Verify app survives Redis flush for non-temporary data.

## Phase 8 — Browser Data Import

- [ ] Define export format from local-first app.
- [ ] Define import validation schema.
- [ ] Add preview step before upload/import.
- [ ] Import conversations and messages.
- [ ] Import attachments into MinIO.
- [ ] Add rollback/delete imported data path.

## Phase 9 — Optional Python RAG Sidecar

- [ ] Define internal RAG API.
- [ ] Add Python service skeleton.
- [ ] Add document parsing flow.
- [ ] Add embedding/indexing flow.
- [ ] Add retrieval/citation flow.
- [ ] Ensure RAG failure does not break normal chat.

## Phase 10 — Single-Server Deployment

- [ ] Add Docker Compose topology under `mm-chat/`.
- [ ] Add `.env.example` for new stack.
- [ ] Add backup script/guide for Postgres and MinIO.
- [ ] Add restore drill guide.
- [ ] Add reverse proxy and private network notes.
- [ ] Add release/rollback checklist.
