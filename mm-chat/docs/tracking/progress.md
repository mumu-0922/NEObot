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

### Phase 10.1 — Standalone project-root cutover

- [x] Relocate the complete Next.js application, assets, tests, manifest,
      lockfile, and Docker build under `mm-chat/frontend/`.
- [x] Add the frontend service and persistent same-origin `/mm-api` edge to the
      single-server Compose topology.
- [x] Fence local frontend builds through the same project-root Compose path;
      keep immutable-digest image promotion as an optional future hardening
      path, not the active standalone gate.
- [x] Add a clean-copy structural gate that rejects outer-root paths, symlinks,
      and build contexts escaping `mm-chat/`.
- [x] Build and run the relocated frontend from `mm-chat/`, verify its Docker
      healthcheck, and prove the same-origin `/mm-api/ready` path reaches the
      healthy Go/Postgres/Redis/MinIO stack from the Windows host.
- [x] Pass the full isolated-copy install/test/build gate for the frontend, Go
      backend, and Python 3.13 RAG package without access to the original root.
- [x] Pass the complete clean-copy frontend, Go, RAG, Compose, backup/restore,
      and visual-regression gate before deleting the former root application.

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
- [x] Keep search writes fail-closed while the composer is showing server
      conversations; route reasoning, skills, and plugins through their
      explicit server-mode paths instead of mutating local conversation state.
- [x] Use abort-only stop/new-chat/session-switch handling in server mode so
      local IndexedDB sync is not invoked.
- [x] Fail closed for local-only actions that do not yet have server endpoints.
- [x] Browser-smoke server-mode file upload, attachment rendering, refresh, and
      local rollback in Phase 11.5.

### Phase 11.4D — Server model, skill, and plugin parity

- [x] Publish the Go-managed provider type/model list through `/api/config`
      without copying `PROVIDER_API_KEY` into the frontend container.
- [x] Resolve and inject explicitly selected text skills before every
      server-mode send while leaving search gated.
- [x] Forward the server-mode reasoning toggle as `useReasoning`, translate it
      to OpenAI-compatible `reasoning_effort: high`, and keep it omitted when
      disabled.
- [x] Add bounded `POST /v1/chat/tools/plan` provider planning through Go and
      reject invalid, oversized, or unoffered provider tool calls.
- [x] Execute only active installed plugin functions through the existing
      hardened Next plugin route; keep plaintext plugin auth out of the Go
      planning request.
- [x] Bound plugin-result context to 64 KiB, mark it as untrusted data, and
      preserve execution errors so the final model cannot silently report
      success.
- [x] Re-enable the existing skill/plugin composer menus in server mode and
      persist their browser-side selection without changing the UI shell.
- [x] Make the CSRF same-origin guard compare browser `Origin` with the external
      HTTP `Host` by default, so Docker host-port mappings do not look
      cross-origin; trust forwarded host/proto only when
      `TRUST_PROXY_HEADERS=true`.
- [x] Verify a real Weather plan/execution/final Go SSE response and persisted
      refresh result, then remove the isolated smoke conversation.

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
- [x] Add Collection/Document/Consent repositories and APIs with locked File
      binding and transactional Outbox writes.
- [x] Pass the complete two-user/two-team ACL, Consent, revision, deletion,
      idempotency, and Outbox producer/source-recovery gate.

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
- [x] Implement Personal/Team Collection repositories, ACLs, revisions, and
      transactional Outbox writes.
- [x] Implement logical Document/Version lifecycle, locked File binding/delete,
      authorized content reads, reprocess, and tombstones.
  - [x] Lock direct File deletion against live Knowledge Version bindings and
        write durable `file.object.delete.requested` Outbox work.
  - [x] Add first Document/Version bind, Parse admission, Job, and Outbox
        transaction.
  - [x] Register strict authenticated first-bind HTTP admission.
  - [x] Add ACL-checked Document list/get and Active-only content routes.
  - [x] Add immutable replacement Version admission with deterministic File
        locks, Parse authority, idempotency, Job, and Outbox.
  - [x] Add same-Version reprocess admission with idempotent Job and Outbox.
  - [x] Add Document tombstone deletion, Job cancellation, per-Version purge
        admission, visibility fences, and deletion Outbox events.
- [x] Implement operator Governance and Collection/User Consent services with
      purpose/data-type and expiry/revision fences.
  - [x] Add strict operator Governance manifest apply/disable commands,
        immutable Profiles, Head revisions, and transactional Outbox events.
  - [x] Add Collection Consent reads/grant/revoke, ACL, expiry validation,
        semantic idempotency, processing revisions, and Outbox.
  - [x] Add User Query Consent reads/grant/revoke, expiry validation, semantic
        idempotency, query revisions, and Outbox.
  - [x] Materialize elapsed Consent expiry, advance subject revisions, and
        emit expiry Outbox without forging a user revocation decision.
- [x] Wire protected Knowledge routes, safe DTOs/errors, bounded metrics, and
      later-frontend adapter contracts.
- [x] Pass unit/race/PostgreSQL 16 ACL, migration, deletion, idempotency, and
      Outbox producer/source-recovery gates.
- [x] Promote Phase 15.1D with fail-closed production env validation,
      immutable image rollback, automated published-migration replay,
      synchronized contracts/deployment/persistence docs, all quality/security
      gates, and independent `P0/P1/P2 = 0/0/0` review.

#### Phase 15.2 — Single-server Python RAG consumer and indexing

- [x] Complete the Owner Grill and lock workload, file, image, retrieval,
      consent, deletion, chat, citation, latency, evaluation, and R2 backup
      boundaries.
- [x] Create the detailed single-server Python Outbox consumer, parsing,
      Postgres hybrid indexing, query, citation, resource, and recovery design
      in `docs/architecture/phase-15-2-single-server-python-rag-consumer-indexing-plan.md`.
- [x] Close independent xhigh design review after three rounds with final
      `P0/P1/P2 = 0/0/0`.
- [x] Freeze the internal Evidence API, workload authentication, Canonical
      Block/Chunk, Generation/Projection, and migration contracts.
- [x] Bake off and pin the digest/version/resource operational baseline for
      PostgreSQL BM25/pgvector, three Chinese tokenizers, two dense shapes,
      Exact/RRF, logical restore, crash recovery, and rollback.
- [x] Close the independent xhigh Phase 15.2A review with final
      `P0/P1/P2 = 0/0/0` after rerunning every changed executable assertion.
- [ ] Promote the production tokenizer/dimension/search DDL only after the
      Relevance Set, SLO, license, upgrade, and rollback gates pass.
- [ ] Implement the private Python consumer/worker, applied-event ledger,
      lease fencing, parser routing, artifacts, chunking, embedding, publish,
      purge, and reconstruction paths.

##### Phase 15.2B — Durable Consumer dark-run

- [x] Lock the executable Phase 15.2B plan, Governance Mapping input,
      migration/function signatures, DLQ/Replay semantics, least-privilege
      roles, dark-run gate, and verification matrix.
- [x] Implement complete extension-independent migration `010`, model-aware
      Governance/Consent compatibility, lease fencing, ledger, purge fan-out,
      replay audit, and conservative rollback.
- [x] Implement the Python worker package, Postgres Function adapter,
      Poll/Rescan, Redis wake, job heartbeat/retry/DLQ, health, metrics, and
      operator replay CLI with real handlers disabled.
- [x] Add isolated Compose/credential/resource wiring and pass all migration,
      replay, crash, Redis-loss, permission, Python, image, and review gates.

##### Phase 15.2C — Generation-bound parsing and indexing

- [x] Complete four parallel xhigh audits of the runtime execution gap,
      generation dispatch, Provider/Search contract, and security/operations
      boundary.
- [x] Close independent review and lock the executable Phase 15.2C plan,
      immutable `010`, Search-only `011`, Dispatcher `012`, Processing Request,
      Gateway, staging/finalizer, deletion, rebuild, activation, rollback, and
      verification contracts.
- [x] Add the C0 closed Provider Fixture Schema, strict loader, RFC 8785 hash,
      secret/placeholder/freeze gates, full-width Jina 1024/2048 public drafts,
      MinerU/Rerank drafts, no-network Fake Provider, and governance-example
      fail-closed guard without enabling runtime handlers; independent review
      reached `P0/P1/P2 = 0/0/0`.
- [x] Lock the C0 Provider Capture Harness threat model, exact Jina/MinerU
      allowlist and call budget, Evidence Snapshot v1, manual review/freeze
      flow, rollback, and staged MinerU local-upload boundary.
- [x] Implement the operator-only no-network-by-default Capture CLI with
      process-environment-only credentials, synthetic inputs, bounded strict
      responses, canonical redacted evidence, private atomic no-overwrite
      output, and injectable streaming MockTransport; keep it outside production
      scripts and the runtime image. Split CLI, HTTP, common validation, and
      parent-FD evidence writing into reviewable modules.
- [x] Add targeted regressions for dry-run, missing credentials, target and
      redirect rejection, Content-Type/size/UTF-8/JSON/shape gates, evidence
      redaction, MinerU `unknown_submission`, permissions, determinism,
      symlink/existing-target refusal, and unchanged dark-run registries.
- [x] Execute and validate authorized success-path Captures for Jina and MinerU,
      revoke temporary credentials, and complete independent Evidence review.
  - [x] Execute and validate the fixed Jina 1024/2048 Embedding plus Rerank
        Capture; verify canonical Hash, closed Schema, `3/3` budget,
        `0700/0600` permissions, redaction, and Git-external storage.
  - [x] Execute the staged MinerU Submit Capture and review its Evidence without
        uploading the Signed URL or polling.
  - [x] Revoke the MinerU Token exposed by an unsafe partial-echo terminal input
        helper, ban partial Key echo, and retain only the redacted Evidence.
  - [x] Execute the first full MinerU Lifecycle attempt: Allocate, Signed PUT,
        and four Poll calls passed; the sole Result Download ended as legacy v2
        `unknown_download` with private redacted Evidence and no retry.
  - [x] Add closed HTTPX transport failure classification for future incomplete
        Evidence while preserving legacy v2 validation and fail-closed behavior
        for non-transport programming errors.
  - [x] Execute one separately authorized follow-up Lifecycle Capture after the
        diagnostic patch passed review; it again stopped at Download and safely
        recorded closed `connect_error` without promotion or retry.
  - [x] Diagnose CDN routing without credentials: Private Proxy TLS failed for
        the CDN while all three fixed MinerU hosts passed direct TLS.
  - [x] Use the Owner-authorized one-time Token for one all-direct Capture; it
        passed Allocate/Upload/Poll and stopped at a redacted Download contract
        failure, with no further Token reuse.
  - [x] Add a closed, backward-compatible `downloadFailureClass` for future
        failed Evidence without retaining status, headers, body, ZIP names, or
        exception details.
  - [x] Execute one no-echo all-direct diagnostic Capture, identify
        `archive_invalid`, move Evidence outside Git, and revoke the Token.
  - [x] Add a closed, backward-compatible `archiveFailureClass` without Entry
        names/content and fix deterministic Mock ZIP timestamps exposed by the
        expanded regression suite.
  - [x] Identify the live Cloud v4 Middle artifact as `layout.json`, preserve
        Local/Open-source `middle.json` compatibility, and complete a real
        `lifecycle_complete` Capture without relaxing the four-role Gate.
- [x] Complete the Evidence-to-Fixture Promotion Readiness audit, snapshot and
      hash current Jina/Elastic/MinerU public authorities outside Git, map every
      proven/unknown field, and keep all Fixtures `draft/blocked`.
- [x] Select MinerU Local Upload Batch as the production candidate instead of
      Remote URL submission; record that the initial staged Capture proved
      Allocate only and later Lifecycle summaries remain non-promotable.
- [x] Persist the draft-only MinerU Local Batch Contract plan with a distinct
      Provider Kind, exact six-operation set, Remote/Local isolation, unknown
      Wire rules, validation gates, and rollback boundary.
- [x] Implement and review the separate `mineru_local_batch` public Draft,
      closed Allocate fixture, no-network Fake replay, Remote/Local isolation,
      unknown-wire rejection, and draft Freeze rejection without Runtime
      activation.
- [x] Map the final MinerU Lifecycle success into the Local Batch Draft as
      `redacted_capture_summary` metadata; keep Upload/Poll/Download Unknown,
      replace stale `*_NOT_CAPTURED` blockers, and reject Summary-only
      Observed Wire, dangling Support references, and unobserved Response Cases;
      close independent review at `P0/P1/P2 = 0/0/0`.
- [x] Extend the closed MinerU Operation Schema for Allocate/Upload/Batch Poll/
      Result Download while preserving the existing Remote URL Draft.
- [x] Persist the isolated MinerU Lifecycle Capture Harness plan with fixed
      Allocate/Upload/Poll/Download budgets, Evidence v2, dynamic-target and
      ZIP gates, redacted failure states, and v1 compatibility boundary.
- [x] Implement and review the no-network-by-default MinerU Lifecycle Capture
      CLI without performing a real Provider call or enabling Runtime.
- [ ] Freeze fixture-citable dynamic-target metadata, Upload/Poll/Download Wire,
      Result Entry Schema/Content, Citation Locator, recovery, stable errors,
      immutable build, Region, terms, and SLA for the Local Batch Contract.
- [x] Add and verify the Owner-approved explicit private-proxy compatibility
      path after the first Jina Capture proved WSL direct egress unavailable;
      generic proxy variables must remain ignored and no retry is allowed.
- [ ] Freeze redacted MinerU/Jina wire fixtures, immutable Model/API builds,
      Provider recovery behavior, license, retention, and SLA.
- [x] Lock the C1 no-network Offline Parser and Canonical IR implementation
      plan, including format/security routing, sandbox protocol, Canonical IR
      and Source Locator v2, deterministic manifests, hierarchical chunking,
      future `012` payload/lineage separation, and Evidence/Citation v2 impact.
- [x] Complete C1.1 Contract and Corpus: package 18 Closed Schemas; freeze all
      24 Logical Hash envelopes and the 22-code Stable Error matrix; add 49
      source fixtures plus 27 deterministic binary recipes; enforce strict
      JSON/JCS, semantic ordering/exact-cover/DAG/count/hash gates, existing-
      wheel packaging checks, and Python/Go/Node equality without enabling
      runtime handlers.
- [x] Complete C1.2 Router and Sandbox Protocol: match all 49 frozen route/error
      expectations; add exact MMCP v1 UDS framing/binding/outcome gates;
      isolate each job behind PID1 Subreaper, pidfd/process-group handshake,
      RLIMIT and hashed Seccomp; add owned quota/marker/flock/dir-FD cleanup;
      pass OOM/timeout/cancel/fork-bomb/residual, Docker no-network UID `10002`,
      866-test/91.16%-coverage, wheel/JCS/security gates without enabling a
      Native Parser or production Registry.
- [x] Complete C1.3A TXT/Markdown/HTML Native Parsers behind the same Child and
      Seccomp boundary: freeze BOM -> UTF-8 -> GB18030 decoding, CommonMark +
      Table and hardened HTML profiles, exact Raw-byte/Scalar/Line locators,
      closed child-internal Native Artifact framing, Parent JCS/length/hash/
      limit/Source-binding validation, and 1069-test/91.19%-coverage plus
      dependency/JCS/wheel/security/Docker gates. Keep MMCP at zero-body
      `FORMAT_UNSUPPORTED`, Canonical IR unstaged, and Registry/Dispatch/
      Provider/Postgres/Redis/MinIO/migrations `011/012` closed; independent
      review closed at `P0/P1/P2 = 0/0/0`.
- [x] Complete C1.3B DOCX/PPTX/XLSX/CSV Native Parsers: upgrade the internal
      Artifact to v2 multi-Source-Unit Locators; share one hardened OPC/XML
      admission capability; preserve document/slide/sheet/table/formula
      structure without fetch or formula execution; pass 1408-test/92.32%-
      coverage, dependency/JCS/wheel/scanner/Docker gates, and close independent
      review at `P0/P1/P2 = 0/0/0`. Keep MMCP zero-body/non-stageable and all
      Provider/Registry/Dispatch/Postgres/Redis/MinIO/`011/012` paths closed.
- [ ] Implement and verify the offline parser, Canonical IR, parent/child
      chunking, Provider fake servers, and deterministic manifests.
- [ ] Select the production tokenizer/dimension/search profile using real Jina
      vectors and the frozen 80/20 relevance/SLO/license gates, then apply
      `011`; Phase 15.2E must not reopen the holdout for tuning.
- [ ] Apply `012`, cut Go producers to Processing Request + Outbox, and add
      generation-bound dispatch, gateways, handlers, staging, atomic publish,
      purge, and rebuild.
- [ ] Complete Canary, controlled stage activation, legacy reconciliation,
      crash/delete/consent/governance races, and independent review while
      keeping user Query and production Promotion off; final review must reach
      `P0/P1/P2 = 0/0/0`.
- [ ] Implement private hybrid query, Go-side source reauthorization,
      strict grounded chat, visible degradation, and clickable citations with
      minimal frontend change.
- [ ] Add Compose resource limits, restic/R2 coordinated backup/restore,
      independent ACL/Consent/deletion/injection/citation/parser corpora, full
      security/performance/recovery gates, and production promotion review;
      consume the Phase 15.2C relevance report without reopening its holdout.

- [x] Replace the single-bootstrap-user ceiling with admin-invited independent
      user sessions and versioned Team membership.
- [x] Add Team, Membership, Personal/Team Collection, Knowledge Document, and
      per-processor Collection/User Consent schemas and APIs.
- [x] Enforce Personal-owner and Team-role ACLs across file binding, metadata,
      consent, deletion, and cross-user/cross-team control-plane tests.
- [ ] Extend those ACL fences through future indexing, query, and citation
      serving paths.
- [x] Freeze the canonical block/chunk schema, ACL invariants, and two-level
      corpus-generation/document-materialization profile contract.
- [ ] Freeze the golden evaluation corpus.
- [x] Define the workload-authenticated Go-to-RAG evidence API, Go-side source
      reauthorization, citation minting, and strict/optional failure contracts.
- [ ] Add private Python query and indexing services, Postgres outbox rescan,
      and non-authoritative Redis wake-up/lease/cache handling.
- [ ] Pass real `knowledge_outbox` consumer duplicate/out-of-order replay,
      contiguous checkpoint, restart/Redis-loss recovery, tombstone
      propagation, and Postgres-to-search projection reconstruction gates.
- [ ] Preserve original files and structured parser artifacts for reproducible
      reindexing.
- [ ] Add format/page-aware parsing, quality gates, and parent/child/window
      chunking with exact provenance.
- [x] Bake off the single-server Postgres search projection mechanics using
      pgvector plus true BM25; do not deploy Qdrant for the locked workload.
- [ ] Pin the production Search Profile after relevance/SLO/license gates.
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

## Active Cutover — Standalone Parity Sliced Migration

Active plan: [`../architecture/standalone-parity-sliced-cutover-plan.md`](../architecture/standalone-parity-sliced-cutover-plan.md).
Active process log: [`standalone-parity-sliced-process.md`](./standalone-parity-sliced-process.md).

- [x] Create the active sliced cutover plan and dedicated process log.
- [x] Record the owner directive: migrate one bounded group at a time and test
      that group immediately.
- [x] G1 Conversation and Message Operations: remove core chat-management
      server-mode blockers with persisted smoke.
  - [x] G1.1 Conversation metadata operations: server-backed chat deletion,
        chat renaming, pinning, and system instruction editing.
  - [x] G1.2 Message deletion and retraction.
  - [x] G1.3 Message editing and edit branches.
  - [x] G1.4 Regeneration and message version switching.
  - [x] G1.5 Chat duplication and assistant presets.
  - [x] G1.6 Smart rename / title generation through server-owned route.
- [x] G2 Related Questions and Agent/Assistant Catalogs: replace
      helper-generation and catalog reads with server-owned contracts.
  - [x] G2.1 Related questions through
        `POST /v1/chat/conversations/{id}/related-questions`.
  - [x] G2.2 Agent/Assistant catalog list/detail through `GET /v1/agents` and
        `GET /v1/agents/{identifier}`.
  - [x] G2.3 Frontend server-mode services call Go `/v1/*` routes instead of
        transitional Next `/api/*` routes.
- [x] G3 Auth, Runtime Config, Provider Settings, and BYOK: make session, model,
      config, and provider state server authoritative.
  - [x] G3.1 Go runtime config, server-default provider models, BYOK public-key
        route, and frontend API-client boundary shells.
  - [x] G3.2 Frontend Auth lifecycle wired to Go login/logout/me.
  - [x] G3.3 Provider Settings/BYOK UI adapters stop direct server-mode
        transitional `/api/*` calls.
  - [x] G3.4 Hosted/dev auth behavior and same-origin smoke.
- [x] G4 Plugin Registry, Install, and Execution Final Ownership: server
      plugin registry/install/execute ownership and live smoke complete; final
      Next route deletion remains in G9.
  - [x] G4.1 Server plugin tool planning and bounded untrusted result context
        wired through Go `/v1/chat/tools/plan` and frontend orchestration.
  - [x] G4.2 Plugin registry/list adapter.
  - [x] G4.3 Plugin install/custom-manifest adapter.
  - [x] G4.4 Plugin execute API-client boundary.
  - [x] G4.5a Go plugin execution fail-closed admission and server-mode
        transitional route retirement.
  - [x] G4.5b Minimal Go plugin execute sandbox.
  - [x] G4.5c.1 Go registry-backed id-only execution bridge.
  - [x] G4.5c.2a Postgres-backed plugin registry persistence.
  - [x] G4.5c.2b Go custom OpenAPI manifest conversion.
  - [x] G4.5c.2c Go built-in plugin result normalizers.
  - [x] G4.5c Registry-backed plugin execute finalization.
  - [x] G4.6a Zero-cost in-process plugin smoke harness producing bounded
        context and a persisted final Go-stream answer.
  - [x] G4.6b Live deployed-frontend smoke with one installed plugin producing
        bounded context and final Go-stream answer.
- [ ] G5 Search and Web-Enrichment Toggle: keep paused until owner reopens, then
      make Search server-owned or explicitly unavailable.
- [ ] G6 Voice, Image Generation, and Code Execution Jobs: move job routes behind
      server admission, storage, and audit controls.
  - [x] G6.1 Server-mode fail-closed capability gates: disabled
        `voice`, `imageGeneration`, and `codeExecution` capabilities prevent
        service-layer fallthrough to transitional Next routes.
  - [x] G6.2 Voice synthesis/transcription Go job admission: Go registers validating fail-closed `/v1/voice/transcribe` and `/v1/voice/synthesize` routes.
  - [x] G6.3 Image generation Go job admission: Go registers a strict `modelRef + prompt` fail-closed `/v1/images/generations` route.
  - [x] G6.4 Code execution Go job admission: Go registers a strict `modelRef + language + code` fail-closed `/v1/code/executions` route.
  - [ ] G6.5 Job audit/rate-limit/cancel metadata and provider smoke.
    - [x] G6.5a Admission audit metadata: voice/image/code fail-closed services record sanitized job events without prompt/code/text/audio payloads.
    - [x] G6.5b Shared job rate-limit and cancellation gates: fail-closed `/v1/jobs/{jobId}/cancel` is registered and covered by global rate-limit middleware.
    - [ ] G6.5c Real voice/image executors with output storage and provider smoke.
      - [x] G6.5c.1 Storage-only result artifact boundary: Go validates
            image/audio artifact metadata and stores future executor outputs
            through backend file/object storage without provider calls.
      - [ ] G6.5c.2 Real voice executor with stored audio artifacts and
            configured-provider smoke.
        - [x] G6.5c.2a Voice executor opt-in seam: Go can call a configured
              executor only after an explicitly configured sanitized admission
              audit recorder accepts the event, passes multipart audio to
              transcription, and blocks synthesis execution until artifact
              storage is configured.
        - [ ] G6.5c.2b Real provider-backed voice executor and authorized
              configured-provider smoke.
          - [x] G6.5c.2b.1 OpenAI-compatible voice executor, Go route wiring,
                and gated live smoke harness for `/audio/speech` and
                `/audio/transcriptions`.
          - [ ] G6.5c.2b.2 Authorized configured-provider voice smoke.
          - [ ] G6.5c.2b.3 Free/simple TTS provider selection and smoke,
                keeping the Go `/v1/voice/*` seam for a future free hosted API;
                local Piper-style TTS is not preferred on this VPS. Browser
                speech synthesis remains local fallback/test guard only.
      - [x] G6.5c.3 Real image executor with stored image artifacts and
            configured-provider smoke.
        - [x] G6.5c.3a Image executor opt-in seam: Go can call a configured
              executor only after an explicitly configured sanitized admission
              audit recorder accepts the event and stores generated bytes
              through backend image artifacts; see
              `docs/contracts/media-job-executor-seams.md`.
        - [x] G6.5c.3b Real provider-backed image executor and authorized
              configured-provider smoke.
          - [x] G6.5c.3b.1 OpenAI-compatible image executor plus gated live
                smoke harness.
          - [x] G6.5c.3b.2 Authorized configured-provider image smoke passes
                against an image-capable key/endpoint.
        - [x] G6.5c.3c Go HTTP route wiring: `cmd/api` wires
              `/v1/images/generations` to the configured image job service with
              sanitized audit logging, OpenAI-compatible executor opt-in, and
              backend file/object-storage artifact storage when dependencies
              are present.
        - [x] G6.5c.3d Frontend server-mode image adapter and capability
              reopen: `generateImage()` calls Go `/v1/images/generations`,
              maps artifact metadata to server-backed image attachments, and
              keeps local mode on the transitional `/api/chat/generate-image`
              route.
    - [x] G6.5d Code execution sandbox contract before any real executor is enabled: documented in `docs/contracts/code-execution-sandbox-contract.md`; runtime remains disabled.
    - [x] G6.5e Live provider smoke authorization gate: default-deny
          `providersmoke` requires explicit enablement, exact quota approval
          text, exact target, and run id before any live voice/image smoke.
- [x] G7 Knowledge, Document Parsing, RAG, and Citations: pass Phase 15 runtime
      parser/index/query/citation gates.
  - [x] G7.1 Decision lock and runtime inventory plan: created
        `docs/architecture/g7-rag-citation-cutover-plan.md` and
        `docs/tracking/g7-rag-citation-process.md` after owner grill; locked
        real MinerU + Jina + Postgres profile, admin env/secret credentials,
        selected-collection queries, strict refusal, auto-indexing, deletion
        invisibility, retry budget, and G9 legacy-route handoff.
  - [x] G7.2 Admin provider config and fail-closed readiness.
  - [x] G7.3 Provider-backed parser/index profile gate.
  - [x] G7.4 Canonical IR to chunks and Postgres projection.
  - [x] G7.5 Worker dispatch, rebuild, delete, and retry loop
        (`G7.5.1` readiness gate, `G7.5.2` job-context admission seam, and
        `G7.5.3` Go parse job materialization binding, `G7.5.4` Go purge job
        Generation binding, `G7.5.5` admitted Python handler skeletons, and
        `G7.5.6` parse handler dependency seam, `G7.5.7` passage-embedding
        dependency seam, `G7.5.8` purge dependency seam, and `G7.5.9` default-off
        Postgres purge projection gateway adapter, and `G7.5.10` live
        Postgres purge gateway integration proof, `G7.5.11` default-off
        Postgres passage-embedding projection gateway, `G7.5.12`
        default-off Jina passage embedding provider gateway, `G7.5.13`
        Jina + projection handler dependency bundle, `G7.5.14` parse source
        gateway composition seam, and `G7.5.15` default-off Postgres parse
        source metadata gateway, and `G7.5.16` default-off local object-byte
        gateway, `G7.5.17` Go private source-object gateway + Python HTTP
        adapter seam, `G7.5.18` default-off Postgres parse projection
        adapter seam, `G7.5.19` default-off Postgres parse projection
        gateway function, and `G7.5.20` default-off MinerU local-batch
        allocate gateway, and `G7.5.21` default-off MinerU signed-upload
        transport seam, and `G7.5.22` default-off MinerU batch poll/result seam
        done, and `G7.5.23` default-off MinerU result ZIP download seam done;
        and `G7.5.24` default-off MinerU result ZIP archive validation done;
        `G7.5.25` default-off MinerU archive artifact extraction done, and
        `G7.5.26` default-off MinerU artifact decode admission done, and
        `G7.5.27` default-off MinerU canonical mapping input done, and
        `G7.5.28` default-off MinerU full-Markdown text-baseline mapper done;
        `G7.5.29` default-off MinerU archive-to-text-baseline composition done;
        `G7.5.30` default-off MinerU text-baseline parser adapter done;
        `G7.5.31` parse-handler dependency composition proof with the MinerU
        parser adapter done; `G7.5.32` conservative MinerU basic page-locator
        mapper done; `G7.5.33` MinerU `sourceText` page-locator admission done;
        `G7.5.34` MinerU opaque table element page-locator admission done;
        `G7.5.35` MinerU opaque image element page-locator admission done;
        `G7.5A` MinerU text-baseline locator hardening closure done; `G7.5T`
        disposable PostgreSQL integration gate restored and proven with cleanup;
        `G7.5B` live `017` parse projection staging proof done; real handler
        dispatch still gated; `G7.5C` Python `PostgresAdapter` parse projection
        live proof done against disposable PostgreSQL; `G7.5D` job-only worker
        promotion gate split from outbox registry gate; `G7.5E` explicit purge
        handler promotion factory added without provider quota; `G7.5F`
        promoted purge job-runner live smoke done against disposable
        PostgreSQL; `G7.5G` explicit passage-embedding promotion factory added
        without live provider calls; `G7.5H` parse source-gateway worker
        settings admission added; `G7.5I` promoted passage-embedding
        job-runner live smoke with mocked Jina and migration `018` fix done;
        `G7.5J` explicit parse dependency factory added while parse remains
        default-unpromoted; `G7.5K` MinerU local-batch archive provider
        composition added behind a default-off seam; `G7.5L` promoted parse
        job-runner live smoke done against disposable PostgreSQL with mocked Go
        source-object bytes, mocked MinerU archive transport, and migration
        `019` source-metadata grant fix; `G7.5M` normal `Worker(settings)`
        parse auto-promotion wired behind existing parse settings/profile gates;
        `G7.5N` parse terminal finalizer and migration `020` atomically enqueue
        pending `passage_embedding` after parse success; `G7.5O` promoted
        parse-to-embedding two-stage worker smoke proves the pending embedding
        job is claimed, mocked Jina vector staging completes, and the search row
        becomes ready; `G7.5P` embedding completion publish finalizer publishes
        the materialization, activates the document/current version, advances the
        projection head, and terminally commits the embedding job).
  - [x] G7.6 Private query and Go reauthorization
        (`G7.6A` selected-collection evidence candidate fetch is live-proven:
        active published materializations in explicitly selected collections can
        return reference-only candidates, while unselected collections return
        none; `G7.6B` Go hydration now reauthorizes selected references through
        Postgres, binds `source_span_hash` + `content_hash`, rejects stale or
        unselected refs, and returns no body on failure; chat answer/citation
        assembly remains G7.7).
  - [x] G7.7 Strict/optional chat answer and basic citations
        (`G7.7A` Go strict-RAG chat skeleton done: selected collection metadata
        is parsed and validated, authenticated session ID is available to RAG
        assembly, and strict mode fails closed with a persisted refusal before
        any answer provider receives hydrated source text; `G7.7B` Go citation
        minting now emits hash-bound citation metadata and bounded snippets at
        the pending answer gate; `G7.7C` answer-purpose governance now requires
        user query consent plus selected-collection answer consent before
        citations/evidence can advance to answer assembly; `G7.7D` strict
        grounded answer context now calls the selected answer provider only
        after governance, buffers provider output, verifies citation markers,
        and persists answered/refusal outcomes fail-closed; `G7.7E` optional
        mode now records explicit no-Knowledge-evidence degradation metadata;
        `G7.7F` frontend server mode now sends selected Knowledge collection
        IDs to Go strict RAG and renders basic citation/status cards; `G7.7G`
        same-origin `/mm-api` smoke verified strict empty-evidence refusal and
        persisted Knowledge metadata on the rebuilt local stack; `G7.7H`
        records message metadata as sufficient for first-version citation cards
        and defers a dedicated citation table).
  - [x] G7.8 Live MinerU + Jina + Postgres smoke and operational proof
        (`G7.8A` preflight found rebuilt local backend missing admin provider
        secrets without consuming quota; after local secret injection, `G7.8B`
        proved real MinerU parse + Jina 1024 passage embedding + Postgres
        publish on doc11; `G7.8C` wired the Go strict-RAG assembler, applied
        migration `024`, and live-proved `outcome=answered`, `citationCount=1`,
        and answer `PHOENIX-G78-LIVE-042 [1]` from the selected knowledge base).
  - [x] G7.9 G8/G9 handoff and G7 closure checklist: closure recorded in
        `docs/tracking/g7-rag-citation-process.md`; G8 owns richer Knowledge UI
        and retrieval-quality upgrades, G9 owns legacy Next RAG/doc-parse route
        removal plus local-authority cleanup, and G10 owns clean-copy/delete
        gates.
- [ ] G8 Teams and Knowledge UI Wiring: connect existing Go control-plane APIs
      to the current frontend theme.
  - [x] G8.1 API client adapter seam: `TeamApi` and `KnowledgeApi` are now
        typed on `NeoChatApiClient`, fail closed in local mode, call Go
        `/v1/teams/*` and `/v1/knowledge/*` in server mode, and expose
        server capability flags with targeted adapter tests.
  - [x] G8.2 Teams shell/actions: added current-theme Settings/Teams tab
        with server-mode Team list/detail, create/rename, member role changes,
        invites, revoke, leave, fail-closed local mode, and identity-boundary
        composition tests.
  - [x] G8.3 Knowledge collection/document shell: server-mode Knowledge Base
        branch with collection CRUD, document list/status, upload/bind, and
        optimistic deletion invisibility through the typed API client.
  - [x] G8.4 Consent UX: collection/query consent screens for administrator
        env-backed MinerU/Jina processing with fail-closed copy, no key capture,
        and API-client-only grant/revoke calls.
  - [x] G8.5 Cross-user/team UX smoke: server-mode Knowledge selection now
        lists Go-visible Personal/Team collections, avoids local OPFS file
        selection, and carries selected collection IDs into strict Go RAG
        config/metadata.
- [x] G9 Data Authority and Legacy Route Removal: remove production local-mode
      authority and replaced Next API handlers.
  - [x] G9.1 Route inventory freeze: added a static guard for the current 25
        transitional `src/app/api/**/route.ts` handlers before deletion slices.
  - [x] G9.2 RAG/doc-parse route removal: removed replaced `/api/rag/*`,
        `/api/doc-parse*`, and `/api/chat/rag-queries` handlers, updated the
        route inventory to 19 handlers, and fail-closed old local services.
  - [x] G9.3 Config/provider/BYOK route removal: removed `/api/config`,
        `/api/providers/models`, and `/api/byok/public-key`, kept server-mode
        `/v1/*` adapters, and made local adapters fail closed.
  - [x] G9.4 Plugin/agent route removal: removed `/api/plugins/*` and
        `/api/agents*`, kept server-mode `/v1/*` adapters, and made local
        plugin/agent adapters fail closed.
  - [x] G9.5 Local production authority removal: hard-fence browser-local
        IndexedDB/localforage/OPFS authority to dev/import-only paths.
    - [x] G9.5a Zustand persistence authority fence: `getAppDbStorage` and
          `getBrowserLocalStorage` return no-op storage in server mode, while
          explicit browser-import direct `appDb`/OPFS reads remain available.
    - [x] G9.5b OPFS write/delete authority fence: `saveToOPFS`,
          `writeToOPFS`, `deleteFromOPFS`, and `deleteOPFSDirectory` throw in
          server mode; OPFS list/read remain import-capable.
    - [x] G9.5c Direct `appDb` authority sweep: replaced direct chat message
          `appDb.setItem/removeItem` calls with runtime helpers that throw in
          server mode; explicit import reads remain available.
  - [x] G9.6 Clean-copy preflight: `verify-standalone.sh` and
        `verify-standalone.sh --full` passed from an isolated `mm-chat/` copy;
        frontend format/lint/typecheck/test/build, Go vet/test, and RAG
        ruff/mypy/pytest all passed without former-root imports/build context.
- [ ] G10 Operations, Visual Regression, Clean Copy, and Delete Plan: complete
      final closure gates before any former-root deletion.
  - [x] G10.1 Former-root delete-plan dry run: added a non-destructive
        candidate manifest script, protected-path boundary, approval phrase,
        rollback steps, and deployment-doc index link.
  - [x] G10.2 Operations and backup/restore closure: record backup checksums,
        Postgres temporary restore drill, MinIO restore drill, Compose
        source-build/restart/rollback evidence, and runtime smoke.
    - [x] G10.2a Local live-stack backup/restore smoke: lower-level Postgres
          and MinIO backups ran with a temporary `BACKUP_DIR`, checksums passed,
          a disposable Postgres restore verified 24 migrations and core tables,
          a temporary MinIO restore bucket restored 17 objects and checked 5
          sampled object keys, backend/frontend restart returned healthy, and
          temp backup artifacts plus the restore DB were removed.
    - [x] G10.2b Build-based Compose closure: owner selected source-build
          deployment instead of registry-image promotion; `docker compose`
          built backend/frontend/rag-worker from `mm-chat/`, ran migrations,
          recreated the app/RAG services, and passed root, `/mm-api/ready`,
          backend `/ready`, and RAG worker health smokes.
      - [x] G10.2b.1 Optional release image script: `scripts/release-images.sh`
            remains available for future registry promotion, but GHCR push and
            digest env proof are not required for the current standalone gate.
      - [x] G10.2b.2 Compose source-build proof: backend, frontend, and RAG
            worker build/up passed from the standalone project tree.
  - [x] G10.3 Visual/interaction closure: record desktop/mobile smoke for app
        shell, chat streaming, model/provider visibility, Knowledge citation
        cards, Files/upload when configured, and navigation.
    - [x] G10.3a Automated UI/visual contract smoke: 70 frontend tests passed
          for app shell/mobile accessibility, citation styling/cards, markdown
          rendering, server Knowledge wiring, model resolution, and server
          defaults; HTTP app shell returned valid Next HTML.
    - [x] G10.3b Browser screenshot/interaction smoke: Windows Chrome at
          `C:\Program Files\Google\Chrome\Application\chrome.exe` ran CDP smoke
          for desktop `1365x768` and mobile `390x844`, captured four PNGs,
          verified app shell/composer/model controls, clicked model/Knowledge/
          search controls, showed the Knowledge citation card and mobile drawer,
          and preserved the server-mode search fail-closed toast.
  - [ ] G10.4 Owner-confirmed former-root cleanup: require the exact owner
        approval phrase before running any generated destructive command.

## Phase 11 Owner Parity Regression Closure

- [x] G11 Owner parity regression closure: fix live owner-test gaps before any
      former-root deletion.
  - [x] G11.1 Chat image understanding: Go chat streaming now resolves
        server-backed `image/*` message attachments from file storage and sends
        them to OpenAI-compatible providers as multimodal `image_url` data URL
        parts; backend chat/httpserver tests pass.
  - [x] G11.2 Single-user Team removal: deleted Team settings UI/deep-link
        state/locales/composition test and made Knowledge collection creation
        Personal-only in the standalone frontend.
  - [x] G11.3 Browser provider settings parity: Go now accepts BYOK-encrypted
        browser provider runtime config for model listing and chat streaming;
        Provider Settings auto-enables fetched models for newly configured
        providers.
    - [x] G11.3a BYOK public JWK WebCrypto compatibility: backend now publishes
          `publicKeyJwk.alg=RSA-OAEP-256` while keeping the envelope-level
          `alg=RSA-OAEP-256+A256GCM`, and the frontend normalizes legacy cached
          public-key responses before `crypto.subtle.importKey`.
    - [x] G11.3b Browser provider preference persistence: server mode still
          blocks browser-local chat/knowledge data authority, but allows the
          core settings preference store to persist theme/language and BYOK
          provider shells so custom providers and fetched model lists survive
          refresh.
    - [x] G11.3c Server Default admin persistence: Provider Settings now edits
          the backend Server Default provider, persists name/type/base URL/model
          list and encrypted secret envelope to Postgres `provider_configs`,
          and makes runtime config, model listing, and chat streaming resolve
          the current server-owned provider config.
    - [x] G11.3d Multi-provider backend authority and model-list repair:
          restored Add/Delete in server mode, added backend list/upsert/delete
          lifecycle for custom providers, made chat/model fetch resolve stored
          providers by ID, stopped save responses from collapsing the fetched
          model catalog, and configured a stable local BYOK key outside Git.
    - [x] G11.3e Provider editor autosave: model selection, enable state, and
          provider type now serialize backend writes automatically; name/base
          URL flush on blur, and save responses no longer overwrite newer
          optimistic model selections.
  - [x] G11.4 Image-model chat dispatch: `gpt-image-*`, `dall-e-*`, and
        `imagen-*` chat selections now use the real image executor, persist the
        generated file as an assistant attachment, complete through the normal
        chat SSE contract, and survive message reload. The same-origin rewrite
        proxy timeout is five minutes for long generation streams.
  - [x] G11.5 Uniform single-user authority: development mode now ignores
        browser Bearer sessions and uses only the fixed Development Owner for
        Knowledge and every other protected capability; removed collection and
        query consent-management UI from Knowledge while keeping server-side
        governance enforcement internal, and automatically provisions MinerU
        parse plus Jina passage-embedding consent for each new collection.
  - [x] G11.6 Original Knowledge layout parity: replaced the migration-only
        split admin layout with the original search + collection-card grid,
        create/edit modal, detail transition, upload zone, and document-list
        presentation while retaining Go-backed persistence and processing.
  - [x] G11.7 Native document indexing repair: Go now routes PDF to MinerU and
        DOCX/PPTX/XLSX/TXT/Markdown/HTML/CSV to the sandboxed Native parser,
        backfills server-owned Native governance/consent, cleans up future
        unbound uploads, and live-proved DOCX Native parse plus real Jina 1024
        embedding publication to an active document; reprocess is limited to
        failed Versions so an active projection cannot collide with the same
        generation's unique materialization keys.
  - [x] G11.8 Multilingual Knowledge candidate recall: migration `025` adds
        bounded exact-phrase and overlapping-bigram signals beside the existing
        lexical lane, live-proving Chinese evidence queries return candidates
        while unrelated or unsupported questions remain strict refusals.
  - [ ] G11.9 Auto Knowledge and Web-augmented chat: replace the per-message
        strict evidence gate with conversation-persistent Auto augmentation per
        `docs/tracking/g11-knowledge-auto-rag-plan.md`.
    - [x] G11.9A Development Hydration and Auto semantics: development mode
          now injects a database-valid internal Session, server-owned answer
          governance is provisioned without invalidating search projections,
          selected Knowledge augments normal streaming with `[K]` citations,
          misses silently fall back to the model, and the strict refusal/status
          path plus frontend strict flags are removed.
    - [x] G11.9B Conversation-persistent multi-Knowledge binding and dedicated
          composer control: Postgres conversation metadata is the sole current
          binding authority, explicit empty selection prevents legacy
          reactivation, up to eight collections survive DTO/store refresh, and
          server mode exposes persistent removable Knowledge chips outside the
          attachment menu.
    - [ ] G11.9C Contextual rewrite, Jina query embedding, hybrid/RRF retrieval,
          and Jina rerank.
    - [ ] G11.9D Structure-aware Parent/Child reindex with generation cutover.
    - [ ] G11.9E Go Tavily/Firecrawl/Exa/Bocha/model-built-in Search parity,
          SearXNG removal, and legacy Next search deletion.
    - [ ] G11.9F Postgres-encrypted administrator provider configuration,
          Docker-Secret master key, and bounded real connection tests.
    - [ ] G11.9G Knowledge/Web/model fusion, `[K]`/`[W]` citations, diagnostics,
          and clean-copy/live closure.

## Phase 16 — Multi-Server or Kubernetes Migration

- [ ] Define target deployment platform and managed service boundaries.
- [ ] Add image tagging and migration-job strategy.
- [ ] Add ingress, probes, and secrets plan.
- [ ] Verify release and rollback in target environment.
