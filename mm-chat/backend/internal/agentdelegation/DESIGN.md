# Child delegation design

## Authority

PostgreSQL migration `087` owns immutable root/child authority, exact Parent
Attempt bindings, reservations, settlements, and reap work. Go derives the
proposal, but the database repeats subset, lease, Kill Switch, and budget checks
inside the Child enqueue transaction.

The authenticated control identity is a separate `UserID` input; subject fields
inside the proposed Grant are never treated as caller authority. Root
registration and Child lookup bind that identity to the durable Run owner.

## Depth and subsets

Only `depth=0 -> depth=1` is valid. The Child uses the Parent model, subject,
package, and runtime. Grant capabilities, selectors, approval, Egress, Secrets,
expiration, and budgets can only narrow. The shared Broker Registry builder
physically removes delegation and management identities/capabilities before
fingerprinting. A reused Tool identity must also preserve the Parent capability,
classification and idempotency class while narrowing actions, resource
selectors, approval and call limits. PostgreSQL repeats the same Registry
projection check. Prefix containment uses literal `starts_with`, not SQL `LIKE`,
so `_` and `%` cannot become wildcard authority.

## Cancellation

Cascade first terminally fences live Child Attempts in PostgreSQL and creates
durable, exact reap targets, then terminalizes remaining Steps and the Run. The
injected reaper destroys the exact Sandbox. A failure remains `failed` and is
retried by reconciliation; it never restores a lease. Reconciliation also
discovers terminal, expired, reclaimed or Kill-Switched Parents and invokes the
same atomic cascade before retrying pending work.

Terminal budget settlement is separate from Sandbox reaping. It requires an
already terminal Child Run, accepts one exact outcome/usage fact, rejects usage
above the reservation and releases only the proven reservation remainder.

## Held boundary

No public route, production worker, mTLS relay, Compose service, or live host is
enabled. `neo-runnerd` keeps no database or secret credentials.

Deterministic PostgreSQL `md5` values are local event and reap identifiers only;
they are not credentials, fingerprints, signatures, lease tokens or
authorization proofs. All authority fingerprints remain SHA-256.
