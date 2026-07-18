# Provider Secret Vault Design

## Goals

- Keep provider plaintext and the master key out of Postgres backups.
- Make restart persistence independent from the ephemeral BYOK RSA key.
- Bind each ciphertext to its exact provider kind and record identity.
- Support staged key rotation without accepting arbitrary algorithms or keys.
- Keep all loader and crypto failures stable and secret-free.

## Non-goals

- No repository implementation inside this package.
- No administrator API, frontend form, connection test, or provider activation.
- No environment-variable master key fallback.
- No remote KMS/HSM integration in the single-server phase.

## Flow

```text
Docker Secret keyring file
          |
          v
 strict bounded loader --> immutable Vault
                              |
            plaintext + provider context
                              |
                              v
                     AES-256-GCM envelope
                              |
                              v
                    future Postgres write

retained old key --> decrypt old envelope --> encrypt with active key
```

## Decisions

| Decision | Reason |
| --- | --- |
| Separate ingress and at-rest encryption | Browser BYOK RSA protects transport; a Docker-Secret keyring owns restart-stable database encryption. |
| AES-256-GCM | Standard-library AEAD provides confidentiality and integrity with a small envelope. |
| Versioned strict JSON keyring | One Docker Secret can carry the active and bounded previous keys needed for safe rotation. |
| Host-owner runtime UID/GID | Compose file secrets retain host mode/ownership; matching a non-root consumer keeps mode `600` readable without running the API as root. |
| Unpadded base64url keys | Closed portable encoding without whitespace or PEM ambiguity. |
| Context in authenticated header | Copying ciphertext between provider records fails authentication. |
| Domain-separated active-key binding | Exact operator plans detect key-byte replacement even if an unsafe key ID is reused, without exposing the key. |
| Immutable in-memory key map | Concurrent reads require no locks and callers cannot retrieve raw keys. |
| Stable sentinel errors | Logs and HTTP mappers never need to expose file paths or crypto details. |

## Security Contract

- exactly 32 decoded bytes per key and at most 16 keys;
- key IDs match `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`;
- keyring files are regular JSON files, at most 64 KiB, with unknown fields and
  trailing values rejected;
- secret plaintext is non-empty and at most 64 KiB; provider-specific callers
  must impose smaller limits;
- context is exact, trimmed, non-empty, at most 256 bytes, and NUL-free;
- a fresh cryptographic nonce is generated for every envelope;
- version, key ID, algorithm, and context are all authenticated as AAD;
- ciphertext, plaintext, decoded keys, file paths, and crypto errors never
  appear in exported error strings.

## Known Limits

- The Go process necessarily holds active key material and decrypted provider
  bytes briefly in memory.
- New writes and lazy imports use the vault in F2.1. F2.2 adds the external
  transactional rewriter; old-key removal remains forbidden until its dry-run,
  exact-plan, backup/restore, rewrite, and active-key-only restart gates pass.
- Model-provider `.env` fallback remains rollback-only until the F2.3 bounded
  connection-test cutover.

## Change History

### 2026-07-18 — G11.9F.2.2

Added strict stored-envelope parsing and the external transactional
backfill/rotation workflow, including retained-key preparation/pruning and
active-key-only restart proof. The vault package still does not access the
database or operator backup state.

### 2026-07-18 — G11.9F.2.1

Wired the vault into model-provider writes and reads, added legacy/env lazy
import, and mounted the mode-`600` keyring into matching non-root Compose
consumers.

### 2026-07-18 — G11.9F.1

Added the strict Docker-Secret keyring loader, context-bound AES-256-GCM
envelope, retained-key decryption, and rotation primitive without production
wiring or persisted-state mutation.
