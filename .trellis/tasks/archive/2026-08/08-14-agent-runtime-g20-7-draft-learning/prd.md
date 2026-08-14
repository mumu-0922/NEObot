# Agent Runtime G20.7 — Draft-only Learning Foundation

## Goal

Implement the held Draft-only learning control foundation: a completed root Run
may propose a bounded, provenance-bound Skill package revision into quarantine;
independent static/isolation/evaluation checks can make it reviewable but never
installable; only an authenticated administrator Promote reruns admission and
creates a new immutable Skill fingerprint. Live Runs, installations, Cron
templates and legacy text Skills remain unchanged.

## Shared baseline

- G20.1–G20.6 foundations remain binding. PostgreSQL is the durable Draft,
  check-claim, decision, promotion-link, cleanup and audit authority; MinIO/S3
  holds quarantined/canonical bytes; Redis is not authority.
- The user approved all recommended choices and direct commits. No further
  confirmation is required and no sub-Agent is used.
- Production Runtime, Runner/Broker relay, Child execution and Scheduler remain
  disabled. Exact-host remains `ISOLATION_UNAVAILABLE`.
- No Draft API/UI/startup/Compose wiring is added in G20.7. G20.8 owns product
  surfaces and shadow execution. Legacy text Skills remain through G20.8 and are
  deleted only by G20.9.

## Requirements

### Immutable quarantined Drafts

- Add isolated `internal/agentlearning` and migration `089`.
- Accept proposals only from a same-user, terminal `succeeded`, depth-0 Run whose
  immutable snapshot binds the exact existing base package fingerprint.
- Rerun `skillsupply.ValidateArchive` before persistence. Require `SKILL.md`, an
  exact Neo Runtime Manifest and at least one bounded test under `tests/`.
- The proposed package fingerprint and version must differ from the base and must
  not already exist. Runtime image/entrypoints/dependencies/capability/Egress/
  Secret/resource authority and `allowed-tools` may not widen or rebind; only the
  package version may change in the manifest authority envelope.
- Store Draft bytes only under a digest-derived `skill-drafts/sha256/` object key.
  Persist fingerprints, sizes, file/test inventory and evidence references only;
  never persist prompt/input bodies, Run output, Secret values, Tool bodies,
  credentials or Workspace bytes.
- Bind a Draft fingerprint to source Run/snapshot, base/proposed package,
  archive/SBOM/runtime, exact tests and bounded provenance. Evidence must contain
  the base package and source Run-event references, and every changed file must
  be mapped to source evidence.

### Check pipeline and safety gates

- Use generation/owner/expiry claims for Draft checking. Expired work is
  reclaimable; stale workers cannot publish, release or terminalize results.
- Require exactly one `static`, `isolation` and `evaluation` result for the same
  immutable Draft generation. Receipts bind artifact/test/suite/evidence
  fingerprints and contain only bounded metrics/reason codes.
- Static checks reject high-confidence prompt override, secret-copy, provenance
  laundering and evaluation-gaming patterns. Isolation and evaluation use
  injected adapters; default production construction is unavailable and never
  executes packages in-process.
- All three passes make a Draft `reviewable`; any policy failure makes it
  `check_failed`. Infrastructure retry is bounded. Failed policy checks cannot be
  rerun in place; a correction creates a new Draft.
- Automated check actors cannot admit, install, Promote, mutate Grant/Secret/
  Runtime authority or alter live state.

### Human review, Promote and cleanup

- Provide in-memory administrator diff data from exact base/Draft archives, with
  bounded text and binary hash/size summaries. Diff content is never durable.
- Reject and Promote require the configured authenticated administrator user,
  expected revision, Draft fingerprint, proposed package fingerprint and a
  bounded reason. Decisions are append-only and exact replay is stable.
- Promote is valid only from `reviewable`, refetches and rehashes Draft bytes,
  reruns full Skill validation and authority-subset checks, rechecks all exact
  check receipts, current source authority and relevant Kill Switches, then
  atomically inserts a new admitted `learning` Skill candidate/package and links
  the Draft. It never edits an existing package/candidate/installation/Run/Cron.
- Store canonical package, SBOM and quarantine replay objects before the database
  promotion transaction. Collision/drift fails closed; no latest-package fallback.
- Rejected/promoted Draft objects enter a bounded owner/generation/expiry cleanup
  queue. Delete object bytes before acknowledging/pruning rows. Learning disabled
  still permits claim reconciliation, audit/read, cleanup and bounded pruning.

### Verification and held boundary

- Add strict `neo-skill-draft.schema.json` plus valid/invalid fixtures and Phase 0
  integration.
- Add unit/race tests for validation, evidence, fingerprint, diff limits, static
  attacks, check claims/results, rejection, Promote replay/drift and cleanup.
- Add migration schema coverage and disposable PostgreSQL 17 fresh/replay,
  least-privilege, source authority, stale claim, check matrix, human-only
  promotion, immutable live-state, cleanup/restart, dump/restore, guarded-down
  and clean down/up proof.
- Add `verify-agent-learning{,-postgres17}.sh`; advance every older PostgreSQL
  tail drill through migration `089` without weakening its original guard.
- Synchronize architecture, contract, deployment, tracking, migration README and
  Trellis Agent Runtime executable specs.

## Acceptance criteria

- [x] Invalid archive/manifest/tests/evidence, non-succeeded/cross-user/Child Run,
      stale snapshot, existing fingerprint or authority widening persists no Draft.
- [x] Prompt injection, secret material, source laundering and evaluation-gaming
      fixtures fail before reviewability and never create a candidate.
- [x] Crash/reclaim and stale check generation produce one immutable check bundle;
      only exact three-pass results reach `reviewable`.
- [x] Automated identities cannot Promote; authenticated administrator stale/
      mismatched Promote fails. Exact replay returns one admitted learning
      candidate with the same new package fingerprint.
- [x] Promotion reruns validation after object fetch and rejects byte/hash/check/
      authority/Kill-Switch drift without mutating base package, installations,
      Run snapshots or Cron revisions.
- [x] Rejection/promotion cleanup is object-before-row, bounded, restart-safe and
      available while Learning/Runtime is disabled.
- [x] Migration `089` passes PostgreSQL 17 replay/authority/claims/checks/promotion/
      cleanup/least-privilege/dump-restore/guarded-down/down-up; old drills return
      to head `089`.
- [x] Focused race/vet/tests, Phase 0, all Agent source/control gates and full
      standalone pass; exact-host remains expected `ISOLATION_UNAVAILABLE`.
- [x] No public/startup/frontend/Chat/Compose enablement or protected runtime/
      legacy text-Skill change occurs.

## Definition of done

- Source, migration, schemas, fixtures, tests, gates and operational/product/
  Trellis documentation are synchronized.
- Security, quality and change verification pass with no secret-bearing evidence.
- Work commit, task archive and journal commits are created without amend or push.

## Technical approach

`agentlearning` validates and fingerprints Draft bytes in Go, stores them in
object quarantine, runs one locally defined static policy plus injected held
isolation/evaluation adapters, and produces bounded review diffs. Migration `089`
owns source-Run authority, immutable Draft/check/decision facts, fenced claims,
atomic Draft-to-admitted-learning-candidate promotion and cleanup claims. Promote
revalidates the exact bytes and writes canonical objects before the PostgreSQL
transaction.

## Decision (ADR-lite)

**Context:** Allowing a successful Agent to rewrite an installed Skill directly
would let untrusted model output expand execution authority and poison future Runs
or Cron templates. Treating evaluator output as admission would also make test
gaming equivalent to privilege escalation.

**Decision:** Use immutable quarantined Drafts, exact source/evidence/test binding,
three independent check classes, no in-place retry after a policy failure, strict
authority non-widening and a separate authenticated human Promote that reruns
admission into a new package fingerprint.

**Consequences:** Learning remains auditable and fail closed without enabling a
production executor. Corrections and authority changes require a new Draft or the
ordinary administrator supply-chain path; G20.8 must add product surfaces without
bypassing this service.

## Out of scope

- Public Draft CRUD/review/Promote API, frontend UI, Chat integration, startup
  learning worker, Redis wake, Compose service or production Draft execution.
- Self-approval, automatic admission/install, evaluator-controlled promotion,
  authority expansion, Child/Cron-generated self-modification or in-place Draft
  editing/rechecking after policy failure.
- Live Provider evaluator, exact-host isolation promotion, mutable executor,
  production shadow/canary, G20.8 UI, G20.9 legacy deletion or G20.10 closure.

## Research references

- [`research/immutable-draft-provenance.md`](research/immutable-draft-provenance.md)
- [`research/evaluation-and-gaming-boundaries.md`](research/evaluation-and-gaming-boundaries.md)
- [`research/human-promotion-and-cleanup.md`](research/human-promotion-and-cleanup.md)
