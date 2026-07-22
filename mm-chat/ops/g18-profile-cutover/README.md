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

This harness is not the production Compose cutover. Its reviewed SQL has now
been frozen as migration `038` and qualified on a verified live PG16 backup
restored into fresh PG17 storage. Migration `038` deliberately rejects PG16,
so the still-running PG16 Compose stack must not be rebuilt or rerun against
the new migration manifest before the separately approved blue-green switch.

The maintenance trigger covers publication into active and building
generations, while the active source view remains bound to the corpus head.
The representative resource gate, active-PG17 backup/restore gate, live-data
restore, and formal migration qualification are complete. Only a fresh final
stop-window backup plus blue-green Compose/data-path authority switch remain
under G18.5B.3b.
