# Repair Memory Worker and backlog health

## Goal

Restore durable Memory processing and safe recall by aligning the stale Memory
Worker with the currently deployed Backend image, draining recoverable queue
work, and preserving the append-only review boundary for historical failures.

## What I already know

- Backend runs `mm-chat/backend:remove-browser-20260828T220000Z` while Memory
  Worker runs the older `answer-continuation-cf98a2db-20260821T090719Z` image.
- Compose intentionally binds Backend, Memory Worker, migrate, and admin to one
  `BACKEND_IMAGE` release fence.
- The live worker heartbeat is current, but its processing loop emits
  `memory_worker_iteration_failed` every second.
- Current user health reports 63 pending captures, 2 stale processing captures,
  and 35 unresolved historical extract dead letters.
- Historical failures must never be deleted or updated to force green health;
  migration 071 allows one reviewed append-only acknowledgement per command.

## Requirements

- Retain the current Memory Worker image as a rollback artifact.
- Recreate only `memory-worker` with the exact image already running Backend.
- Verify heartbeat, processing-loop logs, queue movement, and memory health after
  replacement.
- Do not modify canonical memories, job payloads, or queue rows directly.
- Do not acknowledge historical failures without the exact operator approval
  required by `mm-chat-admin memory-health-acknowledge`.
- If the aligned worker still fails, stop and diagnose the current Provider or
  database boundary before any backlog mutation.

## Acceptance Criteria

- [x] Backend and Memory Worker run the same immutable local image ID.
- [x] Memory Worker is healthy and no longer emits continuous iteration errors.
- [x] Recoverable pending/stale work begins moving or a specific remaining
      failure is identified without data loss.
- [x] Non-target service container IDs remain unchanged.
- [x] Historical dead letters remain untouched unless separately approved.
- [x] Expired final-attempt jobs whose assistant was deleted terminalize as
      `LEASE_EXPIRED` without a dangling Activity, while the live-assistant
      branch still creates exactly one Activity in PostgreSQL regression tests.

## Definition of Done

- [x] Live runtime state and queue counts are recorded.
- [x] Rollback image remains available.
- The task is archived and the operation is recorded in the Trellis journal.

## Out of Scope

- Deleting or rewriting Memory jobs/resolutions.
- Weakening fail-closed Memory Recall health behavior.
- Bulk historical acknowledgement without per-job review and exact approval.
- Changing Memory extraction, embedding, or relevance algorithms unless the
  aligned current worker proves a source defect.

## Technical Approach

1. Preserve the failed image-alignment attempt and verified rollback evidence.
2. Add a forward migration that retains the dead-letter job as canonical
   evidence, writes Activity only while its assistant owner row still exists,
   and never weakens the Activity foreign key.
3. Prove both PostgreSQL branches: a live assistant still gets one Activity;
   a deleted assistant gets no dangling Activity and cannot block expired-lease
   terminalization or later claims.
4. Build and inspect an immutable Backend runtime image, retain the current
   runtime images, back up PostgreSQL/environment state, and apply only the
   reviewed forward migration.
5. Recreate only Memory Worker with the reviewed image and verify image IDs,
   heartbeat, logs, queue movement, migration head, durable row-count
   monotonicity, and unchanged non-target container IDs.
6. Continue only through documented admin acknowledgement commands if the user
   grants the exact required approval after seeing the reviewed failures.

## Root Cause Found

The stale Worker image was real release drift, but aligning it to the live
Backend image did not restore queue progress. A transaction-scoped runtime-role
probe showed that `memory_worker_claim_job(...)` tries to terminalize two
expired final-attempt jobs. `memory_dead_letter_activity_trigger()` then
unconditionally inserts an Activity linked to an assistant message that has
already been deleted, violates
`message_memory_activities_assistant_owner_fk`, and rolls the whole claim back.
This prevents every later pending job from being claimed.

## Decision (ADR-lite)

**Context:** The live deployment violated the shared Backend/Worker image fence,
and an orphan-safe Activity compatibility defect blocks the queue.

**Decision:** Preserve the failed alignment/rollback evidence, repair the
Activity trigger through an additive forward migration, then deploy one shared
reviewed Backend/Worker image.

**Consequences:** This is reversible and preserves every job. Historical
dead-letter health may remain degraded until separately reviewed and
acknowledged through the append-only operator path.

## Deployment Result

- Migration `109_memory_dead_letter_orphan_activity` is applied; schema head is
  `109`.
- Backend and Memory Worker both run immutable image ID
  `sha256:d661111c451103302d91d24c5a337572a6c77d32f73a02f92a29d8ee8c8bd4ac`.
- Continuous `memory_worker_iteration_failed` logs are gone. The two original
  orphaned leases became `dead_letter/LEASE_EXPIRED` with zero invalid Activity
  links, and later captures resumed processing.
- Capture backlog moved from `43 completed / 37 dead-letter / 63 pending /
  2 processing` to the latest observed `71 completed / 57 dead-letter /
  17 pending / 0 processing` while the bounded retry schedule continues.
- Remaining degraded health is Provider/content failure evidence, not Worker
  unavailability. The two pre-existing append-only health resolutions stayed
  unchanged; no historical failure was acknowledged by this repair.
