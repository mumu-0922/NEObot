# Deployment Docs

Deployment docs define the single-server path for the `mm-chat` server-backed
refactor. The current contract is: prove the Go backend skeleton first, then add
Postgres for the MVP chat spine, wire DB-aware runtime readiness in Phase 4.5,
then add MinIO/Redis/RAG only when their phase boundaries are ready.

## Documents

| Guide | Purpose |
| --- | --- |
| [`single-server-compose.md`](./single-server-compose.md) | Phase 3 single-server runbook and future Compose topology draft for backend, Postgres, Redis, MinIO, backup, ports, and file-security boundaries. |
| [`postgres-single-server.md`](./postgres-single-server.md) | Phase 4/4.5 single-server Postgres container plan covering private ports, data directories, env draft, DB-aware health checks, migration execution, backup/restore, and rollback. |
| [`../persistence/runtime-wiring.md`](../persistence/runtime-wiring.md) | Phase 4.5 backend DB env, pgx connector behavior, readiness matrix, migration CLI flow, and rollback boundaries. |

## Current Boundary

- Phase 3/4/4.5 deployment docs only; no Compose implementation file is created
  here.
- Future Compose assets should stay isolated under `mm-chat/`, not overwrite the
  repository-root deployment files.
- MinIO must remain private; the Go backend is the public file authorization
  gateway.
- MVP is `frontend -> Go backend -> Postgres -> provider stream`. Redis, MinIO,
  RAG, browser data import, and production backup automation are later phases.
- Phase 4.5 runtime wiring keeps `DATABASE_URL` empty mode DB-disabled with
  `/ready` returning `200`; when `DATABASE_URL` is set, startup and `/ready`
  ping Postgres and readiness returns `503` on DB ping failure.
- API startup must not auto-run migrations; operators use the migration CLI
  before starting or restarting a DB-enabled backend release.
