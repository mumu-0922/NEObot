# Single-Server Docker Compose Deployment

This is the single-server runtime topology for `mm-chat`, including Phase 15.1B
Identity Services, Phase 15.1C Team Services, and Phase 15.1D Personal/Team
Collections, Documents, private content, Governance, and Processing Consent.
It keeps deployment files inside `mm-chat/` and does not modify the repository-root
`docker-compose.yml`. The stack runs the Go API, Postgres, Redis, and MinIO on
one server; the existing Next.js frontend remains outside this workspace until
a later frontend cutover.

## Files

```text
mm-chat/compose.single-server.yml      # backend + data services
mm-chat/.env.single-server.example     # committed template only
mm-chat/.env.single-server             # local secrets, gitignored
mm-chat/backend/Dockerfile             # Go API + migration + admin binaries
mm-chat/data/                          # runtime volumes, gitignored
mm-chat/backup/                        # backup output, gitignored
```

Copy the template before first use:

```bash
cd mm-chat
cp .env.single-server.example .env.single-server
# Edit every change-me value. Team key placeholders are intentionally unusable
# until replaced with independently generated keys.
```

Use `--env-file .env.single-server` for operator commands. The Compose file has
safe placeholders for config validation, but production runs must use the local
secret file.

## Services and Profiles

| Service        | Profile | Purpose                                                                          | Public exposure |
| -------------- | ------- | -------------------------------------------------------------------------------- | --------------- |
| `postgres`     | default | Canonical users, sessions, Teams, chat, file metadata, imports, and mail outbox. | None            |
| `redis`        | default | Non-authoritative temporary state: rate limit, session cache, cancellation.      | None            |
| `minio`        | default | Private object bytes for uploaded/imported files.                                | None            |
| `minio-init`   | default | Creates bucket and least-privilege app user/policy.                              | None            |
| `migrate`      | `ops`   | One-shot `mm-chat-migrate up`; never auto-runs on API boot.                      | None            |
| `admin`        | `ops`   | One-shot local identity administration; no HTTP listener.                        | None            |
| `backend`      | `app`   | Go API on `127.0.0.1:8080` for reverse proxy or local smoke tests.               | Localhost only  |
| `minio-client` | `ops`   | Utility container for backup/restore scripts.                                    | None            |

No database, Redis, or MinIO port is published. The backend binds to localhost
only so a host-level reverse proxy can expose the same-origin `/mm-api` path
without opening data services.

`minio-init` is intentionally fail-fast: it creates the bucket, applies the app
policy, attaches it to the app user, then verifies the app credentials can write,
stat, and delete a temporary object. If `S3_SECRET_ACCESS_KEY` is rotated,
rerun `minio-init` during a maintenance window and do not start `backend` until
that credential smoke passes.

The `admin` service shares the backend image and database settings, but its
entrypoint is `/usr/local/bin/mm-chat-admin`. It is an operator-only, one-shot
container under the `ops` profile; it is not a long-running administration API.

## First Boot

```bash
cd mm-chat

docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d postgres redis minio minio-init

docker compose --env-file .env.single-server \
  -f compose.single-server.yml build backend

docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile ops run --rm migrate

# Read the first Owner password without putting it in argv, an environment
# variable, or shell history. The command accepts exactly one stdin line.
read -r -s -p "Owner password: " OWNER_PASSWORD
printf "\n"
printf "%s\n" "$OWNER_PASSWORD" | docker compose \
  --env-file .env.single-server -f compose.single-server.yml \
  --profile ops run --rm -T admin bootstrap-identity \
  --email "<owner@example.com>" --display-name "Owner" --password-stdin
unset OWNER_PASSWORD

docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile app up -d backend
```

Run `bootstrap-identity` only after migrations on a fresh installation. It
creates the initial Email/Password Owner, uses
`AUTH_BOOTSTRAP_USER_ID`/`AUTH_BOOTSTRAP_DISPLAY_NAME` when the optional flags
are omitted, and refuses to run after any Credential exists. It is not a
password-reset or break-glass command. There is no `AUTH_BOOTSTRAP_TOKEN`; the
old token is neither configured by this Compose stack nor accepted by
`POST /v1/auth/login`. Passwords must be 15-256 UTF-8 characters/bytes.

The supported account-disable maintenance path uses the Team fencing
transaction rather than direct SQL. It locks the User first, then every active
Membership Team in UUID order, rejects a last-usable-admin disable, revokes
Sessions, advances affected Membership revisions, and writes Outbox events:

```bash
docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile ops run --rm admin \
  disable-account --user-id '<user-uuid>'
```

Do not replace this command with `UPDATE users SET account_status=...` because
that bypasses last-admin and revision fencing.

Apply Processor Governance from a reviewed, credential-free manifest on stdin.
Unknown fields—including keys, tokens, URLs, IDs, status, and revisions—are
rejected. Reapplying the exact active manifest is a no-op:

```bash
cat docs/deployment/governance-mineru.example.json | docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile ops run --rm -T admin \
  governance-apply --manifest-stdin

docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile ops run --rm admin \
  governance-disable --processor mineru --endpoint-id default
```

The manifest contains only bounded lowercase declaration identifiers for the
Processor, endpoint, and model/API version, plus allowlisted purposes and exact
MIME or global `*` data types. Policy declarations are deliberately closed to
the reviewed baseline `global` / `none` / `delete` / `disabled`; supporting new
provider terms requires a reviewed code change. Spaces, URLs, free-form policy
text, duplicate/case-variant keys, and unknown fields are rejected. Credentials remain in service secret
configuration and must never enter Governance JSON or SQL.

Smoke test:

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
curl -fsS http://127.0.0.1:8080/v1/version
curl -fsS http://127.0.0.1:8080/metrics | head
```

`/ready` is additive: a healthy single-server stack should return
`{"status":"ready"}` plus `checks` entries for configured dependencies such as
`database`, `redis`, `storage`, and `team_mail_worker` when Invite delivery is
fully configured. A dependency outage returns `503` with `status=not_ready`;
the response intentionally does not expose raw connection errors or secrets.

## Password Recovery and SMTP

Recovery delivery is server-only and is configured on `backend` through the
following Compose fields:

| Variable               | Default | Contract                                                                |
| ---------------------- | ------- | ----------------------------------------------------------------------- |
| `AUTH_RECOVERY_TTL`    | `30m`   | Lifetime of a one-time Recovery Token.                                  |
| `AUTH_SMTP_ADDR`       | empty   | SMTP `host:port`; blank connection/auth/sender fields disable delivery. |
| `AUTH_SMTP_USERNAME`   | empty   | Optional SMTP username; configure it together with the password.        |
| `AUTH_SMTP_PASSWORD`   | empty   | Optional SMTP password; keep it only in `.env.single-server`.           |
| `AUTH_SMTP_FROM`       | empty   | Required sender mailbox when SMTP is configured.                        |
| `AUTH_SMTP_QUEUE_SIZE` | `100`   | Bounded queue capacity; valid range is 1-10000.                         |
| `AUTH_SMTP_TIMEOUT`    | `10s`   | Positive connect and delivery deadline per message.                     |

Example values in `.env.single-server`:

```dotenv
AUTH_RECOVERY_TTL=30m
AUTH_SMTP_ADDR=smtp.example.com:587
AUTH_SMTP_USERNAME=<smtp-username>
AUTH_SMTP_PASSWORD=<smtp-password>
AUTH_SMTP_FROM=no-reply@example.com
AUTH_SMTP_QUEUE_SIZE=100
AUTH_SMTP_TIMEOUT=10s
```

If SMTP auth is not required, leave both `AUTH_SMTP_USERNAME` and
`AUTH_SMTP_PASSWORD` empty. When any SMTP field is configured, the complete
configuration must validate or the backend refuses to start. Delivery requires
SMTP `STARTTLS` with TLS 1.2 or newer; use a relay endpoint that supports it.

For a syntactically valid request, `POST /v1/auth/recovery/request` returns the
same response whether the account exists, SMTP is disabled/unavailable,
delivery fails, or the bounded queue is full:

```bash
curl -fsS -X POST http://127.0.0.1:8080/v1/auth/recovery/request \
  -H "Content-Type: application/json" \
  --data '{"email":"<user@example.com>"}'
# {"status":"accepted"} with HTTP 202
```

Only a known active identity gets a one-time token queued for delivery. The
email contains the token and its UTC expiry; no token is returned by the API.
If SMTP is disabled, requests are still accepted but no email can be delivered,
so do not expose the recovery UI until a known-mailbox delivery smoke passes.
Malformed payloads and rate limits keep their normal `400`/`429` responses.
Database unavailability remains a `503` and is not hidden as an accepted
request.

`POST /v1/auth/recovery/complete` consumes the token and returns `204`; it
changes the password, increments the Credential revision, revokes sibling
Recovery Tokens and every Session for that user, and does not issue a new
Session. The user must log in again with the new password.

## Team Services and Invite Delivery

`/v1/teams` and `/v1/teams/` are authenticated routes in both required and
development modes. Team CRUD and
membership authorization use Postgres; Invite creation additionally requires
the synchronous SMTP transport, Mail cipher, acceptance URL builder, and the
running durable Mail Outbox worker. When Mail Invite delivery is entirely
unconfigured, normal Team operations remain wired while Invite creation fails
closed with `503 INVITE_DELIVERY_UNAVAILABLE`; a partially configured delivery
stack fails startup instead. If Postgres itself is disabled, the routes stay
registered but database-backed Team operations return `503 DATABASE_REQUIRED`.

| Variable                          | Default                 | Contract                                                                                        |
| --------------------------------- | ----------------------- | ----------------------------------------------------------------------------------------------- |
| `TEAM_CURSOR_ACTIVE_KEY_ID`       | required in hosted mode | Active HMAC signing key ID.                                                                     |
| `TEAM_CURSOR_KEYRING`             | no usable default       | Comma-separated `key-id=base64`; each decoded key is at least 32 bytes.                         |
| `TEAM_MAIL_ACTIVE_KEY_ID`         | none                    | Active AES-256-GCM encryption key ID.                                                           |
| `TEAM_MAIL_KEYRING`               | no usable default       | Comma-separated `key-id=base64`; every decoded key is exactly 32 bytes.                         |
| `TEAM_INVITE_ACCEPT_URL_BASE`     | none                    | HTTPS UI URL in required mode; loopback HTTP only in development. The worker adds `#token=...`. |
| `TEAM_MAIL_WORKER_LEASE_DURATION` | `30s`                   | Positive durable claim lease.                                                                   |
| `TEAM_MAIL_WORKER_POLL_INTERVAL`  | `500ms`                 | Positive idle/error poll interval.                                                              |
| `TEAM_MAIL_WORKER_BACKOFF_BASE`   | `5s`                    | Positive retry-backoff floor.                                                                   |
| `TEAM_MAIL_WORKER_BACKOFF_MAX`    | `15m`                   | Retry cap, not less than the base.                                                              |

Generate independent production keys locally and place only the base64 output
in the uncommitted `.env.single-server` file:

```bash
openssl rand -base64 32 # cursor key; run once
openssl rand -base64 32 # mail key; run independently
```

The committed `change-me-base64-32-byte-random-key` text is intentionally
invalid key material, not a fixed public encryption key. Cursor and Mail key
bytes must differ from each other and from database/Redis passwords,
SMTP/provider credentials, and object-store secrets. Required mode refuses
missing Cursor keys and known committed example keys. Mail Invite
delivery may remain explicitly disabled only when both the Mail keyring and
acceptance URL are absent; once either is set, the complete Mail keyring, URL,
and SMTP transport must validate or startup fails. Malformed base64, wrong key
length, non-HTTPS hosted URL, invalid worker duration/backoff, or partial SMTP
configuration also prevents startup. Startup errors contain field names and
safe key IDs only, never key bytes.

The emailed raw Token exists only after `#token=`. URL fragments are not sent
to the frontend HTTP server, reverse proxy, access log, or backend metric. The
frontend acceptance page must clear the fragment before posting the Token in
the JSON body; do not rewrite it into a query parameter.

Rotation is add-before-switch. First append the new `key-id=base64` entry while
leaving the old active ID, deploy, then change the active ID and deploy again.
Retained Cursor keys verify old cursors only; retained Mail keys decrypt old
Outbox rows only. Remove an old Cursor key after the maximum cursor lifetime,
and remove an old Mail key only after all rows encrypted with it are terminal
and past retention. Never reuse one key in both keyrings.

The Team Mail worker starts and stops with the API process. Invite admission
remains closed until the worker enters its run loop. A worker exit is logged
through the secret-redacting logger, triggers API shutdown, cancels the worker
context, and is awaited before Postgres closes. Delivery is at-least-once; the
stable Message-ID limits duplicates after a crash.

## Metrics

The Go API exposes Prometheus text metrics at `GET /metrics`. The backend port
is bound to `127.0.0.1:8080`, so single-server Prometheus should scrape the
localhost endpoint or a reverse-proxy path protected by an allowlist.

```yaml
scrape_configs:
  - job_name: mm-chat-api
    static_configs:
      - targets: ["127.0.0.1:8080"]
    metrics_path: /metrics
```

Useful starting PromQL:

```promql
rate(mm_chat_http_requests_total[5m])
rate(mm_chat_http_requests_total{status=~"5.."}[5m])
histogram_quantile(0.95, rate(mm_chat_http_request_duration_seconds_bucket[5m]))
mm_chat_dependency_ready
mm_chat_postgres_open_connections
```

Metric labels use bounded route patterns such as
`/v1/files/{id}/content` and
`/v1/teams/{teamId}/invites/{inviteId}`; unknown paths collapse to
`/__unknown__`, and unknown HTTP methods collapse to `OTHER`. Raw UUIDs and
object keys must not appear in labels.
`mm_chat_dependency_ready{dependency="storage"}` represents the
configured file storage readiness. In this Compose deployment that storage
check is the MinIO/S3 bucket readiness check; it is not a direct MinIO admin
metrics scrape.

## Reverse Proxy Boundary

Terminate TLS outside this stack (Nginx, Caddy, Traefik, or a cloud load
balancer). Production firewall baseline: allow `80/tcp`, `443/tcp`, and trusted
admin SSH only. Proxy only same-origin API paths to localhost; the full edge
runbook is [`reverse-proxy-tls.md`](./reverse-proxy-tls.md).

```nginx
location = /mm-api/metrics {
  allow 127.0.0.1;
  deny all;
  rewrite ^/mm-api/(.*)$ /$1 break;
  proxy_pass http://127.0.0.1:8080;
}

location /mm-api/ {
  rewrite ^/mm-api/(.*)$ /$1 break;
  proxy_pass http://127.0.0.1:8080/;
  proxy_http_version 1.1;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-Proto $scheme;
  # Replace any client-supplied chain; the backend trusts this loopback proxy.
  proxy_set_header X-Forwarded-For $remote_addr;
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
6. Verify `/health`, `/ready` including configured dependency checks,
   `/v1/version`, protected Team and Knowledge routes, bounded metric labels,
   chat CRUD/streaming, upload, and browser import. With a disposable test
   account/token, require `GET /v1/me/knowledge/query-consents` to return `200`
   and a missing Bearer token to return `401`; do not mutate production Consent
   merely for smoke testing. Test Invite creation only after a known-mailbox
   SMTP delivery smoke.

```bash
curl -fsS -H "Authorization: Bearer $SMOKE_TOKEN" \
  http://127.0.0.1:8080/v1/me/knowledge/query-consents
curl -fsS http://127.0.0.1:8080/metrics | \
  grep '/v1/me/knowledge/query-consents'
```

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
