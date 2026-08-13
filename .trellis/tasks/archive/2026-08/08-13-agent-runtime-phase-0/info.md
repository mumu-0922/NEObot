# Agent Runtime Phase 0 — Technical Design

## Decision

Build a Neo-owned durable Agent Runtime rather than embedding Hermes Agent or
extending the browser text-Skill path. Phase 0 freezes contracts only; every
production entry remains disabled until later Epic groups pass explicit
promotion gates.

## System Slice

```text
Browser
  -> Go Control API / Durable Orchestrator
  -> PostgreSQL authority + immutable object artifacts
  -> mTLS internal RPC
  -> host-side non-root neo-runnerd
  -> one rootless OCI sandbox per Run
  -> Brokered Tool / Egress / Secret boundaries
```

Control plane decides identity, grants, admission, snapshots, state transitions,
approvals, scheduling, cancellation and audit. Data plane executes only the
frozen launch envelope. Neither the Skill package nor the model is an authority.

## Machine Contracts

| Schema | Authority |
| --- | --- |
| `neo-skill-runtime-manifest` | package/runtime declarations and immutable provenance binding |
| `neo-capability-grant` | server-issued action/resource/approval/budget allowlist |
| `neo-runner-rpc` | versioned control/data-plane requests and responses |
| `neo-run-event` | append-only Run/Step/Attempt transition and side-effect facts |

Draft 2020-12 schemas use strict unknown-field rejection, stable enums, bounded
strings/arrays and fixture-based negative proof. JSON Schema cannot express all
cross-document authority, so the offline verifier also checks invariants such
as child depth, forbidden registry tools, grant/package binding, event ordering
and Prepare/Commit keys.

## State Authority

```text
Run: pending -> admitted -> queued -> running -> {succeeded, failed, canceled,
     killed, outcome_unknown}
Step: pending -> ready -> running -> {succeeded, failed, skipped, canceled,
      killed, outcome_unknown}
Attempt: leased -> starting -> running -> prepared -> committing ->
         {succeeded, failed, canceled, killed, outcome_unknown, lease_expired}
```

Every transition is append-only and guarded by `runId`, `stepId`, `attemptId`,
`sequence`, `leaseGeneration`, and expected prior state. A higher lease
generation invalidates all operations from an older Attempt. Terminal conflict
precedence is `outcome_unknown > killed > canceled > failed > succeeded` for
ambiguous concurrent observations; terminal rows are never rewritten.

## Side Effects

1. `prepare` validates frozen grant and produces a bounded immutable intent.
2. Backend stores intent hash and required approval state.
3. `commit` revalidates kill/revoke/expiry/lease and uses a stable idempotency key.
4. Known idempotent acknowledgement completes the Attempt.
5. Lost acknowledgement after possible effect becomes `outcome_unknown`; no
   automatic second Commit is permitted.

## Security Boundary

- Rootless OCI is a release prerequisite, not a best-effort optimization.
- `neo-runnerd` remains non-root and holds no general application credential.
- Sandboxes never receive the project source tree, host runtime socket, database,
  object-store or provider vault.
- Capability, Egress and Secret grants are independently frozen and intersected.
- Child Agent Tool Registry construction has an unconditional forbidden set;
  `delegate_task` is absent, not merely denied after selection.
- Package ingestion treats every archive, manifest, instruction and asset as
  untrusted supply-chain input.

## Rollout

G20 is decomposed into contract, supply-chain, orchestrator, runner/isolation,
Tool/secret/egress, delegation, Cron/learning, UI/cutover and promotion groups.
Current browser Skills are deleted only in the final cutover group after the new
path has clean-copy, restart, rollback and history-label proof.

## Verification Boundary

`verify-agent-runtime-phase0.sh` validates only checked-in Phase 0 artifacts. It
does not claim OCI isolation has passed on this host and must print that runtime
execution remains disabled. Later groups must add the real Isolation Acceptance
Suite and rootless runtime probe before enabling any execution.
