# Final operations matrix research

## Existing closure contract

G20.10 already defines 16 content-free live checks: exact-host isolation,
clean-copy install, restart, reboot, paired backup/restore, DR,
rollback/forward-fix, hierarchical Kill Switch, credential/mTLS and Runtime
rotation, orphan reconciliation, `outcome_unknown`, metrics/alerts,
capacity/budgets, bounded canary and temporary evidence cleanup. Its evaluator
derives `PROMOTION_READY`; input cannot assert the verdict.

## G21.6 additions

- Advance every release binding and guarded migration-tail drill to migration
  head 095.
- Add a required activation-chain object containing content-free fingerprints
  for control plane, Root, Broker/Artifact, Project mutation, Child, Cron and
  Draft-learning evidence. Each must be fresh and for the same exact release.
- Add a required product-canary receipt binding the activation, request, Run,
  plan and terminal receipt fingerprints without user ID, prompt, path or
  content.
- Extend cleanup with temporary product request/claim/receipt counts. All are
  zero after closure while the sanitized promotion record and immutable audit
  facts remain.
- Add alerts/metrics for product request queue, stale claims, failures and
  canary budget. Labels stay within the existing content-free allowlist.

## Verification strategy

- Offline fixtures cover held template, structurally valid synthetic-ready
  template, chain drift, stale evidence, request/receipt mismatch, residue,
  policy drift and malformed data. None is live promotion evidence.
- PostgreSQL 17 gates prove API/worker/operator role separation, deterministic
  cohort selection, request replay, claim expiry/reconcile, exact Run binding,
  promotion immutability, dump/restore and guarded down/up.
- Source gates prove earlier Runner caller policies are byte-for-byte
  unchanged, the new profile is default-off, the API has no execution
  authority, and protected runtime paths are not read.
- Exact-host runbooks own clean-copy/restart/reboot, paired backup/restore, DR,
  rollback/forward-fix, rotations, alerts, capacity and final cleanup. The
  current development host continues to emit `ISOLATION_UNAVAILABLE`.

