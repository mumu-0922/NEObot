# Persistence Docs

Persistence docs describe the current Postgres source-of-truth contract for the
`mm-chat` server-backed refactor. The schema head is migration `010`;
the Phase 4, 4.5, and 5.x labels below are retained as implementation history,
not as limits on the current runtime.

## Documents

| Guide                                                                      | Purpose                                                                                                                                                                                                                                              |
| -------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`postgres-schema.md`](./postgres-schema.md)                               | Schema detail for the `001`–`009` foundation, including chat/file/import persistence, identity and Team state, Knowledge ACL entities, Processing Jobs, Governance, Consent, Outbox, and migration-runner guarantees.                                |
| [`runtime-wiring.md`](./runtime-wiring.md)                                 | Current DB runtime wiring contract: four credential routes, pgx connector behavior, readiness, migration CLI flow, and rollback boundaries.                                                                                                          |
| [`phase-15-rag-projection-schema.md`](./phase-15-rag-projection-schema.md) | Canonical `010/011` projection contract for corpus generations, document materializations, canonical blocks/chunks, projection fencing, search-profile separation, least-privilege roles, and rollback. `010` is implemented; `011` remains pending. |

## Current Migration Boundary

Postgres owns durable structured state:

```text
Go backend -> Postgres
```

The ordered schema currently consists of:

- `001` creates the original users, sessions, provider configs, chat, file,
  attachment, and audit-log foundation.
- `002` adds assistant `runId` cancellation lookup; `003` adds durable browser
  import batches.
- `004` adds account credentials/recovery, Teams/Memberships/Invites,
  Collections/Documents/Versions, Governance Profiles/Heads, Processing
  Consent and query-consent revision state, plus the Knowledge Outbox.
- `005` adds scoped Team/Invite idempotency, encrypted durable Invite mail
  delivery, pending-Invite uniqueness, and Membership deletion fencing.
- `006` adds Collection display/replay metadata, Document/Version replay and
  visibility fields, and durable stage-specific Knowledge Processing Jobs with
  authority snapshots and a per-Version purge fence.
- `007` idempotently restores that purge-fence index for databases created from
  a short-lived `006` variant that omitted it; current `006` already creates it.
- `008` makes Processor Governance Profiles immutable with a trigger rejecting
  `UPDATE` and `DELETE`.
- `009` adds the one-time Consent expiry-materialization marker and due-work
  index.
- `010` implements the extension-independent durable RAG projection: model-aware
  Governance/Consent bindings, corpus generations, document materializations,
  parser artifacts, canonical blocks/chunks, event ledgers, lease/CAS fencing,
  purge/publish/hydration functions, and least-privilege capability roles.
- `011` remains pending. Tokenizer, vector, BM25/search-extension types and
  indexes, search profiles, and restricted search DDL are not in the current
  schema.

The historical Phase 4.5 runtime wiring keeps DB startup explicit:
`DATABASE_URL` enables Postgres for the API, `/ready` checks it, and API startup
never runs migrations. Operators apply the embedded migration chain with the
independently required `MIGRATION_DATABASE_URL` before starting or restarting a
DB-enabled release. The migration CLI never falls back to `DATABASE_URL`.

The historical Phase 5.1/5.2 boundary first activated chat CRUD and assistant
stream persistence. It has since expanded: auth/session, Team, Knowledge
Collection/Document, Governance, and Consent repositories now use the schema.
Migration `010` also provides the durable, extension-independent projection and
its worker/replay database functions. The `011` tokenizer/vector/search DDL is
still absent.

## Source-of-Truth Rules

- Postgres is canonical for users, credentials, sessions, Teams/Memberships,
  conversations, messages, file metadata, provider configuration metadata,
  browser import state, audit logs, Knowledge ACL entities, Governance,
  Consent, Processing Jobs, and transactional Outbox events.
- File bytes are not stored in Postgres. Migration `010` stores durable
  extension-independent projection state and derived parser/block/chunk
  records; extension-specific tokenizer, vector, and search projections remain
  deferred to `011`.
- Redis remains non-authoritative temporary state.
- Migration-runner metadata in `schema_migrations` is not an application table.
  Each applied row records migration name and a SHA-256 checksum over migration
  identity plus both SQL directions; mismatches fail closed.
- Legacy applied rows without checksums require an operator-reviewed, explicit
  `MIGRATION_DATABASE_URL="postgres://..." go run ./cmd/migrate baseline`.
  Routine deploys must not automate baseline.
- `up`, `down`, and `baseline` hold one PostgreSQL advisory lock across metadata
  validation and the requested operation.
- Migration, API/admin, Worker, and Replay use separate credential routes:
  `MIGRATION_DATABASE_URL`, `DATABASE_URL`, `RAG_WORKER_DATABASE_URL`, and
  `RAG_REPLAY_DATABASE_URL`. Their login names and passwords must remain
  pairwise distinct; no route falls back to another route's URL.
- API startup must not run migrations automatically; operators run the migration
  CLI before starting/restarting a DB-enabled backend release.
- Guarded `010.down` removes the projection surface only after its safety
  preconditions pass. It retains `go_api_runtime`, schema usage, the explicit
  CRUD grants on the authoritative `001`–`009` relations, and
  `knowledge_outbox_id_seq` access needed by the rolled-back schema-`009` API.
- Browser IndexedDB/OPFS data is imported only after explicit user action.
- Provider secrets, raw invitation/recovery tokens, and private object keys stay
  server-side and must not be returned to the browser.

## Related Docs

- [`../contracts/chat-crud-api.md`](../contracts/chat-crud-api.md) defines the
  chat CRUD REST contract and DB-disabled endpoint behavior.
- [`../contracts/knowledge-acl-api.md`](../contracts/knowledge-acl-api.md)
  defines the implemented Team/Knowledge ACL contract and future search/RAG
  boundary.
- [`../architecture/server-refactor-design.md`](../architecture/server-refactor-design.md)
  records the full refactor phases and target architecture.
- [`../inventory/storage.md`](../inventory/storage.md) inventories local-first
  storage and the server replacement.
- [`../deployment/postgres-single-server.md`](../deployment/postgres-single-server.md)
  defines single-server Postgres operation.
- [`../deployment/single-server-compose.md`](../deployment/single-server-compose.md)
  preserves the broader single-server topology background.
