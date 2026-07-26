# Retired PostgreSQL 16 data deletion result

## Outcome

- Deleted only `/home/mumu/projects/neo-chat/mm-chat/data/postgres/`.
- Reclaimed approximately 154 MiB.
- Removed 1,661 files across 27 directories.
- Preserved `data/postgres17/`, `data/minio/`, `data/redis/`, backups,
  secrets, and the active environment file.

## Pre-delete evidence

- No current or stopped Docker container mounted the retired path.
- The final PG16 database dump, owned dump, and roles checksums passed.
- Copied and reverified the logical rollback set at:
  `/home/mumu/projects/neo-chat-former-root-backup-20260726-144734/runtime/postgres16-retired-rollback-20260726`.

## Post-delete verification

- PostgreSQL `17.10` remained healthy.
- Migration head remained `50:jina_runtime_retirement`.
- Knowledge state remained readable: 14 collections and 82 documents.
- Frontend root, backend readiness, same-origin health, and private RAG health
  returned HTTP 200.
- `git status --porcelain -- mm-chat` remained empty.
