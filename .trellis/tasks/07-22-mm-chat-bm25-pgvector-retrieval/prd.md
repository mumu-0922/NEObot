# mm-chat BM25 and pgvector retrieval promotion

## Goal

Promote the independent `mm-chat` server RAG retrieval path from PostgreSQL
`ts_rank` plus extension-independent `REAL[]` cosine scans to true BM25 plus
indexed pgvector Dense retrieval, while preserving the existing Go authority,
selected-collection ACL, generation-bound publication, Jina v4 embeddings,
Jina reranking, citations, single-user deployment model, and rollback path.

## Requirements

- Execute in small groups. Each group must be implemented, tested, recorded,
  and committed before the next group begins.
- Build a reproducible retrieval Golden Set before changing the database
  engine. It must cover exact identifiers, Chinese lexical and semantic
  questions, context-dependent follow-ups, cross-collection ranking, and
  unrelated negatives.
- Replace the lexical baseline with true BM25. The preferred implementation is
  `pg_textsearch` under the PostgreSQL License rather than silently adding an
  AGPL database dependency.
- Replace `REAL[]` query-time full cosine scans with pgvector `vector(1024)` and
  an evaluated cosine index profile.
- Keep `jina-embeddings-v4` at 1024 dimensions for the production migration.
  Existing valid vectors should be shadow-backfilled without consuming new
  embedding quota when their generation/profile identity matches.
- Keep `jina-reranker-v3`, but replace the weak final `>= 0.0` gate with a
  Golden-Set-calibrated relevance policy before retrieval promotion.
- Do not select that policy by blindly raising one global reranker threshold.
  Historical live scores overlap between useful and unused candidates
  (`0.09505704` useful versus `0.11554055` unused), so promotion must be based
  on the frozen Golden Set, per-lane/final metrics, and explicit no-evidence
  behavior rather than an assumed probability interpretation.
- Preserve deterministic RRF across BM25 and Dense lanes and across original
  and context-rewritten query lanes.
- Preserve current-authority reauthorization and hydration after candidate
  selection. BM25/vector indexes are never authorization authorities.
- Use a shadow/dual-read migration. Do not overwrite the current retrieval
  projection or cut over until parity, relevance, latency, deletion, restart,
  restore, and rollback checks pass.
- Upgrade from PostgreSQL 16 only through a backup/restore or equivalent
  blue-green procedure. Never start PostgreSQL 17 directly against the current
  PostgreSQL 16 data directory.
- Benchmark BGE-M3 only as a later shadow generation. Do not mix Jina and BGE
  vectors or trigger a production re-embedding during the BM25/pgvector cutover.
- Do not add new frontend settings for BM25, pgvector, TopK, or thresholds;
  these remain server-owned retrieval profile decisions.
- Do not expose raw private document text or provider credentials in evaluation
  artifacts, logs, metrics, or frontend diagnostics.

## Acceptance Criteria

### Group 1 — Evaluation and strict relevance gate

- [x] A versioned Golden Set schema and fixture cover relevant and unrelated
      multilingual retrieval cases without private source text.
- [x] A deterministic evaluator reports lane recall, final context precision,
      false-positive citations, no-evidence behavior, and latency.
- [x] Current `ts_rank + REAL[] + Jina rerank` behavior has a recorded baseline.
- [x] The final relevance policy rejects the unrelated negative set while
      retaining the approved relevant set.
- [x] Reranker-unavailable degradation cannot mint low-confidence citations.
- [x] A policy change is rejected when it improves one scalar threshold while
      regressing a required Golden Set case or no-evidence behavior.

### Group 2 — PostgreSQL 17 extension image and restore drill

- [x] A digest-pinned PostgreSQL 17 image contains reviewed `pg_textsearch` and
      pgvector versions and passes extension/readiness checks.
- [x] A disposable PostgreSQL 16 backup restores into PostgreSQL 17 with all
      migrations, authoritative rows, object references, and generation heads
      intact.
- [x] Starting the new image against a PostgreSQL 16 data directory is guarded
      and documented as invalid.
- [x] Rollback restores the PostgreSQL 16 backup without relying on the upgraded
      data directory.

### Group 3 — Shadow vector projection

- [x] A generation/profile-bound `vector(1024)` projection exists alongside the
      current `REAL[]` projection.
- [x] Existing finite 1024-lane Jina vectors backfill transactionally with
      count, norm, hash, ownership, generation, and deletion validation.
- [x] Exact and approximate cosine results pass the Golden Set and ACL/deletion
      tests before HNSW is eligible for promotion.
- [x] The existing retrieval path remains the production reader.

### Group 4 — BM25 lane and hybrid dual read

- [x] BM25 indexes only active, published, current-generation child chunks.
- [x] Chinese, identifiers, paths, error codes, and exact phrases remain
      recallable through tested tokenization/exact behavior.
- [x] BM25 and pgvector ranks fuse deterministically through RRF.
- [x] Shadow results include per-lane ranks and scores without leaking source
      text.
- [x] Golden Set quality is not worse than the baseline and false citations are
      reduced to the accepted gate.

### Group 5 — Cutover, operations, and rollback

- [ ] Production candidate reads cut over behind a reversible server feature
      gate/profile pointer.
- [x] Restart, concurrent indexing, deletion, reindex, backup/restore, and
      rollback proofs pass.
- [x] Query latency and PostgreSQL RSS/CPU remain inside the single-server
      deployment budget.
- [ ] Legacy `REAL[]` compatibility data is retained until the observation
      window closes; deletion requires a separate reviewed task.

Progress checkpoint (G18.5A, 2026-07-22):

- [x] A PostgreSQL 16-compatible migration added the durable server-owned
      profile pointer and moved the Go candidate reader behind it while the
      active profile remains `legacy`.
- [x] The pointer defaults to `legacy` revision `1`, uses compare-and-swap
      transitions, fails closed for the unavailable PG17 profile, and blocks
      migration rollback while a non-legacy profile is active.
- [x] Disposable PG16 integration proved exact legacy-reader parity, bounded
      runtime privileges, restart/reapply behavior, and clean rollback.
- [ ] G18.5B must add the PG17 BM25/pgvector implementation, backfill and
      activation path, complete the operational/resource gates above, and
      perform the reviewed blue-green production cutover.

Progress checkpoint (G18.5B.1, 2026-07-22):

- [x] A PG17-only operational candidate connected the durable pointer to the
      reviewed BM25/pgvector projections without adding an incompatible
      embedded migration to the still-running PG16 deployment.
- [x] Activation requires complete identity/content-verified coverage of the
      active Jina v4/1024 generation, preserves the existing reference-only Go
      SQL signature, survives restart, and rolls back through compare-and-swap
      to exact legacy behavior.
- [ ] G18.5B.2 must prove concurrent publication, reindex/generation cutover,
      representative latency and PostgreSQL RSS/CPU budgets, and backup/restore
      behavior before the operational SQL can become migration `038`.
- [ ] G18.5B.3 must create and apply the reviewed formal migration only on the
      restored PG17 target, then cut over Compose/data-path authority with the
      preserved PG16 backup as rollback.

Progress checkpoint (G18.5B.2a, 2026-07-22):

- [x] While the PG17 profile is active, advancing a document projection head
      transactionally inserts and fully verifies that materialization in both
      physical projections; failure cannot commit a query-visible head alone.
- [x] Two independent concurrent publications succeeded without a visibility
      gap, replay was idempotent, deletion became immediately invisible while
      rollback rows remained, and restart/legacy rollback still passed.
- [x] Active-generation publication maintenance is complete; generation
      rebuild/reindex cutover was then closed by G18.5B.2b.

Progress checkpoint (G18.5B.2b, 2026-07-22):

- [x] Building-generation projection heads now populate both PG17 physical
      projections without entering the active-reader authority view.
- [x] The corpus-head mutation is fenced by complete current-document,
      paired-source, and exact physical-projection readiness while the PG17
      profile is active.
- [x] An incomplete target generation failed atomically; a complete synthetic
      generation promoted, served only new references, and rolled back to the
      exact prior references while retaining immutable candidate rows.
- [x] Representative latency/backfill/index-size/RSS/CPU qualification and a
      disposable active-PG17 backup/restore completed in G18.5B.2c.

Progress checkpoint (G18.5B.2c, 2026-07-22):

- [x] A 4096-child active-generation publication completed in `11.019s`
      under the real maintenance trigger and exact dual-projection checks.
- [x] Thirty production-shaped hybrid reads completed at `230.241ms` P95 and
      `241.324ms` maximum after candidate-driven authority lookup replaced the
      corpus-wide rejoin exposed by the first resource run.
- [x] Combined vector/BM25 physical storage was `64,446,464` bytes and cgroup
      memory peaked at `347,545,600` bytes under a hard `1 GiB / 2 CPU`
      container.
- [x] The active PG17 state survived restart, checksummed logical backup,
      restore into a fresh database, migration idempotence, reader/role/row
      verification, controlled legacy rollback, and disposable cleanup.
- [ ] G18.5B.3 still owns the verified live PG16 backup, restore into fresh
      PG17 storage, migration `038`, and blue-green Compose/data-path cutover.

### Group 6 — Optional BGE-M3 shadow benchmark

- [ ] BGE-M3 uses a separate immutable model/profile/generation identity.
- [ ] All BGE document and query vectors are produced by BGE-M3; vector spaces
      are never mixed.
- [ ] Jina v4 and BGE-M3 are compared on the same frozen Golden Set, retrieval
      settings, and reranker policy.
- [ ] No production model switch occurs without an explicit user decision and
      a complete rebuild/cutover/rollback task.

## Definition of Done

- Each completed group has focused unit/integration tests and a recorded live or
  disposable-database proof where applicable.
- Go tests, Python tests, migration contract tests, frontend contract tests (if
  affected), formatting, lint, and type checks pass for the touched layers.
- Plan and process logs are updated with exact commands, outcomes, rollback
  state, and redacted evidence.
- Every completed group is committed separately using the repository's concise
  conventional commit style.
- Production data is never destructively migrated without a verified backup and
  tested restore.

## Technical Approach

The target production path is:

```text
message + bounded conversation context
  -> original query + conditional standalone rewrite
  -> selected Knowledge collections only
       BM25 lane (`pg_textsearch`)
       Dense cosine lane (`pgvector vector(1024)` + evaluated index)
  -> deterministic RRF
  -> `jina-reranker-v3`
  -> calibrated relevance/no-evidence policy
  -> current-authority hydration
  -> answer context + citations
```

The database transition is blue-green/shadow-first:

```text
PG16 backup -> disposable PG17 restore -> extension verification
            -> shadow vector/BM25 projection -> dual-read evaluation
            -> reversible profile-pointer cutover -> observation window
```

## Decision (ADR-lite)

**Context:** The current implementation already has Dense recall, RRF, Jina
reranking, and citations, but uses PostgreSQL `ts_rank` rather than true BM25,
stores embeddings as `REAL[]`, computes cosine through row expansion, and uses a
weak final reranker threshold.

**Decision:** Promote storage/retrieval to PostgreSQL 17 plus `pg_textsearch`
and pgvector through a shadow migration. Retain Jina v4 and Jina Reranker for the
production migration. Treat BGE-M3 as a later isolated benchmark rather than a
simultaneous provider and storage rewrite.

**Consequences:** The project gains true BM25, indexed vector search, and a
measurable relevance gate without forcing immediate re-embedding. The cost is a
PostgreSQL major-version restore, a custom/pinned database image, larger
operational proof, and temporary dual projections. BGE-M3 remains available as
a future self-hosted alternative but is not allowed to complicate the database
promotion.

## Out of Scope

- Replacing MinerU parsing or the structure-aware Parent/Child chunk planner.
- Enabling Jina visual/multi-vector retrieval in the production cutover.
- Storing BGE-M3 Sparse or ColBERT vectors in this migration.
- Exposing retrieval tuning controls to the browser.
- Dropping current `REAL[]` columns or deleting rollback data during the initial
  observation window.
- Adding team/multi-tenant behavior.

## Technical Notes

- Current Compose database: `mm-chat/compose.single-server.yml` uses
  `postgres:16-alpine` with a bind-mounted data directory.
- Current lexical ranking: migration `026` uses `ts_rank`, exact terms, phrase
  matching, and bounded CJK bigrams; it must not be described as BM25.
- Current Dense ranking: migration `027` expands 1024-lane `REAL[]` vectors and
  applies a cosine threshold of `0.48` before RRF.
- Current final reranker threshold:
  `backend/internal/chat/rag_assembly.go` sets
  `ragRerankScoreThreshold = 0.0`.
- Current security boundary: retrieval returns immutable references, then Go
  reauthorizes/hydrates only selected, active, published, current-generation
  evidence.
- `pg_textsearch` currently requires PostgreSQL 17 or 18 and
  `shared_preload_libraries`; pgvector supports PostgreSQL 13+.
- Same vector dimension does not mean same vector space. Jina v4 and BGE-M3
  vectors are incompatible and require separate generations.

## Research References

- [`research/storage-engine-selection.md`](research/storage-engine-selection.md)
- [`research/embedding-model-comparison.md`](research/embedding-model-comparison.md)
