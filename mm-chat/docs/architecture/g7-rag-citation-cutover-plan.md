# G7 RAG and Citation Cutover Plan

Status: Active G7 cutover plan, created after owner grill on 2026-07-15.

This document is the working plan for standalone G7. The older Phase 15 design
files remain authoritative for low-level schema, parser, worker, and ACL
contracts; this file locks the owner-level product/runtime decisions and slices
for promoting RAG from dark-run to production-visible behavior inside
`mm-chat`.

## 1. Locked Owner Decisions

- First production-visible G7 target is a **real provider end-to-end loop**, not
  fake/local-only RAG.
- Data egress policy for this owner-operated deployment is broad by default:
  every Knowledge collection selected for indexing may be sent to configured
  RAG providers. ACL and collection selection still restrict what each user can
  query.
- Provider chain:
  - parser: MinerU, including scanned PDFs and complex formula/table PDFs;
  - embedding: Jina embeddings, 1024 dimensions;
  - rerank: Jina reranker;
  - index/search authority: Postgres with pgvector plus lexical/BM25/exact
    lanes where available;
  - source authorization and citation minting: Go only.
- Provider credentials for automatic background indexing are administrator-owned
  server secrets. First version uses backend environment variables or Docker
  secrets. Admin web configuration is deferred to a later provider-settings
  slice.
- Existing frontend MinerU BYOK/manual parse behavior may remain for legacy or
  manual request paths, but G7 automatic Knowledge indexing does not depend on
  browser-held BYOK keys.
- Upload/bind of a Knowledge file automatically schedules parsing and indexing.
  The browser does not need to stay open.
- Failed provider/index work retries three times with bounded exponential
  backoff. Terminal failure marks the processing job failed and keeps the old
  active version, if any.
- Delete/tombstone is immediately query-invisible. Physical chunk/vector/artifact
  purge is asynchronous.
- Replace/reprocess creates a new version/generation. The old version remains
  query-visible until the new version is fully published. Failed replacements do
  not poison active search.
- Chat RAG only searches Knowledge collections explicitly selected/enabled in
  the current chat. It must not silently search every visible collection.
- Strict Knowledge mode refuses to answer when evidence is missing, indexing is
  incomplete, provider calls fail, citation verification fails, or Go
  reauthorization rejects evidence. Normal chat may degrade to a non-RAG answer,
  but the UI/message metadata must make clear that no Knowledge evidence was
  used.
- First citation UI is a basic card/badge: answer markers such as `[1]`, file
  name, best available location (page/sheet/slide/section/paragraph), and the
  supporting snippet. PDF highlight, preview drawers, and cell-level navigation
  are deferred.
- G7 adds Go-owned server routes and adapters. Existing `mm-chat` Next legacy
  routes under `/api/rag/*`, `/api/doc-parse/*`, and `/api/chat/rag-queries`
  remain until G9 removes replaced routes.
- Provider calls are rate-limited but with owner-friendly wide defaults and
  admin-configurable ceilings.

## 2. Scope and Non-goals

### In scope

- Go route/admission surfaces for RAG provider readiness, indexing status,
  private query, reauthorization, citation minting, and chat integration.
- Python worker promotion from dark-run to real parser/index/query handlers for
  the locked MinerU + Jina + Postgres profile.
- Postgres projection schema/function work needed for 1024-dimensional vectors,
  lexical/exact lanes, version/generation fences, tombstones, rebuild, and
  source-span metadata.
- Compose/runtime env wiring for administrator provider credentials without
  reading or committing deployment `.env` contents.
- Minimal frontend server-mode adapters for selected Knowledge collections,
  status visibility, strict/optional degradation, and basic citation cards.

### Out of scope

- Admin web UI for saving provider keys.
- G9 legacy Next route deletion.
- Qdrant/OpenSearch/Vespa production profile.
- Default search across every visible collection.
- Rich PDF/page highlight preview and advanced citation explorer.
- Browser BYOK as the credential source for background automatic indexing.

## 3. Runtime Authority Chain

```text
Browser selected Knowledge collections
  -> frontend /mm-api adapter
  -> Go auth/session + collection ACL + mode policy
  -> Go processing/job/outbox authority
  -> Python rag-worker parser/index/query handlers
  -> MinerU/Jina provider calls using admin server secrets
  -> Postgres projection/search state
  -> Python returns evidence candidates only
  -> Go reauthorizes source spans against Postgres ACL/version/hash fences
  -> Go mints citations and persists assistant message/citation metadata
  -> frontend renders answer + basic citation cards
```

Python never grants access. Provider output and search results are untrusted
until Go reauthorization passes.

## 4. Proposed Server Routes

Exact route names may change during implementation, but ownership is locked:
G7 routes are Go-owned and exposed through the frontend `/mm-api` edge in server
mode.

- `GET /v1/rag/provider-status` — redacted readiness for MinerU/Jina/Postgres
  projection dependencies.
- `POST /v1/knowledge/documents/{documentId}/index` — explicit reprocess/index
  admission; upload/bind continues to auto-enqueue.
- `GET /v1/knowledge/documents/{documentId}/processing` — processing state,
  retry count, safe error code, active/pending generation.
- `POST /v1/rag/query` — private query for selected collection IDs; returns
  only Go-authorized evidence/citation-ready metadata.
- `POST /v1/chat/conversations/{conversationId}/rag-answer` or integration into
  the existing chat stream endpoint — strict/optional RAG answer path.
- `GET /v1/citations/{citationId}` — fetch compact citation card data if not
  already embedded in message metadata.

Legacy Next routes remain as compatibility surfaces until G9.

## 5. Slice Plan

Each slice must finish with targeted tests, progress/process updates, and a
commit before the next slice starts.

### G7.1 Decision lock and runtime inventory

- Add this plan and a dedicated process log.
- Update the standalone cutover plan and progress tracker.
- Inventory current Go Knowledge routes, Python dark-run registries, frontend
  legacy RAG/doc-parse routes, and provider config gaps.
- No production behavior change.

Validation:

- Markdown format check for changed docs.
- `git status --short -- mm-chat` clean after commit.

### G7.2 Admin provider config and fail-closed readiness

Status: Completed on 2026-07-15.

- Added backend/RAG config for admin-owned MinerU and Jina secrets through
  server environment/secret injection:
  - `RAG_MINERU_API_TOKEN`, fallback alias `DEFAULT_MINERU_API_TOKEN`;
  - `RAG_JINA_API_KEY`, fallback alias `DEFAULT_JINA_API_KEY`;
  - Jina embedding dimension is locked to `1024`.
- Exposed protected Go diagnostic `GET /v1/rag/provider-status` with only
  redacted configured/missing status and embedding dimensions.
- Python worker settings fail closed when dispatch enables `parse` without
  MinerU credentials or `passage_embedding` without Jina credentials. `purge`
  remains credential-free.
- Tests and docs do not read deployment `.env` contents and do not print real
  secret values.

Validation:

- Go config tests for present/missing secret behavior and redaction.
- Python settings tests for present/missing secret behavior and redaction.
- Compose/static docs update for required env/secret names.

### G7.3 Provider-backed parser/index profile gate

Status: Completed on 2026-07-15.

- Added a config-only Python provider profile gate for
  `mineru_jina_postgres_v1`; the default remains `disabled`.
- Provider-backed `parse` and `passage_embedding` stages fail closed unless the
  operator selects the profile, explicitly accepts the still-draft wire fixture
  risk for this owner-operated deployment, and provides the required
  server-owned provider keys.
- Locked profile contract:
  - MinerU parser profile for full PDF scope from the owner decision;
  - Jina embedding model `jina-embeddings-v4`, dimensions `1024`;
  - Jina rerank model `jina-reranker-v3`;
  - provider retry max attempts fixed at `3`;
  - default retry backoff `30s..300s`;
  - provider concurrency default `2`;
  - MinerU rate default `60` requests/minute;
  - Jina rate default `240` requests/minute.
- Production dispatch registries remain empty in this slice. G7.3 does not add
  network/provider handlers and cannot consume provider quota by itself.
- The profile module is intentionally config-only and has no HTTP/client SDK
  imports; errors carry stable config field names only, not secret values or
  provider request/response bodies.

Validation:

- Provider profile loader tests.
- Retry/rate-limit tests.
- No secret/body log tests.

### G7.4 Canonical IR to chunks and Postgres projection

Status: Completed on 2026-07-15.

- Added a pure Python projection builder that stages already-validated Canonical
  IR v2 plus Chunk Manifest v2 into deterministic Postgres row DTOs for:
  canonical Blocks, Parent Chunks, Child Chunks, chunk-to-block spans, and the
  extension-independent child search projection seed.
- Projection UUIDs are deterministic within immutable artifact/materialization
  scopes; content bytes, content hashes, source hashes, parent/child references,
  locator summaries, and manifest counts fail closed before any future DB write.
- Added migration `012_rag_search_projection` with:
  - `knowledge_search_profiles` locked to `mineru_jina_postgres_v1`,
    `jina-embeddings-v4`, dimensions `1024`, and `jina-reranker-v3`;
  - `knowledge_child_search_projections` carrying dense-vector storage as
    extension-independent `REAL[]`, built-in lexical `TSVECTOR`, exact `TEXT[]`,
    source-span/hash fences, locator summaries, and staging/ready/purge states;
  - `knowledge_assert_materialization_search_complete(...)` for G7.5 workers to
    prove all children have ready 1024-dimensional embeddings before publish.
- The migration intentionally does not `CREATE EXTENSION`; pgvector/true BM25
  accelerator promotion remains a later reversible search-profile migration once
  the deployment image/license gates are closed. G7.4 itself still consumes no
  provider quota and does not attach worker dispatch.

Validation:

- Parser hash-DAG fixture projection tests for exact content/hash/source-locator
  projection.
- Migration static contract tests for 1024-dimensional search projection,
  lexical/exact lanes, completeness function, grants, and down-safety.
- Python ruff/mypy and targeted pytest.

### G7.5 Worker dispatch, rebuild, delete, and retry loop

Status: In progress.

G7.5.1 completed on 2026-07-15:

- Extended worker readiness so dispatch-capable workers require the G7.4 search
  projection completeness function before reporting DB function readiness.
- Added the Python Postgres adapter method for
  `knowledge_assert_materialization_search_complete(...)`, with the Jina model
  and `1024` dimensions pinned at the call boundary.
- No production job handler registry entries were promoted in G7.5.1; quota and
  provider calls remain untouched.

G7.5.2 completed on 2026-07-15:

- Added a Python `ProcessingJobContext` admission seam so future worker handlers
  consume typed job authority instead of raw DB claim rows.
- Fail-closed validation now rejects unsupported stage/operation, legacy
  projection-unbound jobs, missing Generation/Materialization binding, malformed
  provider authority, forbidden purge authority, invalid request hashes, and
  runtime provider-profile mismatch with stable redacted error codes.
- Added a handler wrapper for future promoted handlers; production
  `JOB_HANDLER_REGISTRY` remains empty, so this slice still cannot claim work or
  consume MinerU/Jina quota.

G7.5.3 completed on 2026-07-15:

- Go document upload, replacement, and reprocess paths now allocate an explicit
  `materialization_id` alongside the processing `job_id`.
- When a real active Corpus Index Generation exists, Go creates a staging
  `knowledge_document_materializations` row and writes the parse job with
  `legacy_projection_unbound=false`, pinned `index_generation_id`,
  `materialization_id`, exact model authority, and the provider retry budget of
  `3`.
- The matching document/reprocess outbox payload carries the same
  `indexGenerationId` and `materializationId` so later dispatch cannot infer or
  guess projection authority from stale state.
- If no active Generation exists yet, Go preserves the previous legacy-unbound
  fallback so non-RAG document management remains usable until the first
  Generation is promoted.

G7.5.4 completed on 2026-07-15:

- Go document tombstone/delete now binds purge jobs to the active Corpus Index
  Generation when one exists.
- Bound purge jobs are written with `legacy_projection_unbound=false`, pinned
  `index_generation_id`, optional `materialization_id` when the active document
  projection head points at the deleted version, and the retry budget of `3`.
- Tombstone outbox payloads now include `legacyProjectionUnbound` and, when
  bound, `indexGenerationId` / `materializationId`.
- If no active Generation exists, delete keeps the previous legacy purge job
  fallback.

G7.5.5 completed on 2026-07-15:

- Added side-effect-free Python handler skeletons for `parse`,
  `passage_embedding`, and `purge` that accept only an admitted
  `ProcessingJobContext`, plus claim-level constructors that can only enter
  through `with_job_context_admission(...)`.
- Provider-backed skeletons re-check exact stage, Generation binding,
  Materialization binding, provider authority, and the locked
  `mineru_jina_postgres_v1` runtime profile before stopping with a stable
  unpromoted error.
- Purge skeletons re-check exact stage and Generation binding, allow a null
  `materialization_id`, and reject any provider authority so delete/tombstone
  work cannot inherit MinerU/Jina credentials.
- Production `JOB_HANDLER_REGISTRY` remains empty. These skeletons cannot be
  claimed by the worker unless a future slice explicitly promotes a registry,
  and even then they fail closed before provider, object-storage, or projection
  side effects.

G7.5.6 completed on 2026-07-15:

- Added the first real dependency-injected parse handler seam without production
  registration.
- The seam can only enter through `with_job_context_admission(...)`, then
  requires object-storage source, MinerU-compatible parser, and Postgres
  projection gateways before any work proceeds.
- The default dependency bundle fails closed with a stable redacted error before
  storage, provider, or projection calls, preserving the no-quota boundary.
- A fake-gateway unit path now proves the parse flow can fetch a document
  source, receive Canonical IR v2 + Chunk Manifest v2 artifacts, build the G7.4
  Postgres projection batch, compare parser source hash to stored source
  metadata, and stage projection rows.
- Parser/projection validation errors and source-hash mismatches stop before
  projection writes and expose only stable job error codes.
- Production `JOB_HANDLER_REGISTRY` remains empty; G7.5.6 adds a promotion seam,
  not a promoted live handler.

G7.5.7 completed on 2026-07-15:

- Added the dependency-injected `passage_embedding` handler seam without
  production registration.
- The seam can only enter through `with_job_context_admission(...)`, then
  requires a Jina-compatible embedding gateway and a Postgres projection gateway
  before any work proceeds.
- The default dependency bundle fails closed before candidate fetch, provider
  embedding, or projection writes.
- Fake-gateway tests prove the intended flow: fetch child search candidates,
  request passage embeddings, enforce `jina-embeddings-v4` with exactly `1024`
  finite vector lanes, compute a redacted vector hash for `REAL[]` storage,
  stage embeddings, and call the materialization completeness gate.
- Count mismatches, child-id mismatches, invalid vector dimensions, and failed
  completeness checks stop with stable error codes and do not leak raw
  embeddings or provider bodies.
- Production `JOB_HANDLER_REGISTRY` remains empty; G7.5.7 adds a promotion seam,
  not a promoted live Jina handler.

G7.5.8 completed on 2026-07-16:

- Added the dependency-injected `purge` handler seam without production
  registration or provider credentials.
- The seam can only enter through `with_job_context_admission(...)`, then reuses
  the purge authority fence that forbids MinerU/Jina provider authority.
- The default dependency bundle fails closed before any projection call.
- Fake-gateway tests prove the required order: first prove the tombstoned
  document version is no longer query-visible, then purge search projections,
  then assert purge completion.
- Visibility mismatches, remaining ready rows, materialization mismatches, and
  failed completion checks stop with stable error codes before the handler can
  report success.
- Production `JOB_HANDLER_REGISTRY` remains empty; G7.5.8 adds a promotion seam,
  not a promoted live purge handler.

G7.5.9 completed on 2026-07-16:

- Added the first real Postgres projection gateway adapter behind the purge
  dependency seam while keeping production handler registries empty.
- Added token-fenced Postgres functions for the purge sequence:
  `knowledge_mark_purge_invisible(...)`,
  `knowledge_purge_search_projection(...)`, and
  `knowledge_assert_purge_complete(...)`.
- The Python `PostgresAdapter` now implements the purge gateway contract by
  passing the admitted job id, worker id, lease token, Generation, optional
  Materialization, and document/visibility fences through stored-function calls
  only.
- The adapter fails closed before database I/O if the admitted claim row lacks
  a lease token. This preserves the existing JobRunner lease/heartbeat/final
  CAS model and avoids un-fenced projection mutation.
- This slice does not promote a live purge handler or add MinerU/Jina provider
  calls. It only makes one real gateway available for later explicit registry
  promotion.

G7.5.10 completed on 2026-07-16:

- Added a live Postgres integration gate for the migration `001-014` purge
  projection gateway path. The test is guarded by `MM_CHAT_TEST_DATABASE_URL`
  and otherwise skips without reading any env file.
- The gate seeds the smallest active Generation, Materialization, Parent/Child
  Chunk, Search Projection, and token-fenced purge Job needed to exercise the
  stored-function surface end to end.
- It proves `rag_worker_executor` can execute the purge gateway functions but
  cannot mutate search projection base tables directly.
- It proves stale lease tokens fail closed with `RAG_STALE_JOB_LEASE`, active
  documents are visible before tombstone, tombstoned documents become
  query-invisible before projection purge, ready search rows are marked
  `purged`, no ready rows remain, and completion assertion succeeds.
- The first live run caught a PL/pgSQL ambiguity in
  `knowledge_mark_purge_invisible(...)`; migration `014` now qualifies the
  `knowledge_processing_jobs` row as `processing_job` so output column names
  cannot shadow table columns.

G7.5.11 completed on 2026-07-16:

- Added default-off Postgres passage-embedding projection gateway functions:
  `knowledge_fetch_passage_embedding_candidates(...)` and
  `knowledge_stage_passage_embedding(...)`.
- Both functions are token-fenced to a live `passage_embedding` Job,
  non-legacy Generation binding, Worker owner, lease token, lease expiry, and
  Materialization scope. Only `rag_worker_executor` receives Execute.
- The fetch function returns the deterministic child text candidates from
  `knowledge_child_search_projections` for both `staging` and `ready` rows so
  replay after a partial stage remains idempotent and still checks the full
  child count.
- The stage function pins `jina-embeddings-v4`, exactly `1024` REAL lanes, a
  redacted vector hash, and transitions only matching child search rows to
  `ready`.
- The Python `PostgresAdapter` now implements the
  `PassageEmbeddingProjectionGateway` fetch/stage side while keeping the
  existing materialization completeness gate. No real Jina provider call or
  handler registry promotion is included in this slice.

G7.5.12 completed on 2026-07-16:

- Added a default-off `JinaPassageEmbeddingGateway` that implements the
  passage-embedding provider half without registering a production handler.
- The gateway accepts only an explicitly injected administrator Jina API key; it
  does not read `.env`, process environment, browser BYOK keys, or root project
  secrets. Missing or malformed keys fail before HTTP.
- Requests are pinned to `https://api.jina.ai/v1/embeddings`, model
  `jina-embeddings-v4`, task `retrieval.passage`, and exactly `1024` float
  dimensions.
- Responses are converted into `PassageEmbeddingVector` values only after count,
  index, model, finite-number, and dimension validation. Provider request IDs,
  raw bodies, credentials, and embeddings are not surfaced in errors.
- Transport/status failures raise stable retryable job errors for the existing
  three-attempt durable retry path; invalid credentials/shape/vector data fail
  closed with stable permanent job errors.
- `httpx==0.28.1` is now a runtime dependency because the installed RAG package
  contains the provider gateway implementation.

G7.5.13 completed on 2026-07-16:

- Added `build_jina_passage_embedding_handler_dependencies(...)` as the
  explicit composition seam for Jina provider + passage-embedding projection
  dependencies. It returns a `PassageEmbeddingHandlerDependencies` bundle only;
  production registries remain empty.
- The bundle fails closed when the projection gateway is absent, before any Jina
  HTTP call can be made. The API key is still explicit constructor input and is
  not read from env, BYOK state, or deployment files.
- Added a full admitted-handler unit path using the real Jina gateway against an
  `httpx.MockTransport` plus a fake projection gateway. The test proves the
  order: fetch candidates, call Jina once, stage `1024`-lane vectors with stable
  hashes, then assert materialization completeness.

G7.5.14 completed on 2026-07-16:

- Added `ObjectStoreDocumentSourceGateway`, a default-off parse source gateway
  that composes parse-job-scoped file metadata with an object-byte reader and
  returns the existing `DocumentSource` DTO.
- Added `FileSourceMetadata` validation for nonzero `file_id`,
  `local|minio|s3` storage backend, safe internal object keys, lowercase SHA-256,
  bounded nonzero byte size, and normalized content type.
- The gateway verifies the metadata `file_id` matches the admitted parse job,
  object byte length equals metadata, and the downloaded bytes hash to the
  expected source SHA-256 before any parser receives the body.
- No MinIO/S3 SDK, backend env secret, production registry entry, or live object
  store call is introduced in this slice.

G7.5.15 completed on 2026-07-16:

- Added migration `016_rag_parse_source_metadata_gateway` with
  `knowledge_fetch_parse_source_metadata(...)`, a worker-execute-only
  token-fenced function for parse jobs.
- The function validates the live job lease, parse stage, operation,
  materialization binding, file id, non-legacy projection binding, collection
  revision/visibility fences, document visibility, version content hash, and
  staging materialization source hash before returning object metadata.
- Extended the Python `PostgresAdapter` with `fetch_source_metadata(...)` so it
  implements the metadata half of the new source gateway seam and returns
  validated `FileSourceMetadata` values.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice still does not read object bytes, call MinIO/S3, call MinerU, or consume
  provider quota.

G7.5.16 completed on 2026-07-16:

- Added `LocalObjectBytesGateway`, a default-off local filesystem byte reader
  for metadata whose `storage_backend` is exactly `local`.
- The gateway requires an explicit configured root path, reuses the safe
  internal object-key contract, rejects backend mismatches before file reads,
  rejects symlink objects and path escapes, checks the on-disk byte size before
  materializing bytes, and lets the existing composition gateway perform the
  final SHA-256 check.
- No deployment `.env` read, MinIO/S3 SDK, MinerU call, production registry
  entry, or provider quota use is introduced. MinIO/S3 object-byte adapters
  remain a future gated slice.

G7.5.17 completed on 2026-07-16:

- Added a Go private source-object gateway at
  `POST /internal/rag/source-object`. The route is auth-middleware public only
  for this exact POST path and then fail-closed by
  `X-MM-Chat-Internal-Token`; browsers still cannot use object keys or MinIO/S3
  credentials directly.
- The Go gateway reuses `knowledge_fetch_parse_source_metadata(...)`, validates
  the leased parse-job fence, reads bytes through the existing backend
  `storage.ObjectStore`, enforces size and SHA-256, and returns raw bytes with
  redacted headers only.
- Added `RAG_SOURCE_GATEWAY_TOKEN` as an administrator server secret and wired
  the backend service default-off when the token, Postgres repository, or object
  store is absent.
- Added `GoSourceObjectBytesGateway` in Python. It calls the Go internal route
  with job id, worker id, lease token, file id, and materialization id; it
  revalidates response headers, byte size, and SHA-256 before the composition
  gateway performs its final check.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not call MinerU/Jina or spend provider quota.

G7.5.18 completed on 2026-07-16:

- Added the default-off Python Postgres parse projection adapter seam:
  `PostgresAdapter.stage_parse_projection(...)` now serializes parser block,
  parent chunk, child chunk, span, and child-search projection batches into one
  token-fenced `knowledge_stage_parse_projection(...)` call.
- The adapter requires the admitted processing-job lease token and
  materialization id, checks that batch collection/document/version/generation
  identities match the claimed context, and rejects mismatches before any DB
  call with the stable parse-artifact error code.
- Projection payloads are wrapped as JSONB only after converting UUID/Decimal
  values into JSON-safe forms, so the future DB function receives one bounded
  immutable batch instead of ad hoc row writes.
- This slice intentionally adds only the Python adapter seam and unit tests. The
  `knowledge_stage_parse_projection(...)` migration/function and live staging
  proof remain the next gated cut before any handler promotion.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not call MinerU/Jina or spend provider quota.

G7.5.19 completed on 2026-07-16:

- Added migration `017_rag_parse_projection_gateway` with
  `knowledge_stage_parse_projection(...)`, the default-off database counterpart
  to the Python parse projection adapter.
- The function revalidates the leased parse job, staging materialization, source
  SHA-256, collection/document/version visibility fences, index profile chunk
  profile hash, and Jina 1024 search profile before any projection rows are
  accepted.
- It links the parser artifact set to the materialization, then stages blocks,
  parent chunks, child chunks, chunk-block spans, and child-search projection
  rows from JSONB recordsets with count/mismatch gates for each lane.
- It grants only `rag_worker_executor` function execute rights and gives
  `rag_projection_owner` the minimal new insert/select privileges needed by the
  SECURITY DEFINER function.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not call MinerU/Jina or spend provider quota; live DB staging still
  needs an owner-provided `MM_CHAT_TEST_DATABASE_URL` proof.

G7.5.20 completed on 2026-07-16:

- Added `MinerULocalBatchGateway`, a default-off Python gateway for the
  evidence-backed MinerU local-batch `allocate_upload` step only.
- The gateway requires an explicitly injected administrator token, accepts only
  `application/pdf` sources, enforces the 200 MiB public contract limit, and
  sends the fixed full-PDF options selected for G7: OCR on, formula on, table on,
  `model_version=vlm`.
- HTTP calls are locked to `POST https://mineru.net/api/v4/file-urls/batch` with
  no redirects and `trust_env=False`; provider statuses, provider `code`
  failures, and transport failures map to stable redacted retryable errors.
- Allocation responses are parsed into transient batch id plus one HTTPS signed
  upload URL; unsafe URLs, malformed JSON, wrong file-url counts, and oversized
  responses fail closed without logging provider ids, URLs, or secrets.
- This slice intentionally does not implement signed upload, polling, result ZIP
  download, Canonical IR mapping, or parser-handler promotion because those
  MinerU local-batch wire/contracts remain separate gated cuts.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.21 completed on 2026-07-16:

- Added the default-off MinerU signed-upload transport seam to
  `MinerULocalBatchGateway`.
- The seam uploads one admitted PDF body to the single provider-derived signed
  URL with `PUT`, and intentionally sends no `Authorization`, `Cookie`, or
  `Content-Type` headers.
- Upload targets are locked before HTTP to `https`, default port `443`, exact
  host `mineru.oss-cn-shanghai.aliyuncs.com`, `/api-upload/` path prefix, no
  userinfo, no fragment, required signed query, visible ASCII only, no
  encoded/traversal path, and at most 4096 URL bytes.
- Upload `200` and `204` statuses are accepted; provider status failures and
  transport failures map to stable redacted retryable errors.
- This slice still does not implement polling, result ZIP download, Canonical IR
  mapping, parser-handler composition, or production registry promotion.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.22 completed on 2026-07-16:

- Added the default-off MinerU batch poll/result seam to
  `MinerULocalBatchGateway`.
- The seam constructs the only allowed poll target from the validated batch id:
  `GET https://mineru.net/api/v4/extract-results/batch/{batch_id}`.
- Poll requests use the admin MinerU bearer token plus `Accept:
application/json` and `Accept-Encoding: identity`, with no request body or
  `Content-Type`.
- Poll JSON is parsed as a closed single-file shape: matching batch id, matching
  allocated filename, exactly one result, state in
  `waiting-file|pending|running|converting|done|failed`, optional bounded
  `data_id`, and running-progress validation.
- The slice also tightens the existing signed-upload target gate to require a
  signed query and reject encoded/traversal path drift before HTTP.
- `done` requires a result ZIP URL and validates it before returning:
  `https`, default/443 port, exact host `cdn-mineru.openxlab.org.cn`,
  `/pdf/` path prefix, `.zip` suffix, no query, no userinfo, no fragment, no
  control/non-visible ASCII, no encoded/traversal path, and at most 4096 URL
  bytes.
- This slice still does not download the result ZIP, validate archive entries,
  map Canonical IR/chunk manifests, compose the parser handler, or promote any
  production registry.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.23 completed on 2026-07-16:

- Added the default-off MinerU result ZIP download transport seam to
  `MinerULocalBatchGateway`.
- The seam only accepts a `done` poll result whose result URL already passed the
  locked MinerU CDN target gate, then performs one bounded `GET` for archive
  bytes.
- Download requests send `Accept: application/zip` and `Accept-Encoding:
identity`; they intentionally send no `Authorization`, `Cookie`, or
  `Content-Type`. Injected client cookies are cleared before dynamic download
  requests.
- Download responses require status `200`, identity/no content encoding, an
  allowlisted ZIP content type, valid decimal `Content-Length` when present, and
  a compressed body at most 32 MiB.
- This slice still does not validate ZIP entries, map Canonical IR/chunk
  manifests, compose the parser handler, or promote any production registry.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.24 completed on 2026-07-16:

- Added the default-off MinerU result ZIP archive-validation seam to
  `MinerULocalBatchGateway`.
- The seam validates an already downloaded ZIP body without retaining entry
  names or content. It returns only a redacted summary: compressed byte count,
  archive SHA-256, entry count, and presence booleans for full Markdown,
  content-list JSON, middle/layout JSON, and model JSON.
- Archive gates enforce non-empty ZIP bytes, 32 MiB compressed limit, at most
  256 entries, 64 MiB per expanded entry, 128 MiB total expanded bytes, bounded
  compression ratio, valid CRC, no encrypted entries, no symlink entries, no
  duplicate names, and no absolute/traversal/empty/backslash paths.
- Required MinerU artifacts are semantic roles only: `full.md`,
  `content_list.json` or `*_content_list.json`, Cloud v4 `layout.json` or
  `middle.json`/`*_middle.json`, and `model.json` or `*_model.json`.
- This slice still does not read artifact content, map Canonical IR/chunk
  manifests, compose the parser handler, or promote any production registry.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.25 completed on 2026-07-16:

- Added the default-off MinerU archive artifact-extraction seam to
  `MinerULocalBatchGateway`.
- The seam reuses the archive validation gates, then extracts only the four
  required semantic role byte payloads: full Markdown, content-list JSON,
  middle/layout JSON, and model JSON.
- Extraction rejects ambiguous archives with multiple candidates for the same
  semantic role so later Canonical IR mapping cannot silently choose the wrong
  artifact.
- Returned artifacts intentionally do not retain ZIP entry names or paths; they
  carry validated role bytes plus the redacted archive summary from G7.5.24.
- This slice still does not parse Markdown/JSON content, map Canonical IR/chunk
  manifests, compose the parser handler, or promote any production registry.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.26 completed on 2026-07-16:

- Added the default-off MinerU artifact payload decode-admission seam to
  `MinerULocalBatchGateway`.
- The seam accepts only the extracted role bytes from G7.5.25 and decodes full
  Markdown with strict UTF-8.
- It decodes content-list, middle/layout, and model payloads as strict UTF-8
  JSON with duplicate-key rejection and non-finite number rejection.
- Top-level role gates are intentionally narrow: content-list must be a JSON
  array, while middle/layout and model must be JSON objects.
- The decoded payloads are admitted as untrusted data for the later Canonical IR
  mapper; this slice still does not interpret MinerU schema fields, build
  Canonical IR/chunk manifests, compose the parser handler, or promote any
  production registry.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.27 completed on 2026-07-16:

- Added the default-off MinerU canonical-mapping input seam to
  `MinerULocalBatchGateway`.
- The seam accepts an admitted PDF `DocumentSource` plus the validated MinerU
  role byte artifacts, verifies the source body hash against the expected
  source SHA-256, and then reuses the G7.5.26 decode gates.
- It returns a hash-bound in-memory mapper input containing source byte/hash
  metadata, archive byte/hash metadata, deterministic role digest order
  (`full_markdown`, `content_list_json`, `middle_json`, `model_json`), and the
  decoded role payloads.
- The returned object intentionally carries no ZIP entry names, URLs, provider
  ids, Canonical IR, or chunk manifest. It is only the stable pre-mapper bundle
  for the next cut.
- This slice still does not interpret MinerU schema fields, map layout/content
  objects, build Canonical IR/chunk manifests, compose the parser handler, or
  promote any production registry.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.28 completed on 2026-07-16:

- Added a default-off MinerU `full.md` text-baseline mapper on
  `MinerULocalBatchGateway`.
- The mapper accepts only the G7.5.27 hash-bound canonical mapping input plus an
  explicit worker-owned `artifact_set_id`, then returns `ParsedDocumentArtifacts`
  containing projection-ready `canonical-ir.v2` and `chunk-manifest.v2` objects.
- The baseline intentionally uses only `full.md` text for one paragraph block and
  deterministic parent/child chunks. It preserves PDF source hash/byte metadata,
  deterministic locator summaries, text range anchors, provenance references,
  and a frozen text-baseline chunk profile hash.
- Long Markdown is split on UTF-8 code point boundaries into bounded child
  chunks so the projection lane can stage multiple chunks without splitting a
  multibyte character.
- This slice proves compatibility with the G7.4
  `build_postgres_projection_batch(...)` row model in unit tests.
- This slice still does not interpret MinerU `content_list`, `layout/middle`, or
  `model` schemas for page/table/formula/image citations; those remain separate
  mapper cuts before parser-handler composition or registry promotion.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.29 completed on 2026-07-16:

- Added a default-off archive-to-text-baseline composition seam on
  `MinerULocalBatchGateway`.
- The seam accepts a PDF `DocumentSource`, already downloaded MinerU result ZIP
  bytes, and an explicit worker-owned `artifact_set_id`, then runs the validated
  chain in memory:
  `extract_result_archive_artifacts -> prepare_canonical_mapping_input -> build_text_baseline_parse_artifacts`.
- The composition reuses archive validation, role extraction, source hash
  binding, strict artifact decode gates, and the G7.5.28 full-Markdown baseline
  mapper without performing any HTTP request.
- Unit tests prove successful projection through G7.4
  `build_postgres_projection_batch(...)`, invalid archive rejection, and source
  hash mismatch rejection.
- This slice still does not perform Allocate/Upload/Poll/Download orchestration,
  parse-handler dependency wiring, content-list/layout/model schema mapping, or
  production registry promotion.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.30 completed on 2026-07-16:

- Added `MinerUTextBaselineArchiveParserGateway`, a default-off ParserGateway
  shaped adapter over the G7.5.29 archive-to-text-baseline composition seam.
- Added a narrow `MinerUResultArchiveProvider` protocol so the parser adapter
  can receive already downloaded MinerU ZIP bytes from an injected dependency
  instead of allocating/uploading/polling/downloading by itself.
- The adapter admits only a `ProcessingJobContext` in `parse` stage with a
  non-zero `materialization_id`, validates PDF content type and source SHA-256,
  then derives a deterministic text-baseline `artifact_set_id` from
  materialization id, source hash, and the frozen chunk profile hash.
- Missing archive provider fails closed with
  `MINERU_GATEWAY_DEPENDENCY_UNCONFIGURED` before any archive fetch. Invalid
  context fails with `MINERU_GATEWAY_CONTEXT_INVALID` before provider access.
- The parser adapter reuses ZIP validation, semantic role extraction, strict
  decode gates, source hash binding, and projection-ready full-Markdown baseline
  mapping from previous slices.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.31 completed on 2026-07-16:

- Added a parse-handler dependency composition proof that uses the real
  `MinerUTextBaselineArchiveParserGateway` inside `ParseHandlerDependencies`.
- The test keeps document bytes and MinerU result archive bytes injected by fake
  gateways, then runs `admitted_parse_handler_with_dependencies(...)` through
  source fetch, parser adapter, G7.4 projection build, and projection staging.
- The proof verifies deterministic source-hash propagation, text-baseline parent
  chunk content, exact-term projection, and the expected side-effect order:
  source fetch -> archive fetch -> projection stage.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.32 completed on 2026-07-16:

- Added a conservative MinerU basic page locator admission for the
  full-Markdown text-baseline mapper.
- The mapper now admits a page bbox only when `content_list` has exactly one
  full-text match and the `layout/middle` artifact has exactly one matching
  element with non-negative `pageIndex` and positive half-open
  `bboxMilliPoint`.
- Admitted page regions are inserted before text-position views in the block and
  chunk locator sets, so the G7.4 projection emits `page_bbox` locators for
  basic citation cards.
- Ambiguous or malformed matched page locators fail closed with
  `MINERU_GATEWAY_ARTIFACT_INVALID`. Missing/unrecognized locator evidence still
  falls back to the previous text-baseline line-range locator instead of
  inventing page evidence.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.33 completed on 2026-07-16:

- Extended the same conservative page-bbox admission from G7.5.32 to MinerU
  `sourceText` fields.
- This allows formula-like full-Markdown baselines to project a basic
  `page_bbox` locator when both `content_list` and `layout/middle` agree on the
  same `sourceText`, page index, and bbox.
- The output remains a text-baseline paragraph/chunk projection; this does not
  claim Formula IR semantics, LaTeX normalization, table structure, image
  assets, or production dispatch.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.34 completed on 2026-07-16:

- Extended the conservative page-bbox admission to single table elements without
  parsing table cells or emitting Table IR.
- The mapper now accepts `content_list` entries whose `type`/`kind` is `table`
  and whose `text` equals the full-Markdown baseline, then locates exactly one
  `layout/middle` element whose `kind`/`type` is `table` with page index and
  bbox.
- Rows/cells inside the table element remain opaque; only the element-level
  `page_bbox` is projected for basic citation cards. Multiple table candidates
  fail closed as ambiguous.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5.35 completed on 2026-07-16:

- Extended the conservative element-level page-bbox admission to single image
  elements without reading image bytes, persisting image paths, or emitting Asset
  IR.
- The mapper now accepts `content_list` entries whose `type`/`kind` is `image`
  and whose `text` equals the full-Markdown baseline, then locates exactly one
  `layout/middle` element whose `kind`/`type` is `image` with page index and
  bbox.
- Image path/provider asset metadata remains opaque; only the element-level
  `page_bbox` is projected for basic citation cards. Multiple image candidates
  fail closed as ambiguous.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5A completed on 2026-07-16:

- Consolidated the MinerU text-baseline locator hardening work into a medium
  slice after the owner approved moving away from tiny locator-only cuts.
- Added fail-closed tests for duplicate `content_list` full-text matches,
  missing page index, malformed/negative/zero-area/non-integer bbox values, and
  ambiguous formula `sourceText` layout candidates.
- Locked the formula boundary: formula-like `sourceText` may project a basic
  `page_bbox` only when the layout element repeats the same `sourceText`;
  formula kind-only layout elements are not inferred as evidence.
- Added a no-match fallback proof: if `content_list` does not agree with the
  full-Markdown baseline, matching layout geometry is ignored and the mapper
  preserves the safe line-range locator.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not spend provider quota in tests.

G7.5C completed on 2026-07-16:

- Added a Python live integration proof for
  `PostgresAdapter.stage_parse_projection(...)` against a disposable
  PostgreSQL 16 database migrated through `017_rag_parse_projection_gateway`.
- The test seeds the constrained parse-job authority used by the Go-side live
  proof, then verifies the Python DTO-to-JSONB serialization path writes all
  parse projection lanes: artifact set, block, parent chunk, child chunk, chunk
  span, and child search projection.
- The proof runs only when `RAG_TEST_DATABASE_URL` points at an explicit test
  database. The disposable container is removed after the run.
- Production `DISPATCH_REGISTRY` and `JOB_HANDLER_REGISTRY` remain empty. This
  slice does not call MinerU/Jina providers and does not spend provider quota.

G7.5D completed on 2026-07-16:

- Split the Python worker promotion gate so outbox planners and processing-job
  handlers are validated independently.
- A job-only worker with explicitly injected stage handlers can now pass the
  startup gate without a promoted outbox event registry; an outbox-only worker
  with a promoted planner still passes without job stages.
- The worker starts the outbox consumer only when an outbox registry is present,
  and starts the job runner only when job stages are configured.
- Default production exports remain unchanged: `DISPATCH_REGISTRY` and
  `JOB_HANDLER_REGISTRY` are still empty, so no handler is silently promoted.

Remaining G7.5 work:

- G7.5T disposable PostgreSQL integration gate has been restored and proven
  against a throwaway `postgres:16-alpine` database; the test DB is deleted
  after each run, and this does not promote handlers or complete the `017`
  parse projection staging proof.
- G7.5B is complete: `knowledge_stage_parse_projection(...)` is live-proven
  against a disposable PostgreSQL 16 database, including worker-execute,
  stale-lease rejection, profile-fence rejection, and staged row assertions for
  artifact sets, blocks, parent chunks, child chunks, spans, and child search
  projections. Richer formula semantics, optional table-cell/image-asset
  semantics, and live provider smoke remain later gated work. Production
  MinIO/S3 object access should prefer the Go private source-object gateway
  rather than giving Python static object-store credentials.
- G7.5C is complete: the Python `PostgresAdapter.stage_parse_projection(...)`
  path is live-proven against disposable PostgreSQL with migrations `001`
  through `017` applied. This closes the adapter-to-function serialization
  proof, but not handler promotion or live provider smoke.
- G7.5D is complete: explicit job-only promotion is no longer blocked by an
  unrelated empty outbox registry. Real parse, embedding, and purge handlers
  still need a separate dependency factory and registry wiring gate before
  production dispatch can claim jobs.
- Promote the composed default-off Jina + Postgres embedding dependencies only
  behind an explicit readiness/registry gate.
- Promote purge dispatch behind an explicit readiness/registry gate now that
  the purge Postgres gateway has unit, static, and live integration coverage.
- Implement index, reprocess, tombstone purge, rebuild, retry, and DLQ behavior.
- Promote handlers one stage at a time behind readiness and registry gates.

Validation:

- Outbox duplicate/out-of-order/replay tests.
- Delete/tombstone/rebuild tests, including the G7.5.10 live purge projection
  integration gate when `MM_CHAT_TEST_DATABASE_URL` is available.
- Passage-embedding fetch/stage tests, including the G7.5.11 `015` migration
  compile gate and Python adapter parameter fence.
- Parse source metadata fetch tests, including the G7.5.15 `016` migration
  static gate and Python adapter lease/materialization fence.
- Local object-byte gateway tests, including explicit root, backend mismatch,
  size mismatch, symlink, and composition hash verification.
- Go private source-object gateway tests, including token gate, auth-required
  middleware bypass, metadata/object mismatch redaction, and Python HTTP adapter
  lease/header/hash validation.
- Parse projection adapter tests, including lease/materialization requirements,
  context/batch mismatch rejection, JSONB payload conversion, and token-fenced
  function parameters; include the G7.5C live Python adapter proof when
  `RAG_TEST_DATABASE_URL` points at a disposable PostgreSQL database.
- Parse projection migration tests, including token fences, materialization/profile
  gates, JSONB recordset lanes, artifact-set binding, worker-only execute
  grants, and rollback.
- Worker promotion-gate tests, including dark default, outbox-only promotion,
  job-only promotion, and missing-stage-handler rejection.
- MinerU local-batch allocate tests, including missing-token no-HTTP behavior,
  PDF/size/filename gates, locked request body, retryable status/transport
  mapping, response validation, signed-upload URL validation, and redaction.
- MinerU signed-upload transport tests, including locked target URL validation,
  raw PDF PUT body, no auth/cookie/content-type headers, retryable
  status/transport mapping, non-PDF no-HTTP rejection, and redaction.
- MinerU poll/result tests, including locked poll target construction, bearer
  request shape, closed state/result JSON parsing, running progress validation,
  done-result URL target validation, retryable status/code/transport mapping,
  and redaction.
- MinerU result ZIP download tests, including done-state admission, CDN target
  reuse, no auth/cookie/content-type headers, identity encoding, ZIP content
  types, content-length/body size bounds, retryable status/transport mapping,
  and redaction.
- MinerU archive-validation tests, including redacted summary output, required
  role presence, invalid ZIPs, unsafe paths, duplicate entries, symlinks, CRC
  mismatch, entry/total expanded-size limits, compression-ratio limits, and
  no entry-name/content retention.
- MinerU archive artifact-extraction tests, including role byte extraction,
  nested role paths without name retention, duplicate semantic-role rejection,
  and validation reuse.
- MinerU artifact decode-admission tests, including strict UTF-8 Markdown,
  content-list array admission, middle/model object admission, invalid UTF-8,
  malformed JSON, duplicate JSON keys, non-finite JSON numbers, and top-level
  role type rejection.
- MinerU canonical-mapping input tests, including source hash binding, archive
  hash binding, deterministic role digest ordering, decoded payload reuse, no
  entry-name/IR/manifest exposure, source mismatch rejection, and decode-gate
  reuse.
- MinerU full-Markdown text-baseline mapper tests, including projection-ready
  `ParsedDocumentArtifacts`, deterministic `canonical-ir.v2` and
  `chunk-manifest.v2` output, G7.4 projection row compatibility, long Markdown
  chunk splitting, and wrong-input rejection.
- MinerU archive-to-text-baseline composition tests, including validated
  archive extraction, source hash binding, projection-ready output, invalid ZIP
  rejection, and source mismatch rejection.
- MinerU text-baseline parser adapter tests, including default-off dependency
  failure before archive fetch, parse-context admission, deterministic
  artifact-set derivation, projection-ready output, and archive validation reuse.
- Parse-handler dependency composition tests with the MinerU text-baseline parser
  adapter, including source fetch, archive fetch, projection build, projection
  stage, source-hash propagation, and exact-term projection.
- MinerU basic page-locator mapper tests, including single full-text content-list
  match, layout/middle page bbox admission, projected `page_bbox` block/chunk
  locators, and ambiguous locator rejection.
- MinerU `sourceText` page-locator tests for formula-like text baselines,
  including content-list and layout/middle agreement plus projected `page_bbox`
  output without Formula IR promotion.
- MinerU table element page-locator tests, including `content_list.type=table`
  agreement, opaque row/cell payloads, projected element-level `page_bbox`, and
  ambiguous table candidate rejection.
- MinerU image element page-locator tests, including `content_list.type=image`
  agreement, opaque image path/asset payloads, projected element-level
  `page_bbox`, and ambiguous image candidate rejection.
- MinerU locator hardening tests, including duplicate content-list matches,
  missing page index, malformed/negative/zero-area/non-integer bbox rejection,
  formula kind-only fallback, ambiguous formula `sourceText` rejection, and
  no-content-match line-range fallback.
- Retry-three-times and terminal-failure tests.

### G7.6 Private query and Go reauthorization

- Query only the current chat-selected collection IDs.
- Python returns evidence candidates; Go rechecks ACL, collection membership,
  document version, visibility epoch, source hash, and consent/governance state.
- Strict query fails closed if required lanes/providers are unavailable.

Validation:

- Two-user/two-team isolation tests.
- Source reauthorization rejection tests.
- Optional degradation tests.
- Prompt-injection fixture tests.

### G7.7 Strict/optional chat answer and citations

- Wire RAG answer generation into Go chat flow or a Go-owned RAG answer route.
- Strict mode refuses unknowns; normal chat may degrade with explicit metadata.
- Persist message/citation metadata atomically where applicable.
- Frontend renders basic citation markers/cards for selected Knowledge answers.

Validation:

- Strict refusal tests.
- Citation coverage/hash tests.
- Frontend adapter/render tests.
- Deployed same-origin smoke through `/mm-api`.

### G7.8 Live provider smoke and operational proof

- Run owner-authorized MinerU + Jina + Postgres smoke with small bounded files,
  including at least one PDF path and one citation-producing query.
- Capture only redacted evidence: no keys, no document bodies beyond test
  snippets intentionally used for citation display, no provider IDs unless
  allowed by the provider contract.

Validation:

- Live smoke artifact path recorded in process log.
- Provider quota target and run ID recorded without secrets.
- Cleanup/retry/rebuild evidence recorded.

### G7.9 Legacy handoff to G8/G9

- Mark G7 complete only after Go/Python/frontend server-mode path passes.
- Defer rich Knowledge UI to G8 and Next route deletion/local-authority cleanup
  to G9.

Validation:

- G7 completion checklist.
- Explicit list of G8/G9 carryovers.

## 6. Failure Matrix

| Condition                                   | Strict Knowledge result                    | Normal chat result                                |
| ------------------------------------------- | ------------------------------------------ | ------------------------------------------------- |
| Selected collection has no ready generation | Refuse with indexing status                | Degrade and mark no Knowledge evidence            |
| MinerU/Jina unavailable during indexing     | Job retries up to three times, then failed | Existing active version remains usable if present |
| Query provider/index lane unavailable       | Refuse                                     | Degrade with no Knowledge evidence                |
| Go reauthorization rejects all evidence     | Refuse                                     | Degrade or answer without Knowledge evidence      |
| Citation/source hash mismatch               | Refuse and do not persist forged citation  | Degrade or omit Knowledge citations               |
| Document deleted/tombstoned                 | Immediately invisible                      | Immediately invisible                             |
| Replacement indexing fails                  | Keep old active version                    | Keep old active version                           |

## 7. Security and Logging Rules

- Never commit provider keys or deployment env files.
- Do not read `.env.single-server` during implementation or reporting.
- Secret-bearing env values, Authorization headers, provider IDs that are not
  approved for persistence, raw provider responses, document bodies, embeddings,
  and full prompts are forbidden in logs/audit.
- Audit may store fixed provider kind, model/profile ID, dimensions, retry count,
  duration bucket, status, and sanitized error class.
- Browser BYOK keys are not used for automatic background indexing.
- Admin provider secrets are loaded server-side only and later may move to an
  admin encrypted settings UI in a separate slice.

## 8. Completion Rule

G7 completes when a clean or reset baseline can reproduce:

1. upload/bind selected Knowledge file;
2. automatic MinerU/Jina/Postgres indexing using admin server secrets;
3. selected-collection RAG query;
4. Go source reauthorization;
5. strict answer refusal for insufficient evidence;
6. successful grounded answer with basic citation cards;
7. immediate delete invisibility and replacement-ready atomic switch;
8. targeted tests plus one owner-authorized live smoke.
