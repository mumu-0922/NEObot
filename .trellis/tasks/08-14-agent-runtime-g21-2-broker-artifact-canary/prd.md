# Agent Runtime G21.2 — Read-only Broker and Artifact canary

## Goal

Add a separately activated, least-privilege production canary that wires the
existing Runner Prepare/Commit seam to durable Broker authority, executes only
reviewed synthetic Project/Workspace/MCP reads plus bounded Artifact
publication, and proves replay, stale-lease denial, object-before-row cleanup,
credential-free Sandbox operation and terminal `outcome_unknown` handling
without enabling user-facing or mutable Agent effects.

## Shared baseline

- G20.1-G20.10 and G21.0-G21.1 remain binding. Legacy pure-text Skills stay
  deleted and admitted Package Skills remain the only future execution source.
- The user approved all recommended decisions, direct verified commits and no
  ordinary follow-up preference questions. No sub-Agent delegation is used.
- The current development host remains `ISOLATION_UNAVAILABLE`; do not install
  host dependencies, create accounts/sub-IDs/cgroups/systemd units or provision
  live credentials.
- Never touch `.env.single-server`, `data/`, `secrets/` or `backup/`.
- No API/Chat/browser route, rootful container fallback or generic Tool access
  is permitted.

## Requirements

### Independent activation and identities

- Add a strict `broker_artifact_canary` activation stage, default-off Compose
  profile and standalone process distinct from G21.0 control and G21.1 Root
  canary.
- Require `spiffe://neo-chat/agent-runtime-broker-canary` for lifecycle plus
  Prepare/Commit calls and a different Runner-to-Broker relay mTLS identity.
- Preserve the G21.0 and G21.1 method sets without widening them.
- Require a distinct PostgreSQL LOGIN recursively inheriting exactly
  `agent_orchestrator_runtime`, `agent_runner_control`, `agent_effect_control`
  and new `agent_artifact_control`, with no table DML or elevated attribute.
- Bind the exact release, target, policy, Runner/relay endpoints, both mTLS
  trust tuples, authority public key, canary plan and migration head `091` in
  short-lived external activation evidence.

### Real Runner relay wiring

- Add a private literal-HTTPS mTLS relay from `neo-runnerd` to the Broker
  canary service. Runner forwards only authority-verified Prepare/Commit bodies
  and holds no Broker database, object-store, MCP, Provider or vault credential.
- The relay independently verifies the original signed authority ticket,
  unsigned body fingerprint, caller, Runner, Attempt and plan binding before
  invoking `agentbroker.Service`.
- No direct Broker shortcut may be presented as end-to-end canary evidence.
  The default relay stays unavailable unless the dedicated configuration is
  complete and the Broker canary identity is enabled.

### Reviewed read-only actions

- Extend Broker service wiring to use the existing separate
  `ReadOnlyExecutor` contract. Registered read executors accept only Registry
  entries that are `read`, idempotent and automatic.
- Implement bounded, traversal/link/special-file-safe synthetic Project and
  Workspace reads from exact read-only canary roots.
- Execute one exact auth-none, manifest-approved read-only MCP Tool through the
  private MCP Runner connector. Generic MCP servers, credentials, unknown or
  mutable classification and arbitrary Tool names remain denied.
- Return only canonical receipt fingerprints through the durable Commit
  protocol. Raw arguments/results never enter logs, events or Broker rows.
- Use one synthetic Run per terminal action rather than changing migration
  `086`'s one-Commit terminal state machine.

### Artifact publication authority

- Add migration `091_agent_artifact_publication` with the narrow
  `agent_artifact_control` role and exact authorize/attach functions. The role
  has no direct `agent_artifacts` DML and no owner membership.
- Before object upload and again before row attachment, recheck the exact
  committing intent, Tool/action, user, Run, Attempt/generation, live lease,
  snapshot, Grant/Registry, non-revocation, Kill Switch, max bytes and media
  allowlist.
- Implement PostgreSQL Artifact repository wiring and use the existing bounded
  same-byte scan, object-before-row and compensating object deletion path.
- Prove exact replay succeeds while row/name/object collisions fail closed.
  Quarantine is removed after success or rejected content; a row failure
  removes the newly written object.

### Canary flow and failure handling

- Strictly load a mounted synthetic plan containing separate Project,
  Workspace, MCP, Artifact and acknowledgement-loss action plans. Each has a
  frozen Grant/Registry, immutable roots or server identity, bounded resources,
  no Secret refs and `networkMode=none`.
- For each action, idempotently enqueue and claim one Run, launch/heartbeat the
  exact Sandbox, call Prepare then Commit through Runner, and reconcile the
  terminal Sandbox to zero inventory.
- Prove exact Prepare and Commit replay produce no second execution; a stale
  lease/generation is denied before executor/object access.
- Exercise one possible-send read executor whose status cannot resolve the
  dispatch. Persist `outcome_unknown`, never issue a second Commit under any
  key, and treat later canary replay as terminal observation only.
- Sandbox launch inputs, mounts, env and process receive no database URL,
  object-store key, MCP token, relay key, Provider secret or vault handle.

### Production gates and documentation

- Add focused source/unit/PostgreSQL 17/Compose/preflight gates for G21.2,
  including clean down/up, least privilege, replay/collision, stale lease,
  object cleanup, credential absence and `outcome_unknown` no-retry.
- Update earlier migration-tail drills to peel/reapply `091` before testing
  older guarded migrations and finish at the new head.
- Integrate G21.2 into Phase 0 and full standalone verification while retaining
  G21.0/G21.1 regression gates and the expected current-host failure.
- Synchronize architecture, executable contract, deployment, tracking,
  migration docs and Trellis specs.

## Acceptance criteria

- [x] Runner ingress keeps three exact caller method sets and the private relay
      independently verifies the original signed authority before Broker use.
- [x] Project/Workspace/MCP canaries traverse Runner Prepare/Commit with one
      execution per exact intent; mutable/unknown/widened actions are denied.
- [x] Migration 091 proves authorize/attach replay, collision denial, live
      generation/lease/snapshot/Grant/Kill fences and function-only least
      privilege on PostgreSQL 17.
- [x] Artifact tests prove bounded same-byte scanning, object-before-row,
      compensating delete and successful quarantine cleanup.
- [x] A possible-send unresolved action terminalizes Run/Step/Attempt as
      `outcome_unknown`; restart/replay performs no second dispatch.
- [x] Compose/example/preflight stay default-off, keep credentials out of
      Runner/Sandbox, and require distinct relay/canary identities and database
      principals.
- [x] G21.2, G21.1 regression, Phase 0 and full standalone gates pass; the
      current host still reports `ISOLATION_UNAVAILABLE` and protected runtime
      paths remain untouched.

## Definition of done

- Source, tests, migration, fixtures, scripts, Compose/example config, docs and
  Trellis specs agree on the read-only/artifact-canary-only boundary.
- Verification passes, then work/task/journal commits are created without
  amend or push.

## Technical approach

Add an `agentbrokerrelay` private mTLS adapter between Runner and a new
`agentbrokercanary` service. The canary process composes the existing
Orchestrator, Runner authority/client and Broker repository with a strict
plan-backed resolver, reviewed read executors, private MCP Runner connector,
normal object store and PostgreSQL-backed Artifact publisher. Migration 091
adds only the missing narrow Artifact publication functions and role.

Keep migration `086`'s terminal contract unchanged by allocating one synthetic
Run per action. The exact-host flow launches a no-network/no-secret Sandbox for
each Run, drives Prepare/Commit through Runner, then reconciles terminal runtime
state. Development verification uses deterministic fakes and disposable
PostgreSQL/object storage; it does not impersonate exact-host acceptance.

## Decision (ADR-lite)

**Context:** Direct Broker calls would not prove Runner wiring; embedding the
Broker in Runner would give the host daemon privileged credentials; reusing
the G21.1 identity would silently widen a reviewed launch-only principal; and
sequencing several actions in one Run conflicts with the verified Commit
terminal state machine.

**Decision:** Use a separate activation/profile/caller/LOGIN, a separate
Runner-to-Broker mTLS relay, migration 091 function-only Artifact authority,
and one synthetic Run per terminal action. Keep all mutable effects false.

**Consequences:** G21.2 proves the first real Broker/Artifact production seam
while user Runtime remains disabled. The service carries narrowly scoped MCP
and object-store credentials, but Runner and Sandbox do not. General Tool
results, Project mutation and user Artifact policy remain later release work.

## Out of scope

- User-facing/API/Chat Agent execution, general scheduling and full Hermes-like
  autonomous Runtime enablement.
- Project mutation, arbitrary filesystem access, generic MCP marketplace or
  credentialed MCP Tools, Provider/model calls, Secret Broker and generic
  Egress.
- Child Agents, Cron, Learning, Skill installation, bounded mutable effects and
  final production closure.
- Live host installation, live credentials/evidence and protected runtime-state
  changes.

## Research references

- [`research/relay-boundary.md`](research/relay-boundary.md)
- [`research/read-only-actions.md`](research/read-only-actions.md)
- [`research/artifact-authority.md`](research/artifact-authority.md)
