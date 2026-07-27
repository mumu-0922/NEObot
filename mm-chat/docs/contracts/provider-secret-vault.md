# Provider Secret Vault Contract

## 1. Scope / Trigger

G11.9F.1 created the restart-stable at-rest encryption primitive. G11.9F.2.1
wires it into model-provider administrator writes, runtime reads, and
single-server Compose without changing schema or calling a provider. F2.2 adds
transactional backfill/rotation plus backup/restart proof. F2.3 makes a bounded
real connection test mandatory for activation and removes the model-provider
environment fallback.

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
func (s *Service) TestAdminProviderConnection(context.Context, string) (AdminProviderConnectionResponse, error)
func (s *Service) ActivateAdminProvider(context.Context, string) (AdminProviderConnectionResponse, error)
func ProviderConnectionTestValid(StoredProviderConfig) bool
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
{
  "v": 1,
  "activeKid": "provider-2026-07",
  "keys": [{ "kid": "provider-2026-07", "key": "<base64url-32-bytes>" }]
}
```

Persistable envelope:

```json
{
  "v": 1,
  "kid": "provider-2026-07",
  "alg": "A256GCM",
  "nonce": "...",
  "ciphertext": "...",
  "context": "provider:search:tavily"
}
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

| Condition                                                               | Result                                                                             |
| ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Missing/empty path or unreadable file                                   | `ErrKeyringUnavailable`                                                            |
| Non-regular, oversized, malformed, unknown-field, or trailing JSON file | `ErrInvalidKeyring`                                                                |
| Wrong version, duplicate/invalid/missing active key, non-32-byte key    | `ErrInvalidKeyring`                                                                |
| Empty/oversized plaintext                                               | `ErrInvalidPlaintext`                                                              |
| Empty/oversized/untrimmed/NUL context                                   | `ErrInvalidContext`                                                                |
| Envelope context differs from expected context                          | `ErrContextMismatch`                                                               |
| Unknown key, wrong version/algorithm, bad encoding/nonce, tampering     | `ErrInvalidEnvelope`                                                               |
| Admin writes a secret without a configured vault                        | `ErrProviderSecretVaultUnavailable` / HTTP `503 PROVIDER_SECRET_VAULT_UNAVAILABLE` |
| Stored vault/legacy envelope is corrupt, unknown, or copied             | `ErrProviderSecretInvalid` / redacted `PROVIDER_SECRET_UNAVAILABLE`                |
| Runtime UID/GID differs from the mode-`600` source owner                | preflight rejection; startup otherwise fails closed                                |
| Provider is disabled                                                    | `ErrProviderDisabled` / HTTP `409 PROVIDER_DISABLED`                               |
| Connection proof is missing or no longer matches configuration          | `ErrProviderActivationRequired` / HTTP `409 PROVIDER_ACTIVATION_REQUIRED`          |
| Bounded upstream connection test fails                                  | `ErrProviderConnectionTestFailed` / HTTP `502 PROVIDER_CONNECTION_TEST_FAILED`     |
| Provider changes while its test is in flight                            | `ErrProviderConfigChanged` / HTTP `409 PROVIDER_CONFIG_CHANGED`                    |

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
- Good activation: test the exact Provider ID/type/normalized base URL/vault
  envelope, atomically persist its fingerprint and timestamp, then enable it.
- Bad activation: alter type, base URL, or API Key after testing; the proof is
  cleared, the provider is disabled, and every runtime resolver fails closed
  until another successful activation.

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
- connection-test request/response bounds, redirect rejection, OpenAI/Gemini
  authentication shapes, fingerprint invalidation, concurrent-change fencing,
  disabled/untested fail-closed resolution, and restart persistence;
- formal model-list, chat-stream, and image-generation smokes after removing
  the model-provider environment variables from both Compose and the runtime
  environment.

## 7. Wrong vs Correct

Wrong: write the browser RSA envelope or plaintext directly to Postgres, trust
an unchecked `enabled` flag, or depend on an ephemeral process key or provider
API Key environment fallback after restart.

Correct: decrypt browser ingress only inside Go, re-encrypt with the
Docker-Secret-backed vault under a record-specific context, persist only that
envelope, and activate the exact stored configuration only after a bounded
connection test.

## 8. Rollback / Next Gate

F.1 rollback deletes the unused package and contract; no state exists to
migrate. F2.1 mounts the stable keyring and writes vault envelopes while
retaining dual-read rollback. F2.2 adds exact-plan transactional bulk
backfill/rotation plus backup/restore and restart proof. F2.3 has removed model
`.env` fallback after the bounded connection-test activation gate passed.

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
- connection-test activation and environment removal are completed by F2.3.

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
- at the F2.2 cutover only, an unreadable active `SERVER_DEFAULT` legacy row
  could use the then-configured `PROVIDER_API_KEY` migration fallback; an
  unreadable custom legacy row was counted as blocked and prevented execute;
- active duplicate User/Provider records, unknown/malformed/trailing envelope
  fields, context copy, missing retained keys, stale plan state, oversized
  state, or any SQL failure aborts the entire transaction;
- empty rows remain empty, deleted ciphertext rows rotate, and no schema or
  provider network request is involved;
- connection attestations bind the exact encrypted secret reference. When a
  row has a valid pre-rotation model, Search, or RAG attestation, the same
  locked transaction recomputes that fingerprint over the newly encrypted
  reference while preserving its tested timestamp and enabled state. An
  absent, malformed, or stale attestation remains invalid and is never
  promoted by rotation;
- after execute, dry-run must report `changed_rows=0`, `blocked_rows=0`, and all
  ciphertext rows current before a retained key may be pruned.

Stable failures:

| Condition                                           | Result                                 |
| --------------------------------------------------- | -------------------------------------- |
| Missing DB/vault                                    | `ErrProviderSecretRewriteUnavailable`  |
| Malformed/ambiguous/oversized/context-invalid state | `ErrProviderSecretRewriteInvalid`      |
| Dry-run state or active key changed before execute  | `ErrProviderSecretRewritePlanMismatch` |
| Unrecoverable custom legacy ciphertext exists       | `ErrProviderSecretRewriteBlocked`      |

Required proof covers pure plan/action/hash tests, plan-mismatch and blocked
zero-write behavior, real Postgres legacy/old/current/empty/deleted rows,
active-key-only reload, keyring prepare/prune, owner-only dump/checksum,
restore drill, plaintext/keyring absence, backend restart, and a final no-op
audit.

## 11. G11.9F.2.3 Connection-Test Activation

Administrator endpoints:

```text
POST /v1/admin/providers/{providerId}/test
POST /v1/admin/providers/{providerId}/activate
```

Contract:

- `test` performs the bounded real model-list request and commits an attestation
  without changing a disabled provider to enabled;
- `activate` performs the same test and atomically commits both attestation and
  enabled state;
- the request is capped at 15 seconds, the response at 2 MiB, and normalized
  model IDs at 2048; redirects, URL userinfo/query/fragment, malformed JSON,
  non-2xx responses, and unsupported provider types fail closed;
- OpenAI and OpenAI Compatible use Bearer authentication against `/models`;
  Gemini uses `x-goog-api-key` against `/v1beta/models`;
- the SHA-256 attestation binds Provider ID, canonical type, normalized base
  URL, and the exact encrypted secret reference. It contains no plaintext Key;
- changing type, base URL, or API Key clears the attestation and disables the
  provider. Name/model-list-only changes retain a still-valid proof;
- Postgres commits with expected prior type/base URL/secret/enabled values, so
  an administrator write racing the network call produces
  `PROVIDER_CONFIG_CHANGED` rather than blessing stale state;
- public config, model listing, chat, image generation, and answer-governance
  bootstrap resolve only enabled providers with a valid attestation;
- model-provider settings and Keys are now Postgres/vault-only. The legacy
  model-provider environment variables are not parsed, passed by Compose, or
  present in backend/frontend containers. `PROVIDER_TIMEOUT` and the
  Docker-Secret keyring settings remain infrastructure configuration.

Rollback to an F2.2 image requires the retained keyring and pre-cutover
Postgres backup. Re-provision the former provider environment values from an
operator-owned secret source only if that older image requires them; do not
restore them to an F2.3 deployment.

## 12. G11.9F.3 Search Provider Contexts

Search administrator Keys use the existing BYOK ingress and vault, but never a
model-provider context:

```text
browser ingress: provider:search:<tavily|firecrawl|exa|bocha>
vault at rest:   provider:search:<userId>:<SEARCH:PROVIDER>
```

- Search rows are identified by `config.kind="search"`, a validated
  `config.searchProvider`, and the matching reserved `SEARCH:*` record ID;
- model rows with an empty legacy kind continue to use
  `provider:model:<userId>:<providerId>`;
- copied Search/model ciphertext, mismatched provider metadata, unknown kinds,
  malformed envelopes, and missing retained keys fail before rewrite or
  runtime resolution;
- current and retained-old-key Search vault envelopes participate in the same
  locked rewrite/rotation plan as model rows;
- legacy BYOK Search rows are blocked rather than guessed through a model
  ingress context; the administrator must replace that Key explicitly;
- Search provider Keys have no `.env` fallback. Reads expose only `hasApiKey`,
  and runtime receives plaintext only for the duration of one resolved call.

The one-active and connection-attestation behavior is defined in
[`search-provider-admin.md`](search-provider-admin.md).

## 13. G11.9F.4.2 RAG Provider Contexts

MinerU, Jina, and SiliconFlow administrator Keys use fixed records and distinct
contexts:

```text
browser ingress: provider:rag:<mineru|jina|siliconflow>
vault at rest:   provider:rag:<userId>:<RAG:MINERU|RAG:JINA|RAG:SILICONFLOW>
```

- RAG rows require `config.kind="rag"`, a matching `config.ragProvider`, and
  the exact reserved record ID; model and Search readers reject them;
- MinerU is complementary to retrieval. Jina and SiliconFlow are exact
  alternative retrieval profiles and may coexist for Candidate rebuild and
  rollback without sharing vectors;
- save-and-test may import the matching legacy environment Key only when that
  administrator record has no stored secret. The environment is not a read
  fallback for dynamic status or the future scoped gateway;
- activation repeats a bounded real test and binds the exact vault envelope,
  fixed provider endpoint, parser/model profile, and 1024 retrieval dimensions;
- dynamic `/v1/rag/provider-status` reports `ready` when MinerU and at least one
  retrieval record decrypt under their own contexts; retrieval-only readiness
  is `partial` with native indexing/retrieval available and PDF parsing
  disabled, while missing both retrieval records makes retrieval unavailable;
- current and retained-old-key RAG envelopes participate in the common
  rotation plan. Legacy BYOK RAG rows and reserved IDs using a model context
  are rejected rather than guessed or migrated across contexts.

The public routes and Python/Go operation boundary are defined in
[`rag-provider-admin-gateway.md`](rag-provider-admin-gateway.md). F4.2 keeps
the existing Python provider environment path only for rollback; F4.4 removes
it after the scoped Go gateway is live.

## 14. SiliconFlow Voice Provider Context

Production Voice uses a third provider authority that is never interchangeable
with Model, Search, or RAG:

```text
browser ingress: provider:voice:siliconflow
vault at rest:   provider:voice:<userId>:VOICE:SILICONFLOW
```

- `VOICE:SILICONFLOW` requires `config.kind="voice"`,
  `voiceProvider="siliconflow"`, and the fixed CosyVoice2/base-URL/`claire`
  tuple. `VOICE:ELEVENLABS` and `VOICE:MIMO` remain non-executable reservations.
- Administrator writes accept only encrypted BYOK ingress. There is no Voice
  Key environment fallback, no reuse of `RAG:SILICONFLOW`, and no reuse of a
  model-provider envelope.
- The production connection attestation binds the exact record, tuple, and
  encrypted secret reference. Save/clear/credential changes invalidate the
  proof; activation repeats the bounded real TTS test and commits only if the
  row is unchanged.
- Common key rotation recomputes an existing valid SiliconFlow Voice
  attestation over the rewritten vault envelope while preserving its tested
  time and enabled state. Missing, malformed, stale, ElevenLabs, or MiMo Voice
  attestations remain invalid and rotation never invents one.
- Cross-record or cross-kind ciphertext copies fail AES-GCM context
  authentication. Runtime resolution exposes plaintext only in-process while
  constructing the bounded executor and returns no secret material to clients.

The routes, fixed tuple, cache retention, and rollback procedure are defined in
[`voice-provider-reservation.md`](voice-provider-reservation.md).
