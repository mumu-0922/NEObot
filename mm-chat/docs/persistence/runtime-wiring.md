# Postgres Runtime Wiring

This document defines the current runtime contract between the Go API,
migration CLI, Memory Worker, RAG Worker/Replay processes, and Postgres. The
original connector contract remains in force, while the current schema head is
`069`.

## 1. Scope

In scope:

- Separate Migration, API/admin, Memory Worker, RAG Worker, and Replay credential routes.
- Postgres connector using the `pgx` driver.
- Startup connectivity check when DB is enabled.
- DB-aware `/ready` behavior.
- Embedded SQL migrations exposed through a Go migration CLI.
- Schema head `069`, including durable Memory capture, provenance, Review,
  direct action/Activity/Usage, lexical/hybrid shadow, governance/portability,
  derived L2 Scene, independent derived L3 Persona, and lease-fenced L1
  auto-capture promotion boundaries.
- Operator-facing migration and rollback boundaries.

Out of scope:

- Automatic migrations during API startup.
- Compose implementation files.
- MinIO, Redis, browser import, or multi-server deployment details.
- Automatic Memory reader promotion and external Memory engines.

## 2. Environment Variables

| Variable                  | Route        | Required | Meaning                                                                                                          |
| ------------------------- | ------------ | -------- | ---------------------------------------------------------------------------------------------------------------- |
| `MIGRATION_DATABASE_URL`  | Migration    | Yes      | Dedicated bootstrap/migrator connection string read only by `cmd/migrate`; there is no `DATABASE_URL` fallback.  |
| `DATABASE_URL`            | API/admin    | No       | API Postgres connection string. Empty means DB disabled mode; non-empty enables startup ping and DB readiness.   |
| `MEMORY_WORKER_DATABASE_URL` | Memory Worker | Yes   | Dedicated long-running connection with only `memory_worker_runtime`.                                             |
| `RAG_WORKER_DATABASE_URL` | RAG Worker   | Yes      | Dedicated long-running Worker connection string with only the `rag_worker_executor` capability.                  |
| `RAG_REPLAY_DATABASE_URL` | RAG Replay   | Execute  | Dedicated one-shot Replay connection string; required for `rag-replay --execute`, not for dry-run intent output. |
| `DB_MAX_OPEN_CONNS`       | Go API/admin | No       | Maximum open DB connections. Backend default is code-defined when unset.                                         |
| `DB_MAX_IDLE_CONNS`       | Go API/admin | No       | Maximum idle DB connections. Backend default is code-defined when unset.                                         |
| `DB_CONN_MAX_LIFETIME`    | Go API/admin | No       | Maximum connection lifetime as a Go duration such as `30m`. Backend default is code-defined when unset.          |
| `MEMORY_LEXICAL_SHADOW_ENABLED` | Go API | No | Default `false`; enables provider-free migration-058 comparison/diagnostics only, never projection maintenance or prompt injection. |
| `MEMORY_HYBRID_SHADOW_ENABLED` | API + Memory Worker | No | Default `false`; one switch gates migration-059 embedding claims and hybrid comparison Provider calls. It never changes the reader, prompt, or Usage. |
| `MEMORY_TOOL_LOOP_ENABLED` | Go API | No | Default `false`; exposes first-round `search_memory` only on eligible Tool-capable turns. When true, Server installs the fixed production BGE/Luna policy, reauthorizes its stored Provider tuple per Judge attempt, then uses migration-065 final hydration and same-model continuation. False is immediate reader/Judge rollback. Never pass it to the Memory Worker. |
| `MEMORY_L2_SCENE_SHADOW_ENABLED` | API + Memory Worker | No | Default `false`; gates migration-062 Scene refresh/query-embedding/rerank Provider work. Provider-free stale purge remains enabled. |
| `MEMORY_L2_SCENE_READER_ENABLED` | Go API | No | Default `false`; requests active Scene injection, which still requires database promotion, current L1 reader authority, and user policy. Never pass this flag to the Worker. |
| `MEMORY_L3_PERSONA_SHADOW_ENABLED` | API + Memory Worker | No | Default `false`; gates migration-063 Persona refresh/query-embedding/rerank Provider work. Provider-free stale purge remains enabled. |
| `MEMORY_L3_PERSONA_READER_ENABLED` | Go API | No | Default `false`; requests active Persona injection, which still requires database promotion, current L1 reader authority, current Persona/member authority, and user policy. Never pass this flag to the Worker. |

Rules:

- Never log any database URL or credential. Redact URL userinfo before logging.
- Use pairwise-distinct login names and passwords for Migration, API/admin,
  Memory Worker, RAG Worker, and Replay. They target the same database but do not share runtime
  capabilities.
- The migration CLI requires `MIGRATION_DATABASE_URL` and reads no runtime URL.
  An unset or blank value fails closed; it never falls back to `DATABASE_URL`.
- The API/admin route uses only `DATABASE_URL`; Memory Worker uses only
  `MEMORY_WORKER_DATABASE_URL`; RAG Worker uses only `RAG_WORKER_DATABASE_URL`;
  executable Replay uses only
  `RAG_REPLAY_DATABASE_URL`.
- `sslmode=disable` is acceptable only on a private single-server Docker
  network. Use TLS for cross-host or untrusted networks.
- Invalid or blank pool settings fall back to backend defaults so startup does
  not panic on a partially configured environment.

## 3. Connector Behavior

### `DATABASE_URL` empty

- DB is disabled.
- API startup must not require Postgres.
- `/health` remains process liveness.
- `/ready` remains `200 OK` for runtime with DB intentionally disabled.
- DB-backed product endpoints fail explicitly instead of pretending durable
  persistence exists.

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

## 4. Database Principal Boundary

| Route     | URL variable              | LOGIN capability      | Boundary                                                                                       |
| --------- | ------------------------- | --------------------- | ---------------------------------------------------------------------------------------------- |
| Migration | `MIGRATION_DATABASE_URL`  | Bootstrap/migrator    | Owns DDL and migration metadata; never used by API, Memory/RAG Worker, or Replay.              |
| API/admin | `DATABASE_URL`            | `go_api_runtime`      | Existing API access plus narrow Memory action/governance/portability, `058`/`059` comparison, `062` Scene, and `063` Persona search/governance capabilities; no projection/observation table CRUD or promotion authority. |
| Memory Worker | `MEMORY_WORKER_DATABASE_URL` | `memory_worker_runtime` | Executes only lease/source/profile-fenced capture/purge/review, `066` governance-backed safe-add auto-capture plus `067`–`069` authority/profile hardening, L1 embedding, `062` Scene, and `063` Persona refresh/purge/embedding functions; receives no reader promotion, general governance, or table CRUD authority. |
| Worker    | `RAG_WORKER_DATABASE_URL` | `rag_worker_executor` | Executes `010` Claim/CAS/Publish/Purge functions; no authority-table DML or Replay capability. |
| Replay    | `RAG_REPLAY_DATABASE_URL` | `rag_replay_operator` | Executes only the `010` replay functions in the operator-triggered one-shot process.           |

Migrations `010` and `054` create and validate their capability roles as `NOLOGIN` roles.
Deployment provisions separate LOGIN principals and grants exactly one matching
runtime capability to each API, Memory Worker, RAG Worker, and Replay login. The migrator does not
inherit a runtime capability; `go_api_runtime` does not inherit owner, Worker,
or Replay roles.

## 5. Readiness Matrix

| Runtime state                                      | Startup expectation            | `/health`                  | `/ready`                             |
| -------------------------------------------------- | ------------------------------ | -------------------------- | ------------------------------------ |
| `DATABASE_URL` empty                               | Start without DB connector.    | `200` if process is alive. | `200`; DB is intentionally disabled. |
| `DATABASE_URL` set and startup ping succeeds       | Start with DB connector.       | `200` if process is alive. | `200` while DB ping succeeds.        |
| `DATABASE_URL` set and startup ping fails          | Fail fast before serving HTTP. | Not served.                | Not served.                          |
| `DATABASE_URL` set, startup passed, DB later fails | Keep process observable.       | `200` if process is alive. | `503` until DB ping recovers.        |

API readiness is connectivity-oriented. It does not run migrations or mutate
schema. Operators establish schema head `069` before starting a release that
depends on it.

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

### Phase 14 Metrics Extension

The Go API exposes `GET /metrics` in Prometheus text format. The endpoint
publishes bounded HTTP request counters, response-byte counters, latency
histograms, process/build metadata, configured dependency readiness gauges, and
Postgres `database/sql` pool stats when the DB pool is enabled.

Dependency gauge rules:

- `mm_chat_dependency_ready{dependency="database"}` mirrors the DB readiness
  check when `DATABASE_URL` enables Postgres.
- `mm_chat_dependency_ready{dependency="redis"}` mirrors Redis readiness only
  when `REDIS_URL` enables Redis.
- `mm_chat_dependency_ready{dependency="storage"}` mirrors the configured file
  store. For `STORAGE_BACKEND=minio|s3`, this is the MinIO/S3 bucket readiness
  check.

HTTP metric labels must stay low-cardinality. Dynamic identifiers are rendered
as route patterns such as `/v1/chat/conversations/{id}/stream` or
`/v1/files/{id}/content`; unknown paths are collapsed to `/__unknown__`, and
unknown HTTP methods are collapsed to `OTHER`. Never expose raw UUIDs, run IDs,
object keys, query strings, bearer tokens, or provider parameters in metric
labels.

## 6. Migration CLI Flow

Migrations are run by an operator or deployment step before the API release is
started/restarted. API startup must not auto-migrate. The current embedded chain
includes `013_rag_worker_projection_gate`; later pgvector/true BM25 accelerator
DDL will be added only through another reversible migration.

Expected source-run command shape:

```bash
cd mm-chat/backend

# Required independently; cmd/migrate never reads or falls back to DATABASE_URL.
MIGRATION_DATABASE_URL="postgres://..." go run ./cmd/migrate up
```

Rollback/reset command shape for development or an intentional destructive
rollback window:

```bash
cd mm-chat/backend

MIGRATION_DATABASE_URL="postgres://..." go run ./cmd/migrate down --all
```

Runner contract:

- SQL migration files should be embedded into the Go CLI so the executed SQL
  matches the backend release artifact.
- The runner records applied versions in
  `schema_migrations(version, name, checksum, applied_at)`, serializes every
  operation with a PostgreSQL advisory lock, and requires explicit `baseline`
  acceptance for legacy rows without checksums.
- The metadata table is migration-runner state, not a domain application table
  like `users`, `conversations`, or `messages`.
- Operators should verify both app tables and runner metadata after `up`.
- The runner owns each migration transaction; SQL migration files must not
  include `BEGIN`, `COMMIT`, or `ROLLBACK`.
- `010` implements the durable projection consistency layer. `012` adds the
  extension-independent child search projection staging tables and completeness
  function. Extension-specific pgvector/true BM25 accelerator DDL remains
  reserved for a later reversible search-profile migration.

Inspect app tables and runner state through an approved migrator session without
placing a password in SQL or command output:

```sql
SELECT tablename
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY tablename;

SELECT version, name
FROM schema_migrations
ORDER BY version;
```

## 7. Rollback Boundaries

Application rollback:

- Keep the previous backend binary/image available.
- Stop the new backend and restart the previous release if DB-aware startup or
  readiness fails after deployment.
- Frontend rollback uses the retained previous Server-mode frontend/backend
  image pair. Do not create a second browser-local Memory authority.

Database rollback:

- Take a pre-migration logical dump before running `up` in any production-like
  environment.
- Use
  `MIGRATION_DATABASE_URL="postgres://..." go run ./cmd/migrate down --all`
  only for development resets or an explicit destructive rollback window.
- Prefer restoring the pre-migration dump for production-like rollback because
  down migrations can lose writes created after the migration.
- `010.down` is guarded: it rejects active leases, generation-bound Jobs,
  post-`010` authority rows, and non-empty projection state instead of silently
  discarding them.
- After those guards pass, `010.down` removes the `010` projection surface and
  model-aware additions but retains `go_api_runtime`, schema usage, explicit
  CRUD on the authoritative `001`–`009` relations, and access to
  `knowledge_outbox_id_seq`. The API login therefore keeps the least-privilege
  capability required by a rolled-back schema-`009` API; do not replace it with
  the migrator or projection-owner credential.

Configuration rollback:

- Clearing `DATABASE_URL` returns the API to DB-disabled readiness, but it also
  removes durable DB dependency from runtime behavior.
- Do not use DB disabled mode as a silent fallback for endpoints that require
  persisted conversations, sessions, files, or audit logs.
