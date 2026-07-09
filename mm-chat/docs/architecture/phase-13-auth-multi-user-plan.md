# Phase 13 Auth and Multi-User Hardening Plan

## Objective

Replace fixed development-user behavior with request-scoped identity and prepare
server mode for hosted multi-user deployment. The first slice keeps development
fallback for local testing, then later slices enforce login-required behavior.

## Scope

- Resolve a user identity once per request and attach it to `context.Context`.
- Scope conversations, messages, files, import batches, and run cancellation by
  that request identity.
- Add `/v1/me`, login, and logout endpoints after the backend data path no
  longer depends on package-level fixed-user constants.
- Keep provider secrets and object-storage keys server-side only.
- Verify two users cannot read, mutate, import, roll back, download, or cancel
  each other's data.

## Non-Goals

- No third-party OAuth in this phase.
- No frontend UI redesign; only minimal API-client/session wiring when needed.
- No RAG identity model until Phase 15.
- No production-only force-delete for modified import batches.

## Implementation Slices

### 13.1 Request Identity Plumbing

- Add shared auth context helpers with a development-user fallback.
- Add optional Bearer session middleware backed by the existing Postgres session
  resolver and Redis session cache.
- Change Go repositories and file object keys to read `userId` from request
  context instead of hard-coded development-user fields.
- Keep missing Bearer tokens as development-user fallback for local mode only.

Verification: unit tests for context helpers, middleware success/failure, file
object-key scoping, and backend `go test ./...`.

### 13.2 Auth Endpoints

- Define `POST /v1/auth/login`, `POST /v1/auth/logout`, and `GET /v1/me`.
- Choose the first credential model: password-based local accounts or a
  configured single-owner bootstrap token.
- Store only password/session hashes; never return raw tokens after creation.
- Revoke sessions durably in Postgres and clear Redis cache entries.

Verification: login returns a Bearer token, `/v1/me` returns the current user,
logout invalidates the token, and expired/revoked sessions return 401.

### 13.3 Enforced Hosted Mode

- Add an explicit auth mode config such as `AUTH_MODE=development|required`.
- In required mode, reject missing credentials before chat/files/import routes.
- Keep `/health`, `/ready`, `/v1/version`, and login routes public.
- Preserve development fallback only when not in hosted/required mode.

Verification: unauthenticated hosted requests fail closed; development mode
keeps existing local smoke behavior.

### 13.4 Two-User Isolation

- Create two sessions for two users in integration tests.
- Verify isolation across chat CRUD, SSE run cancellation, file metadata/content,
  browser import commit/status/rollback, and idempotency keys.
- Verify object keys include the owning user path.

Verification: cross-user reads/mutations return not-found or unauthorized errors
without leaking resource existence.

## Rollback

- Set auth mode back to development fallback and restart the Go API.
- Existing rows remain owned by their original user IDs; do not rewrite user IDs
  during rollback.
- If session middleware misbehaves, remove the resolver option from server
  wiring while preserving repository context fallback.

## Progress Tracking

`mm-chat/docs/tracking/progress.md` remains the checklist. Mark a Phase 13 slice
only after implementation, tests, and dated process evidence are complete.
