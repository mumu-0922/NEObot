# Secret Rotation Runbook

This runbook covers single-server `mm-chat` secrets in
`mm-chat/.env.single-server`. Do not commit that file, command output with
secrets, or copied provider keys.

## Rotation Rules

- Take a Postgres and MinIO backup before rotating data-plane credentials.
- Rotate one secret class at a time.
- Update `.env.single-server`, restart only the affected services, then verify
  `/ready` and the affected user flow.
- Keep the old value until verification passes; revoke or remove it only after
  rollback is no longer needed.
- Record only secret names, timestamps, and verification results in docs.

## Secret Inventory

| Secret                         | Used by                    | Rotation impact                                                         |
| ------------------------------ | -------------------------- | ----------------------------------------------------------------------- |
| `AUTH_BOOTSTRAP_TOKEN`         | Go backend login           | New logins use the new bootstrap token; existing sessions remain valid. |
| Session bearer tokens          | Browser clients, Postgres  | Revoke by logout or DB session revocation; Redis cache may need purge.  |
| `PROVIDER_API_KEY`             | Go backend provider client | Restart backend; verify chat streaming.                                 |
| `POSTGRES_PASSWORD`            | Postgres role, backend     | Alter DB role, update `DATABASE_URL`, restart backend/migrate users.    |
| `REDIS_PASSWORD` / `REDIS_URL` | Redis, backend             | Restart Redis and backend; temporary cache/cancel/rate state may reset. |
| `S3_SECRET_ACCESS_KEY`         | Backend MinIO app user     | Prefer create-new app user, restart backend, then disable old user.     |
| `MINIO_ROOT_PASSWORD`          | MinIO admin/bootstrap      | Maintenance restart; never use root credentials from the app.           |
| TLS private key/cert           | Reverse proxy              | Reload proxy; backend/data services are unaffected.                     |

## Auth Bootstrap Token

Changing `AUTH_BOOTSTRAP_TOKEN` only changes the login bootstrap secret. It does
not revoke already-issued bearer sessions.

```bash
# edit mm-chat/.env.single-server: AUTH_BOOTSTRAP_TOKEN=<new-bootstrap-token>
docker compose --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml --profile app up -d backend

curl -fsS http://127.0.0.1:8080/ready
```

To revoke all active sessions during an incident, run a maintenance-window DB
update and clear session cache keys or restart Redis after confirming Redis holds
only non-authoritative temporary state:

```bash
docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_DB:?POSTGRES_DB is required}"
psql --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" \
  --command="update sessions set revoked_at = coalesce(revoked_at, now()), updated_at = now() where revoked_at is null;"
'
```

## Provider API Key

```bash
# edit PROVIDER_API_KEY in mm-chat/.env.single-server
docker compose --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml --profile app up -d backend

curl -fsS http://127.0.0.1:8080/ready
# Run one authenticated chat stream smoke through the frontend or API client.
```

Rollback: restore the old provider key in `.env.single-server` and restart
`backend`.

## Postgres Password

For an existing Postgres volume, changing `POSTGRES_PASSWORD` in the env file is
not enough. Rotate the database role first, then update the backend connection
string.

```bash
docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_DB:?POSTGRES_DB is required}"
psql --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" \
  --command="alter role \"$POSTGRES_USER\" with password '\''<new-postgres-password>'\'';"
'
```

Then update both `POSTGRES_PASSWORD` and `DATABASE_URL` in
`.env.single-server`, restart the backend, and verify readiness:

```bash
docker compose --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml --profile app up -d backend
curl -fsS http://127.0.0.1:8080/ready
```

Rollback: alter the role back to the previous password, restore the previous
`DATABASE_URL`, then restart `backend`.

## Redis Password

Update both `REDIS_PASSWORD` and `REDIS_URL` so the Redis service and backend
stay aligned.

```bash
# edit REDIS_PASSWORD and REDIS_URL in mm-chat/.env.single-server
docker compose --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml up -d redis

docker compose --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml --profile app up -d backend

curl -fsS http://127.0.0.1:8080/ready
```

Redis stores only non-authoritative temporary state. After rotation, users may
need to retry an in-flight stream, and rate-limit counters may reset.

## MinIO App Credentials

Prefer adding a new app user instead of changing the existing app user's secret
in place.

```bash
docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server \
  -f mm-chat/compose.single-server.yml \
  --profile ops run --rm -T --entrypoint /bin/sh minio-client -euc '
: "${MINIO_ROOT_USER:?MINIO_ROOT_USER is required}"
: "${MINIO_ROOT_PASSWORD:?MINIO_ROOT_PASSWORD is required}"
: "${S3_BUCKET:?S3_BUCKET is required}"
mc alias set root http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
mc admin user add root "<new-s3-access-key-id>" "<new-s3-secret-access-key>"
mc admin policy attach root mm-chat-files --user "<new-s3-access-key-id>"
'
```

Update `S3_ACCESS_KEY_ID` and `S3_SECRET_ACCESS_KEY` in `.env.single-server`,
restart the backend, upload/download a small file, then disable or remove the
old app user with root credentials after rollback is no longer needed.

## MinIO Root Credentials

Rotate root credentials in a maintenance window. Update `MINIO_ROOT_USER` and
`MINIO_ROOT_PASSWORD`, restart `minio`, rerun `minio-init`, then verify bucket
and app-user access. Do not put root credentials in backend env variables.

## TLS Certificates

Use the reverse proxy's normal certificate renewal path. After renewal or
rotation:

```bash
curl -I https://chat.example.com/
curl -fsS https://chat.example.com/mm-api/ready
```

Rollback is proxy-local: restore the previous certificate/key files and reload
the reverse proxy. Do not restart Postgres, Redis, or MinIO for TLS-only work.
