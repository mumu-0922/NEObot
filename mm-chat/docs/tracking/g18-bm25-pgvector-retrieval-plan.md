# G18 BM25 and pgvector Retrieval Plan

Status: in progress. G18 promotes the independent server RAG path from
PostgreSQL `ts_rank` plus `REAL[]` cosine scans to true BM25 plus indexed
pgvector retrieval without weakening current authority, deletion, citation,
or rollback boundaries.

## Execution rule

- Deliver one bounded group at a time.
- Test, record, and commit every completed group before starting the next.
- Keep the current production reader until shadow quality and operations gates
  pass.
- Keep source text, provider credentials, and private user data out of fixtures,
  reports, logs, and commits.
- Never mount the PostgreSQL 16 data directory into PostgreSQL 17.

## G18.1 Evaluation and relevance gate

Status: complete (2026-07-22).

- Add a versioned synthetic Golden Set covering exact identifiers, Chinese
  lexical/semantic questions, contextual rewrites, cross-collection ranking,
  and unrelated negatives.
- Add a deterministic evaluator for per-lane recall, final-context precision,
  negative false-citation rate, no-evidence accuracy, and P95 latency.
- Record the existing `ts_rank + REAL[] + jina-reranker-v3` baseline before
  changing storage.
- Calibrate against the complete Golden Set; do not infer that reranker scores
  are probabilities or raise a global threshold from isolated examples.
- Fail closed to ordinary Model/Web answering when configured reranking is not
  authorized, unavailable, or malformed; unreranked candidates cannot mint
  Knowledge citations.

Promotion baseline: all required lanes recalled their approved evidence,
final-context precision and no-evidence accuracy were `1.0`, negative
false-citation rate was `0`, and Knowledge-stage P95 was `25.402s` on the live
disposable synthetic set. The high P95 is owned by the contextual rewrite
provider path and remains a comparison guard for later storage groups.

## G18.2 PostgreSQL 17 image and restore drill

- Build and digest-pin a reviewed PostgreSQL 17 image containing
  `pg_textsearch` and pgvector.
- Restore a disposable PostgreSQL 16 backup into PostgreSQL 17 and verify
  migrations, authority rows, projections, generation heads, and objects.
- Prove rollback from the preserved PostgreSQL 16 backup.

## G18.3 Shadow pgvector projection

- Add a generation/profile-bound `vector(1024)` projection beside `REAL[]`.
- Backfill compatible finite Jina v4 vectors transactionally without provider
  calls.
- Evaluate exact cosine first, then HNSW recall and latency; keep the existing
  reader in production.

## G18.4 BM25 and hybrid dual read

- Add BM25 only for active, published, current-generation child chunks.
- Preserve identifier, path, error-code, phrase, Chinese, and semantic recall.
- Fuse BM25 and Dense lanes through deterministic RRF and emit only redacted
  shadow diagnostics.

## G18.5 Cutover and rollback

- Cut over behind a reversible server-owned retrieval-profile pointer.
- Prove restart, concurrent indexing, deletion, reindex, backup/restore,
  resource budget, and rollback.
- Retain `REAL[]` rollback data through the observation window.

## G18.6 Optional BGE-M3 shadow benchmark

- Use a separate immutable embedding generation and never mix vector spaces.
- Compare BGE-M3 and Jina v4 on the same frozen Golden Set.
- Require an explicit decision and full rebuild/cutover plan before any
  production model switch.

## Verification

- Focused Go evaluator/chat tests for G18.1.
- Disposable database integration tests for every storage group.
- Backend `gofmt`, `go vet ./...`, and `go test ./...` after each Go group.
- Python checks when the RAG worker is touched.
- Live provider/database proof only with synthetic fixtures and complete
  cleanup.

## Rollback

G18.1 adds no schema. G18.2 is blue-green and retains the PostgreSQL 16
backup. G18.3 and G18.4 are shadow-only until G18.5. The production reader and
legacy `REAL[]` projection remain the rollback anchors through observation.
