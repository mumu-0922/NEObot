# Lower Password Minimum and Update Owner Credentials

## Goal

Lower the Server Auth password minimum from 15 Unicode characters to 9 while
preserving the existing maximum-length, UTF-8, hashing, and session-revocation
boundaries. Then update the running local Owner account to the user-supplied
email and password and prove the new credentials can log in.

## Requirements

- Apply a single shared minimum of 9 Unicode characters to bootstrap, invite
  acceptance, password recovery, login verification, and every other Server
  Auth flow that uses the shared password validator.
- Preserve the 256-byte maximum, valid UTF-8 requirement, no trimming at the
  backend identity boundary, Argon2id parameters, login rate limiting, and
  existing generic authentication failure behavior.
- Update focused backend tests so 9 characters are accepted and 8 are denied,
  including Unicode-rune behavior.
- Update active authentication, deployment, rotation, architecture, and
  tracking documentation from the old 15-character contract to 9 characters.
- Rebuild and recreate only the local backend service needed to activate the
  source change; do not alter unrelated runtime data.
- Update the default Owner credential row to the requested canonical email and
  password in one transaction, increment `credential_revision`, and revoke all
  existing Owner sessions.
- Never echo or persist the plaintext password in Git, shell arguments,
  environment variables, temporary plaintext files, logs, or the final report.
- Verify the new credentials through the live login API and immediately log out
  the verification session.

## Acceptance Criteria

- [x] `validatePassword` accepts exactly 9 Unicode runes and rejects 8.
- [x] Focused auth tests, all backend tests, and `go vet ./...` pass.
- [x] Documentation consistently describes the Server Auth password range as
      9–256 characters/bytes.
- [x] The backend runtime is healthy on the rebuilt image.
- [x] The Owner email is updated without a duplicate-email conflict.
- [x] All pre-existing Owner sessions are revoked.
- [x] A live login with the requested credentials succeeds, and the validation
      session is logged out afterward.
- [x] No plaintext password, session token, or password hash appears in command
      output or committed files.

## Definition of Done

- Source, tests, and operational documentation are synchronized.
- Backend quality checks pass and the live service is healthy.
- Runtime credential mutation is backed up minimally, applied transactionally,
  and verified end to end.
- Source/task changes are committed with focused conventional commits.

## Technical Approach

Change the shared backend constant and public validation error in
`backend/internal/auth/password.go`, then update boundary cases in
`password_test.go`. Because every credential creation, recovery, and verification
path already funnels through this validator, no parallel policy mechanism is
introduced. Update all current contract references discovered by repository
search.

For the live credential change, use a short-lived in-package Go operator helper
that reads the password only from stdin, hashes it with the existing Auth
implementation, updates the Owner credential and revokes sessions in a single
PostgreSQL transaction, then removes the helper. Validate through the public API
without printing the request body or returned token.

## Decision (ADR-lite)

**Context**: The requested password is valid under a 9-character minimum but
invalid under the former 15-character policy.

**Decision**: Change the product-wide shared Server Auth policy to 9 characters
rather than bypassing validation for one database row.

**Consequences**: New and recovered credentials may be shorter than before, but
all existing hashing, maximum length, UTF-8 validation, rate limits, and session
revocation controls remain intact. Existing credentials remain compatible.

## Out of Scope

- Changing deployment-level `ACCESS_PASSWORD` behavior.
- Changing Argon2id cost parameters or session duration.
- Adding a general-purpose password-reset CLI in this task.
- Modifying chats, Skills, files, models, or other users' data.

## Technical Notes

- Product root: `mm-chat/`.
- Shared validator: `mm-chat/backend/internal/auth/password.go`.
- Policy tests: `mm-chat/backend/internal/auth/password_test.go`.
- Current live services are healthy before the change.
- Runtime state under `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, and
  `mm-chat/.env.single-server` must not be deleted or rewritten.
