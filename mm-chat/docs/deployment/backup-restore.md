# Backup and Restore Runbook

This runbook covers the Phase 10 single-server Docker Compose deployment for
`mm-chat`. Production operations use the clean-environment Compose/backup
wrappers, `compose.single-server.yml`, and `compose.production.yml`; secrets
come only from the validated mode-`0600` env file.

The backup scripts do not print secrets or archive env files. Production must
call `backup-single-server-production.sh`, which clears host Compose/env
overrides before invoking both lower-level backup scripts. Their override/fallback
surface below is for isolated CI and disposable drills only.

## Backup paths and overrides

Default paths are derived from the script location:

```text
mm-chat/scripts/backup-postgres.sh
mm-chat/scripts/backup-minio.sh
mm-chat/scripts/backup-single-server-production.sh
mm-chat/backup/postgres/
mm-chat/backup/minio/
```

Non-production CI/drill overrides:

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
Never use these override forms for production backup or restore.

## Create backups

Run Postgres and MinIO backups in the same maintenance window when possible so
file metadata in Postgres stays aligned with object bytes in MinIO.

```bash
cd /home/mumu/projects/neo-chat

./mm-chat/scripts/backup-single-server-production.sh \
  mm-chat/.env.single-server
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

mm-chat/scripts/compose-single-server-production.sh mm-chat/.env.single-server \
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
mm-chat/scripts/compose-single-server-production.sh mm-chat/.env.single-server \
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

mm-chat/scripts/compose-single-server-production.sh mm-chat/.env.single-server \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi

exec psql --set=ON_ERROR_STOP=1 \
  --username="$POSTGRES_USER" --dbname=neo_chat_restore_drill
' <<'SQL'
SELECT 1 AS database_readable;

DO $acceptance$
DECLARE
  object_count INTEGER;
  required_table TEXT;
BEGIN
  WITH expected(version, name, checksum) AS (
    VALUES
      (1::BIGINT, 'initial_schema',
       'baedb43eb3c4a3c586e6b4fb2c31232952f5751cd44c04538d1edfec809f2da8'),
      (2::BIGINT, 'messages_run_id_index',
       '898797afe75a718c9973ce96fcb1f5813c6bec096a2a1090552d05941fb4028d'),
      (3::BIGINT, 'import_batches',
       'e38b8489e1d8622d872616be173fa59550e1c9244104216362af53b618dd9aca'),
      (4::BIGINT, 'phase15_identity_knowledge_acl',
       'dca5618aea9955d41bdbe33c448c5f2a526fd01c518f4927c7232b405533fcec'),
      (5::BIGINT, 'phase15_team_services',
       'eb11fec51508c28e04c46de5999f1ea669efc78e6aa63716e87de74605a78a3c'),
      (6::BIGINT, 'phase15_knowledge_services',
       '18cbd6b05d10b0560093549b56dd4a10fa276c3b93e5737039f3129f0e07a307'),
      (7::BIGINT, 'phase15_knowledge_deletion',
       'ae0b251db4b10aaa378a48679044708cb13148162e24f5544c3e10f9b219fd2a'),
      (8::BIGINT, 'phase15_governance_immutability',
       '3e5579e3ca2556db9a2fdad7af9553ee972153fc921fbd51a6279e51d64edc36'),
      (9::BIGINT, 'phase15_consent_expiry_materialization',
       '288e94681c5ea0efb55a9c1489babc9f9d850b66196a9a15a8741a90c70b2774')
  ), drift AS (
    (SELECT version, name, checksum FROM expected
     EXCEPT
     SELECT version, name, checksum FROM schema_migrations)
    UNION ALL
    (SELECT version, name, checksum FROM schema_migrations
     EXCEPT
     SELECT version, name, checksum FROM expected)
  )
  SELECT count(*) INTO object_count FROM drift;
  IF object_count <> 0 THEN
    RAISE EXCEPTION
      'restore acceptance: migration manifest differs from release (% rows)',
      object_count;
  END IF;

  FOREACH required_table IN ARRAY ARRAY[
    'knowledge_collections',
    'knowledge_documents',
    'knowledge_document_versions',
    'user_query_consent_state',
    'processor_governance_profiles',
    'processor_governance_heads',
    'processing_consents',
    'knowledge_processing_jobs',
    'knowledge_outbox'
  ] LOOP
    IF to_regclass(format('%I.%I', current_schema(), required_table)) IS NULL THEN
      RAISE EXCEPTION 'restore acceptance: missing core table %', required_table;
    END IF;
  END LOOP;

  SELECT count(*) INTO object_count
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'processing_consents'
    AND column_name = 'expiry_materialized_at'
    AND data_type = 'timestamp with time zone';
  IF object_count <> 1 THEN
    RAISE EXCEPTION
      'restore acceptance: processing_consents.expiry_materialized_at missing';
  END IF;

  SELECT count(*) INTO object_count
  FROM pg_index index_state
  JOIN pg_class index_relation ON index_relation.oid = index_state.indexrelid
  JOIN pg_class table_relation ON table_relation.oid = index_state.indrelid
  JOIN pg_namespace namespace ON namespace.oid = table_relation.relnamespace
  WHERE namespace.nspname = current_schema()
    AND table_relation.relname = 'processing_consents'
    AND index_relation.relname = 'idx_processing_consents_expiry_due'
    AND index_state.indisvalid
    AND pg_get_indexdef(index_state.indexrelid) LIKE '%expires_at, id%'
    AND pg_get_expr(index_state.indpred, index_state.indrelid)
      LIKE '%superseded_at IS NULL%'
    AND pg_get_expr(index_state.indpred, index_state.indrelid) LIKE '%granted%'
    AND pg_get_expr(index_state.indpred, index_state.indrelid)
      LIKE '%expires_at IS NOT NULL%'
    AND pg_get_expr(index_state.indpred, index_state.indrelid)
      LIKE '%expiry_materialized_at IS NULL%';
  IF object_count <> 1 THEN
    RAISE EXCEPTION 'restore acceptance: Consent expiry index invalid';
  END IF;

  SELECT count(*) INTO object_count
  FROM pg_trigger trigger_state
  JOIN pg_class table_relation ON table_relation.oid = trigger_state.tgrelid
  JOIN pg_namespace namespace ON namespace.oid = table_relation.relnamespace
  JOIN pg_proc trigger_function ON trigger_function.oid = trigger_state.tgfoid
  WHERE namespace.nspname = current_schema()
    AND table_relation.relname = 'processor_governance_profiles'
    AND trigger_state.tgname = 'processor_governance_profiles_immutable'
    AND NOT trigger_state.tgisinternal
    AND trigger_state.tgenabled <> 'D'
    AND (trigger_state.tgtype::INTEGER & 1) = 1
    AND (trigger_state.tgtype::INTEGER & 2) = 2
    AND (trigger_state.tgtype::INTEGER & 8) = 8
    AND (trigger_state.tgtype::INTEGER & 16) = 16
    AND trigger_function.proname = 'reject_processor_governance_profile_mutation';
  IF object_count <> 1 THEN
    RAISE EXCEPTION 'restore acceptance: Governance immutability trigger invalid';
  END IF;

  SELECT count(*) INTO object_count
  FROM pg_index index_state
  JOIN pg_class index_relation ON index_relation.oid = index_state.indexrelid
  JOIN pg_class table_relation ON table_relation.oid = index_state.indrelid
  JOIN pg_namespace namespace ON namespace.oid = table_relation.relnamespace
  WHERE namespace.nspname = current_schema()
    AND table_relation.relname = 'knowledge_processing_jobs'
    AND index_relation.relname = 'idx_knowledge_processing_jobs_purge_fence'
    AND index_state.indisunique
    AND index_state.indisvalid
    AND pg_get_indexdef(index_state.indexrelid)
      LIKE '%document_id, document_version_id, document_visibility_epoch%'
    AND pg_get_expr(index_state.indpred, index_state.indrelid) LIKE '%stage%'
    AND pg_get_expr(index_state.indpred, index_state.indrelid) LIKE '%operation%'
    AND pg_get_expr(index_state.indpred, index_state.indrelid) LIKE '%purge%';
  IF object_count <> 1 THEN
    RAISE EXCEPTION 'restore acceptance: Knowledge purge fence invalid';
  END IF;

  SELECT count(*) INTO object_count
  FROM knowledge_document_versions document_version
  LEFT JOIN files file ON file.id = document_version.file_id
  WHERE file.id IS NULL
     OR document_version.content_hash <> file.sha256
     OR length(trim(file.object_key)) = 0;
  IF object_count <> 0 THEN
    RAISE EXCEPTION
      'restore acceptance: % Document Version/File metadata mismatches',
      object_count;
  END IF;
END
$acceptance$;

SELECT version, name
FROM schema_migrations
ORDER BY version;

SELECT 'knowledge_collections' AS table_name, count(*) AS row_count
FROM knowledge_collections
UNION ALL
SELECT 'knowledge_documents', count(*) FROM knowledge_documents
UNION ALL
SELECT 'knowledge_document_versions', count(*) FROM knowledge_document_versions
UNION ALL
SELECT 'user_query_consent_state', count(*) FROM user_query_consent_state
UNION ALL
SELECT 'processor_governance_profiles', count(*)
FROM processor_governance_profiles
UNION ALL
SELECT 'processor_governance_heads', count(*) FROM processor_governance_heads
UNION ALL
SELECT 'processing_consents', count(*) FROM processing_consents
UNION ALL
SELECT 'knowledge_processing_jobs', count(*) FROM knowledge_processing_jobs
UNION ALL
SELECT 'knowledge_outbox', count(*) FROM knowledge_outbox
ORDER BY table_name;
SQL

mkdir -p mm-chat/backup/restore
mm-chat/scripts/compose-single-server-production.sh mm-chat/.env.single-server \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi

exec psql --set=ON_ERROR_STOP=1 --tuples-only --no-align \
  --username="$POSTGRES_USER" --dbname=neo_chat_restore_drill \
  --command="SELECT file.object_key
    FROM knowledge_document_versions document_version
    JOIN files file ON file.id = document_version.file_id
    WHERE document_version.status NOT IN ('\''tombstoned'\'', '\''deleted'\'')
      AND file.upload_status = '\''available'\''
      AND file.deleted_at IS NULL
      AND file.storage_backend IN ('\''minio'\'', '\''s3'\'')
    ORDER BY document_version.created_at DESC, document_version.id
    LIMIT 5;"
' > mm-chat/backup/restore/knowledge-object-sample.txt

mm-chat/scripts/compose-single-server-production.sh mm-chat/.env.single-server \
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
3. Use the controlled `minio-restore` production-profile service to create a
   drill bucket; do not inject ad hoc env, volumes, or entrypoints.
4. Mirror the staged backup into the drill bucket.
5. List objects and, when available, verify restored `files.object_key` values
   sampled through Knowledge Document Versions in the Postgres drill with
   `mc stat`.
6. Remove the drill bucket and local staging directory.

```bash
cd /home/mumu/projects/neo-chat

(cd mm-chat/backup/minio && sha256sum -c <chosen-archive>.tar.gz.sha256)

rm -rf mm-chat/backup/restore/minio-drill
mkdir -p mm-chat/backup/restore/minio-drill
test -f mm-chat/backup/restore/knowledge-object-sample.txt
tar -xzf mm-chat/backup/minio/<chosen-archive>.tar.gz \
  -C mm-chat/backup/restore/minio-drill

mm-chat/scripts/compose-single-server-production.sh mm-chat/.env.single-server \
  --profile restore run --rm -T minio-restore

rm -rf mm-chat/backup/restore/minio-drill
rm -f mm-chat/backup/restore/knowledge-object-sample.txt
```

The Postgres acceptance block fails closed unless every migration `001` through
`009` exactly matches the release's version, name, and embedded-SQL checksum;
the Knowledge core tables are readable; migration `009`'s Consent expiry
column/index exists; migration `008`'s Governance immutability trigger is
enabled; and the purge fence is a valid unique index. It also rejects Document
Version/File hash or object-key mismatches and exports up to five live Knowledge
object keys. The MinIO drill must `mc stat` every exported key; an empty sample
is valid only when the restored database has no eligible live Knowledge
Document Version.

Use root/admin MinIO credentials for the temporary-bucket drill. The application
S3 credentials are intentionally scoped to the production bucket and may not be
allowed to create or remove drill buckets.

No committed wrapper performs destructive replacement of the real bucket. A
production MinIO restore requires a separately reviewed, one-time change that
pins the backup pair and destination, stops backend writes, takes a final fresh
Postgres plus MinIO backup, and reuses the same clean-environment boundary. Do
not repurpose `minio-restore`, which is deliberately limited to a temporary
bucket and always removes that drill bucket.

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
