# Auth and Identity API Contract

## 1. Scope / Trigger

Phase 15.1B replaces the Phase 13 public Bootstrap Token login with independent
Email/Password identities. Go owns credential verification, Invite Acceptance,
Recovery, Bearer Sessions, abuse control, and mailbox delivery. Postgres is the
authorization authority; Redis is never sufficient to authorize a request.

Migration `004` already laid down the identity/team schema foundation and the
current repository already serves Login, Recovery, and a new-account-only
`POST /v1/auth/invites/accept` path. Phase 15.1C keeps this document
authoritative for Login/Recovery/Session semantics while widening that same
public Invite Acceptance route for Team services, existing-account acceptance,
durable encrypted Invite delivery, and the Phase 15.1B Bearer Session reused by
every protected `/v1/teams*` route.

Public registration, OAuth/MFA, and frontend identity UI remain outside this
slice. Team administration route shapes live in
[`knowledge-acl-api.md`](./knowledge-acl-api.md).

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
- `POST /v1/auth/invites/accept` is the only anonymous Team-related route. It
  accepts the same `{ token, password }` DTO for both new-account bootstrap and
  existing-account Team join.
- Invite email links carry the Token only in the client-side URL fragment
  `#token=...`, never in a path or query. The frontend must read and immediately
  clear that fragment, then send the Token only in this JSON request body.
- Invite Acceptance requires one Active Invite plus its durable encrypted
  identity Mail Outbox row in `sent`. `pending`, `processing`, `failed`,
  `cancelled`, `accepted`, `revoked`, and expired delivery all fail closed with
  the same public `410 INVITE_NOT_ACTIVE` result.
- For a new canonical email, Invite Acceptance creates the User, Credential,
  active Team Membership, Membership Revision increment, and Session atomically.
- For an existing active Credential, Invite Acceptance verifies the submitted
  current Password before the write transaction, then re-locks the same active
  User and Credential revision fence before inserting/reactivating only the
  Membership and Session. It never overwrites the Credential or increments
  `credential_revision`.
- Recovery Request always returns the same `202` response. A known Active User
  gets a one-time 30-minute Token delivered only through the bounded server-side
  SMTP queue. Invite delivery uses a separate durable encrypted Mail Outbox and
  never the in-memory Recovery queue.
- Recovery Completion atomically consumes the Token, increments
  `credential_revision`, changes the Password, revokes sibling Recovery Tokens,
  and revokes every Session. It does not issue a new Session.
- Password Recovery/Rekey and Team Invite Acceptance share the same
  User-before-Credential fence order. Account disable first locks the target
  User, then affected Teams, and must reject any mutation that would strand the
  last usable Team admin with `409 LAST_TEAM_ADMIN` before revoking Sessions.
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

The same DTO covers both Invite branches:

- new account: `password` becomes the first Credential for the invited mailbox;
- existing active account: `password` must match the current Credential for that
  same canonical mailbox, and the Credential is reused rather than replaced.

The body may not include `userId`, `teamId`, `role`, `teamRole`, ACL fields, or
revision hints. `400 FORBIDDEN_IDENTITY_FIELD` for that complete set is the
15.1C strict-DTO behavior. Other unrecognized members fail closed as
`400 INVALID_AUTH_PAYLOAD`; neither class reaches the repository.

Successful Login and Invite Acceptance return the raw Session Token once:

```json
{
  "user": { "id": "uuid", "displayName": "Owner", "role": "user" },
  "token": "raw-session-token-returned-once",
  "expiresAt": "2026-07-17T12:00:00Z"
}
```

The global User `role` remains the compatibility metadata value `"user"`
and never grants Team authority. Team `admin|member` authority is resolved from
Postgres membership state and, on Team APIs, is exposed as `teamRole`.

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

| Condition                                                                                                                        | Result                                |
| -------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| Malformed JSON, Email, Password, or Token shape                                                                                  | `400 INVALID_AUTH_PAYLOAD`            |
| Caller-supplied User/Team/Role/teamRole/ACL/Fence field                                                                          | `400 FORBIDDEN_IDENTITY_FIELD`        |
| Auth body over 8 KiB                                                                                                             | `413 PAYLOAD_TOO_LARGE`               |
| Failed Login or invalid Recovery Token                                                                                           | `401 INVALID_CREDENTIALS`             |
| Missing/invalid/expired/revoked Bearer                                                                                           | `401 UNAUTHENTICATED`                 |
| Audited account disable would remove the last usable Team admin                                                                  | `409 LAST_TEAM_ADMIN`                 |
| Unknown/expired/used/revoked/unsent Invite, wrong current Password, stale Credential fence, or disabled/deleted invited identity | `410 INVITE_NOT_ACTIVE`               |
| IP or hashed subject limit exceeded                                                                                              | `429 RATE_LIMITED` with `Retry-After` |
| Postgres unavailable                                                                                                             | `503 DATABASE_REQUIRED`               |
| SMTP unavailable, queue full, or delivery failure                                                                                | Same Recovery `202`; no disclosure    |

Only Login, Invite Acceptance, Recovery Request, and Recovery Completion are
anonymous. Health/readiness/version/metrics keep their existing operational
rules. `/v1/auth/register` does not exist.

## 6. Good / Base / Bad Cases

- Good: a mailbox Invite for a new account creates one Credential and Session,
  then ordinary Email/Password Login creates an independent Session.
- Good: an existing active account can accept an Invite to an additional Team by
  proving the invited mailbox Token and current Password; the Credential hash and
  `credential_revision` stay unchanged while Membership and Session are added.
- Good: Recovery changes the Password and all old Bearer Tokens immediately
  fail because Postgres is rechecked.
- Base: local `AUTH_MODE=development` still permits the synthetic Development
  User on legacy non-Team routes when no Bearer is supplied; `/v1/teams*`
  always requires and verifies a Bearer Session in both modes.
- Bad: `{ "token": "old-bootstrap-token" }` sent to Login is rejected.
- Bad: a Redis snapshot for a revoked Session cannot authorize a request.
- Bad: Team Admin cannot obtain a member's raw Invite Token or Recovery Token,
  reset their Credential, or disable the last usable Team admin.

## 7. Tests Required

- Unit: Email/Password policy, randomized Argon2id Salt, strict PHC parsing,
  bounded Argon2 concurrency, and Dummy verification.
- Unit: strict DTOs, 8 KiB limit, forbidden identity fields (`role`,
  `teamRole`, ACL, and revision hints), bounded metrics, hashed rate-limit keys,
  local fallback, and trusted-loopback proxy handling.
- Integration: one-time/concurrent Invite and Recovery consumption,
  new-account Invite bootstrap, existing-account second-Team join,
  Credential Revision fencing, sent-mail prerequisite, uniform
  `INVITE_NOT_ACTIVE` disclosure, Session revoke-all, disabled Account
  rejection, last-admin account-disable fencing, and database-only Secret
  Hashes.
- Runtime: one-time bootstrap command, required-mode Login/Me/revoke-all smoke,
  protected `/v1/teams*` Bearer reuse, no Password/Email/Token in logs, no
  plaintext Invite payload persisted outside encrypted Mail Outbox ciphertext,
  and temporary PostgreSQL cleanup.

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
