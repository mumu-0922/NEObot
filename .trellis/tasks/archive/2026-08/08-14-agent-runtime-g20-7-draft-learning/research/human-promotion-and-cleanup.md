# Human promotion, admission replay and cleanup

## Comparable patterns

- Protected deployment environments require a human approval identity distinct
  from the producer and evaluator identities.
- Artifact promotion reruns validation on bytes fetched from quarantine, then
  publishes canonical package/SBOM objects before committing registry metadata.
- Object-backed deletion uses object-before-row cleanup with durable retry state;
  database truth is not removed before byte deletion is proven.

## Conventions that apply here

- Reject and Promote require an authenticated administrator user, exact Draft
  revision/fingerprints and an append-only decision. Scheduler/model/Runner actors
  are never valid promotion identities.
- Promote refetches Draft bytes, verifies size/hash, reruns canonical Skill
  validation, rechecks unchanged authority manifest plus all three exact passes,
  and atomically creates an admitted `learning` candidate with a new package
  fingerprint.
- Object writes precede the PostgreSQL promotion transaction. Exact replay returns
  the same admission; mismatch is a collision and never falls forward.
- Rejected/promoted Draft quarantine objects enter a bounded, generation-fenced
  cleanup queue. Runtime/Learning disabled still allows reconcile and cleanup.
- Migration down is guarded while Draft facts or promoted `learning` candidates
  exist. Operators may not delete protected runtime state merely to force a
  rollback.

## Repository mapping

- Promotion reuses `skillsupply.ValidateArchive` but does not reuse the existing
  public candidate review route. Migration `089` performs the Draft decision and
  Skill package/candidate insertion in one transaction.
- G20.8 may add the administrator UI/API over this service. G20.7 adds no HTTP,
  Chat, frontend, startup worker, Compose service or production Runtime wiring.

