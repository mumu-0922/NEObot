# G20.2 Durable Orchestrator — Technical Design
## Decision

Build one isolated `agentorchestrator` bounded context backed by migration
`084`. All state changes enter PostgreSQL functions that lock authority rows,
validate exact state/generation/lease/Kill-Switch conditions, append a per-Run
event and update projections in one transaction. No API/startup code imports
the package yet.

## Data flow

```text
trusted internal snapshot builder
  -> canonical snapshot + ordered Step plan
  -> idempotent PostgreSQL enqueue
  -> immutable snapshot / current projections / append-only events
  -> exact Step acquisition + generation lease
  -> heartbeat / Attempt transition under exact credential fence
  -> terminal projection
  -> bounded retention
```

Restart reads the same projections and events. Redis/process state is absent.

## Privilege model

```text
agent_orchestrator_owner   NOLOGIN; owns tables/functions
agent_orchestrator_runtime NOLOGIN; SELECT + exact function EXECUTE only
go_api_runtime             no G20.2 authority (no HTTP exposure)
```

`SECURITY DEFINER` functions pin `search_path` to the migration schema plus
`pg_catalog`/`pg_temp`. Runtime roles have no table DML and cannot own schema,
tables or functions.

## Recovery boundary

- Projection and event append are transactional, so ordinary restart requires
  no replay before reads.
- Recovery inventory marks each current Attempt live or expired.
- Reclaim is acquisition of a Step whose current Attempt expired: append
  expiry, fence old credentials, increment generation, create replacement.
- Rebuild is an explicit repair/restore operation deriving status, generation
  and next sequence from events; opaque live lease tokens remain projection
  secrets and are not event material.

## Rollback

- Keep migration `084` applied when any authority exists; down refuses data
  loss.
- Because no runtime route/process is wired, application rollback simply uses
  the previous backend image while `084` rows remain inert.
- Disposable clean replay deletes fixtures through the retention/fixture
  teardown path, then proves `083 -> 084 -> 083 -> 084`.
