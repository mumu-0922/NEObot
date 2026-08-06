# Negative-guard online capture lane design

## Existing reusable chain

- `internal/memorycapture/transport_stable_memory_judge_development.go`
  owns the last passing Development quality semantics: 300 Development cases,
  serial BGE/Luna execution, two Judge retries, typed failure reconciliation,
  criteria v3, aggregate-only report, and non-promotional owner review.
- `internal/memorycapture/profiles.go` hashes capture mode, reader version,
  policy descriptor, execution policy, corpus inputs, fixed Provider tuple, and
  cost authority before Provider construction.
- `cmd/memory-regression-capture/main.go` and
  `scripts/run-memory-regression.sh` enforce exact capture-mode dispatch,
  independent credential files, split selection, artifact publication, and
  cleanup.
- The new guard policy differs from schema-v14 selection only by its policy
  identity and version/SHA-bound deterministic pre-admission abstention.

## Constraints

- Reusing the existing schema-v14 mode with a policy override would mutate or
  ambiguously reinterpret historical evidence. A new identity is required.
- The schema-v15 production Validation lane and cost-basis v10 are immutable
  and cannot be reused for Development.
- New optional profile fields must use `omitempty` so v3-v15 configuration JSON
  and hashes remain byte-identical.
- The guard can reduce admission/rerank/Judge request counts. Telemetry must
  reconcile logical Judge requests from actual candidate-bearing, non-guarded
  cases rather than assume one Judge call per Development case.
- Fake protocol is lifecycle evidence only. It is short and mandatory before a
  live run because it catches command/profile/report/artifact/cleanup drift
  without consuming the one live authority.

## Feasible approaches

### A. Distinct full Development lane (recommended)

Add a new capture mode, reader/profile/report identity, cost-basis v11, and
aggregate report/manifest. Reuse the proven schema-v14 transport controller and
quality criteria, but bind
`HybridShadowNegativePolicyGuardDevelopmentPolicy()` plus the exact guard
version/SHA. Run Fake once, then all 300 Development cases live.

Benefits: changes one selection variable, yields comparable quality evidence,
and spends quota once on a conclusive run. Cost: broader plumbing and a full
live run.

### B. Override schema-v14 policy in place

Add a CLI switch that substitutes the guard policy inside the historical mode.

Benefit: fewer code changes. Cost: invalid evidence provenance, historical
hash drift risk, and ambiguous artifacts. Reject.

### C. Targeted live sample

Add a small Development-only slice or case selector and call Providers only on
guard-heavy cases.

Benefit: fastest first signal. Cost: no full positive-quality, slice, cost, or
stability gate; a second full run is still required. This saves quota rather
than total engineering time and conflicts with the owner's stated preference.

## Recommendation

Implement Approach A. Use provider quota to remove uncertainty, not to bypass
the short mandatory Fake lifecycle or evidence bindings. Stop after the live
Development report for owner review; a future one-shot Validation remains a
separate task and authorization.
