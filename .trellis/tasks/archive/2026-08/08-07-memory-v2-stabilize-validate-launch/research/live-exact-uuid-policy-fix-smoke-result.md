# Live exact-UUID policy-fix admitted smoke result

## Authority and deployment

The owner granted one fresh exact-UUID policy-fix admitted-smoke authority with
`provider.source=server-default`. It was consumed exactly once on 2026-08-10;
there was no automatic replay.

```text
image:        mm-chat/backend:memory-v2-v25-exact-uuid-policy-fix-candidate-20260810t011729z
image ID:     sha256:c79fd467421342c63185321fa966039419528ee507c9b0538f69901fe3568e08
schema:       070
admitted UUID 00000000-0000-0000-0000-000000000001
evidence:     /var/tmp/neo-chat-memory-v2-single-user-launch-20260809T162030Z
```

Migration `070` had already been applied exactly once under its separate live
authority. The migration changed no persistent table count. Before smoke, only
`memory-worker` and `backend` were recreated with the reviewed image and the
three reviewed rollout gates enabled.

## Admitted result

The single application Chat POST completed with HTTP `200` and curl status
zero. The stream reached `message.completed`; both the Tool step and final
generation completed. The database recorded a completed assistant message, a
completed `OK/applied/NONE` hybrid observation with one released final Memory,
and one Usage row. The cumulative stream Usage was 287 tokens.

```text
conversation: d4aae40d-4e8f-4059-9460-27c33d920b90
assistant:    b036f2b6-4afc-4219-b70f-1c323d76e1fa
run:          08e012ee-591e-4173-8529-c1b58b13ecec
```

The earlier disposable non-admitted proof returned HTTP `401`, wrote zero new
hybrid observations, and emitted zero Provider/Memory work log lines. It used
no live Provider request.

## Cleanup and health blocker

The successful smoke conversation was deleted through the API with HTTP `204`.
PostgreSQL confirms `status=deleted` and a non-null `deleted_at`. No Chat or
Provider request was replayed after cleanup.

The required authenticated health check returned:

```text
status=degraded
reasonCode=memory_index_failed
workerAvailable=true
embeddingWorkerAvailable=true
readyCount=1
pendingCount=5
failedCount=2
```

Content-free SQL proved that this is not projection-join duplication:

```text
current eligible Memory       1
current matching projection   ready=1
projection pending/failed     0/0
capture pending               review_expire=5, all scheduled in the future
capture dead letter           extract=2
dead-letter reason codes      EXTRACTION_INVALID=1, SOURCE_DRIFT=1
```

The API intentionally sums capture pending/processing with projection pending,
and capture dead letters with projection failures. One old
`EXTRACTION_INVALID` source remains active; the newer `SOURCE_DRIFT` source
conversation is deleted. Migration `070` has no acknowledgement/resolution
surface, so mutating or deleting these records would destroy evidence rather
than satisfy the reviewed health contract.

## Fail-closed rollback

Because health was genuinely degraded under the current contract, the launch
gate failed even though the admitted retrieval itself succeeded. The prepared
behavior rollback was applied atomically without touching schema or data. Only
`memory-worker` and `backend` were recreated.

```text
MEMORY_HYBRID_SHADOW_ENABLED=false
MEMORY_TOOL_LOOP_ENABLED=false
MEMORY_TOOL_LOOP_CANARY_USER_IDS=
backend health=healthy
memory-worker health=healthy
schema/user_memories/users=70/2/1
GET /v1/memory-health=disabled/memory_disabled
```

The reviewed image remains deployed, but Memory v2 is not launched. The
successful smoke authority is consumed and must not be replayed. A future
attempt requires a separately reviewed health remediation and live rollout
authority; migration `070`, schema-v24, schema-v25, and this smoke must not be
rerun.

## Offline migration-071 remediation

The owner selected an evidence-preserving successor rather than deleting or
rewriting queue history. Migration `071` is implemented offline with these
boundaries:

- capture health counts only `extract`; the five future `review_expire` jobs
  remain stored but no longer masquerade as capture indexing;
- each historical extract failure remains degraded until one exact-job,
  exact-error, owner-bound acknowledgement is appended;
- `SOURCE_DRIFT` may use `source_no_longer_current` only while the same-owner
  source Conversation exists and is non-active;
- other reviewed dead letters use `historical_failure_accepted` only after a
  24-hour terminal age;
- the original job/error/audit/Activity evidence is unchanged, runtime roles
  have no table CRUD, and the Worker cannot acknowledge;
- `071` down is allowed only before acknowledgement evidence exists.

A disposable PostgreSQL 17 test passed clean `070 -> 071 -> 070 -> 071`, both
valid resolution paths, same-input idempotence, cross-user/error/status/age/
source rejection, append-only denial, least privilege, and post-resolution
rollback refusal. This is offline evidence only. Live migration `071` and each
of the two exact live acknowledgements require separate authorization. They do
not authorize another Provider smoke.

Focused race tests passed for admin, usermemory, and migration packages, and
the exact PostgreSQL 17 replay also passed under the race detector. The full
standalone gate passed Frontend `964/964`, every Backend package plus vet, and
RAG `1906 passed / 7 skipped`. Static security/quality scans found no issue.

The packaged but undeployed successor is:

```text
image:    mm-chat/backend:memory-v2-v25-schema071-health-resolution-candidate-20260810t025553z
image ID: sha256:7f54f00a930da27d5671d7f23b7ebcf9615f62198266886c151b6bc1a6a7be0b
evidence: /var/tmp/neo-chat-memory-v2-071-offline-20260810T025553Z
```

An image-package smoke proved the API, migrator, admin, and Worker binaries are
present and executable; the migrator embeds `071` and the admin binary exposes
the bounded acknowledgement command. The live containers still run the prior
policy-fix image at schema `070`; no deployment occurred.

The exact image then passed an end-to-end isolated PostgreSQL 17 exercise on a
private Docker network: fresh `001 -> 071`, clean `071 -> 070 -> 071`, one
synthetic `EXTRACTION_INVALID` admin acknowledgement with `created=true`,
same-input replay with `created=false`, unchanged original error plus zero
remaining capture failures, and guarded-down refusal with schema still `071`.
The disposable database/container/network were destroyed.
