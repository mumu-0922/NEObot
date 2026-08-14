# Agent Cron design

## Goals and non-goals

The module provides a restart-safe scheduling control plane before any public or
production Scheduler exists. Its goals are immutable approval-bound revisions,
strict timezone-aware schedule calculation, durable claims, bounded backlog and
retry behavior, trigger-time authority revalidation, atomic normal-Run enqueue,
sanitized audit, and guarded cleanup.

It does not expose Cron CRUD, wire a startup loop, execute a Run, resolve Secret
plaintext, perform an effect, or promote the Runner. It also does not support
automatic replace/cancel overlap, unbounded buffering, surprise backfill, or
Child Cron.

## Authority and data flow

```text
approved immutable revision
  -> exact next_trigger_at cursor
  -> owner + generation + expiry cursor claim
  -> bounded Go occurrence calculation
  -> unique UTC occurrence facts + fenced cursor advance
  -> owner + generation + expiry trigger claim
  -> locked trigger-time authority and overlap recheck
  -> atomic normal Orchestrator Run enqueue + trigger link
  -> sanitized terminal audit fact
```

Go owns validation, canonical fingerprints, embedded-IANA timezone calculation,
and bounded occurrence planning. PostgreSQL migration `088` is the only durable
authority for logical templates, revisions, approvals/revocations, exact
cursors, claims, occurrence state, Run links, final denials, audit, and cleanup.
`FOR UPDATE SKIP LOCKED` chooses queue candidates only; the mutation functions
repeat decisive checks while holding the exact rows.

## Key decisions and trade-offs

- **Go calculates, PostgreSQL fences.** Reusing the pinned Cron library and Go
  tzdata avoids an extension-specific SQL calculator; the cost is an explicit
  bounded Go/PostgreSQL handoff that both sides validate.
- **Immutable revisions over mutable schedules.** Approval and replay remain
  explainable, at the cost of retaining revision history until cleanup.
- **One buffered occurrence over BufferAll.** This bounds outage/overlap load
  and storage, at the cost of deliberately skipping excess occurrences.
- **Normal Orchestrator Runs over a parallel executor.** Cron inherits existing
  Run idempotency and effect fences, while production activation must wait for
  the same Runtime promotion gates.

## Frozen revision and approval

A logical template contains only lifecycle, the active revision, its cursor, and
lease state. Changing owner, input reference/fingerprint, schedule/timezone,
model, budget, Skill installation/admission/package/runtime, Grant and Registry
fingerprints, capabilities, Egress, Secret references, expiry, steps, scopes, or
policies creates a new revision and approval.

The approval class is either `automation_read_only` or
`automation_brokered_effect`. The latter authorizes Scheduler Run creation only;
it never bypasses migration `086`'s per-effect Prepare/Commit authority.
Approval and Grant revocations are append-only.

## Time and occurrence identity

The pinned parser is `github.com/robfig/cron/v3 v3.0.1` with exactly five
fields. The timezone is a separate canonical IANA name loaded with Go's embedded
`time/tzdata`. A spring-forward nonexistent wall time is skipped; both UTC
instants in a fall-back repeated wall time are retained.

The durable identity is the exact template ID, revision, and UTC scheduled
instant. The occurrence fingerprint and Orchestrator idempotency key derive from
that identity, so crash recovery and acknowledgement loss cannot manufacture a
replacement Run.

## Claims, missed work, and overlap

Cursor and trigger claims use owner, monotonic generation, and expiry. A stale
worker cannot advance a cursor, enqueue, release, or terminalize. A crash before
advance lets the same cursor be reclaimed; a crash after materialization finds
the unique existing occurrence; a crash after enqueue finds the same linked
Run.

Missed policies are bounded to `skip`, `fire_once`, and `catch_up`, with a
maximum 24-hour window and at most 100 catch-up Runs. Overlap policies are
`skip`, `buffer_one`, and `allow`. `buffer_one` retains at most one pending
occurrence while an earlier matching scope is live. Retries reuse the exact
occurrence and stop after the frozen attempt bound; they never retry an effect.

Pause blocks future materialization. Resume records a bounded missed-window fact
and moves the cursor to the next future instant. Delete tombstones the template;
bounded prune removes terminal history before removing an eligible tombstone.

## Trigger-time security

Before Run enqueue, PostgreSQL revalidates the current active revision, live
owner, exact admitted/installed Skill package and runtime, unrevoked Grant and
automation approval, expiry, global/scheduler/user/project/skill/Secret Kill
Switch scopes, and overlap. Any mismatch becomes a stable sanitized skipped
reason and never falls forward to the latest Skill, Grant, model, Secret, or
revision.

`agent_cron_control` has SELECT plus only the exact `SECURITY DEFINER` function
executions; it has no table DML. Other runtime roles gain no Cron authority. The
NOLOGIN owner is the only direct cleanup authority, and `agent_cron_prune`
provides its bounded controlled path. The Runner receives no database, vault,
object-store, Provider, or Cron credential.

## Held boundary and rollback

No HTTP route, frontend, Chat path, application startup, Redis wake loop,
Compose service, or production Runner/Broker relay invokes this package.
Runtime and Scheduler remain disabled, while reconciliation and cleanup stay
available to the narrow control plane.

Migration down refuses with `AGENT_CRON_DOWN_DATA_EXISTS` while Cron templates,
revisions, triggers, or audit facts remain. Operators must use lifecycle and
bounded cleanup before a deliberate rollback; protected runtime state is never
deleted to satisfy the guard.
