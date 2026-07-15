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

Remaining G7.5 work:

- Replace the skeleton stops with real admitted Python parse /
  passage-embedding / purge implementations connected to Generation-bound Go
  jobs and outbox payloads.
- Implement index, reprocess, tombstone purge, rebuild, retry, and DLQ behavior.
- Enforce immediate query invisibility for deleted/tombstoned versions.

Validation:

- Outbox duplicate/out-of-order/replay tests.
- Delete/tombstone/rebuild tests.
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
