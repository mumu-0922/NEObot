# Phase 11+ Roadmap

This roadmap records work discovered after the original Phase 0-10 plan. Any new
plan, scope change, or reordered phase must be written here or in a linked doc
before implementation starts, then mirrored in `docs/tracking/progress.md`.

## Planning Rule

- Do not rely on chat memory for future work.
- Every new phase must define objective, scope, outputs, verification, rollback,
  and tracking checklist before code changes start.
- Every completed checklist item must have a dated entry in
  `docs/tracking/process.md`.
- Keep work isolated under `mm-chat/` until the owner explicitly approves edits
  to the existing app.

## Current Baseline

The Go backend stack is already running locally through Docker Compose:
Postgres, Redis, private MinIO, file upload/download, browser import, and real
OpenAI-compatible streaming are verified. The existing Next.js frontend has not
yet been wired to server mode.

## Phase 11 — Frontend Server-Mode Integration

Objective: connect the existing Next.js/React app to the Go backend without
breaking local mode.

Scope:

- Add or complete a server-mode implementation of the frontend API client in
  small reversible slices.
- Wire conversation CRUD, message CRUD, SSE streaming, and file upload/download
  after the adapter scaffold is in place.
- Preserve the existing Next.js/React frontend technology stack and UI. Phase
  11 integration must be adapter/service-layer work first, not a component or
  design-system rewrite.
- Keep `local` mode as default rollback until server mode is verified.
- Do not remove browser-local storage paths in this phase.
- Defer browser import/export UI, real auth/session UI, RAG/knowledge flows, and
  provider-settings redesign unless a later phase explicitly reopens them.

Outputs:

- Frontend `local|server` mode switch documentation.
- Server-mode adapter code and tests.
- Browser smoke path against `http://127.0.0.1:8080`.
- Dated process entries and progress checklist updates for each completed
  Phase 11 slice.

Verification:

- Existing local mode still works.
- Server mode can create a conversation, send a user message, stream an
  assistant response, upload a file, and refresh without losing server data.
- Each slice has targeted tests or browser smoke evidence before its progress
  checkbox is marked complete.

Rollback:

- Switch `NEXT_PUBLIC_API_MODE=local` and keep the Go backend running for
  further debugging.
- Revert the current slice only; do not revert completed backend phases unless
  live evidence shows the backend contract is wrong.

Execution rule:

- Complete the slices in order: 11.1, 11.2, 11.3, 11.4, then 11.5.
- Do not start application code for a slice until its objective, scope, outputs,
  verification, rollback, and tracking checklist are documented.
- Do not mark any Phase 11 progress checkbox complete without a matching dated
  entry in `docs/tracking/process.md`.
- When original app files under `src/` are required, prefer additive adapter
  files and one narrow service boundary change over UI/store rewrites.

### 11.1 — Adapter Scaffold

Objective: introduce the server-mode frontend adapter shell behind the existing
API boundary while preserving the current local adapter behavior.

Scope:

- Locate the existing frontend API boundary, mode selection path, and local-mode
  callers that must remain stable.
- Add or complete only the server adapter scaffold, shared request helpers,
  runtime flag resolution, and compile-safe method placeholders needed by later
  slices.
- Decide and document the browser network edge for server mode: same-origin
  proxy/reverse proxy, or direct backend base URL after CORS allowlisting is
  added and verified.
- Keep `NEXT_PUBLIC_API_MODE=local` as the default behavior.
- First-slice boundary: do not touch browser import/export UI, auth/session UI
  or enforcement, RAG/knowledge flows, provider-settings redesign, or unrelated
  product UI.
- Owner-approved original-app edits are limited to minimal service/API-client
  integration files. Do not change visible UI, component composition, styling,
  routes, or frontend framework choices in this slice.

Outputs:

- Adapter scaffold for `local|server` selection.
- Mode/base-URL configuration notes for local development.
- CORS/proxy decision notes for browser smoke testing.
- Targeted scaffold tests or type coverage when implementation starts.

Verification:

- Local mode still resolves to the existing browser-local path.
- Server mode resolves the configured API base URL without hidden feature
  activation beyond the scaffold.
- Browser network path is proven by either same-origin proxy behavior or CORS
  headers for the chosen frontend origin.
- Lint/typecheck and targeted adapter tests pass after code work.

Rollback:

- Set `NEXT_PUBLIC_API_MODE=local`.
- Revert only the adapter scaffold if mode selection destabilizes local mode.

Tracking checklist:

- Use the Phase 11.1 checklist in `docs/tracking/progress.md`.

### 11.2 — Conversation and Message CRUD

Objective: wire server-mode conversation and message operations to the Go
backend through the adapter, without streaming or file transfer yet.

Scope:

- Map frontend conversation/message DTOs to the current Go chat CRUD contract.
- Wire supported conversation create/list behavior to the Go backend; for
  missing read/update/delete endpoints, use server-data derivation where safe
  or return explicit unsupported instead of implicit browser-local fallback.
- Wire supported message create/list behavior to the Go backend; for missing
  read/update/delete endpoints, use server-data derivation where safe or return
  explicit unsupported instead of implicit browser-local fallback.
- Preserve local-mode storage and rollback semantics.
- Do not add import UI, auth UI/enforcement, RAG, provider settings redesign,
  SSE streaming, or file transfer in this slice.

Outputs:

- Server-mode conversation/message adapter methods.
- Error-envelope mapping for backend validation, not-found, conflict, and
  database-required responses.
- Targeted tests for adapter mapping and local-mode fallback.

Verification:

- Server mode can create/list conversations and create/list messages against the
  local Go backend.
- Refreshing the browser reloads server-owned chat state.
- Local mode still creates and reads browser-local conversations/messages.

Rollback:

- Switch `NEXT_PUBLIC_API_MODE=local`.
- Revert only the CRUD adapter slice if server DTO mapping is wrong.

Tracking checklist:

- Use the Phase 11.2 checklist in `docs/tracking/progress.md`.

#### 11.2B Sub-slices

The service/store integration is split because the existing local frontend has a
synchronous `createSession(): string` action while the server creates
conversations asynchronously.

- `11.2B-1` — add a CRUD mapper/gateway above the API client. It maps
  `ConversationDTO`/`ChatMessageDTO` into legacy-compatible records and remains
  unused by UI/store runtime.
- `11.2B-2` — integrate only the server-mode read path for experiments:
  `listConversations` for history metadata and `listMessages` for selected
  conversation messages. Local mode remains the existing IndexedDB path.
- `11.2B-3` — decide and implement the write path only after resolving the
  async session-create mismatch. Do not claim full server-backed send/create
  while `ChatApp` still creates local sessions and local assistant placeholders
  first.

Guardrails:

- Do not branch on `/v1/chat/*` or `NEXT_PUBLIC_API_MODE` in components.
- Do not change visible UI, state shape, message tree algorithms, or local
  provider streaming in 11.2B.
- Server mode must not silently fall back to browser-local persistence for
  unsupported CRUD features.

### 11.3 — SSE Stream

Objective: connect server-mode assistant streaming to the Go `/stream` SSE
contract after the user message is already persisted.

Scope:

- Send the persisted `userMessageId`, `modelRef`, and `idempotencyKey` required
  by the Go stream contract.
- Consume `message.started`, `message.delta`, `usage.updated`,
  `message.completed`, `message.error`, and `message.cancelled` frames.
- Map stream errors to the existing UI state without creating duplicate user
  messages.
- Preserve local-mode streaming behavior.
- Do not add file transfer, import UI, auth/RAG, or provider settings redesign
  in this slice.

Outputs:

- Server-mode stream adapter and event mapping.
- Targeted tests or smoke harness for streamed assistant messages.
- Cancellation/error handling notes if frontend behavior differs from local
  mode.

Verification:

- Server mode streams an assistant response from the local Go backend and
  persists the assistant row.
- Stream cancellation or provider error reaches a terminal UI state.
- Local-mode streaming remains unchanged.

Rollback:

- Switch `NEXT_PUBLIC_API_MODE=local`.
- Revert only the stream adapter slice if SSE event handling regresses.

Tracking checklist:

- Use the Phase 11.3 checklist in `docs/tracking/progress.md`.

#### 11.3C — Terminal Server Generation State

Objective: make the hidden server stream facade expose explicit non-persisted
lifecycle state before any visible UI wiring.

Scope:

- Add server-only generation state under `serverReadState`, including the
  current server session id, persisted user message id, assistant message id,
  active backend `runId`, terminal status, and terminal error details.
- Capture `message.started.runId` while streaming and clear it when the stream
  completes, fails, or is cancelled.
- Keep the state non-persisted and separate from legacy local chat fields.
- Do not add visible cancel controls, change `ChatApp`, or alter the local
  provider stream path in this slice.

Outputs:

- Store-level terminal state for completed, failed, unsupported, and cancelled
  server streams.
- Targeted tests for run-id propagation, terminal status mapping, error
  preservation, and persisted-state exclusion.
- Progress entries proving the hidden lifecycle boundary.

Verification:

- Successful server streams end in a completed terminal state and no active
  backend run id.
- Provider errors and unsupported stream results end in failed terminal state
  with the server error envelope preserved.
- Cancelled streams end in cancelled terminal state and no active backend run
  id.
- Local mode and legacy persisted chat metadata remain unchanged.

Rollback:

- Switch `NEXT_PUBLIC_API_MODE=local`.
- Revert only the 11.3C store/test slice if lifecycle mapping regresses.

#### 11.3D — Live Backend SSE Smoke

Objective: verify the already-built Go backend can stream and persist an
assistant response through the real local Compose stack before any visible UI
wiring starts.

Scope:

- Use the existing single-server Compose stack and backend on
  `http://127.0.0.1:8080`.
- Exercise health, readiness, version, conversation create, user-message append,
  assistant SSE stream, and post-stream message listing.
- Use configured provider environment from `.env.single-server` but do not
  record secrets in docs or logs.
- Run targeted local-mode regression tests after the server smoke.
- Do not change application code or visible frontend behavior in this slice.

Outputs:

- Dated process entry with command shape, redacted environment boundary, smoke
  IDs/statuses, artifact path, and cleanup/reset notes.
- Progress checklist entries proving server SSE persistence and local rollback
  behavior.

Verification:

- `message.completed` is observed in the SSE stream.
- Listing messages after the stream returns the persisted user row and assistant
  row.
- Legacy local `chatService`/store tests pass.

Rollback:

- Keep `NEXT_PUBLIC_API_MODE=local` for visible frontend use.
- Stop or reset only the local Compose stack if smoke data needs cleanup.

### 11.4 — File Upload and Download

Objective: wire server-mode file upload/download and message attachment
references to the Go file API.

Scope:

- Upload browser-selected files through `POST /v1/files`.
- Download file content through the backend gateway instead of exposing object
  keys, buckets, MinIO URLs, or local paths.
- Attach server file references to newly created messages where the existing UI
  already supports attachments.
- Preserve OPFS/local file behavior in local mode.
- Do not add browser import UI, RAG indexing, auth UI/enforcement, or unrelated
  file-management redesign.

Outputs:

- Server-mode file adapter methods.
- Attachment reference mapping between frontend state and server DTOs.
- Targeted upload/download/attachment tests or browser smoke evidence.

Verification:

- Server mode uploads a file, downloads the same bytes through the API, attaches
  the file to a message, and reloads attachment metadata after refresh.
- No private object-store path or secret appears in browser responses.
- Local-mode OPFS behavior remains unchanged.

Rollback:

- Switch `NEXT_PUBLIC_API_MODE=local`.
- Revert only the file adapter slice if upload/download mapping regresses.

Tracking checklist:

- Use the Phase 11.4 checklist in `docs/tracking/progress.md`.

#### 11.4A — Server File API Client Adapter

Objective: add the server-mode frontend API-client methods for the Go file API
without wiring visible UI, OPFS replacement, or message attachment flows yet.

Scope:

- Extend the API-client contract with `files` methods for upload, metadata,
  content download, and delete.
- Implement server-mode calls for:
  - `POST /v1/files` as `multipart/form-data`;
  - `GET /v1/files/{id}`;
  - `GET /v1/files/{id}/content`;
  - `DELETE /v1/files/{id}`.
- Keep local mode as explicit unsupported/fail-closed for this adapter.
- Enable only the `files` API capability in configured server mode.
- Do not change visible components, OPFS utilities, local attachment storage,
  message input behavior, or `ChatApp` in this slice.

Outputs:

- Server file adapter methods and DTO/input types.
- Shared HTTP helper support for multipart JSON responses and binary downloads.
- Targeted tests for request shape, URL encoding, binary download, error
  normalization, local unsupported behavior, and private object-store path
  boundaries.

Verification:

- Server-mode upload sends browser `FormData` without manually setting a
  multipart `Content-Type`.
- Download content is fetched through `/v1/files/{id}/content`.
- Responses do not expose object keys, bucket names, local paths, or MinIO/S3
  URLs; returned `downloadUrl` values must exactly match
  `/v1/files/{id}/content` for a UUID file ID.
- Local mode remains disabled for file API calls.

Rollback:

- Switch `NEXT_PUBLIC_API_MODE=local`.
- Revert only the 11.4A API-client/test slice if file adapter behavior
  regresses.

### 11.5 — Browser Smoke and Local Rollback

Objective: prove the server-mode path end-to-end in a browser and prove the
local-mode rollback path still works.

Scope:

- Run the local Docker backend stack and the Next.js app in server mode.
- Smoke conversation creation, user message persistence, SSE assistant stream,
  file upload/download, attachment rendering, refresh persistence, and rollback
  to local mode.
- Record exact commands, env flags, URLs, and cleanup/reset notes.
- Do not expand scope into import UI, auth, RAG, production hosting, or
  Kubernetes/multi-server concerns.

Outputs:

- Browser smoke evidence for server mode against `http://127.0.0.1:8080`.
- Local-mode rollback evidence using `NEXT_PUBLIC_API_MODE=local`.
- Known gaps and next-phase candidates documented before Phase 12 starts.

Verification:

- Server-mode browser smoke passes against the Compose backend.
- Switching back to local mode restores browser-local behavior without data-loss
  claims beyond the documented test data boundary.
- `docs/tracking/progress.md` and `docs/tracking/process.md` are synced before
  any Phase 11 item is marked complete.

Rollback:

- Stop server-mode frontend, restart with `NEXT_PUBLIC_API_MODE=local`, and keep
  backend data for debugging unless cleanup is explicitly requested.

Tracking checklist:

- Use the Phase 11.5 checklist in `docs/tracking/progress.md`.

## Phase 12 — Browser Data Export/Import UI

Objective: expose the Phase 8 import backend through an explicit user-controlled
browser migration flow.

Scope:

- Add browser export package generation from IndexedDB/localforage and OPFS.
- Show import preview before commit.
- Commit only after user confirmation.
- Provide rollback/delete imported batch UI where safe.

Verification:

- A local conversation with attachments exports, previews, imports, and renders
  from server state after refresh.

Rollback:

- Imported batches remain identifiable and can be rolled back if unmodified.

## Phase 13 — Auth and Multi-User Hardening

Objective: replace fixed development-user behavior with real accounts and
session handling.

Scope:

- Implement login/logout/me endpoints or chosen auth provider integration.
- Enforce user ownership on chat, files, imports, and future RAG calls.
- Keep provider secrets server-side only.
- Add session-cache invalidation behavior around Redis.

Verification:

- Two users cannot read or mutate each other's conversations, files, imports, or
  stream runs.

Rollback:

- Disable hosted/multi-user mode and return to development-user mode only in
  non-production environments.

## Phase 14 — Production Hardening and Observability

Objective: make the single-server deployment operable beyond local smoke tests.

Scope:

- Add structured logs, request IDs, metrics, and backup/restore drills.
- Add reverse proxy reference config and TLS notes.
- Add deployment runbook for release, rollback, and secret rotation.
- Add rate-limit and upload abuse checks for hosted mode.

Verification:

- Restore drill succeeds from Postgres and MinIO backups.
- Operator can trace one chat stream from request to persisted assistant row.

Rollback:

- Revert to previous release image and verified backup pair.

## Phase 15 — Optional Python RAG Sidecar

Objective: add document parsing, embeddings, retrieval, and citations only after
core server chat and frontend server mode are stable.

Scope:

- Define internal Go-to-RAG API.
- Add Python FastAPI service skeleton.
- Add indexing and retrieval flow for server-owned files.
- Ensure RAG failure never breaks normal chat.

Verification:

- Index one uploaded document and answer a grounded question with citations.

Rollback:

- Disable RAG endpoints and keep chat/files/import available.

## Phase 16 — Multi-Server or Kubernetes Migration

Objective: move beyond one-server Compose only when single-server operations are
stable and traffic requires it.

Scope:

- Externalize Postgres/Redis/Object storage as managed or separately operated
  services.
- Add container image tags, health probes, secrets, ingress, and migration jobs.
- Decide Helm/Kustomize/GitOps only after deployment targets are known.

Verification:

- One release can deploy, migrate, stream, upload, and roll back in the target
  cluster or multi-server environment.

Rollback:

- Keep the single-server Compose deployment as the fallback path until the new
  platform survives restore and rollback drills.
