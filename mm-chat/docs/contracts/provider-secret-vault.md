# Provider Secret Vault Contract

## 1. Scope / Trigger

G11.9F.1 created the restart-stable at-rest encryption primitive. G11.9F.2.1
wires it into model-provider administrator writes, runtime reads, and
single-server Compose without changing schema or calling a provider. F2.2 adds
transactional backfill/rotation plus backup/restart proof; connection-test
activation remains F2.3.

## 2. Signatures

```go
func NewVault(KeyringConfig) (*Vault, error)
func LoadVaultFile(path string) (*Vault, error)
func (v *Vault) ActiveKID() string
func (v *Vault) Encrypt(plaintext []byte, context string) (Envelope, error)
func (v *Vault) Decrypt(envelope Envelope, context string) ([]byte, error)
func (v *Vault) NeedsRotation(envelope Envelope) bool
func (v *Vault) Rotate(envelope Envelope, context string) (Envelope, bool, error)
func WithProviderSecretVault(vault *providersecrets.Vault) ServiceOption
func (s *Service) UpsertAdminProviderConfig(context.Context, string, UpdateAdminProviderConfigRequest) (AdminProviderConfigResponse, error)
func (s *Service) ResolveStoredProvider(context.Context, string) (ResolvedProvider, error)
```

Runtime/deployment fields:

```text
PROVIDER_SECRET_KEYRING_SOURCE=<host mode-600 keyring path>
PROVIDER_SECRET_KEYRING_FILE=/run/secrets/mm_chat_provider_keyring
MM_CHAT_RUNTIME_UID=<id -u>
MM_CHAT_RUNTIME_GID=<id -g>
```

Keyring file:

```json
{"v":1,"activeKid":"provider-2026-07","keys":[{"kid":"provider-2026-07","key":"<base64url-32-bytes>"}]}
```

Persistable envelope:

```json
{"v":1,"kid":"provider-2026-07","alg":"A256GCM","nonce":"...","ciphertext":"...","context":"provider:search:tavily"}
```

## 3. Contracts

- the keyring file is the only input for at-rest key material and is intended
  to be mounted from Docker Secret storage;
- one active key encrypts; active or retained previous keys may decrypt;
- key bytes are never returned by the public API;
- `v`, `kid`, `alg`, and `context` form authenticated additional data;
- every encryption uses a fresh random GCM nonce;
- persisted envelopes never contain plaintext;
- rotation decrypts with a retained key and encrypts with the active key;
- errors remain stable and contain no path, key, nonce, ciphertext, plaintext,
  or underlying crypto detail.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing/empty path or unreadable file | `ErrKeyringUnavailable` |
| Non-regular, oversized, malformed, unknown-field, or trailing JSON file | `ErrInvalidKeyring` |
| Wrong version, duplicate/invalid/missing active key, non-32-byte key | `ErrInvalidKeyring` |
| Empty/oversized plaintext | `ErrInvalidPlaintext` |
| Empty/oversized/untrimmed/NUL context | `ErrInvalidContext` |
| Envelope context differs from expected context | `ErrContextMismatch` |
| Unknown key, wrong version/algorithm, bad encoding/nonce, tampering | `ErrInvalidEnvelope` |
| Admin writes a secret without a configured vault | `ErrProviderSecretVaultUnavailable` / HTTP `503 PROVIDER_SECRET_VAULT_UNAVAILABLE` |
| Stored vault/legacy envelope is corrupt, unknown, or copied | `ErrProviderSecretInvalid` / redacted `PROVIDER_SECRET_UNAVAILABLE` |
| Runtime UID/GID differs from the mode-`600` source owner | preflight rejection; startup otherwise fails closed |

## 5. Good / Base / Bad Cases

- Good: encrypt a model Key with its exact record context, store only the JSON
  envelope, restart with the same Docker Secret, and decrypt it.
- Good rotation: load a keyring with new active plus old retained key, rotate
  every old envelope, verify reload, then remove the old key.
- Base: an envelope already uses the active key; `Rotate` returns it unchanged
  with `changed=false`.
- Base migration: metadata-only save converts a legacy BYOK/default env secret
  to vault ciphertext; a new custom provider remains secretless.
- Bad: copy a Tavily envelope into a Jina record or alter its header/ciphertext;
  context/authentication validation fails closed.
- Bad deployment: mount a host UID-owned mode-`600` file into a different
  container UID; preflight rejects the mismatch instead of widening mode or
  running the API as root.

## 6. Tests Required

- encrypt/decrypt round-trip and plaintext absence in marshalled JSON;
- context mismatch, ciphertext tamper, and unknown-key rejection;
- strict bounded file loading and missing-file behavior;
- invalid version, active key, duplicate ID, ID syntax, and key length;
- retained-old-key rotation, active-key no-op, and old-key rejection of the new
  envelope;
- full Go tests, race test for this package, vet, module, quality, and security
  gates.
- model-provider ingress-to-vault, restart reload, legacy/env lazy import,
  custom-provider isolation, corrupt/missing-vault redaction, and context-copy
  rejection;
- guarded real-Postgres ciphertext-only integration against an isolated
  `mm_chat_*_test` database, followed by database deletion;
- Compose render, mode/owner preflight, image build, read-only Secret mount,
  non-root runtime UID, explicit restart, and health proof.

## 7. Wrong vs Correct

Wrong: write the browser RSA envelope or plaintext directly to Postgres and
depend on an ephemeral process key or `PROVIDER_API_KEY` fallback after restart.

Correct: decrypt browser ingress only inside Go, re-encrypt with the
Docker-Secret-backed vault under a record-specific context, persist only that
envelope, and activate the provider only after a bounded connection test.

## 8. Rollback / Next Gate

F.1 rollback deletes the unused package and contract; no state exists to
migrate. F2.1 mounts the stable keyring and writes vault envelopes while
retaining dual-read rollback. F2.2 adds exact-plan transactional bulk
backfill/rotation plus backup/restore and restart proof. F2.3 removes model
`.env` fallback only after a bounded connection-test activation gate.

The compatibility direction is intentionally one-way: F2.1 reads legacy BYOK
rows, but an older image cannot read newly written vault envelopes. Before the
first production administrator save, image rollback is state-free. Afterwards,
retain the F2.1 image and keyring and restore a pre-cutover Postgres backup
before starting the older image; never remove the active keyring first.

## 9. G11.9F.2.1 Repository Cutover

- `PROVIDER_SECRET_KEYRING_FILE` contains only the in-container read-only path;
  raw key material never enters the environment;
- Compose mounts the host `PROVIDER_SECRET_KEYRING_SOURCE` only into `backend`
  and `admin`;
- Compose runs those two consumers as the configured non-root
  `MM_CHAT_RUNTIME_UID:GID`, which preflight requires to match the mode-`600`
  keyring owner; this avoids both world-readable key material and a root API;
- administrator API Key bodies remain RSA BYOK ingress envelopes;
- Go decrypts ingress, immediately encrypts with the vault context
  `provider:model:<userId>:<providerId>`, clears the temporary byte slice, and
  persists only the vault JSON envelope;
- metadata save lazily imports a valid legacy BYOK row or Server Default env
  secret; a new custom provider never inherits the default secret;
- reads accept vault and legacy BYOK algorithms during this rollback slice,
  but corrupt/unknown envelopes and missing vaults fail with redacted stable
  errors;
- no schema migration is needed because `encrypted_secret_ref` already stores
  an opaque bounded string;
- connection-test activation and env removal remain F2.3.

## 10. G11.9F.2.2 Transactional Rewrite

Operator signature:

```text
admin provider-secrets-rewrite
admin provider-secrets-rewrite --execute \
  --expected-plan-sha256 <dry-run-sha256> \
  --confirmed-backup-sha256 <verified-dump-sha256>
```

Contract:

- dry-run is the default and returns only bounded counts plus a SHA-256 plan;
- the plan binds the active vault key ID plus a domain-separated keyed binding
  of its bytes, every row ID/User/Provider/config/ciphertext/deletion
  state/action, and a one-way digest of any Server Default env fallback used by
  that exact plan;
- execute requires both the exact dry-run digest and an operator-confirmed,
  checksum-verified pre-rewrite Postgres backup;
- one Serializable transaction takes a `SHARE ROW EXCLUSIVE` table lock,
  validates every active and deleted row before updating any row, then rewrites
  legacy BYOK and retained-old-key vault envelopes;
- an unreadable active `SERVER_DEFAULT` legacy row may use the still-configured
  `PROVIDER_API_KEY` migration fallback; an unreadable custom legacy row is
  counted as blocked and prevents execute;
- active duplicate User/Provider records, unknown/malformed/trailing envelope
  fields, context copy, missing retained keys, stale plan state, oversized
  state, or any SQL failure aborts the entire transaction;
- empty rows remain empty, deleted ciphertext rows rotate, and no schema or
  provider network request is involved;
- after execute, dry-run must report `changed_rows=0`, `blocked_rows=0`, and all
  ciphertext rows current before a retained key may be pruned.

Stable failures:

| Condition | Result |
| --- | --- |
| Missing DB/vault | `ErrProviderSecretRewriteUnavailable` |
| Malformed/ambiguous/oversized/context-invalid state | `ErrProviderSecretRewriteInvalid` |
| Dry-run state or active key changed before execute | `ErrProviderSecretRewritePlanMismatch` |
| Unrecoverable custom legacy ciphertext exists | `ErrProviderSecretRewriteBlocked` |

Required proof covers pure plan/action/hash tests, plan-mismatch and blocked
zero-write behavior, real Postgres legacy/old/current/empty/deleted rows,
active-key-only reload, keyring prepare/prune, owner-only dump/checksum,
restore drill, plaintext/keyring absence, backend restart, and a final no-op
audit.
