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
- [ ] Superseded by Phase 15: separate strict-grounded fail-closed behavior from
      optional-enrichment chat degradation.

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

- [x] Map frontend conversation/message DTOs to the current Go chat CRUD
      contract.
- [x] Wire supported conversation create/list behavior to the Go backend;
      missing read/update/delete endpoints must use server-data derivation where
      safe or explicit unsupported, not implicit browser-local fallback.
- [x] Wire supported message create/list behavior to the Go backend; missing
      read/update/delete endpoints must use server-data derivation where safe
      or explicit unsupported, not implicit browser-local fallback.
- [x] Map backend validation, not-found, conflict, and database-required errors
      into existing frontend error handling.
- [x] Verify server mode can create/list conversations and create/list messages
      against the local Go backend.
- [x] Verify browser refresh reloads server-owned conversation/message state.
- [x] Verify local mode still creates and reads browser-local chat state.

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

### Phase 11.2B-1 — CRUD mapper and service gateway

This slice prepares the legacy service/store bridge without wiring UI or store
runtime behavior.

- [x] Add a lightweight `chatCrudService` gateway above the API client.
- [x] Map `ConversationDTO` into legacy-compatible session metadata.
- [x] Map `ChatMessageDTO` into legacy-compatible message records.
- [x] Convert server `ModelRef` values to the current provider/model string
      convention.
- [x] Convert server attachment metadata to backend file-content gateway URLs.
- [x] Fail closed when server CRUD capability is disabled or unsupported server
      roles are returned.
- [x] Add targeted gateway and mapper tests.

### Phase 11.2B-2 — Store server read path

This slice adds explicit store actions for server-mode read experiments. The
actions are not called by UI/bootstrap yet.

- [x] Add `refreshServerSessions()` to load server conversation metadata through
      the CRUD gateway.
- [x] Add `selectServerSession(id)` to load server messages through the CRUD
      gateway.
- [x] Store server read results in non-persisted `serverReadState`, not the
      legacy `sessions/currentSessionId/activeMessages` fields.
- [x] Keep local IndexedDB select/hydration path unchanged.
- [x] Avoid writing server-owned messages to `session_messages_*` during server
      read actions.
- [x] Return `false` without server or local-storage calls when server CRUD is
      disabled.
- [x] Add targeted store tests for refresh, select, disabled mode, stale reads,
      and persist boundary.

### Phase 11.2B-3 — Store server write facade

This slice resolves the async create mismatch by adding opt-in server write
actions. It still does not connect visible UI/bootstrap to server mode.

- [x] Keep legacy `createSession(): string` unchanged for `ChatApp`, sidebar,
      hooks, and local tests.
- [x] Add async `createServerSession()` for server conversation creation.
- [x] Add async `appendServerUserMessage()` for persisted server user messages.
- [x] Generate `idempotencyKey` before server create/append calls when callers
      omit one.
- [x] Convert the selected legacy model string to server `ModelRef`.
- [x] Store server write results only in non-persisted `serverReadState`.
- [x] Avoid `addMessage`, `syncActiveSession`, legacy session fields, and
      `session_messages_*` during server write actions.
- [x] Return `null` without server or local-storage calls when server CRUD is
      disabled.
- [x] Return successful server write ids/messages even when their snapshot
      update is stale, so later streaming can still use persisted server ids.
- [x] Avoid duplicate active messages on idempotent append retries and keep
      known server message counts monotonic.
- [x] Add targeted store tests for create, append, disabled mode, local-state
      isolation, and IndexedDB isolation.

### Phase 11.3 — SSE stream

- [x] Send persisted `userMessageId`, `modelRef`, and `idempotencyKey` to the
      Go `/stream` endpoint in server mode.
- [x] Consume `message.started`, `message.delta`, `usage.updated`,
      `message.completed`, `message.error`, and `message.cancelled` frames.
- [x] Map stream completion, cancellation, and provider errors to terminal
      server-read state without duplicate user messages.
- [x] Verify server mode streams and persists an assistant response against the
      local Go backend.
- [x] Verify local-mode streaming behavior remains unchanged.

### Phase 11.3A — Server API client SSE adapter

This slice implements the server API client stream transport only. It does not
wire visible UI, `ChatApp`, or store generation state to server streaming yet.

- [x] Add incremental SSE parsing for chunked `ReadableStream` responses,
      including CRLF line endings split across chunks.
- [x] Implement `streamAssistantMessage()` against
      `POST /v1/chat/conversations/{id}/stream`.
- [x] Send only the stream body whitelist: `userMessageId`, `modelRef`,
      optional config/instructions/metadata, and `idempotencyKey`.
- [x] Dispatch `message.started`, `message.delta`, `usage.updated`,
      `message.completed`, `message.error`, and `message.cancelled` to
      handlers.
- [x] Ignore duplicate `sequence` values and fail closed on sequence gaps with
      recoverable `STREAM_INTERRUPTED`.
- [x] Implement `cancelRun()` against `POST /v1/chat/runs/{runId}/cancel`.
- [x] Abort after `message.started` calls the cancel endpoint using the captured
      server `runId`.
- [x] Enable server-mode `chatStream` capability in the API client scaffold.
- [x] Add targeted tests for stream request shape, terminal results, JSON
      errors, cancelled/EOF terminal handling, duplicate/gap sequence handling,
      CRLF chunk boundaries, abort cancellation, and cancel endpoint routing.

### Phase 11.3B — Store server stream facade

This slice adds a hidden server stream facade above the API client. It still
does not wire visible UI or `ChatApp` to server streaming.

- [x] Split stream concerns into `chatStreamService` instead of extending CRUD
      service with SSE lifecycle semantics.
- [x] Keep `chatCrudService` focused on conversation/message CRUD and DTO
      mapping.
- [x] Add `sendServerMessageAndStream()` to append a persisted server user
      message and stream the assistant response.
- [x] Update only non-persisted `serverReadState`; do not touch local
      `sessions/currentSessionId/activeMessages/activeMessageTree`.
- [x] Avoid `addMessage`, `syncActiveSession`, local provider streaming,
      `session_messages_*`, and IndexedDB during the server stream facade.
- [x] Insert/update assistant placeholders from `message.started` and
      `message.delta`, then replace with terminal server message when present.
- [x] Avoid applying assistant draft events to non-current server snapshots before
      the terminal server message, preventing hidden message-count inflation.
- [x] Fail closed when either server CRUD or server stream capability is
      disabled.
- [x] Add targeted tests for stream service mapping, capability gating, store
      append+stream flow, local-state isolation, and IndexedDB isolation.

### Phase 11.3C — Terminal server generation state

This slice maps hidden server streaming lifecycle into non-persisted
`serverReadState` so later UI wiring has explicit terminal state. It still does
not connect visible UI, `ChatApp`, or server cancel controls.

- [x] Add non-persisted server generation state with session, user message,
      assistant message, and active backend run identifiers.
- [x] Capture `message.started.runId` as `activeServerRunId` during streaming
      and clear it on terminal completion, failure, or cancellation.
- [x] Map completed, failed, unsupported, and cancelled stream results to
      terminal generation state without duplicating user messages.
- [x] Preserve server read/write stale guards so superseded streams cannot
      overwrite the latest server snapshot.
- [x] Keep server generation state out of persisted chat metadata.
- [x] Add targeted tests for successful streaming, provider failure,
      unsupported fallback, cancellation, run-id propagation, and
      error-envelope preservation.

### Phase 11.3D — Live backend SSE smoke

This slice verifies the Go backend runtime path directly. It does not wire the
visible frontend UI.

- [x] Confirm the single-server Compose backend, Postgres, Redis, and MinIO
      services are running and healthy enough for smoke.
- [x] Verify `/health`, `/ready`, and `/v1/version` on `127.0.0.1:8080`.
- [x] Create a server conversation through `POST /v1/chat/conversations`.
- [x] Append a persisted user message through
      `POST /v1/chat/conversations/{id}/messages`.
- [x] Stream an assistant response through
      `POST /v1/chat/conversations/{id}/stream` and observe
      `message.completed`.
- [x] List server messages after streaming and verify both user and assistant
      rows are persisted.
- [x] Run targeted local-mode regression tests for the legacy
      `chatService`/store path.
- [x] Record smoke command shape, artifact path, result IDs, and cleanup notes
      in `process.md`.

### Phase 11.4 — File upload and download

- [x] Upload browser-selected files through the server file API.
- [x] Download file content through the backend gateway without exposing object
      keys, buckets, MinIO URLs, or local paths.
- [x] Attach server file references to newly created messages where the current
      UI already supports attachments.
- [x] Verify server mode uploads, downloads, attaches, and refreshes file
      metadata against the local Go backend.
- [x] Verify local-mode OPFS/file behavior remains unchanged.

### Phase 11.4A — Server file API client adapter

This slice adds file API methods to the hidden API-client boundary only. It
does not wire visible UI, OPFS replacement, or message attachment flows.

- [x] Add file DTO/input types and `FileApi` to the API-client contract.
- [x] Add local-mode file shell that fails closed as unsupported.
- [x] Add server-mode `uploadFile()` using `multipart/form-data` for
      `POST /v1/files`.
- [x] Add server-mode metadata, content download, and delete methods for
      `GET /v1/files/{id}`, `GET /v1/files/{id}/content`, and
      `DELETE /v1/files/{id}`.
- [x] Enable `files` capability only for configured server mode.
- [x] Verify the adapter never exposes object keys, bucket names, local paths,
      or MinIO/S3 URLs in file records.
- [x] Add targeted tests for upload request shape, URL encoding, binary
      download, error normalization, local unsupported behavior, and capability
      gating.

### Phase 11.4B — File service gateway and live attachment smoke

This slice keeps visible UI unchanged while adding the service/mapper seam that
the UI can call in the next wiring slice.

- [x] Add `fileService` for server-mode chat file upload and download metadata
      mapping.
- [x] Add server attachment mapper for `Attachment[]` to
      `AppendUserMessageInput.attachments`.
- [x] Preserve server attachment metadata when mapping Go chat DTOs to legacy
      frontend messages.
- [x] Add reusable live smoke script for upload, metadata, byte download,
      message attach, and message-list refresh verification.
- [x] Add targeted tests for gateway fail-closed behavior, request conversion,
      and metadata preservation.
- [x] Record commands, artifacts, cleanup caveats, and review result in
      `docs/tracking/process.md`.

### Phase 11.4C — Server-mode browser send wiring

- [x] Expose existing server read/write store methods through the chat shell
      state hook.
- [x] Switch visible sessions/messages to `serverReadState` only in configured
      server mode.
- [x] Upload inline/base64 attachments at send time through `fileService` and
      send only server `fileId` references to Go messages.
- [x] Keep `MessageInput` UI, OPFS utilities, and local-mode send path
      unchanged.
- [x] Disable local plugin, skill, search, and reasoning writes while the
      composer is showing server conversations.
- [x] Use abort-only stop/new-chat/session-switch handling in server mode so
      local IndexedDB sync is not invoked.
- [x] Fail closed for local-only actions that do not yet have server endpoints.
- [x] Browser-smoke server-mode file upload, attachment rendering, refresh, and
      local rollback in Phase 11.5.

### Phase 11.5 — Browser smoke and local rollback

- [x] Run server-mode browser smoke through `/mm-api` to the local Docker
      backend at `http://127.0.0.1:8080`.
- [x] Smoke conversation creation, user message persistence, SSE assistant
      stream, file upload/download, attachment rendering, and refresh
      persistence.
- [x] Switch back to `NEXT_PUBLIC_API_MODE=local` and verify browser-local
      behavior still works.
- [x] Record smoke commands, env flags, cleanup/reset notes, and known gaps in
      `process.md`.

## Phase 12 — Browser Data Export/Import UI

- [x] Create detailed local implementation plan in
      `docs/architecture/phase-12-browser-import-ui-plan.md`.
- [x] Add browser export package generation for IndexedDB/localforage and OPFS.
- [x] Add import preview UI.
- [x] Add user-confirmed import commit UI.
- [x] Add safe imported-batch rollback UI.
- [x] Run local browser import smoke through `/mm-api` and record evidence.

## Phase 13 — Auth and Multi-User Hardening

- [x] Replace fixed development user with real session-aware identity.
- [x] Add login/logout/me or chosen auth-provider flow.
- [x] Enforce ownership across conversations, messages, files, imports, and runs.
- [x] Verify two-user isolation.

### Phase 13.1 — Request identity plumbing

- [x] Create detailed implementation plan in
      `docs/architecture/phase-13-auth-multi-user-plan.md`.
- [x] Add backend auth context helpers with development-user fallback.
- [x] Add optional Bearer session middleware backed by the existing session
      resolver and Redis cache path.
- [x] Scope chat, file, browser-import, and run-cancellation repository
      operations by request context identity instead of fixed struct user IDs.
- [x] Add targeted tests for auth context, session middleware, and user-scoped
      file object keys.

### Phase 13.2 — Bootstrap auth endpoints

- [x] Add bootstrap-token login service that creates Postgres session rows and
      returns a raw bearer token once.
- [x] Add `POST /v1/auth/login`, `POST /v1/auth/logout`, and `GET /v1/me` Go
      routes.
- [x] Revoke sessions durably on logout and clear Redis session-cache entries
      when configured.
- [x] Add auth environment keys for backend and single-server Compose startup.
- [x] Add targeted tests for config loading, login, logout, `/v1/me`, and auth
      route registration.

### Phase 13.3 — Enforced hosted auth mode

- [x] Add `AUTH_MODE=development|required` config with fail-closed handling for
      unknown non-empty values.
- [x] Keep development-user fallback only in `development` mode.
- [x] Reject missing credentials before protected chat, file, import, and `/me`
      routes in `required` mode.
- [x] Keep `/health`, `/ready`, `/v1/version`, and `POST /v1/auth/login`
      public.
- [x] Add targeted tests for required-mode rejection, public-route exemptions,
      missing resolver failure, and config parsing.

### Phase 13.4 — Two-user isolation

- [x] Add two-session/session-resolver integration coverage.
- [x] Verify chat conversations, messages, attachments, assistant finalize, and
      run cancellation are scoped by request user.
- [x] Verify file metadata, delete, and object-store access do not cross users.
- [x] Verify browser import commit/status/rollback/idempotency/object keys are
      scoped by request user.
- [x] Preserve not-found style errors for cross-user access to avoid leaking
      resource existence.

## Phase 14 — Production Hardening and Observability

- [x] Add structured logs and request IDs.
- [x] Add health visibility for API, DB, Redis, and storage readiness.
- [x] Add metrics visibility for API, DB, Redis, and MinIO.
- [x] Run documented backup and restore drill.
- [x] Add reverse proxy/TLS production notes.
- [x] Add secret rotation notes.

## Phase 15 — Accuracy-First Server RAG

- [x] Create the evidence-based accuracy-first architecture in
      `docs/architecture/phase-15-accuracy-first-rag-design.md`.
- [x] Create the owner-review implementation profile in
      `docs/architecture/phase-15-recommended-implementation-profile.md`.
- [x] Record the Owner decision approving all-processor Collection Data Consent
      for the Bootstrap Public Collection; runtime Consent rows remain pending.
- [x] Confirm the small-team product model: per-user Personal Knowledge, Shared
      Team Knowledge, Team Admin management, and Jina credential availability.
- [x] Define the future Phase 15 Knowledge ACL API, identity, consent, revision,
      file-binding, deletion, and isolation-test contract.

### Phase 15.1 — Go/Postgres knowledge control plane

- [x] Add reversible identity, Team, Membership, Invite, Collection, logical
      Document/Version, Governance, Consent, and Outbox schema in migration
      `004`.
- [x] Verify migration `001` through `004` Up, positive and negative database
      constraints, `004`-only Down, and zero Phase 15 catalog residue on an
      isolated PostgreSQL 16 database.
- [x] Add credential, invite, recovery, and independent-login services.
- [x] Add Team/Membership repositories, APIs, revision updates, and last-Admin
      protection.
- [ ] Add Collection/Document/Consent repositories and APIs with locked File
      binding and transactional Outbox writes.
- [ ] Pass the complete two-user/two-team ACL, Consent, revision, deletion,
      idempotency, and Outbox replay gate.

#### Phase 15.1C — Team services

- [x] Lock the detailed Team/Membership/Invite design in
      `docs/architecture/phase-15-1c-team-services-plan.md`.
- [x] Close the independent xhigh design review with `P0/P1/P2 = 0`.
- [x] Synchronize Team/Auth/frontend contracts and add reversible migration
      `005`.
- [x] Implement Team CRUD, Membership revision/last-Admin fencing, and
      account-disable coordination.
- [x] Implement hash-only Invites, encrypted durable Mail Outbox delivery, and
      new/existing-account acceptance.
- [x] Wire protected Team routes, authenticated cursors, configuration,
      metrics, and worker lifecycle.
- [x] Pass unit/race/PostgreSQL 16/migration replay/security/promotion gates.

#### Phase 15.1D — Collection, Document, and Consent services

- [x] Lock the detailed design in
      `docs/architecture/phase-15-1d-collection-document-consent-plan.md`.
- [x] Synchronize public Knowledge DTO/error contracts and add reversible
      migration `006` for display metadata, idempotency, and Processing Jobs.
- [ ] Implement Personal/Team Collection repositories, ACLs, revisions, and
      transactional Outbox writes.
- [ ] Implement logical Document/Version lifecycle, locked File binding/delete,
      authorized content reads, reprocess, and tombstones.
- [ ] Implement operator Governance and Collection/User Consent services with
      purpose/data-type and expiry/revision fences.
- [ ] Wire protected Knowledge routes and pass unit/race/PostgreSQL 16 ACL,
      migration, deletion, idempotency, and Outbox replay gates.

- [ ] Replace the single-bootstrap-user ceiling with admin-invited independent
      user sessions and versioned Team membership.
- [ ] Add Team, Membership, Personal/Team Collection, Knowledge Document, and
      per-processor Collection/User Consent schemas and APIs.
- [ ] Enforce Personal-owner and Team-role ACLs across file binding, indexing,
      query, citation, consent, deletion, and cross-user/cross-team tests.
- [ ] Freeze the canonical block/chunk schema, ACL invariants, index profiles,
      and golden evaluation corpus.
- [ ] Define the workload-authenticated Go-to-RAG evidence API, Go-side source
      reauthorization, citation minting, and strict/optional failure contracts.
- [ ] Add private Python query and indexing services, Postgres outbox rescan,
      and non-authoritative Redis wake-up/lease/cache handling.
- [ ] Preserve original files and structured parser artifacts for reproducible
      reindexing.
- [ ] Add format/page-aware parsing, quality gates, and parent/child/window
      chunking with exact provenance.
- [ ] Bake off and pin the search projection; treat Qdrant as the leading
      rebuildable ACL-filtered dense/sparse/multi-vector candidate.
- [ ] Add hybrid recall, RRF fusion, measured cross-encoder reranking, dynamic
      context expansion, and source-level citations.
- [ ] Gate contextual retrieval, ColBERT, query decomposition, RAPTOR, and
      GraphRAG by evaluation and query class.
- [ ] Gate visual retrieval, sandboxed table execution, and tenant-safe domain
      adaptation with dedicated relevance, security, privacy, and deletion
      tests.
- [ ] Add unified model-job admission control, external-processing governance,
      generation/projection revision fencing, and coordinated backup manifests.
- [ ] Pass parser, retrieval, citation, abstention, deletion, tenant-isolation,
      injection, backup/restore/tombstone-replay, and strict/optional failure
      gates on a frozen holdout.

## Phase 16 — Multi-Server or Kubernetes Migration

- [ ] Define target deployment platform and managed service boundaries.
- [ ] Add image tagging and migration-job strategy.
- [ ] Add ingress, probes, and secrets plan.
- [ ] Verify release and rollback in target environment.
