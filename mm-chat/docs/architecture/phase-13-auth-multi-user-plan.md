# Phase 13 Auth and Multi-User Hardening Plan

> **Historical status:** Phase 15.1B supersedes the Phase 13 auth baseline. This
> document retains the Phase 13 execution plan and historical decisions; use
> [`phase-15-1b-identity-services-plan.md`](./phase-15-1b-identity-services-plan.md)
> and [`../contracts/auth-session-api.md`](../contracts/auth-session-api.md) for
> the current identity and session contract.

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

## Phase 15 Team Knowledge Extension

Phase 13 is the implemented request-identity baseline; it does not provide Team
RBAC. Phase 15 extends it for the confirmed small-team product model:

- every user has an independent account/session and a private Personal
  Knowledge scope;
- a Team has `admin|member` membership and Shared Team Knowledge;
- Team Admin manages membership, Team documents, and Team processing consent,
  but cannot read another user's Personal Knowledge;
- Team Member queries Team Knowledge by default and cannot upload/delete Team
  documents or change processing consent;
- login defaults to Admin invitation with public registration disabled;
- invite acceptance establishes an Argon2id password credential; ordinary
  member login uses verified email/password after Phase 15 cutover;
- password recovery uses a single-use token delivered to the verified mailbox,
  revokes all sessions, and cannot be received or consumed by Team Admin;
  anonymous recovery request remains uniformly answered and rate-limited, but
  gives the requester no token or account-existence signal;
- the Bootstrap Token is limited to initial operator provisioning/break-glass,
  not ordinary member login;
- the last Active Admin cannot be removed or downgraded;
- Team Membership Revision becomes part of the RAG authorization fingerprint.

Team Role is a membership attribute, not a global `users.role`, and Chat
Workspace remains a UI grouping rather than an authorization scope. The future
API and persistence boundary is defined in
[`../contracts/knowledge-acl-api.md`](../contracts/knowledge-acl-api.md); this
section does not claim the Phase 15 schema is already implemented.

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
