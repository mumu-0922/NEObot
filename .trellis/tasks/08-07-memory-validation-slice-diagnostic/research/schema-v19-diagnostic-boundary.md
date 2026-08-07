# Schema-v19 diagnostic boundary

## Decisive retained evidence

The consumed schema-v18 live Validation
`memory-regression-20260806t101512z-a057b161` is immutable and must not be
rerun. Its aggregate-only report proves:

- Candidate Recall@20 was `1.0`.
- Final Recall@5 was `64/65 = 0.9846153846`.
- Current-fact accuracy was `54/55 = 0.9818181818`.
- `mixed_language_entity` and `stable_fact` each had current-fact accuracy
  `9/10 = 0.9`; every other quality and safety gate passed.
- There was exactly one current-fact miss overall. Because the same single miss
  reduced both failed slices, it belonged to their intersection.
- The protected Validation corpus has exactly two cases in that intersection.
  Aggregate-only evidence intentionally cannot identify which one failed.

This narrows the defect to one of two protected Validation shapes and proves it
occurred after Candidate Top 20. It does not distinguish BGE rerank exclusion
from Luna Judge rejection.

## Existing observability gap

`CandidateCalibrationTrace` is process-local and currently retains Candidate
IDs, the union of Provider-sent IDs, final IDs, and final scores. The Recorder
does not preserve separate opaque snapshots for:

1. the BGE reranked set before Luna;
2. the Luna-selected ordinals/IDs before intersection;
3. the final intersection classified against the Golden current fact.

Consequently, another aggregate-only run could reproduce a miss without
locating the responsible stage.

## Safe successor seam

Use only the protected Development split. Do not select or replay either
Validation-intersection case and never inspect Holdout. The Development split
contains:

- `85` cases in the union of `mixed_language_entity` and `stable_fact`;
- `5` cases in their intersection;
- `60` mixed-language cases and `30` stable-fact cases.

A new diagnostic-only lane may retain case-level **opaque** stage identities in
a private mode-`0600` artifact. It must omit query text, Memory text, Provider
request/response bodies, raw errors, credentials, embeddings, and URLs. The
artifact has diagnostic authority only: `policySelected=false`,
`promotionEligible=false`, `releaseEligible=false`.

## Feasible execution plans

### A. Three focused repetitions (recommended)

Run the fixed 85-case Development union three times in one bounded live
schema-v19 attempt (`255` case executions). Classify each expected current fact
as Candidate-present, BGE-present, Luna-selected, and Final-present. Repetition
separates a systematic stage defect from Provider nondeterminism while costing
less than three full Development runs.

### B. One focused repetition

Run 85 cases once. This is fastest, but a clean run cannot distinguish a fixed
implementation from a stochastic non-reproduction.

### C. One full 300-case Development run

This provides unchanged global metrics, but spends most requests outside the
two failed slices and still gives only one sample per relevant case. It is more
appropriate after a concrete fix than before root-cause localization.

## Recommended sequence

1. Add schema-v19 diagnostic identities and private opaque stage traces.
2. Prove a network-free Fake PostgreSQL 17 lifecycle and cleanup.
3. Execute exactly one bounded live diagnostic containing three fixed focused
   repetitions.
4. Classify the root cause without altering v18 or production runtime.
5. Apply one minimal versioned policy/prompt/selection fix only if the evidence
   isolates a correctable cause.
6. Run a new full 300-case Development successor. Only a pass may authorize a
   separately versioned 100-case Validation successor and exact-user canary.
