# agentlearningworker design

## Goals

- Run isolation and evaluation in the accepted rootless Runner path.
- Bind every byte of evidence to one activation, Draft, check generation,
  package/runtime/archive, Workspace and suite.
- Persist a result and remove the Sandbox before migration 089 accepts the
  three-check receipt bundle.

## Non-goals

- Draft proposal/review, Reject/Promote, package admission, Broker effects,
  network, Secrets, arbitrary images/argv, or product cohort selection.

## Lifecycle

```text
scoped Draft claim
  -> probe exact Runner features
  -> begin migration-094 Draft-only Attempt
  -> signed launch (empty Tool Registry, network none)
  -> poll signed result for draft-check-result.json
  -> recompute size/SHA-256 and validate all bindings
  -> persist immutable result
  -> signed cancel/reap
  -> mark cleanup complete
  -> return migration-089 CheckReceipt
```

The transport `run_*` identity satisfies the Runner protocol but has no row in
`agent_runs` or `agent_attempts`. Migration 094 cannot call Broker, delegation,
Cron or admission functions.

## Security decisions

- The plan file must be absolute, regular, non-symlink and mode-private.
- The caller is fixed to
  `spiffe://neo-chat/agent-runtime-draft-learning`.
- Only `launch`, `result`, and `cancel` are signed; probe/list/reconcile are
  lifecycle reads, and heartbeat/Prepare/Commit remain unavailable.
- Artifact payloads are strict JSON capped at 64 KiB. The worker recomputes the
  receipt hash and migration 094 revalidates every field independently.
- The worker database role has function-only access and no Promote/direct DML.

## Recovery and limits

Lost lease tokens are not recreated. Reconciliation must wait out signed
authority, cancel the exact stale Sandbox, terminalize the durable Attempt, and
only then release the Draft claim for a new generation. G21.5 is restricted to
one pre-staged synthetic Draft/Workspace; generic archive staging is deferred.

## Change history

- 2026-08-15: initial exact-target Draft Runner transport.
