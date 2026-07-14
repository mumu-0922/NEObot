# Offline JCS Interoperability Fixtures

This directory contains three suites. The first two remain intentionally
separate:

- `c1-contract-profile-v1.json` is project-authored, float-free C1 data. It
  accepts only containers composed from `null`, booleans, Unicode scalar
  strings other than NUL, and integers in `[-(2^53-1), 2^53-1]`. Accepted
  inputs must already be byte-for-byte RFC 8785 canonical JSON.
- `rfc8785-v1.json` contains RFC 8785 float serialization and UTF-16 property
  ordering vectors. It is conformance material, not a C1 payload allowance.
- `logical-hash-golden-v1.json` contains only provenance and a raw-file
  SHA-256 binding for the canonical checked-in
  `../parser_contracts/logical_hash/golden-v1.json`. The 24 logical-ID
  envelopes are loaded from that source file, never copied into this fixture
  directory. Each runtime independently computes
  `SHA256(ASCII(domain-with-one-terminal-LF) || JCS(envelopeWithoutDomain))`.

The first two manifests conform to Draft 2020-12
`jcs-vector-manifest-v1.schema.json`; the logical-hash provenance manifest
conforms to `jcs-logical-hash-manifest-v1.schema.json`. Every object shape in
the manifest and gate-summary schemas is closed. Schema IDs use only
`https://schemas.mm-chat.invalid/parser/`.

`inputHex` and `expectedHex` preserve exact bytes, including malformed or
non-canonical inputs. Implementations compare decoded bytes directly and also
recompute every SHA-256. No newline or whitespace stripping is permitted.
`fixtureSetSha256` and `provenance.materialSha256` are the SHA-256 of the JCS
canonical bytes of the manifest's `cases` array.

## Provenance and licensing

The RFC suite records its source, publication revision, and derived-material
hash in its manifest. Its number cases are from RFC 8785 Appendix B; string and
UTF-16 ordering cases are from Sections 3.2.2.2 and 3.2.3. The applicable IETF
code-component notice is vendored as `LICENSE-IETF-RFC8785.txt`; these vectors
must not be represented as project MIT material.

The C1 suite is original MM Chat test data under the repository license. Its
manifest still records a revision and material hash so fixture changes cannot
be silently relabeled.

The logical-hash suite is also project-authored material. Its manifest binds
the raw SHA-256 and repository-relative path of the parser-contract golden;
the gate reports that same raw hash for every passing runtime and at the
aggregate level.
