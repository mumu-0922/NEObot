# Storage engine selection

## Existing runtime facts

- `mm-chat/compose.single-server.yml` runs `postgres:16-alpine` and bind-mounts
  `./data/postgres`.
- Migration `012` deliberately created an extension-independent search
  projection with `REAL[]` embeddings.
- Migration `026` provides PostgreSQL FTS, exact-term, phrase, and CJK-bigram
  ranking with `ts_rank`; project architecture explicitly states that this is a
  baseline and must not be called BM25.
- Migration `027` computes Dense cosine similarity by expanding the stored and
  query `REAL[]` values, then fuses lexical and Dense ranks with RRF.

## Candidates

### `pg_textsearch` + pgvector on PostgreSQL 17

- `pg_textsearch` provides BM25 ranking, configurable `k1`/`b`, BM25 indexes,
  Block-Max WAND top-k execution, partial indexes, and PostgreSQL text search
  configurations.
- Current upstream documentation supports PostgreSQL 17 and 18 and requires
  `shared_preload_libraries = 'pg_textsearch'`.
- License: PostgreSQL License.
- pgvector provides `vector`, `halfvec`, exact search, HNSW/IVFFlat, cosine
  distance, and PostgreSQL transactional semantics.
- Consequence: current PG16 data must be dump/restored or upgraded through a
  separately verified major-version process; its data directory cannot be
  mounted directly into PG17.

### ParadeDB / `pg_search` + pgvector

- Provides a mature Tantivy-backed BM25 implementation and packaged database
  image.
- Upstream repository license is AGPL-3.0, which changes distribution/network
  source obligations and must not be introduced silently.
- Existing mm-chat architecture also records crash recovery, restore,
  tokenizer, memory, upgrade, and rollback as promotion gates.

### Keep PostgreSQL FTS + pgvector

- Lowest migration risk and can stay on PostgreSQL 16.
- Does not satisfy the approved requirement for true BM25.
- Remains the rollback/baseline path, not the target.

## Recommendation

Use a pinned custom PostgreSQL 17 image containing reviewed versions of
`pg_textsearch` and pgvector. Prove backup/restore and extension readiness on a
disposable clone before creating any production shadow projection. Preserve
the old PG16 backup and do not mutate it in place.

## Sources inspected

- `https://github.com/timescale/pg_textsearch` README and PostgreSQL License
- `https://github.com/pgvector/pgvector` README
- `https://github.com/paradedb/paradedb` AGPL-3.0 license
- `mm-chat/docs/architecture/phase-15-2-single-server-python-rag-consumer-indexing-plan.md`
- `mm-chat/docs/architecture/phase-15-2c-generation-bound-indexing-plan.md`
- `mm-chat/backend/migrations/012_rag_search_projection.up.sql`
- `mm-chat/backend/migrations/026_rag_cjk_bigram_normalization.up.sql`
- `mm-chat/backend/migrations/027_rag_hybrid_dense_candidates.up.sql`
