# Product request authority research

## Existing authority boundary

- `agentcontrol.Service.EnqueueRootRun` is an intentional hard hold and never
  persists work. The authenticated API currently owns only migration-090
  product read models and narrow user/admin transitions.
- `agent_product_shadow_snapshot` already provides policy revision, opt-in
  generation, deterministic cohort selection, observation/error budgets and
  Kill-Switch fencing. It deliberately returns `effective=false` and
  `ISOLATION_UNAVAILABLE` after all eligibility checks pass.
- The API process does not inherit `agent_orchestrator_runtime` or
  `agent_runner_control`. Preserving this split prevents a compromised HTTP
  process from constructing leases or Runner authority.

## Comparable durable hand-off patterns

1. Transactional outbox/job tables let the request-facing process append a
   bounded intent while a separate worker owns execution authority.
2. Lease-based queue claims use `FOR UPDATE SKIP LOCKED`, a generation/token
   and an expiry so restart can reclaim abandoned work without duplicate
   execution.
3. Idempotent submission binds a caller-supplied or server-issued request ID to
   the full immutable intent and rejects drift rather than silently replaying a
   different request.

## Recommended mapping

- Migration 095 should add an append-only product-canary request authority.
  The API inserts only through one `SECURITY DEFINER` function and receives no
  direct table DML or Orchestrator/Runner membership.
- Submission must bind the authenticated user, current Shadow policy revision,
  opt-in generation, admission/package/runtime fingerprints and one active
  operator-provisioned activation. Browser input must contain no prompt,
  package selector, argv, Tool, Egress, Secret or resource override.
- The activation supplies a global bounded request budget and exact plan
  fingerprint. One request per user/opt-in generation plus a stable request ID
  makes retries replay-safe.
- A dedicated worker claims via an expiring lease, enqueues/runs the fixed
  plan, and completes the request with a content-free Run/Attempt/receipt
  binding. Failure/release/reconcile functions remain activation-scoped.
- `effective=true` may be derived only when all existing Shadow eligibility
  fences and one live migration-095 activation agree. Absence of that row keeps
  this development host at `ISOLATION_UNAVAILABLE` without an API flag or
  in-process adapter.

## Rejected alternatives

- API-to-Orchestrator/Runner calls widen the most exposed process.
- A memory queue loses replay and restart authority.
- Reusing Shadow observations as execution requests conflates telemetry with
  durable work and cannot bind a Run receipt.

