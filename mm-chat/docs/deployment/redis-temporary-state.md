# Redis Temporary State Runbook

Phase 7 introduces Redis as a non-authoritative coordination layer. Postgres
remains the source of truth for conversations, messages, files, run status, and
ownership. A Redis flush must not delete canonical user data.

## Scope

Implemented so far:

- Redis client wiring in the Go API when `REDIS_URL` is set.
- Read-through session-cache substrate for bearer session middleware.
- Short-lived stream cancellation flags for cross-process coordination.
- Fixed-window HTTP rate-limit middleware when `REDIS_RATE_LIMIT_ENABLED=true`.
- Startup fail-fast when Redis is configured but unreachable or invalid.

Deferred:

- Enforced hosted auth mode and full two-user isolation tests.
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
- Keep `REDIS_SESSION_CACHE_TTL` short. Redis is only a lookup cache and
  revocation-hint layer; Postgres `sessions` remains canonical.
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

Current identity is the request `RemoteAddr` host, hashed before it is stored in
Redis. The backend does not trust `X-Forwarded-For` yet; if it is deployed
behind a reverse proxy, all clients may share the proxy IP until a trusted-proxy
configuration is added.

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

The auth substrate has a Postgres-backed resolver and Redis cache store. Phase 13
uses it for optional bearer session middleware; missing credentials still fall
back to the development user until enforced auth mode is enabled.

Read-through behavior:

```text
token hash
  -> Redis session cache lookup
  -> Redis revocation hint check
  -> Postgres sessions/users lookup on cache miss or Redis error
  -> cache active session snapshot with short TTL
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
- Redis cache read/write errors fall back to Postgres; Postgres errors fail
  closed.
- Expired or revoked sessions are not cached. Durable revoke must update
  Postgres first, then delete the cache entry and optionally set the short-lived
  revocation hint.
- A Redis flush only causes cache misses; active sessions can be rebuilt from
  Postgres.

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
  still work; session resolution falls back to Postgres when wired into HTTP,
  and rate-limit middleware is inactive without a Redis store.
- Redis outage after startup: durable cancel still writes Postgres; cross-process
  provider interruption, session-cache speed, and rate limiting may degrade
  until Redis recovers.
- Redis flush: session-cache entries, active cross-process cancellation flags,
  and rate-limit counters are lost, but users, sessions, conversations,
  messages, files, and run status remain readable from Postgres.

## Verification

```bash
cd mm-chat/backend
MM_CHAT_TEST_REDIS_URL=redis://:<password>@127.0.0.1:6379/0 go test ./internal/redisstate
go test ./internal/auth -run SessionResolver
go test ./internal/httpserver -run RateLimit
```

The auth integration test that exercises Redis `FLUSHDB` requires a disposable
Redis database and an explicit guard:

```bash
MM_CHAT_TEST_REDIS_URL=redis://:<password>@127.0.0.1:6379/0 \
MM_CHAT_TEST_REDIS_ALLOW_FLUSH=true \
MM_CHAT_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5432/mm_chat?sslmode=disable \
go test ./internal/auth -run TestSessionResolverIntegrationFallsBackToPostgresAfterRedisFlush
```

For full smoke, run Postgres and Redis together, create/read chat data through
the API, `FLUSHDB` Redis, then read the same conversation/message again. Only
active temporary jobs may be affected.
