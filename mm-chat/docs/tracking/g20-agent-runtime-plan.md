# G20 Neo Agent Runtime Epic Plan

Status: G20.0 Phase 0, G20.1 Skill supply chain, G20.2 durable Orchestrator,
G20.3 Runner, G20.4 brokered effects, G20.5 depth-1 Child delegation, G20.6
durable Cron scheduling, G20.7 Draft-only learning, and G20.8 Agent Center/
default-off Shadow product control complete. Exact-host isolation and production
Runner/Broker/Child/Scheduler/Learning/Shadow promotion are held.

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

Status: source/control foundation complete (2026-08-13); production isolation
promotion held because the exact-host probe returns `ISOLATION_UNAVAILABLE`.

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

Implemented evidence: TLS 1.3 mTLS RPC/server/client, PostgreSQL plus fsync
replay fences, Runner/lease-owner-bound authority ticket, strict Workspace
rehash, per-Attempt Podman create/inspect/start, exact userns/seccomp/cgroup/
mount/resource checks, bounded tmpfs Scratch, per-Attempt Unix Artifact intake,
wall kill, exact reap/reconcile, migration `085` and disposable PostgreSQL 17
drill. Held evidence: approved release installation and the complete suite as
the exact `neo-runner` service account. No Runtime/API/Chat route is enabled.

## G20.4 — Tool, Egress, Secret and side effects

Status: source/control foundation complete (2026-08-14); production relay,
mutable effects and exact-host promotion held.

- Build Registry from frozen Capability Grant and current Tool authority.
- Add none/allowlist/brokered Egress with DNS/IP/redirect/reconnect/size fences.
- Add action-scoped short-lived Secret Broker handles with zero-persistence
  canaries.
- Implement Prepare/Commit, explicit approvals, idempotency receipts and
  `outcome_unknown` across MCP/external writes and Project patches.
- Serialize authenticated pre-Commit cancellation against Commit on the exact
  durable intent; persist immutable cancellation facts and prove one winner.
- Persist append-only Grant revocation and pair durable handle revocation with
  immediate in-memory Secret byte zeroization.

Promotion gate: authorization matrix, SSRF/DNS rebinding, secret zero-leak,
Project CAS, Artifact quarantine and full crash/acknowledgement-loss matrix pass.
Only an official synthetic read-only canary may run before mutable promotion.

Implemented evidence: deterministic Grant/Tool Registry intersection with
physical depth-1 forbidden-Tool removal; migration `086` immutable intents,
append-only approvals, one-claim receipts, budgets, lease/Kill-Switch fences and
secret-handle digests; shared `internal/safenet`; exact Egress and single-use
Secret brokers; bounded Project patch plus deterministic CAS fake;
object-before-row Artifact seam; mutable MCP possible-send no-retry adapter;
strict Runner Prepare/Commit relay; disposable PostgreSQL 17 replay,
least-privilege, concurrency, guarded down, dump/restore and all older-tail
drills. Held evidence: public Agent API, Chat/frontend/startup wiring,
authenticated production relay, real Project/object/vault adapters, live
mutable executor/canary and target-host isolation. The current host remains
`ISOLATION_UNAVAILABLE`, and legacy text Skills remain untouched through G20.8.

## G20.5 — Child Agent depth 1

Status: source/control foundation complete (2026-08-14); public/startup Child
execution and exact-host promotion held.

- Add root-to-child durable lineage, subset snapshot derivation and parent
  budget accounting.
- Physically remove `delegate_task`, Cron/grant/secret/runtime management from
  depth-1 Registry before fingerprinting.
- Enforce depth, Parent binding and subset rules at API, snapshot, Registry,
  Runner launch and database boundaries.
- Bind authenticated control user separately from proposed Grant subject, and
  preserve Parent Registry capability/classification/idempotency while actions,
  selectors, approval and call limits narrow.
- Cascade cancel/kill and reconcile orphaned Children.

Promotion gate: depth 2, forged Parent, widened model/package/grant/egress/
secret/budget, registry alias and stale Parent attacks all fail before launch;
cancel/kill leaves no descendants.

Implemented evidence: `internal/agentdelegation` server-derived root/child
authority; migration `087` immutable lineage, exact Parent Attempt binding,
transactional four-dimensional reservations, terminal settlements and durable
reaps; Broker identity/capability alias removal; Runner signed `runLineage`;
stale Parent/Child lease, snapshot/Grant/Registry/Kill-Switch launch fences;
Child-first cancel/kill plus terminal/expired/reclaimed/Kill-Switch recovery;
focused race/vet, PostgreSQL 17 concurrency/least-privilege/cascade/recovery/
dump-restore/down-up proof; and every older PostgreSQL tail drill advanced to
`087`. Held evidence: public Agent/delegation API, Chat/frontend/startup wiring,
production Child launch/reaper transport, live canary and target-host isolation.
The current host remains `ISOLATION_UNAVAILABLE`; legacy text Skills remain
untouched through G20.8 and are deleted only by G20.9.

## G20.6 — Cron durable scheduling

Status: source/control foundation complete (2026-08-14); public/startup
Scheduler, Cron API/UI and exact-host Runtime promotion are held.

- Add versioned Cron templates with exact schedule/timezone, owner, input,
  model, budgets, package/runtime/Grant fingerprints, Egress and Secret refs.
- Require an automation approval class; recheck current revoke/expiry/Kill
  Switch on every trigger.
- Add missed-run, overlap, retry, pause, delete, audit and cleanup behavior.
- Never auto-expand a template after grant/Skill/model changes.

Promotion gate: timezone/DST, overlap, restart, revoked owner/Skill/Secret,
budget, Kill Switch and stale-template matrices pass with no duplicate effect.

Implemented evidence: `internal/agentcron` strict five-field
`robfig/cron/v3@v3.0.1` plus embedded-IANA timezone calculation; migration
`088` immutable revisions/automation approvals, exact cursor and trigger claims,
unique UTC occurrences, atomic normal-Run enqueue, trigger-time authority,
sanitized audit and bounded cleanup; strict `neo.cron-template/v1` schema and
fixtures; focused race/vet plus `verify-agent-cron{,-postgres17}.sh` fresh/
replay, concurrency, restart, acknowledgement replay, overlap, revocation,
least-privilege, guarded-down, dump/restore and clean down/up proof. Every older
PostgreSQL tail drill now returns through the current head `094`.

Held evidence: public Cron CRUD/trigger/backfill API, frontend/Chat integration,
startup/Redis Scheduler, production Run execution and exact-host promotion. The
current host remains `ISOLATION_UNAVAILABLE`; pure-text Skills remain untouched
through G20.8 and are deleted only by G20.9.

## G20.7 — Draft-only learning

Status: source/control foundation complete (2026-08-14); public review surfaces,
startup check worker, production isolation/evaluation and exact-host Runtime
promotion are held.

- Allow completed Runs to propose quarantined Draft package revisions with
  bounded evidence and tests.
- Run static/isolation/evaluation checks without making Draft installable.
- Add administrator diff/review/reject/Promote. Promote reruns admission and
  creates a new immutable fingerprint.
- Prove live Runs, installed versions and Cron templates never mutate in place.

Promotion gate: prompt-injection, secret-copy, source laundering, evaluation
gaming, rejected Draft cleanup and human-only Promote tests pass.

Implemented evidence: `internal/agentlearning` immutable archive/evidence/test/
changed-path fingerprints, version-only authority comparison, bounded ephemeral
administrator diff, static prompt/secret/laundering/gaming policy, injected held
isolation/evaluation adapters, human Reject/Promote and collision-safe canonical
object writes; migration `089` same-user succeeded depth-0 source authority,
exact three-check generation claims, append-only decisions/audits, atomic new
`learning` candidate/package admission, object-before-row cleanup and guarded
rollback; strict `neo.skill-draft/v1` schema/fixtures; focused race/vet plus
`verify-agent-learning{,-postgres17}.sh` fresh/replay, least-privilege, stale
claims, decision replay, cleanup, dump/restore and clean down/up proof. Every
older PostgreSQL tail drill returns to head `094`.

Held evidence: Chat wiring, startup/Redis learning worker, live Provider
evaluation, production Draft execution/canary and exact-host isolation. The
authenticated administrator Draft list/detail/diff/Reject/Promote surface is
delivered by G20.8. Learning and Runtime remain disabled; pure-text Skills are
untouched through G20.8 and deleted only by G20.9.

## G20.8 — Product UI and shadow execution

Status: product/control foundation complete (2026-08-14); executable Shadow,
production Runtime/Scheduler/Learning workers and exact-host promotion held.

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

Implemented evidence: `internal/agentcontrol` authenticated user/admin facade;
migration `090` sanitized Run process/Schedule/Draft/Artifact views, exact
cancel/approval/lifecycle/review functions, append-only cancellation and
default-off Shadow policy/opt-in/boot-generation/observation authority; strict
least privilege with no worker DML/claim/lease/Commit authority; top-level
URL-addressable Agent Center with Package Skills, Runs, Schedules and
administrator Learning Review, desktop list/detail, mobile drill-in, keyboard,
focus restoration and live status states; strict typed/Zod server client and
authenticated Artifact stream; deterministic local content-free legacy Skill
inventory, explicit local raw backup and deletion dry-run with no mutation;
`verify-agent-product-shadow{,-postgres17}.sh` plus every older Agent/MCP/
Assistant/Skill PostgreSQL tail drill returning to head `094`.

Held evidence: the current host remains `ISOLATION_UNAVAILABLE`; no package
executes in the API/browser, no executable Shadow job is scheduled, and no
Shadow output enters Chat/admission/promotion. Production promotion still needs
the exact-host Isolation Acceptance Suite, paired backup/restore, clean restart
and observed bounded cohort evidence. G20.8 performs no legacy deletion.

## G20.9 — Legacy Skill deletion and production cutover

Status: source/browser/server cutover complete (2026-08-14); exact-host
production Runtime promotion, live backup/apply/canary and rollback rehearsal
remain held.

- Freeze then hard delete `installedSkills`, `customSkills`, `activeSkillIds`,
  `skillAutoSelect`, Conversation/Workspace `activeSkills`, browser selection/
  context execution and legacy catalogs/definitions. Do not migrate/wrap data.
- Preserve historical messages but render old `skillInvocations` only as the
  read-only fact “旧版技能已退役”.
- Bump/purge browser persistence and remove old API/schema/code references.
- Make admitted Package Runtime the sole eligible Skill path while keeping
  execution held until exact-host acceptance; no dual execute or hidden
  fallback.

Implemented evidence: persistence version `7` with marker-last compensating
localStorage/IndexedDB purge; Settings/Chat normalization and `partialize`
non-resurrection; complete removal of legacy editor/sidebar/URL/composer/
workspace/catalog/service/resolver/prompt injection; history collapse to
`legacySkillRetired: true` and one localized label; Go Conversation
create/update/read stripping; default-dry-run PostgreSQL cutover requiring an
exact count and full-backup SHA-256 fingerprint while deleting only
`metadata.activeSkills`; G20.9 itself added no migration. Focused source and
PostgreSQL 17 gates now finish at the current G21.5 schema head `094`.

Held evidence: this source cutover does not claim a live database was modified
or the production rollback window was exercised. Package Skills are now the
only eligible Skill domain, but no Package executor runs while the exact host
returns `ISOLATION_UNAVAILABLE`. Production still requires G20.8 backup evidence,
live expected-count confirmation, clean restart, full restore/rollback rehearsal
and bounded canary proof.

Promotion gate: destructive dry run from verified backup, exact state deletion,
history preservation, zero legacy execution references, full quality/security/
isolation/backup/restore/restart/live canary and rollback rehearsal pass. Record
the rollback window and prefer forward repair afterward.

## G20.10 — Production closure

Status: source/operations closure complete (2026-08-14); exact-host production
promotion held at `ISOLATION_UNAVAILABLE`.

- Close metrics/alerts, capacity/budget defaults, on-call/incident runbooks,
  retention, backup/restore and disaster recovery.
- Repeat Isolation Acceptance on exact production host/runtime/Runner/bundle.
- Exercise hierarchical Kill Switches, credential/runtime rotation, orphan
  reconciliation and `outcome_unknown` operator workflow.
- Delete temporary canary/Draft/Artifact/Run evidence that is not part of the
  content-free promotion record.

Implemented evidence: conservative versioned single-server policy; strict
production-policy and 16-check closure schemas/fixtures; read-only fail-closed
evaluator with stable ready/held/invalid decisions; offline positive/held/stale/
drift/incomplete/duplicate/residue matrix; and synchronized metrics/alerts,
capacity, retention, backup/restore/DR, Kill Switch, rotation, orphan,
`outcome_unknown` and cleanup runbooks. No migration `091`, worker, production
adapter, Runtime flag or fallback was added.

Held evidence: the checked-in record is `template` evidence and the exact host
remains `ISOLATION_UNAVAILABLE`. No live package/canary, reboot, production
restore/rollback, rotation or cleanup was claimed. Epic production completion
requires an external exact-target `production` record whose policy/release-
bound evaluation returns `PROMOTION_READY`.

Promotion gate: full clean-copy and live matrix, restart/host reboot, backup
restore, rollback/forward-fix, Kill Switch and sanitized evidence review pass.

G21.4 follow-on evidence adds migration `093`, the isolated lifecycle-only
Child caller/controller, strict one-Parent/one-Child plan and production reap
transport without widening the held G20.5 product surface. Its focused source,
restart and PostgreSQL gates pass only as synthetic control evidence; public
delegation remains disabled until exact-host activation.

G21.5 follow-on evidence adds migration `094`, two independent exact-target
worker roles/profiles and a sixth lifecycle-only Runner caller. Cron target
membership is enforced inside locking SQL and introduces no Runner/object
credential. Draft isolation/evaluation uses non-Orchestrator attempts, strict
result artifacts and cancel-before-receipt cleanup while preserving separate
human Promote. The current schema head and every guarded older tail drill now
return to `094`; generic Scheduler/Learning and product cohorts remain disabled.

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
