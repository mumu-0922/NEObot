# Live Release Boundary

## Observed authority

- Git worktree was clean before task creation.
- Product fix commit: `ae221181`.
- Live Backend image ID: `sha256:d661111c4511...` with retained tag
  `mm-chat/backend:memory-orphan-activity-20260828T234500Z`.
- Live Backend uses only `compose.single-server.yml`, is healthy, runs the API
  command, and shares its image with the independently running Memory Worker.
- PostgreSQL 17 live head is migration 109.
- PostgreSQL data is bound to the current default
  `mm-chat/data/postgres17`; MinIO data is under the active local runtime.

## Configuration finding

`preflight-single-server.sh` is a strict production-promotion gate. The active
source-build env predates several required promotion fields and stops first on
`POSTGRES_DATA_DIR`. The running container and a current source-build Compose
render nevertheless have identical Backend environment values except `PATH`.

The release must therefore not synthesize missing production settings or swap
to `compose.production.yml`. It should retain the current topology, snapshot
the env privately, and change only release identity plus Backend image.

## Recovery boundary

- Use one shared set ID for `backup-postgres.sh` and `backup-minio.sh`.
- Validate checksums before deployment.
- Restore only to a unique temporary database and temporary MinIO bucket.
- Compare the restored schema manifest to the live schema dynamically through
  head 109. The older runbook SQL literal covers only migrations 001..010 and
  is not current release authority.
- Sample object keys remain private transient files under `backup/restore/`;
  output records only counts.
- Always remove the temporary database, bucket, staging, and sample files.

## Deployment boundary

- Explicit Docker target: `runtime`.
- Candidate tag: `mm-chat/backend:skill-cleanup-ae221181`.
- Recreate Backend only with `--no-build --no-deps --force-recreate`.
- Do not recreate the Memory Worker merely because it references the same image
  setting; its running image remains an unrelated container for this fix.
- Roll back by restoring the protected env values and recreating Backend from
  the already-retained old tag. No database rollback is required.

