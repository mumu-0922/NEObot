# Single-Server Docker Compose Deployment

This is the Phase 10 runtime topology for `mm-chat`. It keeps deployment files
inside `mm-chat/` and does not modify the repository-root `docker-compose.yml`.
The stack runs the Go API, Postgres, Redis, and MinIO on one server; the
existing Next.js frontend remains outside this workspace until a later frontend
cutover.

## Files

```text
mm-chat/compose.single-server.yml      # backend + data services
mm-chat/.env.single-server.example     # committed template only
mm-chat/.env.single-server             # local secrets, gitignored
mm-chat/backend/Dockerfile             # Go API + migration binaries
mm-chat/data/                          # runtime volumes, gitignored
mm-chat/backup/                        # backup output, gitignored
```

Copy the template before first use:

```bash
cd mm-chat
cp .env.single-server.example .env.single-server
# Edit every change-me value before production.
```

Use `--env-file .env.single-server` for operator commands. The Compose file has
safe placeholders for config validation, but production runs must use the local
secret file.

## Services and Profiles

| Service | Profile | Purpose | Public exposure |
| --- | --- | --- | --- |
| `postgres` | default | Canonical data store for users, sessions, chat, files metadata, imports. | None |
| `redis` | default | Non-authoritative temporary state: rate limit, session cache, cancellation. | None |
| `minio` | default | Private object bytes for uploaded/imported files. | None |
| `minio-init` | default | Creates bucket and least-privilege app user/policy. | None |
| `migrate` | `ops` | One-shot `mm-chat-migrate up`; never auto-runs on API boot. | None |
| `backend` | `app` | Go API on `127.0.0.1:8080` for reverse proxy or local smoke tests. | Localhost only |
| `minio-client` | `ops` | Utility container for backup/restore scripts. | None |

No database, Redis, or MinIO port is published. The backend binds to localhost
only so a host-level reverse proxy can expose `/api` without opening data
services.

`minio-init` is intentionally fail-fast: it creates the bucket, applies the app
policy, attaches it to the app user, then verifies the app credentials can write,
stat, and delete a temporary object. If `S3_SECRET_ACCESS_KEY` is rotated,
rerun `minio-init` during a maintenance window and do not start `backend` until
that credential smoke passes.

## First Boot

```bash
cd mm-chat

docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d postgres redis minio minio-init

docker compose --env-file .env.single-server \
  -f compose.single-server.yml build backend

docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile ops run --rm migrate

docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile app up -d backend
```

Smoke test:

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
curl -fsS http://127.0.0.1:8080/v1/version
```

## Reverse Proxy Boundary

Terminate TLS outside this stack (Nginx, Caddy, Traefik, or a cloud load
balancer). Production firewall baseline: allow `80/tcp`, `443/tcp`, and trusted
admin SSH only. Proxy only the backend API path to localhost:

```nginx
location /api/ {
  proxy_pass http://127.0.0.1:8080/;
  proxy_http_version 1.1;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-Proto $scheme;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_buffering off; # required for SSE chat streaming
}
```

Do not proxy MinIO API or console publicly. If admin access is needed, use SSH
tunnel/VPN to the Docker network or host.

## Release Checklist

1. Pull the target Git commit and inspect `git diff --stat HEAD~1..HEAD -- mm-chat`.
2. Update `MM_CHAT_VERSION` in `.env.single-server` to the release tag/commit.
3. Build the backend image:
   ```bash
   docker compose --env-file .env.single-server -f compose.single-server.yml build backend
   ```
4. Run migrations explicitly:
   ```bash
   docker compose --env-file .env.single-server -f compose.single-server.yml --profile ops run --rm migrate
   ```
5. Restart the API:
   ```bash
   docker compose --env-file .env.single-server -f compose.single-server.yml --profile app up -d backend
   ```
6. Verify `/health`, `/ready`, `/v1/version`, chat CRUD, streaming, upload, and browser import smoke paths.

## Rollback Checklist

- Code rollback: checkout the previous commit, rebuild `backend`, then restart
  only the `backend` service.
- Schema rollback: run the migration image only when the release notes say the
  down migration is safe for current data:
  ```bash
  docker compose --env-file .env.single-server \
    -f compose.single-server.yml --profile ops \
    run --rm migrate /usr/local/bin/mm-chat-migrate down
  ```
- Data rollback: restore Postgres/MinIO from a verified backup in a disposable
  drill first; production restore is destructive.
- Frontend rollback: switch the existing frontend back to local mode until the
  server API is healthy.

## Verification

Static validation:

```bash
docker compose --env-file .env.single-server.example \
  -f compose.single-server.yml --profile app --profile ops config
```

Runtime validation should start with infra, run `migrate`, then start `backend`;
do not rely on API startup to apply schema changes.
