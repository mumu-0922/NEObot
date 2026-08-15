# Agent Runtime G21.6 — Product canary and final production closure

## Goal

Expose one fixed, bounded Agent product canary to authenticated users who are
explicitly opted in and deterministically selected, through a durable API-to-
worker hand-off and the exact rootless Runner path. Bind that canary to the full
G21.0-G21.5 activation chain and close the release only when the strict live
operations matrix evaluates to `PROMOTION_READY`. Keep this development host
honestly held at `ISOLATION_UNAVAILABLE`.

## Shared baseline

- G20.1-G20.10, G21.0-G21.5 and migrations 083-094 remain binding. Legacy text
  Skills stay deleted; admitted Package Skills are the only eligible Runtime
  domain.
- The user approved all recommended decisions and direct verified commits. No
  additional ordinary preference question or sub-Agent delegation is needed.
- Never read or modify `.env.single-server`, `data/`, `secrets/` or `backup/`.
  Do not install host dependencies, create host identities/cgroups/systemd, or
  manufacture live evidence on this machine.
- Checked-in evidence is template/offline-only. Documentation is English.

## Requirements

### Two-stage activation without a promotion cycle

- Add a strict `product_canary` activation stage that requires fresh, same-
  release G21.0-G21.5 evidence and migration head 095. Bind exact release,
  policy, plan, admission/package/runtime and the seven prior activation
  fingerprints.
- The bounded activation may permit only a finite request budget for the
  existing opt-in/deterministic cohort. It is not final promotion and cannot
  widen package, Runtime, plan, cohort or budget in place.
- Add append-only final promotion authority binding the exact activation,
  product-canary receipt, closure fingerprint and `PROMOTION_READY` decision.
  Only the separate operator authority may record it after the read-only
  evaluator passes; API and workers cannot.
- Disabled, expired, stale or drifted activation immediately blocks new
  requests. Rollback stops/disables the profile and preserves immutable audit,
  request, receipt, incident and promotion facts.

### Migration 095 and least privilege

- Add `095_agent_product_canary_activation` with immutable operator-provisioned
  activation, append-only user requests, lease claims, terminal receipts and
  final promotion records.
- Add `agent_product_canary_worker` as a restricted NOLOGIN function-only role.
  The thirteenth exact LOGIN inherits exactly that role plus
  `agent_orchestrator_runtime` and `agent_runner_control`; it has no owner/admin,
  direct table DML, Shadow-policy, activation-provisioning or promotion
  authority.
- `go_api_runtime` receives only current-user enqueue/read function authority.
  It never inherits Orchestrator/Runner/worker roles and never constructs a
  lease, authority ticket, Sandbox, argv or Tool Registry.
- All request claim/release/complete/reconcile/prune functions are activation-
  scoped, lease/generation fenced, replay-safe and bounded. Dirty down is
  guarded when activation, work, claims, receipts or promotion facts remain.

### Authenticated product request boundary

- Replace the hard-held `POST /v1/agent-center/runs` seam with strict JSON
  containing only expected Shadow policy revision and opt-in generation. The
  server issues the request ID; no prompt, arguments, package, model, Tool,
  Egress, Secret, argv, Workspace or resource override is accepted.
- Submission atomically rechecks authenticated user existence, opt-in,
  deterministic cohort, policy/activation window, admission/package/runtime,
  request budget, Kill Switch and idempotency before appending one request.
- Extend Agent Center status with a product-canary state. The action is visible
  and enabled only when Shadow `effective=true`; otherwise the exact held reason
  remains visible. Accepted submission returns a bounded request DTO and the UI
  refreshes Runs/status without optimistic execution claims.
- Migration 095 may derive Shadow `effective=true` only from all existing
  eligibility fences plus a current exact activation. With no activation row,
  current development and normal default configuration remain
  `ISOLATION_UNAVAILABLE`.

### Dedicated product-canary worker and Runner caller

- Add `agent-runtime-product-canary` as a separate default-off worker/profile
  and add Runner caller
  `spiffe://neo-chat/agent-runtime-product-canary` with only
  `probe/list/reconcile/launch/heartbeat/cancel`. Preserve all six prior caller
  policies and both relay routes without widening.
- Freeze a private strict plan containing admitted package/runtime/workspace,
  Grant/Registry and Sandbox fingerprints, fixed image/argv, depth zero, empty
  Tool Registry, read-only rootfs, `networkMode=none`, no Egress/Secrets and
  bounded resources below production policy.
- A claimed request changes only user and stable idempotency binding. Reuse the
  signed G21.1 lifecycle mechanics: durable enqueue/acquire, Runner and
  PostgreSQL heartbeat, signed cancel/reap, atomic terminal chain and exact
  request receipt. The first product canary is a bounded smoke Run, not generic
  prompt/package execution.
- Crash/restart resolves the exact request/Run/Attempt before replacement,
  waits lost leases to expiry and never changes idempotency key. Health requires
  no stale claim/pending terminalization and zero activation Runner residue.

### Final closure and operations matrix

- Advance production policy, closure schema/evaluator/fixtures and every
  guarded migration-tail drill to head 095.
- Keep the existing 16 live operations checks and add strict full activation-
  chain and product-canary receipt bindings. The evaluator, not input, derives
  `PROMOTION_READY`; template/offline/disposable evidence can never promote.
- Extend zero-residue closure to temporary product requests, claims and receipt
  projections while retaining sanitized promotion/incident/audit authority.
- Add product request queue/stale-claim/failure/budget metrics and alerts using
  only the existing bounded content-free label allowlist.
- Update exact-host clean-copy/restart/reboot, paired backup/restore, DR,
  rollback/forward-fix, credential/mTLS/Runtime rotation, capacity, alert and
  cleanup runbooks. No current-host live claim is permitted.

### Gates and documentation

- Add focused Go race/vet, frontend Vitest/typecheck, PostgreSQL 17, activation,
  closure, Compose/preflight, role denial, crash/restart, dump/restore, guarded
  down/up and caller-policy regression gates.
- Integrate G21.6 into Phase 0 and full standalone verification while retaining
  all G21.0-G21.5 source gates and the expected local
  `ISOLATION_UNAVAILABLE` probe.
- Synchronize architecture, contracts, deployment, backup/restore, G20/G21
  tracking, process record, migration docs and Trellis backend/frontend/
  operations specs.

## Acceptance criteria

- [x] Only an authenticated opted-in, selected user under a current exact
      activation can append one immutable fixed-plan request; browser/API input
      cannot influence execution authority or payload.
- [x] API, worker and operator roles remain disjoint; the thirteenth LOGIN has
      exactly the three intended memberships and no direct DML/elevated role.
- [x] The seventh execution caller has only lifecycle methods and every earlier
      caller/relay policy is unchanged.
- [x] Worker crash/restart/replay produces one request, one idempotent Run chain,
      one terminal receipt and zero Runner residue.
- [x] Shadow/UI remain held without activation and expose the canary action only
      when `effective=true`; this host still reports
      `ISOLATION_UNAVAILABLE`.
- [x] Closure binds migration 095, all prior activation evidence and the exact
      product receipt; stale/drifted/incomplete/residual/template evidence
      cannot return `PROMOTION_READY`.
- [x] PostgreSQL dump/restore and guarded clean down/up pass; Phase 0, G21.0-
      G21.6 and full standalone gates pass without touching protected runtime
      state.

## Definition of done

- Source, tests, migration, schemas/fixtures, Compose/example config, deploy
  templates, runbooks, tracking and Trellis specs agree on the bounded product
  canary and final-promotion boundary.
- Verification passes, then work/task/journal commits are created without amend
  or push.

## Technical approach

Migration 095 becomes the authority bridge: migration-090 Shadow eligibility
plus an operator-provisioned bounded activation yields `effective=true`; the
API appends a fixed request; a narrow worker claims it and composes existing
Orchestrator/Runner controls; completion writes a content-free receipt. The
Runner lifecycle implementation is reused through explicit product caller,
actor and reason bindings so G21.1 defaults cannot drift.

The production-closure contract retains its 16 operational checks and gains a
closed activation-chain object plus exact product receipt. A separate immutable
promotion function accepts only the reviewed closure binding after the
read-only evaluator emits `PROMOTION_READY`.

## Decision (ADR-lite)

**Context:** Direct API execution violates least privilege, while requiring the
final closure before the first product canary creates a circular gate.

**Decision:** Use a durable API-to-worker request authority and a two-stage
bounded-activation/final-promotion model. Reuse the signed root-canary execution
mechanics under a new fixed plan and caller instead of opening the generic
Runtime.

**Consequences:** G21.6 provides a real but deliberately narrow product path and
honest final closure. Arbitrary prompts/packages, effects, Scheduler, Learning
and delegation remain separately gated; any release/plan/policy drift requires
a new activation and closure.

## Out of scope

- Arbitrary prompt/arguments, package selection, generic product Agent Runs,
  Broker effects, Provider/model calls, MCP, Egress, Secrets or Child work.
- Enabling generic Scheduler/Learning, automatic Skill install/promotion or
  widening the product cohort beyond the fixed canary budget.
- Live host provisioning, credentials, activation/promotion evidence or edits
  to protected runtime-state paths on this development host.

## Research references

- [`research/product-request-authority.md`](research/product-request-authority.md)
- [`research/product-runner-caller.md`](research/product-runner-caller.md)
- [`research/promotion-circularity.md`](research/promotion-circularity.md)
- [`research/final-operations-matrix.md`](research/final-operations-matrix.md)
