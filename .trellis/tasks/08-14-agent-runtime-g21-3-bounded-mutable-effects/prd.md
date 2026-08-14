# Agent Runtime G21.3 — Bounded Project mutation canary

## Goal

Add the first independently activated, explicitly approved mutable Agent action:
one exact synthetic Project compare-and-swap write. Prove durable status,
acknowledgement-loss recovery, terminal no-retry behavior, cleanup and least
privilege through the existing Runner-to-Broker seam without enabling generic
mutation, MCP writes, Egress, Secrets, user Projects, API/Chat Agent execution
or production promotion on this development host.

## Shared baseline

- G20.1-G20.10 and G21.0-G21.2 remain binding. Legacy text Skills stay deleted;
  only admitted Package Skills may eventually supply execution.
- The user approved all recommended decisions, direct verified commits and no
  ordinary follow-up preference questions. No sub-Agent delegation is used.
- The current development host remains `ISOLATION_UNAVAILABLE`; do not install
  host dependencies, create accounts/sub-IDs/cgroups/systemd units or provision
  live credentials/evidence.
- Never read or modify `.env.single-server`, `data/`, `secrets/` or `backup/`.
- All checked-in evidence is synthetic/template-only and all documentation is
  English.

## Requirements

### Action-by-action activation

- Add a strict `project_mutation_canary` activation stage, default-off Compose
  profile and standalone command distinct from control, Root and Broker/
  Artifact canaries.
- Require G21.0, G21.1 and G21.2 readiness before G21.3 may start. Keep broad
  `AGENT_RUNTIME_ENABLED`, `AGENT_BROKER_MUTATION_ENABLED`, MCP write, generic
  Egress, Secrets, delegation, scheduling, Skill install and learning false.
- Admit exactly one synthetic `project.patch` Tool with capability
  `project.write`, action `apply_patch`, classification `mutable`,
  `idempotent=false`, `approval=per_commit`, one exact resource, one UTF-8 write,
  no delete, no traversal and a small exact byte budget.
- Use one synthetic Run/Attempt for the action. User Projects, package-selected
  arguments and arbitrary paths are not eligible.

### Independent identity and private relay

- Add caller identity `spiffe://neo-chat/agent-runtime-project-canary` and a
  distinct Runner-to-Project relay identity. Preserve all three earlier caller
  method sets byte-for-byte.
- Extend Runner relay wiring to select one fully configured private literal-
  HTTPS mTLS relay by the already-authenticated original caller. Reject partial
  tuples, caller/relay cross-routing, hostnames, public/wildcard/link-local
  endpoints and identity reuse before forwarding.
- The relay independently verifies the original signed authority ticket,
  unsigned body fingerprint, caller, Runner, Attempt and plan binding before
  invoking Broker service.
- Add a ninth LOGIN recursively inheriting exactly
  `agent_orchestrator_runtime`, `agent_runner_control`,
  `agent_effect_control` and new `agent_project_mutation_control`, with no
  elevated attributes, owner membership or direct table DML.

### Separate explicit approval

- Add a strict short-lived `neo.agent-project-mutation-approval/v1` document
  signed by a dedicated Ed25519 operator key. The canary mounts only its public
  key; the private key stays outside the process and Git.
- Bind release commit, migration head `092`, target, activation/plan identity,
  caller, exact request/idempotency identity, Tool/action/resource, actor,
  reason and validity window. The activation record binds the approval document
  and public-key fingerprints.
- Require approval, Runner-authority and TLS keys/identities to be distinct.
  Insecure/symlinked files, signature or binding drift, placeholder material and
  stale/future windows fail before database/Runner access.
- After exact Prepare, verify the signed document and append one `per_commit`
  approval for that immutable intent. The process cannot mint a replacement
  approval or widen it to another action.

### Durable synthetic Project CAS authority

- Add migration `092_agent_project_mutation_canary` with operator-provisioned
  synthetic Project resources, immutable mutation/status/cleanup facts and a
  function-only `agent_project_mutation_control` role.
- Runtime authority cannot create arbitrary resources. Exact-host operations
  must provision one reviewed baseline resource while all canaries are off.
- Before CAS, recheck the exact committing effect intent, positive approval,
  Tool/action/resource, user, Run/Step/Attempt, generation/lease, snapshot,
  Grant/Registry, non-revocation, Kill Switch, base revision, patch fingerprint,
  UTF-8 shape and byte bound.
- Atomically update the synthetic content/revision and append the stable
  idempotency receipt. Exact replay returns the same receipt; key/fingerprint,
  base-revision, resource and content collisions fail closed.
- Expose exact stable status: unchanged base plus no receipt proves not sent;
  committed receipt proves success; any conflicting state is ambiguous and may
  not be retried.
- Restore the exact baseline only after terminal committed authority, retain the
  content-free mutation/status/cleanup audit, and make cleanup replay-safe.

### Crash and terminal behavior

- Prove crash/failure points before Commit claim, after claim before Project
  CAS, after CAS before acknowledgement, after Broker completion and during
  cleanup/restart.
- A committed Project receipt after acknowledgement loss completes exactly once.
  A durable not-sent proof fails without redispatch. Unavailable or conflicting
  status terminalizes Run/Step/Attempt as `outcome_unknown`; restart and replay
  never issue a second Project CAS under any key.
- Pre-Commit cancellation and approval rejection perform zero Project writes.
  Stale lease/generation, revoked Grant or Kill Switch denial occurs before CAS.
- Sandbox launch inputs, mounts, env and process receive no database URL, relay
  key, approval material, MCP/S3/Provider/vault credential or generic network.

### Production gates and documentation

- Add strict schemas/fixtures and focused source/unit/PostgreSQL 17/Compose/
  enabled-preflight gates for activation, signed approval, caller relay routing,
  least privilege, CAS/status/cleanup, crash/no-retry and credential absence.
- Advance the migration head and every older guarded migration-tail drill from
  `091` to `092`, preserving clean peel/reapply behavior.
- Integrate G21.3 into Phase 0 and full standalone verification while retaining
  G21.0-G21.2 regressions and the expected current-host
  `ISOLATION_UNAVAILABLE` result.
- Synchronize architecture, executable contract, deployment, backup/restore,
  G20/G21 tracking, process record, migration docs and Trellis specs.

## Acceptance criteria

- [x] Four Runner callers retain exact disjoint method/relay policy; Broker and
      Project identities cannot cross-route or reuse relay identities.
- [x] The only new action is one plan-bound synthetic Project CAS write with
      `per_commit` approval; generic mutation/MCP write/Egress/Secrets remain
      unavailable.
- [x] The canary verifies a separate offline-signed approval and cannot approve
      itself, widen bindings or reuse the approval for another intent/action.
- [x] Migration `092` proves function-only role access, exact authorization,
      CAS/replay/collision/status/cleanup, stale/Grant/Kill fences, dump/restore
      and clean guarded down/up on PostgreSQL 17.
- [x] Crash tests prove after-CAS acknowledgement loss resolves committed,
      provably not-sent resolves failed without redispatch, and ambiguous status
      becomes terminal `outcome_unknown` with no second CAS.
- [x] Compose/example/preflight are default-off, require distinct principals/
      identities/keys and keep every controller credential out of Runner and
      Sandbox.
- [x] G21.3 plus G21.0-G21.2 regressions, Phase 0 and full standalone pass; the
      current host still reports `ISOLATION_UNAVAILABLE` and protected runtime
      paths remain untouched.

## Definition of done

- Source, tests, migration, fixtures, scripts, Compose/example configuration,
  docs and Trellis specs agree on the single-action synthetic mutation boundary.
- Verification passes, then work/task/journal commits are created without
  amend or push.

## Technical approach

Create `agentprojectcanary` plus an `agent-runtime-project-canary` command. Reuse
Orchestrator, Runner authority/client, Broker repository and private relay
contracts, but add an exact caller-to-relay router in `neo-runnerd`. A strict
plan resolver registers one Project effect executor. Migration `092` implements
an independently queryable synthetic CAS/status/cleanup authority so restart
reconciliation never guesses whether a write occurred.

The controller prepares through Runner, verifies the offline-signed approval,
records the exact Broker approval, commits through Runner and reconciles the
terminal Sandbox. The Project executor returns only a canonical receipt
fingerprint. Raw synthetic content stays solely inside its dedicated Project
canary table and is restored to the operator-provisioned baseline after a
successful terminal receipt.

## Decision (ADR-lite)

**Context:** Reusing the G21.2 profile would widen a read/Artifact identity that
also carries S3/MCP credentials. Enabling an MCP write would add remote network,
credential and status semantics before the first Project mutation is proven. A
filesystem sidecar ledger cannot atomically decide every crash point without a
real Product Project store contract.

**Decision:** Activate one separately identified synthetic PostgreSQL-backed
Project CAS canary, require an external signed `per_commit` approval, add
function-only migration `092`, and route its Runner Broker calls through a
caller-specific private relay. Keep all broad mutation flags false.

**Consequences:** G21.3 proves the mutable approval/idempotency/crash seam but
still does not let users or arbitrary Package Skills write Projects. A later
stage can replace the synthetic CAS executor with an actual Product Project
adapter or admit one exact MCP write only after its own status/cleanup evidence.

## Out of scope

- User-facing/API/Chat Agent execution or general Hermes-like autonomous
  Runtime enablement.
- User Project mutation, multi-file patches, deletes, arbitrary paths or
  package-controlled write arguments.
- MCP writes, generic remote network/Egress, Secret/Vault use, Provider/model
  calls and arbitrary external effects.
- Child Agents, Cron, Learning, Skill installation and final product cohort.
- Live host provisioning, live credentials/evidence and any protected runtime-
  state changes.

## Research references

- [`research/action-rollout.md`](research/action-rollout.md)
- [`research/approval-authority.md`](research/approval-authority.md)
- [`research/project-cas-status.md`](research/project-cas-status.md)
- [`research/relay-isolation.md`](research/relay-isolation.md)
