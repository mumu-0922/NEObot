# Persistence Docs

Persistence docs define the Phase 4 Postgres source-of-truth contract, the
Phase 4.5 runtime-wiring boundary, and the Phase 5.1 repository usage boundary
for the `mm-chat` server-backed refactor. They are schema and operations
references for migration and deployment work; only the Phase 5.1 chat CRUD path
claims a narrow repository usage boundary.

## Documents

| Guide | Purpose |
| --- | --- |
| [`postgres-schema.md`](./postgres-schema.md) | Phase 4 Postgres schema responsibilities plus the Phase 5.1 repository usage boundary for `users`, `conversations`, and `messages`; files, provider configs, audit logs, and auth/session usage remain later work. |
| [`runtime-wiring.md`](./runtime-wiring.md) | Phase 4.5 DB runtime wiring contract: env vars, pgx connector behavior, readiness matrix, migration CLI flow, and rollback boundaries. |

## Phase Boundary

Phase 4 owns durable structured persistence:

```text
Go backend -> Postgres
```

Phase 4.5 wires the Go backend to Postgres when `DATABASE_URL` is set, keeps DB
optional when it is empty, adds DB-aware `/ready`, and exposes migrations through
an explicit CLI. API startup must not auto-migrate.

Phase 5.1 adds the first narrow repository usage boundary for chat CRUD:
`users`, `conversations`, and `messages` only. Chat CRUD endpoints must return
`503 DATABASE_REQUIRED` when DB runtime wiring is disabled; they must not fall
back to in-memory persistence.

Phase 5.2 adds the first assistant streaming-row persistence boundary: create a
`streaming` assistant message for an existing user message, then finalize it to
`completed`, `failed`, or `cancelled` after the provider stream. It still uses
only `users`, `conversations`, and `messages`.

Phase 4/4.5/5.1/5.2 does **not** add real provider adapters, explicit SSE
cancellation endpoints, auth, multi-user permissions, MinIO file bytes, Redis
session/cache/cancellation state, RAG services, browser data import, or a
committed Docker Compose implementation. Those remain later phases unless a
later task explicitly changes the boundary.

## Source-of-Truth Rules

- Postgres is canonical for users, sessions, conversations, messages, file
  metadata, provider configuration metadata, and audit logs. Phase 5.1 code uses
  only `users`, `conversations`, and `messages`.
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

- [`../contracts/chat-crud-api.md`](../contracts/chat-crud-api.md)
  defines the Phase 5.1 REST contract for chat CRUD and DB-disabled endpoint
  behavior.
- [`../architecture/server-refactor-design.md`](../architecture/server-refactor-design.md)
  defines the full refactor phases and target architecture.
- [`../inventory/storage.md`](../inventory/storage.md) inventories current
  local-first storage and the target server replacement.
- [`../deployment/postgres-single-server.md`](../deployment/postgres-single-server.md)
  defines the single-server Postgres container plan for Phase 4.
- [`../deployment/single-server-compose.md`](../deployment/single-server-compose.md)
  records the broader single-server topology draft without creating Compose
  assets.
