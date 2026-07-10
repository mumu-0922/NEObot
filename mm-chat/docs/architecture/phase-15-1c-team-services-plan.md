# Phase 15.1C Team Services Plan

## Objective and Boundary

Implement the authoritative Team, Membership, and Invitation control plane in
Go/Postgres. The existing Next.js/React UI remains unchanged; this slice adds
server contracts and backend routes only. Knowledge Collection CRUD, Team
deletion, Processor Consent, Python RAG, and frontend wiring remain outside
15.1C.

Phase 15.1C builds on migration `004` and Phase 15.1B identities. A global
`CurrentUser.role = "user"` is compatibility metadata and never grants Team
authority. Every Team permission is resolved from the current Postgres
`team_memberships` row.

## Locked Product Decisions

- Any authenticated user may create a Team and becomes its first active Admin.
- One user may be Admin in one Team and Member in another.
- Existing active accounts may accept invitations to additional Teams by
  proving the invited mailbox token and their current password. Their credential
  is never replaced. New accounts use the same DTO to set their first password.
- A removed Membership may be reactivated only through a new mailbox Invite.
- Members may list visible Team/member-safe metadata and leave voluntarily.
  Only active Admins may rename a Team, list/create/revoke Invites, change roles,
  or remove another Member.
- Team deletion is deferred to 15.1D because Collection ACL, Outbox, and
  tombstone behavior must be committed atomically with it.
- Invitation creation fails closed with `503 INVITE_DELIVERY_UNAVAILABLE` when
  SMTP, encryption, or the durable sender is unavailable. Invite and encrypted
  Mail Outbox work commit atomically; raw tokens are never returned to an Admin.
- The emailed acceptance URL carries the raw Invite Token only in a URL
  fragment (`#token=...`). Fragments are not sent in HTTP request targets,
  reverse-proxy/access logs, or server metrics. The future frontend reads and
  clears the fragment before posting `{ token, password }` in the request body;
  token-bearing paths and queries are forbidden.

## Public API Contract

All routes below require a Phase 15.1B Bearer Session except the existing
`POST /v1/auth/invites/accept` route.

```text
POST   /v1/teams
GET    /v1/teams?cursor&limit
GET    /v1/teams/{teamId}
PATCH  /v1/teams/{teamId}
GET    /v1/teams/{teamId}/members?cursor&limit
DELETE /v1/teams/{teamId}/membership
PATCH  /v1/teams/{teamId}/members/{userId}
DELETE /v1/teams/{teamId}/members/{userId}
POST   /v1/teams/{teamId}/invites
GET    /v1/teams/{teamId}/invites?cursor&limit
DELETE /v1/teams/{teamId}/invites/{inviteId}
```

Write DTOs use `teamRole`, never bare `role`; `teamId`, target `userId`, actor,
ACL, and revision fields are path/context-derived and forbidden in request
bodies. Team names are trimmed, valid UTF-8, free of control/format characters,
1-100 runes, and at most 256 bytes. Team Roles are exactly `admin|member`.

`POST /v1/teams` and `POST .../invites` require a body
`idempotencyKey` of 1-128 bytes, following the existing chat/import convention.
The key is scoped by creator/Team and backed by Postgres uniqueness. Duplicate
keys return `409 IDEMPOTENCY_CONFLICT`; no retry can generate a second Invite
Token or email.

Lists use an opaque, versioned, HMAC-SHA-256-authenticated cursor, `limit=50` by
default, and a range of `1..100`. Encoded cursors are rejected above 1024 bytes
before decoding. Canonical signed content binds `keyId`, `version`, endpoint/
`resourceKind`, request User ID, optional Team ID, normalized filter digest, and
the sort tuple; verification uses constant-time comparison. Cursors cannot be
replayed across users, Teams, filters, or list endpoints. An active signing key
plus verify-only key ring permits bounded rotation. Lists never return a total
count; every page rechecks current Membership. Team and Invite order is
`(created_at DESC, id DESC)`; Member order is
`(created_at ASC, user_id ASC)`. A cursor is pagination state, not an
authorization credential.

### Safe Response Shapes

```ts
type TeamRole = "admin" | "member";

interface TeamDto {
  id: string;
  name: string;
  membershipRevision: number;
  myMembership: {
    teamRole: TeamRole;
    status: "active";
    joinedAt: string;
    updatedAt: string;
  };
  createdAt: string;
  updatedAt: string;
}

interface TeamMemberDto {
  userId: string;
  displayName: string;
  teamRole: TeamRole;
  status: "active";
  joinedAt: string;
  updatedAt: string;
}

interface TeamInviteDto {
  id: string;
  teamId: string;
  maskedEmail: string;
  teamRole: TeamRole;
  status: "pending" | "accepted" | "revoked" | "expired";
  deliveryStatus: "pending" | "processing" | "sent" | "failed" | "cancelled";
  expiresAt: string;
  createdAt: string;
  updatedAt: string;
}

interface ApiPage<T> {
  items: T[];
  nextCursor?: string;
}
```

Member responses omit email. Invite responses expose only a masked mailbox and
never contain raw/token-hash fields. Invite Acceptance remains single-use and
never echoes the Invite Token.

## Error and Disclosure Matrix

Authorization order is fixed: validate session and bounded input; resolve Team
visibility; distinguish visible Member from Admin; only then resolve nested
Member/Invite IDs.

| HTTP | Code                          | Contract                                                                 |
| ---- | ----------------------------- | ------------------------------------------------------------------------ |
| 400  | `INVALID_TEAM_PAYLOAD`        | Invalid name, JSON, body fields, UUID, cursor, or limit.                 |
| 400  | `INVALID_INVITE_PAYLOAD`      | Invalid mailbox, Team Role, or idempotency key.                          |
| 400  | `INVALID_MEMBERSHIP_PAYLOAD`  | Invalid Team Role or body shape.                                         |
| 400  | `FORBIDDEN_IDENTITY_FIELD`    | Caller supplies actor, target identity, ACL, or revision hints.          |
| 401  | `UNAUTHENTICATED`             | Bearer Session is absent or invalid.                                     |
| 403  | `TEAM_ADMIN_REQUIRED`         | Active Member invokes an Admin-only operation.                           |
| 404  | `TEAM_NOT_FOUND`              | Team is absent/deleted or caller has no active Membership.               |
| 404  | `TEAM_MEMBER_NOT_FOUND`       | Visible Admin targets an absent/removed/cross-Team Member.               |
| 404  | `INVITE_NOT_FOUND`            | Visible Admin targets an absent/cross-Team Invite.                       |
| 409  | `LAST_TEAM_ADMIN`             | Mutation would leave no usable active Admin.                             |
| 409  | `INVITE_CONFLICT`             | Active Membership or pending Invite already exists for the Team/mailbox. |
| 409  | `IDEMPOTENCY_CONFLICT`        | Scoped idempotency key already exists.                                   |
| 410  | `INVITE_NOT_ACTIVE`           | Invite is accepted, expired, or otherwise unusable.                      |
| 503  | `INVITE_DELIVERY_UNAVAILABLE` | SMTP, encryption, or durable sender admission is unavailable.            |

Unknown and cross-Team resources use the same `404` envelope. Only an active
Member may receive `403` for a visible Team. Repeated deletion of an already
revoked Invite is `204`; accepted or expired Invites return `410`.

## Schema and Migration 005

Never edit committed migration `004`. Add reversible migration `005` to:

- add nullable, non-blank `idempotency_key` columns to `teams` and
  `team_invites` for backward-compatible existing rows;
- add unique indexes scoped to `(created_by_user_id, idempotency_key)` and
  `(team_id, invited_by_user_id, idempotency_key)` when keys are non-null;
- add one pending-Invite invariant on `(team_id, email)`; expired pending rows
  are revoked under the Team lock before issuing a replacement;
- change `team_memberships.user_id` deletion from `CASCADE` to `RESTRICT`, so a
  future User deletion cannot bypass Membership Revision and last-Admin checks;
- add `identity_mail_outbox`, containing Invite identity, key ID, AES-256-GCM
  nonce/ciphertext, durable delivery status, lease/retry timestamps, bounded
  attempt count, stable Message-ID, and no plaintext token/password/session
  material. One row exists per Invite and stores the mailbox, raw token, and
  rendered acceptance URL only inside the authenticated ciphertext.

Migration `005` Up drops `team_memberships_user_id_fkey` and recreates that
same named constraint with `ON DELETE RESTRICT` in the migration transaction.
Down first drops Mail Outbox and all `005` indexes/columns, then replaces the
same FK with the original `ON DELETE CASCADE`. Verification must run
`004 -> 005 -> 005 Down -> 005 Up` with existing Team/User/Membership rows and
must prove no interval is committed without the FK.

`004` already supplies Team, Membership, Invite, and `knowledge_outbox` fields.
No new Team-role or revision table is needed.

## Transaction and Lock Contract

Membership-effective writers follow one global cross-aggregate lock order:

```text
optional non-locking identity/visibility preflight
-> target User row fence(s) FOR UPDATE in UUID order, when the User exists
-> teams row(s) FOR UPDATE in UUID order
-> recheck actor active Membership/Role
-> target Membership
-> target Invite
-> target Credential when required
-> membership_revision
-> knowledge_outbox
-> identity_mail_outbox when required
-> COMMIT
```

This order applies to Team create, Membership role/remove/self-leave, existing
User Invite Acceptance/reactivation, and account disable. New-account Invite
Acceptance has no User row to lock: it serializes first identity creation with
a transaction-scoped canonical-email fence, then locks Team and Invite; a
concurrent unique-email loser fails closed rather than switching branches.
Team rename and Invite issue/revoke do not change effective Membership, so they
start at Team and never acquire a User row. No writer may lock a Team and then a
User, or lock a Membership/Invite before its Team.

Password Recovery/Rekey must also acquire its User row before its Credential;
the existing joint `FOR UPDATE OF user, credential` query is split into that
deterministic order. It never acquires a Team lock. This keeps recovery,
existing-user Invite Acceptance, and account disable on the same User fence.

Invite Acceptance must therefore be refactored from the current joint
planner-dependent Invite/Team lock: resolve its immutable Team and identity
snapshot without a lock; acquire the existing User row or new-email fence;
lock Team; lock/revalidate Invite and Mail Outbox; then lock/revalidate the
Credential if present. Public acceptance has no actor Membership. The mail
worker locks only Mail Outbox rows and never acquires Team, Membership, Invite,
User, or Credential locks, preventing a reverse edge.

Creating a Team inserts Team + creator active Admin + initial Outbox event in
one transaction and keeps `membership_revision=1`. A real Membership add,
reactivation, role change, removal, or self-leave increments the revision
exactly once and inserts a `team.membership.changed` Outbox event in the same
transaction. No-op, rejected, rolled-back, Invite create/revoke, Team rename,
or natural Invite expiry does not increment it.

Before demoting/removing/leaving an active Admin, the transaction checks for
another active Admin Membership whose User account is active and not deleted.
The Team row acts as the per-Team mutex, so concurrent last-Admin mutations
cannot both commit. Phase 15.1C also adds the supported account-disable
transaction: first lock the target User row, then enumerate and lock all active
Membership Teams in UUID order, re-read the Membership set, and reject
disablement that would remove the last usable Admin. While the User fence is
held, every supported Membership activation/reactivation path must wait before
it can lock a Team, eliminating the enumerate-then-insert race. The transaction
then updates the account, revokes Sessions, advances every affected Team
revision, and writes one Outbox event per Team atomically. Direct account-status
or Membership SQL updates are forbidden application paths.

Outbox payloads include only Team ID, target User ID, operation, Team Role,
status, and committed Membership Revision. They exclude email, raw tokens,
token hashes, passwords, and Session material.

## Invitation and Existing-Account Acceptance

- Invite creation canonicalizes email through the single auth implementation,
  generates a 32-byte lowercase-hex token, stores only its SHA-256 hash, and
  uses a 72-hour TTL.
- No configured SMTP sender, active encryption key, decrypt key ring, or
  durable worker admission means no Invite row is created.
- The Service encrypts the mailbox, raw token, acceptance URL, and template
  data with AES-256-GCM before persistence. A fresh 96-bit nonce is required;
  AAD binds schema version, Outbox ID, Invite ID, and Team ID. The Invite hash
  and encrypted Mail Outbox row commit in the same Team transaction. Plaintext
  exists only in bounded process memory and is never logged or returned.
- Required/hosted mode accepts only an HTTPS acceptance base URL. Development
  may use HTTP only on a loopback host. Startup rejects committed example key
  material and incomplete cursor/Mail-key/acceptance-URL/SMTP combinations.
- Configuration supplies a dedicated active key ID and a key ring of base64
  32-byte keys. Only the active key encrypts; retained old keys are decrypt-only
  until every row using them is terminal and past retention. Missing, duplicate,
  or malformed key material fails startup/admission closed.
- The worker claims due `pending`/retryable rows with a lease, bounded batch,
  `FOR UPDATE SKIP LOCKED`, capped exponential backoff with jitter, and a fixed
  maximum attempt count. Expired `processing` leases are reclaimed after a
  crash. Success stores `sent`; exhaustion stores terminal `failed`; errors are
  sanitized and never persist relay credentials or decrypted content.
- Invite delivery never enters the existing in-memory Recovery queue. The
  durable worker is the sole Invite sender and calls only an extracted bounded
  SMTP transport. Immediately before relay I/O it locks and rechecks the Outbox
  row, then holds that row lock through the bounded SMTP call and terminal/retry
  state update. Revocation therefore either cancels first or waits for that send
  attempt; the worker never needs an Invite/Team lock.
- Delivery is at-least-once: use one stable RFC Message-ID per Outbox row and a
  provider idempotency key when supported. A crash after SMTP acceptance but
  before `sent` may cause a duplicate email, but never a second Invite Token.
- Revocation marks every not-yet-sent Mail Outbox row `cancelled` in the same
  Team transaction. If a bounded SMTP attempt already owns the Outbox row,
  revocation waits, then revokes the Invite after the attempt is durably
  `sent`/retryable; any delivered token is unusable after that commit. Expired
  Invites are likewise cancelled before a worker sends them.
- Acceptance locks and requires both an active Invite and its Mail Outbox row
  in `sent`; `pending`, `processing`, `failed`, or `cancelled` delivery uses the
  uniform `INVITE_NOT_ACTIVE` response.
- Acceptance first reads a non-locking identity/credential snapshot. For a new
  canonical email, the Service derives an Argon2id hash and the transaction
  creates User/Credential/Membership and Session atomically. If another flow
  creates that identity first, this branch loses closed and does not overwrite
  or reinterpret the credential.
- For an existing active credential, the Service verifies the submitted current
  password outside the write transaction and passes the observed credential
  User ID, Credential identity, and revision as a fence. The transaction first
  locks and rechecks the same active User, then Team, Invite, sent Mail Outbox,
  and the same Credential identity/revision before inserting/reactivating only
  the Membership and Session. Password hash and credential revision are never
  changed. Concurrent password recovery/rekey wins its User/Credential lock or
  invalidates the fence; acceptance loses closed with the uniform public error.
- Disabled/deleted identities, wrong passwords, revoked/expired tokens, active
  duplicate Memberships, unsent mail, stale credential revisions, branch races,
  and concurrent losers use the uniform `INVITE_NOT_ACTIVE` public result.

## Module and Wiring

Add `internal/teams` as a vertical slice with domain types/errors, Service,
Postgres Repository, HTTP Handler, UUID/cursor helpers, mail cryptography/worker,
and focused tests.
Handler owns routing/strict JSON/DTO mapping; Service owns validation, generated
IDs/tokens, credential verification, encryption, delivery admission, and
context requirements; Repository owns visibility, authorization, locks,
revision fences, durable leases, and Outbox atomicity.

Wire one Postgres Repository, the generalized identity SMTP sender, and one
durable Mail Outbox worker into `cmd/api` and `httpserver`. Register `/v1/teams`
and `/v1/teams/` as protected routes. Refactor account disable through the Team
fencing service instead of direct User mutation. Add bounded dynamic Team route
labels to metrics; raw UUIDs, emails, tokens, ciphertext, and relay errors must
never become labels or log fields.

Require a dedicated Team cursor active key ID plus signing/verify-only key ring
of at least 32 random bytes per key, and the Mail Outbox active key ID/decrypt
key ring in configuration. Cursor and encryption keys must be distinct from
each other and from Session, database/Redis passwords, SMTP, object-store, and
provider secrets. Startup validation reports only key IDs/config field names,
never key bytes.

## Verification Matrix

- Unit: validation, cursor bounds/context/HMAC/tamper rejection, idempotency,
  masking, raw/hash token split, AES-GCM round trip/AAD/key rotation, disabled
  delivery, lease/backoff/reclaim, and existing-account password/revision fence.
- HTTP: complete method/path matrix, 8 KiB bodies, unknown/forbidden fields,
  DTO safety, `404`/`403` ordering, `409`/`410`/`503`, protected routing, bounded
  metric labels.
- PostgreSQL 16: Team+Admin atomic create, visible lists, Admin/Member/outsider
  matrix, idempotency uniqueness, pending Invite uniqueness, hash-only tokens,
  Invite revoke/accept races, existing-account second-Team join, removed-user
  rejoin, account-disable fencing, encrypted Mail Outbox atomicity, acceptance
  only after `sent`, revision/Outbox equality, and rollback on Outbox failure.
- Concurrency: two Admins concurrently demote/remove/leave; exactly one may
  remove the final pair. Concurrent same-role patch advances at most once.
  Concurrent Invite acceptance has one winner and one revision increment.
  Account disable cannot race a role change into zero usable Admins. Invite
  revoke/worker/accept races never permit use of a revoked or unsent token.
  Existing-user acceptance cannot insert a Membership between account-disable
  enumeration and commit; recovery cannot pass a stale Credential fence.
- Promotion: `gofmt`, `go vet`, race tests, `go test ./...`, `govulncheck`,
  Compose config/build, fresh `001->005` migration drill,
  `004->005->Down->005` replay, cursor/key configuration checks, secret scan,
  and independent xhigh review with `P0/P1/P2 = 0`.

## Execution Checklist

- [x] **15.1C-1 Contract and migration:** finalize Team DTO/error/pagination/
      idempotency contracts; add cursor/Mail Outbox key configuration and
      reversible migration `005`.
- [x] **15.1C-2 Team repository:** implement Team create/list/get/rename,
      member/invite reads, Team-first locking, revision, last-Admin, and Outbox.
- [x] **15.1C-3 Invitation lifecycle:** implement hash-only issue/revoke,
      encrypted durable Mail Outbox/SMTP worker, existing-account acceptance,
      credential revision fencing, and removed-Membership reactivation.
- [x] **15.1C-4 HTTP and wiring:** add strict Team routes, protected server/API
      wiring, account-disable fencing, stable authenticated cursors, safe
      DTOs/errors, and bounded metric labels.
- [x] **15.1C-5 Verification:** pass unit/HTTP/race and real PostgreSQL 16
      idempotency, isolation, lock-order, last-Admin, Outbox, and delivery gates.
- [x] **15.1C-6 Promotion:** synchronize contracts/deployment/tracking, pass
      quality/security gates, independent xhigh review, and commit by explicit
      allowlist.

## Rollback

Before frontend Team wiring, rollback uses the previous API image and migration
`005` Down only on an isolated/pre-release database. Production rollback is
forward-fix: the nullable idempotency columns and stricter User FK are additive
authority safeguards and must not be destructively removed after live Team
mutations. Team/member/invite data created during drills uses isolated fixtures.
