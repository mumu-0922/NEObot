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

## 3. Contracts

- BM25 source admission requires the active corpus head, active generation,
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
- Migration rollback must fail atomically with
  `RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY` unless the active pointer is
  `legacy`. Application rollback precedes migration rollback.

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
| Migration down is attempted under a non-legacy profile | Rollback aborts atomically and migration `037` remains applied |

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
  shadow diagnostic to `go_api_runtime`, or emitting source text in a report.

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
  SELECT child_chunk_id,
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
JOIN current_authority source USING (child_chunk_id)
ORDER BY probe.score, probe.child_chunk_id;
```

The index is explicit, unrelated zeros are rejected, ordering is deterministic,
and current authority remains the final visibility gate.
