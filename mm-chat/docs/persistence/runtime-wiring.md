# Phase 4.5 Postgres Runtime Wiring

This document defines the Phase 4.5 runtime contract between the Go backend and
Postgres. It is a documentation contract for the DB connector, pgx driver,
migration runner, and DB-aware readiness path; verify exact package names and
metadata table names against the backend implementation before release.

## 1. Scope

Phase 4.5 wires the existing backend skeleton to Postgres without changing the
Phase 4 application schema.

In scope:

- Backend DB configuration from environment variables.
- Postgres connector using the `pgx` driver.
- Startup connectivity check when DB is enabled.
- DB-aware `/ready` behavior.
- Embedded SQL migrations exposed through a Go migration CLI.
- Operator-facing migration and rollback boundaries.

Out of scope:

- Automatic migrations during API startup.
- DB repository CRUD for conversations/messages/provider configs.
- Compose implementation files.
- MinIO, Redis, RAG, browser import, or multi-server deployment.

## 2. Environment Variables

| Variable               | Required | Meaning                                                                                                             |
| ---------------------- | -------- | ------------------------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`         | No       | Postgres connection string. Empty means DB disabled mode. Non-empty enables DB startup ping and DB-aware readiness. |
| `DB_MAX_OPEN_CONNS`    | No       | Maximum open DB connections. Backend default is code-defined when unset.                                            |
| `DB_MAX_IDLE_CONNS`    | No       | Maximum idle DB connections. Backend default is code-defined when unset.                                            |
| `DB_CONN_MAX_LIFETIME` | No       | Maximum connection lifetime as a Go duration such as `30m`. Backend default is code-defined when unset.             |

Rules:

- Do not log `DATABASE_URL` with credentials. Redact userinfo before logging.
- `sslmode=disable` is acceptable only on a private single-server Docker
  network. Use TLS for cross-host or untrusted networks.
- Invalid or blank pool settings fall back to backend defaults so startup does
  not panic on a partially configured environment.

## 3. Connector Behavior

### `DATABASE_URL` empty

- DB is disabled.
- API startup must not require Postgres.
- `/health` remains process liveness.
- `/ready` remains `200 OK` for the skeleton/runtime without DB dependencies.
- DB-backed product endpoints, when later added, must fail explicitly instead of
  pretending durable persistence exists.

### `DATABASE_URL` non-empty

- Backend creates a Postgres connector using `github.com/jackc/pgx/v5` through
  the `database/sql` stdlib adapter.
- Backend applies pool settings from `DB_MAX_OPEN_CONNS`,
  `DB_MAX_IDLE_CONNS`, and `DB_CONN_MAX_LIFETIME` when present.
- API startup opens the DB and runs `PingContext` before advertising readiness.
- `/ready` runs a DB ping and returns `503 Service Unavailable` if the ping
  fails.
- Shutdown should close the DB handle after HTTP serving stops accepting new
  work.

## 4. Readiness Matrix

| Runtime state                                      | Startup expectation            | `/health`                  | `/ready`                             |
| -------------------------------------------------- | ------------------------------ | -------------------------- | ------------------------------------ |
| `DATABASE_URL` empty                               | Start without DB connector.    | `200` if process is alive. | `200`; DB is intentionally disabled. |
| `DATABASE_URL` set and startup ping succeeds       | Start with DB connector.       | `200` if process is alive. | `200` while DB ping succeeds.        |
| `DATABASE_URL` set and startup ping fails          | Fail fast before serving HTTP. | Not served.                | Not served.                          |
| `DATABASE_URL` set, startup passed, DB later fails | Keep process observable.       | `200` if process is alive. | `503` until DB ping recovers.        |

Phase 4.5 readiness is connectivity-oriented. It should not run migrations and
should not mutate schema. If a later phase adds schema-version readiness, that
must be documented as a separate readiness gate.

### Phase 14 Readiness Extension

The Go API now reports configured dependency checks as additive JSON detail:

```json
{
  "status": "ready",
  "checks": {
    "database": { "status": "ready" },
    "redis": { "status": "ready" },
    "storage": { "status": "ready" }
  }
}
```

Only configured dependencies appear. If Redis is disabled or storage is local,
the check set reflects the runtime wiring that actually exists. Failed checks
return `503` with `status=not_ready` and `DEPENDENCY_NOT_READY`; raw dependency
errors are not exposed in the HTTP body; `/ready` reports only per-check
`ready`/`not_ready` state.

## 5. Migration CLI Flow

Migrations are run by an operator or deployment step before the API release is
started/restarted. API startup must not auto-migrate.

Expected source-run command shape:

```bash
cd mm-chat/backend

# DATABASE_URL must point at the target Postgres database.
go run ./cmd/migrate up
```

Rollback/reset command shape for development or an intentional destructive
rollback window:

```bash
cd mm-chat/backend

go run ./cmd/migrate down --all
```

Runner contract:

- SQL migration files should be embedded into the Go CLI so the executed SQL
  matches the backend release artifact.
- The runner records applied versions in `schema_migrations(version, name, applied_at)`.
- The metadata table is migration-runner state, not a domain application table
  like `users`, `conversations`, or `messages`.
- Operators should verify both app tables and runner metadata after `up`.
- The runner owns each migration transaction; SQL migration files must not
  include `BEGIN`, `COMMIT`, or `ROLLBACK`.

Inspection draft:

```bash
docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c \
  "select tablename from pg_tables where schemaname = 'public' order by tablename;"

docker exec -i mm-chat-postgres psql -U neo_chat -d neo_chat -c \
  "select * from schema_migrations order by version;"
```

## 6. Rollback Boundaries

Application rollback:

- Keep the previous backend binary/image available.
- Stop the new backend and restart the previous release if DB-aware startup or
  readiness fails after deployment.
- While local-first mode remains available, frontend rollback is still
  `NEXT_PUBLIC_API_MODE=local`; this does not delete Postgres data.

Database rollback:

- Take a pre-migration logical dump before running `up` in any production-like
  environment.
- Use `go run ./cmd/migrate down --all` only for development resets or an
  explicit destructive rollback window.
- Prefer restoring the pre-migration dump for production-like rollback because
  down migrations can lose writes created after the migration.

Configuration rollback:

- Clearing `DATABASE_URL` returns the API to DB disabled mode for skeleton
  readiness, but it also removes durable DB dependency from runtime behavior.
- Do not use DB disabled mode as a silent fallback for endpoints that require
  persisted conversations, sessions, files, or audit logs.
