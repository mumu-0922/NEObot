# Release and Rollback Runbook

This runbook applies to the single-server `mm-chat` Compose stack. It assumes
real secrets live in `mm-chat/.env.single-server` and runtime data lives in
`mm-chat/data/`.

## Pre-Release Gate

```bash
cd mm-chat/backend
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
govulncheck ./...

cd ..
docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile app --profile ops config
docker compose --env-file .env.single-server -f compose.single-server.yml build backend
./scripts/backup-postgres.sh
./scripts/backup-minio.sh
```

Confirm the backup files and `.sha256` sidecars exist before touching schema or
restarting the API. The promotion record must also include the disposable
PostgreSQL 16 Knowledge/migration race suite and fresh/historical migration
replay results; a skipped integration suite is not a pass.

## Deploy

```bash
cd mm-chat
docker compose --env-file .env.single-server -f compose.single-server.yml up -d postgres redis minio minio-init
docker compose --env-file .env.single-server -f compose.single-server.yml --profile ops run --rm migrate
docker compose --env-file .env.single-server -f compose.single-server.yml --profile app up -d backend
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
docker compose --env-file .env.single-server -f compose.single-server.yml --profile ops \
  run --rm migrate /usr/local/bin/mm-chat-migrate baseline
docker compose --env-file .env.single-server -f compose.single-server.yml --profile ops run --rm migrate
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

- **API image bad, schema compatible**: checkout previous commit, rebuild, restart `backend`.
- **Latest migration bad before user traffic**: stop `backend`, run the
  migration image with `down` once for each explicitly approved version, then
  redeploy the matching previous image. One invocation rolls back only one
  version:
  ```bash
  docker compose --env-file .env.single-server \
    -f compose.single-server.yml --profile ops \
    run --rm migrate /usr/local/bin/mm-chat-migrate down
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
