# Product Runner caller research

## Current Runner policy

`neo-runnerd` has six mutually distinct execution callers after G21.5. Each
caller receives an explicit method set; adding a later caller must not mutate
the earlier lists. The Root canary already proves signed launch/heartbeat/
cancel and durable Orchestrator terminalization, while the Draft worker proves
bounded result retrieval.

## Product-canary requirements

- Add a seventh execution identity:
  `spiffe://neo-chat/agent-runtime-product-canary`.
- Permit only `probe`, `list`, `reconcile`, `launch`, `heartbeat`, and `cancel`.
  The fixed first product plan needs no `prepare`, `commit` or `result`, so it
  cannot reach Broker effects or read Runner quarantine artifacts.
- The exact LOGIN inherits exactly
  `agent_product_canary_worker`, `agent_orchestrator_runtime` and
  `agent_runner_control`. It must have no owner/admin role, direct DML,
  provisioning, Shadow-policy or promotion authority.
- Keep the API and worker credentials disjoint. Runner mTLS and Ed25519 private
  authority keys are mounted only into the default-off worker profile.

## Execution-plan shape

- The checked-in schema freezes package/runtime/workspace/Grant/Registry
  fingerprints, image, argv and resource limits. Per-request derivation may
  change only user ID and stable idempotency binding.
- The Tool Registry is empty, depth is zero, `networkMode=none`, rootfs is
  read-only, Egress and Secrets are absent, and wall/resources remain below the
  production policy.
- Product canary completion is a bounded smoke Run: launch, prove Runner and
  PostgreSQL heartbeats, signed cancel/reap, atomically terminalize the durable
  Run chain, then bind the request receipt. It does not accept arbitrary user
  prompt/arguments and does not imply generic package execution.
- Crash recovery resolves an existing live attempt or waits for lease expiry;
  it never creates a second idempotency key. Health requires no expired claim,
  no pending terminalization and zero Runner residue for the activation.

## Reuse decision

Reuse the proven G21.1 execution mechanics through a product-specific wrapper
and configurable caller/actor/reason bindings rather than introducing a
second unsigned launch implementation. Root-canary defaults and tests remain
frozen; product mode is accepted only through the new constructor and plan
schema.

