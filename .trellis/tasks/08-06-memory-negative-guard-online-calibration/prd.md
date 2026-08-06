# Calibrate Memory negative-policy guard online

## Goal

Add a separately versioned Development capture lane for the new deterministic
negative meta-policy guard, prove its lifecycle locally, then run the complete
Development split against the real fixed BGE/Luna Provider pair without using
cost avoidance as a stopping condition.

## What I already know

- The guard and Development-only policy are committed in `022a1168`.
- Production Memory recall remains disabled and production-v1 must not change.
- The owner explicitly authorized direct online Provider testing and does not
  want quota cost to delay Development/live-smoke work.
- A short Fake lifecycle remains mandatory to prevent wasting a live one-run
  authority on wiring or artifact errors.
- The previous schema-v15 Validation authorization is consumed; its report,
  corpus, and policy remain immutable.

## Assumptions (temporary)

- Use only the frozen Development split for quality evidence; do not inspect or
  run Holdout or rerun the consumed Validation split.
- Reuse the active attested Vault-backed BGE/Luna values through new private
  one-run credential files and wipe them on every exit.
- Run the full Development case set rather than a quota-saving sample.

## Open Questions

- None.

## Requirements (evolving)

- Add a distinct capture/profile/report identity for the negative-guard policy.
- Add cost-basis v11 with the schema-v14 maximum request/token ceilings; unused
  authority is allowed and actual attempts must still reconcile exactly.
- Bind the guard version/SHA and new Development policy descriptor hash.
- Preserve existing production and historical Development artifact bytes.
- Prove Fake lifecycle before the live call, then proceed directly online.
- Treat Provider quota as authorized; do not weaken retries, evidence, privacy,
  cleanup, or promotion gates to save cost.
- Keep all runtime Memory flags false and persistent live Memory untouched.

## Acceptance Criteria (evolving)

- [x] New lane rejects production, Validation, and Holdout identities.
- [x] Fake lifecycle completes with zero network and cleans all scoped state.
- [x] Live Development uses the exact attested fixed BGE/Luna pair.
- [x] Guard provenance, attempts, cost, quality, false-injection, and cleanup
      reconcile in retained aggregate artifacts.
- [x] No production policy/flag/data mutation, promotion, Release, or Push.

## Definition of Done

- Focused race tests, all backend tests/vet, and full standalone gate pass.
- Fake lifecycle passes before live execution.
- The live Development artifact is independently verified and summarized.
- Specs/docs record the new lane and immutable result.
- Work is committed only after an explicit commit-plan confirmation.

## Out of Scope

- Holdout, production policy mutation, schema-v15 rerun, promotion, Release,
  deployment, migration, or Memory recall re-enable.
- Treating broad quota approval as authorization for a future one-shot
  Validation or Holdout.

## Technical Notes

- Reuse the schema-v14 serial Provider controller, typed retry taxonomy,
  criteria v3, aggregate report shape, independent credential isolation, and
  cleanup behavior without modifying their historical identities.
- Add optional guard provenance fields with `omitempty` to preserve historical
  profile JSON/hashes.
- Recommended new identities are a distinct capture mode, reader v14, profile
  config v16, calibration report v16, and cost-basis v11. The existing generic
  relevance-run v1 manifest may be reused with the new capture/admission mode.
- Live credentials and Provider responses must never be persisted or printed.

## Research References

- [`research/capture-lane-design.md`](research/capture-lane-design.md) —
  existing reusable chain, provenance constraints, and approach comparison.

## Decision (ADR-lite)

**Context**: The guard must receive real BGE/Luna quality evidence without
mutating schema-v14 Development or schema-v15 Validation history.

**Decision**: Add one distinct full Development lane that changes
only the relevance policy/guard variable, run its mandatory Fake lifecycle,
then immediately spend Provider quota on all 300 Development cases.

**Consequences**: The result is comparable and conclusive but remains
non-promotional. Validation, promotion, and recall re-enable stay blocked.

**Confirmation**: The owner selected Approach A and explicitly authorized
direct online Provider testing without a quota-saving constraint.

## Result

- Fake PostgreSQL 17 lifecycle passed all 300 Development cases with 30 exact
  guard abstentions, zero network, two private artifacts, and zero scoped
  residue.
- Live run `memory-regression-20260806t064355z-65407a6a` completed all 300
  cases through the exact active attested BGE/Luna tuple. False injection fell
  to zero and every safety counter remained zero.
- The immutable report failed: five Judge abstentions plus three terminal
  `PROVIDER_TRANSPORT_FAILED` cases reduced the `preference_instruction` and
  `stable_fact` current-fact slices below criterion. It remains
  `policySelected=false` and `promotionEligible=false`; no rerun or later-stage
  authority was inferred.
- Both credentials, the consumed cost source, comparison snapshots, and all
  scoped runtime objects were destroyed. Forty-three sampled live Memory
  relation counts were unchanged and both runtime Memory flags remained false.
- Final review made report publication fail closed when a guard trace lacks
  completed `NO_CANDIDATES` or the fixed corpus records zero guard abstentions;
  mutation tests, focused race, all backend checks, and standalone full pass.
