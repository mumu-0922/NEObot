# Recover Memory runtime and diagnose v20 abstentions

## Goal

Restore the stopped backend and Memory Worker without changing PostgreSQL,
Memory flags, or reader authority, then create a separately versioned,
Development-only diagnostic for the six Luna abstentions that caused the
schema-v20 `stable_fact` and `temporal_correction` slice failures.

## What I already know

- The working tree was clean when this task was created.
- The schema-v20 Fake completed 300/300 and passed with zero network and zero
  scoped residue.
- The sole schema-v20 live Development completed 300/300 but failed:
  Candidate Recall@20 `1.0`, Final Recall@5 `0.9692307692`, current-fact
  accuracy `0.9636363636`, false injection `0/135`, and six Luna abstentions.
- `stable_fact` and `temporal_correction` each scored `0.9333333333` against an
  unchanged `0.95` threshold. All eleven transient Judge transport failures
  recovered and there were no terminal failures.
- Schema-v21 remains reserved for Validation and is unreachable until a fresh
  full Development successor passes. Validation/Holdout must not be read or
  run during this task.
- Live PostgreSQL is healthy and was not restored or changed by the previous
  task. The backend and Memory Worker are stopped with exit `127` after Docker
  Desktop lost the old WSL bind-mount handle for the mode-`0600` Provider
  keyring. The source keyring file still exists.
- Both stopped containers reference the same missing historical image ID
  `sha256:3b8976...`. Their configured tag now resolves to the retained image
  `sha256:501a8568...`; offline extraction proved its API and Worker binaries
  byte-identical to the stopped containers.
- Runtime Memory flags must stay false and the canary allowlist must stay
  empty. All Memory data must be preserved.

## Assumptions (temporary)

- The already configured retained image is an acceptable recovery candidate
  because both service binaries match the stopped containers byte-for-byte;
  database/config/runtime gates still must pass before recreation.
- Runtime recovery and Development diagnostics can share one task only if a
  recovery failure stops before Provider work.
- The next diagnostic should retain only opaque case/stage identities and
  aggregate evidence, never synthetic query/Memory plaintext or Provider
  bodies.

## Requirements (evolving)

- Preserve the live PostgreSQL container, volume, migration version, relation
  counts, and all Memory data.
- Because the live `neo_chat` database was discovered to contain zero public
  tables, preserve an exclusive mode-`0600` empty-state dump and restore the
  already isolated-PostgreSQL-17-verified pre-066 backup before service
  recreation. Validate archive hash/catalog, migration authority, bounded
  relation counts, roles, and backend compatibility before running only the
  explicitly authorized migrations `067`–`069`.
- The verified backup contains migration `066`, while the retained runtime
  image embeds through `069` and the logical dump intentionally omits ACLs.
  Rehearse a schema-first recovery in an isolated PostgreSQL 17 instance:
  migrate an empty database to `069`, truncate its data while retaining
  owners/ACLs, restore the v66 data-only payload, then apply only `067`–`069`.
  Only an exact passing rehearsal may be replayed into a new live recovery
  database and atomically selected while retaining the current empty database.
- Recreate only `backend` and `memory-worker`; do not build, pull, migrate,
  restart PostgreSQL, or enable Memory.
- Record before/after container IDs, image IDs, health, runtime flags, and
  unrelated-container IDs. Retain an exact rollback image/reference.
- Stop before recreation if the candidate image cannot be derived from and
  verified against the stopped live container or another explicitly approved
  source.
- Preserve every schema-v15 through schema-v20 artifact and consumed-run
  authority. Do not reinterpret or rerun schema-v20.
- Add a fresh Development-only diagnostic over exactly 57 cases in the union
  of the two failed slices (`30 + 30 - 3`), repeated three times for 171
  executions, with deterministic selection and enough opaque stage evidence
  to identify the six Luna abstention patterns as systematic or stochastic.
- Run network-free Fake lifecycle and cleanup proof before any authorized live
  Provider request.
- Make no semantic prompt/BGE/final-rank change in the diagnostic phase.
- Any subsequent repair requires a separately versioned full 300-case
  Development successor; schema-v21 Validation remains blocked until that
  successor passes every unchanged gate.

## Acceptance Criteria (evolving)

- [x] Backend and Memory Worker become healthy on one verified recovery image;
      PostgreSQL and unrelated container IDs remain unchanged.
- [x] Memory runtime flags remain false, canary remains empty, the explicitly
      authorized recovery remains at migration `069`, post-recovery Memory
      relation counts remain unchanged, and the prior empty database rollback
      is retained.
- [x] Diagnostic selection contains only Development cases in the union of
      `stable_fact` and `temporal_correction`, with no Validation/Holdout
      admission.
- [x] Fake completes the exact diagnostic plan with zero network, credentials,
      or scoped Docker residue.
- [x] At most one complete live diagnostic is consumed with reconciled
      attempts, retries, tokens, cost, permissions, privacy, and cleanup.
- [x] Evidence identifies an actionable Luna abstention pattern without
      retaining plaintext, raw Provider output, credentials, URLs, or scores.
- [x] Schema-v21 Validation and canary remain unreachable throughout this
      task.
- [x] Focused tests, race tests, backend tests/vet, script lifecycle tests,
      Compose renders, full standalone verification, docs/spec sync, and
      secret/security scans pass.

## Definition of Done

- Runtime services are safely restored or the task stops with a precise,
  non-destructive recovery blocker.
- One new diagnostic authority is implemented, Fake-proven, and—only after
  clean recovery and explicit live gates—run once online.
- The result is retained as immutable diagnostic evidence and selects no
  reader, Validation, canary, migration, or Memory data mutation.

## Decision (ADR-lite)

**Context**: Docker Desktop invalidated the stopped services' old bind-mount
handle and the historical image ID disappeared, while a retained image remains
available under the configured tag.

**Decision**: Use the retained image only after the API and Worker binaries
have matched the stopped containers byte-for-byte. Capture pre-state, render
Compose, recreate only `backend` and `memory-worker` with no build/pull/deps,
then require health and unchanged database/runtime authority before beginning
the 171-execution diagnostic lifecycle.

**Consequences**: The two service container IDs will change, while PostgreSQL
and unrelated services must not. Any recovery drift stops the task before
credentials or Provider work. The diagnostic remains non-promotional and
schema-v21 remains reserved.

## Out of Scope

- Restoring any dump other than the exact verified pre-066 artifact, running
  migrations other than the exact authorized `067`–`069` sequence, deleting
  Memory data, enabling L1/L2/L3 readers, or changing canary state.
- Rerunning schema-v20, constructing schema-v21, inspecting Holdout, weakening
  quality thresholds, or selecting the best of repeated live results.
- L2 Scene or L3 Persona promotion.

## Technical Notes

- Previous evidence:
  `.trellis/tasks/archive/2026-08/08-07-memory-validation-slice-diagnostic/research/schema-v20-result.md`.
- Runtime recovery follows
  `.trellis/spec/operations/runtime-recreate-image-pinning.md`.
- Diagnostic and reader contracts follow
  `.trellis/spec/backend/memory-v2-benchmark.md` and
  `.trellis/spec/backend/memory-v2-hybrid-shadow.md`.
- [`research/runtime-and-diagnostic-boundary.md`](research/runtime-and-diagnostic-boundary.md)
  records the byte-level recovery proof and exact Development-only slice plan.

## Confirmation

The owner selected option `A`: restore the two services with the byte-verified
retained image, then automatically proceed through Fake and the authorized
online diagnostic if every recovery and cleanup gate passes.
After the live database was proven empty, the owner separately selected option
`A` authorizing restoration of the already verified pre-066 backup. The
current empty database must first be retained as rollback evidence; this does
not authorize migrations, Memory enablement, or any other data source.
The backup then proved to contain schema v66 without ACL/login authority while
the retained image requires v69. The owner selected option `A` again,
authorizing an isolated schema-first v66-to-v69 rehearsal and, only after every
gate passes, the identical live recovery into a new database. This supersedes
the earlier no-migration assumption only for exact migrations 67 through 69;
it authorizes no later migration.
