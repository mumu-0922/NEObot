# Auth and Identity API Contract

## 1. Scope / Trigger

Phase 15.1B replaces the Phase 13 public Bootstrap Token login with independent
Email/Password identities. Go owns credential verification, Invite Acceptance,
Recovery, Bearer Sessions, abuse control, and mailbox delivery. Postgres is the
authorization authority; Redis is never sufficient to authorize a request.

Public registration, OAuth/MFA, Team administration, and frontend identity UI
remain outside this slice.

## 2. Signatures

```http
POST   /v1/auth/login
POST   /v1/auth/invites/accept
POST   /v1/auth/recovery/request
POST   /v1/auth/recovery/complete
POST   /v1/auth/logout
GET    /v1/me
DELETE /v1/me/sessions
```

Protected requests use:

```http
Authorization: Bearer <64-character raw session token>
```

Initial Owner provisioning is operator-only:

```bash
printf '%s\n' '<password>' | docker compose ... run --rm -T admin \
  bootstrap-identity --email owner@example.com --password-stdin
```

The command refuses to run after any Credential exists and revokes old
pre-Credential Bootstrap Sessions.

## 3. Contracts

- Email canonicalization is `lower(trim(email))`, at most 254 bytes, without
  provider-specific dot or plus folding.
- Passwords are 15–256 UTF-8 characters/bytes and are never trimmed or
  normalized.
- New Password Hashes use Argon2id PHC
  `v=19,m=65536,t=3,p=2`, a random 16-byte Salt, and a 32-byte Hash. Verification
  rejects malformed or resource-amplifying PHC values before Argon2 allocation.
- Session, Invite, and Recovery Tokens contain 32 CSPRNG bytes encoded as
  lowercase hex. Postgres stores only SHA-256 Hashes.
- Unknown Email, wrong Password, disabled/deleted Account, and absent
  Credential all return the same Login error. Unknown Email performs Dummy
  Argon2id verification.
- Invite Acceptance locks and consumes one Active Invite, creates a new User,
  Credential, Team Membership, Membership Revision, and Session in one
  transaction. Existing identities cannot have their Password overwritten.
- Recovery Request always returns the same `202` response. A known Active User
  gets a one-time 30-minute Token delivered only through the bounded server-side
  SMTP queue.
- Recovery Completion atomically consumes the Token, increments
  `credential_revision`, changes the Password, revokes sibling Recovery Tokens,
  and revokes every Session. It does not issue a new Session.
- Every Bearer resolution rechecks Postgres. A positive Redis snapshot cannot
  bypass Logout, Recovery, revoke-all, Account disablement, or deletion.
- Public Auth routes enforce an 8 KiB body limit plus independent hashed IP and
  account/Token limits. The bounded local limiter remains active if Redis is
  absent or fails.

## 4. Endpoint DTOs

### Login

```json
{ "email": "user@example.com", "password": "..." }
```

### Invite Acceptance

```json
{ "token": "64-character-token", "password": "..." }
```

Successful Login and Invite Acceptance return the raw Session Token once:

```json
{
  "user": { "id": "uuid", "displayName": "Owner", "role": "user" },
  "token": "raw-session-token-returned-once",
  "expiresAt": "2026-07-17T12:00:00Z"
}
```

Team `admin|member` Role never appears in this global User DTO.

### Recovery

```json
POST /v1/auth/recovery/request
{ "email": "user@example.com" }

202
{ "status": "accepted" }
```

```json
POST /v1/auth/recovery/complete
{ "token": "64-character-token", "newPassword": "..." }

204 No Content
```

`POST /v1/auth/logout` revokes the current Session. `DELETE /v1/me/sessions`
revokes every Session owned by the authenticated User. Both return `204`.

## 5. Validation & Error Matrix

| Condition                                         | Result                                |
| ------------------------------------------------- | ------------------------------------- |
| Malformed JSON, Email, or Password                | `400 INVALID_AUTH_PAYLOAD`            |
| Caller-supplied User/Role/Team/ACL/Fence field    | `400 FORBIDDEN_IDENTITY_FIELD`        |
| Auth body over 8 KiB                              | `413 PAYLOAD_TOO_LARGE`               |
| Failed Login or invalid Recovery Token            | `401 INVALID_CREDENTIALS`             |
| Missing/invalid/expired/revoked Bearer            | `401 UNAUTHENTICATED`                 |
| Unknown/expired/used/revoked Invite               | `410 INVITE_NOT_ACTIVE`               |
| IP or hashed subject limit exceeded               | `429 RATE_LIMITED` with `Retry-After` |
| Postgres unavailable                              | `503 DATABASE_REQUIRED`               |
| SMTP unavailable, queue full, or delivery failure | Same Recovery `202`; no disclosure    |

Only Login, Invite Acceptance, Recovery Request, and Recovery Completion are
anonymous. Health/readiness/version/metrics keep their existing operational
rules. `/v1/auth/register` does not exist.

## 6. Good / Base / Bad Cases

- Good: a mailbox Invite creates one Credential and Session, then ordinary
  Email/Password Login creates an independent Session.
- Good: Recovery changes the Password and all old Bearer Tokens immediately
  fail because Postgres is rechecked.
- Base: local `AUTH_MODE=development` still permits the synthetic Development
  User when no Bearer is supplied; explicit Bearers are always verified.
- Bad: `{ "token": "old-bootstrap-token" }` sent to Login is rejected.
- Bad: a Redis snapshot for a revoked Session cannot authorize a request.
- Bad: Team Admin cannot obtain a member's Recovery Token or reset their
  Credential.

## 7. Tests Required

- Unit: Email/Password policy, randomized Argon2id Salt, strict PHC parsing,
  bounded Argon2 concurrency, and Dummy verification.
- Unit: strict DTOs, 8 KiB limit, forbidden identity fields, bounded metrics,
  hashed rate-limit keys, local fallback, and trusted-loopback proxy handling.
- Integration: one-time/concurrent Invite and Recovery consumption,
  Credential Revision fencing, Session revoke-all, disabled Account rejection,
  and database-only Secret Hashes.
- Runtime: one-time bootstrap command, required-mode Login/Me/revoke-all smoke,
  no Password/Email/Token in logs, and temporary PostgreSQL cleanup.

## 8. Wrong vs Correct

### Wrong

```go
if cachedSession.Valid { // stale positive authorization
    allow()
}
```

### Correct

```go
session, err := postgres.LookupSessionByTokenHash(ctx, tokenHash)
if err != nil || session.RevokedAt != nil || !session.ExpiresAt.After(now) {
    deny()
}
```
