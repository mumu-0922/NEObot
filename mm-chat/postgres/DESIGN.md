# PostgreSQL 17 retrieval image design

## Purpose

This image is the G18 storage base for true BM25 plus pgvector. It changes only
the database runtime. Search projection DDL, shadow reads, and production
cutover remain separate reviewed groups.

## Reproducibility contract

- PostgreSQL:
  `17.10-bookworm@sha256:4f736ae292687621d4dbe0d499ffd024a36bd2ee7d8ca6f2ccd4c800f047b394`.
- `pg_textsearch 1.3.1`: commit
  `578ff529894992fb9e67cae4c69424e65c84868e`, archive SHA-256
  `8632f91231251dc3e19395ef6a0d4d158d5f5920ba420691471771418e2a7cc7`.
- pgvector `0.8.5`: commit
  `159b79aaad5983fb7459c1e3df2897fbb2d11788`, archive SHA-256
  `9a483fad70ae2e0a50b3dccb6c4b4931d9a07375a1d5815e82b57870448a7d52`.

The multi-stage build compiles against PostgreSQL 17 headers and copies only
installed extension artifacts and license texts into the final digest-pinned
official image. pgvector CPU-native flags are disabled so the artifact does not
silently depend on the builder host's instruction set. Extension versions are
rechecked by init SQL at runtime.

## Safety boundaries

- The image rejects an existing non-17 `PG_VERSION` before invoking the
  official entrypoint.
- G18.2 uses only project-scoped disposable volumes and synthetic rows.
- No host port, production env file, provider credential, user object, or
  `mm-chat/data/postgres` path is available to the restore drill.
- `POSTGRES_HOST_AUTH_METHOD=trust` is limited to the isolated internal drill
  network. It is not a production default.
- PostgreSQL 16 rollback restores the preserved logical backup into another
  fresh PostgreSQL 16 database; it never attempts an in-place downgrade.

## Deferred work

G18.3 adds the generation-bound `vector(1024)` shadow projection. G18.4 adds
the BM25 lane. G18.5 owns any production data migration, profile cutover,
observation window, and rollback decision.
