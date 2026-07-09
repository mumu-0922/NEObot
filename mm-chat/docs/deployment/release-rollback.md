# Release and Rollback Runbook

This runbook applies to the single-server `mm-chat` Compose stack. It assumes
real secrets live in `mm-chat/.env.single-server` and runtime data lives in
`mm-chat/data/`.

## Pre-Release Gate

```bash
cd mm-chat
docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile app --profile ops config
docker compose --env-file .env.single-server -f compose.single-server.yml build backend
./scripts/backup-postgres.sh
./scripts/backup-minio.sh
```

Confirm the backup files and `.sha256` sidecars exist before touching schema or
restarting the API.

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
```

For a full single-server release, inspect `/ready` and require configured
dependency checks to be `ready`:

```bash
curl -fsS http://127.0.0.1:8080/ready | jq '.status, .checks'
```

## Rollback Decision Tree

- **API image bad, schema compatible**: checkout previous commit, rebuild, restart `backend`.
- **Migration bad before user traffic**: stop `backend`, run the migration image with `down`, then redeploy the previous image:
  ```bash
  docker compose --env-file .env.single-server \
    -f compose.single-server.yml --profile ops \
    run --rm migrate /usr/local/bin/mm-chat-migrate down
  ```
- **Migration bad after user traffic**: prefer forward fix. Down migration may destroy or orphan data.
- **Object storage issue**: stop upload/import paths, verify MinIO backup, restore into a temporary bucket first.
- **Redis issue**: flush or recreate Redis only; Postgres/MinIO remain authoritative.

## Post-Release Notes

Record release commit, migration output, backup filenames, smoke-test results,
and rollback decision in `mm-chat/docs/tracking/process.md`.
