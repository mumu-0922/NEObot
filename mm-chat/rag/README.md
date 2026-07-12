# MM Chat RAG Worker — Phase 15.2B

Python 3.13 durable-consumer mechanics for MM Chat. This package is deliberately
**dark-run by default**: it validates PostgreSQL functions and the singleton
worker lock, then serves health and metrics without claiming outbox events or
processing jobs.

Phase B does not parse files, call embedding providers, read object storage,
build search projections, or purge data. `DISPATCH_REGISTRY` and
`JOB_HANDLER_REGISTRY` are intentionally empty until Phase 15.2C promotion.

## Safety boundaries

- PostgreSQL is authoritative. Runtime SQL calls frozen `SECURITY DEFINER`
  functions only, except PostgreSQL's built-in session advisory lock.
- The worker never runs DDL, schema migration, ORM, Alembic, or direct table DML.
- Redis is an optional, lossy wake hint (`payload == "1"`). One-second PostgreSQL
  polling and 30-second forced rescans continue when Redis is absent or flushed.
- Logs redact credentials, URLs, payloads, query/body/content, tokens, and object
  keys. Metrics have only fixed-cardinality labels.
- Replay uses a separate `RAG_REPLAY_DATABASE_URL` and is dry-run unless
  `--execute` is supplied with all CAS/audit inputs.

## Local quality gates

No command loads repository `.env` files.

```bash
uv sync --locked
uv run ruff check .
uv run ruff format --check .
uv run mypy src
uv run pytest --cov=mm_chat_rag --cov-report=term-missing
uv run pip-audit --skip-editable
./tests/docker-smoke.sh
```

The Docker smoke builds the image, creates an isolated disposable PostgreSQL
fixture, starts `rag-worker` as UID `10001` with a read-only root filesystem,
waits for `/health`, and executes `rag-replay --help`. It does not read repository
`.env` files or connect to deployment services.

Integration tests skip only when their explicit DSN is absent:

```bash
RAG_TEST_DATABASE_URL='postgresql://...' uv run pytest -m integration
RAG_TEST_REDIS_URL='redis://...' uv run pytest -m integration
```

The PostgreSQL DSN must target a database migrated through `010` and a role with
only the Phase 15.2B worker grants.

## Runtime

Required:

```text
RAG_WORKER_DATABASE_URL=postgresql://...
```

Safe defaults:

```text
RAG_WORKER_DISPATCH_ENABLED=false
RAG_WORKER_JOB_STAGES=
RAG_WORKER_POLL_SECONDS=1
RAG_WORKER_RESCAN_SECONDS=30
RAG_WORKER_OUTBOX_LEASE_SECONDS=30
RAG_WORKER_JOB_LEASE_SECONDS=90
RAG_WORKER_HEARTBEAT_SECONDS=30
```

Optional Redis wake-up:

```text
RAG_WORKER_REDIS_URL=redis://...
REDIS_KEY_PREFIX=mm-chat
```

Endpoints on the private listener (default `:8081`):

- `GET /health`: event-loop liveness; this is the container healthcheck.
- `GET /ready`: DB/functions/lock/consumer/projection/Redis components.
  `projection=not_ready` is expected before migration `011` and is not a core
  dark-run readiness failure.
- `GET /metrics`: Prometheus exposition.

## Replay

Dry-run (does not require a DSN):

```bash
rag-replay outbox --id "$EVENT_ID" --expected-error-code PROVIDER_TIMEOUT
```

Execution requires exact failed ID, expected current error code, operator UUID,
and non-empty reason. Job replay also requires a fresh successor UUID:

```bash
RAG_REPLAY_DATABASE_URL='postgresql://...' rag-replay job \
  --id "$JOB_ID" \
  --expected-error-code PROVIDER_TIMEOUT \
  --successor-job-id "$SUCCESSOR_ID" \
  --operator-id "$OPERATOR_ID" \
  --reason 'approved incident replay' \
  --execute
```
