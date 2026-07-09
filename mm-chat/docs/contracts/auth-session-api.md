# Auth Session API Contract

## 1. Scope / Trigger

Phase 13 starts replacing fixed development-user ownership with request-scoped
identity. This contract covers the Phase 13.1 backend boundary: optional Bearer
session resolution and context-scoped ownership for chat, files, imports, and
run cancellation.

## 2. Signatures

HTTP request identity input:

```http
Authorization: Bearer <raw-session-token>
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

- Raw Bearer tokens are never stored or passed into repositories.
- HTTP middleware computes `sha256(raw-session-token)` as lowercase hex and
  resolves it through Postgres-backed sessions with optional Redis cache.
- Successful resolution attaches browser-safe `userId`, `displayName`, and
  `role` to `context.Context`.
- Repositories must read owner identity from context, not from request JSON,
  URL params, package manifests, or struct-level fixed-user fields.
- Missing Bearer credentials keep the development-user fallback until enforced
  auth mode is added.

## 4. Validation & Error Matrix

| Condition                      | Result                                            |
| ------------------------------ | ------------------------------------------------- |
| Missing `Authorization`        | Continue as development user in development mode. |
| Malformed `Authorization`      | `401 UNAUTHENTICATED`.                            |
| Unknown session hash           | `401 UNAUTHENTICATED`.                            |
| Expired session                | `401 UNAUTHENTICATED`.                            |
| Revoked session                | `401 UNAUTHENTICATED`.                            |
| Redis session cache miss/error | Fall back to Postgres resolver.                   |
| Postgres resolver error        | Fail closed with `401 UNAUTHENTICATED`.           |

## 5. Good/Base/Bad Cases

- Good: `Authorization: Bearer abc` resolves to user A; all chat, file, import,
  and run queries use user A's ID.
- Base: no `Authorization` during local development; existing dev-user smoke
  flows keep working.
- Bad: request body contains `userId`; handlers ignore it and repositories use
  context identity only.

## 6. Tests Required

- Unit: auth context round-trip and development fallback.
- Unit: middleware hashes raw Bearer tokens and attaches session user context.
- Unit: invalid/expired/revoked sessions return 401 and do not call next
  handler.
- Unit: file upload object key uses request user path.
- Integration: two-user isolation for chat/files/imports/runs after auth
  endpoints exist.

## 7. Wrong vs Correct

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
