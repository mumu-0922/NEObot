# Release and Rollback Runbook

This runbook applies to the single-server `mm-chat` Compose stack. It assumes
real secrets live in `mm-chat/.env.single-server` and runtime data lives in
`mm-chat/data/`.

## Active Release Mode — Compose Source Build

The current owner-selected release path is source-build Compose deployment from
the standalone `mm-chat/` tree. Do not require GHCR, remote registry images, or
`@sha256:` digest env proof for the standalone cutover gate.

The Compose file already carries the required build contexts:

```text
backend     -> ./backend/Dockerfile
frontend    -> ./frontend/Dockerfile
rag-worker  -> ./rag/Dockerfile
```

Optional image publishing still exists through `scripts/release-images.sh`, but
it is a future hardening/promotion path, not the default deployment flow.

## Pre-Release Gate

Run from the standalone project root:

```bash
cd mm-chat
bash scripts/verify-standalone.sh
bash scripts/verify-standalone.sh --full

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  build backend frontend rag-worker
```

Then run migrations and start the app/RAG services from the freshly built local
images:

```bash
docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile ops run --rm migrate

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  up -d backend frontend rag-worker
```

Verify the runtime edge:

```bash
front_port="$(awk -F= '$1=="FRONTEND_PORT"{print $2}' .env.single-server)"
: "${front_port:=3000}"
curl -fsS "http://127.0.0.1:${front_port}/"
curl -fsS "http://127.0.0.1:${front_port}/mm-api/ready"
curl -fsS http://127.0.0.1:8080/ready
```

For the RAG worker, verify its container health or run an internal health probe:

```bash
docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile rag-worker exec -T rag-worker \
  python -c "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8081/health', timeout=3).read().decode())"
```

## Backup Gate

Before migrations or service replacement, create and verify backups. The lower
level backup scripts work with the normal build-based Compose file and do not
require registry image variables:

```bash
cd mm-chat
COMPOSE_FILE=compose.single-server.yml \
ENV_FILE=.env.single-server \
bash scripts/backup-postgres.sh

COMPOSE_FILE=compose.single-server.yml \
ENV_FILE=.env.single-server \
bash scripts/backup-minio.sh
```

Keep the generated `.sha256` sidecars with the backup files. Restore drills are
required before destructive former-root cleanup; follow
[`backup-restore.md`](./backup-restore.md) for the temporary Postgres and MinIO
restore procedure.

## Deploy

For an ordinary single-server update:

```bash
cd mm-chat
git fetch --all --tags
git checkout <release-commit-or-tag>

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  build backend frontend rag-worker

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile ops run --rm migrate

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  up -d backend frontend rag-worker
```

Then run the smoke checks from the pre-release gate.

## Rollback Decision Tree

- **App build bad, schema compatible**: checkout the previous known-good commit
  or tag, rebuild `backend frontend rag-worker`, and recreate only the affected
  services.
- **Latest migration bad before user traffic**: stop `backend`, run the
  migration command with `down` only for the explicitly approved version, then
  redeploy the previous known-good commit. One invocation rolls back only one
  version.
- **Migration bad after user traffic**: prefer a forward fix. Down migration may
  destroy or orphan data.
- **Knowledge migrations after live writes**: forward-fix only unless a reviewed
  rollback plan proves no active leases, bound jobs, authority rows, namespace
  conflicts, projection state, or tombstones will be lost.
- **Object storage issue**: stop upload/import paths, verify MinIO backup, and
  restore into a temporary bucket before touching the live bucket.
- **Redis issue**: flush or recreate Redis only; Postgres/MinIO remain
  authoritative.

## Optional Registry Image Promotion

If a future deployment wants registry-published immutable artifacts, use:

```bash
cd mm-chat
docker login ghcr.io
./scripts/release-images.sh \
  --push \
  --image-namespace ghcr.io/mumu-0922 \
  --tag <release-id>
```

The script writes `.release/images/<release-id>/production-images.env` with
`MM_CHAT_VERSION`, `BACKEND_IMAGE`, `FRONTEND_IMAGE`, and `RAG_IMAGE` digest
lines. This optional path may be paired with
`compose-single-server-production.sh` and `preflight-single-server.sh`, but it
is not required for the current build-based standalone cutover.

## Post-Release Notes

Record release commit, migration output, backup filenames, smoke-test results,
and rollback decision in `mm-chat/docs/tracking/standalone-parity-sliced-process.md`.
