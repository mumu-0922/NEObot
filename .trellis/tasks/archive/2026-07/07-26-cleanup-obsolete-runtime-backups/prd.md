# Cleanup obsolete runtime backups

## Goal

Remove superseded milestone backups under `mm-chat/backup/` while retaining the
latest standalone PostgreSQL recovery dump and the complete PG17 production
cutover recovery/audit package.

## What I already know

- The user approved the recommended cleanup after reviewing deletion impact.
- `mm-chat/backup/` is not mounted by current runtime containers.
- Current authoritative runtime data lives under `mm-chat/data/postgres17/`,
  `mm-chat/data/minio/`, `mm-chat/data/redis/`, and `mm-chat/secrets/`.
- The retained artifacts are small enough that aggressive cleanup is not needed.

## Requirements

- Retain `mm-chat/backup/g18-pg17-production-cutover/` in full.
- Retain the latest standalone PostgreSQL dump and checksum:
  `postgres/postgres-20260726T065352Z.dump*`.
- Remove the older standalone PostgreSQL dump and superseded milestone backups.
- Remove empty backup directories after confirming they contain no files.
- Do not touch current runtime data, current secrets, or unrelated dirty files.
- Verify retained checksum files and current service health after cleanup.

## Acceptance Criteria

- [x] Only `g18-pg17-production-cutover/` and the latest PostgreSQL dump pair
      remain under `mm-chat/backup/`.
- [x] The retained PostgreSQL dump passes SHA-256 verification.
- [x] Frontend, backend, PostgreSQL, MinIO, Redis, and RAG services remain healthy.
- [x] Deleted paths are reported with reclaimed space.

## Out of Scope

- Deleting or modifying live data under `mm-chat/data/`.
- Rotating credentials contained in retained production-cutover evidence.
- Creating a new full MinIO or PostgreSQL backup.

## Rollback

This cleanup intentionally removes obsolete local recovery points. The retained
production-cutover package and latest standalone database dump remain the local
rollback anchors.

## Verification Record

- Reversible external archive:
  `/home/mumu/projects/neo-chat-runtime-backup-prune-20260726T082633Z`
- Archive SHA-256:
  `bdcd604631d2992fc42c6ab1919d091651ba11209e8f8457ec2b503e8a0704db`
- PostgreSQL restore drill: 58 application tables restored successfully.
- MinIO extraction drill: 41 files, 20,063,183 bytes.
- Removed apparent bytes: 42,126,273.
- `bash mm-chat/scripts/verify-standalone.sh --full`: passed.
- Live frontend, same-origin backend, backend health/readiness, and MinIO health:
  HTTP 200; all health-enabled containers remained healthy.
