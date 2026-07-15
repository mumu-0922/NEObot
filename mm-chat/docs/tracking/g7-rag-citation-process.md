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
