# Persistence Docs

Persistence docs describe the current Postgres source-of-truth contract for the
`mm-chat` server-backed refactor. The schema is current through migration `009`;
the Phase 4, 4.5, and 5.x labels below are retained as implementation history,
not as limits on the current runtime.

## Documents

| Guide                                        | Purpose                                                                                                                                                                                                                              |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [`postgres-schema.md`](./postgres-schema.md) | Current schema and runtime boundary through migration `009`, including chat/file/import persistence, identity and Team state, Knowledge ACL entities, Processing Jobs, Governance, Consent, Outbox, and migration-runner guarantees. |
| [`runtime-wiring.md`](./runtime-wiring.md)   | DB runtime wiring contract: env vars, pgx connector behavior, readiness matrix, migration CLI flow, and rollback boundaries.                                                                                                         |

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

The historical Phase 4.5 runtime wiring keeps DB startup explicit:
`DATABASE_URL` enables Postgres, `/ready` checks it, and API startup never runs
migrations. Operators apply the embedded migration chain before starting or
restarting a DB-enabled release.

The historical Phase 5.1/5.2 boundary first activated chat CRUD and assistant
stream persistence. It has since expanded: auth/session, Team, Knowledge
Collection/Document, Governance, and Consent repositories now use the schema.
Search indexes and downstream RAG execution are not part of the current
Postgres schema.

## Source-of-Truth Rules

- Postgres is canonical for users, credentials, sessions, Teams/Memberships,
  conversations, messages, file metadata, provider configuration metadata,
  browser import state, audit logs, Knowledge ACL entities, Governance,
  Consent, Processing Jobs, and transactional Outbox events.
- File bytes and derived search/RAG artifacts are not stored in Postgres. Store
  only metadata, ownership, integrity, object-location references, and durable
  work/fence state.
- Redis remains non-authoritative temporary state.
- Migration-runner metadata in `schema_migrations` is not an application table.
  Each applied row records migration name and a SHA-256 checksum over migration
  identity plus both SQL directions; mismatches fail closed.
- Legacy applied rows without checksums require an operator-reviewed, explicit
  `go run ./cmd/migrate baseline`. Routine deploys must not automate baseline.
- `up`, `down`, and `baseline` hold one PostgreSQL advisory lock across metadata
  validation and the requested operation.
- API startup must not run migrations automatically; operators run the migration
  CLI before starting/restarting a DB-enabled backend release.
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
