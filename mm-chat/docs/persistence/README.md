# Persistence Docs

Persistence docs define the Phase 4 Postgres source-of-truth contract and the
Phase 4.5 runtime-wiring boundary for the `mm-chat` server-backed refactor. They
are schema and operations references for migration and deployment work; they do
not claim that a database repository layer is already implemented.

## Documents

| Guide | Purpose |
| --- | --- |
| [`postgres-schema.md`](./postgres-schema.md) | Phase 4 Postgres schema responsibilities, field intent, relationships, indexes, and data-boundary rules for users, sessions, conversations, messages, file metadata, and audit logs. |
| [`runtime-wiring.md`](./runtime-wiring.md) | Phase 4.5 DB runtime wiring contract: env vars, pgx connector behavior, readiness matrix, migration CLI flow, and rollback boundaries. |

## Phase Boundary

Phase 4 owns durable structured persistence:

```text
Go backend -> Postgres
```

Phase 4.5 wires the Go backend to Postgres when `DATABASE_URL` is set, keeps DB
optional when it is empty, adds DB-aware `/ready`, and exposes migrations through
an explicit CLI. API startup must not auto-migrate.

Phase 4/4.5 does **not** add MinIO file bytes, Redis session/cache hardening,
RAG services, browser data import, or a committed Docker Compose implementation.
Those remain later phases unless a later task explicitly changes the boundary.

## Source-of-Truth Rules

- Postgres is canonical for users, sessions, conversations, messages, file
  metadata, provider configuration metadata, and audit logs.
- File bytes are not stored in Postgres. Store only metadata, ownership,
  integrity, and object-location references.
- Redis, when introduced later, must remain non-authoritative temporary state.
- Migration-runner metadata in `schema_migrations` is not an application table;
  it records applied SQL versions for the runner.
- API startup must not run migrations automatically; operators run the migration
  CLI before starting/restarting a DB-enabled backend release.
- Browser IndexedDB/OPFS data is imported only after explicit user action.
- Provider secrets stay server-side and must not be returned to the browser.

## Related Docs

- [`../architecture/server-refactor-design.md`](../architecture/server-refactor-design.md)
  defines the full refactor phases and target architecture.
- [`../inventory/storage.md`](../inventory/storage.md) inventories current
  local-first storage and the target server replacement.
- [`../deployment/postgres-single-server.md`](../deployment/postgres-single-server.md)
  defines the single-server Postgres container plan for Phase 4.
- [`../deployment/single-server-compose.md`](../deployment/single-server-compose.md)
  records the broader single-server topology draft without creating Compose
  assets.
