# G20.6 Cron Foundation — Technical Design

## Decision

Add `internal/agentcron` plus migration `088`. Go strictly parses 5-field Cron
with pinned IANA timezone semantics and computes bounded occurrences. PostgreSQL
owns immutable revision approval, cursor/claim fencing, occurrence identity,
trigger-time authority, normal Run enqueue, audit and cleanup.

## Data flow

```text
approved immutable revision
  -> exact next_trigger_at cursor
  -> due cursor lease claim
  -> bounded Go occurrence planning in frozen timezone
  -> occurrence facts + cursor advance transaction
  -> occurrence lease claim
  -> lifecycle/owner/Skill/Grant/approval/expiry/Kill/overlap recheck
  -> agent_orchestrator_enqueue_run in the same SQL transaction
  -> exact trigger -> Run link, or sanitized skipped/denied audit fact
```

## Lifecycle

```text
create revision 1 + approval -> active
edit frozen field -> revision N + new approval + new cursor
active -> paused -> active (resume skips paused window)
active|paused -> deleted tombstone
retention cleanup -> prune terminal history -> remove expired tombstone
```

## Claim/restart model

```text
claim = owner + generation + expires_at
stale owner/generation -> reject
crash before cursor advance -> same due cursor is reclaimed
crash after occurrence insert -> unique occurrence survives and is reclaimed
crash/ack loss after Run enqueue -> same stable idempotency identity returns Run
```

## Privilege model

```text
agent_cron_owner    NOLOGIN; owns migration 088 objects/functions
agent_cron_control  NOLOGIN; SELECT + exact SECURITY DEFINER execution only
go_api_runtime / agent_orchestrator_runtime / agent_runner_control /
agent_effect_control / agent_delegation_control
                    no Cron table DML or scheduling authority
neo-runnerd         no database, Cron, vault or Provider credential
```

## Held boundary

No HTTP route, frontend, Chat, startup worker, Compose service or production
Runner/Broker relay imports or invokes `agentcron`. The package and migration are
source/control foundations and disposable-PostgreSQL evidence only.

## Rollback

- Keep Scheduler disabled; removing Go/schema/docs artifacts does not alter live
  behavior.
- Migration `088` down refuses while templates/revisions/triggers/audits exist.
- Tombstone/prune through the narrow control functions before a deliberate clean
  down. Never delete protected runtime state to satisfy the guard.
