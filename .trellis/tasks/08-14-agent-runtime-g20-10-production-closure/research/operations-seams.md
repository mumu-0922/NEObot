# G20.10 operations seam inventory

## Existing durable authority

- Migration `084` owns append-only hierarchical Kill Switch revisions,
  Run/Step/Attempt/event authority, recovery inventory, projection rebuild and
  bounded terminal-Run pruning.
- Migration `085` owns replay-fenced Runner requests, expected Sandbox
  projection, recovery inventory and bounded completed-request/Sandbox pruning.
- Migration `086` owns immutable Prepare/Commit intents, approvals,
  cancellations, receipts, active Secret-handle digests, expiry and committing
  reconciliation. A possible send with no exact receipt becomes terminal
  `outcome_unknown` and must not be retried.
- Migration `087` owns Parent/Child lineage, reservations, settlement, cascade
  and durable reap work. Reconciliation remains callable while execution is
  disabled.
- Migrations `088`-`090` own Cron reconcile/prune, Draft object-before-row
  cleanup/prune, Agent Center projections, Artifact authority and default-off
  Shadow observations.

## Existing operational artifacts

- `scripts/verify-agent-runner-host.sh` is the only exact-host prerequisite
  gate. The checked-in example manifest is deliberately unapproved and the
  current host must return `ISOLATION_UNAVAILABLE`.
- `scripts/backup-single-server-production.sh` already creates one paired,
  checksum-bound PostgreSQL/MinIO set manifest.
- `docs/deployment/backup-restore.md` already requires restore with workers
  stopped and preserves deletion replay authority.
- Every Agent slice has focused source and disposable PostgreSQL 17 gates, but
  no aggregate production promotion record currently binds those results to
  one release/Runner/runtime/policy tuple.

## Gaps to close without enabling Runtime

1. No strict, content-free record proves the full live matrix belongs to the
   exact same immutable release.
2. Capacity/budget/retention defaults and alert thresholds are described only
   indirectly, not frozen in a reviewable operations policy.
3. On-call actions for isolation drift, orphan cleanup, secret/runtime
   rotation and `outcome_unknown` need a single fail-closed runbook.
4. Temporary canary/Draft/Artifact/Run cleanup needs an explicit zero-count
   promotion condition while preserving the content-free promotion record.

## Implementation consequence

G20.10 should add no application startup worker or production execution path on
this host. It should add a strict operations policy, production-closure evidence
schema, validator, fixtures, offline self-test and synchronized runbooks. The
validator must be able to reject/hold a record without accessing live services
or protected runtime paths.
