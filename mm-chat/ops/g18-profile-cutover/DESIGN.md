# G18.5B.1 PG17 profile cutover candidate design

## Goal

Prove the smallest reversible link between the server-owned migration `037`
pointer and the already-reviewed PG17 BM25/pgvector projections. The Go method,
SQL input signature, selected-collection authority, reference-only output, and
post-candidate Go hydration remain unchanged.

## Why this remains operational DDL

The independent runtime still uses PostgreSQL 16. Committing a PG17-only
embedded migration before the blue-green restore would make normal PG16
`migrate up` fail. This slice therefore proves the exact router behavior under
`ops/` while leaving the embedded manifest at `1–37`. A later cutover slice may
promote the reviewed SQL only after the preserved PG16 backup is restored into
fresh PG17 storage and all operations gates pass.

## Activation state machine

```text
legacy @ revision 1
  -> vector backfill complete
  -> BM25 backfill complete
  -> readiness verifies every current source identity/content row
  -> compare-and-swap activation
pg17_bm25_pgvector_v1 @ revision 2
  -> restart/read proof
  -> compare-and-swap rollback
legacy @ revision 3
```

`knowledge_assert_pg17_retrieval_profile_ready()` binds the active corpus
generation to its unique Jina v4/1024 search profile. It re-verifies every
current source against both immutable projections, including identity, hashes,
visibility revisions, vector round-trip, normalized terms, and derived BM25
text. Missing or mismatched projection coverage raises
`RAG_RETRIEVAL_PROFILE_BACKFILL_INCOMPLETE` before pointer mutation.

The activation function serializes vector backfill, BM25 backfill, and pointer
mutation with advisory locks `3`, `4`, and `5` in ascending order. The pointer
row is also locked and compared against the caller's expected profile/revision.
This slice does not yet claim concurrent publication/reindex fencing; that is a
required G18.5B.2 operations proof.

## Reader and trust boundary

On `pg17_bm25_pgvector_v1`,
`knowledge_fetch_profiled_query_evidence_candidates(...)` validates the legacy
`REAL[]` query shape, casts it to `VECTOR(1024)`, invokes the reviewed hybrid
reader as the projection owner, and maps only immutable references plus the RRF
score back to the unchanged result shape. It does not expose source text,
exact terms, per-lane ranks, or per-lane scores.

`go_api_runtime` and `rag_worker_executor` execute only the profiled reader.
`rag_replay_operator` alone executes readiness, backfills, private diagnostics,
and pointer mutation. All SECURITY DEFINER functions pin the captured schema,
`pg_catalog`, and `pg_temp` without `$user`.

## Rollback

The router down script first requires `active_profile = 'legacy'`. It restores
the exact migration `037` behavior before removing the readiness function: the
profiled reader delegates only to the legacy function, and the PG17 target is
again unavailable. The G18.4 and G18.3 down scripts then remove the physical
candidate projections while retaining the original `REAL[]` source rows,
legacy reader, pointer, revision, and immutable transition history.

## Known limit

This slice proves activation correctness and restart durability on a frozen
synthetic corpus. It does not yet prove concurrent document publication,
generation reindex/cutover, representative latency, PostgreSQL RSS/CPU, or a
real backup/restore. Those gates remain mandatory before formal migration and
Compose/data-path cutover.

## Change history

- 2026-07-22: initial disposable PG17 activation and rollback candidate.
