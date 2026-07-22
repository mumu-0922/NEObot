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

## Resolved G18.2 versions

The disposable restore drill locked and verified the following build inputs:

- PostgreSQL `17.10-bookworm`, official base-image digest
  `sha256:4f736ae292687621d4dbe0d499ffd024a36bd2ee7d8ca6f2ccd4c800f047b394`;
- `pg_textsearch 1.3.1`, commit
  `578ff529894992fb9e67cae4c69424e65c84868e`, source archive SHA-256
  `8632f91231251dc3e19395ef6a0d4d158d5f5920ba420691471771418e2a7cc7`;
- pgvector `0.8.5`, commit
  `159b79aaad5983fb7459c1e3df2897fbb2d11788`, source archive SHA-256
  `9a483fad70ae2e0a50b3dccb6c4b4931d9a07375a1d5815e82b57870448a7d52`.

The pgvector build disables CPU-native compiler flags so the resulting runtime
does not silently depend on the Docker builder host's instruction set. The
image entrypoint rejects an existing non-17 `PG_VERSION` before invoking the
official entrypoint.

## Shadow DDL staging boundary

The independent single-server Compose runtime remains PostgreSQL 16 until the
reviewed blue-green cutover. Therefore the G18.3 `vector(1024)` DDL must not be
embedded into the ordinary `1–36` backend migration set yet: PostgreSQL 16's
current official image has no pgvector type, so doing so would break routine
`migrate up` before the database transition is authorized.

G18.3 applies the reviewed shadow DDL only to a disposable PostgreSQL 17 clone,
proves backfill/exact/HNSW/deletion/rollback, and leaves `schema_migrations`
unchanged. G18.5 owns promotion of that DDL into the normal migration sequence
after a verified production backup has been restored into fresh PG17 storage.

## Resolved G18.4 query behavior

Live `pg_textsearch 1.3.1` probes established that `<@>` returns a negative
BM25 score, smaller values rank higher, and an unrelated row receives `0`.
Shadow reads therefore require `score < 0` and retain the raw negative score in
diagnostics rather than interpreting it as a probability. The explicit
`to_bm25query(query, index_name)` form produced a real BM25 index scan.

The `simple` configuration recalls full Chinese terms but does not segment
arbitrary Chinese substrings. The selected shadow profile appends at most 512
overlapping CJK ideograph bigrams while leaving Latin text to the normal
tokenizer. A discarded prototype generated bigrams for all compacted text and
caused unrelated English queries to match common two-letter fragments. Exact
terms remain a separate deterministic ordering signal inside the lexical lane.

`pg_textsearch` can apply collection/authority filtering after Top-K selection.
G18.4 uses bounded 8x overfetch followed by current-authority reauthorization;
G18.5 must validate selectivity, oversampling, latency, and memory against a
representative single-server corpus before production cutover.

## Sources inspected

- `https://github.com/timescale/pg_textsearch` README and PostgreSQL License
- `https://github.com/pgvector/pgvector` README
- `https://github.com/paradedb/paradedb` AGPL-3.0 license
- `mm-chat/docs/architecture/phase-15-2-single-server-python-rag-consumer-indexing-plan.md`
- `mm-chat/docs/architecture/phase-15-2c-generation-bound-indexing-plan.md`
- `mm-chat/backend/migrations/012_rag_search_projection.up.sql`
- `mm-chat/backend/migrations/026_rag_cjk_bigram_normalization.up.sql`
- `mm-chat/backend/migrations/027_rag_hybrid_dense_candidates.up.sql`
