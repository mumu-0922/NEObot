# Phase 15.1B Identity Services Plan

## Objective and Boundary

Replace the public Bootstrap Token login with independent Email/Password
accounts while preserving the existing Bearer Session contract. This slice is
Go/Postgres only: no registration, OAuth/MFA, Team administration, Knowledge
CRUD, RAG, or frontend redesign.

Postgres remains authoritative on every Bearer request. Redis may cache a
Session snapshot or revocation hint, but a positive Redis hit cannot authorize
a request until a future monotonic auth epoch exists.

## Locked Contracts

- Argon2id PHC: `v=19,m=65536,t=3,p=2`, 16-byte random salt, 32-byte hash.
- Password: 15–256 UTF-8 characters/bytes; no trim or provider-specific
  normalization. Argon2 work uses bounded concurrency.
- Email: `lower(trim(email))`, one mailbox, at most 254 bytes; no dot or plus
  folding.
- Session/Invite/Recovery tokens: 32 CSPRNG bytes encoded as lowercase hex;
  only SHA-256 hashes persist.
- Session TTL remains 7 days; Invite TTL is 72 hours; Recovery TTL is 30
  minutes.
- Existing credentials cannot be overwritten by Invite Acceptance.
- Recovery Request always returns `202 {"status":"accepted"}`. Completion
  returns `204`, changes the password, increments the credential revision, and
  revokes every Session without issuing a new one.

## Execution Checklist

- [x] **15.1B-1 Crypto and authority:** add canonical Email and bounded
      Argon2id helpers; make Session resolution recheck active Postgres state;
      retain Redis only as non-authoritative acceleration.
- [x] **15.1B-2 Repository transactions:** add revision-fenced Email/Password
      Login, atomic Invite Acceptance, Recovery issue/consume, revoke-all, and
      one-time bootstrap-identity provisioning.
- [x] **15.1B-3 HTTP surface:** implement Email/Password Login, Invite Accept,
      Recovery Request/Complete, and `DELETE /v1/me/sessions`; enforce strict 8 KiB
      bodies, uniform errors, and no caller-supplied identity fields.
- [x] **15.1B-4 Delivery and abuse control:** add a server-only bounded SMTP
      Recovery delivery queue and independent IP plus hashed account/token rate
      limits with an in-memory fail-closed fallback.
- [x] **15.1B-5 Promotion gate:** pass unit, HTTP, Redis/cache, and real
      PostgreSQL tests for one-time tokens, concurrent consumption, credential
      revision fencing, disabled users, revocation, non-enumeration, and secret
      non-disclosure.

## Bootstrap and Operations

The old Bootstrap Token is not accepted by `/v1/auth/login`. A local operator
command reads the first Owner password from standard input, refuses to run once
any credential exists, and creates the initial Email/Password identity after
migration `004`. Raw passwords and one-time tokens never enter command
arguments, environment values, logs, metrics, URLs, API responses, or Git.

SMTP configuration is server-only. When delivery is unavailable or fails,
Recovery Request keeps the same `202` response and never exposes whether an
account exists. The SMTP queue is bounded; overload drops delivery rather than
creating unbounded goroutines.

## Verification and Rollback

Verification requires `go test ./...`, a fresh PostgreSQL 16 integration drill,
security/quality gates, and independent xhigh review. Before frontend identity
wiring, rollback must restore the previous image **and** its matching Phase 13
Compose/environment contract in the same deployment change, including the old
`AUTH_BOOTSTRAP_TOKEN` secret and service wiring. Do not switch only the image:
the current Compose/env contract intentionally omits `AUTH_BOOTSTRAP_TOKEN`,
while the old image requires that legacy login configuration. No destructive
Down migration is required because this slice reuses migration `004`;
credential rows created during testing use isolated fixtures only.
