# Memory v2 provenance and deletion contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`055_memory_provenance_deletion`, canonical Global Memory writes, automatic
evidence, revision history, visibility epochs, targeted tombstones, deletion
manifests, or the Memory Worker's `purge` stage.

PR4 keeps the v1 Global-only reader and HTTP payloads. Semantic Review,
temporal/conflict routing, Project/Conversation auto-routing, hybrid retrieval,
encrypted off-host manifest export/replay, and retention pruning remain later
batches.

## 2. Signatures

Canonical/API capabilities:

```text
memory_upsert_global_manual(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT,
  TEXT[], UUID, UUID, BOOLEAN) RETURNS TABLE (...v1 Memory fields...)
memory_update_global_manual(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT,
  TEXT[], BOOLEAN) RETURNS TABLE (...v1 Memory fields...)
memory_delete_global(UUID, UUID, UUID, UUID, UUID, UUID) RETURNS BOOLEAN
```

Worker capabilities:

```text
memory_worker_hydrate_capture(UUID, UUID, UUID) RETURNS TABLE (...)
memory_worker_apply_capture_candidate(UUID, UUID, UUID, UUID,
  TEXT, TEXT, TEXT, SMALLINT, TEXT[]) RETURNS TABLE (...v1 Memory fields...)
memory_worker_purge_memory(UUID, UUID, UUID) RETURNS BOOLEAN
```

Deletion takes authenticated user ID plus target Memory, event, job, tombstone,
and manifest UUIDs. It does not change the public HTTP request/response shape.

## 3. Contracts

- `user_memories` remains the only canonical plaintext row. It carries a
  monotonic `revision`, live `visibility_epoch`, canonical `content_hash`,
  `authority_kind`, and optional extraction-profile hash.
- Every automatic Memory has at least one `user` evidence row. Evidence stores
  message/Conversation IDs, source hash, role, and observation time only; it
  never copies message text. `assistant_context` cannot be sole authority.
- A canonical change appends exactly one prior snapshot at the new revision.
  Revision rows reject direct UPDATE/DELETE. The one permitted UPDATE clears a
  non-null snapshot and sets `ONLINE_PURGED`; parent/account cascade may delete
  the row after the canonical parent is gone.
- Automatic exact matches never overwrite non-`auto` authority. A tombstoned
  content hash cannot be automatically recreated. An explicit manual action
  may create a new canonical row with the same content without clearing the old
  tombstone.
- Single delete locks user state and target Memory, immediately sets
  `deleted_at`/disabled, appends a delete revision, targeted tombstone,
  ID/hash-only manifest, `memory.deleted` outbox event, and `purge` job in one
  transaction.
- `purge` has nullable source/provider job fields and non-null target Memory /
  tombstone IDs. Go claim scans must therefore use `sql.Null*` for all
  extract-only columns.
- Stage dispatch happens before Provider hydration. `purge` must never resolve
  a Provider. It rechecks live lease, user, epoch, target, scope generation,
  tombstone, and deletion state, then idempotently clears canonical content,
  normalized content, tags, source references, extraction profile, evidence,
  and revision snapshots.
- Purge rows use `max_attempts=128`; with the bounded 15-minute worker backoff,
  cleanup keeps retry authority beyond the 24-hour online deletion SLA. IDs,
  hashes, tombstone, manifest, operation, time, and result remain.
- The database manifest is not encrypted or signed by migration `055`.
  Authenticated encrypted off-host export, restore replay, and physical backup
  retention belong to PR10; never label an unkeyed SHA-256 as a signature.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Existing AI row lacks a surviving same-user `role=user` source | Backfill disables it; do not infer authority. |
| Cross-user Memory/message/Conversation evidence | Composite FK rejects it. |
| Direct revision mutation/delete while parent exists | `MEMORY_REVISION_APPEND_ONLY`. |
| Manual exact row meets automatic candidate | Return manual row unchanged; no automatic overwrite. |
| Apply epoch differs from live user state | `MEMORY_VISIBILITY_EPOCH_DRIFT`, terminal. |
| Deleted content hash is proposed automatically | `MEMORY_CAPTURE_CANDIDATE_TOMBSTONED`, terminal. |
| Deleted source reaches hydration | `MEMORY_CAPTURE_SOURCE_TOMBSTONED`, terminal. |
| Purge lease/epoch/target/tombstone drifts | Reject without plaintext mutation. |
| Purge is replayed under the same live lease | Return success and keep already-wiped state unchanged. |
| Delete commits | v1 List/Recall immediately returns zero for the target. |
| `memory_worker_runtime` attempts table access | Permission denied; functions only. |
| Down sees provenance/delete history, non-default state, AI rows, or purge work | Fail closed with a `MEMORY_PROVENANCE_ROLLBACK_*` code. |

## 5. Good / Base / Bad Cases

- **Good**: an extract response returns after the user deletes the matching
  Memory. Apply rechecks the tombstone and dead-letters; purge wipes plaintext
  without loading a Provider; a later explicit manual rebuild gets a new row.
- **Base**: a manual Global Memory is created and edited once. The canonical row
  advances to revision two and exactly one prior snapshot exists; v1 HTTP output
  is unchanged.
- **Bad**: make purge fields look like extract fields, scan nullable columns into
  Go strings directly, let the worker clear tombstones, physically delete
  immutable backup dumps, call a Provider from purge, or drop `055` after a
  manifest/tombstone exists.

## 6. Tests Required

- Static migration tests assert canonical columns, ID/hash-only evidence,
  append-only revision trigger, stage-specific job shape, 128-attempt purge,
  narrow grants, no-resurrection checks, and every down guard.
- Disposable PostgreSQL 17 must prove `054 -> 055` backfill, cross-user evidence
  rejection, manual precedence, one-snapshot revision, forbidden history
  mutation, delete immediate invisibility, old-response rejection, provider-free
  purge, idempotent wipe, manifest result, stale epoch rejection, manual rebuild,
  guarded down, clean down, and re-up.
- Go tests must prove nullable purge claims, stage dispatch before hydration,
  tombstone errors after Provider return become terminal, and existing v1 CRUD
  HTTP behavior remains compatible.
- Run focused race tests, `go test ./...`, `go vet ./...`, Compose/preflight,
  backend image build, and the full standalone gate. Offline tests never call a
  live Provider.

## 7. Wrong vs Correct

### Wrong

```go
capture, _ := repo.Hydrate(ctx, job) // called before stage dispatch
if job.Stage == "purge" {
    return repo.Purge(ctx, job)
}
```

This loads deleted source/provider state for a job that must be provider-free.

### Correct

```go
switch job.Stage {
case "purge":
    return repo.Purge(ctx, job)
case "extract":
    capture, err := repo.Hydrate(ctx, job)
    // provider-backed extraction follows
}
```

The SQL capability still rechecks lease, user, epoch, target, and tombstone;
Go stage dispatch alone is not deletion authority.
