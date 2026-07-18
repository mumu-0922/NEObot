# Provider Secret Vault Contract

## 1. Scope / Trigger

G11.9F.1 creates the restart-stable at-rest encryption primitive required
before existing administrator provider settings can move from BYOK ingress
envelopes and `.env` fallback into one Postgres authority. This slice has no
runtime, database, Compose, API, UI, or provider-network effect.

## 2. Signatures

```go
func NewVault(KeyringConfig) (*Vault, error)
func LoadVaultFile(path string) (*Vault, error)
func (v *Vault) ActiveKID() string
func (v *Vault) Encrypt(plaintext []byte, context string) (Envelope, error)
func (v *Vault) Decrypt(envelope Envelope, context string) ([]byte, error)
func (v *Vault) NeedsRotation(envelope Envelope) bool
func (v *Vault) Rotate(envelope Envelope, context string) (Envelope, bool, error)
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

## 5. Good / Base / Bad Cases

- Good: encrypt a model Key with its exact record context, store only the JSON
  envelope, restart with the same Docker Secret, and decrypt it.
- Good rotation: load a keyring with new active plus old retained key, rotate
  every old envelope, verify reload, then remove the old key.
- Base: an envelope already uses the active key; `Rotate` returns it unchanged
  with `changed=false`.
- Bad: copy a Tavily envelope into a Jina record or alter its header/ciphertext;
  context/authentication validation fails closed.

## 6. Tests Required

- encrypt/decrypt round-trip and plaintext absence in marshalled JSON;
- context mismatch, ciphertext tamper, and unknown-key rejection;
- strict bounded file loading and missing-file behavior;
- invalid version, active key, duplicate ID, ID syntax, and key length;
- retained-old-key rotation, active-key no-op, and old-key rejection of the new
  envelope;
- full Go tests, race test for this package, vet, module, quality, and security
  gates.

## 7. Wrong vs Correct

Wrong: write the browser RSA envelope or plaintext directly to Postgres and
depend on an ephemeral process key or `PROVIDER_API_KEY` fallback after restart.

Correct: decrypt browser ingress only inside Go, re-encrypt with the
Docker-Secret-backed vault under a record-specific context, persist only that
envelope, and activate the provider only after a bounded connection test.

## 8. Rollback / Next Gate

F.1 rollback deletes the unused package and contract; no state exists to
migrate. G11.9F.2 must add a migration-compatible repository envelope, stable
keyring configuration, transactional rotation/import behavior, and restart
proof before any current provider row or `.env` fallback changes.
