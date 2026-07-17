# Single-Server Docker Compose Deployment

This is the single-server runtime topology for `mm-chat`, including Phase 15.1B
Identity Services, Phase 15.1C Team Services, Phase 15.1D Personal/Team
Collections, and the Phase 15.2B default-dark-run RAG worker wiring.
It keeps deployment files inside `mm-chat/` and does not modify the repository-root
`docker-compose.yml`. The stack runs the Go API, Postgres, Redis, and MinIO on
one server. The complete Next.js frontend is built from `mm-chat/frontend/`
and exposes the only browser-facing application port.

## Files

```text
mm-chat/compose.yml                    # canonical local Compose entrypoint
mm-chat/compose.single-server.yml      # frontend + backend + data services
mm-chat/.env.single-server.example     # committed template only
mm-chat/.env.single-server             # local secrets, gitignored
mm-chat/backend/Dockerfile             # Go API + migration + admin binaries
mm-chat/frontend/Dockerfile            # standalone Next.js server image
mm-chat/rag/Dockerfile                 # Python dark-run worker image
mm-chat/scripts/preflight-single-server.sh # production promotion gate
mm-chat/scripts/compose-single-server-production.sh # clean-env production entrypoint
mm-chat/compose.production.yml          # removes all production build paths
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

| Service        | Profile      | Purpose                                                                          | Public exposure |
| -------------- | ------------ | -------------------------------------------------------------------------------- | --------------- |
| `postgres`     | default      | Canonical users, sessions, Teams, chat, file metadata, imports, and mail outbox. | None            |
| `redis`        | default      | Non-authoritative temporary state: rate limit, session cache, cancellation.      | None            |
| `minio`        | default      | Private object bytes for uploaded/imported files.                                | None            |
| `minio-init`   | default      | Creates bucket and least-privilege app user/policy.                              | None            |
| `migrate`      | `ops`        | One-shot `mm-chat-migrate up`; never auto-runs on API boot.                      | None            |
| `admin`        | `ops`        | One-shot local identity administration; no HTTP listener.                        | None            |
| `frontend`     | `app`        | Next.js UI and same-origin `/mm-api` edge on `127.0.0.1:3000`.                   | Localhost only  |
| `backend`      | `app`        | Go API on `127.0.0.1:8080` for reverse proxy or local smoke tests.               | Localhost only  |
| `minio-client` | `ops`        | Utility container for backup/restore scripts.                                    | None            |
| `rag-worker`   | `rag-worker` | Phase 15.2B durable-consumer mechanics; dispatch defaults off.                   | None            |
| `rag-replay`   | `rag-ops`    | One-shot, fail-closed Outbox/Job Replay CLI.                                     | None            |

No database, Redis, or MinIO port is published. The backend binds to localhost
only so a host-level reverse proxy can expose the same-origin `/mm-api` path
without opening data services.

### Phase 15.2B dark-run worker

`rag-worker` has no host port and joins `private` for provider egress plus the
internal `rag-private` network for database/Redis/backend traffic. Postgres,
Redis, and the backend also join `rag-private` so the worker can reach the
token-gated Go source-object gateway at
`RAG_SOURCE_GATEWAY_URL=http://backend:8080` without exposing it on the host.
Only a healthy Postgres service is a hard `depends_on` condition. Redis remains
a non-authoritative wake hint: an outage may degrade wake latency but must not
block startup or replace the Postgres poll and forced rescan.

The container is fenced to one CPU, 448 MiB RAM, 64 PIDs, a read-only root
filesystem, a 64 MiB `/tmp` tmpfs, `cap_drop: ALL`, and
`no-new-privileges`. Its environment contains only the Worker Postgres URL,
Redis wake settings, bounded Worker controls, and the G7 RAG provider secrets
when those are configured. It receives no MinIO, S3, chat Provider, or Replay
credential. The healthcheck calls container-local `GET /health` on port `8081`;
no port is published or proxied.

| Variable                  | Boundary                                                                  |
| ------------------------- | ------------------------------------------------------------------------- |
| `RAG_IMAGE`               | Python image; production requires a full registry `@sha256:` digest.      |
| `MIGRATION_DATABASE_URL`  | Required bootstrap/migrator URL; never falls back to the API URL.         |
| `DATABASE_URL`            | Non-superuser API login; shared only by `backend` and `admin`.            |
| `RAG_WORKER_DATABASE_URL` | Worker login inheriting only `rag_worker_executor`.                       |
| `RAG_REPLAY_DATABASE_URL` | Replay login inheriting only `rag_replay_operator`.                       |
| `RAG_MINERU_API_TOKEN`    | Admin-owned MinerU secret for G7 automatic parsing; redacted status only. |
| `RAG_MINERU_RESULT_PROXY_URL` | Optional internal ZIP download proxy for Docker Desktop/WSL CDN TLS workarounds; default empty. |
| `RAG_JINA_API_KEY`        | Admin-owned Jina secret for G7 embedding/rerank; redacted status only.    |

`POSTGRES_USER` is the empty-volume bootstrap and migrator login referenced by
`MIGRATION_DATABASE_URL`. The API login inherits only `go_api_runtime` and must
be neither superuser nor `CREATEROLE`. All four routes use pairwise-distinct
login names and passwords. The `migrate` service receives only
`MIGRATION_DATABASE_URL`; `admin` deliberately uses the API `DATABASE_URL`, not
the migrator credential. Production preflight checks separation and syntax
without printing URL values.

Phase 15.2B remains a dark-run boundary:

```text
RAG_WORKER_DISPATCH_ENABLED=false
RAG_WORKER_JOB_STAGES=
RAG_MINERU_API_TOKEN=
RAG_MINERU_RESULT_PROXY_URL=
RAG_JINA_API_KEY=
RAG_PROVIDER_PROFILE=disabled
RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED=false
RAG_PROVIDER_RETRY_MAX_ATTEMPTS=3
RAG_PROVIDER_INITIAL_RETRY_SECONDS=30
RAG_PROVIDER_MAX_RETRY_SECONDS=300
RAG_PROVIDER_CONCURRENCY=2
RAG_MINERU_REQUESTS_PER_MINUTE=60
RAG_JINA_REQUESTS_PER_MINUTE=240
RAG_JINA_EMBEDDING_MODEL=jina-embeddings-v4
RAG_JINA_RERANK_MODEL=jina-reranker-v3
```

At this gate the Worker may validate DB functions, its singleton lock, health,
and metrics mechanics, but it must not claim real Parse, Embedding, Purge, or
projection work. Healthy `/health` proves process/event-loop liveness only; it
does not prove projection readiness or that Search/RAG is available.

G7.2 adds the protected Go diagnostic `GET /v1/rag/provider-status`. It reports
only whether the server-owned MinerU/Jina credentials are configured and the
locked Jina embedding dimension (`1024`); it never returns key material. Python
worker dispatch fails closed when `parse` is enabled without
`RAG_MINERU_API_TOKEN` or `passage_embedding` is enabled without
`RAG_JINA_API_KEY`. Legacy `DEFAULT_MINERU_API_TOKEN` and
`DEFAULT_JINA_API_KEY` aliases remain accepted only as migration fallback names.
`RAG_MINERU_RESULT_PROXY_URL` is optional and normally empty. It exists only for
bounded local smoke environments where Docker Desktop/WSL container egress can
reach MinerU API/upload endpoints but fails TLS handshakes to the MinerU result
CDN. When set, the worker still validates the provider `full_zip_url` as
`https://cdn-mineru.openxlab.org.cn/pdf/*.zip` before sending it to the trusted
internal proxy; no MinerU token, Authorization header, cookie, or provider
secret is forwarded to that proxy.

G7.3 adds an explicit provider-backed runtime profile gate. Keep
`RAG_PROVIDER_PROFILE=disabled` until the operator intentionally enables
`mineru_jina_postgres_v1`. Provider-backed `parse` or `passage_embedding`
dispatch requires `RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED=true`, recording
that the owner accepts the still-draft MinerU/Jina public wire fixture risk for
this sandbox deployment. The profile is config-only in G7.3: it fixes retry
max attempts at `3`, defaults provider concurrency to `2`, keeps MinerU/Jina
rate ceilings broad (`60`/`240` requests per minute), and does not add any
network provider handler by itself.

Replay runs as a separate one-shot service so its DSN never enters the
long-running Worker. Dry-run validates an exact intent without touching the DB:

```bash
./scripts/compose-single-server-production.sh .env.single-server \
  --profile rag-ops run --rm rag-replay outbox \
  --id '<failed-event-uuid>' \
  --expected-error-code '<stable-error-code>'
```

Execution additionally requires `--execute`, an operator UUID, and a non-empty
reason; Job Replay also requires a fresh successor UUID. Never put a DSN on the
command line or use Compose `-e` overrides. The production wrapper rejects env
overrides, and `rag-replay` receives only `RAG_REPLAY_DATABASE_URL`.

`minio-init` is intentionally fail-fast: it creates the bucket, applies the app
policy, attaches it to the app user, then verifies the app credentials can write,
stat, and delete a temporary object. If `S3_SECRET_ACCESS_KEY` is rotated,
rerun `minio-init` during a maintenance window and do not start `backend` until
that credential smoke passes.

The `admin` service shares the backend image and API runtime `DATABASE_URL`, but
its entrypoint is `/usr/local/bin/mm-chat-admin`. It never receives
`MIGRATION_DATABASE_URL` or the bootstrap/migrator credential. It is an
operator-only, one-shot container under the `ops` profile; it is not a
long-running administration API.

`backend`, `migrate`, and `admin` all resolve the same `BACKEND_IMAGE`; this is
the release fence that keeps API code, migration SQL, and operator commands on
one build. Production must set a full registry `@sha256:` digest; mutable tags
are local-development only and cannot pass promotion preflight. Every
production Compose command goes through
`scripts/compose-single-server-production.sh`, which clears host-variable
precedence and applies `compose.production.yml` to remove all three `build:`
paths. Retain the previous backend image ID and registry digest through the
rollback window.

`rag-worker` and `rag-replay` resolve the separate `RAG_IMAGE`. The production
overlay removes both `build:` paths, and preflight rejects a mutable RAG image
reference. Retain its previous digest independently from the backend rollback
artifact.

The production overlay uses Compose `!reset`; require Docker Compose `2.24.4`
or newer. An older parser must fail the release rather than falling back to the
base file with `build:` enabled.

## Local Development First Boot

```bash
cd mm-chat

docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d postgres redis minio minio-init

docker compose --env-file .env.single-server \
  -f compose.single-server.yml build backend

docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile ops run --rm migrate

# Existing schema-009 deployments only: mount the reviewed Governance Mapping
# and pass --phase15-governance-map exactly as documented in
# postgres-single-server.md. A normal no-argument migrate intentionally fails
# closed when published Governance rows are not covered.

# Fresh install only: stop here. Migration 010 has created the NOLOGIN
# capability roles; securely create and grant the API, Worker, and Replay
# LOGIN principals as documented in postgres-single-server.md. Do not start
# admin, backend, or rag-worker until live role verification passes.

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

# Optional Phase 15.2B dark-run only; keep dispatch disabled and stages empty.
docker compose --env-file .env.single-server \
  -f compose.single-server.yml --profile rag-worker up -d rag-worker
```

The `build backend` step is for a local first boot only. For production, publish
the image elsewhere, set its full registry digest as `BACKEND_IMAGE`, and pull
that exact artifact through the Production Release Checklist; never use this
local-development sequence for a production release. The exact secure role
creation and live verification commands are in
[`postgres-single-server.md`](./postgres-single-server.md#fresh-install-role-provisioning).

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
./scripts/compose-single-server-production.sh .env.single-server \
  --profile ops run --rm admin \
  disable-account --user-id '<user-uuid>'
```

Do not replace this command with `UPDATE users SET account_status=...` because
that bypasses last-admin and revision fencing.

Do not apply Processor Governance from the old example manifest. Phase 15.2C
found that syntactically valid `default/model-v1/v1` placeholders could become
an `approved` profile. The replacement
`governance-mineru.blocked.json` deliberately does not match the Governance CLI
shape and must be rejected.

Only a credential-free manifest derived from a `lifecycle.state=frozen`
[`provider-wire-fixture.md`](../contracts/provider-wire-fixture.md) contract may
be reviewed and supplied on stdin for normal production. For the bounded G7 live
smoke only, `governance-apply` may proceed when the operator explicitly sets
`RAG_PROVIDER_PROFILE=mineru_jina_postgres_v1` and
`RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED=true`; this records acceptance of the
still-draft MinerU/Jina wire risk without exposing provider secrets to the admin
container. Without that exact pair, the apply command is intentionally
unavailable. After C0 closes, record the generated manifest path and exact
Contract/Terms/Fixture hashes in the release evidence; never pipe the blocked
file:

```text
governance apply: BLOCKED — PROVIDER_WIRE_CONTRACT_NOT_FROZEN unless the G7
draft profile acceptance gate is set
```

The manifest contains only bounded lowercase declaration identifiers for the
Processor, endpoint, model, and model/API version, plus allowlisted purposes and
exact MIME or global `*` data types. Governance is keyed by the exact
`processor + endpointId + modelId` identity. Policy declarations are
deliberately closed to the reviewed baseline `global` / `none` / `delete` /
`disabled`; supporting new provider terms requires a reviewed code change.
Spaces, URLs, free-form policy text, duplicate/case-variant keys, and unknown
fields are rejected. Credentials remain in service secret configuration and
must never enter Governance JSON or SQL.

Always pass the exact frozen `--model-id` to `governance-disable` so only the reviewed model Head
is disabled, and include `modelId` in every new Governance manifest. A legacy
apply manifest without `modelId`, or a legacy disable command without
`--model-id`, resolves only when the given Processor and endpoint already
identify exactly one model. Zero or multiple matches fail closed instead of
creating or choosing a model implicitly.

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

`/v1/teams` and `/v1/teams/` require a Session only in `AUTH_MODE=required`.
`AUTH_MODE=development` is the independent single-user mode: every non-public
request uses the fixed Development Owner and ignores browser Bearer headers.
The standalone frontend does not expose Team UI, but the backend compatibility
routes remain available. Team CRUD and membership authorization use Postgres;
Invite creation additionally requires
the synchronous SMTP transport, Mail cipher, acceptance URL builder, and the
running durable Mail Outbox worker. When Mail Invite delivery is entirely
unconfigured, normal Team operations remain wired while Invite creation fails
closed with `503 INVITE_DELIVERY_UNAVAILABLE`; a partially configured delivery
stack fails startup instead. If Postgres itself is disabled, the routes stay
registered but database-backed Team operations return `503 DATABASE_REQUIRED`.

| Variable                          | Default           | Contract                                                                                           |
| --------------------------------- | ----------------- | -------------------------------------------------------------------------------------------------- |
| `TEAM_CURSOR_ACTIVE_KEY_ID`       | `local-dev`       | Active HMAC signing key ID; replace before production.                                             |
| `TEAM_CURSOR_KEYRING`             | local-dev sample  | Comma-separated `key-id=base64`; each decoded key is at least 32 bytes; replace before production. |
| `TEAM_MAIL_ACTIVE_KEY_ID`         | none              | Active AES-256-GCM encryption key ID.                                                              |
| `TEAM_MAIL_KEYRING`               | no usable default | Comma-separated `key-id=base64`; every decoded key is exactly 32 bytes.                            |
| `TEAM_INVITE_ACCEPT_URL_BASE`     | none              | HTTPS UI URL in required mode; loopback HTTP only in development. The worker adds `#token=...`.    |
| `TEAM_MAIL_WORKER_LEASE_DURATION` | `30s`             | Positive durable claim lease.                                                                      |
| `TEAM_MAIL_WORKER_POLL_INTERVAL`  | `500ms`           | Positive idle/error poll interval.                                                                 |
| `TEAM_MAIL_WORKER_BACKOFF_BASE`   | `5s`              | Positive retry-backoff floor.                                                                      |
| `TEAM_MAIL_WORKER_BACKOFF_MAX`    | `15m`             | Retry cap, not less than the base.                                                                 |

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

## Consent Expiry Worker

The Consent expiry worker is part of the Go API process and starts whenever
`DATABASE_URL` enables the Postgres-backed runtime. The current Compose path
always enables Postgres, so every `backend` container runs this worker. It uses
the runtime defaults of 100 Consent rows per batch and a `1s` idle poll; these
values are not Compose environment settings.

The worker fails closed. A processing/database error emits the redacted
`consent_expiry_worker_failed` log event, reports a runtime failure, cancels the
shared runtime context, gracefully shuts down the API, and exits non-zero.
Compose then applies `restart: unless-stopped`. Route that exact log event and
backend restart/unavailability signals to the production alert channel; do not
treat the restart loop as recovery without investigating the underlying
Postgres or Consent row failure.

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
2. Set `MM_CHAT_VERSION` plus full registry `@sha256:` values for
   `BACKEND_IMAGE` and `RAG_IMAGE`; retain each currently running digest as an
   independent rollback artifact. Keep RAG dispatch disabled.
3. Run the production promotion gate before any migration or restart:
   ```bash
   ./scripts/preflight-single-server.sh .env.single-server
   ./scripts/compose-single-server-production.sh .env.single-server \
     --profile app --profile ops --profile rag-worker --profile rag-ops \
     config --quiet
   ```
4. Pull the exact production digest without allowing a build:
   ```bash
   ./scripts/compose-single-server-production.sh .env.single-server \
     --profile app pull backend
   ```
5. Run migrations explicitly from that same `BACKEND_IMAGE`. The service must
   receive the separately required `MIGRATION_DATABASE_URL`; missing it is a
   hard failure and must not fall back to `DATABASE_URL`. For a published
   schema `009` with Governance rows, replace this fresh-database form with the
   reviewed read-only Mapping mount documented in
   [`postgres-single-server.md`](./postgres-single-server.md#migration-execution):
   ```bash
   ./scripts/compose-single-server-production.sh .env.single-server \
     --profile ops run --rm migrate
   ```
6. On a fresh install, create the three runtime LOGIN principals only after
   migration `010` has created the NOLOGIN capability roles. On every release,
   run the live attribute, membership, and four-login connection checks in
   [`postgres-single-server.md`](./postgres-single-server.md#live-role-verification).
   Do not promote if the migrator has API capability, the API login has
   owner/migrator membership, or any route shares a login/password.
7. Restart the API. Both `backend` and future one-shot `admin` commands use the
   least-privilege API `DATABASE_URL`:
   ```bash
   ./scripts/compose-single-server-production.sh .env.single-server \
     --profile app up -d --no-build backend
   ```
8. Start or recreate the Phase 15.2B Worker only in dark-run mode:
   ```bash
   ./scripts/compose-single-server-production.sh .env.single-server \
     --profile rag-worker pull rag-worker
   ./scripts/compose-single-server-production.sh .env.single-server \
     --profile rag-worker up -d --no-build rag-worker
   ./scripts/compose-single-server-production.sh .env.single-server \
     --profile rag-worker ps rag-worker
   ```
   Require the Compose health state to become healthy. Do not publish port
   `8081`, and do not treat projection readiness as a Phase B promotion gate.
9. Verify Go `/health`, `/ready` including configured dependency checks,
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

- Code rollback: use the retained previous image ID or registry digest; never
  rebuild. Put the prior digest in a secured rollback env file and recreate only
  `backend` with `--no-build --force-recreate`, as shown in
  [`release-rollback.md`](./release-rollback.md).
- Schema rollback: run the migration image only when the release notes say the
  down migration is safe for current data. Use a secured env file whose
  `BACKEND_IMAGE` is the retained schema-matching digest:
  ```bash
  ./scripts/compose-single-server-production.sh .env.rollback \
    --profile ops run --rm migrate /usr/local/bin/mm-chat-migrate down
  ```
  Migration `010.down` must retain `go_api_runtime` and its schema-`009` API
  capability. Keep the API login on that role; never grant API capability to
  the bootstrap/migrator login or `rag_projection_owner` as a workaround.
- Data rollback: restore Postgres/MinIO from a verified backup in a disposable
  drill first; production restore is destructive.
- Frontend rollback: switch the existing frontend back to local mode until the
  server API is healthy.

## Verification

Local-development static validation with the committed example (never a
production env file):

```bash
docker compose --env-file .env.single-server.example \
  -f compose.single-server.yml --profile app --profile ops \
  --profile rag-worker --profile rag-ops config
```

Runtime validation should start with infra, run `migrate`, then start `backend`;
do not rely on API startup to apply schema changes.
