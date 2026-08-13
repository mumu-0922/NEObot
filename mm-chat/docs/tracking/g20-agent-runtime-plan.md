# G20 Neo Agent Runtime Epic Plan

Status: G20.0 Phase 0, G20.1 Skill supply chain, and G20.2 durable Orchestrator
foundation complete; G20.3 not started.

## Locked outcome

Deliver Hermes-class package Skills, durable Agent execution, one-level Child
Agents, Cron and review-gated learning through a Neo-owned Go/PostgreSQL control
plane, host-side non-root `neo-runnerd` and one rootless OCI Sandbox per Run.
Assistant, Skill and Tool remain separate. Current pure-text Skills are hard
deleted only at final cutover and are never migrated or wrapped.

## Execution rule

- One bounded group per tested commit; keep all production switches off until
  that group's promotion gate passes.
- Treat model output, Skill packages, source metadata, Tool results, Workspace
  files, network responses and Draft learning output as untrusted.
- Never substitute rootful Docker, `sudo`, privileged containers, host network,
  an unconfined profile or prompt-only policy for required enforcement.
- Freeze package/runtime/grant/model/budget/lineage per Run. Later updates affect
  later Runs only.
- Keep the current text-Skill path as rollback authority through G20.8; do not
  delete it in an earlier group.
- Cleanup/reconciliation/retention stays active whenever execution is disabled.

## G20.0 — Architecture and executable contracts

Status: complete (2026-08-13).

- Pin Hermes Agent and Agent Skills research evidence.
- Freeze C4/ArchiMate topology, trust boundaries and STRIDE model.
- Define Run/Step/Attempt, lease/recovery, Runner RPC, Manifest, Grant, Egress,
  Secret Broker, storage, Prepare/Commit, depth 1, Cron, Draft learning, Kill
  Switch, acceptance suite and legacy cutover.
- Add Draft 2020-12 schemas, positive/negative fixtures and offline verifier.
- Make no runtime code, migration, host, Compose, browser state or live behavior
  change.

Promotion gate: schemas and fixtures pass, cross-contract invariants and
fail-closed boundaries pass, docs/spec/index references are synchronized.

## G20.1 — Skill supply chain and Store authority

Status: complete (2026-08-13).

- Add server-owned Skill source adapters for official, LobeHub, exact Git commit
  and ZIP upload into no-execute quarantine.
- Validate Agent Skills/Neo Manifest, archive paths/types/limits, immutable
  dependencies, fingerprints and SBOM.
- Add administrator admission/rejection, immutable versions, install/uninstall
  references and source drift handling. Package declarations never grant Tools.
- Keep execution off; admit only synthetic/read-only fixtures initially.

Promotion gate: malicious archive corpus, source-ref drift, fingerprint/SBOM,
admission CAS, ownership, backup/restore and offline replay pass; no candidate
file executes during ingestion.

## G20.2 — Durable Orchestrator foundation

Status: complete (2026-08-13).

- Add PostgreSQL Run/Step/Attempt/event, snapshot, lease and Kill Switch
  authority plus least-privilege roles.
- Implement legal transitions, terminal precedence, lease heartbeat/reclaim,
  idempotent enqueue and restart recovery.
- Reuse G19 sanitized process trace and MCP snapshot patterns without coupling
  Tool grants to Skill installation.
- Redis remains an optional ID-only wake/cancel hint.

Promotion gate: disposable PostgreSQL 17 up/replay/down/up, race/lease/restart,
event projection rebuild, stale Attempt denial, retention and backup/restore
pass while no Sandbox can launch.

## G20.3 — `neo-runnerd` and rootless isolation

- Build a dedicated non-root host daemon, mTLS versioned Runner RPC, replay
  fence and capability probe.
- Pin one approved rootless OCI/runtime/storage/network stack for the target
  deployment and fail closed on feature/version drift.
- Implement fresh per-Run Sandbox, immutable rootfs/package, cgroup v2 limits,
  seccomp, no capabilities/no-new-privileges, exact kill/reap and Scratch cleanup.
- Implement Workspace snapshot input and Artifact Broker without host project
  bind or object-store credentials.

Promotion gate: exact release passes the full Isolation Acceptance Suite,
resource exhaustion, escape negatives, cancel/kill/orphan/reboot cleanup and
clean-host reinstall. Global Runtime stays off until evidence review.

## G20.4 — Tool, Egress, Secret and side effects

- Build Registry from frozen Capability Grant and current Tool authority.
- Add none/allowlist/brokered Egress with DNS/IP/redirect/reconnect/size fences.
- Add action-scoped short-lived Secret Broker handles with zero-persistence
  canaries.
- Implement Prepare/Commit, explicit approvals, idempotency receipts and
  `outcome_unknown` across MCP/external writes and Project patches.

Promotion gate: authorization matrix, SSRF/DNS rebinding, secret zero-leak,
Project CAS, Artifact quarantine and full crash/acknowledgement-loss matrix pass.
Only an official synthetic read-only canary may run before mutable promotion.

## G20.5 — Child Agent depth 1

- Add root-to-child durable lineage, subset snapshot derivation and parent
  budget accounting.
- Physically remove `delegate_task`, Cron/grant/secret/runtime management from
  depth-1 Registry before fingerprinting.
- Enforce depth, Parent binding and subset rules at API, snapshot, Registry,
  Runner launch and database boundaries.
- Cascade cancel/kill and reconcile orphaned Children.

Promotion gate: depth 2, forged Parent, widened model/package/grant/egress/
secret/budget, registry alias and stale Parent attacks all fail before launch;
cancel/kill leaves no descendants.

## G20.6 — Cron durable scheduling

- Add versioned Cron templates with exact schedule/timezone, owner, input,
  model, budgets, package/runtime/Grant fingerprints, Egress and Secret refs.
- Require an automation approval class; recheck current revoke/expiry/Kill
  Switch on every trigger.
- Add missed-run, overlap, retry, pause, delete, audit and cleanup behavior.
- Never auto-expand a template after grant/Skill/model changes.

Promotion gate: timezone/DST, overlap, restart, revoked owner/Skill/Secret,
budget, Kill Switch and stale-template matrices pass with no duplicate effect.

## G20.7 — Draft-only learning

- Allow completed Runs to propose quarantined Draft package revisions with
  bounded evidence and tests.
- Run static/isolation/evaluation checks without making Draft installable.
- Add administrator diff/review/reject/Promote. Promote reruns admission and
  creates a new immutable fingerprint.
- Prove live Runs, installed versions and Cron templates never mutate in place.

Promotion gate: prompt-injection, secret-copy, source laundering, evaluation
gaming, rejected Draft cleanup and human-only Promote tests pass.

## G20.8 — Product UI and shadow execution

- Add separate Skill Store/install/admission views, Run process/approval/cancel,
  Artifact, Child, Cron and Draft review surfaces without merging Assistant/
  Tool administration.
- Run server-only synthetic/read-only shadow/canary cohorts with strict user and
  administrator controls.
- Prove reload, restart, failure, cancellation, screen-reader and mobile paths.
- Inventory exact legacy text-Skill state and prepare destructive cutover drill.

Promotion gate: frontend/backend/full standalone gates, runtime acceptance on
the exact deployment, clean-copy/restart, backup/restore and observed canary
budgets/errors pass. Legacy execution remains authoritative until G20.9.

## G20.9 — Legacy Skill deletion and production cutover

- Freeze then hard delete `installedSkills`, `customSkills`, `activeSkillIds`,
  `skillAutoSelect`, Conversation/Workspace `activeSkills`, browser selection/
  context execution and legacy catalogs/definitions. Do not migrate/wrap data.
- Preserve historical messages but render old `skillInvocations` only as the
  read-only fact “旧版技能已退役”.
- Bump/purge browser persistence and remove old API/schema/code references.
- Switch once to the new Runtime; no dual execute or hidden fallback.

Promotion gate: destructive dry run from verified backup, exact state deletion,
history preservation, zero legacy execution references, full quality/security/
isolation/backup/restore/restart/live canary and rollback rehearsal pass. Record
the rollback window and prefer forward repair afterward.

## G20.10 — Production closure

- Close metrics/alerts, capacity/budget defaults, on-call/incident runbooks,
  retention, backup/restore and disaster recovery.
- Repeat Isolation Acceptance on exact production host/runtime/Runner/bundle.
- Exercise hierarchical Kill Switches, credential/runtime rotation, orphan
  reconciliation and `outcome_unknown` operator workflow.
- Delete temporary canary/Draft/Artifact/Run evidence that is not part of the
  content-free promotion record.

Promotion gate: full clean-copy and live matrix, restart/host reboot, backup
restore, rollback/forward-fix, Kill Switch and sanitized evidence review pass.

## Epic completion definition

- All Skills execute from admitted immutable packages through the Neo Runtime.
- Every Run is durable/recoverable and every effect is grant/lease/idempotency
  fenced.
- Rootless isolation has exact production evidence, not a configuration claim.
- Child depth never exceeds one and Child Registry lacks delegation capability.
- Cron and learning cannot inherit or self-create new authority.
- Legacy text-Skill definitions/state are gone; history retains only retirement
  facts.
- Assistant, Skill and Tool stores, APIs and authorization remain separate.
