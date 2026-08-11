# Live sole-user Memory v2 launch result

## Outcome

Memory v2 is live for the deployment's sole exact user:

```text
user_id=00000000-0000-0000-0000-000000000001
MEMORY_HYBRID_SHADOW_ENABLED=true
MEMORY_TOOL_LOOP_ENABLED=true
MEMORY_TOOL_LOOP_CANARY_USER_IDS=00000000-0000-0000-0000-000000000001
```

The deployed artifact is unchanged:

```text
image=mm-chat/backend:memory-v2-v25-schema071-health-resolution-candidate-20260810t025553z
image_id=sha256:7f54f00a930da27d5671d7f23b7ebcf9615f62198266886c151b6bc1a6a7be0b
backend=healthy
memory-worker=healthy
```

No migration, acknowledgement, Provider request, or application Chat POST was
performed during launch. The already-consumed admitted smoke was not replayed.

## Provider-free preflight

Before enabling behavior, aggregate-only checks proved:

```text
schema/resolutions/user_memories/users=71/2/2/1
sole_user=00000000-0000-0000-0000-000000000001
capture_active=0
embedding_active=0
projection_ready_pending_failed=1/0/0
hybrid_observations=4
memory_usages=1
```

The active environment and its SHA-256 were captured with mode `0600`; the
evidence directory is mode `0700`. A behavior rollback was prepared with the
same candidate image, both flags false, and an empty canary.

## Fail-closed verifier correction

The first flag activation reached ready health, but the final verification
harness queried the nonexistent identity path `/v1/auth/me`. Its HTTP `404`
triggered the prepared behavior rollback, which restored both flags to false,
cleared the canary, and recreated only `memory-worker` and `backend`. Schema,
data, and the candidate image were unchanged, and no Provider or Chat request
occurred. This evidence is retained under:

```text
/var/tmp/neo-chat-memory-v2-sole-user-launch-20260810T032745Z
```

Source inspection and a provider-free GET proved the correct identity route is
`/v1/me`. The corrected retry used a fresh evidence root and did not reuse or
replay any consumed live authority.

## Successful launch verification

The retry atomically changed only the two reviewed flags and the exact-UUID
canary, then force-recreated only `memory-worker` and `backend` with
`--no-build --no-deps`. Final checks proved:

```text
GET /v1/me fixed owner UUID=00000000-0000-0000-0000-000000000001
GET /v1/memory-health HTTP=200 status=ready
worker_available/embedding_worker_available=true/true
capture_pending_processing_dead=0/0/0
projection_ready_pending_failed=1/0/0
schema/resolutions/user_memories/users=71/2/2/1
hybrid_observations=4
memory_usages=1
```

The live runtime uses `AUTH_MODE=development`, so the fixed owner is injected
by the development-session middleware without exposing a raw bearer token. A
disposable same-image instance with `AUTH_MODE=required` returned HTTP `401`
for an unauthenticated `GET /v1/memory-health`. Its logs contained zero
Provider/Memory-work events, and before/after durable counts were identical.

After an additional observation interval, both live services remained healthy,
the HTTP health endpoint remained ready, active capture/embedding work remained
zero, and startup logs contained zero Provider/Memory-work events. Successful
evidence is retained at mode `0700/0600` under:

```text
/var/tmp/neo-chat-memory-v2-sole-user-launch-retry-20260810T033338Z
```

## Rollback boundary

Behavior rollback remains available by setting both Memory flags false,
clearing the canary, and recreating only `memory-worker` and `backend` on the
same pinned image. Migration `071` cannot be downed because append-only
resolution evidence exists. Adding a second user requires a new rollout
decision; the current single-user acceptance does not authorize widening.
