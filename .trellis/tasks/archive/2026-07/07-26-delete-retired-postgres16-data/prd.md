# Delete retired PostgreSQL 16 data

## Goal

Delete only `/home/mumu/projects/neo-chat/mm-chat/data/postgres/`, the retired
PostgreSQL 16 physical data directory, after proving that it is not mounted by
any current or stopped container and that the retained PG16 logical rollback
artifacts are checksummed. Preserve the active PostgreSQL 17 deployment and all
other runtime data.

## What I already know

- The owner explicitly requested deletion after confirming that PostgreSQL 17
  is the complete current authority.
- The running PostgreSQL container mounts only `mm-chat/data/postgres17/`.
- Production preflight requires `POSTGRES_DATA_DIR=./data/postgres17` and
  rejects the retired path.
- PostgreSQL 17 is at migration `50:jina_runtime_retirement`; its July 26 dump
  and paired MinIO archive passed temporary restore drills.
- The retired PG16 directory is approximately 154 MiB and is documented only
  as a physical rollback anchor.
- Logical PG16 dumps, roles, checksums, and cutover evidence remain under
  `mm-chat/backup/g18-pg17-cutover/` and
  `mm-chat/backup/g18-pg17-production-cutover/`.

## Requirements

- Recheck all Docker containers, including stopped containers, for mounts that
  reference `mm-chat/data/postgres/`.
- Verify the relevant PG16 dump and role-file checksums before deletion.
- Resolve and validate the target as the exact non-symlink directory beneath
  `mm-chat/data/`.
- Delete no path other than `mm-chat/data/postgres/`.
- Preserve `postgres17/`, `minio/`, `redis/`, all backups, secrets, and the live
  environment file.
- After deletion, verify that the active PostgreSQL container remains healthy,
  migration head remains 50, and frontend/backend/same-origin/RAG health checks
  pass.
- Confirm `mm-chat/` tracked source remains clean.

## Acceptance criteria

- [x] No current or stopped container mounts the retired PG16 path.
- [x] Retained PG16 logical rollback checksums pass.
- [x] `mm-chat/data/postgres/` no longer exists.
- [x] The three active runtime data directories still exist.
- [x] PostgreSQL 17 remains healthy at migration head 50.
- [x] Frontend, backend, same-origin proxy, and RAG health return HTTP 200.
- [x] No tracked source change is introduced.

## Definition of done

- Exact-path deletion is complete and verified.
- Active runtime and application health are unchanged.
- Rollback evidence remains available through logical backups.

## Out of scope

- Deleting or compacting PostgreSQL 17.
- Deleting MinIO, Redis, secrets, environment files, or backup artifacts.
- Changing Compose, source, documentation, or Git history.
