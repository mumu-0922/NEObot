# PostgreSQL 17 retrieval image design

## Purpose

This image is the production G18 storage base for true BM25 plus pgvector. It
changes only the database runtime; migration `038`, the retrieval profile
pointer, and application hydration remain the separately reviewed schema and
authority boundaries.

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
- Production Compose binds only `mm-chat/data/postgres17` to this image,
  constrains it to 1 GiB / 2 CPUs, and preloads pg_textsearch with the qualified
  PostgreSQL settings. Production requires an immutable image digest and
  removes the local build path.
- PostgreSQL 16 rollback uses the unchanged `mm-chat/data/postgres` directory
  or restores the preserved logical backup into another fresh PostgreSQL 16
  database; it never attempts an in-place downgrade.
- The API runtime login is a non-superuser member of `go_api_runtime`, not the
  image bootstrap/migration owner, and has no direct projection access.

## Production state

G18.3, G18.4, and G18.5 are promoted through formal migration `038`. Production
runs PostgreSQL `17.10` on `data/postgres17` with
`pg17_bm25_pgvector_v1@2` active. The PG16 directory, final logical backups,
MinIO archive, and legacy `REAL[]` rows remain intact until a separate reviewed
observation-window cleanup authorizes their deletion.
