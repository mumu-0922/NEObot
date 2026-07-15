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

- Add backend/RAG config for admin-owned MinerU and Jina secrets using env or
  Docker secrets.
- Expose redacted provider readiness to frontend/admin diagnostics.
- Fail closed when required provider secret or projection dependency is absent.
- Do not log secret values or read deployment `.env` contents in tests.

Validation:

- Go config tests for present/missing secret behavior and redaction.
- Python settings tests for present/missing secret behavior and redaction.
- Compose/static docs update for required env/secret names.

### G7.3 Provider-backed parser/index profile gate

- Promote the locked MinerU Local Batch plus Jina 1024 embedding/rerank profile
  from draft/dark-run into an explicit runtime profile only after required
  fixture/wire blockers for this deployment are either closed or recorded as
  accepted owner risk.
- Add rate-limit/concurrency defaults and retry budget of three attempts.
- Keep provider request/response bodies out of logs.

Validation:

- Provider profile loader tests.
- Retry/rate-limit tests.
- No secret/body log tests.

### G7.4 Canonical IR to chunks and Postgres projection

- Stage parser outputs into canonical IR and deterministic parent/child chunks.
- Apply/create the Postgres projection needed for Jina 1024 vectors,
  lexical/exact lanes, source spans, version fences, tombstones, and generations.
- Publish only complete generations.

Validation:

- Parser fixture tests for PDF and supported native formats.
- Chunk exact-cover/hash/source-locator tests.
- Migration replay/down-safety where applicable.

### G7.5 Worker dispatch, rebuild, delete, and retry loop

- Connect Go processing jobs/outbox to Python handler execution.
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
