# Provider Secret Vault Design

## Goals

- Keep provider plaintext and the master key out of Postgres backups.
- Make restart persistence independent from the ephemeral BYOK RSA key.
- Bind each ciphertext to its exact provider kind and record identity.
- Support staged key rotation without accepting arbitrary algorithms or keys.
- Keep all loader and crypto failures stable and secret-free.

## Non-goals

- No repository migration or runtime credential resolution in G11.9F.1.
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
| Unpadded base64url keys | Closed portable encoding without whitespace or PEM ambiguity. |
| Context in authenticated header | Copying ciphertext between provider records fails authentication. |
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
- Rotation is only a primitive in F.1; F.2+ must add transactional row rewrite,
  restart proof, and old-key removal gates.
- Docker Secret mounting and removal of provider `.env` fallbacks are deferred
  until the repository cutover has a tested rollback path.

## Change History

### 2026-07-18 — G11.9F.1

Added the strict Docker-Secret keyring loader, context-bound AES-256-GCM
envelope, retained-key decryption, and rotation primitive without production
wiring or persisted-state mutation.
