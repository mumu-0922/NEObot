# Live schema-v25 single-user bounded-miss Validation result

## Immutable authority and result

The owner granted one fresh exact live schema-v25 single-user bounded-miss
Validation authority. It was consumed exactly once from
`2026-08-09T16:07:42Z` through `16:12:52Z`; no rerun occurred.

```text
run:        memory-regression-20260809t160732z-c5e0da74
capture:    7ec205c3-9c65-4684-8063-e52e35d4c551
cases:      100/100
passed:     true
report:     ead77fd8f505813ec7bae1f0b8cf934c6798797afb9760c15b5a2017adbc2be3
manifest:   d97f3c4f1ee865eaf47e9ca188df4875c7266e07ab562d3687d9f9699c521bce
evidence:   /var/tmp/neo-chat-single-user-bounded-miss-v25-live-20260809T160731Z
```

The report remains aggregate-only and non-promotional. Its
`owner_review_no_automatic_release` action preserves the separation between
evaluation and rollout; the owner's earlier pass-then-launch decision is the
separate rollout intent.

## Quality and telemetry

- Candidate Recall@20, Final Recall@5, current-fact accuracy, NDCG@5, and MRR@5
  were all `1.0`.
- False injection was `0/45`; every required slice and every safety/privacy
  gate passed.
- Routes reconciled at `35/10/55/0` empty/guard/Judge-completed/failed, with
  zero final Judge abstentions.
- The Judge made `63` attempts, including `6` recovered transport retries,
  `3` confirmation attempts, and `1` recovered confirmation retry.
- Request/input/output upper bounds were `63/112543/8064`, below the frozen
  `900/900000/115200` authority.
- Overall diagnostic p95/p99 latency was `8130/11157 ms`.

Both credential copies and every scoped container, network, and volume were
destroyed. The two retained artifacts are mode `0600` under a mode-`0700`
private root.

## Launch preflight blocker

The live database has exactly one active user,
`00000000-0000-0000-0000-000000000001`, and the fixed Provider tuple remains
attested. The database is at migration `069`, not `070`. The reviewed candidate
backend requires migration `070` for fail-closed Memory health and Worker
heartbeats, while the launch acceptance contract permits no implicit schema
change.

Fresh PostgreSQL/MinIO backups and the live environment/container/count
snapshot are retained under
`/var/tmp/neo-chat-memory-v2-single-user-launch-20260809T162030Z` and backup set
`memory-v2-launch-20260809T162030Z`. An isolated restore proved
`069 -> 070 -> 069 -> 070` with every pre-existing table count unchanged and
complete teardown. The candidate backend is
`mm-chat/backend:memory-v2-v25-candidate-20260809t162030z`, image ID
`sha256:5171f2e1055edde6381231e1c50f1423f8714bb9862367135c90a29829d18a63`.

No live migration, environment change, service recreation, or smoke occurred.
Both Memory flags remain false and the canary remains empty. Launch requires
explicit expanded authority for live migration `070` before the existing
exact-UUID rollout plan may resume.

## Authorized migration and rolled-back launch result

The owner later granted the exact live migration-`070` authority. Migration
`070` was applied once, its function/grant checks passed, and all persistent
table counts remained unchanged. It must not be rerun. The reviewed backend was
subsequently replaced by the exact-UUID parser and production-policy admission
fix image:

```text
mm-chat/backend:memory-v2-v25-exact-uuid-policy-fix-candidate-20260810t011729z
sha256:c79fd467421342c63185321fa966039419528ee507c9b0538f69901fe3568e08
```

One separately authorized admitted smoke using `provider.source=server-default`
completed with HTTP `200`, a completed Memory Tool step, one final Memory,
one Usage row, and `message.completed`. The smoke authority is consumed and
will not be replayed. The conversation was soft-deleted with HTTP `204`.

The post-smoke health gate returned `degraded/memory_index_failed` despite one
ready current projection and live extraction/embedding workers. Content-free
SQL decomposed the aggregate into five future `review_expire` jobs and two
historical extract dead letters, with no pending or failed current projection.
This matches the current migration-`070` and Go aggregation semantics; it is
not a projection join/count defect.

The prepared behavior rollback therefore restored both Memory flags to false
and cleared the exact-UUID canary, recreating only `memory-worker` and
`backend`. Both services are healthy, schema remains `070`, and
`user_memories/users` remain `2/1`. The passing schema-v25 evidence remains
valid, but Memory v2 is not launched. Detailed content-free evidence is in
[`live-exact-uuid-policy-fix-smoke-result.md`](live-exact-uuid-policy-fix-smoke-result.md).
