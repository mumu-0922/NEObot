# Provider Secret Vault

`providersecrets` is the at-rest encryption primitive for administrator-managed
model, Search, MinerU, Jina, and future Voice credentials. It keeps the
decryption root in a bounded keyring file intended for a Docker Secret and
produces a versioned ciphertext envelope suitable for Postgres JSON/text
storage.

## Responsibilities

- strictly load one bounded versioned keyring file;
- encrypt non-empty secret bytes with the active AES-256-GCM key;
- bind ciphertext to a caller-owned provider context through authenticated
  additional data;
- decrypt envelopes using the active or a retained previous key;
- rotate an old envelope onto the active key without exposing key material;
- return stable errors without paths, key IDs, ciphertext, or plaintext.

The package does not accept browser input, access Postgres, authorize admin
requests, call providers, or log secrets. G11.9F.2.1 wires it behind the
existing BYOK ingress envelope: new provider secrets and lazily imported legacy
defaults are re-encrypted before the repository write. Transactional bulk
rotation remains G11.9F.2.2.

## Keyring Format

```json
{
  "v": 1,
  "activeKid": "provider-2026-07",
  "keys": [
    {
      "kid": "provider-2026-07",
      "key": "<32 random bytes encoded as unpadded base64url>"
    }
  ]
}
```

Production will mount this document read-only from Docker Secret storage. A
rotation first adds the new active key while retaining the previous key, then
rewrites every envelope, verifies reload, and only then removes the previous
key.

In the single-server Compose deployment the source remains owned by the
deployment user with mode `600`. Compose preserves that ownership for
file-backed secrets, so `backend` and `admin` run as the matching configured
non-root UID/GID rather than weakening file permissions.

## Usage

```go
vault, err := providersecrets.LoadVaultFile(
    "/run/secrets/mm_chat_provider_keyring",
)
if err != nil {
    return err
}
envelope, err := vault.Encrypt(
    []byte(apiKey),
    "provider:search:tavily",
)
```

See [DESIGN.md](DESIGN.md) and
[`docs/contracts/provider-secret-vault.md`](../../../docs/contracts/provider-secret-vault.md).
