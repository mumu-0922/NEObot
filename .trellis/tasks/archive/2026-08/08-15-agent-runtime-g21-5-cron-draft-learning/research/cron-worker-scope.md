# G21.5 Cron worker scope research

## Question

How can a production Cron worker claim only the reviewed synthetic activation
Template instead of every active Template created by migration 088?

## Repository findings

- `agentcron.Service.RunCycle` is already a complete durable loop: claim due
  cursors, materialize UTC occurrences, advance the cursor, claim triggers,
  enqueue ordinary Runs, release retryable failures, reconcile and prune.
- `agent_cron_claim_due` and `agent_cron_claim_triggers` in migration 088 select
  globally from every eligible active Template/Trigger. They accept only owner,
  time, lease and limit; a worker using them cannot enforce a cohort.
- `agent_cron_reconcile` and `agent_cron_prune` are global too. A supposedly
  synthetic worker could therefore mutate or delete unrelated Cron state even
  if its primary claims were scoped in Go.
- `agent_cron_control` combines Template CRUD/revocation with worker claims and
  maintenance. Inheriting it violates the least-privilege boundary required by
  an independently deployed worker.
- The existing denial path already rechecks Template revision, installation,
  admission, package, Grant, approval, expiry and hierarchical Kill Switches at
  enqueue. The missing boundary is activation membership, not Run materialization.

## Comparable patterns

- Kubernetes CronJob controllers use label/namespace selection and optimistic
  resource versions rather than trusting an application-side post-filter.
- Temporal schedules and Sidekiq/BullMQ workers separate queue ownership from
  schedule administration and bind workers to named task queues.
- Database job queues commonly place the cohort predicate inside the same
  `FOR UPDATE SKIP LOCKED` statement that acquires the lease; selecting broadly
  and rejecting afterward temporarily steals unrelated work.

## Feasible approaches

### A. Application post-filter

Claim globally through migration 088, discard non-plan rows in Go, and release
those claims.

Rejected: it steals unrelated leases, advances retry counters under failure and
leaves global reconcile/prune authority intact.

### B. Caller-supplied Template allowlist

Add wrapper functions that accept Template IDs and filter inside SQL.

Rejected: a compromised worker role can supply another valid Template ID. The
plan is only a configuration convention, not database authority.

### C. Operator-provisioned activation target plus scoped wrappers (recommended)

Migration 094 adds an immutable activation target binding activation ID,
Template ID, revision and revision fingerprint. A non-login worker role may
execute only target-scoped claim/advance/enqueue/release/reconcile/prune
wrappers. Provisioning remains migrator/operator-only. Every wrapper rechecks
the target, and claims apply the target predicate in the locking query.

This preserves migration 088, prevents unrelated lease capture, supports exact
rollback by disabling one activation target, and leaves product cohorts to
G21.6.

## Required proof

- Two due Templates: the worker can claim only the bound one.
- Revision/fingerprint drift, disabled/expired target and forged activation ID
  fail before claim.
- DST, missed-run, overlap, retry, approval/Grant/package revocation and Kill
  Switch behavior remain unchanged for the bound Template.
- Reconcile and prune cannot affect an unrelated Template.
- The worker role has no Template CRUD/revoke, direct table DML, owner
  membership or activation-provision authority.
