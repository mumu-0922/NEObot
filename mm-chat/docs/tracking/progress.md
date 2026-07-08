# mm-chat Refactor Progress

Update this file whenever a phase or task is completed. Every `[x]` entry must have a matching dated note in [`process.md`](./process.md).

## Planning Rule — New Work Must Be Documented

- [x] Record the rule that new plans and scope changes must be written to docs before implementation.
- [x] Add Phase 11+ roadmap under `docs/architecture/phase-11-plus-roadmap.md`.

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
- [x] Add session cache integration.
- [x] Add stream cancellation flag storage.
- [x] Verify app survives Redis flush for non-temporary data.

## Phase 8 — Browser Data Import

- [x] Define export format from local-first app.
- [x] Define import validation schema.
- [x] Add preview step before upload/import.
- [x] Address browser import contract review findings.
- [x] Import conversations and messages.
- [x] Import attachments into MinIO.
- [x] Add rollback/delete imported data path.

## Phase 9 — Optional Python RAG Sidecar

Deferred behind Phase 11-14. Keep this original placeholder for history; use
Phase 15 as the active RAG implementation gate.

- [ ] Define internal RAG API.
- [ ] Add Python service skeleton.
- [ ] Add document parsing flow.
- [ ] Add embedding/indexing flow.
- [ ] Add retrieval/citation flow.
- [ ] Ensure RAG failure does not break normal chat.

## Phase 10 — Single-Server Deployment

- [x] Add Docker Compose topology under `mm-chat/`.
- [x] Add `.env.example` for new stack.
- [x] Add backup script/guide for Postgres and MinIO.
- [x] Add restore drill guide.
- [x] Add reverse proxy and private network notes.
- [x] Add release/rollback checklist.

## Phase 11 — Frontend Server-Mode Integration

Phase 11 starts as documentation-first planning. Do not mark any implementation
checkbox complete until the slice is implemented, verified, and recorded in
[`process.md`](./process.md).

Phase 11.1 opening constraints recorded on 2026-07-08:

- Target only adapter scaffold, `local|server` mode selection, and the browser
  network-edge decision.
- Do not wire conversation/message CRUD, SSE streaming, or file
  upload/download in the 11.1 opening slice.
- Original owner constraint remains active: refactor work belongs under
  `mm-chat/`; changes under `src/` require owner approval or an explicit
  pending decision before editing.
- Multi-agent execution plus a review agent is required before any Phase 11.1
  implementation checkbox can be marked complete.
- First verify whether the scaffold can live entirely under `mm-chat/`. If not,
  request/confirm the allowed original-app modification boundary before touching
  the original app.

Owner integration constraint recorded on 2026-07-08:

- Preserve the existing Next.js/React frontend stack and visible UI.
- Keep original-app changes minimal and service-layer first.
- Use the adapter boundary to connect functionality; do not rewrite components,
  styling, state shape, or product flows unless a later phase explicitly
  authorizes it.
- `src/` changes are allowed only for narrow API-client/service integration and
  targeted tests.

### Phase 11.1 — Adapter scaffold

- [x] Identify the existing frontend API boundary, mode selector, and local-mode
      callers that must remain stable.
- [x] Add or complete the server-mode adapter scaffold behind the API boundary.
- [x] Document `NEXT_PUBLIC_API_MODE=local|server` and server base-URL behavior.
- [x] Document and verify the browser network edge for server mode: same-origin
      proxy/reverse proxy or explicit backend CORS allowlist.
- [x] Verify `NEXT_PUBLIC_API_MODE=local` still preserves the current local
      rollback path.
- [x] Confirm the first slice does not touch browser import/export UI, auth
      UI/enforcement, RAG/knowledge flows, provider-settings redesign, or
      unrelated product UI.

### Phase 11.1A — Isolated scaffold under `mm-chat/`

This pre-integration slice keeps the original app read-only while preparing the
adapter code shape. It does not complete the full Phase 11.1 app-boundary
wiring above.

- [x] Identify the current frontend chat/API boundary read-only.
- [x] Create an isolated `mm-chat/frontend/` TypeScript API-client scaffold.
- [x] Add `local|server` mode resolution with safe fallback to local mode.
- [x] Add server HTTP helper and Go SSE frame parser scaffolds.
- [x] Add targeted tests for mode resolution, base URL/network-edge handling,
      HTTP error normalization, and SSE parsing.
- [x] Confirm no original app `src/` files were modified by this scaffold.

### Phase 11.1B — Original app adapter boundary

This integration slice adds the same compile-safe adapter boundary to the
existing app without activating it from UI, stores, routes, or legacy
`chatService.ts`.

- [x] Add `src/services/api/client/*` scaffold with `local|server` mode
      resolution.
- [x] Add explicit unsupported local/server chat shells for not-yet-wired CRUD
      and stream methods.
- [x] Add server HTTP helper and Go named-SSE parser under the original app
      service layer.
- [x] Add targeted tests for mode fallback, base URL handling, network-edge
      classification, HTTP error normalization, and SSE protocol checks.
- [x] Verify the scaffold is not imported by `src/components`, `src/features`,
      `src/store`, or `src/services/api/chatService.ts`.
- [x] Run multi-agent implementation plus independent review before marking the
      slice complete.

### Phase 11.2 — Conversation and message CRUD

- [ ] Map frontend conversation/message DTOs to the current Go chat CRUD
      contract.
- [ ] Wire supported conversation create/list behavior to the Go backend;
      missing read/update/delete endpoints must use server-data derivation where
      safe or explicit unsupported, not implicit browser-local fallback.
- [ ] Wire supported message create/list behavior to the Go backend; missing
      read/update/delete endpoints must use server-data derivation where safe
      or explicit unsupported, not implicit browser-local fallback.
- [ ] Map backend validation, not-found, conflict, and database-required errors
      into existing frontend error handling.
- [ ] Verify server mode can create/list conversations and create/list messages
      against the local Go backend.
- [ ] Verify browser refresh reloads server-owned conversation/message state.
- [ ] Verify local mode still creates and reads browser-local chat state.

### Phase 11.2A — Server CRUD adapter methods

This slice implements the server adapter methods and targeted tests only. It
does not yet wire the legacy UI/service entrypoint or prove browser refresh
persistence.

- [x] Confirm current Go CRUD request/response shapes from backend handler and
      contract docs.
- [x] Add `ApiPage<T>` and align chat/message DTOs with backend CRUD fields.
- [x] Implement server-mode `createConversation` and `listConversations`.
- [x] Implement server-mode `appendUserMessage` and `listMessages`.
- [x] Preserve unsupported/fail-closed behavior for SSE stream and cancel in
      this slice.
- [x] Add targeted unit tests for request bodies, URL paths, page unwrapping,
      blank-content blocking, and invalid page responses.

### Phase 11.3 — SSE stream

- [ ] Send persisted `userMessageId`, `modelRef`, and `idempotencyKey` to the
      Go `/stream` endpoint in server mode.
- [ ] Consume `message.started`, `message.delta`, `usage.updated`,
      `message.completed`, `message.error`, and `message.cancelled` frames.
- [ ] Map stream completion, cancellation, and provider errors to terminal UI
      state without duplicate user messages.
- [ ] Verify server mode streams and persists an assistant response against the
      local Go backend.
- [ ] Verify local-mode streaming behavior remains unchanged.

### Phase 11.4 — File upload and download

- [ ] Upload browser-selected files through the server file API.
- [ ] Download file content through the backend gateway without exposing object
      keys, buckets, MinIO URLs, or local paths.
- [ ] Attach server file references to newly created messages where the current
      UI already supports attachments.
- [ ] Verify server mode uploads, downloads, attaches, and refreshes file
      metadata against the local Go backend.
- [ ] Verify local-mode OPFS/file behavior remains unchanged.

### Phase 11.5 — Browser smoke and local rollback

- [ ] Run server-mode browser smoke against the local Docker backend at
      `http://127.0.0.1:8080`.
- [ ] Smoke conversation creation, user message persistence, SSE assistant
      stream, file upload/download, attachment rendering, and refresh
      persistence.
- [ ] Switch back to `NEXT_PUBLIC_API_MODE=local` and verify browser-local
      behavior still works.
- [ ] Record smoke commands, env flags, cleanup/reset notes, and known gaps in
      `process.md`.

## Phase 12 — Browser Data Export/Import UI

- [ ] Add browser export package generation for IndexedDB/localforage and OPFS.
- [ ] Add import preview UI.
- [ ] Add user-confirmed import commit UI.
- [ ] Add safe imported-batch rollback UI.

## Phase 13 — Auth and Multi-User Hardening

- [ ] Replace fixed development user with real session-aware identity.
- [ ] Add login/logout/me or chosen auth-provider flow.
- [ ] Enforce ownership across conversations, messages, files, imports, and runs.
- [ ] Verify two-user isolation.

## Phase 14 — Production Hardening and Observability

- [ ] Add structured logs and request IDs.
- [ ] Add metrics/health visibility for API, DB, Redis, and MinIO.
- [ ] Run documented backup and restore drill.
- [ ] Add reverse proxy/TLS production notes.
- [ ] Add secret rotation notes.

## Phase 15 — Optional Python RAG Sidecar

- [ ] Define internal Go-to-RAG API after frontend server mode is stable.
- [ ] Add Python FastAPI service skeleton.
- [ ] Add indexing, retrieval, and citation flow.
- [ ] Verify RAG failure does not break normal chat.

## Phase 16 — Multi-Server or Kubernetes Migration

- [ ] Define target deployment platform and managed service boundaries.
- [ ] Add image tagging and migration-job strategy.
- [ ] Add ingress, probes, and secrets plan.
- [ ] Verify release and rollback in target environment.
