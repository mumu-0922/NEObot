# Current Memory v2 boundary and rollout choices

## Proven boundary

- The active product still uses v1 memory authority.
- Memory v2 already has fail-closed global and exact-user gates.
- The latest diagnostic isolated the residual defect to stochastic Luna
  selection, not Candidate retrieval, BGE rerank, final hydration, storage, or
  terminal transport failure.
- Existing contracts forbid schema-v21 Validation and canary activation until a
  fresh, full 300-case Development successor passes every unchanged gate.

## Rollout choices

### A. Exact-user canary, then widen (recommended)

Enable the global Tool gate with one exact authenticated UUID, prove admitted
and non-admitted behavior, observe bounded live operation, and widen only after
the smoke gate passes. This uses the existing product contract and gives the
narrowest rollback surface.

### B. Immediate all-user activation

This would require changing the current exact-user admission contract or
populating every user identity at once. It expands cost and regression blast
radius and lacks an existing broad-promotion mechanism. It is not recommended.

## Owner decision

The deployment has only one user. Full launch will therefore use option A's
existing exact-user mechanism for that sole user, but it is the complete
production rollout for the current deployment rather than a partial audience
experiment. No unrestricted admission path will be introduced.

## Failure boundary

Any failed or incomplete Development/Validation run, reconciliation drift,
privacy leak, cost overrun, cleanup residue, runtime health regression, or
persistent-count drift must leave both Memory flags false and the allowlist
empty. Launch rollback is clearing the allowlist first, then setting the global
Tool flag false; v1 remains available throughout.
