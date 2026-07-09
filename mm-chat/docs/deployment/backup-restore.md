# Backup and Restore Runbook

This runbook covers the Phase 10 single-server Docker Compose deployment for
`mm-chat`. It assumes the Compose file is `mm-chat/compose.single-server.yml`
with `postgres`, `minio`, and `minio-client` services, and that secrets are
injected into containers by Compose from the deployment environment.

The backup scripts do not print secrets or archive env files. When
`mm-chat/.env.single-server` exists, they pass it to Docker Compose so new
utility containers receive the same environment as the running stack; otherwise
they fall back to the committed example for dry-run validation.

## Backup paths and overrides

Default paths are derived from the script location:

```text
mm-chat/scripts/backup-postgres.sh
mm-chat/scripts/backup-minio.sh
mm-chat/backup/postgres/
mm-chat/backup/minio/
```

Supported overrides:

```bash
PROJECT_NAME=mm-chat \
COMPOSE_FILE=/absolute/path/compose.single-server.yml \
ENV_FILE=/absolute/path/.env.single-server \
BACKUP_DIR=/mnt/mm-chat-backup \
./mm-chat/scripts/backup-postgres.sh

PROJECT_NAME=mm-chat \
COMPOSE_FILE=/absolute/path/compose.single-server.yml \
ENV_FILE=/absolute/path/.env.single-server \
BACKUP_DIR=/mnt/mm-chat-backup \
./mm-chat/scripts/backup-minio.sh
```

`COMPOSE_FILE` may contain the normal Docker Compose `:`-separated file list.
`PROJECT_NAME` or `COMPOSE_PROJECT_NAME` targets a non-default Compose project,
which is useful for restore drills and CI smoke tests. Relative override paths
are resolved from the caller's current directory.

`PROJECT_NAME` changes Compose names and networks only. It does not isolate the
bind mounts in `mm-chat/compose.single-server.yml`; those still point at
`mm-chat/data/*`. For live-stack drills, restore into a temporary database and
temporary bucket, or run a disposable copy from a separate project directory.

## Create backups

Run Postgres and MinIO backups in the same maintenance window when possible so
file metadata in Postgres stays aligned with object bytes in MinIO.

```bash
cd /home/mumu/projects/neo-chat

./mm-chat/scripts/backup-postgres.sh
./mm-chat/scripts/backup-minio.sh
```

Outputs:

```text
mm-chat/backup/postgres/postgres-<UTC>.dump
mm-chat/backup/postgres/postgres-<UTC>.dump.sha256
mm-chat/backup/minio/minio-<UTC>.tar.gz
mm-chat/backup/minio/minio-<UTC>.tar.gz.sha256
```

The Postgres dump uses `pg_dump --format=custom --no-owner --no-acl` from the
`postgres` service. The MinIO backup runs `mc mirror` from the `minio-client`
Compose service, mirrors `S3_BUCKET`, then archives the mirrored tree as
`tar.gz`. The MinIO backup container runs as the invoking host UID/GID so the
operator can remove temporary staging files after the archive is created.

## Verify backup checksums

Always verify before copying off-host and again after copying back for a restore
or drill.

```bash
(cd mm-chat/backup/postgres && sha256sum -c <chosen-dump>.dump.sha256)
(cd mm-chat/backup/minio && sha256sum -c <chosen-archive>.tar.gz.sha256)
```

Keep the `.sha256` file beside the artifact it signs. If the path inside the
checksum file no longer matches after moving artifacts, run verification from
the original parent directory or regenerate a new checksum only after a trusted
copy has been verified.

## Postgres restore drill

Restores are destructive when pointed at the production database. Before any
production restore, rehearse against a temporary database in the same Postgres
container or on a disposable server.

1. Verify the dump checksum.
2. Create a temporary drill database.
3. Restore the dump into the temporary database.
4. Run table and readiness checks.
5. Drop the temporary database when the drill is complete.

```bash
cd /home/mumu/projects/neo-chat

(cd mm-chat/backup/postgres && sha256sum -c <chosen-dump>.dump.sha256)

docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi

export PGHOST="${PGHOST:-127.0.0.1}"
export PGPORT="${PGPORT:-5432}"

dropdb --username="$POSTGRES_USER" --if-exists neo_chat_restore_drill
createdb --username="$POSTGRES_USER" neo_chat_restore_drill
'

cat mm-chat/backup/postgres/<chosen-dump>.dump | \
docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi

export PGHOST="${PGHOST:-127.0.0.1}"
export PGPORT="${PGPORT:-5432}"

pg_restore \
  --clean \
  --if-exists \
  --no-owner \
  --no-acl \
  --username="$POSTGRES_USER" \
  --dbname=neo_chat_restore_drill
'

docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi

psql --username="$POSTGRES_USER" --dbname=neo_chat_restore_drill \
  --command="select 1;"
psql --username="$POSTGRES_USER" --dbname=neo_chat_restore_drill \
  --command="select version, name from schema_migrations order by version;"
psql --username="$POSTGRES_USER" --dbname=neo_chat_restore_drill \
  --command="select count(*) from conversations;"
psql --username="$POSTGRES_USER" --dbname=neo_chat_restore_drill \
  --command="select count(*) from messages;"
psql --username="$POSTGRES_USER" --dbname=neo_chat_restore_drill \
  --command="select count(*) from files;"
'

docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi

export PGHOST="${PGHOST:-127.0.0.1}"
export PGPORT="${PGPORT:-5432}"

dropdb --username="$POSTGRES_USER" --if-exists neo_chat_restore_drill
'
```

For a real production restore, stop backend writes first, take a fresh final
backup, confirm the rollback point, then restore into a freshly created target
DB. Dropping or cleaning the production DB will destroy current production data.
Do not run that path until the temporary-database drill succeeds.

## MinIO restore drill

MinIO restores are destructive when mirrored back to the production bucket with
`--remove`. Rehearse into a temporary bucket first.

1. Verify the archive checksum.
2. Extract the archive to a local restore staging directory.
3. Use the `minio-client` Compose service to create a drill bucket.
4. Mirror the staged backup into the drill bucket.
5. List objects and, when available, verify restored `files.object_key` values
   from the Postgres drill with `mc stat`.
6. Remove the drill bucket and local staging directory.

```bash
cd /home/mumu/projects/neo-chat

(cd mm-chat/backup/minio && sha256sum -c <chosen-archive>.tar.gz.sha256)

rm -rf mm-chat/backup/restore/minio-drill
mkdir -p mm-chat/backup/restore/minio-drill
tar -xzf mm-chat/backup/minio/<chosen-archive>.tar.gz \
  -C mm-chat/backup/restore/minio-drill

docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  --profile ops run --rm -T \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  --entrypoint /bin/sh \
  -v "$PWD/mm-chat/backup/restore/minio-drill:/restore-source:ro" \
  minio-client -euc '
: "${S3_BUCKET:?S3_BUCKET is required}"
: "${MINIO_ROOT_USER:?MINIO_ROOT_USER is required}"
: "${MINIO_ROOT_PASSWORD:?MINIO_ROOT_PASSWORD is required}"

alias_name="rootdrill"
endpoint="${S3_ENDPOINT:-${MINIO_ENDPOINT:-http://minio:9000}}"
drill_bucket="${S3_BUCKET}-restore-drill-$(date +%Y%m%d%H%M%S)"

mc alias set "$alias_name" "$endpoint" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
mc mb --ignore-existing "${alias_name}/${drill_bucket}" >/dev/null
mc mirror --overwrite --remove "/restore-source/${S3_BUCKET}" "${alias_name}/${drill_bucket}" >/dev/null

mc find "${alias_name}/${drill_bucket}" > /tmp/mm-chat-restore-objects.txt
object_count=0
while IFS= read -r object_path; do
  if [ -n "$object_path" ]; then
    object_count=$((object_count + 1))
  fi
done < /tmp/mm-chat-restore-objects.txt
echo "restored_object_count=${object_count}"

mc rb --force "${alias_name}/${drill_bucket}" >/dev/null
echo "cleanup=drill_bucket_removed"
'

rm -rf mm-chat/backup/restore/minio-drill
```

Use root/admin MinIO credentials for the temporary-bucket drill. The application
S3 credentials are intentionally scoped to the production bucket and may not be
allowed to create or remove drill buckets.

Only after the drill passes should an operator consider restoring into the real
bucket. The production form replaces the destination bucket contents with the
backup contents:

```bash
mc mirror --overwrite --remove \
  "/restore-source/${S3_BUCKET}" \
  "${alias_name}/${S3_BUCKET}"
```

Run that production form only in a maintenance window, after stopping backend
writes and taking a final fresh Postgres plus MinIO backup pair.

## Frequency and retention

Minimum schedule for a single-server deployment:

- Daily Postgres logical dump and MinIO mirror/archive, preferably in the same
  low-write window.
- Pre-deploy Postgres and MinIO backup before every schema, storage, or image
  rollout.
- Monthly restore drill for both Postgres and MinIO.
- Immediate ad-hoc backup before any destructive maintenance.

Retention baseline:

```text
daily backups: keep 14 days
weekly backups: keep 8 weeks if storage allows
monthly drill references: keep 12 months
pre-deploy backups: keep through the full rollback window
```

Store at least one encrypted copy off-host. Back up deployment secrets and the
release identifier separately; these scripts intentionally do not archive env
files, private keys, provider tokens, or Docker images.

## Operational risks

- Postgres metadata and MinIO object bytes must be restored as a pair. A dump
  from one time and a bucket archive from another can produce missing-file or
  orphan-object states.
- `mc mirror --remove` deletes destination objects that are not in the source.
  This is correct for exact restore, but catastrophic if pointed at the wrong
  bucket.
- Redis is non-authoritative for this deployment plan and is not covered by the
  canonical backup set.
- The scripts validate checksums only after artifact creation; operators must
  verify checksums after transfer and before restore.
- Production restore must be tested first in a temporary database/bucket or a
  disposable server.
