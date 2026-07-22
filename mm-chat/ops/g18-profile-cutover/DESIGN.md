# G18.5B PG17 profile cutover qualification design

## Goal

Prove the smallest reversible link between the server-owned migration `037`
pointer and the already-reviewed PG17 BM25/pgvector projections. The Go method,
SQL input signature, selected-collection authority, reference-only output, and
post-candidate Go hydration remain unchanged.

## Operational provenance and formal migration boundary

The operational modules first proved the exact router behavior while the
embedded manifest remained at `1–37`. After every operations/resource gate
passed and the owned live PG16 dump restored into an isolated fresh PG17
volume, those reviewed modules were frozen into formal migration `038`.

Migration `038` checks PostgreSQL major `17`, the `pg_textsearch` preload, and
exact extension versions before creating either extension and any projection
object. It is intentionally incompatible with PG16. Until the separately
approved production blue-green switch, the still-running PG16 Compose stack
must not execute the new manifest; the existing process and data directory
remain untouched.

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
The initial activation function alone does not fence later publication or
reindex. The following G18.5B.2a maintenance closes active-generation
publication; generation rebuild/cutover remains a separate gate.

## Active-generation publication maintenance

G18.5B.2a attaches an AFTER trigger to the decisive durable mutation emitted by
`knowledge_complete_embedding_and_publish(...)`:

```text
materialization published
  -> document projection head inserted/advanced
  -> active PG17 maintenance trigger
  -> vector + BM25 rows inserted and fully re-verified
  -> transaction commits
```

While the pointer is `legacy`, the trigger is a no-op; activation readiness
later catches any projection gap and requires an operator backfill. While the
pointer is `pg17_bm25_pgvector_v1`, head mutation and both physical projection
writes are atomic. A source/conversion/verification failure aborts the head
mutation and therefore the surrounding embedding-publish transaction.

`knowledge_sync_pg17_retrieval_materialization(UUID)` acquires advisory locks
`3` then `4`, the same order used by activation. It writes from generation-
scoped, published, current-document build sources that admit `building`,
`verified`, `active`, and `retired` generations. The separate reader source
remains restricted to the active corpus head, so building or rollback rows
cannot become query authority early. The sync inserts both projections with
idempotent conflict handling, then re-verifies complete identity/content
coverage before returning. Only `rag_replay_operator` can call it directly;
normal publication reaches it only through the SECURITY DEFINER trigger.

The disposable proof publishes two prepared heads from independent sessions.
The global locks serialize the small projection critical section without
weakening the surrounding database authority. Both candidates become visible,
an idempotent replay inserts zero rows, and tombstoning one document hides it
immediately while all physical rollback rows remain.

## Generation rebuild and corpus-head fence

G18.5B.2b separates BM25 build admission from read authority. The build source
allows a rebuilding generation to populate and validate pgvector/BM25 rows as
each document projection head is published. `knowledge_bm25_shadow_sources`
still joins the singleton corpus head and therefore exposes only the active
generation to candidate readers.

`knowledge_assert_pg17_generation_ready(UUID)` requires every current active
document to have at least one source child in the selected Jina v4/1024 search
profile. It pairs BM25 and Dense sources on immutable identity and visibility
fields, then verifies exact vector and BM25 physical rows. A BEFORE trigger on
`knowledge_corpus_projection_head.active_index_generation_id` acquires locks
`3 -> 4` and runs that assertion whenever the PG17 retrieval profile is active.
Because promotion and rollback change generation status before advancing the
corpus head, both paths cross the same final fence; an incomplete target aborts
the surrounding transaction without leaving partial generation state.

The disposable proof staged one of three current documents and confirmed the
head switch failed with `RAG_RETRIEVAL_GENERATION_BACKFILL_INCOMPLETE`. After
all three heads were published, backfill replay inserted zero rows, promotion
served only the new child references, and rollback restored the old references
while retaining the new generation's immutable projection rows.

## Resource and restore qualification

G18.5B.2c adds a 4096-child single-user corpus to the already active generation
without publishing its document projection head. Advancing that one head is
the measured atomic backfill boundary: the trigger inserts and verifies both
physical projections before the transaction commits. The hard envelope is a
1 GiB / 2 CPU PG17 container, 120-second publication backfill, 500 ms query
P95, 1000 ms query maximum, 512 MiB combined physical projections/indexes, and
900 MiB cgroup memory peak.

Thirty production-shaped hybrid reads select the real collection, vary exact
terms and 1024-dimensional query vectors, require the intended child in the
bounded result, and measure the unchanged profiled reader. The first resource
attempt exposed a corpus-wide authority rejoin: the optimizer repeatedly
expanded thousands of active-source rows for a handful of Dense candidates.
The active BM25 source views are internal, owner-only accelerator authorities;
they are not exposed to runtime roles and therefore do not use
`security_barrier`. Candidate reauthorization is driven through a bounded
`LATERAL` lookup with an optimizer fence so each probe resolves by immutable
child identity instead of materializing or repeating the whole corpus. The Go
hydration boundary still performs final authority checks before text or
citations are emitted.

After resource qualification, the harness restarts PostgreSQL, creates a
custom-format logical backup of the active PG17 database, records and verifies
its SHA-256, restores into a fresh `template0` database, and proves migration
idempotence. The restored database must retain profile revision, active and
physical row counts, operational functions, runtime reader behavior, and the
replay/runtime role boundary before rollback testing continues.

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

## Remaining boundary

This module proves activation, restart durability, concurrent publication,
generation reindex/cutover, representative synthetic resource budgets, and an
active-PG17 logical backup/restore. G18.5B.3a additionally proved checksummed
live PostgreSQL/MinIO backup, owned-role restore into isolated PG17, migration
`038` extension creation, live-row backfill, active rollback refusal,
controlled down/re-up, idempotence, runtime role boundaries, and restart.

The only remaining boundary is G18.5B.3b: take a fresh stop-window backup,
restore into the production green PG17 data path, activate, and switch Compose
traffic. The old PG16 data directory and backup remain rollback authorities.

## Change history

- 2026-07-22: initial disposable PG17 activation and rollback candidate.
- 2026-07-22: active-generation atomic publication maintenance and concurrent
  publish/delete proof.
- 2026-07-22: building-generation maintenance plus atomic promotion/rollback
  readiness fence.
- 2026-07-22: 4096-child resource qualification, candidate-driven authority
  lookup, and active-PG17 backup/restore proof.
- 2026-07-22: formal migration `038` qualification against restored live data,
  including extension creation, down/re-up replay, and restart proof.
