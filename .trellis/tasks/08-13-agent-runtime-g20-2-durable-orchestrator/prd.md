# Agent Runtime G20.2 — Durable Orchestrator Foundation
## Goal

Implement the Neo-owned durable Agent control-plane foundation: immutable Run
snapshots, PostgreSQL Run/Step/Attempt projections, append-only events,
generation-fenced leases, idempotent enqueue, restart recovery, projection
rebuild, hierarchical Kill Switch authority and bounded terminal retention.
The slice must be independently testable while remaining impossible to launch
a Runner, Sandbox, Tool or side effect.

## Shared Baseline

- PostgreSQL 17 is the only state, sequence, lease and Kill Switch authority.
- Redis is optional ID-only notification infrastructure and is not introduced
  in this group.
- Assistant, Skill and Tool stores remain separate. G20.1 installation or
  `allowed-tools` metadata grants no capability.
- Run snapshots are canonical/fingerprinted and immutable. Later policy,
  package, model or grant changes never rewrite an existing snapshot.
- Every legal mutation appends a sanitized per-Run event in the same database
  transaction as its projection update.
- Current pure-text Skills stay unchanged through G20.8 and are deleted without
  migration/wrapping only in G20.9.
- `/v1/code/executions` remains `CODE_EXECUTION_UNAVAILABLE`.
- User confirmed all recommended choices and authorized direct verified
  commits without another confirmation prompt.

## Requirements

### Durable model

- Add migration `084` with:
  - immutable `agent_run_snapshots`;
  - owner-bound `agent_runs` current projection and idempotency authority;
  - ordered `agent_steps` plus monotonic generation;
  - exact generation-bound `agent_attempts` and live lease credentials;
  - per-Run sequenced append-only `agent_run_events`;
  - immutable/revisioned `agent_kill_switches` and a durable switch epoch.
- Use composite ownership/lineage foreign keys and uniqueness so a Step,
  Attempt or event cannot be attached across user/Run boundaries.
- Store only bounded canonical JSON. Event detail rejects disallowed/raw-content
  keys and remains low-cardinality diagnostics.
- Runtime roles receive function execution/read capabilities only and no direct
  insert/update/delete on control-plane tables. Owner roles are restricted
  `NOLOGIN` roles without superuser/createdb/createrole/replication/bypass-RLS.
- Down migration refuses while any Run/snapshot/event/Kill-Switch authority
  exists.

### State machine and terminal fencing

Implement the exact Phase 0 legal edges:

```text
Run: pending -> admitted -> queued -> running
     pending|admitted|queued -> canceled|killed|failed
     running -> succeeded|failed|canceled|killed|outcome_unknown

Step: pending -> ready -> running
      pending|ready -> skipped|canceled|killed|failed
      running -> succeeded|failed|canceled|killed|outcome_unknown

Attempt: leased -> starting -> running -> prepared -> committing
         leased|starting|running|prepared -> failed|canceled|killed|lease_expired
         committing -> succeeded|failed|killed|outcome_unknown
```

- A transition binds expected prior state and, for Attempt mutations, exact
  Attempt/generation/lease credentials.
- Invalid edges return `INVALID_TRANSITION`; expired/reclaimed credentials
  return `LEASE_STALE`; neither creates an event or projection mutation.
- The first valid terminal transition is immutable. Conflicting later
  observations can append one bounded diagnostic fact only.
- Preserve conservative terminal precedence as a reconciliation policy:
  `outcome_unknown > killed > canceled > failed > succeeded`; it never rewrites
  the first terminal fact.

### Enqueue and immutable snapshot

- Provide a typed internal Go service that canonicalizes and fingerprints a
  bounded JSON snapshot and a bounded ordered Step plan.
- Enqueue is owner/idempotency-key bound. Replaying the exact request returns
  the same Run; reusing the key with different snapshot/steps fails with
  `IDEMPOTENCY_CONFLICT`.
- One transaction stores the immutable snapshot, Run/Steps and initial events,
  legally advances the Run to `queued` and Steps to `ready`, and exposes no
  launch path.
- No HTTP route, frontend service, feature flag, worker goroutine, Runner URL or
  command is wired in G20.2.

### Lease, heartbeat, expiry and reclaim

- Lease acquisition locks the exact Run/Step, checks current Kill Switch
  authority and increments Step generation.
- The first lease advances `queued -> running` and `ready -> running`; reclaim
  keeps the Step running, appends predecessor `lease.expired`, then creates a
  distinct Attempt with the next generation and a new opaque token.
- Heartbeat extends only a still-live exact Attempt/generation/owner/token and
  cannot resurrect an expired Attempt.
- Every Attempt progress/terminal mutation requires the same live lease fence.
  A stale Attempt cannot publish progress, output, Artifact, Prepare or Commit;
  G20.2 proves the generic mutation fence without implementing those brokers.

### Restart recovery and projection rebuild

- List nonterminal Runs with current Attempt lease state from PostgreSQL only;
  this is the restart recovery input and works without Redis/process memory.
- Rebuild Run/Step/Attempt status, Step generation and next event sequence from
  the append-only ledger and prove equivalence after controlled projection
  corruption in a disposable database.
- Never infer an external write did not happen. No Prepare/Commit implementation
  exists in this group.

### Hierarchical Kill Switches

- Support revisioned durable scopes: global Runtime, scheduler, runner, source,
  admission, Skill fingerprint, Tool, capability, action, Egress destination,
  Secret reference, Project, user and Run.
- Support `deny_new`, `cancel`, and `kill`; disabling a switch appends a new
  inactive revision with actor/reason/CAS and increments a durable epoch.
- Resolve all matching scopes and return the strongest active mode. A narrow
  inactive revision never overrides an active broad deny.
- `deny_new` rejects acquisition. `cancel`/`kill` also fence heartbeat and
  nonterminal Attempt transitions. Cleanup, audit, recovery, rebuild and
  retention remain callable while switches are active.
- Physical Runner/Sandbox cancellation and kill are explicitly deferred.

### Retention, backup/restore and diagnostics

- Add bounded retention that deletes only terminal Runs older than an explicit
  cutoff, with all dependent snapshot/projection/event rows removed together.
- Structured PostgreSQL backup/restore automatically includes G20.2 authority;
  add a disposable dump/restore drill that compares content-free counts and
  rebuilt projections while Runtime remains unavailable.
- Durable events and errors include only stable IDs, fingerprints, state,
  sequence, generation, bounded actor/reason and allowlisted facts. They exclude
  prompts, Skill/Workspace content, Tool arguments/results, stdout/stderr,
  Artifact bytes, Secret handles/values, bearer/lease tokens and host paths.

## Internal Service Surface

```text
EnqueueRun / GetRun
TransitionRun / TransitionStep / TransitionAttempt
ObserveTerminalConflict
AcquireStep / HeartbeatAttempt
ListRecoveryRuns
RebuildProjection
AppendKillSwitch / ResolveKillSwitch
PruneTerminalRuns
```

The interface is internal to `backend/internal/agentorchestrator`; it is not a
production Runtime API.

## Acceptance Criteria

- [ ] Exact enqueue replay returns one Run; mismatched replay is rejected.
- [ ] Every legal state edge succeeds atomically; every illegal/stale edge has
      no projection or event side effect.
- [ ] Concurrent event/transition attempts produce a gap-free unique sequence
      and only one winning projection transition.
- [ ] Lease heartbeat binds exact credentials; expiry/reclaim creates a higher
      generation and the predecessor can never mutate authority again.
- [ ] Restart recovery uses PostgreSQL alone and identifies live versus expired
      nonterminal Attempts.
- [ ] Event rebuild reproduces Run/Step/Attempt terminal state, Step generation
      and next sequence.
- [ ] Kill Switch revisions/CAS/hierarchy/mode precedence fence new/current
      work without disabling cleanup/rebuild/retention.
- [ ] Runtime roles cannot mutate tables directly or execute owner-only rebuild
      and retention outside their intended function grants.
- [ ] PostgreSQL 17 fresh/replay/non-empty-down refusal/clean down-up plus
      dump/restore and retention gates pass.
- [ ] Focused Go unit/race/vet tests, Backend full tests, frontend/RAG gates and
      full standalone verification pass.
- [ ] No Runner RPC, `neo-runnerd`, Sandbox/OCI launch, Tool Grant, broker,
      side-effect, Child Agent, Cron, learning, Chat/legacy Skill integration or
      `/v1/code/executions` behavior is added.

## Deliverables

- `mm-chat/backend/internal/agentorchestrator/` with types, state machine,
  PostgreSQL repository/service, tests, `README.md` and `DESIGN.md`.
- `mm-chat/backend/migrations/084_agent_orchestrator_foundation.{up,down}.sql`
  and migration/schema/PostgreSQL integration coverage.
- `mm-chat/scripts/verify-agent-orchestrator.sh` and
  `mm-chat/scripts/verify-agent-orchestrator-postgres17.sh`.
- Agent Runtime contract/architecture/deployment/tracking/spec and migration
  documentation synchronized to implemented G20.2 behavior.

## Out of Scope

- Public/authenticated Agent Run HTTP routes or frontend UI.
- Runtime enablement/configuration/startup wiring and Redis hints.
- Runner RPC, `neo-runnerd`, rootless OCI, Workspace/Scratch/Artifact handling.
- Capability Grant resolution, Tool/Egress/Secret brokers, approvals and
  Prepare/Commit effects.
- Child Agents, Cron, Draft learning, Chat execution and legacy Skill deletion.

## Research References

- [`research/process-trace-reuse.md`](research/process-trace-reuse.md)
- [`research/postgres-lease-authority.md`](research/postgres-lease-authority.md)
- [`research/orchestrator-state-and-recovery.md`](research/orchestrator-state-and-recovery.md)
