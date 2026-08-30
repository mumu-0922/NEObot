# Deploy Skill Cleanup and Verify Backup Restore

## Goal

Deploy the committed Conversation Skill-selection cleanup to the active
single-server Backend, while first creating and rehearsing one matched
PostgreSQL/MinIO `pre-deploy` recovery set. Prove the new live Backend removes
only a temporary Conversation's Skill selection before soft deletion and does
not alter installed Skill inventory or unrelated services.

## Requirements

1. Capture the live Backend container/image, database migration head, bounded
   row-count baseline, current environment hash, and all unrelated service IDs
   before mutation.
2. Preserve the currently running Backend image under its existing retained
   tag and save a mode-`0600` copy of the active environment outside Git for
   exact behavior rollback.
3. Create PostgreSQL and MinIO backups with one shared unique `pre-deploy` set
   identifier using the active source-build Compose topology. Verify both
   checksum sidecars before any service recreation.
4. Restore the PostgreSQL dump into a uniquely named temporary database in the
   same PostgreSQL 17 server. Verify:
   - restored migration `(version, name, checksum)` authority exactly matches
     the live database through head `109`;
   - representative core, Chat, Skill, MCP, File, Knowledge, and Memory tables
     are readable;
   - bounded row counts match the pre-backup baseline;
   - no foreign-key orphan check fails.
5. Export bounded Knowledge, MCP-result, and Skill-package object-key samples
   from the temporary restored database. Restore the paired MinIO archive only
   into a unique temporary drill bucket, `stat` every sample, then remove the
   drill bucket, staging directory, sample files, and temporary database.
6. Build only `mm-chat/backend` from the clean committed source containing
   `ae221181`, explicitly selecting Docker target `runtime`. Tag the candidate
   `mm-chat/backend:skill-cleanup-ae221181` and require image user
   `mmchat:mmchat` plus `CMD ["/usr/local/bin/mm-chat-api"]`.
7. Keep the existing source-build Compose topology. Update only the ignored
   active environment's `BACKEND_IMAGE` and `MM_CHAT_VERSION`, render the
   candidate, and confirm its Backend environment differs from the running
   container only in release identity/image-owned `PATH`.
8. Recreate Backend only with `--no-build --no-deps --force-recreate`. Do not
   recreate Frontend, PostgreSQL, Redis, MinIO, RAG Worker, MCP Runner, Memory
   Worker, or Agent Host.
9. Require Backend health/readiness, frontend same-origin health, unchanged
   schema head and row-count baseline, unchanged unrelated container IDs, and
   no recent Backend `ERROR`, `FATAL`, or panic evidence.
10. Run a Provider-free authenticated regression using one short-lived marked
    Session and one temporary Conversation:
    - bind one already-installed Skill to the temporary Conversation;
    - delete the Conversation through the public same-origin API;
    - verify the Conversation is inaccessible and soft-deleted;
    - verify its Skill selection is absent;
    - verify the installed Skill Library count is unchanged;
    - revoke/delete the exact Session and remove all temporary residue.
11. Record only non-secret release, backup, restore, health, count, and cleanup
    evidence. Never print environment contents, tokens, credentials, Provider
    data, private object keys, or user content.

## Acceptance Criteria

- One checksum-verified matched PostgreSQL/MinIO backup set exists before the
  Backend is recreated.
- Both temporary PostgreSQL and MinIO restores succeed against isolated targets
  and leave no temporary database, bucket, staging, or sample residue.
- Only the Backend container ID changes; every unrelated service ID remains
  byte-identical.
- The live Backend runs the inspected `skill-cleanup-ae221181` candidate and is
  healthy through both direct and same-origin readiness paths.
- Schema head remains `109_memory_dead_letter_orphan_activity`; baseline counts
  do not decrease.
- The Provider-free deletion regression removes the exact temporary Skill
  selection, preserves Library inventory, and leaves zero active Conversation,
  Session, selection, or runtime residue.
- Rollback inputs remain present: retained old image, protected prior env copy,
  and matched backup artifacts.

## Definition of Done

- Restore rehearsal, deployment, health, lifecycle regression, and cleanup are
  all evidenced without secrets.
- No product source change is required unless an operational defect is found.
- Any new operational invariant is synchronized into `.trellis/spec/` and
  product deployment documentation.
- Work is committed, task archived, and journal recorded; nothing is pushed.

## Technical Approach

Use the repository's active source-build release mode rather than the strict
registry-digest production wrapper. The current `.env.single-server` predates
new promotion-only keys and therefore cannot pass
`preflight-single-server.sh`; repairing that configuration would be a separate
production-hardening change. Direct comparison proves the current Compose
render reproduces the running Backend environment except image-owned `PATH`.

Backups use `backup-postgres.sh` and `backup-minio.sh` with the same explicit
set ID. Restore authority is established by comparing the restored database to
the live database, not by the legacy runbook's historical `001..010` manifest
literal. Deployment uses the already-retained old tag for rollback and a new
explicit `runtime` candidate tag. No migration is run because source and live
schema are already at head 109.

## Decision (ADR-lite)

**Context:** The live stack is healthy and source-build based, while the strict
production promotion preflight now requires configuration keys and immutable
registry digests absent from the active environment.

**Decision:** Preserve the active topology, rehearse recovery first, deploy one
explicit local immutable candidate to Backend only, and defer promotion-env
hardening.

**Consequences:** This closes the product defect with minimal blast radius and
an exact local rollback. The deployment remains a local retained-tag release,
not a registry-digest production promotion.

## Out of Scope

- Applying or rolling back migrations.
- Recreating Frontend, RAG, MCP Runner, Memory Worker, storage, or Agent Host.
- Repeating the live Provider smoke.
- Restoring into the production database or production bucket.
- Adding all newly required production-promotion env keys or publishing images.
- Pruning old backup sets or deleting retained rollback images.

## Technical Notes

- Live schema head observed before planning: `109_memory_dead_letter_orphan_activity`.
- Running Backend image is retained as
  `mm-chat/backend:memory-orphan-activity-20260828T234500Z`.
- Running Compose labels name only `compose.single-server.yml` for Backend.
- Active Backend command is `/usr/local/bin/mm-chat-api`; current container is
  healthy and runs as the configured ordinary UID/GID.
- Current Compose render and running Backend environment differ only in `PATH`.
- Strict promotion preflight currently stops at missing `POSTGRES_DATA_DIR` and
  the active env lacks additional newer promotion-only keys; no secret value
  was inspected or recorded.

