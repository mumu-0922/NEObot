# Server Auth Identity

## Scenario: Maintain Email/Password identity boundaries

### 1. Scope / Trigger

Use this contract when changing Server Auth email or password validation,
Argon2id hashing, Login, Invite Acceptance, Recovery, credential revisions, or
session revocation. Deployment-level `ACCESS_PASSWORD` is a separate frontend
gate and is not covered here.

### 2. Signatures

The shared backend identity boundary is:

```go
func CanonicalizeEmail(value string) (string, error)
func validatePassword(password string) error
func validatePasswordForVerification(password string) error
func hashPassword(ctx context.Context, password string) (string, error)
func verifyPassword(ctx context.Context, password, encodedHash string) (bool, error)
```

Public credential routes are:

```text
POST /v1/auth/login
POST /v1/auth/invites/accept
POST /v1/auth/recovery/request
POST /v1/auth/recovery/complete
POST /v1/auth/logout
POST /v1/me/password
DELETE /v1/me/sessions
```

Authenticated Password Change accepts exactly:

```json
{
  "currentPassword": "string, preserved byte-for-byte",
  "newPassword": "string, preserved byte-for-byte"
}
```

It returns `204 No Content`; both successful Password Change and
`DELETE /v1/me/sessions` invalidate the current browser Session as well as all
other Sessions, so the frontend must clear its in-memory/session-storage token
and return to Login.

The operator-only bootstrap command is
`mm-chat-admin bootstrap-identity --email <mailbox> --password-stdin`.

### 3. Contracts

- Email is `lower(trim(email))`, one mailbox, at most 254 bytes, with no
  provider-specific dot or plus folding.
- New passwords contain 8–256 ASCII visible characters (`U+0021` through
  `U+007E`). ASCII letters, numbers, and symbols are allowed; whitespace,
  controls, Unicode, trimming, and normalization are forbidden.
- Bootstrap, new-identity Invite Acceptance, Recovery Completion, and the new
  password in Authenticated Password Change use the shared strict validator.
- Login, existing-identity Invite Acceptance, and current-password verification
  use the bounded legacy validator so hashes issued under the previous UTF-8
  policy remain usable until the password is rotated. Legacy verification does
  not authorize creation of a new non-ASCII password.
- New hashes are Argon2id PHC with `v=19,m=65536,t=3,p=2`, a random 16-byte
  salt, a 32-byte digest, and bounded hashing concurrency.
- Recovery Completion atomically replaces the hash, increments
  `credential_revision`, consumes the token, revokes sibling recovery tokens,
  and revokes all user Sessions without issuing a replacement Session.
- Authenticated Password Change verifies the current password through the same
  Argon2id boundary, atomically replaces the hash behind an expected-revision
  fence, increments `credential_revision`, revokes active Recovery Tokens, and
  revokes all user Sessions without issuing a replacement Session.
- Unknown email, wrong password, missing credential, and disabled/deleted
  account remain indistinguishable at the Login boundary.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| New password has exactly 8 ASCII visible characters | Accept |
| New password has fewer than 8 characters, whitespace, control, or non-ASCII | `ErrInvalidIdentityInput` |
| New password exceeds 256 characters | `ErrInvalidIdentityInput` |
| Existing password matches a legacy valid UTF-8 hash with whitespace/Unicode | Permit verification; do not reissue it as a new password |
| Stored PHC has the wrong algorithm, version, parameters, or field sizes | Reject before Argon2 allocation |
| Login credential does not match | Generic `ErrInvalidCredential`; unknown email still performs dummy Argon2id work |
| Recovery token is invalid, expired, used, or raced | Generic `ErrInvalidCredential`; no partial credential/session mutation |
| Current password is wrong or the Credential revision races | `ErrInvalidCredential`; no partial credential/token/session mutation |
| Existing credential revision changes during Login session creation | Reject the raced Login session |

### 5. Good / Base / Bad Cases

- **Good**: change the shared strict new-password validator, update exact-boundary
  tests and public docs, then prove bootstrap/invite/recovery/change callers.
- **Base**: existing UTF-8 passwords continue to verify through the bounded
  legacy validator with no rehash or migration.
- **Bad**: bypass validation with a direct hash update, lower only the Recovery
  route, trim the password, or return a distinct unknown-email response.

### 6. Tests Required

- Unit-test exactly 8 ASCII characters, 7 characters, every rejected character
  class, 256 characters, 257 characters, and legacy verification compatibility
  in `internal/auth/password_test.go`.
- Prove malformed PHC inputs fail before resource-intensive hashing.
- Prove Invite Acceptance rejects an invalid password before token snapshot or
  repository mutation.
- Prove authenticated Password Change rejects a wrong current password before
  mutation, rejects a raced revision, and atomically revokes every Session and
  active Recovery Token after the Credential update.
- Run the deterministic Auth Playwright journey for Login refresh/expiry,
  Recovery Completion, Password Change whitespace rejection plus ASCII-symbol
  acceptance, and all-Session revocation; each successful mutation must visibly
  return the current browser to Login.
- Run `go test ./internal/auth`, `go test ./...`, and `go vet ./...`.
- For a live credential rotation, prove Recovery or the owning operator path
  increments the revision, revokes existing Sessions, permits one public Login,
  and removes the verification Session afterward without logging secrets.

### 7. Wrong vs Correct

#### Wrong

```go
// A second policy drifts from bootstrap, invite, and recovery behavior.
if len(request.Password) < 8 {
    return ErrInvalidIdentityInput
}
```

#### Correct

```go
if err := validatePassword(input.Password); err != nil {
    return ErrInvalidIdentityInput
}
```

Keep strict password creation and bounded legacy verification as separately
named shared validators. Do not copy either policy into route handlers.
