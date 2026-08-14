# Agent Runtime G21.1 — Root Run launch canary

## Goal

Add a separately activated, least-privilege production worker that executes
exactly one synthetic Root Run through the existing durable Orchestrator and
rootless Runner, proving claim, signed launch authority, heartbeat,
cancellation, append-only terminal state, restart recovery and exact Sandbox
cleanup without enabling user-facing execution or Broker effects.

## Shared baseline

- G20.1-G20.10 and G21.0 remain binding, including migrations `083`-`090`,
  Package-Skill-only authority and deleted legacy text Skills.
- The user approved all recommended decisions, direct verified commits and no
  further ordinary preference questions. No sub-Agent delegation is used.
- The current development host remains `ISOLATION_UNAVAILABLE`; do not install
  host dependencies, create identities/sub-IDs/cgroups/systemd units or
  provision live credentials.
- Never touch `.env.single-server`, `data/`, `secrets/` or `backup/`.
- No browser/API/rootful Docker fallback is permitted.

## Requirements

### Independent activation and identities

- Add a strict, short-lived `root_run_canary` activation contract distinct
  from G21.0 `control_plane`. It binds the exact release, target, policy,
  Runner endpoint/manifest/binary, canary mTLS identity, authority public key
  and immutable canary plan.
- Require `spiffe://neo-chat/agent-runtime-root-canary`, a separate mTLS
  certificate/key, authority private key and a distinct PostgreSQL LOGIN.
- The LOGIN must recursively inherit exactly `agent_orchestrator_runtime` and
  `agent_runner_control`, with no extra role or elevated attribute.
- Preserve G21.0's dedicated LOGIN and control-only activation unchanged.

### Runner method isolation

- Enforce caller-specific Runner methods. G21.0 control remains physically
  limited to `probe/list/reconcile`.
- The Root canary identity may call only
  `probe/list/reconcile/launch/heartbeat/cancel`; it may not call
  `prepare/commit` or any Broker surface.
- Bind signed authority to the exact request ID, nonce, request fingerprint,
  user, Run/Step/Attempt/generation, owner, lease-token digest, snapshot,
  Runner and current Kill-Switch epoch.

### One synthetic Root Run

- Load one strict immutable canary plan for a pre-provisioned synthetic user.
  It must use depth `0`, an empty Tool Registry, `networkMode=none`, no Secret
  refs, no Egress, no Artifact publication, read-only rootfs, empty
  capabilities and bounded resource limits.
- Idempotently enqueue one content-free Run and claim its only Step. Never
  expose an HTTP/API/Chat trigger.
- Launch through the exact Runner, persist expected/running Sandbox state,
  transition the Attempt to running and exercise both Runner and PostgreSQL
  heartbeats.
- Cooperatively cancel the Sandbox, reap exact runtime/Scratch state and
  atomically terminalize the expected Sandbox plus Attempt/Step/Run with
  append-only `canceled` events.
- Prove the post-cancel Runner inventory contains no canary Sandbox.

### Restart and failure recovery

- Replaying the same plan/activation must resolve the same idempotent Run and
  never launch a second Run after terminal success.
- A live Attempt whose opaque lease token was lost on restart remains fenced;
  wait for expiry, reconcile stale Runner state and reclaim under a new lease
  generation. Never persist or reconstruct lease credentials.
- Any failure after launch must prefer exact signed kill/cancel and durable
  failure/cancellation. Cleanup/reconcile remains available while all broader
  Runtime flags are off.
- Activation, plan, certificate, authority-key, release or host drift exits
  fail closed with a stable content-free error.

### Production wiring and gates

- Add a separate default-off `agent-runtime-root-canary` Compose profile and
  binary. Keep `AGENT_RUNTIME_ENABLED`, Broker, Child, Cron and Learning false.
- The worker has no port and no Provider/object-store/Redis/MCP credential,
  uses a read-only rootfs and receives only its exact read-only plan/evidence
  plus mTLS/authority files.
- Production preflight validates the separate profile, private endpoint,
  secure regular files, distinct identities/principals, exact flags and a
  ready stage-specific activation.
- Add a focused G21.1 verifier and integrate it into Agent Runtime Phase 0 and
  full standalone verification. The exact-host test remains expected-nonzero
  on this machine.
- Synchronize architecture, contract, deployment, tracking and Trellis specs.

## Acceptance criteria

- [x] Control and canary mTLS identities have disjoint Runner method policies;
      control launch and canary Broker methods are rejected before execution.
- [x] A focused service test proves enqueue/claim, signed launch, heartbeat,
      cancel, atomic terminal events and zero remote inventory.
- [x] Crash/restart tests prove idempotent terminal replay and expired lease
      reconciliation/reclaim without durable lease-token storage.
- [x] Canary plan/activation validators reject unknown fields, placeholders,
      stale evidence, widened authorization, Tools, Egress/Secrets, mutable
      runtime settings and binding drift.
- [x] Compose/example/preflight remain default-off and require distinct
      canary credentials and a database principal inheriting exactly the two
      approved roles.
- [x] G21.1, Phase 0 and full standalone gates pass; the current host still
      reports `ISOLATION_UNAVAILABLE` and protected runtime paths are untouched.

## Definition of done

- Source, tests, schemas, fixtures, scripts, Compose/example config, docs and
  Trellis specs agree on the Root-canary-only boundary.
- Verification passes, then work/task/journal commits are created without
  amend or push.

## Technical approach

Compose the existing `agentorchestrator` and `agentrunner` services in a new
internal Root-canary service and standalone command. Reuse migrations `084`
and `085`; use a LOGIN that inherits exactly both existing runtime roles rather
than adding migration `091`. Add a safe two-phase request/ticket binder and a
single PostgreSQL transaction for Sandbox plus Run terminalization to remove
the lost-token crash window.

## Decision (ADR-lite)

**Context:** Reusing the G21.0 worker/certificate would widen maintenance
authority, while three independent terminal transitions can strand a Run after
a crash because lease tokens are intentionally memory-only.

**Decision:** Use an independent stage/profile/identity/principal, enforce
method policy at Runner ingress, and atomically append terminal projections via
the existing SECURITY DEFINER functions. Add no migration.

**Consequences:** G21.1 proves the narrow production execution path without
unlocking general Runtime. G21.2 can add read-only Broker/Artifact canaries only
under another explicit activation stage.

## Out of scope

- User-facing/API/Chat execution and general Root Run scheduling.
- Broker Prepare/Commit, Project mutation, Provider calls, Secret handles,
  published Artifacts or mutable external effects.
- Child Agents, Cron, Learning, Shadow promotion and final production closure.
- Migration `091`, live host install, live credentials/evidence and protected
  runtime-state changes.

## Research references

- [`research/runtime-flow.md`](research/runtime-flow.md)
- [`research/activation-and-identity.md`](research/activation-and-identity.md)
- [`research/canary-plan-and-cleanup.md`](research/canary-plan-and-cleanup.md)
