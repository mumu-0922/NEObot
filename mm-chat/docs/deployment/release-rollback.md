# Release and Rollback Runbook

This runbook applies to the single-server `mm-chat` Compose stack. It assumes
real secrets live in `mm-chat/.env.single-server` and runtime data lives in
`mm-chat/data/`.

Each production release must set `BACKEND_IMAGE` to a full immutable registry
digest (`registry/repository@sha256:<64 lowercase hex>`). Mutable tags are for
local development only and cannot pass promotion preflight. Keep the previous
container image ID and registry digest until post-release verification and the
rollback window are complete.

Production env files contain direct values only. Preflight rejects every `$`
to prevent Compose interpolation from changing a value after validation; use
URL-safe/base64 secrets and percent-encode URL credentials where required. The
production Compose wrapper clears host environment precedence and applies an
override that removes all backend/migrate/admin `build:` definitions. It also
rejects alternate Compose/env files and every explicit build request.

## Pre-Release Gate

```bash
cd mm-chat/backend
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
govulncheck ./...

cd ..
chmod 600 .env.single-server
./scripts/test-preflight-single-server.sh
./scripts/preflight-single-server.sh .env.single-server
MM_CHAT_TEST_DATABASE_URL="$DISPOSABLE_PG16_URL" \
  ./scripts/verify-phase15d-postgres.sh
./scripts/compose-single-server-production.sh .env.single-server \
  --profile app --profile ops config --quiet
./scripts/backup-single-server-production.sh .env.single-server
```

Do not build or overwrite a production artifact on the server. Pull the
configured digest instead:

```bash
./scripts/compose-single-server-production.sh .env.single-server \
  --profile app pull backend
```

Confirm the backup files and `.sha256` sidecars exist before touching schema or
restarting the API. The promotion record must also include the disposable
PostgreSQL 16 Knowledge/migration race suite and fresh/historical migration
replay results; a skipped integration suite is not a pass. The migration suite
pins the published `2010d73` `001-006` file hashes, materializes its legacy
checksum-less metadata, requires explicit baseline acceptance, and upgrades it
with the current `007-009` migrations.

`DISPOSABLE_PG16_URL` must target an isolated PostgreSQL 16 database whose
schemas may be created and dropped. Never point this verification script at the
live application database.

Before replacing an existing API container, capture its immutable image ID
without printing environment values:

```bash
backend_id="$(./scripts/compose-single-server-production.sh \
  .env.single-server --profile app ps -q backend)"
if [ -n "$backend_id" ]; then
  docker inspect --format \
    'imageId={{.Image}} imageRef={{.Config.Image}}' "$backend_id"
fi
```

Record that image ID and the container's `.Config.Image` registry reference in
the release record. Verify the configured digest is pullable before running
migrations. Never delete the previous image during the rollback window.

## Deploy

```bash
cd mm-chat
./scripts/compose-single-server-production.sh .env.single-server \
  up -d postgres redis minio minio-init
./scripts/compose-single-server-production.sh .env.single-server \
  --profile ops run --rm migrate
./scripts/compose-single-server-production.sh .env.single-server \
  --profile app up -d --no-build backend
```

Then verify:

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
curl -fsS http://127.0.0.1:8080/v1/version
curl -fsS -H "Authorization: Bearer $SMOKE_TOKEN" \
  http://127.0.0.1:8080/v1/me/knowledge/query-consents
```

If `up` fails because an already-applied legacy row has no checksum, stop the
release. Review the checked-out migration pairs and historical replay evidence,
then perform the one-time operator acceptance and rerun `up`:

```bash
./scripts/compose-single-server-production.sh .env.single-server \
  --profile ops run --rm migrate /usr/local/bin/mm-chat-migrate baseline
./scripts/compose-single-server-production.sh .env.single-server \
  --profile ops run --rm migrate
```

Never place `baseline` in an unattended routine deploy. It explicitly accepts
the checked-out Up/Down SQL for legacy rows; normal `up`/`down` must remain
fail-closed until that one-time review occurs.

For a full single-server release, inspect `/ready` and require configured
dependency checks to be `ready`:

```bash
curl -fsS http://127.0.0.1:8080/ready | jq '.status, .checks'
```

## Rollback Decision Tree

- **API image bad, schema compatible**: do not rebuild or use a shell-variable
  override. Copy the protected env file to `.env.rollback`, replace only
  `BACKEND_IMAGE` with the recorded previous registry digest using a secure
  editor, keep mode `0600`, then pull and recreate only `backend`:
  ```bash
  ./scripts/compose-single-server-production.sh .env.rollback \
    --profile app pull backend
  ./scripts/compose-single-server-production.sh .env.rollback \
    --profile app up -d --no-build --force-recreate backend
  ```
- **Latest migration bad before user traffic**: stop `backend`, run the
  migration image with `down` once for each explicitly approved version, then
  redeploy the matching previous image. One invocation rolls back only one
  version:
  ```bash
  ./scripts/compose-single-server-production.sh .env.rollback \
    --profile ops run --rm migrate /usr/local/bin/mm-chat-migrate down
  ```
- **Migration bad after user traffic**: prefer forward fix. Down migration may destroy or orphan data.
- **Knowledge migrations `006`-`009` after live writes**: forward-fix only.
  Never drop authoritative Documents, Consent history/materialization markers,
  Processing Jobs, or Outbox rows to roll back an API image.
- **Object storage issue**: stop upload/import paths, verify MinIO backup, restore into a temporary bucket first.
- **Redis issue**: flush or recreate Redis only; Postgres/MinIO remain authoritative.

## Post-Release Notes

Record release commit, migration output, backup filenames, smoke-test results,
and rollback decision in `mm-chat/docs/tracking/process.md`.
