# Memory v2 durable capture worker contracts

## 1. Scope / Trigger

Apply this contract when changing completed-turn Memory capture, migration
`054_memory_outbox_jobs_worker`, the private Go Memory Worker, Redis Memory
wake signals, Server provider hydration for Memory, or the worker's Compose
and database-role wiring.

PR3 retains the Global-only v1 Memory reader and HTTP CRUD contract. Project
routing, evidence/revision/tombstone, review/conflict handling, embeddings,
L2/L3, and external Memory engines belong to later batches.

## 2. Signatures

### Chat capture

```go
type FinalizeAssistantMessageInput struct {
    // existing fields omitted
    MemoryCapture *MemoryCaptureInput
}

type MemoryCaptureInput struct {
    EventID, JobID, UserMessageID string
    ProviderSource, ProviderID, ModelID string
    EventSchemaMajor int
}
```

Only `Status == "completed"` may carry capture. The repository finalizes the
assistant message and calls `memory_append_turn_completed_event(...)` in the
same transaction. Its nullable return is the only wake authority: publish
Redis only when the returned event ID is non-null.

### Database

```text
memory_outbox(event_id, user_id, event_schema_major, event_type,
  aggregate_id, visibility_epoch, payload, status/attempt/lease/error/times)

memory_jobs(job_id, user_id, event_id, stage, idempotency_key,
  source conversation/message/assistant IDs + source_hash,
  provider source/id/record/update time + model/profile hash,
  scope/project generations + visibility_epoch,
  status/attempt/lease/error/times)
```

Narrow functions:

```sql
memory_append_turn_completed_event(UUID, UUID, UUID, UUID, UUID, UUID,
  TEXT, TEXT, TEXT, SMALLINT) RETURNS UUID
memory_worker_claim_job(UUID, UUID, INTEGER) RETURNS TABLE (...)
memory_worker_hydrate_capture(UUID, UUID, UUID) RETURNS TABLE (...)
memory_worker_apply_capture_candidate(UUID, UUID, UUID, UUID,
  TEXT, TEXT, TEXT, SMALLINT, TEXT[]) RETURNS TABLE (...)
memory_worker_complete_job(UUID, UUID, UUID) RETURNS VOID
memory_worker_retry_job(UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN)
  RETURNS TEXT
memory_worker_readiness() RETURNS TABLE (...)
```

### Command and environment

```bash
mm-chat-memory-worker run
mm-chat-memory-worker healthcheck
```

Required:

```text
MEMORY_WORKER_DATABASE_URL
PROVIDER_SECRET_KEYRING_FILE
```

Bounded settings:

```text
MEMORY_WORKER_MAX_OPEN_CONNS=4
MEMORY_WORKER_MAX_IDLE_CONNS=2
MEMORY_WORKER_CONN_MAX_LIFETIME=30m
MEMORY_WORKER_CONCURRENCY=2
MEMORY_WORKER_LEASE_DURATION=2m
MEMORY_WORKER_POLL_INTERVAL=1s
MEMORY_WORKER_BACKOFF_BASE=5s
MEMORY_WORKER_BACKOFF_MAX=15m
PROVIDER_TIMEOUT=45s
REDIS_URL=<optional>
REDIS_KEY_PREFIX=mm-chat
```

`PROVIDER_TIMEOUT + 5s` must be less than the lease duration.

## 3. Contracts

- PostgreSQL is the only queue authority. Redis channel
  `mm-chat:memory:outbox:v1` carries one UUID `event_id` and no content.
- Event payload is an object containing only schema major, source IDs/hash,
  scope/project generations, visibility epoch, and provider profile
  ID/model/hash references. It never contains message/Memory text or secrets.
- The API has execute permission only on the append function. The worker login
  inherits only `memory_worker_runtime`, which has execute permission only on
  the worker functions and no direct table CRUD.
- All worker functions are `SECURITY DEFINER`, owned by restricted NOLOGIN
  `memory_runtime_owner`, with an application-schema/`pg_catalog`/`pg_temp`
  search path. The worker role must never inherit the owner role.
- Claim increments a bounded attempt and binds worker ID, lease token, and
  expiry. Hydrate/apply/complete/retry revalidate the live lease.
- Hydration rechecks user/source/assistant ownership, source hash, active
  Learn policy, Conversation/Project generations, Provider record/timestamp,
  and profile hash before returning bounded source/provider data.
- Worker supports event schema current `N` and `N-1`, and terminally rejects
  any other major or unsupported stage before hydration.
- Provider execution is bounded to 12,000 source characters, 45 seconds by
  default, 32 KiB output, and five accepted candidates. `context` and
  credential-like content/tags are discarded.
- Apply reuses `usermemory.Service.StoreExtracted` and the v1 Global upsert.
  Candidate-wide atomic proposals and manual-precedence review are deferred.
- Redis startup/subscription failure is warning-only; polling continues.
  Worker failure or backlog never blocks chat or v1 Recall.
- Rollback never deletes queued work: down `054` requires both tables empty.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Learn disabled, Memory disabled, or inactive Project | Append returns `NULL`; no event/job and no Redis wake. |
| Assistant/source/conversation ownership mismatch | `MEMORY_CAPTURE_*_INVALID`; finalize transaction rolls back. |
| Duplicate assistant capture with the same payload | Return the original event; keep one event and one extract job. |
| Duplicate capture conflicts with pinned payload | `MEMORY_CAPTURE_EVENT_CONFLICT`; never rewrite the first event. |
| Invalid/expired/stale lease | `MEMORY_JOB_LEASE_LOST` or `MEMORY_OUTBOX_LEASE_LOST`. |
| Deleted/changed/cross-user source or Learn/generation drift | `MEMORY_CAPTURE_SOURCE_DRIFT`; terminal dead-letter. |
| Provider missing or request-only profile cannot be hydrated | `MEMORY_PROVIDER_UNAVAILABLE`; fail closed. |
| Provider record/timestamp/profile hash changed | `MEMORY_PROFILE_DRIFT`; terminal dead-letter. |
| Provider timeout/failure or invalid bounded output | Retry with sanitized code and bounded backoff. |
| Crash before completion | Reclaim expired lease while attempts remain. |
| Crash on the final attempt | Mark event/job `dead_letter` with `LEASE_EXPIRED`. |
| Unknown schema or stage | Terminal dead-letter before source hydration. |
| Worker attempts direct table read/write | PostgreSQL permission denied. |
| Down migration sees any event/job history | `MEMORY_WORKER_ROLLBACK_REQUIRES_EMPTY_QUEUE`. |

## 5. Good / Base / Bad Cases

- **Good**: a completed eligible turn atomically creates one ID-only event/job;
  Redis wakes a private worker; the worker claims, hydrates, applies bounded
  candidates, and completes under the same live lease.
- **Base**: Redis is absent and extraction yields no candidates. PostgreSQL
  polling still claims the job and completes it without creating Memory.
- **Bad**: enqueue body text in Redis, run an API-local consumer goroutine,
  grant the worker table CRUD/owner membership, log Provider errors/source
  text, accept a stale lease, or delete rows to force rollback.

## 6. Tests Required

- Run focused race tests for `internal/memoryworker`, `internal/chat`,
  `internal/redisstate`, `internal/providerfactory`, `internal/runtimeconfig`,
  `internal/migration`, and `cmd/memory-worker`.
- Run `go test ./...` and `go vet ./...` from `mm-chat/backend`.
- Against disposable PostgreSQL 17, replay `001 -> 054`, down `054`, and re-up.
  Assert finalize/event rollback atomicity, ineligible `NULL`, duplicate
  idempotency, stale lease rejection, crash reclaim, final-attempt
  dead-letter, cross-user/source denial, candidate apply/complete, restricted
  role direct-table denial, and guarded rollback.
- Run `scripts/test-preflight-single-server.sh` and render Compose with the
  `app` and `memory-worker` profiles. Assert private-only networking, no ports,
  independent credential/pool, read-only/cap-drop/resource limits, keyring
  secret, PostgreSQL-only dependency, and `PROVIDER_TIMEOUT` mapping.
- Build the backend image and prove it contains
  `/usr/local/bin/mm-chat-memory-worker`.
- Offline tests use fake Providers and never call a Live Provider.

## 7. Wrong vs Correct

### Wrong

```go
go extractAndStore(requestContext, provider, fullUserText)
redis.Enqueue(fullUserText)
```

This loses work on process/Redis failure, duplicates private text, retains
request-local credentials, and has no replay fence.

### Correct

```text
assistant finalize + ID-only event/job in one PostgreSQL transaction
  -> publish returned event_id best-effort
  -> claim/hydrate/apply/complete through lease-fenced SQL functions
  -> PostgreSQL polling recovers when Redis or the process fails
```
