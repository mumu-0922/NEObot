# agentcronworker design

## Goals

- Drive the proven migration-088 Cron state machine for one G21.5 target.
- Reconcile before claim and bound every batch, lease, poll and retention call.
- Keep target selection in migration-094 `FOR UPDATE SKIP LOCKED` functions.

## Non-goals

- Template administration, product cohorts, Run execution, Runner access, or
  automatic activation.

## Flow

```text
operator target -> migration-094 scoped repository -> reconcile
                                                -> agentcron RunCycle
                                                -> periodic scoped prune
```

`Service` depends on the narrow `Scheduler` interface so ordering and failure
behavior remain unit-testable. A reconciliation error prevents any claim. A
schedule error stops the process, allowing the durable lease to expire and the
next reviewed restart to reconcile it. Pruning is maintenance-only and never
changes a live occurrence.

## Security decisions

- The activation ID is stored in `agentcron.PostgresRepository` and prepended to
  every worker SQL call.
- The worker LOGIN inherits only `agent_cron_worker`; SQL functions rebind the
  exact Template/revision/fingerprint on every mutation.
- The package has no credential, Runner, HTTP, Chat or object-store dependency.

## Known limits

G21.5 intentionally supports a single synthetic activation target. Migration
094 is retained on rollback; product cohort selection belongs to G21.6.

## Change history

- 2026-08-15: initial exact-target Cron worker loop.
