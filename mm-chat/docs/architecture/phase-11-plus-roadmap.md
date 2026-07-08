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

- Add or complete a server-mode implementation of the frontend API client.
- Wire conversation CRUD, message CRUD, SSE streaming, and file upload/download.
- Keep `local` mode as default rollback until server mode is verified.
- Do not remove browser-local storage paths in this phase.

Outputs:

- Frontend `local|server` mode switch documentation.
- Server-mode adapter code and tests.
- Browser smoke path against `http://127.0.0.1:8080`.

Verification:

- Existing local mode still works.
- Server mode can create a conversation, send a user message, stream an
  assistant response, upload a file, and refresh without losing server data.

Rollback:

- Switch `NEXT_PUBLIC_API_MODE=local` and keep the Go backend running for
  further debugging.

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
