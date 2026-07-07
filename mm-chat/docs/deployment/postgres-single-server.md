# Single-Server Postgres Container Plan

This document is the Phase 4/4.5 deployment plan for running Postgres on the
`mm-chat` single-server path and wiring the Go backend to it. It complements
[`single-server-compose.md`](./single-server-compose.md) and
[`../persistence/runtime-wiring.md`](../persistence/runtime-wiring.md), and
intentionally does not create a Compose file, root `docker-compose.yml`
replacement, or production repository implementation.

Phase 4/4.5 target:

```text
Next.js frontend -> Go backend -> Postgres container
```

Redis, MinIO, RAG services, reverse-proxy hardening, and backup automation are
later phases unless a later task explicitly changes their boundary.

## 1. Scope and Non-Goals

In scope:

- One Postgres container on the same server as the `mm-chat` backend.
- Private Docker network access from the backend to Postgres.
- Persistent data directory under `mm-chat/data/postgres/`.
- Environment variable draft for Postgres and backend DB access.
- Manual backup/restore, health checks, migration execution, and rollback notes.

Non-goals:

- No committed Compose implementation in this phase.
- No modification of the repository-root `docker-compose.yml`.
- No public Postgres port exposure.
- No MinIO/Redis/RAG deployment in Phase 4.
- No claim that DB repository code has already landed.
- No automatic migration execution during API startup.

## 2. Network and Port Policy

Postgres must not be reachable from the public internet.

Target shape:

```text
public browser
  -> reverse proxy / frontend / backend public edge
    -> backend container or process
      -> postgres:5432 on private Docker network only
```

Rules:

- Do not publish `5432/tcp` to `0.0.0.0`.
- Prefer no `ports:` entry for Postgres in future Compose.
- If local operator access is required, use `docker exec`, an SSH tunnel bound to
  `127.0.0.1`, or a VPN-only admin path.
- Firewall baseline remains only public `80/tcp`, `443/tcp`, and trusted-admin
  `22/tcp` for production-like hosts.

Manual network sketch for development or ops testing:

```bash
cd mm-chat

docker network create mm-chat-internal

docker run -d \
  --name mm-chat-postgres \
  --network mm-chat-internal \
  --restart unless-stopped \
  --env-file ./.env.single-server \
  -v "$PWD/data/postgres:/var/lib/postgresql/data" \
  postgres:16
```

The command above is illustrative for an operator runbook. It is not a committed
Compose implementation and does not expose a host port.

## 3. Data Directories

Run deployment commands from `mm-chat/` so relative paths stay isolated from the
repository root.

```text
mm-chat/data/postgres/          # Postgres PGDATA volume, runtime only
mm-chat/backup/postgres/        # logical dumps and restore staging, runtime only
mm-chat/.env.single-server      # uncommitted secrets/env file
mm-chat/.env.single-server.example  # future sanitized example only
```

Rules:

- `data/` and `backup/` are runtime artifacts and must not be committed.
- Back up Postgres logical dumps together with the application release ID and
  encrypted environment/secrets backup.
- Do not place MinIO data or Redis data in this plan; they are later phases.

## 4. Environment Variable Draft

Use a private environment file such as `mm-chat/.env.single-server`. Do not
commit real values.

### Postgres container

```env
POSTGRES_DB=neo_chat
POSTGRES_USER=neo_chat
POSTGRES_PASSWORD=<replace-with-strong-secret>
PGDATA=/var/lib/postgresql/data/pgdata
```

### Go backend

Existing Phase 3 backend config already uses `MM_CHAT_ADDR` and
`MM_CHAT_VERSION`. Phase 4.5 DB wiring reads `DATABASE_URL` plus DB pool
settings.

```env
APP_ENV=production
MM_CHAT_ADDR=:8080
MM_CHAT_VERSION=<release-or-commit>
DATABASE_URL=postgres://neo_chat:<replace-with-strong-secret>@postgres:5432/neo_chat?sslmode=disable
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=30m
```

Notes:

- `sslmode=disable` is acceptable only for an internal single-server Docker
  network. Use TLS when crossing hosts or untrusted networks.
- The hostname `postgres` assumes a future Compose service name or equivalent
  Docker DNS alias. Manual `docker run` setups must use the actual container
  name/network alias.
- Keep `POSTGRES_PASSWORD` and `DATABASE_URL` aligned through secret management,
  not hard-coded source.

## 5. Health Checks

Container-level readiness should use `pg_isready` inside the Postgres container:

```bash
docker exec mm-chat-postgres pg_isready -U neo_chat -d neo_chat
```

For a future Compose healthcheck, keep the same intent without exposing a port:

```text
pg_isready -U neo_chat -d neo_chat
```

Phase 4.5 application readiness contract:

| Backend DB mode | Startup expectation | `/health` | `/ready` |
| --- | --- | --- | --- |
| `DATABASE_URL` empty | Start with DB disabled; do not require Postgres. | `200` if process is alive. | `200`; DB is intentionally disabled. |
| `DATABASE_URL` set and DB ping succeeds | Create pgx-backed connector and pass `PingContext`. | `200` if process is alive. | `200` while DB ping succeeds. |
| `DATABASE_URL` set and startup DB ping fails | Fail fast before serving HTTP. | Not served. | Not served. |

Readiness must not mutate schema and must not run migrations. Migrations are an
explicit CLI/deployment step before starting or restarting a DB-enabled backend
release.

Operational smoke checks after Postgres starts:

```bash
# from the host, through docker exec; no public port needed
docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c 'select 1;'

# backend process checks
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

## 6. Migration Execution

Migration files are owned by the backend/migration worker. Phase 4.5 deployment
uses the Go migration CLI with embedded SQL; API startup must not auto-migrate.

Runbook draft:

1. Start Postgres on the private network.
2. Wait for `pg_isready` to pass.
3. Point `DATABASE_URL` at the target database from the same release context
   that will run the backend.
4. Run `up` migrations from `mm-chat/backend`.
5. Verify the expected app tables and migration-runner metadata exist.
6. Start or restart the backend with the same `DATABASE_URL`.
7. Confirm `/ready` is `200` while the DB ping succeeds.

Source-run command shape:

```bash
cd mm-chat/backend

export DATABASE_URL='postgres://neo_chat:<replace-with-strong-secret>@postgres:5432/neo_chat?sslmode=disable'

go run ./cmd/migrate up
```

Development/destructive rollback command shape:

```bash
cd mm-chat/backend

export DATABASE_URL='postgres://neo_chat:<replace-with-strong-secret>@postgres:5432/neo_chat?sslmode=disable'

go run ./cmd/migrate down --all
```

Post-migration inspection:

```bash
docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c '\dt'
docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c \
  "select tablename from pg_tables where schemaname = 'public' order by tablename;"

docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c \
  "select * from schema_migrations order by version;"
```

`schema_migrations` is the Phase 4.5 migration-runner metadata table. Treat it
as runner state, not an application/domain table.

## 7. Backup Plan

Use logical dumps for Phase 4 because the dataset is structured and should be
portable across a single-server restore.

Minimum backup contents:

1. Postgres custom-format dump, including the migration-runner metadata table
   (`schema_migrations`).
2. Release identifier or container image tag for the backend using that schema.
3. Encrypted copy of deployment environment/secrets.
4. Backup manifest with timestamp, DB name, migration state, release ID, and
   checksum.

Manual backup command:

```bash
cd mm-chat
mkdir -p backup/postgres

stamp="$(date -u +%Y%m%dT%H%M%SZ)"

docker exec -i mm-chat-postgres pg_dump \
  -U neo_chat \
  -d neo_chat \
  -Fc \
  --no-owner \
  --no-privileges \
  > "backup/postgres/neo_chat_${stamp}.dump"

sha256sum "backup/postgres/neo_chat_${stamp}.dump" \
  > "backup/postgres/neo_chat_${stamp}.dump.sha256"
```

Retention draft:

```text
daily logical dump: keep 14 days
pre-deploy dump: keep at least through the rollback window
monthly restore drill: keep latest successful drill artifact reference
```

## 8. Restore Plan

Restore into a fresh or intentionally reset database. Do not run restore against
a live production DB without a maintenance window and a tested rollback point.

Fresh restore outline:

```bash
cd mm-chat

# 1. Stop backend writes before restoring.
# 2. Start a clean Postgres container/data directory.
# 3. Copy or mount the chosen dump under backup/postgres/.
# 4. Restore.

docker exec -i mm-chat-postgres dropdb -U neo_chat --if-exists neo_chat
docker exec -i mm-chat-postgres createdb -U neo_chat neo_chat

docker exec -i mm-chat-postgres pg_restore \
  -U neo_chat \
  -d neo_chat \
  --clean \
  --if-exists \
  < backup/postgres/<chosen-dump>.dump
```

Restore verification:

```bash
docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c '\dt'
docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c 'select 1;'

# with DATABASE_URL set, DB-aware readiness must ping Postgres
curl -fsS http://127.0.0.1:8080/ready
```

A complete later restore drill should also verify conversation reads and file
metadata consistency. File bytes are not part of Phase 4 and will be paired with
MinIO restore drills in a later phase.

## 9. Rollback Plan

Application rollback:

- Keep the previous backend release image/binary during deployment.
- Take a pre-migration Postgres dump before applying schema changes.
- If the new backend fails, stop it and restart the previous backend release.
- While local-first mode remains available, switch frontend mode back to local:

```env
NEXT_PUBLIC_API_MODE=local
```

Database rollback:

- In development, run `cd mm-chat/backend && go run ./cmd/migrate down --all`
  only when an intentional destructive reset is acceptable.
- In production-like use, prefer restoring the pre-migration dump only during a
  maintenance window, because destructive rollback can lose writes made after
  the migration.
- If migrations are forward-only, document the forward fix and keep DB data for
  later retry rather than deleting user data.

Container rollback:

```bash
# stop only the Postgres container if the whole Phase 4 stack is being backed out
docker stop mm-chat-postgres

# preserve data for inspection/retry unless an operator intentionally removes it
# rm -rf mm-chat/data/postgres  # destructive; do not run casually
```

## 10. Operational Boundaries

- Postgres is the structured source of truth for Phase 4.
- File bytes stay out of Postgres; MinIO is planned later.
- Redis is not required for Phase 4 and must not become canonical storage later.
- RAG services are not required for core chat persistence.
- Future Compose assets should live under `mm-chat/` and must not overwrite the
  repository-root deployment files.
