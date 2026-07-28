# Memory v2 Portability, Restore Replay, and Retention

## 1. Scope / Trigger

This contract applies to Memory v2 user export/import, imported revision
history, encrypted off-host deletion manifests, restore-time deletion replay,
backup-set metadata, and backup retention. It does not promote a new Memory
reader: the v1 Global Top 5 remains the only prompt and Usage authority.

The three surfaces share one security boundary:

- authenticated user portability may read or add only the current user's L1
  canonical Memory;
- operator deletion replay may only make restored data less visible and erase
  plaintext;
- retention may delete only complete, verified backup sets under one fixed
  backup root.

No path in this slice calls a Provider, reads Live Memory during tests, writes
raw chat text to an archive, or treats an external identifier as local
authority.

## 2. Signatures

Authenticated HTTP routes:

```text
POST /v1/memory-export
POST /v1/memory-import/dry-run
POST /v1/memory-import/confirm
```

Export accepts strict JSON:

```json
{"passphrase":"request-only","includeHistory":true}
```

Import accepts `multipart/form-data`. `package` is the encrypted
`.mm-memory`; `passphrase`, `mappings`, and, for confirm, `planToken` are
bounded form fields. Confirm resubmits the same encrypted package.

Operator commands:

```text
mm-chat-admin memory-deletions-export --output <file> --passphrase-stdin
mm-chat-admin memory-deletions-replay --input <file> --passphrase-stdin
mm-chat-admin backup-retention --backup-root <root>
mm-chat-admin backup-retention --backup-root <root> \
  --execute --expected-plan-sha256 <sha256>
```

Passphrases are accepted only through request bodies, standard input, or a
mode-`0600` passphrase file. They are forbidden in argv, environment, logs,
metrics, jobs, database rows, and temporary filenames.

`.mm-memory` is an `age` v1 passphrase/scrypt authenticated stream produced by
`filippo.io/age v1.3.1`. Its plaintext is UTF-8 JSONL. Every line is a strict
single JSON object, serialized without insignificant whitespace and followed
by `\n`:

```text
manifest
settings suggestion (zero or one)
projects ordered by portable ref
memories ordered by portable ref
optional revisions ordered by memory ref then revision
```

The manifest is the first line:

```json
{
  "kind":"manifest",
  "format":"mm-memory",
  "formatVersion":1,
  "schemaVersion":1,
  "createdAt":"2026-07-28T00:00:00Z",
  "exporterRelease":"release-id",
  "includeHistory":true,
  "counts":{"settings":1,"projects":1,"memories":2,"revisions":3},
  "recordsSha256":"64-lowercase-hex"
}
```

`recordsSha256` hashes the exact bytes of every record line after the manifest,
including each terminating newline. Portable references are package-local
`project-%06d`, `conversation-%06d`, and `memory-%06d` strings. Source instance
user UUIDs and Conversation metadata are not exported.

## 3. Contracts

### Export

- Export runs in one PostgreSQL `REPEATABLE READ, READ ONLY` snapshot. The
  first scan determines counts and `recordsSha256`; the second scan writes the
  same ordered records into the `age` stream.
- Only current, non-deleted L1 canonical rows visible to the authenticated user
  are exported. The record carries content, type, importance, tags, scope,
  lifecycle, enabled state, validity, expiry, sensitivity, subject/fact keys,
  confidence, observed time, and original authority as descriptive metadata.
- Projects carry package-local refs, name, description, and lifecycle. Settings
  are a suggestion only. Revisions are optional and contain a continuous hash
  chain plus the available bounded prior typed snapshot; purged plaintext is
  never reconstructed.
- Raw messages/conversations, evidence excerpts, Usage, Activity, Review
  candidates, exact/BM25/vector projections, embeddings, L2/L3, outbox/jobs,
  logs, Provider payloads, and credentials are excluded.
- Plaintext archives never touch disk. An encrypted temporary file is allowed
  so HTTP status can fail closed before response headers are committed.

### Import planning

The parser order is fixed:

```text
age decrypt and authenticate
  -> format/schema/count/hash validation
  -> local secret detector
  -> field/type/content/history validation
  -> current-user scope mapping and dry-run resolution
```

The parser uses bounded line buffers and streaming counters. Hard maxima are:

```text
decrypted bytes  256 MiB
projects         1,000
memories         50,000
revisions        200,000
content          2,000 Unicode code points
```

Operators may configure smaller values only. Enlarging a hard maximum requires
a new CPU/RSS/DoS benchmark and contract revision.

Secrets and credentials are rejected before any staging or persistence. Their
result contains only ordinal, SHA-256, and a bounded reason code. Planning uses
an in-memory bounded representation or replays the encrypted seekable upload;
there is no plaintext staging table or plaintext temporary archive.

Detection may mark a record while plaintext is streaming, but a package-level
secret or semantic error must not return before the `age` reader reaches EOF
and authenticates the final chunk. Authentication failure takes precedence
over a secret-like Project field so a modified tail cannot be mistaken for an
otherwise valid rejected package.

Every Memory resolves to exactly one result:

| Result             | Meaning and confirm behavior                                   |
| ------------------ | -------------------------------------------------------------- |
| `NOOP`             | exact current same-scope Memory; no write                       |
| `ADD`              | new valid Memory; atomically added on confirm                   |
| `REVIEW`           | current fact conflict; reported, never overwrites canonical     |
| `REJECT`           | secret, malformed, unsupported, or explicitly skipped; no write |
| `SCOPE_REQUIRED`   | Project/Conversation mapping or re-scope is missing; no write   |

Project mappings may select a current user's existing Project or explicitly
create one from bounded package metadata. Conversation mappings may select a
current user's current Conversation, re-scope to an authorized Project or
Global, or skip. Package UUIDs, portable refs, source Memory IDs, and mapping
payloads never bypass current-user SQL reauthorization.

Mapping keys are trimmed before duplicate detection; semantically duplicate
keys such as `project-000001` and ` project-000001 ` reject the whole mapping
payload. Imported prior snapshots participate in the same scope planning as
the current canonical row. A missing prior Project/Conversation mapping yields
`SCOPE_REQUIRED/HISTORY_SCOPE_MAPPING_REQUIRED`; an explicitly skipped prior
scope yields `REJECT/HISTORY_SCOPE_UNAVAILABLE`. If several revisions disagree,
the missing mapping result takes precedence so the user can repair the plan.

Imported settings are returned as suggestions but are never applied by
confirm. In particular, import cannot enable Use, Learn, Sensitive, L2, or L3.

### Confirm

Dry-run returns a ten-minute HMAC token with an independent domain separator.
It binds token version/key ID, authenticated user ID, import ID, package hash,
manifest hash, mappings hash, deterministic plan hash, authority state hash,
issue time, and expiry. The authority state hash binds current Memory
ID/revision/content hash/scope generation, relevant Project/Conversation
revision or scope generation, and settings revision state.

Confirm must:

1. re-read and fully authenticate the same encrypted package;
2. re-create the same deterministic plan;
3. verify token signature, user, expiry, package/manifest/mappings/plan hashes;
4. begin a transaction and re-check the authority state hash;
5. stream a second authenticated parse into the transaction;
6. atomically create authorized Projects and only `ADD` rows;
7. record an ID/hash/count/result-only import batch for idempotency.

Any package, mapping, token, revision, generation, or state drift returns a
stable conflict and requires a new dry-run. Confirm replay returns the original
completed batch result and never creates duplicate Memory.

Imported canonical rows use `source='import'` and
`authority_kind='import'`, have no local message evidence, and receive fresh
local UUIDs. Optional history is accepted only after a continuous revision and
old/new SHA-256 chain is proven. Imported history uses an explicit import actor
and mapped local scope references; it does not claim local message provenance.
Derived projections are rebuilt only from the new local canonical rows.

### Deletion manifest and restore

The off-host deletion package uses the same `age` helper and strict JSONL
parser with format `mm-memory-deletions`, schema version 1, record count, and
records SHA-256. Entry records contain only manifest/event/user/memory/
tombstone opaque IDs, content hash, scope generation, visibility epoch,
deleted/purged UTC time, and bounded result code.

Replay is Provider-free and idempotent. For every entry it matches both Memory
ID and content hash before making that row disabled/deleted, recreating bounded
tombstone/manifest evidence, removing evidence and derived rows, and wiping
canonical/revision plaintext. A mismatch never erases an unrelated row and is
reported with an ID/hash-only result. After all entries, replay discards and
rebuilds every eligible L1 projection while the backend is still unopened.

Supported restore order is mandatory:

```text
stop/keep backend unopened
  -> restore immutable database backup
  -> migrate to current schema
  -> replay latest encrypted off-host deletion package
  -> rebuild projections and queue current vector derivations
  -> run deletion/authority verification
  -> open backend
```

### Backup sets and retention

The production wrapper creates one owner-only `backup/sets/<set-id>.json` and
matching `.sha256` for the PostgreSQL and MinIO artifacts produced under the
same set ID. The strict manifest records version, set ID, class
`daily|weekly|pre-deploy`, UTC creation time, `containsMemoryPlaintext=true`,
and exact relative artifact/checksum paths and SHA-256 values.

Retention computes expiry from manifest time: daily is 14 days; weekly and
pre-deploy are 56 days. Dry-run is the default. Execute requires the SHA-256 of
the complete deterministic plan printed by dry-run, then recomputes and
revalidates the plan immediately before deletion.

Only complete sets whose manifest/checksum pair and every artifact/checksum
pair verify may enter a plan. Every path must remain under the lexical and
resolved fixed backup root; the root, `sets` directory, manifests, checksums,
artifacts, and traversed parents must not be symlinks. Drift, missing files,
duplicate paths, invalid classes/times, or checksum mismatch fails the whole
command closed. Orphans and files not referenced by a verified set are never
deleted.

## 4. Validation and Error Matrix

| Condition | Required result |
| --- | --- |
| Wrong passphrase, truncated age stream, or modified ciphertext | reject without partial plan/write |
| Secret-like Project metadata plus a modified ciphertext tail | authentication failure; do not return the semantic secret error first |
| Unknown/duplicate JSON field, duplicate ref, trailing JSON, non-UTF-8 | reject package |
| Manifest not first, count/hash/order mismatch, unknown major | reject package |
| Any hard cap exceeded | stop streaming with stable limit error |
| Secret candidate | `REJECT` with ordinal/hash only; zero plaintext persistence |
| Mapping keys become duplicates after normalization | reject mapping before planning |
| Project or Conversation mapping crosses user | reject mapping; no candidate write |
| Mapping absent | `SCOPE_REQUIRED`; confirm cannot write that row |
| Imported prior scope mapping is absent or skipped | current ADD becomes `SCOPE_REQUIRED` or `REJECT`; confirm cannot reach unresolved history |
| Existing same-scope normalized content | `NOOP` |
| Existing current same-scope fact with different content | `REVIEW`; never overwrite |
| Token expired/tampered or user/package/plan mismatch | conflict; rerun dry-run |
| Revision/scope/settings authority drift | conflict; rerun dry-run |
| Confirm replay | return completed ID/hash/count result; no duplicate |
| Deletion replay ID/hash mismatch | bounded mismatch result; do not erase row |
| Restore attempts to open backend before replay | unsupported; operational gate fails |
| Backup path escapes root or any component is symlink | reject entire retention plan |
| Manifest/artifact/checksum missing or mismatched | reject entire retention plan |
| Execute plan hash differs from fresh plan | delete nothing |

## 5. Good / Base / Bad Cases

- **Good**: export a scoped package with history, tamper-test it, dry-run against
  a second current user with explicit mappings, confirm only ADD rows, prove
  source/import authority and no evidence, then restore an older backup and
  replay the encrypted deletion package before projection rebuild/API start.
- **Base**: export Global L1 without history, import one exact NOOP and one ADD,
  retain settings as a visible suggestion, and leave the v1 reader unchanged.
- **Bad**: treat a database dump as a portable archive, pass a password on the
  command line, trust source UUIDs, stage decrypted JSON in PostgreSQL, apply
  settings automatically, overwrite a conflict, or prune files by filename age
  without verifying their set manifest and checksum pair.

## 6. Tests Required

- Pin `filippo.io/age v1.3.1`; test scrypt round trip, wrong passphrase,
  ciphertext/header/body tamper, truncation, and close/final authentication.
- Test canonical JSONL determinism, strict unknown/duplicate/trailing rejection,
  manifest-first/order/count/hash checks, each hard cap, bounded line memory,
  Unicode code-point length, duplicate refs, normalized mapping-key duplicates,
  and revision chain validation.
- Prove secret zero-persistence, settings suggestion-only behavior,
  cross-user Project/Conversation denial, each dry-run result, deterministic
  plan/token, token expiry/tamper, package/mapping/plan/state drift, atomic
  confirm rollback, confirm idempotency, final-chunk authentication before a
  Project secret error, and missing/skipped imported-history scope planning.
- Prove imported source/authority/history shape, no local message evidence,
  projection rebuild, and byte-equivalent v1 prompt/Usage behavior.
- On PostgreSQL 17, prove runtime table-CRUD denial, SECURITY DEFINER
  reauthorization, replay ID/hash fence, online plaintext wipe, idempotency,
  full projection rebuild, and guarded `060 -> 061 -> 060 -> 061`.
- Test backup-set creation and retention daily/weekly/pre-deploy boundaries,
  default dry-run, execute plan binding, missing/tampered/duplicate pairs,
  symlink and traversal attacks, plan drift, partial delete failure reporting,
  and orphan preservation.
- Run focused race tests, all backend tests and vet, frontend format/lint/
  typecheck/tests/build, Compose/preflight/backend image, and
  `scripts/verify-standalone.sh --full`. Offline tests make zero Live Provider
  calls and do not read or modify Live Memory.

## 7. Wrong vs Correct

### Wrong

```text
multipart upload -> decrypt to /tmp/memory.json -> INSERT staging rows
  -> trust source project_id -> overwrite conflicts -> turn imported settings on
```

This leaks plaintext, creates a second authority, permits IDOR, and makes a
failed or tampered package partially durable.

### Correct

```text
encrypted seekable upload
  -> full authenticated bounded parse
  -> secret-before-staging gate
  -> current-user mapped deterministic dry-run
  -> HMAC-bound short confirmation
  -> re-authenticate + state-fenced atomic ADD-only apply
  -> local projections rebuilt; v1 reader unchanged

verified immutable backup
  -> current migration
  -> encrypted deletion replay while backend is closed
  -> plaintext wipe + projection rebuild
  -> authority proof
  -> backend open
```
