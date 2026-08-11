# Live migration-071 result

## Consumed authority

The owner granted exactly one live migration-071 authority:

```text
I_AUTHORIZE_ONE_LIVE_MIGRATION_071_MEMORY_WORKER_HEALTH_RESOLUTIONS
```

It authorized a fresh PostgreSQL backup, one migration application, candidate
image deployment with Memory behavior still disabled, and bounded read-only
verification. It did not authorize any historical-job acknowledgement, Memory
activation, application Chat request, or Provider call.

## Before-state and backup

Content-free preflight confirmed schema `070`, `user_memories/users=2/1`, two
unresolved extract dead letters (`EXTRACTION_INVALID` and `SOURCE_DRIFT`), five
future `review_expire` jobs, no resolution table, both Memory flags false, an
empty exact-UUID canary, and healthy old-image services.

A fresh custom-format PostgreSQL backup and checksum were created and verified
under the private mode-`0700/0600` evidence root:

```text
/var/tmp/neo-chat-memory-v2-071-live-20260810T030409Z
```

The first Compose invocation used an unsupported `run --no-build` flag and was
rejected before creating the migrator or contacting the database. Old services
were restored, schema `070` and health were reverified, and the corrected
invocation then performed the sole actual migration application.

## Migration and deployment result

The frozen candidate migrator returned exactly:

```text
up 071_memory_worker_health_resolutions
```

Post-migration verification proved:

```text
schema/user_memories/users=71/2/1
resolutions=0
capture pending/processing/dead=0/0/2
projection ready/pending/failed=1/0/0
review_expire pending/future=5/5
API acknowledgement execute=true
Worker acknowledgement execute=false
API/Worker resolution table CRUD=false
append-only trigger=enabled
function/table owner=memory_runtime_owner
```

The exact candidate is now deployed to only `backend` and `memory-worker`:

```text
image:    mm-chat/backend:memory-v2-v25-schema071-health-resolution-candidate-20260810t025553z
image ID: sha256:7f54f00a930da27d5671d7f23b7ebcf9615f62198266886c151b6bc1a6a7be0b
services: healthy/healthy
```

`MEMORY_HYBRID_SHADOW_ENABLED=false`, `MEMORY_TOOL_LOOP_ENABLED=false`, and the
canary remains empty. The Worker heartbeat is live; embedding capability is
expectedly false while the hybrid flag is false. No resolution row, job/error
mutation, Memory row, Provider request, or application Chat request occurred.

## Exact historical acknowledgements

After the migration authority was consumed, the owner separately authorized
the two exact one-job operations. The packaged admin capability created these
append-only rows exactly once:

```text
job_id=d141be78-01e7-47e5-9db8-09d6e3938451
expected_error_code=SOURCE_DRIFT
resolution_code=source_no_longer_current
created=true
original_job_hash=513a3b70810467c42c51c32b4ea927172680f3ba56e23d2de018a7e2e39801c0

job_id=3b384bee-87f5-41e2-9d28-72a8114c4459
expected_error_code=EXTRACTION_INVALID
resolution_code=historical_failure_accepted
created=true
original_job_hash=37e2dc19ceb6f8bacf61044c405cb36fdb74e13dd4367781633d135de38650a2
```

The original job hashes remained unchanged. Verification then proved schema,
resolution, Memory, and user counts `71/2/2/1`, unresolved extract dead letters
`0`, capture health `0/0/0`, and projection health `1/0/0`. The one-shot admin
environment was overwritten with a content-free destruction marker after use.
Because resolution evidence now exists, down migration `071` is permanently
guarded; rollback is behavior-only.

## Launch continuation

The retained admitted smoke was not replayed. A Provider-free preflight found
no pending/processing capture or embedding work, the exact sole user remained
`00000000-0000-0000-0000-000000000001`, and the reviewed candidate image ID
was unchanged. The successful launch result is recorded separately in
[`live-memory-v2-launch-result.md`](./live-memory-v2-launch-result.md).

Migration `071` and both acknowledgement authorities are consumed and must not
be rerun.
