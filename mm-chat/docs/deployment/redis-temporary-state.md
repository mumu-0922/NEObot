# Redis Temporary State Runbook

Phase 7 introduces Redis as a non-authoritative coordination layer. Postgres
remains the source of truth for conversations, messages, files, run status, and
ownership. A Redis flush must not delete canonical user data.

## Scope

Implemented in this slice:

- Redis client wiring in the Go API when `REDIS_URL` is set.
- Short-lived stream cancellation flags for cross-process coordination.
- Startup fail-fast when Redis is configured but unreachable or invalid.

Deferred:

- Rate-limit middleware.
- Session cache integration.
- Temporary upload/job state.

## Runtime Configuration

```env
REDIS_URL=redis://:<password>@redis:6379/0
REDIS_KEY_PREFIX=mm-chat
REDIS_RUN_CANCEL_TTL=10m
```

Rules:

- Leave `REDIS_URL` empty to disable Redis and keep existing in-process cancel
  behavior.
- Keep Redis on a private Docker/host network; do not publish `6379` publicly.
- Do not log `REDIS_URL` because it may contain credentials.
- Use one `REDIS_KEY_PREFIX` per environment to avoid key collisions.

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
  still work.
- Redis outage after startup: durable cancel still writes Postgres; cross-process
  provider interruption may degrade until the stream ends or the process recovers.
- Redis flush: active cross-process cancellation flags are lost, but
  conversations, messages, files, and run status remain readable from Postgres.

## Verification

```bash
cd mm-chat/backend
MM_CHAT_TEST_REDIS_URL=redis://:<password>@127.0.0.1:6379/0 go test ./internal/redisstate
```

For full smoke, run Postgres and Redis together, create/read chat data through
the API, `FLUSHDB` Redis, then read the same conversation/message again. Only
active temporary jobs may be affected.
