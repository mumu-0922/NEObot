# MM Chat RAG Worker — Phase 15.2B

Python 3.13 durable-consumer mechanics for MM Chat. This package is deliberately
**dark-run by default**: it validates PostgreSQL functions and the singleton
worker lock, then serves health and metrics without claiming outbox events or
processing jobs.

Phase B does not parse files, call embedding providers, read object storage,
build search projections, or purge data. `DISPATCH_REGISTRY` and
`JOB_HANDLER_REGISTRY` are intentionally empty until Phase 15.2C promotion.

Phase 15.2C C0 adds **test-only** redacted Provider Contract intake under
`tests/fixtures/provider_contracts/`. The checked-in MinerU/Jina fixtures remain
blocked drafts; they do not enable provider calls or production handlers.

The operator-only C0 Provider Capture Harness lives in
`tools/provider_capture.py`. It defaults to a redacted, zero-network dry-run and
is not a production console script. Real egress requires explicit `--execute`
plus process-environment credentials. A WSL operator may explicitly set
`PROVIDER_CAPTURE_PROXY_URL` to a literal private/loopback HTTP proxy; generic
proxy variables remain ignored. MinerU capture is intentionally staged at the
v4 local-upload Submit response: it does not PUT the signed URL or poll. A
separate `tools/provider_capture_mineru_lifecycle.py` CLI implements the full
bounded synthetic Allocate/Upload/Poll/Download chain with Evidence v2. Its
first real Capture reached `done` but ended as `unknown_download`; it remains
outside Runtime and no Fixture was promoted.

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
uv run mypy src tests/support
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

Provider Contract validation and in-memory replay do not read `.env`, start a
listener, or make network calls:

```bash
uv run pytest -p no:cacheprovider tests/unit/test_provider_contracts.py
```

The Provider Capture Harness also makes no network call by default and creates
no evidence file in dry-run mode:

```bash
uv run python -B -m tools.provider_capture
uv run python -B -m tools.provider_capture_mineru_lifecycle
uv run pytest -p no:cacheprovider tests/unit/test_provider_capture.py
uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_lifecycle_capture.py \
  tests/unit/test_provider_lifecycle_security.py
```

Authorized execution, exact budgets, evidence schema, review/freeze procedure,
and rollback are specified in
[`../docs/contracts/provider-capture-harness.md`](../docs/contracts/provider-capture-harness.md).
The example credential file contains empty values only. The harness does not
load it or any other dotenv file. `-B` is required by the CLI contract so a
dry-run creates neither evidence nor Python bytecode cache files. MinerU
`unknown_submission` writes its recovery evidence but returns exit code `3`.
Generated `provider-capture-*/` directories are ignored as a defense in depth;
reviewed Evidence must still be moved to a private Git-external store.
Provider Keys must be injected with a complete no-echo prompt or controlled
Secret mechanism. Do not implement partial visibility with repeated
`read -s -n1`: long clipboard pastes can leak into terminal scrollback.
When direct WSL egress is unavailable, copy the Owner-controlled proxy into the
dedicated variable for that one subshell, for example
`export PROVIDER_CAPTURE_PROXY_URL="$https_proxy"`; the harness validates a
literal RFC1918/loopback address and still uses `trust_env=false`.

`jsonschema`, `types-jsonschema`, `rfc8785`, and `httpx` are dev-only. Docker
uses `uv sync --no-dev`, and `.dockerignore` excludes `tests`, so fixtures and
Fake Provider support do not enter the runtime image. The Dockerfile copies only
`src/`; `tools/` is neither copied nor registered in `[project.scripts]`.

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
