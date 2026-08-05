# Memory v2 durable capture worker contracts

## 1. Scope / Trigger

Apply this contract when changing completed-turn Memory capture, migration
`054_memory_outbox_jobs_worker`, the private Go Memory Worker, Redis Memory
wake signals, Server provider hydration for Memory, or the worker's Compose
and database-role wiring. The production L1 successor additionally covers
`066_memory_auto_capture_promotion`,
`067_memory_auto_capture_authority_hardening`, and
`068_memory_auto_capture_tool_evidence_profile` plus
`069_memory_auto_capture_compatible_tool_profile`. Runtime availability and
settings UX additionally use `070_memory_worker_health`.

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
memory_worker_promote_capture_candidates(UUID, UUID, UUID)
  RETURNS TABLE (promoted_count, review_count, rejected_count)
memory_worker_heartbeat(UUID, INTEGER, BOOLEAN) RETURNS BOOLEAN
memory_worker_retire(UUID) RETURNS BOOLEAN
memory_user_health(UUID) RETURNS TABLE (
  worker_available, embedding_worker_available,
  capture_pending_count, capture_processing_count,
  capture_dead_letter_count, projection_ready_count,
  projection_pending_count, projection_failed_count
)
```

Authenticated HTTP health is `GET /v1/memory-health`. It returns only bounded
`ready|indexing|degraded|disabled`, a fixed reason code, the two worker booleans,
ready/pending/failed aggregate counts, and the fixed `gpt-5.6-luna` judge
identity. It never returns a database/Provider/Base URL error or plaintext.

Production extraction uses `chat.ToolRoundProvider.CompleteToolRound(...)`
with exactly one required call per round:

```text
propose_memory_candidates
propose_memory_candidate_decisions
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
  Worker failure or backlog never blocks chat generation. Existing non-empty,
  fully authorized final Memory may still be released, but a candidate-empty
  Tool result must consult user health and may not masquerade as a healthy miss.
- Rollback never deletes queued work: down `054` requires both tables empty.

### Runtime health (`070`)

- `Worker.Run` must successfully register a heartbeat before starting any lane,
  refresh it every five seconds with a 20-second TTL, cancel all lanes on a
  refresh failure, and best-effort retire it within two seconds after stop.
- A heartbeat stores only worker UUID, embedding-enabled capability, and
  timestamps. PostgreSQL expiry—not a process-local bool—is liveness authority.
- `memory_worker_runtime` may execute heartbeat, retire, and readiness only;
  `go_api_runtime` may execute only `memory_user_health`. Neither can read or
  mutate the heartbeat table directly.
- User health counts only the requested user's current eligible canonical
  Memory/projection authority. A missing current projection counts as pending;
  stale generations, disabled/deleted/expired Memory, and other users do not.
- Status precedence is Tool/User disabled -> `disabled`; missing extraction or
  embedding worker -> `degraded`; failed projection/dead-letter -> `degraded`;
  pending/processing capture or projection -> `indexing`; otherwise `ready`.
- A globally disabled Tool flag returns bounded `disabled` without requiring a
  settings or health repository read. Once the Tool flag and user Use settings
  are enabled, a missing health repository is HTTP 503
  `MEMORY_HEALTH_UNAVAILABLE`; raw repository errors remain hidden.
- Down `070` refuses while any heartbeat is live. Stop/retire the Worker or wait
  for TTL expiry; runtime roles must never delete rows to force rollback.

### Production L1 successor (`066`–`069`)

- Free-text Provider JSON is no longer write authority. Each extraction or
  decision round requires exactly one completed Provider-issued Tool Call with
  the exact name, non-empty non-synthetic call ID, no failure category, no
  prose fallback, and strict bounded arguments. Protocol-invalid responses use
  at most three total attempts and end as sanitized `EXTRACTION_INVALID`.
  Extraction profile v5 enumerates the exact hydrated user-role IDs for
  authority and assistant-role IDs for optional context in the Tool schema;
  semantic validation still rejects any drift.
- One exact committed candidate batch is promoted only through
  `memory_worker_promote_capture_candidates(...)`; Go never duplicates the
  canonical insert/evidence/audit transaction.
- Migration `066` establishes governance-backed safe-add promotion. Because
  its bytes are already applied authority, they are immutable. Migration `067`
  is the forward fix that additionally binds exact Tool profile hashes, batch
  completeness, candidate content/hash, and every evidence row's current
  message role, content hash, completion timestamp, deletion state, and source
  Conversation.
- Migration `068` preserves the applied `067` bytes and advances only the
  extraction authority to profile v4 so evidence-role enums and SQL promotion
  profile hashes agree.
- Migration `069` preserves applied `068` bytes and removes the Provider-
  rejected `uniqueItems` keyword. Bounded per-role enums remain in the v5
  schema; local validation retains duplicate/forgery rejection.
- Only current normal, non-temporary, explicitly confirmed, conflict-free
  `SHADOW_ADD` may auto-promote. Tombstones, exact/fact conflicts, related
  targets, Sensitive-disabled candidates, temporary facts, stale
  lease/source/profile/scope/epoch/evidence, and disabled settings fail closed
  or remain Review as classified.
- The function reuses `memory_governance_decide_review(...)` so canonical
  Memory, evidence, `auto_accept`/`AUTO_CAPTURED` audit, and completed assistant
  Activity commit atomically. Replay cannot create a second canonical row.
- `memory_worker_runtime` remains function-only. Migrations `066`–`069` grant
  no table CRUD or reader/prompt promotion authority.
- Logs persist only fixed extraction/provider failure categories; they never
  include Provider response bodies, Tool arguments, chat text, or raw errors.

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
| Missing/duplicate/wrong-name/failed/oversized/malformed/synthetic Tool Call | Retry within three total protocol attempts, then `EXTRACTION_INVALID`; persist no Provider output. |
| Committed batch profile/count differs from the required Tool authority | `MEMORY_PROFILE_DRIFT`; no canonical write. |
| Candidate bytes differ from `candidate_hash` | Retain as Review with `AUTO_PROMOTION_CANDIDATE_DRIFT`. |
| Any retained evidence row is missing, changed, deleted, incomplete, wrong-role, wrong-Conversation, or timestamp-stale | Retain as Review with `AUTO_PROMOTION_EVIDENCE_STALE`, except primary source authority drift that fails the lease transaction. |
| `067` down sees `auto_accept` or `AUTO_CAPTURED` history | `MEMORY_AUTO_CAPTURE_AUTHORITY_ROLLBACK_REQUIRES_NO_PROMOTIONS`; preserve history. |
| `068` down sees `auto_accept` or `AUTO_CAPTURED` history | `MEMORY_AUTO_CAPTURE_TOOL_PROFILE_ROLLBACK_REQUIRES_NO_PROMOTIONS`; preserve history. |
| `069` down sees `auto_accept` or `AUTO_CAPTURED` history | `MEMORY_AUTO_CAPTURE_COMPATIBLE_PROFILE_ROLLBACK_REQUIRES_NO_PROMOTIONS`; preserve history. |
| Initial or periodic heartbeat fails | Start no lane or cancel all active lanes; return failure so the process restarts. |
| Heartbeat worker/capability is `NULL` or TTL is outside 5–120 seconds | `MEMORY_WORKER_HEARTBEAT_INVALID`; create or refresh no row. |
| API or worker attempts direct heartbeat-table CRUD | PostgreSQL permission denied. |
| Tool flag is disabled | HTTP health is bounded `disabled` without requiring repository health. |
| Tool/User Use is enabled but health capability is unavailable | HTTP 503 `MEMORY_HEALTH_UNAVAILABLE`; candidate-empty Tool result is `memory_status_unavailable`. |
| Health sees no live embedding-capable Worker | `degraded`; do not report a candidate-empty Tool read as a healthy miss. |
| Current eligible Memory has no current projection | Count it as pending and report `indexing`. |
| `070` down sees a live heartbeat | `MEMORY_HEALTH_ROLLBACK_REQUIRES_STOPPED_WORKERS`; preserve all state. |

## 5. Good / Base / Bad Cases

- **Good**: a completed eligible turn atomically creates one ID-only event/job;
  Redis wakes a private worker; the worker claims, hydrates, applies bounded
  candidates, and completes under the same live lease.
- **Base**: Redis is absent and extraction yields no candidates. PostgreSQL
  polling still claims the job and completes it without creating Memory.
- **Bad**: enqueue body text in Redis, run an API-local consumer goroutine,
  grant the worker table CRUD/owner membership, log Provider errors/source
  text, accept a stale lease, or delete rows to force rollback.
- **Production successor Good**: exact candidate and decision Tool Calls commit
  one complete v5 batch; `069` retains every `067` fence and atomically promotes one
  safe school fact with evidence/audit/Activity.
- **Production successor Base**: an exact empty Tool batch completes the job
  without canonical writes; an ineligible temporary/conflicting candidate
  remains Review.
- **Production successor Bad**: edit applied `066`–`069` bytes or checksums, trust adapter-
  synthesized Tool IDs, promote a partial batch, or write canonical Memory
  directly from Go.
- **Health Good**: one embedding-capable Worker heartbeats, the current user's
  projections are ready, and settings show `ready`; another user's failed row
  cannot affect the response.
- **Health Base**: capture/projection work is pending, so settings show
  `indexing` and an actually invoked empty Memory Tool exposes one safe indexing
  reason while the answer continues without Memory.
- **Health Bad**: infer liveness from Compose/container status, return a raw SQL
  error to the browser, treat missing projections as ready, or fall back to the
  retired reader when the Worker is absent.

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
- Pin the applied `066`–`069` checksums; replay
  `066 -> 067 -> 066 -> 067` and `067 -> 068 -> 067 -> 068` on disposable
  PostgreSQL 17 plus `068 -> 069 -> 068 -> 069`, and assert Tool-profile/batch/candidate/all-evidence
  fences, tombstone/conflict/temporary/Sensitive behavior, atomic promotion,
  idempotent replay, projection enqueue, function-only denial, and all promotion-
  history rollback guards.
- Replay `069 -> 070 -> 069 -> 070` on disposable PostgreSQL 17. Prove worker
  heartbeat/retire/readiness, API/worker cross-function denial, direct-table
  denial, invalid heartbeat input rejection, active-heartbeat rollback refusal,
  user isolation, missing/pending/ready/failed projection counts, bounded 503
  health failure, Tool-disabled repository independence, and clean re-up.
- Frontend tests must prove independent Governance/Health loading, a bounded
  degraded badge on Health failure, periodic refresh, and fixed Sol/Luna model
  responsibility labels without copying Provider configuration into the UI.

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

Production L1 successor:

```text
exact Provider-issued Tool Calls -> atomic hash-pinned candidate batch
  -> migration-069 compatible profile + authority recheck
  -> existing governance decision transaction
  -> canonical + evidence + audit + Activity, or fail closed / Review
```

Runtime health:

```text
Wrong: container-is-running -> assume Memory is ready -> treat empty as miss
Correct: PostgreSQL heartbeat + current-user projection/capture state
  -> bounded status -> healthy empty or explicit fail-closed Tool result
```
