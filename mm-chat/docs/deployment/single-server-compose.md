# Single-Server Compose Deployment Draft

This document is the Phase 3 deployment draft for the `mm-chat` refactor. It
defines where the single-server Docker Compose topology should live, how the Go
backend skeleton should be run, and what operational boundaries must be kept
when later phases add Postgres, Redis, and MinIO.

No Compose implementation file is created in Phase 3. This is a contract for a
later implementation pass.

## 1. Scope and Phase Boundary

### Phase 3: Go backend skeleton

Phase 3 owns only the new backend skeleton and smoke-test runtime path:

```text
mm-chat/backend/
  go.mod
  cmd/api/main.go
  internal/config/
  internal/health/
  internal/httpserver/
```

Expected Phase 3 endpoints:

```text
GET /health
GET /ready
GET /v1/version
```

Phase 3 does **not** migrate current frontend traffic, does **not** add durable
chat persistence, and does **not** expose file storage.

### MVP: server-backed chat spine

The first shippable server-backed MVP is:

```text
Next.js frontend -> Go API backend -> Postgres
                              -> model provider streaming
```

MVP includes durable conversations/messages and provider streaming through the
backend. MVP excludes Redis hardening, MinIO file storage, browser OPFS import,
RAG sidecars, multi-server deployment, and Kubernetes.

### Later phases

| Phase | Service | Boundary |
| --- | --- | --- |
| Phase 4 | Postgres | Source of truth for users, sessions, conversations, messages, file metadata, audit logs. |
| Phase 6 | MinIO | Private object store for file bytes only; browser never talks to it directly. |
| Phase 7 | Redis | Non-authoritative temporary state: rate limits, session cache, cancellation flags, short jobs. |
| Phase 9+ | RAG service | Optional internal sidecar; core chat must work without it. |
| Phase 10 | Ops | Backups, restore drills, reverse proxy, rollback runbooks. |

## 2. Compose Location

Keep refactor deployment assets isolated from the repository-root deployment
files. The later Compose implementation should live under `mm-chat/`, not at the
current repository root.

Recommended future path:

```text
mm-chat/compose.single-server.yml
mm-chat/.env.single-server.example
mm-chat/data/                 # gitignored runtime data, never committed
mm-chat/backup/               # local backup landing zone, never committed
```

Operational commands should be run from `mm-chat/` so relative paths such as
`./data/postgres` resolve inside the refactor workspace:

```bash
cd mm-chat
# future phase only; no Compose file exists in Phase 3
# docker compose -f compose.single-server.yml up -d
```

Do not overwrite the repository-root `docker-compose.yml`; it belongs to the
existing app deployment surface.

## 3. Phase 3 Single-Server Runbook

Until Compose exists, run the Go skeleton directly on the server or developer
host.

```bash
cd mm-chat/backend

go test ./...
go run ./cmd/api
```

Smoke tests:

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
curl -fsS http://127.0.0.1:8080/v1/version
```

Frontend server mode remains opt-in during Phase 3:

```env
NEXT_PUBLIC_API_MODE=server
NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:8080
```

Rollback is switching the frontend back to local mode and stopping the backend:

```env
NEXT_PUBLIC_API_MODE=local
```

## 4. Target Topology Draft

### Phase 3 skeleton topology

```text
public/dev client
  -> Go backend :8080
```

No Postgres, Redis, or MinIO dependency should be required for `/health`,
`/ready`, or `/v1/version` in the earliest skeleton. In Phase 4.5, Postgres is
still optional when `DATABASE_URL` is empty; when `DATABASE_URL` is set, `/ready`
becomes DB-aware and returns `503` on DB ping failure.

### MVP topology

```text
public browser
  -> reverse proxy :80/:443
    -> frontend :3000
    -> backend :8080
      -> postgres :5432  # internal only
```

### Later full single-server topology

```text
public browser
  -> reverse proxy :80/:443
    -> frontend :3000
    -> backend :8080
      -> postgres :5432     # internal only
      -> redis :6379        # internal only
      -> minio :9000        # internal only
      -> rag-service :8000  # optional, internal only

admin operator
  -> SSH tunnel/VPN only
    -> minio console :9001
```

## 5. Port Policy

| Component | Port | Exposure | Notes |
| --- | ---: | --- | --- |
| Reverse proxy | 80, 443 | Public | Only public entry in production. Terminates TLS. |
| Frontend | 3000 | Internal or localhost | Public access should normally flow through the reverse proxy. |
| Go backend | 8080 | Internal; dev may bind localhost | Public API path should be reverse-proxied, not wide-open. |
| Postgres | 5432 | Internal only | Never publish to the public internet. |
| Redis | 6379 | Internal only | Temporary state only; require auth if exposed inside shared networks. |
| MinIO API | 9000 | Internal only | Backend is the only file gateway. |
| MinIO console | 9001 | Private admin only | Use SSH tunnel/VPN; do not expose publicly. |
| RAG service | 8000 | Internal only | Optional later sidecar. |

Production firewall baseline: allow only `80/tcp`, `443/tcp`, and `22/tcp` from
trusted admin networks. Every data service should stay on the private Compose
network.

## 6. Environment Variable Draft

Use a dedicated environment file for the single-server deployment, for example
`mm-chat/.env.single-server`. Do not commit real secrets.

### Frontend

```env
NEXT_PUBLIC_API_MODE=server
NEXT_PUBLIC_API_BASE_URL=https://chat.example.com/api
```

### Go backend

```env
APP_ENV=production
MM_CHAT_ADDR=:8080
MM_CHAT_VERSION=<release-or-commit>
PUBLIC_BASE_URL=https://chat.example.com
CORS_ALLOWED_ORIGINS=https://chat.example.com
SESSION_SECRET=<secret>
DATABASE_URL=postgres://<user>:<password>@postgres:5432/<db>?sslmode=disable
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=30m
REDIS_URL=redis://:<password>@redis:6379/0
OBJECTSTORE_DRIVER=s3
S3_ENDPOINT=http://minio:9000
S3_BUCKET=neo-chat-files
S3_REGION=us-east-1
S3_ACCESS_KEY_ID=<secret>
S3_SECRET_ACCESS_KEY=<secret>
S3_FORCE_PATH_STYLE=true
FILE_MAX_BYTES=52428800
RAG_SERVICE_URL=http://rag-service:8000
```

### Postgres

```env
POSTGRES_DB=neo_chat
POSTGRES_USER=neo_chat
POSTGRES_PASSWORD=<secret>
```

### Redis

```env
REDIS_PASSWORD=<secret>
```

### MinIO

```env
MINIO_ROOT_USER=<secret>
MINIO_ROOT_PASSWORD=<secret>
MINIO_DEFAULT_BUCKETS=neo-chat-files
```

### Backup job

```env
BACKUP_DIR=./backup
BACKUP_RETENTION_DAYS=14
```

## 7. Data Directories

All mutable server data should live under `mm-chat/data/` when the Compose file
is run from `mm-chat/`:

```text
mm-chat/data/postgres/   # Postgres data volume
mm-chat/data/redis/      # optional Redis persistence for restart convenience
mm-chat/data/minio/      # object storage data
mm-chat/backup/          # generated backup archives and restore staging
```

Rules:

- `data/` and `backup/` are runtime artifacts and must not be committed.
- Postgres remains the source of truth for structured records.
- MinIO stores file bytes; Postgres stores ownership, metadata, hash, size, MIME,
  and object key.
- Redis must not contain canonical conversations, messages, or files.

## 8. Backup Boundary

Back up these artifacts together:

1. Postgres logical dump, including the migration-runner metadata table
   `schema_migrations`.
2. MinIO bucket/object data for uploaded files and knowledge sources.
3. Encrypted deployment environment/secrets backup.
4. Release identifier or container image tags used during the backup window.

Do not treat these as canonical backups:

- Redis cache data. It may be snapshotted for restart convenience, but the app
  must survive a Redis flush except for active temporary jobs.
- Build output, `node_modules`, Go build cache, or container layers. Rebuild them
  from source/image tags.
- Browser IndexedDB/OPFS data before the explicit import phase. That data still
  belongs to the user agent, not the server backup set.

Consistency rule: Postgres metadata and MinIO objects must be restorable as a
pair. Prefer maintenance windows, filesystem snapshots, or an ordered backup
that records the application release and migration-runner state.

Minimum restore drill:

```text
fresh server
  -> restore env/secrets
  -> restore Postgres
  -> restore MinIO data
  -> start services
  -> run /health and DB-aware /ready when DATABASE_URL is set
  -> upload/download test file
  -> verify historical conversation read
```

## 9. MinIO Security Boundary

MinIO must not be exposed to the public internet.

Required file-access shape:

```text
browser
  -> Go backend file API
    -> auth/session check
    -> ownership and permission check
    -> size/MIME/hash policy
    -> MinIO private endpoint
```

Rules:

- Browser responses return `file_id` or backend download URLs, never MinIO bucket
  names, object keys, or public object-store URLs.
- Object keys are generated server-side, preferably UUID/path based, never raw
  user filenames.
- Downloads stream through `GET /v1/files/:id/content` or equivalent backend
  endpoints after permission checks.
- Uploads go through backend validation before writing to MinIO.
- MinIO console access is admin-only through SSH tunnel/VPN, not a public DNS
  record.

## 10. MVP vs Later Stage Checklist

MVP accepts:

- Go backend health/readiness/version endpoints with `MM_CHAT_ADDR` and `MM_CHAT_VERSION`.
- Server mode behind `NEXT_PUBLIC_API_MODE=server`.
- Postgres-backed conversations and messages once Phase 4 lands.
- Provider streaming proxied through the backend.
- Rollback to `NEXT_PUBLIC_API_MODE=local` while local-first mode remains.

MVP rejects:

- Direct MinIO browser access.
- Redis as a source of truth.
- Automatic browser OPFS/IndexedDB upload without user action.
- RAG dependency for core chat.
- Public database, Redis, or MinIO ports.
- Kubernetes or multi-server assumptions before the single-server path works.

Later phases may add:

- MinIO-backed file upload/download through the backend gateway.
- Redis-backed rate limits, cancellation flags, and temporary job state.
- Optional Python FastAPI RAG sidecar.
- Backup automation and monthly restore drills.
- Reverse-proxy hardening, TLS automation, metrics, and alerts.
