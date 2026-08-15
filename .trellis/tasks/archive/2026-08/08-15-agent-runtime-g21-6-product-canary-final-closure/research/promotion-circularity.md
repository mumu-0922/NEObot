# Promotion circularity research

## The cycle

The production-closure record requires a real bounded canary, but opening a
product cohort must itself require current G21.0-G21.5 evidence. Requiring the
final `PROMOTION_READY` record before the first canary would make the release
impossible to complete; allowing Shadow policy alone would bypass the staged
activation chain.

## Recommended two-authority model

1. **Bounded-canary activation** is operator-provisioned after fresh G21.0-
   G21.5 evidence. It binds the exact release, migration 095, policy/plan,
   admission/package/runtime and all seven prior activation evidence
   fingerprints. It permits only the finite eligible cohort and request budget.
2. **Final promotion authority** is append-only and may be recorded only after
   the strict closure evaluator returns `PROMOTION_READY` for the same release,
   policy, activation and product-canary receipt. It binds the closure record
   fingerprint and reviewer decision but cannot rewrite the original canary
   activation.

This is analogous to a deployment pre-authorization followed by an immutable
promotion attestation. The first authority permits gathering the one missing
live fact; the second states that the complete operational matrix passed.

## Fail-closed rules

- Template/offline/disposable evidence can validate schemas and replay
  semantics but cannot create either live authority.
- A stale, disabled, expired or drifted bounded activation immediately makes
  Shadow `effective=false` and prevents new requests. Existing claimed work is
  canceled/reconciled by exact identity.
- Promotion never widens the frozen plan or cohort in-place. A new release,
  package/runtime, policy or plan needs a new activation and closure.
- The database function accepts only a content-free `PROMOTION_READY` binding;
  the operator runbook must invoke the read-only evaluator first. The worker
  and API roles cannot call the promotion function.
- Rollback disables the activation/profile while retaining requests, receipts,
  closure fingerprint and incident/audit facts.

## Rejected alternatives

- Treating `effective=true` as final promotion erases the distinction between
  canary authorization and release closure.
- Trusting a `promotionState` field supplied inside evidence makes the record
  self-asserting.
- Updating the Shadow policy to encode deployment readiness mixes product
  eligibility with exact-host operational authority.

