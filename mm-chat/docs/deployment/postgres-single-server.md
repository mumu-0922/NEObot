# Single-Server Postgres Runtime

This document describes the current Postgres runtime for the `mm-chat`
single-server deployment. The implementation is
`mm-chat/compose.single-server.yml`: the Go API, Postgres 16, Redis, and MinIO
run on one private Compose network, while only the API is published on
`127.0.0.1:8080`.

It complements [`single-server-compose.md`](./single-server-compose.md),
[`backup-restore.md`](./backup-restore.md), and
[`../persistence/runtime-wiring.md`](../persistence/runtime-wiring.md). The
repository-root `docker-compose.yml` remains outside this deployment path.

## 1. Current Scope

Postgres is the canonical structured store for Identity, Sessions, Teams,
chat, file metadata, imports, Knowledge Collections/Documents/Versions,
Governance, Consent, processing Jobs, and durable Outbox rows. The current
runtime includes:

- the `postgres` service with data under `mm-chat/data/postgres/`;
- the Go `backend` using pgx-backed repositories and DB readiness;
- the one-shot `migrate` service using embedded SQL migrations;
- the one-shot `admin` service for supported identity and Governance commands;
- logical backup and restore scripts under `mm-chat/scripts/`.

API startup never applies migrations. Operators must run the migration service
before starting a release that requires a newer schema.

## 2. Network and Port Policy

Postgres has no host `ports:` mapping. `backend`, `migrate`, and `admin` reach
it as `postgres:5432` on the private Compose network.

```text
public browser
  -> TLS reverse proxy
    -> 127.0.0.1:8080 Go API
      -> postgres:5432 on the private Compose network
```

Rules:

- Never publish `5432/tcp` to `0.0.0.0`.
- Use the production Compose wrapper's `exec postgres`, or a separately
  reviewed SSH/VPN admin path, for operator access.
- Keep the production firewall limited to public `80/tcp`, `443/tcp`, and
  trusted-admin `22/tcp`.

Start Postgres through the committed Compose topology:

```bash
cd mm-chat

./scripts/compose-single-server-production.sh .env.single-server \
  up -d postgres
```

Do not create a parallel manually named Postgres container; it bypasses the
Compose health, network, volume, and release assumptions in this runbook.

## 3. Data and Configuration

```text
mm-chat/data/postgres/               # live PGDATA, gitignored
mm-chat/backup/postgres/             # logical dumps, gitignored
mm-chat/.env.single-server.example   # committed template
mm-chat/.env.single-server           # local production values, gitignored
```

The current Compose/runtime contract uses:

| Variable               | Compose default | Purpose                                      |
| ---------------------- | --------------- | -------------------------------------------- |
| `POSTGRES_DB`          | `neo_chat`      | Database created by the Postgres container.  |
| `POSTGRES_USER`        | `neo_chat`      | Database role used by the stack.             |
| `POSTGRES_PASSWORD`    | placeholder     | Database password; replace before promotion. |
| `DATABASE_URL`         | placeholder URL | Go API, migration, and admin connection URL. |
| `DB_MAX_OPEN_CONNS`    | `10`            | Maximum open DB connections.                 |
| `DB_MAX_IDLE_CONNS`    | `5`             | Maximum idle DB connections.                 |
| `DB_CONN_MAX_LIFETIME` | `30m`           | Maximum connection lifetime.                 |

Keep the URL user, password, and database aligned with the Postgres fields.
`sslmode=disable` is acceptable only on this single-host private Docker
network; use TLS whenever the DB connection crosses hosts or an untrusted
network. Never print the URL or password in validation output.

### Release image fence

Compose resolves `backend`, `migrate`, and `admin` from the same
`BACKEND_IMAGE`. Production must use a full registry `@sha256:` digest so API
code, embedded migrations, and admin commands cannot drift across builds. A
mutable tag is allowed only for local development and cannot pass production
preflight.

Before every production migration or restart:

```bash
cd mm-chat
./scripts/preflight-single-server.sh .env.single-server
./scripts/compose-single-server-production.sh .env.single-server \
  --profile app --profile ops config --quiet
```

The preflight validates required production settings without printing their
values. The production wrapper starts Compose with a clean host environment and
an override that removes `build:` from backend/migrate/admin. Retain the
previous backend image ID or registry digest through the rollback window; do
not prune it after deploying the new image.

## 4. Health and Readiness

The `postgres` healthcheck runs `pg_isready` inside the container. Inspect it
without exposing credentials:

```bash
cd mm-chat

./scripts/compose-single-server-production.sh .env.single-server ps postgres

./scripts/compose-single-server-production.sh .env.single-server \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_DB:?POSTGRES_DB is required}"
pg_isready --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"
'
```

The Compose backend waits for the Postgres healthcheck. The Go API then parses
the DB settings and pings Postgres before serving; a failed open/ping exits
instead of falling back to non-database repositories. While running, `/ready`
includes the `database` check and returns `503` when it fails. `/health` remains
a process-liveness endpoint.

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

Readiness never mutates schema, creates buckets, or runs migrations.

## 5. Migration Execution

The Go migration runner owns transaction boundaries, takes a Postgres advisory
lock, validates migration names/checksums, and records each applied migration
in `schema_migrations`. The current schema head is `009`.

Apply migrations from the same immutable `BACKEND_IMAGE` used by `backend` and
`admin`:

```bash
cd mm-chat

./scripts/preflight-single-server.sh .env.single-server

./scripts/compose-single-server-production.sh .env.single-server \
  --profile ops run --rm migrate
```

Inspect migration state without placing credentials in argv or output:

```bash
./scripts/compose-single-server-production.sh .env.single-server \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_DB:?POSTGRES_DB is required}"
if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi
exec psql --set=ON_ERROR_STOP=1 \
  --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" \
  --command="SELECT version, name FROM schema_migrations ORDER BY version;"
'
```

Acceptance requires versions `001` through `009`, ending at
`009_phase15_consent_expiry_materialization`. Treat `schema_migrations` as
runner state, not a domain table. Never use `baseline` routinely; it exists
only to accept reviewed legacy rows that lack checksums.

Down migrations are destructive and are not the normal production rollback.
After live Knowledge writes, prefer a forward fix or a verified pre-migration
restore rather than dropping authoritative Documents, Consent history, Jobs,
or Outbox events.

## 6. Backup and Restore

Use the committed scripts rather than ad hoc `docker exec pg_dump` commands:

```bash
cd /home/mumu/projects/neo-chat

./mm-chat/scripts/backup-single-server-production.sh \
  mm-chat/.env.single-server
```

Postgres dumps and MinIO archives should come from the same maintenance window.
Keep each checksum, release identifier, migration head, and encrypted secret
backup with the recovery record.

The executable temporary-database drill and full restore acceptance are in
[`backup-restore.md`](./backup-restore.md). Acceptance verifies migrations
through `009`, Knowledge core table row counts, Consent expiry schema,
Governance immutability, the purge fence, and sampled Document
Version/File/object consistency. A production restore is not approved until
that disposable drill passes.

## 7. Rollback and Operational Boundaries

- Retain the previous immutable backend image ID/digest and a verified
  pre-migration Postgres dump.
- If only API code fails and the schema remains compatible, recreate `backend`
  from the retained previous image.
- If schema/data must be restored, stop backend writes and follow the verified
  restore runbook during a maintenance window.
- Preserve failed-release data for diagnosis; never casually remove
  `mm-chat/data/postgres/`.
- Postgres stores file metadata and internal object keys; MinIO stores object
  bytes. Restore and verify both sides together.
- Redis remains non-authoritative temporary state and cannot replace Postgres
  authorization or persistence decisions.
- RAG/search workers may consume Knowledge Jobs/Outbox later, but the current
  Go/Postgres Knowledge control-plane records are already authoritative.
