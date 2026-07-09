# Deployment Docs

Deployment docs define the single-server path for the `mm-chat` server-backed
refactor. The current runtime path is Docker Compose under `mm-chat/`: Go API,
Postgres, Redis, and private MinIO on one server, with migrations and backups
run explicitly by operators.

## Documents

| Guide                                                                  | Purpose                                                                                                                                                                           |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`single-server-compose.md`](./single-server-compose.md)               | Phase 10 Docker Compose topology, first boot, reverse proxy boundary, release, and rollback checklist.                                                                            |
| [`postgres-single-server.md`](./postgres-single-server.md)             | Phase 4/4.5 single-server Postgres container plan covering private ports, data directories, env draft, DB-aware health checks, migration execution, backup/restore, and rollback. |
| [`redis-temporary-state.md`](./redis-temporary-state.md)               | Phase 7 Redis runbook for non-authoritative temporary state, stream cancellation flags, private-network rules, and flush behavior.                                                |
| [`backup-restore.md`](./backup-restore.md)                             | Backup scripts, checksum verification, Postgres restore drill, MinIO restore drill, retention, and destructive-restore warnings.                                                  |
| [`reverse-proxy-tls.md`](./reverse-proxy-tls.md)                       | Phase 14 reverse proxy and TLS edge runbook, including same-origin `/mm-api`, SSE buffering, upload limits, metrics exposure, and rollback.                                       |
| [`secret-rotation.md`](./secret-rotation.md)                           | Phase 14 rotation procedures for auth bootstrap, sessions, provider keys, Postgres, Redis, MinIO app/root credentials, and TLS certificates.                                      |
| [`release-rollback.md`](./release-rollback.md)                         | Compact release and rollback runbook for image deploys, migrations, and failed releases.                                                                                          |
| [`../persistence/runtime-wiring.md`](../persistence/runtime-wiring.md) | Phase 4.5 backend DB env, pgx connector behavior, readiness matrix, migration CLI flow, and rollback boundaries.                                                                  |

## Current Boundary

- Compose assets are isolated under `mm-chat/`; do not overwrite the
  repository-root deployment files.
- Runtime data and local backups belong under `mm-chat/data/` and
  `mm-chat/backup/`, both gitignored.
- MinIO must remain private; the Go backend is the public file authorization
  gateway. Runtime config uses `STORAGE_BACKEND=minio|s3` plus `S3_*`
  variables; do not use stale `OBJECTSTORE_DRIVER` / `FILE_MAX_BYTES` names.
- MVP is `frontend -> Go backend -> Postgres -> provider stream`. Redis is now
  available for non-authoritative session-cache snapshots, temporary
  cancellation flags, HTTP rate-limit counters, and optional auth cache.
  Runtime auth endpoints are implemented; RAG remains a later phase.
- Phase 14 runtime readiness keeps disabled dependencies out of `/ready`. When
  configured, `/ready` checks Postgres, Redis, and storage with additive
  `checks` detail and returns `503` with `status=not_ready` if any configured
  dependency fails. Readiness checks must not run migrations or create buckets.
- Phase 14 metrics are exposed by the Go API at `GET /metrics` in Prometheus
  text format. Keep the endpoint on localhost or behind a reverse-proxy
  allowlist. HTTP metrics use bounded route labels, dependency gauges mirror
  configured `/ready` checks, and the single-server MinIO state is represented
  by the `storage` dependency gauge rather than direct MinIO admin scraping.
- API startup must not auto-run migrations; operators run the `migrate` service
  or `mm-chat-migrate` before starting or restarting a DB-enabled backend
  release.
- Single-server Compose secrets belong in `mm-chat/.env.single-server`, which is
  ignored by Git. Copy `mm-chat/.env.single-server.example` and replace every
  `change-me` value before production. Direct local `go run` development can
  still use `mm-chat/backend/.env` from `mm-chat/backend/.env.example`; load it
  with `set -a; . ./mm-chat/backend/.env; set +a`. The Go API reads process
  environment variables; it does not auto-load `.env`, commit provider API keys,
  or print provider API keys.
- The current real provider path is OpenAI-compatible streaming:
  `PROVIDER_TYPE=openai_compatible`,
  `PROVIDER_BASE_URL=<your OpenAI-compatible relay /v1 URL>`,
  `PROVIDER_MODEL=gpt-5.5`, and a local-only `PROVIDER_API_KEY`.
- The current real object-store path supports `STORAGE_BACKEND=local`,
  `minio`, or `s3`. Use `S3_BUCKET_AUTO_CREATE=false` in production and
  provision the bucket/credentials outside the app release.
- Redis config uses `REDIS_URL`, `REDIS_KEY_PREFIX`, `REDIS_RUN_CANCEL_TTL`,
  `REDIS_SESSION_CACHE_TTL`, `REDIS_RATE_LIMIT_ENABLED`,
  `REDIS_RATE_LIMIT_REQUESTS`, and
  `REDIS_RATE_LIMIT_WINDOW`. Leave `REDIS_URL` empty to disable Redis; if set,
  the API fails fast when Redis cannot be parsed or pinged.
