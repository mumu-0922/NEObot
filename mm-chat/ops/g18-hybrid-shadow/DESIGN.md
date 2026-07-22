# G18.4 BM25 and hybrid shadow retrieval design

## Goal and non-goals

The module proves true `pg_textsearch 1.3.1` BM25 and pgvector cosine retrieval
can share the current immutable evidence identity, selected-collection scope,
current-authority fences, and deterministic RRF behavior.

It does not modify the running PostgreSQL 16 Compose service, add a production
migration, expose a browser tuning surface, grant a production runtime role,
call Jina, rerank source text, or cut over the Go reader. Those promotion and
operational duties remain G18.5.

## Architecture

```text
active corpus head + published/current authority
       |
       +--> knowledge_bm25_shadow_sources
       |      lexical text is internal, never diagnostic output
       |             |
       |             v
       |    verified BM25 shadow backfill
       |      simple tokenizer + exact terms + bounded CJK bigrams
       |             |
       |             v
       |    pg_textsearch BM25 index
       |
       +--> G18.3 vector(1024) shadow + HNSW
                      |
                      v
           deterministic lane ranks
                 RRF k=60
                      |
                      v
        UUID/hash/rank/score diagnostics only
```

## Authority and identity

`knowledge_bm25_shadow_sources` joins the active corpus head, active generation,
ready Jina v4/1024 search row, active projection head, published materialization,
and current collection/document/version visibility revisions. The shadow table
copies the complete immutable reference and hash tuple plus derived BM25 text.
A `BEFORE INSERT` validation trigger re-reads the source view and rejects forged
identity, terms, or derived text.

Immutable shadow rows remain after a document tombstone to preserve a rollback
artifact. They are no longer present in the authority source view, and every
diagnostic read joins that view again, so they become immediately invisible.
G18.5 must connect the same authority rule to normal indexing and purge
operations before production cutover.

## Lexical behavior

`pg_textsearch` returns a negative BM25 score: smaller values rank higher and
`0` means no match. The reader therefore filters `score < 0`; it never treats
the score as a probability.

The `simple` text configuration already tokenizes Latin words, identifiers, and
paths. Exact-term overlap is retained as a deterministic lexical ordering
signal. Chinese text additionally receives at most 512 overlapping ideograph
bigrams. Latin bigrams are deliberately forbidden: an early probe showed that
short English fragments such as `er` made unrelated weather/cooking queries
match identifiers and policy text.

The explicit `to_bm25query(query, index_name)` form binds the intended index.
The proof requires an actual
`Index Scan using idx_knowledge_child_bm25_shadow_text`.

## Hybrid and query-lane fusion

BM25 and Dense candidates each receive deterministic ranks with UUID tie-breaks.
A candidate receives `1 / (60 + lane_rank)` from each lane, and fused results
are ordered by score, best lane rank, then child UUID. The diagnostic function
returns nullable per-lane rank/score fields and final rank/score, but no lexical,
chunk, parent, or hydrated source text.

The current Go assembler may run both original and standalone-rewritten query
lanes and performs another deterministic `k=60` RRF. The drill executes that
outer fusion twice and requires byte-equivalent UUID ordering. Storage does not
attempt conversation rewrite itself.

## Security

- Only `rag_replay_operator` can call backfill or diagnostics.
- `go_api_runtime` and `rag_worker_executor` receive no G18.4 table or function
  access, so shadow data cannot become the production reader accidentally.
- All SECURITY DEFINER functions pin the current schema followed by
  `pg_catalog, pg_temp`, with no `$user` resolution.
- Input bounds cover collection count, query bytes, limit, vector dimension,
  finiteness, and non-zero norm.
- Fixtures contain synthetic identifiers only. Reports contain references,
  ranks, scores, plans, and pass/fail summaries, never source text or secrets.

## Known limits before cutover

`pg_textsearch` may apply selective collection/authority filters after its
Top-K index step. G18.4 uses bounded 8x overfetch and current-authority
reauthorization, which is sufficient for the synthetic proof but is not yet a
single-server production budget. G18.5 must calibrate overfetch, latency,
PostgreSQL RSS/CPU, deletion/reindex concurrency, and restart behavior on a
representative corpus before moving the profile pointer.

The PG17-only DDL remains outside `backend/migrations` while the independent
Compose runtime is PostgreSQL 16. Migration 037 adds only the extension-free
legacy profile pointer. G18.5 must promote the already-reviewed PG17 DDL
only after restoring a verified backup into fresh PG17 storage; it must never
mount `mm-chat/data/postgres` directly into PG17.

## Change history

- 2026-07-22: initial G18.4 BM25/Dense/RRF/Golden/security/rollback proof.
