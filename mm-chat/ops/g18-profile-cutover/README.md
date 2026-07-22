# G18.5B PG17 profile cutover qualification

This disposable harness connects migration `037`'s durable profile pointer to
the reviewed G18.3 pgvector and G18.4 BM25 shadow projections. It is the frozen
operational provenance for formal PG17-only migration `038`; it first proved
the activation/rollback state machine before that migration was applied to the
isolated restored-live-data PG17 target.

## Run

```bash
mm-chat/scripts/run-g18-profile-cutover-drill.sh
```

The drill uses only synthetic data and an internal Docker network. It proves:

- activation is rejected while pgvector is ready but BM25 is not yet
  completely backfilled;
- the ready profile returns reference-only BM25/pgvector results through the
  existing Go repository function signature;
- runtime roles can read candidates but cannot mutate the pointer or execute
  private diagnostics;
- the active pointer and reader survive a PostgreSQL restart;
- two concurrently published document heads transactionally populate both
  physical projections and become query-visible without a maintenance gap;
- building-generation heads populate isolated projection rows without becoming
  active-reader authority; an incomplete corpus-head switch is rejected
  atomically, while a complete generation can be promoted and rolled back;
- a 4096-chunk corpus stays inside the reviewed backfill, query-latency,
  projection-size, 1 GiB memory, and 2 CPU deployment envelope;
- the active PG17 database survives restart, logical backup, checksum,
  restore into a fresh database, migration replay, reader, row, and role
  verification;
- deletion immediately removes authority visibility while immutable rollback
  rows remain, and materialization sync replay is idempotent;
- rollback is rejected while PG17 is active, then succeeds after an
  operator-controlled compare-and-swap to `legacy`;
- migration `037` and the legacy `REAL[]` reader remain after all candidate
  PG17 layers are removed.

Reports remain under `/tmp/mm-chat-g18-profile-cutover.*`. The exit trap removes
all project-scoped containers, networks, and volumes.

## Boundary

This harness remains disposable and must never be pointed at production. Its
reviewed SQL is frozen as migration `038`. The separately approved production
blue-green switch is complete: Compose now owns PostgreSQL `17.10` on
`data/postgres17`, with pgvector `0.8.5`, pg_textsearch `1.3.1`, and
`pg17_bm25_pgvector_v1@2` active. The unchanged `data/postgres` PG16 directory
and final checksummed backups remain rollback authorities.

The maintenance trigger covers publication into active and building
generations, while the active source view remains bound to the corpus head.
The representative resource gate, active-PG17 backup/restore gate, live-data
restore, formal migration qualification, final stop-window backup, production
data-path switch, application health, runtime-role, MinIO object, migration
replay, and restart/reconnect proofs are complete under G18.5B.3b.
