# RAG retrieval storage contracts

## 1. Scope / Trigger

Apply this contract when changing `mm-chat` lexical or Dense retrieval,
PostgreSQL extensions/major version, search projections, candidate diagnostics,
or the production retrieval profile pointer.

The current independent Compose runtime is PostgreSQL 16. PG17-only
`pg_textsearch`/pgvector DDL must remain in disposable operational modules
until a reviewed blue-green restore promotes it. Never start PG17 against
`mm-chat/data/postgres` or another PG16 data directory.

## 2. Signatures

Current G18.4 shadow signatures:

```sql
knowledge_backfill_bm25_shadow(
  p_index_generation_id UUID,
  p_search_profile_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  inserted_count BIGINT,
  verified_shadow_count BIGINT
)

knowledge_fetch_hybrid_shadow_diagnostics(
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding VECTOR(1024),
  p_limit INTEGER
) RETURNS TABLE(
  -- immutable UUID/hash references only
  bm25_rank INTEGER,
  bm25_score DOUBLE PRECISION,
  dense_rank INTEGER,
  dense_score DOUBLE PRECISION,
  fused_rank INTEGER,
  fused_score DOUBLE PRECISION
)
```

Production remains on:

```sql
knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
)
```

Migration `037` routes that stable signature to the legacy
`knowledge_fetch_hybrid_query_evidence_candidates(...)` implementation while
the durable pointer is `legacy`. The PG17 branch must remain fail-closed until
the reviewed PG17 storage migration exists and has passed its activation gates.

Profile mutation is operator-only and compare-and-swap:

```sql
knowledge_set_retrieval_profile(
  p_expected_profile TEXT,
  p_target_profile TEXT,
  p_expected_revision BIGINT,
  p_reason TEXT
) RETURNS TABLE(active_profile TEXT, revision BIGINT)
```

The accepted profile identities are `legacy` and
`pg17_bm25_pgvector_v1`; the singleton starts at `legacy` revision `1`.

The PG17 activation candidate adds:

```sql
knowledge_assert_pg17_retrieval_profile_ready()
RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
```

Active-generation publication maintenance uses:

```sql
knowledge_sync_pg17_retrieval_materialization(
  p_materialization_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  vector_inserted_count BIGINT,
  bm25_inserted_count BIGINT,
  verified_count BIGINT
)
```

Generation promotion/rollback readiness uses:

```sql
knowledge_assert_pg17_generation_ready(
  p_index_generation_id UUID
) RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  document_count BIGINT,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
```

## 3. Contracts

- BM25 reader-source admission requires the active corpus head and generation,
  ready Jina v4/1024 search row, active document projection head, published
  materialization, and current collection/document/version visibility.
- `pg_textsearch <@>` returns a negative raw BM25 score: lower is better and
  `0` is no match. Filter `score < 0`; never interpret it as a probability.
- Use explicit `to_bm25query(query, index_name)` for the intended BM25 index.
- Latin text uses `simple` tokenization. Add at most 512 CJK ideograph bigrams;
  never generate generic Latin bigrams.
- Dense shadow vectors are `vector(1024)` and must retain the Jina v4 profile,
  generation, float32 hash, finite components, and non-zero norm.
- Fuse BM25/Dense and original/rewrite lanes with deterministic
  `1 / (60 + lane_rank)` RRF plus stable UUID tie-breaks.
- Candidate indexes are not authorization authorities. Rejoin current
  authority before diagnostics, then reauthorize/hydrate again in Go before
  answer context or citations.
- Resolve bounded BM25/Dense probe IDs through a candidate-driven `LATERAL`
  current-authority lookup. Do not let PostgreSQL decorrelate the authority
  view into a corpus-wide or per-candidate repeated expansion; retain an
  optimizer fence such as `OFFSET 0` and prove it with representative latency.
- The BM25 build/active source views are internal accelerator authorities,
  owned and directly readable only by `rag_projection_owner`. They do not use
  `security_barrier`, because it blocks child-ID predicate pushdown and caused
  corpus-wide materialization. This exception is valid only while `PUBLIC` and
  runtime roles have no direct `SELECT`, the SECURITY DEFINER reader returns
  references only, and Go performs final hydration reauthorization.
- Shadow diagnostics may expose UUIDs, hashes, ranks, and scores only. Do not
  output source text, exact terms, provider credentials, or private queries.
- Only `rag_replay_operator` receives shadow EXECUTE. Production runtime roles
  receive no shadow table/function privileges.
- The retrieval profile head is a database-owned singleton with a monotonic
  revision. Profile mutations use compare-and-swap over expected profile and
  revision, append immutable transition history, and are executable only by
  `rag_replay_operator`.
- A profile target whose schema or verified backfill is unavailable must raise
  `RAG_RETRIEVAL_PROFILE_UNAVAILABLE` without mutating pointer state.
- PG17 activation must bind the active generation to its unique Jina v4/1024
  search profile and re-verify every current source row against both physical
  projections. Matching counts alone are insufficient; identity, hashes,
  visibility revisions, vector round-trip, normalized terms, and derived BM25
  text must agree.
- Serialize vector backfill, BM25 backfill, and pointer activation with
  advisory locks `3`, `4`, and `5` acquired in ascending order. Readiness and
  pointer mutation occur inside the same activation call.
- Keep PG17-only candidate DDL outside embedded migrations while the deployed
  database is PG16. Promote it to migration `038` only on the verified fresh
  PG17 restore; otherwise normal PG16 `migrate up` would become unusable.
- When the PG17 profile is active, the AFTER trigger on
  `knowledge_document_projection_heads` is the publication boundary. It must
  populate and verify both projections in the same transaction as the head
  mutation; partial projection success or a query-visible unsynchronized head
  is forbidden.
- When the pointer is `legacy`, the maintenance trigger is a no-op. Before
  later PG17 activation, readiness/backfill must close any rows published in
  that interval.
- Direct materialization sync is operator-only, idempotent, and locks vector
  then BM25 (`3 -> 4`). Runtime publication reaches it only through the hardened
  projection-owner trigger.
- Separate write admission from read authority. The BM25 build source may admit
  current published heads from `building`, `verified`, `active`, or `retired`
  generations, matching the pgvector build source. The BM25 reader source must
  remain joined to the singleton active corpus head.
- While the PG17 profile is active, every corpus-head generation change must
  acquire advisory locks `3 -> 4` and verify complete current-document
  coverage, paired BM25/vector source identity, and exact physical projection
  coverage for the target generation. The fence applies equally to promotion
  and rollback and aborts the surrounding transaction on failure.
- Migration rollback must fail atomically with
  `RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY` unless the active pointer is
  `legacy`. Application rollback precedes migration rollback.
- Pre-cutover resource qualification uses a hard 1 GiB / 2 CPU PG17 container,
  a 4096-child active-generation publication, 30 production-shaped profiled
  reads, and these gates: backfill `<= 120s`, query P95 `<= 500ms`, query max
  `<= 1000ms`, combined vector/BM25 physical storage `<= 512MiB`, and cgroup
  memory peak `<= 900MiB`.
- A qualified active-PG17 logical backup must be checksummed, restored into a
  fresh `template0` database, and re-verified for migration idempotence,
  profile revision, active/physical row counts, runtime reader behavior,
  operational functions, and role grants.

## 4. Validation & Error Matrix

| Condition | Required result |
|---|---|
| PG major is not 17 or extension version differs | DDL aborts before creating shadow objects |
| Generation/profile is null, inactive, or incompatible | `RAG_BM25_SHADOW_ARGUMENT_INVALID` or `RAG_BM25_SHADOW_PROFILE_MISMATCH` |
| Insert identity does not match current source | `RAG_BM25_SHADOW_SOURCE_MISMATCH` |
| Derived BM25 text/terms differ | `RAG_BM25_SHADOW_CONTENT_MISMATCH` |
| Backfill postcondition count differs | `RAG_BM25_SHADOW_BACKFILL_INCOMPLETE` |
| Collections/query/limit/vector invalid or vector norm is zero | `RAG_HYBRID_SHADOW_ARGUMENT_INVALID` |
| Document/collection becomes non-current | Candidate disappears immediately; immutable rollback row may remain |
| Reranker is configured but unavailable/unauthorized/malformed | No Knowledge evidence or citation is minted |
| Profile compare-and-swap sees a stale expected profile/revision | `RAG_RETRIEVAL_PROFILE_CONFLICT`; pointer/history unchanged |
| PG17 profile is selected before its implementation is available | `RAG_RETRIEVAL_PROFILE_UNAVAILABLE`; pointer unchanged |
| Active generation/profile is missing at PG17 activation | `RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING`; pointer unchanged |
| Either PG17 projection is incomplete or mismatched | `RAG_RETRIEVAL_PROFILE_BACKFILL_INCOMPLETE`; pointer/history unchanged |
| Active-profile materialization source is absent/incomplete | `RAG_RETRIEVAL_MATERIALIZATION_SOURCE_INCOMPLETE`; head/publish transaction aborts |
| Post-insert vector or BM25 verification is incomplete | `RAG_RETRIEVAL_MATERIALIZATION_SYNC_INCOMPLETE`; both projection writes and head mutation roll back |
| Target generation lacks any current document, paired source, vector, or BM25 row | `RAG_RETRIEVAL_GENERATION_BACKFILL_INCOMPLETE`; generation/head transition rolls back atomically |
| Migration down is attempted under a non-legacy profile | Rollback aborts atomically and migration `037` remains applied |
| Any representative backfill/latency/storage/memory gate is exceeded | The disposable qualification aborts; migration `038` remains forbidden |
| Restored PG17 state loses profile, rows, reader behavior, functions, or grants | Restore qualification aborts; no Compose/data-path cutover |

Every SECURITY DEFINER function must pin the current schema followed by
`pg_catalog, pg_temp` and must not resolve through `$user`.

## 5. Good / Base / Bad Cases

- **Good:** an active published Jina v4 row backfills once, replays with zero
  inserts, returns exact identifier/CJK/Dense candidates, and disappears after
  tombstone reauthorization.
- **Base:** unrelated weather/cooking queries return zero candidates in both
  lexical and Dense lanes; ordinary Model/Web answering may continue without a
  Knowledge citation.
- **Bad:** mixing Jina and BGE vectors because both have 1024 dimensions,
  mounting a PG16 directory into PG17, accepting BM25 score `0`, granting a
  shadow diagnostic to `go_api_runtime`, emitting source text in a report, or
  joining a bounded probe to an authority view that expands the entire corpus.

## 6. Tests Required

The disposable drill must assert:

1. all current production migrations apply and remain unchanged afterward;
2. idempotent backfill count/identity/content/hash postconditions;
3. exact identifiers, paths, phrases, Chinese lexical/bigram recall, semantic
   Dense recall, context rewrite, cross-collection selection, and two unrelated
   no-evidence cases;
4. repeated BM25/Dense and original/rewrite RRF ordering is deterministic;
5. EXPLAIN uses the intended BM25 and HNSW indexes;
6. production roles lack access while `rag_replay_operator` executes live;
7. a tombstone is immediately invisible without destroying rollback data;
8. G18.4-only rollback retains G18.3, final rollback retains `REAL[]`, and all
   disposable containers/volumes are removed.
9. The PG16-compatible profile reader is row-for-row identical to the legacy
   reader at the default pointer; runtime roles cannot mutate the pointer;
   unavailable/conflicting transitions and non-legacy migration rollback fail
   without partial state changes.
10. The PG17 candidate rejects partial backfill, activates only after exact
    readiness, returns reference-only results under both runtime roles,
    survives restart, rejects active-profile rollback, and restores exact
    legacy parity before removing PG17 objects.
11. Two concurrent active-generation head publications populate both
    projections atomically, become immediately query-visible, replay with zero
    inserts, retain physical rows after deletion, disappear from authorized
    reads immediately, and survive restart/rollback checks.
12. A partially indexed building generation receives physical rows but remains
    absent from the active reader; attempted corpus-head cutover is atomic and
    rejected. Complete publication permits promotion, serves only target-
    generation references, and permits exact rollback while retaining both
    generations' physical rows.
13. A 4096-child publication and 30 profiled reads pass the fixed single-server
    backfill/latency/storage/memory gates; restart and a checksummed logical
    backup/restore preserve exact active/physical rows, profile state, reader
    behavior, functions, and role boundaries.

After the drill, run `go vet ./...`, `go test ./...`, and the frozen G18
evaluator.

## 7. Wrong vs Correct

### Wrong

```sql
-- `0` is no match, not a useful probability score.
ORDER BY bm25_text <@> p_query
LIMIT 20;
```

This can return unrelated zero-score rows and does not bind the intended index
or current authority.

### Correct

```sql
WITH probe AS (
  SELECT child_chunk_id, content_hash,
    bm25_text <@> to_bm25query(p_query, 'reviewed_bm25_index') AS score
  FROM bm25_shadow
  WHERE bm25_text <@> to_bm25query(
    p_query,
    'reviewed_bm25_index'
  ) < 0
  ORDER BY score, child_chunk_id
  LIMIT p_oversample
)
SELECT probe.child_chunk_id, probe.score
FROM probe
CROSS JOIN LATERAL (
  SELECT source.child_chunk_id
  FROM current_authority source
  WHERE source.child_chunk_id = probe.child_chunk_id
    AND source.content_hash = probe.content_hash
  OFFSET 0
) authorized
ORDER BY probe.score, probe.child_chunk_id;
```

The index is explicit, unrelated zeros are rejected, ordering is deterministic,
and current authority remains the final visibility gate without being
decorrelated into a corpus-wide join.
