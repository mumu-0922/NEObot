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

## 2026-07-15 — G7.5.1 Worker Projection Completeness Gate

Objective: start G7.5 with a narrow readiness/adapter slice that lets future
worker handlers prove G7.4 search projection completeness before publishing,
without enabling provider calls or handler dispatch.

Implemented behavior:

- Added migration `013_rag_worker_projection_gate` to replace
  `knowledge_rag_worker_readiness()` with a stricter required-function set that
  includes `knowledge_assert_materialization_search_complete(uuid,bigint,text,integer)`.
- Readiness detail now exposes a bounded `searchCompletenessGate` value of
  `ready|not_ready`; it does not include document IDs, SQL errors, provider
  bodies, or secrets.
- Added `PostgresAdapter.assert_materialization_search_complete(...)`, pinned to
  `jina-embeddings-v4` and `1024` dimensions at the Python boundary.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice cannot claim work or consume MinerU/Jina quota by itself.

Touched files:

```text
backend/migrations/013_rag_worker_projection_gate.up.sql
backend/migrations/013_rag_worker_projection_gate.down.sql
backend/internal/migration/phase15_rag_worker_projection_gate_schema_test.go
rag/src/mm_chat_rag/postgres.py
rag/tests/unit/test_postgres.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/persistence/README.md
docs/persistence/postgres-schema.md
docs/persistence/runtime-wiring.md
docs/persistence/phase-15-rag-projection-schema.md
docs/tracking/g7-rag-citation-process.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_postgres.py
cd mm-chat/rag && uv run ruff check src/mm_chat_rag/postgres.py tests/unit/test_postgres.py
cd mm-chat/rag && uv run mypy src/mm_chat_rag/postgres.py
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/migration \
  -run 'TestPhase15RAGWorkerProjectionGateReadinessContract|TestEmbeddedMigrations'
```

Result:

```text
Python Postgres adapter tests passed: 18 passed.
Ruff and mypy passed for the changed Python adapter.
Go migration readiness-gate tests passed.
```

Next G7.5 slice: add the first promoted-but-still-bounded handler seam for
index/reprocess/delete work, with production dispatch still gated by explicit
profile, registry, and retry limits.

Final G7.5.1 verification before commit:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...
cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_postgres.py
cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/persistence/README.md \
  ../docs/persistence/postgres-schema.md \
  ../docs/persistence/runtime-wiring.md \
  ../docs/persistence/phase-15-rag-projection-schema.md \
  ../docs/tracking/g7-rag-citation-process.md
cd mm-chat && git diff --check -- .
```

Final result:

```text
Go backend full package tests passed.
Python Postgres adapter tests passed: 18 passed.
Prettier check passed for changed docs.
git diff --check passed for mm-chat scoped diff.
```

## 2026-07-15 — G7.5.2 Job Context Admission Seam

Objective: add the first handler-adjacent seam for G7.5 without enabling real
provider execution. Future parse / passage-embedding / purge handlers must
start from a typed context, not a raw `JobClaim.values` map.

Implemented behavior:

- Added `rag/src/mm_chat_rag/job_context.py` with `ProcessingJobContext` and
  `ProviderAuthority` dataclasses.
- Added `admit_processing_job_context(...)`, which converts the DB claim row
  into typed IDs, revisions, request hash, projection binding, provider
  authority, and optional runtime profile proof.
- Fail-closed stable error codes cover:
  - unsupported stage or operation;
  - legacy projection-unbound jobs;
  - missing `index_generation_id` / provider-stage `materialization_id`;
  - malformed provider authority;
  - forbidden provider/governance fields on purge;
  - malformed request hash / counters / UUIDs;
  - disabled or mismatched runtime provider profile.
- Added `with_job_context_admission(...)` in `handlers.py` so future promoted
  handlers can be wrapped without reading raw claim rows.
- Production `JOB_HANDLER_REGISTRY` remains empty. This slice cannot claim real
  work and does not touch MinerU/Jina quota.

Touched files:

```text
rag/src/mm_chat_rag/job_context.py
rag/src/mm_chat_rag/handlers.py
rag/tests/unit/test_job_context.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_job_context.py tests/unit/test_jobs.py
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  tests/unit/test_job_context.py
cd mm-chat/rag && uv run mypy src/mm_chat_rag/job_context.py \
  src/mm_chat_rag/handlers.py
cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
cd mm-chat && git diff --check -- .
```

Result:

```text
Targeted Python job-context and runner tests passed: 25 passed.
Python RAG unit suite passed: 1461 passed.
Ruff and mypy passed for changed Python files.
Prettier check passed for changed docs.
git diff --check passed for mm-chat scoped diff.
```

Next G7.5 slice: bind Go-created jobs to Generation/Materialization authority
instead of legacy projection-unbound rows, then connect the admitted context to
bounded parse / passage-embedding / purge handler skeletons.

## 2026-07-15 — G7.5.3 Go Parse Job Materialization Binding

Objective: let Go-created parse jobs become claimable by the G7.5 Python worker
when a real active Corpus Index Generation exists, without enabling real
provider handler execution.

Implemented behavior:

- Service-layer document upload, replacement, and reprocess now allocate a
  distinct `materialization_id` in addition to the existing job/document/version
  IDs.
- Added `insertParseProcessingJob(...)` and `allocateParseMaterialization(...)`
  in Go:
  - finds the active Corpus Index Generation from `knowledge_corpus_projection_head`;
  - creates a staging `knowledge_document_materializations` row with the active
    Generation's `base_profile_hash`;
  - inserts the parse processing job with `legacy_projection_unbound=false`,
    `index_generation_id`, `materialization_id`, exact model/governance/consent
    authority, and `max_attempts=3`;
  - preserves the old `legacy_projection_unbound=true` fallback when no active
    Generation exists yet.
- Document version and reprocess outbox payloads now include
  `legacyProjectionUnbound`; when bound, they also include
  `indexGenerationId` and `materializationId`.
- Added a Postgres integration regression covering active-Generation document
  upload, materialization creation, outbox projection IDs, and claimability via
  `knowledge_claim_processing_job(...)`. The regression is gated by
  `MM_CHAT_TEST_DATABASE_URL` like the existing Postgres tests.

Touched files:

```text
backend/internal/knowledge/parse_jobs_postgres.go
backend/internal/knowledge/types.go
backend/internal/knowledge/service.go
backend/internal/knowledge/document_service.go
backend/internal/knowledge/documents_postgres.go
backend/internal/knowledge/versions_postgres.go
backend/internal/knowledge/reprocess_postgres.go
backend/internal/knowledge/service_test.go
backend/internal/knowledge/repository_postgres_test.go
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build \
  go test -count=1 ./internal/knowledge
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...
```

Result:

```text
Go knowledge package compile/unit pass. Postgres integration rows are present
but skipped unless MM_CHAT_TEST_DATABASE_URL is configured, matching existing
repository test behavior.
Go backend full package tests passed.
```

Next G7.5 slice: bind delete/tombstone purge jobs to Generation/Purge authority
and then attach admitted Python handler skeletons to the now Generation-bound
parse job context.

## 2026-07-15 — G7.5.4 Go Purge Job Generation Binding

Objective: make document tombstone/delete purge work claimable by the Python
worker when a real active Corpus Index Generation exists, while preserving the
legacy fallback before first Generation promotion.

Implemented behavior:

- `DeleteDocument` now resolves purge projection authority per tombstoned
  version:
  - no active Generation → keep `legacy_projection_unbound=true`;
  - active Generation → insert purge job with `legacy_projection_unbound=false`,
    `index_generation_id`, optional `materialization_id`, and `max_attempts=3`.
- If the active document projection head points at a materialization for the
  deleted version, the purge job and tombstone event carry that
  `materialization_id`; otherwise the purge job is Generation-bound with a null
  materialization, matching the existing DB shape for document-scope purge.
- Tombstone outbox payloads now include `legacyProjectionUnbound` and, when
  bound, `indexGenerationId` / `materializationId`.
- The active-Generation repository regression now also deletes the document and
  asserts the resulting purge job is Generation-bound and claimable via
  `knowledge_claim_processing_job(..., ARRAY['purge'])`.

Touched files:

```text
backend/internal/knowledge/deletion_postgres.go
backend/internal/knowledge/parse_jobs_postgres.go
backend/internal/knowledge/repository_postgres_test.go
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build \
  go test -count=1 ./internal/knowledge
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...
```

Result:

```text
Go knowledge package compile/unit pass. Postgres integration rows are present
but skipped unless MM_CHAT_TEST_DATABASE_URL is configured, matching existing
repository test behavior.
Go backend full package tests passed.
```

Next G7.5 slice: attach admitted Python handler skeletons to the
Generation-bound parse and purge contexts, still without enabling real
MinerU/Jina provider calls.

## 2026-07-15 — G7.5.5 Admitted Python Handler Skeletons

Objective: attach the first Python handler-shaped boundary to the
Generation-bound parse, passage-embedding, and purge contexts without enabling
real provider calls, object-storage reads, projection writes, or production job
dispatch.

Implemented behavior:

- Added `job_handlers.py` with async skeletons for:
  - `parse_handler_skeleton(...)`;
  - `passage_embedding_handler_skeleton(...)`;
  - `purge_handler_skeleton(...)`.
- Added claim-level constructor helpers for those skeletons; they all route
  through `with_job_context_admission(...)` instead of accepting raw claim rows.
- Skeletons accept only typed `ProcessingJobContext` values. A raw `JobClaim`
  or any non-context object is rejected with a stable redacted error code before
  stage-specific checks.
- Provider-backed skeletons re-check stage, non-zero Generation binding,
  non-null Materialization binding, provider authority, and the runtime
  `mineru_jina_postgres_v1` profile selected by the admission wrapper.
- Purge skeletons re-check stage and Generation binding, permit a null
  `materialization_id`, and reject provider authority.
- All skeleton paths end in `JOB_HANDLER_SKELETON_UNPROMOTED`, so an accidental
  manual wiring cannot silently mark parse/index/purge work as completed.
- Production `JOB_HANDLER_REGISTRY` remains empty, preserving the current
  no-claim/no-quota safety boundary.

Touched files:

```text
rag/src/mm_chat_rag/job_handlers.py
rag/tests/unit/test_job_handlers.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_job_context.py tests/unit/test_job_handlers.py \
  tests/unit/test_jobs.py
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py tests/unit/test_job_handlers.py
cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py
```

Result:

```text
36 targeted Python tests passed.
Ruff passed on the admission/skeleton files.
Mypy passed on the admission/skeleton source files.
```

Next G7.5 slice: add the first real admitted handler dependency seam for
storage/provider/projection execution, still default-off, then promote only one
stage under explicit registry and readiness gates.

## 2026-07-15 — G7.5.6 Parse Handler Dependency Seam

Objective: add the first real parse handler execution seam while keeping
production dispatch empty and avoiding any live MinerU/Jina/provider quota.

Implemented behavior:

- Added `job_handler_dependencies.py` with explicit parse-stage Protocols for:
  - `DocumentSourceGateway`;
  - `ParserGateway`;
  - `ParseProjectionGateway`.
- Added `ParseHandlerDependencies`; an empty bundle fails closed with
  `JOB_HANDLER_DEPENDENCY_UNCONFIGURED` before object-storage, provider, or
  projection calls.
- Added `parse_handler_with_dependencies(...)` and
  `admitted_parse_handler_with_dependencies(...)`:
  - claim-level entry is still wrapped by `with_job_context_admission(...)`;
  - the contextual handler reuses the parse authority fence from
    `job_handlers.py`;
  - fake storage/parser/projection gateways can now prove the intended flow
    without real network clients.
- The fake execution path fetches a document source, accepts MinerU-compatible
  Canonical IR v2 + Chunk Manifest v2 artifacts, builds the G7.4
  `PostgresProjectionBatch`, verifies the parser source hash against stored
  source metadata, and stages the batch through the projection gateway.
- Projection/artifact failures and source-hash mismatches are redacted into
  stable error codes and stop before projection staging.
- Production `JOB_HANDLER_REGISTRY` remains empty. This slice cannot claim
  worker jobs or consume provider quota unless a later slice explicitly wires
  real gateways and promotes a registry.

Touched files:

```text
rag/src/mm_chat_rag/job_handler_dependencies.py
rag/src/mm_chat_rag/job_handlers.py
rag/tests/unit/test_job_handler_dependencies.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_job_context.py tests/unit/test_job_handlers.py \
  tests/unit/test_job_handler_dependencies.py tests/unit/test_jobs.py
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py \
  tests/unit/test_job_handlers.py tests/unit/test_job_handler_dependencies.py
cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_deployment_boundary.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
```

Result:

```text
43 targeted Python tests passed.
Ruff passed on the admission, skeleton, and parse dependency files.
Mypy passed on the admission, skeleton, and parse dependency source files.
7 production-registry boundary tests passed; registries remain empty.
```

Next G7.5 slice: add the passage-embedding dependency seam for Jina embedding
inputs/vectors and the 1024-dimensional projection completeness path, still
default-off and still without live provider calls.

## 2026-07-15 — G7.5.7 Passage Embedding Dependency Seam

Objective: add the Jina passage-embedding execution seam with 1024-dimensional
vector validation and projection completeness wiring, while keeping production
dispatch empty and avoiding live provider quota.

Implemented behavior:

- Extended `job_handler_dependencies.py` with explicit passage-embedding
  Protocols:
  - `PassageEmbeddingGateway`;
  - `PassageEmbeddingProjectionGateway`.
- Added `PassageEmbeddingHandlerDependencies`; an empty bundle fails closed with
  `JOB_HANDLER_DEPENDENCY_UNCONFIGURED` before candidate fetch, Jina embedding,
  or projection writes.
- Added `passage_embedding_handler_with_dependencies(...)` and
  `admitted_passage_embedding_handler_with_dependencies(...)`:
  - claim-level entry remains wrapped by `with_job_context_admission(...)`;
  - the contextual handler reuses the `passage_embedding` authority fence from
    `job_handlers.py`;
  - fake projection/provider gateways now prove the intended flow without real
    network clients.
- Added typed DTOs for child candidates, provider vectors, and staged embedding
  updates. The seam enforces:
  - model `jina-embeddings-v4`;
  - dimensions exactly `1024`;
  - finite numeric vector lanes;
  - matching provider result count and child IDs;
  - stable SHA-256 over float32 lane bytes before staging;
  - `assert_materialization_search_complete(...)` after staging.
- Invalid vectors, count mismatches, child mismatches, and failed completeness
  checks are redacted into stable error codes and do not expose raw embeddings
  or provider response bodies.
- Production `JOB_HANDLER_REGISTRY` remains empty. This slice cannot claim
  worker jobs or consume provider quota unless a later slice explicitly wires
  real gateways and promotes a registry.

Touched files:

```text
rag/src/mm_chat_rag/job_handler_dependencies.py
rag/tests/unit/test_job_handler_dependencies.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_job_context.py tests/unit/test_job_handlers.py \
  tests/unit/test_job_handler_dependencies.py tests/unit/test_jobs.py
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py \
  tests/unit/test_job_handlers.py tests/unit/test_job_handler_dependencies.py
cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_deployment_boundary.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
```

Result:

```text
51 targeted Python tests passed.
Ruff passed on the admission, skeleton, parse dependency, and embedding
dependency files.
Mypy passed on the admission, skeleton, and dependency source files.
7 production-registry boundary tests passed; registries remain empty.
```

Next G7.5 slice: add the purge dependency seam for immediate invisibility and
projection cleanup, still default-off and without provider credentials.

## 2026-07-16 — G7.5.8 Purge Dependency Seam

Objective: add the purge execution seam for immediate query invisibility and
projection cleanup while keeping production dispatch empty and avoiding any
provider credentials.

Implemented behavior:

- Extended `job_handler_dependencies.py` with a purge-only
  `PurgeProjectionGateway`.
- Added `PurgeHandlerDependencies`; an empty bundle fails closed with
  `JOB_HANDLER_DEPENDENCY_UNCONFIGURED` before any projection call.
- Added `purge_handler_with_dependencies(...)` and
  `admitted_purge_handler_with_dependencies(...)`:
  - claim-level entry remains wrapped by `with_job_context_admission(...)`;
  - the contextual handler reuses the purge authority fence from
    `job_handlers.py`, so purge remains credential-free and provider authority
    is forbidden.
- Added typed DTOs for purge invisibility and projection cleanup proof.
- The fake execution path now proves the required sequence:
  1. `mark_purge_invisible(...)` must prove the tombstoned document version is
     no longer query-visible using the admitted collection/document visibility
     epochs;
  2. `purge_search_projection(...)` must remove or mark ready search rows for
     the admitted Generation/Materialization scope;
  3. `assert_purge_complete(...)` must confirm the cleanup before success.
- Query-visible results, document/epoch mismatches, remaining ready child search
  rows, materialization mismatches, and failed completion checks are redacted
  into stable error codes.
- Production `JOB_HANDLER_REGISTRY` remains empty. This slice cannot claim
  worker jobs unless a later slice explicitly wires the real Postgres gateway
  and promotes a registry.

Touched files:

```text
rag/src/mm_chat_rag/job_handler_dependencies.py
rag/tests/unit/test_job_handler_dependencies.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification run during the slice:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_job_context.py tests/unit/test_job_handlers.py \
  tests/unit/test_job_handler_dependencies.py tests/unit/test_jobs.py
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py \
  tests/unit/test_job_handlers.py tests/unit/test_job_handler_dependencies.py
cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_deployment_boundary.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
```

Result:

```text
60 targeted Python tests passed.
Ruff passed on the admission, skeleton, and dependency files.
Mypy passed on the admission, skeleton, and dependency source files.
7 production-registry boundary tests passed; registries remain empty.
```

Next G7.5 slice: add the first real Postgres projection gateway adapter behind
one seam, still default-off, then promote only a single stage behind explicit
readiness and registry gates.

## 2026-07-16 — G7.5.9 Default-off Postgres Purge Projection Gateway Adapter

Objective: attach the first real Postgres projection gateway behind one admitted
handler seam without promoting production dispatch or touching provider quota.
The selected stage is `purge` because it is credential-free and exercises the
immediate-invisibility/delete contract before MinerU/Jina adapters are enabled.

Implemented behavior:

- Added a typed `lease_token` fence to `ProcessingJobContext` when the database
  claim row provides it. Legacy/fake contexts can still omit it, but the real
  Postgres purge adapter fails closed before database I/O with
  `JOB_CONTEXT_LEASE_FENCE_MISSING`.
- Added migration `014_rag_purge_projection_gateway` with worker-execute-only
  stored-function contracts:
  - `knowledge_mark_purge_invisible(...)` verifies the admitted purge job is
    token-fenced and returns whether the target document version is still
    query-visible;
  - `knowledge_purge_search_projection(...)` marks matching
    `knowledge_child_search_projections` rows as `purged` for the admitted
    Generation and optional Materialization scope;
  - `knowledge_assert_purge_complete(...)` rejects if any matching `ready`
    child search projection row remains.
- Added `PostgresAdapter` methods implementing the existing
  `PurgeProjectionGateway` protocol by calling only the new frozen
  stored-function surface. No base-table DML was added to Python.
- Added unit/static coverage for the Python gateway call parameters, missing
  lease fence behavior, SQL allowlist, and migration privilege/rollback
  contract.
- Production `JOB_HANDLER_REGISTRY` and `DISPATCH_REGISTRY` remain empty. This
  slice provides a default-off adapter only; it cannot claim purge work until a
  later explicit readiness/registry promotion slice.

Touched files:

```text
backend/migrations/014_rag_purge_projection_gateway.up.sql
backend/migrations/014_rag_purge_projection_gateway.down.sql
backend/internal/migration/phase15_rag_purge_projection_gateway_schema_test.go
rag/src/mm_chat_rag/job_context.py
rag/src/mm_chat_rag/postgres.py
rag/tests/unit/test_job_context.py
rag/tests/unit/test_postgres.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_job_context.py tests/unit/test_job_handlers.py \
  tests/unit/test_job_handler_dependencies.py tests/unit/test_postgres.py \
  tests/unit/test_jobs.py
# 80 passed

cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py \
  src/mm_chat_rag/postgres.py tests/unit/test_job_context.py \
  tests/unit/test_job_handlers.py tests/unit/test_job_handler_dependencies.py \
  tests/unit/test_postgres.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/job_context.py src/mm_chat_rag/handlers.py \
  src/mm_chat_rag/job_handlers.py src/mm_chat_rag/job_handler_dependencies.py \
  src/mm_chat_rag/postgres.py
# passed

cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/migration \
  -run 'TestPhase15RAG(PurgeProjectionGateway|SearchProjection|WorkerProjectionGate)'
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_deployment_boundary.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
# 7 passed; production registries remain empty

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed

# secret scan for owner-provided provider endpoint/key; patterns redacted
# no matches

git diff --check -- mm-chat
# passed
```

Residual risk:

- The new purge stored functions are static/unit covered in this slice; a live
  Postgres migration/integration run remains for the later promotion gate.
- Readiness is intentionally not expanded to require these functions yet,
  because the real purge handler registry is still default-off.

Next G7.5 slice: add one real parse-side gateway adapter or finish the purge
promotion gate, but only after a dedicated readiness/registry plan keeps the
single-stage blast radius narrow.

## 2026-07-16 — G7.5.10 Live Postgres Purge Projection Gateway Gate

Objective: prove the default-off purge Postgres gateway against a real
PostgreSQL 16 migration chain before any purge handler promotion. The gate must
be safe on machines without a test database, so it uses
`MM_CHAT_TEST_DATABASE_URL` and skips when that variable is absent.

Implemented behavior:

- Added
  `backend/internal/migration/phase15_rag_purge_projection_gateway_integration_test.go`.
- The integration test applies embedded migrations `001-014`, then seeds the
  smallest valid active corpus Generation, published Materialization,
  Parent/Child Chunk, Jina 1024 Search Projection row, and token-fenced purge
  `knowledge_processing_jobs` row.
- The test verifies the complete purge gateway order:
  1. `rag_worker_executor` can execute the worker-only purge gateway functions;
  2. `rag_worker_executor` cannot mutate `knowledge_child_search_projections`
     base rows directly;
  3. a stale lease token fails closed with `RAG_STALE_JOB_LEASE`;
  4. an active document is query-visible before tombstone;
  5. the same document becomes query-invisible after tombstone;
  6. purge completion rejects while a ready search row remains;
  7. `knowledge_purge_search_projection(...)` marks the ready row `purged`;
  8. `knowledge_assert_purge_complete(...)` succeeds after no ready rows
     remain.
- The first live run exposed a PL/pgSQL ambiguity in
  `knowledge_mark_purge_invisible(...)`: output table columns such as
  `collection_id` shadowed unqualified base-table columns. Migration `014` now
  aliases `knowledge_processing_jobs` as `processing_job` and qualifies the
  lease/admission predicate.
- Production `JOB_HANDLER_REGISTRY` and `DISPATCH_REGISTRY` remain empty. This
  slice is proof only, not dispatch promotion.

Touched files:

```text
backend/migrations/014_rag_purge_projection_gateway.up.sql
backend/internal/migration/phase15_rag_purge_projection_gateway_schema_test.go
backend/internal/migration/phase15_rag_purge_projection_gateway_integration_test.go
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/migration \
  -run TestPhase15RAGPurgeProjectionGatewayLivePostgres -count=1 -v
# skipped when MM_CHAT_TEST_DATABASE_URL is absent

docker run --rm -d --name mm-chat-pg-g7510 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=mm_chat \
  -p 127.0.0.1:15432:5432 postgres:16-alpine
cd mm-chat/backend && MM_CHAT_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  GOCACHE=/tmp/neo-chat-go-build \
  go test ./internal/migration \
    -run 'TestPhase15RAG(PurgeProjectionGateway|SearchProjection|WorkerProjectionGate)' \
    -count=1 -v
# PASS
docker rm -f mm-chat-pg-g7510
```

Residual risk:

- This proves the purge gateway surface only. Parse object-store/MinerU
  adapters, passage Jina embedding adapters, dispatch registry promotion,
  retry/DLQ behavior, and query/citation surfaces remain gated future slices.

## 2026-07-16 — G7.5.11 Default-off Postgres Passage Embedding Projection Gateway

Objective: attach the Postgres fetch/stage half of the
`passage_embedding` handler seam without calling Jina or promoting production
dispatch. This keeps the blast radius to one stage-specific database gateway.

Implemented behavior:

- Added migration `015_rag_passage_embedding_projection_gateway` with two
  worker-execute-only functions:
  - `knowledge_fetch_passage_embedding_candidates(...)` validates the admitted
    `passage_embedding` Job lease, Generation, and Materialization, then
    returns deterministic child text candidates from the search projection.
  - `knowledge_stage_passage_embedding(...)` validates the same lease fence,
    enforces `jina-embeddings-v4`, exactly `1024` REAL lanes, and a redacted
    vector hash, then marks the matching child search projection row `ready`.
- Fetch includes both `staging` and `ready` search rows. That makes a replay
  after a partial stage idempotent: the handler re-embeds the full
  Materialization set and the existing completeness gate still receives the
  full expected child count.
- Extended `PostgresAdapter` with
  `fetch_passage_embedding_candidates(...)` and
  `stage_passage_embeddings(...)`, while preserving the existing
  `knowledge_assert_materialization_search_complete(...)` call for final
  completeness.
- Kept production `JOB_HANDLER_REGISTRY` and `DISPATCH_REGISTRY` empty. This
  slice enables only a default-off projection gateway; it does not spend Jina
  quota and does not claim live work.

Touched files:

```text
backend/migrations/015_rag_passage_embedding_projection_gateway.up.sql
backend/migrations/015_rag_passage_embedding_projection_gateway.down.sql
backend/internal/migration/phase15_rag_passage_embedding_projection_gateway_schema_test.go
backend/internal/migration/phase15_rag_purge_projection_gateway_integration_test.go
rag/src/mm_chat_rag/postgres.py
rag/tests/unit/test_postgres.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/migration \
  -run 'TestPhase15RAG(PassageEmbeddingProjectionGateway|PurgeProjectionGateway|SearchProjection|WorkerProjectionGate)' \
  -count=1 -v
# PASS, with live Postgres test skipped when MM_CHAT_TEST_DATABASE_URL is absent

docker run --rm -d --name mm-chat-pg-g7511 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=mm_chat \
  -p 127.0.0.1:15433:5432 postgres:16-alpine
cd mm-chat/backend && MM_CHAT_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15433/mm_chat?sslmode=disable' \
  GOCACHE=/tmp/neo-chat-go-build \
  go test ./internal/migration \
  -run 'TestPhase15RAG(PassageEmbeddingProjectionGateway|PurgeProjectionGateway)' \
  -count=1 -v
# PASS; migration 015 compiles in the live 001-latest chain
docker rm -f mm-chat-pg-g7511

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_postgres.py tests/unit/test_job_handler_dependencies.py
# 46 passed

cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/postgres.py tests/unit/test_postgres.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/postgres.py src/mm_chat_rag/job_handler_dependencies.py
# passed
```

Residual risk:

- The real Jina passage embedding provider gateway is still absent, so this
  cannot embed without fake/unit gateways.
- Parse object-store/MinerU/Postgres staging, purge/embedding registry
  promotion, retry/DLQ behavior, and query/citation surfaces remain future
  gated slices.

## 2026-07-16 — G7.5.12 Default-off Jina Passage Embedding Provider Gateway

Objective: attach the real Jina passage-embedding provider half behind the
existing dependency-injected seam without promoting production dispatch or
spending live provider quota during tests.

Implemented behavior:

- Added `rag/src/mm_chat_rag/jina_gateway.py` with
  `JinaPassageEmbeddingGateway`. The gateway matches the existing
  `PassageEmbeddingGateway` protocol and remains default-off because no
  production `JOB_HANDLER_REGISTRY` entry references it.
- The gateway requires an explicit constructor-injected API key. It does not
  read `.env`, `os.environ`, browser BYOK state, or the legacy root project;
  missing, whitespace-padded, non-visible-ASCII, or oversized credentials fail
  before HTTP with `JINA_GATEWAY_CREDENTIALS_MISSING`.
- The Jina request is locked to:
  - `POST https://api.jina.ai/v1/embeddings`;
  - model `jina-embeddings-v4`;
  - dimensions `1024`;
  - task `retrieval.passage`;
  - `embedding_type=float`, no multivector, no tokenized input, no truncation.
- The response path validates model, item count, unique indexes, vector length,
  and finite numeric lanes before returning `PassageEmbeddingVector` values
  bound to the original child chunk IDs.
- Redaction/failure policy:
  - provider status and transport failures become stable retryable job errors
    (`JINA_GATEWAY_STATUS_INVALID` / `JINA_GATEWAY_REQUEST_FAILED`) so the
    durable worker can apply the configured three-attempt retry budget once the
    handler is promoted;
  - malformed provider shape, count mismatch, invalid vector dimensions, and
    non-finite vectors become stable permanent errors without raw provider
    bodies, request IDs, credentials, or embeddings.
- Promoted `httpx==0.28.1` from dev-only usage to a runtime dependency and
  refreshed `uv.lock`, because `mm_chat_rag.jina_gateway` is installed package
  code rather than an operator-only tool.

Touched files:

```text
rag/pyproject.toml
rag/uv.lock
rag/src/mm_chat_rag/jina_gateway.py
rag/tests/unit/test_jina_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/jina_gateway.py tests/unit/test_jina_gateway.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/jina_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_jina_gateway.py \
  tests/unit/test_job_handler_dependencies.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
# 33 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- The gateway is real but still not connected to worker settings or a promoted
  handler registry, so no live job can call it yet.
- Parse-side object storage, MinerU parsing, parse projection staging, handler
  promotion, and live provider smoke remain future gated slices.
- The first real quota-consuming Jina call is deferred to a later explicit live
  smoke/promote gate with redacted evidence.

## 2026-07-16 — G7.5.13 Jina + Projection Handler Dependency Bundle

Objective: compose the real Jina passage-embedding provider gateway with the
existing passage-embedding projection gateway seam under an explicit default-off
dependency bundle, without promoting production job handlers or touching live
provider quota.

Implemented behavior:

- Added `build_jina_passage_embedding_handler_dependencies(...)` in
  `rag/src/mm_chat_rag/jina_gateway.py`. The function builds a
  `PassageEmbeddingHandlerDependencies` bundle from:
  - an explicitly supplied Jina API key;
  - an explicitly supplied `PassageEmbeddingProjectionGateway`;
  - an optional injected `httpx.AsyncClient` for tests or later controlled
    runtime ownership.
- The bundle is fail-closed and default-off:
  - no production `JOB_HANDLER_REGISTRY` or `DISPATCH_REGISTRY` entry references
    it;
  - missing projection dependency raises `JOB_HANDLER_DEPENDENCY_UNCONFIGURED`
    before constructing a provider request or making HTTP;
  - credentials remain explicit input and are never read from `.env`, process
    environment, browser BYOK state, or old root-project files.
- Extended `rag/tests/unit/test_jina_gateway.py` with a full admitted handler
  proof using the real Jina gateway over `httpx.MockTransport` and a fake
  projection gateway. The test verifies:
  1. admission accepts a valid `passage_embedding` job under the locked provider
     profile;
  2. projection candidates are fetched first;
  3. one locked Jina embedding request is made;
  4. staged vectors retain the candidate child IDs, model
     `jina-embeddings-v4`, dimensions `1024`, and redacted vector hashes;
  5. materialization completeness is asserted with the expected child count.

Touched files:

```text
rag/src/mm_chat_rag/jina_gateway.py
rag/tests/unit/test_jina_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/jina_gateway.py tests/unit/test_jina_gateway.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/jina_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_jina_gateway.py \
  tests/unit/test_job_handler_dependencies.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
# 35 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This composes the dependency bundle only; worker settings and production
  handler registry promotion remain gated future work.
- The test uses `httpx.MockTransport`, so no live provider quota is consumed.
- Parse-side object storage, MinerU parsing, parse projection staging, purge
  promotion, and live smoke remain future slices.

## 2026-07-16 — G7.5.14 Parse Source Gateway Composition Seam

Objective: give the parse handler a real, testable source-fetch composition
boundary before attaching Postgres file metadata or MinIO/S3 credentials. This
keeps the slice default-off and avoids expanding blast radius into object-store
networking.

Implemented behavior:

- Added `rag/src/mm_chat_rag/source_gateway.py` with:
  - `FileSourceMetadata`, the validated file/object metadata contract for one
    admitted parse job;
  - `SourceMetadataGateway`, a future Postgres/token-fenced metadata seam;
  - `ObjectBytesGateway`, a future local/MinIO/S3 byte-reader seam;
  - `ObjectStoreDocumentSourceGateway`, the composition gateway that implements
    the existing `DocumentSourceGateway` behavior expected by parse handler
    dependencies.
- Validation and authority checks are fail closed:
  - file id must be a nonzero UUID and must match `ProcessingJobContext.file_id`;
  - storage backend is limited to `local|minio|s3`;
  - object keys must be internal object-store keys, not paths, URLs, traversal
    segments, Windows paths, or colon-bearing drive/URL strings;
  - source bytes must match metadata byte size and SHA-256 before returning a
    `DocumentSource`;
  - dependency absence fails with `JOB_HANDLER_DEPENDENCY_UNCONFIGURED` before
    metadata or object reads.
- Added `rag/tests/unit/test_source_gateway.py` covering success, default-off
  behavior, unsafe object keys, invalid metadata fields, file-id mismatch before
  object read, size mismatch, hash mismatch, and redacted stable errors.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice adds no MinIO/S3 SDK, no `.env` reads, and no live object-store calls.

Touched files:

```text
rag/src/mm_chat_rag/source_gateway.py
rag/tests/unit/test_source_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/source_gateway.py tests/unit/test_source_gateway.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/source_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_source_gateway.py \
  tests/unit/test_job_handler_dependencies.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
# 44 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This is a composition seam only. Postgres metadata retrieval and real
  local/MinIO/S3 byte adapters are still future gated slices.
- Parse provider execution remains blocked until MinerU and parse projection
  gateways are implemented and explicitly promoted.
- The gateway currently materializes source bytes in memory; a later object-byte
  adapter may stream internally but must still return bounded verified bytes to
  the current parser contract.

## 2026-07-16 — G7.5.15 Default-off Postgres Parse Source Metadata Gateway

Objective: attach the Postgres metadata half of the parse source gateway without
reading object bytes, calling MinIO/S3, calling MinerU, or promoting production
dispatch. This keeps the slice limited to one token-fenced database function and
one Python adapter method.

Implemented behavior:

- Added migration `016_rag_parse_source_metadata_gateway` with
  `knowledge_fetch_parse_source_metadata(...)`, a `rag_worker_executor`-only
  SECURITY DEFINER function.
- The function validates the live parse job fence before returning metadata:
  - `status = processing`, `stage = parse`, operation in
    `initial|replace|reprocess`;
  - non-legacy projection binding, matching `file_id`, matching
    `materialization_id`, non-null Generation, matching worker id, lease token,
    and unexpired lease;
  - available, non-deleted, non-empty file metadata;
  - file SHA-256 equals the document-version content hash and the staging
    materialization source hash;
  - collection ACL/visibility/processing revision and document visibility fences
    still match the admitted job.
- Extended `rag/src/mm_chat_rag/postgres.py` with
  `fetch_source_metadata(...)`. It uses the same `_call(...)` function allowlist,
  requires the admitted lease token and materialization id before DB I/O, and
  converts the returned row into validated `FileSourceMetadata`.
- Extended `rag/tests/unit/test_postgres.py` with success, lease-token missing,
  materialization missing, invalid-row, SQL allowlist, and parameter-fence
  coverage.
- Added static migration contract coverage in
  `backend/internal/migration/phase15_rag_parse_source_metadata_gateway_schema_test.go`.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty; this
  slice cannot spend provider quota and does not touch deployment `.env` values.

Touched files:

```text
backend/migrations/016_rag_parse_source_metadata_gateway.up.sql
backend/migrations/016_rag_parse_source_metadata_gateway.down.sql
backend/internal/migration/phase15_rag_parse_source_metadata_gateway_schema_test.go
rag/src/mm_chat_rag/postgres.py
rag/tests/unit/test_postgres.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/migration \
  -run 'TestPhase15RAG(ParseSourceMetadataGateway|PassageEmbeddingProjectionGateway|PurgeProjectionGateway|SearchProjection|WorkerProjectionGate)' \
  -count=1
# PASS

docker run --rm -d --name mm-chat-pg-g7515 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=mm_chat \
  -p 127.0.0.1:15436:5432 postgres:16-alpine
cd mm-chat/backend && MIGRATION_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15436/mm_chat?sslmode=disable' \
  GOCACHE=/tmp/neo-chat-go-build \
  go run ./cmd/migrate up
# PASS; applied through 016_rag_parse_source_metadata_gateway
docker rm -f mm-chat-pg-g7515

cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/postgres.py tests/unit/test_postgres.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/postgres.py src/mm_chat_rag/source_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_postgres.py \
  tests/unit/test_source_gateway.py \
  tests/unit/test_job_handler_dependencies.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
# 70 passed
```

Residual risk:

- This proves metadata retrieval only. Real object-byte adapters, MinerU parser
  gateway, parse projection staging, and parse handler registry promotion remain
  future gated slices.
- The new migration compiles in a fresh live PostgreSQL chain, but this slice
  does not seed a full parse job fixture or fetch real object bytes.

## 2026-07-16 — G7.5.16 Default-off Local Object Bytes Gateway

Objective: attach the first real object-byte reader behind the parse source
composition seam without reading deployment `.env`, adding MinIO/S3 credentials,
calling MinerU, or promoting production dispatch. This slice covers the local
filesystem storage backend only; MinIO/S3 remain later gated adapters.

Implemented behavior:

- Added `LocalObjectBytesGateway` in `rag/src/mm_chat_rag/source_gateway.py`.
- The gateway is explicit and default-off:
  - it requires a caller-supplied existing root directory;
  - it only accepts `FileSourceMetadata.storage_backend == "local"`;
  - it reuses the internal object-key safety rules and resolves paths under the
    configured root;
  - it rejects missing objects, directories, symlink objects, path escapes, and
    size mismatches with stable redacted errors;
  - it checks object size before materializing bytes and leaves final SHA-256
    verification to `ObjectStoreDocumentSourceGateway`.
- Extended `rag/tests/unit/test_source_gateway.py` with local read success
  through the full source composition gateway, missing-root, non-local backend,
  size mismatch, and symlink rejection coverage.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota can be spent by this slice.

Touched files:

```text
rag/src/mm_chat_rag/source_gateway.py
rag/tests/unit/test_source_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/source_gateway.py tests/unit/test_source_gateway.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/source_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_source_gateway.py \
  tests/unit/test_postgres.py \
  tests/unit/test_job_handler_dependencies.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
# 75 passed
```

Residual risk:

- This enables only local filesystem object reads. Production single-server
  deployments using MinIO/S3 still need a separate credentialed object-byte
  adapter slice and smoke test.
- Parse execution remains blocked until the MinerU parser gateway, parse
  projection gateway, and explicit registry promotion gates are implemented.

## 2026-07-16 — G7.5.17 Go Private Source Object Gateway + Python HTTP Adapter Seam

Objective: replace the planned direct Python MinIO/S3 byte access path with a
Go-owned private source-object gateway. Go remains the authority for Postgres
lease validation, object-store access, size checks, and SHA-256 verification;
Python remains a default-off worker client that can only request bytes for its
currently leased parse job.

Implemented behavior:

- Added `backend/internal/ragsource` with a token-gated internal route contract:
  `POST /internal/rag/source-object` and header
  `X-MM-Chat-Internal-Token`.
- The Go service calls `knowledge_fetch_parse_source_metadata(...)`, validates
  the returned metadata, reads the object through the existing
  `storage.ObjectStore`, checks `Content-Length`/object size and SHA-256, and
  returns only raw bytes plus redacted source headers.
- Wired the route into `httpserver` so auth-required deployments bypass normal
  bearer-session middleware only for this exact internal POST path; the handler
  still rejects missing/wrong internal tokens before parsing job JSON.
- Added `RAG_SOURCE_GATEWAY_TOKEN` to backend config and single-server Compose.
  Blank token keeps the gateway default-off.
- Added `GoSourceObjectBytesGateway` in `rag/src/mm_chat_rag/source_gateway.py`.
  It sends job id, worker id, lease token, file id, and materialization id to
  Go, rejects missing lease/materialization before HTTP, maps 5xx/transport
  failures to retryable stable errors, and revalidates file id, source SHA-256,
  response length, body length, and body hash.
- Updated the source gateway protocol so object-byte readers receive both the
  admitted `ProcessingJobContext` and `FileSourceMetadata`. The local reader
  remains default-off and keeps the same local-only checks.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  MinerU/Jina call or provider quota is consumed by this slice.

Touched files:

```text
backend/internal/ragsource/types.go
backend/internal/ragsource/repository.go
backend/internal/ragsource/service.go
backend/internal/ragsource/handler.go
backend/internal/ragsource/service_handler_test.go
backend/internal/httpserver/server.go
backend/internal/httpserver/server_test.go
backend/internal/config/config.go
backend/internal/config/config_test.go
backend/cmd/api/main.go
backend/.env.example
compose.single-server.yml
.env.single-server.example
rag/src/mm_chat_rag/source_gateway.py
rag/tests/unit/test_source_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/ragsource ./internal/httpserver ./internal/config ./cmd/api -count=1
# passed

cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/source_gateway.py tests/unit/test_source_gateway.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/source_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_source_gateway.py
# 37 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_source_gateway.py \
  tests/unit/test_job_handler_dependencies.py \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty
# 62 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This is still a default-off source-byte seam. Real parse dispatch remains
  blocked until MinerU parsing, parse artifact projection, handler composition,
  and explicit registry promotion are implemented.
- The Go gateway currently materializes source bytes in memory after enforcing a
  512 MiB cap. Large-document streaming can be optimized later without changing
  the authority boundary.
- `RAG_SOURCE_GATEWAY_TOKEN` must be provided to both backend and worker in a
  real deployment; it must never be logged or committed with a real value.

## 2026-07-16 — G7.5.18 Default-off Postgres Parse Projection Adapter Seam

Objective: add the Python-side Postgres adapter seam for parse projection
staging without creating the DB function yet, promoting handlers, calling MinerU
or Jina, or consuming provider quota. This slice makes the future staging
function call shape explicit and testable while keeping production registries
empty.

Implemented behavior:

- Added `PostgresAdapter.stage_parse_projection(...)` in
  `rag/src/mm_chat_rag/postgres.py`.
- The adapter requires the claimed job lease token and materialization id before
  any DB call.
- It serializes one `PostgresProjectionBatch` into five JSONB lanes: blocks,
  parent chunks, child chunks, chunk-block spans, and child search projections.
- It checks that batch rows match the admitted processing context for
  materialization, index generation, collection, document, and document version
  where applicable. Mismatches fail closed before touching Postgres.
- UUID and Decimal values are converted to JSON-safe primitives before wrapping
  payloads in `psycopg.types.json.Jsonb`.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  parser, embedding, or provider quota path is enabled by this slice.

Touched files:

```text
rag/src/mm_chat_rag/postgres.py
rag/tests/unit/test_postgres.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/postgres.py tests/unit/test_postgres.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/postgres.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_postgres.py
# 30 passed
```

Residual risk:

- `knowledge_stage_parse_projection(...)` is not created by this slice. Live DB
  parse staging must wait for the next migration/function cut and its compile or
  integration proof.
- Real parse dispatch remains blocked until MinerU execution, parse projection
  staging, handler composition, retry behavior, and explicit registry promotion
  gates are implemented.

## 2026-07-16 — G7.5.19 Default-off Postgres Parse Projection Gateway Function

Objective: add the Postgres `knowledge_stage_parse_projection(...)` function
that the Python adapter from G7.5.18 calls, while keeping production handler
registries empty and avoiding MinerU/Jina provider calls.

Implemented behavior:

- Added migration `017_rag_parse_projection_gateway` with an up/down pair.
- The function is a token-fenced `SECURITY DEFINER` worker gateway requiring a
  live parse job lease, non-legacy projection binding, staging materialization,
  source hash match, chunk-profile match, and Jina 1024 search profile.
- It links `parse_artifact_set_id` onto the staging materialization, inserts or
  reuses the parser artifact set, and stages five projection lanes from JSONB
  recordsets: blocks, parent chunks, child chunks, chunk-block spans, and child
  search projections.
- Each lane has a count/mismatch gate so partial or context-mismatched payloads
  fail closed instead of being treated as success.
- Added a static migration contract test covering token fences, materialization
  and profile gates, JSONB recordset lanes, artifact/search-profile binding,
  worker-only execute grants, and rollback.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  parser, embedding, or provider quota path is enabled by this slice.

Touched files:

```text
backend/migrations/017_rag_parse_projection_gateway.up.sql
backend/migrations/017_rag_parse_projection_gateway.down.sql
backend/internal/migration/phase15_rag_parse_projection_gateway_schema_test.go
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/migration -count=1
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed

# secret scan for owner-provided provider endpoint/key; patterns redacted
# no matches

git diff --check -- mm-chat
# passed
```

Residual risk:

- `MM_CHAT_TEST_DATABASE_URL` is not configured in this shell, so this slice has
  static migration contract coverage but not a live Postgres compile/staging
  proof yet.
- Real parse dispatch remains blocked until MinerU execution, handler
  composition, retry behavior, and explicit registry promotion gates are
  implemented.

## 2026-07-16 — G7.5.20 Default-off MinerU Local-batch Allocate Gateway

Objective: add the first evidence-backed MinerU provider gateway slice without
pretending the full parse lifecycle is solved. This slice covers only the
local-batch `allocate_upload` request/response seam; signed upload, polling,
result ZIP download, Canonical IR normalization, and handler promotion remain
later gated work.

Implemented behavior:

- Added `MinerULocalBatchGateway` in `rag/src/mm_chat_rag/mineru_gateway.py`.
- The gateway requires an explicitly injected admin MinerU token and validates it
  before any HTTP call.
- It accepts only `DocumentSource` values with `application/pdf` content type and
  enforces the public 200 MiB MinerU local-batch file limit before network I/O.
- It sends one locked local-batch allocate request to
  `POST https://mineru.net/api/v4/file-urls/batch` with `is_ocr`,
  `enable_formula`, and `enable_table` enabled and `model_version=vlm`.
- It rejects unsafe filenames before network I/O, rejects malformed provider
  JSON, requires exactly one signed HTTPS upload URL, and maps provider status,
  provider `code` failure, and transport failure to stable redacted retryable
  errors.
- Added unit coverage for missing-token no-HTTP behavior, locked request shape,
  PDF/content-size/filename fences, provider status failure, provider code
  failure, transport failure, invalid payloads, and unsafe signed upload URLs.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 13 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed

# secret scan for owner-provided provider endpoint/key; patterns redacted
# no matches

git diff --check -- mm-chat
# passed
```

Residual risk:

- This is not a complete MinerU parser gateway. Signed upload, poll/result,
  result ZIP validation, Canonical IR/chunk manifest mapping, and parse-handler
  composition are still required before parse dispatch can be promoted.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.21 Default-off MinerU Signed-upload Transport Seam

Objective: add the next narrow MinerU local-batch seam without promoting parse
dispatch or claiming the full provider lifecycle is complete. This slice covers
only the provider-derived signed-upload `PUT` transport after allocation; polling,
result download, ZIP validation, Canonical IR normalization, and handler
composition remain later gated cuts.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with `upload_document(...)`.
- The upload path revalidates the source as `application/pdf` before network I/O
  and sends the exact admitted bytes as the raw `PUT` body.
- The signed upload request intentionally omits `Authorization`, `Cookie`, and
  `Content-Type` headers so provider credentials cannot leak to the object
  upload target.
- The upload target is validated before HTTP: `https`, exact host
  `mineru.oss-cn-shanghai.aliyuncs.com`, default/443 port only,
  `/api-upload/` path prefix, no userinfo, no fragment, required signed query,
  visible ASCII only, no encoded/traversal path, and URL length at most 4096
  bytes.
- Upload `200` and `204` responses are accepted; non-accepted statuses and
  transport failures become stable redacted retryable errors.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy src/mm_chat_rag/mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 25 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed
```

Residual risk:

- This is still not a complete MinerU parser gateway. Poll/result, result ZIP
  validation, Canonical IR/chunk manifest mapping, and parse-handler composition
  are still required before parse dispatch can be promoted.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.22 Default-off MinerU Batch Poll/Result Seam

Objective: add the next narrow MinerU local-batch seam after allocation and
signed upload without downloading result ZIPs or promoting parse dispatch. This
slice covers one poll request plus closed result-state parsing only.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with `poll_batch_result(...)`.
- Allocation now preserves the safe filename used in the allocate request so
  the poll response must match the exact single-file batch.
- The poll target is constructed from the validated batch id only:
  `GET https://mineru.net/api/v4/extract-results/batch/{batch_id}`.
- Poll requests send the admin MinerU bearer token with `Accept:
application/json` and `Accept-Encoding: identity`; they send no body and no
  `Content-Type`.
- Poll response parsing is closed and single-file: matching batch id, exactly
  one `extract_result`, matching filename, states limited to
  `waiting-file`, `pending`, `running`, `converting`, `done`, or `failed`,
  optional bounded `data_id`, and running-progress bounds.
- The existing signed-upload target gate is tightened to require a signed query
  and reject encoded/traversal path drift before HTTP.
- `done` requires a result ZIP URL and validates the download target before
  exposing it to later slices: exact host `cdn-mineru.openxlab.org.cn`,
  `/pdf/` prefix, `.zip` suffix, default/443 port, no query/userinfo/fragment,
  visible ASCII only, no encoded/traversal path, and at most 4096 bytes.
- Provider HTTP status, provider `code` failure, and transport failures map to
  stable redacted retryable errors. Malformed poll/result shapes fail closed
  without logging batch ids, URLs, provider errors, or trace details.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 43 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed
```

Residual risk:

- This is still not a complete MinerU parser gateway. Result ZIP download,
  archive validation, Canonical IR/chunk manifest mapping, and parse-handler
  composition are still required before parse dispatch can be promoted.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.23 Default-off MinerU Result ZIP Download Seam

Objective: add the next narrow MinerU local-batch seam after a `done` poll
result without parsing ZIP entries or promoting parse dispatch. This slice
downloads a bounded archive body only; archive validation and Canonical IR
mapping remain later cuts.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with `download_result_archive(...)`.
- The download path accepts only a `MinerULocalBatchPollResult` in `done` state
  with a result URL that passes the locked MinerU CDN target gate.
- Download requests send `Accept: application/zip` and `Accept-Encoding:
identity`; they send no bearer token, no cookie, and no `Content-Type`.
  Injected client cookies are cleared before the dynamic CDN request.
- Download responses require HTTP `200`, identity/no content encoding,
  allowlisted ZIP content type, valid decimal `Content-Length` when present,
  and a compressed body no larger than 32 MiB.
- Provider status and transport failures map to stable redacted retryable
  errors. Invalid response headers or oversized bodies fail closed without
  logging URLs, provider body details, or transport messages.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 52 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed
```

Residual risk:

- This is still not a complete MinerU parser gateway. Result ZIP entry
  validation, Canonical IR/chunk manifest mapping, and parse-handler composition
  are still required before parse dispatch can be promoted.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.24 Default-off MinerU Result ZIP Archive Validation

Objective: validate downloaded MinerU result ZIP structure before any Canonical
IR mapping or handler promotion. This slice proves archive safety and required
artifact-role presence only; it does not read or retain entry content and does
not normalize provider output.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with `validate_result_archive(...)`.
- The archive seam accepts already downloaded ZIP bytes and returns a redacted
  `MinerULocalBatchArchiveSummary`: compressed byte count, archive SHA-256,
  entry count, and presence booleans for full Markdown, content-list JSON,
  middle/layout JSON, and model JSON.
- The validator rejects empty/non-ZIP bodies, oversized compressed archives,
  too many entries, oversized expanded entries, oversized total expanded bytes,
  suspicious compression ratios, CRC mismatches, duplicate names, encrypted
  entries, symlink entries, absolute paths, traversal paths, empty path
  segments, and backslash paths.
- Required MinerU roles follow the captured Cloud v4 shape: `full.md`,
  `content_list.json` or `*_content_list.json`, `layout.json` or
  `middle.json`/`*_middle.json`, and `model.json` or `*_model.json`.
- No entry names or content are retained outside transient validation; the
  returned summary is safe for later projection/admission logs.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 64 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed
```

Residual risk:

- This is still not a complete MinerU parser gateway. Canonical IR/chunk
  manifest mapping and parse-handler composition are still required before parse
  dispatch can be promoted.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5A MinerU Text-Baseline Locator Hardening Closure

Objective: consolidate the remaining MinerU text-baseline locator boundary work
into one medium slice after the owner approved moving faster than one tiny
locator cut at a time, while still avoiding DB/live/provider/handler promotion.

Implemented behavior:

- Added reusable test helpers for constructing locator-focused MinerU archives
  without touching provider quota.
- Added fail-closed coverage for duplicate `content_list` matches.
- Added fail-closed coverage for malformed locator geometry: missing page index,
  negative page index, missing bbox, negative bbox coordinate, zero-area bbox,
  and non-integer bbox coordinate.
- Added formula boundary coverage: formula `sourceText` with matching layout
  `sourceText` may project `page_bbox`, but formula kind-only layout elements
  are not inferred as evidence and fall back to line-range.
- Added ambiguous formula `sourceText` rejection to avoid assigning evidence to
  the wrong formula.
- Added no-content-match fallback coverage: if `content_list` does not agree
  with `full.md`, matching layout geometry is ignored and the text-baseline
  line-range locator is preserved.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 107 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 26 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- G7.5A deliberately stops before DB integration, handler registry gates,
  dispatch/retry/DLQ, and live MinerU/Jina/Postgres smoke.
- Rich formula semantics, table-cell addressing, and image asset materialization
  remain separate future cuts.

## 2026-07-16 — G7.5.35 MinerU Opaque Image Element Page Locator Admission

Objective: extend the conservative page-bbox locator seam to single image
elements while keeping image bytes, paths, and Asset IR out of this slice.

Implemented behavior:

- Reused the element-kind-only locator path for `type=image`/`kind=image` after a
  unique `content_list` full-text match.
- For image baselines, the mapper may match a single `layout/middle` image
  element by semantic kind even when that element has no duplicate full text
  field.
- The admitted output is still only the element-level `page_bbox`; image paths,
  object bytes, captions, OCR regions, and Asset IR are not parsed or persisted.
- Multiple candidate image elements fail closed with
  `MINERU_GATEWAY_ARTIFACT_INVALID` to avoid assigning evidence to the wrong
  image.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 97 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 26 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This is only image element-level citation location. Image asset storage,
  caption/OCR subregions, table-cell addressing, Formula IR, and live Provider
  smoke remain gated later cuts.

## 2026-07-16 — G7.5.34 MinerU Opaque Table Element Page Locator Admission

Objective: extend the conservative page-bbox locator seam to single table
elements while keeping table cells opaque and avoiding Table IR promotion.

Implemented behavior:

- `content_list` matching now records the matched semantic `type`/`kind` instead
  of only a boolean full-text match.
- For `type=table`/`kind=table`, the mapper may match a single
  `layout/middle` table element by semantic kind even when that element has no
  duplicate full text field.
- The admitted output is still only the element-level `page_bbox`; rows and cells
  inside the Provider table payload are not parsed, normalized, or projected.
- Multiple candidate table elements fail closed with
  `MINERU_GATEWAY_ARTIFACT_INVALID` to avoid assigning evidence to the wrong
  table.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 95 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 26 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This is only table element-level citation location. Table-cell addressing,
  structural Table IR, formula/image assets, and live Provider smoke remain
  gated later cuts.

## 2026-07-16 — G7.5.33 MinerU SourceText Page Locator Admission

Objective: extend the conservative G7.5.32 page-bbox locator seam to
formula-like MinerU `sourceText` without promoting Formula IR or live dispatch.

Implemented behavior:

- Reused the same full-text agreement gate for both `text` and `sourceText`
  fields in `content_list` and `layout/middle` elements.
- Added a regression test where a formula-like `sourceText` in `content_list`
  agrees with a `layout/middle` element carrying `pageIndex` and
  `bboxMilliPoint`, producing a projected `page_bbox` locator.
- The mapper still emits the existing text-baseline paragraph/chunk artifacts;
  it does not emit Formula IR, parse LaTeX, map tables/images, or register
  production handlers.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 93 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 26 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This is still a text-baseline locator improvement only. Formula semantics,
  table cells, image assets, and multi-element Markdown mapping remain gated
  later cuts.

## 2026-07-16 — G7.5.32 Conservative MinerU Basic Page Locator Mapper

Objective: add the first citation-grade locator improvement to the MinerU
full-Markdown baseline without claiming table/formula/image semantics or
promoting production dispatch.

Implemented behavior:

- Added `MinerUPageRegionLocator` admission for one page-index plus bbox tuple.
- The full-Markdown baseline mapper now tries to derive a page region only when
  `content_list` has exactly one entry whose `text` equals `full.md`, and the
  `layout/middle` artifact has exactly one element with the same text plus
  `pageIndex` and `bboxMilliPoint`.
- The admitted page region is prepended to block and chunk locator-set views, so
  the existing G7.4 projection chooses `page_bbox` for basic citation cards.
- If no recognized locator evidence exists, the mapper preserves the previous
  safe line-range text locator. If matching locator evidence is ambiguous or
  malformed, the mapper fails closed with `MINERU_GATEWAY_ARTIFACT_INVALID`.
- This slice intentionally does not map tables, formulas, images, multi-element
  Markdown, or real live Provider variants beyond the locked conservative
  shape.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 92 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 26 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This mapper only promotes one conservative page-bbox shape for the text
  baseline. Rich table/formula/image locators and multi-block content-list
  mapping remain later gated cuts.
- No live MinerU quota is consumed by this slice; live Provider smoke remains in
  G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.31 MinerU Parser Adapter Dependency Composition Proof

Objective: prove the real default-off MinerU text-baseline parser adapter fits
the existing parse-handler dependency seam without registering production
handlers or performing provider I/O.

Implemented behavior:

- Added a focused unit test in `test_job_handler_dependencies.py` that injects
  `MinerUTextBaselineArchiveParserGateway` into `ParseHandlerDependencies`.
- The test uses fake document-source and archive-provider gateways, so the flow
  is fully deterministic and performs no external network request.
- The admitted parse handler now has a regression proof for the concrete MinerU
  adapter path: source fetch -> archive fetch -> parser adapter -> G7.4
  projection build -> projection stage.
- Assertions cover successful `JobResult`, source SHA-256 propagation,
  text-baseline parent chunk content, exact-term projection, and side-effect
  order.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/tests/unit/test_job_handler_dependencies.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check tests/unit/test_job_handler_dependencies.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_job_handler_dependencies.py
# 25 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 26 passed

cd mm-chat/frontend && corepack pnpm prettier --check \
  ../docs/architecture/g7-rag-citation-cutover-plan.md \
  ../docs/tracking/g7-rag-citation-process.md \
  ../docs/tracking/progress.md
# passed
```

Residual risk:

- This is a composition proof only; production parse-handler registration and
  real archive retrieval orchestration remain gated.
- Citation-grade page/table/formula/image mapping still requires MinerU
  `content_list`, `layout/middle`, and `model` interpretation in later cuts.

## 2026-07-16 — G7.5.30 Default-off MinerU Text-Baseline Parser Adapter

Objective: expose the existing MinerU archive-to-text-baseline chain through a
ParserGateway-shaped adapter while keeping production dispatch disabled and
archive retrieval injected.

Implemented behavior:

- Added `MinerUResultArchiveProvider`, a narrow async protocol for dependencies
  that can supply already downloaded MinerU result ZIP bytes.
- Added `MinerUTextBaselineArchiveParserGateway.parse_document(...)`.
- The adapter validates `ProcessingJobContext` before dependency access: only
  `stage="parse"` and a non-zero `materialization_id` are admitted.
- Missing archive provider fails closed with
  `MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED` before archive fetch. Invalid context
  fails with `MINERU_GATEWAY_CONTEXT_INVALID` before archive fetch.
- The adapter validates PDF source content type and source body hash before
  calling the injected archive provider.
- The returned `artifact_set_id` is deterministic for the parse materialization,
  source SHA-256, and frozen text-baseline chunk profile hash.
- The adapter reuses the G7.5.29 composition path, so invalid archives still fail
  through the existing ZIP/archive gates and successful output remains compatible
  with G7.4 `build_postgres_projection_batch(...)`.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 90 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed
```

Residual risk:

- This is still a default-off parser adapter over already downloaded archive
  bytes; it does not yet wire parse-handler dependencies or promote production
  dispatch.
- Page/table/formula/image-level citations still require interpreting MinerU
  `content_list`, `layout/middle`, and `model` artifacts in separate mapper cuts.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.29 Default-off MinerU Archive-to-Text-Baseline Composition

Objective: compose the already implemented result-ZIP validation/extraction,
hash-bound mapper input, and full-Markdown text-baseline mapper into one
archive-body seam that returns projection-ready parser artifacts.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with
  `build_text_baseline_parse_artifacts_from_archive(...)`.
- The seam accepts a PDF `DocumentSource`, already downloaded MinerU archive
  bytes, and a worker-owned `artifact_set_id`.
- It runs the existing default-off chain in memory:
  `extract_result_archive_artifacts -> prepare_canonical_mapping_input -> build_text_baseline_parse_artifacts`.
- It reuses all previously gated protections: archive structure validation,
  semantic role extraction, strict JSON/text decoding, source body hash binding,
  and the G7.5.28 projection-ready full-Markdown mapper.
- It performs no HTTP request and does not allocate/upload/poll/download any
  provider resource.
- Unit tests prove successful G7.4 projection, invalid archive rejection, and
  source hash mismatch rejection.
- This slice still does not wire the parser handler dependency bundle, interpret
  `content_list/layout/model` schemas for citation-grade locators, or promote
  any production registry.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 86 passed
```

Residual risk:

- This is still a local archive-body seam, not the final provider orchestration
  or parser-handler registration.
- Page/table/formula/image-level citations still require interpreting MinerU
  `content_list`, `layout/middle`, and `model` artifacts in separate mapper cuts.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.28 Default-off MinerU Full Markdown Text Baseline Mapper

Objective: turn the hash-bound MinerU mapper input into projection-ready parser
artifacts using only `full.md` text, without claiming page/table/formula/image
schema interpretation.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with
  `build_text_baseline_parse_artifacts(...)`.
- The mapper accepts only `MinerULocalBatchCanonicalMappingInput` plus an
  explicit worker-owned `artifact_set_id`.
- It emits `ParsedDocumentArtifacts` with deterministic `canonical-ir.v2` and
  `chunk-manifest.v2` objects:
  - PDF source byte/hash metadata is preserved from the object source.
  - `full.md` becomes one paragraph block in the Canonical IR text buffer.
  - Parent/child chunks are deterministic and use the frozen
    `MINERU_TEXT_BASELINE_CHUNK_PROFILE_HASH`.
  - Locators use text-range anchors over the `full.md` text baseline and avoid
    inventing page bbox evidence.
  - Provenance binds the block to the full-Markdown role digest.
- Long Markdown is split on UTF-8 code point boundaries into bounded child
  chunks, avoiding multibyte split corruption.
- Unit tests feed the emitted artifacts through G7.4
  `build_postgres_projection_batch(...)`, proving the baseline mapper can stage
  rows through the current pure projection model.
- This slice still does not interpret MinerU `content_list`, `layout/middle`, or
  `model` schemas; page/table/formula/image citations remain separate mapper
  cuts.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 83 passed
```

Residual risk:

- This is a text-baseline mapper only. Page/table/formula/image-level citations
  still require Provider artifact schema mapping from `content_list`,
  `layout/middle`, and `model`.
- Parser-handler composition and production registry promotion remain gated.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.27 Default-off MinerU Canonical Mapping Input

Objective: bind decoded MinerU role payloads back to the exact source/archive
and role byte hashes before any Canonical IR/chunk manifest mapper is allowed
to interpret Provider fields.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with
  `prepare_canonical_mapping_input(...)`.
- The seam accepts an admitted PDF `DocumentSource` plus validated MinerU role
  byte artifacts from the extraction path.
- It verifies `DocumentSource.source_sha256` against the actual source body
  SHA-256 before decoding, so stale object metadata cannot enter the mapper
  seam.
- It reuses the G7.5.26 decode gates for strict UTF-8, duplicate-key rejection,
  non-finite JSON rejection, and role-specific top-level checks.
- It returns an in-memory mapper input with source byte/hash metadata, archive
  byte/hash metadata, deterministic role digests in
  `full_markdown/content_list_json/middle_json/model_json` order, and decoded
  role payloads.
- The mapper input intentionally exposes no ZIP entry names, URLs, provider ids,
  `canonical_ir`, or `chunk_manifest`. It does not interpret MinerU schema
  fields or promote any handler.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 80 passed
```

Residual risk:

- This is still not a complete MinerU parser gateway. Canonical IR/chunk
  manifest mapping and parse-handler composition are still required before parse
  dispatch can be promoted.
- The seam does not claim live MinerU layout/content schema interpretation; that
  must remain a separate gated mapper cut.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.26 Default-off MinerU Artifact Decode Admission

Objective: admit extracted MinerU role payloads into typed Python values without
interpreting Provider schema or building Canonical IR. This is the final
pre-mapping seam before Canonical IR/chunk manifest work.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with `decode_result_archive_artifacts(...)`.
- The decode seam accepts only `MinerULocalBatchArchiveArtifacts` from the
  validated extraction path.
- It decodes full Markdown as strict UTF-8 text.
- It decodes content-list, middle/layout, and model payloads as strict UTF-8
  JSON with duplicate-key rejection and non-finite number rejection.
- It applies role-specific top-level gates: content-list must be a JSON array;
  middle/layout and model must be JSON objects.
- It returns decoded payloads with the redacted archive summary, but does not
  interpret MinerU fields, normalize Markdown/JSON, build Canonical IR/chunk
  manifests, or register production handlers.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 77 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed
```

Residual risk:

- This is still not a complete MinerU parser gateway. Canonical IR/chunk
  manifest mapping and parse-handler composition are still required before parse
  dispatch can be promoted.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5.25 Default-off MinerU Archive Artifact Extraction

Objective: extract the four required MinerU semantic role payloads from an
already validated ZIP without parsing provider content, retaining entry names,
or promoting parse dispatch. This creates the narrow input surface for the next
Canonical IR mapping cut.

Implemented behavior:

- Extended `MinerULocalBatchGateway` with `extract_result_archive_artifacts(...)`.
- The extraction seam revalidates the archive through the G7.5.24 gates before
  opening any role payload.
- It extracts only full Markdown, content-list JSON, middle/layout JSON, and
  model JSON bytes, then returns them with the redacted archive summary.
- It rejects ambiguous archives with multiple candidates for the same semantic
  role, preventing later Canonical IR mapping from silently choosing by path
  ordering.
- It does not retain ZIP entry names/paths in the returned object and does not
  parse Markdown or JSON content.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. No
  provider quota is consumed by tests.

Touched files:

```text
rag/src/mm_chat_rag/mineru_gateway.py
rag/tests/unit/test_mineru_gateway.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && uv run ruff check \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run mypy \
  src/mm_chat_rag/mineru_gateway.py tests/unit/test_mineru_gateway.py
# passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider tests/unit/test_mineru_gateway.py
# 68 passed

cd mm-chat/rag && uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_capture.py::test_production_dispatch_remains_disabled_and_registries_empty \
  tests/unit/test_job_handler_dependencies.py
# 25 passed
```

Residual risk:

- This is still not a complete MinerU parser gateway. Canonical IR/chunk
  manifest mapping and parse-handler composition are still required before parse
  dispatch can be promoted.
- No live MinerU quota is consumed by this slice; the first real provider smoke
  remains in G7.8 or an explicitly owner-authorized bounded smoke cut.

## 2026-07-16 — G7.5T Disposable Postgres Integration Gate Recovery

Objective: run the owner-requested real PostgreSQL integration proof against a
throwaway database, delete that database after the run, and repair only the test
harness/fixtures needed for the gate to remain replayable. This is a test-gate
slice, not the G7.5B `017` parse projection staging proof.

Implemented behavior:

- Restored `scripts/verify-phase15d-postgres.sh` by forcing package execution
  through `go test -p=1`. The knowledge and migration packages both touch
  cluster-global RAG capability roles, so parallel package execution can make
  unrelated migrations fail closed with role-safety errors.
- Updated Postgres knowledge integration fixtures to stop using governance
  placeholder identities rejected by runtime validation:
  - endpoint `default` -> `hosted-main`;
  - model `model-v1` -> `model-stable-20260712`;
  - model API version `v1` -> `api-20260623` or `api-20260624` where the test
    needs two revisions.
- Scoped legacy migration replay tests that assert migration `010` behavior to
  a test filesystem containing migrations through `010`, so later G7 migration
  files do not invalidate the historical regression contract.
- Added a knowledge-package migration test helper for the same through-version
  fixture pattern, used by the migration-009 compatibility test.
- Hardened repository integration ID fixtures so exhausted deterministic IDs
  return readable test errors instead of panicking.
- Ran the final proof against a fresh `postgres:16-alpine` container named
  `mm-chat-test-postgres` on `127.0.0.1:15432`, then removed the container.

Touched files:

```text
backend/internal/knowledge/consent_expiry_worker_postgres_test.go
backend/internal/knowledge/consents_postgres_test.go
backend/internal/knowledge/delete_reprocess_concurrency_postgres_test.go
backend/internal/knowledge/first_bind_concurrency_postgres_test.go
backend/internal/knowledge/governance_postgres_test.go
backend/internal/knowledge/membership_concurrency_postgres_test.go
backend/internal/knowledge/migration_test_helpers_test.go
backend/internal/knowledge/query_consents_postgres_test.go
backend/internal/knowledge/repository_postgres_test.go
backend/internal/migration/phase15_knowledge_tail_replay_test.go
backend/internal/migration/phase15_rag_projection_migration_test.go
scripts/verify-phase15d-postgres.sh
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/backend && go test ./internal/knowledge ./internal/migration
# passed

MM_CHAT_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  bash mm-chat/scripts/verify-phase15d-postgres.sh
# passed against disposable PostgreSQL 16

MIGRATION_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  go run ./cmd/migrate up
# applied 001 through 017 on the disposable DB public schema for the RAG adapter

cd mm-chat/rag && \
  RAG_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  uv run pytest -p no:cacheprovider -m integration tests/integration/test_postgres_integration.py
# 1 passed

docker ps -a --format '{{.Names}}' | grep -Fx mm-chat-test-postgres
# no match; disposable test database deleted
```

Residual risk:

- This validates the disposable Postgres integration gate and existing RAG
  adapter readiness test. It does not yet prove `knowledge_stage_parse_projection(...)`
  with live `017` staging rows; that remains the next G7.5B slice.
- No provider quota was consumed. No application database or compose-managed
  `mm-chat-postgres-1` database was modified.

## 2026-07-16 — G7.5B Live Parse Projection Staging Proof

Objective: prove migration `017_rag_parse_projection_gateway` with a real
PostgreSQL 16 database by calling `knowledge_stage_parse_projection(...)` through
its worker-execute path and verifying every staged projection lane. This closes
the previously static-only `017` proof; it does not promote parse dispatch or
call MinerU/Jina providers.

Implemented behavior:

- Added `backend/internal/migration/phase15_rag_parse_projection_gateway_integration_test.go`.
- The live test seeds a minimal but fully constrained parse job authority:
  - user, file, collection, document, and document version;
  - MinerU governance profile/head and collection processing consent;
  - Jina/Postgres index profile, search profile, index generation;
  - staging materialization bound to the non-legacy parse processing job;
  - active worker lease and stale lease token.
- The test calls `knowledge_stage_parse_projection(...)` with JSONB recordsets
  for all five staging lanes:
  - `knowledge_parser_artifact_sets`;
  - `knowledge_blocks`;
  - `knowledge_parent_chunks`;
  - `knowledge_child_chunks` plus `knowledge_chunk_block_spans`;
  - `knowledge_child_search_projections`.
- The test proves the worker boundary by executing the function as
  `rag_worker_executor`, then verifies the same role cannot mutate the base
  search projection table directly.
- The test proves fail-closed fences for stale lease token and chunk-profile
  mismatch.
- Fixed migration `017` by removing unnecessary `FOR SHARE` locks from immutable
  profile lookups (`knowledge_index_profiles`, `knowledge_search_profiles`).
  Those tables are already immutable by trigger; row-locking would require
  broader privileges than the function needs and broke the worker-execute path.

Touched files:

```text
backend/migrations/017_rag_parse_projection_gateway.up.sql
backend/internal/migration/phase15_rag_parse_projection_gateway_integration_test.go
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/backend && go test ./internal/migration
# passed

MM_CHAT_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  go test -count=1 ./internal/migration \
  -run TestPhase15RAGParseProjectionGatewayLivePostgres -v
# PASS against disposable PostgreSQL 16

MM_CHAT_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  bash mm-chat/scripts/verify-phase15d-postgres.sh
# backend/internal/knowledge passed
# backend/internal/migration passed, including the new 017 live proof

MIGRATION_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  go run ./cmd/migrate up
# applied 001 through 017 on the disposable DB public schema for the RAG adapter

cd mm-chat/rag && \
  RAG_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  uv run pytest -p no:cacheprovider -m integration tests/integration/test_postgres_integration.py
# 1 passed

docker ps -a --format '{{.Names}}' | grep -Fx mm-chat-test-postgres
# no match; disposable test database deleted
```

Residual risk:

- Parse projection staging is now live-proven at the DB function boundary, but
  real parse dispatch remains gated until handler registry promotion, retry/DLQ,
  and end-to-end provider smoke gates are explicitly opened.
- This slice does not call MinerU or Jina and consumes no provider quota.

## 2026-07-16 — G7.5C Python PostgresAdapter Parse Projection Live Proof

Objective: prove the Python-side adapter introduced in G7.5.18 can serialize
its DTOs into the JSONB payload expected by the live `017` database function.
This closes the adapter-to-function proof after the Go-side function proof in
G7.5B. It still does not promote parse dispatch or call MinerU/Jina providers.

Implemented behavior:

- Added
  `rag/tests/integration/test_postgres_parse_projection_integration.py`.
- The test is gated on `RAG_TEST_DATABASE_URL` and skips unless an explicit
  disposable PostgreSQL URL is provided.
- The fixture seeds the minimal authority needed to stage parse projections:
  user, file, collection, document, document version, MinerU governance
  profile/head, collection consent, Jina/Postgres index profile, search
  profile, index generation, staging materialization, processing job, worker
  lease token, and request hash.
- The test calls `PostgresAdapter.stage_parse_projection(...)` with one block,
  parent chunk, child chunk, chunk-block span, and child search projection.
- The assertion reads the database after the call and verifies all six live
  staged rows exist: artifact set, block, parent chunk, child chunk,
  chunk-block span, and child search projection.

Touched files:

```text
rag/tests/integration/test_postgres_parse_projection_integration.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
docker run --rm -d \
  --name mm-chat-test-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=mm_chat \
  -p 127.0.0.1:15432:5432 \
  postgres:16-alpine
# disposable PostgreSQL 16 ready

cd mm-chat/backend && \
  MIGRATION_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  go run ./cmd/migrate up
# applied migrations 001 through 017

cd mm-chat/rag && \
  RAG_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  uv run pytest -p no:cacheprovider -m integration \
  tests/integration/test_postgres_parse_projection_integration.py -v
# 1 passed

cd mm-chat/rag && \
  uv run ruff check tests/integration/test_postgres_parse_projection_integration.py
# passed

cd mm-chat/rag && \
  uv run mypy src/mm_chat_rag/postgres.py \
  tests/integration/test_postgres_parse_projection_integration.py
# passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider tests/unit/test_postgres.py -v
# 30 passed

docker ps -a --format '{{.Names}}' | grep -Fx mm-chat-test-postgres
# no match; disposable test database deleted
```

Residual risk:

- The Python adapter-to-`017` staging path is live-proven, but the production
  parse handler registry remains gated.
- This slice uses synthetic rows and local fixture content; live MinerU parsing,
  live Jina embedding, retry/DLQ, and end-to-end publish/query proof remain
  later G7.5/G7.8 work.
- No provider quota was consumed. No compose-managed or application database
  was modified.

## 2026-07-16 — G7.5D Job-only Worker Promotion Gate

Objective: unblock the next registry-promotion slices by separating Python
worker outbox promotion from processing-job promotion. Before this slice, any
`RAG_WORKER_DISPATCH_ENABLED=true` worker required a non-empty outbox event
registry even when the intended work was job-only. That incorrectly coupled
future parse/embedding/purge job runners to unrelated outbox planner promotion.

Implemented behavior:

- Updated `Worker.validate_promotion_gate()` so:
  - dark-run defaults still pass without claiming work;
  - enabled workers with neither outbox registry nor job stages still fail
    closed;
  - enabled workers with job stages still require a promoted handler for every
    configured stage;
  - enabled job-only workers can pass with explicitly injected job handlers and
    no outbox registry;
  - enabled outbox-only workers can pass with an explicit outbox planner and no
    job stages.
- Updated `Worker.run()` to start:
  - `DurableConsumer` only when an outbox registry is present;
  - `JobRunner` only when job stages are configured.
- Set job-only readiness to keep `consumer=disabled` rather than pretending an
  outbox consumer is ready.
- Added unit coverage for job-only and outbox-only promotion gates.

Touched files:

```text
rag/src/mm_chat_rag/worker.py
rag/tests/unit/test_replay_worker.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && \
  uv run ruff check src/mm_chat_rag/worker.py tests/unit/test_replay_worker.py
# passed

cd mm-chat/rag && \
  uv run mypy src/mm_chat_rag/worker.py tests/unit/test_replay_worker.py
# passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider \
  tests/unit/test_replay_worker.py tests/unit/test_jobs.py tests/unit/test_settings.py -v
# 48 passed
```

Residual risk:

- This is a startup/promotion gate only. It does not build a production handler
  registry, does not call provider APIs, and does not claim real jobs by
  default.
- The next promotion slice still needs explicit dependency factory wiring for
  purge, passage embedding, and parse before `async_main()` can safely promote
  real handlers from env/secret settings.

## 2026-07-16 — G7.5E Explicit Purge Handler Promotion Factory

Objective: promote exactly one real job handler path behind worker settings
without mutating the frozen module-level registries. This follows G7.5D: once
job-only workers no longer require an outbox planner, the safe first promoted
stage is `purge` because it uses only token-fenced Postgres gateways and does
not consume provider quota.

Implemented behavior:

- Added `build_promoted_job_handler_registry(...)` in `rag/src/mm_chat_rag/worker.py`.
- The factory promotes only `purge` by composing:
  - `PurgeHandlerDependencies(projection=PostgresAdapter(...))`;
  - `admitted_purge_handler_with_dependencies(...)`.
- `Worker(...)` now uses the factory only when the caller leaves the default
  frozen `JOB_HANDLER_REGISTRY` in place and `settings.job_stages` is non-empty.
- `parse` and `passage_embedding` are intentionally not auto-promoted; if they
  appear in `RAG_WORKER_JOB_STAGES`, the startup gate still fails until their
  provider dependency factories are wired.
- `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty at module import.

Touched files:

```text
rag/src/mm_chat_rag/worker.py
rag/tests/unit/test_replay_worker.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && \
  uv run ruff check src/mm_chat_rag/worker.py tests/unit/test_replay_worker.py
# passed

cd mm-chat/rag && \
  uv run mypy src/mm_chat_rag/worker.py tests/unit/test_replay_worker.py
# passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider \
  tests/unit/test_replay_worker.py tests/unit/test_jobs.py \
  tests/unit/test_job_handler_dependencies.py tests/unit/test_settings.py -v
# 75 passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_deployment_boundary.py \
  tests/unit/test_provider_capture.py -v
# 67 passed
```

Residual risk:

- This is a settings-gated purge promotion factory, not yet a live promoted
  purge job-runner smoke against disposable PostgreSQL.
- Parse and passage-embedding promotion remain blocked until their provider
  dependency factories, rate/concurrency gates, and live smoke tests are added.
- No MinerU/Jina provider calls were made; no provider quota was consumed.

## 2026-07-16 — G7.5F Promoted Purge Job-runner Live Smoke

Objective: prove the settings-promoted purge handler from G7.5E works through
the real Python `JobRunner` and live PostgreSQL functions, not just unit fakes.
This closes the narrow claim → handler → projection → finish loop for purge.

Implemented behavior:

- Added
  `rag/tests/integration/test_worker_purge_promotion_integration.py`.
- The test is gated on `RAG_TEST_DATABASE_URL` and skips unless an explicit
  disposable PostgreSQL URL is provided.
- The fixture seeds a complete tombstoned document/materialization/search
  projection state:
  - active collection, file, document version, projection head, index
    generation, materialization, parent chunk, child chunk, and one ready child
    search projection;
  - document tombstoned before the purge handler runs, so
    `mark_purge_invisible(...)` must return `query_visible=false`;
  - one pending `knowledge_processing_jobs` row for `stage='purge'`.
- The test constructs `Worker(Settings(... job_stages=("purge",)))`, verifies
  the auto-built purge handler registry, opens the Postgres adapter, and runs
  `JobRunner.process_one()`.
- The assertion verifies:
  - the purge job is `succeeded`;
  - `attempt_count=1`;
  - job lease owner/token are cleared and `completed_at` is set;
  - the child search projection moved from `ready` to `purged`;
  - `purged_at` is set.

Touched files:

```text
rag/tests/integration/test_worker_purge_promotion_integration.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && \
  uv run ruff check tests/integration/test_worker_purge_promotion_integration.py
# passed

cd mm-chat/rag && \
  uv run mypy src/mm_chat_rag/worker.py \
  tests/integration/test_worker_purge_promotion_integration.py
# passed

docker run --rm -d \
  --name mm-chat-test-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=mm_chat \
  -p 127.0.0.1:15432:5432 \
  postgres:16-alpine
# disposable PostgreSQL 16 ready

cd mm-chat/backend && \
  MIGRATION_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  go run ./cmd/migrate up
# applied migrations 001 through 017

cd mm-chat/rag && \
  RAG_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  uv run pytest -p no:cacheprovider -m integration \
  tests/integration/test_worker_purge_promotion_integration.py -v
# 1 passed

docker ps -a --format '{{.Names}}' | grep -Fx mm-chat-test-postgres
# no match; disposable test database deleted
```

Residual risk:

- This proves one promoted purge job through `process_one()`, not a long-running
  worker lifecycle with health server, Redis wakeups, or deployment env files.
- Parse and passage-embedding promotions remain gated; no MinerU/Jina provider
  calls were made and no provider quota was consumed.

## 2026-07-16 — G7.5G Explicit Passage-embedding Promotion Factory

Objective: promote the already tested Jina + Postgres passage-embedding
dependency bundle through the worker factory, while keeping parse unpromoted and
avoiding any live provider call in this slice.

Implemented behavior:

- Extended `build_promoted_job_handler_registry(...)` so
  `RAG_WORKER_JOB_STAGES=passage_embedding` builds a handler with:
  - `build_jina_passage_embedding_handler_dependencies(...)`;
  - the worker `PostgresAdapter` as projection gateway;
  - `admitted_passage_embedding_handler_with_dependencies(...)`;
  - the validated `ProviderRuntimeProfile`.
- `purge` promotion remains unchanged.
- `parse` remains intentionally absent from the factory, so mixed
  `parse,passage_embedding,purge` settings still fail closed on missing parse
  handler promotion.
- Module-level `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty.

Touched files:

```text
rag/src/mm_chat_rag/worker.py
rag/tests/unit/test_replay_worker.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && \
  uv run ruff check src/mm_chat_rag/worker.py tests/unit/test_replay_worker.py
# passed

cd mm-chat/rag && \
  uv run mypy src/mm_chat_rag/worker.py tests/unit/test_replay_worker.py
# passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider \
  tests/unit/test_replay_worker.py tests/unit/test_jina_gateway.py \
  tests/unit/test_job_handler_dependencies.py tests/unit/test_settings.py -v
# 78 passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_deployment_boundary.py \
  tests/unit/test_provider_capture.py -v
# 67 passed
```

Residual risk:

- This is a settings-gated handler factory proof only; it does not claim a live
  embedding job and does not call Jina.
- A disposable PostgreSQL job-runner smoke with mocked or real Jina remains
  pending before embedding dispatch can be called operationally closed.
- Parse promotion remains blocked until source-object, MinerU archive provider,
  parse projection, and provider-smoke gates are connected.

## 2026-07-16 — G7.5H Parse Source Gateway Settings Admission

Objective: close the settings gap before parse promotion. Compose already passes
`RAG_SOURCE_GATEWAY_URL` and `RAG_SOURCE_GATEWAY_TOKEN` to the RAG worker, and
the Python `GoSourceObjectBytesGateway` already enforces the runtime HTTP/token
boundary. The worker settings now admit and require those values when parse
dispatch is enabled.

Implemented behavior:

- Added `source_gateway_url` and `source_gateway_token` to
  `rag/src/mm_chat_rag/settings.py`.
- Added `RAG_SOURCE_GATEWAY_URL` and `RAG_SOURCE_GATEWAY_TOKEN` parsing.
- Parse dispatch now requires, in order:
  - MinerU API token;
  - source gateway URL;
  - source gateway token.
- Source gateway URL must be an `http`/`https` service URL.
- Source gateway token must be non-empty visible ASCII and at most 4096 bytes.
- Updated parse-related unit fixtures so direct `Settings(...)` construction
  satisfies the new parse source authority gate.

Touched files:

```text
rag/src/mm_chat_rag/settings.py
rag/tests/unit/test_settings.py
rag/tests/unit/test_provider_profile.py
rag/tests/unit/test_replay_worker.py
rag/tests/unit/test_jobs.py
rag/tests/unit/test_postgres.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && \
  uv run ruff check src/mm_chat_rag/settings.py \
  tests/unit/test_settings.py tests/unit/test_provider_profile.py \
  tests/unit/test_replay_worker.py tests/unit/test_jobs.py \
  tests/unit/test_postgres.py
# passed

cd mm-chat/rag && \
  uv run mypy src/mm_chat_rag/settings.py tests/unit/test_settings.py
# passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider \
  tests/unit/test_settings.py tests/unit/test_provider_profile.py \
  tests/unit/test_replay_worker.py tests/unit/test_jobs.py \
  tests/unit/test_postgres.py -v
# 108 passed
```

Residual risk:

- This is configuration admission only. Parse handler promotion still needs
  source gateway composition, MinerU archive/result provider composition, parse
  projection staging, and live provider smoke gates.
- No provider API calls were made and no deployment `.env` or secret file was
  read.


## 2026-07-16 — G7.5I Promoted Passage Embedding Job Runner Smoke

Objective: close the promoted `passage_embedding` job-runner proof without
spending real Jina quota. This slice uses a disposable PostgreSQL 16 database and
a mocked Jina HTTP transport while preserving the actual Worker, JobRunner,
Postgres adapter, provider admission, and SQL staging path.

Implemented behavior:

- Added `rag/tests/integration/test_worker_embedding_promotion_integration.py`.
- The integration test seeds:
  - Jina governance profile/head and collection processing consent;
  - Knowledge collection, file, document version, index profile, search profile,
    materialization, parent chunk, child chunk, and one staged child-search row;
  - one pending `passage_embedding` processing job with projection binding.
- The test monkeypatches only the Jina dependency factory client so the
  Worker-built promoted handler still runs, but provider traffic is captured by
  `httpx.MockTransport` instead of the real Jina endpoint.
- The proof verifies:
  - the Worker factory promotes exactly `passage_embedding`;
  - `JobRunner.process_one()` claims and finishes the job;
  - the Jina request uses the locked embeddings URL, bearer header, and the
    staged child text only;
  - provider key bytes are not present in the JSON body;
  - the staged search row becomes `ready`, has a 1024-lane vector, and stores the
    expected vector SHA-256;
  - a second `process_one()` finds no pending work.
- Fixed `rag/src/mm_chat_rag/postgres.py` so
  `knowledge_stage_passage_embedding(...)` receives `%s::real[]` and `%s::text`,
  not psycopg-inferred `double precision[]` / unknown parameters.
- Added migration `018_rag_passage_embedding_stage_function_fix` to replace the
  stage function. The new body no longer updates `embedding_model_id` or
  `embedding_dimensions`; it filters those immutable constants instead, keeping
  the existing least-privilege `UPDATE(status, embedding_vector,
  embedding_vector_sha256, ready_at, purged_at)` grant sufficient.
- Added a Go schema contract test for migration `018`.

Touched files:

```text
backend/migrations/018_rag_passage_embedding_stage_function_fix.up.sql
backend/migrations/018_rag_passage_embedding_stage_function_fix.down.sql
backend/internal/migration/phase15_rag_passage_embedding_stage_function_fix_schema_test.go
rag/src/mm_chat_rag/postgres.py
rag/tests/integration/test_worker_embedding_promotion_integration.py
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
docs/tracking/progress.md
```

Verification:

```text
cd mm-chat/rag && \
  uv run ruff check src/mm_chat_rag/postgres.py \
  tests/integration/test_worker_embedding_promotion_integration.py \
  tests/unit/test_postgres.py
# passed

cd mm-chat/rag && \
  uv run pytest -p no:cacheprovider \
  tests/unit/test_postgres.py tests/unit/test_replay_worker.py \
  tests/unit/test_jina_gateway.py tests/unit/test_job_handler_dependencies.py \
  tests/integration/test_worker_embedding_promotion_integration.py -q
# 83 passed, 1 skipped

cd mm-chat/backend && \
  GOCACHE=/tmp/neo-chat-go-build go test ./internal/migration \
  -run TestPhase15RAGPassageEmbeddingStageFunctionFixContract -count=1 -v
# passed

# disposable live proof
# - postgres:16-alpine container: mm-chat-test-postgres
# - applied migrations 001 through 018
cd mm-chat/rag && \
  RAG_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:15432/mm_chat?sslmode=disable' \
  uv run pytest -p no:cacheprovider \
  tests/integration/test_worker_embedding_promotion_integration.py -v
# 1 passed

# cleanup verified
# docker ps -a --format '{{.Names}}' | grep -Fx mm-chat-test-postgres
# no output
```

Residual risk:

- This does not call real Jina. Real provider smoke remains a later G7.8 gate
  once the owner explicitly wants quota-consuming proof.
- Parse promotion remains blocked until source-object, MinerU archive/result
  provider, and parse dependency factory promotion are wired.
