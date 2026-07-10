# Redis Temporary State Runbook

Phase 7 introduces Redis as a non-authoritative coordination layer. Postgres
remains the source of truth for identity/session authorization, conversations,
messages, files, run status, and ownership. A Redis flush must not delete
canonical user data.

## Scope

Implemented so far:

- Redis client wiring in the Go API when `REDIS_URL` is set.
- Non-authoritative session snapshots and revocation hints for bearer session
  middleware. Every bearer authorization still rechecks Postgres first.
- Short-lived stream cancellation flags for cross-process coordination.
- Fixed-window HTTP rate-limit middleware when `REDIS_RATE_LIMIT_ENABLED=true`.
- Startup fail-fast when Redis is configured but unreachable or invalid.
- Single-server Compose defaults to `AUTH_MODE=required`, so non-public routes
  reject missing or invalid bearer credentials.

Deferred:

- Temporary upload/job state.

## Runtime Configuration

```env
REDIS_URL=redis://:<password>@redis:6379/0
REDIS_KEY_PREFIX=mm-chat
REDIS_RUN_CANCEL_TTL=10m
REDIS_SESSION_CACHE_TTL=5m
REDIS_RATE_LIMIT_ENABLED=true
REDIS_RATE_LIMIT_REQUESTS=120
REDIS_RATE_LIMIT_WINDOW=1m
```

Rules:

- Leave `REDIS_URL` empty to disable Redis and keep existing in-process cancel
  behavior.
- Keep Redis on a private Docker/host network; do not publish `6379` publicly.
- Do not log `REDIS_URL` because it may contain credentials.
- Use one `REDIS_KEY_PREFIX` per environment to avoid key collisions.
- Keep `REDIS_SESSION_CACHE_TTL` short. Redis stores only non-authoritative
  snapshots and revocation hints; Postgres `sessions` and `users` remain
  canonical for every authorization decision.
- Keep `REDIS_RATE_LIMIT_ENABLED=false` in local development if repeated manual
  API smoke tests should never receive `429`.
- Setting `REDIS_RATE_LIMIT_ENABLED=true` requires `REDIS_URL`; startup fails
  fast instead of silently running without rate limits.

## HTTP Rate Limit Contract

The middleware applies to non-health HTTP routes only. Exempt paths:

```text
/health
/ready
/v1/version
```

The general HTTP limiter hashes the request `RemoteAddr` host before storing it
in Redis. Public Identity routes add independent IP and hashed account/token
limits. For those routes, `X-Forwarded-For` is accepted only when the immediate
peer is loopback, and the backend uses the rightmost valid address supplied by
that trusted proxy. The edge proxy must replace any client-supplied chain with
`$remote_addr`; it must not use `$proxy_add_x_forwarded_for` on `/mm-api/`.

Counters are incremented with a Redis Lua script so `INCR` and TTL assignment
happen atomically for new window keys. Fixed-window key shape:

```text
{prefix}:rate_limit:http:{sha256(remote-ip)}:{window-bucket}
```

A blocked request returns:

```http
HTTP/1.1 429 Too Many Requests
Retry-After: <seconds>
X-RateLimit-Limit: <limit>
X-RateLimit-Remaining: 0
X-RateLimit-Reset: <unix-seconds>
Content-Type: application/json; charset=utf-8

{"error":{"code":"RATE_LIMITED","message":"too many requests"}}
```

Runtime Redis errors fail open because Redis is temporary state; startup still
fails fast when `REDIS_URL` is configured but cannot be parsed or pinged.

## Session Cache Contract

The auth resolver hashes each presented bearer token and performs a mandatory
Postgres `sessions`/`users` lookup before authorizing the request. A Redis
positive cache hit must never authorize a request or bypass that lookup. This
ensures session revocation, password recovery, account disablement, deletion,
expiry, and revoke-all transactions take effect on the next bearer request.

Single-server Compose currently defaults to `AUTH_MODE=required`. Missing
credentials on non-public routes therefore fail closed. Direct backend
development may still use `AUTH_MODE=development`, but every bearer credential
that is presented is resolved through Postgres rather than trusted from Redis.

Authorization behavior:

```text
bearer token
  -> SHA-256 token hash
  -> mandatory Postgres sessions/users lookup
  -> validate active account plus unexpired, unrevoked session
  -> authorize from the canonical Postgres row
  -> best-effort Redis snapshot write with a short TTL
```

Redis key shapes:

```text
{prefix}:sessions:token:{sha256(token_hash)}
{prefix}:sessions:{sessionId}:revoked
```

Cached values may include only browser-safe fields: `sessionId`, `userId`,
`displayName`, `role`, `expiresAt`, and `cachedAt`. They must not include raw
bearer tokens, token hashes, provider secrets, IP addresses, or user agents.

Rules:

- Cache TTL is the sooner of `REDIS_SESSION_CACHE_TTL` and the canonical
  `sessions.expires_at`.
- Redis snapshot/hint errors do not change the Postgres authorization result;
  Postgres lookup errors fail closed.
- Expired or revoked sessions are not cached. Durable revoke must update
  Postgres first, then best-effort delete the snapshot and optionally set the
  short-lived revocation hint.
- Redis snapshot deletion, revocation-hint cleanup, and manual purge are cache
  hygiene only. They are not prerequisites for authorization correctness
  because no positive Redis entry can bypass the mandatory Postgres check.
- A Redis flush removes only non-authoritative snapshots and hints. Active
  sessions remain authorized, and revoked or disabled sessions remain denied,
  according to Postgres.

## Cancellation Flag Contract

Cancel endpoint flow:

```text
POST /v1/chat/runs/{runId}/cancel
  -> update Postgres message status to cancelled
  -> cancel same-process active run if present
  -> SET {prefix}:runs:{runId}:cancelled 1 EX REDIS_RUN_CANCEL_TTL
```

Stream workers poll the flag while the provider request is active. When a flag
is found, the worker cancels its provider context, emits `message.cancelled`,
and finalizes the assistant row as `cancelled`. After stream exit, the worker
clears the flag; TTL is the fallback cleanup path.

## Failure and Flush Behavior

- Redis disabled: same-process cancellation and durable Postgres cancellation
  still work; bearer session resolution continues to require Postgres, and
  rate-limit middleware is inactive without a Redis store.
- Redis outage after startup: durable cancel still writes Postgres; cross-process
  provider interruption, session snapshot/hint maintenance, and rate limiting
  may degrade until Redis recovers. Authorization still follows Postgres.
- Redis flush: session-cache entries, active cross-process cancellation flags,
  and rate-limit counters are lost, but users, sessions, conversations,
  messages, files, and run status remain readable from Postgres. No Redis purge
  or rebuild is required to preserve authorization correctness.

## Verification

```bash
cd mm-chat/backend
MM_CHAT_TEST_REDIS_URL=redis://:<password>@127.0.0.1:6379/0 go test ./internal/redisstate
go test ./internal/auth -run SessionResolver
go test ./internal/httpserver -run RateLimit
```

The auth integration test that exercises Redis `FLUSHDB` and confirms the next
resolution still uses current Postgres state requires a disposable Redis
database and an explicit guard:

```bash
MM_CHAT_TEST_REDIS_URL=redis://:<password>@127.0.0.1:6379/0 \
MM_CHAT_TEST_REDIS_ALLOW_FLUSH=true \
MM_CHAT_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5432/mm_chat?sslmode=disable \
go test ./internal/auth -run TestSessionResolverIntegrationFallsBackToPostgresAfterRedisFlush
```

For full smoke, run Postgres and Redis together, create/read chat data through
the API, `FLUSHDB` Redis, then read the same conversation/message again. Only
active temporary jobs may be affected.
