# Depth, Budget and Cascade Authority

## Comparable patterns

- Capability delegation narrows an immutable Parent grant rather than copying
  ambient caller permissions.
- Hierarchical schedulers reserve Parent quota transactionally before creating
  work, then settle actual usage; local counters alone permit oversubscription.
- Structured-concurrency cancellation invalidates descendants before waiting
  for process cleanup. Cleanup failure remains durable and retryable rather than
  resurrecting authority.
- Lease-based workers reject stale generations at every result/side-effect
  boundary; lineage does not replace lease fencing.

## Mapping to Neo Chat

- Store root/parent/depth and normalized Grant/Registry/model/package/runtime in
  an immutable migration-`087` authority row.
- Lock one Parent budget account and reserve all four dimensions in the same
  transaction as `agent_orchestrator_enqueue_run`.
- Bind Child authority to the exact Parent Attempt generation/owner/token digest
  and recheck it at launch admission.
- Fence durable Child Attempts first, append terminal events, enqueue exact reap
  targets second, and only then call an injected Runner reaper.

## Rejected alternatives

- JSON-only Client snapshot containment: aliases and selector-prefix semantics
  are too easy to widen.
- In-memory budget counters: unsafe under restart and concurrent delegation.
- Runner database lookup: violates the credential-free Runner boundary.
- Recursive generic lineage: unnecessary attack surface; G20 is explicitly
  limited to depth 1.
