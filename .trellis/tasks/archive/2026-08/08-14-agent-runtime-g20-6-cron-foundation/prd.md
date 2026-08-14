# Agent Runtime G20.6 — Cron Durable Scheduling Foundation

## Goal

Implement the held Cron scheduling control foundation: immutable approved
template revisions, strict schedule/timezone calculation, PostgreSQL-owned
cursors/claims/occurrences, bounded missed/overlap/retry policies, trigger-time
authority revalidation, audit and cleanup. The slice must create normal durable
Runs without duplicate occurrence effects while all public/startup/production
Scheduler and Agent execution wiring remains disabled.

## Shared baseline

- G20.1–G20.5 source/control foundations remain binding. PostgreSQL 17 is the
  sole durable template, cursor, occurrence, claim, Run, revocation, Kill Switch
  and audit authority; Redis is ID-only hinting at most.
- User already approved all recommended choices and direct commits. No further
  confirmation is required, and no sub-Agent is used.
- Current exact host remains `ISOLATION_UNAVAILABLE`; no source/fake test is
  production Runtime or rootless isolation evidence.
- Public Agent/Cron API, Chat/frontend/startup wiring, production worker and
  production Runner/Broker relay remain absent.
- Pure-text Skills remain untouched through G20.8 and are deleted only by G20.9.

## Requirements

### Immutable versioned templates

- Add an isolated `internal/agentcron` package and migration `088`.
- A logical template owns only lifecycle, current revision, schedule cursor and
  lease state. Every change to a frozen field creates a new immutable revision.
- Freeze exact owner/subject, input ref + fingerprint, 5-field schedule, IANA
  timezone, calculator version, model, four-dimensional budget, exact Skill
  installation/admission/package/runtime, Grant ID/fingerprint, Registry
  fingerprint, Egress, Secret refs, expiry, Step plan/scopes and scheduling
  policies.
- Never persist prompt/input body, Secret value, Workspace bytes, Tool arguments,
  Tool results or credentials in Cron rows, audits or snapshots.
- A revision requires an append-only approval bound to its fingerprint and class
  `automation_read_only` or `automation_brokered_effect`. The latter authorizes
  Run creation only and never bypasses per-effect Prepare/Commit approval.
- Approval revocation is append-only. Grant/Skill/model expansion or replacement
  never mutates or enlarges an existing revision.

### Exact schedule/time semantics

- Pin `robfig/cron/v3@v3.0.1`; accept exactly standard 5-field Cron and reject
  seconds, descriptors and embedded `TZ=`/`CRON_TZ=` prefixes.
- Load an independent IANA timezone from embedded Go tzdata. Store and dedupe
  occurrences by UTC instant.
- Spring-forward nonexistent wall times are skipped; fall-back duplicate wall
  times produce two distinct UTC occurrences. Tests freeze both behaviors.
- PostgreSQL stores the exact next cursor. Go computes bounded next instants;
  stale claim generations cannot advance the cursor.

### Durable missed/overlap/retry/restart flow

- Use short generation/owner/expiry claims for due template cursors and pending
  occurrences. `FOR UPDATE SKIP LOCKED` selects queue candidates only; final
  checks repeat under the exact row locks.
- Occurrence identity is unique on template + revision + scheduled UTC instant.
  The normal Orchestrator Run uses a stable Cron idempotency key derived from the
  same identity, and trigger link + Run enqueue commit atomically.
- Missed policies are bounded `skip`, `fire_once` and `catch_up`, with explicit
  catchup window and `maxCatchupRuns <= 100`; no outage creates an unbounded
  backlog.
- Overlap policies are `skip`, `buffer_one` and `allow`. Do not auto-replace or
  kill a running Run, and do not support unbounded buffer-all.
- Pause stops future materialization. Resume records a sanitized missed-window
  fact and advances to the next future instant; it never surprise-backfills.
  Delete tombstones the template and preserves outstanding/history facts until
  bounded retention cleanup.
- Retry only an occurrence not proven enqueued. It retains the exact occurrence
  and Run idempotency identity, uses bounded attempts/backoff, and never retries
  an effect or creates a replacement Run after enqueue.

### Trigger-time authority and failure facts

- Before every Run enqueue, recheck: active/current template revision; nondeleted
  owner; exact admitted and installed Skill/package/runtime; unrevoked Grant and
  automation approval; expiry; current global/scheduler/user/project/skill/
  Secret Kill Switch scopes; budget and overlap.
- Failure creates a sanitized skipped/denied audit fact with a stable reason. It
  must not fall back to latest Skill, Grant, model, Secret or revision.
- Claim expiry/restart recovery cannot duplicate an occurrence or Run. Old claim
  owner/generation cannot advance, enqueue, release or terminalize.
- Runtime/Scheduler disabled still permits claim reconciliation, exhausted retry
  terminalization, audit access and bounded retention cleanup.

### Verification and held promotion

- Add strict Cron JSON Schema plus valid/invalid fixtures and Phase 0 integration.
- Add unit/race tests for validation, fingerprints, DST, missed policies, stale
  claims, overlap, retry and lifecycle.
- Add migration schema tests and a disposable PostgreSQL 17 fresh/replay,
  concurrency, restart, authority-revocation, overlap, idempotency,
  least-privilege, dump/restore, guarded-down and clean down/up drill.
- Add `verify-agent-cron{,-postgres17}.sh`; advance every older PostgreSQL tail
  drill through migration `088` without weakening its original guard.
- Synchronize architecture, contracts, deployment, tracking, migration README
  and Trellis Agent Runtime specs.

## Acceptance criteria

- [ ] Frozen revision/schema rejects unknown fields, ambiguous Cron syntax,
      invalid timezone/policies/budgets and content/credential-bearing metadata.
- [ ] DST gap and overlap behavior is deterministic; occurrence keys use exact UTC
      instants and two workers cannot materialize the same occurrence twice.
- [ ] Claim crash/restart, stale generation and acknowledgement loss produce at
      most one linked normal Run with the stable Cron idempotency identity.
- [ ] `skip`/`fire_once`/bounded `catch_up`, `skip`/`buffer_one`/`allow`, pause,
      resume, delete and bounded retry/cleanup behaviors are durably audited.
- [ ] Owner deletion, Skill uninstall/rejection/runtime drift, Grant/approval
      revocation, expiry, Secret-scope and scheduler/global/user/project/skill
      Kill Switches all deny before Run enqueue with stable sanitized reasons.
- [ ] Editing or expanding Grant/Skill/model creates a new revision/approval;
      stale claimed revisions fail and old revisions never inherit authority.
- [ ] Migration `088` passes PostgreSQL 17 replay/concurrency/restart/authority/
      overlap/idempotency/least-privilege/dump-restore/guarded-down/down-up proof;
      every old tail drill returns to head `088`.
- [ ] Focused race/vet/tests, Phase 0, all Agent source/control gates and full
      standalone pass; exact-host remains expected `ISOLATION_UNAVAILABLE`.
- [ ] No public/startup/frontend/Chat/Compose enablement and no protected runtime
      state or legacy text-Skill change occurs.

## Deliverables

- `mm-chat/backend/internal/agentcron/` with README, DESIGN, service/repository
  and tests.
- migration `088_agent_cron_foundation.{up,down}.sql` and schema coverage.
- `neo-cron-template.schema.json` plus positive/negative fixtures.
- `mm-chat/scripts/verify-agent-cron{,-postgres17}.sh` and tail-drill updates.
- synchronized Agent Runtime architecture/contracts/deployment/tracking/Trellis
  specs and migration documentation.

## Out of scope

- Public Cron CRUD/trigger/backfill API, frontend UI, Chat integration, startup
  Scheduler, Redis wake wiring or production Run execution.
- Auto-Replace/Cancel overlap, unbounded BufferAll, silent backfill, Child Cron,
  autonomous Grant/Secret/model changes or Cron self-approval.
- Real vault/model availability adapters, live mutable executor/canary, Compose,
  target-host provisioning/certificates or exact-host promotion.
- G20.7 Draft learning, G20.8 product/shadow execution and G20.9 legacy deletion.

## Technical approach

Use `agentcron` as a held scheduling bounded context. Go owns strict Cron parsing,
tzdata and bounded occurrence planning; migration `088` owns immutable revisions,
approval/revocation facts, exact cursors, claims, occurrences, final authority
checks and atomic normal-Run enqueue. This follows the existing
Orchestrator/Broker/Delegation separation without importing Cron into HTTP,
startup or Runner paths.

## Decision (ADR-lite)

**Context:** Process-local timers lose cursor/claim state on restart, cannot
serialize multiple schedulers and can enqueue with stale expanded authority.
Computing Cron in SQL would add extension/timezone coupling, while allowing
embedded timezone or unbounded catchup creates ambiguous or runaway schedules.

**Decision:** Pin strict `robfig/cron/v3` calculation in Go with embedded tzdata,
but persist and fence every cursor/claim/occurrence in PostgreSQL. Use immutable
revision + explicit automation approval, bounded catchup/buffer/retry, and one
atomic occurrence-to-Orchestrator enqueue path.

**Consequences:** The control plane is restart-safe, auditable and fail closed,
while production activation remains held. A future private worker may call this
package; public API/UI, current vault/model resolvers and exact-host promotion
remain separate later gates.

## Research references

- [`research/cron-schedule-timezone-dst.md`](research/cron-schedule-timezone-dst.md)
- [`research/durable-claim-missed-overlap-restart.md`](research/durable-claim-missed-overlap-restart.md)
- [`research/immutable-revision-approval-authority.md`](research/immutable-revision-approval-authority.md)
