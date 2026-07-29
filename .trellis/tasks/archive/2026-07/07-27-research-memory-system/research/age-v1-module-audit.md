# `filippo.io/age` v1.3.1 source and license audit

## Decision

Memory portability and deletion-manifest encryption will use
`filippo.io/age v1.3.1` directly. Neo Chat will use `NewScryptRecipient`,
`Encrypt`, `NewScryptIdentity`, and `Decrypt`; it will not implement a custom
KDF, AEAD, envelope, or password-file format.

## Reproducible module evidence

Command executed from `mm-chat/backend` on 2026-07-28:

```bash
go mod download -json filippo.io/age@v1.3.1
```

Resolved evidence:

```text
module:     filippo.io/age
version:    v1.3.1
origin:     https://github.com/FiloSottile/age
tag commit: b8564adb6d58329b8a3e267360ca2b0abc4efe1d
module sum: h1:hbzdQOJkuaMEpRCLSN1/C5DX74RPcNCk6oqhKMXmZi0=
go.mod sum: h1:EZorDTYUxt836i3zdori5IJX/v2Lj6kWFU0cfh6C0D4=
```

The downloaded source exposes the required APIs in `scrypt.go` and `age.go`.
The module license is the 3-clause BSD license, with copyright notices for the
age authors, Google LLC, and Filippo Valsorda. The backend binary may include
the module under those redistribution terms; the existing product does not
claim endorsement by the project or contributors.

## Security boundary

- Keep the library default scrypt work factor in production. Tests may lower
  it only through an injected test seam; exported production packages never do.
- Encryption is complete only after closing the writer successfully. Import is
  complete only after reading the decrypted stream through authenticated EOF.
- Wrong passphrases, changed headers/body, and truncation are explicit negative
  tests.
- Passphrases are request/command-memory-only and are never accepted through an
  environment variable or command-line value.
- The inner JSONL format still needs strict schema, count, hash, order, secret,
  scope, and resource caps; authenticated encryption does not make imported
  plaintext trusted.
