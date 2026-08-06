# Disable Memory recall while preserving data

## Goal

Apply the immutable schema-v15 Orange required action by disabling production
Memory recall and related hybrid Provider work while preserving all canonical
Memory, vector, observation, chat, and Usage data.

## Requirements

- Change only the active runtime flags in `mm-chat/.env.single-server`:
  - `MEMORY_TOOL_LOOP_ENABLED=false`
  - `MEMORY_HYBRID_SHADOW_ENABLED=false`
- Create a protected external rollback copy of the pre-change environment file
  before mutation and record its SHA-256 without printing secrets.
- Attempt the repository production preflight, but follow the live Docker
  deployment truth when the current local-image stack is intentionally not a
  registry-image production environment. Validate the exact active
  `compose.yml` topology before restarting anything.
- Recreate only `backend` and `memory-worker` from their existing local image;
  do not build, pull, migrate, or restart unrelated services.
- Verify both containers are healthy and receive the expected disabled flags.
- Create a protected logical dump of all Memory-named PostgreSQL relations and
  verify representative persistent Memory row counts do not decrease across
  the operation.

## Acceptance Criteria

- [x] `MEMORY_TOOL_LOOP_ENABLED=false` reaches the backend.
- [x] `MEMORY_HYBRID_SHADOW_ENABLED=false` reaches backend and Memory Worker.
- [x] Backend and Memory Worker are healthy after recreation.
- [x] PostgreSQL and all unrelated services remain running.
- [x] Persistent Memory row counts sampled before and after do not decrease.
- [x] A restorable custom-format dump of Memory-named PostgreSQL relations is
      retained with mode `0600` and a recorded SHA-256.
- [x] The protected rollback copy exists outside the repository with mode
      `0600` and a recorded SHA-256.
- [x] No migration, deletion, Validation rerun, Holdout, release, or Push occurs.

## Definition of Done

- Active runtime is fail-closed for Memory recall and hybrid Provider work.
- Data-preservation and live-health checks pass.
- Exact rollback procedure and artifact location are reported without exposing
  credentials.

## Technical Approach

The production wrapper was attempted first and rejected the unchanged active
environment because `FRONTEND_IMAGE` is absent. Docker runtime labels prove
that `backend` and `memory-worker` were created from `compose.yml` using the
shared local `mm-chat/backend:local` image, while PostgreSQL was created from
`compose.single-server.yml`. Therefore use the exact `compose.yml` topology for
the two target services. Create a custom-format logical Memory dump, snapshot
aggregate table counts through the existing PostgreSQL container, edit the two
exact flag assignments atomically, render Compose, and use
`up -d --no-build --no-deps --force-recreate` for the two affected services.

## Decision (ADR-lite)

**Context**: schema-v15 Validation is Orange because false injection rate was
`0.09`, above the frozen `0.02` limit, and requires
`disable_memory_recall_preserve_data`.

**Decision**: disable both the API Tool Loop and shared hybrid shadow switch,
while retaining every persistence layer and creating a rollback copy.

**Consequences**: models temporarily cannot recall stored Memory and no new
embedding/hybrid Provider work is claimed. Stored data remains available for
offline diagnosis and a later reviewed re-enable operation.

## Out of Scope

- Deleting, rewriting, or rolling back Memory data or schema migrations.
- Re-running Validation, running Holdout, promoting, or releasing.
- Changing model/provider credentials or application source.
- Re-enabling Memory before the nine false-injection cases are remediated and a
  new authorization is granted.

## Technical Notes

- Authorization was explicit: "授权关闭 Memory recall，保留全部数据".
- Pre-change flags were both `true`; `backend` and `memory-worker` were healthy
  on image ID
  `sha256:c3d74851905f14b1001c32a9f9fde2fe7b291b576a47576fc854360f6696c62d`.
- The first recreate exposed mutable-tag drift: `mm-chat/backend:local` had
  moved to an image that requires migration `070`, while the live database is
  intentionally at `069`. The failed attempt triggered its automatic config
  rollback before any migration or data mutation, but the untagged original
  image was no longer available after container replacement.
- Git timing identifies `5d421e3b` as the last backend commit before the
  original containers started and before migration `070` was introduced. A
  clean Git archive of that backend tree was rebuilt as
  `mm-chat/backend:memory-recall-safe-5d421e3b` and explicitly pinned in the
  active environment. Its Worker binary has no migration-070 heartbeat
  dependency.
- Final backend and Memory Worker image ID:
  `sha256:3b8976a152083ecd37c5067feab6f5a97eed9fbc109386379e0e2985b6b5f0cc`.
- Final verification: both services healthy, `/health` and `/ready` returned
  `200`, recent ERROR/FATAL/panic count was zero, and all 43 sampled Memory
  relations had identical before/after row counts.
- Protected evidence and rollback artifacts:
  `/home/mumu/.local/state/neo-chat/memory-recall-rollback/20260806T020015Z`.
- Runtime guidance: `mm-chat/docs/deployment/release-rollback.md`.
- Runtime state is excluded from Git and must never be staged.
