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
| [`redis-temporary-state.md`](./redis-temporary-state.md) | Phase 7 Redis runbook for non-authoritative temporary state, stream cancellation flags, private-network rules, and flush behavior. |
| [`../persistence/runtime-wiring.md`](../persistence/runtime-wiring.md) | Phase 4.5 backend DB env, pgx connector behavior, readiness matrix, migration CLI flow, and rollback boundaries. |

## Current Boundary

- Phase 3/4/4.5 deployment docs only; no Compose implementation file is created
  here.
- Future Compose assets should stay isolated under `mm-chat/`, not overwrite the
  repository-root deployment files.
- MinIO must remain private; the Go backend is the public file authorization
  gateway. Runtime config uses `STORAGE_BACKEND=minio|s3` plus `S3_*`
  variables; do not use stale `OBJECTSTORE_DRIVER` / `FILE_MAX_BYTES` names.
- MVP is `frontend -> Go backend -> Postgres -> provider stream`. Redis is now
  available only for non-authoritative temporary cancellation flags; rate limits,
  sessions, RAG, browser data import, and production backup automation remain
  later phases.
- Phase 4.5 runtime wiring keeps `DATABASE_URL` empty mode DB-disabled with
  `/ready` returning `200`; when `DATABASE_URL` is set, startup and `/ready`
  ping Postgres and readiness returns `503` on DB ping failure.
- API startup must not auto-run migrations; operators use the migration CLI
  before starting or restarting a DB-enabled backend release.
- Local provider secrets belong in `mm-chat/backend/.env`, which is ignored by
  Git. Use `mm-chat/backend/.env.example` as the template and inject it with
  Docker `--env-file` or a future Compose `env_file` entry. For direct local
  `go run`, load it first with `set -a; . ./mm-chat/backend/.env; set +a`.
  The Go API reads process environment variables; it does not auto-load `.env`,
  commit provider API keys, or print provider API keys.
- The current real provider path is OpenAI-compatible streaming:
  `PROVIDER_TYPE=openai_compatible`,
  `PROVIDER_BASE_URL=<your OpenAI-compatible relay /v1 URL>`,
  `PROVIDER_MODEL=gpt-5.5`, and a local-only `PROVIDER_API_KEY`.
- The current real object-store path supports `STORAGE_BACKEND=local`,
  `minio`, or `s3`. Use `S3_BUCKET_AUTO_CREATE=false` in production and
  provision the bucket/credentials outside the app release.
- Redis config uses `REDIS_URL`, `REDIS_KEY_PREFIX`, and
  `REDIS_RUN_CANCEL_TTL`. Leave `REDIS_URL` empty to disable Redis; if set, the
  API fails fast when Redis cannot be parsed or pinged.
