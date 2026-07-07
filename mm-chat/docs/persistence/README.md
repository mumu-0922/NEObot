# Persistence Docs

Persistence docs define the Phase 4 Postgres source-of-truth contract for the
`mm-chat` server-backed refactor. They are schema and operations references for
migration and deployment work; they do not claim that a database repository layer
is already implemented.

## Documents

| Guide | Purpose |
| --- | --- |
| [`postgres-schema.md`](./postgres-schema.md) | Phase 4 Postgres schema responsibilities, field intent, relationships, indexes, and data-boundary rules for users, sessions, conversations, messages, file metadata, and audit logs. |

## Phase Boundary

Phase 4 owns durable structured persistence:

```text
Go backend -> Postgres
```

Phase 4 does **not** add MinIO file bytes, Redis session/cache hardening, RAG
services, browser data import, or a committed Docker Compose implementation.
Those remain later phases unless a later task explicitly changes the boundary.

## Source-of-Truth Rules

- Postgres is canonical for users, sessions, conversations, messages, file
  metadata, provider configuration metadata, and audit logs.
- File bytes are not stored in Postgres. Store only metadata, ownership,
  integrity, and object-location references.
- Redis, when introduced later, must remain non-authoritative temporary state.
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
