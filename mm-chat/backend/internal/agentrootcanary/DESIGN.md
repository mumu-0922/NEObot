# agentrootcanary design

## Goals

- Prove one exact synthetic Root Run across durable claim, signed Runner RPC,
  heartbeat, cancel, terminal event and cleanup boundaries.
- Keep G21.0 maintenance identity and general Runtime authority physically
  separate.
- Preserve memory-only lease credentials while supporting safe restart.
- Eliminate the crash window between Sandbox and Orchestrator terminal writes.

## Non-goals

- User-facing/API/Chat execution or general scheduling.
- Broker Prepare/Commit, Provider, Secret, Egress, Artifact publication,
  Project mutation, Child, Cron or Learning behavior.
- Target-host provisioning, evidence creation or production promotion.

## Data flow

```text
root_run_canary activation + immutable plan
                    |
                    v
          idempotent enqueue/claim
                    |
                    v
     request-bound Ed25519 launch ticket
                    |
                    v
      rootless Runner Sandbox (network none)
                    |
                    v
     Runner heartbeat + PostgreSQL heartbeat
                    |
                    v
        signed cancel + exact Runner reap
                    |
                    v
one PostgreSQL transaction:
  Sandbox terminal + Attempt canceled
  + Step canceled + Run canceled + events
                    |
                    v
           reconcile/list == empty
```

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Separate mTLS identity and LOGIN | Reusing control would widen maintenance authority into execution. | Operators provision independent files and a seventh principal. |
| Reuse migrations `084`/`085` | Existing SECURITY DEFINER APIs already express the required transitions. | No migration `091`; role composition is deployment state. |
| Atomic terminal transaction | Independent terminal calls can lose the memory-only token between commits. | Sandbox plus Attempt/Step/Run and events succeed or roll back together. |
| Strict immutable plan | A canary must not become a generic workload launcher. | Unknown fields, Tools, Egress, Secrets and mutable settings fail closed. |
| Wait for live tokenless lease expiry | Reconstructing or persisting a lease token would weaken fencing. | Restart may be delayed by at most the bounded lease duration. |
| Zero final inventory | Success must prove exact reap, not merely a terminal database row. | A non-expired expected Sandbox keeps replacement cycles unavailable. |
| Best-effort cancel on post-launch failure | A known workload should be killed before waiting for lease recovery. | Heartbeat failure still returns error after cleanup for explicit revalidation. |

## Trust boundaries

- Plan, activation, manifests and Runner responses are untrusted until strict
  shape, fingerprint, identity and time-window validation completes.
- Runner method authorization is selected from the verified mTLS caller before
  service execution. The canary never receives `prepare` or `commit`.
- Signed tickets bind caller, request replay identity, canonical body
  fingerprint, Run lineage, lease digest, snapshot, Runner and Kill-Switch
  epoch.
- The database stores only the lease-token SHA-256 digest. Raw lease tokens
  remain in process memory and are never logged or reconstructed.
- The LOGIN has no direct table DML and recursively inherits exactly
  `agent_orchestrator_runtime` and `agent_runner_control`.

## Failure and recovery

- A terminal idempotent replay performs no second launch and rechecks cleanup.
- A live Attempt without its token returns `ROOT_CANARY_RECOVERY_PENDING` until
  expiry; normal Orchestrator reclaim creates a new generation.
- After a known running projection, Runner or PostgreSQL heartbeat failure
  attempts signed cancel and atomic terminalization before returning failure.
- If cancel or terminalization is unavailable, the lease and projection stay
  durable and fenced for later expiry/reconcile; no false success is written.
- Launch success followed by a crash before expected projection can leave an
  orphan on Runner. Recovery reconciliation reaps it after authority expiry.

## Known limitations

- Source/disposable PostgreSQL tests are not exact-host isolation evidence.
- The canary intentionally supports only one immutable plan and requires zero
  final inventory; concurrent production workloads belong to later G21 groups.
- Activation and plan creation are operator workflows outside this package.

## Change history

### 2026-08-14 — G21.1 initial implementation

Added strict plan handling, durable Orchestrator/Runner composition, signed
launch/heartbeat/cancel, restart fencing and atomic canceled terminalization.
