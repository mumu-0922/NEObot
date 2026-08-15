# Agent Runtime G21.5 — Exact Cron and Draft-learning workers

## Goal

Activate two independent production-path workers for one operator-reviewed
synthetic Cron Template and one operator-reviewed quarantined Skill Draft.
Prove durable Cron scheduling and rootless Draft isolation/evaluation through
exact activation plans, while preserving human-only Promote, least privilege,
crash recovery and cleanup. Do not enable a product cohort, generic Agent
Runtime/Scheduler/Learning, public/API/Chat execution or production promotion on
this development host.

## Shared baseline

- G20.1-G20.10 and G21.0-G21.4 remain binding. Legacy text Skills stay deleted;
  only admitted Package Skills may execute as product work.
- The user approved all recommended decisions, direct verified commits and no
  ordinary follow-up preference questions. No sub-Agent delegation is used.
- The current development host remains `ISOLATION_UNAVAILABLE`; do not install
  host dependencies, create accounts/sub-IDs/cgroups/systemd units or provision
  live credentials/evidence.
- Never read or modify `.env.single-server`, `data/`, `secrets/` or `backup/`.
- Checked-in evidence is template/synthetic-only and documentation is English.

## Requirements

### Two independent activation stages

- Add strict `cron_worker` and `draft_learning_worker` activation evidence,
  strict plan schemas/loaders, standalone commands and separate default-off
  Compose profiles.
- Add `AGENT_CRON_WORKER_ENABLED` and
  `AGENT_DRAFT_LEARNING_WORKER_ENABLED`; neither implies the other. Keep broad
  Runtime, Scheduler, Learning, Skill install, Broker mutation, delegation,
  public/API/Chat and product cohort flags false.
- Require fresh G21.0-G21.4 readiness and migration head `094` for either stage.
  Cron requires no Runner/object-store credential. Draft learning uses a
  distinct lifecycle-only Runner caller and object-store credential that never
  reaches the Runner or Sandbox.
- Preserve all five earlier Runner callers and both relay routes. Add
  `spiffe://neo-chat/agent-runtime-draft-learning` with only
  `probe/list/reconcile/launch/result/cancel`; no heartbeat, Prepare or Commit.

### Migration 094 and least privilege

- Add `094_agent_cron_learning_activation`; do not rewrite migrations 088/089.
- Add immutable operator-provisioned activation targets binding exact activation
  ID, Template/Draft identity and fingerprints, plan fingerprint and validity.
- Add `agent_cron_worker` and `agent_learning_worker` NOLOGIN roles. Exact-host
  LOGINs inherit exactly one corresponding worker role with no elevated
  attributes, owner/control membership, direct table DML or activation
  provisioning.
- Cron worker functions expose only target-scoped claim/advance/enqueue/release/
  reconcile/prune. Learning worker functions expose only target-scoped check
  claim/complete/release, Runner-check lifecycle, cleanup/reconcile/prune.
- Learning worker has no Propose, diff/review, Reject, Promote or package
  candidate/version insertion authority. Human review remains a separate
  administrator path using existing migration-089 invariants.

### Exact synthetic Cron worker

- Bind one active Template ID, current revision and revision fingerprint. Apply
  activation membership inside every `FOR UPDATE SKIP LOCKED` claim, not as a
  Go post-filter.
- Reuse `agentcron` scheduling and enqueue semantics for timezone/DST, missed
  skip/fire-once/catch-up, overlap skip/buffer/allow, stable UTC occurrences,
  retries, approval/Grant/package revocation, expiry and Kill Switches.
- Scoped reconcile/prune may not mutate another Template or Trigger. Plan drift,
  disabled/expired target or changed Template revision fails before claim.
- Enqueued Cron work remains an ordinary durable Orchestrator Run but G21.5 does
  not start a generic execution worker or admit user Templates.

### Rootless Draft check transport

- Bind one quarantined Draft ID/fingerprint, proposed package/runtime/archive
  fingerprints, pre-staged Workspace ID/fingerprint, fixed checker image/argv,
  empty Tool Registry, no Egress/Secrets and bounded resources/suites.
- Store durable non-Orchestrator Runner-check attempts keyed by activation,
  Draft check generation and kind. A Draft-check lease cannot be used as a
  normal Agent Attempt or to access Broker, Provider, Cron, delegation or
  admission functions.
- Add a read-only Runner `result` RPC. It returns only the exact bounded
  quarantined result artifact for the authenticated Attempt; all earlier
  callers remain unauthorized for that method.
- Isolation and evaluation run in separate fixed rootless Sandboxes on the
  accepted Runner path. Each writes a strict content-free artifact binding
  Draft/generation/kind/package/runtime/archive/workspace/suite, status, reason,
  duration and bounded integer metrics.
- The worker validates and durably records results, cancels/reaps the exact
  Sandbox, then completes migration-089's exact static/isolation/evaluation
  receipt bundle. A deterministic fake is test-only and never production
  evidence.

### Human Promote and cleanup

- Recheck source Run/snapshot/package provenance, unchanged authority, three
  passing receipts and Kill Switch immediately before the separate
  administrator Promote. Learning never edits an installed package in place;
  Promote creates a new immutable candidate/package version.
- Bind activation evidence to a real human decision ID/actor class and promoted
  package fingerprint without giving that credential or authority to the
  worker.
- After human Promote or Reject, the worker performs exact target-scoped
  object-before-row cleanup with bounded retry/reconcile. It cannot claim or
  prune unrelated Drafts/audits.

### Crash, restart and rollback

- Prove crashes before/after Cron cursor claim/materialization/enqueue/release
  and Draft claim/launch/result persistence/Runner cancel/check completion/
  object delete. Restart produces no duplicate occurrence, live check attempt,
  receipt, promotion or cleanup acknowledgement.
- Lost Draft Runner tokens wait for lease/authority expiry; reconciliation
  removes exact stale Sandboxes before replacement work. Health requires no
  stale target claim, no pending Runner check and zero target Runner residue.
- Rollback independently disables/stops one target/profile, leaves migration
  094 applied and preserves audit/cleanup facts. Down is disposable-only and
  guarded against active targets, claims, Runner checks or unresolved cleanup.

### Gates, migration head and documentation

- Add focused Go race/vet, PostgreSQL 17, contract fixture, Compose,
  enabled-preflight, role/credential-absence, crash/restart and scoped-isolation
  gates for both stages.
- Advance production policy and every guarded migration-tail drill from `093`
  to `094`, preserving peel/reapply and final-head behavior.
- Integrate G21.5 into Phase 0 and full standalone verification while retaining
  G21.0-G21.4 regressions and the expected current-host
  `ISOLATION_UNAVAILABLE` result.
- Synchronize architecture, contracts, deployment, backup/restore, G20/G21
  tracking, process record, migration docs and Trellis backend/operations specs.

## Acceptance criteria

- [x] Cron and Draft-learning profiles/flags/LOGINs are independent, default-off
      and cannot enable broad Runtime/Scheduler/Learning or a product cohort.
- [x] Two due Templates and two quarantined Drafts prove each worker can claim,
      reconcile and prune only its exact operator-bound target.
- [x] Cron proves DST/missed/overlap/retry/revocation/Kill behavior without
      duplicate occurrences or unrelated mutations across crash/restart.
- [x] The sixth Runner caller has only probe/list/reconcile/launch/result/cancel;
      earlier callers cannot read result artifacts and all relay policies remain
      unchanged.
- [x] Isolation/evaluation use exact rootless Runner Sandboxes with durable
      Draft-only attempts, strict result bindings, replay-safe cancellation and
      zero residue; no unadmitted Draft becomes an ordinary Agent Run.
- [x] Worker SQL roles have function-only target authority and cannot
      Propose/Reject/Promote/direct-DML; a separate human Promote remains
      required and cleanup is object-before-row.
- [x] Migration 094 proves least privilege, dump/restore and guarded clean
      down/up on PostgreSQL 17; all older migration-tail drills return to 094.
- [x] G21.5 plus G21.0-G21.4 regressions, Phase 0 and full standalone pass; this
      host still reports `ISOLATION_UNAVAILABLE` and protected runtime paths are
      untouched.

## Definition of done

- Source, tests, migration, fixtures, Compose/example configuration, deployment
  bundle templates, docs and Trellis specs agree on the two exact-target worker
  boundaries.
- Verification passes, then work/task/journal commits are created without amend
  or push.

## Technical approach

Create `agentcronworker`/`agent-runtime-cron-worker` and
`agentlearningworker`/`agent-runtime-draft-learning-worker`. Add activation
plan loaders and evidence verification following G21.1-G21.4 conventions.
Migration 094 owns activation targets, narrow roles, scoped wrappers and
Draft-check attempts. Existing `agentcron` and `agentlearning` services receive
activation-scoped repository adapters rather than broad migration-088/089
repositories.

Extend the Runner protocol with a bounded `result` method backed by its existing
attempt-fenced artifact quarantine. The Draft worker uses the standard signed
lifecycle path against a pre-staged exact Workspace and fixed checker image,
then maps strict artifacts into migration-089 receipts. Cron remains Runner-free
and only enqueues the exact scheduled normal Run.

## Decision (ADR-lite)

**Context:** Migration 088 claims all active Templates, migration 089 claims all
quarantined Drafts, and both control roles combine worker with administrative
authority. Learning also lacks production isolation/evaluation adapters, while
ordinary Runner authority would incorrectly require treating a Draft as an
admitted package Run.

**Decision:** Add migration 094 with operator-bound exact targets, function-only
worker roles and durable Draft-only Runner attempts. Deploy Cron and Draft
learning as separate default-off stages. Add a lifecycle `result` RPC for the
Draft caller only, using the existing rootless Runner and artifact intake.

**Consequences:** G21.5 proves production scheduling/check/cleanup seams without
opening a product cohort or autonomous promotion. Exact Workspace staging and
one-Draft/one-Template targeting remain intentional stage limits; G21.6 owns
bounded user eligibility and final closure.

## Out of scope

- Public/API/Chat Agent execution, generic Runtime/Scheduler/Learning, user
  cohort selection or arbitrary existing Templates/Drafts.
- Automatic Skill installation, autonomous/self Promote, in-place package
  mutation or worker-held administrator credentials.
- Generic Draft archive upload to Runner, arbitrary checker images/argv/tests,
  Broker effects, Provider/model calls, MCP, Egress, Secrets or Child work.
- Live host provisioning, live credentials/evidence or protected runtime-state
  changes.

## Research references

- [`research/cron-worker-scope.md`](research/cron-worker-scope.md)
- [`research/learning-worker-authority.md`](research/learning-worker-authority.md)
- [`research/draft-check-transport.md`](research/draft-check-transport.md)
- [`research/activation-rollout.md`](research/activation-rollout.md)
