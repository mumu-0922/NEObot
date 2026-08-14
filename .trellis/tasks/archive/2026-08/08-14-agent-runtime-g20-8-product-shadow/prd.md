# Agent Runtime G20.8 — Product UI and held shadow execution

## Goal

Expose the already durable package-Skill, Run, approval, Artifact, Child, Cron
and learning-Draft control planes through separate authenticated product
surfaces, while adding a default-off synthetic/read-only Shadow policy and a
content-free inventory/dry-run for the approved G20.9 deletion of legacy
pure-text Skills. Legacy execution remains authoritative in G20.8.

## Shared baseline

- G20.1-G20.7 contracts, migrations `083`-`089` and Kill Switch boundaries are
  binding. PostgreSQL/object storage remain authority; Redis is wake-only.
- The user approved all recommended choices and direct commits. No additional
  preference prompt or sub-Agent is required.
- Package Skills and Assistants remain separate products. The current frontend
  Skill panel is legacy pure-text state and is not silently converted.
- Production Runtime, Scheduler and Learning workers remain disabled by
  default. Current exact-host status is `ISOLATION_UNAVAILABLE`; no API-process
  or browser fallback executor is permitted.
- G20.9 owns destructive legacy deletion/cutover. G20.10 owns closure and final
  production promotion.

## Requirements

### Authenticated Agent control API

- Add a narrow G20.8 HTTP/control layer over existing services rather than
  granting frontend/database table access.
- User-owned read models expose Package Skill Store/library detail, Run list/
  detail/events/steps/attempts, bounded Artifact metadata/download links, Child
  tree/settlement, prepared approval facts and Cron templates/revisions.
- User mutations are limited to existing authority transitions: install/
  uninstall, enqueue/cancel a root Run when eligible, decide an explicit
  approval, and create/pause/resume/delete Cron revisions. Every write binds
  user, expected revision/generation and exact fingerprints.
- Administrator-only reads/actions expose candidate admission and learning
  Draft list/detail/check receipts/bounded diff plus Reject/Promote. Never
  expose Draft/package/Run/Tool/Secret bodies through list diagnostics.
- Cancellation and approval remain service/Broker/Orchestrator transitions; no
  handler writes tables directly or claims Runner identity.
- Add migration `090` only for missing bounded read-model/shadow/inventory
  authority. It must be additive, least-privilege, replayable and guarded down.

### Separate Agent Center UI

- Add a top-level URL-addressable Agent Center with Package Skills, Runs,
  Schedules and administrator-only Learning Review views. Do not merge it into
  Assistant Hub, MCP administration or legacy Skill editing.
- Package Skills use the server `/v1/skills/*` Store/library/admission contract
  and display immutable package/runtime fingerprints plus held-runtime state.
- Runs provide reload-safe list/detail, step/attempt timeline, approval cards,
  Artifact metadata, Child hierarchy and explicit cancel/kill status.
- Schedules show immutable revision, approval, next trigger, missed/overlap
  policy and pause/resume/delete without optimistic authority substitution.
- Learning Review shows exact check receipts, ephemeral bounded diff and
  fingerprint/revision-bound Reject/Promote with stale-conflict reload.
- Support desktop list/detail, mobile drill-in, keyboard operation, focus
  restoration, screen-reader names/status announcements, loading/empty/error/
  held states and restart/reload recovery.

### Held Shadow/canary control

- Add default-off administrator policy plus explicit user opt-in. Cohort
  assignment is deterministic and binds policy revision, user, admitted package
  and runtime fingerprints.
- G20.8 modes are `synthetic` and `read_only` only. Effective authority excludes
  writes, Secrets, Egress, delegation, Cron creation and Draft promotion.
- Use injected adapters for synthetic contract replay. Never execute package
  code in the API/browser process; exact-host unavailability returns
  `ISOLATION_UNAVAILABLE` and schedules no executable work.
- Persist content-free, generation-fenced observations only: IDs/fingerprints,
  counts, latency buckets and stable reason codes. Shadow output never enters
  Chat or becomes admission/promotion authority.
- Budget breach, Kill Switch, user opt-out, time expiry, restart or fingerprint
  drift fences new work. Reconcile/retention remain available while disabled.

### Legacy inventory and G20.9 preparation

- Inventory `installedSkills`, `customSkills`, `activeSkillIds` and legacy
  catalog/definition caches locally as counts, normalized IDs, fingerprints,
  invalid/orphan counts and exact storage keys.
- Provide an explicit local backup export and a dry-run deletion plan. No raw
  legacy body is uploaded or logged by inventory.
- Dry-run proves Assistant, MCP, Chat, Conversation and file state are outside
  the deletion set. No legacy state is deleted and no package is auto-installed
  in G20.8.

## Acceptance criteria

- [x] Cross-user/non-admin reads and writes fail; stale revision/fingerprint/
      generation mutations change no durable authority.
- [x] Agent Center Package Skills, Runs, Schedules and Learning Review remain
      distinct from Assistant/MCP/legacy Skill state and survive reload/restart.
- [x] Approval/cancel/Run/Cron/Draft actions call existing service transitions,
      are exact-replay safe and expose only sanitized errors.
- [x] Mobile, keyboard and screen-reader tests cover navigation, status, focus,
      approval, cancellation, held Runtime and stale-conflict reload.
- [x] Shadow defaults off, requires admin policy plus user opt-in, admits only
      synthetic/read-only authority and stores no prompt/output/Tool/Secret body.
- [x] Exact-host unavailable schedules no executable Shadow work and returns
      stable `ISOLATION_UNAVAILABLE`; no in-process/browser fallback exists.
- [x] Budget/Kill/opt-out/restart/drift fences stale Shadow generations and
      cleanup/reconcile work while execution is disabled.
- [x] Legacy inventory and backup/dry-run are deterministic and delete nothing;
      unrelated browser/server state is byte-unchanged.
- [x] Migration `090` fresh/replay/least-privilege/dump-restore/guarded-down and
      clean down/up pass if persistence is added; older drills return to head.
- [x] Frontend/backend/race/vet/Phase 0/security/accessibility/full standalone
      gates pass; exact deployment acceptance remains an explicit promotion
      prerequisite rather than being forged.

## Definition of done

- Backend DTOs/handlers/read models, frontend API client/Agent Center, Shadow
  policy/observations, legacy inventory, tests, gates and docs are synchronized.
- A clean restart/reload and backup/restore drill reproduces exact control state.
- Work commit, task archive and journal commits are created without amend/push.

## Technical approach

Add an isolated `agentcontrol` HTTP/read facade that composes existing
Orchestrator/Broker/delegation/Cron/learning/supply services, plus additive
PostgreSQL read/shadow functions only where the services lack safe projections.
Frontend adds one Agent Center domain/client with runtime Zod validation and
URL state. Shadow is a held service with injected synthetic adapters and no
startup execution until configuration and exact-host gates both pass.

## Decision (ADR-lite)

**Decision:** Ship one separate Agent Center, server-authoritative writes,
default-off synthetic/read-only Shadow and inventory-only legacy preparation.
Do not merge package Skills with Assistants or legacy text Skills, and do not
fake executable canary evidence while exact-host isolation is unavailable.

**Consequences:** Users gain honest control/review surfaces before cutover;
production authority does not change. G20.9 can delete legacy state from a
verified manifest, while executable Shadow/production promotion remains gated
by exact deployment evidence.

## Out of scope

- Deleting legacy pure-text Skills or changing current Chat Skill invocation.
- Automatic legacy-to-package conversion or name-based package installation.
- Rootful Docker, host execution, browser execution, in-process package code,
  unrestricted canary, automatic evaluator promotion or Child depth above one.
- Assistant/MCP redesign, public anonymous control routes or production flag
  activation without exact-host/backup/canary evidence.

## Research references

- [`research/product-surface-boundaries.md`](research/product-surface-boundaries.md)
- [`research/shadow-canary-boundaries.md`](research/shadow-canary-boundaries.md)
- [`research/legacy-skill-cutover-inventory.md`](research/legacy-skill-cutover-inventory.md)
