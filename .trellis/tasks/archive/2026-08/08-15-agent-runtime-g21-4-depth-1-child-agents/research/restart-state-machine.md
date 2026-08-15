# G21.4 restart state-machine research

## Stable identities

The Root Run uses the plan idempotency key. Child enqueue uses a second fixed
idempotency key; its request fingerprint excludes the generated Child Run ID,
so exact replay resolves the same lineage. A controller must query existing
lineage before proposing work under a newer Parent Attempt, otherwise the
durable collision fence correctly rejects the drift.

## Required recovery behavior

- Before new work, reconcile expired/terminal/Kill-Switched Parents and retry
  pending Child reaps.
- Once any Child lineage exists, never enqueue another Child for this canary.
- If the controller lost Parent or Child lease tokens, wait for lease expiry.
  Reconciliation kills/reaps the Child first; the Root Step may then be
  reacquired solely to cancel and terminalize the Parent.
- Crash after Child cascade performs only Child reap completion and Parent
  cleanup.
- Crash after signed Parent cancel may relaunch only the Parent generation
  needed for cleanup; it never creates a replacement Child.
- Health requires terminal Parent/Child authority, zero pending reaps and empty
  Runner inventory.

## Terminal proof

Successful completion leaves one immutable lineage/reservation/settlement or
cascade fact, terminal Runner projections and no live Sandboxes. Any conflict,
multiple Child lineages or unknown Runner identity fails closed instead of
guessing or issuing another delegation.

