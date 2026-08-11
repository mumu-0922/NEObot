# Live migration-071 and exact health-resolution plan

## Frozen candidate and current boundary

```text
candidate image: mm-chat/backend:memory-v2-v25-schema071-health-resolution-candidate-20260810t025553z
candidate ID:    sha256:7f54f00a930da27d5671d7f23b7ebcf9615f62198266886c151b6bc1a6a7be0b
offline evidence: /var/tmp/neo-chat-memory-v2-071-offline-20260810T025553Z
current live schema: 070
current live image:  mm-chat/backend:memory-v2-v25-exact-uuid-policy-fix-candidate-20260810t011729z
current live ID:     sha256:c79fd467421342c63185321fa966039419528ee507c9b0538f69901fe3568e08
current behavior:    both Memory flags false; exact-UUID allowlist empty
current counts:      user_memories=2; users=1
Provider authority:  none; consumed admitted smoke must not be replayed
```

Every live write below is separately gated. Read-only preflight must use only
IDs, status/stage, bounded error codes, timestamps/ages, Conversation lifecycle,
role privileges, schema version, and aggregate counts. It must not select
message or Memory content, Provider bodies, secrets, or raw errors.

## Gate A — one live migration-071 (completed)

Required fresh authority:

```text
I_AUTHORIZE_ONE_LIVE_MIGRATION_071_MEMORY_WORKER_HEALTH_RESOLUTIONS
```

1. Reconfirm the frozen image ID, schema `070`, flags/canary disabled, healthy
   current services, one user, two Memory rows, two unresolved extract dead
   letters, five future `review_expire` jobs, and an empty/nonexistent
   resolution table.
2. Create and verify a fresh PostgreSQL backup and capture a mode-`0600`
   before-state. Do not reuse or overwrite the migration-070 evidence set.
3. Stop only `backend` and `memory-worker`; keep PostgreSQL available. Apply
   `071` exactly once using the frozen candidate migrator and the migration-only
   database credential. No API/admin/Worker credential is accepted here.
4. Require exactly one changed migration named
   `071_memory_worker_health_resolutions`. Verify schema `071`, unchanged
   persistent counts, empty resolution evidence, composite ownership FK,
   append-only trigger, exact function grants, Worker denial, and health counts
   `capture_pending=0/capture_processing=0/capture_dead_letter=2` for the sole
   user. The five `review_expire` rows must still exist unchanged.
5. Recreate `backend` and `memory-worker` on the candidate image with both
   Memory flags still false and the canary empty. Require both containers
   healthy. Do not acknowledge a job, enable Memory, or issue an application
   Chat request under Gate A.

Failure before an acknowledgement: keep flags/canary disabled, stop the new
services, down `071` only if its resolution table is empty, restore the prior
image, and verify schema `070` plus unchanged counts. A failed transactional up
requires no down. No automatic retry is authorized.

## Gate B — exact SOURCE_DRIFT acknowledgement

After Gate A, run one content-free preflight to obtain the sole unresolved
`SOURCE_DRIFT` extract job UUID. Require same bootstrap owner, `dead_letter`,
exact error `SOURCE_DRIFT`, non-null terminal time, and a present same-owner
Conversation that is non-active. Print the exact UUID and frozen predicates,
then stop for fresh authority:

```text
I_AUTHORIZE_ONE_LIVE_MEMORY_HEALTH_ACKNOWLEDGEMENT \
  job_id=d141be78-01e7-47e5-9db8-09d6e3938451 \
  expected_error_code=SOURCE_DRIFT \
  resolution_code=source_no_longer_current
```

Execute the packaged admin command once in a one-shot private-network container
that receives only `DATABASE_URL`, `AUTH_BOOTSTRAP_USER_ID`, and
`AUTH_BOOTSTRAP_DISPLAY_NAME`; do not mount or export Provider credentials:

```text
mm-chat-admin memory-health-acknowledge
  --job-id d141be78-01e7-47e5-9db8-09d6e3938451
  --expected-error-code SOURCE_DRIFT
  --resolution-code source_no_longer_current
  --approval I_ACKNOWLEDGE_ONE_HISTORICAL_MEMORY_CAPTURE_HEALTH_FAILURE
```

Require `created=true`, one append-only row, unchanged original job/error, and
capture dead-letter count `1`. Never retry an ambiguous result automatically;
read the exact resolution row and same-input idempotent state first.

## Gate C — exact EXTRACTION_INVALID acknowledgement

Run a new content-free preflight for the sole remaining
`EXTRACTION_INVALID` extract job. Require same bootstrap owner, `dead_letter`,
exact error `EXTRACTION_INVALID`, and terminal age at least 24 hours. Print the
exact UUID and age, then stop for fresh authority:

```text
I_AUTHORIZE_ONE_LIVE_MEMORY_HEALTH_ACKNOWLEDGEMENT \
  job_id=<exact-canonical-uuid> \
  expected_error_code=EXTRACTION_INVALID \
  resolution_code=historical_failure_accepted
```

Execute the same one-shot admin boundary exactly once with the second job and
fixed approval. Require `created=true`, two total append-only resolutions,
unchanged original jobs/errors, capture counts `0/0/0`, projection counts
`ready=1/pending=0/failed=0`, unchanged `user_memories/users=2/1`, and both
Worker capabilities live.

After the first acknowledgement, migration down is intentionally unavailable.
Rollback is behavioral only: keep both Memory flags false, clear the canary,
and retain schema `071`, the original jobs, and all resolution evidence.

## Launch continuation after Gate C

The already-consumed admitted smoke is retained as behavior evidence and is not
replayed. With no Provider authority and no application Chat POST:

1. Reconfirm candidate image ID, schema `071`, exact two resolutions, unchanged
   rows/counts, ready direct health, and exact sole-user UUID.
2. Atomically set the two reviewed Memory flags true and the canary to only that
   canonical UUID; recreate only `backend` and `memory-worker` on the candidate
   image.
3. Require authenticated `GET /v1/memory-health` = `ready`, a disposable
   unauthenticated/non-admitted request = HTTP `401`, zero new hybrid
   observations/Usage and zero Provider/Memory work for that request, unchanged
   persistent counts, and both services healthy.
4. Preserve a prepared behavior rollback that restores both flags false and an
   empty canary without changing schema or deleting any row. Capture the final
   environment hash, container/image IDs, schema/counts, health, non-admitted
   proof, and rollback-readiness evidence.

Any health/count/identity/privilege drift stops with Memory disabled. No stage
permits a Provider request, migration replay, bulk acknowledgement, job/error
mutation, evidence deletion, or automatic retry.
