# Agent Runtime G20.5 — Child Agent Depth-1 Delegation

## Goal

Implement the held one-level Child Agent control foundation: durable root-to-child
lineage, server-derived subset snapshots and grants, transactional Parent budget
reservations, launch-time Parent/lease fences, and Child-first cancel/kill/reap
recovery. The slice must reject recursive or widened delegation before enqueue or
Runner launch while keeping all public Agent, Chat, frontend and production
execution wiring disabled.

## Shared baseline

- G20.1 supply-chain, G20.2 Orchestrator, G20.3 Runner and G20.4 Broker
  foundations remain binding. PostgreSQL 17 is the sole durable lineage, lease,
  budget, Kill Switch and recovery authority.
- Root is `depth=0`; Child is exactly `depth=1`. No Child may delegate again.
- `neo-runnerd` remains non-root and credential-free. The current host remains
  `ISOLATION_UNAVAILABLE`; source/fake tests are not promotion evidence.
- Public Agent API, Chat/frontend, startup workers and production Child launch
  stay absent. `/v1/code/executions` remains fail closed.
- Pure-text Skills remain unchanged through G20.8 and are deleted only by G20.9.
- All recommended design choices are pre-authorized. No sub-Agent is used.

## Requirements

### Server-derived Child authority

- Add an isolated `internal/agentdelegation` control package. It loads the
  immutable Parent authority from PostgreSQL and derives Child lineage, Grant,
  Tool Registry and snapshot server-side; caller/model fields are proposals only.
- Root authority registration binds user/Project/Assistant, exact Run/snapshot,
  model, admitted package/runtime, canonical Grant and Registry fingerprints,
  expiration, Tool identities, Egress, Secret refs and budget.
- Child model is exactly Parent-selected for this single-model foundation;
  package/runtime and subject are exact Parent bindings. Child capabilities,
  actions/resources, approval, Egress, Secrets, expiration and budgets may only
  narrow Parent authority.
- Enqueue binds the exact live Parent Attempt generation/owner/token digest.
  Forged Parent, non-root Parent, terminal/expired/reclaimed Parent lease,
  snapshot drift, Kill Switch or replay mismatch rejects before Child Run insert.

### Registry and recursive-delegation denial

- Reuse G20.4 Registry construction. For depth 1, physically remove
  `delegate_task`, `cron_manage`, `grant_manage`, `secret_manage` and
  `runtime_manage` before fingerprinting.
- Reject the same forbidden set when it appears as a canonical capability behind
  another Tool identity, in a forged signed Registry, or in Runner launch input.
- Child registry identities must be a subset of the durable Parent Registry and
  must equal the server-derived fingerprint stored with the Child authority.

### Durable lineage and Parent budget accounting

- Add migration `087` with immutable delegation authority, root/parent/depth
  constraints, exact Parent Attempt bindings, budget accounts/reservations,
  append-only settlement facts and durable reap work.
- Child enqueue and Parent reservation are one transaction under the Parent
  authority lock. Concurrent Children cannot oversubscribe remaining wall,
  model-token, Tool-call or Artifact-byte budget.
- Exact enqueue replay consumes no second reservation. A mismatched replay is
  rejected. Terminal settlement is monotonic/idempotent, cannot exceed the
  reservation and releases only proven unused capacity.
- Database roles remain least-privilege: a narrow delegation control role gets
  SELECT plus exact SECURITY DEFINER functions and no table DML. Public API,
  Runner and Broker roles gain no delegation authority.

### Launch admission, cascade and recovery

- Add a Backend launch-admission check that rebinds Child lineage, frozen
  fingerprints, Registry identities, current Child lease, exact still-live
  Parent Attempt and current Kill Switches before a Runner call.
- Extend Runner launch lineage shape so depth 0/1, root/parent relation and
  forbidden Child Tool names are rejected locally before OCI create. Signed
  request fingerprinting covers lineage.
- Parent cancel/kill first atomically fences all live Child leases and queues
  exact Sandbox reap targets, then invokes an injected credential-free reaper.
  Reap failure remains durable and retryable; it never restores the lease.
- Recovery discovers terminal, reclaimed, expired or Kill-Switched Parents,
  fences their Children, and retries durable reap work. No descendant remains
  accepted after a successful cascade/reconcile.

### Verification and held promotion

- Add focused unit/race tests, migration schema coverage, strict RPC fixtures,
  a disposable PostgreSQL 17 fresh/replay/concurrency/least-privilege/launch-
  fence/cascade/recovery/dump-restore/down-up drill, and an offline G20.5 gate.
- Advance every older PostgreSQL tail drill through migration `087` without
  weakening its original guarded-down assertions.
- Update architecture, contracts, deployment, tracking and Trellis specs.
- Do not change live environment, protected runtime state, Compose, certificates,
  API/Chat/frontend behavior or exact-host release state.

## Acceptance criteria

- [x] Root-to-child lineage is durable and only `0 -> 1`; depth 2 and forged or
      cross-user Parent bindings fail before enqueue.
- [x] Child model/package/runtime/subject/Grant/Egress/Secret/expiry/budget are
      proven subsets of immutable Parent authority in Go and PostgreSQL.
- [x] Forbidden delegation/management Tools and capability aliases are absent
      before Registry fingerprinting; forged Registry and Runner launch reject.
- [x] Concurrent Child reservations cannot oversubscribe any Parent budget;
      exact replay is free and settlement cannot widen remaining authority.
- [x] Stale Parent/Child lease, snapshot/Grant/Registry drift or Kill Switch
      rejects launch admission before Runner/OCI action.
- [x] Parent cancel/kill fences Child leases before reaping; failures persist for
      reconciliation and successful replay leaves no live descendant.
- [x] Migration `087` passes PostgreSQL 17 replay, concurrency, least privilege,
      dump/restore, guarded down and clean down/up; all older tail drills reach
      `087` and return to head.
- [x] Focused race/vet/tests, Phase 0, Runner/Broker/Orchestrator chains and full
      standalone verification pass; protected runtime paths remain untouched.
- [x] Exact-host gate remains expected-nonzero `ISOLATION_UNAVAILABLE`; public
      Runtime/Chat/frontend and legacy text Skills remain unchanged.

## Deliverables

- `mm-chat/backend/internal/agentdelegation/` with README/DESIGN and tests.
- `mm-chat/backend/migrations/087_agent_child_delegation.{up,down}.sql` plus
  schema/PostgreSQL coverage.
- Broker Child subset/alias hardening and Runner launch-lineage validation.
- `mm-chat/scripts/verify-agent-delegation*.sh` and advanced tail drills.
- Agent Runtime schemas, fixtures, docs, tracking and Trellis specs synchronized
  to the held G20.5 foundation.

## Out of scope

- Public Agent/delegation/approval routes, frontend UI, Chat integration,
  startup worker or production Child execution.
- General model switching, multiple packages per Child, autonomous approvals,
  Child Cron/grant/secret/runtime management or recursive delegation.
- Exact-host provisioning, live certificates/Secrets, Compose wiring, live
  canary, release promotion or protected runtime-state edits.
- Cron (G20.6), Draft learning (G20.7), product/shadow execution (G20.8) and
  legacy text-Skill deletion (G20.9).

## Technical approach

Create a separate Backend delegation bounded context that reuses G20.2 durable
Run primitives and G20.4 Grant/Registry types without creating a package cycle.
Migration `087` stores immutable normalized Parent/Child authority and performs
the final subset, lease, Kill Switch and budget-reservation checks in the same
transaction as Child enqueue. Runner sees only a signed lineage/fingerprint
projection. Cancel/kill creates durable reap work before any host action.

## Decision (ADR-lite)

**Context:** Client/model-derived Child snapshots, process-local budgets or a
Runner-owned Parent check permit authority widening, concurrent oversubscription
and stale-Parent launch. Importing Broker into Orchestrator also creates an
undesirable dependency cycle in existing package tests.

**Decision:** Use `agentdelegation` as the coordinating bounded context, keep
PostgreSQL as the final transactional authority, reuse Broker Grant/Registry
logic, and keep Runner as a signed local shape/fence plus reaping endpoint.

**Consequences:** The security boundary is replayable and independently testable,
with production activation still held. Root authority must be explicitly
registered by the future private control worker before delegation; that wiring
belongs to G20.8, not this slice.

## Research references

- [`research/current-delegation-seams.md`](research/current-delegation-seams.md)
- [`research/depth-budget-and-cascade-authority.md`](research/depth-budget-and-cascade-authority.md)
