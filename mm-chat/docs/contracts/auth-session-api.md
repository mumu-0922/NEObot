# Auth Session API Contract

## 1. Scope / Trigger

Phase 13 replaces fixed development-user ownership with request-scoped identity
and adds the first server auth endpoints. The current credential model is a
single configured bootstrap owner token; later phases may add password or
external identity providers without changing repository ownership rules.

## 2. Signatures

HTTP identity input:

```http
Authorization: Bearer <raw-session-token>
```

Auth endpoints:

```http
POST /v1/auth/login
POST /v1/auth/logout
GET  /v1/me
```

Backend resolver signature:

```go
ResolveByTokenHash(ctx context.Context, tokenHash string) (auth.Session, error)
```

Context ownership helpers:

```go
auth.WithUser(ctx, auth.User{ID, DisplayName, Role}) context.Context
auth.UserOrDevelopment(ctx) auth.User
```

## 3. Contracts

- `AUTH_BOOTSTRAP_TOKEN` is accepted only by `POST /v1/auth/login` and is never
  returned in an API response.
- Login returns a newly generated bearer session token once; Postgres stores only
  `sha256(raw-session-token)` in `sessions.token_hash`.
- HTTP middleware computes `sha256(raw-session-token)` as lowercase hex and
  resolves it through Postgres-backed sessions with optional Redis cache.
- Successful resolution attaches browser-safe `userId`, `displayName`, and
  `role` to `context.Context`.
- Repositories must read owner identity from context, not from request JSON, URL
  params, package manifests, or struct-level fixed-user fields.
- Missing Bearer credentials keep the development-user fallback until enforced
  auth mode is added.

## 4. Endpoint DTOs

### `POST /v1/auth/login`

Request:

```json
{ "token": "bootstrap-owner-token" }
```

Response:

```json
{
  "user": {
    "id": "00000000-0000-0000-0000-000000000001",
    "displayName": "Owner",
    "role": "user"
  },
  "token": "raw-session-token-returned-once",
  "expiresAt": "2026-07-16T12:00:00Z"
}
```

### `POST /v1/auth/logout`

Requires `Authorization: Bearer <raw-session-token>`. Revokes the canonical
Postgres session and deletes/marks Redis session-cache entries when Redis is
configured. Success returns `204 No Content`.

### `GET /v1/me`

Returns the request user from context. In development fallback mode with no
Bearer header, this returns the development user until Phase 13.3 makes hosted
mode fail closed.

## 5. Validation & Error Matrix

| Condition                      | Result                                            |
| ------------------------------ | ------------------------------------------------- |
| Missing `AUTH_BOOTSTRAP_TOKEN` | `503 AUTH_NOT_CONFIGURED` on login.               |
| Wrong login token              | `401 INVALID_CREDENTIALS`.                        |
| Missing `Authorization`        | Continue as development user in development mode. |
| Malformed `Authorization`      | `401 UNAUTHENTICATED`.                            |
| Unknown session hash           | `401 UNAUTHENTICATED`.                            |
| Expired session                | `401 UNAUTHENTICATED`.                            |
| Revoked session                | `401 UNAUTHENTICATED`.                            |
| Redis session cache miss/error | Fall back to Postgres resolver.                   |
| Postgres resolver error        | Fail closed with `401 UNAUTHENTICATED`.           |

## 6. Good/Base/Bad Cases

- Good: login with the bootstrap token creates a new session row, returns a raw
  bearer token once, and `/v1/me` resolves that token to the owner user.
- Base: no `Authorization` during local development; existing dev-user smoke
  flows keep working.
- Bad: request body contains `userId`; handlers ignore it and repositories use
  context identity only.

## 7. Tests Required

- Unit: auth context round-trip and development fallback.
- Unit: login creates a session with a hashed token and returns the raw token
  once.
- Unit: logout revokes the session and clears Redis cache hints.
- Unit: middleware hashes raw Bearer tokens and attaches session user context.
- Unit: invalid/expired/revoked sessions return 401 and do not call next
  handler.
- Unit: file upload object key uses request user path.
- Integration: two-user isolation for chat/files/imports/runs after enforced
  auth mode exists.

## 8. Wrong vs Correct

### Wrong

```go
repo.userID = chat.DevUserID
query(..., repo.userID)
```

### Correct

```go
user := auth.UserOrDevelopment(ctx)
query(..., user.ID)
```
