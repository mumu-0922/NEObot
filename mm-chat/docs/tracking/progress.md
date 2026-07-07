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

- [ ] Create backend directory under `mm-chat/backend/`.
- [ ] Initialize Go module.
- [ ] Add config loader.
- [ ] Add router and middleware skeleton.
- [ ] Add `/health`, `/ready`, `/v1/version`.
- [ ] Add basic tests.

## Phase 4 — Postgres Persistence

- [ ] Add Postgres container plan.
- [ ] Add migrations directory.
- [ ] Create users/sessions schema.
- [ ] Create conversations/messages schema.
- [ ] Create files metadata schema.
- [ ] Create audit logs schema.
- [ ] Verify migration up/down locally.

## Phase 5 — Chat Streaming Spine

- [ ] Add provider interface.
- [ ] Add mock provider for tests.
- [ ] Add first real provider adapter.
- [ ] Add conversation/message CRUD endpoints.
- [ ] Add SSE streaming endpoint.
- [ ] Add cancellation endpoint.
- [ ] Persist assistant response after stream completion.

## Phase 6 — File Storage with MinIO

- [ ] Add object storage interface.
- [ ] Add local filesystem implementation for dev fallback.
- [ ] Add S3/MinIO implementation.
- [ ] Add file upload endpoint.
- [ ] Add file content download endpoint.
- [ ] Add permission checks.
- [ ] Add SHA-256 verification.

## Phase 7 — Redis Temporary State

- [ ] Add Redis container plan.
- [ ] Add rate limit middleware.
- [ ] Add session cache integration.
- [ ] Add stream cancellation flag storage.
- [ ] Verify app survives Redis flush for non-temporary data.

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
