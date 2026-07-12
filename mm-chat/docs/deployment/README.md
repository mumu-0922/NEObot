# Deployment Docs

Deployment docs define the single-server path for the `mm-chat` server-backed
refactor. The current runtime path is Docker Compose under `mm-chat/`: Go API,
Postgres, Redis, and private MinIO on one server, with migrations and backups
run explicitly by operators. The Go/Postgres runtime now includes Identity,
Teams, Knowledge Collections/Documents, Governance, Consent, Jobs, and durable
Outbox state.

## Documents

| Guide                                                                  | Purpose                                                                                                                                                                  |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [`single-server-compose.md`](./single-server-compose.md)               | Phase 10 Docker Compose topology, first boot, reverse proxy boundary, release, and rollback checklist.                                                                   |
| [`postgres-single-server.md`](./postgres-single-server.md)             | Current Compose/Go Postgres runtime covering private ports, data directories, DB-aware health checks, migration head `009`, backup/restore, image fencing, and rollback. |
| [`redis-temporary-state.md`](./redis-temporary-state.md)               | Phase 7 Redis runbook for non-authoritative temporary state, stream cancellation flags, private-network rules, and flush behavior.                                       |
| [`backup-restore.md`](./backup-restore.md)                             | Backup scripts, checksum verification, Postgres restore drill, MinIO restore drill, retention, and destructive-restore warnings.                                         |
| [`reverse-proxy-tls.md`](./reverse-proxy-tls.md)                       | Phase 14 reverse proxy and TLS edge runbook, including same-origin `/mm-api`, SSE buffering, upload limits, metrics exposure, and rollback.                              |
| [`secret-rotation.md`](./secret-rotation.md)                           | Identity/recovery/SMTP/session rotation procedures plus provider keys, Postgres, Redis, MinIO app/root credentials, and TLS certificates.                                |
| [`release-rollback.md`](./release-rollback.md)                         | Compact release and rollback runbook for image deploys, migrations, and failed releases.                                                                                 |
| [`../persistence/runtime-wiring.md`](../persistence/runtime-wiring.md) | Phase 4.5 backend DB env, pgx connector behavior, readiness matrix, migration CLI flow, and rollback boundaries.                                                         |

## Current Boundary

- Compose assets are isolated under `mm-chat/`; do not overwrite the
  repository-root deployment files.
- Runtime data and local backups belong under `mm-chat/data/` and
  `mm-chat/backup/`, both gitignored.
- MinIO must remain private; the Go backend is the public file authorization
  gateway. Runtime config uses `STORAGE_BACKEND=minio|s3` plus `S3_*`
  variables; do not use stale `OBJECTSTORE_DRIVER` / `FILE_MAX_BYTES` names.
- The current server path is `frontend -> Go backend -> Postgres/provider`, with
  Redis for non-authoritative session hints, temporary cancellation flags, and
  HTTP rate-limit counters, plus MinIO for private object bytes. Identity,
  Teams, Knowledge control-plane CRUD, immutable Document Versions,
  Governance, Consent, Jobs, and Outbox producers are implemented. Search/RAG
  consumers remain outside this Compose runtime. Every bearer authorization
  rechecks Postgres; Redis never participates in the final authorization
  decision.
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
  release. The current migration head is `009`.
- Compose resolves `backend`, `migrate`, and `admin` from one `BACKEND_IMAGE`.
  Production uses a full registry `@sha256:` digest, runs
  `scripts/compose-single-server-production.sh` so host variables cannot
  override the validated env file and the production override removes every
  `build:` path. Retain the previous image ID/digest through rollback.
- The Consent expiry worker starts with every Postgres-backed API process using
  batch `100` and idle poll `1s`. `consent_expiry_worker_failed` is a paging
  signal: the API shuts down and exits non-zero rather than serving with expiry
  materialization stopped.
- Single-server Compose secrets belong in `mm-chat/.env.single-server`, which is
  ignored by Git. Copy `mm-chat/.env.single-server.example` and replace every
  placeholder before production. Direct local `go run` development can
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
