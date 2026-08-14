# G20.5 Child Delegation — Technical Design

## Decision

Add `internal/agentdelegation` plus migration `087`. The Backend derives Child
authority from an immutable registered root authority; PostgreSQL repeats the
decisive Parent/lease/subset/budget checks and atomically creates the Child Run
and reservation. Runner receives only an exact signed lineage projection.

## Data flow

```text
registered depth-0 Run + exact live Parent Attempt
  -> load immutable Parent authority
  -> derive narrowed Child Grant
  -> build depth-1 Registry with forbidden set physically removed
  -> canonical Child snapshot/fingerprints
  -> PostgreSQL Parent lock + subset/Kill/lease/budget recheck
  -> Child Run + lineage + reservation in one transaction
  -> launch admission rechecks Parent and Child leases/fingerprints
  -> signed Runner lineage projection
```

## Cancel/kill flow

```text
Parent cancel/kill request
  -> PostgreSQL lock Parent and live Children
  -> terminally fence Child Attempts/Steps/Runs
  -> enqueue exact reap targets
  -> injected Runner reaper
  -> mark reaped, or retain failed/pending work for reconciliation
  -> Parent transition may proceed
```

## Privilege model

```text
agent_delegation_owner    NOLOGIN; owns migration 087 objects
agent_delegation_control  NOLOGIN; SELECT + exact function EXECUTE only
agent_orchestrator_runtime / agent_effect_control / agent_runner_control
                           no delegation table DML
neo-runnerd               no DB/Grant/Secret authority
```

## Rollback

- Keep migration `087` inert when production wiring is absent.
- Down refuses while any delegation authority/reservation/reap fact exists.
- Roll back Backend/Runner artifacts without deleting live authority; cleanup
  and reconciliation remain required before a guarded schema down.
