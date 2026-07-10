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

| Secret                         | Used by                    | Rotation impact                                                          |
| ------------------------------ | -------------------------- | ------------------------------------------------------------------------ |
| Account passwords              | Identity users, Postgres   | Rotate through Recovery; all sessions for that user are revoked.         |
| Recovery tokens                | Identity users, Postgres   | One-time, 30 minutes by default; completing Recovery consumes the token. |
| SMTP username/password         | Go backend Recovery mailer | Restart backend; verify delivery before revoking the old credential.     |
| Session bearer tokens          | Browser clients, Postgres  | Revoke through the authenticated session endpoints.                      |
| `PROVIDER_API_KEY`             | Go backend provider client | Restart backend; verify chat streaming.                                  |
| `POSTGRES_PASSWORD`            | Postgres role, backend     | Alter DB role, update `DATABASE_URL`, restart backend/migrate users.     |
| `REDIS_PASSWORD` / `REDIS_URL` | Redis, backend             | Restart Redis and backend; temporary cache/cancel/rate state may reset.  |
| `S3_SECRET_ACCESS_KEY`         | Backend MinIO app user     | Prefer create-new app user, restart backend, then disable old user.      |
| `MINIO_ROOT_PASSWORD`          | MinIO admin/bootstrap      | Maintenance restart; never use root credentials from the app.            |
| TLS private key/cert           | Reverse proxy              | Reload proxy; backend/data services are unaffected.                      |

## Account Password Recovery

There is no rotatable `AUTH_BOOTSTRAP_TOKEN`. The one-time
`admin bootstrap-identity` command provisions only the first Credential and
refuses to run once any Credential exists; it is not a password-reset path.

Request Recovery without revealing whether the mailbox exists:

```bash
curl -fsS -X POST http://127.0.0.1:8080/v1/auth/recovery/request \
  -H "Content-Type: application/json" \
  --data '{"email":"<user@example.com>"}'
# HTTP 202: {"status":"accepted"}
```

The one-time Recovery Token is delivered by email and expires after
`AUTH_RECOVERY_TTL` (`30m` by default). Do not put the token or new password in
command arguments, environment variables, shell history, logs, or tickets. One
stdin-only completion method is:

```bash
python3 - <<'PY' |
import getpass
import json

print(json.dumps({
    "token": getpass.getpass("Recovery token: "),
    "newPassword": getpass.getpass("New password: "),
}))
PY
curl -fsS -o /dev/null -w '%{http_code}\n' \
  -X POST http://127.0.0.1:8080/v1/auth/recovery/complete \
  -H "Content-Type: application/json" --data-binary @-
# 204
```

Completion atomically changes the password, increments its Credential revision,
consumes the token, revokes sibling Recovery Tokens, and revokes every Session
for that user. It does not issue a replacement Session; verify by logging in
again through the normal client with the new password. Passwords must be 15-256
UTF-8 characters/bytes and are not trimmed or normalized.

## SMTP Credentials

Rotate SMTP credentials independently of account passwords:

1. Obtain a new relay credential without revoking the old one.
2. Update `AUTH_SMTP_USERNAME` and `AUTH_SMTP_PASSWORD` together in
   `mm-chat/.env.single-server`. If the relay changes, also update
   `AUTH_SMTP_ADDR=<smtp-host:port>` and `AUTH_SMTP_FROM=<sender-mailbox>`.
3. Restart only the backend and verify readiness:
   ```bash
   docker compose --env-file mm-chat/.env.single-server \
     -f mm-chat/compose.single-server.yml --profile app up -d backend
   curl -fsS http://127.0.0.1:8080/ready
   ```
4. Request Recovery for a known operator test mailbox and confirm that the
   email arrives with a token and expiry. The API still returns the same `202`
   when delivery fails or the queue is full, so the HTTP response alone is not
   a delivery smoke.
5. Revoke the old SMTP credential only after delivery succeeds.

`AUTH_SMTP_ADDR` must be `host:port`; delivery requires `STARTTLS` with TLS 1.2
or newer. If SMTP auth is not required, leave both username and password empty.
A partial or invalid SMTP configuration prevents backend startup. Rollback by
restoring the old SMTP fields and restarting `backend`.

## Session Revocation

A signed-in user revokes all of their own sessions, including the current one,
through the authenticated Identity API:

```bash
curl -fsS -o /dev/null -w '%{http_code}\n' \
  -X DELETE http://127.0.0.1:8080/v1/me/sessions \
  -H "Authorization: Bearer <session-token>"
# 204
```

Use `POST /v1/auth/logout` when only the current Session should be revoked.
Postgres is authoritative on every Bearer request, and the API invalidates the
corresponding Redis hints; no Redis purge is required for these normal user
flows.

For an all-user security incident only, an operator may revoke every active
Session in a maintenance window with a database update:

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

This direct update bypasses API cache cleanup, but stale positive Redis entries
cannot authorize a request because Postgres is rechecked. Restarting Redis is
therefore not required for authorization correctness, though in-flight streams
may still need operational cancellation.

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
