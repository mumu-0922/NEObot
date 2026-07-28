# Single-Server Postgres Runtime

This document describes the current Postgres runtime for the `mm-chat`
single-server deployment. The implementation is
`mm-chat/compose.single-server.yml`: the Go API, PostgreSQL 17, Redis, MinIO,
and the RAG worker run on private Compose networks, while only the API is
published on `127.0.0.1:8080`. The database runtime is PostgreSQL `17.10` with
pgvector `0.8.5` and pg_textsearch `1.3.1`.

It complements [`single-server-compose.md`](./single-server-compose.md),
[`backup-restore.md`](./backup-restore.md), and
[`../persistence/runtime-wiring.md`](../persistence/runtime-wiring.md). The
repository-root `docker-compose.yml` remains outside this deployment path.

## 1. Current Scope

Postgres is the canonical structured store for Identity, Sessions, Teams,
chat, file metadata, imports, Knowledge Collections/Documents/Versions,
Governance, Consent, processing Jobs, and durable Outbox rows. The current
runtime includes:

- the `postgres` service with live data under `mm-chat/data/postgres17/`;
- the Go `backend` using pgx-backed repositories and DB readiness;
- the one-shot `migrate` service using embedded SQL migrations;
- the one-shot `admin` service for supported identity and Governance commands;
- the `rag-worker` profile for the independent indexing runtime;
- the one-shot `rag-replay` profile with a separate operator-only DB login;
- logical backup and restore scripts under `mm-chat/scripts/`.

API startup never applies migrations. Operators must run the migration service
before starting a release that requires a newer schema.

## 2. Network and Port Policy

Postgres has no host `ports:` mapping. `backend`, `memory-worker`, `migrate`, and `admin` reach
it as `postgres:5432` on the private Compose network.

```text
public browser
  -> TLS reverse proxy
    -> 127.0.0.1:8080 Go API
      -> postgres:5432 on the private Compose network
    rag-worker -> postgres:5432 on the internal rag-private network
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
mm-chat/data/postgres17/             # live PostgreSQL 17 PGDATA, gitignored
mm-chat/data/postgres/               # retired PostgreSQL 16 rollback anchor
mm-chat/backup/postgres/             # logical dumps, gitignored
mm-chat/.env.single-server.example   # committed template
mm-chat/.env.single-server           # local production values, gitignored
```

The current Compose/runtime contract uses:

| Variable                  | Compose default           | Purpose                                                                                 |
| ------------------------- | ------------------------- | --------------------------------------------------------------------------------------- |
| `POSTGRES_IMAGE`          | reviewed local PG17 image | PostgreSQL 17 plus exact retrieval extensions; immutable digest required in production. |
| `POSTGRES_DATA_DIR`       | `./data/postgres17`       | Fresh PG17 data path; production preflight rejects `./data/postgres`.                   |
| `POSTGRES_DB`             | `neo_chat`                | Database created by the Postgres container.                                             |
| `POSTGRES_USER`           | `neo_chat_migrator`       | Empty-volume bootstrap and migration login only.                                        |
| `POSTGRES_PASSWORD`       | placeholder               | Bootstrap/migrator password; replace before promotion.                                  |
| `MIGRATION_DATABASE_URL`  | migrator placeholder URL  | Required one-shot URL for `POSTGRES_USER`; no fallback.                                 |
| `DATABASE_URL`            | API placeholder URL       | `neo_chat_api` URL for the Go API and `admin`.                                          |
| `MEMORY_WORKER_DATABASE_URL` | Memory Worker placeholder URL | Login inheriting only `memory_worker_runtime`.                                      |
| `DB_MAX_OPEN_CONNS`       | `10`                      | Maximum open DB connections.                                                            |
| `DB_MAX_IDLE_CONNS`       | `5`                       | Maximum idle DB connections.                                                            |
| `DB_CONN_MAX_LIFETIME`    | `30m`                     | Maximum connection lifetime.                                                            |
| `RAG_WORKER_DATABASE_URL` | Worker placeholder URL    | Long-running least-privilege Worker login.                                              |
| `RAG_REPLAY_DATABASE_URL` | Replay placeholder URL    | Operator-only Replay login; not injected into the Worker.                               |

Keep `MIGRATION_DATABASE_URL` aligned with `POSTGRES_USER`,
`POSTGRES_PASSWORD`, and `POSTGRES_DB`. The four runtime URLs use the same
database but must not reuse that login or password.
`sslmode=disable` is acceptable only on this single-host private Docker
network; use TLS whenever the DB connection crosses hosts or an untrusted
network. Never print the URL or password in validation output.

### Database principal boundary

| Route     | Secret variable           | LOGIN requirement                          | Capability            |
| --------- | ------------------------- | ------------------------------------------ | --------------------- |
| Migration | `MIGRATION_DATABASE_URL`  | Same bootstrap/migrator as `POSTGRES_USER` | Schema migration only |
| API/admin | `DATABASE_URL`            | `NOSUPERUSER NOCREATEROLE` runtime login   | `go_api_runtime`      |
| Memory Worker | `MEMORY_WORKER_DATABASE_URL` | Dedicated long-running login          | `memory_worker_runtime` |
| Worker    | `RAG_WORKER_DATABASE_URL` | Dedicated long-running login               | `rag_worker_executor` |
| Replay    | `RAG_REPLAY_DATABASE_URL` | Dedicated operator-only, one-shot login    | `rag_replay_operator` |

All five routes require pairwise-distinct login names and passwords.
`MIGRATION_DATABASE_URL` is independently required by `migrate`; an unset or
invalid value fails the command and never falls back to `DATABASE_URL`.
`backend` and the one-shot `admin` share the API runtime URL. Neither receives
the bootstrap/migrator credential.

Migrations `010` and `054` define the capability roles as NOLOGIN roles. LOGIN principals
inherit only the matching capability shown above. Do not grant the Worker the
Replay role. Do not grant `go_api_runtime` to `POSTGRES_USER`,
`rag_projection_owner`, or any owner/migrator role, and do not make the API
LOGIN a member of an owner/migrator role. Production preflight validates URL
shape, target database/host, principal and password separation, secret-file
ownership/mode, and immutable image digests without echoing credentials.

### Release image fence

Compose resolves `backend`, `memory-worker`, `migrate`, and `admin` from the same
`BACKEND_IMAGE`. The RAG profile independently resolves `RAG_IMAGE`, and the
database resolves `POSTGRES_IMAGE`. Production requires full registry
`@sha256:` digests for all release images; mutable tags are allowed only for
local development and cannot pass production preflight. Local Compose can
build the PostgreSQL image from `mm-chat/postgres`, while the production
overlay removes that and every other release `build:` path.

Before every production migration or restart:

```bash
cd mm-chat
./scripts/preflight-single-server.sh .env.single-server
./scripts/compose-single-server-production.sh .env.single-server \
  --profile app --profile ops config --quiet
```

The preflight validates required production settings without printing their
values. The production wrapper starts Compose with a clean host environment and
an override that removes `build:` from PostgreSQL, backend/migrate/admin,
frontend, and RAG services. Retain the previous PostgreSQL and application
image IDs or registry digests through the rollback window; do not prune them
after deploying the new release.

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
in `schema_migrations`. The current schema head is `054`. Migration `038`
requires PostgreSQL major `17`, the `pg_textsearch` preload, and exact pgvector
`0.8.5` / pg_textsearch `1.3.1` extension versions. Migrations `039` and `040`
retain the dedicated API role by exposing only hardened document-lifecycle and
source-metadata function calls; they do not grant direct projection-table
access. Migration `041` pins every current-schema SECURITY DEFINER function to
the application schema, `pg_catalog`, and `pg_temp` without changing ownership
or grants. Migrations `051` and `052` add the bounded SiliconFlow TTS cache
authority. Migration `053` adds the inactive Memory v2 Project/scope/settings
foundation. Migration `054` adds the ID-only durable Memory capture outbox,
lease-fenced worker capabilities, and guarded rollback; it does not switch the
Memory reader or add Project API/UI.

Apply migrations from the same immutable `BACKEND_IMAGE` used by `backend` and
`admin`:

```bash
cd mm-chat

./scripts/preflight-single-server.sh .env.single-server

./scripts/compose-single-server-production.sh .env.single-server \
  --profile ops run --rm migrate
```

The command above is valid for a fresh database with no Governance rows. An
existing schema at `009` must instead supply the reviewed, credential-free
Phase 15 Governance Mapping as a read-only one-shot mount. Do not copy the
mapping into the image or keep it in the runtime environment:

```bash
mapping_file="$(realpath ./release/phase15-governance-map.json)"
test -f "${mapping_file}"

./scripts/compose-single-server-production.sh .env.single-server \
  --profile ops run --rm \
  --volume "${mapping_file}:/run/mm-chat/phase15-governance-map.json:ro" \
  migrate /usr/local/bin/mm-chat-migrate up \
  --phase15-governance-map=/run/mm-chat/phase15-governance-map.json
unset mapping_file
```

The file must cover every existing Profile and Head exactly. Migration `010`
recomputes each `profileContractHash` from the locked schema-`009` row plus the
mapped `modelId`; missing, extra, duplicate, ambiguous, or hash-mismatched
evidence rolls back the complete migration. The runner never logs or persists
the mapping.

Inspect migration state without placing credentials in argv or output:

```bash
./scripts/compose-single-server-production.sh .env.single-server \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_DB:?POSTGRES_DB is required}"
exec psql --set=ON_ERROR_STOP=1 \
  --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" \
  --command="SELECT version, name FROM schema_migrations ORDER BY version::INTEGER;"
'
```

Acceptance requires versions `001` through `041`, ending at
`041_security_definer_search_path_hardening`. Treat `schema_migrations` as runner state,
not a domain table. Never use `baseline` routinely; it exists only to accept
reviewed legacy rows that lack checksums.

### Fresh-install role provisioning

On an empty volume, the official Postgres entrypoint creates only
`POSTGRES_USER`. Treat it as the bootstrap/migrator route. The safe first-install
order is:

1. Start Postgres with the bootstrap/migrator fields and set the independently
   required `MIGRATION_DATABASE_URL` to that same login.
2. Run migrations through `054`. Migrations `010` and `054` create and validate
   the NOLOGIN capability roles; do not pre-create LOGIN roles with broad grants.
3. Connect as `POSTGRES_USER`, create the API, Memory Worker, RAG Worker, and Replay principals as
   NOLOGIN, assign each password through interactive `psql` input, then enable
   LOGIN and grant exactly one matching capability.
4. Store the four runtime URLs in the protected env file, run preflight, and
   perform the live verification below before starting API or either Worker or Replay.

The following `psql` input contains login names but no password literals.
Choose deployment-specific names. `\password` prompts twice without echo and
does not place the secret in argv, environment variables, SQL history, or the
server log:

```bash
cd mm-chat
./scripts/compose-single-server-production.sh .env.single-server \
  exec postgres sh -ceu '
exec psql --set=ON_ERROR_STOP=1 \
  --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"
'
```

```psql
\set api_login 'neo_chat_api'
\set memory_worker_login 'memory_worker'
\set worker_login 'rag_worker'
\set replay_login 'rag_replay'

SELECT format(
  'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT NOREPLICATION NOBYPASSRLS',
  :'memory_worker_login'
) \gexec
SELECT format(
  'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT NOREPLICATION NOBYPASSRLS',
  :'api_login'
) \gexec
SELECT format(
  'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT NOREPLICATION NOBYPASSRLS',
  :'worker_login'
) \gexec
SELECT format(
  'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT NOREPLICATION NOBYPASSRLS',
  :'replay_login'
) \gexec

\password :api_login
\password :memory_worker_login
\password :worker_login
\password :replay_login

SELECT format('ALTER ROLE %I LOGIN', :'api_login') \gexec
SELECT format('ALTER ROLE %I LOGIN', :'memory_worker_login') \gexec
SELECT format('ALTER ROLE %I LOGIN', :'worker_login') \gexec
SELECT format('ALTER ROLE %I LOGIN', :'replay_login') \gexec
SELECT format('GRANT go_api_runtime TO %I', :'api_login') \gexec
SELECT format('GRANT memory_worker_runtime TO %I', :'memory_worker_login') \gexec
SELECT format('GRANT rag_worker_executor TO %I', :'worker_login') \gexec
SELECT format('GRANT rag_replay_operator TO %I', :'replay_login') \gexec
```

Run this only after the complete migration set succeeds. If any step fails,
leave the affected principal NOLOGIN, correct the cause, and rerun only the
missing safe step; do not compensate by granting owner or migrator membership.

### Live role verification

Inspect attributes and direct memberships from the live database. These
queries deliberately omit `pg_authid.rolpassword`:

```psql
\set migrator_login 'neo_chat_migrator'
\set api_login 'neo_chat_api'
\set memory_worker_login 'memory_worker'
\set worker_login 'rag_worker'
\set replay_login 'rag_replay'

SELECT rolname, rolcanlogin, rolsuper, rolcreatedb, rolcreaterole,
       rolreplication, rolbypassrls
FROM pg_roles
WHERE rolname IN (
  :'migrator_login', :'api_login', :'memory_worker_login', :'worker_login', :'replay_login',
  'go_api_runtime', 'rag_projection_owner',
  'memory_runtime_owner', 'memory_worker_runtime',
  'rag_worker_executor', 'rag_replay_operator'
)
ORDER BY rolname;

SELECT member.rolname AS login_name, granted.rolname AS capability
FROM pg_auth_members AS membership
JOIN pg_roles AS member ON member.oid = membership.member
JOIN pg_roles AS granted ON granted.oid = membership.roleid
WHERE member.rolname IN (:'api_login', :'memory_worker_login', :'worker_login', :'replay_login')
ORDER BY member.rolname, granted.rolname;
```

Expected results are exactly the four LOGIN-to-capability edges documented in
the table. Runtime LOGIN roles are neither superusers nor role creators;
capability and owner roles are NOLOGIN. The migrator must not inherit
`go_api_runtime`, and the API login must not inherit an owner/migrator role.

Finally, test each of the five login names separately with `psql --password`.
This template prompts without echo; repeat it for migrator, API, Memory Worker,
RAG Worker, and Replay, changing only the non-secret login name:

```bash
./scripts/compose-single-server-production.sh .env.single-server \
  exec postgres psql --host=postgres --dbname=neo_chat \
  --username="<login-name>" --password \
  --command='SELECT current_user;'
```

Do not put a DSN or `PGPASSWORD` on the command line. Each result must equal the
route's expected login name before promotion.

Down migrations are destructive and are not the normal production rollback.
After live Knowledge writes, prefer a forward fix or a verified pre-migration
restore rather than dropping authoritative Documents, Consent history, Jobs,
or Outbox events.

The guarded `010.down` removes only the API grants introduced by `010`. It
retains `go_api_runtime` and the capability needed by the rolled-back API at
schema `009`, so the API login remains least-privilege after application
rollback. Never work around a down migration by moving API grants onto the
bootstrap/migrator login or `rag_projection_owner`.

## 6. Backup and Restore

Use the committed scripts rather than ad hoc `docker exec pg_dump` commands:

```bash
cd /path/to/mm-chat

./scripts/backup-single-server-production.sh .env.single-server
```

Postgres dumps and MinIO archives should come from the same maintenance window.
Keep each checksum, release identifier, migration head, and encrypted secret
backup with the recovery record.

The executable temporary-database drill and full restore acceptance are in
[`backup-restore.md`](./backup-restore.md). Acceptance verifies migrations
through `041`, retrieval profile/readiness, Knowledge core table row counts,
Consent expiry schema, Governance immutability, the purge fence, and sampled
Document Version/File/object consistency. A production restore is not approved
until that disposable drill passes.

## 7. Rollback and Operational Boundaries

- Retain the previous immutable backend image ID/digest and a verified
  pre-migration Postgres dump.
- If only API code fails and the schema remains compatible, recreate `backend`
  from the retained previous image.
- If schema/data must be restored, stop backend writes and follow the verified
  restore runbook during a maintenance window.
- Preserve failed-release data for diagnosis; never casually remove either
  `mm-chat/data/postgres17/` or the retired `mm-chat/data/postgres/` rollback
  anchor.
- Postgres stores file metadata and internal object keys; MinIO stores object
  bytes. Restore and verify both sides together.
- Redis remains non-authoritative temporary state and cannot replace Postgres
  authorization or persistence decisions.
- A PostgreSQL-major rollback is blue-green: stop writers, restore the retained
  previous Compose/env authority, and start PG16 only against the preserved
  `data/postgres` directory or a fresh verified PG16 restore. Never mount that
  directory into the PG17 image, and never treat `038.down` as an in-place
  downgrade.
- Stop the `rag-worker` profile to halt new indexing while retaining
  authoritative Go/Postgres control-plane records. Do not compensate for a
  retrieval failure by widening runtime database privileges.
