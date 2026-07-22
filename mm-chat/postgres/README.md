# mm-chat PostgreSQL 17 retrieval image

This directory builds the local PostgreSQL runtime required by G18 retrieval:

- PostgreSQL `17.10`, based on the digest-pinned official Bookworm image;
- `pg_textsearch 1.3.1` for true BM25;
- `pgvector 0.8.5` for `vector(1024)` and evaluated ANN indexes.

Both extension source archives are pinned by upstream commit and SHA-256. The
runtime image contains their PostgreSQL-licensed binaries and license files,
but not the compiler toolchain.

## Build

The G18.2 drill builds this local image before starting its isolated Compose
project:

```bash
mm-chat/scripts/run-g18-postgres17-restore-drill.sh
```

For an isolated manual build:

```bash
docker build \
  -t mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5 \
  mm-chat/postgres
```

The image preloads `pg_textsearch` and creates both extensions in each fresh
`POSTGRES_DB` initialized by the official entrypoint.

## Major-version guard

The wrapper entrypoint reads `${PGDATA}/PG_VERSION` before PostgreSQL starts.
Only major `17` is accepted. A PostgreSQL 16 directory exits with status `78`
and an explicit logical-backup/restore instruction.

Never mount `mm-chat/data/postgres` into this image. PostgreSQL major versions
do not share a physical data-directory format. Use a verified logical backup,
a fresh PostgreSQL 17 volume, and the documented rollback backup instead.

See [`DESIGN.md`](./DESIGN.md) for provenance and trust boundaries.
