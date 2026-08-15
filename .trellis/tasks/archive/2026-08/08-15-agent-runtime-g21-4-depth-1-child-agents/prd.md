# Agent Runtime G21.4 — Depth-1 Child Agent canary

## Goal

Add the first independently activated production-path Child Agent canary: one
synthetic depth-zero Parent launches exactly one synthetic depth-one Child,
proves strict authority/budget subsets, physically removes delegation from the
Child, then performs durable Child-first cancel/reap and restart recovery. Do
not enable public/user delegation, depth two, generic Agent Runtime, package-
selected child work, Broker effects, Cron, learning or production promotion on
this development host.

## Shared baseline

- G20.1-G20.10 and G21.0-G21.3 remain binding. Legacy text Skills stay deleted;
  only admitted Package Skills may eventually execute.
- The user approved all recommended decisions, direct verified commits and no
  ordinary follow-up preference questions. No sub-Agent delegation is used to
  implement this task.
- The current development host remains `ISOLATION_UNAVAILABLE`; do not install
  host dependencies, provision accounts/cgroups/systemd, or create live
  credentials/evidence.
- Never read or modify `.env.single-server`, `data/`, `secrets/` or `backup/`.
- Checked-in evidence is synthetic/template-only and documentation is English.

## Requirements

### Independent staged activation

- Add strict `child_agent_canary` activation evidence, standalone command and
  default-off Compose profile. Require current G21.0-G21.3 readiness.
- Inside the dedicated profile only, require
  `AGENT_CHILD_CANARY_ENABLED=true` and `AGENT_DELEGATION_ENABLED=true` while
  broad Runtime, Broker read/mutation, MCP write, Egress, Secrets, Scheduler,
  Skill install and Learning remain false.
- Add caller `spiffe://neo-chat/agent-runtime-child-canary`. Runner grants it
  lifecycle methods only and no Prepare/Commit relay. Preserve all four prior
  callers and both relay routes byte-for-byte.
- Add a tenth LOGIN recursively inheriting exactly
  `agent_orchestrator_runtime`, `agent_runner_control` and
  `agent_delegation_control`, with no elevated attributes, owner membership,
  provision authority or direct table DML.

### Exact synthetic Parent/Child plan

- Add strict `neo.agent-child-run-canary-plan/v1` schema and plan loader for
  exactly one Parent and one Child. Bind one synthetic user/subject/model,
  package/runtime, grant window, stable Root/Child idempotency keys, Sandboxes,
  argv, leases and four-dimensional budgets.
- Parent has exactly `delegate_task/create` for one synthetic child resource.
  Child preserves subject/model/package/runtime, has a strict Grant/budget/
  expiry subset, no Egress/Secrets and no effect execution.
- Child deliberately requests `delegate_task`; server derivation must
  physically remove it before Registry fingerprinting, leaving an empty
  depth-one Registry. Alias/capability rebinding also fails.
- Parent and Child Sandboxes remain rootless, read-only, capability-free,
  `networkMode=none`, credential-free and bounded. User/package arguments are
  ineligible.

### Durable execution and depth fences

- Enqueue/acquire/launch/heartbeat the Parent through Orchestrator and Runner,
  then register its exact delegation authority against the durable Root Run.
- Enqueue the Child only from the exact live Parent Attempt. Recheck user,
  Parent, model, package/runtime, snapshot, Grant/Registry, generation, lease,
  Kill Switch and budget in Go and PostgreSQL before reservation/launch.
- Admit and launch one depth-one Child with exact lineage
  `rootRunId=parentRunId=<Parent>` and the derived empty Registry.
- Depth two, forged Parent, second Child, widened subject/model/package/runtime,
  Grant/Registry/action/selector/approval/Egress/Secret/expiry/budget and stale
  Parent/Child attempts fail before Runner launch.
- Keep Parent reservation and Child terminal usage/cascade facts durable and
  replay-safe. Never create a replacement Child under a new idempotency key.

### Production Child reap transport

- Add migration `093_agent_child_canary_reap_transport`; do not rewrite
  migration `087`.
- Add function-only pending reap inventory that returns exact Step/Attempt,
  optional Runner Sandbox projection and the latest matching launch-authority
  expiry without exposing lease tokens.
- Replace reap completion so success atomically terminalizes the exact Runner
  Sandbox projection and durable reap; failure remains retryable. Runtime roles
  keep no direct Runner/delegation DML.
- Wait through the bounded late-launch authority window, list/reconcile the
  exact Child out of Runner inventory while retaining the Parent, verify Child
  absence, then complete the durable reap.
- Guard migration down until no pending/failed reap or live depth-one Sandbox
  requires the new bridge; clean down restores migration-087 behavior.

### Child-first cancellation and restart

- Cascade Child Attempts/Steps/Runs first, then exact Runner reap, durable reap
  completion, signed Parent cancel and atomic Parent Sandbox/Attempt/Step/Run
  terminalization.
- On restart, reconcile expired/terminal/Kill-Switched Parents and failed reaps
  before new work. Existing lineage forbids another Child.
- Lost Parent/Child lease tokens wait for expiry. A Parent generation may be
  reacquired only to finish Parent cleanup after its sole Child is terminal.
- Prove crash points before/after Child enqueue, launch authority, launch,
  cascade, Runner reap, durable reap completion and Parent cancel. Restart
  never produces a second Child or leaves a descendant.
- Health requires terminal lineage, zero pending reaps and empty Runner
  inventory.

### Gates, migration head and documentation

- Add activation/plan schemas, valid/invalid fixtures, focused Go race/vet,
  PostgreSQL 17, Compose, enabled-preflight, restart/late-launch and credential-
  absence gates.
- Advance production policy and every guarded older migration-tail drill from
  `092` to `093`, preserving peel/reapply and final-head behavior.
- Integrate G21.4 into Phase 0 and full standalone while retaining G21.0-G21.3
  regressions and expected current-host `ISOLATION_UNAVAILABLE`.
- Synchronize architecture, contracts, deployment, backup/restore, G20/G21
  tracking, process record, migration docs and Trellis backend/operations specs.

## Acceptance criteria

- [x] The fifth Runner caller has lifecycle-only authority, no relay or
      Prepare/Commit; every prior caller/relay policy remains unchanged.
- [x] Exactly one synthetic Parent and one depth-one Child run with strict
      subject/model/package/runtime/Grant/Registry/expiry/budget subsets.
- [x] The Child Registry is physically empty after requesting
      `delegate_task`; depth two and every widening/alias/stale case fail before
      Runner launch.
- [x] Migration `093` proves function-only exact reap inventory, late-launch
      expiry fencing, replay-safe atomic Runner/delegation completion, least
      privilege, dump/restore and guarded clean down/up on PostgreSQL 17.
- [x] Child-first cascade/reap completes before Parent cancel, and crash/restart
      tests prove zero second Child, zero pending reap and zero Runner residue.
- [x] Compose/example/preflight are default-off, require current G21.0-G21.3,
      a distinct tenth principal/caller/key set and keep all controller
      credentials out of Runner and Sandboxes.
- [x] G21.4 plus G21.0-G21.3 regressions, Phase 0 and full standalone pass; the
      current host still reports `ISOLATION_UNAVAILABLE` and protected runtime
      paths remain untouched.

## Definition of done

- Source, migration, tests, fixtures, Compose/example configuration, docs and
  Trellis specs agree on the one-Parent/one-Child, depth-one-only boundary.
- Verification passes, then work/task/journal commits are created without
  amend or push.

## Technical approach

Create `agentchildcanary` plus `agent-runtime-child-canary`. Reuse the durable
Orchestrator, Runner authority/client and G20.5 delegation contracts. Add a
strict plan resolver that constructs the Root Grant/Registry after the stable
Root Run exists and lets `agentdelegation` derive the sole Child snapshot,
Grant, Registry and reservation.

Migration `093` closes the G20.5 held transport gap with a narrowly exposed
reap inventory and atomic completion bridge. The controller waits out signed
late launches, reconciles only the exact Child Attempt, then cancels the Parent.
Stable Root/Child idempotency and existing-lineage checks make restart cleanup
monotonic: it may finish reaping or Parent cleanup, never redelegate.

## Decision (ADR-lite)

**Context:** Migration `087` owns durable cascade facts but intentionally left
Runner reaping behind an interface. After cascade terminalizes a Child Attempt,
the ordinary live-attempt recovery query cannot recover its exact Sandbox, and
an already signed launch may still arrive during the authority TTL.

**Decision:** Add migration `093` with function-only reap inventory and atomic
Runner/delegation completion, wait through the launch-authority expiry, and
activate one separate lifecycle-only synthetic Child canary with a tenth LOGIN.

**Consequences:** G21.4 proves depth and restart cleanup without enabling Child
effects or product delegation. It advances the migration head and therefore
requires all prior activation and migration-tail fixtures to bind `093`.

## Out of scope

- Public/API/Chat delegation, user cohort access or package-selected Child
  prompts/arguments.
- More than one Child, depth two, sibling fan-out or nested delegation.
- Child Broker effects, Project/Workspace/MCP/Artifact actions, Egress, Secrets
  or Provider calls.
- Cron, learning, Skill installation, general Runtime enablement and G21.6
  product promotion.
- Live host provisioning, live credentials/evidence or protected runtime-state
  changes.

## Research references

- [`research/activation-rollout.md`](research/activation-rollout.md)
- [`research/reap-transport.md`](research/reap-transport.md)
- [`research/authority-subsets.md`](research/authority-subsets.md)
- [`research/restart-state-machine.md`](research/restart-state-machine.md)
