# G7 RAG and Citation Process Log

This process log records standalone G7 work. It is intentionally separate from
`standalone-parity-sliced-process.md` so each G7 slice can be replayed without
scanning earlier migration history.

## 2026-07-15 — G7.1 Owner Grill and Cutover Decisions Locked

Objective: start G7 by locking owner decisions before code changes, because
RAG provider, credential, data-egress, citation, and strictness choices change
implementation boundaries.

Owner decisions locked:

- first G7 target is a real provider loop, not fake/local-only;
- all Knowledge data selected for indexing may egress to configured providers in
  this owner-operated deployment;
- provider chain is MinerU + Jina + Postgres:
  - MinerU parses PDFs, including scanned and complex formula/table PDFs;
  - Jina provides 1024-dimensional embeddings and reranking;
  - Postgres owns projection/search state for the first standalone profile;
  - Go owns ACL/source reauthorization and citation minting;
- automatic background indexing uses administrator-owned server secrets from env
  or Docker secrets; admin web key configuration is deferred;
- frontend MinerU BYOK/manual parse paths may remain but are not the credential
  source for G7 automatic indexing;
- uploads/binds auto-enqueue indexing; browser presence is not required;
- retries are capped at three attempts with bounded backoff;
- delete/tombstone is immediately query-invisible; purge is async;
- replacement publishes only after the new version/generation is ready;
- chat queries only collections explicitly selected/enabled in the current chat;
- strict Knowledge refuses unknowns; normal chat may degrade only with explicit
  no-Knowledge-evidence metadata;
- first citation UI is a basic marker/card, not a rich PDF highlighter;
- G7 adds Go-owned routes while `mm-chat` legacy Next `/api/rag/*`,
  `/api/doc-parse/*`, and `/api/chat/rag-queries` remain for G9 deletion;
- provider calls are rate-limited with wide, configurable defaults.

Artifacts added:

```text
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
```

Expected validation for this slice:

```text
cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/architecture/standalone-parity-sliced-cutover-plan.md \
  ../docs/tracking/progress.md
```

Next slice: G7.2 admin provider config and fail-closed readiness.

## 2026-07-15 — G7.2 Admin Provider Config and Fail-Closed Readiness

Objective: make the G7 MinerU + Jina provider prerequisites visible and
fail-closed before promoting real parser/index dispatch.

Implemented behavior:

- Go backend config now loads administrator-owned RAG provider secrets:
  - `RAG_MINERU_API_TOKEN` with `DEFAULT_MINERU_API_TOKEN` fallback;
  - `RAG_JINA_API_KEY` with `DEFAULT_JINA_API_KEY` fallback;
  - Jina embedding dimensions fixed at `1024`.
- Go exposes protected `GET /v1/rag/provider-status` through the normal session
  identity middleware. The response includes only configured/missing status and
  embedding dimensions; it never serializes secret values.
- Python `rag-worker` settings now accept the same provider secret names and
  fail closed if dispatch enables `parse` without MinerU or
  `passage_embedding` without Jina. `purge` remains credential-free.
- Compose and env templates now pass the provider secret names to `backend` and
  `rag-worker`. Blank values keep G7 provider readiness false and dispatch-safe.

Touched files:

```text
backend/internal/config/config.go
backend/internal/config/config_test.go
backend/internal/ragproviders/handler.go
backend/internal/ragproviders/handler_test.go
backend/internal/ragproviders/status.go
backend/internal/httpserver/server.go
backend/internal/httpserver/server_test.go
backend/internal/httpserver/metrics.go
backend/internal/httpserver/metrics_test.go
rag/src/mm_chat_rag/settings.py
rag/tests/unit/test_settings.py
compose.single-server.yml
backend/.env.example
.env.single-server.example
docs/deployment/single-server-compose.md
```

Verification run during the slice:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build \
  go test ./internal/config ./internal/ragproviders ./internal/httpserver

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_settings.py
```

Result:

```text
Go targeted packages passed.
Python settings tests passed: 25 passed.
```

Next slice: G7.3 provider-backed parser/index profile gate with retry/rate-limit
profile defaults and no provider body/secret logging.

Final G7.2 verification before commit:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...
cd mm-chat/rag && uv run mypy src/mm_chat_rag/settings.py
cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_settings.py
cd mm-chat/rag && uv run ruff check src/mm_chat_rag/settings.py tests/unit/test_settings.py
cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md \
  ../docs/architecture/standalone-parity-sliced-cutover-plan.md \
  ../docs/deployment/single-server-compose.md \
  ../compose.single-server.yml \
  ../.env.single-server.example
cd mm-chat && git diff --check -- .
```

Final result:

```text
Go backend full package test passed.
Python settings mypy/pytest/ruff passed.
Prettier check passed for changed docs/Compose template.
git diff --check passed for mm-chat scoped diff.
```

## 2026-07-15 — G7.3 Provider-backed Parser/Index Profile Gate

Objective: promote the owner-locked MinerU + Jina + Postgres shape into an
explicit runtime profile gate without yet attaching quota-consuming provider
handlers.

Implemented behavior:

- Added Python config-only provider profile module:
  - profile id: `mineru_jina_postgres_v1`;
  - default profile remains `disabled`;
  - Jina embedding model locked to `jina-embeddings-v4`;
  - Jina embedding dimensions locked to `1024`;
  - Jina rerank model locked to `jina-reranker-v3`;
  - provider retry max attempts fixed at `3`;
  - default retry window `30s..300s`;
  - default provider concurrency `2`;
  - default MinerU/Jina request ceilings `60` and `240` requests per minute.
- Provider-backed `parse` and `passage_embedding` dispatch now require:
  - selected profile `RAG_PROVIDER_PROFILE=mineru_jina_postgres_v1`;
  - `RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED=true` to record accepted owner risk
    for draft public MinerU/Jina wire fixtures;
  - required server-owned provider secrets from G7.2.
- `purge` still requires no provider profile or provider key.
- Production dispatch/job registries remain empty. G7.3 cannot consume provider
  quota and does not introduce HTTP clients, provider SDKs, or network handlers.
- Legacy worker/job tests were updated to construct explicit fake provider
  profile settings instead of relying on implicit `parse` without credentials.

Touched files:

```text
rag/src/mm_chat_rag/provider_profile.py
rag/src/mm_chat_rag/settings.py
rag/tests/unit/test_provider_profile.py
rag/tests/unit/test_settings.py
rag/tests/unit/test_jobs.py
rag/tests/unit/test_replay_worker.py
compose.single-server.yml
.env.single-server.example
docs/deployment/single-server-compose.md
docs/architecture/g7-rag-citation-cutover-plan.md
docs/architecture/standalone-parity-sliced-cutover-plan.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/settings.py \
  src/mm_chat_rag/provider_profile.py \
  tests/unit/test_settings.py \
  tests/unit/test_provider_profile.py \
  tests/unit/test_jobs.py \
  tests/unit/test_replay_worker.py

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/settings.py \
  src/mm_chat_rag/provider_profile.py

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_settings.py \
  tests/unit/test_provider_profile.py \
  tests/unit/test_replay_worker.py \
  tests/unit/test_jobs.py \
  tests/unit/test_consumer.py \
  tests/unit/test_models_handlers_retry.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_deployment_boundary.py
```

Result:

```text
Python profile/settings/job gate tests passed: 97 passed.
Ruff and mypy passed for changed Python files.
```

Next slice: G7.4 canonical IR to chunks and Postgres projection. G7.5 will attach
actual dispatch/handler execution; G7.3 deliberately stops before quota-consuming
network calls.

Final G7.3 verification before commit:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/settings.py \
  src/mm_chat_rag/provider_profile.py \
  tests/unit/test_settings.py \
  tests/unit/test_provider_profile.py \
  tests/unit/test_jobs.py \
  tests/unit/test_replay_worker.py \
  tests/unit/test_postgres.py
cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/settings.py \
  src/mm_chat_rag/provider_profile.py
cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md \
  ../docs/architecture/standalone-parity-sliced-cutover-plan.md \
  ../docs/deployment/single-server-compose.md \
  ../compose.single-server.yml \
  ../.env.single-server.example
```

Final result:

```text
RAG unit tests passed: 1436 passed.
Ruff and mypy passed for changed Python files.
Prettier check passed for changed docs/Compose template.
```

Additional final gate:

```text
cd mm-chat && git diff --check -- .
```

Result: clean.

## 2026-07-15 — G7.4 Canonical IR to Chunks and Postgres Projection

Objective: create the deterministic projection seam between parser artifacts,
chunk manifests, and Postgres search state without yet enabling worker dispatch
or provider quota consumption.

Implemented behavior:

- Added `rag/src/mm_chat_rag/projection.py`, a pure projection builder for
  already-validated Canonical IR v2 plus Chunk Manifest v2 artifacts.
- The builder emits deterministic row DTOs for:
  - `knowledge_blocks`;
  - `knowledge_parent_chunks`;
  - `knowledge_child_chunks`;
  - `knowledge_chunk_block_spans`;
  - `knowledge_child_search_projections` seeds.
- Projection IDs are stable UUIDv5 values scoped by immutable artifact set or
  materialization IDs. A materialization replacement changes chunk IDs while the
  same parser artifact keeps block IDs stable.
- The builder fails closed on source hash mismatch, stale content byte/hash
  bindings, missing parent chunks, non-text block references, invalid locators,
  and stale manifest counts.
- Added migration `012_rag_search_projection`:
  - `knowledge_search_profiles` locks `mineru_jina_postgres_v1`, Jina embedding
    model `jina-embeddings-v4`, dimensions `1024`, and reranker
    `jina-reranker-v3`;
  - `knowledge_child_search_projections` stores extension-independent dense
    `REAL[]` vectors, generated built-in lexical `TSVECTOR`, exact `TEXT[]`,
    source-span/hash fences, locator summaries, and staging/ready/purge state;
  - `knowledge_assert_materialization_search_complete(...)` proves every child
    chunk has a ready 1024-dimensional embedding projection before G7.5 publish
    code may promote it.
- G7.4 intentionally does not add `CREATE EXTENSION`, provider HTTP clients, job
  handlers, or dispatch registry entries. pgvector/true BM25 accelerator DDL is
  left to a later reversible search-profile migration after deployment image and
  license gates are closed.

Touched files:

```text
rag/src/mm_chat_rag/projection.py
rag/tests/unit/test_projection.py
backend/migrations/012_rag_search_projection.up.sql
backend/migrations/012_rag_search_projection.down.sql
backend/internal/migration/phase15_rag_search_projection_schema_test.go
docs/architecture/g7-rag-citation-cutover-plan.md
docs/architecture/standalone-parity-sliced-cutover-plan.md
docs/persistence/README.md
docs/persistence/postgres-schema.md
docs/persistence/runtime-wiring.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_projection.py
cd mm-chat/rag && uv run ruff check src/mm_chat_rag/projection.py tests/unit/test_projection.py
cd mm-chat/rag && uv run mypy src/mm_chat_rag/projection.py
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/migration \
  -run 'TestPhase15RAGSearchProjectionSchemaContract|TestLoadOrders|TestEmbeddedMigrations'
```

Result:

```text
Python projection tests passed: 7 passed.
Python ruff and mypy passed for the new projection module.
Go migration static contract tests passed.
```

Next slice: G7.5 worker dispatch, rebuild, delete, and retry loop. G7.5 must
call the G7.4 projection builder plus `knowledge_assert_materialization_search_complete(...)`
before any materialization publish path is promoted.

Final G7.4 verification before commit:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...
cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/architecture/standalone-parity-sliced-cutover-plan.md \
  ../docs/architecture/phase-15-2c-generation-bound-indexing-plan.md \
  ../docs/persistence/README.md \
  ../docs/persistence/postgres-schema.md \
  ../docs/persistence/runtime-wiring.md \
  ../docs/persistence/phase-15-rag-projection-schema.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
cd mm-chat && git diff --check -- .
```

Final result:

```text
RAG unit tests passed: 1443 passed.
Go backend full package tests passed.
Prettier check passed for changed docs.
git diff --check passed for mm-chat scoped diff.
```
